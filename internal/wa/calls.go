package wa

import (
	"context"
	"errors"
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

type CallOutcome string

const (
	OutcomeMissed   CallOutcome = "missed"
	OutcomeAnswered CallOutcome = "answered"
	OutcomeDeclined CallOutcome = "declined"
)

type CallEvent struct {
	Kind    CallKind
	CallID  string
	From    string
	Chat    string
	Video   bool
	Group   bool
	TS      time.Time
	Outcome CallOutcome
}

type callState struct {
	ev      CallEvent
	outcome CallOutcome
	caller  types.JID
}

// CallText is the chat line stored for a finished call.
func CallText(outcome CallOutcome, video bool) string {
	kind := "voice call"
	if video {
		kind = "video call"
	}
	switch outcome {
	case OutcomeAnswered:
		return "[" + kind + "]"
	case OutcomeDeclined:
		return "[declined " + kind + "]"
	}
	return "[missed " + kind + "]"
}

func isVideoOffer(data *waBinary.Node) bool {
	if data == nil {
		return false
	}
	_, ok := data.GetOptionalChildByTag("video")
	return ok
}

// OnCall is called when a call rings, is missed, or is picked up / declined.
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
	caller := meta.From
	if !meta.CallCreator.IsEmpty() {
		caller = meta.CallCreator
	}
	from := c.normalize(ctx, caller)
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
	c.calls[meta.CallID] = &callState{ev: ev, caller: meta.From}
	c.mu.Unlock()
	c.emitCall(ev)
}

func (c *Client) callSettled(id string, outcome CallOutcome) {
	c.mu.Lock()
	st, ok := c.calls[id]
	if ok && st.outcome == "" {
		st.outcome = outcome
	}
	c.mu.Unlock()
	if ok {
		ev := st.ev
		ev.Kind, ev.Outcome = CallHandled, outcome
		c.emitCall(ev)
	}
}

func (c *Client) callEnded(id string) {
	c.mu.Lock()
	st, ok := c.calls[id]
	delete(c.calls, id)
	c.mu.Unlock()
	if !ok {
		return
	}
	ev := st.ev
	ev.Outcome = st.outcome
	if ev.Outcome == "" {
		ev.Outcome = OutcomeMissed
	}
	c.check(c.Store.EnsureChat(ev.Chat, ev.Group))
	c.check(c.Store.PutMessage(store.Message{
		ChatJID:   ev.Chat,
		ID:        "call-" + ev.CallID,
		SenderJID: ev.From,
		TS:        ev.TS,
		Text:      CallText(ev.Outcome, ev.Video),
		Status:    store.StatusSent,
		Read:      ev.Outcome != OutcomeMissed,
	}))
	c.notify()
	if ev.Outcome == OutcomeMissed {
		ev.Kind = CallMissed
	} else {
		ev.Kind = CallHandled
	}
	c.emitCall(ev)
}

// DeclineCall rejects a ringing call; the caller sees it as declined.
func (c *Client) DeclineCall(ctx context.Context, callID string) error {
	c.mu.Lock()
	st, ok := c.calls[callID]
	c.mu.Unlock()
	if !ok {
		return errors.New("that call already ended")
	}
	if err := c.WA.RejectCall(ctx, st.caller, callID); err != nil {
		return err
	}
	c.callSettled(callID, OutcomeDeclined)
	c.callEnded(callID)
	return nil
}
