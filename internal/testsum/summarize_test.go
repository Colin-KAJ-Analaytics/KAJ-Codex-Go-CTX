package testsum

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRejectsNonTestGoCommands(t *testing.T) {
	for _, args := range [][]string{{"go"}, {"go", "env"}, {"go", "generate", "./..."}, {"go", "mod", "tidy"}} {
		if _, err := Run(context.Background(), args); err == nil {
			t.Fatalf("expected rejection for %v", args)
		}
	}
}

func TestSummarizePassingOutput(t *testing.T) {
	if !strings.Contains(Summarize("ok\t./internal/auth\t0.1s\n"), "Tests passed") {
		t.Fatal("expected pass summary")
	}
}

func TestSummarizeFailureAndPanic(t *testing.T) {
	raw := "--- FAIL: TestVerify (0.00s)\n    firebase_test.go:84: expected ErrExpiredToken, got nil\npanic: boom\nFAIL\t./internal/auth\n"
	got := Summarize(raw)
	for _, want := range []string{"TestVerify", "firebase_test.go:84", "expected ErrExpiredToken", "panic: boom"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestSubtestsKeepCompleteNames(t *testing.T) {
	raw := "--- FAIL: TestX/case_two/has-punctuation#01 (0.00s)\nFAIL\tp\n"
	if got := Summarize(raw); !strings.Contains(got, "TestX/case_two/has-punctuation#01") {
		t.Fatal(got)
	}
}

func TestPassWithFailureWords(t *testing.T) {
	raw := "=== RUN   TestHandlesFAILure\n    x_test.go:4: expected FAIL string\n--- PASS: TestHandlesFAILure (0.00s)\nPASS\nok\tp\t0.1s\n"
	for _, got := range []string{Summarize(raw), SummarizeWithExit(raw, 0)} {
		if !strings.HasPrefix(got, "Tests passed.") {
			t.Fatal(got)
		}
	}
}

func TestExitStatusAndDiagnostics(t *testing.T) {
	for _, raw := range []string{"# p\nx.go:3:12: undefined: missing\n", "go: cannot load module\n", "exec: go not found\n", "context deadline exceeded\n"} {
		got := SummarizeWithExit(raw, 1)
		if !strings.HasPrefix(got, "FAIL") || strings.Contains(got, "Tests passed") || !strings.Contains(got, strings.TrimSpace(strings.Split(raw, "\n")[0])) {
			t.Fatal(got)
		}
	}
}

func TestSummaryBoundsAllDiagnosticListsAndLongLines(t *testing.T) {
	var raw strings.Builder
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&raw, "--- FAIL: TestX/case%d (0.00s)\nfile%d.go:%d: expected %s\n", i, i, i+1, strings.Repeat("x", 1000))
	}
	got := Summarize(raw.String())
	if len(got) > maxSummaryBytes || !strings.Contains(got, "Omitted:") || !strings.Contains(got, "file0.go:1") {
		t.Fatalf("unbounded or missing first error: %d bytes", len(got))
	}
	if strings.Contains(got, "file999.go:1000") {
		t.Fatal("unbounded references")
	}
}

func TestRunCompilerFailurePreservesExitAndMessage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken_test.go")
	if err := os.WriteFile(path, []byte("package broken\nvar _ = MissingSymbol\n"), 0644); err != nil {
		t.Fatal(err)
	}
	sum, err := Run(context.Background(), []string{"go", "test", path})
	if err != nil {
		t.Fatal(err)
	}
	if sum.ExitCode == 0 || !strings.HasPrefix(sum.Output, "FAIL") || !strings.Contains(sum.Output, "MissingSymbol") {
		t.Fatalf("%#v", sum)
	}
}
