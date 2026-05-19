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
