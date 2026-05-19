package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"goctx/internal/codex"
	"goctx/internal/diffsummary"
	"goctx/internal/indexer"
	"goctx/internal/routes"
	"goctx/internal/scanner"
	"goctx/internal/slicer"
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
	switch args[0] {
	case "scan":
		return runScan(root, stdout, stderr)
	case "map":
		return runMap(root, stdout, stderr)
	case "symbols":
		if len(args) != 2 {
			return usageErr(stderr, "usage: goctx symbols <file>")
		}
		return runSymbols(args[1], stdout, stderr)
	case "slice":
		return runSlice(args[1:], stdout, stderr)
	case "task":
		if len(args) != 2 {
			return usageErr(stderr, "usage: goctx task \"<task description>\"")
		}
		return runTask(root, args[1], stdout, stderr)
	case "routes":
		return runRoutes(root, stdout, stderr)
	case "test-summary":
		return runTestSummary(args[1:], stdout, stderr)
	case "diff-summary":
		if err := diffsummary.Summarize(root, stdout); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		return 0
	case "token-report":
		sc, err := scanner.Scan(root)
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		tokens.Report(stdout, sc)
		return 0
	case "init-codex":
		force := len(args) == 2 && args[1] == "--force"
		if len(args) > 2 || (len(args) == 2 && !force) {
			return usageErr(stderr, "usage: goctx init-codex [--force]")
		}
		res, err := codex.Init(root, force)
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

func runScan(root string, stdout, stderr io.Writer) int {
	sc, err := scanner.Scan(root)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
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

func runMap(root string, stdout, stderr io.Writer) int {
	pkgs, err := indexer.LoadPackages(context.Background(), root)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	for _, p := range pkgs {
		fmt.Fprintf(stdout, "Package: %s\nDir: %s\n", p.ImportPath, p.Dir)
		writeList(stdout, "Files:", baseList(p.Files))
		writeList(stdout, "Tests:", baseList(p.TestFiles))
		fmt.Fprintln(stdout, "Symbols:")
		limit := 40
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

func runTask(root, task string, stdout, stderr io.Writer) int {
	pkgs, err := indexer.LoadPackages(context.Background(), root)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	sc, err := scanner.Scan(root)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	taskpack.Write(stdout, taskpack.Rank(task, pkgs, sc))
	return 0
}

func runRoutes(root string, stdout, stderr io.Writer) int {
	res, err := routes.Extract(root)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	fmt.Fprintln(stdout, "Routes:")
	for _, r := range res.Routes {
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
