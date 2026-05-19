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
	Name  string
	Score int
}

type Result struct {
	Task     string
	Packages []RankedItem
	Files    []RankedItem
	Symbols  []RankedItem
	Tests    []string
	Avoid    []string
}

var stop = map[string]bool{"a": true, "an": true, "and": true, "or": true, "the": true, "to": true, "of": true, "in": true, "for": true, "fix": true, "add": true, "update": true}

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
	res := Result{Task: task, Avoid: []string{"generated files", "vendor", "go.sum", "migrations", "frontend assets"}}
	fileScores := map[string]int{}
	symScores := map[string]int{}
	for _, p := range pkgs {
		pscore := scoreText(p.ImportPath+" "+p.Name+" "+strings.Join(p.Imports, " "), terms)
		for _, f := range p.Files {
			s := scoreText(f, terms)
			if strings.HasSuffix(f, "_test.go") {
				s += scoreText(strings.TrimSuffix(f, "_test.go"), terms)
			}
			if s > 0 {
				fileScores[f] += s
				pscore += s / 2
			}
		}
		for _, sym := range p.Symbols {
			s := scoreText(indexer.DisplayName(sym)+" "+sym.File, terms)
			if s > 0 {
				symScores[indexer.DisplayName(sym)] += s
				fileScores[sym.File] += s
				pscore += s
			}
		}
		if pscore > 0 {
			res.Packages = append(res.Packages, RankedItem{Name: p.ImportPath, Score: pscore})
		}
		for _, f := range p.TestFiles {
			if fileScores[strings.TrimSuffix(f, "_test.go")+".go"] > 0 || scoreText(f, terms) > 0 {
				dir := "./" + dirOf(f)
				res.Tests = appendUnique(res.Tests, "go test "+dir)
			}
		}
	}
	for f, s := range fileScores {
		if !ignored(scan, f) && s > 0 {
			res.Files = append(res.Files, RankedItem{Name: f, Score: s})
		}
	}
	for s, score := range symScores {
		if score > 0 {
			res.Symbols = append(res.Symbols, RankedItem{Name: s, Score: score})
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

func expandDomain(terms []string) []string {
	m := map[string][]string{"auth": {"jwt", "firebase", "token", "middleware"}, "route": {"handler", "echo", "gin", "chi", "http"}, "routes": {"handler", "echo", "gin", "chi", "http"}, "database": {"supabase", "postgres", "sql", "repository"}, "config": {"env", "secret"}, "test": {"failing", "panic", "error"}}
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
func ignored(scan scanner.Result, path string) bool {
	for _, f := range scan.Files {
		if f.Path == path {
			return f.IgnoredByDefault
		}
	}
	return false
}
