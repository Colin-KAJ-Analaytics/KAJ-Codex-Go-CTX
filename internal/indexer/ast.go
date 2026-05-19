package indexer

import (
	"go/format"
	"go/printer"
	"go/token"
	"strings"
)

func ExprString(fset *token.FileSet, x any) string {
	var b strings.Builder
	_ = printer.Fprint(&b, fset, x)
	return b.String()
}

func FormatSource(src []byte) []byte {
	out, err := format.Source(src)
	if err != nil {
		return src
	}
	return out
}
