package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"goctx/internal/baseline"
	"goctx/internal/codex"
	"goctx/internal/codexpack"
	"goctx/internal/diffsummary"
	"goctx/internal/discovery"
	"goctx/internal/indexer"
	"goctx/internal/output"
	"goctx/internal/routes"
	"goctx/internal/scanner"
	"goctx/internal/slicer"
	"goctx/internal/smells"
	"goctx/internal/taskpack"
	"goctx/internal/testsum"
	"goctx/internal/tokens"
)

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(stdout, Usage)
		return 0
	}
	root, _ := os.Getwd()
	opts := output.DefaultOptions(root)
	switch args[0] {
	case "scan":
		rest, err := output.ParseCommon(args[1:], &opts)
		if err != nil || len(rest) != 0 {
			return usageErr(stderr, errOrUsage(err, "usage: goctx scan [--root path] [--budget n] [--format text|json|markdown]"))
		}
		return runScan(opts, stdout, stderr)
	case "discover":
		rest, err := output.ParseCommon(args[1:], &opts)
		if err != nil || len(rest) != 0 {
			return usageErr(stderr, errOrUsage(err, "usage: goctx discover [--root path] [--format text|json]"))
		}
		return runDiscover(opts, stdout, stderr)
	case "map":
		rest, err := output.ParseCommon(args[1:], &opts)
		if err != nil || len(rest) != 0 {
			return usageErr(stderr, errOrUsage(err, "usage: goctx map [--root path] [--module module] [--budget n] [--format text|json|markdown]"))
		}
		return runMap(opts, stdout, stderr)
	case "symbols":
		if len(args) != 2 {
			return usageErr(stderr, "usage: goctx symbols <file>")
		}
		return runSymbols(args[1], stdout, stderr)
	case "slice":
		return runSlice(args[1:], stdout, stderr)
	case "task":
		rest, err := output.ParseCommon(args[1:], &opts)
		if err != nil || len(rest) != 1 {
			return usageErr(stderr, errOrUsage(err, "usage: goctx task [flags] \"<task description>\""))
		}
		return runTask(opts, rest[0], stdout, stderr)
	case "codex-pack":
		rest, err := output.ParseCommon(args[1:], &opts)
		if err != nil || len(rest) != 1 {
			return usageErr(stderr, errOrUsage(err, "usage: goctx codex-pack [flags] \"<task description>\""))
		}
		return runCodexPack(opts, rest[0], stdout, stderr)
	case "routes":
		rest, err := output.ParseCommon(args[1:], &opts)
		if err != nil || len(rest) != 0 {
			return usageErr(stderr, errOrUsage(err, "usage: goctx routes [flags]"))
		}
		return runRoutes(opts, stdout, stderr)
	case "test-summary":
		return runTestSummary(args[1:], stdout, stderr)
	case "diff-summary":
		rest, err := output.ParseCommon(args[1:], &opts)
		if err != nil || len(rest) != 0 {
			return usageErr(stderr, errOrUsage(err, "usage: goctx diff-summary [--root path]"))
		}
		if err := diffsummary.Summarize(opts.Root, stdout); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		return 0
	case "baseline":
		return runBaseline(root, args[1:], stdout, stderr)
	case "token-report":
		rest, err := output.ParseCommon(args[1:], &opts)
		if err != nil || len(rest) != 0 {
			return usageErr(stderr, errOrUsage(err, "usage: goctx token-report [flags]"))
		}
		return runTokenReport(opts, stdout, stderr)
	case "doctor":
		rest, err := output.ParseCommon(args[1:], &opts)
		if err != nil || len(rest) != 0 {
			return usageErr(stderr, errOrUsage(err, "usage: goctx doctor [--root path] [--module module] [--format text|json]"))
		}
		return runDoctor(opts, stdout, stderr)
	case "smells":
		rest, err := output.ParseCommon(args[1:], &opts)
		if err != nil || len(rest) != 0 {
			return usageErr(stderr, errOrUsage(err, "usage: goctx smells [flags]"))
		}
		return runSmells(opts, stdout, stderr)
	case "init-codex":
		rest, err := output.ParseCommon(args[1:], &opts)
		if err != nil || len(rest) != 0 {
			return usageErr(stderr, errOrUsage(err, "usage: goctx init-codex [--force] [--strict]"))
		}
		res, err := codex.InitWithOptions(root, opts.Force, opts.Strict)
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		fmt.Fprint(stdout, codex.FormatResults(res))
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n%s", args[0], Usage)
		return 2
	}
}

func runScan(opts output.Options, stdout, stderr io.Writer) int {
	sc, err := scanner.Scan(opts.Root)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if opts.Format == output.FormatJSON {
		env := output.Envelope{Command: "scan", Root: opts.Root, BudgetEstimatedTokens: opts.Budget, Confidence: "high"}
		env.Files, env.Omitted, env.ActualEstimatedTokens = budgetedFileItems(sc.Files, opts, false)
		_ = output.WriteJSON(stdout, env)
		return 0
	}
	counts := map[scanner.FileKind]int{}
	ignored := 0
	for _, f := range sc.Files {
		counts[f.Kind]++
		if f.IgnoredByDefault {
			ignored++
		}
	}
	fmt.Fprintln(stdout, "Repository scan:")
	fmt.Fprintf(stdout, "- Go source files: %d\n", counts[scanner.KindGoSource])
	fmt.Fprintf(stdout, "- Go test files: %d\n", counts[scanner.KindGoTest])
	fmt.Fprintf(stdout, "- Ignored generated files: %d\n", counts[scanner.KindGenerated])
	fmt.Fprintf(stdout, "- Ignored files/dirs by default: %d\n\n", ignored)
	fmt.Fprintln(stdout, "Ignored by default:")
	n := 0
	for _, f := range sc.Files {
		if f.IgnoredByDefault {
			fmt.Fprintf(stdout, "- %s: %s\n", f.Path, f.Reason)
			n++
			if n >= 25 {
				fmt.Fprintln(stdout, "- ...")
				break
			}
		}
	}
	return 0
}

func runDiscover(opts output.Options, stdout, stderr io.Writer) int {
	res, err := discovery.Discover(opts.Root)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if opts.Format == output.FormatJSON {
		_ = discovery.WriteJSON(stdout, res)
		return 0
	}
	discovery.WriteText(stdout, res)
	return 0
}

func runMap(opts output.Options, stdout, stderr io.Writer) int {
	root, disc, _ := discovery.SelectRoot(opts.Root, opts.Module)
	idx, err := indexer.LoadPackagesFlexible(context.Background(), root)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if opts.Format == output.FormatJSON {
		env := output.Envelope{Command: "map", Root: root, Module: disc.Selected, BudgetEstimatedTokens: opts.Budget, Confidence: idx.Confidence, Warnings: idx.Warnings}
		for _, p := range idx.Packages {
			for _, f := range p.Files {
				env.Files = append(env.Files, output.FileItem{Path: f, Kind: "go"})
			}
			for _, s := range p.Symbols {
				env.Symbols = append(env.Symbols, output.SymbolItem{Name: indexer.DisplayName(s), Kind: string(s.Kind), File: s.File, Line: s.StartLine, Receiver: s.Receiver})
			}
		}
		env.ActualEstimatedTokens = int64(len(env.Files)*20 + len(env.Symbols)*15 + 100)
		_ = output.WriteJSON(stdout, env)
		return 0
	}
	if len(idx.Warnings) > 0 {
		fmt.Fprintf(stdout, "Confidence: %s\n", idx.Confidence)
		for _, w := range idx.Warnings {
			fmt.Fprintf(stdout, "Warning: %s\n", w)
		}
		fmt.Fprintln(stdout)
	}
	for _, p := range idx.Packages {
		fmt.Fprintf(stdout, "Package: %s\nDir: %s\n", p.ImportPath, p.Dir)
		writeList(stdout, "Files:", baseList(p.Files))
		writeList(stdout, "Tests:", baseList(p.TestFiles))
		fmt.Fprintln(stdout, "Symbols:")
		limit := opts.MaxSymbols
		for i, s := range p.Symbols {
			if i >= limit {
				fmt.Fprintln(stdout, "- ...")
				break
			}
			fmt.Fprintf(stdout, "- %s %s\n", s.Kind, indexer.DisplayName(s))
		}
		fmt.Fprintln(stdout)
	}
	return 0
}

func runSymbols(path string, stdout, stderr io.Writer) int {
	syms, err := indexer.SymbolsInFile(path)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	for _, s := range syms {
		fmt.Fprintf(stdout, "%s %s %s:%d-%d\n", s.Kind, indexer.DisplayName(s), filepath.ToSlash(path), s.StartLine, s.EndLine)
	}
	return 0
}

func runSlice(args []string, stdout, stderr io.Writer) int {
	if len(args) != 3 || args[1] != "--symbol" {
		return usageErr(stderr, "usage: goctx slice <file> --symbol <name>")
	}
	res, err := slicer.SliceFile(args[0], args[2])
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if res.NotFound {
		fmt.Fprintf(stderr, "symbol %q not found. Available symbols:\n", args[2])
		for _, s := range res.Available {
			fmt.Fprintf(stderr, "- %s at line %d\n", indexer.DisplayName(s), s.StartLine)
		}
		return 1
	}
	if res.Ambiguous {
		fmt.Fprintf(stderr, "symbol %q is ambiguous:\n", args[2])
		for _, s := range res.Matches {
			fmt.Fprintf(stderr, "- %s at line %d\n", indexer.DisplayName(s), s.StartLine)
		}
		return 1
	}
	fmt.Fprint(stdout, res.Source)
	return 0
}

func runTask(opts output.Options, task string, stdout, stderr io.Writer) int {
	root, disc, _ := discovery.SelectRoot(opts.Root, opts.Module)
	idx, _ := indexer.LoadPackagesFlexible(context.Background(), root)
	sc, err := scanner.Scan(root)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	res := taskpack.Rank(task, idx.Packages, sc)
	res.Warnings = append(res.Warnings, idx.Warnings...)
	res.Confidence = idx.Confidence
	if opts.Format == output.FormatJSON {
		env := taskEnvelope("task", root, disc.Selected, res, opts)
		env.Warnings = append(env.Warnings, idx.Warnings...)
		_ = output.WriteJSON(stdout, env)
		return 0
	}
	taskpack.Write(stdout, res)
	return 0
}

func runCodexPack(opts output.Options, task string, stdout, stderr io.Writer) int {
	res, err := codexpack.Build(context.Background(), task, opts)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if opts.Format == output.FormatJSON {
		_ = output.WriteJSON(stdout, codexpack.Envelope("codex-pack", res, opts))
		return 0
	}
	codexpack.WriteText(stdout, res, opts)
	return 0
}

func runRoutes(opts output.Options, stdout, stderr io.Writer) int {
	root, disc, _ := discovery.SelectRoot(opts.Root, opts.Module)
	res, err := routes.Extract(root)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if opts.Format == output.FormatJSON {
		env := output.Envelope{Command: "routes", Root: root, Module: disc.Selected, BudgetEstimatedTokens: opts.Budget, Confidence: "medium"}
		for i, r := range res.Routes {
			if i >= opts.MaxRoutes {
				env.Omitted = append(env.Omitted, output.Omission{Kind: "routes", Count: len(res.Routes) - opts.MaxRoutes, Reason: "max routes limit"})
				break
			}
			env.Routes = append(env.Routes, output.RouteItem{Method: r.Method, Path: r.Path, File: r.File, Line: r.Line, Handler: r.Handler, Dynamic: r.Dynamic})
		}
		env.ActualEstimatedTokens = int64(len(env.Routes)*30 + 100)
		env.Data = map[string]any{"wrapper_candidates": res.WrapperCandidates}
		_ = output.WriteJSON(stdout, env)
		return 0
	}
	fmt.Fprintln(stdout, "Routes:")
	for i, r := range res.Routes {
		if i >= opts.MaxRoutes {
			fmt.Fprintf(stdout, "- omitted %d routes due to max-routes\n", len(res.Routes)-opts.MaxRoutes)
			break
		}
		dyn := ""
		if r.Dynamic {
			dyn = " dynamic"
		}
		fmt.Fprintf(stdout, "%-6s %-24s %s:%d %s%s\n", r.Method, r.Path, r.File, r.Line, r.Handler, dyn)
	}
	if len(res.Middleware) > 0 {
		fmt.Fprintln(stdout, "\nMiddleware/use calls:")
		for _, m := range res.Middleware {
			fmt.Fprintf(stdout, "- %s at %s:%d\n", m.Expr, m.File, m.Line)
		}
	}
	if len(res.WrapperCandidates) > 0 {
		fmt.Fprintln(stdout, "\nPossible route wrappers:")
		for _, c := range res.WrapperCandidates {
			fmt.Fprintf(stdout, "- %s at %s:%d (%s)\n", c.Name, c.File, c.Line, c.Reason)
		}
	}
	return 0
}

func runTestSummary(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "--" {
		return usageErr(stderr, "usage: goctx test-summary -- <go command and args>")
	}
	sum, err := testsum.Run(context.Background(), args[1:])
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return sum.ExitCode
	}
	fmt.Fprint(stdout, sum.Output)
	return sum.ExitCode
}

func runBaseline(root string, args []string, stdout, stderr io.Writer) int {
	opts := output.DefaultOptions(root)
	rest, err := output.ParseCommon(args, &opts)
	if err != nil || len(rest) != 1 {
		return usageErr(stderr, errOrUsage(err, "usage: goctx baseline create|update|status [--root path]"))
	}
	switch rest[0] {
	case "create", "update":
		s, err := baseline.Create(opts.Root)
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		if err := baseline.Write(opts.Root, s); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		fmt.Fprintf(stdout, "OK   baseline %s written: .goctx/baseline.json\nFiles: %d\n", rest[0], len(s.Files))
	case "status":
		if err := baseline.Status(opts.Root, stdout); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
	default:
		return usageErr(stderr, "usage: goctx baseline create|update|status [--root path]")
	}
	return 0
}

func runTokenReport(opts output.Options, stdout, stderr io.Writer) int {
	sc, err := scanner.Scan(opts.Root)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if opts.Format == output.FormatJSON {
		env := output.Envelope{Command: "token-report", Root: opts.Root, BudgetEstimatedTokens: opts.Budget, Confidence: "high"}
		env.Files, env.Omitted, env.ActualEstimatedTokens = budgetedFileItems(sc.Files, opts, true)
		_ = output.WriteJSON(stdout, env)
		return 0
	}
	tokens.Report(stdout, sc)
	return 0
}

func runDoctor(opts output.Options, stdout, stderr io.Writer) int {
	root, disc, _ := discovery.SelectRoot(opts.Root, opts.Module)
	idx, _ := indexer.LoadPackagesFlexible(context.Background(), root)
	env := output.Envelope{Command: "doctor", Root: root, Module: disc.Selected, Confidence: idx.Confidence, Warnings: append(disc.Warnings, idx.Warnings...)}
	checks := []string{}
	checks = append(checks, "OK   current directory: "+opts.Root)
	if len(disc.Modules) > 0 {
		checks = append(checks, "OK   Go module discovered: "+disc.Selected)
	} else {
		checks = append(checks, "WARN no Go module discovered")
	}
	if idx.Confidence == "high" {
		checks = append(checks, "OK   package load succeeded")
	} else {
		checks = append(checks, "WARN package loading degraded; fallback available")
	}
	if _, err := os.Stat(filepath.Join(root, ".goctx", "baseline.json")); err == nil {
		checks = append(checks, "OK   .goctx/baseline.json found")
	} else {
		checks = append(checks, "WARN .goctx/baseline.json missing")
	}
	for _, p := range []string{"AGENTS.md", ".agents/skills/go-token-efficient/SKILL.md", ".codex/GOCTX_HOOKS.md"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(p))); err == nil {
			checks = append(checks, "OK   "+p+" found")
		} else {
			checks = append(checks, "WARN "+p+" missing")
		}
	}
	env.Data = map[string]any{"checks": checks}
	env.ActualEstimatedTokens = int64(len(checks)*20 + 80)
	if opts.Format == output.FormatJSON {
		_ = output.WriteJSON(stdout, env)
		return 0
	}
	for _, c := range checks {
		fmt.Fprintln(stdout, c)
	}
	return 0
}

func runSmells(opts output.Options, stdout, stderr io.Writer) int {
	root, _, _ := discovery.SelectRoot(opts.Root, opts.Module)
	opts.Root = root
	res, err := smells.Analyze(root)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if opts.Format == output.FormatJSON {
		_ = output.WriteJSON(stdout, smells.Envelope(res, opts))
		return 0
	}
	smells.WriteText(stdout, res, opts)
	return 0
}

func usageErr(stderr io.Writer, msg string) int {
	fmt.Fprintln(stderr, msg)
	return 2
}

func writeList(w io.Writer, title string, xs []string) {
	fmt.Fprintln(w, title)
	if len(xs) == 0 {
		fmt.Fprintln(w, "- none")
		return
	}
	for _, x := range xs {
		fmt.Fprintf(w, "- %s\n", x)
	}
}

func baseList(xs []string) []string {
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		out = append(out, filepath.Base(x))
	}
	return out
}

func errOrUsage(err error, usage string) string {
	if err != nil {
		return err.Error()
	}
	return usage
}

func reasonSlice(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}

func budgetedFileItems(files []scanner.FileInfo, opts output.Options, largestFirst bool) ([]output.FileItem, []output.Omission, int64) {
	cp := append([]scanner.FileInfo(nil), files...)
	if largestFirst {
		sort.Slice(cp, func(i, j int) bool { return cp[i].SizeBytes > cp[j].SizeBytes })
	}
	var out []output.FileItem
	var used int64 = 100
	omitted := 0
	limit := opts.MaxFiles
	if largestFirst && limit < 20 {
		limit = 20
	}
	for _, f := range cp {
		if f.SizeBytes == 0 {
			continue
		}
		next := int64(35)
		if len(out) >= limit || (opts.Budget > 0 && len(out) > 0 && used+next > opts.Budget) {
			omitted++
			continue
		}
		out = append(out, output.FileItem{Path: f.Path, Kind: string(f.Kind), SizeBytes: f.SizeBytes, EstimatedTokens: tokens.Estimate(f.SizeBytes), Reasons: reasonSlice(f.Reason)})
		used += next
	}
	var omissions []output.Omission
	if omitted > 0 {
		omissions = append(omissions, output.Omission{Kind: "files", Count: omitted, Reason: "budget or max files limit"})
	}
	return out, omissions, used
}

func taskEnvelope(command, root, module string, res taskpack.Result, opts output.Options) output.Envelope {
	env := output.Envelope{Command: command, Root: root, Module: module, BudgetEstimatedTokens: opts.Budget, Confidence: res.Confidence, Warnings: res.Warnings, Tests: res.Tests, ReadPlan: res.ReadPlan}
	for i, f := range res.Files {
		if i >= opts.MaxFiles {
			env.Omitted = append(env.Omitted, output.Omission{Kind: "files", Count: len(res.Files) - opts.MaxFiles, Reason: "max files limit"})
			break
		}
		env.Files = append(env.Files, output.FileItem{Path: f.Name, Score: f.Score, Reasons: f.Reasons})
	}
	for i, s := range res.Symbols {
		if i >= opts.MaxSymbols {
			env.Omitted = append(env.Omitted, output.Omission{Kind: "symbols", Count: len(res.Symbols) - opts.MaxSymbols, Reason: "max symbols limit"})
			break
		}
		env.Symbols = append(env.Symbols, output.SymbolItem{Name: s.Name, Score: s.Score, Reasons: s.Reasons})
	}
	env.ActualEstimatedTokens = int64(len(env.Files)*80 + len(env.Symbols)*20 + len(env.Tests)*15 + len(env.ReadPlan)*20 + 120)
	env.Data = map[string]any{"task": res.Task, "avoid": res.Avoid, "savings": res.Savings}
	return env
}
