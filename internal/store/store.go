package store

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

type Status int

const (
	StatusPending Status = iota
	StatusSent
	StatusDelivered
	StatusRead
	StatusFailed
)

var MutedForever = time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

type Chat struct {
	JID        string
	Name       string
	LastTS     time.Time
	LastText   string
	Unread     int
	Pinned     bool
	Archived   bool
	MutedUntil time.Time
	IsGroup    bool
}

func (c Chat) Muted() bool {
	return c.MutedUntil.After(time.Now())
}

type Message struct {
	ChatJID     string
	ID          string
	SenderJID   string
	SenderName  string
	FromMe      bool
	TS          time.Time
	Text        string
	Status      Status
	Read        bool
	Edited      bool
	Deleted     bool
	MediaType   string
	Media       []byte
	QuoteID     string
	QuoteSender string
	QuoteText   string
	Forwarded   bool
	Starred     bool
	Reactions   map[string]string
}

type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS chats (
	jid         TEXT PRIMARY KEY,
	name        TEXT NOT NULL DEFAULT '',
	last_ts     INTEGER NOT NULL DEFAULT 0,
	pinned      INTEGER NOT NULL DEFAULT 0,
	archived    INTEGER NOT NULL DEFAULT 0,
	muted_until INTEGER NOT NULL DEFAULT 0,
	is_group    INTEGER NOT NULL DEFAULT 0,
	sync_unread INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS messages (
	chat_jid    TEXT NOT NULL,
	id          TEXT NOT NULL,
	sender_jid  TEXT NOT NULL DEFAULT '',
	sender_name TEXT NOT NULL DEFAULT '',
	from_me     INTEGER NOT NULL DEFAULT 0,
	ts          INTEGER NOT NULL,
	text        TEXT NOT NULL DEFAULT '',
	status      INTEGER NOT NULL DEFAULT 0,
	read        INTEGER NOT NULL DEFAULT 0,
	edited      INTEGER NOT NULL DEFAULT 0,
	deleted     INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (chat_jid, id)
);
CREATE INDEX IF NOT EXISTS messages_chat_ts ON messages (chat_jid, ts);
CREATE INDEX IF NOT EXISTS messages_id ON messages (id);
`

func Open(db *sql.DB) (*Store, error) {
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.addColumn("messages", "media_type", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return nil, err
	}
	for _, col := range [][2]string{
		{"media", "BLOB"},
		{"quote_id", "TEXT NOT NULL DEFAULT ''"},
		{"quote_sender", "TEXT NOT NULL DEFAULT ''"},
		{"quote_text", "TEXT NOT NULL DEFAULT ''"},
		{"forwarded", "INTEGER NOT NULL DEFAULT 0"},
		{"starred", "INTEGER NOT NULL DEFAULT 0"},
		{"reactions", "TEXT NOT NULL DEFAULT ''"},
	} {
		if err := s.addColumn("messages", col[0], col[1]); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) addColumn(table, column, def string) error {
	rows, err := s.db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + def)
	return err
}

func (s *Store) EnsureChat(jid string, isGroup bool) error {
	_, err := s.db.Exec(`INSERT INTO chats (jid, is_group) VALUES (?, ?) ON CONFLICT (jid) DO NOTHING`, jid, isGroup)
	return err
}

func (s *Store) SetChatName(jid, name string) error {
	if name == "" {
		return nil
	}
	_, err := s.db.Exec(`UPDATE chats SET name = ? WHERE jid = ?`, name, jid)
	return err
}

func (s *Store) SetChatNameIfEmpty(jid, name string) error {
	if name == "" {
		return nil
	}
	_, err := s.db.Exec(`UPDATE chats SET name = ? WHERE jid = ? AND name = ''`, name, jid)
	return err
}

func (s *Store) SetPinned(jid string, pinned bool) error {
	_, err := s.db.Exec(`UPDATE chats SET pinned = ? WHERE jid = ?`, pinned, jid)
	return err
}

func (s *Store) SetArchived(jid string, archived bool) error {
	_, err := s.db.Exec(`UPDATE chats SET archived = ? WHERE jid = ?`, archived, jid)
	return err
}

func (s *Store) SetMutedUntil(jid string, until time.Time) error {
	_, err := s.db.Exec(`UPDATE chats SET muted_until = ? WHERE jid = ?`, until.Unix(), jid)
	return err
}

func (s *Store) SetSyncUnread(jid string, n int) error {
	_, err := s.db.Exec(`UPDATE chats SET sync_unread = ? WHERE jid = ?`, n, jid)
	return err
}

func (s *Store) TouchChat(jid string, ts time.Time) error {
	_, err := s.db.Exec(`UPDATE chats SET last_ts = MAX(last_ts, ?) WHERE jid = ?`, ts.Unix(), jid)
	return err
}

func (s *Store) PutMessage(m Message) error {
	if err := s.EnsureChat(m.ChatJID, strings.HasSuffix(m.ChatJID, "@g.us")); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		INSERT INTO messages (chat_jid, id, sender_jid, sender_name, from_me, ts, text, status, read, media_type, media,
			quote_id, quote_sender, quote_text, forwarded)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (chat_jid, id) DO UPDATE SET
			quote_id = CASE WHEN excluded.quote_id <> '' THEN excluded.quote_id ELSE messages.quote_id END,
			quote_sender = CASE WHEN excluded.quote_id <> '' THEN excluded.quote_sender ELSE messages.quote_sender END,
			quote_text = CASE WHEN excluded.quote_id <> '' THEN excluded.quote_text ELSE messages.quote_text END,
			forwarded = MAX(messages.forwarded, excluded.forwarded),
			media_type = CASE WHEN excluded.media_type <> '' THEN excluded.media_type ELSE messages.media_type END,
			media = COALESCE(excluded.media, messages.media),
			sender_name = CASE WHEN excluded.sender_name <> '' THEN excluded.sender_name ELSE messages.sender_name END,
			text = CASE WHEN messages.edited = 1 OR messages.deleted = 1 THEN messages.text ELSE excluded.text END,
			status = MAX(messages.status, excluded.status),
			read = MAX(messages.read, excluded.read)`,
		m.ChatJID, m.ID, m.SenderJID, m.SenderName, m.FromMe, m.TS.Unix(), m.Text, m.Status, m.Read, m.MediaType, m.Media,
		m.QuoteID, m.QuoteSender, m.QuoteText, m.Forwarded)
	if err != nil {
		return err
	}
	return s.TouchChat(m.ChatJID, m.TS)
}

func (s *Store) EditMessage(chatJID, id, text string) error {
	_, err := s.db.Exec(`UPDATE messages SET text = ?, edited = 1 WHERE chat_jid = ? AND id = ?`, text, chatJID, id)
	return err
}

func (s *Store) DeleteMessage(chatJID, id string) error {
	_, err := s.db.Exec(`UPDATE messages SET text = '', deleted = 1 WHERE chat_jid = ? AND id = ?`, chatJID, id)
	return err
}

func (s *Store) SetStatus(chatJID, id string, status Status) error {
	_, err := s.db.Exec(`UPDATE messages SET status = ? WHERE chat_jid = ? AND id = ? AND (status < ? OR ? = ?)`,
		status, chatJID, id, status, status, StatusFailed)
	return err
}

func (s *Store) SetStatusByIDs(ids []string, status Status) error {
	for _, id := range ids {
		if _, err := s.db.Exec(`UPDATE messages SET status = ? WHERE id = ? AND from_me = 1 AND status < ?`, status, id, status); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) MarkChatRead(jid string) error {
	if _, err := s.db.Exec(`UPDATE messages SET read = 1 WHERE chat_jid = ? AND read = 0`, jid); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE chats SET sync_unread = 0 WHERE jid = ?`, jid)
	return err
}

func (s *Store) MarkReadByIDs(chatJID string, ids []string) error {
	for _, id := range ids {
		if _, err := s.db.Exec(`UPDATE messages SET read = 1 WHERE chat_jid = ? AND id = ?`, chatJID, id); err != nil {
			return err
		}
	}
	_, err := s.db.Exec(`UPDATE chats SET sync_unread = 0 WHERE jid = ?`, chatJID)
	return err
}

func (s *Store) DeleteChat(jid string) error {
	if _, err := s.db.Exec(`DELETE FROM messages WHERE chat_jid = ?`, jid); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM chats WHERE jid = ?`, jid)
	return err
}

func (s *Store) MarkUnreadByIDs(chatJID string, ids []string) error {
	for _, id := range ids {
		if _, err := s.db.Exec(`UPDATE messages SET read = 0 WHERE chat_jid = ? AND id = ?`, chatJID, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UnreadIncoming(jid string) ([]Message, error) {
	rows, err := s.db.Query(`SELECT id, sender_jid FROM messages WHERE chat_jid = ? AND from_me = 0 AND read = 0`, jid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		m := Message{ChatJID: jid}
		if err := rows.Scan(&m.ID, &m.SenderJID); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) Chats() ([]Chat, error) {
	rows, err := s.db.Query(`
		SELECT c.jid, c.name, c.last_ts, c.pinned, c.archived, c.muted_until, c.is_group,
			MAX(c.sync_unread, (SELECT COUNT(*) FROM messages m WHERE m.chat_jid = c.jid AND m.from_me = 0 AND m.read = 0)),
			COALESCE((SELECT m.text FROM messages m WHERE m.chat_jid = c.jid ORDER BY m.ts DESC LIMIT 1), '')
		FROM chats c
		WHERE c.last_ts > 0
		ORDER BY c.pinned DESC, c.last_ts DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Chat
	for rows.Next() {
		var c Chat
		var lastTS, muted int64
		if err := rows.Scan(&c.JID, &c.Name, &lastTS, &c.Pinned, &c.Archived, &muted, &c.IsGroup, &c.Unread, &c.LastText); err != nil {
			return nil, err
		}
		c.LastTS = time.Unix(lastTS, 0)
		c.MutedUntil = time.Unix(muted, 0)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) Chat(jid string) (Chat, bool, error) {
	var c Chat
	var lastTS, muted int64
	err := s.db.QueryRow(`SELECT jid, name, last_ts, pinned, archived, muted_until, is_group FROM chats WHERE jid = ?`, jid).
		Scan(&c.JID, &c.Name, &lastTS, &c.Pinned, &c.Archived, &muted, &c.IsGroup)
	if err == sql.ErrNoRows {
		return c, false, nil
	}
	if err != nil {
		return c, false, err
	}
	c.LastTS, c.MutedUntil = time.Unix(lastTS, 0), time.Unix(muted, 0)
	return c, true, nil
}

func (s *Store) Messages(jid string, limit int) ([]Message, error) {
	rows, err := s.db.Query(`
		SELECT id, sender_jid, sender_name, from_me, ts, text, status, read, edited, deleted, media_type,
			quote_id, quote_sender, quote_text, forwarded, starred, reactions
		FROM (SELECT rowid AS rid, * FROM messages WHERE chat_jid = ? ORDER BY ts DESC, rowid DESC LIMIT ?)
		ORDER BY ts ASC, rid ASC`, jid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		m := Message{ChatJID: jid}
		var ts int64
		var reactions string
		if err := rows.Scan(&m.ID, &m.SenderJID, &m.SenderName, &m.FromMe, &ts, &m.Text, &m.Status, &m.Read, &m.Edited, &m.Deleted, &m.MediaType,
			&m.QuoteID, &m.QuoteSender, &m.QuoteText, &m.Forwarded, &m.Starred, &reactions); err != nil {
			return nil, err
		}
		if reactions != "" {
			_ = json.Unmarshal([]byte(reactions), &m.Reactions)
		}
		m.TS = time.Unix(ts, 0)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) SetStarred(chatJID, id string, starred bool) error {
	_, err := s.db.Exec(`UPDATE messages SET starred = ? WHERE chat_jid = ? AND id = ?`, starred, chatJID, id)
	return err
}

func (s *Store) SetReaction(chatJID, id, sender, emoji string) error {
	var raw string
	err := s.db.QueryRow(`SELECT reactions FROM messages WHERE chat_jid = ? AND id = ?`, chatJID, id).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	reactions := map[string]string{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &reactions)
	}
	if emoji == "" {
		delete(reactions, sender)
	} else {
		reactions[sender] = emoji
	}
	data, err := json.Marshal(reactions)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE messages SET reactions = ? WHERE chat_jid = ? AND id = ?`, string(data), chatJID, id)
	return err
}

// RecentMedia returns the newest messages of one media kind, across all chats.
func (s *Store) RecentMedia(kind string, limit int) ([]Message, error) {
	rows, err := s.db.Query(`SELECT chat_jid, id, ts FROM messages WHERE media_type = ? AND deleted = 0 AND media IS NOT NULL ORDER BY ts DESC LIMIT ?`, kind, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		var ts int64
		if err := rows.Scan(&m.ChatJID, &m.ID, &ts); err != nil {
			return nil, err
		}
		m.TS = time.Unix(ts, 0)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) Media(chatJID, id string) (string, []byte, error) {
	var kind string
	var data []byte
	err := s.db.QueryRow(`SELECT media_type, media FROM messages WHERE chat_jid = ? AND id = ?`, chatJID, id).Scan(&kind, &data)
	return kind, data, err
}

func (s *Store) LastMessage(jid string) (Message, bool, error) {
	m := Message{ChatJID: jid}
	var ts int64
	err := s.db.QueryRow(`SELECT id, sender_jid, from_me, ts FROM messages WHERE chat_jid = ? ORDER BY ts DESC LIMIT 1`, jid).
		Scan(&m.ID, &m.SenderJID, &m.FromMe, &ts)
	if err == sql.ErrNoRows {
		return m, false, nil
	}
	if err != nil {
		return m, false, err
	}
	m.TS = time.Unix(ts, 0)
	return m, true, nil
}
