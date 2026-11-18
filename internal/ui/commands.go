package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/hafizosina/watui/internal/config"
)

type command struct {
	name string
	args string
	desc string
}

var commands = []command{
	{"new", "", "new chat or group"},
	{"info", "", "details of the open chat (contact / group)"},
	{"me", "", "your own profile"},
	{"profile", "<name>|add|rename", "switch, add or rename a profile"},
	{"theme", "<name>", "pick a color theme"},
	{"sticker", "", "send a recent sticker"},
	{"upload", "[path…]", "send files (no path: file picker)"},
	{"big", "", "large Compose on / off (ctrl+g)"},
	{"archived", "", "show / hide archived chats"},
	{"read", "", "mark the open chat as read"},
	{"pin", "", "pin the chat"},
	{"unpin", "", "unpin the chat"},
	{"mute", "", "mute the chat"},
	{"unmute", "", "unmute the chat"},
	{"archive", "", "archive the chat"},
	{"unarchive", "", "unarchive the chat"},
	{"pause", "", "pause / resume the voice note"},
	{"speed", "", "voice note speed 1× → 1.5× → 2×"},
	{"stop", "", "stop the voice note"},
	{"set", "<key>=<value>", "change a setting for this session"},
	{"help", "", "keybindings"},
	{"quit", "", "quit watui"},
}

var settingValues = map[string][]string{
	"images":                {"always", "thumbnail", "off"},
	"notify":                {"on", "private", "off"},
	"notify_sound":          {"watui", "sword", "chime", "soft", "ping", "system", "off"},
	"notify_sound_personal": {"sword", "chime", "soft", "ping", "system", "off", ""},
	"notify_sound_mention":  {"sword", "chime", "soft", "ping", "system", "off", ""},
	"notify_sound_group":    {"chime", "sword", "soft", "ping", "system", "off", ""},
	"collapse_lines":        {"0", "15", "30"},
	"show_deleted":          {"on", "off"},
	"image_width":           {"30", "40", "60"},
	"image_height":          {"10", "15", "20"},
	"theme":                 nil,
}

type suggestion struct {
	value string
	label string
	desc  string
}

// commandSuggestions completes what's typed after ":".
func (m Model) commandSuggestions(input string) []suggestion {
	name, rest, hasArg := strings.Cut(input, " ")
	if !hasArg {
		var out []suggestion
		for _, c := range commands {
			if strings.HasPrefix(c.name, name) {
				out = append(out, suggestion{value: c.name, label: c.name, desc: c.desc})
			}
		}
		if len(out) == 0 && name != "" {
			for _, c := range commands {
				if strings.Contains(c.name, name) {
					out = append(out, suggestion{value: c.name, label: c.name, desc: c.desc})
				}
			}
		}
		return out
	}
	var options []suggestion
	switch name {
	case "theme", "themes", "colorscheme", "colo":
		for _, t := range ThemeNames() {
			d := ""
			if t == currentTheme {
				d = "current"
			}
			options = append(options, suggestion{value: name + " " + t, label: t, desc: d})
		}
	case "profile", "profiles", "p":
		sub, _, hasSub := strings.Cut(rest, " ")
		if hasSub && sub == "rename" {
			for _, p := range m.profiles {
				options = append(options, suggestion{value: name + " rename " + p.Name + " ", label: p.Name, desc: "new label after it"})
			}
			return filterSuggestions(options, rest[len("rename "):], name+" rename ")
		}
		options = append(options,
			suggestion{value: name + " add ", label: "add", desc: "link another account"},
			suggestion{value: name + " rename ", label: "rename", desc: "change a profile's label"})
		for _, p := range m.profiles {
			options = append(options, suggestion{value: name + " " + p.Name, label: p.Name, desc: "switch"})
		}
	case "set":
		key, _, hasEq := strings.Cut(rest, "=")
		if hasEq {
			vals := settingValues[key]
			if key == "theme" {
				vals = ThemeNames()
			}
			for _, v := range vals {
				options = append(options, suggestion{value: "set " + key + "=" + v, label: v})
			}
			return filterSuggestions(options, strings.TrimPrefix(rest, key+"="), "set "+key+"=")
		}
		for _, k := range sortedKeys(settingValues) {
			options = append(options, suggestion{value: "set " + k + "=", label: k + "="})
		}
	default:
		return nil
	}
	return filterSuggestions(options, rest, name+" ")
}

func filterSuggestions(options []suggestion, typed, prefix string) []suggestion {
	var out []suggestion
	for _, o := range options {
		if strings.HasPrefix(strings.TrimPrefix(o.value, prefix), typed) {
			out = append(out, o)
		}
	}
	return out
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

const maxSuggestions = 10

func (m Model) suggestionView(list []suggestion) (string, int, int) {
	w := 20
	for _, s := range list {
		w = max(w, lipgloss.Width(s.label)+lipgloss.Width(s.desc)+8)
	}
	w = min(w, max(24, m.width-4))
	start := 0
	if m.cmdSel >= maxSuggestions {
		start = m.cmdSel - maxSuggestions + 1
	}
	var rows []string
	for i := start; i < len(list) && i < start+maxSuggestions; i++ {
		s := list[i]
		bg := lipgloss.NewStyle()
		if i == m.cmdSel {
			bg = bg.Background(colHighlight)
		}
		label := bg.Foreground(colBlue).Bold(i == m.cmdSel).Render(" " + s.label)
		desc := ""
		if s.desc != "" {
			desc = bg.Foreground(colComment).Render("  " + ansi.Truncate(s.desc, max(4, w-lipgloss.Width(label)-4), "…"))
		}
		row := label + desc
		rows = append(rows, row+bg.Render(strings.Repeat(" ", max(0, w-2-lipgloss.Width(row)))))
	}
	h := len(rows) + 2
	return panel("Commands", "tab / ^n next · enter run", strings.Join(rows, "\n"), w, h, true), w, h
}

// kindSounds lists the per-kind sounds that are set, for :set.
func kindSounds(cfg config.Config) string {
	out := ""
	for _, kv := range [][2]string{
		{"notify_sound_personal", cfg.NotifySoundPersonal},
		{"notify_sound_mention", cfg.NotifySoundMention},
		{"notify_sound_group", cfg.NotifySoundGroup},
	} {
		if kv[1] != "" {
			out += " " + kv[0] + "=" + kv[1]
		}
	}
	return out
}
