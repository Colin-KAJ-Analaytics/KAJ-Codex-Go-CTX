package output

import (
	"encoding/json"
	"fmt"
	"io"
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
