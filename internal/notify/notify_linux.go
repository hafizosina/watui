package notify

import (
	"bufio"
	"os/exec"
	"strconv"
	"strings"
)

func Available() bool {
	_, err := exec.LookPath("notify-send")
	return err == nil
}

// show pops the notification with notify-send: it replaces the chat's previous
// one, and a click (or an action button) calls onOpen.
func (n *Notifier) show(note Note, id uint32, systemSound bool) {
	args := []string{"--app-name=watui", "--icon=mail-message-new", "--print-id", "--action=default=Open"}
	if systemSound {
		args = append(args, "--hint=string:sound-name:message-new-instant")
	} else {
		args = append(args, "--hint=boolean:suppress-sound:true")
	}
	for _, a := range note.Actions {
		args = append(args, "--action="+a.Name+"="+a.Label)
	}
	if id != 0 {
		args = append(args, "--replace-id="+strconv.FormatUint(uint64(id), 10))
	}
	// "--": a title or body starting with "-" must not be read as an option
	args = append(args, "--", note.Title, note.Body)

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
			if line != "" && n.onOpen != nil {
				n.onOpen(note.Key, line)
			}
		}
		_ = cmd.Wait()
	}()
}
