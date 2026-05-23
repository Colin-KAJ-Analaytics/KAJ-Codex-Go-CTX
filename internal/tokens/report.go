package tokens

import (
	"fmt"
	"io"
	"sort"

	"goctx/internal/scanner"
)

func Report(w io.Writer, scan scanner.Result) {
	files := append([]scanner.FileInfo(nil), scan.Files...)
	sort.Slice(files, func(i, j int) bool { return Estimate(files[i].SizeBytes) > Estimate(files[j].SizeBytes) })
	fmt.Fprintln(w, "Token risk report (rough estimate: size_bytes / 4):")
	limit := 20
	if len(files) < limit {
		limit = len(files)
	}
	for i := 0; i < limit; i++ {
		f := files[i]
		if f.SizeBytes == 0 {
			continue
		}
		fmt.Fprintf(w, "%d. %s\n   size: %d bytes\n   estimated tokens: %d\n", i+1, f.Path, f.SizeBytes, Estimate(f.SizeBytes))
		if f.Reason != "" {
			fmt.Fprintf(w, "   reason: %s\n", f.Reason)
		}
	}
	fmt.Fprintln(w, "\nSuggested Codex exclusions:")
	for _, e := range SuggestedExclusions() {
		fmt.Fprintf(w, "- %s\n", e)
	}
}

func SuggestedExclusions() []string {
	return []string{"vendor/", "node_modules/", "dist/", "build/", "coverage/", ".goctx/", "go.sum", "**/*.pb.go", "**/*_generated.go", "**/*.gen.go", "mock_*.go"}
}
