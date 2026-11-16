package ui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/zhenzhu/watui/internal/store"
	"github.com/zhenzhu/watui/internal/wa"
)

func TestMentionQuery(t *testing.T) {
	cases := map[string]struct {
		query string
		ok    bool
	}{
		"@":              {"", true},
		"hi @Gari":       {"Gari", true},
		"line\n@sa":      {"sa", true},
		"mail a@b":       {"", false},
		"hi @Gari Tam":   {"", false}, // a space ends it
		"no mention":     {"", false},
		"@Garincha_T.2-": {"Garincha_T.2-", true},
	}
	for text, want := range cases {
		e := newEditor()
		e.SetValue(text)
		e.CursorEnd()
		e.mode = vimInsert
		_, q, ok := e.mentionQuery()
		if q != want.query || ok != want.ok {
			t.Errorf("%q: got (%q, %t), want (%q, %t)", text, q, ok, want.query, want.ok)
		}
	}
}

var testMembers = []wa.Mentionable{
	{JID: "111@lid", User: "111", Name: "Andi", Phone: "+6283333333333"},
	{JID: "222@lid", User: "222", Name: "Garincha Tambunan Antam", Phone: "+6281299990000"},
	{JID: "333@lid", User: "333", Name: "Sari Dewi", Phone: "+6282222222222"},
	{JID: "444@lid", User: "444", Name: "Tamara"},
}

func TestMatchMembers(t *testing.T) {
	names := func(q string) []string {
		var out []string
		for _, m := range matchMembers(testMembers, q, 8) {
			out = append(out, m.Name)
		}
		return out
	}
	if got := names(""); len(got) != 4 {
		t.Errorf("empty query: %v", got)
	}
	// word start ("Tambunan", "Tamara") before plain contains
	if got, want := names("tam"), []string{"Garincha Tambunan Antam", "Tamara"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tam: %v, want %v", got, want)
	}
	if got, want := names("ewi"), []string{"Sari Dewi"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ewi: %v, want %v", got, want)
	}
	if got, want := names("62812999"), []string{"Garincha Tambunan Antam"}; !reflect.DeepEqual(got, want) {
		t.Errorf("number: %v, want %v", got, want)
	}
}

func TestMentionIsOneUnit(t *testing.T) {
	e := newEditor()
	e.mode = vimInsert
	e.SetValue("cek @Fac")
	e.CursorEnd()
	from, _, _ := e.mentionQuery()
	e.insertAtom(from, atom{label: "@Fachri Mobile WGS", user: "111", jid: "111@lid"})
	typeText(e, "ya")
	if got, want := e.Value(), "cek @Fachri Mobile WGS ya"; got != want {
		t.Fatalf("Value = %q, want %q", got, want)
	}
	if text, jids := e.SendValue(); text != "cek @111 ya" || !reflect.DeepEqual(jids, []string{"111@lid"}) {
		t.Fatalf("SendValue = %q %v", text, jids)
	}
	// the cursor steps over the whole name
	press(e, "left", "left", "left", "left")
	if e.cur != 4 || !isAtom(e.buf[e.cur]) {
		t.Fatalf("after 4× left: cur=%d on %q, want the mention at 4", e.cur, string(e.buf[e.cur]))
	}
	// typing next to it doesn't change the name
	typeText(e, "x")
	press(e, "right", "backspace") // backspace right after the mention deletes all of it
	if got, want := e.Value(), "cek x ya"; got != want {
		t.Fatalf("after backspace: %q, want %q", got, want)
	}
	if _, jids := e.SendValue(); jids != nil {
		t.Errorf("deleted mention still sent: %v", jids)
	}
	// x on it in NORMAL deletes the whole mention too; u brings it back as a mention
	e.SetValue("hi ")
	e.CursorEnd()
	e.insertAtom(3, atom{label: "@Fachri Mobile WGS", user: "111", jid: "111@lid"})
	press(e, "esc")
	e.cur = 3
	press(e, "x")
	if got := e.Value(); got != "hi  " {
		t.Errorf("after x: %q", got)
	}
	press(e, "u")
	if _, jids := e.SendValue(); len(jids) != 1 {
		t.Errorf("after undo: jids = %v", jids)
	}
}

func TestMentionSurvivesEditor(t *testing.T) {
	e := newEditor()
	e.mode = vimInsert
	e.SetValue("@An")
	e.CursorEnd()
	e.insertAtom(0, atom{label: "@Andi", user: "111", jid: "111@lid"})
	e.SetValueKeepMentions("halo @Andi, dan @Andika")
	text, jids := e.SendValue()
	if text != "halo @111, dan @Andika" || len(jids) != 1 {
		t.Errorf("SendValue = %q %v", text, jids)
	}
}

func TestHighlightMentions(t *testing.T) {
	line := highlightMentions("cek @Andi Wijaya dan @You ya", []string{"@Andi", "@Andi Wijaya", "@You"})
	if ansi.Strip(line) != "cek @Andi Wijaya dan @You ya" {
		t.Fatalf("text changed: %q", ansi.Strip(line))
	}
	if !strings.Contains(line, styleMention.Render("@Andi Wijaya")) || !strings.Contains(line, styleMentionMe.Render("@You")) {
		t.Errorf("not highlighted: %q", line)
	}
}

func typeText(e *editor, s string) {
	for _, r := range s {
		e.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func press(e *editor, keys ...string) {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "left":
			msg = tea.KeyMsg{Type: tea.KeyLeft}
		case "right":
			msg = tea.KeyMsg{Type: tea.KeyRight}
		case "backspace":
			msg = tea.KeyMsg{Type: tea.KeyBackspace}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		e.HandleKey(msg)
	}
}

func TestMentionPopupInsertsName(t *testing.T) {
	m := modelWithContacts(t)
	group := "120363001@g.us"
	m.openJID = group
	m.focus = focusChat
	m.members[group] = testMembers
	m.enterInsert()
	for _, r := range "minta cek @tam" {
		m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, _, list, active := m.mentionSuggestions()
	if !active || len(list) != 2 {
		t.Fatalf("popup: active=%t list=%v", active, list)
	}
	if !strings.Contains(ansi.Strip(m.View()), "Mention @tam") {
		t.Error("popup not drawn")
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyCtrlN}, tea.KeyMsg{Type: tea.KeyTab})
	if got, want := m.input.Value(), "minta cek @Tamara "; got != want {
		t.Fatalf("compose = %q, want %q", got, want)
	}
	if _, _, _, active := m.mentionSuggestions(); active {
		t.Error("popup still open after insert")
	}
	if !strings.Contains(m.input.View(), styleMention.Render("@Tamara")) {
		t.Error("mention not highlighted in Compose")
	}
	text, jids := m.input.SendValue()
	if text != "minta cek @444 " || !reflect.DeepEqual(jids, []string{"444@lid"}) {
		t.Errorf("send: %q %v", text, jids)
	}
	// esc hides the popup, keeps INSERT
	for _, r := range " @sa" {
		m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m = step(m, key("esc"))
	if _, _, _, active := m.mentionSuggestions(); active || m.input.mode != vimInsert {
		t.Errorf("after esc: popup=%t mode=%v", active, m.input.mode)
	}
}

func TestHighlightKeepsLayout(t *testing.T) {
	// the bug only shows with colors on: trailing padding hides behind escape codes
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)
	m := modelWithContacts(t)
	text := "itu @Nden BE WGS. coba test nden, ini agak panjang supaya baris kedua juga kena wrap di panel pesan"
	for _, fromMe := range []bool{false, true} {
		msg := store.Message{ID: "x", FromMe: fromMe, Text: text}
		plain := m.renderText(msg, text, 60)
		marked := m.renderText(msg, text, 60, "@Nden BE WGS")
		for i, l := range strings.Split(marked, "\n") {
			if strings.HasSuffix(ansi.Strip(l), " ") {
				t.Errorf("fromMe=%t line %d has trailing padding: %q", fromMe, i, ansi.Strip(l))
			}
		}
		if ansi.Strip(marked) != ansi.Strip(plain) {
			t.Errorf("fromMe=%t: highlighting changed the layout\nplain:  %q\nmarked: %q", fromMe, ansi.Strip(plain), ansi.Strip(marked))
		}
		if !strings.Contains(marked, styleMention.Render("@Nden BE WGS")) {
			t.Errorf("fromMe=%t: not highlighted", fromMe)
		}
	}
}
