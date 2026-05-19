package tokens

import (
	"bytes"
	"strings"
	"testing"

	"goctx/internal/scanner"
)

func TestEstimate(t *testing.T) {
	if Estimate(400) != 100 {
		t.Fatal("bad estimate")
	}
}

func TestReportSuggestedExclusions(t *testing.T) {
	var b bytes.Buffer
	Report(&b, scanner.Result{Files: []scanner.FileInfo{{Path: "x.pb.go", Kind: scanner.KindGenerated, SizeBytes: 400, Reason: "generated file"}}})
	if !strings.Contains(b.String(), "**/*.pb.go") || !strings.Contains(b.String(), "estimated tokens: 100") {
		t.Fatalf("bad report:\n%s", b.String())
	}
}
