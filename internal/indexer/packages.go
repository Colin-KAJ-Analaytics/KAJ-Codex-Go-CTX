package indexer

import (
	"context"
	"os"
	"path/filepath"
	"sort"

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

func LoadPackages(ctx context.Context, root string) ([]PackageInfo, error) {
	cfg := &packages.Config{Context: ctx, Dir: root, Mode: packages.NeedName | packages.NeedFiles | packages.NeedImports | packages.NeedModule, Tests: false}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, err
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
	return out, nil
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
