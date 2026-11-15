package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCommandSuggestions(t *testing.T) {
	defer ApplyTheme(builtinThemes[DefaultTheme])
	m := step(testModel(t), tea.WindowSizeMsg{Width: 110, Height: 34}, ChangedMsg{})
	m = step(m, key(":"))
	m = typeInto(m, "th")
	if list := m.commandSuggestions(m.cmdTyped); len(list) != 1 || list[0].value != "theme" {
		t.Fatalf(":th should suggest theme, got %+v", list)
	}
	if !strings.Contains(m.View(), "pick a color theme") {
		t.Fatal("suggestion popup should show the description")
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.prompt.Value() != "theme" {
		t.Fatalf("tab should complete to theme, got %q", m.prompt.Value())
	}
	m = typeInto(m, " gr")
	m = step(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.prompt.Value() != "theme gruvbox-dark" {
		t.Fatalf("theme argument should complete, got %q", m.prompt.Value())
	}
	m = step(m, key("enter"))
	if currentTheme != "gruvbox-dark" {
		t.Fatal("enter should run the completed command")
	}

	m = step(m, key(":"))
	m = typeInto(m, "set im")
	list := m.commandSuggestions(m.cmdTyped)
	if len(list) != 3 || list[0].label != "image_height=" {
		t.Fatalf(":set im should list image settings, got %+v", list)
	}
	m.prompt.SetValue("set images=")
	m.cmdTyped = "set images="
	if list := m.commandSuggestions(m.cmdTyped); len(list) != 3 || list[1].value != "set images=thumbnail" {
		t.Fatalf("setting values should be suggested, got %+v", list)
	}

	m = step(m, key("esc"), key(":"))
	m = step(m, tea.KeyMsg{Type: tea.KeyTab}, tea.KeyMsg{Type: tea.KeyTab})
	if m.prompt.Value() != "info" {
		t.Fatalf("tab on an empty line should cycle through all commands, got %q", m.prompt.Value())
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.prompt.Value() != "new" {
		t.Fatalf("shift+tab should go back, got %q", m.prompt.Value())
	}
	checkFrame(t, m.View(), 110, 34)
}
