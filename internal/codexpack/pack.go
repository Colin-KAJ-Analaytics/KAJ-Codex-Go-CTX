package codexpack

import (
	"context"
	"fmt"
	"io"
	"sort"
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
	idx, err := indexer.LoadPackagesFlexible(ctx, root)
	if err != nil {
		return Result{}, err
	}
	tp := taskpack.Rank(task, idx.Packages, sc)
	tp.Warnings = append(tp.Warnings, idx.Warnings...)
	tp.Confidence = idx.Confidence
	rt, err := routes.Extract(root)
	if err != nil {
		return Result{}, err
	}
	return Result{Task: task, Discovery: disc, Index: idx, Scan: sc, TaskPack: tp, Routes: rt, Frameworks: detectFrameworks(idx.Packages), Hints: detectHints(idx.Packages)}, nil
}

func WriteText(w io.Writer, r Result, opts output.Options) {
	header := fmt.Sprintf("Task Context Pack\nTask: %s\nConfidence: %s\n", r.Task, r.Index.Confidence)
	for _, warning := range append(append([]string(nil), r.Discovery.Warnings...), r.Index.Warnings...) {
		header += "Warning: " + warning + "\n"
	}
	limited := 0
	for _, limit := range []struct{ count, max int }{{len(r.TaskPack.Files), opts.MaxFiles}, {len(r.TaskPack.Symbols), opts.MaxSymbols}, {len(relevantRoutes(r)), opts.MaxRoutes}} {
		if limit.max > 0 && limit.count > limit.max {
			limited += limit.count - limit.max
		}
	}
	if limited > 0 {
		header += fmt.Sprintf("Omitted by item limits: %d context items.\n", limited)
	}
	var items []output.TextItem
	add := func(section, text string, priority int) {
		items = append(items, output.TextItem{Section: section, Text: text, Priority: priority})
	}
	for i, cmd := range r.TaskPack.ReadPlan {
		add("Recommended read plan", "- "+cmd, 105-i)
	}
	for i, test := range r.TaskPack.Tests {
		add("Suggested focused tests", "- "+test, 110-i)
	}
	for i, f := range limitRanked(r.TaskPack.Files, opts.MaxFiles) {
		text := "- " + f.Name
		if !opts.Quiet {
			text += fmt.Sprintf(" - score %d", f.Score)
			for _, reason := range f.Reasons {
				text += "\n  reason: " + reason
			}
		}
		add("Likely files", text, 90-i)
	}
	for i, sym := range limitRanked(r.TaskPack.Symbols, opts.MaxSymbols) {
		text := "- " + sym.Name
		if sym.File != "" {
			text += fmt.Sprintf(" %s:%d", sym.File, sym.Line)
		}
		add("Likely symbols", text, 85-i)
	}
	for i, rt := range relevantRoutes(r) {
		if opts.MaxRoutes > 0 && i >= opts.MaxRoutes {
			break
		}
		add("Relevant routes", fmt.Sprintf("- %s %s %s:%d %s", rt.Method, rt.Path, rt.File, rt.Line, rt.Handler), 80-i)
	}
	if r.Discovery.Selected != "" {
		add("Repo shape", "- module: "+r.Discovery.Selected, 30)
	}
	if len(r.Frameworks) > 0 {
		add("Repo shape", "- framework hints: "+strings.Join(r.Frameworks, ", "), 30)
	}
	if len(r.Hints) > 0 {
		add("Repo shape", "- code hints: "+strings.Join(r.Hints, ", "), 30)
	}
	if !opts.Quiet {
		for _, avoid := range r.TaskPack.Avoid {
			add("Avoid unless required", "- "+avoid, 5)
		}
		add("Estimated context reduction (rough heuristic, not measured task savings)", fmt.Sprintf("- naive: %d; recommended: %d estimated tokens; reduction: %.1f%%", r.TaskPack.Savings.NaiveEstimatedTokens, r.TaskPack.Savings.RecommendedEstimatedTokens, r.TaskPack.Savings.ReductionPercent), 1)
	}
	output.WriteTextItems(w, header, items, opts.Budget)
}

func Envelope(command string, r Result, opts output.Options) output.Envelope {
	env := output.Envelope{Command: command, Root: r.Scan.Root, Module: r.Discovery.Selected, BudgetEstimatedTokens: opts.Budget, Confidence: r.Index.Confidence, Warnings: append(append([]string(nil), r.Discovery.Warnings...), r.Index.Warnings...), Tests: r.TaskPack.Tests, ReadPlan: r.TaskPack.ReadPlan}
	for _, f := range r.TaskPack.Files {
		env.Files = append(env.Files, output.FileItem{Path: f.Name, Score: f.Score, Reasons: f.Reasons})
	}
	for _, s := range r.TaskPack.Symbols {
		env.Symbols = append(env.Symbols, output.SymbolItem{Name: s.Name, File: s.File, Line: s.Line, Score: s.Score, Reasons: s.Reasons})
	}
	env.Routes = relevantRoutes(r)
	env.Data = map[string]any{"task": r.Task, "framework_hints": r.Frameworks, "hints": r.Hints, "savings": r.TaskPack.Savings, "index_mode": r.Index.Mode}
	output.ApplyBudget(&env, opts)
	return env
}

// relevantRoutes scores before limits are applied, retaining only task matches.
func relevantRoutes(r Result) []output.RouteItem {
	terms := taskpack.Tokenize(r.Task)
	var out []output.RouteItem
	for _, rt := range r.Routes.Routes {
		text := strings.ToLower(rt.Path + " " + rt.Handler + " " + rt.File + " " + rt.Method)
		score := 0
		for _, term := range terms {
			if strings.Contains(text, term) {
				score += 10
			}
		}
		if score > 0 {
			out = append(out, output.RouteItem{Method: rt.Method, Path: rt.Path, File: rt.File, Line: rt.Line, Handler: rt.Handler, Dynamic: rt.Dynamic, Score: score})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Method < out[j].Method
	})
	return out
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
	sort.Strings(out)
	return out
}

func limitRanked(xs []taskpack.RankedItem, n int) []taskpack.RankedItem {
	if n <= 0 || len(xs) <= n {
		return xs
	}
	return xs[:n]
}
