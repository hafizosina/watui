package ui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/zhenzhu/watui/internal/media"
	"github.com/zhenzhu/watui/internal/wa"
)

type infoState struct {
	title   string
	loading bool
	err     error
	person  *wa.Person
	group   *wa.Group
	cursor  int
	offset  int
}

type infoLoadedMsg struct {
	person *wa.Person
	group  *wa.Group
	err    error
}

const infoPicCols, infoPicRows = 24, 10

func (m *Model) openInfo(title string, load func(ctx context.Context) infoLoadedMsg) tea.Cmd {
	m.info = &infoState{title: title, loading: true}
	m.menu = nil
	m.enterNormal()
	m.focus = focusChat
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return load(ctx)
	}
}

func (m *Model) personInfoCmd(jid, title string) tea.Cmd {
	client := m.wa
	return m.openInfo(title, func(ctx context.Context) infoLoadedMsg {
		p, err := client.PersonInfo(ctx, jid)
		return infoLoadedMsg{person: &p, err: err}
	})
}

func (m *Model) myInfoCmd() tea.Cmd {
	client := m.wa
	return m.openInfo("My profile", func(ctx context.Context) infoLoadedMsg {
		p, err := client.MyInfo(ctx)
		return infoLoadedMsg{person: &p, err: err}
	})
}

func (m *Model) groupInfoCmd(jid, title string) tea.Cmd {
	client := m.wa
	return m.openInfo(title, func(ctx context.Context) infoLoadedMsg {
		g, err := client.GroupInfo(ctx, jid)
		return infoLoadedMsg{group: &g, err: err}
	})
}

func (m Model) infoLoaded(msg infoLoadedMsg) (tea.Model, tea.Cmd) {
	if m.info == nil {
		return m, nil
	}
	m.info.loading = false
	m.info.err = msg.err
	m.info.person, m.info.group = msg.person, msg.group
	if msg.err == nil && m.images != nil {
		jid, pic := "", ""
		if msg.person != nil {
			jid, pic = msg.person.JID, msg.person.PictureID
		} else if msg.group != nil {
			jid, pic = msg.group.JID, msg.group.PictureID
		}
		if pic != "" {
			m.images.Request(jid, wa.PictureKey(jid, pic), media.Full, media.Box{MaxCols: infoPicCols, MaxRows: infoPicRows})
		}
	}
	return m, nil
}

func (m Model) infoKey(key string) (tea.Model, tea.Cmd, bool) {
	in := m.info
	n := 0
	if in.group != nil {
		n = len(in.group.Members)
	}
	switch key {
	case "esc", "q":
		m.info = nil
		m.renderMessages(false)
		return m, nil, true
	case "j", "down":
		in.cursor = min(max(0, n-1), in.cursor+1)
	case "k", "up":
		in.cursor = max(0, in.cursor-1)
	case "ctrl+d":
		in.cursor = min(max(0, n-1), in.cursor+10)
	case "ctrl+u":
		in.cursor = max(0, in.cursor-10)
	case "G":
		in.cursor = max(0, n-1)
	case "o":
		if in.group != nil {
			g := in.group
			m.info = nil
			m.startChatWith(g.JID, g.Name)
			return m, nil, true
		}
		fallthrough
	case "enter":
		if in.person != nil && !in.person.IsMe {
			p := in.person
			m.info = nil
			m.startChatWith(p.JID, p.Name)
			return m, nil, true
		}
		if in.group != nil && in.cursor < n {
			mem := in.group.Members[in.cursor]
			if !mem.IsMe {
				m.info = nil
				m.startChatWith(mem.JID, mem.Name)
			}
		}
	case "y":
		if in.group != nil && in.cursor < n && in.group.Members[in.cursor].Phone != "" {
			return m, copyCmd(in.group.Members[in.cursor].Phone), true
		}
		if in.person != nil && in.person.Phone != "" {
			return m, copyCmd(in.person.Phone), true
		}
	case "space", " ":
		if in.group != nil && in.cursor < n && !in.group.Members[in.cursor].IsMe {
			mem := in.group.Members[in.cursor]
			return m, m.personInfoCmd(mem.JID, mem.Name), true
		}
	case "i", "a", "4":
		m.setFlash("this is a profile — press enter to message, esc to go back", false)
	default:
		return m, nil, false
	}
	return m, nil, true
}

func (m Model) infoTitle() string {
	return "[3] Info · " + sanitize(m.info.title)
}

func (m Model) infoFooter() string {
	if m.info.group != nil {
		return "o open group chat · j/k member · enter message member · space details · y copy · esc back"
	}
	if m.info.person != nil && !m.info.person.IsMe {
		return "enter message · y copy number · esc back"
	}
	return "y copy number · esc back"
}

func initials(name string) string {
	var out []rune
	for _, word := range strings.Fields(name) {
		for _, r := range word {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				out = append(out, unicode.ToUpper(r))
				break
			}
		}
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 0 {
		return "?"
	}
	return string(out)
}

func avatar(name, key string, cols, rows int) []string {
	bg := lipgloss.NewStyle().Background(senderColor(key)).Foreground(colOnAccent).Bold(true)
	blank := bg.Render(strings.Repeat(" ", cols))
	out := make([]string, rows)
	ini := initials(name)
	for i := range out {
		out[i] = blank
		if i == rows/2 {
			pad := (cols - lipgloss.Width(ini)) / 2
			out[i] = bg.Render(strings.Repeat(" ", pad) + ini + strings.Repeat(" ", cols-pad-lipgloss.Width(ini)))
		}
	}
	return out
}

func (m Model) pictureLines(jid, pic, name string) ([]string, int) {
	if pic != "" && m.images != nil {
		rows, _, _ := m.images.Lines(wa.PictureKey(jid, pic), media.Full)
		if len(rows) > 0 {
			return rows, lipgloss.Width(rows[0])
		}
	}
	return avatar(name, jid, 12, 6), 12
}

func sideBySide(left []string, leftW int, right []string) []string {
	n := max(len(left), len(right))
	out := make([]string, n)
	for i := 0; i < n; i++ {
		l := strings.Repeat(" ", leftW)
		if i < len(left) {
			l = left[i] + strings.Repeat(" ", max(0, leftW-lipgloss.Width(left[i])))
		}
		r := ""
		if i < len(right) {
			r = right[i]
		}
		out[i] = "  " + l + "   " + r
	}
	return out
}

func section(title string) string {
	return "  " + lipgloss.NewStyle().Foreground(colBlue).Bold(true).Render(title)
}

func wrapLines(text string, w, maxLines int) []string {
	var out []string
	for _, para := range strings.Split(strings.TrimSpace(sanitize(text)), "\n") {
		wrapped := lipgloss.NewStyle().Width(max(10, w)).Render(para)
		for _, l := range strings.Split(wrapped, "\n") {
			out = append(out, "  "+styleText.Render(strings.TrimRight(l, " ")))
		}
	}
	if maxLines > 0 && len(out) > maxLines {
		out = append(out[:maxLines], "  "+styleDim.Render("…"))
	}
	return out
}

func button(label, key string) string {
	b := lipgloss.NewStyle().Background(colGreen).Foreground(colOnAccent).Bold(true).Padding(0, 1).Render(label)
	return b + styleDim.Render("  "+key)
}

func badge(text string, bg lipgloss.Color) string {
	return lipgloss.NewStyle().Background(bg).Foreground(colOnAccent).Bold(true).Padding(0, 1).Render(text)
}

func (m Model) infoView(w, h int) string {
	in := m.info
	switch {
	case in.loading:
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, styleDim.Render("◌ loading details…"))
	case in.err != nil:
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, styleError.Render(in.err.Error()))
	}
	if p := in.person; p != nil {
		return m.personView(p, w)
	}
	return m.groupView(in, w, h)
}

func (m Model) personView(p *wa.Person, w int) string {
	name := p.Name
	if name == "" {
		name = p.PushName
	}
	if name == "" {
		name = p.Phone
	}
	pic, picW := m.pictureLines(p.JID, p.PictureID, name)
	textW := max(10, w-picW-8)

	right := []string{"", lipgloss.NewStyle().Foreground(colFg).Bold(true).Render(ansi.Truncate(sanitize(name), textW, "…"))}
	sub := p.Phone
	if p.PushName != "" && p.PushName != name {
		sub = "~" + sanitize(p.PushName) + "  ·  " + p.Phone
	}
	right = append(right, styleDim.Render(ansi.Truncate(sub, textW, "…")))
	if p.IsMe {
		right = append(right, "", badge("you", colGreen))
	} else {
		right = append(right, "", button(iconChat+" Message "+ansi.Truncate(sanitize(name), max(8, textW-20), "…"), "enter"))
	}
	if p.Business != "" {
		right = append(right, "", badge("business", colYellow)+" "+styleText.Render(ansi.Truncate(sanitize(p.Business), textW-12, "…")))
	}

	lines := append([]string{""}, sideBySide(pic, picW, right)...)
	lines = append(lines, "", section("About"))
	if strings.TrimSpace(p.About) == "" {
		lines = append(lines, "  "+styleDim.Render("—"))
	} else {
		lines = append(lines, wrapLines(p.About, w-4, 6)...)
	}
	if !p.IsMe {
		lines = append(lines, "", section(fmt.Sprintf("Groups in common (%d)", len(p.Groups))))
		if len(p.Groups) == 0 {
			lines = append(lines, "  "+styleDim.Render("none"))
		}
		for i, g := range p.Groups {
			if i == 8 {
				lines = append(lines, "  "+styleDim.Render(fmt.Sprintf("… and %d more", len(p.Groups)-8)))
				break
			}
			lines = append(lines, "  "+styleDim.Render(iconGroup+" ")+styleText.Render(ansi.Truncate(sanitize(g), w-8, "…")))
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) groupView(in *infoState, w, h int) string {
	g := in.group
	pic, picW := m.pictureLines(g.JID, g.PictureID, g.Name)
	textW := max(10, w-picW-8)
	admins := 0
	for _, mem := range g.Members {
		if mem.Admin {
			admins++
		}
	}
	right := []string{"",
		lipgloss.NewStyle().Foreground(colFg).Bold(true).Render(ansi.Truncate(sanitize(g.Name), textW, "…")),
		styleDim.Render(fmt.Sprintf("Group · %d members · %d admins", len(g.Members), admins)),
	}
	if !g.Created.IsZero() {
		created := "Created " + g.Created.Local().Format("02 Jan 2006")
		if g.CreatedBy != "" {
			created += " by " + sanitize(g.CreatedBy)
		}
		right = append(right, styleDim.Render(ansi.Truncate(created, textW, "…")))
	}
	var flags []string
	if g.Announce {
		flags = append(flags, badge("admins only send", colOrange))
	}
	if g.Locked {
		flags = append(flags, badge("admins only edit", colOrange))
	}
	if len(flags) > 0 {
		right = append(right, "", strings.Join(flags, " "))
	}
	right = append(right, "", button(iconChat+" Open group chat", "o"))

	lines := append([]string{""}, sideBySide(pic, picW, right)...)
	if strings.TrimSpace(g.Topic) != "" {
		lines = append(lines, "", section("Description"))
		lines = append(lines, wrapLines(g.Topic, w-4, 5)...)
	}
	lines = append(lines, "", section(fmt.Sprintf("Members (%d)", len(g.Members))))

	nameW := 0
	for _, mem := range g.Members {
		nameW = max(nameW, lipgloss.Width(sanitize(mem.Name)))
	}
	nameW = min(nameW, max(10, w/2-6))
	room := max(3, h-len(lines))
	if in.cursor < in.offset {
		in.offset = in.cursor
	}
	if in.cursor >= in.offset+room {
		in.offset = in.cursor - room + 1
	}
	end := min(len(g.Members), in.offset+room)
	for i := in.offset; i < end; i++ {
		mem := g.Members[i]
		sel := i == in.cursor && m.focus == focusChat && m.mode == modeNormal
		bg := lipgloss.NewStyle()
		marker := "  "
		if sel {
			bg = bg.Background(colHighlight)
			marker = lipgloss.NewStyle().Foreground(colBlue).Bold(true).Render("▌ ")
		}
		name := ansi.Truncate(sanitize(mem.Name), nameW, "…")
		nameStyle := bg.Foreground(senderColor(mem.JID)).Bold(sel)
		if mem.IsMe {
			nameStyle = bg.Foreground(colBlue).Bold(true)
		}
		row := nameStyle.Render(name) + bg.Render(strings.Repeat(" ", nameW-lipgloss.Width(name)+2)) + bg.Foreground(colComment).Render(fmt.Sprintf("%-16s", mem.Phone))
		switch {
		case mem.Owner:
			row += " " + badge("owner", colMagenta)
		case mem.Admin:
			row += " " + badge("admin", colGreen)
		}
		lines = append(lines, marker+row)
	}
	if end < len(g.Members) {
		lines[len(lines)-1] = "  " + styleDim.Render(fmt.Sprintf("↓ %d more", len(g.Members)-end+1))
	}
	return strings.Join(lines, "\n")
}
