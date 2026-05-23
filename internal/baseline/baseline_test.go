package baseline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateWriteLoadBaseline(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "x.go"), []byte("package x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	s, err := Create(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Files) == 0 {
		t.Fatal("expected baseline files")
	}
	if err := Write(root, s); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Files) != len(s.Files) {
		t.Fatal("loaded baseline mismatch")
	}
}
