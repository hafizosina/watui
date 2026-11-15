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
	chatsH := mainH - m.profilesH()
	msgsH := mainH - m.composeH()

	left := lipgloss.JoinVertical(lipgloss.Left,
		panel(m.profilesTitle(), m.profilesFooter(), m.profilesView(listW-2), listW, m.profilesH(), m.mode == modeNormal && m.focus == focusProfiles),
		panel(m.chatsTitle(), m.chatsFooter(), m.listView(listW-2), listW, chatsH, m.mode != modeInsert && m.focus == focusList),
	)
	var right string
	if m.showHelp {
		right = panel("Keybindings", "any key to close", helpView(chatW-2), chatW, mainH, true)
	} else if m.info != nil {
		right = panel(m.infoTitle(), m.infoFooter(), m.infoView(chatW-2, mainH-2), chatW, mainH, m.mode == modeNormal && m.focus == focusChat)
	} else {
		right = lipgloss.JoinVertical(lipgloss.Left,
			panel(m.messagesTitle(), m.messagesFooter(), m.messagesView(chatW-2, msgsH-2), chatW, msgsH, m.mode != modeInsert && m.focus == focusChat),
			panel(m.composeTitle(), m.composeFooter(), m.input.View(), chatW, m.composeH(), m.mode == modeInsert),
		)
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	if m.mode == modeInsert && m.info == nil && !m.showHelp {
		if _, query, list := m.emojiSuggestions(); len(list) > 0 {
			box, bw, bh := m.emojiView(query, list)
			body = overlay(body, box, min(listW+2, max(0, m.width-bw)), max(0, mainH-m.composeH()-bh))
		}
	}
	if m.mode == modeCommand {
		if list := m.commandSuggestions(m.cmdTyped); len(list) > 0 {
			box, _, bh := m.suggestionView(list)
			body = overlay(body, box, 0, max(0, mainH-bh))
		}
	}
	if m.stickers != nil {
		box, bw, bh := m.stickerView()
		body = overlay(body, box, max(0, (m.width-bw)/2), max(0, (mainH-bh)/2))
	}
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

func (m Model) chatsTitle() string {
	title := "[2] Chats"
	if m.showArchived {
		title = "[2] Archived"
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
		return "[3] Messages"
	}
	chat, _ := m.openChat()
	title := "[3] " + m.names[m.openJID]
	if chat.IsGroup {
		title += " · group"
	}
	if chat.Archived {
		title += " · archived"
	}
	if chat.Muted() {
		title += " " + styleMuted.Render(iconMute+" muted")
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
		return "[4] Editing message · esc in normal mode cancels"
	case m.replyTo != nil:
		who := "You"
		if !m.replyTo.FromMe {
			who = m.wa.ContactName(m.replyTo.SenderJID, m.replyTo.SenderName)
		}
		return "[4] Replying to " + sanitize(who) + ": " + strings.ReplaceAll(sanitize(m.replyTo.Text), "\n", " ")
	}
	return "[4] Compose"
}

func vimStyle(v vimMode) lipgloss.Style {
	base := lipgloss.NewStyle().Foreground(colOnAccent).Bold(true).Padding(0, 1)
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

func (m Model) specialRow(c store.Chat, selected bool, w int) string {
	bg := lipgloss.NewStyle()
	if selected {
		bg = bg.Background(colHighlight)
		if m.focus != focusList || m.mode == modeInsert {
			bg = bg.Background(colGutter)
		}
	}
	text, hint := iconArchive+"  Archived", fmt.Sprintf("%d ", c.Unread)
	if c.JID == rowBack {
		text, hint = "←  Back to chats", ""
	}
	line := bg.Foreground(colBlue).Bold(true).Render(" "+text) + bg.Render(strings.Repeat(" ", max(1, w-lipgloss.Width(text)-1-lipgloss.Width(hint)))) + bg.Foreground(colComment).Render(hint)
	sub := bg.Foreground(colComment).Render(" " + map[bool]string{true: "enter to return", false: "enter to open · A also toggles"}[c.JID == rowBack])
	return line + "\n" + padRow(sub, w, bg)
}

func (m Model) chatRow(c store.Chat, selected bool, w int) string {
	if isSpecialRow(c.JID) {
		return m.specialRow(c, selected, w)
	}
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
	pinIcon, muteIcon := "", ""
	if c.Pinned {
		pinIcon = " " + iconPin
	}
	if c.Muted() {
		muteIcon = " " + iconMute
	}
	line1 := paint(lipgloss.NewStyle(), " ") + paint(nameStyle, name) + paint(styleDim, pinIcon) + paint(styleMuted, muteIcon) +
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

	replyCount := map[string]int{}
	for _, msg := range m.messages {
		if msg.QuoteID != "" && !msg.Deleted {
			replyCount[msg.QuoteID]++
		}
	}
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
		key := m.blockKey(msg, newGroup, w, replyCount[msg.ID])
		var block []string
		if c, ok := m.blocks[msg.ID]; ok && c.key == key && key != "" {
			block = c.lines
			if c.foldable {
				m.foldable[msg.ID] = true
			}
		} else {
			bodyW := w
			if msg.FromMe {
				bodyW = max(24, w*7/10)
			}
			var content []string
			if newGroup {
				nameStyle := lipgloss.NewStyle().Foreground(senderColor(msg.SenderJID)).Bold(true)
				if msg.FromMe {
					nameStyle = styleMe
				}
				content = append(content, nameStyle.Render(nameOf(msg))+" "+styleDim.Render(msg.TS.Local().Format("15:04")))
			}
			if msg.Forwarded && !msg.Deleted {
				content = append(content, styleDim.Italic(true).Render("↪ forwarded"))
			}
			if msg.QuoteID != "" && !msg.Deleted {
				qbar := lipgloss.NewStyle().Foreground(senderColor(msg.QuoteSender)).Render("┃ ")
				who := lipgloss.NewStyle().Foreground(senderColor(msg.QuoteSender)).Bold(true).Render(nameOfJID(msg.QuoteSender, ""))
				quoted := ansi.Truncate(strings.ReplaceAll(sanitize(msg.QuoteText), "\n", " "), max(10, bodyW-8), "…")
				content = append(content, qbar+who, qbar+styleDim.Render(quoted))
			}
			q, show := m.shownImages[msg.ID]
			for _, l := range strings.Split(m.renderBody(msg, bodyW, show, q), "\n") {
				content = append(content, strings.TrimPrefix(l, "   "))
			}
			if r := reactionLine(msg.Reactions); r != "" {
				content = append(content, r)
			}
			if n := replyCount[msg.ID]; n > 0 {
				label := "↩ 1 reply"
				if n > 1 {
					label = fmt.Sprintf("↩ %d replies", n)
				}
				content = append(content, styleDim.Render(label))
			}
			block = m.bubble(content, msg, w)
			if key != "" {
				m.blocks[msg.ID] = cachedBlock{key: key, lines: block, foldable: m.foldable[msg.ID]}
			}
		}
		block = append([]string(nil), block...)
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

type cachedBlock struct {
	key      string
	lines    []string
	foldable bool
}

// blockKey fingerprints everything that changes how a message looks; "" means don't cache.
func (m Model) blockKey(msg store.Message, newGroup bool, w, replies int) string {
	if m.player != nil {
		if st, ok := m.player.State(); ok && st.MsgID == msg.ID {
			return ""
		}
	}
	img := ""
	if q, ok := m.shownImages[msg.ID]; ok && m.images != nil {
		lines, status, _ := m.images.Lines(msg.ID, q)
		img = fmt.Sprintf("%d:%d:%s", q, len(lines), status)
		if len(lines) > 0 {
			img += lines[0][:min(len(lines[0]), 24)]
		}
	}
	reactions := make([]string, 0, len(msg.Reactions))
	for who, e := range msg.Reactions {
		reactions = append(reactions, who+"="+e)
	}
	sort.Strings(reactions)
	return fmt.Sprintf("%d|%t|%s|%d|%t|%t|%t|%t|%s|%s|%s|%s|%d|%t|%s|%s",
		w, newGroup, msg.Text, msg.Status, msg.Edited, msg.Deleted, msg.Starred, msg.Forwarded,
		msg.QuoteID, msg.QuoteText, strings.Join(reactions, ","), msg.MediaType, replies, m.expanded[msg.ID], img, msg.SenderName)
}

// bubble frames one message: others get a bar in their color on the left,
// your own messages sit on the right with a blue bar.
func (m Model) bubble(content []string, msg store.Message, w int) []string {
	out := make([]string, len(content))
	if !msg.FromMe {
		bar := lipgloss.NewStyle().Foreground(senderColor(msg.SenderJID)).Render("▎")
		for i, l := range content {
			out[i] = " " + bar + " " + l
		}
		return out
	}
	blockW := 0
	for _, l := range content {
		blockW = max(blockW, lipgloss.Width(l))
	}
	pad := max(0, w-blockW-2)
	bar := lipgloss.NewStyle().Foreground(colBlue).Render("▐")
	for i, l := range content {
		out[i] = " " + strings.Repeat(" ", pad) + l + strings.Repeat(" ", blockW-lipgloss.Width(l)) + " " + bar
	}
	return out
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
	return strings.Join(parts, "  ")
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
		if msg.MediaType == wa.MediaAudio {
			continue
		}
		q := m.imageQuality()
		if msg.MediaType == wa.MediaVideo && !strings.HasPrefix(msg.Text, "[gif]") {
			q = media.Thumbnail
		}
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
	if msg.MediaType == wa.MediaAudio {
		return m.audioLine(msg, text) + m.suffix(msg)
	}
	if msg.MediaType == wa.MediaDoc {
		return m.documentLines(msg, text, w)
	}
	if line, ok := callLine(text); ok {
		return "   " + line
	}
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

func (m Model) audioLine(msg store.Message, text string) string {
	label := strings.Trim(strings.SplitN(text, "]", 2)[0], "[")
	if m.player != nil {
		if st, ok := m.player.State(); ok && st.MsgID == msg.ID {
			icon := "▶"
			if st.Paused {
				icon = "⏸"
			}
			const barW = 24
			done := 0
			if st.Duration > 0 {
				done = int(float64(barW) * float64(st.Position) / float64(st.Duration))
			}
			done = max(0, min(barW-1, done))
			bar := lipgloss.NewStyle().Foreground(colGreen).Render(strings.Repeat("━", done)+"●") + styleDim.Render(strings.Repeat("─", barW-done-1))
			speed := ""
			if st.Speed != 1 {
				speed = styleKey.Render(fmt.Sprintf(" %.1g×", st.Speed))
			}
			return "   " + lipgloss.NewStyle().Foreground(colGreen).Bold(true).Render(icon) + " " + bar + " " +
				styleText.Render(clock(st.Position)+" / "+clock(st.Duration)) + speed
		}
	}
	return "   " + styleKey.Render("▷ ") + styleText.Render(label) + styleDim.Render("  · space → Play")
}

func (m Model) documentLines(msg store.Message, text string, w int) string {
	label, caption := text, ""
	if strings.HasPrefix(text, "[document:") {
		label, caption, _ = strings.Cut(strings.TrimPrefix(text, "[document:"), "]")
		label = strings.TrimSpace(label)
	}
	line := "   " + styleKey.Render(iconFile+" ") + styleText.Bold(true).Render(ansi.Truncate(label, max(10, w-24), "…")) +
		styleDim.Render("  · space → Open") + m.suffix(msg)
	if caption = strings.TrimSpace(caption); caption != "" {
		return line + "\n" + m.renderText(store.Message{}, caption, w)
	}
	return line
}

func clock(d time.Duration) string {
	s := int(d.Seconds())
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func captionOf(msg store.Message) string {
	text := sanitize(msg.Text)
	for _, tag := range []string{"[image]", "[sticker]"} {
		if strings.HasPrefix(text, tag) {
			return strings.TrimSpace(strings.TrimPrefix(text, tag))
		}
	}
	if msg.MediaType == wa.MediaVideo && strings.HasPrefix(text, "[") {
		label, caption, _ := strings.Cut(strings.TrimPrefix(text, "["), "]")
		out := styleKey.Render("▶ "+label) + styleDim.Render("  · space → Play")
		if caption = strings.TrimSpace(caption); caption != "" {
			out += "\n" + caption
		}
		return out
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
	if banner, ok := m.ringBanner(); ok {
		b := lipgloss.NewStyle().Background(colRed).Foreground(colOnAccent).Bold(true)
		return b.Render(ansi.Truncate(banner, m.width, "…") + strings.Repeat(" ", max(0, m.width-lipgloss.Width(banner))))
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
	case m.focus == focusProfiles:
		return [][2]string{{"Profiles", "enter"}, {"Actions", "space"}, {"Add", "a"}, {"Next/prev profile", "gt/gT"}, {"Chats", "^j"}}
	case m.focus == focusList:
		return [][2]string{{"Open", "enter"}, {"Actions", "space"}, {"New chat", "n"}, {"Write", "i"}, {"Search", "/"}, {"Next/prev chat", "J/K"}, {"Archived", "A"}, {"Command", ":"}, {"Keybindings", "?"}}
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
		{"n", "new chat / new group (search contacts or type a number)"},
		{"space", "chat menu: pin, mute, archive, read/unread, delete, leave"},
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
		{" ", "play/pause o · speed . · stop x · save S · open image o"},
		{" ", "copy y · forward f · star s · pin p · delete d"},
		{"", ""},
		{"", "Global"},
		{"1 / 2 / 3 / 4", "focus profiles / chats / messages / compose"},
		{"gt / gT", "next / previous profile"},
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
		{":theme [name]", "pick a color theme (j/k previews)"},
		{":new", "new chat / new group"},
		{":set images=off", "always | thumbnail | off"},
		{":set", "show settings"},
		{":read", "mark chat read"},
		{":pause :speed :stop", "control the playing voice note"},
		{":profile <name>", "switch profile"},
		{":profile add <name>", "link another WhatsApp account (QR)"},
		{":profile rename <name> <label>", "change how a profile is shown"},
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

func padRow(s string, w int, bg lipgloss.Style) string {
	return s + bg.Render(strings.Repeat(" ", max(0, w-lipgloss.Width(s))))
}

func callLine(text string) (string, bool) {
	if !strings.HasPrefix(text, "[") || !strings.HasSuffix(text, " call]") {
		return "", false
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(text, "["), "]")
	switch {
	case strings.HasPrefix(inner, "missed "):
		return lipgloss.NewStyle().Foreground(colRed).Render(iconPhone + " Missed " + strings.TrimPrefix(inner, "missed ")), true
	case strings.HasPrefix(inner, "declined "):
		return lipgloss.NewStyle().Foreground(colOrange).Render(iconPhone + " Declined " + strings.TrimPrefix(inner, "declined ")), true
	case inner == "voice call" || inner == "video call":
		return lipgloss.NewStyle().Foreground(colGreen).Render(iconPhone + " " + strings.ToUpper(inner[:1]) + inner[1:] + " · answered on another device"), true
	}
	return "", false
}

func (m Model) emojiView(query string, list []emojiEntry) (string, int, int) {
	w := 32
	var rows []string
	for i, e := range list {
		sel := i == min(m.emojiSel, len(list)-1)
		bg := lipgloss.NewStyle()
		if sel {
			bg = bg.Background(colHighlight)
		}
		code := ansi.Truncate(":"+e.code+":", w-8, "…")
		row := bg.Render(" "+e.glyph+"  ") + bg.Foreground(colFg).Bold(sel).Render(code)
		rows = append(rows, row+bg.Render(strings.Repeat(" ", max(0, w-2-lipgloss.Width(row)))))
	}
	h := len(rows) + 2
	return panel("Emoji :"+query, "tab insert · ^n/^p", strings.Join(rows, "\n"), w, h, true), w, h
}
