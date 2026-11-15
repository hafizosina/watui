package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestEmojiCompletion(t *testing.T) {
	m := step(testModel(t), tea.WindowSizeMsg{Width: 110, Height: 34}, ChangedMsg{})
	m = step(m, key("j"), key("enter"), key("i"))
	m = typeInto(m, "haha :jo")
	if _, _, list := m.emojiSuggestions(); len(list) == 0 || list[0].glyph != "😂" {
		t.Fatalf(":jo should suggest 😂 first, got %+v", list)
	}
	if !strings.Contains(m.View(), "Emoji :jo") {
		t.Fatal("the emoji popup should be visible")
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyTab})
	if got := m.input.Value(); got != "haha 😂" {
		t.Fatalf("tab should insert the emoji, got %q", got)
	}
	m = typeInto(m, " :thumbsu")
	m = step(m, tea.KeyMsg{Type: tea.KeyTab})
	if got := m.input.Value(); !strings.HasSuffix(got, "👍") {
		t.Fatalf(":thumbsu should complete to 👍, got %q", got)
	}
	m.input.Reset()
	m = typeInto(m, "rapat jam 10:30")
	if _, _, list := m.emojiSuggestions(); len(list) != 0 {
		t.Fatal("10:30 must not open the emoji popup")
	}
	m.input.Reset()
	m = typeInto(m, ":smi")
	m = step(m, key("esc"))
	if _, _, list := m.emojiSuggestions(); len(list) != 0 || m.input.mode != vimInsert {
		t.Fatal("first esc should only hide the popup")
	}
	m = step(m, key("esc"))
	if m.input.mode != vimNormal {
		t.Fatal("second esc should go to normal mode")
	}
	checkFrame(t, m.View(), 110, 34)
}
