package slicer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSample(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample_test.go")
	src := `package p

// Claims docs.
type Claims struct{}

type Verifier interface { Verify() error }

func Top() {
	println("top")
}

func (c *Claims) VerifyToken() {
	println("method")
}

func TestVerifyToken(t *testing.T) {}
`
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSliceTopLevelFunction(t *testing.T) {
	res, err := SliceFile(writeSample(t), "Top")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Source, "func Top()") || strings.Contains(res.Source, "VerifyToken") {
		t.Fatalf("bad slice:\n%s", res.Source)
	}
}

func TestSlicePointerMethodAndTypeAndInterfaceAndTest(t *testing.T) {
	path := writeSample(t)
	for _, sym := range []string{"(*Claims).VerifyToken", "Claims", "Verifier", "TestVerifyToken"} {
		res, err := SliceFile(path, sym)
		if err != nil {
			t.Fatal(err)
		}
		if res.Source == "" || res.NotFound || res.Ambiguous {
			t.Fatalf("expected slice for %s: %#v", sym, res)
		}
	}
}

func TestSliceUnknownSymbol(t *testing.T) {
	res, err := SliceFile(writeSample(t), "Missing")
	if err != nil {
		t.Fatal(err)
	}
	if !res.NotFound || len(res.Available) == 0 {
		t.Fatalf("expected not found with available symbols")
	}
}
