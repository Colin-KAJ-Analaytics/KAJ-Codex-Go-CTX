package codex

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type FileResult struct {
	Path    string
	Created bool
	Skipped bool
}

func Init(root string, force bool) ([]FileResult, error) {
	files := map[string]string{
		"AGENTS.md": AgentsTemplate,
		".agents/skills/go-token-efficient/SKILL.md":      SkillTemplate,
		".agents/skills/go-token-efficient/scripts/goctx": ScriptTemplate,
		".codex/GOCTX_HOOKS.md":                           HooksTemplate,
		".codex/config.example.toml":                      ConfigTemplate,
	}
	order := []string{"AGENTS.md", ".agents/skills/go-token-efficient/SKILL.md", ".agents/skills/go-token-efficient/scripts/goctx", ".codex/GOCTX_HOOKS.md", ".codex/config.example.toml"}
	var results []FileResult
	for _, rel := range order {
		body := files[rel]
		path := filepath.Join(root, filepath.FromSlash(rel))
		if _, err := os.Stat(path); err == nil && !force {
			results = append(results, FileResult{Path: rel, Skipped: true})
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return results, err
		}
		mode := fs.FileMode(0644)
		if filepath.Base(path) == "goctx" {
			mode = 0755
		}
		if err := os.WriteFile(path, []byte(body), mode); err != nil {
			return results, err
		}
		results = append(results, FileResult{Path: rel, Created: true})
	}
	return results, nil
}

func FormatResults(results []FileResult) string {
	out := ""
	for _, r := range results {
		if r.Skipped {
			out += fmt.Sprintf("skipped: %s\n", r.Path)
		} else if r.Created {
			out += fmt.Sprintf("created: %s\n", r.Path)
		}
	}
	return out
}
