package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeSetupCreatesSharedImportAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	res, err := InitClaude(root, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatal(res)
	}
	agents, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if !strings.Contains(string(agents), "Do not use git commands.") {
		t.Fatal("lost shared rule")
	}
	bridge, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if string(bridge) != ClaudeTemplate {
		t.Fatalf("%q", bridge)
	}
	res, err = InitClaude(root, false, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if !r.Skipped {
			t.Fatal(res)
		}
	}
}

func TestClaudeSetupPreservesExistingInstructions(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{"AGENTS.md": "custom shared rules", "CLAUDE.md": "custom Claude rules"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	res, err := InitClaude(root, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if !res[1].Updated {
		t.Fatal(res)
	}
	shared, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	bridge, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if string(shared) != "custom shared rules" || !strings.HasPrefix(string(bridge), "custom Claude rules\n") || !strings.Contains(string(bridge), "\n@AGENTS.md\n") {
		t.Fatalf("%q %q", shared, bridge)
	}
	// Force only resets the Claude bridge, never the shared policy.
	if _, err = InitClaude(root, true, false); err != nil {
		t.Fatal(err)
	}
	shared, _ = os.ReadFile(filepath.Join(root, "AGENTS.md"))
	bridge, _ = os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if string(shared) != "custom shared rules" || string(bridge) != ClaudeTemplate {
		t.Fatal("force replaced shared policy or failed to reset bridge")
	}
}

func TestClaudeSetupUsesExistingDotClaudeEntry(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".claude", "CLAUDE.md")
	if err := os.WriteFile(path, []byte("existing\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := InitClaude(root, false, false); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "@../AGENTS.md") {
		t.Fatal(string(data))
	}
	if _, err := os.Stat(filepath.Join(root, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatal("shadowed existing entry")
	}
}

func TestClaudeSetupIgnoresImportExamples(t *testing.T) {
	root := t.TempDir()
	body := "Example:\n```text\n@AGENTS.md\n```\n"
	path := filepath.Join(root, "CLAUDE.md")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := InitClaude(root, false, false); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != body+"\n@AGENTS.md\n" {
		t.Fatalf("%q", data)
	}
}

func TestClaudeSetupDoesNotFollowInstructionSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "instructions.md")
	if err := os.WriteFile(target, []byte("external"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "CLAUDE.md")); err != nil {
		t.Skip(err)
	}
	if _, err := InitClaude(root, true, false); err == nil {
		t.Fatal("expected refusal")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "external" {
		t.Fatal("changed symlink target")
	}
}
