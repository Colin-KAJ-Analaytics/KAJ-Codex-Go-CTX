package scanner

import (
	"os"
	"path/filepath"
	"strings"
)

const largeFileBytes = 256 * 1024

func Classify(root, rel string, size int64) FileInfo {
	return ClassifyWithConfig(root, rel, size, LoadConfig(root))
}

func ClassifyWithConfig(root, rel string, size int64, cfg Config) FileInfo {
	rel = filepath.ToSlash(rel)
	base := filepath.Base(rel)
	lower := strings.ToLower(rel)
	info := FileInfo{Path: rel, SizeBytes: size}
	switch {
	case base == "go.sum":
		info.Kind, info.IgnoredByDefault, info.Reason = KindDependencyLock, true, "dependency lock, high token noise"
	case base == ".DS_Store":
		info.Kind, info.IgnoredByDefault, info.Reason = KindBuildArtifact, true, "platform metadata"
	case matchesAny(base, rel, cfg.IgnoreFiles):
		info.Kind, info.IgnoredByDefault, info.Reason = KindGenerated, true, "generated file"
	case looksLikeBinaryArtifact(root, rel, size):
		info.Kind, info.IgnoredByDefault, info.Reason = KindBuildArtifact, true, "binary/build artifact"
	case strings.HasPrefix(base, "mock_"):
		info.Kind, info.IgnoredByDefault, info.Reason = KindGenerated, true, "mock/generated-like file"
	case isMigration(lower):
		info.Kind = KindMigration
		info.Reason = "migration/schema file"
		info.IgnoredByDefault = size > largeFileBytes
		if info.IgnoredByDefault {
			info.Reason = "large migration/schema dump"
		}
	case isFixture(lower):
		info.Kind = KindFixture
		info.Reason = "fixture file"
		info.IgnoredByDefault = size > cfg.MaxFixtureBytes
		if info.IgnoredByDefault {
			info.Reason = "large fixture file"
		}
	case strings.HasSuffix(base, "_test.go"):
		info.Kind = KindGoTest
	case strings.HasSuffix(base, ".go"):
		if HasGeneratedHeader(filepath.Join(root, filepath.FromSlash(rel))) {
			info.Kind, info.IgnoredByDefault, info.Reason = KindGenerated, true, "generated file header"
		} else {
			info.Kind = KindGoSource
		}
	case strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".txt") || strings.HasSuffix(lower, ".rst"):
		info.Kind = KindDocumentation
	default:
		info.Kind = KindUnknown
	}
	return info
}

func looksLikeBinaryArtifact(root, rel string, size int64) bool {
	if size < 1024*1024 || filepath.Ext(rel) != "" {
		return false
	}
	data := make([]byte, 4)
	f, err := os.Open(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return false
	}
	defer f.Close()
	n, _ := f.Read(data)
	if n < 4 {
		return false
	}
	return string(data[:4]) == "\x7fELF" || string(data[:4]) == "\xcf\xfa\xed\xfe" || string(data[:4]) == "\xfe\xed\xfa\xcf" || string(data[:2]) == "MZ"
}

func isGeneratedName(base string) bool {
	return strings.HasSuffix(base, ".pb.go") || strings.HasSuffix(base, "_generated.go") || strings.HasSuffix(base, ".gen.go")
}

func isMigration(path string) bool {
	return strings.Contains(path, "/migrations/") || strings.HasPrefix(path, "migrations/") || strings.Contains(path, "/db/migrations/") || strings.Contains(path, "/sql/") || strings.HasPrefix(path, "sql/") || strings.HasSuffix(path, ".sql")
}

func isFixture(path string) bool {
	return strings.Contains(path, "/testdata/") || strings.Contains(path, "/fixtures/") || strings.HasPrefix(path, "testdata/") || strings.HasPrefix(path, "fixtures/")
}

func matchesAny(base, rel string, patterns []string) bool {
	for _, p := range patterns {
		if ok, _ := filepath.Match(p, base); ok {
			return true
		}
		if ok, _ := filepath.Match(filepath.ToSlash(p), rel); ok {
			return true
		}
	}
	return isGeneratedName(base)
}
