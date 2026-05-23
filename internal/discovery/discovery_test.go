package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverSingleModule(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	write(t, filepath.Join(root, "main.go"), "package main\nfunc main(){}\n")
	res, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Modules) != 1 || res.Selected != "." {
		t.Fatalf("unexpected discovery: %#v", res)
	}
	if len(res.Modules[0].MainPackages) != 1 {
		t.Fatalf("expected main package: %#v", res.Modules[0])
	}
}

func TestDiscoverNestedModuleFromAbove(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "backend", "go.mod"), "module example.com/backend\n")
	write(t, filepath.Join(root, "backend", "cmd", "api", "main.go"), "package main\nfunc main(){}\n")
	res, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Modules) != 1 || res.Modules[0].Path != "backend" {
		t.Fatalf("unexpected modules: %#v", res.Modules)
	}
}

func TestDiscoverGoWork(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "go.work"), "go 1.22\nuse ./backend\n")
	write(t, filepath.Join(root, "backend", "go.mod"), "module example.com/backend\n")
	res, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if res.GoWork == "" || len(res.Modules) != 1 {
		t.Fatalf("expected go.work and module: %#v", res)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}
