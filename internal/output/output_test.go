package output

import "testing"

func TestApplyBudgetKeepsRelevantItemsAndReportsOmissions(t *testing.T) {
	env := Envelope{
		Files: []FileItem{
			{Path: "internal/low.go", Score: 1},
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
