package codexpack

import (
	"bytes"
	"context"
	"encoding/json"
	"goctx/internal/indexer"
	"goctx/internal/routes"
	"goctx/internal/taskpack"
	"os"
	"path/filepath"
	"strings"
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

func TestRoutesRankBeforeLimitAndIgnoreUnrelatedRoutes(t *testing.T) {
	r := Result{Task: "fix login", Routes: routes.Result{Routes: []routes.Route{
		{Method: "GET", Path: "/health", File: "health.go", Handler: "Health"},
		{Method: "POST", Path: "/login", File: "auth.go", Handler: "Login"},
	}}}
	opts := output.DefaultOptions("")
	opts.MaxRoutes = 1
	env := Envelope("codex-pack", r, opts)
	if len(env.Routes) != 1 || env.Routes[0].Path != "/login" {
		t.Fatalf("%#v", env.Routes)
	}
	var b bytes.Buffer
	WriteText(&b, r, opts)
	if !strings.Contains(b.String(), "/login") || strings.Contains(b.String(), "/health") {
		t.Fatal(b.String())
	}
}

func TestPackTextKeepsActionsBeforeLargeReasonsAndQuietRemovesAdvice(t *testing.T) {
	r := Result{Task: "fix login", TaskPack: taskpack.Result{
		Files:    []taskpack.RankedItem{{Name: "auth.go", Reasons: []string{strings.Repeat("huge ", 1000)}}},
		ReadPlan: []string{"goctx slice auth.go --symbol Login"}, Tests: []string{"go test ./auth"}, Avoid: []string{"vendor"},
	}}
	opts := output.DefaultOptions("")
	opts.Budget = 250
	var b bytes.Buffer
	WriteText(&b, r, opts)
	if output.EstimateText(b.String()) > 250 || !strings.Contains(b.String(), "go test ./auth") || !strings.Contains(b.String(), "--symbol Login") {
		t.Fatal(b.String())
	}
	opts.Quiet = true
	b.Reset()
	WriteText(&b, r, opts)
	if strings.Contains(b.String(), "reason:") || strings.Contains(b.String(), "Avoid unless") || strings.Contains(b.String(), "reduction") || !strings.Contains(b.String(), "auth.go") {
		t.Fatal(b.String())
	}
}

func TestFrameworkHintsSorted(t *testing.T) {
	got := detectImports([]indexer.PackageInfo{{Imports: []string{"foobar"}}}, map[string]string{"foo": "Z", "bar": "A"})
	if strings.Join(got, ",") != "A,Z" {
		t.Fatal(got)
	}
}
