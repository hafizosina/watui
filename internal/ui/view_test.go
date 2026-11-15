package ui

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/zhenzhu/watui/internal/config"
	"github.com/zhenzhu/watui/internal/store"
	"github.com/zhenzhu/watui/internal/wa"
)

func testModel(t *testing.T) Model {
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
	now := time.Now()
	put := func(chat, id, sender, name string, fromMe bool, ago time.Duration, text string, status store.Status, read bool) {
		if err := st.PutMessage(store.Message{ChatJID: chat, ID: id, SenderJID: sender, SenderName: name, FromMe: fromMe, TS: now.Add(-ago), Text: text, Status: status, Read: read}); err != nil {
			t.Fatal(err)
		}
	}
	dm := "628111@s.whatsapp.net"
	group := "1203630@g.us"
	put(dm, "1", dm, "Budi", false, 26*time.Hour, "Halo, besok jadi meeting?", store.StatusSent, true)
	put(dm, "2", "me@s.whatsapp.net", "", true, 25*time.Hour, "Jadi, jam 10 ya", store.StatusRead, true)
	put(dm, "3", dm, "Budi", false, 10*time.Minute, "Oke siap. Ini agak panjang supaya kelihatan wrapping-nya bekerja dengan benar di panel pesan sebelah kanan.", store.StatusSent, false)
	put(group, "4", "628222@s.whatsapp.net", "Sari", false, 3*time.Hour, "[image] deploy result", store.StatusSent, true)
	put(group, "5", "628333@s.whatsapp.net", "Andi", false, 2*time.Hour, "LGTM", store.StatusSent, false)
	put(group, "6", "me@s.whatsapp.net", "", true, time.Hour, "merged", store.StatusDelivered, true)
	if err := st.SetChatName(group, "BE Squad"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetPinned(group, true); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteMessage(group, "5"); err != nil {
		t.Fatal(err)
	}
	client := wa.New(whatsmeow.NewClient(device, waLog.Noop), st, waLog.Noop)
	return New(client, config.Default(), nil)
}

func step(m Model, msgs ...tea.Msg) Model {
	var tm tea.Model = m
	for _, msg := range msgs {
		tm, _ = tm.Update(msg)
	}
	return tm.(Model)
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func checkFrame(t *testing.T, view string, w, h int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) != h {
		t.Errorf("frame has %d lines, want %d", len(lines), h)
	}
	for i, l := range lines {
		if lw := lipgloss.Width(l); lw > w {
			t.Errorf("line %d is %d wide, want <= %d", i, lw, w)
		}
	}
}

func TestRender(t *testing.T) {
	const w, h = 110, 30
	m := step(testModel(t), tea.WindowSizeMsg{Width: w, Height: h}, ChangedMsg{})
	if len(m.chats) != 2 || m.names[m.chats[0].JID] != "BE Squad" {
		t.Fatalf("pinned group should be first, got %+v", m.chats)
	}
	checkFrame(t, m.View(), w, h)

	m = step(m, key("j"), key("enter"))
	if m.openJID != "628111@s.whatsapp.net" || m.focus != focusChat {
		t.Fatalf("enter should open the DM, got %q", m.openJID)
	}
	open := m.View()
	checkFrame(t, open, w, h)

	m = step(m, key("i"))
	if m.mode != modeInsert {
		t.Fatal("i should enter insert mode")
	}
	m = step(m, key("esc"), key("esc"), key("h"), key("/"), key("squ"))
	if len(m.chats) != 1 {
		t.Fatalf("search should leave 1 chat, got %d", len(m.chats))
	}
	m = step(m, key("esc"), key("?"))
	checkFrame(t, m.View(), w, h)

	if os.Getenv("WATUI_SHOW") != "" {
		t.Log("\n" + open)
	}
}

func TestMessageMenu(t *testing.T) {
	const w, h = 110, 30
	m := step(testModel(t), tea.WindowSizeMsg{Width: w, Height: h}, ChangedMsg{})
	m = step(m, key("j"), key("enter"))
	if m.sel != len(m.messages)-1 {
		t.Fatalf("newest message should be selected, got %d", m.sel)
	}
	m = step(m, key("k"))
	if m.sel != len(m.messages)-2 {
		t.Fatalf("k should select the previous message, got %d", m.sel)
	}
	m = step(m, key(" "))
	if m.menu == nil || m.menu.title != "Message" {
		t.Fatal("space should open the message menu")
	}
	frame := m.View()
	checkFrame(t, frame, w, h)
	if !strings.Contains(frame, "Reply") || !strings.Contains(frame, "Forward") {
		t.Fatal("menu should list Reply and Forward")
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyCtrlN}, tea.KeyMsg{Type: tea.KeyCtrlP}, key("enter"))
	if m.menu != nil || m.replyTo == nil || m.mode != modeInsert {
		t.Fatalf("picking Reply should close the menu and start a reply, menu=%v reply=%v", m.menu, m.replyTo)
	}
	if !strings.Contains(m.View(), "Replying to") {
		t.Fatal("compose title should show the reply")
	}
	m = step(m, key("esc"), key("esc"), key(" "), key("f"))
	if m.menu == nil || !m.menu.filterable {
		t.Fatal("f should open the forward picker")
	}
	m = step(m, key("b"), key("e"))
	if got := len(m.menu.visible()); got != 1 {
		t.Fatalf("typing 'be' should leave 1 chat, got %d", got)
	}
	if os.Getenv("WATUI_SHOW") != "" {
		t.Log("\n" + frame)
	}
}

func TestCtrlHL(t *testing.T) {
	m := step(testModel(t), tea.WindowSizeMsg{Width: 110, Height: 30}, ChangedMsg{})
	m = step(m, key("enter"), tea.KeyMsg{Type: tea.KeyCtrlH})
	if m.focus != focusList {
		t.Fatal("ctrl+h should focus the chat list")
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyCtrlL})
	if m.focus != focusChat {
		t.Fatal("ctrl+l should focus the messages")
	}
	m = step(m, key("i"), tea.KeyMsg{Type: tea.KeyCtrlH})
	if m.focus != focusList || m.mode != modeNormal {
		t.Fatal("ctrl+h from compose should leave compose and focus the chat list")
	}
}
