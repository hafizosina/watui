package store

import (
	"database/sql"
	"time"
)

// Receipt kinds, as WhatsApp reports them for messages you sent.
const (
	ReceiptDelivered = "delivered"
	ReceiptRead      = "read"
	ReceiptPlayed    = "played" // voice note / video played
)

const receiptsSchema = `
CREATE TABLE IF NOT EXISTS receipts (
	chat_jid TEXT NOT NULL,
	msg_id   TEXT NOT NULL,
	user_jid TEXT NOT NULL,
	kind     TEXT NOT NULL,
	ts       INTEGER NOT NULL,
	PRIMARY KEY (chat_jid, msg_id, user_jid, kind)
);`

type Receipt struct {
	User string
	Kind string
	TS   time.Time
}

// PutReceipt records that user got/read/played the messages; the first time counts.
func (s *Store) PutReceipt(chatJID string, ids []string, user, kind string, ts time.Time) error {
	for _, id := range ids {
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO receipts (chat_jid, msg_id, user_jid, kind, ts) VALUES (?, ?, ?, ?, ?)`,
			chatJID, id, user, kind, ts.Unix()); err != nil {
			return err
		}
	}
	return nil
}

// Receipts lists who got, read and played a message you sent, oldest first.
func (s *Store) Receipts(chatJID, id string) ([]Receipt, error) {
	rows, err := s.db.Query(`SELECT user_jid, kind, ts FROM receipts WHERE chat_jid = ? AND msg_id = ? ORDER BY ts`, chatJID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Receipt
	for rows.Next() {
		var r Receipt
		var ts int64
		if err := rows.Scan(&r.User, &r.Kind, &ts); err != nil {
			return nil, err
		}
		r.TS = time.Unix(ts, 0)
		out = append(out, r)
	}
	return out, rows.Err()
}

// MessageTimes: when an incoming message reached this computer and when you read it
// (zero when not recorded: synced from history, or from before watui kept track).
func (s *Store) MessageTimes(chatJID, id string) (received, read time.Time, err error) {
	var rec, rd int64
	err = s.db.QueryRow(`SELECT received_at, read_at FROM messages WHERE chat_jid = ? AND id = ?`, chatJID, id).Scan(&rec, &rd)
	if err == sql.ErrNoRows {
		return time.Time{}, time.Time{}, nil
	}
	if rec > 0 {
		received = time.Unix(rec, 0)
	}
	if rd > 0 {
		read = time.Unix(rd, 0)
	}
	return received, read, err
}

func unixOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}
