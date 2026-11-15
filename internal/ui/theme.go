package ui

import (
	"hash/fnv"

	"github.com/charmbracelet/lipgloss"
)

var (
	colBg        = lipgloss.Color("#1a1b26")
	colBgDark    = lipgloss.Color("#16161e")
	colHighlight = lipgloss.Color("#283457")
	colGutter    = lipgloss.Color("#3b4261")
	colComment   = lipgloss.Color("#565f89")
	colFgDark    = lipgloss.Color("#a9b1d6")
	colFg        = lipgloss.Color("#c0caf5")
	colBlue      = lipgloss.Color("#7aa2f7")
	colCyan      = lipgloss.Color("#7dcfff")
	colGreen     = lipgloss.Color("#9ece6a")
	colMagenta   = lipgloss.Color("#bb9af7")
	colOrange    = lipgloss.Color("#ff9e64")
	colRed       = lipgloss.Color("#f7768e")
	colYellow    = lipgloss.Color("#e0af68")
	colTeal      = lipgloss.Color("#1abc9c")
)

var senderColors = []lipgloss.Color{colCyan, colGreen, colMagenta, colOrange, colYellow, colTeal, colRed, lipgloss.Color("#2ac3de"), lipgloss.Color("#ff007c")}

func senderColor(jid string) lipgloss.Color {
	h := fnv.New32a()
	h.Write([]byte(jid))
	return senderColors[h.Sum32()%uint32(len(senderColors))]
}

var (
	styleBorder     = lipgloss.NewStyle().Foreground(colGutter)
	styleBorderOn   = lipgloss.NewStyle().Foreground(colBlue)
	styleTitle      = lipgloss.NewStyle().Foreground(colBlue).Bold(true)
	styleDim        = lipgloss.NewStyle().Foreground(colComment)
	styleText       = lipgloss.NewStyle().Foreground(colFg)
	styleName       = lipgloss.NewStyle().Foreground(colFg).Bold(true)
	styleUnreadName = lipgloss.NewStyle().Foreground(colFg).Bold(true)
	styleBadge      = lipgloss.NewStyle().Foreground(colBgDark).Background(colGreen).Bold(true)
	styleBadgeMuted = lipgloss.NewStyle().Foreground(colBgDark).Background(colComment).Bold(true)
	styleSelected   = lipgloss.NewStyle().Background(colHighlight)
	styleMe         = lipgloss.NewStyle().Foreground(colBlue).Bold(true)
	styleDay        = lipgloss.NewStyle().Foreground(colComment)
	styleDeleted    = lipgloss.NewStyle().Foreground(colComment).Italic(true)
	styleRead       = lipgloss.NewStyle().Foreground(colCyan)
	styleFailed     = lipgloss.NewStyle().Foreground(colRed)
	styleError      = lipgloss.NewStyle().Foreground(colRed)
	styleInfo       = lipgloss.NewStyle().Foreground(colFgDark)
	styleStatusBar  = lipgloss.NewStyle().Background(colBgDark).Foreground(colFgDark)
	styleKey        = lipgloss.NewStyle().Foreground(colBlue).Bold(true)
	styleMuted      = lipgloss.NewStyle().Foreground(colOrange).Bold(true)
)

func modeStyle(m mode) lipgloss.Style {
	base := lipgloss.NewStyle().Foreground(colBgDark).Bold(true).Padding(0, 1)
	switch m {
	case modeInsert:
		return base.Background(colGreen)
	case modeCommand:
		return base.Background(colYellow)
	case modeSearch:
		return base.Background(colMagenta)
	}
	return base.Background(colBlue)
}
