package indexer

import "testing"

func TestSymbolsFromSource(t *testing.T) {
	src := []byte(`package p
type Claims struct{}
type Verifier interface{ Verify() error }
const Answer = 42
var value = 1
func NewVerifier() {}
func (v *Claims) VerifyToken() {}
func TestVerifyToken(t *testing.T) {}
`)
	syms, err := SymbolsFromSource("auth_test.go", src)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]SymbolKind{"Claims": SymbolType, "Verifier": SymbolInterface, "Answer": SymbolConst, "value": SymbolVar, "NewVerifier": SymbolFunc, "(*Claims).VerifyToken": SymbolMethod, "TestVerifyToken": SymbolTest}
	for _, s := range syms {
		delete(want, DisplayName(s))
		if s.StartLine == 0 || s.EndLine == 0 {
			t.Fatalf("missing line numbers for %#v", s)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing symbols: %#v", want)
	}
}
