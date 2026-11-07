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
	if len(args) < 2 || args[1] != "test" {
		return Summary{ExitCode: 2}, fmt.Errorf("test-summary only allows go test commands")
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
	raw := out.String()
	if err != nil {
		if ctx.Err() != nil {
			raw += "\n" + ctx.Err().Error()
		} else if _, ok := err.(*exec.ExitError); !ok {
			raw += "\n" + err.Error()
		}
		if exit < 1 {
			exit = 1
		}
	}
	return Summary{Output: SummarizeWithExit(raw, exit), ExitCode: exit}, nil
}

// Summarize infers status from anchored Go test records. Run uses the process
// exit code instead, including failures that never produce a FAIL record.
func Summarize(raw string) string { return summarize(raw, nil) }

func SummarizeWithExit(raw string, exit int) string { return summarize(raw, &exit) }

const maxSummaryBytes = 12000

var (
	failTestRE = regexp.MustCompile(`^--- FAIL: (.+) \([0-9.]+s\)$`)
	failPkgRE  = regexp.MustCompile(`^FAIL(?:\s+(\S+))?(?:\s|$)`)
	passRE     = regexp.MustCompile(`^(?:ok\s|PASS$|\?\s+\S+\s+\[no test files\])`)
	refRE      = regexp.MustCompile(`([A-Za-z0-9_./\\:-]+\.go:\d+)`)
)

func summarize(raw string, exit *int) string {
	lines := strings.Split(raw, "\n")
	var failedPkgs, failedTests, refs, panics, expected, diagnostics []string
	failure, passed := false, false
	omitted := 0
	seen := map[string]bool{}
	add := func(dst *[]string, value string, limit int, kind string) {
		key := kind + "\x00" + value
		if seen[key] {
			return
		}
		seen[key] = true
		if len(*dst) >= limit {
			omitted++
			return
		}
		if len(value) > 500 {
			value = value[:500] + " ... [line shortened]"
			omitted++
		}
		*dst = append(*dst, value)
	}
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if m := failTestRE.FindStringSubmatch(t); m != nil {
			failure = true
			add(&failedTests, m[1], 20, "test")
		}
		if m := failPkgRE.FindStringSubmatch(t); m != nil {
			failure = true
			if m[1] != "" {
				add(&failedPkgs, m[1], 10, "pkg")
			}
		}
		passed = passed || passRE.MatchString(t)
		low := strings.ToLower(t)
		isPanic := strings.HasPrefix(t, "panic:") || strings.HasPrefix(t, "fatal error:")
		if isPanic {
			failure = true
			add(&panics, t, 8, "panic")
		}
		if strings.Contains(low, "expected") || strings.Contains(low, "actual") || strings.Contains(low, "got ") || strings.Contains(low, "want ") {
			add(&expected, t, 12, "message")
		}
		matches := refRE.FindAllString(t, -1)
		for _, m := range matches {
			add(&refs, m, 10, "ref")
		}
		// Keep compiler/setup diagnostics, not just their source locations.
		if len(matches) > 0 || strings.HasPrefix(t, "# ") || strings.HasPrefix(t, "go:") {
			add(&diagnostics, t, 8, "diagnostic")
		}
	}
	if exit != nil {
		failure = *exit != 0
		passed = *exit == 0
	}
	var b strings.Builder
	if failure {
		b.WriteString("FAIL")
		if len(failedPkgs) > 0 {
			b.WriteString(" " + strings.Join(failedPkgs, ", "))
		}
		if exit != nil {
			fmt.Fprintf(&b, " (exit %d)", *exit)
		}
		b.WriteByte('\n')
		// Launch/setup errors can lack a Go diagnostic prefix. Retain bounded
		// initial output so a nonzero exit always has an explanation when available.
		if len(diagnostics) == 0 && len(failedTests) == 0 && len(panics) == 0 && len(expected) == 0 {
			for _, line := range lines {
				if t := strings.TrimSpace(line); t != "" {
					add(&diagnostics, t, 8, "diagnostic")
				}
			}
		}
	} else if passed {
		b.WriteString("Tests passed.\n")
	} else {
		b.WriteString("Test status unknown (no result records).\n")
	}
	for _, section := range []struct {
		name  string
		lines []string
	}{
		{"Failed tests", failedTests}, {"Diagnostics", diagnostics}, {"Key messages", expected}, {"Panic lines", panics}, {"Relevant file references", refs},
	} {
		if len(section.lines) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n%s:\n", section.name)
		for _, line := range section.lines {
			fmt.Fprintf(&b, "- %s\n", line)
		}
	}
	if omitted > 0 {
		fmt.Fprintf(&b, "\nOmitted: %d additional or oversized diagnostic items.\n", omitted)
	}
	if len(lines) > 80 {
		fmt.Fprintf(&b, "Raw output summarized: %d lines (request raw output for full details).\n", len(lines))
	}
	result := b.String()
	if len(result) > maxSummaryBytes {
		result = result[:maxSummaryBytes-100] + "\nOmitted: additional diagnostics due to summary size limit.\n"
	}
	return result
}

func WriteSummary(w io.Writer, raw string) { _, _ = io.WriteString(w, Summarize(raw)) }
