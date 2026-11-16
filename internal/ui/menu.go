package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/zhenzhu/watui/internal/media"
	"github.com/zhenzhu/watui/internal/player"
	"github.com/zhenzhu/watui/internal/store"
	"github.com/zhenzhu/watui/internal/wa"
)

type menuItem struct {
	label  string
	key    string
	danger bool
	run    func(m *Model) tea.Cmd
}

type menu struct {
	title      string
	items      []menuItem
	cursor     int
	filterable bool
	filter     string
	dynamic    func(filter string) []menuItem
	preview    func(m *Model, it menuItem)
	cancel     func(m *Model)
}

const menuMaxRows = 12

func (mn *menu) visible() []menuItem {
	var out []menuItem
	if mn.dynamic != nil {
		out = append(out, mn.dynamic(mn.filter)...)
	}
	if !mn.filterable || mn.filter == "" {
		return append(out, mn.items...)
	}
	needle := strings.ToLower(mn.filter)
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, needle)
	if strings.HasPrefix(digits, "0") {
		digits = digits[1:]
	}
	for _, it := range mn.items {
		label := strings.ToLower(it.label)
		if strings.Contains(label, needle) || (len(digits) >= 4 && strings.Contains(label, digits)) {
			out = append(out, it)
		}
	}
	return out
}

func (m Model) menuKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	mn := m.menu
	items := mn.visible()
	key := k.String()
	switch key {
	case "esc", "ctrl+c", "ctrl+[":
		if mn.cancel != nil {
			mn.cancel(&m)
		}
		m.menu = nil
		return m, nil
	case "ctrl+n", "down", "tab":
		if len(items) > 0 {
			mn.cursor = (mn.cursor + 1) % len(items)
			m.previewMenu()
		}
		return m, nil
	case "ctrl+p", "up", "shift+tab":
		if len(items) > 0 {
			mn.cursor = (mn.cursor - 1 + len(items)) % len(items)
			m.previewMenu()
		}
		return m, nil
	case "enter":
		if mn.cursor < len(items) {
			return m.runItem(items[mn.cursor])
		}
		return m, nil
	}
	if mn.filterable {
		switch {
		case key == "backspace":
			if mn.filter != "" {
				r := []rune(mn.filter)
				mn.filter = string(r[:len(r)-1])
				mn.cursor = 0
			}
		case k.Type == tea.KeyRunes || key == " ":
			mn.filter += string(k.Runes)
			if key == " " {
				mn.filter += " "
			}
			mn.cursor = 0
		}
		return m, nil
	}
	switch key {
	case "j":
		if len(items) > 0 {
			mn.cursor = (mn.cursor + 1) % len(items)
			m.previewMenu()
		}
		return m, nil
	case "k":
		if len(items) > 0 {
			mn.cursor = (mn.cursor - 1 + len(items)) % len(items)
			m.previewMenu()
		}
		return m, nil
	case "q":
		m.menu = nil
		return m, nil
	}
	for _, it := range items {
		if it.key != "" && it.key == key {
			return m.runItem(it)
		}
	}
	return m, nil
}

func (m *Model) previewMenu() {
	mn := m.menu
	if mn == nil || mn.preview == nil {
		return
	}
	if items := mn.visible(); mn.cursor < len(items) {
		mn.preview(m, items[mn.cursor])
	}
}

func (m Model) runItem(it menuItem) (tea.Model, tea.Cmd) {
	m.menu = nil
	cmd := it.run(&m)
	return m, cmd
}

func (m Model) menuView() (string, int, int) {
	mn := m.menu
	items := mn.visible()
	w := 42
	for _, it := range items {
		w = max(w, lipgloss.Width(it.label)+lipgloss.Width(it.key)+8)
	}
	w = min(w, max(20, m.width-4))
	iw := w - 2

	var lines []string
	if mn.filterable {
		lines = append(lines, " "+styleKey.Render("> ")+styleText.Render(mn.filter)+lipgloss.NewStyle().Foreground(colGreen).Render("▏"))
		lines = append(lines, styleBorder.Render(strings.Repeat("─", iw)))
	}
	start := 0
	if mn.cursor >= menuMaxRows {
		start = mn.cursor - menuMaxRows + 1
	}
	end := min(len(items), start+menuMaxRows)
	for i := start; i < end; i++ {
		it := items[i]
		selected := i == mn.cursor
		bg := lipgloss.NewStyle()
		if selected {
			bg = bg.Background(colHighlight)
		}
		labelStyle := bg.Foreground(colFg)
		if it.danger {
			labelStyle = bg.Foreground(colRed)
		}
		if selected {
			labelStyle = labelStyle.Bold(true)
		}
		label := ansi.Truncate(it.label, iw-lipgloss.Width(it.key)-4, "…")
		gap := max(1, iw-2-lipgloss.Width(label)-lipgloss.Width(it.key))
		lines = append(lines, bg.Render(" ")+labelStyle.Render(label)+bg.Render(strings.Repeat(" ", gap))+bg.Foreground(colBlue).Render(it.key)+bg.Render(" "))
	}
	if len(items) == 0 {
		lines = append(lines, styleDim.Render(" nothing matches"))
	}
	h := len(lines) + 2
	footer := "^n/^p move · enter pick · esc close"
	if len(items) > menuMaxRows {
		footer = fmt.Sprintf("%d of %d · ", mn.cursor+1, len(items)) + footer
	}
	return panel(mn.title, footer, strings.Join(lines, "\n"), w, h, true), w, h
}

func overlay(base, box string, x, y int) string {
	baseLines := strings.Split(base, "\n")
	for i, line := range strings.Split(box, "\n") {
		row := y + i
		if row < 0 || row >= len(baseLines) {
			continue
		}
		bl := baseLines[row]
		left := ansi.Truncate(bl, x, "")
		if pad := x - lipgloss.Width(left); pad > 0 {
			left += strings.Repeat(" ", pad)
		}
		right := ansi.TruncateLeft(bl, x+lipgloss.Width(line), "")
		baseLines[row] = left + "\x1b[0m" + line + "\x1b[0m" + right
	}
	return strings.Join(baseLines, "\n")
}

func (m Model) selected() (store.Message, bool) {
	if m.sel >= 0 && m.sel < len(m.messages) {
		return m.messages[m.sel], true
	}
	return store.Message{}, false
}

func (m *Model) openMessageMenu() {
	msg, ok := m.selected()
	if !ok {
		return
	}
	var items []menuItem
	if m.foldable[msg.ID] {
		label := "Expand message"
		if m.expanded[msg.ID] {
			label = "Collapse message"
		}
		items = append(items, menuItem{label: label, key: "z", run: func(m *Model) tea.Cmd {
			if m.expanded[msg.ID] {
				delete(m.expanded, msg.ID)
			} else {
				m.expanded[msg.ID] = true
			}
			m.renderMessages(false)
			m.revealSelection()
			return nil
		}})
	}
	if msg.QuoteID != "" {
		target := msg.QuoteID
		items = append(items, menuItem{label: "Go to replied message", key: "g", run: func(m *Model) tea.Cmd {
			m.jumpToMessage(target)
			return nil
		}})
	}
	if replies := m.repliesTo(msg.ID); len(replies) == 1 {
		target := replies[0].ID
		items = append(items, menuItem{label: "Go to reply", key: "G", run: func(m *Model) tea.Cmd {
			m.jumpToMessage(target)
			return nil
		}})
	} else if len(replies) > 1 {
		items = append(items, menuItem{label: fmt.Sprintf("Replies (%d)…", len(replies)), key: "G", run: func(m *Model) tea.Cmd {
			m.openRepliesMenu(replies)
			return nil
		}})
	}
	if !msg.Deleted {
		items = append(items, menuItem{label: "Reply", key: "r", run: func(m *Model) tea.Cmd {
			m.replyTo, m.editing = &msg, nil
			m.enterInsert()
			return nil
		}})
	}
	if wa.CanEdit(msg) {
		items = append(items, menuItem{label: "Edit", key: "e", run: func(m *Model) tea.Cmd {
			m.editing, m.replyTo = &msg, nil
			m.input.SetValue(msg.Text)
			m.input.CursorEnd()
			m.enterInsert()
			return nil
		}})
	}
	if !msg.Deleted {
		items = append(items, menuItem{label: "React", key: "+", run: func(m *Model) tea.Cmd {
			m.openReactMenu(msg)
			return nil
		}})
	}
	if msg.MediaType == wa.MediaVideo && !m.gone(msg) {
		items = append(items, menuItem{label: "Play video", key: "o", run: func(m *Model) tea.Cmd {
			m.setFlash("downloading video…", false)
			client := m.wa
			return m.actionCmd("", func(ctx context.Context) error {
				path, _, err := client.MediaFile(ctx, msg.ChatJID, msg.ID)
				if err != nil {
					return err
				}
				return player.OpenVideo(path, client.IsGIF(msg.ChatJID, msg.ID))
			})
		}})
	}
	if msg.MediaType == wa.MediaAudio && !m.gone(msg) && m.player != nil {
		st, playing := m.player.State()
		if playing && st.MsgID == msg.ID {
			label := "Pause"
			if st.Paused {
				label = "Resume"
			}
			items = append(items,
				menuItem{label: label, key: "o", run: func(m *Model) tea.Cmd {
					return errCmd(m.player.TogglePause())
				}},
				menuItem{label: fmt.Sprintf("Speed (now %.1g×)", st.Speed), key: ".", run: func(m *Model) tea.Cmd {
					_, err := m.player.CycleSpeed()
					return errCmd(err)
				}},
				menuItem{label: "Stop", key: "x", run: func(m *Model) tea.Cmd {
					m.player.Stop()
					return nil
				}},
			)
		} else {
			items = append(items, menuItem{label: "Play", key: "o", run: func(m *Model) tea.Cmd {
				m.setFlash("downloading…", false)
				client, pl := m.wa, m.player
				return m.actionCmd("", func(ctx context.Context) error {
					path, _, err := client.MediaFile(ctx, msg.ChatJID, msg.ID)
					if err != nil {
						return err
					}
					secs := client.MediaSeconds(msg.ChatJID, msg.ID)
					return pl.PlayAudio(msg.ID, path, time.Duration(secs)*time.Second)
				})
			}})
		}
	}
	if msg.MediaType == wa.MediaDoc && !m.gone(msg) {
		client := m.wa
		items = append(items, menuItem{label: "Open file", key: "o", run: func(m *Model) tea.Cmd {
			m.setFlash("downloading…", false)
			return m.actionCmd("", func(ctx context.Context) error {
				path, _, err := client.MediaFile(ctx, msg.ChatJID, msg.ID)
				if err != nil {
					return err
				}
				return exec.Command("xdg-open", path).Start()
			})
		}})
		if isTextFile(client.FileName(msg.ChatJID, msg.ID)) {
			items = append(items, menuItem{label: "View in nvim (read-only)", key: "v", run: func(m *Model) tea.Cmd {
				m.setFlash("downloading…", false)
				return func() tea.Msg {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
					defer cancel()
					path, _, err := client.MediaFile(ctx, msg.ChatJID, msg.ID)
					return viewFileMsg{path: path, err: err}
				}
			}})
		}
	}
	if msg.MediaType != "" && !m.gone(msg) {
		items = append(items, menuItem{label: "Save to ~/Downloads/WhatsApp", key: "S", run: func(m *Model) tea.Cmd {
			client := m.wa
			when := msg.TS.Local().Format("20060102-150405")
			return func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				path, err := client.SaveMedia(ctx, msg.ChatJID, msg.ID, when)
				if err != nil {
					return actionDoneMsg{err: err}
				}
				return actionDoneMsg{info: "saved " + path}
			}
		}})
	}
	if (msg.MediaType == wa.MediaImage || msg.MediaType == wa.MediaSticker || msg.MediaType == wa.MediaVideo) && !m.gone(msg) && m.images != nil {
		q, shown := m.shownImages[msg.ID]
		if shown && q == media.Full {
			items = append(items, menuItem{label: "Hide image", key: "v", run: func(m *Model) tea.Cmd {
				m.forceHide[msg.ID] = true
				delete(m.forceShow, msg.ID)
				m.renderMessages(false)
				return nil
			}})
		} else {
			items = append(items, menuItem{label: "Show image", key: "v", run: func(m *Model) tea.Cmd {
				m.forceShow[msg.ID] = true
				delete(m.forceHide, msg.ID)
				m.renderMessages(false)
				m.revealSelection()
				return nil
			}})
		}
		if path := m.images.Path(msg.ID); path != "" && (msg.MediaType == wa.MediaImage || msg.MediaType == wa.MediaSticker) {
			items = append(items, menuItem{label: "Open image in viewer", key: "o", run: func(m *Model) tea.Cmd {
				return runDetached("xdg-open", path)
			}})
		}
	}
	if msg.MediaType == "" && !m.gone(msg) && isMediaText(msg.Text) {
		items = append(items, menuItem{label: "Media not available (old message)", key: "v", run: func(m *Model) tea.Cmd {
			m.setFlash("Saved before watui kept media info. To get it: quit, run `watui logout`, then `watui` and scan the QR.", true)
			return nil
		}})
	}
	if !m.gone(msg) && msg.Text != "" {
		items = append(items, menuItem{label: "Copy text", key: "y", run: func(m *Model) tea.Cmd {
			return copyCmd(plainText(msg))
		}})
	}
	if wa.CanForward(msg) {
		items = append(items, menuItem{label: "Forward…", key: "f", run: func(m *Model) tea.Cmd {
			m.openForwardMenu(msg)
			return nil
		}})
	}
	if !msg.Deleted {
		label := "Star"
		if msg.Starred {
			label = "Unstar"
		}
		items = append(items, menuItem{label: label, key: "s", run: func(m *Model) tea.Cmd {
			return m.actionCmd(label+"red", func(ctx context.Context) error { return m.wa.Star(ctx, msg, !msg.Starred) })
		}})
		items = append(items, menuItem{label: "Pin…", key: "p", run: func(m *Model) tea.Cmd {
			m.openPinMenu(msg)
			return nil
		}})
	}
	if wa.CanRevoke(msg) {
		items = append(items, menuItem{label: "Delete for everyone", key: "d", danger: true, run: func(m *Model) tea.Cmd {
			m.menu = &menu{title: "Delete for everyone?", items: []menuItem{
				{label: "Cancel", key: "n", run: func(m *Model) tea.Cmd { return nil }},
				{label: "Yes, delete it", key: "y", danger: true, run: func(m *Model) tea.Cmd {
					return m.actionCmd("deleted", func(ctx context.Context) error { return m.wa.Revoke(ctx, msg) })
				}},
			}}
			return nil
		}})
	}
	switch {
	case msg.FromMe:
		items = append(items, menuItem{label: "View my profile", key: "i", run: func(m *Model) tea.Cmd {
			return m.myInfoCmd()
		}})
	default:
		who := msg.SenderJID
		if chat, _ := m.openChat(); !chat.IsGroup || who == "" {
			who = m.openJID
		}
		name := m.wa.ContactName(who, msg.SenderName)
		if chat, _ := m.openChat(); !chat.IsGroup {
			name = m.names[m.openJID]
		}
		items = append(items, menuItem{label: "View profile · " + sanitize(name), key: "i", run: func(m *Model) tea.Cmd {
			return m.personInfoCmd(who, name)
		}})
	}
	items = append(items, menuItem{label: "Message info", key: "I", run: func(m *Model) tea.Cmd {
		m.openMessageInfo(msg)
		return nil
	}})
	m.menu = &menu{title: "Message", items: items}
}

var reactions = []string{"👍", "❤️", "😂", "😮", "😢", "🙏", "🔥", "🎉"}

func (m *Model) openReactMenu(msg store.Message) {
	var items []menuItem
	for i, emoji := range reactions {
		emoji := emoji
		items = append(items, menuItem{label: emoji, key: fmt.Sprint(i + 1), run: func(m *Model) tea.Cmd {
			return m.actionCmd("reacted "+emoji, func(ctx context.Context) error { return m.wa.React(ctx, msg, emoji) })
		}})
	}
	if _, ok := msg.Reactions[m.wa.OwnJID()]; ok {
		items = append(items, menuItem{label: "Remove my reaction", key: "x", danger: true, run: func(m *Model) tea.Cmd {
			return m.actionCmd("reaction removed", func(ctx context.Context) error { return m.wa.React(ctx, msg, "") })
		}})
	}
	m.menu = &menu{title: "React", items: items}
}

func (m *Model) openPinMenu(msg store.Message) {
	pin := func(label string, d time.Duration) menuItem {
		return menuItem{label: "Pin for " + label, run: func(m *Model) tea.Cmd {
			return m.actionCmd("pinned for "+label, func(ctx context.Context) error { return m.wa.Pin(ctx, msg, true, d) })
		}}
	}
	items := []menuItem{pin("24 hours", 24*time.Hour), pin("7 days", 7*24*time.Hour), pin("30 days", 30*24*time.Hour)}
	items[0].key, items[1].key, items[2].key = "1", "2", "3"
	items = append(items, menuItem{label: "Unpin", key: "u", run: func(m *Model) tea.Cmd {
		return m.actionCmd("unpinned", func(ctx context.Context) error { return m.wa.Pin(ctx, msg, false, 0) })
	}})
	m.menu = &menu{title: "Pin message", items: items}
}

func (m *Model) openForwardMenu(msg store.Message) {
	chats := append([]store.Chat(nil), m.allChats...)
	sort.SliceStable(chats, func(i, j int) bool { return chats[i].LastTS.After(chats[j].LastTS) })
	var items []menuItem
	for _, c := range chats {
		c := c
		name := m.names[c.JID]
		items = append(items, menuItem{label: name, run: func(m *Model) tea.Cmd {
			return m.actionCmd("forwarded to "+name, func(ctx context.Context) error { return m.wa.Forward(ctx, msg, c.JID) })
		}})
	}
	m.menu = &menu{title: "Forward to… (type to search)", items: items, filterable: true}
}

func (m *Model) actionCmd(done string, fn func(ctx context.Context) error) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := fn(ctx); err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{info: done}
	}
}

func errCmd(err error) tea.Cmd {
	if err == nil {
		return nil
	}
	return func() tea.Msg { return actionDoneMsg{err: err} }
}

func plainText(msg store.Message) string {
	text := sanitize(msg.Text)
	if msg.MediaType != "" && strings.HasPrefix(text, "[") {
		if _, rest, ok := strings.Cut(text, "]"); ok {
			return strings.TrimSpace(rest)
		}
	}
	return text
}

func isMediaText(text string) bool {
	for _, tag := range []string{"[image]", "[sticker]", "[video", "[gif]", "[voice note", "[audio", "[document"} {
		if strings.HasPrefix(text, tag) {
			return true
		}
	}
	return false
}

func copyCmd(text string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("xclip", "-selection", "clipboard")
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err != nil {
			return actionDoneMsg{err: fmt.Errorf("copy: %w", err)}
		}
		return actionDoneMsg{info: "copied"}
	}
}

func runDetached(name string, args ...string) tea.Cmd {
	return func() tea.Msg {
		if err := exec.Command(name, args...).Start(); err != nil {
			return actionDoneMsg{err: err}
		}
		return nil
	}
}

type viewFileMsg struct {
	path string
	err  error
}

var textExts = map[string]bool{
	".txt": true, ".md": true, ".csv": true, ".tsv": true, ".log": true, ".json": true, ".xml": true,
	".yml": true, ".yaml": true, ".toml": true, ".ini": true, ".conf": true, ".cfg": true, ".env": true,
	".properties": true, ".sql": true, ".py": true, ".go": true, ".java": true, ".kt": true, ".js": true,
	".ts": true, ".tsx": true, ".jsx": true, ".c": true, ".h": true, ".cpp": true, ".hpp": true, ".rs": true,
	".rb": true, ".php": true, ".sh": true, ".bash": true, ".zsh": true, ".html": true, ".css": true,
	".scss": true, ".lua": true, ".vim": true, ".gradle": true, ".dart": true, ".swift": true, ".http": true,
}

func isTextFile(name string) bool {
	return textExts[strings.ToLower(filepath.Ext(name))]
}

func viewFileCmd(path string) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nvim"
	}
	parts := strings.Fields(editor)
	args := parts[1:]
	if strings.Contains(parts[0], "vi") {
		args = append(args, "-R")
	}
	cmd := exec.Command(parts[0], append(args, path)...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return actionDoneMsg{err: err}
		}
		return nil
	})
}

func (m Model) repliesTo(id string) []store.Message {
	var out []store.Message
	for _, msg := range m.messages {
		if msg.QuoteID == id && !msg.Deleted {
			out = append(out, msg)
		}
	}
	return out
}

func (m *Model) jumpToMessage(id string) {
	for i, msg := range m.messages {
		if msg.ID == id {
			m.sel = i
			m.focus = focusChat
			m.renderMessages(false)
			m.revealSelection()
			return
		}
	}
	m.setFlash("that message is older than what's loaded here", true)
}

func (m *Model) openRepliesMenu(replies []store.Message) {
	chat, _ := m.openChat()
	var items []menuItem
	for _, r := range replies {
		r := r
		who := "You"
		if !r.FromMe {
			who = m.names[m.openJID]
			if chat.IsGroup {
				who = m.wa.ContactName(r.SenderJID, r.SenderName)
			}
		}
		text := ansi.Truncate(strings.ReplaceAll(sanitize(m.withNames(r.Text)), "\n", " "), 40, "…")
		items = append(items, menuItem{label: sanitize(who) + " " + r.TS.Local().Format("15:04") + " · " + text, run: func(m *Model) tea.Cmd {
			m.jumpToMessage(r.ID)
			return nil
		}})
	}
	m.menu = &menu{title: fmt.Sprintf("Replies (%d)", len(replies)), items: items}
}
