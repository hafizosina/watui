package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestThemes(t *testing.T) {
	defer ApplyTheme(builtinThemes[DefaultTheme])
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := step(testModel(t), tea.WindowSizeMsg{Width: 110, Height: 34}, ChangedMsg{})

	m = step(m, key(":"))
	m = typeInto(m, "theme")
	m = step(m, key("enter"))
	if m.menu == nil || !strings.HasPrefix(m.menu.title, "Theme") {
		t.Fatal(":theme should open the theme picker")
	}
	start := currentTheme
	m = step(m, key("j"))
	if currentTheme == start {
		t.Fatal("moving in the picker should preview another theme")
	}
	m = step(m, key("esc"))
	if currentTheme != start || colBlue != builtinThemes[start].Blue {
		t.Fatalf("esc should restore %s, got %s", start, currentTheme)
	}

	m = step(m, key(":"))
	m = typeInto(m, "theme nord")
	m = step(m, key("enter"))
	if currentTheme != "nord" || colBlue != lipgloss.Color("#81a1c1") {
		t.Fatalf(":theme nord should switch, got %s", currentTheme)
	}
	checkFrame(t, m.View(), 110, 34)

	dir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "watui", "themes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mine.conf"), []byte("# my colors\nbase = gruvbox-dark\nblue = #123456\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.conf"), []byte("blue = navy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(ThemeNames(), " "), "mine") {
		t.Fatal("custom themes should be listed")
	}
	th, err := LoadTheme("mine")
	if err != nil || th.Blue != "#123456" || th.Green != builtinThemes["gruvbox-dark"].Green {
		t.Fatalf("custom theme should override blue on top of gruvbox, got %+v %v", th, err)
	}
	if _, err := LoadTheme("broken"); err == nil || !strings.Contains(err.Error(), "hex color") {
		t.Fatalf("a bad color should be a clear error, got %v", err)
	}
	if _, err := LoadTheme("nope"); err == nil || !strings.Contains(err.Error(), "unknown theme") {
		t.Fatalf("unknown theme should be an error, got %v", err)
	}
}

func TestSetThemeAppliesNow(t *testing.T) {
	m := step(testModel(t), tea.WindowSizeMsg{Width: 110, Height: 30}, ChangedMsg{})
	defer func() { _ = m.setTheme(DefaultTheme) }()
	m2, _ := m.runCommand("set theme=nord")
	m = m2.(Model)
	if currentTheme != "nord" || m.flashErr {
		t.Errorf(":set theme=nord should switch now, theme=%q flash=%q", currentTheme, m.flash)
	}
	m2, _ = m.runCommand("set theme=nope")
	if m = m2.(Model); !m.flashErr || m.cfg.Theme != "nord" {
		t.Errorf("an unknown theme should be an error and keep nord, got theme=%q", m.cfg.Theme)
	}
}

func TestBlankEditor(t *testing.T) {
	t.Setenv("EDITOR", "   ")
	if got := editorCommand(); len(got) != 1 || got[0] != "nvim" {
		t.Errorf("blank $EDITOR should fall back to nvim, got %q", got)
	}
	t.Setenv("EDITOR", "code --wait")
	if got := editorCommand(); len(got) != 2 || got[0] != "code" {
		t.Errorf("got %q", got)
	}
	_ = viewFileCmd("/tmp/x.txt") // used to panic on a blank $EDITOR
}
