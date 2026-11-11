package codexpack

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"goctx/internal/scanner"
	"goctx/internal/tokens"
)

type EmbedFile struct {
	Path            string `json:"path"`
	EstimatedTokens int64  `json:"estimated_tokens"`
}

// EmbedDeclaration describes a single pattern on a Go embed variable.
type EmbedDeclaration struct {
	File     string      `json:"file"`
	Line     int         `json:"line"`
	Variable string      `json:"variable"`
	Pattern  string      `json:"pattern"`
	Files    []EmbedFile `json:"files"`
}

// Embeds lists source declarations in the scanned tree.
func Embeds(scan scanner.Result) []EmbedDeclaration {
	return readSource(scan, true).embeds
}

type agentSource struct {
	embeds   []EmbedDeclaration
	assets   map[string]AgentFile
	files    map[string]*ast.File
	bindings map[string]map[string][]string
}

var embedWords = regexp.MustCompile("\"(?:\\\\.|[^\"\\\\])*\"|`[^`]*`|[^\\s]+")

func readAgentSource(scan scanner.Result) agentSource {
	return readSource(scan, false)
}

func readSource(scan scanner.Result, inventory bool) agentSource {
	info := agentSource{assets: map[string]AgentFile{}, files: map[string]*ast.File{}, bindings: map[string]map[string][]string{}}
	for _, f := range scan.Files {
		if f.IgnoredByDefault || (f.Kind != scanner.KindGoSource && f.Kind != scanner.KindGoTest) {
			continue
		}
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, filepath.Join(scan.Root, filepath.FromSlash(f.Path)), nil, parser.ParseComments)
		if err != nil {
			continue
		}
		info.files[f.Path] = parsed
		if f.Kind == scanner.KindGoTest && !inventory {
			continue
		}
		for _, decl := range parsed.Decls {
			vars, ok := decl.(*ast.GenDecl)
			if !ok || vars.Tok != token.VAR {
				continue
			}
			for _, spec := range vars.Specs {
				v := spec.(*ast.ValueSpec)
				if len(v.Names) != 1 {
					continue
				}
				doc := v.Doc
				if doc == nil && len(vars.Specs) == 1 {
					doc = vars.Doc
				}
				if doc == nil {
					continue
				}
				for _, line := range doc.List {
					body, ok := strings.CutPrefix(line.Text, "//go:embed ")
					if !ok {
						body, ok = strings.CutPrefix(line.Text, "//go:embed\t")
					}
					if !ok {
						continue
					}
					for _, word := range embedWords.FindAllString(body, -1) {
						if strings.HasPrefix(word, "\"") || strings.HasPrefix(word, "`") {
							value, err := strconv.Unquote(word)
							if err != nil {
								continue
							}
							word = value
						}
						if info.bindings[f.Path] == nil {
							info.bindings[f.Path] = map[string][]string{}
						}
						name := v.Names[0].Name
						info.bindings[f.Path][name] = append(info.bindings[f.Path][name], word)
						declaration := EmbedDeclaration{File: f.Path, Line: fset.Position(line.Pos()).Line, Variable: name, Pattern: word, Files: []EmbedFile{}}
						for _, asset := range scan.Files {
							if (!inventory && asset.IgnoredByDefault) || strings.HasSuffix(asset.Path, "/") {
								continue
							}
							rel, err := filepath.Rel(filepath.FromSlash(path.Dir(f.Path)), filepath.FromSlash(asset.Path))
							if err != nil || !filepath.IsLocal(rel) {
								continue
							}
							if embedMatches(word, filepath.ToSlash(rel)) {
								if inventory {
									st, err := os.Lstat(filepath.Join(scan.Root, filepath.FromSlash(asset.Path)))
									if err == nil && st.Mode().IsRegular() {
										declaration.Files = append(declaration.Files, EmbedFile{Path: asset.Path, EstimatedTokens: tokens.Estimate(st.Size())})
									}
								} else if _, seen := info.assets[asset.Path]; !seen {
									info.assets[asset.Path] = AgentFile{Path: asset.Path, EstimatedTokens: tokens.Estimate(asset.SizeBytes), EmbedPattern: word}
								}
							}
						}
						if inventory {
							info.embeds = append(info.embeds, declaration)
						}
					}
				}
			}
		}
	}
	return info
}

func embedMatches(pattern, name string) bool {
	pattern, all := strings.CutPrefix(pattern, "all:")
	if !filepath.IsLocal(pattern) || pattern == "." {
		return false
	}
	// Direct glob matches may explicitly select hidden entries. When a
	// directory is embedded recursively, Go skips hidden descendants.
	for prefix := name; prefix != "."; prefix = path.Dir(prefix) {
		if ok, _ := path.Match(pattern, prefix); !ok {
			continue
		}
		if all || prefix == name {
			return true
		}
		for _, part := range strings.Split(strings.TrimPrefix(name, prefix+"/"), "/") {
			if strings.HasPrefix(part, ".") || strings.HasPrefix(part, "_") {
				return false
			}
		}
		return true
	}
	return false
}

func importedAs(file *ast.File, ident, importPath string) bool {
	for _, imp := range file.Imports {
		if imp.Path.Value != strconv.Quote(importPath) {
			continue
		}
		if imp.Name == nil && ident == path.Base(importPath) || imp.Name != nil && imp.Name.Name == ident {
			return true
		}
	}
	return false
}

// routeAsset resolves only a direct, literal ServeFileFS call from a handler
// in the registration file. Directory-serving and dynamic handlers are hints.
func (s agentSource) routeAsset(file, handler string) string {
	parsed := s.files[file]
	if parsed == nil {
		return ""
	}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name.Name != handler || fn.Body == nil || len(fn.Body.List) != 1 {
			continue
		}
		stmt, ok := fn.Body.List[0].(*ast.ExprStmt)
		if !ok {
			continue
		}
		call, ok := stmt.X.(*ast.CallExpr)
		if !ok || len(call.Args) != 4 {
			continue
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "ServeFileFS" {
			continue
		}
		pkg, ok := selector.X.(*ast.Ident)
		if !ok || pkg.Obj != nil || !importedAs(parsed, pkg.Name, "net/http") {
			continue
		}
		fs, ok := call.Args[2].(*ast.Ident)
		if !ok || fs.Obj == nil || fs.Obj.Kind != ast.Var {
			continue
		}
		name := literalString(call.Args[3])
		if name == "" || !filepath.IsLocal(name) {
			continue
		}
		target := path.Join(path.Dir(file), name)
		if _, ok := s.assets[target]; !ok {
			continue
		}
		for _, pattern := range s.bindings[file][fs.Name] {
			if embedMatches(pattern, name) {
				return target
			}
		}
	}
	return ""
}

func literalString(expr ast.Expr) string {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	value, _ := strconv.Unquote(lit.Value)
	return value
}

func (s agentSource) focusedTest(symbol AgentSymbol, route string) string {
	var best string
	for file, parsed := range s.files {
		if !strings.HasSuffix(file, "_test.go") {
			continue
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") || fn.Body == nil {
				continue
			}
			matched := false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if id, ok := call.Fun.(*ast.Ident); ok && id.Obj == nil && symbol.File != "" && id.Name == symbol.Name && path.Dir(file) == path.Dir(symbol.File) {
					matched = true
				}
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && route != "" && sel.Sel.Name == "NewRequest" && len(call.Args) >= 2 {
					if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Obj == nil && (importedAs(parsed, pkg.Name, "net/http/httptest") || importedAs(parsed, pkg.Name, "net/http")) && literalString(call.Args[1]) == route {
						matched = true
					}
				}
				return !matched
			})
			if matched {
				command := "goctx test-summary -- go test " + quoteArg(packageArg(path.Dir(file))) + " -run " + quoteArg("^"+fn.Name.Name+"$")
				if best == "" || command < best {
					best = command
				}
			}
		}
	}
	return best
}

// readAsset reads bounded source for local reference matching.
func readAsset(root, name string) string {
	file, err := os.Open(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		return ""
	}
	defer file.Close()
	if st, err := file.Stat(); err != nil || !st.Mode().IsRegular() {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(file, 128*1024))
	if err != nil {
		return ""
	}
	return string(data)
}

type linkedAsset struct {
	file   AgentFile
	score  int
	source string
}

// linkedAssets matches root-relative URLs only through known static mounts.
func (s agentSource) linkedAssets(root, entry string, terms map[string]bool, mounts []staticMount) []linkedAsset {
	if path.Ext(entry) != ".html" {
		return nil
	}
	var found []linkedAsset
	seen := map[string]bool{entry: true}
	for _, match := range htmlAssetRef.FindAllStringSubmatch(readAsset(root, entry), -1) {
		ref := strings.Trim(match[1], "\"'")
		if strings.ContainsAny(ref, ":?#\\") || strings.HasPrefix(ref, "//") {
			continue
		}
		candidate := path.Join(path.Dir(entry), ref)
		if strings.HasPrefix(ref, "/") {
			candidate = ""
			bestPrefix := 0
			ambiguous := false
			if path.Clean(ref) != ref {
				continue
			}
			for _, mount := range mounts {
				if !strings.HasPrefix(ref, mount.prefix) || len(mount.prefix) < bestPrefix {
					continue
				}
				name := strings.TrimPrefix(ref, "/")
				if mount.strip {
					name = strings.TrimPrefix(ref, mount.prefix)
				}
				target := path.Join(mount.dir, name)
				if len(mount.prefix) == bestPrefix && candidate != target {
					ambiguous = true
					continue
				}
				candidate, bestPrefix, ambiguous = target, len(mount.prefix), false
			}
			if ambiguous {
				continue
			}
		}
		f, ok := s.assets[candidate]
		if !ok || seen[candidate] {
			continue
		}
		seen[candidate] = true
		f.AssetURL = ref
		source := ""
		score := matchTerms(terms, path.Base(f.Path)) * 10
		if ext := path.Ext(f.Path); ext == ".js" || ext == ".mjs" {
			source = readAsset(root, f.Path)
			score += matchTerms(terms, source)
		}
		found = append(found, linkedAsset{f, score, source})
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].score > found[j].score })
	return found
}

var htmlAssetRef = regexp.MustCompile(`(?i)\b(?:src|href)\s*=\s*("[^"]+"|'[^']+')`)
var literalFetch = regexp.MustCompile(`\bfetch\s*\(\s*["'](/[^"'?#]+)["']`)
