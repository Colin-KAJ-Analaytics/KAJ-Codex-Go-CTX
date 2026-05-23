package baseline

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"goctx/internal/scanner"
)

type FileEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Kind   string `json:"kind"`
}

type Snapshot struct {
	Timestamp string      `json:"timestamp"`
	Root      string      `json:"root"`
	Files     []FileEntry `json:"files"`
}

func Create(root string) (Snapshot, error) {
	sc, err := scanner.Scan(root)
	if err != nil {
		return Snapshot{}, err
	}
	s := Snapshot{Timestamp: time.Now().UTC().Format(time.RFC3339), Root: filepath.ToSlash(root)}
	for _, f := range sc.Files {
		if f.IgnoredByDefault {
			continue
		}
		h, err := hashFile(filepath.Join(root, filepath.FromSlash(f.Path)))
		if err != nil {
			continue
		}
		s.Files = append(s.Files, FileEntry{Path: f.Path, SHA256: h, Size: f.SizeBytes, Kind: string(f.Kind)})
	}
	return s, nil
}

func Write(root string, s Snapshot) error {
	dir := filepath.Join(root, ".goctx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "baseline.json"), append(data, '\n'), 0644)
}

func Load(root string) (Snapshot, error) {
	data, err := os.ReadFile(filepath.Join(root, ".goctx", "baseline.json"))
	if err != nil {
		return Snapshot{}, err
	}
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		var legacy []FileEntry
		if err2 := json.Unmarshal(data, &legacy); err2 != nil {
			return Snapshot{}, err
		}
		s.Files = legacy
	}
	return s, nil
}

func Status(root string, w io.Writer) error {
	s, err := Load(root)
	if os.IsNotExist(err) {
		fmt.Fprintln(w, "WARN .goctx/baseline.json missing")
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "OK   .goctx/baseline.json found")
	fmt.Fprintf(w, "Files: %d\n", len(s.Files))
	if s.Timestamp != "" {
		fmt.Fprintf(w, "Timestamp: %s\n", s.Timestamp)
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
