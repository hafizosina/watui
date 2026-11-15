package ui

import (
	"sort"
	"strings"
	"sync"

	"github.com/kyokomi/emoji/v2"
)

type emojiEntry struct {
	code  string
	glyph string
}

var (
	emojiOnce  sync.Once
	emojiList  []emojiEntry
	emojiFirst = []string{"joy", "rofl", "smile", "grin", "sweat_smile", "heart", "heart_eyes", "thumbsup", "+1", "pray",
		"fire", "ok_hand", "sob", "cry", "thinking", "wink", "blush", "clap", "muscle", "tada", "100", "sunglasses",
		"kissing_heart", "laughing", "relieved", "pensive", "scream", "rage", "smirk", "upside_down_face", "wave", "eyes"}
)

func emojis() []emojiEntry {
	emojiOnce.Do(func() {
		for code, glyph := range emoji.CodeMap() {
			code = strings.Trim(code, ":")
			if strings.Contains(code, "skin_tone") || strings.Contains(code, "tone") && strings.Contains(code, "_tone") {
				continue
			}
			emojiList = append(emojiList, emojiEntry{code: code, glyph: strings.TrimSpace(glyph)})
		}
		rank := map[string]int{}
		for i, c := range emojiFirst {
			rank[c] = i + 1
		}
		sort.Slice(emojiList, func(i, j int) bool {
			ri, rj := rank[emojiList[i].code], rank[emojiList[j].code]
			if (ri > 0) != (rj > 0) {
				return ri > 0
			}
			if ri != rj {
				return ri < rj
			}
			if len(emojiList[i].code) != len(emojiList[j].code) {
				return len(emojiList[i].code) < len(emojiList[j].code)
			}
			return emojiList[i].code < emojiList[j].code
		})
	})
	return emojiList
}

func emojiMatches(query string, limit int) []emojiEntry {
	query = strings.ToLower(query)
	var prefix, inside []emojiEntry
	seen := map[string]bool{}
	for _, e := range emojis() {
		if seen[e.glyph] {
			continue
		}
		switch {
		case strings.HasPrefix(e.code, query):
			prefix = append(prefix, e)
			seen[e.glyph] = true
		case strings.Contains(e.code, query):
			inside = append(inside, e)
			seen[e.glyph] = true
		}
		if len(prefix) >= limit {
			break
		}
	}
	out := append(prefix, inside...)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func isShortcodeRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '+' || r == '-'
}

// emojiQuery returns the ":word" being typed right before the cursor, if any.
func (e *editor) emojiQuery() (int, string, bool) {
	if e.mode != vimInsert {
		return 0, "", false
	}
	i := e.cur
	for i > 0 && isShortcodeRune(e.buf[i-1]) {
		i--
	}
	if i == 0 || e.buf[i-1] != ':' || e.cur-i < 2 {
		return 0, "", false
	}
	colon := i - 1
	if colon > 0 {
		if prev := e.buf[colon-1]; prev != ' ' && prev != '\n' && prev != '\t' {
			return 0, "", false
		}
	}
	return colon, string(e.buf[i:e.cur]), true
}

func (e *editor) replaceBeforeCursor(from int, text string) {
	e.buf = append(e.buf[:from], append([]rune(text), e.buf[e.cur:]...)...)
	e.cur = from + len([]rune(text))
}
