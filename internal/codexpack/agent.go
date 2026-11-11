package codexpack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"goctx/internal/baseline"
	"goctx/internal/discovery"
	"goctx/internal/indexer"
	"goctx/internal/output"
	"goctx/internal/tokens"
)

type AgentSymbol struct {
	Name      string `json:"name"`
	File      string `json:"file"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Signature string `json:"signature,omitempty"`
}

func SymbolLocation(s indexer.SymbolInfo, file string) AgentSymbol {
	return AgentSymbol{indexer.DisplayName(s), filepath.ToSlash(file), s.StartLine, s.EndLine, s.Signature}
}

type AgentFile struct {
	Path            string `json:"path"`
	EstimatedTokens int64  `json:"estimated_tokens"`
	EmbedPattern    string `json:"embed_pattern,omitempty"`
	AssetURL        string `json:"asset_url,omitempty"`
}

type AgentRoute struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Asset  string `json:"asset,omitempty"`
}

type AgentPack struct {
	Version         int           `json:"version"`
	Decision        string        `json:"decision"`
	Confidence      string        `json:"confidence"`
	IndexHealth     string        `json:"index_health,omitempty"`
	Module          string        `json:"module,omitempty"`
	Files           []AgentFile   `json:"files,omitempty"`
	Symbols         []AgentSymbol `json:"symbols,omitempty"`
	Routes          []AgentRoute  `json:"routes,omitempty"`
	Test            string        `json:"test,omitempty"`
	TestScope       string        `json:"test_scope,omitempty"`
	EstimatedTokens int64         `json:"estimated_tokens"`
	Omitted         int           `json:"omitted"`
	Warnings        []string      `json:"warnings,omitempty"`
	Evidence        []string      `json:"evidence,omitempty"`
}

var camelBoundary = regexp.MustCompile(`([a-z0-9])([A-Z])`)
var backendTask = regexp.MustCompile(`(?i)\b(handler|endpoint|backend|api|middleware|http status)\b`)
var agentWord = regexp.MustCompile(`[a-z0-9]+`)
var genericWords = map[string]bool{
	"a": true, "an": true, "the": true, "for": true, "to": true, "in": true, "of": true, "on": true, "and": true,
	"go": true, "code": true, "change": true, "fix": true, "update": true, "add": true, "edit": true,
	"server": true, "handler": true, "handlers": true, "route": true, "routes": true, "page": true,
	"api": true, "web": true, "http": true, "static": true, "embedded": true, "asset": true,
	"file": true, "test": true, "function": true, "internal": true,
}

func agentTerms(s string) map[string]bool {
	terms := map[string]bool{}
	for _, word := range agentWord.FindAllString(strings.ToLower(camelBoundary.ReplaceAllString(s, "${1} ${2}")), -1) {
		if len(word) > 1 && !genericWords[word] {
			terms[word] = true
		}
	}
	return terms
}

func matchTerms(task map[string]bool, value string) int {
	score := 0
	for term := range agentTerms(value) {
		if task[term] {
			score++
		}
	}
	return score
}

func BuildAgent(ctx context.Context, task string, opts output.Options) (AgentPack, error) {
	disc, err := discovery.Discover(opts.Root)
	if err != nil {
		return AgentPack{}, err
	}
	if opts.Module != "" {
		found := false
		for _, m := range disc.Modules {
			if m.Path == opts.Module || m.ModulePath == opts.Module || filepath.Base(m.Path) == opts.Module {
				found = true
				break
			}
		}
		if !found {
			return AgentPack{}, fmt.Errorf("module %q not found", opts.Module)
		}
	}
	if opts.Module != "" || len(disc.Modules) <= 1 {
		r, err := Build(ctx, task, opts)
		if err != nil {
			return AgentPack{}, err
		}
		pack, _ := agentResult(r, opts)
		return pack, nil
	}
	// A caller already inside a nested module supplies strong location evidence.
	// At the workspace root, compare modules by task evidence.
	if disc.Selected != "" && disc.Selected != "." && !opts.AllModules {
		opts.Module = disc.Selected
		r, err := Build(ctx, task, opts)
		if err != nil {
			return AgentPack{}, err
		}
		pack, _ := agentResult(r, opts)
		return pack, nil
	}
	best := AgentPack{Version: 1, Decision: "needs_context", Confidence: "low"}
	bestScore := -1
	tied := false
	for _, m := range disc.Modules {
		if err := ctx.Err(); err != nil {
			return AgentPack{}, err
		}
		opts.Module = m.Path
		r, err := Build(ctx, task, opts)
		if err != nil {
			return AgentPack{}, err
		}
		pack, score := agentResult(r, opts)
		if score > bestScore {
			best, bestScore, tied = pack, score, false
		} else if score == bestScore {
			tied = true
		}
	}
	if tied {
		return AgentPack{Version: 1, Decision: "needs_context", Confidence: "low", Warnings: []string{"Multiple modules match equally. Select one with --module."}}, nil
	}
	return best, nil
}

func agentResult(r Result, opts output.Options) (AgentPack, int) {
	pack := AgentPack{Version: 1, Decision: "needs_context", Confidence: "low", IndexHealth: r.Index.Confidence, Module: r.Discovery.Selected, Warnings: r.Index.Warnings}
	terms := agentTerms(r.Task)
	var nested []string
	for _, f := range r.Scan.Files {
		if strings.HasSuffix(f.Path, "/go.mod") {
			nested = append(nested, path.Dir(f.Path)+"/")
		}
	}
	inModule := func(file string) bool {
		for _, dir := range nested {
			if strings.HasPrefix(file, dir) {
				return false
			}
		}
		return true
	}
	scoped := r.Scan
	scoped.Files = nil
	for _, f := range r.Scan.Files {
		if inModule(f.Path) {
			scoped.Files = append(scoped.Files, f)
		}
	}
	info := readAgentSource(scoped)
	mounts, pages := info.staticRoutes()
	sort.Slice(pages, func(i, j int) bool {
		if pages[i].Path != pages[j].Path {
			return pages[i].Path < pages[j].Path
		}
		return pages[i].Asset < pages[j].Asset
	})
	type candidate struct {
		symbol AgentSymbol
		file   AgentFile
		score  int
	}
	var choices []candidate
	var matchedRoutes []AgentRoute
	var targetRoute string
	backend := backendTask.MatchString(r.Task)
	fileSizes := map[string]int64{}
	for _, f := range r.Scan.Files {
		fileSizes[f.Path] = f.SizeBytes
	}
	for _, page := range pages {
		if matchTerms(terms, page.Path) == 0 && matchTerms(terms, path.Base(page.Asset)) == 0 {
			continue
		}
		matchedRoutes = append(matchedRoutes, page)
		if targetRoute == "" {
			targetRoute = page.Path
		}
		if !backend {
			choices = append(choices, candidate{file: info.assets[page.Asset], score: 100 + matchTerms(terms, page.Path)*10})
		}
	}
	for _, rt := range r.Routes.Routes {
		if rt.Dynamic || !inModule(rt.File) {
			continue
		}
		method, url := rt.Method, rt.Path
		if m, p, ok := strings.Cut(url, " "); ok && strings.HasPrefix(p, "/") {
			method, url = m, p
		}
		if url == "/" && targetRoute != "" && !strings.Contains(" "+r.Task+" ", " / ") {
			continue
		}
		if matchTerms(terms, url+" "+rt.Handler) == 0 && !strings.Contains(" "+r.Task+" ", " "+url+" ") {
			continue
		}
		asset := info.routeAsset(rt.File, rt.Handler)
		matchedRoutes = append(matchedRoutes, AgentRoute{method, url, asset})
		if targetRoute == "" {
			targetRoute = url
		}
		if asset != "" && !backend {
			f := info.assets[asset]
			choices = append(choices, candidate{file: f, score: 100 + matchTerms(terms, url)*10})
		}
	}
	for _, f := range info.assets {
		if n := matchTerms(terms, path.Base(f.Path)); n > 0 && !backend {
			score := 25 + n*10
			for _, word := range strings.Fields(r.Task) {
				word = strings.Trim(word, "\"'`(),;:")
				if strings.EqualFold(path.Base(word), path.Base(f.Path)) {
					score += 110
					break
				}
			}
			choices = append(choices, candidate{file: f, score: score})
		}
	}
	for _, pkg := range r.Index.Packages {
		for _, s := range pkg.Symbols {
			if !inModule(s.File) || s.Kind == indexer.SymbolTest || strings.HasSuffix(s.File, "_test.go") {
				continue
			}
			hits := matchTerms(terms, s.Name)
			score := hits * 10
			for _, rt := range r.Routes.Routes {
				if rt.File == s.File && rt.Handler == s.Name && matchTerms(terms, rt.Path) > 0 {
					score += 20
					break
				}
			}
			if score > 0 {
				if terms[strings.ToLower(s.Name)] {
					score += 8
				}
				score += 2 * matchTerms(terms, path.Base(s.File))
				choices = append(choices, candidate{symbol: SymbolLocation(s, s.File), file: AgentFile{Path: s.File, EstimatedTokens: tokens.Estimate(fileSizes[s.File])}, score: score})
			}
		}
	}
	sort.SliceStable(choices, func(i, j int) bool {
		if choices[i].score != choices[j].score {
			return choices[i].score > choices[j].score
		}
		if choices[i].file.Path != choices[j].file.Path {
			return choices[i].file.Path < choices[j].file.Path
		}
		return choices[i].symbol.StartLine < choices[j].symbol.StartLine
	})
	if len(choices) > 0 {
		// Only hash files in the highest-scoring group.
		var paths []string
		for _, c := range choices {
			if c.score != choices[0].score {
				break
			}
			paths = append(paths, c.file.Path)
		}
		if len(paths) > 1 {
			changed := baseline.ChangedCandidates(r.Scan.Root, paths)
			for i, c := range choices {
				if c.score != choices[0].score {
					break
				}
				if changed[c.file.Path] {
					choices[0], choices[i] = choices[i], choices[0]
					break
				}
			}
		}
		c := choices[0]
		pack.Files = []AgentFile{c.file}
		pack.Confidence = "medium"
		if c.symbol.Name != "" {
			pack.Decision = "likely_go_edit"
			pack.Symbols = []AgentSymbol{c.symbol}
			if c.score >= 20 {
				pack.Confidence = "high"
			}
		} else {
			pack.Decision = "likely_asset_edit"
			if c.score >= 100 {
				pack.Confidence = "high"
				for _, rt := range matchedRoutes {
					if rt.Asset == c.file.Path {
						targetRoute = rt.Path
						break
					}
				}
			}
		}
		for _, other := range choices[1:] {
			if other.score < c.score-5 {
				break
			}
			if other.symbol.Name != "" && c.symbol.Name != "" && other.symbol != c.symbol {
				pack.Symbols = append(pack.Symbols, other.symbol)
				if other.file.Path != c.file.Path {
					pack.Files = append(pack.Files, other.file)
				}
				pack.Confidence = "medium"
				break
			}
			if other.symbol.Name == "" && c.symbol.Name == "" && other.file.Path != c.file.Path {
				pack.Files = append(pack.Files, other.file)
				pack.Confidence = "medium"
				break
			}
		}
		if r.Index.Confidence != "high" && pack.Decision == "likely_go_edit" {
			pack.Confidence = "medium"
		}
		if pack.Decision == "likely_asset_edit" && len(pack.Files) == 1 {
			linked := info.linkedAssets(r.Scan.Root, c.file.Path, terms, mounts)
			if len(linked) > 0 {
				best := linked[0]
				if best.score > 0 && best.source != "" {
					pack.Files = []AgentFile{best.file, c.file}
				} else {
					pack.Files = append(pack.Files, best.file)
				}
				pack.Omitted += len(linked) - 1
				// A script reference identifies a related endpoint, not test
				// coverage for the particular browser behavior.
				for _, ref := range literalFetch.FindAllStringSubmatch(best.source, -1) {
					for _, rt := range r.Routes.Routes {
						method, url, ok := strings.Cut(rt.Path, " ")
						if !ok {
							method, url = rt.Method, rt.Path
						}
						if rt.Dynamic || !inModule(rt.File) || url != ref[1] || !strings.HasPrefix(url, "/api/") {
							continue
						}
						exists := false
						for _, previous := range matchedRoutes {
							if previous.Path == url && previous.Method == method {
								exists = true
								break
							}
						}
						if !exists {
							matchedRoutes = append(matchedRoutes, AgentRoute{Method: method, Path: url})
						}
					}
				}
				if opts.Explain {
					pack.Evidence = append(pack.Evidence, c.file.Path+" references "+best.file.AssetURL+" which resolves to "+best.file.Path)
				}
			}
		}
		pack.Test = info.focusedTest(c.symbol, targetRoute)
		if pack.Test != "" {
			pack.TestScope = "source_or_route"
			if pack.Decision == "likely_asset_edit" {
				pack.TestScope = "page_smoke"
			}
		}
		if pack.Test == "" && c.symbol.File != "" {
			dir := path.Dir(c.symbol.File)
			for _, pkg := range r.Index.Packages {
				if pkg.Dir == dir || (dir == "." && pkg.Dir == "") {
					if len(pkg.TestFiles) > 0 {
						pack.Test = "goctx test-summary -- go test " + quoteArg(packageArg(dir))
						pack.TestScope = "package"
					}
					break
				}
			}
		}
		if pack.Test != "" {
			pack.Test = strings.Replace(pack.Test, "go test ", "go test -C "+quoteArg(r.Scan.Root)+" ", 1)
		}
		if opts.Explain {
			pack.Evidence = append(pack.Evidence, "Ranked by task terms, literal route evidence, and baseline changes for equal matches.")
		}
	}
	sort.SliceStable(matchedRoutes, func(i, j int) bool {
		return (matchedRoutes[i].Path == targetRoute) && (matchedRoutes[j].Path != targetRoute)
	})
	limit := min(5, opts.MaxRoutes)
	if limit <= 0 {
		limit = 5
	}
	if len(matchedRoutes) > limit {
		pack.Omitted += len(matchedRoutes) - limit
		matchedRoutes = matchedRoutes[:limit]
	}
	pack.Routes = matchedRoutes
	if len(pack.Symbols) > opts.MaxSymbols && opts.MaxSymbols > 0 {
		pack.Omitted += len(pack.Symbols) - opts.MaxSymbols
		pack.Symbols = pack.Symbols[:opts.MaxSymbols]
	}
	if len(pack.Files) > opts.MaxFiles && opts.MaxFiles > 0 {
		pack.Omitted += len(pack.Files) - opts.MaxFiles
		pack.Files = pack.Files[:opts.MaxFiles]
		if len(pack.Symbols) > 1 && pack.Symbols[1].File != pack.Files[0].Path {
			pack.Symbols = pack.Symbols[:1]
			pack.Omitted++
		}
	}
	if len(choices) > 0 {
		return pack, choices[0].score
	}
	return pack, 0
}

func packageArg(dir string) string {
	if dir == "." {
		return "."
	}
	return "./" + dir
}

func quoteArg(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

func (p AgentPack) Write(w io.Writer, opts output.Options) error {
	for {
		var data []byte
		for {
			if opts.FormatExplicit && opts.Format != output.FormatJSON {
				var b strings.Builder
				fmt.Fprintf(&b, "%s (%s)\nmodule: %s\n", p.Decision, p.Confidence, p.Module)
				if p.IndexHealth != "" {
					fmt.Fprintf(&b, "index health: %s\n", p.IndexHealth)
				}
				for _, f := range p.Files {
					fmt.Fprintf(&b, "%s (%d estimated tokens)\n", f.Path, f.EstimatedTokens)
					if f.AssetURL != "" {
						fmt.Fprintln(&b, "  URL:", f.AssetURL)
					}
				}
				for _, s := range p.Symbols {
					fmt.Fprintf(&b, "%s:%d-%d %s\n", s.File, s.StartLine, s.EndLine, s.Signature)
				}
				for _, r := range p.Routes {
					fmt.Fprintf(&b, "%s %s %s\n", r.Method, r.Path, r.Asset)
				}
				if p.Test != "" {
					fmt.Fprintf(&b, "%s test: %s\n", p.TestScope, p.Test)
				}
				for _, line := range append(append([]string(nil), p.Warnings...), p.Evidence...) {
					fmt.Fprintln(&b, line)
				}
				fmt.Fprintf(&b, "estimated tokens: %d, omitted: %d\n", p.EstimatedTokens, p.Omitted)
				data = []byte(b.String())
			} else {
				var err error
				data, err = json.Marshal(p)
				if err != nil {
					return err
				}
				data = append(data, '\n')
			}
			n := int64(len(data)) / 4
			if n == p.EstimatedTokens {
				break
			}
			p.EstimatedTokens = n
		}
		if opts.Budget <= 0 || p.EstimatedTokens <= opts.Budget {
			_, err := w.Write(data)
			return err
		}
		switch {
		case len(p.Evidence) > 0:
			p.Evidence = p.Evidence[:len(p.Evidence)-1]
		case len(p.Warnings) > 0:
			p.Warnings = p.Warnings[:len(p.Warnings)-1]
		case len(p.Routes) > 0:
			p.Routes = p.Routes[:len(p.Routes)-1]
		case p.Test != "":
			p.Test, p.TestScope = "", ""
		case len(p.Symbols) > 1:
			p.Symbols = p.Symbols[:1]
		case len(p.Files) > 1:
			p.Files = p.Files[:1]
		default:
			return fmt.Errorf("agent result needs at least %d estimated tokens; increase --budget", p.EstimatedTokens)
		}
		p.Omitted++
	}
}
