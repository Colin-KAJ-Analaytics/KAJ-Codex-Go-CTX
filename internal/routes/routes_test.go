package routes

import (
	"os"
	"path/filepath"
	"testing"
)

func extractSource(t *testing.T, src string) Result {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "routes.go")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	res, err := ExtractFile(root, path)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestRouteExtractionFrameworks(t *testing.T) {
	cases := []string{
		`package p; func r(e E){ e.GET("/health", HealthHandler); e.Use(Auth) }`,
		`package p; func r(g G){ g.POST("/users", CreateUser) }`,
		`package p; func r(c C){ c.Get("/projects", ListProjects) }`,
		`package p; func r(c C){ c.Route("/api", func(r C){}) }`,
		`package p; import "net/http"; func r(){ http.HandleFunc("/x", handler) }`,
	}
	for _, src := range cases {
		res := extractSource(t, src)
		if len(res.Routes) == 0 {
			t.Fatalf("expected route for %s", src)
		}
	}
}

func TestMiddlewareExtraction(t *testing.T) {
	res := extractSource(t, `package p; func r(e E){ e.Use(RequireAuth) }`)
	if len(res.Middleware) != 1 || res.Middleware[0].Expr != "RequireAuth" {
		t.Fatalf("expected middleware, got %#v", res.Middleware)
	}
}
