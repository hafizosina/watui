package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSoundSettings(t *testing.T) {
	c := Default()
	for _, ok := range []string{"notify_sound=sword", "notify_sound=watui", "notify_sound_personal=sword",
		"notify_sound_mention=ping", "notify_sound_group=off", "notify_sound_group=", "notify_sound_personal=system"} {
		if err := c.Set(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	if c.NotifySoundPersonal != "system" || c.NotifySoundMention != "ping" || c.NotifySoundGroup != "" {
		t.Errorf("got %+v", c)
	}
	file := filepath.Join(t.TempDir(), "ding.ogg")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := c.Set("notify_sound_mention=" + file); err != nil || c.NotifySoundMention != file {
		t.Errorf("a sound file should be accepted: %v", err)
	}
	for _, bad := range []string{"notify_sound=", "notify_sound=trumpet", "notify_sound_group=/no/such.ogg"} {
		if err := c.Set(bad); err == nil {
			t.Errorf("%s should be rejected", bad)
		}
	}
}
