package store

import (
	"encoding/json"
	"time"
)

const draftsSchema = `
CREATE TABLE IF NOT EXISTS drafts (
	jid     TEXT PRIMARY KEY,
	data    TEXT NOT NULL,
	updated INTEGER NOT NULL DEFAULT 0
);
`

// Draft is an unsent message left in Compose when you switched to another chat.
type Draft struct {
	Text     string         `json:"text"` // as Compose shows it: mentions as their "@Name" labels
	Mentions []DraftMention `json:"mentions,omitempty"`
	ReplyTo  *Message       `json:"reply_to,omitempty"`
}

type DraftMention struct {
	Label string `json:"label"`
	User  string `json:"user"`
	JID   string `json:"jid"`
}

func (d Draft) Empty() bool { return d.Text == "" && d.ReplyTo == nil }

// SetDraft saves the chat's draft; an empty draft deletes it.
func (s *Store) SetDraft(jid string, d Draft) error {
	if d.Empty() {
		_, err := s.db.Exec(`DELETE FROM drafts WHERE jid = ?`, jid)
		return err
	}
	if d.ReplyTo != nil {
		r := *d.ReplyTo
		r.Media = nil // the thumbnail stays in messages
		d.ReplyTo = &r
	}
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO drafts (jid, data, updated) VALUES (?, ?, ?)
		ON CONFLICT (jid) DO UPDATE SET data = excluded.data, updated = excluded.updated`,
		jid, string(data), time.Now().Unix())
	return err
}

// Drafts returns every saved draft by chat JID.
func (s *Store) Drafts() (map[string]Draft, error) {
	rows, err := s.db.Query(`SELECT jid, data FROM drafts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Draft{}
	for rows.Next() {
		var jid, data string
		if err := rows.Scan(&jid, &data); err != nil {
			return nil, err
		}
		var d Draft
		if json.Unmarshal([]byte(data), &d) == nil && !d.Empty() {
			out[jid] = d
		}
	}
	return out, rows.Err()
}
