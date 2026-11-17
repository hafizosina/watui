package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hafizosina/watui/internal/store"
)

func TestMatchSpans(t *testing.T) {
	for _, c := range []struct {
		line, q string
		want    [][2]int
	}{
		{"Jadi, jam 10 ya jadi", "jadi", [][2]int{{0, 4}, {16, 20}}},
		{"aaaa", "aa", [][2]int{{0, 2}, {2, 4}}},
		{"Ünïcode ÜNÏ", "ünï", [][2]int{{0, 5}, {10, 15}}}, // Ü and ï are 2 bytes each
		{"nothing", "x", nil},
		{"anything", "", nil},
	} {
		if got := matchSpans(c.line, c.q); fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("matchSpans(%q, %q) = %v, want %v", c.line, c.q, got, c.want)
		}
	}
}

func openDM(t *testing.T) (Model, string) {
	t.Helper()
	m := step(testModel(t), tea.WindowSizeMsg{Width: 110, Height: 30}, ChangedMsg{})
	dm := "628111@s.whatsapp.net"
	m.open(dm)
	m.focus = focusChat
	return m, dm
}

func TestSearchInChat(t *testing.T) {
	m, _ := openDM(t)
	selText := func() string { msg, _ := m.selected(); return msg.Text }

	m = step(m, key("/"))
	if m.mode != modeMsgSearch {
		t.Fatalf("/ in the messages pane should search the chat, mode=%v", m.mode)
	}
	m = step(m, key("JADI"), key("enter"))
	if m.msgSearch != "JADI" || len(m.msgHits) != 2 {
		t.Fatalf("want 2 hits for JADI, got %d (%q)", len(m.msgHits), m.msgSearch)
	}
	if !strings.HasPrefix(selText(), "Jadi, jam 10") || !strings.Contains(m.messagesTitle(), "1 of 2") {
		t.Fatalf("enter should go to the nearest match above: %q, title %q", selText(), m.messagesTitle())
	}
	m = step(m, key("n"))
	if !strings.HasPrefix(selText(), "Halo") || !strings.Contains(m.messagesTitle(), "2 of 2") {
		t.Errorf("n should go to the older match: %q %q", selText(), m.messagesTitle())
	}
	m = step(m, key("n"))
	if !strings.HasPrefix(selText(), "Jadi") || !strings.Contains(m.flash, "back to the newest") {
		t.Errorf("n past the oldest should wrap to the newest: %q flash=%q", selText(), m.flash)
	}
	m = step(m, key("N"))
	if !strings.HasPrefix(selText(), "Halo") {
		t.Errorf("N past the newest should wrap to the oldest: %q", selText())
	}
	checkFrame(t, m.View(), 110, 30)

	m = step(m, key("esc"))
	if m.msgSearch != "" || m.focus != focusChat {
		t.Errorf("esc should clear the search and stay in the messages, search=%q focus=%v", m.msgSearch, m.focus)
	}
	m = step(m, key("/"), key("zzz"), key("enter"))
	if !m.flashErr || len(m.msgHits) != 0 {
		t.Errorf("no match should say so, flash=%q", m.flash)
	}
}

func TestSearchLoadsOlderHistory(t *testing.T) {
	m, dm := openDM(t)
	old := time.Now().Add(-48 * time.Hour)
	for i := 0; i < 300; i++ {
		text := fmt.Sprintf("filler %d", i)
		if i == 0 {
			text = "the needle is here"
		}
		if err := m.wa.Store.PutMessage(store.Message{ChatJID: dm, ID: fmt.Sprintf("old%d", i), SenderJID: dm, TS: old.Add(time.Duration(i) * time.Minute), Text: text, Read: true}); err != nil {
			t.Fatal(err)
		}
	}
	m.open(dm)
	m.focus = focusChat
	if m.indexOf("old0") >= 0 {
		t.Fatal("the needle should start out of the loaded page")
	}
	m = step(m, key("/"), key("needle"), key("enter"))
	if cur, _ := m.selected(); cur.ID != "old0" {
		t.Errorf("search should load older history and land on the needle, got %q", cur.ID)
	}
}
