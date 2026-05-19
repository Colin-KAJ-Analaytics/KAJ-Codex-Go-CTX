package diffsummary

import (
	"bytes"
	"strings"
	"testing"
)

func TestNoBaselineBehavior(t *testing.T) {
	var b bytes.Buffer
	if err := Summarize(t.TempDir(), &b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "No .goctx baseline found") {
		t.Fatalf("unexpected output: %s", b.String())
	}
}
