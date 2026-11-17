package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMultiSelect(t *testing.T) {
	m := step(testModel(t), tea.WindowSizeMsg{Width: 110, Height: 30}, ChangedMsg{})
	m = step(m, key("enter"))
	n := len(m.messages)
	if n < 2 || m.focus != focusChat {
		t.Fatalf("need an open chat with ≥2 messages, got %d", n)
	}

	m = step(m, key("v"))
	if got := len(m.selection()); got != 1 {
		t.Fatalf("v: %d selected, want 1", got)
	}
	m = step(m, key("v"))
	if m.hasSelection() {
		t.Fatal("v again should unmark")
	}

	// V on the last message, then k to the first: everything in between is selected.
	m = step(m, key("V"))
	for range n - 1 {
		m = step(m, key("k"))
	}
	if got := len(m.selection()); got != n {
		t.Fatalf("V range: %d selected, want %d", got, n)
	}
	if !strings.Contains(m.messagesTitle(), "selected") {
		t.Errorf("title %q should show the count", m.messagesTitle())
	}
	m = step(m, key("V"), key("j"))
	if got := len(m.selection()); got != n {
		t.Fatalf("committed range should stay put when the cursor moves: %d, want %d", got, n)
	}
	if text := m.selectionText(m.selection()); strings.Count(text, "\n") != n-1 {
		t.Errorf("copied text should have one line per message:\n%s", text)
	}

	m = step(m, key(" "))
	if m.menu == nil || !strings.Contains(m.menu.title, "selected") {
		t.Fatalf("space with a selection should open the bulk menu, got %+v", m.menu)
	}
	m = step(m, key("x"))
	if m.hasSelection() || m.menu != nil {
		t.Fatal("clear selection should empty it and close the menu")
	}

	m = step(m, key("v"), key("esc"))
	if m.hasSelection() || m.focus != focusChat {
		t.Fatal("esc should clear the selection and stay in the messages")
	}
}
