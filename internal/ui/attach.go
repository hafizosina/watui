package ui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zhenzhu/watui/internal/wa"
)

// Attachments: p (messages pane) or ctrl+v (Compose) pastes a copied file or
// picture, u / :upload picks files. They wait in Compose (the text becomes the
// caption) until ctrl+s sends them; esc in the messages pane drops them.

type attachment struct {
	path string
	temp bool // a pasted picture we wrote to disk; removed after sending
}

// clipboardMsg is what pasting found: files to attach, or plain text.
type clipboardMsg struct {
	files []attachment
	text  string
	err   error
	vim   string // "p" / "P" when it came from vim normal mode in Compose
}

func pasteCmd() tea.Cmd {
	return func() tea.Msg { return readClipboard() }
}

// clipboardWriter puts Compose's yanks on the system clipboard; tests swap it out.
var clipboardWriter = func(text string) {
	if _, err := exec.LookPath("xclip"); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "xclip", "-selection", "clipboard", "-i")
	cmd.Stdin = strings.NewReader(text)
	_ = cmd.Run() // xclip keeps serving it in the background
}

// vimPasteCmd is p / P in Compose's vim normal mode. It pastes whatever was
// copied last, here or in another app: a file or picture is attached, text is
// pasted (line-wise when it is our own line yank).
func vimPasteCmd(key string) tea.Cmd {
	return func() tea.Msg {
		msg := readClipboard()
		msg.vim = key
		return msg
	}
}

func xclipOut(target string) ([]byte, error) {
	args := []string{"-selection", "clipboard", "-o"}
	if target != "" {
		args = append(args, "-t", target)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "xclip", args...).Output()
}

func readClipboard() clipboardMsg {
	if _, err := exec.LookPath("xclip"); err != nil {
		return clipboardMsg{err: errors.New("paste needs xclip (sudo apt install xclip)")}
	}
	raw, err := xclipOut("TARGETS")
	if err != nil {
		return clipboardMsg{err: errors.New("clipboard is empty")}
	}
	targets := map[string]bool{}
	for _, t := range strings.Fields(string(raw)) {
		targets[t] = true
	}
	// files copied in Nemo / Nautilus / most file managers
	for _, t := range []string{"x-special/gnome-copied-files", "text/uri-list"} {
		if !targets[t] {
			continue
		}
		if out, err := xclipOut(t); err == nil {
			if files := filesFromURIs(string(out)); len(files) > 0 {
				return clipboardMsg{files: files}
			}
		}
	}
	// a copied picture (screenshot tool, browser "Copy image", …)
	for _, t := range []string{"image/png", "image/jpeg"} {
		if !targets[t] {
			continue
		}
		data, err := xclipOut(t)
		if err != nil || len(data) == 0 {
			continue
		}
		path, err := writePasted(data, "."+strings.TrimPrefix(t, "image/"))
		if err != nil {
			return clipboardMsg{err: err}
		}
		return clipboardMsg{files: []attachment{{path: path, temp: true}}}
	}
	out, err := xclipOut("")
	if err != nil || len(out) == 0 {
		return clipboardMsg{err: errors.New("nothing to paste")}
	}
	text := string(out)
	// a copied path (or several, one per line) is a file too
	if files := filesFromURIs(text); len(files) > 0 {
		return clipboardMsg{files: files}
	}
	return clipboardMsg{text: text}
}

// filesFromURIs reads file:// URIs or absolute paths, one per line; any line
// that isn't an existing file makes the whole thing plain text.
func filesFromURIs(s string) []attachment {
	var out []attachment
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "copy" || line == "cut" {
			continue
		}
		path := line
		if strings.HasPrefix(line, "file://") {
			u, err := url.Parse(line)
			if err != nil {
				return nil
			}
			path = u.Path
		} else if strings.HasPrefix(line, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				path = filepath.Join(home, line[2:])
			}
		}
		if !filepath.IsAbs(path) {
			return nil
		}
		if st, err := os.Stat(path); err != nil || st.IsDir() {
			return nil
		}
		out = append(out, attachment{path: path})
	}
	return out
}

func writePasted(data []byte, ext string) (string, error) {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".cache")
	}
	dir := filepath.Join(base, "watui", "pasted")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "pasted-"+time.Now().Format("20060102-150405")+ext)
	return path, os.WriteFile(path, data, 0o600)
}

// filesPickedMsg comes back from the file picker.
type filesPickedMsg struct {
	files []attachment
	err   error
}

func pickFilesCmd() tea.Cmd {
	return func() tea.Msg {
		home, _ := os.UserHomeDir()
		cmd := exec.Command("zenity", "--file-selection", "--multiple", "--separator=\n",
			"--title=Send files with watui", "--filename="+home+"/")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) && exit.ExitCode() == 1 {
				return filesPickedMsg{} // cancelled
			}
			return filesPickedMsg{err: fmt.Errorf("file picker: %v %s", err, strings.TrimSpace(stderr.String()))}
		}
		var files []attachment
		for _, p := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if p != "" {
				files = append(files, attachment{path: p})
			}
		}
		return filesPickedMsg{files: files}
	}
}

// openFilePicker opens the desktop file dialog, or the :upload prompt when
// there's no dialog to show.
func (m *Model) openFilePicker() tea.Cmd {
	if m.openJID == "" {
		m.setFlash("open a chat first", true)
		return nil
	}
	if _, err := exec.LookPath("zenity"); err == nil && os.Getenv("DISPLAY")+os.Getenv("WAYLAND_DISPLAY") != "" {
		m.setFlash("choose files in the dialog…", false)
		return pickFilesCmd()
	}
	m.mode = modeCommand
	m.prompt.SetValue("upload ")
	m.prompt.CursorEnd()
	m.cmdTyped, m.cmdSel = m.prompt.Value(), 0
	m.prompt.Focus()
	return nil
}

// addAttachments queues files and opens Compose so a caption can be typed.
func (m *Model) addAttachments(files []attachment) {
	if m.openJID == "" {
		m.setFlash("open a chat first", true)
		return
	}
	var bad []string
	for _, f := range files {
		if _, _, err := wa.SendKind(f.path, false); err != nil {
			bad = append(bad, err.Error())
			continue
		}
		m.attach = append(m.attach, f)
	}
	if len(bad) > 0 {
		m.setFlash(strings.Join(bad, "; "), true)
	}
	if len(m.attach) == 0 {
		return
	}
	m.editing, m.replyTo = nil, nil // files go out as new messages, not as a reply
	if m.mode != modeInsert {
		m.enterInsert()
	}
}

func (m *Model) clearAttachments() {
	for _, a := range m.attach {
		if a.temp {
			_ = os.Remove(a.path)
		}
	}
	m.attach = nil
}

// uploadArgs turns ":upload ~/a b.pdf ~/c.png" into paths; a whole argument
// that names a file wins, so paths with spaces work without quotes.
func uploadArgs(arg string) ([]attachment, error) {
	arg = strings.TrimSpace(arg)
	expand := func(p string) string {
		p = strings.Trim(p, `"'`)
		if strings.HasPrefix(p, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				return filepath.Join(home, p[2:])
			}
		}
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
		return p
	}
	if st, err := os.Stat(expand(arg)); err == nil && !st.IsDir() {
		return []attachment{{path: expand(arg)}}, nil
	}
	var out []attachment
	for _, p := range strings.Fields(arg) {
		path := expand(p)
		if st, err := os.Stat(path); err != nil || st.IsDir() {
			return nil, fmt.Errorf("no such file: %s", p)
		}
		out = append(out, attachment{path: path})
	}
	return out, nil
}

func (m Model) attachTitle() string {
	names := make([]string, len(m.attach))
	for i, a := range m.attach {
		names[i] = filepath.Base(a.path)
	}
	what := "1 file"
	if len(m.attach) > 1 {
		what = fmt.Sprintf("%d files", len(m.attach))
	}
	return "[4] 📎 " + what + ": " + sanitize(strings.Join(names, ", ")) + " · type a caption, ctrl+s sends · esc (normal mode) drops"
}

// sendFilesCmd uploads and sends the queued files in order; the caption goes
// with the first one.
func (m *Model) sendFilesCmd(caption string) tea.Cmd {
	client, jid, files := m.wa, m.openJID, m.attach
	m.attach, m.replyTo, m.editing = nil, nil, nil
	what := filepath.Base(files[0].path)
	if len(files) > 1 {
		what = fmt.Sprintf("%d files", len(files))
	}
	m.setFlash("sending "+what+"…", false)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		var failed []string
		for i, f := range files {
			c := ""
			if i == 0 {
				c = caption
			}
			if err := client.SendFile(ctx, jid, f.path, c, false); err != nil {
				failed = append(failed, err.Error())
			}
			if f.temp {
				_ = os.Remove(f.path)
			}
		}
		if len(failed) > 0 {
			return actionDoneMsg{err: fmt.Errorf("%d of %d not sent: %s", len(failed), len(files), failed[0])}
		}
		return actionDoneMsg{info: "sent " + what}
	}
}

func (m Model) gotClipboard(msg clipboardMsg) (tea.Model, tea.Cmd) {
	if msg.vim != "" && len(msg.files) == 0 && m.mode == modeInsert && m.input.mode == vimNormal {
		after := msg.vim == "p"
		own := len(m.input.register) > 0 && msg.text == string(m.input.register)
		switch {
		case own, msg.text == "" && len(m.input.register) > 0:
			m.input.paste(after) // our own yank (keeps line-wise), or the clipboard can't be read
		case msg.text != "":
			reg, regLine := m.input.register, m.input.regLine
			m.input.register, m.input.regLine = []rune(strings.ReplaceAll(msg.text, "\r\n", "\n")), false
			m.input.paste(after)
			m.input.register, m.input.regLine = reg, regLine
		case msg.err != nil:
			m.setFlash(msg.err.Error(), true)
		}
		return m, nil
	}
	switch {
	case msg.err != nil:
		m.setFlash(msg.err.Error(), true)
	case len(msg.files) > 0:
		m.addAttachments(msg.files)
	case msg.text != "":
		if m.openJID == "" {
			return m, nil
		}
		if m.mode != modeInsert {
			m.enterInsert()
		}
		m.input.save()
		m.input.insert([]rune(strings.ReplaceAll(msg.text, "\r\n", "\n")))
	}
	return m, nil
}
