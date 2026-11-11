package output

import (
	"bytes"
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
	Agent          bool
	FormatExplicit bool
	Explain        bool
	Symbol         string
	Root           string
	Module         string
	AllModules     bool
	Budget         int64
	MaxFiles       int
	MaxSymbols     int
	MaxRoutes      int
	Format         Format
	Force          bool
	Strict         bool
	Quiet          bool
	CompactJSON    bool
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
	Score   int    `json:"score,omitempty"`
	Method  string `json:"method"`
	Path    string `json:"path"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Handler string `json:"handler,omitempty"`
	Dynamic bool   `json:"dynamic,omitempty"`
}

type Envelope struct {
	CompactJSON           bool         `json:"-"`
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
			opts.FormatExplicit = true
		case "--agent":
			opts.Agent = true
		case "--explain":
			opts.Explain = true
		case "--symbol":
			i++
			if i >= len(args) || strings.HasPrefix(args[i], "--") {
				return nil, fmt.Errorf("--symbol requires a name")
			}
			opts.Symbol = args[i]
		case "--quiet":
			opts.Quiet = true
		case "--compact-json":
			opts.CompactJSON = true
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

// marshalEnvelope includes the trailing newline and formatting actually sent
// to the caller. The estimate is byte-based, not a model tokenizer count.
func marshalEnvelope(env Envelope) ([]byte, error) {
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
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	if !env.CompactJSON {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(env); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func measuredEnvelope(env Envelope) ([]byte, int64, error) {
	// Only the number's digit count affects the next estimate; convergence takes
	// a handful of passes even near a power-of-ten boundary.
	for i := 0; i < 10; i++ {
		data, err := marshalEnvelope(env)
		if err != nil {
			return nil, 0, err
		}
		n := tokens.Estimate(int64(len(data)))
		if n == env.ActualEstimatedTokens {
			return data, n, nil
		}
		env.ActualEstimatedTokens = n
	}
	return nil, 0, fmt.Errorf("output estimate did not converge")
}

func WriteJSON(w io.Writer, env Envelope, options ...Options) error {
	if len(options) > 0 {
		env.CompactJSON = options[0].CompactJSON
	}
	data, _, err := measuredEnvelope(env)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// ApplyBudget selects whole items by relevance using their serialized size.
// Mandatory metadata is retained even if the requested budget is too small;
// that case is reported explicitly rather than understating the output size.
func ApplyBudget(env *Envelope, opts Options) {
	if env == nil {
		return
	}
	env.CompactJSON = opts.CompactJSON
	env.BudgetEstimatedTokens = opts.Budget
	files := append([]FileItem(nil), env.Files...)
	symbols := append([]SymbolItem(nil), env.Symbols...)
	routes := append([]RouteItem(nil), env.Routes...)
	tests := append([]string(nil), env.Tests...)
	readPlan := append([]string(nil), env.ReadPlan...)
	if opts.Quiet {
		for i := range files {
			files[i].Reasons = nil
		}
		for i := range symbols {
			symbols[i].Reasons = nil
		}
		if data, ok := env.Data.(map[string]any); ok {
			copy := map[string]any{}
			for k, v := range data {
				if k != "avoid" && k != "savings" {
					copy[k] = v
				}
			}
			env.Data = copy
		}
	}
	type candidate struct {
		kind         string
		index, score int
	}
	var candidates []candidate
	for i, f := range files {
		candidates = append(candidates, candidate{"files", i, fileRelevance(f)})
	}
	for i, v := range symbols {
		candidates = append(candidates, candidate{"symbols", i, symbolRelevance(v)})
	}
	for i, v := range routes {
		candidates = append(candidates, candidate{"routes", i, routeRelevance(v)})
	}
	for i := range tests {
		candidates = append(candidates, candidate{"tests", i, 70})
	}
	for i := range readPlan {
		candidates = append(candidates, candidate{"read_plan", i, 65})
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
	prior := append([]Omission(nil), env.Omitted...)
	env.Files, env.Symbols, env.Routes, env.Tests, env.ReadPlan = nil, nil, nil, nil, nil
	updateOmissions := func() {
		env.Omitted = append([]Omission(nil), prior...)
		for _, item := range []struct {
			kind    string
			missing int
		}{
			{"files", len(files) - len(env.Files)}, {"symbols", len(symbols) - len(env.Symbols)},
			{"routes", len(routes) - len(env.Routes)}, {"tests", len(tests) - len(env.Tests)}, {"read_plan", len(readPlan) - len(env.ReadPlan)},
		} {
			if item.missing > 0 {
				env.Omitted = append(env.Omitted, Omission{item.kind, item.missing, "budget or item limit"})
			}
		}
	}
	sizer, sizeErr := newJSONSizer(*env, prior)
	selectedBytes := 0
	for _, c := range candidates {
		var item any
		var existing int
		switch c.kind {
		case "files":
			item, existing = files[c.index], len(env.Files)
		case "symbols":
			item, existing = symbols[c.index], len(env.Symbols)
		case "routes":
			item, existing = routes[c.index], len(env.Routes)
		case "tests":
			item, existing = tests[c.index], len(env.Tests)
		case "read_plan":
			item, existing = readPlan[c.index], len(env.ReadPlan)
		}
		switch c.kind {
		case "files":
			if opts.MaxFiles > 0 && len(env.Files) >= opts.MaxFiles {
				continue
			}
			env.Files = append(env.Files, files[c.index])
		case "symbols":
			if opts.MaxSymbols > 0 && len(env.Symbols) >= opts.MaxSymbols {
				continue
			}
			env.Symbols = append(env.Symbols, symbols[c.index])
		case "routes":
			if opts.MaxRoutes > 0 && len(env.Routes) >= opts.MaxRoutes {
				continue
			}
			env.Routes = append(env.Routes, routes[c.index])
		case "tests":
			env.Tests = append(env.Tests, tests[c.index])
		case "read_plan":
			env.ReadPlan = append(env.ReadPlan, readPlan[c.index])
		}
		cost, err := sizer.itemCost(item, existing)
		n := sizer.estimate(selectedBytes+cost, map[string]int{
			"files": len(files) - len(env.Files), "symbols": len(symbols) - len(env.Symbols),
			"routes": len(routes) - len(env.Routes), "tests": len(tests) - len(env.Tests), "read_plan": len(readPlan) - len(env.ReadPlan),
		})
		if sizeErr != nil || err != nil || (opts.Budget > 0 && n > opts.Budget) {
			switch c.kind {
			case "files":
				env.Files = env.Files[:len(env.Files)-1]
			case "symbols":
				env.Symbols = env.Symbols[:len(env.Symbols)-1]
			case "routes":
				env.Routes = env.Routes[:len(env.Routes)-1]
			case "tests":
				env.Tests = env.Tests[:len(env.Tests)-1]
			case "read_plan":
				env.ReadPlan = env.ReadPlan[:len(env.ReadPlan)-1]
			}
		} else {
			selectedBytes += cost
		}
	}
	updateOmissions()
	_, n, err := measuredEnvelope(*env)
	if err != nil {
		env.Warnings = append(env.Warnings, "cannot estimate JSON: "+err.Error())
	}
	if opts.Budget > 0 && n > opts.Budget {
		env.Warnings = append(env.Warnings, "mandatory metadata exceeds requested budget; increase --budget")
	}
	_, env.ActualEstimatedTokens, _ = measuredEnvelope(*env)
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
	if r.Score > 0 {
		return r.Score
	}
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
