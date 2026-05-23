package codexpack

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"goctx/internal/output"
)

func TestCodexPackIncludesReadPlanAndSavings(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	write(t, filepath.Join(root, "auth.go"), "package app\nfunc VerifyToken() {}\n")
	write(t, filepath.Join(root, "auth_test.go"), "package app\nfunc TestVerifyToken() {}\n")
	opts := output.DefaultOptions(root)
	res, err := Build(context.Background(), "fix auth token", opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.TaskPack.ReadPlan) == 0 || res.TaskPack.Savings.RecommendedEstimatedTokens == 0 {
		t.Fatalf("expected read plan and savings: %#v", res.TaskPack)
	}
	var b bytes.Buffer
	if err := output.WriteJSON(&b, Envelope("codex-pack", res, opts)); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(b.Bytes(), &decoded); err != nil {
		t.Fatal(err)
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
