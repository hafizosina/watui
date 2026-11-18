package notify

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltinSounds(t *testing.T) {
	dir := t.TempDir()
	data := map[string][]byte{}
	for name := range Builtin {
		path, err := BuiltinFile(dir, name)
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(b, []byte("RIFF")) || !bytes.Contains(b[:16], []byte("WAVE")) {
			t.Errorf("%s is not a WAV file", name)
		}
		if len(b) < 20000 {
			t.Errorf("%s is suspiciously short: %d bytes", name, len(b))
		}
		data[name] = b
	}
	for i, a := range BuiltinNames {
		for _, b := range BuiltinNames[i+1:] {
			if bytes.Equal(data[a], data[b]) {
				t.Errorf("%s and %s sound the same", a, b)
			}
		}
	}
	if len(data["ring"]) <= len(data["chime"]) {
		t.Error("the call ring should be longer than the message chime")
	}
	if _, err := BuiltinFile(dir, "nope"); err == nil {
		t.Error("unknown built-in name should fail")
	}
}

func TestSetSound(t *testing.T) {
	dir := t.TempDir()
	file := func(name string) string { return filepath.Join(dir, "watui-"+name+".wav") }
	custom := filepath.Join(dir, "my.ogg")

	for _, c := range []struct {
		name    string
		base    string
		perKind map[SoundKind]string
		want    map[SoundKind]string
	}{
		{"defaults", "watui", nil, map[SoundKind]string{
			SoundMessage: file("sword"), SoundMention: file("sword"), SoundGroup: file("chime"), SoundCall: file("ring")}},
		{"mention stands out, groups quiet", "watui", map[SoundKind]string{SoundMention: "ping", SoundGroup: "off"}, map[SoundKind]string{
			SoundMessage: file("sword"), SoundMention: file("ping"), SoundGroup: "off", SoundCall: file("ring")}},
		{"one file for all", custom, nil, map[SoundKind]string{
			SoundMessage: custom, SoundMention: custom, SoundGroup: custom, SoundCall: custom}},
		{"file for personal only", "watui", map[SoundKind]string{SoundMessage: custom}, map[SoundKind]string{
			SoundMessage: custom, SoundMention: file("sword"), SoundGroup: file("chime"), SoundCall: file("ring")}},
		{"built-in name as base; calls still ring", "soft", nil, map[SoundKind]string{
			SoundMessage: file("soft"), SoundMention: file("soft"), SoundGroup: file("soft"), SoundCall: file("ring")}},
		{"all off", "off", map[SoundKind]string{SoundCall: "chime"}, map[SoundKind]string{
			SoundMessage: "off", SoundMention: "off", SoundGroup: "off", SoundCall: "off"}},
		{"system, group override", "system", map[SoundKind]string{SoundGroup: "soft"}, map[SoundKind]string{
			SoundMessage: "system", SoundMention: "system", SoundGroup: file("soft"), SoundCall: "system"}},
	} {
		n := New(nil)
		n.SetSound(c.base, c.perKind, dir)
		for kind, want := range c.want {
			if got := n.sounds[kind]; got != want {
				t.Errorf("%s: kind %d = %q, want %q", c.name, kind, got, want)
			}
		}
	}
}
