package store

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/msg.db")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStatusTicks(t *testing.T) {
	s := testStore(t)
	const chat = "628111@s.whatsapp.net"
	put := func(id string) {
		if err := s.PutMessage(Message{ChatJID: chat, ID: id, FromMe: true, TS: time.Now(), Text: "x", Status: StatusPending}); err != nil {
			t.Fatal(err)
		}
	}
	status := func(id string) Status {
		msgs, err := s.Messages(chat, 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range msgs {
			if m.ID == id {
				return m.Status
			}
		}
		t.Fatalf("%s not stored", id)
		return 0
	}
	put("timeout")
	_ = s.SetStatus(chat, "timeout", StatusFailed)
	if got := status("timeout"); got != StatusFailed {
		t.Fatalf("pending → failed: got %v", got)
	}
	_ = s.SetStatusByIDs([]string{"timeout"}, StatusDelivered)
	if got := status("timeout"); got != StatusDelivered {
		t.Errorf("a delivery receipt should clear failed, got %v", got)
	}
	_ = s.SetStatus(chat, "timeout", StatusFailed)
	if got := status("timeout"); got != StatusDelivered {
		t.Errorf("a late error must not undo a delivery, got %v", got)
	}
	_ = s.SetStatusByIDs([]string{"timeout"}, StatusSent)
	if got := status("timeout"); got != StatusDelivered {
		t.Errorf("ticks never go back, got %v", got)
	}

	put("retry")
	_ = s.SetStatus(chat, "retry", StatusFailed)
	_ = s.SetStatus(chat, "retry", StatusRead)
	if got := status("retry"); got != StatusRead {
		t.Errorf("read should clear failed, got %v", got)
	}
}
