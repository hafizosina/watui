package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func keysOf(seq ...string) []tea.KeyMsg {
	var out []tea.KeyMsg
	for _, s := range seq {
		switch s {
		case "esc":
			out = append(out, tea.KeyMsg{Type: tea.KeyEsc})
		case "enter":
			out = append(out, tea.KeyMsg{Type: tea.KeyEnter})
		case "ctrl+r":
			out = append(out, tea.KeyMsg{Type: tea.KeyCtrlR})
		default:
			for _, r := range s {
				out = append(out, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			}
		}
	}
	return out
}

func run(start string, mode vimMode, cur int, seq ...string) *editor {
	e := newEditor()
	e.SetWidth(40)
	e.SetValue(start)
	e.Focus(mode)
	e.cur = cur
	for _, k := range keysOf(seq...) {
		e.HandleKey(k)
	}
	return e
}

func TestEditor(t *testing.T) {
	cases := []struct {
		name  string
		start string
		cur   int
		mode  vimMode
		keys  []string
		want  string
	}{
		{"type", "", 0, vimInsert, []string{"halo bro"}, "halo bro"},
		{"jk to normal then x", "", 0, vimInsert, []string{"abc", "jk", "x"}, "ab"},
		{"j alone stays", "", 0, vimInsert, []string{"jalan"}, "jalan"},
		{"dw", "halo bro besok", 0, vimNormal, []string{"dw"}, "bro besok"},
		{"cw", "halo bro", 0, vimNormal, []string{"cw", "hai", "esc"}, "hai bro"},
		{"ciw middle", "halo bro besok", 6, vimNormal, []string{"ciw", "kak", "esc"}, "halo kak besok"},
		{"diw", "halo bro besok", 6, vimNormal, []string{"diw"}, "halo  besok"},
		{"dd", "a\nb\nc", 2, vimNormal, []string{"dd"}, "a\nc"},
		{"dd last line", "a\nb", 2, vimNormal, []string{"dd"}, "a"},
		{"yy p", "a\nb", 0, vimNormal, []string{"yy", "p"}, "a\na\nb"},
		{"D", "halo bro", 4, vimNormal, []string{"D"}, "halo"},
		{"A", "halo", 0, vimNormal, []string{"A", " bro", "esc"}, "halo bro"},
		{"o", "a", 0, vimNormal, []string{"o", "b", "esc"}, "a\nb"},
		{"O", "b", 0, vimNormal, []string{"O", "a", "esc"}, "a\nb"},
		{"u undo", "halo", 0, vimNormal, []string{"x", "x", "u"}, "alo"},
		{"ctrl+r redo", "halo", 0, vimNormal, []string{"x", "u", "ctrl+r"}, "alo"},
		{"v e d", "halo bro", 0, vimNormal, []string{"v", "e", "d"}, " bro"},
		{"V d", "a\nb\nc", 2, vimNormal, []string{"V", "d"}, "a\nc"},
		{"v y P", "halo", 0, vimNormal, []string{"v", "l", "y", "$", "p"}, "haloha"},
		{"v ~", "halo", 0, vimNormal, []string{"v", "$", "~"}, "HALO"},
		{"J", "a\nb", 0, vimNormal, []string{"J"}, "a b"},
		{"r", "halo", 0, vimNormal, []string{"rH"}, "Halo"},
		{"$ x", "halo", 0, vimNormal, []string{"$", "x"}, "hal"},
		{"b de", "halo bro", 7, vimNormal, []string{"b", "de"}, "halo "},
		{"gg", "a\nb", 2, vimNormal, []string{"gg", "x"}, "\nb"},
	}
	for _, c := range cases {
		e := run(c.start, c.mode, c.cur, c.keys...)
		if got := e.Value(); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestEditorModes(t *testing.T) {
	e := run("", 0, 0, "abc", "esc")
	if e.mode != vimNormal {
		t.Fatalf("esc in insert should go to normal, got %v", e.mode)
	}
	if e.HandleKey(keysOf("esc")[0]) != editLeave {
		t.Fatal("esc in normal should leave the compose box")
	}
	e = run("halo", vimNormal, 0, "v")
	if e.mode != vimVisual {
		t.Fatal("v should enter visual")
	}
	e.HandleKey(keysOf("esc")[0])
	if e.mode != vimNormal {
		t.Fatal("esc in visual should go back to normal")
	}
}

func TestEditorGrowShowsEarlierLines(t *testing.T) {
	e := newEditor()
	e.SetWidth(40)
	e.Focus(vimInsert)
	var lines []string
	for i := 1; i <= 10; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	e.SetValue(strings.Join(lines, "\n"))
	e.CursorEnd()
	if v := ansi.Strip(e.View()); hasRow(v, "line 1") || !hasRow(v, "line 10") {
		t.Fatalf("3-line box should show the end:\n%s", v)
	}
	e.SetHeight(14)
	v := ansi.Strip(e.View())
	if !hasRow(v, "line 1") || !hasRow(v, "line 10") {
		t.Errorf("a taller box should show every line, top included:\n%s", v)
	}
}

func hasRow(view, text string) bool {
	for _, row := range strings.Split(view, "\n") {
		if strings.TrimRight(strings.TrimSpace(row), "▏") == text {
			return true
		}
	}
	return false
}
