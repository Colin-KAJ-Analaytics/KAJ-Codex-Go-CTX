package output

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"goctx/internal/tokens"
)

type Format string

const (
	FormatText     Format = "text"
	FormatJSON     Format = "json"
	FormatMarkdown Format = "markdown"
)

type Options struct {
	Root       string
	Module     string
	AllModules bool
	Budget     int64
	MaxFiles   int
	MaxSymbols int
	MaxRoutes  int
	Format     Format
	Force      bool
	Strict     bool
}

type Omission struct {
	Kind   string `json:"kind"`
	Count  int    `json:"count"`
	Reason string `json:"reason"`
}

type FileItem struct {
	Path            string   `json:"path"`
	Kind            string   `json:"kind,omitempty"`
	SizeBytes       int64    `json:"size_bytes,omitempty"`
	EstimatedTokens int64    `json:"estimated_tokens,omitempty"`
	Score           int      `json:"score,omitempty"`
	Reasons         []string `json:"reasons,omitempty"`
	Line            int      `json:"line,omitempty"`
}

type SymbolItem struct {
	Name     string   `json:"name"`
	Kind     string   `json:"kind,omitempty"`
	File     string   `json:"file,omitempty"`
	Line     int      `json:"line,omitempty"`
	Score    int      `json:"score,omitempty"`
	Reasons  []string `json:"reasons,omitempty"`
	Receiver string   `json:"receiver,omitempty"`
}

type RouteItem struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Handler string `json:"handler,omitempty"`
	Dynamic bool   `json:"dynamic,omitempty"`
}

type Envelope struct {
	Command               string       `json:"command"`
	Root                  string       `json:"root,omitempty"`
	Module                string       `json:"module,omitempty"`
	BudgetEstimatedTokens int64        `json:"budget_estimated_tokens,omitempty"`
	ActualEstimatedTokens int64        `json:"actual_estimated_tokens"`
	Warnings              []string     `json:"warnings"`
	Omitted               []Omission   `json:"omitted"`
	Files                 []FileItem   `json:"files"`
	Symbols               []SymbolItem `json:"symbols"`
	Routes                []RouteItem  `json:"routes"`
	Tests                 []string     `json:"tests"`
	ReadPlan              []string     `json:"read_plan"`
	Confidence            string       `json:"confidence,omitempty"`
	Data                  any          `json:"data,omitempty"`
}

func DefaultOptions(root string) Options {
	return Options{Root: root, Budget: 2000, MaxFiles: 20, MaxSymbols: 40, MaxRoutes: 40, Format: FormatText}
}

func ParseCommon(args []string, opts *Options) ([]string, error) {
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--root":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--root requires a path")
			}
			opts.Root = args[i]
		case "--module":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--module requires a path or module name")
			}
			opts.Module = args[i]
		case "--all-modules":
			opts.AllModules = true
		case "--budget":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--budget requires a number")
			}
			n, err := strconv.ParseInt(args[i], 10, 64)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("--budget must be a positive integer")
			}
			opts.Budget = n
		case "--max-files":
			i++
			n, err := parsePositive(args, i, "--max-files")
			if err != nil {
				return nil, err
			}
			opts.MaxFiles = n
		case "--max-symbols":
			i++
			n, err := parsePositive(args, i, "--max-symbols")
			if err != nil {
				return nil, err
			}
			opts.MaxSymbols = n
		case "--max-routes":
			i++
			n, err := parsePositive(args, i, "--max-routes")
			if err != nil {
				return nil, err
			}
			opts.MaxRoutes = n
		case "--format":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--format requires text, json, or markdown")
			}
			f := Format(args[i])
			if f != FormatText && f != FormatJSON && f != FormatMarkdown {
				return nil, fmt.Errorf("invalid --format %q: use text, json, or markdown", args[i])
			}
			opts.Format = f
		case "--force":
			opts.Force = true
		case "--strict":
			opts.Strict = true
		default:
			rest = append(rest, a)
		}
	}
	return rest, nil
}

func parsePositive(args []string, i int, name string) (int, error) {
	if i >= len(args) {
		return 0, fmt.Errorf("%s requires a number", name)
	}
	n, err := strconv.Atoi(args[i])
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return n, nil
}

func EstimateText(s string) int64 {
	return tokens.Estimate(int64(len(s)))
}

func WriteJSON(w io.Writer, env Envelope) error {
	if env.Warnings == nil {
		env.Warnings = []string{}
	}
	if env.Omitted == nil {
		env.Omitted = []Omission{}
	}
	if env.Files == nil {
		env.Files = []FileItem{}
	}
	if env.Symbols == nil {
		env.Symbols = []SymbolItem{}
	}
	if env.Routes == nil {
		env.Routes = []RouteItem{}
	}
	if env.Tests == nil {
		env.Tests = []string{}
	}
	if env.ReadPlan == nil {
		env.ReadPlan = []string{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(env)
}

// ApplyBudget keeps the most relevant context items and records every omitted
// item. The envelope overhead is intentionally included in the estimate so a
// caller can compare the reported value with its requested budget.
func ApplyBudget(env *Envelope, opts Options) {
	if env == nil {
		return
	}
	files := append([]FileItem(nil), env.Files...)
	symbols := append([]SymbolItem(nil), env.Symbols...)
	routes := append([]RouteItem(nil), env.Routes...)
	tests := append([]string(nil), env.Tests...)
	readPlan := append([]string(nil), env.ReadPlan...)

	type candidate struct {
		kind  string
		index int
		score int
		cost  int64
	}
	var candidates []candidate
	for i, f := range files {
		candidates = append(candidates, candidate{kind: "files", index: i, score: fileRelevance(f), cost: 80})
	}
	for i, s := range symbols {
		candidates = append(candidates, candidate{kind: "symbols", index: i, score: symbolRelevance(s), cost: 20})
	}
	for i, r := range routes {
		candidates = append(candidates, candidate{kind: "routes", index: i, score: routeRelevance(r), cost: 30})
	}
	for i := range readPlan {
		candidates = append(candidates, candidate{kind: "read_plan", index: i, score: 5, cost: 20})
	}
	for i := range tests {
		candidates = append(candidates, candidate{kind: "tests", index: i, score: 45, cost: 15})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if candidates[i].kind != candidates[j].kind {
			return candidates[i].kind < candidates[j].kind
		}
		return candidates[i].index < candidates[j].index
	})

	selected := map[string]map[int]bool{}
	for _, kind := range []string{"files", "symbols", "routes", "read_plan", "tests"} {
		selected[kind] = map[int]bool{}
	}
	used := int64(100)
	for _, c := range candidates {
		if opts.Budget > 0 && used+c.cost > opts.Budget {
			continue
		}
		selected[c.kind][c.index] = true
		used += c.cost
	}

	var omitted []Omission
	filterFiles := make([]FileItem, 0, len(files))
	for i, f := range files {
		if selected["files"][i] {
			filterFiles = append(filterFiles, f)
		}
	}
	if n := len(files) - len(filterFiles); n > 0 {
		omitted = append(omitted, Omission{Kind: "files", Count: n, Reason: "budget or max files limit"})
	}
	filterSymbols := make([]SymbolItem, 0, len(symbols))
	for i, s := range symbols {
		if selected["symbols"][i] {
			filterSymbols = append(filterSymbols, s)
		}
	}
	if n := len(symbols) - len(filterSymbols); n > 0 {
		omitted = append(omitted, Omission{Kind: "symbols", Count: n, Reason: "budget or max symbols limit"})
	}
	filterRoutes := make([]RouteItem, 0, len(routes))
	for i, r := range routes {
		if selected["routes"][i] {
			filterRoutes = append(filterRoutes, r)
		}
	}
	if n := len(routes) - len(filterRoutes); n > 0 {
		omitted = append(omitted, Omission{Kind: "routes", Count: n, Reason: "budget or max routes limit"})
	}
	filterStrings := func(kind string, values []string) []string {
		out := make([]string, 0, len(values))
		for i, value := range values {
			if selected[kind][i] {
				out = append(out, value)
			}
		}
		return out
	}
	filterReadPlan := filterStrings("read_plan", readPlan)
	if n := len(readPlan) - len(filterReadPlan); n > 0 {
		omitted = append(omitted, Omission{Kind: "read_plan", Count: n, Reason: "budget limit"})
	}
	filterTests := filterStrings("tests", tests)
	if n := len(tests) - len(filterTests); n > 0 {
		omitted = append(omitted, Omission{Kind: "tests", Count: n, Reason: "budget limit"})
	}

	if opts.MaxFiles > 0 && len(filterFiles) > opts.MaxFiles {
		omitted = append(omitted, Omission{Kind: "files", Count: len(filterFiles) - opts.MaxFiles, Reason: "max files limit"})
		filterFiles = filterFiles[:opts.MaxFiles]
	}
	if opts.MaxSymbols > 0 && len(filterSymbols) > opts.MaxSymbols {
		omitted = append(omitted, Omission{Kind: "symbols", Count: len(filterSymbols) - opts.MaxSymbols, Reason: "max symbols limit"})
		filterSymbols = filterSymbols[:opts.MaxSymbols]
	}
	if opts.MaxRoutes > 0 && len(filterRoutes) > opts.MaxRoutes {
		omitted = append(omitted, Omission{Kind: "routes", Count: len(filterRoutes) - opts.MaxRoutes, Reason: "max routes limit"})
		filterRoutes = filterRoutes[:opts.MaxRoutes]
	}
	env.Files, env.Symbols, env.Routes = filterFiles, filterSymbols, filterRoutes
	env.ReadPlan, env.Tests = filterReadPlan, filterTests
	env.Omitted = omitted
	env.ActualEstimatedTokens = int64(100 + len(env.Files)*80 + len(env.Symbols)*20 + len(env.Routes)*30 + len(env.ReadPlan)*20 + len(env.Tests)*15)
}

func fileRelevance(f FileItem) int {
	if f.Score > 0 {
		return f.Score
	}
	path := strings.ToLower(f.Path)
	score := 20
	if strings.HasSuffix(path, "main.go") {
		score += 50
	}
	if strings.HasPrefix(path, "cmd/") || strings.Contains(path, "/cmd/") {
		score += 30
	}
	if strings.HasSuffix(path, "_test.go") {
		score -= 10
	}
	return score
}

func symbolRelevance(s SymbolItem) int {
	name := strings.ToLower(s.Name)
	if s.Score > 0 {
		score := s.Score + 20
		if symbolNameMatches(name, "load", "package", "extract", "route", "handler", "middleware", "auth", "fallback", "error") {
			score += 60
		}
		if strings.HasPrefix(name, "test") {
			score += 45
		}
		return score
	}
	if name == "main" {
		return 80
	}
	if strings.Contains(name, "route") || strings.Contains(name, "handler") || strings.Contains(name, "auth") {
		return 60
	}
	if s.Kind == "func" || s.Kind == "method" {
		return 40
	}
	return 20
}

func symbolNameMatches(name string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(name, term) {
			return true
		}
	}
	return false
}

func routeRelevance(r RouteItem) int {
	score := 50
	if r.Dynamic {
		score -= 10
	}
	if r.Handler != "" {
		score += 10
	}
	return score
}

func TruncateLines(lines []string, budget int64) ([]string, int, int64) {
	if budget <= 0 {
		return lines, 0, EstimateText(strings.Join(lines, "\n"))
	}
	var out []string
	var used int64
	for _, line := range lines {
		next := EstimateText(line + "\n")
		if len(out) > 0 && used+next > budget {
			return out, len(lines) - len(out), used
		}
		out = append(out, line)
		used += next
	}
	return out, 0, used
}

func MinPositive(a, b int) int {
	if a <= 0 {
		return b
	}
	if b <= 0 || a < b {
		return a
	}
	return b
}
