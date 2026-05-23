package scanner

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	IgnoreDirs      []string
	IgnoreFiles     []string
	MaxFileBytes    int64
	MaxFixtureBytes int64
	Warnings        []string
}

func DefaultConfig() Config {
	return Config{
		IgnoreDirs:      []string{"vendor", "node_modules", "dist", "build", "coverage", "tmp", "bin", ".next", ".cache", ".git", ".goctx"},
		IgnoreFiles:     []string{"*.pb.go", "*.gen.go", "*_generated.go", "wire_gen.go", "mock_*.go", ".DS_Store"},
		MaxFileBytes:    262144,
		MaxFixtureBytes: 65536,
	}
}

func LoadConfig(root string) Config {
	cfg := DefaultConfig()
	if err := loadGoctxYAML(filepath.Join(root, ".goctx.yaml"), &cfg); err != nil && !os.IsNotExist(err) {
		cfg.Warnings = append(cfg.Warnings, fmt.Sprintf(".goctx.yaml ignored: %v", err))
	}
	for _, name := range []string{".goctxignore", ".gitignore"} {
		if err := loadIgnoreFile(filepath.Join(root, name), &cfg); err != nil && !os.IsNotExist(err) {
			cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("%s ignored: %v", name, err))
		}
	}
	return cfg
}

func loadGoctxYAML(path string, cfg *Config) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	section := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || line == "ignore:" {
			continue
		}
		switch line {
		case "dirs:":
			section = "dirs"
			continue
		case "files:":
			section = "files"
			continue
		}
		if strings.HasPrefix(line, "max_file_bytes:") {
			n, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "max_file_bytes:")), 10, 64)
			if err != nil {
				return err
			}
			cfg.MaxFileBytes = n
			continue
		}
		if strings.HasPrefix(line, "max_fixture_bytes:") {
			n, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "max_fixture_bytes:")), 10, 64)
			if err != nil {
				return err
			}
			cfg.MaxFixtureBytes = n
			continue
		}
		if strings.HasPrefix(line, "- ") {
			item := strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "- ")), "\"'")
			if section == "dirs" {
				cfg.IgnoreDirs = appendUniqueString(cfg.IgnoreDirs, strings.TrimSuffix(item, "/"))
			} else if section == "files" {
				cfg.IgnoreFiles = appendUniqueString(cfg.IgnoreFiles, item)
			}
		}
	}
	return sc.Err()
}

func loadIgnoreFile(path string, cfg *Config) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		line = strings.TrimPrefix(line, "/")
		if strings.HasSuffix(line, "/") || !strings.ContainsAny(line, "*?[") {
			cfg.IgnoreDirs = appendUniqueString(cfg.IgnoreDirs, strings.TrimSuffix(line, "/"))
		} else {
			cfg.IgnoreFiles = appendUniqueString(cfg.IgnoreFiles, line)
		}
	}
	return sc.Err()
}

func appendUniqueString(xs []string, x string) []string {
	if x == "" {
		return xs
	}
	for _, v := range xs {
		if v == x {
			return xs
		}
	}
	return append(xs, x)
}
