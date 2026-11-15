package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/zhenzhu/watui/internal/config"
	"github.com/zhenzhu/watui/internal/media"
	"github.com/zhenzhu/watui/internal/store"
	"github.com/zhenzhu/watui/internal/wa"
)

const (
	iconPin   = ""
	iconMute  = ""
	iconGroup = ""
)

const (
	statusPanelH = 3
)

func (m Model) composeH() int {
	return m.input.Height() + 2
}

func (m Model) mainH() int {
	return m.height - 1
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	listW := m.listWidth()
	chatW := m.width - listW
	mainH := m.mainH()
	chatsH := mainH - statusPanelH
	msgsH := mainH - m.composeH()

	left := lipgloss.JoinVertical(lipgloss.Left,
		panel("watui", "", m.statusView(), listW, statusPanelH, false),
		panel(m.chatsTitle(), m.chatsFooter(), m.listView(listW-2), listW, chatsH, m.mode != modeInsert && m.focus == focusList),
	)
	var right string
	if m.showHelp {
		right = panel("Keybindings", "any key to close", helpView(chatW-2), chatW, mainH, true)
	} else {
		right = lipgloss.JoinVertical(lipgloss.Left,
			panel(m.messagesTitle(), m.messagesFooter(), m.messagesView(chatW-2, msgsH-2), chatW, msgsH, m.mode != modeInsert && m.focus == focusChat),
			panel(m.composeTitle(), m.composeFooter(), m.input.View(), chatW, m.composeH(), m.mode == modeInsert),
		)
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	if m.menu != nil {
		box, bw, bh := m.menuView()
		body = overlay(body, box, max(0, (m.width-bw)/2), max(0, (mainH-bh)/2))
	}
	return body + "\n" + m.bottomLine()
}

func panel(title, footer, body string, w, h int, active bool) string {
	if w < 4 || h < 2 {
		return ""
	}
	iw := w - 2
	border := lipgloss.NewStyle().Foreground(colGutter)
	titleStyle := lipgloss.NewStyle().Foreground(colFgDark)
	if active {
		border = lipgloss.NewStyle().Foreground(colGreen).Bold(true)
		titleStyle = lipgloss.NewStyle().Foreground(colGreen).Bold(true)
	}

	title = ansi.Truncate(title, max(0, iw-2), "…")
	top := border.Render("╭─") + titleStyle.Render(title) + border.Render(strings.Repeat("─", max(0, iw-1-lipgloss.Width(title)))+"╮")

	footer = ansi.Truncate(footer, max(0, iw-2), "…")
	bottom := border.Render("╰" + strings.Repeat("─", max(0, iw-1-lipgloss.Width(footer))))
	if footer != "" {
		bottom += titleStyle.Render(footer)
	}
	bottom += border.Render("─╯")
	if footer == "" {
		bottom = border.Render("╰" + strings.Repeat("─", iw) + "╯")
	}

	lines := strings.Split(body, "\n")
	out := make([]string, 0, h)
	out = append(out, top)
	side := border.Render("│")
	for i := 0; i < h-2; i++ {
		line := ""
		if i < len(lines) {
			line = ansi.Truncate(lines[i], iw, "")
		}
		pad := max(0, iw-lipgloss.Width(line))
		out = append(out, side+line+strings.Repeat(" ", pad)+side)
	}
	out = append(out, bottom)
	return strings.Join(out, "\n")
}

func (m Model) statusView() string {
	state, syncing := m.wa.State()
	var conn string
	switch {
	case state == wa.StateLoggedOut:
		conn = lipgloss.NewStyle().Foreground(colRed).Render("● logged out")
	case state == wa.StateDisconnected:
		conn = lipgloss.NewStyle().Foreground(colRed).Render("○ offline")
	case state == wa.StateConnecting:
		conn = lipgloss.NewStyle().Foreground(colYellow).Render("◌ connecting")
	case syncing:
		conn = lipgloss.NewStyle().Foreground(colYellow).Render("◌ syncing")
	default:
		conn = lipgloss.NewStyle().Foreground(colGreen).Render("● online")
	}
	unread := 0
	for _, c := range m.allChats {
		if c.Unread > 0 && !c.Muted() && !c.Archived {
			unread++
		}
	}
	line := " " + conn
	if me := m.wa.Me(); me != "" {
		line += styleDim.Render(" · ") + styleText.Render(me)
	}
	if unread > 0 {
		line += styleDim.Render(" · ") + lipgloss.NewStyle().Foreground(colGreen).Render(fmt.Sprintf("%d unread", unread))
	}
	return line
}

func (m Model) chatsTitle() string {
	title := "[1] Chats"
	if m.showArchived {
		title = "[1] Archived"
	}
	if m.filter != "" {
		title += " /" + m.filter
	}
	return title
}

func (m Model) chatsFooter() string {
	if len(m.chats) == 0 {
		return ""
	}
	return fmt.Sprintf("%d of %d", m.cursor+1, len(m.chats))
}

func (m Model) messagesTitle() string {
	if m.openJID == "" {
		return "[2] Messages"
	}
	chat, _ := m.openChat()
	title := "[2] " + m.names[m.openJID]
	if chat.IsGroup {
		title += " · group"
	}
	if chat.Muted() {
		title += " · muted"
	}
	if chat.Archived {
		title += " · archived"
	}
	return title
}

func (m Model) messagesFooter() string {
	if m.openJID == "" || m.vp.AtBottom() {
		return ""
	}
	return fmt.Sprintf("%d%%", int(m.vp.ScrollPercent()*100))
}

func (m Model) composeTitle() string {
	switch {
	case m.editing != nil:
		return "[3] Editing message · esc in normal mode cancels"
	case m.replyTo != nil:
		who := "You"
		if !m.replyTo.FromMe {
			who = m.wa.ContactName(m.replyTo.SenderJID, m.replyTo.SenderName)
		}
		return "[3] Replying to " + sanitize(who) + ": " + strings.ReplaceAll(sanitize(m.replyTo.Text), "\n", " ")
	}
	return "[3] Compose"
}

func vimStyle(v vimMode) lipgloss.Style {
	base := lipgloss.NewStyle().Foreground(colBgDark).Bold(true).Padding(0, 1)
	switch v {
	case vimNormal:
		return base.Background(colBlue)
	case vimVisual, vimVisualLine:
		return base.Background(colMagenta)
	}
	return base.Background(colGreen)
}

func (m Model) composeFooter() string {
	if m.mode != modeInsert {
		return ""
	}
	switch m.input.mode {
	case vimNormal:
		if m.input.pending != "" {
			return m.input.pending + "…"
		}
		return "NORMAL · i insert · v visual · ctrl+s send · esc leave"
	case vimVisual, vimVisualLine:
		return m.input.mode.String() + " · d delete · y yank · c change · esc"
	}
	return "INSERT · ctrl+s send · enter newline · esc/jk normal"
}

func (m Model) listView(w int) string {
	if len(m.chats) == 0 {
		return styleDim.Render(" no chats yet")
	}
	var rows []string
	end := min(len(m.chats), m.offset+m.listRows())
	for i := m.offset; i < end; i++ {
		rows = append(rows, m.chatRow(m.chats[i], i == m.cursor, w))
	}
	return strings.Join(rows, "\n")
}

func (m Model) chatRow(c store.Chat, selected bool, w int) string {
	var bg lipgloss.TerminalColor = lipgloss.NoColor{}
	if selected {
		bg = colHighlight
		if m.focus != focusList || m.mode == modeInsert {
			bg = colGutter
		}
	}
	paint := func(s lipgloss.Style, text string) string {
		return s.Background(bg).Render(text)
	}

	icons := ""
	if c.Pinned {
		icons += " " + iconPin
	}
	if c.Muted() {
		icons += " " + iconMute
	}
	when := shortTime(c.LastTS)
	name := m.names[c.JID]
	if c.IsGroup {
		name = iconGroup + " " + name
	}
	name = ansi.Truncate(name, max(1, w-2-lipgloss.Width(when)-lipgloss.Width(icons)-1), "…")
	nameStyle := styleText
	if selected {
		nameStyle = nameStyle.Bold(true)
	}
	if c.Unread > 0 {
		nameStyle = styleUnreadName
	}
	gap1 := max(0, w-2-lipgloss.Width(name)-lipgloss.Width(icons)-lipgloss.Width(when))
	line1 := paint(lipgloss.NewStyle(), " ") + paint(nameStyle, name) + paint(styleDim, icons) +
		paint(lipgloss.NewStyle(), strings.Repeat(" ", gap1)) + paint(styleDim, when) + paint(lipgloss.NewStyle(), " ")

	badge := ""
	if c.Unread > 0 {
		n := fmt.Sprintf(" %d ", c.Unread)
		if c.Muted() {
			badge = styleBadgeMuted.Render(n)
		} else {
			badge = styleBadge.Render(n)
		}
	}
	preview := strings.ReplaceAll(sanitize(c.LastText), "\n", " ")
	preview = ansi.Truncate(preview, max(1, w-3-lipgloss.Width(badge)), "…")
	gap2 := max(0, w-2-lipgloss.Width(preview)-lipgloss.Width(badge))
	line2 := paint(lipgloss.NewStyle(), " ") + paint(styleDim, preview) +
		paint(lipgloss.NewStyle(), strings.Repeat(" ", gap2)) + badge + paint(lipgloss.NewStyle(), " ")
	return line1 + "\n" + line2
}

func (m Model) messagesView(w, h int) string {
	if m.openJID == "" {
		msg := styleTitle.Render("watui") + "\n\n" + styleDim.Render("j/k move · enter open · i write · ? keybindings")
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, msg)
	}
	return m.vp.View()
}

func (m *Model) renderMessages(forceBottom bool) {
	if m.vp.Width <= 0 {
		return
	}
	atBottom := forceBottom || m.vp.AtBottom()
	w := m.vp.Width - 2
	chat, _ := m.openChat()
	senderNames := map[string]string{}
	nameOfJID := func(jid, fallback string) string {
		if jid == m.wa.OwnJID() {
			return "You"
		}
		if !chat.IsGroup && jid == m.openJID {
			return m.names[m.openJID]
		}
		if n, ok := senderNames[jid]; ok {
			return n
		}
		n := sanitize(m.wa.ContactName(jid, fallback))
		senderNames[jid] = n
		return n
	}
	nameOf := func(msg store.Message) string {
		if msg.FromMe {
			return "You"
		}
		if !chat.IsGroup {
			return m.names[m.openJID]
		}
		return nameOfJID(msg.SenderJID, msg.SenderName)
	}

	m.shownImages = m.imageSet(w)
	m.foldable = map[string]bool{}
	showSel := m.focus == focusChat && m.mode == modeNormal
	marker := lipgloss.NewStyle().Foreground(colBlue).Bold(true).Render("▌")

	var lines []string
	m.spans = make([][2]int, len(m.messages))
	var prev *store.Message
	for i := range m.messages {
		msg := m.messages[i]
		newDay := prev == nil || !sameDay(prev.TS, msg.TS)
		if newDay {
			if prev != nil {
				lines = append(lines, "")
			}
			label := " " + msg.TS.Local().Format("Mon, 02 Jan 2006") + " "
			side := max(0, (w-lipgloss.Width(label))/2)
			lines = append(lines, " "+styleDay.Render(strings.Repeat("─", side)+label+strings.Repeat("─", side)))
		}
		newGroup := newDay || prev.SenderJID != msg.SenderJID || prev.FromMe != msg.FromMe || msg.TS.Sub(prev.TS) > 5*time.Minute
		if newGroup && !newDay {
			lines = append(lines, "")
		}
		var block []string
		if newGroup {
			nameStyle := lipgloss.NewStyle().Foreground(senderColor(msg.SenderJID)).Bold(true)
			if msg.FromMe {
				nameStyle = styleMe
			}
			block = append(block, " "+nameStyle.Render(nameOf(msg))+" "+styleDim.Render(msg.TS.Local().Format("15:04")))
		}
		if msg.Forwarded && !msg.Deleted {
			block = append(block, "   "+styleDim.Italic(true).Render("↪ forwarded"))
		}
		if msg.QuoteID != "" && !msg.Deleted {
			bar := lipgloss.NewStyle().Foreground(senderColor(msg.QuoteSender)).Render("┃ ")
			who := lipgloss.NewStyle().Foreground(senderColor(msg.QuoteSender)).Bold(true).Render(nameOfJID(msg.QuoteSender, ""))
			quoted := ansi.Truncate(strings.ReplaceAll(sanitize(msg.QuoteText), "\n", " "), max(10, w-8), "…")
			block = append(block, "   "+bar+who, "   "+bar+styleDim.Render(quoted))
		}
		q, show := m.shownImages[msg.ID]
		block = append(block, strings.Split(m.renderBody(msg, w, show, q), "\n")...)
		if r := reactionLine(msg.Reactions); r != "" {
			block = append(block, "   "+r)
		}
		if showSel && i == m.sel {
			for j, l := range block {
				block[j] = marker + strings.TrimPrefix(l, " ")
			}
		}
		m.spans[i] = [2]int{len(lines), len(lines) + len(block) - 1}
		lines = append(lines, block...)
		prev = &m.messages[i]
	}
	pad := max(0, m.vp.Height-len(lines))
	for i := range m.spans {
		m.spans[i][0] += pad
		m.spans[i][1] += pad
	}
	m.vp.SetContent(strings.Repeat("\n", pad) + strings.Join(lines, "\n"))
	if atBottom {
		m.vp.GotoBottom()
	}
}

func reactionLine(reactions map[string]string) string {
	if len(reactions) == 0 {
		return ""
	}
	counts := map[string]int{}
	var order []string
	for _, emoji := range reactions {
		if counts[emoji] == 0 {
			order = append(order, emoji)
		}
		counts[emoji]++
	}
	sort.Slice(order, func(i, j int) bool {
		if counts[order[i]] != counts[order[j]] {
			return counts[order[i]] > counts[order[j]]
		}
		return order[i] < order[j]
	})
	parts := make([]string, len(order))
	for i, e := range order {
		parts[i] = e + " " + styleDim.Render(fmt.Sprint(counts[e]))
	}
	return lipgloss.NewStyle().Background(colBgDark).Render(" ") + strings.Join(parts, "  ")
}

const maxImagesPerChat = 40

func (m Model) imageQuality() media.Quality {
	if m.cfg.Images == config.ImagesThumbnail {
		return media.Thumbnail
	}
	return media.Full
}

func (m Model) imageSet(w int) map[string]media.Quality {
	show := map[string]media.Quality{}
	if m.images == nil {
		return show
	}
	box := media.Box{MaxCols: max(4, min(m.cfg.ImageWidth, w-6)), MaxRows: m.cfg.ImageHeight}
	auto := 0
	for i := len(m.messages) - 1; i >= 0; i-- {
		msg := m.messages[i]
		if msg.MediaType == "" || msg.Deleted || m.forceHide[msg.ID] {
			continue
		}
		q := m.imageQuality()
		switch {
		case m.forceShow[msg.ID]:
			q = media.Full
		case m.cfg.Images == config.ImagesOff || auto >= maxImagesPerChat:
			continue
		default:
			auto++
		}
		show[msg.ID] = q
		m.images.Request(msg.ChatJID, msg.ID, q, box)
	}
	return show
}

func (m Model) renderBody(msg store.Message, w int, showImage bool, q media.Quality) string {
	if msg.Deleted {
		return "   " + styleDeleted.Render("message deleted")
	}
	text := sanitize(msg.Text)
	var picture []string
	if showImage {
		lines, status, ok := m.images.Lines(msg.ID, q)
		switch {
		case len(lines) > 0:
			for _, l := range lines {
				picture = append(picture, "   "+l)
			}
			text = captionOf(msg)
		case ok && status != "":
			text = sanitize(msg.Text) + " " + styleDim.Render("("+status+")")
		}
	}
	body := m.renderText(msg, text, w)
	if len(picture) == 0 {
		return body
	}
	if text == "" {
		return strings.Join(picture, "\n") + m.suffix(msg)
	}
	return strings.Join(picture, "\n") + "\n" + body
}

func captionOf(msg store.Message) string {
	text := sanitize(msg.Text)
	for _, tag := range []string{"[image]", "[sticker]", "[gif]"} {
		if strings.HasPrefix(text, tag) {
			return strings.TrimSpace(strings.TrimPrefix(text, tag))
		}
	}
	return text
}

func (m Model) suffix(msg store.Message) string {
	suffix := ""
	if msg.Edited {
		suffix += styleDim.Render(" (edited)")
	}
	if msg.Starred {
		suffix += lipgloss.NewStyle().Foreground(colYellow).Render(" ★")
	}
	if msg.FromMe {
		suffix += " " + statusIcon(msg.Status)
	}
	return suffix
}

func (m Model) renderText(msg store.Message, text string, w int) string {
	if text == "" {
		return ""
	}
	body := styleText.Width(max(10, w-4)).Render(text)
	suffix := m.suffix(msg)
	lines := strings.Split(strings.TrimRight(body, " \n"), "\n")
	for i := range lines {
		lines[i] = "   " + strings.TrimRight(lines[i], " ")
	}
	limit := m.cfg.CollapseLines
	if limit > 0 && len(lines) > limit+2 {
		m.foldable[msg.ID] = true
		if !m.expanded[msg.ID] {
			hidden := len(lines) - limit
			lines = append(lines[:limit], "   "+lipgloss.NewStyle().Foreground(colBlue).Render(fmt.Sprintf("⋯ %d more lines", hidden))+styleDim.Render(" · space → Expand"))
		}
	}
	lines[len(lines)-1] += suffix
	return strings.Join(lines, "\n")
}

func statusIcon(s store.Status) string {
	switch s {
	case store.StatusPending:
		return styleDim.Render("◷")
	case store.StatusSent:
		return styleDim.Render("✓")
	case store.StatusDelivered:
		return styleDim.Render("✓✓")
	case store.StatusRead:
		return styleRead.Render("✓✓")
	case store.StatusFailed:
		return styleFailed.Render("✗ failed")
	}
	return ""
}

func (m Model) bottomLine() string {
	switch m.mode {
	case modeCommand:
		return styleKey.Render(":") + m.prompt.View()
	case modeSearch:
		return styleKey.Render("/") + m.prompt.View()
	}
	right := modeStyle(m.mode).Render(m.mode.String())
	if m.mode == modeInsert {
		right = vimStyle(m.input.mode).Render(m.input.mode.String())
	}
	room := max(0, m.width-lipgloss.Width(right)-1)
	var left string
	if m.flash != "" && time.Now().Before(m.flashUntil) {
		if m.flashErr {
			left = styleError.Render(ansi.Truncate(m.flash, room, "…"))
		} else {
			left = lipgloss.NewStyle().Foreground(colYellow).Render(ansi.Truncate(m.flash, room, "…"))
		}
	} else {
		left = ansi.Truncate(hints(m.hintPairs()), room, "…")
	}
	gap := max(1, m.width-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", gap) + right
}

func (m Model) hintPairs() [][2]string {
	switch {
	case m.mode == modeInsert:
		switch m.input.mode {
		case vimNormal:
			return [][2]string{{"Send", "ctrl+s"}, {"Insert", "i a o"}, {"Visual", "v V"}, {"Delete", "x dd dw"}, {"Change", "cw ciw"}, {"Undo", "u"}, {"Leave", "esc"}}
		case vimVisual, vimVisualLine:
			return [][2]string{{"Delete", "d"}, {"Yank", "y"}, {"Change", "c"}, {"Case", "~ u U"}, {"Swap end", "o"}, {"Cancel", "esc"}}
		}
		return [][2]string{{"Send", "ctrl+s"}, {"Newline", "enter"}, {"Normal", "esc / jk"}, {"Editor", "ctrl+e"}}
	case m.focus == focusList:
		return [][2]string{{"Open", "enter"}, {"Write", "i"}, {"Search", "/"}, {"Next/prev chat", "J/K"}, {"Archived", "A"}, {"Command", ":"}, {"Keybindings", "?"}}
	}
	if m.menu != nil {
		return [][2]string{{"Move", "^n/^p"}, {"Pick", "enter"}, {"Close", "esc"}}
	}
	return [][2]string{{"Select", "j/k"}, {"Menu", "space"}, {"Write", "i"}, {"Editor", "e"}, {"Back", "q"}, {"Next/prev chat", "J/K"}, {"Keybindings", "?"}}
}

func hints(pairs [][2]string) string {
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = lipgloss.NewStyle().Foreground(colBlue).Render(p[0] + ": " + p[1])
	}
	return strings.Join(parts, styleDim.Render(" | "))
}

func helpView(w int) string {
	rows := [][2]string{
		{"", "Chats"},
		{"j / k", "move down / up"},
		{"gg / G", "first / last chat"},
		{"ctrl+d / ctrl+u", "half page down / up"},
		{"enter", "open chat"},
		{"i / a", "open chat and start typing"},
		{"/", "search chats (esc clears)"},
		{"A", "toggle archived chats"},
		{"", ""},
		{"", "Messages"},
		{"j / k", "select next / previous message"},
		{"ctrl+d / ctrl+u", "jump 10 messages"},
		{"gg / G", "oldest / newest"},
		{"space", "message menu"},
		{"i / a / enter", "start typing"},
		{"e", "write the message in $EDITOR"},
		{"q / esc", "back to chats"},
		{"", ""},
		{"", "Message menu"},
		{"ctrl+n / ctrl+p", "next / previous option"},
		{"enter", "pick option (or press its letter)"},
		{"esc", "close"},
		{"options", "expand/collapse z · reply r · edit e · react + · show/hide image v"},
		{" ", "open image o · copy y · forward f · star s · pin p · delete d"},
		{"", ""},
		{"", "Global"},
		{"1 / 2 / 3", "focus chats / messages / compose"},
		{"tab", "switch between chats and messages"},
		{"ctrl+h / ctrl+l", "chats / messages (then tmux pane)"},
		{"ctrl+j / ctrl+k", "messages ↔ compose (then tmux pane)"},
		{"J / K", "open next / previous chat"},
		{":", "command line"},
		{"?", "keybindings"},
		{"", ""},
		{"", "Compose (vim: INSERT / NORMAL / VISUAL)"},
		{"ctrl+s", "send (any mode)"},
		{"esc / jk", "insert → normal"},
		{"esc", "normal → leave compose"},
		{"h j k l w b e 0 ^ $ gg G", "move"},
		{"i a I A o O", "insert"},
		{"x X dd D dw diw cw ciw cc C s S", "delete / change"},
		{"yy Y yiw p P", "yank / paste"},
		{"u ctrl+r J ~ r", "undo / redo / join / case / replace"},
		{"v V", "visual / visual line (d y c ~ o)"},
		{"ctrl+e", "edit in $EDITOR"},
		{"", ""},
		{"", "Commands"},
		{":pin  :unpin", "pin chat"},
		{":mute  :unmute", "mute chat"},
		{":archive  :unarchive", "archive chat"},
		{":archived", "toggle archived view"},
		{":set images=off", "always | thumbnail | off"},
		{":set", "show settings"},
		{":read", "mark chat read"},
		{":q", "quit"},
	}
	var b strings.Builder
	for _, r := range rows {
		switch {
		case r[0] == "" && r[1] != "":
			b.WriteString(" " + lipgloss.NewStyle().Foreground(colGreen).Bold(true).Render(r[1]) + "\n")
		case r[0] == "":
			b.WriteString("\n")
		default:
			b.WriteString("   " + styleKey.Render(fmt.Sprintf("%-32s", r[0])) + styleText.Render(r[1]) + "\n")
		}
	}
	return b.String()
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Local().Date()
	by, bm, bd := b.Local().Date()
	return ay == by && am == bm && ad == bd
}

func shortTime(t time.Time) string {
	if t.IsZero() || t.Unix() <= 0 {
		return ""
	}
	now := time.Now()
	switch {
	case sameDay(t, now):
		return t.Local().Format("15:04")
	case sameDay(t, now.AddDate(0, 0, -1)):
		return "yesterday"
	case now.Sub(t) < 6*24*time.Hour:
		return t.Local().Format("Mon")
	}
	return t.Local().Format("02/01/06")
}
