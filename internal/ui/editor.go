package ui

import (
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type vimMode int

const (
	vimInsert vimMode = iota
	vimNormal
	vimVisual
	vimVisualLine
)

func (v vimMode) String() string {
	switch v {
	case vimNormal:
		return "NORMAL"
	case vimVisual:
		return "VISUAL"
	case vimVisualLine:
		return "V-LINE"
	}
	return "INSERT"
}

type editResult int

const (
	editNone editResult = iota
	editLeave
)

type snapshot struct {
	buf []rune
	cur int
}

type editor struct {
	buf     []rune
	cur     int
	goalCol int
	mode    vimMode
	anchor  int
	focused bool

	pending  string
	register []rune
	regLine  bool
	undo     []snapshot
	redo     []snapshot

	lastJ    time.Time
	lastJPos int

	width       int
	height      int
	scroll      int
	placeholder string
}

func newEditor() *editor {
	return &editor{mode: vimInsert, height: 3, placeholder: "press i to write a message"}
}

func (e *editor) Value() string      { return string(e.buf) }
func (e *editor) Height() int        { return e.height }
func (e *editor) SetWidth(w int)     { e.width = max(4, w) }
func (e *editor) Focus(mode vimMode) { e.focused, e.mode = true, mode }
func (e *editor) Blur()              { e.focused = false; e.pending = "" }

func (e *editor) SetValue(s string) {
	e.buf = []rune(s)
	e.cur = len(e.buf)
	e.undo, e.redo = nil, nil
}

func (e *editor) Reset() {
	e.buf, e.cur, e.scroll = nil, 0, 0
	e.undo, e.redo = nil, nil
	e.pending = ""
}

func (e *editor) CursorEnd() { e.cur = len(e.buf) }

func (e *editor) save() {
	e.undo = append(e.undo, snapshot{buf: append([]rune(nil), e.buf...), cur: e.cur})
	if len(e.undo) > 200 {
		e.undo = e.undo[1:]
	}
	e.redo = nil
}

func (e *editor) lineStart(i int) int {
	for i > 0 && e.buf[i-1] != '\n' {
		i--
	}
	return i
}

func (e *editor) lineEnd(i int) int {
	for i < len(e.buf) && e.buf[i] != '\n' {
		i++
	}
	return i
}

func (e *editor) col() int { return e.cur - e.lineStart(e.cur) }

func (e *editor) clampNormal() {
	e.cur = max(0, min(e.cur, len(e.buf)))
	if e.mode == vimInsert {
		return
	}
	if e.cur > e.lineStart(e.cur) && e.cur == e.lineEnd(e.cur) {
		e.cur--
	}
}

func (e *editor) insert(rs []rune) {
	e.buf = append(e.buf[:e.cur], append(append([]rune(nil), rs...), e.buf[e.cur:]...)...)
	e.cur += len(rs)
}

func (e *editor) del(from, to int) []rune {
	from, to = max(0, from), min(len(e.buf), to)
	if from >= to {
		return nil
	}
	cut := append([]rune(nil), e.buf[from:to]...)
	e.buf = append(e.buf[:from], e.buf[to:]...)
	e.cur = from
	return cut
}

type class int

func classOf(r rune) class {
	switch {
	case unicode.IsSpace(r):
		return 0
	case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
		return 1
	}
	return 2
}

func (e *editor) wordForward(i int) int {
	n := len(e.buf)
	if i >= n {
		return n
	}
	c := classOf(e.buf[i])
	if c != 0 {
		for i < n && classOf(e.buf[i]) == c {
			i++
		}
	}
	for i < n && classOf(e.buf[i]) == 0 {
		i++
	}
	return i
}

func (e *editor) wordEnd(i int) int {
	n := len(e.buf)
	i++
	for i < n && classOf(e.buf[i]) == 0 {
		i++
	}
	if i >= n {
		return max(0, n-1)
	}
	c := classOf(e.buf[i])
	for i+1 < n && classOf(e.buf[i+1]) == c {
		i++
	}
	return i
}

func (e *editor) wordBack(i int) int {
	i--
	for i > 0 && classOf(e.buf[i]) == 0 {
		i--
	}
	if i <= 0 {
		return 0
	}
	c := classOf(e.buf[i])
	for i > 0 && classOf(e.buf[i-1]) == c {
		i--
	}
	return i
}

func (e *editor) innerWord(around bool) (int, int) {
	if len(e.buf) == 0 {
		return 0, 0
	}
	i := min(e.cur, len(e.buf)-1)
	c := classOf(e.buf[i])
	from, to := i, i+1
	for from > 0 && classOf(e.buf[from-1]) == c && e.buf[from-1] != '\n' {
		from--
	}
	for to < len(e.buf) && classOf(e.buf[to]) == c && e.buf[to] != '\n' {
		to++
	}
	if around {
		for to < len(e.buf) && e.buf[to] == ' ' {
			to++
		}
	}
	return from, to
}

func (e *editor) firstNonBlank(i int) int {
	i = e.lineStart(i)
	for i < len(e.buf) && (e.buf[i] == ' ' || e.buf[i] == '\t') {
		i++
	}
	return i
}

func (e *editor) vertical(delta int) {
	goal := e.goalCol
	for ; delta > 0; delta-- {
		end := e.lineEnd(e.cur)
		if end >= len(e.buf) {
			break
		}
		e.cur = end + 1
	}
	for ; delta < 0; delta++ {
		start := e.lineStart(e.cur)
		if start == 0 {
			break
		}
		e.cur = e.lineStart(start - 1)
	}
	e.cur = min(e.lineStart(e.cur)+goal, e.lineEnd(e.cur))
	e.clampNormal()
	e.goalCol = goal
}

func (e *editor) motion(key string) (int, bool) {
	switch key {
	case "h", "left", "backspace":
		if e.cur > e.lineStart(e.cur) {
			return e.cur - 1, true
		}
		return e.cur, true
	case "l", "right", " ":
		if e.cur < e.lineEnd(e.cur) {
			return e.cur + 1, true
		}
		return e.cur, true
	case "w", "W":
		return e.wordForward(e.cur), true
	case "e", "E":
		return e.wordEnd(e.cur), true
	case "b", "B":
		return e.wordBack(e.cur), true
	case "0", "home":
		return e.lineStart(e.cur), true
	case "^":
		return e.firstNonBlank(e.cur), true
	case "$", "end":
		return e.lineEnd(e.cur), true
	case "G":
		return e.lineStart(len(e.buf)), true
	}
	return 0, false
}

func (e *editor) HandleKey(k tea.KeyMsg) editResult {
	key := k.String()
	switch e.mode {
	case vimInsert:
		return e.insertKey(k, key)
	case vimVisual, vimVisualLine:
		return e.visualKey(key)
	}
	return e.normalKey(key)
}

func (e *editor) insertKey(k tea.KeyMsg, key string) editResult {
	switch key {
	case "esc", "ctrl+[", "ctrl+c":
		e.toNormal()
		return editNone
	case "enter", "ctrl+j":
		e.insert([]rune{'\n'})
	case "backspace":
		if e.cur > 0 {
			e.del(e.cur-1, e.cur)
		}
	case "delete":
		e.del(e.cur, e.cur+1)
	case "left":
		e.cur = max(0, e.cur-1)
	case "right":
		e.cur = min(len(e.buf), e.cur+1)
	case "up":
		e.goalCol = e.col()
		e.vertical(-1)
	case "down":
		e.goalCol = e.col()
		e.vertical(1)
	case "home", "ctrl+a":
		e.cur = e.lineStart(e.cur)
	case "end", "ctrl+e":
		e.cur = e.lineEnd(e.cur)
	case "ctrl+w":
		e.del(e.wordBack(e.cur), e.cur)
	case "ctrl+u":
		e.del(e.lineStart(e.cur), e.cur)
	case "tab":
		e.insert([]rune("    "))
	default:
		if k.Type != tea.KeyRunes && k.Type != tea.KeySpace {
			return editNone
		}
		rs := k.Runes
		if k.Type == tea.KeySpace {
			rs = []rune{' '}
		}
		if !k.Paste && len(rs) == 1 {
			if rs[0] == 'k' && e.cur > 0 && e.cur == e.lastJPos && time.Since(e.lastJ) < 500*time.Millisecond && e.buf[e.cur-1] == 'j' {
				e.del(e.cur-1, e.cur)
				e.toNormal()
				return editNone
			}
			if rs[0] == 'j' {
				e.lastJ, e.lastJPos = time.Now(), e.cur+1
			}
		}
		e.insert(rs)
	}
	return editNone
}

func (e *editor) toNormal() {
	if e.mode == vimInsert && e.cur > e.lineStart(e.cur) {
		e.cur--
	}
	e.mode = vimNormal
	e.pending = ""
	e.clampNormal()
}

func (e *editor) toInsert(at int) {
	e.save()
	e.cur = max(0, min(at, len(e.buf)))
	e.mode = vimInsert
}

func (e *editor) yank(from, to int, line bool) {
	from, to = max(0, from), min(len(e.buf), to)
	if from < to {
		e.register = append([]rune(nil), e.buf[from:to]...)
		e.regLine = line
	}
}

func (e *editor) lineRange(i int) (int, int) {
	from, to := e.lineStart(i), e.lineEnd(i)
	if to < len(e.buf) {
		to++
	} else if from > 0 {
		from--
	}
	return from, to
}

func (e *editor) operate(op string, from, to int, line bool) {
	if from > to {
		from, to = to, from
	}
	e.yank(from, to, line)
	if op == "y" {
		e.cur = from
		e.clampNormal()
		return
	}
	e.save()
	e.del(from, to)
	if op == "c" {
		e.mode = vimInsert
		return
	}
	e.clampNormal()
}

func (e *editor) normalKey(key string) editResult {
	if e.pending != "" {
		return e.pendingKey(key)
	}
	if to, ok := e.motion(key); ok {
		e.cur = to
		e.clampNormal()
		e.goalCol = e.col()
		return editNone
	}
	switch key {
	case "esc", "ctrl+[", "ctrl+c":
		return editLeave
	case "j", "down", "enter":
		e.vertical(1)
	case "k", "up":
		e.vertical(-1)
	case "i":
		e.toInsert(e.cur)
	case "a":
		at := e.cur
		if e.cur < e.lineEnd(e.cur) {
			at++
		}
		e.toInsert(at)
	case "I":
		e.toInsert(e.firstNonBlank(e.cur))
	case "A":
		e.toInsert(e.lineEnd(e.cur))
	case "o":
		e.toInsert(e.lineEnd(e.cur))
		e.insert([]rune{'\n'})
	case "O":
		e.toInsert(e.lineStart(e.cur))
		e.insert([]rune{'\n'})
		e.cur--
	case "x", "delete":
		if e.cur < e.lineEnd(e.cur) {
			e.operate("d", e.cur, e.cur+1, false)
		}
	case "X":
		if e.cur > e.lineStart(e.cur) {
			e.operate("d", e.cur-1, e.cur, false)
		}
	case "s":
		e.operate("c", e.cur, min(e.cur+1, e.lineEnd(e.cur)), false)
	case "S":
		e.operate("c", e.lineStart(e.cur), e.lineEnd(e.cur), false)
	case "D":
		e.operate("d", e.cur, e.lineEnd(e.cur), false)
	case "C":
		e.operate("c", e.cur, e.lineEnd(e.cur), false)
	case "Y":
		from, to := e.lineRange(e.cur)
		e.yank(from, to, true)
	case "J":
		end := e.lineEnd(e.cur)
		if end < len(e.buf) {
			e.save()
			e.buf[end] = ' '
			e.cur = end
		}
	case "~":
		if e.cur < e.lineEnd(e.cur) {
			e.save()
			r := e.buf[e.cur]
			if unicode.IsUpper(r) {
				e.buf[e.cur] = unicode.ToLower(r)
			} else {
				e.buf[e.cur] = unicode.ToUpper(r)
			}
			e.cur++
			e.clampNormal()
		}
	case "p", "P":
		e.paste(key == "p")
	case "u":
		e.undoStep(&e.undo, &e.redo)
	case "ctrl+r":
		e.undoStep(&e.redo, &e.undo)
	case "v":
		e.mode, e.anchor = vimVisual, e.cur
	case "V":
		e.mode, e.anchor = vimVisualLine, e.cur
	case "d", "c", "y", "g", "r":
		e.pending = key
	}
	return editNone
}

func (e *editor) pendingKey(key string) editResult {
	p := e.pending
	e.pending = ""
	if key == "esc" || key == "ctrl+[" {
		return editNone
	}
	switch p {
	case "g":
		if key == "g" {
			e.cur = 0
		}
		return editNone
	case "r":
		if r := []rune(key); len(r) == 1 && e.cur < e.lineEnd(e.cur) {
			e.save()
			e.buf[e.cur] = r[0]
		}
		return editNone
	case "d", "c", "y":
		switch key {
		case p:
			if p == "c" {
				e.operate("c", e.firstNonBlank(e.cur), e.lineEnd(e.cur), false)
				return editNone
			}
			from, to := e.lineRange(e.cur)
			e.operate(p, from, to, true)
			return editNone
		case "i", "a":
			e.pending = p + key
			return editNone
		case "w", "W":
			if p == "c" {
				e.operate(p, e.cur, e.wordEnd(e.cur)+1, false)
				return editNone
			}
			to := e.wordForward(e.cur)
			if end := e.lineEnd(e.cur); to > end && end > e.cur {
				to = end
			}
			e.operate(p, e.cur, to, false)
			return editNone
		case "e", "E":
			e.operate(p, e.cur, e.wordEnd(e.cur)+1, false)
			return editNone
		}
		if to, ok := e.motion(key); ok {
			e.operate(p, e.cur, to, false)
		}
		return editNone
	case "di", "da", "ci", "ca", "yi", "ya":
		if key == "w" || key == "W" {
			from, to := e.innerWord(p[1] == 'a')
			e.operate(p[:1], from, to, false)
		}
		return editNone
	}
	return editNone
}

func (e *editor) selection() (int, int) {
	from, to := min(e.anchor, e.cur), max(e.anchor, e.cur)
	if e.mode == vimVisualLine {
		return e.lineStart(from), e.lineEnd(to)
	}
	return from, min(to+1, len(e.buf))
}

func (e *editor) visualKey(key string) editResult {
	if e.pending == "g" {
		e.pending = ""
		if key == "g" {
			e.cur = 0
		}
		return editNone
	}
	if to, ok := e.motion(key); ok {
		e.cur = to
		e.clampNormal()
		e.goalCol = e.col()
		return editNone
	}
	from, to := e.selection()
	line := e.mode == vimVisualLine
	switch key {
	case "esc", "ctrl+[", "ctrl+c":
		e.mode = vimNormal
	case "v":
		if e.mode == vimVisual {
			e.mode = vimNormal
		} else {
			e.mode = vimVisual
		}
	case "V":
		if e.mode == vimVisualLine {
			e.mode = vimNormal
		} else {
			e.mode = vimVisualLine
		}
	case "j", "down":
		e.vertical(1)
	case "k", "up":
		e.vertical(-1)
	case "g":
		e.pending = "g"
	case "o":
		e.anchor, e.cur = e.cur, e.anchor
	case "d", "x":
		e.mode = vimNormal
		if line {
			if to < len(e.buf) {
				to++
			} else if from > 0 {
				from--
			}
		}
		e.operate("d", from, to, line)
	case "y":
		e.mode = vimNormal
		e.yank(from, to, line)
		e.cur = from
		e.clampNormal()
	case "c", "s":
		e.mode = vimNormal
		e.operate("c", from, to, false)
	case "~", "u", "U":
		e.save()
		for i := from; i < to; i++ {
			r := e.buf[i]
			switch {
			case key == "u":
				e.buf[i] = unicode.ToLower(r)
			case key == "U":
				e.buf[i] = unicode.ToUpper(r)
			case unicode.IsUpper(r):
				e.buf[i] = unicode.ToLower(r)
			default:
				e.buf[i] = unicode.ToUpper(r)
			}
		}
		e.mode = vimNormal
		e.cur = from
	case "p", "P":
		e.mode = vimNormal
		reg, regLine := e.register, e.regLine
		e.operate("d", from, to, line)
		e.register, e.regLine = reg, regLine
		e.paste(false)
	}
	return editNone
}

func (e *editor) paste(after bool) {
	if len(e.register) == 0 {
		return
	}
	e.save()
	reg := e.register
	if e.regLine {
		text := strings.TrimSuffix(string(reg), "\n")
		if after {
			e.cur = e.lineEnd(e.cur)
			e.insert([]rune("\n" + text))
			e.cur = e.lineStart(e.cur)
		} else {
			e.cur = e.lineStart(e.cur)
			e.insert([]rune(text + "\n"))
			e.cur = e.lineStart(e.cur - 1)
		}
		e.clampNormal()
		return
	}
	if after && e.cur < e.lineEnd(e.cur) {
		e.cur++
	}
	e.insert(reg)
	e.cur--
	e.clampNormal()
}

func (e *editor) undoStep(from, to *[]snapshot) {
	if len(*from) == 0 {
		return
	}
	s := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	*to = append(*to, snapshot{buf: append([]rune(nil), e.buf...), cur: e.cur})
	e.buf, e.cur = s.buf, s.cur
	e.clampNormal()
}

func (e *editor) cursorCell(r rune) string {
	blank := r == ' ' && (e.cur >= len(e.buf) || e.buf[e.cur] == '\n')
	switch {
	case e.mode == vimInsert && blank:
		return lipgloss.NewStyle().Foreground(colGreen).Bold(true).Render("▏")
	case e.mode == vimInsert:
		return lipgloss.NewStyle().Foreground(colGreen).Underline(true).Bold(true).Render(string(r))
	case e.pending != "" && blank:
		return lipgloss.NewStyle().Foreground(colBlue).Render("▄")
	case e.pending != "":
		return lipgloss.NewStyle().Foreground(colBgDark).Background(colYellow).Render(string(r))
	case e.mode == vimVisual || e.mode == vimVisualLine:
		return lipgloss.NewStyle().Foreground(colBgDark).Background(colMagenta).Render(string(r))
	}
	return lipgloss.NewStyle().Foreground(colBgDark).Background(colBlue).Render(string(r))
}

func (e *editor) View() string {
	w, h := e.width-1, e.height
	if len(e.buf) == 0 && !e.focused {
		return " " + styleDim.Render(e.placeholder)
	}
	selFrom, selTo := -1, -1
	if e.mode == vimVisual || e.mode == vimVisualLine {
		selFrom, selTo = e.selection()
	}
	selStyle := lipgloss.NewStyle().Background(colHighlight).Foreground(colFg)

	type cell struct {
		r   rune
		pos int
	}
	var rows [][]cell
	var row []cell
	rowW := 0
	cursorRow := 0
	for i := 0; i <= len(e.buf); i++ {
		if i == e.cur {
			cursorRow = len(rows)
		}
		if i == len(e.buf) {
			row = append(row, cell{' ', i})
			break
		}
		r := e.buf[i]
		if r == '\n' {
			row = append(row, cell{' ', i})
			rows = append(rows, row)
			row, rowW = nil, 0
			continue
		}
		rw := max(1, lipgloss.Width(string(r)))
		if rowW+rw > w {
			rows = append(rows, row)
			row, rowW = nil, 0
			if i == e.cur {
				cursorRow = len(rows)
			}
		}
		row = append(row, cell{r, i})
		rowW += rw
	}
	rows = append(rows, row)

	if cursorRow < e.scroll {
		e.scroll = cursorRow
	}
	if cursorRow >= e.scroll+h {
		e.scroll = cursorRow - h + 1
	}
	var out []string
	for ri := e.scroll; ri < min(len(rows), e.scroll+h); ri++ {
		var b strings.Builder
		b.WriteString(" ")
		for _, c := range rows[ri] {
			s := string(c.r)
			switch {
			case e.focused && c.pos == e.cur:
				b.WriteString(e.cursorCell(c.r))
			case c.pos >= selFrom && c.pos < selTo:
				b.WriteString(selStyle.Render(s))
			default:
				b.WriteString(styleText.Render(s))
			}
		}
		out = append(out, b.String())
	}
	return strings.Join(out, "\n")
}
