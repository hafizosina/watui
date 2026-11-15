package notify

import (
	"os"
	"testing"
	"time"
)

func TestSendReturnsIDForReplace(t *testing.T) {
	if os.Getenv("WATUI_NOTIFY_TEST") == "" || !Available() {
		t.Skip("set WATUI_NOTIFY_TEST=1 to show a real notification")
	}
	n := New(nil)
	n.Send(Note{Key: "k", Title: "watui", Body: "test — first"})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		n.mu.Lock()
		id := n.ids["k"]
		n.mu.Unlock()
		if id != 0 {
			n.Send(Note{Key: "k", Title: "watui", Body: "test — replaced the first one"})
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("notification id never arrived, replacing would not work")
}
