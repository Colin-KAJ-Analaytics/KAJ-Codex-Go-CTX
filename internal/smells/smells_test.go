package smells

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeLargeFunction(t *testing.T) {
	root := t.TempDir()
	var b strings.Builder
	b.WriteString("package x\nfunc Big(){\n")
	for i := 0; i < 130; i++ {
		b.WriteString("println(1)\n")
	}
	b.WriteString("}\n")
	if err := os.WriteFile(filepath.Join(root, "x.go"), []byte(b.String()), 0644); err != nil {
		t.Fatal(err)
	}
	res, err := Analyze(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Smells) == 0 {
		t.Fatal("expected smell")
	}
}
