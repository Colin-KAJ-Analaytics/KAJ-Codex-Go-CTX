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
	fmt.Fprintln(w, "Suggested focused tests:")
	for _, t := range r.Tests {
		fmt.Fprintf(w, "- %s\n", t)
	}
	fmt.Fprintln(w, "\nAvoid unless task requires:")
	for _, a := range r.Avoid {
		fmt.Fprintf(w, "- %s\n", a)
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
	}
	fmt.Fprintln(w)
}
