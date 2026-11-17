package wa

import (
	"context"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"github.com/zhenzhu/watui/internal/store"
)

func TestOnlyAuthorsEditOrDelete(t *testing.T) {
	c := testClient(t)
	ctx := context.Background()
	group := types.NewJID("1203630", types.GroupServer)
	sari := types.NewJID("628222", types.DefaultUserServer)
	andi := types.NewJID("628333", types.DefaultUserServer)
	put := func(id string, sender types.JID, fromMe bool) {
		if err := c.Store.PutMessage(store.Message{ChatJID: group.String(), ID: id, SenderJID: sender.String(), FromMe: fromMe, TS: time.Now(), Text: "original"}); err != nil {
			t.Fatal(err)
		}
	}
	put("s1", sari, false)
	put("me1", types.NewJID("999", types.DefaultUserServer), true)

	edit := func(id string, by types.JID, fromMe bool) {
		c.onProtocol(ctx, group, types.MessageInfo{MessageSource: types.MessageSource{Chat: group, Sender: by, IsFromMe: fromMe}},
			&waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(), Key: &waCommon.MessageKey{ID: proto.String(id)},
				EditedMessage: &waE2E.Message{Conversation: proto.String("edited by " + by.User)}})
	}
	text := func(id string) string {
		msgs, err := c.Store.Messages(group.String(), 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range msgs {
			if m.ID == id {
				return m.Text
			}
		}
		return ""
	}

	edit("s1", andi, false)
	if got := text("s1"); got != "original" {
		t.Errorf("someone else edited Sari's message: %q", got)
	}
	edit("me1", andi, false)
	if got := text("me1"); got != "original" {
		t.Errorf("someone else edited my message: %q", got)
	}
	edit("s1", sari, false)
	if got := text("s1"); got != "edited by 628222" {
		t.Errorf("Sari should be able to edit her message, got %q", got)
	}
	edit("me1", c.ownJID(), true)
	if got := text("me1"); got == "original" {
		t.Error("my own edit from another device should apply")
	}
}
