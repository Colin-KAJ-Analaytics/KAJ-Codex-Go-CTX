package scanner

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type FileKind string

const (
	KindGoSource       FileKind = "go_source"
	KindGoTest         FileKind = "go_test"
	KindGenerated      FileKind = "generated"
	KindVendor         FileKind = "vendor"
	KindBuildArtifact  FileKind = "build_artifact"
	KindDependencyLock FileKind = "dependency_lock"
	KindMigration      FileKind = "migration"
	KindFixture        FileKind = "fixture"
	KindDocumentation  FileKind = "documentation"
	KindUnknown        FileKind = "unknown"
)

type FileInfo struct {
	Path             string
	Kind             FileKind
	SizeBytes        int64
	IgnoredByDefault bool
	Reason           string
}

type Result struct {
	Root  string
	Files []FileInfo
}

func Scan(root string) (Result, error) {
	var files []FileInfo
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if ignoreDir(rel) {
				info, statErr := d.Info()
				size := int64(0)
				if statErr == nil {
					size = info.Size()
				}
				files = append(files, FileInfo{Path: rel + "/", Kind: dirKind(rel), SizeBytes: size, IgnoredByDefault: true, Reason: ignoreDirReason(rel)})
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files = append(files, Classify(root, rel, info.Size()))
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return Result{Root: root, Files: files}, err
}

func (r Result) IncludedGoFiles() []string {
	var out []string
	for _, f := range r.Files {
		if !f.IgnoredByDefault && (f.Kind == KindGoSource || f.Kind == KindGoTest) {
			out = append(out, filepath.Join(r.Root, filepath.FromSlash(f.Path)))
		}
	}
	return out
}

func HasGeneratedHeader(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	return ReaderHasGeneratedHeader(f)
}

func ReaderHasGeneratedHeader(r io.Reader) bool {
	sc := bufio.NewScanner(io.LimitReader(r, 8192))
	for i := 0; i < 30 && sc.Scan(); i++ {
		line := sc.Text()
		if strings.Contains(line, "Code generated") && strings.Contains(line, "DO NOT EDIT") {
			return true
		}
	}
	return false
}
