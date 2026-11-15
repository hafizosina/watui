package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) setTheme(name string) error {
	t, err := LoadTheme(name)
	if err != nil {
		return err
	}
	ApplyTheme(t)
	m.renderMessages(false)
	return nil
}

func (m *Model) openThemePicker() {
	original := currentTheme
	var items []menuItem
	cursor := 0
	for i, name := range ThemeNames() {
		name := name
		label := name
		if name == original {
			label += "  · current"
			cursor = i
		}
		items = append(items, menuItem{label: label, run: func(m *Model) tea.Cmd {
			if err := m.setTheme(name); err != nil {
				m.setFlash(err.Error(), true)
				return nil
			}
			m.setFlash("theme "+name+" (for this session — set theme = "+name+" in ~/.config/watui/config to keep it)", false)
			return nil
		}})
	}
	m.menu = &menu{
		title:  "Theme · j/k previews",
		items:  items,
		cursor: cursor,
		preview: func(m *Model, it menuItem) {
			_ = m.setTheme(strings.Fields(it.label)[0])
		},
		cancel: func(m *Model) {
			_ = m.setTheme(original)
		},
	}
}
