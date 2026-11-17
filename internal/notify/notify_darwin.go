package notify

import "os/exec"

func Available() bool {
	_, err := exec.LookPath("osascript")
	return err == nil
}

// show pops a macOS notification. osascript can't replace an earlier one or
// report a click, so there is no "open the chat" or Decline button here.
func (n *Notifier) show(note Note, _ uint32, systemSound bool) {
	script := []string{"-e", "on run argv"}
	if systemSound {
		script = append(script, "-e", `display notification (item 2 of argv) with title (item 1 of argv) sound name "Glass"`)
	} else {
		script = append(script, "-e", "display notification (item 2 of argv) with title (item 1 of argv)")
	}
	script = append(script, "-e", "end run", "--", note.Title, note.Body)
	go func() {
		cmd := exec.Command("osascript", script...)
		if cmd.Start() == nil {
			_ = cmd.Wait()
		}
	}()
}
