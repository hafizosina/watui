package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/zhenzhu/watui/internal/config"
	"github.com/zhenzhu/watui/internal/media"
	"github.com/zhenzhu/watui/internal/player"
	"github.com/zhenzhu/watui/internal/profile"
	"github.com/zhenzhu/watui/internal/store"
	"github.com/zhenzhu/watui/internal/wa"
)

type mode int

const (
	modeNormal mode = iota
	modeInsert
	modeCommand
	modeSearch
)

func (m mode) String() string {
	switch m {
	case modeInsert:
		return "INSERT"
	case modeCommand:
		return "COMMAND"
	case modeSearch:
		return "SEARCH"
	}
	return "NORMAL"
}

type focus int

const (
	focusList focus = iota
	focusChat
	focusProfiles
)

const (
	firstPage = 200
	olderPage = 300
)

type ChangedMsg struct {
	Profile string
}

type tickMsg time.Time

type actionDoneMsg struct {
	info string
	err  error
}

type editorDoneMsg struct {
	text string
	err  error
}

type Model struct {
	profiles   []*profile.Profile
	active     int
	profCursor int
	unread     map[string]int
	opts       Options

	wa     *wa.Client
	cfg    config.Config
	images *media.Manager
	player *player.Player

	width, height int
	mode          mode
	focus         focus
	focused       bool
	showHelp      bool
	pendingG      bool

	allChats     []store.Chat
	chats        []store.Chat
	names        map[string]string
	cursor       int
	offset       int
	filter       string
	showArchived bool

	openJID      string
	messages     []store.Message
	sel          int
	spans        [][2]int
	menu         *menu
	replyTo      *store.Message
	editing      *store.Message
	forceShow    map[string]bool
	forceHide    map[string]bool
	shownImages  map[string]media.Quality
	pendingGroup []string
	info         *infoState
	ringing      *ringing
	emojiSel     int
	cmdSel       int
	loaded       int
	blocks       map[string]cachedBlock
	cmdTyped     string
	stickers     *stickerPicker
	emojiHidden  string
	expanded     map[string]bool
	foldable     map[string]bool

	vp         viewport.Model
	input      *editor
	prompt     textinput.Model
	flash      string
	flashErr   bool
	flashUntil time.Time
}

func New(profiles []*profile.Profile, cfg config.Config, pl *player.Player, opts Options) Model {
	m := newModel(profiles[0].WA, cfg, profiles[0].Images, pl)
	if cfg.Theme != "" && cfg.Theme != currentTheme {
		if err := m.setTheme(cfg.Theme); err != nil {
			m.setFlash(err.Error(), true)
		}
	}
	m.profiles = profiles
	m.opts = opts
	return m
}

func newModel(client *wa.Client, cfg config.Config, images *media.Manager, pl *player.Player) Model {
	in := newEditor()

	pr := textinput.New()
	pr.Prompt = ""
	pr.CharLimit = 200

	return Model{
		wa:          client,
		cfg:         cfg,
		images:      images,
		player:      pl,
		focused:     true,
		names:       map[string]string{},
		forceShow:   map[string]bool{},
		forceHide:   map[string]bool{},
		shownImages: map[string]media.Quality{},
		expanded:    map[string]bool{},
		unread:      map[string]int{},
		blocks:      map[string]cachedBlock{},
		foldable:    map[string]bool{},
		vp:          viewport.New(0, 0),
		input:       in,
		prompt:      pr,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(reload, tick())
}

func reload() tea.Msg { return ChangedMsg{} }

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) setFlash(text string, isErr bool) {
	m.flash = text
	m.flashErr = isErr
	m.flashUntil = time.Now().Add(4 * time.Second)
	if isErr {
		m.flashUntil = time.Now().Add(8 * time.Second)
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		m.renderMessages(true)
		return m, nil
	case tea.FocusMsg:
		m.focused = true
		return m, m.markReadCmd()
	case tea.BlurMsg:
		m.focused = false
		return m, nil
	case ChangedMsg:
		if p := m.activeProfile(); p != nil && msg.Profile != "" && msg.Profile != p.Name {
			for _, other := range m.profiles {
				if other.Name == msg.Profile {
					m.countUnread(other)
				}
			}
			return m, nil
		}
		m.refresh()
		return m, m.markReadCmd()
	case infoLoadedMsg:
		return m.infoLoaded(msg)
	case viewFileMsg:
		if msg.err != nil {
			m.setFlash(msg.err.Error(), true)
			return m, nil
		}
		return m, viewFileCmd(msg.path)
	case CallMsg:
		return m.call(msg)
	case DeclineCallMsg:
		return m.declineCall(msg.Profile, msg.CallID)
	case IncomingMsg:
		return m.incoming(msg)
	case OpenChatMsg:
		return m.openFromNotification(msg)
	case newChatMsg:
		return m.newChatReady(msg)
	case profileAddedMsg:
		return m.profileAdded(msg)
	case profileRemovedMsg:
		return m.profileRemoved(msg)
	case tickMsg:
		if m.player != nil {
			if _, playing := m.player.State(); playing {
				m.renderMessages(false)
			}
		}
		return m, tick()
	case actionDoneMsg:
		if msg.err != nil {
			m.setFlash(msg.err.Error(), true)
		} else if msg.info != "" {
			m.setFlash(msg.info, false)
		}
		return m, nil
	case editorDoneMsg:
		if msg.err != nil {
			m.setFlash("editor: "+msg.err.Error(), true)
			return m, nil
		}
		m.input.SetValue(strings.TrimRight(msg.text, "\n"))
		m.input.CursorEnd()
		m.enterInsert()
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) layout() {
	chatW := m.width - m.listWidth()
	m.vp.Width = chatW - 2
	m.vp.Height = max(1, m.mainH()-m.composeH()-2)
	m.input.SetWidth(chatW - 2)
	m.prompt.Width = m.width - 2
}

func (m Model) listWidth() int {
	return min(36, max(20, m.width/3))
}

func (m *Model) refresh() {
	chats, err := m.wa.Store.Chats()
	if err != nil {
		m.setFlash("load chats: "+err.Error(), true)
		return
	}
	m.allChats = chats
	if p := m.activeProfile(); p != nil {
		n := 0
		for _, c := range chats {
			if c.Unread > 0 && !c.Muted() && !c.Archived {
				n++
			}
		}
		m.unread[p.Name] = n
	}
	for _, c := range chats {
		m.names[c.JID] = sanitize(m.wa.ChatName(c))
	}
	m.applyFilter()
	if m.openJID != "" {
		msgs, err := m.wa.Store.Messages(m.openJID, max(m.loaded, firstPage))
		if err != nil {
			m.setFlash("load messages: "+err.Error(), true)
			return
		}
		selID, wasLast := "", m.sel >= len(m.messages)-1
		if cur, ok := m.selected(); ok {
			selID = cur.ID
		}
		m.messages = msgs
		m.sel = len(msgs) - 1
		if !wasLast {
			for i, msg := range msgs {
				if msg.ID == selID {
					m.sel = i
					break
				}
			}
		}
		m.renderMessages(false)
	}
}

func (m *Model) applyFilter() {
	selected := ""
	if m.cursor < len(m.chats) {
		selected = m.chats[m.cursor].JID
	}
	needle := strings.ToLower(m.filter)
	m.chats = m.chats[:0:0]
	if needle == "" {
		if m.showArchived {
			m.chats = append(m.chats, store.Chat{JID: rowBack})
		} else {
			n := 0
			for _, c := range m.allChats {
				if c.Archived {
					n++
				}
			}
			if n > 0 {
				m.chats = append(m.chats, store.Chat{JID: rowArchived, Unread: n})
			}
		}
	}
	for _, c := range m.allChats {
		if c.Archived != m.showArchived && needle == "" {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(m.names[c.JID]), needle) {
			continue
		}
		m.chats = append(m.chats, c)
	}
	m.cursor = 0
	if len(m.chats) > 1 && isSpecialRow(m.chats[0].JID) {
		m.cursor = 1
	}
	for i, c := range m.chats {
		if c.JID == selected {
			m.cursor = i
			break
		}
	}
	m.clampCursor()
}

const (
	rowArchived = "__archived__"
	rowBack     = "__back__"
)

func isSpecialRow(jid string) bool {
	return jid == rowArchived || jid == rowBack
}

func (m *Model) toggleArchived() {
	m.showArchived = !m.showArchived
	m.filter = ""
	m.cursor = 0
	m.applyFilter()
}

func (m *Model) clampCursor() {
	if m.cursor >= len(m.chats) {
		m.cursor = len(m.chats) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	rows := m.listRows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m Model) listRows() int {
	return max(1, (m.mainH()-m.profilesH()-2)/2)
}

func (m Model) currentChat() (store.Chat, bool) {
	if m.cursor < len(m.chats) && !isSpecialRow(m.chats[m.cursor].JID) {
		return m.chats[m.cursor], true
	}
	return store.Chat{}, false
}

func (m Model) onSpecialRow() bool {
	return m.cursor < len(m.chats) && isSpecialRow(m.chats[m.cursor].JID)
}

func (m Model) openChat() (store.Chat, bool) {
	for _, c := range m.allChats {
		if c.JID == m.openJID {
			return c, true
		}
	}
	return store.Chat{}, false
}

func (m *Model) open(jid string) {
	m.info = nil
	if m.openJID != jid {
		m.input.Reset()
		m.replyTo, m.editing = nil, nil
	}
	if m.opts.Notifier != nil {
		if p := m.activeProfile(); p != nil {
			m.opts.Notifier.Forget(NoteKey(p.Name, jid))
		}
	}
	m.openJID = jid
	m.loaded = firstPage
	msgs, err := m.wa.Store.Messages(jid, m.loaded)
	if err != nil {
		m.setFlash("load messages: "+err.Error(), true)
	}
	m.messages = msgs
	m.sel = len(msgs) - 1
	m.renderMessages(true)
}

func (m *Model) enterInsert() {
	m.enterCompose(vimInsert)
}

func (m *Model) enterCompose(v vimMode) {
	if m.openJID == "" {
		return
	}
	m.info = nil
	m.mode = modeInsert
	m.focus = focusChat
	m.input.Focus(v)
	if v == vimNormal {
		m.input.clampNormal()
	}
	m.renderMessages(false)
}

func (m *Model) enterNormal() {
	m.mode = modeNormal
	m.input.Blur()
	m.prompt.Blur()
}

func (m Model) markReadCmd() tea.Cmd {
	if !m.focused || m.focus != focusChat || m.openJID == "" {
		return nil
	}
	chat, ok := m.openChat()
	if !ok || chat.Unread == 0 {
		return nil
	}
	client, jid := m.wa, m.openJID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := client.MarkRead(ctx, jid); err != nil {
			return actionDoneMsg{err: err}
		}
		return nil
	}
}

func (m *Model) sendCmd(text string) tea.Cmd {
	client, jid := m.wa, m.openJID
	replyTo, editing := m.replyTo, m.editing
	m.replyTo, m.editing = nil, nil
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		var err error
		switch {
		case editing != nil:
			err = client.Edit(ctx, *editing, text)
		case replyTo != nil:
			err = client.Reply(ctx, jid, text, *replyTo)
		default:
			err = client.Send(ctx, jid, text)
		}
		if err != nil {
			return actionDoneMsg{err: err}
		}
		return nil
	}
}

func (m Model) editorCmd() tea.Cmd {
	f, err := os.CreateTemp("", "watui-*.md")
	if err != nil {
		return func() tea.Msg { return editorDoneMsg{err: err} }
	}
	_, _ = f.WriteString(m.input.Value())
	_ = f.Close()
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nvim"
	}
	parts := strings.Fields(editor)
	cmd := exec.Command(parts[0], append(parts[1:], f.Name())...)
	path := f.Name()
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		defer os.Remove(path)
		if err != nil {
			return editorDoneMsg{err: err}
		}
		data, err := os.ReadFile(path)
		return editorDoneMsg{text: string(data), err: err}
	})
}

// loadOlder fetches older messages when the top of the loaded history is reached.
func (m *Model) loadOlder() bool {
	if m.openJID == "" || len(m.messages) < m.loaded {
		return false
	}
	total, err := m.wa.Store.CountMessages(m.openJID)
	if err != nil || total <= len(m.messages) {
		return false
	}
	keep := ""
	if cur, ok := m.selected(); ok {
		keep = cur.ID
	}
	m.loaded += olderPage
	msgs, err := m.wa.Store.Messages(m.openJID, m.loaded)
	if err != nil {
		return false
	}
	m.messages = msgs
	for i, msg := range msgs {
		if msg.ID == keep {
			m.sel = i
		}
	}
	m.renderMessages(false)
	m.revealSelection()
	m.setFlash(fmt.Sprintf("loaded older messages (%d of %d)", len(msgs), total), false)
	return true
}
