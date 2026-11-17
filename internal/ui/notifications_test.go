package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hafizosina/watui/internal/config"
	"github.com/hafizosina/watui/internal/profile"
	"github.com/hafizosina/watui/internal/store"
	"github.com/hafizosina/watui/internal/wa"
)

func TestNotificationRules(t *testing.T) {
	personal := linkedProfile(t, profile.Default, "62822")
	work := linkedProfile(t, "work", "62811")
	m := New([]*profile.Profile{personal, work}, config.Default(), nil, Options{})
	m = step(m, tea.WindowSizeMsg{Width: 110, Height: 34}, ChangedMsg{})

	dm := "628111@s.whatsapp.net"
	group := "1203630@g.us"
	in := func(p, chat, sender, name, text string) IncomingMsg {
		return IncomingMsg{Profile: p, Msg: store.Message{ChatJID: chat, ID: "N" + text, SenderJID: sender, SenderName: name, TS: time.Now(), Text: text}}
	}

	note, ok := m.noteFor(in("default", dm, dm, "Budi", "halo bro"))
	if !ok || note.Title != "[default] +628111" || note.Body != "halo bro" {
		t.Fatalf("DM should notify with profile prefix, got ok=%v %+v", ok, note)
	}
	note, ok = m.noteFor(in("work", group, "628222@s.whatsapp.net", "Sari", "deploy done"))
	if !ok || note.Title != "[work] Sari @ BE Squad" {
		t.Fatalf("group note should show sender @ group, got %+v", note)
	}
	m = step(m, key("g"), key("t"), key("1"), key(" "))
	labels := strings.Join(labelsOf(m.menu.items), "|")
	if !strings.Contains(labels, "Turn off desktop notifications") {
		t.Fatalf("profile menu should offer turning notifications off, got %s", labels)
	}
	m = step(m, key("n"))
	if work.NotifyEnabled() {
		t.Fatal("work notifications should be off")
	}
	if _, ok := m.noteFor(in("work", group, "628222@s.whatsapp.net", "Sari", "muted profile")); ok {
		t.Error("a profile with notifications off must not notify")
	}
	if _, ok := m.noteFor(in("default", dm, dm, "Budi", "other profile")); !ok {
		t.Error("other profiles must still notify")
	}
	if !strings.Contains(m.View(), iconMute) {
		t.Error("profile box should show the muted bell")
	}
	if err := work.SetNotify(true); err != nil {
		t.Fatal(err)
	}
	m = step(m, key("g"), key("T"), key("2"))

	old := in("default", dm, dm, "Budi", "kemarin")
	old.Msg.TS = time.Now().Add(-10 * time.Minute)
	if _, ok := m.noteFor(old); ok {
		t.Error("messages older than 2 minutes (offline backlog) must not notify")
	}

	if err := personal.WA.Store.SetMutedUntil(dm, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.noteFor(in("default", dm, dm, "Budi", "muted")); ok {
		t.Error("muted chats must not notify")
	}
	if err := work.WA.Store.SetMutedUntil(group, store.MutedForever); err != nil {
		t.Fatal(err)
	}
	quiet := in("work", group, "628222@s.whatsapp.net", "Sari", "lunch?")
	if _, ok := m.noteFor(quiet); ok {
		t.Error("a normal message in a muted group must not notify")
	}
	mention := in("work", group, "628222@s.whatsapp.net", "Sari", "@you can you check?")
	mention.Msg.MentionsMe = true
	if note, ok := m.noteFor(mention); !ok || note.Title != "[work] @ Sari mentioned you · BE Squad" {
		t.Errorf("a mention in a muted group should notify, got ok=%v %+v", ok, note)
	}
	reply := in("work", group, "628222@s.whatsapp.net", "Sari", "agreed")
	reply.Msg.RepliesMe = true
	if note, ok := m.noteFor(reply); !ok || note.Title != "[work] ↩ Sari replied to you · BE Squad" {
		t.Errorf("a reply to me in a muted group should notify, got ok=%v %+v", ok, note)
	}
	_ = work.WA.Store.SetMutedUntil(group, time.Time{})
	_ = personal.WA.Store.SetMutedUntil(dm, time.Time{})

	m = step(m, key("2"), key("j"), key("enter"))
	if _, ok := m.noteFor(in("default", m.openJID, m.openJID, "Budi", "reading")); ok {
		t.Error("the open chat in a focused window must not notify")
	}
	m = step(m, tea.BlurMsg{})
	if _, ok := m.noteFor(in("default", m.openJID, m.openJID, "Budi", "away")); !ok {
		t.Error("the open chat should notify when the window is not focused")
	}

	m.cfg.Notify = config.NotifyPrivate
	if note, _ := m.noteFor(in("work", group, "628222@s.whatsapp.net", "Sari", "secret")); note.Body != "New message" {
		t.Errorf("private mode must hide the text, got %q", note.Body)
	}
	m.cfg.Notify = config.NotifyOff
	if _, ok := m.noteFor(in("work", group, "628222@s.whatsapp.net", "Sari", "x")); ok {
		t.Error("notify=off must not notify")
	}

	m = step(m, OpenChatMsg{Profile: "work", JID: group})
	if m.activeProfile().Name != "work" || m.openJID != group || m.focus != focusChat {
		t.Fatalf("clicking should switch to work and open the group, got %s %q", m.activeProfile().Name, m.openJID)
	}
}

func TestCallNotes(t *testing.T) {
	m := step(testModel(t), tea.WindowSizeMsg{Width: 110, Height: 34}, ChangedMsg{})
	ev := wa.CallEvent{Kind: wa.CallRinging, CallID: "X", From: "628111@s.whatsapp.net", Chat: "628111@s.whatsapp.net", Video: true, TS: time.Now()}
	note, ok := m.callNote(CallMsg{Profile: profile.Default, Ev: ev})
	if !ok || note.Title != "+628111" || note.Body != "Incoming video call — answer on your phone" {
		t.Fatalf("ringing note wrong: %+v", note)
	}
	ev.Kind = wa.CallMissed
	if note, _ = m.callNote(CallMsg{Profile: profile.Default, Ev: ev}); note.Body != "Missed video call" {
		t.Fatalf("missed note wrong: %+v", note)
	}
	ev.Kind, ev.TS = wa.CallRinging, time.Now().Add(-5*time.Minute)
	if _, ok := m.callNote(CallMsg{Profile: profile.Default, Ev: ev}); ok {
		t.Error("an old ringing event (after reconnect) must not notify")
	}
	if err := m.wa.Store.PutMessage(store.Message{ChatJID: "628111@s.whatsapp.net", ID: "call-X", SenderJID: "628111@s.whatsapp.net", TS: time.Now(), Text: "[missed voice call]"}); err != nil {
		t.Fatal(err)
	}
	m = step(m, ChangedMsg{}, key("2"), key("j"), key("enter"))
	if !strings.Contains(m.vp.View(), "Missed voice call") {
		t.Error("missed call should render as a red Missed voice call line")
	}
}

func TestRingingBanner(t *testing.T) {
	m := step(testModel(t), tea.WindowSizeMsg{Width: 120, Height: 34}, ChangedMsg{})
	ev := wa.CallEvent{Kind: wa.CallRinging, CallID: "R1", From: "628111@s.whatsapp.net", Chat: "628111@s.whatsapp.net", Video: true, TS: time.Now()}
	m = step(m, CallMsg{Profile: profile.Default, Ev: ev})
	frame := m.View()
	if !strings.Contains(frame, "Incoming video call from +628111") || !strings.Contains(frame, "ctrl+x decline") {
		t.Fatal("a ringing call should show the banner with ctrl+x decline")
	}
	checkFrame(t, frame, 120, 34)
	note, _ := m.callNote(CallMsg{Profile: profile.Default, Ev: ev})
	if len(note.Actions) != 1 || note.Actions[0].Name != "decline:R1" {
		t.Fatalf("ringing notification should carry a Decline action, got %+v", note.Actions)
	}
	var cmd tea.Cmd
	var tm tea.Model
	tm, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	m = tm.(Model)
	if m.ringing != nil || cmd == nil {
		t.Fatal("ctrl+x should decline (send a command) and clear the banner")
	}
	m = step(m, CallMsg{Profile: profile.Default, Ev: ev})
	ev.Kind = wa.CallHandled
	m = step(m, CallMsg{Profile: profile.Default, Ev: ev})
	if m.ringing != nil {
		t.Fatal("a call answered elsewhere should clear the banner")
	}
	for text, want := range map[string]string{
		"[missed voice call]":   "Missed voice call",
		"[declined video call]": "Declined video call",
		"[voice call]":          "Voice call · answered on another device",
	} {
		if line, ok := callLine(text); !ok || !strings.Contains(line, want) {
			t.Errorf("callLine(%q) = %q, want %q", text, line, want)
		}
	}
}
