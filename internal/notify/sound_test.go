package notify

import (
	"bytes"
	"os"
	"testing"
)

func TestSoundFiles(t *testing.T) {
	msg, call, err := SoundFiles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"message": msg, "call": call} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(data, []byte("RIFF")) || !bytes.Contains(data[:16], []byte("WAVE")) {
			t.Errorf("%s sound is not a WAV file", name)
		}
		if len(data) < 20000 {
			t.Errorf("%s sound is suspiciously short: %d bytes", name, len(data))
		}
	}
	m, _ := os.Stat(msg)
	c, _ := os.Stat(call)
	if c.Size() <= m.Size() {
		t.Error("the call ring should be longer than the message chime")
	}
}
