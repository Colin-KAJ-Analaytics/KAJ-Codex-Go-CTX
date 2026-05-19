package indexer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
)

type SymbolKind string

const (
	SymbolFunc      SymbolKind = "func"
	SymbolMethod    SymbolKind = "method"
	SymbolType      SymbolKind = "type"
	SymbolInterface SymbolKind = "interface"
	SymbolConst     SymbolKind = "const"
	SymbolVar       SymbolKind = "var"
	SymbolTest      SymbolKind = "test"
)

type SymbolInfo struct {
	Name      string
	Receiver  string
	Kind      SymbolKind
	File      string
	StartLine int
	EndLine   int
	Exported  bool
}

func SymbolsInFile(path string) ([]SymbolInfo, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	return SymbolsFromAST(fset, file, path), nil
}

func SymbolsFromSource(filename string, src []byte) ([]SymbolInfo, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	return SymbolsFromAST(fset, file, filename), nil
}

func SymbolsFromAST(fset *token.FileSet, file *ast.File, path string) []SymbolInfo {
	var out []SymbolInfo
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			kind := SymbolFunc
			recv := ""
			if d.Recv != nil && len(d.Recv.List) > 0 {
				kind = SymbolMethod
				recv = receiverName(d.Recv.List[0].Type)
			}
			if kind == SymbolFunc && strings.HasPrefix(d.Name.Name, "Test") && strings.HasSuffix(path, "_test.go") {
				kind = SymbolTest
			}
			out = append(out, SymbolInfo{Name: d.Name.Name, Receiver: recv, Kind: kind, File: filepath.ToSlash(path), StartLine: fset.Position(startWithDoc(d.Doc, d.Pos())).Line, EndLine: fset.Position(d.End()).Line, Exported: ast.IsExported(d.Name.Name)})
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				start := startWithDoc(d.Doc, spec.Pos())
				switch s := spec.(type) {
				case *ast.TypeSpec:
					kind := SymbolType
					if _, ok := s.Type.(*ast.InterfaceType); ok {
						kind = SymbolInterface
					}
					out = append(out, SymbolInfo{Name: s.Name.Name, Kind: kind, File: filepath.ToSlash(path), StartLine: fset.Position(start).Line, EndLine: fset.Position(spec.End()).Line, Exported: ast.IsExported(s.Name.Name)})
				case *ast.ValueSpec:
					kind := SymbolVar
					if d.Tok == token.CONST {
						kind = SymbolConst
					}
					for _, n := range s.Names {
						out = append(out, SymbolInfo{Name: n.Name, Kind: kind, File: filepath.ToSlash(path), StartLine: fset.Position(start).Line, EndLine: fset.Position(spec.End()).Line, Exported: ast.IsExported(n.Name)})
					}
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].StartLine < out[j].StartLine
	})
	return out
}

func startWithDoc(doc *ast.CommentGroup, pos token.Pos) token.Pos {
	if doc != nil {
		return doc.Pos()
	}
	return pos
}

func receiverName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "(*" + receiverName(t.X) + ")"
	case *ast.IndexExpr:
		return receiverName(t.X)
	case *ast.IndexListExpr:
		return receiverName(t.X)
	default:
		return ""
	}
}

func DisplayName(s SymbolInfo) string {
	if s.Receiver == "" {
		return s.Name
	}
	if strings.HasPrefix(s.Receiver, "(*") {
		return s.Receiver + "." + s.Name
	}
	return s.Receiver + "." + s.Name
}
