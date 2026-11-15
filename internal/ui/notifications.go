package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/zhenzhu/watui/internal/config"
	"github.com/zhenzhu/watui/internal/notify"
	"github.com/zhenzhu/watui/internal/profile"
	"github.com/zhenzhu/watui/internal/store"
	"github.com/zhenzhu/watui/internal/wa"
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

// noteFor decides whether a new message deserves a desktop notification.
func (m Model) noteFor(in IncomingMsg) (notify.Note, bool) {
	if m.cfg.Notify == config.NotifyOff || in.Msg.FromMe || time.Since(in.Msg.TS) > notifyMaxAge {
		return notify.Note{}, false
	}
	p, idx := m.profileByName(in.Profile)
	if p == nil || !p.NotifyEnabled() {
		return notify.Note{}, false
	}
	chat, ok, err := p.WA.Store.Chat(in.Msg.ChatJID)
	if err != nil || !ok || chat.Muted() || chat.Archived {
		return notify.Note{}, false
	}
	if idx == m.active && m.focused && m.openJID == in.Msg.ChatJID {
		return notify.Note{}, false
	}
	chatName := sanitize(p.WA.ChatName(chat))
	title := chatName
	if chat.IsGroup {
		title = sanitize(p.WA.ContactName(in.Msg.SenderJID, in.Msg.SenderName)) + " @ " + chatName
	}
	if len(m.profiles) > 1 {
		title = "[" + p.Label() + "] " + title
	}
	body := "New message"
	if m.cfg.Notify == config.NotifyOn {
		body = ansi.Truncate(strings.TrimSpace(sanitize(in.Msg.Text)), 300, "…")
		if body == "" {
			body = "New message"
		}
	}
	return notify.Note{Key: NoteKey(p.Name, in.Msg.ChatJID), Title: title, Body: body}, true
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
	go notify.FocusTerminal()
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
	body := "Incoming " + kind + " — answer on your phone"
	if c.Ev.Kind == wa.CallMissed {
		body = "Missed " + kind
	}
	return notify.Note{Key: NoteKey(p.Name, c.Ev.Chat), Title: title, Body: body}, true
}

func (m Model) call(c CallMsg) (tea.Model, tea.Cmd) {
	if note, ok := m.callNote(c); ok && m.opts.Notifier != nil {
		m.opts.Notifier.Send(note)
	}
	return m, nil
}
