package testsum

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type Summary struct {
	Output   string
	ExitCode int
}

func Run(ctx context.Context, args []string) (Summary, error) {
	if len(args) == 0 {
		return Summary{ExitCode: 2}, fmt.Errorf("missing command after --")
	}
	if args[0] != "go" {
		return Summary{ExitCode: 2}, fmt.Errorf("test-summary only allows commands whose first arg is go")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	exit := 0
	if err != nil {
		exit = 1
		if ee, ok := err.(*exec.ExitError); ok {
			exit = ee.ExitCode()
		}
	}
	return Summary{Output: Summarize(out.String()), ExitCode: exit}, nil
}

func Summarize(raw string) string {
	lines := strings.Split(raw, "\n")
	var failedPkgs, failedTests, refs, panics, expected []string
	failTestRE := regexp.MustCompile(`^--- FAIL: ([A-Za-z0-9_]+)`)
	failPkgRE := regexp.MustCompile(`^(FAIL)\s+(\S+)`)
	refRE := regexp.MustCompile(`([A-Za-z0-9_./-]+\.go:\d+)`)
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if m := failTestRE.FindStringSubmatch(t); m != nil {
			failedTests = appendUnique(failedTests, m[1])
		}
		if m := failPkgRE.FindStringSubmatch(t); m != nil {
			failedPkgs = appendUnique(failedPkgs, m[2])
		}
		if strings.Contains(strings.ToLower(t), "panic") {
			panics = appendBounded(panics, t, 8)
		}
		low := strings.ToLower(t)
		if strings.Contains(low, "expected") || strings.Contains(low, "actual") || strings.Contains(low, "got ") || strings.Contains(low, "want ") {
			expected = appendBounded(expected, t, 12)
		}
		for _, m := range refRE.FindAllString(t, -1) {
			refs = appendUnique(refs, m)
		}
	}
	var b strings.Builder
	if strings.Contains(raw, "FAIL") || len(failedTests) > 0 {
		if len(failedPkgs) > 0 {
			fmt.Fprintf(&b, "FAIL %s\n\n", strings.Join(failedPkgs, ", "))
		} else {
			b.WriteString("FAIL\n\n")
		}
		if len(failedTests) > 0 {
			b.WriteString("Failed tests:\n")
			for i, t := range failedTests {
				fmt.Fprintf(&b, "%d. %s\n", i+1, t)
			}
		}
	} else {
		b.WriteString("Tests passed.\n")
	}
	if len(expected) > 0 {
		b.WriteString("\nKey messages:\n")
		for _, l := range expected {
			fmt.Fprintf(&b, "- %s\n", l)
		}
	}
	if len(panics) > 0 {
		b.WriteString("\nPanic lines:\n")
		for _, l := range panics {
			fmt.Fprintf(&b, "- %s\n", l)
		}
	}
	if len(refs) > 0 {
		b.WriteString("\nRelevant file references:\n")
		for _, r := range refs {
			fmt.Fprintf(&b, "- %s\n", r)
		}
	}
	if n := nonEmpty(lines); n > 80 {
		fmt.Fprintf(&b, "\nRaw output omitted: %d lines suppressed\n", n)
	}
	return b.String()
}

func WriteSummary(w io.Writer, raw string) { _, _ = io.WriteString(w, Summarize(raw)) }

func appendUnique(xs []string, x string) []string {
	for _, v := range xs {
		if v == x {
			return xs
		}
	}
	return append(xs, x)
}

func appendBounded(xs []string, x string, max int) []string {
	if len(xs) >= max {
		return xs
	}
	return append(xs, x)
}

func nonEmpty(lines []string) int {
	n := 0
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}
