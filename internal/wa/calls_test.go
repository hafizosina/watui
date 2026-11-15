package wa

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/zhenzhu/watui/internal/store"
)

func testClient(t *testing.T) *Client {
	t.Helper()
	ctx := context.Background()
	container, err := sqlstore.New(ctx, "sqlite3", "file:"+t.TempDir()+"/wa.db?_foreign_keys=on", waLog.Noop)
	if err != nil {
		t.Fatal(err)
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/msg.db")
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	return New(whatsmeow.NewClient(device, waLog.Noop), st, waLog.Noop)
}

func TestCalls(t *testing.T) {
	c := testClient(t)
	var got []CallEvent
	c.OnCall(func(ev CallEvent) { got = append(got, ev) })
	caller := types.NewJID("628111", types.DefaultUserServer)
	meta := func(id string) types.BasicCallMeta {
		return types.BasicCallMeta{From: caller, CallCreator: caller, CallID: id, Timestamp: time.Now()}
	}
	videoOffer := &waBinary.Node{Tag: "offer", Content: []waBinary.Node{{Tag: "audio"}, {Tag: "video"}}}

	c.handle(&events.CallOffer{BasicCallMeta: meta("A"), Data: videoOffer})
	c.handle(&events.CallTerminate{BasicCallMeta: meta("A")})
	if len(got) != 2 || got[0].Kind != CallRinging || !got[0].Video || got[1].Kind != CallMissed {
		t.Fatalf("unanswered video call: want ringing+missed, got %+v", got)
	}
	msgs, err := c.Store.Messages(caller.String(), 10)
	if err != nil || len(msgs) != 1 || msgs[0].Text != "[missed video call]" || msgs[0].Read {
		t.Fatalf("missed call should be an unread message in the chat, got %+v %v", msgs, err)
	}

	got = nil
	c.handle(&events.CallOffer{BasicCallMeta: meta("B"), Data: &waBinary.Node{Tag: "offer", Content: []waBinary.Node{{Tag: "audio"}}}})
	c.handle(&events.CallAccept{BasicCallMeta: meta("B")})
	c.handle(&events.CallTerminate{BasicCallMeta: meta("B")})
	if len(got) != 3 || got[0].Video || got[1].Kind != CallHandled || got[2].Outcome != OutcomeAnswered {
		t.Fatalf("answered voice call: want ringing+handled+ended(answered), got %+v", got)
	}
	msgs, _ = c.Store.Messages(caller.String(), 10)
	if len(msgs) != 2 || msgs[1].Text != "[voice call]" || !msgs[1].Read {
		t.Fatalf("an answered call should be logged as a read [voice call], got %+v", msgs)
	}

	got = nil
	c.handle(&events.CallOffer{BasicCallMeta: meta("C"), Data: videoOffer})
	c.handle(&events.CallReject{BasicCallMeta: meta("C")})
	c.handle(&events.CallTerminate{BasicCallMeta: meta("C")})
	msgs, _ = c.Store.Messages(caller.String(), 10)
	if len(msgs) != 3 || msgs[2].Text != "[declined video call]" {
		t.Fatalf("a call declined on the phone should be logged as declined, got %+v", msgs)
	}
}

func TestMentionsMe(t *testing.T) {
	c := testClient(t)
	own := types.NewJID("6281200000001", types.DefaultUserServer)
	c.WA.Store.ID = &types.JID{User: own.User, Device: 3, Server: types.DefaultUserServer}
	if !c.mentionsMe(context.Background(), []string{"6289999@s.whatsapp.net", own.String()}) {
		t.Error("own number in mentions should count")
	}
	if c.mentionsMe(context.Background(), []string{"6289999@s.whatsapp.net"}) {
		t.Error("someone else's mention must not count")
	}
}
