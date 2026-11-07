package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"goctx/internal/output"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMapJSONHonorsBudgetAndReportsOmissions(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/audit\n\ngo 1.23\n"), 0644); err != nil {
		t.Fatal(err)
	}
	src := "package main\nfunc main() {}\n"
	for i := 0; i < 40; i++ {
		src += fmt.Sprintf("func Function%d() {}\n", i)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"map", "--root", root, "--format", "json", "--budget", "500"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run returned %d: %s", code, stderr.String())
	}
	var env struct {
		ActualEstimatedTokens int `json:"actual_estimated_tokens"`
		Omitted               []struct {
			Kind  string `json:"kind"`
			Count int    `json:"count"`
		} `json:"omitted"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if env.ActualEstimatedTokens > 500 {
		t.Fatalf("estimated output %d exceeds budget", env.ActualEstimatedTokens)
	}
	if int64(env.ActualEstimatedTokens) != output.EstimateText(stdout.String()) {
		t.Fatal("estimate does not match serialized bytes")
	}
	if len(env.Omitted) == 0 {
		t.Fatal("expected omission counts in budgeted map output")
	}
}

func TestTaskReturnsPackageSelectionErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"task", "--root", filepath.Join(t.TempDir(), "missing"), "audit"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected task failure for missing repository")
	}
	if stderr.Len() == 0 {
		t.Fatal("expected actionable error output")
	}
}

func TestInitClaudeUsesRootAndPreservesRules(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init-claude", "--root", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("%d %s", code, &stderr)
	}
	data, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil || string(data) != "@AGENTS.md\n" {
		t.Fatalf("%q %v", data, err)
	}
}

func TestCommonCompactFlagsReachJSONWriter(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/compact\ngo 1.23\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "auth.go"), []byte("package compact\nfunc VerifyToken(){}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"task", "fix auth", "--root", root, "--quiet", "--format", "json", "--compact-json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("%d %s", code, &stderr)
	}
	if strings.Count(stdout.String(), "\n") != 1 || strings.Contains(stdout.String(), "reasons") || strings.Contains(stdout.String(), "savings") {
		t.Fatal(stdout.String())
	}
	var env output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.ActualEstimatedTokens != output.EstimateText(stdout.String()) {
		t.Fatal("incorrect size")
	}
}

func TestDoctorReportsMissingExecutable(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/doctor\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"doctor", "--root", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("%d %s", code, &stderr)
	}
	if !strings.Contains(stdout.String(), "goctx missing from PATH") || !strings.Contains(stdout.String(), "go install ./cmd/goctx") {
		t.Fatal(stdout.String())
	}
}
