package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitGeneratesFilesAndSkips(t *testing.T) {
	root := t.TempDir()
	res, err := Init(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 5 {
		t.Fatalf("expected 5 results, got %d", len(res))
	}
	skill, err := os.ReadFile(filepath.Join(root, ".agents", "skills", "go-token-efficient", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(skill), "name: go-token-efficient") {
		t.Fatal("missing skill frontmatter")
	}
	res, err = Init(root, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if !r.Skipped {
			t.Fatalf("expected skipped file, got %#v", r)
		}
	}
}

func TestInitForceOverwrites(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("custom"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(root, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "custom" {
		t.Fatal("expected force overwrite")
	}
}
