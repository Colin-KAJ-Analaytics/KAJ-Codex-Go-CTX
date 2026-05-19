package routes

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"goctx/internal/scanner"
)

type Route struct {
	Method  string
	Path    string
	File    string
	Line    int
	Handler string
	Dynamic bool
}

type Middleware struct {
	Path string
	File string
	Line int
	Expr string
}

type Result struct {
	Routes     []Route
	Middleware []Middleware
}

func Extract(root string) (Result, error) {
	sc, err := scanner.Scan(root)
	if err != nil {
		return Result{}, err
	}
	var res Result
	for _, f := range sc.Files {
		if f.IgnoredByDefault || (f.Kind != scanner.KindGoSource && f.Kind != scanner.KindGoTest) {
			continue
		}
		r, err := ExtractFile(root, filepath.Join(root, filepath.FromSlash(f.Path)))
		if err != nil {
			continue
		}
		res.Routes = append(res.Routes, r.Routes...)
		res.Middleware = append(res.Middleware, r.Middleware...)
	}
	sort.Slice(res.Routes, func(i, j int) bool {
		if res.Routes[i].File != res.Routes[j].File {
			return res.Routes[i].File < res.Routes[j].File
		}
		return res.Routes[i].Line < res.Routes[j].Line
	})
	return res, nil
}

func ExtractFile(root, path string) (Result, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return Result{}, err
	}
	rel := path
	if root != "" {
		if r, err := filepath.Rel(root, path); err == nil {
			rel = filepath.ToSlash(r)
		}
	}
	var res Result
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		name := sel.Sel.Name
		pos := fset.Position(call.Pos())
		if isUse(name) {
			expr := ""
			if len(call.Args) > 0 {
				expr = exprString(fset, call.Args[0])
			}
			res.Middleware = append(res.Middleware, Middleware{File: rel, Line: pos.Line, Expr: expr})
			return true
		}
		method, ok := routeMethod(name)
		if !ok || len(call.Args) == 0 {
			return true
		}
		pathText, dyn := pathArg(fset, call.Args[0])
		handler := ""
		if len(call.Args) > 1 {
			handler = exprString(fset, call.Args[1])
		}
		res.Routes = append(res.Routes, Route{Method: method, Path: pathText, File: rel, Line: pos.Line, Handler: handler, Dynamic: dyn})
		return true
	})
	return res, nil
}

func routeMethod(name string) (string, bool) {
	methods := map[string]string{"GET": "GET", "POST": "POST", "PUT": "PUT", "PATCH": "PATCH", "DELETE": "DELETE", "OPTIONS": "OPTIONS", "HEAD": "HEAD", "Get": "GET", "Post": "POST", "Put": "PUT", "Patch": "PATCH", "Delete": "DELETE", "Options": "OPTIONS", "Head": "HEAD", "Route": "GROUP", "HandleFunc": "ANY", "Handle": "ANY"}
	m, ok := methods[name]
	return m, ok
}

func isUse(name string) bool { return name == "Use" }

func pathArg(fset *token.FileSet, expr ast.Expr) (string, bool) {
	if lit, ok := expr.(*ast.BasicLit); ok && lit.Kind == token.STRING {
		p := strings.Trim(lit.Value, "`\"")
		return p, strings.ContainsAny(p, ":*{")
	}
	return exprString(fset, expr), true
}

func exprString(fset *token.FileSet, n ast.Node) string {
	var b bytes.Buffer
	if err := printer.Fprint(&b, fset, n); err != nil {
		return fmt.Sprintf("%T", n)
	}
	return b.String()
}
