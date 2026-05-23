package codexpack

import (
	"context"
	"fmt"
	"io"
	"strings"

	"goctx/internal/discovery"
	"goctx/internal/indexer"
	"goctx/internal/output"
	"goctx/internal/routes"
	"goctx/internal/scanner"
	"goctx/internal/taskpack"
)

type Result struct {
	Task       string
	Discovery  discovery.Result
	Index      indexer.LoadResult
	Scan       scanner.Result
	TaskPack   taskpack.Result
	Routes     routes.Result
	Frameworks []string
	Hints      []string
}

func Build(ctx context.Context, task string, opts output.Options) (Result, error) {
	root, disc, err := discovery.SelectRoot(opts.Root, opts.Module)
	if err != nil {
		return Result{}, err
	}
	sc, err := scanner.Scan(root)
	if err != nil {
		return Result{}, err
	}
	idx, _ := indexer.LoadPackagesFlexible(ctx, root)
	tp := taskpack.Rank(task, idx.Packages, sc)
	tp.Warnings = append(tp.Warnings, idx.Warnings...)
	tp.Confidence = idx.Confidence
	rt, _ := routes.Extract(root)
	return Result{Task: task, Discovery: disc, Index: idx, Scan: sc, TaskPack: tp, Routes: rt, Frameworks: detectFrameworks(idx.Packages), Hints: detectHints(idx.Packages)}, nil
}

func WriteText(w io.Writer, r Result, opts output.Options) {
	var lines []string
	lines = append(lines, "Task Context Pack")
	lines = append(lines, fmt.Sprintf("Task: %s", r.Task))
	lines = append(lines, fmt.Sprintf("Budget: %d estimated tokens", opts.Budget))
	lines = append(lines, fmt.Sprintf("Confidence: %s", r.Index.Confidence))
	lines = append(lines, "")
	lines = append(lines, "Repo shape:")
	if r.Discovery.Selected != "" {
		lines = append(lines, "- module: "+r.Discovery.Selected)
	}
	if len(r.Frameworks) > 0 {
		lines = append(lines, "- framework hints: "+strings.Join(r.Frameworks, ", "))
	}
	if len(r.Hints) > 0 {
		lines = append(lines, "- code hints: "+strings.Join(r.Hints, ", "))
	}
	lines = append(lines, "")
	lines = append(lines, "Likely files:")
	for i, f := range limitRanked(r.TaskPack.Files, opts.MaxFiles) {
		lines = append(lines, fmt.Sprintf("%d. %s - score %d", i+1, f.Name, f.Score))
		for _, reason := range f.Reasons {
			lines = append(lines, "   reason: "+reason)
		}
	}
	lines = append(lines, "")
	lines = append(lines, "Likely symbols:")
	for _, s := range limitRanked(r.TaskPack.Symbols, opts.MaxSymbols) {
		lines = append(lines, "- "+s.Name)
	}
	lines = append(lines, "")
	lines = append(lines, "Relevant routes:")
	for i, rt := range r.Routes.Routes {
		if i >= opts.MaxRoutes {
			break
		}
		lines = append(lines, fmt.Sprintf("- %s %s %s:%d %s", rt.Method, rt.Path, rt.File, rt.Line, rt.Handler))
	}
	lines = append(lines, "")
	lines = append(lines, "Recommended read plan:")
	for _, cmd := range r.TaskPack.ReadPlan {
		lines = append(lines, "- "+cmd)
	}
	lines = append(lines, "")
	lines = append(lines, "Suggested focused tests:")
	for _, t := range r.TaskPack.Tests {
		lines = append(lines, "- "+t)
	}
	lines = append(lines, "")
	lines = append(lines, "Avoid unless required:")
	for _, a := range r.TaskPack.Avoid {
		lines = append(lines, "- "+a)
	}
	lines = append(lines, "")
	lines = append(lines, "Estimated context reduction:")
	lines = append(lines, fmt.Sprintf("- naive context: %d estimated tokens", r.TaskPack.Savings.NaiveEstimatedTokens))
	lines = append(lines, fmt.Sprintf("- goctx recommended context: %d estimated tokens", r.TaskPack.Savings.RecommendedEstimatedTokens))
	lines = append(lines, fmt.Sprintf("- estimated reduction: %.1f%% less context", r.TaskPack.Savings.ReductionPercent))
	out, omitted, used := output.TruncateLines(lines, opts.Budget)
	for _, l := range out {
		fmt.Fprintln(w, l)
	}
	fmt.Fprintf(w, "\nActual: %d estimated tokens\n", used)
	if omitted > 0 {
		fmt.Fprintf(w, "Omitted:\n- %d low-priority output lines due to budget\n", omitted)
	}
}

func Envelope(command string, r Result, opts output.Options) output.Envelope {
	env := output.Envelope{Command: command, Root: r.Scan.Root, Module: r.Discovery.Selected, BudgetEstimatedTokens: opts.Budget, Confidence: r.Index.Confidence, Warnings: append(r.Discovery.Warnings, r.Index.Warnings...), Tests: r.TaskPack.Tests, ReadPlan: r.TaskPack.ReadPlan}
	for _, f := range limitRanked(r.TaskPack.Files, opts.MaxFiles) {
		env.Files = append(env.Files, output.FileItem{Path: f.Name, Score: f.Score, Reasons: f.Reasons})
	}
	if opts.MaxFiles > 0 && len(r.TaskPack.Files) > opts.MaxFiles {
		env.Omitted = append(env.Omitted, output.Omission{Kind: "files", Count: len(r.TaskPack.Files) - opts.MaxFiles, Reason: "max files limit"})
	}
	for _, s := range limitRanked(r.TaskPack.Symbols, opts.MaxSymbols) {
		env.Symbols = append(env.Symbols, output.SymbolItem{Name: s.Name, Score: s.Score, Reasons: s.Reasons})
	}
	if opts.MaxSymbols > 0 && len(r.TaskPack.Symbols) > opts.MaxSymbols {
		env.Omitted = append(env.Omitted, output.Omission{Kind: "symbols", Count: len(r.TaskPack.Symbols) - opts.MaxSymbols, Reason: "max symbols limit"})
	}
	for i, rt := range r.Routes.Routes {
		if i >= opts.MaxRoutes {
			env.Omitted = append(env.Omitted, output.Omission{Kind: "routes", Count: len(r.Routes.Routes) - opts.MaxRoutes, Reason: "max routes limit"})
			break
		}
		env.Routes = append(env.Routes, output.RouteItem{Method: rt.Method, Path: rt.Path, File: rt.File, Line: rt.Line, Handler: rt.Handler, Dynamic: rt.Dynamic})
	}
	env.Data = map[string]any{"task": r.Task, "framework_hints": r.Frameworks, "hints": r.Hints, "savings": r.TaskPack.Savings, "index_mode": r.Index.Mode}
	env.ActualEstimatedTokens = int64(len(env.Files)*80 + len(env.Symbols)*20 + len(env.Routes)*30 + len(env.ReadPlan)*20 + 200)
	return env
}

func detectFrameworks(pkgs []indexer.PackageInfo) []string {
	return detectImports(pkgs, map[string]string{"github.com/labstack/echo": "Echo", "github.com/gin-gonic/gin": "Gin", "github.com/go-chi/chi": "Chi", "net/http": "net/http"})
}

func detectHints(pkgs []indexer.PackageInfo) []string {
	return detectImports(pkgs, map[string]string{"firebase": "Firebase-like auth", "jwt": "JWT-like auth", "database/sql": "SQL database", "pgx": "Postgres-like database", "supabase": "Supabase-like database"})
}

func detectImports(pkgs []indexer.PackageInfo, needles map[string]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range pkgs {
		for _, imp := range p.Imports {
			for needle, label := range needles {
				if strings.Contains(strings.ToLower(imp), strings.ToLower(needle)) && !seen[label] {
					seen[label] = true
					out = append(out, label)
				}
			}
		}
	}
	return out
}

func limitRanked(xs []taskpack.RankedItem, n int) []taskpack.RankedItem {
	if n <= 0 || len(xs) <= n {
		return xs
	}
	return xs[:n]
}
