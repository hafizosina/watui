package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

var clipped string

// keep tests off the real clipboard
func TestMain(m *testing.M) {
	clipboardWriter = func(text string) { clipped = text }
	os.Exit(m.Run())
}

func TestFilesFromURIs(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a b.pdf")
	b := filepath.Join(dir, "c.png")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	nemo := "copy\nfile://" + strings.ReplaceAll(a, " ", "%20") + "\nfile://" + b
	if got := filesFromURIs(nemo); len(got) != 2 || got[0].path != a || got[1].path != b {
		t.Errorf("nemo copy: got %+v", got)
	}
	if got := filesFromURIs(a + "\r\n"); len(got) != 1 || got[0].path != a {
		t.Errorf("plain path: got %+v", got)
	}
	for _, text := range []string{"hello world", a + "\nnot a path", filepath.Join(dir, "missing.txt"), dir, "relative.txt"} {
		if got := filesFromURIs(text); got != nil {
			t.Errorf("%q should be text, got %+v", text, got)
		}
	}
}

func TestUploadArgs(t *testing.T) {
	dir := t.TempDir()
	spaced := filepath.Join(dir, "my file.txt")
	other := filepath.Join(dir, "other.txt")
	for _, p := range []string{spaced, other} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := uploadArgs(" " + spaced + " "); err != nil || len(got) != 1 || got[0].path != spaced {
		t.Errorf("path with a space: %+v %v", got, err)
	}
	if got, err := uploadArgs(other + " '" + other + "'"); err != nil || len(got) != 2 {
		t.Errorf("two paths: %+v %v", got, err)
	}
	if _, err := uploadArgs(filepath.Join(dir, "nope.txt")); err == nil {
		t.Error("a missing file should be an error")
	}
}

func TestAttachFlow(t *testing.T) {
	m := step(testModel(t), tea.WindowSizeMsg{Width: 110, Height: 30}, ChangedMsg{})
	m = step(m, key("enter"))
	path := filepath.Join(t.TempDir(), "photo.png")
	if err := os.WriteFile(path, []byte("\x89PNG\r\n\x1a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m = step(m, clipboardMsg{files: []attachment{{path: path}}})
	if len(m.attach) != 1 || m.mode != modeInsert {
		t.Fatalf("pasting a file should queue it and open Compose: attach=%d mode=%v", len(m.attach), m.mode)
	}
	if title := m.composeTitle(); !strings.Contains(title, "photo.png") {
		t.Errorf("compose title %q should name the file", title)
	}
	m = step(m, key("esc"), key("esc"), key("esc")) // insert → vim normal → leave Compose → drop
	if len(m.attach) != 0 {
		t.Fatal("esc in the messages pane should drop the attachments")
	}

	m = step(m, clipboardMsg{text: "hello"})
	if m.mode != modeInsert || m.input.Value() != "hello" {
		t.Fatalf("pasting text should land in Compose, got %q", m.input.Value())
	}
}

func TestVimPaste(t *testing.T) {
	m := step(testModel(t), tea.WindowSizeMsg{Width: 110, Height: 30}, ChangedMsg{})
	m = step(m, key("enter"), key("i"), key("ab"), key("esc"))
	if m.mode != modeInsert || m.input.mode != vimNormal {
		t.Fatalf("want Compose in vim normal mode, got %v/%v", m.mode, m.input.mode)
	}
	// nothing yanked: p pastes the clipboard text after the cursor
	m = step(m, clipboardMsg{text: "X", vim: "p"})
	if got := m.input.Value(); got != "abX" {
		t.Errorf("clipboard text: got %q, want abX", got)
	}
	// our own yank went to the clipboard: P pastes it
	m = step(m, key("0"), key("y"), key("l"))
	if clipped != "a" {
		t.Errorf("yank should reach the system clipboard, got %q", clipped)
	}
	m = step(m, clipboardMsg{text: clipped, vim: "P"})
	if got := m.input.Value(); got != "aabX" {
		t.Errorf("own yank: got %q, want aabX", got)
	}
	// then something copied in another app wins over the old yank
	m = step(m, clipboardMsg{text: "Z", vim: "P"})
	if got := m.input.Value(); got != "Zaab"+"X" {
		t.Errorf("copied elsewhere: got %q, want ZaabX", got)
	}
	// line yank comes back line-wise
	m = step(m, key("y"), key("y"))
	m = step(m, clipboardMsg{text: clipped, vim: "p"})
	if got := m.input.Value(); got != "ZaabX\nZaabX" {
		t.Errorf("yy p: got %q", got)
	}
	m = step(m, key("u"))
	// a copied file is attached whatever the register holds
	path := filepath.Join(t.TempDir(), "doc.pdf")
	if err := os.WriteFile(path, []byte("%PDF"), 0o600); err != nil {
		t.Fatal(err)
	}
	m = step(m, clipboardMsg{files: []attachment{{path: path}}, vim: "p"})
	if len(m.attach) != 1 {
		t.Errorf("a copied file should be attached, got %d", len(m.attach))
	}
}
