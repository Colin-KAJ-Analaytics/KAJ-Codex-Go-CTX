package output

import (
	"fmt"
	"strings"
	"testing"
)

func TestIncrementalSizerMatchesSerializedEnvelope(t *testing.T) {
	for _, compact := range []bool{false, true} {
		for _, count := range []int{0, 1, 2, 10, 101} {
			for _, priorCount := range []int{0, 1, 2} {
				t.Run(fmt.Sprintf("compact=%v/items=%d/prior=%d", compact, count, priorCount), func(t *testing.T) {
					prior := []Omission{}
					for i := 0; i < priorCount; i++ {
						prior = append(prior, Omission{Kind: "earlier", Count: 100 + i, Reason: "upstream <&>\n保留"})
					}
					env := Envelope{Command: "task", Root: "/tmp/long path", CompactJSON: compact, Warnings: []string{"keep <&>\nwarning"}, Data: map[string]any{"text": "雪 \"quotes\"\n", "nested": []string{"a", "b"}}}
					sizer, err := newJSONSizer(env, prior)
					if err != nil {
						t.Fatal(err)
					}
					selectedBytes := 0
					for i := 0; i < count; i++ {
						f := FileItem{Path: fmt.Sprintf("file%d.go", i), Reasons: []string{strings.Repeat("<&>\n雪", i%7)}}
						sym := SymbolItem{Name: fmt.Sprintf("(*Worker).Run%d", i), File: f.Path, Line: i + 1}
						route := RouteItem{Method: "GET", Path: fmt.Sprintf("/path/%d", i), File: f.Path, Handler: "Handler"}
						for _, item := range []any{f, sym, route, "go test ./auth", "goctx symbols auth.go"} {
							cost, err := sizer.itemCost(item, i)
							if err != nil {
								t.Fatal(err)
							}
							selectedBytes += cost
						}
						env.Files = append(env.Files, f)
						env.Symbols = append(env.Symbols, sym)
						env.Routes = append(env.Routes, route)
						env.Tests = append(env.Tests, "go test ./auth")
						env.ReadPlan = append(env.ReadPlan, "goctx symbols auth.go")
					}
					for _, missing := range []int{0, 1, 9, 10, 99, 100, 1000} {
						counts := map[string]int{"files": missing, "symbols": missing + 1, "routes": 0, "tests": missing, "read_plan": 0}
						env.Omitted = append([]Omission(nil), prior...)
						for _, kind := range []string{"files", "symbols", "routes", "tests", "read_plan"} {
							if n := counts[kind]; n > 0 {
								env.Omitted = append(env.Omitted, Omission{Kind: kind, Count: n, Reason: "budget or item limit"})
							}
						}
						_, want, err := measuredEnvelope(env)
						if err != nil {
							t.Fatal(err)
						}
						if got := sizer.estimate(selectedBytes, counts); got != want {
							t.Fatalf("missing=%d estimate=%d serialized=%d", missing, got, want)
						}
					}
				})
			}
		}
	}
}
