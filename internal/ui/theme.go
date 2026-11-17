package ui

import (
	"bufio"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type Theme struct {
	Name      string
	Bg        lipgloss.Color
	BgDark    lipgloss.Color
	Highlight lipgloss.Color
	Gutter    lipgloss.Color
	Comment   lipgloss.Color
	FgDark    lipgloss.Color
	Fg        lipgloss.Color
	Blue      lipgloss.Color
	Cyan      lipgloss.Color
	Green     lipgloss.Color
	Magenta   lipgloss.Color
	Orange    lipgloss.Color
	Red       lipgloss.Color
	Yellow    lipgloss.Color
	Teal      lipgloss.Color
	OnAccent  lipgloss.Color
}

var builtinThemes = map[string]Theme{
	"tokyonight": {Bg: "#1a1b26", BgDark: "#16161e", Highlight: "#283457", Gutter: "#3b4261", Comment: "#565f89", FgDark: "#a9b1d6", Fg: "#c0caf5",
		Blue: "#7aa2f7", Cyan: "#7dcfff", Green: "#9ece6a", Magenta: "#bb9af7", Orange: "#ff9e64", Red: "#f7768e", Yellow: "#e0af68", Teal: "#1abc9c", OnAccent: "#16161e"},
	"tokyonight-day": {Bg: "#e1e2e7", BgDark: "#d0d5e3", Highlight: "#b7c1e3", Gutter: "#a8aecb", Comment: "#848cb5", FgDark: "#6172b0", Fg: "#3760bf",
		Blue: "#2e7de9", Cyan: "#007197", Green: "#587539", Magenta: "#9854f1", Orange: "#b15c00", Red: "#f52a65", Yellow: "#8c6c3e", Teal: "#118c74", OnAccent: "#ffffff"},
	"catppuccin-mocha": {Bg: "#1e1e2e", BgDark: "#181825", Highlight: "#313244", Gutter: "#45475a", Comment: "#6c7086", FgDark: "#a6adc8", Fg: "#cdd6f4",
		Blue: "#89b4fa", Cyan: "#89dceb", Green: "#a6e3a1", Magenta: "#cba6f7", Orange: "#fab387", Red: "#f38ba8", Yellow: "#f9e2af", Teal: "#94e2d5", OnAccent: "#11111b"},
	"catppuccin-latte": {Bg: "#eff1f5", BgDark: "#e6e9ef", Highlight: "#ccd0da", Gutter: "#bcc0cc", Comment: "#9ca0b0", FgDark: "#6c6f85", Fg: "#4c4f69",
		Blue: "#1e66f5", Cyan: "#04a5e5", Green: "#40a02b", Magenta: "#8839ef", Orange: "#fe640b", Red: "#d20f39", Yellow: "#df8e1d", Teal: "#179299", OnAccent: "#ffffff"},
	"gruvbox-dark": {Bg: "#282828", BgDark: "#1d2021", Highlight: "#3c3836", Gutter: "#504945", Comment: "#928374", FgDark: "#bdae93", Fg: "#ebdbb2",
		Blue: "#83a598", Cyan: "#8ec07c", Green: "#b8bb26", Magenta: "#d3869b", Orange: "#fe8019", Red: "#fb4934", Yellow: "#fabd2f", Teal: "#689d6a", OnAccent: "#1d2021"},
	"nord": {Bg: "#2e3440", BgDark: "#242933", Highlight: "#3b4252", Gutter: "#4c566a", Comment: "#616e88", FgDark: "#d8dee9", Fg: "#eceff4",
		Blue: "#81a1c1", Cyan: "#88c0d0", Green: "#a3be8c", Magenta: "#b48ead", Orange: "#d08770", Red: "#bf616a", Yellow: "#ebcb8b", Teal: "#8fbcbb", OnAccent: "#242933"},
	"dracula": {Bg: "#282a36", BgDark: "#21222c", Highlight: "#44475a", Gutter: "#44475a", Comment: "#6272a4", FgDark: "#bfbfbf", Fg: "#f8f8f2",
		Blue: "#bd93f9", Cyan: "#8be9fd", Green: "#50fa7b", Magenta: "#ff79c6", Orange: "#ffb86c", Red: "#ff5555", Yellow: "#f1fa8c", Teal: "#8be9fd", OnAccent: "#21222c"},
}

const DefaultTheme = "tokyonight"

var (
	colBg, colBgDark, colHighlight, colGutter, colComment, colFgDark, colFg       lipgloss.Color
	colBlue, colCyan, colGreen, colMagenta, colOrange, colRed, colYellow, colTeal lipgloss.Color
	colOnAccent                                                                   lipgloss.Color
	senderColors                                                                  []lipgloss.Color
	currentTheme                                                                  string

	styleBorder, styleBorderOn, styleTitle, styleDim, styleText, styleName lipgloss.Style
	styleUnreadName, styleBadge, styleBadgeMuted, styleSelected, styleMe   lipgloss.Style
	styleBadgeForMe, styleMention, styleMentionMe                          lipgloss.Style
	styleSearchHit                                                         lipgloss.Style
	styleDay, styleDeleted, styleRead, styleFailed, styleError, styleInfo  lipgloss.Style
	styleStatusBar, styleKey, styleMuted                                   lipgloss.Style
)

func init() {
	ApplyTheme(builtinThemes[DefaultTheme])
	currentTheme = DefaultTheme
}

func themesDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "watui", "themes")
}

// ThemeNames lists built-in themes plus any in ~/.config/watui/themes.
func ThemeNames() []string {
	seen := map[string]bool{}
	var names []string
	for n := range builtinThemes {
		names = append(names, n)
		seen[n] = true
	}
	entries, _ := os.ReadDir(themesDir())
	for _, e := range entries {
		n := strings.TrimSuffix(e.Name(), ".conf")
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".conf") && !seen[n] {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

// LoadTheme finds a theme by name: a file in ~/.config/watui/themes/<name>.conf
// (missing colors fall back to its "base" or tokyonight), or a built-in one.
func LoadTheme(name string) (Theme, error) {
	path := filepath.Join(themesDir(), name+".conf")
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		t, ok := builtinThemes[name]
		if !ok {
			return Theme{}, fmt.Errorf("unknown theme %q (have: %s)", name, strings.Join(ThemeNames(), ", "))
		}
		t.Name = name
		return t, nil
	}
	if err != nil {
		return Theme{}, err
	}
	defer f.Close()
	values := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return Theme{}, fmt.Errorf("%s: expected key = value, got %q", path, line)
		}
		values[strings.ToLower(strings.TrimSpace(k))] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	t := builtinThemes[DefaultTheme]
	if base, ok := builtinThemes[values["base"]]; ok {
		t = base
	}
	fields := map[string]*lipgloss.Color{
		"bg": &t.Bg, "bg_dark": &t.BgDark, "highlight": &t.Highlight, "gutter": &t.Gutter, "comment": &t.Comment,
		"fg_dark": &t.FgDark, "fg": &t.Fg, "blue": &t.Blue, "cyan": &t.Cyan, "green": &t.Green, "magenta": &t.Magenta,
		"orange": &t.Orange, "red": &t.Red, "yellow": &t.Yellow, "teal": &t.Teal, "on_accent": &t.OnAccent,
	}
	for k, v := range values {
		if k == "base" {
			continue
		}
		dst, ok := fields[k]
		if !ok {
			return Theme{}, fmt.Errorf("%s: unknown color %q", path, k)
		}
		if !strings.HasPrefix(v, "#") || (len(v) != 7 && len(v) != 4) {
			return Theme{}, fmt.Errorf("%s: %s must be a hex color like #7aa2f7, got %q", path, k, v)
		}
		*dst = lipgloss.Color(v)
	}
	t.Name = name
	return t, sc.Err()
}

func ApplyTheme(t Theme) {
	colBg, colBgDark, colHighlight, colGutter, colComment = t.Bg, t.BgDark, t.Highlight, t.Gutter, t.Comment
	colFgDark, colFg, colBlue, colCyan, colGreen = t.FgDark, t.Fg, t.Blue, t.Cyan, t.Green
	colMagenta, colOrange, colRed, colYellow, colTeal, colOnAccent = t.Magenta, t.Orange, t.Red, t.Yellow, t.Teal, t.OnAccent
	senderColors = []lipgloss.Color{colCyan, colGreen, colMagenta, colOrange, colYellow, colTeal, colRed, colBlue}
	if t.Name != "" {
		currentTheme = t.Name
	}

	styleBorder = lipgloss.NewStyle().Foreground(colGutter)
	styleBorderOn = lipgloss.NewStyle().Foreground(colBlue)
	styleTitle = lipgloss.NewStyle().Foreground(colBlue).Bold(true)
	styleDim = lipgloss.NewStyle().Foreground(colComment)
	styleText = lipgloss.NewStyle().Foreground(colFg)
	styleName = lipgloss.NewStyle().Foreground(colFg).Bold(true)
	styleUnreadName = lipgloss.NewStyle().Foreground(colFg).Bold(true)
	styleBadge = lipgloss.NewStyle().Foreground(colOnAccent).Background(colGreen).Bold(true)
	styleBadgeMuted = lipgloss.NewStyle().Foreground(colOnAccent).Background(colComment).Bold(true)
	styleBadgeForMe = lipgloss.NewStyle().Foreground(colOnAccent).Background(colMagenta).Bold(true)
	styleMention = lipgloss.NewStyle().Foreground(colBlue).Bold(true)
	styleMentionMe = lipgloss.NewStyle().Foreground(colMagenta).Bold(true)             // same color as the @ badge
	styleSearchHit = lipgloss.NewStyle().Foreground(colOnAccent).Background(colYellow) // like vim's search highlight
	styleSelected = lipgloss.NewStyle().Background(colHighlight)
	styleMe = lipgloss.NewStyle().Foreground(colBlue).Bold(true)
	styleDay = lipgloss.NewStyle().Foreground(colComment)
	styleDeleted = lipgloss.NewStyle().Foreground(colComment).Italic(true)
	styleRead = lipgloss.NewStyle().Foreground(colCyan)
	styleFailed = lipgloss.NewStyle().Foreground(colRed)
	styleError = lipgloss.NewStyle().Foreground(colRed)
	styleInfo = lipgloss.NewStyle().Foreground(colFgDark)
	styleStatusBar = lipgloss.NewStyle().Background(colBgDark).Foreground(colFgDark)
	styleKey = lipgloss.NewStyle().Foreground(colBlue).Bold(true)
	styleMuted = lipgloss.NewStyle().Foreground(colOrange).Bold(true)
}

func senderColor(jid string) lipgloss.Color {
	h := fnv.New32a()
	h.Write([]byte(jid))
	return senderColors[h.Sum32()%uint32(len(senderColors))]
}

func modeStyle(m mode) lipgloss.Style {
	base := lipgloss.NewStyle().Foreground(colOnAccent).Bold(true).Padding(0, 1)
	switch m {
	case modeInsert:
		return base.Background(colGreen)
	case modeCommand:
		return base.Background(colYellow)
	case modeSearch, modeMsgSearch:
		return base.Background(colMagenta)
	}
	return base.Background(colBlue)
}
