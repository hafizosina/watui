package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/cancelreader"
	"golang.org/x/sys/unix"

	"github.com/zhenzhu/watui/internal/media"
	"github.com/zhenzhu/watui/internal/notify"
	"github.com/zhenzhu/watui/internal/profile"
	"github.com/zhenzhu/watui/internal/wa"
)

type Options struct {
	Root     string
	Watch    func(p *profile.Profile)
	Notifier *notify.Notifier
}

type profileAddedMsg struct {
	p     *profile.Profile
	isNew bool
	err   error
}

func (m Model) profilesH() int {
	return 3
}

func (m Model) activeProfile() *profile.Profile {
	if m.active >= 0 && m.active < len(m.profiles) {
		return m.profiles[m.active]
	}
	return nil
}

func (m Model) profilesTitle() string {
	title := "[1] Profile"
	if m.wa == nil {
		return title
	}
	state, syncing := m.wa.State()
	switch {
	case state == wa.StateLoggedOut:
		title += " · logged out"
	case state == wa.StateDisconnected:
		title += " · ○ offline"
	case state == wa.StateConnecting:
		title += " · ◌ connecting"
	case syncing:
		title += " · ◌ syncing"
	default:
		title += " · ● online"
	}
	return title
}

func (m Model) profilesFooter() string {
	if m.player != nil {
		if st, ok := m.player.State(); ok {
			icon := "▶"
			if st.Paused {
				icon = "⏸"
			}
			return icon + " " + clock(st.Position) + "/" + clock(st.Duration)
		}
	}
	if m.focus == focusProfiles && m.mode == modeNormal {
		return "enter list · space actions"
	}
	return ""
}

func (m Model) profilesView(w int) string {
	p := m.activeProfile()
	if p == nil {
		return ""
	}
	others := 0
	for _, o := range m.profiles {
		if o != p {
			others += m.unread[o.Name]
		}
	}
	badge := ""
	if others > 0 {
		badge = styleDim.Render("others ") + styleBadge.Render(fmt.Sprintf(" %d ", others)) + " "
	} else if len(m.profiles) > 1 {
		badge = styleDim.Render(fmt.Sprintf("%d profiles ", len(m.profiles)))
	}
	line := lipgloss.NewStyle().Foreground(colGreen).Render(" ● ") + lipgloss.NewStyle().Foreground(colFg).Bold(true).Render(p.Label())
	if !p.NotifyEnabled() {
		line += styleMuted.Render(" " + iconMute + " muted")
	}
	if me := p.WA.Me(); me != "" && p.Linked() {
		room := max(1, w-lipgloss.Width(line)-lipgloss.Width(badge)-3)
		line += styleDim.Render(" · " + ansi.Truncate(sanitize(me), room, "…"))
	}
	return line + strings.Repeat(" ", max(1, w-lipgloss.Width(line)-lipgloss.Width(badge))) + badge
}

func (m Model) profilesKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "enter", "o", "l":
		m.openProfilePicker()
	case " ", "m":
		m.profCursor = m.active
		m.openProfileMenu()
	case "a":
		m.startAddPrompt()
	case "esc", "q":
		m.focus = focusList
	}
	return m, nil
}

func (m *Model) openProfilePicker() {
	var items []menuItem
	for i, p := range m.profiles {
		i := i
		label := "○ " + p.Label()
		if i == m.active {
			label = "● " + p.Label()
		}
		if !p.NotifyEnabled() {
			label += "  · " + iconMute + " notifications off"
		}
		switch {
		case !p.Linked():
			label += "  · not linked"
		case m.unread[p.Name] > 0:
			label += fmt.Sprintf("  · %d unread", m.unread[p.Name])
		}
		key := ""
		if i < 9 {
			key = fmt.Sprint(i + 1)
		}
		items = append(items, menuItem{label: label, key: key, run: func(m *Model) tea.Cmd {
			next, cmd := m.switchProfile(i)
			*m = next.(Model)
			return cmd
		}})
	}
	items = append(items, menuItem{label: "+ Add profile…", key: "a", run: func(m *Model) tea.Cmd {
		m.startAddPrompt()
		return nil
	}})
	m.menu = &menu{title: "Profiles", items: items, cursor: m.active}
}

func (m *Model) startAddPrompt() {
	m.mode = modeCommand
	m.prompt.SetValue("profile add ")
	m.prompt.CursorEnd()
	m.prompt.Focus()
}

func (m Model) switchProfile(i int) (tea.Model, tea.Cmd) {
	if i < 0 || i >= len(m.profiles) {
		return m, nil
	}
	p := m.profiles[i]
	if !p.Linked() {
		return m, m.loginCmd(p, false)
	}
	if i == m.active {
		m.focus = focusList
		return m, nil
	}
	m.active, m.profCursor = i, i
	m.wa, m.images = p.WA, p.Images
	m.allChats, m.chats = nil, nil
	m.names = map[string]string{}
	m.cursor, m.offset, m.filter, m.showArchived = 0, 0, "", false
	m.openJID, m.messages, m.sel, m.spans = "", nil, 0, nil
	m.replyTo, m.editing, m.menu = nil, nil, nil
	m.input.Reset()
	m.forceShow, m.forceHide = map[string]bool{}, map[string]bool{}
	m.expanded, m.foldable = map[string]bool{}, map[string]bool{}
	m.shownImages = map[string]media.Quality{}
	m.enterNormal()
	m.focus = focusList
	m.vp.SetContent("")
	m.refresh()
	m.setFlash("switched to profile "+p.Label(), false)
	return m, nil
}

func (m Model) stepProfile(delta int) (tea.Model, tea.Cmd) {
	if len(m.profiles) < 2 {
		return m, nil
	}
	next := (m.active + delta + len(m.profiles)) % len(m.profiles)
	for tries := 0; tries < len(m.profiles) && !m.profiles[next].Linked(); tries++ {
		next = (next + delta + len(m.profiles)) % len(m.profiles)
	}
	return m.switchProfile(next)
}

func (m Model) profileCommand(arg string) (tea.Model, tea.Cmd) {
	sub, name, _ := strings.Cut(strings.TrimSpace(arg), " ")
	name = strings.TrimSpace(name)
	switch sub {
	case "":
		m.focus = focusProfiles
		m.profCursor = m.active
		return m, nil
	case "rename":
		target, label, _ := strings.Cut(name, " ")
		for _, p := range m.profiles {
			if p.Name == target {
				if err := p.SetLabel(label); err != nil && !os.IsNotExist(err) {
					m.setFlash(err.Error(), true)
					return m, nil
				}
				m.setFlash("renamed to "+p.Label(), false)
				return m, nil
			}
		}
		m.setFlash("usage: :profile rename <name> <new label>", true)
		return m, nil
	case "add":
		if name == "" {
			m.setFlash("usage: :profile add <name>", true)
			return m, nil
		}
		if err := profile.ValidName(name); err != nil {
			m.setFlash(err.Error(), true)
			return m, nil
		}
		for i, p := range m.profiles {
			if p.Name == name {
				return m.switchProfile(i)
			}
		}
		if m.opts.Root == "" {
			m.setFlash("adding profiles is not available here", true)
			return m, nil
		}
		p, err := profile.Open(m.opts.Root, name)
		if err != nil {
			m.setFlash(err.Error(), true)
			return m, nil
		}
		return m, m.loginCmd(p, true)
	}
	for i, p := range m.profiles {
		if p.Name == sub {
			return m.switchProfile(i)
		}
	}
	m.setFlash("no profile named "+sub+" (add it with :profile add "+sub+")", true)
	return m, nil
}

func (m Model) loginCmd(p *profile.Profile, isNew bool) tea.Cmd {
	return tea.Exec(&loginExec{p: p}, func(err error) tea.Msg {
		return profileAddedMsg{p: p, isNew: isNew, err: err}
	})
}

func (m Model) profileAdded(msg profileAddedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		if msg.isNew {
			msg.p.Close()
			_ = profile.Remove(m.opts.Root, msg.p.Name)
		}
		m.setFlash("profile "+msg.p.Name+": "+msg.err.Error(), true)
		return m, nil
	}
	idx := -1
	for i, p := range m.profiles {
		if p == msg.p {
			idx = i
		}
	}
	if idx < 0 {
		m.profiles = append(m.profiles, msg.p)
		idx = len(m.profiles) - 1
		if m.opts.Watch != nil {
			m.opts.Watch(msg.p)
		}
	}
	m.active = -1
	return m.switchProfile(idx)
}

func (m *Model) countUnread(p *profile.Profile) {
	if p == nil || p.WA == nil {
		return
	}
	chats, err := p.WA.Store.Chats()
	if err != nil {
		return
	}
	n := 0
	for _, c := range chats {
		if c.Unread > 0 && !c.Muted() && !c.Archived {
			n++
		}
	}
	m.unread[p.Name] = n
}

type loginExec struct {
	p      *profile.Profile
	stdin  io.Reader
	stdout io.Writer
}

func (l *loginExec) SetStdin(r io.Reader)  { l.stdin = r }
func (l *loginExec) SetStdout(w io.Writer) { l.stdout = w }
func (l *loginExec) SetStderr(io.Writer)   {}

func (l *loginExec) Run() error {
	out := l.stdout
	if out == nil {
		out = os.Stdout
	}
	fd := int(os.Stdin.Fd())
	if old, err := unix.IoctlGetTermios(fd, unix.TCGETS); err == nil {
		t := *old
		t.Lflag &^= unix.ISIG | unix.ICANON | unix.ECHO
		t.Cc[unix.VMIN], t.Cc[unix.VTIME] = 1, 0
		_ = unix.IoctlSetTermios(fd, unix.TCSETS, &t)
		defer unix.IoctlSetTermios(fd, unix.TCSETS, old)
	}
	cancel := make(chan struct{})
	cr, err := cancelreader.NewReader(os.Stdin)
	done := make(chan struct{})
	if err == nil {
		go func() {
			defer close(done)
			buf := make([]byte, 1)
			for {
				n, err := cr.Read(buf)
				if err != nil {
					return
				}
				if n == 1 && (buf[0] == '\r' || buf[0] == '\n' || buf[0] == 3 || buf[0] == 'q' || buf[0] == 27) {
					close(cancel)
					return
				}
			}
		}()
	} else {
		close(done)
	}
	loginErr := l.p.Login(out, cancel)
	if cr != nil {
		cr.Cancel()
		<-done
		cr.Close()
	}
	if loginErr != nil {
		select {
		case <-cancel:
			return errors.New("cancelled")
		default:
		}
	}
	return loginErr
}

type profileRemovedMsg struct {
	p   *profile.Profile
	err error
}

func (m *Model) openProfileMenu() {
	if m.profCursor >= len(m.profiles) {
		return
	}
	p := m.profiles[m.profCursor]
	idx := m.profCursor
	var items []menuItem
	if idx != m.active || !p.Linked() {
		label := "Switch to " + p.Label()
		if !p.Linked() {
			label = "Link with QR code"
		}
		items = append(items, menuItem{label: label, key: "o", run: func(m *Model) tea.Cmd {
			next, cmd := m.switchProfile(idx)
			*m = next.(Model)
			return cmd
		}})
	}
	if p.Linked() && idx == m.active {
		items = append(items, menuItem{label: "My profile", key: "i", run: func(m *Model) tea.Cmd {
			return m.myInfoCmd()
		}})
	}
	items = append(items, menuItem{label: "Rename…", key: "r", run: func(m *Model) tea.Cmd {
		m.mode = modeCommand
		m.prompt.SetValue("profile rename " + p.Name + " ")
		m.prompt.CursorEnd()
		m.prompt.Focus()
		return nil
	}})
	if p.NotifyEnabled() {
		items = append(items, menuItem{label: "Turn off desktop notifications", key: "n", run: func(m *Model) tea.Cmd {
			return errOrFlash(m, p.SetNotify(false), "notifications off for "+p.Label())
		}})
	} else {
		items = append(items, menuItem{label: "Turn on desktop notifications", key: "n", run: func(m *Model) tea.Cmd {
			return errOrFlash(m, p.SetNotify(true), "notifications on for "+p.Label())
		}})
	}
	if p.Linked() {
		items = append(items, menuItem{label: "Reconnect", key: "c", run: func(m *Model) tea.Cmd {
			return m.actionCmd("reconnecting "+p.Label(), func(ctxDone context.Context) error { return p.Reconnect() })
		}})
		items = append(items, menuItem{label: "Log out (unlink this computer)", key: "L", danger: true, run: func(m *Model) tea.Cmd {
			m.menu = &menu{title: "Log out " + p.Label() + "?", items: []menuItem{
				{label: "Cancel", key: "n", run: func(m *Model) tea.Cmd { return nil }},
				{label: "Yes, log out", key: "y", danger: true, run: func(m *Model) tea.Cmd {
					return m.actionCmd("logged out of "+p.Label()+" (Enter on it to link again)", func(ctxDone context.Context) error { return p.Logout() })
				}},
			}}
			return nil
		}})
	}
	if p.Name != profile.Default {
		items = append(items, menuItem{label: "Remove profile (log out + delete its chats)", key: "d", danger: true, run: func(m *Model) tea.Cmd {
			m.menu = &menu{title: "Remove " + p.Label() + "?", items: []menuItem{
				{label: "Cancel", key: "n", run: func(m *Model) tea.Cmd { return nil }},
				{label: "Yes, remove it", key: "y", danger: true, run: func(m *Model) tea.Cmd {
					root := m.opts.Root
					return func() tea.Msg {
						if p.Linked() {
							_ = p.Logout()
						}
						p.Close()
						return profileRemovedMsg{p: p, err: profile.Remove(root, p.Name)}
					}
				}},
			}}
			return nil
		}})
	}
	items = append(items, menuItem{label: "Add profile…", key: "a", run: func(m *Model) tea.Cmd {
		m.startAddPrompt()
		return nil
	}})
	m.menu = &menu{title: "Profile · " + p.Label(), items: items}
}

func (m Model) profileRemoved(msg profileRemovedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.setFlash("remove "+msg.p.Label()+": "+msg.err.Error(), true)
	}
	idx := -1
	for i, p := range m.profiles {
		if p == msg.p {
			idx = i
		}
	}
	if idx < 0 {
		return m, nil
	}
	wasActive := idx == m.active
	m.profiles = append(m.profiles[:idx:idx], m.profiles[idx+1:]...)
	delete(m.unread, msg.p.Name)
	if idx < m.active {
		m.active--
	}
	m.profCursor = min(m.profCursor, len(m.profiles)-1)
	if wasActive {
		m.active = -1
		return m.switchProfile(0)
	}
	if msg.err == nil {
		m.setFlash("removed profile "+msg.p.Label(), false)
	}
	return m, nil
}

func errOrFlash(m *Model, err error, ok string) tea.Cmd {
	if err != nil {
		m.setFlash(err.Error(), true)
		return nil
	}
	m.setFlash(ok, false)
	return nil
}
