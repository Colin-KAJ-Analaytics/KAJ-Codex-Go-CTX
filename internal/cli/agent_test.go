package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"goctx/internal/codexpack"
)

func TestAgentChoosesEmbeddedPageOrGoHandler(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"web/go.mod":             "module example.com/web\ngo 1.23\n",
		"worker/go.mod":          "module example.com/worker\ngo 1.23\n",
		"worker/worker.go":       "package worker\nfunc Dispatch() {}\n",
		"web/static/canvas.html": `<script src="app.js"></script>`,
		"web/static/app.js":      strings.Repeat(" ", 86400),
		"web/server.go": `package web
import ("embed"; "net/http")
//go:embed static
var assets embed.FS
func Canvas(w http.ResponseWriter,r *http.Request) { http.ServeFileFS(w,r,assets,"static/canvas.html") }
func InspectPipeline(w http.ResponseWriter,r *http.Request) { w.WriteHeader(204) }
func Handler() http.Handler {
 mux:=http.NewServeMux()
 mux.HandleFunc("GET /canvas",Canvas)
 mux.HandleFunc("POST /api/inspect",InspectPipeline)
 return mux
}
`,
		"web/server_test.go": `package web
import ("net/http/httptest";"testing")
func TestCanvas(t *testing.T) {
 w:=httptest.NewRecorder()
 Handler().ServeHTTP(w,httptest.NewRequest("GET","/canvas",nil))
 if w.Code!=200 {t.Fatal(w.Code)}
}
func TestInspect(t *testing.T) {
 w:=httptest.NewRecorder()
 InspectPipeline(w,httptest.NewRequest("POST","/api/inspect",nil))
 if w.Code!=204 {t.Fatal(w.Code)}
}
`,
	}
	for name, body := range files {
		file := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	run := func(task string, extra ...string) codexpack.AgentPack {
		t.Helper()
		var out, stderr bytes.Buffer
		args := append([]string{"task", task, "--agent", "--root", root, "--budget", "350"}, extra...)
		if code := Run(args, &out, &stderr); code != 0 {
			t.Fatalf("%d: %s", code, &stderr)
		}
		var p codexpack.AgentPack
		if err := json.Unmarshal(out.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		if p.EstimatedTokens != int64(out.Len())/4 || p.EstimatedTokens > 350 {
			t.Fatal(out.String())
		}
		return p
	}
	page := run("fix the embedded canvas page")
	if page.Module != "web" || page.Decision != "likely_asset_edit" || len(page.Symbols) != 0 || len(page.Files) != 2 || page.Files[0].Path != "static/canvas.html" || page.Files[1].EstimatedTokens != 21600 || len(page.Routes) != 1 || page.Routes[0].Asset != "static/canvas.html" || !strings.Contains(page.Test, "^TestCanvas$") {
		t.Fatalf("%+v", page)
	}
	goEdit := run("fix InspectPipeline API handler")
	if goEdit.Module != "web" || goEdit.Decision != "likely_go_edit" || len(goEdit.Symbols) != 1 || !strings.Contains(goEdit.Symbols[0].Signature, "func InspectPipeline") || goEdit.Symbols[0].StartLine < 1 || !strings.Contains(goEdit.Test, "^TestInspect$") {
		t.Fatalf("%+v", goEdit)
	}
	command := exec.Command("go", "test", "-C", filepath.Join(root, "web"), ".", "-run", "^TestCanvas$")
	if data, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, data)
	}
	unknown := run("fix server handler")
	if unknown.Decision != "needs_context" || unknown.Confidence != "low" {
		t.Fatalf("%+v", unknown)
	}
}

func TestSymbolCommandsAcceptRootAndFormats(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "one.go"), []byte("package one\nfunc Example(s string) string { return s }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"symbols", "one.go", "--quiet", "--format", "json", "--root", root},
		{"slice", "one.go", "--symbol", "Example", "--quiet", "--format", "json", "--root", root},
	} {
		var out, stderr bytes.Buffer
		if code := Run(args, &out, &stderr); code != 0 || !json.Valid(out.Bytes()) || !strings.Contains(out.String(), "func Example(s string) string") {
			t.Fatalf("%d: %s %s", code, &stderr, &out)
		}
	}
	var out, stderr bytes.Buffer
	if code := Run([]string{"symbols", "one.go", "--typo"}, &out, &stderr); code != 2 || !strings.Contains(stderr.String(), "--typo") {
		t.Fatal(stderr.String())
	}
}

func TestAgentFollowsMountedCanvasScript(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"web/go.mod":                 "module example.com/mounted\ngo 1.23\n",
		"web/static/canvas.html":     `<script src="/assets/theme.js"></script><script src="/assets/app.js"></script>`,
		"web/static/assets/theme.js": "const theme = 'dark';",
		"web/static/assets/app.js":   `function openPipeline() { fetch("/api/inspect", { method: "POST" }); }`,
		"web/server.go": `package web
import ("embed"; "io/fs"; "net/http")
//go:embed static
var embedded embed.FS
func Handler() http.Handler {
 static, err:=fs.Sub(embedded,"static")
 if err!=nil { panic(err) }
 files:=http.FileServer(http.FS(static))
 mux:=http.NewServeMux()
 mux.Handle("/assets/", wrap(files))
 mux.Handle("/", wrap(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request) {
  if r.URL.Path=="/canvas" { page(w,static,"canvas.html");return }
 })))
 mux.HandleFunc("GET /healthz",func(w http.ResponseWriter,r *http.Request){})
 mux.HandleFunc("POST /api/inspect",func(w http.ResponseWriter,r *http.Request){})
 return mux
}
func wrap(next http.Handler) http.Handler {
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request) {
  w.Header().Set("X-Test","yes")
  next.ServeHTTP(w,r)
 })
}
func page(w http.ResponseWriter,files fs.FS,name string) {
 data,_:=fs.ReadFile(files,name)
 w.Write(data)
}
`,
		"web/server_test.go": `package web
import ("testing"; "net/http/httptest")
func TestCanvas(t *testing.T) {
 Handler().ServeHTTP(httptest.NewRecorder(),httptest.NewRequest("GET","/canvas",nil))
}
`,
	}
	for name, body := range files {
		file := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	var out, stderr bytes.Buffer
	if code := Run([]string{"task", "open a pipeline file on the canvas", "--agent", "--root", root, "--budget", "250"}, &out, &stderr); code != 0 {
		t.Fatalf("%d: %s", code, &stderr)
	}
	var p codexpack.AgentPack
	if err := json.Unmarshal(out.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Module != "web" || p.Decision != "likely_asset_edit" || len(p.Files) != 2 || p.Files[0].Path != "static/assets/app.js" || p.Files[0].AssetURL != "/assets/app.js" || len(p.Routes) != 2 || p.Routes[0].Path != "/canvas" || p.Routes[1].Path != "/api/inspect" || p.Routes[1].Method != "POST" || p.TestScope != "page_smoke" || !strings.Contains(p.Test, "^TestCanvas$") || p.EstimatedTokens != int64(out.Len())/4 || p.EstimatedTokens > 250 {
		t.Fatal(out.String())
	}
}
