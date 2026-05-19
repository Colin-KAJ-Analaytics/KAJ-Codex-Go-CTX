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

func ignoreDir(rel string) bool {
	for _, p := range strings.Split(rel, "/") {
		if _, ok := ignoredDirs[p]; ok {
			return true
		}
	}
	return false
}

func dirKind(rel string) FileKind {
	for _, p := range strings.Split(rel, "/") {
		if k, ok := ignoredDirs[p]; ok {
			return k
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
