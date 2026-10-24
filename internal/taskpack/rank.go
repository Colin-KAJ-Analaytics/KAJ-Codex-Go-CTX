package taskpack

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	"goctx/internal/indexer"
	"goctx/internal/scanner"
)

type RankedItem struct {
	Name    string
	Score   int
	Reasons []string
}

type Result struct {
	Task       string
	Packages   []RankedItem
	Files      []RankedItem
	Symbols    []RankedItem
	Tests      []string
	Avoid      []string
	ReadPlan   []string
	Warnings   []string
	Omitted    []string
	Confidence string
	Savings    Savings
}

type Savings struct {
	NaiveEstimatedTokens       int64
	RecommendedEstimatedTokens int64
	ReductionPercent           float64
}

var stop = map[string]bool{
	"a": true, "an": true, "and": true, "or": true, "the": true, "to": true, "of": true, "in": true, "for": true,
	"fix": true, "add": true, "update": true, "internal": true, "repo": true, "repository": true, "code": true,
	"source": true, "task": true, "context": true, "implementation": true,
}

func Tokenize(s string) []string {
	s = splitCamel(s)
	re := regexp.MustCompile(`[A-Za-z0-9]+`)
	raw := re.FindAllString(strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(s, "_", " "), "-", " ")), -1)
	var out []string
	seen := map[string]bool{}
	for _, t := range raw {
		if len(t) < 2 || stop[t] || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	out = expandDomain(out)
	return out
}

func Rank(task string, pkgs []indexer.PackageInfo, scan scanner.Result) Result {
	terms := Tokenize(task)
	res := Result{Task: task, Avoid: []string{"generated files", "vendor", "go.sum", "migrations", "large fixtures", "large generated files"}, Confidence: "medium"}
	fileScores := map[string]int{}
	fileReasons := map[string][]string{}
	symScores := map[string]int{}
	symReasons := map[string][]string{}
	relevantDirs := map[string]bool{}
	for _, p := range pkgs {
		pscore := scoreText(p.ImportPath+" "+p.Name+" "+strings.Join(p.Imports, " "), terms)
		pkgReasons := reasonsForText(p.ImportPath+" "+p.Name+" "+strings.Join(p.Imports, " "), terms, "package/import matched")
		for _, f := range p.Files {
			s := scoreText(f, terms)
			if strings.HasSuffix(f, "_test.go") {
				s += scoreText(strings.TrimSuffix(f, "_test.go"), terms)
			}
			if strings.Contains(strings.ToLower(f), "route") || strings.Contains(strings.ToLower(f), "handler") {
				if containsAny(terms, "route", "routes", "handler", "http", "api", "endpoint") {
					s += 6
					fileReasons[f] = appendUnique(fileReasons[f], "route/handler path matched task")
				}
			}
			if s > 0 {
				fileScores[f] += s
				fileReasons[f] = append(fileReasons[f], reasonsForText(f, terms, "path matched task")...)
				pscore += s / 2
			}
		}
		for _, sym := range p.Symbols {
			s := symbolTaskScore(sym, terms)
			if s > 0 {
				symScores[indexer.DisplayName(sym)] += s
				symReasons[indexer.DisplayName(sym)] = append(symReasons[indexer.DisplayName(sym)], reasonsForText(indexer.DisplayName(sym), terms, "symbol matched task")...)
				fileScores[sym.File] += s
				fileReasons[sym.File] = append(fileReasons[sym.File], "contains matching symbol "+indexer.DisplayName(sym))
				pscore += s
				if !strings.HasSuffix(sym.File, "_test.go") {
					relevantDirs[dirOf(sym.File)] = true
				}
			}
		}
		if pscore > 0 {
			res.Packages = append(res.Packages, RankedItem{Name: p.ImportPath, Score: pscore, Reasons: pkgReasons})
		}
		for _, f := range p.TestFiles {
			if relevantDirs[dirOf(f)] || scoreText(f, terms) > 0 {
				dir := "./" + dirOf(f)
				res.Tests = appendUnique(res.Tests, "go test "+dir)
			}
		}
	}
	for f, s := range fileScores {
		if !ignored(scan, f) && s > 0 {
			res.Files = append(res.Files, RankedItem{Name: f, Score: applyFilePenalty(scan, f, s), Reasons: uniqueStrings(fileReasons[f])})
		}
	}
	for s, score := range symScores {
		if score > 0 {
			res.Symbols = append(res.Symbols, RankedItem{Name: s, Score: score, Reasons: uniqueStrings(symReasons[s])})
		}
	}
	sortRanked(res.Packages)
	sortRanked(res.Files)
	sortRanked(res.Symbols)
	res.Packages = top(res.Packages, 8)
	res.Files = top(res.Files, 12)
	res.Symbols = top(res.Symbols, 15)
	if len(res.Tests) == 0 {
		res.Tests = []string{"go test ./..."}
	}
	res.ReadPlan = buildReadPlan(res)
	res.Savings = estimateSavings(res, scan)
	return res
}

func scoreText(text string, terms []string) int {
	text = strings.ToLower(splitCamel(text))
	score := 0
	for _, t := range terms {
		if strings.Contains(text, t) {
			score += 3
		}
	}
	return score
}

func symbolTaskScore(sym indexer.SymbolInfo, terms []string) int {
	name := indexer.DisplayName(sym)
	score := scoreText(name+" "+sym.File, terms)
	if scoreText(sym.File, terms) > 0 && !strings.HasSuffix(sym.File, "_test.go") {
		score += 6
	}
	lowerName := strings.ToLower(splitCamel(name))
	if containsAny(terms, "route", "routes", "extract", "extraction", "http", "api", "endpoint") && nameContainsAny(lowerName, "route", "extract", "handler", "middleware", "wrapper", "path") {
		score += 24
	}
	if containsAny(terms, "package", "packages", "load", "loader", "loading", "index", "module", "error", "errors", "fallback") && nameContainsAny(lowerName, "load", "package", "module", "import", "fallback", "warning", "error") {
		score += 24
	}
	if sym.Kind == indexer.SymbolTest && containsAny(terms, "test", "error", "errors", "failure", "failing") {
		score += 12
	}
	return score
}

func nameContainsAny(name string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(name, term) {
			return true
		}
	}
	return false
}

func reasonsForText(text string, terms []string, reason string) []string {
	text = strings.ToLower(splitCamel(text))
	for _, t := range terms {
		if strings.Contains(text, t) {
			return []string{reason + " (" + t + ")"}
		}
	}
	return nil
}

func expandDomain(terms []string) []string {
	m := map[string][]string{
		"auth":       {"jwt", "firebase", "token", "middleware"},
		"route":      {"handler", "echo", "gin", "chi", "http", "extract"},
		"routes":     {"handler", "echo", "gin", "chi", "http", "extract"},
		"extract":    {"extraction", "route"},
		"extraction": {"extract", "route"},
		"loader":     {"load", "package", "packages"},
		"loading":    {"load", "package", "packages"},
		"load":       {"loader", "loading"},
		"package":    {"packages"},
		"packages":   {"package"},
		"error":      {"errors", "failure", "fallback", "warning"},
		"errors":     {"error", "failure", "fallback", "warning"},
		"database":   {"supabase", "postgres", "sql", "repository"},
		"config":     {"env", "secret"},
		"test":       {"failing", "panic", "error"},
	}
	seen := map[string]bool{}
	var out []string
	for _, t := range terms {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
		for _, e := range m[t] {
			if !seen[e] {
				seen[e] = true
				out = append(out, e)
			}
		}
	}
	return out
}

func splitCamel(s string) string {
	var b strings.Builder
	var prev rune
	for _, r := range s {
		if prev != 0 && unicode.IsLower(prev) && unicode.IsUpper(r) {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
		prev = r
	}
	return b.String()
}

func sortRanked(xs []RankedItem) {
	sort.Slice(xs, func(i, j int) bool {
		if xs[i].Score == xs[j].Score {
			return xs[i].Name < xs[j].Name
		}
		return xs[i].Score > xs[j].Score
	})
}
func top(xs []RankedItem, n int) []RankedItem {
	if len(xs) < n {
		return xs
	}
	return xs[:n]
}
func dirOf(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[:i]
	}
	return "."
}
func appendUnique(xs []string, x string) []string {
	for _, v := range xs {
		if v == x {
			return xs
		}
	}
	return append(xs, x)
}

func uniqueStrings(xs []string) []string {
	var out []string
	for _, x := range xs {
		out = appendUnique(out, x)
	}
	return out
}

func containsAny(terms []string, qs ...string) bool {
	for _, t := range terms {
		for _, q := range qs {
			if t == q {
				return true
			}
		}
	}
	return false
}

func applyFilePenalty(scan scanner.Result, path string, score int) int {
	for _, f := range scan.Files {
		if f.Path == path && f.SizeBytes > 128*1024 {
			return score - 5
		}
	}
	return score
}

func buildReadPlan(res Result) []string {
	var plan []string
	for _, f := range top(res.Files, 5) {
		plan = append(plan, "goctx symbols "+f.Name)
	}
	for i, s := range res.Symbols {
		if i >= 5 {
			break
		}
		file := ""
		for _, f := range res.Files {
			for _, r := range f.Reasons {
				if strings.Contains(r, s.Name) {
					file = f.Name
					break
				}
			}
			if file != "" {
				break
			}
		}
		if file != "" {
			plan = append(plan, "goctx slice "+file+" --symbol "+s.Name)
		}
	}
	for _, t := range res.Tests {
		plan = append(plan, "goctx test-summary -- "+t)
		if len(plan) >= 10 {
			break
		}
	}
	return plan
}

func estimateSavings(res Result, scan scanner.Result) Savings {
	var naive int64
	for _, f := range res.Files {
		for _, sf := range scan.Files {
			if sf.Path == f.Name {
				naive += sf.SizeBytes / 4
			}
		}
	}
	if naive == 0 {
		for _, sf := range scan.Files {
			if !sf.IgnoredByDefault && (sf.Kind == scanner.KindGoSource || sf.Kind == scanner.KindGoTest) {
				naive += sf.SizeBytes / 4
			}
		}
	}
	rec := int64(250 + len(res.Files)*80 + len(res.Symbols)*20 + len(res.ReadPlan)*20)
	var pct float64
	if naive > 0 && rec < naive {
		pct = float64(naive-rec) / float64(naive) * 100
	}
	return Savings{NaiveEstimatedTokens: naive, RecommendedEstimatedTokens: rec, ReductionPercent: pct}
}
func ignored(scan scanner.Result, path string) bool {
	for _, f := range scan.Files {
		if f.Path == path {
			return f.IgnoredByDefault
		}
	}
	return false
}
