package diffsummary

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"goctx/internal/scanner"
)

type BaselineEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

func Summarize(root string, w io.Writer) error {
	base := filepath.Join(root, ".goctx", "baseline.json")
	f, err := os.Open(base)
	if os.IsNotExist(err) {
		fmt.Fprintln(w, "No .goctx baseline found. Diff summary is unavailable in MVP until a baseline is created. This command intentionally does not use git commands.")
		fmt.Fprintln(w, "Changed-file summary requires baseline metadata. A future goctx snapshot command can create it.")
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	var entries []BaselineEntry
	if err := json.NewDecoder(f).Decode(&entries); err != nil {
		return err
	}
	old := map[string]BaselineEntry{}
	for _, e := range entries {
		old[e.Path] = e
	}
	sc, err := scanner.Scan(root)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "Diff summary from .goctx/baseline.json:")
	for _, cur := range sc.Files {
		if cur.IgnoredByDefault {
			continue
		}
		h, err := hashFile(filepath.Join(root, filepath.FromSlash(cur.Path)))
		if err != nil {
			continue
		}
		if prev, ok := old[cur.Path]; !ok {
			fmt.Fprintf(w, "- added: %s\n", cur.Path)
		} else if prev.SHA256 != h {
			fmt.Fprintf(w, "- changed: %s\n", cur.Path)
		}
	}
	return nil
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:]), nil
}
