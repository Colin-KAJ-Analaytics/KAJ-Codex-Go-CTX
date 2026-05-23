package discovery

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"goctx/internal/scanner"
)

type Module struct {
	Path         string   `json:"path"`
	ModulePath   string   `json:"module_path"`
	Type         string   `json:"type"`
	MainPackages []string `json:"main_packages"`
	CommandLike  []string `json:"command_like"`
}

type Result struct {
	Root       string   `json:"root"`
	CWD        string   `json:"cwd"`
	GoWork     string   `json:"go_work,omitempty"`
	Modules    []Module `json:"modules"`
	Selected   string   `json:"selected,omitempty"`
	Warnings   []string `json:"warnings"`
	Confidence string   `json:"confidence"`
}

func Discover(start string) (Result, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return Result{}, err
	}
	root := repoCandidate(abs)
	res := Result{Root: filepath.ToSlash(root), CWD: filepath.ToSlash(abs), Confidence: "medium"}
	goWork := nearestUp(abs, "go.work")
	if goWork != "" {
		res.GoWork = filepath.ToSlash(goWork)
		res.Root = filepath.ToSlash(filepath.Dir(goWork))
		root = filepath.Dir(goWork)
	}
	modules, err := findModules(root)
	if err != nil {
		return res, err
	}
	for i := range modules {
		fillModuleShape(root, &modules[i])
	}
	res.Modules = modules
	selected := selectNearest(abs, modules)
	if selected != "" {
		res.Selected = selected
		res.Confidence = "high"
	} else if len(modules) == 1 {
		res.Selected = modules[0].Path
		res.Confidence = "high"
	} else if len(modules) > 1 {
		res.Warnings = append(res.Warnings, "multiple Go modules discovered; use --module to select one")
	}
	if len(modules) == 0 {
		res.Warnings = append(res.Warnings, "no go.mod files discovered")
		res.Confidence = "low"
	}
	return res, nil
}

func SelectRoot(start, module string) (string, Result, error) {
	res, err := Discover(start)
	if err != nil {
		return start, res, err
	}
	if module == "" {
		if res.Selected != "" {
			return filepath.Join(fromSlash(res.Root), fromSlash(res.Selected)), res, nil
		}
		return fromSlash(res.Root), res, nil
	}
	for _, m := range res.Modules {
		if m.Path == module || m.ModulePath == module || filepath.Base(m.Path) == module {
			res.Selected = m.Path
			return filepath.Join(fromSlash(res.Root), fromSlash(m.Path)), res, nil
		}
	}
	res.Warnings = append(res.Warnings, fmt.Sprintf("module %q not found; using discovered root", module))
	return fromSlash(res.Root), res, nil
}

func WriteText(w io.Writer, res Result) {
	fmt.Fprintln(w, "Discovered Go modules:")
	if len(res.Modules) == 0 {
		fmt.Fprintln(w, "- none")
	}
	for i, m := range res.Modules {
		fmt.Fprintf(w, "%d. %s\n", i+1, emptyDot(m.Path))
		fmt.Fprintf(w, "   module: %s\n", m.ModulePath)
		fmt.Fprintf(w, "   type: %s\n", m.Type)
		if len(m.MainPackages) > 0 {
			fmt.Fprintln(w, "   main packages:")
			for _, p := range m.MainPackages {
				fmt.Fprintf(w, "   - %s\n", p)
			}
		}
	}
	if res.Selected != "" {
		fmt.Fprintf(w, "\nSelected module: %s\n", emptyDot(res.Selected))
	}
	if len(res.Warnings) > 0 {
		fmt.Fprintln(w, "\nWarnings:")
		for _, msg := range res.Warnings {
			fmt.Fprintf(w, "- %s\n", msg)
		}
	}
	if len(res.Modules) > 1 {
		fmt.Fprintln(w, "\nRecommended:")
		fmt.Fprintf(w, "goctx task --module %s \"<task>\"\n", emptyDot(res.Selected))
	}
}

func WriteJSON(w io.Writer, res Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(res)
}

func findModules(root string) ([]Module, error) {
	var mods []Module
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && scannerDefaultIgnoredDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Base(path) == "go.mod" {
			dir := filepath.Dir(path)
			r, _ := filepath.Rel(root, dir)
			mods = append(mods, Module{Path: filepath.ToSlash(r), ModulePath: readModulePath(path), Type: "go.mod"})
		}
		return nil
	})
	sort.Slice(mods, func(i, j int) bool { return mods[i].Path < mods[j].Path })
	return mods, err
}

func fillModuleShape(root string, mod *Module) {
	modRoot := filepath.Join(root, filepath.FromSlash(mod.Path))
	sc, err := scanner.Scan(modRoot)
	if err != nil {
		return
	}
	for _, f := range sc.Files {
		if f.IgnoredByDefault || f.Kind != scanner.KindGoSource {
			continue
		}
		if isMainPackage(filepath.Join(modRoot, filepath.FromSlash(f.Path))) {
			dir := filepath.ToSlash(filepath.Dir(f.Path))
			if dir == "." {
				dir = "."
			}
			mod.MainPackages = appendUnique(mod.MainPackages, dir)
			if strings.HasPrefix(dir, "cmd/") || dir == "cmd" {
				mod.CommandLike = appendUnique(mod.CommandLike, dir)
			}
		}
	}
	sort.Strings(mod.MainPackages)
	sort.Strings(mod.CommandLike)
}

func repoCandidate(start string) string {
	if gw := nearestUp(start, "go.work"); gw != "" {
		return filepath.Dir(gw)
	}
	if gm := nearestUp(start, "go.mod"); gm != "" {
		return filepath.Dir(gm)
	}
	return start
}

func nearestUp(start, name string) string {
	dir := start
	for {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
		next := filepath.Dir(dir)
		if next == dir {
			return ""
		}
		dir = next
	}
}

func selectNearest(cwd string, mods []Module) string {
	var best string
	bestLen := -1
	for _, m := range mods {
		if m.Path == "." {
			return "."
		}
		p := filepath.Clean(filepath.Join(repoCandidate(cwd), filepath.FromSlash(m.Path)))
		if strings.HasPrefix(cwd, p) && len(p) > bestLen {
			bestLen = len(p)
			best = m.Path
		}
	}
	return best
}

func readModulePath(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

func isMainPackage(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(io.LimitReader(f, 4096))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "package ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "package ")) == "main"
		}
	}
	return false
}

func scannerDefaultIgnoredDir(rel string) bool {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	ignored := map[string]bool{"vendor": true, "node_modules": true, "dist": true, "build": true, "coverage": true, "tmp": true, "bin": true, ".next": true, ".cache": true, ".git": true, ".goctx": true}
	for _, p := range parts {
		if ignored[p] {
			return true
		}
	}
	return false
}

func appendUnique(xs []string, x string) []string {
	for _, v := range xs {
		if v == x {
			return xs
		}
	}
	return append(xs, x)
}

func fromSlash(p string) string {
	if p == "" {
		return p
	}
	return filepath.FromSlash(p)
}

func emptyDot(s string) string {
	if s == "" || s == "." {
		return "."
	}
	return s
}
