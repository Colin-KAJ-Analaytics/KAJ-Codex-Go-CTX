package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const ClaudeTemplate = "@AGENTS.md\n"

// InitClaude keeps shared rules in AGENTS.md and adds only an import to Claude's
// project instructions. Existing shared rules are never replaced by this command.
func InitClaude(root string, force, strict bool) ([]FileResult, error) {
	var results []FileResult
	agents := AgentsTemplate
	if strict {
		agents = StrictAgentsTemplate
	}
	agentsPath := filepath.Join(root, "AGENTS.md")
	if _, err := os.Lstat(agentsPath); err == nil {
		info, err := os.Stat(agentsPath)
		if err != nil || !info.Mode().IsRegular() {
			return results, fmt.Errorf("AGENTS.md must resolve to a readable regular file")
		}
		if _, err := os.ReadFile(agentsPath); err != nil {
			return results, err
		}
		results = append(results, FileResult{Path: "AGENTS.md", Skipped: true})
	} else if !os.IsNotExist(err) {
		return results, err
	} else {
		if err := os.MkdirAll(root, 0755); err != nil {
			return results, err
		}
		if err := os.WriteFile(agentsPath, []byte(agents), 0644); err != nil {
			return results, err
		}
		results = append(results, FileResult{Path: "AGENTS.md", Created: true})
	}
	rel, importLine := "CLAUDE.md", "@AGENTS.md"
	// Use an existing .claude entry point instead of shadowing it with a new one.
	if _, err := os.Lstat(filepath.Join(root, rel)); os.IsNotExist(err) {
		if _, err := os.Lstat(filepath.Join(root, ".claude", "CLAUDE.md")); err == nil {
			rel, importLine = ".claude/CLAUDE.md", "@../AGENTS.md"
		}
	}
	path := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(path)
	if err != nil && !os.IsNotExist(err) {
		return results, err
	}
	if err == nil && !info.Mode().IsRegular() {
		return results, fmt.Errorf("refusing to replace non-regular Claude instructions: %s", path)
	}
	body := importLine + "\n"
	existed := err == nil
	if existed && !force {
		data, err := os.ReadFile(path)
		if err != nil {
			return results, err
		}
		if hasImport(string(data), importLine) {
			return append(results, FileResult{Path: rel, Skipped: true}), nil
		}
		// Append without rewriting or discarding the user's instructions.
		body = string(data)
		if body != "" && !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		body += "\n" + importLine + "\n"
	}
	mode := os.FileMode(0644)
	if existed {
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		return results, err
	}
	return append(results, FileResult{Path: rel, Created: !existed, Updated: existed}), nil
}

// Ignore examples inside fenced code blocks and inline code spans.
func hasImport(body, importLine string) bool {
	fence := ""
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			marker := line[:3]
			if fence == "" {
				fence = marker
			} else if fence == marker {
				fence = ""
			}
			continue
		}
		if fence == "" && line == importLine {
			return true
		}
	}
	return false
}
