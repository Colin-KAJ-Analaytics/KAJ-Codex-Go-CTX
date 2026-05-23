package indexer

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"goctx/internal/scanner"

	"golang.org/x/tools/go/packages"
)

type PackageInfo struct {
	Name       string
	ImportPath string
	Dir        string
	Files      []string
	TestFiles  []string
	Imports    []string
	Symbols    []SymbolInfo
}

type LoadResult struct {
	Packages   []PackageInfo
	Mode       string
	Confidence string
	Warnings   []string
}

func LoadPackages(ctx context.Context, root string) ([]PackageInfo, error) {
	res, err := LoadPackagesFlexible(ctx, root)
	return res.Packages, err
}

func LoadPackagesFlexible(ctx context.Context, root string) (LoadResult, error) {
	cfg := &packages.Config{Context: ctx, Dir: root, Mode: packages.NeedName | packages.NeedFiles | packages.NeedImports | packages.NeedModule, Tests: false}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return loadASTOnly(root, []string{err.Error()})
	}
	var warnings []string
	for _, p := range pkgs {
		for _, e := range p.Errors {
			warnings = append(warnings, e.Msg)
		}
	}
	seen := map[string]bool{}
	var out []PackageInfo
	for _, p := range pkgs {
		if p.PkgPath == "" || seen[p.PkgPath] {
			continue
		}
		seen[p.PkgPath] = true
		info := PackageInfo{Name: p.Name, ImportPath: p.PkgPath}
		dirAbs := ""
		for _, f := range p.GoFiles {
			rel := relPath(root, f)
			if filepath.Base(f) == "" {
				continue
			}
			if ignoredByScanner(root, rel, f) {
				continue
			}
			if dirAbs == "" {
				dirAbs = filepath.Dir(f)
			}
			if info.Dir == "" {
				info.Dir = filepath.ToSlash(filepath.Dir(rel))
				if info.Dir == "." {
					info.Dir = "."
				}
			}
			if filepath.Ext(f) == ".go" {
				info.Files = append(info.Files, rel)
				syms, _ := SymbolsInFile(f)
				for _, s := range syms {
					s.File = rel
					info.Symbols = append(info.Symbols, s)
				}
			}
		}
		if dirAbs != "" {
			tests, _ := filepath.Glob(filepath.Join(dirAbs, "*_test.go"))
			for _, f := range tests {
				rel := relPath(root, f)
				if ignoredByScanner(root, rel, f) {
					continue
				}
				info.Files = append(info.Files, rel)
				info.TestFiles = append(info.TestFiles, rel)
				syms, _ := SymbolsInFile(f)
				for _, s := range syms {
					s.File = rel
					info.Symbols = append(info.Symbols, s)
				}
			}
		}
		for _, f := range p.OtherFiles {
			_ = f
		}
		for path := range p.Imports {
			info.Imports = append(info.Imports, path)
		}
		sort.Strings(info.Files)
		sort.Strings(info.TestFiles)
		sort.Strings(info.Imports)
		sort.Slice(info.Symbols, func(i, j int) bool {
			if info.Symbols[i].File != info.Symbols[j].File {
				return info.Symbols[i].File < info.Symbols[j].File
			}
			return info.Symbols[i].StartLine < info.Symbols[j].StartLine
		})
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ImportPath < out[j].ImportPath })
	if len(out) == 0 || allEmpty(out) {
		if len(warnings) == 0 {
			warnings = append(warnings, "go/packages returned no useful package metadata")
		}
		return loadASTOnly(root, warnings)
	}
	conf := "high"
	if len(warnings) > 0 {
		conf = "medium"
	}
	return LoadResult{Packages: out, Mode: "go/packages", Confidence: conf, Warnings: compactWarnings(warnings)}, nil
}

func ignoredByScanner(root, rel, abs string) bool {
	st, err := os.Stat(abs)
	if err != nil {
		return false
	}
	return scanner.Classify(root, rel, st.Size()).IgnoredByDefault
}

func relPath(root, path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if rel, err := filepath.Rel(root, path); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

func ExistingGoModule(root string) bool {
	_, err := os.Stat(filepath.Join(root, "go.mod"))
	return err == nil
}

func loadASTOnly(root string, warnings []string) (LoadResult, error) {
	sc, err := scanner.Scan(root)
	if err != nil {
		return LoadResult{Mode: "scanner-only", Confidence: "low", Warnings: append(warnings, err.Error())}, err
	}
	byDir := map[string]*PackageInfo{}
	for _, f := range sc.Files {
		if f.IgnoredByDefault || (f.Kind != scanner.KindGoSource && f.Kind != scanner.KindGoTest) {
			continue
		}
		abs := filepath.Join(root, filepath.FromSlash(f.Path))
		syms, err := SymbolsInFile(abs)
		if err != nil {
			warnings = append(warnings, "parse failed: "+f.Path)
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(f.Path))
		if dir == "." {
			dir = "."
		}
		p := byDir[dir]
		if p == nil {
			pkgName := packageName(abs)
			p = &PackageInfo{Name: pkgName, ImportPath: dir, Dir: dir}
			byDir[dir] = p
		}
		p.Files = append(p.Files, f.Path)
		if strings.HasSuffix(f.Path, "_test.go") {
			p.TestFiles = append(p.TestFiles, f.Path)
		}
		for _, s := range syms {
			s.File = f.Path
			p.Symbols = append(p.Symbols, s)
		}
		p.Imports = append(p.Imports, importsForFile(abs)...)
	}
	var out []PackageInfo
	for _, p := range byDir {
		sort.Strings(p.Files)
		sort.Strings(p.TestFiles)
		p.Imports = uniqueSorted(p.Imports)
		sort.Slice(p.Symbols, func(i, j int) bool {
			if p.Symbols[i].File != p.Symbols[j].File {
				return p.Symbols[i].File < p.Symbols[j].File
			}
			return p.Symbols[i].StartLine < p.Symbols[j].StartLine
		})
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ImportPath < out[j].ImportPath })
	conf := "medium"
	if len(out) == 0 {
		conf = "low"
	}
	return LoadResult{Packages: out, Mode: "ast-only", Confidence: conf, Warnings: compactWarnings(append([]string{"package loading degraded to AST-only fallback"}, warnings...))}, nil
}

func packageName(path string) string {
	f, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(f), "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "package ") {
			return strings.Fields(l)[1]
		}
	}
	return ""
}

func importsForFile(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var imports []string
	for _, l := range strings.Split(string(data), "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "\"") && strings.HasSuffix(l, "\"") {
			imports = append(imports, strings.Trim(l, "\""))
		}
		if strings.HasPrefix(l, "import ") && strings.Contains(l, "\"") {
			parts := strings.Split(l, "\"")
			if len(parts) >= 2 {
				imports = append(imports, parts[1])
			}
		}
	}
	return imports
}

func uniqueSorted(xs []string) []string {
	sort.Strings(xs)
	out := xs[:0]
	for _, x := range xs {
		if x == "" || (len(out) > 0 && out[len(out)-1] == x) {
			continue
		}
		out = append(out, x)
	}
	return out
}

func allEmpty(pkgs []PackageInfo) bool {
	for _, p := range pkgs {
		if len(p.Files) > 0 || len(p.Symbols) > 0 {
			return false
		}
	}
	return true
}

func compactWarnings(warnings []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, w := range warnings {
		w = strings.TrimSpace(w)
		if w == "" || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
		if len(out) >= 5 {
			out = append(out, "additional warnings omitted")
			break
		}
	}
	return out
}
