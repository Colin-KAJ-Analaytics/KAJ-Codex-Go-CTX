package taskpack

import (
	"testing"

	"goctx/internal/indexer"
	"goctx/internal/scanner"
)

func TestAuthTaskRanksAuthFiles(t *testing.T) {
	pkgs := []indexer.PackageInfo{{ImportPath: "goctx/internal/auth", Name: "auth", Files: []string{"internal/auth/firebase.go", "internal/auth/firebase_test.go"}, TestFiles: []string{"internal/auth/firebase_test.go"}, Symbols: []indexer.SymbolInfo{{Name: "VerifyToken", Receiver: "(*FirebaseVerifier)", File: "internal/auth/firebase.go"}}}}
	res := Rank("fix Firebase JWT auth middleware", pkgs, scanner.Result{})
	if len(res.Files) == 0 || res.Files[0].Name != "internal/auth/firebase.go" {
		t.Fatalf("expected auth file first, got %#v", res.Files)
	}
	if len(res.Tests) == 0 || res.Tests[0] != "go test ./internal/auth" {
		t.Fatalf("expected focused auth test, got %#v", res.Tests)
	}
}

func TestGeneratedFilesExcluded(t *testing.T) {
	pkgs := []indexer.PackageInfo{{ImportPath: "p", Files: []string{"internal/api/user.pb.go"}, Symbols: []indexer.SymbolInfo{{Name: "AuthToken", File: "internal/api/user.pb.go"}}}}
	sc := scanner.Result{Files: []scanner.FileInfo{{Path: "internal/api/user.pb.go", IgnoredByDefault: true}}}
	res := Rank("auth token", pkgs, sc)
	for _, f := range res.Files {
		if f.Name == "internal/api/user.pb.go" {
			t.Fatal("generated file should be excluded")
		}
	}
}

func TestPackageLoaderTaskKeepsImplementationAndPackageTests(t *testing.T) {
	pkgs := []indexer.PackageInfo{{
		ImportPath: "goctx/internal/indexer",
		Name:       "indexer",
		Files:      []string{"internal/indexer/packages.go", "internal/indexer/symbols.go", "internal/indexer/symbols_test.go"},
		TestFiles:  []string{"internal/indexer/symbols_test.go"},
		Symbols: []indexer.SymbolInfo{
			{Name: "LoadPackagesFlexible", File: "internal/indexer/packages.go", Kind: indexer.SymbolFunc},
			{Name: "loadASTOnly", File: "internal/indexer/packages.go", Kind: indexer.SymbolFunc},
			{Name: "TestLoadPackagesFlexibleASTFallback", File: "internal/indexer/symbols_test.go", Kind: indexer.SymbolTest},
		},
	}}
	res := Rank("fix package loader errors in internal/indexer", pkgs, scanner.Result{})
	if !hasRanked(res.Symbols, "LoadPackagesFlexible") || !hasRanked(res.Symbols, "loadASTOnly") {
		t.Fatalf("expected package-loader implementation symbols, got %#v", res.Symbols)
	}
	if len(res.Tests) != 1 || res.Tests[0] != "go test ./internal/indexer" {
		t.Fatalf("expected focused indexer test, got %#v", res.Tests)
	}
}

func TestRouteTaskKeepsExtractionSymbolsAndTests(t *testing.T) {
	pkgs := []indexer.PackageInfo{{
		ImportPath: "goctx/internal/routes",
		Name:       "routes",
		Files:      []string{"internal/routes/routes.go", "internal/routes/routes_test.go"},
		TestFiles:  []string{"internal/routes/routes_test.go"},
		Symbols: []indexer.SymbolInfo{
			{Name: "Extract", File: "internal/routes/routes.go", Kind: indexer.SymbolFunc},
			{Name: "ExtractFile", File: "internal/routes/routes.go", Kind: indexer.SymbolFunc},
			{Name: "TestRouteExtractionFrameworks", File: "internal/routes/routes_test.go", Kind: indexer.SymbolTest},
		},
	}}
	res := Rank("improve route extraction in internal/routes", pkgs, scanner.Result{})
	if !hasRanked(res.Symbols, "Extract") || !hasRanked(res.Symbols, "ExtractFile") {
		t.Fatalf("expected route extraction symbols, got %#v", res.Symbols)
	}
	if len(res.Tests) != 1 || res.Tests[0] != "go test ./internal/routes" {
		t.Fatalf("expected focused route test, got %#v", res.Tests)
	}
}

func hasRanked(items []RankedItem, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}
