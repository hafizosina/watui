package ui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/hafizosina/watui/internal/config"
	"github.com/hafizosina/watui/internal/notify"
	"github.com/hafizosina/watui/internal/profile"
	"github.com/hafizosina/watui/internal/store"
	"github.com/hafizosina/watui/internal/wa"
)

type IncomingMsg struct {
	Profile string
	Msg     store.Message
}

type OpenChatMsg struct {
	Profile string
	JID     string
}

const notifyMaxAge = 2 * time.Minute

func NoteKey(profile, jid string) string {
	return profile + "|" + jid
}

func SplitNoteKey(key string) (string, string) {
	p, jid, _ := strings.Cut(key, "|")
	return p, jid
}

func (m Model) profileByName(name string) (*profile.Profile, int) {
	for i, p := range m.profiles {
		if p.Name == name {
			return p, i
		}
	}
	return nil, -1
}

// ApplySounds hands the notification sound settings to the notifier.
func ApplySounds(n *notify.Notifier, cfg config.Config) {
	dir := notify.SoundDir()
	if n == nil || dir == "" {
		return
	}
	n.SetSound(cfg.NotifySound, map[notify.SoundKind]string{
		notify.SoundMessage: cfg.NotifySoundPersonal,
		notify.SoundMention: cfg.NotifySoundMention,
		notify.SoundGroup:   cfg.NotifySoundGroup,
	}, dir)
}

// noteFor decides whether a new message deserves a desktop notification.
// focusTerminal is swapped out in tests so they never switch the real tmux window or raise kitty.
var focusTerminal = notify.FocusTerminal

func (m Model) noteFor(in IncomingMsg) (notify.Note, bool) {
	if m.cfg.Notify == config.NotifyOff || in.Msg.FromMe || time.Since(in.Msg.TS) > notifyMaxAge {
		return notify.Note{}, false
	}
	p, idx := m.profileByName(in.Profile)
	if p == nil || !p.NotifyEnabled() {
		return notify.Note{}, false
	}
	chat, ok, err := p.WA.Store.Chat(in.Msg.ChatJID)
	if err != nil || !ok {
		return notify.Note{}, false
	}
	forMe := in.Msg.MentionsMe || in.Msg.RepliesMe
	if (chat.Muted() || chat.Archived) && !forMe {
		return notify.Note{}, false
	}
	if idx == m.active && m.focused && m.openJID == in.Msg.ChatJID {
		return notify.Note{}, false
	}
	chatName := sanitize(p.WA.ChatName(chat))
	sender := sanitize(p.WA.ContactName(in.Msg.SenderJID, in.Msg.SenderName))
	title := chatName
	switch {
	case in.Msg.MentionsMe:
		title = "@ " + sender + " mentioned you · " + chatName
	case in.Msg.RepliesMe && chat.IsGroup:
		title = "↩ " + sender + " replied to you · " + chatName
	case chat.IsGroup:
		title = sender + " @ " + chatName
	}
	if len(m.profiles) > 1 {
		title = "[" + p.Label() + "] " + title
	}
	body := "New message"
	if m.cfg.Notify == config.NotifyOn {
		body = ansi.Truncate(strings.TrimSpace(sanitize(p.WA.ResolveMentions(in.Msg.Text))), 300, "…")
		if body == "" {
			body = "New message"
		}
	}
	sound := notify.SoundMessage
	switch {
	case chat.IsGroup && forMe:
		sound = notify.SoundMention
	case chat.IsGroup:
		sound = notify.SoundGroup
	}
	return notify.Note{Key: NoteKey(p.Name, in.Msg.ChatJID), Title: title, Body: body, Sound: sound}, true
}

func (m Model) incoming(in IncomingMsg) (tea.Model, tea.Cmd) {
	if note, ok := m.noteFor(in); ok && m.opts.Notifier != nil {
		m.opts.Notifier.Send(note)
	}
	return m, nil
}

func (m Model) openFromNotification(msg OpenChatMsg) (tea.Model, tea.Cmd) {
	p, idx := m.profileByName(msg.Profile)
	if p == nil {
		return m, nil
	}
	var model tea.Model = m
	if idx != m.active {
		model, _ = m.switchProfile(idx)
	}
	m = model.(Model)
	m.showArchived, m.filter = false, ""
	m.applyFilter()
	for i, c := range m.chats {
		if c.JID == msg.JID {
			m.cursor = i
		}
	}
	m.clampCursor()
	m.menu = nil
	m.enterNormal()
	m.open(msg.JID)
	m.focus = focusChat
	m.renderMessages(true)
	if m.opts.Notifier != nil {
		m.opts.Notifier.Forget(NoteKey(msg.Profile, msg.JID))
	}
	go focusTerminal()
	return m, m.markReadCmd()
}

type CallMsg struct {
	Profile string
	Ev      wa.CallEvent
}

const (
	ringMaxAge   = time.Minute
	missedMaxAge = 10 * time.Minute
)

func (m Model) callNote(c CallMsg) (notify.Note, bool) {
	if m.cfg.Notify == config.NotifyOff || c.Ev.Kind == wa.CallHandled {
		return notify.Note{}, false
	}
	age := time.Since(c.Ev.TS)
	if (c.Ev.Kind == wa.CallRinging && age > ringMaxAge) || (c.Ev.Kind == wa.CallMissed && age > missedMaxAge) {
		return notify.Note{}, false
	}
	p, _ := m.profileByName(c.Profile)
	if p == nil || !p.NotifyEnabled() {
		return notify.Note{}, false
	}
	kind := "voice call"
	if c.Ev.Video {
		kind = "video call"
	}
	title := sanitize(p.WA.ContactName(c.Ev.From, ""))
	if c.Ev.Group {
		if chat, ok, _ := p.WA.Store.Chat(c.Ev.Chat); ok {
			title += " @ " + sanitize(p.WA.ChatName(chat))
		}
		kind = "group " + kind
	}
	if len(m.profiles) > 1 {
		title = "[" + p.Label() + "] " + title
	}
	note := notify.Note{Key: NoteKey(p.Name, c.Ev.Chat), Title: title, Body: "Incoming " + kind + " — answer on your phone", Sound: notify.SoundCall}
	if c.Ev.Kind == wa.CallMissed {
		note.Body = "Missed " + kind
		note.Sound = notify.SoundMessage
	} else {
		note.Actions = []notify.Action{{Name: "decline:" + c.Ev.CallID, Label: "Decline"}}
	}
	return note, true
}

type DeclineCallMsg struct {
	Profile string
	CallID  string
}

type ringing struct {
	profile string
	ev      wa.CallEvent
	until   time.Time
}

func (m Model) call(c CallMsg) (tea.Model, tea.Cmd) {
	switch {
	case c.Ev.Kind == wa.CallRinging && time.Since(c.Ev.TS) < ringMaxAge:
		m.ringing = &ringing{profile: c.Profile, ev: c.Ev, until: time.Now().Add(90 * time.Second)}
	case m.ringing != nil && m.ringing.ev.CallID == c.Ev.CallID:
		m.ringing = nil
	}
	if note, ok := m.callNote(c); ok && m.opts.Notifier != nil {
		m.opts.Notifier.Send(note)
	}
	return m, nil
}

func (m Model) declineCall(profileName, callID string) (tea.Model, tea.Cmd) {
	p, _ := m.profileByName(profileName)
	if p == nil {
		return m, nil
	}
	if m.ringing != nil && m.ringing.ev.CallID == callID {
		m.ringing = nil
	}
	return m, m.actionCmd("call declined", func(ctx context.Context) error { return p.WA.DeclineCall(ctx, callID) })
}

func (m Model) ringBanner() (string, bool) {
	r := m.ringing
	if r == nil || time.Now().After(r.until) {
		return "", false
	}
	p, _ := m.profileByName(r.profile)
	if p == nil {
		return "", false
	}
	kind := "voice call"
	if r.ev.Video {
		kind = "video call"
	}
	who := sanitize(p.WA.ContactName(r.ev.From, ""))
	if r.ev.Group {
		if chat, ok, _ := p.WA.Store.Chat(r.ev.Chat); ok {
			who += " @ " + sanitize(p.WA.ChatName(chat))
		}
	}
	if len(m.profiles) > 1 {
		who = "[" + p.Label() + "] " + who
	}
	return " " + iconPhone + "  Incoming " + kind + " from " + who + "  ·  ctrl+x decline  ·  answer on your phone ", true
}
