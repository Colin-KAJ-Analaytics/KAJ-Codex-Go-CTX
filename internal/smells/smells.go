package smells

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"goctx/internal/output"
	"goctx/internal/scanner"
)

type Smell struct {
	Priority string   `json:"priority"`
	File     string   `json:"file"`
	Line     int      `json:"line,omitempty"`
	Reasons  []string `json:"reasons"`
	Advice   string   `json:"advice"`
	Score    int      `json:"score"`
}

type Result struct {
	Smells []Smell `json:"smells"`
}

func Analyze(root string) (Result, error) {
	sc, err := scanner.Scan(root)
	if err != nil {
		return Result{}, err
	}
	var out []Smell
	for _, f := range sc.Files {
		if f.IgnoredByDefault {
			if f.Kind == scanner.KindFixture && f.SizeBytes > 0 {
				out = append(out, Smell{Priority: "P2", File: f.Path, Score: int(f.SizeBytes / 1024), Reasons: []string{"huge fixture/golden/snapshot-like file"}, Advice: "avoid reading unless task explicitly needs fixture contents"})
			}
			continue
		}
		if f.Kind != scanner.KindGoSource && f.Kind != scanner.KindGoTest {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(f.Path))
		sm := analyzeGoFile(path, f)
		out = append(out, sm...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].File < out[j].File
		}
		return out[i].Score > out[j].Score
	})
	return Result{Smells: out}, nil
}

func WriteText(w io.Writer, res Result, opts output.Options) {
	lines := []string{"Token risk smells:"}
	limit := opts.MaxFiles
	if limit <= 0 || limit > len(res.Smells) {
		limit = len(res.Smells)
	}
	for i := 0; i < limit; i++ {
		s := res.Smells[i]
		lines = append(lines, fmt.Sprintf("%s %s", s.Priority, s.File))
		for _, r := range s.Reasons {
			lines = append(lines, "- "+r)
		}
		if s.Advice != "" {
			lines = append(lines, "- recommendation: "+s.Advice)
		}
		lines = append(lines, "")
	}
	out, omitted, used := output.TruncateLines(lines, opts.Budget)
	for _, l := range out {
		fmt.Fprintln(w, l)
	}
	fmt.Fprintf(w, "Actual: %d estimated tokens\n", used)
	if omitted > 0 {
		fmt.Fprintf(w, "Omitted:\n- %d low-priority smell lines due to budget\n", omitted)
	}
}

func Envelope(res Result, opts output.Options) output.Envelope {
	env := output.Envelope{Command: "smells", Root: opts.Root, BudgetEstimatedTokens: opts.Budget, Confidence: "medium"}
	limit := opts.MaxFiles
	if limit <= 0 || limit > len(res.Smells) {
		limit = len(res.Smells)
	}
	for i := 0; i < limit; i++ {
		s := res.Smells[i]
		env.Files = append(env.Files, output.FileItem{Path: s.File, Score: s.Score, Reasons: append(s.Reasons, s.Advice), Line: s.Line})
	}
	if limit < len(res.Smells) {
		env.Omitted = append(env.Omitted, output.Omission{Kind: "smells", Count: len(res.Smells) - limit, Reason: "max files limit"})
	}
	env.ActualEstimatedTokens = int64(len(env.Files)*80 + 100)
	env.Data = res
	return env
}

func analyzeGoFile(path string, info scanner.FileInfo) []Smell {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Count(string(data), "\n") + 1
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, data, 0)
	if err != nil {
		return nil
	}
	var out []Smell
	var funcs, handlers, exported int
	imports := map[string]bool{}
	for _, imp := range file.Imports {
		imports[strings.Trim(imp.Path.Value, "\"")] = true
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		funcs++
		if ast.IsExported(fn.Name.Name) {
			exported++
		}
		name := strings.ToLower(fn.Name.Name)
		if strings.Contains(name, "handler") || strings.HasPrefix(name, "handle") || strings.HasPrefix(name, "serve") {
			handlers++
		}
		fnLines := fset.Position(fn.End()).Line - fset.Position(fn.Pos()).Line + 1
		if fnLines > 120 {
			out = append(out, Smell{Priority: "P1", File: info.Path, Line: fset.Position(fn.Pos()).Line, Score: fnLines, Reasons: []string{fmt.Sprintf("very large function: %d lines", fnLines)}, Advice: "use goctx slice for this symbol; avoid broad full-file reads"})
		}
	}
	if lines > 800 {
		out = append(out, Smell{Priority: "P1", File: info.Path, Score: lines, Reasons: []string{fmt.Sprintf("very large Go file: %d lines", lines)}, Advice: "inspect symbols first and slice specific functions"})
	}
	if handlers >= 10 {
		reasons := []string{fmt.Sprintf("%d handler-like functions", handlers)}
		if imports["database/sql"] || hasImportContaining(imports, "pgx") || hasImportContaining(imports, "gorm") {
			reasons = append(reasons, "handler-like file imports database package")
		}
		out = append(out, Smell{Priority: "P1", File: info.Path, Score: handlers * 20, Reasons: reasons, Advice: "use route map, symbols, and slices before reading full controller files"})
	}
	base := strings.ToLower(filepath.Base(filepath.Dir(info.Path)) + "/" + filepath.Base(info.Path))
	if (strings.Contains(base, "util") || strings.Contains(base, "helper") || strings.Contains(base, "common")) && exported > 40 {
		out = append(out, Smell{Priority: "P2", File: info.Path, Score: exported * 10, Reasons: []string{fmt.Sprintf("%d exported functions in util/helper/common-like file", exported)}, Advice: "inspect symbols only; names may be ambiguous"})
	}
	if funcs > 80 {
		out = append(out, Smell{Priority: "P2", File: info.Path, Score: funcs * 5, Reasons: []string{fmt.Sprintf("unusually high symbol count: %d functions", funcs)}, Advice: "use symbol-level context"})
	}
	return out
}

func hasImportContaining(imports map[string]bool, needle string) bool {
	for imp := range imports {
		if strings.Contains(strings.ToLower(imp), strings.ToLower(needle)) {
			return true
		}
	}
	return false
}
