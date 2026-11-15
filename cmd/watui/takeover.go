package main

import (
	"errors"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	tkGreen   = lipgloss.Color("#9ece6a")
	tkBlue    = lipgloss.Color("#7aa2f7")
	tkYellow  = lipgloss.Color("#e0af68")
	tkRed     = lipgloss.Color("#f7768e")
	tkFg      = lipgloss.Color("#c0caf5")
	tkComment = lipgloss.Color("#565f89")
	tkSelect  = lipgloss.Color("#283457")
)

type takeoverState int

const (
	tkAsking takeoverState = iota
	tkClosing
)

type takeoverDone struct {
	lock   *instanceLock
	forced bool
	err    error
}

type takeoverModel struct {
	dir     string
	running *runningError
	cursor  int
	state   takeoverState
	width   int
	height  int
	result  takeoverDone
	chose   bool
}

func takeOverPrompt(dir string, running *runningError) (*instanceLock, error) {
	final, err := tea.NewProgram(takeoverModel{dir: dir, running: running}, tea.WithAltScreen()).Run()
	if err != nil {
		return nil, err
	}
	m := final.(takeoverModel)
	if !m.chose {
		return nil, errors.New("not opened: another watui is running")
	}
	return m.result.lock, m.result.err
}

func (m takeoverModel) Init() tea.Cmd { return nil }

func (m takeoverModel) close() (tea.Model, tea.Cmd) {
	m.state = tkClosing
	m.chose = true
	dir, pid := m.dir, m.running.pid
	return m, func() tea.Msg {
		lock, forced, err := takeOver(dir, pid)
		return takeoverDone{lock: lock, forced: forced, err: err}
	}
}

func (m takeoverModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case takeoverDone:
		m.result = msg
		return m, tea.Quit
	case tea.KeyMsg:
		if m.state == tkClosing {
			return m, nil
		}
		switch msg.String() {
		case "j", "down", "ctrl+n", "tab", "k", "up", "ctrl+p", "shift+tab":
			m.cursor = 1 - m.cursor
		case "y", "Y":
			return m.close()
		case "n", "N", "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "enter":
			if m.cursor == 0 {
				return m.close()
			}
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m takeoverModel) View() string {
	if m.width == 0 {
		return ""
	}
	title := lipgloss.NewStyle().Foreground(tkYellow).Bold(true).Render("watui is already running")
	label := lipgloss.NewStyle().Foreground(tkComment).Width(10)
	value := lipgloss.NewStyle().Foreground(tkFg)
	r := m.running
	var info []string
	add := func(k, v string) {
		if v != "" {
			info = append(info, label.Render(k)+value.Render(v))
		}
	}
	add("process", itoa(r.pid))
	add("program", r.exe)
	add("terminal", r.tty)
	add("tmux", strings.TrimPrefix(r.pane, "window "))

	note := lipgloss.NewStyle().Foreground(tkComment).Render("Two copies share one WhatsApp login and kick each other off.")

	var body string
	if m.state == tkClosing {
		body = lipgloss.NewStyle().Foreground(tkBlue).Render("◌ closing the other watui…") + "\n" +
			lipgloss.NewStyle().Foreground(tkComment).Render("  asking it to quit cleanly, forcing it after 5s")
	} else {
		opts := []string{"Close it and open here", "Cancel"}
		var rows []string
		for i, o := range opts {
			style := lipgloss.NewStyle().Foreground(tkFg).Padding(0, 1)
			key := "y"
			if i == 1 {
				key = "n"
				style = style.Foreground(tkComment)
			}
			if i == 0 {
				style = style.Foreground(tkRed)
			}
			line := o + strings.Repeat(" ", max(1, 30-len(o))) + lipgloss.NewStyle().Foreground(tkBlue).Render(key)
			if i == m.cursor {
				style = style.Background(tkSelect).Bold(true)
				line = o + strings.Repeat(" ", max(1, 30-len(o))) + lipgloss.NewStyle().Foreground(tkBlue).Background(tkSelect).Render(key)
			}
			rows = append(rows, style.Render(line))
		}
		body = strings.Join(rows, "\n")
	}

	hint := lipgloss.NewStyle().Foreground(tkComment).Render("y/n · j/k ^n/^p move · enter pick · esc cancel")
	content := strings.Join([]string{title, "", strings.Join(info, "\n"), "", note, "", body, "", hint}, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tkGreen).
		Padding(1, 3).
		Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func itoa(n int) string {
	if n <= 0 {
		return ""
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
