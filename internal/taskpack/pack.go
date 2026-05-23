package taskpack

import (
	"fmt"
	"io"
)

func Write(w io.Writer, r Result) {
	fmt.Fprintln(w, "Task Context Pack")
	fmt.Fprintf(w, "Task: %s\n", r.Task)
	fmt.Fprintln(w, "Heuristic ranking only; verify before editing.")
	fmt.Fprintln(w)
	writeRanked(w, "Likely packages:", r.Packages)
	writeRanked(w, "Likely files:", r.Files)
	writeRanked(w, "Likely symbols:", r.Symbols)
	fmt.Fprintln(w, "Recommended read plan:")
	for _, cmd := range r.ReadPlan {
		fmt.Fprintf(w, "- %s\n", cmd)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Suggested focused tests:")
	for _, t := range r.Tests {
		fmt.Fprintf(w, "- %s\n", t)
	}
	fmt.Fprintln(w, "\nAvoid unless task requires:")
	for _, a := range r.Avoid {
		fmt.Fprintf(w, "- %s\n", a)
	}
	if r.Savings.NaiveEstimatedTokens > 0 {
		fmt.Fprintln(w, "\nEstimated context reduction:")
		fmt.Fprintf(w, "- naive context: %d estimated tokens\n", r.Savings.NaiveEstimatedTokens)
		fmt.Fprintf(w, "- goctx recommended context: %d estimated tokens\n", r.Savings.RecommendedEstimatedTokens)
		fmt.Fprintf(w, "- estimated reduction: %.1f%% less context\n", r.Savings.ReductionPercent)
	}
}

func writeRanked(w io.Writer, title string, xs []RankedItem) {
	fmt.Fprintln(w, title)
	if len(xs) == 0 {
		fmt.Fprintln(w, "- no strong matches")
		return
	}
	for i, x := range xs {
		fmt.Fprintf(w, "%d. %s - score %d\n", i+1, x.Name, x.Score)
		for _, r := range x.Reasons {
			fmt.Fprintf(w, "   reason: %s\n", r)
		}
	}
	fmt.Fprintln(w)
}
