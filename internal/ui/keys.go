package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.menu != nil {
		return m.menuKey(k)
	}
	if m.mode == modeNormal || m.mode == modeInsert {
		switch k.String() {
		case "ctrl+h":
			if m.focus == focusList && m.mode == modeNormal {
				return m, tmuxPane("-L")
			}
			m.enterNormal()
			m.focus = focusList
			return m, nil
		case "ctrl+l":
			if m.focus == focusList && m.openJID != "" {
				m.enterNormal()
				m.focus = focusChat
				m.renderMessages(false)
				return m, m.markReadCmd()
			}
			m.enterNormal()
			return m, tmuxPane("-R")
		case "ctrl+k":
			switch {
			case m.mode == modeInsert && m.input.mode == vimNormal:
				m.enterNormal()
				m.focus = focusChat
				m.renderMessages(false)
				return m, nil
			case m.mode == modeNormal:
				return m, tmuxPane("-U")
			}
		case "ctrl+j":
			switch {
			case m.mode == modeNormal && m.focus == focusChat && m.openJID != "":
				m.enterCompose(vimNormal)
				return m, nil
			case m.mode == modeNormal:
				return m, tmuxPane("-D")
			case m.mode == modeInsert && m.input.mode == vimNormal:
				return m, tmuxPane("-D")
			}
		}
	}
	switch m.mode {
	case modeInsert:
		return m.insertKey(k)
	case modeCommand, modeSearch:
		return m.promptKey(k)
	}
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}
	return m.normalKey(k)
}

func (m Model) insertKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "ctrl+s":
		text := strings.TrimSpace(m.input.Value())
		if text == "" || m.openJID == "" {
			return m, nil
		}
		m.input.Reset()
		cmd := m.sendCmd(text)
		return m, cmd
	case "ctrl+e":
		return m, m.editorCmd()
	}
	if m.input.HandleKey(k) == editLeave {
		m.enterNormal()
	}
	return m, nil
}

func (m Model) promptKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "ctrl+c", "ctrl+[":
		if m.mode == modeSearch {
			m.filter = ""
			m.applyFilter()
		}
		m.enterNormal()
		return m, nil
	case "enter":
		value := m.prompt.Value()
		wasCommand := m.mode == modeCommand
		m.enterNormal()
		if wasCommand {
			return m.runCommand(strings.TrimSpace(value))
		}
		m.focus = focusList
		return m, nil
	case "backspace":
		if m.prompt.Value() == "" {
			if m.mode == modeSearch {
				m.filter = ""
				m.applyFilter()
			}
			m.enterNormal()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.prompt, cmd = m.prompt.Update(k)
	if m.mode == modeSearch {
		m.filter = m.prompt.Value()
		m.applyFilter()
	}
	return m, cmd
}

func (m Model) normalKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := k.String()
	if m.pendingG {
		m.pendingG = false
		if key == "g" {
			m.top()
			return m, nil
		}
	}
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case ":":
		m.mode = modeCommand
		m.prompt.SetValue("")
		m.prompt.Focus()
		return m, nil
	case "/":
		m.mode = modeSearch
		m.focus = focusList
		m.prompt.SetValue(m.filter)
		m.prompt.CursorEnd()
		m.prompt.Focus()
		return m, nil
	case "?":
		m.showHelp = true
		return m, nil
	case "g":
		m.pendingG = true
		return m, nil
	case "G":
		m.bottom()
		return m, nil
	case "tab":
		if m.focus == focusList && m.openJID != "" {
			m.focus = focusChat
			return m, m.markReadCmd()
		}
		m.focus = focusList
		return m, nil
	case "1":
		m.focus = focusList
		return m, nil
	case "2":
		if m.openJID != "" {
			m.focus = focusChat
			return m, m.markReadCmd()
		}
		return m, nil
	case "3":
		m.enterInsert()
		return m, nil
	case "J":
		return m.stepChat(1)
	case "K":
		return m.stepChat(-1)
	case "e":
		if m.openJID != "" {
			m.focus = focusChat
			return m, m.editorCmd()
		}
		return m, nil
	}
	if m.focus == focusList {
		return m.listKey(key)
	}
	return m.chatKey(key)
}

func (m Model) listKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "j", "down":
		m.cursor++
	case "k", "up":
		m.cursor--
	case "ctrl+d":
		m.cursor += m.listRows() / 2
	case "ctrl+u":
		m.cursor -= m.listRows() / 2
	case "ctrl+f", "pgdown":
		m.cursor += m.listRows()
	case "ctrl+b", "pgup":
		m.cursor -= m.listRows()
	case "enter", "right", "o":
		if c, ok := m.currentChat(); ok {
			m.open(c.JID)
			m.focus = focusChat
			return m, m.markReadCmd()
		}
	case "i", "a":
		if c, ok := m.currentChat(); ok {
			m.open(c.JID)
			m.enterInsert()
			return m, m.markReadCmd()
		}
	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.applyFilter()
		}
	case "A":
		m.showArchived = !m.showArchived
		m.filter = ""
		m.cursor = 0
		m.applyFilter()
		if m.showArchived {
			m.setFlash("showing archived chats (A to go back)", false)
		}
	}
	m.clampCursor()
	return m, nil
}

func (m Model) chatKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "j", "down":
		m.moveSelection(1)
	case "k", "up":
		m.moveSelection(-1)
	case "ctrl+d":
		m.moveSelection(10)
	case "ctrl+u":
		m.moveSelection(-10)
	case "ctrl+f", "pgdown":
		m.moveSelection(25)
	case "ctrl+b", "pgup":
		m.moveSelection(-25)
	case " ", "m":
		m.openMessageMenu()
	case "esc":
		if m.replyTo != nil || m.editing != nil {
			m.replyTo, m.editing = nil, nil
			m.input.Reset()
			return m, nil
		}
		m.focus = focusList
	case "left", "q":
		m.focus = focusList
	case "i", "a", "enter":
		m.enterInsert()
	}
	return m, nil
}

const longScrollStep = 3

func (m *Model) moveSelection(delta int) {
	if len(m.messages) == 0 {
		return
	}
	if m.sel >= 0 && m.sel < len(m.spans) && (delta == 1 || delta == -1) {
		start, end := m.spans[m.sel][0], m.spans[m.sel][1]
		top, h := m.vp.YOffset, m.vp.Height
		if delta == 1 && end >= top+h {
			m.vp.SetYOffset(min(top+longScrollStep, end-h+1))
			return
		}
		if delta == -1 && start < top {
			m.vp.SetYOffset(max(top-longScrollStep, start))
			return
		}
	}
	next := max(0, min(len(m.messages)-1, m.sel+delta))
	if next == m.sel {
		return
	}
	m.sel = next
	m.renderMessages(false)
	m.revealSelection(delta < 0)
}

func (m *Model) revealSelection(fromBelow ...bool) {
	if m.sel < 0 || m.sel >= len(m.spans) {
		return
	}
	start, end := m.spans[m.sel][0], m.spans[m.sel][1]
	top, h := m.vp.YOffset, m.vp.Height
	tall := end-start+1 > h
	switch {
	case tall && len(fromBelow) > 0 && fromBelow[0]:
		if end >= top+h || end < top {
			m.vp.SetYOffset(end - h + 1)
		}
	case tall:
		if end < top || start >= top+h {
			m.vp.SetYOffset(start)
		}
	case start < top:
		m.vp.SetYOffset(start)
	case end >= top+h:
		m.vp.SetYOffset(end - h + 1)
	}
	if m.sel == len(m.messages)-1 && !tall {
		m.vp.GotoBottom()
	}
}

func (m *Model) top() {
	if m.focus == focusList {
		m.cursor = 0
		m.clampCursor()
		return
	}
	m.sel = 0
	m.renderMessages(false)
	m.vp.GotoTop()
}

func (m *Model) bottom() {
	if m.focus == focusList {
		m.cursor = len(m.chats) - 1
		m.clampCursor()
		return
	}
	m.sel = len(m.messages) - 1
	m.renderMessages(false)
	m.vp.GotoBottom()
}

func (m Model) stepChat(delta int) (tea.Model, tea.Cmd) {
	if len(m.chats) == 0 {
		return m, nil
	}
	m.cursor += delta
	m.clampCursor()
	c, _ := m.currentChat()
	m.open(c.JID)
	m.focus = focusChat
	return m, m.markReadCmd()
}

func (m Model) runCommand(cmd string) (tea.Model, tea.Cmd) {
	name, _, _ := strings.Cut(cmd, " ")
	switch name {
	case "":
		return m, nil
	case "q", "q!", "qa", "quit", "wq", "x":
		return m, tea.Quit
	case "h", "help":
		m.showHelp = true
		return m, nil
	case "set":
		_, arg, _ := strings.Cut(cmd, " ")
		arg = strings.TrimSpace(arg)
		if arg == "" {
			m.setFlash(fmt.Sprintf("images=%s image_width=%d image_height=%d collapse_lines=%d", m.cfg.Images, m.cfg.ImageWidth, m.cfg.ImageHeight, m.cfg.CollapseLines), false)
			return m, nil
		}
		if err := m.cfg.Set(arg); err != nil {
			m.setFlash(err.Error(), true)
			return m, nil
		}
		if m.images != nil {
			m.images.Reset()
		}
		m.renderMessages(false)
		m.setFlash("set "+arg+" (for this session — edit ~/.config/watui/config to keep it)", false)
		return m, nil
	case "archived":
		m.showArchived = !m.showArchived
		m.cursor = 0
		m.applyFilter()
		return m, nil
	case "read":
		if m.openJID == "" {
			return m, nil
		}
		client, jid := m.wa, m.openJID
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			return actionDoneMsg{info: "marked as read", err: client.MarkRead(ctx, jid)}
		}
	case "pin", "unpin", "archive", "unarchive", "mute", "unmute":
		target := m.openJID
		if m.focus == focusList {
			if c, ok := m.currentChat(); ok {
				target = c.JID
			}
		}
		if target == "" {
			m.setFlash("no chat selected", true)
			return m, nil
		}
		return m, m.chatActionCmd(name, target)
	}
	m.setFlash("unknown command: "+cmd, true)
	return m, nil
}

func tmuxPane(dir string) tea.Cmd {
	if os.Getenv("TMUX") == "" {
		return nil
	}
	return func() tea.Msg {
		_ = exec.Command("tmux", "select-pane", dir).Run()
		return nil
	}
}

func (m Model) chatActionCmd(action, jid string) tea.Cmd {
	client := m.wa
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		var err error
		switch action {
		case "pin":
			err = client.SetPinned(ctx, jid, true)
		case "unpin":
			err = client.SetPinned(ctx, jid, false)
		case "archive":
			err = client.SetArchived(ctx, jid, true)
		case "unarchive":
			err = client.SetArchived(ctx, jid, false)
		case "mute":
			err = client.SetMuted(ctx, jid, true)
		case "unmute":
			err = client.SetMuted(ctx, jid, false)
		}
		return actionDoneMsg{info: action + " done", err: err}
	}
}
