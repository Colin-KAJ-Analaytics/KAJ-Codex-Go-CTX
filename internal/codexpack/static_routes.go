package codexpack

import (
	"go/ast"
	"path"
	"strings"
)

type staticMount struct {
	prefix, dir string
	strip       bool
}

func selectorCall(expr ast.Expr, file *ast.File, pkg, name string) *ast.CallExpr {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return nil
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Obj != nil || !importedAs(file, id.Name, pkg) {
		return nil
	}
	return call
}

func assigned(expr ast.Expr) ast.Expr {
	id, ok := expr.(*ast.Ident)
	if !ok || id.Obj == nil {
		return expr
	}
	stmt, ok := id.Obj.Decl.(*ast.AssignStmt)
	if ok && len(stmt.Lhs) > 0 && len(stmt.Rhs) == 1 {
		if lhs, ok := stmt.Lhs[0].(*ast.Ident); ok && lhs.Obj == id.Obj {
			return stmt.Rhs[0]
		}
	}
	return expr
}

func (s agentSource) embeddedSub(file string, expr ast.Expr) (string, bool) {
	parsed := s.files[file]
	call := selectorCall(assigned(expr), parsed, "io/fs", "Sub")
	if call == nil || len(call.Args) != 2 {
		return "", false
	}
	id, ok := call.Args[0].(*ast.Ident)
	if !ok || id.Obj == nil {
		return "", false
	}
	spec, ok := id.Obj.Decl.(*ast.ValueSpec)
	if !ok || len(s.bindings[file][id.Name]) == 0 {
		return "", false
	}
	sel, ok := spec.Type.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "FS" {
		return "", false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || !importedAs(parsed, pkg.Name, "embed") {
		return "", false
	}
	dir := literalString(call.Args[1])
	if dir == "" || path.Clean(dir) != dir || strings.HasPrefix(dir, "/") || dir == ".." || strings.HasPrefix(dir, "../") {
		return "", false
	}
	return path.Join(path.Dir(file), dir), true
}

// handlerArgument recognizes a same-file middleware function that forwards to
// its handler parameter. Arbitrary calls are not treated as transparent.
func handlerArgument(file *ast.File, expr ast.Expr) ast.Expr {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return expr
	}
	id, ok := call.Fun.(*ast.Ident)
	if !ok || id.Obj == nil {
		return expr
	}
	fn, ok := id.Obj.Decl.(*ast.FuncDecl)
	if !ok || fn.Body == nil || fn.Type.Params == nil || len(fn.Type.Params.List) != 1 || len(fn.Type.Params.List[0].Names) != 1 || len(fn.Body.List) != 1 {
		return expr
	}
	ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return expr
	}
	adapter := selectorCall(ret.Results[0], file, "net/http", "HandlerFunc")
	if adapter == nil || len(adapter.Args) != 1 {
		return expr
	}
	closure, ok := adapter.Args[0].(*ast.FuncLit)
	if !ok || len(closure.Body.List) == 0 {
		return expr
	}
	stmt, ok := closure.Body.List[len(closure.Body.List)-1].(*ast.ExprStmt)
	if !ok {
		return expr
	}
	forward, ok := stmt.X.(*ast.CallExpr)
	if !ok {
		return expr
	}
	sel, ok := forward.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "ServeHTTP" {
		return expr
	}
	next, ok := sel.X.(*ast.Ident)
	if !ok || next.Obj != fn.Type.Params.List[0].Names[0].Obj {
		return expr
	}
	if closure.Type.Params == nil || len(closure.Type.Params.List) != 2 || len(forward.Args) != 2 {
		return expr
	}
	for i, field := range closure.Type.Params.List {
		arg, ok := forward.Args[i].(*ast.Ident)
		if !ok || len(field.Names) != 1 || arg.Obj != field.Names[0].Obj {
			return expr
		}
	}
	return call.Args[0]
}

func (s agentSource) staticRoutes() ([]staticMount, []AgentRoute) {
	var mounts []staticMount
	var pages []AgentRoute
	for name, file := range s.files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Handle" {
				return true
			}
			if selectorCall(call, file, "net/http", "Handle") == nil && selectorCall(assigned(sel.X), file, "net/http", "NewServeMux") == nil {
				return true
			}
			prefix := literalString(call.Args[0])
			if !strings.HasPrefix(prefix, "/") || !strings.HasSuffix(prefix, "/") {
				return true
			}
			handler := assigned(handlerArgument(file, call.Args[1]))
			strip := false
			if p := selectorCall(handler, file, "net/http", "StripPrefix"); p != nil && len(p.Args) == 2 && literalString(p.Args[0]) == prefix {
				handler, strip = assigned(p.Args[1]), true
			}
			if server := selectorCall(handler, file, "net/http", "FileServer"); server != nil && len(server.Args) == 1 {
				if fs := selectorCall(server.Args[0], file, "net/http", "FS"); fs != nil && len(fs.Args) == 1 {
					if dir, ok := s.embeddedSub(name, fs.Args[0]); ok {
						mounts = append(mounts, staticMount{prefix, dir, strip})
					}
				}
			}
			if adapter := selectorCall(handler, file, "net/http", "HandlerFunc"); adapter != nil && len(adapter.Args) == 1 {
				if fn, ok := adapter.Args[0].(*ast.FuncLit); ok {
					for _, page := range s.conditionalPages(name, fn) {
						if strings.HasPrefix(page.Path, prefix) {
							pages = append(pages, page)
						}
					}
				}
			}
			return true
		})
	}
	return mounts, pages
}

func (s agentSource) conditionalPages(file string, fn *ast.FuncLit) []AgentRoute {
	var pages []AgentRoute
	for _, stmt := range fn.Body.List {
		branch, ok := stmt.(*ast.IfStmt)
		if !ok || branch.Init != nil || len(branch.Body.List) != 2 {
			continue
		}
		if _, ok := branch.Body.List[1].(*ast.ReturnStmt); !ok {
			continue
		}
		eq, ok := branch.Cond.(*ast.BinaryExpr)
		if !ok || eq.Op.String() != "==" {
			continue
		}
		left, ok := eq.X.(*ast.SelectorExpr)
		if !ok || left.Sel.Name != "Path" {
			continue
		}
		url, ok := left.X.(*ast.SelectorExpr)
		if !ok || url.Sel.Name != "URL" {
			continue
		}
		req, ok := url.X.(*ast.Ident)
		if !ok || fn.Type.Params == nil || len(fn.Type.Params.List) != 2 || len(fn.Type.Params.List[1].Names) != 1 || req.Obj != fn.Type.Params.List[1].Names[0].Obj {
			continue
		}
		route := literalString(eq.Y)
		if !strings.HasPrefix(route, "/") {
			continue
		}
		expr, ok := branch.Body.List[0].(*ast.ExprStmt)
		if !ok {
			continue
		}
		call, ok := expr.X.(*ast.CallExpr)
		if !ok || len(call.Args) != 3 {
			continue
		}
		helper, ok := call.Fun.(*ast.Ident)
		if !ok || helper.Obj == nil || !readsEmbeddedFile(s.files[file], helper.Obj) {
			continue
		}
		dir, ok := s.embeddedSub(file, call.Args[1])
		name := literalString(call.Args[2])
		if !ok || name == "" || path.Clean(name) != name || strings.HasPrefix(name, "/") || name == ".." || strings.HasPrefix(name, "../") {
			continue
		}
		target := path.Join(dir, name)
		if _, ok := s.assets[target]; ok {
			pages = append(pages, AgentRoute{"ANY", route, target})
		}
	}
	return pages
}

func readsEmbeddedFile(file *ast.File, object *ast.Object) bool {
	fn, ok := object.Decl.(*ast.FuncDecl)
	if !ok || fn.Body == nil || fn.Type.Params == nil || len(fn.Type.Params.List) != 3 {
		return false
	}
	if len(fn.Type.Params.List[1].Names) != 1 || len(fn.Type.Params.List[2].Names) != 1 {
		return false
	}
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || selectorCall(call, file, "io/fs", "ReadFile") == nil || len(call.Args) != 2 {
			return !found
		}
		fs, ok := call.Args[0].(*ast.Ident)
		name, nok := call.Args[1].(*ast.Ident)
		found = ok && nok && fs.Obj == fn.Type.Params.List[1].Names[0].Obj && name.Obj == fn.Type.Params.List[2].Names[0].Obj
		return !found
	})
	return found
}
