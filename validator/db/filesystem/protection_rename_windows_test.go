package filesystem

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProtectionWindowsPath(t *testing.T) {
	suffix := strings.Repeat(`long\`, 60) + "record.yaml"
	for _, test := range []struct{ path, want string }{
		{`C:\short\record.yaml`, `C:\short\record.yaml`},
		{`C:\` + suffix, `\\?\C:\` + suffix},
		{`\\server\share\` + suffix, `\\?\UNC\server\share\` + suffix},
		{`\\?\C:\` + suffix, `\\?\C:\` + suffix},
	} {
		got, err := protectionWindowsPath(test.path)
		if err != nil || got != test.want {
			t.Fatalf("path %q: got %q, %v; want %q", test.path, got, err, test.want)
		}
	}
}

func TestRenameProtectionFileWindowsErrors(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(source, []byte("history"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, paths := range [][2]string{{"\x00", source}, {source, "\x00"}, {source, filepath.Join(source, "absent")}} {
		if err := renameProtectionFile(paths[0], paths[1]); err == nil {
			t.Fatal("expected replacement failure")
		}
	}
	got, err := os.ReadFile(source)
	if err != nil || string(got) != "history" {
		t.Fatalf("source changed: %q, %v", got, err)
	}
}

func TestRenameProtectionFileWindowsLongPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), strings.Repeat(`long-directory\`, 20))
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	source, target := filepath.Join(dir, "source"), filepath.Join(dir, "target")
	for _, data := range []string{"first", "replacement"} {
		if err := os.WriteFile(source, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if err := renameProtectionFile(source, target); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(target)
		if err != nil || string(got) != data {
			t.Fatalf("target: %q, %v", got, err)
		}
	}
}
