package notify

import (
	"bufio"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

type Note struct {
	Key   string
	Title string
	Body  string
}

type Notifier struct {
	onOpen func(key string)

	mu  sync.Mutex
	ids map[string]uint32
}

func Available() bool {
	_, err := exec.LookPath("notify-send")
	return err == nil
}

func New(onOpen func(key string)) *Notifier {
	return &Notifier{onOpen: onOpen, ids: map[string]uint32{}}
}

// Send shows (or updates, per Key) a desktop notification; clicking it calls onOpen(Key).
func (n *Notifier) Send(note Note) {
	if !Available() {
		return
	}
	n.mu.Lock()
	id := n.ids[note.Key]
	n.mu.Unlock()

	args := []string{"--app-name=watui", "--icon=mail-message-new", "--print-id",
		"--hint=string:sound-name:message-new-instant", "--action=default=Open"}
	if id != 0 {
		args = append(args, "--replace-id="+strconv.FormatUint(uint64(id), 10))
	}
	args = append(args, note.Title, note.Body)

	go func() {
		cmd := exec.Command("notify-send", args...)
		if path, err := exec.LookPath("stdbuf"); err == nil {
			cmd = exec.Command(path, append([]string{"-oL", "notify-send"}, args...)...)
		}
		out, err := cmd.StdoutPipe()
		if err != nil || cmd.Start() != nil {
			return
		}
		sc := bufio.NewScanner(out)
		first := true
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if first {
				first = false
				if v, err := strconv.ParseUint(line, 10, 32); err == nil {
					n.mu.Lock()
					n.ids[note.Key] = uint32(v)
					n.mu.Unlock()
					continue
				}
			}
			if line == "default" && n.onOpen != nil {
				n.onOpen(note.Key)
			}
		}
		_ = cmd.Wait()
	}()
}

// Forget drops the replace id so the next note for this key pops up fresh.
func (n *Notifier) Forget(key string) {
	n.mu.Lock()
	delete(n.ids, key)
	n.mu.Unlock()
}

// FocusTerminal brings watui's tmux pane and terminal window to the front.
func FocusTerminal() {
	if pane := os.Getenv("TMUX_PANE"); pane != "" {
		_ = exec.Command("tmux", "select-window", "-t", pane).Run()
		_ = exec.Command("tmux", "select-pane", "-t", pane).Run()
	}
	if _, err := exec.LookPath("wmctrl"); err != nil {
		return
	}
	if win := os.Getenv("WINDOWID"); win != "" {
		if exec.Command("wmctrl", "-i", "-a", win).Run() == nil {
			return
		}
	}
	_ = exec.Command("wmctrl", "-x", "-a", "kitty").Run()
}
