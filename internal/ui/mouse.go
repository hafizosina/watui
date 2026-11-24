package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Mouse: the wheel scrolls whatever is under the pointer (or the open popup),
// left click focuses and picks, right click opens the menu (like Space).

func (m Model) handleMouse(ev tea.MouseMsg) (tea.Model, tea.Cmd) {
	if ev.Action != tea.MouseActionPress {
		return m, nil
	}
	switch ev.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		return m.mouseWheel(ev, ev.Button == tea.MouseButtonWheelDown)
	case tea.MouseButtonLeft, tea.MouseButtonRight:
		if m.stepTo != "" {
			settled := m.settleStep()
			next, cmd := m.mouseClick(ev, ev.Button == tea.MouseButtonRight)
			return next, tea.Batch(settled, cmd)
		}
		return m.mouseClick(ev, ev.Button == tea.MouseButtonRight)
	}
	return m, nil
}

func (m Model) mouseWheel(ev tea.MouseMsg, down bool) (tea.Model, tea.Cmd) {
	d, k, mk := -1, "k", tea.KeyMsg{Type: tea.KeyUp}
	if down {
		d, k, mk = 1, "j", tea.KeyMsg{Type: tea.KeyDown}
	}
	switch {
	case m.menu != nil:
		return m.menuKey(mk)
	case m.stickers != nil:
		return m.stickerKey(k)
	case m.mode == modeCommand || m.mode == modeSearch || m.mode == modeMsgSearch:
		return m, nil
	}
	listW := m.listWidth()
	if ev.X < listW {
		if ev.Y < m.profilesH() {
			return m, nil
		}
		m.cursor += d
		m.clampCursor()
		return m, nil
	}
	if m.showHelp {
		return m, nil
	}
	if m.info != nil {
		next, cmd, _ := m.infoKey(k)
		return next, cmd
	}
	if m.openJID != "" {
		m.moveSelection(d)
	}
	return m, nil
}

func (m Model) mouseClick(ev tea.MouseMsg, right bool) (tea.Model, tea.Cmd) {
	if m.menu != nil {
		return m.menuClick(ev, right)
	}
	if m.stickers != nil {
		return m.stickerClick(ev)
	}
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}
	if m.mode == modeCommand || m.mode == modeSearch || m.mode == modeMsgSearch {
		m.enterNormal()
	}
	listW := m.listWidth()
	mainH := m.mainH()
	if ev.Y >= mainH {
		return m, nil
	}

	if ev.X < listW {
		m.enterNormal()
		if ev.Y < m.profilesH() {
			m.focus = focusProfiles
			m.profCursor = m.active
			if right {
				m.openProfileMenu()
			}
			return m, nil
		}
		row := ev.Y - m.profilesH() - 1
		if row < 0 {
			m.focus = focusList
			return m, nil
		}
		i := m.offset + row/2
		if i >= min(len(m.chats), m.offset+m.listRows()) {
			m.focus = focusList
			return m, nil
		}
		m.cursor = i
		m.focus = focusList
		if right {
			if !m.onSpecialRow() {
				m.openChatMenu()
			}
			return m, nil
		}
		return m.listKey("enter")
	}

	if m.info != nil {
		m.focus = focusChat
		return m, nil
	}
	if m.openJID == "" {
		return m, nil
	}
	switch msgsBottom := mainH - m.composeH(); {
	case ev.Y < headerH:
		m.enterNormal()
		m.focus = focusChat
		return m.runCommand("info")
	case ev.Y >= msgsBottom:
		if m.mode != modeInsert {
			m.enterInsert()
		}
		return m, nil
	}

	m.enterNormal()
	m.focus = focusChat
	line := m.vp.YOffset + ev.Y - headerH - 1
	for i, sp := range m.spans {
		if i < len(m.messages) && line >= sp[0] && line <= sp[1] {
			m.sel = i
			break
		}
	}
	m.renderMessages(false)
	if right {
		if m.hasSelection() {
			m.openSelectionMenu()
		} else {
			m.openMessageMenu()
		}
	}
	return m, m.markReadCmd()
}

// menuClick picks the clicked item; a click outside the box closes it.
func (m Model) menuClick(ev tea.MouseMsg, right bool) (tea.Model, tea.Cmd) {
	mn := m.menu
	_, bw, bh := m.menuView()
	bx, by := max(0, (m.width-bw)/2), max(0, (m.mainH()-bh)/2)
	if ev.X < bx || ev.X >= bx+bw || ev.Y < by || ev.Y >= by+bh {
		return m.menuKey(tea.KeyMsg{Type: tea.KeyEsc})
	}
	if right {
		return m, nil
	}
	row := ev.Y - by - 1
	if mn.filterable {
		row -= 2
	}
	items := mn.visible()
	start := 0
	if mn.cursor >= menuMaxRows {
		start = mn.cursor - menuMaxRows + 1
	}
	i := start + row
	if row < 0 || i >= min(len(items), start+menuMaxRows) {
		return m, nil
	}
	mn.cursor = i
	m.previewMenu()
	return m.menuKey(tea.KeyMsg{Type: tea.KeyEnter})
}

// stickerClick sends the clicked sticker; a click outside closes the picker.
func (m Model) stickerClick(ev tea.MouseMsg) (tea.Model, tea.Cmd) {
	sp := m.stickers
	_, bw, bh := m.stickerView()
	bx, by := max(0, (m.width-bw)/2), max(0, (m.mainH()-bh)/2)
	if ev.X < bx || ev.X >= bx+bw || ev.Y < by || ev.Y >= by+bh {
		m.stickers = nil
		return m, nil
	}
	per := m.stickerPerRow()
	cellW := stickerCols + stickerGapCol
	rowsTotal := (len(sp.refs) + per - 1) / per
	visibleRows := max(1, min(rowsTotal, (m.mainH()-6)/(stickerRows+2)))
	first := max(0, sp.sel/per-visibleRows+1)
	x, y := ev.X-bx-2, ev.Y-by-1
	if x < 0 || y < 0 {
		return m, nil
	}
	c, r := x/cellW, first+y/(stickerRows+1)
	i := r*per + c
	if c >= per || i >= len(sp.refs) {
		return m, nil
	}
	sp.sel = i
	return m.stickerKey("enter")
}
