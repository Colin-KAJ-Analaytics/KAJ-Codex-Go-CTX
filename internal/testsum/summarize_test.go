package testsum

import (
	"context"
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
