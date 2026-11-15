package wa

import (
	"context"
	"time"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"

	"github.com/zhenzhu/watui/internal/store"
)

type CallKind int

const (
	CallRinging CallKind = iota
	CallMissed
	CallHandled
)

type CallEvent struct {
	Kind   CallKind
	CallID string
	From   string
	Chat   string
	Video  bool
	Group  bool
	TS     time.Time
}

type callState struct {
	ev      CallEvent
	handled bool
}

func callText(video bool) string {
	if video {
		return "[missed video call]"
	}
	return "[missed voice call]"
}

func isVideoOffer(data *waBinary.Node) bool {
	if data == nil {
		return false
	}
	_, ok := data.GetOptionalChildByTag("video")
	return ok
}

// OnCall is called when a call rings, is missed, or is picked up elsewhere.
func (c *Client) OnCall(fn func(CallEvent)) {
	c.mu.Lock()
	c.onCall = fn
	c.mu.Unlock()
}

func (c *Client) emitCall(ev CallEvent) {
	c.mu.Lock()
	fn := c.onCall
	c.mu.Unlock()
	if fn != nil {
		fn(ev)
	}
}

func (c *Client) callOffer(ctx context.Context, meta types.BasicCallMeta, video bool) {
	from := c.normalize(ctx, meta.From)
	if !meta.CallCreator.IsEmpty() {
		from = c.normalize(ctx, meta.CallCreator)
	}
	chat := from
	group := !meta.GroupJID.IsEmpty()
	if group {
		chat = meta.GroupJID.ToNonAD()
	}
	ev := CallEvent{Kind: CallRinging, CallID: meta.CallID, From: from.String(), Chat: chat.String(), Video: video, Group: group, TS: meta.Timestamp}
	c.mu.Lock()
	if _, seen := c.calls[meta.CallID]; seen {
		c.mu.Unlock()
		return
	}
	c.calls[meta.CallID] = &callState{ev: ev}
	c.mu.Unlock()
	c.emitCall(ev)
}

func (c *Client) callHandled(id string) {
	c.mu.Lock()
	st, ok := c.calls[id]
	if ok {
		st.handled = true
	}
	c.mu.Unlock()
	if ok {
		ev := st.ev
		ev.Kind = CallHandled
		c.emitCall(ev)
	}
}

func (c *Client) callEnded(id string) {
	c.mu.Lock()
	st, ok := c.calls[id]
	delete(c.calls, id)
	c.mu.Unlock()
	if !ok || st.handled {
		return
	}
	ev := st.ev
	ev.Kind = CallMissed
	c.check(c.Store.EnsureChat(ev.Chat, ev.Group))
	c.check(c.Store.PutMessage(store.Message{
		ChatJID:   ev.Chat,
		ID:        "call-" + ev.CallID,
		SenderJID: ev.From,
		TS:        ev.TS,
		Text:      callText(ev.Video),
		Status:    store.StatusSent,
	}))
	c.notify()
	c.emitCall(ev)
}
