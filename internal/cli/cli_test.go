package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMapJSONHonorsBudgetAndReportsOmissions(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/audit\n\ngo 1.23\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"map", "--root", root, "--format", "json", "--budget", "190"}, &stdout, &stderr)
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
	if env.ActualEstimatedTokens > 190 {
		t.Fatalf("estimated output %d exceeds budget", env.ActualEstimatedTokens)
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
