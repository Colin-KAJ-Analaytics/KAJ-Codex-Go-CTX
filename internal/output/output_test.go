package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestApplyBudgetKeepsRelevantItemsAndReportsOmissions(t *testing.T) {
	env := Envelope{
		Files: []FileItem{
			{Path: "internal/low.go", Score: 1, Reasons: []string{strings.Repeat("irrelevant ", 1000)}},
			{Path: "cmd/main.go", Score: 100},
			{Path: "internal/auth.go", Score: 80},
		},
		Symbols: []SymbolItem{
			{Name: "low", Score: 1},
			{Name: "VerifyToken", Score: 90},
		},
		ReadPlan: []string{"read main", "read low"},
		Tests:    []string{"go test ./internal/auth"},
	}

	ApplyBudget(&env, Options{Budget: 300, MaxFiles: 20, MaxSymbols: 20, MaxRoutes: 20})

	if env.ActualEstimatedTokens > 300 {
		t.Fatalf("estimated output %d exceeds budget", env.ActualEstimatedTokens)
	}
	if len(env.Omitted) == 0 {
		t.Fatal("expected explicit omissions")
	}
	if len(env.Files) == 0 || env.Files[0].Path != "cmd/main.go" {
		t.Fatalf("relevance ranking not preserved: %#v", env.Files)
	}
}

func TestApplyBudgetRetainsTaskSymbolsAndTestsBeforeReadPlan(t *testing.T) {
	env := Envelope{
		Files: []FileItem{
			{Path: "internal/routes/routes.go", Score: 114},
			{Path: "internal/cli/cli.go", Score: 72},
			{Path: "internal/output/output.go", Score: 72},
			{Path: "internal/taskpack/rank.go", Score: 63},
		},
		Symbols: []SymbolItem{
			{Name: "Extract", Score: 9},
			{Name: "ExtractFile", Score: 9},
			{Name: "TestRouteExtractionFrameworks", Score: 12},
		},
		ReadPlan: []string{"goctx symbols internal/routes/routes.go", "goctx symbols internal/cli/cli.go", "goctx slice internal/routes/routes.go --symbol Result"},
		Tests:    []string{"go test ./internal/routes"},
	}

	ApplyBudget(&env, Options{Budget: 1200, MaxFiles: 20, MaxSymbols: 20, MaxRoutes: 20})

	if !hasSymbol(env.Symbols, "Extract") || !hasSymbol(env.Symbols, "ExtractFile") {
		t.Fatalf("expected implementation symbols to survive budget, got %#v", env.Symbols)
	}
	if !hasString(env.Tests, "go test ./internal/routes") {
		t.Fatalf("expected focused test to survive budget, got %#v", env.Tests)
	}
	if env.ActualEstimatedTokens > 1200 {
		t.Fatalf("estimated output %d exceeds budget", env.ActualEstimatedTokens)
	}
}

func hasSymbol(items []SymbolItem, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}

func hasString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func TestJSONBudgetUsesRenderedBytes(t *testing.T) {
	for _, compact := range []bool{false, true} {
		env := Envelope{Command: "task", Warnings: []string{"keep this warning"}, Omitted: []Omission{{Kind: "upstream", Count: 2, Reason: "limit"}}, Files: []FileItem{
			{Path: "huge.go", Score: 200, Reasons: []string{strings.Repeat("long reason ", 2000)}},
			{Path: "useful.go", Score: 100},
		}, Tests: []string{"go test ./focused"}}
		opts := Options{Budget: 500, CompactJSON: compact}
		ApplyBudget(&env, opts)
		var b bytes.Buffer
		if err := WriteJSON(&b, env); err != nil {
			t.Fatal(err)
		}
		want := EstimateText(b.String())
		if env.ActualEstimatedTokens != want || want > opts.Budget {
			t.Fatalf("reported=%d emitted=%d", env.ActualEstimatedTokens, want)
		}
		if len(env.Files) != 1 || env.Files[0].Path != "useful.go" || len(env.Omitted) < 2 || env.Omitted[0].Kind != "upstream" {
			t.Fatalf("lost relevant context or omissions: %#v", env)
		}
	}
}

func TestTinyBudgetReportsMandatoryOverflow(t *testing.T) {
	env := Envelope{Command: "task", Warnings: []string{"preserve me"}}
	ApplyBudget(&env, Options{Budget: 1})
	if env.ActualEstimatedTokens <= 1 || !strings.Contains(strings.Join(env.Warnings, " "), "exceeds") {
		t.Fatalf("%#v", env)
	}
}

func TestQuietCompactJSONRetainsWarnings(t *testing.T) {
	env := Envelope{Command: "task", Warnings: []string{"warning"}, Files: []FileItem{{Path: "f.go", Reasons: []string{"explanation"}}}, Data: map[string]any{"task": "fix", "avoid": []string{"vendor"}, "savings": 10}}
	ApplyBudget(&env, Options{Quiet: true, CompactJSON: true, Budget: 1000})
	var b bytes.Buffer
	if err := WriteJSON(&b, env); err != nil {
		t.Fatal(err)
	}
	if strings.Count(b.String(), "\n") != 1 || strings.Contains(b.String(), "explanation") || strings.Contains(b.String(), "savings") || !strings.Contains(b.String(), "warning") {
		t.Fatal(b.String())
	}
}

func TestTextBudgetRetainsActionsAndWholeItems(t *testing.T) {
	var b bytes.Buffer
	WriteTextItems(&b, "Task Context Pack\n", []TextItem{
		{Section: "Advice", Text: strings.Repeat("boilerplate ", 500), Priority: 1},
		{Section: "Read plan", Text: "- goctx slice auth.go --symbol Verify", Priority: 100},
		{Section: "Tests", Text: "- go test ./auth", Priority: 110},
	}, 150)
	got := b.String()
	if EstimateText(got) > 150 || strings.Contains(got, "boilerplate") || !strings.Contains(got, "go test ./auth") || !strings.Contains(got, "--symbol Verify") || !strings.Contains(got, "Omitted:") {
		t.Fatal(got)
	}
}
