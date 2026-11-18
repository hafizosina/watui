package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/hafizosina/watui/internal/store"
)

func TestDraftKeptAcrossChats(t *testing.T) {
	m := step(testModel(t), tea.WindowSizeMsg{Width: 110, Height: 30}, ChangedMsg{})
	dm, group := "628111@s.whatsapp.net", "1203630@g.us"

	m.open(group)
	typeText(m.input, "hai ")
	m.input.insertAtom(len(m.input.buf), atom{label: "@Sari", user: "628222", jid: "628222@s.whatsapp.net"})
	typeText(m.input, "cek ya")
	reply := m.messages[0]
	m.replyTo = &reply

	m.open(dm)
	if v := m.input.Value(); v != "" {
		t.Fatalf("other chat should start empty, got %q", v)
	}
	if m.replyTo != nil {
		t.Fatal("reply should stay with the group's draft")
	}
	if row := ansi.Strip(m.View()); !strings.Contains(row, "Draft: hai @Sari cek ya") {
		t.Errorf("chat list should show the group's draft:\n%s", row)
	}

	m.open(group)
	if v := m.input.Value(); v != "hai @Sari cek ya" {
		t.Fatalf("draft not restored, got %q", v)
	}
	if text, jids := m.input.SendValue(); text != "hai @628222 cek ya" || len(jids) != 1 {
		t.Fatalf("mention not restored: %q %v", text, jids)
	}
	if m.replyTo == nil || m.replyTo.ID != reply.ID {
		t.Fatalf("reply not restored: %+v", m.replyTo)
	}
}

func TestDraftSurvivesRestart(t *testing.T) {
	m := step(testModel(t), tea.WindowSizeMsg{Width: 110, Height: 30}, ChangedMsg{})
	dm := "628111@s.whatsapp.net"
	m.open(dm)
	typeText(m.input, "belum selesai")
	m.SaveDraft() // what main does when watui quits

	drafts, err := m.wa.Store.Drafts()
	if err != nil {
		t.Fatal(err)
	}
	if drafts[dm].Text != "belum selesai" {
		t.Fatalf("draft not saved: %+v", drafts)
	}

	m.drafts = nil // a fresh start
	m.openJID = ""
	m.input.Reset()
	m = step(m, ChangedMsg{})
	m.open(dm)
	if v := m.input.Value(); v != "belum selesai" {
		t.Fatalf("draft not restored after restart, got %q", v)
	}
}

func TestDraftClearedWhenSentOrEmptied(t *testing.T) {
	m := step(testModel(t), tea.WindowSizeMsg{Width: 110, Height: 30}, ChangedMsg{})
	dm, group := "628111@s.whatsapp.net", "1203630@g.us"

	m.open(dm)
	typeText(m.input, "kirim ini")
	m.open(group)
	m.open(dm)
	_ = m.sendCmd("kirim ini", nil) // ctrl+s
	m.input.Reset()
	if _, ok := m.drafts[dm]; ok {
		t.Fatal("sending should drop the draft")
	}
	if d, _ := m.wa.Store.Drafts(); len(d) != 0 {
		t.Fatalf("sending should delete the saved draft: %+v", d)
	}

	typeText(m.input, "hapus")
	m.open(group)
	m.open(dm)
	m.input.Reset() // deleted it all
	typeText(m.input, "   ")
	m.open(group)
	if _, ok := m.drafts[dm]; ok {
		t.Fatal("a blank Compose should not leave a draft")
	}
}

func TestEditIsNotADraft(t *testing.T) {
	m := step(testModel(t), tea.WindowSizeMsg{Width: 110, Height: 30}, ChangedMsg{})
	dm, group := "628111@s.whatsapp.net", "1203630@g.us"

	m.open(dm)
	typeText(m.input, "draft lama")
	m.open(group)
	m.open(dm)
	own := store.Message{ChatJID: dm, ID: "2", FromMe: true, Text: "Jadi, jam 10 ya"}
	m.editing, m.replyTo = &own, nil
	m.input.SetValue(own.Text)
	m.open(group)
	m.open(dm)
	if v := m.input.Value(); v != "draft lama" {
		t.Fatalf("leaving an edit should keep the earlier draft, got %q", v)
	}
}
