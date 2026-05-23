package scanner

import "strings"

var ignoredDirs = map[string]FileKind{
	"vendor":       KindVendor,
	"node_modules": KindBuildArtifact,
	"dist":         KindBuildArtifact,
	"build":        KindBuildArtifact,
	"coverage":     KindBuildArtifact,
	"tmp":          KindBuildArtifact,
	"bin":          KindBuildArtifact,
	".next":        KindBuildArtifact,
	".cache":       KindBuildArtifact,
	".git":         KindBuildArtifact,
}

func ignoreDir(rel string, cfg Config) bool {
	for _, p := range strings.Split(rel, "/") {
		if containsDir(cfg.IgnoreDirs, p) {
			return true
		}
	}
	return false
}

func dirKind(rel string, cfg Config) FileKind {
	for _, p := range strings.Split(rel, "/") {
		if k, ok := ignoredDirs[p]; ok {
			return k
		}
		if containsDir(cfg.IgnoreDirs, p) {
			return KindBuildArtifact
		}
	}
	return KindBuildArtifact
}

func ignoreDirReason(rel string) string {
	if strings.Contains(rel, "vendor") {
		return "vendored dependency"
	}
	return "build/dependency output"
}

func containsDir(dirs []string, name string) bool {
	for _, d := range dirs {
		if strings.TrimSuffix(d, "/") == name {
			return true
		}
	}
	return false
}
