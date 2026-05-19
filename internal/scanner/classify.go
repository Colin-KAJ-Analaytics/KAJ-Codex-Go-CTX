package scanner

import (
	"path/filepath"
	"strings"
)

const largeFileBytes = 256 * 1024

func Classify(root, rel string, size int64) FileInfo {
	rel = filepath.ToSlash(rel)
	base := filepath.Base(rel)
	lower := strings.ToLower(rel)
	info := FileInfo{Path: rel, SizeBytes: size}
	switch {
	case base == "go.sum":
		info.Kind, info.IgnoredByDefault, info.Reason = KindDependencyLock, true, "dependency lock, high token noise"
	case isGeneratedName(base):
		info.Kind, info.IgnoredByDefault, info.Reason = KindGenerated, true, "generated file"
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
		info.IgnoredByDefault = size > largeFileBytes
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

func isGeneratedName(base string) bool {
	return strings.HasSuffix(base, ".pb.go") || strings.HasSuffix(base, "_generated.go") || strings.HasSuffix(base, ".gen.go")
}

func isMigration(path string) bool {
	return strings.Contains(path, "/migrations/") || strings.HasPrefix(path, "migrations/") || strings.Contains(path, "/db/migrations/") || strings.Contains(path, "/sql/") || strings.HasPrefix(path, "sql/") || strings.HasSuffix(path, ".sql")
}

func isFixture(path string) bool {
	return strings.Contains(path, "/testdata/") || strings.Contains(path, "/fixtures/") || strings.HasPrefix(path, "testdata/") || strings.HasPrefix(path, "fixtures/")
}
