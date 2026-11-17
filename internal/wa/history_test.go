package wa

import (
	"context"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"google.golang.org/protobuf/proto"
)

func TestHistorySameSecondKeepsOrder(t *testing.T) {
	c := testClient(t)
	chat := "628222@s.whatsapp.net"
	sec := uint64(time.Now().Add(-time.Hour).Unix())
	msg := func(id, text string, ts, order uint64) *waHistorySync.HistorySyncMsg {
		return &waHistorySync.HistorySyncMsg{
			MsgOrderID: proto.Uint64(order),
			Message: &waWeb.WebMessageInfo{
				Key:              &waCommon.MessageKey{RemoteJID: proto.String(chat), FromMe: proto.Bool(false), ID: proto.String(id)},
				MessageTimestamp: proto.Uint64(ts),
				Message:          &waE2E.Message{Conversation: proto.String(text)},
			},
		}
	}
	// WhatsApp sends history newest first; three messages share one second
	c.onHistorySync(context.Background(), &waHistorySync.HistorySync{Conversations: []*waHistorySync.Conversation{{
		ID:          proto.String(chat),
		UnreadCount: proto.Uint32(2),
		Messages: []*waHistorySync.HistorySyncMsg{
			msg("m4", "four", sec+1, 4),
			msg("m3", "three", sec, 3),
			msg("m2", "two", sec, 2),
			msg("m1", "one", sec, 1),
		},
	}}})
	msgs, err := c.Store.Messages(chat, 10)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	unread := map[string]bool{}
	for _, m := range msgs {
		got = append(got, m.Text)
		if !m.Read {
			unread[m.ID] = true
		}
	}
	if want := "one two three four"; join(got) != want {
		t.Errorf("order: got %q, want %q", join(got), want)
	}
	if !unread["m4"] || !unread["m3"] || unread["m2"] || unread["m1"] {
		t.Errorf("the newest 2 should be unread, got %v", unread)
	}
	last, ok, err := c.Store.LastMessage(chat)
	if err != nil || !ok || last.ID != "m4" {
		t.Errorf("last message: %+v %v %v", last, ok, err)
	}
}

func join(s []string) string {
	out := ""
	for i, x := range s {
		if i > 0 {
			out += " "
		}
		out += x
	}
	return out
}
