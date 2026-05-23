package diffsummary

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"goctx/internal/baseline"
	"goctx/internal/scanner"
)

func Summarize(root string, w io.Writer) error {
	snap, err := baseline.Load(root)
	if os.IsNotExist(err) {
		fmt.Fprintln(w, "No .goctx baseline found. Diff summary is unavailable in MVP until a baseline is created. This command intentionally does not use git commands.")
		fmt.Fprintln(w, "Run: goctx baseline create")
		return nil
	}
	if err != nil {
		return err
	}
	old := map[string]baseline.FileEntry{}
	for _, e := range snap.Files {
		old[e.Path] = e
	}
	sc, err := scanner.Scan(root)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "Diff summary from .goctx/baseline.json:")
	changed := 0
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
			changed++
		} else if prev.SHA256 != h {
			fmt.Fprintf(w, "- changed: %s\n", cur.Path)
			changed++
		}
	}
	if changed == 0 {
		fmt.Fprintln(w, "- no changes detected")
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
