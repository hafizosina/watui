package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func mouse(b tea.MouseButton, x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: b, Action: tea.MouseActionPress}
}

func TestMouse(t *testing.T) {
	m := step(testModel(t), tea.WindowSizeMsg{Width: 110, Height: 30}, ChangedMsg{})
	listW := m.listWidth()
	firstChatY := m.profilesH() + 1
	if m.chats[0].JID == rowArchived {
		firstChatY += 2
	}

	// left click on a chat opens it
	m = step(m, mouse(tea.MouseButtonLeft, 3, firstChatY))
	if m.openJID == "" || m.focus != focusChat {
		t.Fatalf("click on a chat should open it, open=%q focus=%v", m.openJID, m.focus)
	}
	n := len(m.messages)
	if n < 2 {
		t.Fatalf("need ≥2 messages, got %d", n)
	}

	// wheel over the messages moves the selection
	sel := m.sel
	m = step(m, mouse(tea.MouseButtonWheelUp, listW+5, headerH+2))
	if m.sel >= sel {
		t.Fatalf("wheel up should move up from %d, got %d", sel, m.sel)
	}
	m = step(m, mouse(tea.MouseButtonWheelDown, listW+5, headerH+2))
	if m.sel != sel {
		t.Fatalf("wheel down should come back to %d, got %d", sel, m.sel)
	}

	// clicking a message selects it
	sp := m.spans[0]
	if y := sp[0] - m.vp.YOffset + headerH + 1; y > headerH && y < m.mainH()-m.composeH() {
		m = step(m, mouse(tea.MouseButtonLeft, listW+5, y))
		if m.sel != 0 {
			t.Fatalf("click on the first message should select it, sel=%d", m.sel)
		}
	}

	// right click opens the message menu, a click outside closes it
	m = step(m, mouse(tea.MouseButtonRight, listW+5, headerH+2))
	if m.menu == nil {
		t.Fatal("right click on a message should open the menu")
	}
	m = step(m, mouse(tea.MouseButtonLeft, 0, 0))
	if m.menu != nil {
		t.Fatal("click outside should close the menu")
	}

	// click on the compose box starts writing
	m = step(m, mouse(tea.MouseButtonLeft, listW+5, m.mainH()-2))
	if m.mode != modeInsert {
		t.Fatalf("click on compose should enter insert, mode=%v", m.mode)
	}

	// wheel over the chat list moves its cursor
	cur := m.cursor
	m = step(m, mouse(tea.MouseButtonWheelDown, 3, firstChatY))
	if m.cursor != min(cur+1, len(m.chats)-1) {
		t.Fatalf("wheel over the list should move the cursor, %d → %d", cur, m.cursor)
	}

	// right click on a chat opens its menu; clicking an item runs it
	m = step(m, mouse(tea.MouseButtonRight, 3, firstChatY))
	if m.menu == nil {
		t.Fatal("right click on a chat should open the chat menu")
	}
	_, bw, bh := m.menuView()
	bx, by := (m.width-bw)/2, (m.mainH()-bh)/2
	before := m.menu
	m = step(m, mouse(tea.MouseButtonLeft, bx+3, by+1))
	if m.menu == before {
		t.Fatal("clicking a menu item should run it")
	}
}
