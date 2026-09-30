package model

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// Default upstream scan scope. Paths are relative to the upstream source root.
const (
	// ScanRootDefault is the directory scanned when an upstream does not configure a scope.
	ScanRootDefault = "skills"
	// ScanRootWholeSource selects the entire upstream source.
	ScanRootWholeSource = "."
)

// ScanMode describes how an upstream scan scope is expressed, for display purposes.
type ScanMode string

const (
	// ScanModeDefault uses the built-in default directory (skills).
	ScanModeDefault ScanMode = "default"
	// ScanModeCustom uses one or more user-provided directories.
	ScanModeCustom ScanMode = "custom"
	// ScanModeWhole scans the entire upstream source.
	ScanModeWhole ScanMode = "whole"
)

// ScanConfig defines which directories inside an upstream source are scanned for skills.
// Directory paths are relative to the source root and are matched with path boundaries.
type ScanConfig struct {
	Roots   []string `json:"roots,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

// DefaultScanConfig returns the built-in scan scope used when an upstream does not configure one.
func DefaultScanConfig() ScanConfig {
	return ScanConfig{Roots: []string{ScanRootDefault}}
}

// NormalizeScanPath cleans a single configured directory path. It reports false when the
// value is empty, absolute, escapes the source root, or is otherwise not a usable relative path.
func NormalizeScanPath(raw string) (string, bool) {
	p := strings.TrimSpace(raw)
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimSpace(p)
	if p == "" {
		return "", false
	}
	if p == "." || p == "./" {
		return ScanRootWholeSource, true
	}
	// Reject absolute and Windows drive-qualified paths before trimming separators so
	// "/etc" cannot silently become the relative path "etc".
	if strings.HasPrefix(p, "/") {
		return "", false
	}
	if len(p) >= 2 && p[1] == ':' {
		return "", false
	}
	p = strings.Trim(p, "/")
	if p == "" {
		return "", false
	}
	p = path.Clean(p)
	if p == ScanRootWholeSource {
		return ScanRootWholeSource, true
	}
	if p == ".." || strings.HasPrefix(p, "../") {
		return "", false
	}
	return p, true
}

// NormalizeScanConfig validates and canonicalizes a scan scope. When no roots are
// configured the built-in default is applied. The whole-source marker (".") may only
// appear alone, and cannot be used as an exclude entry.
func NormalizeScanConfig(scan ScanConfig) (ScanConfig, error) {
	roots, err := normalizeScanList(scan.Roots)
	if err != nil {
		return ScanConfig{}, err
	}
	exclude, err := normalizeScanList(scan.Exclude)
	if err != nil {
		return ScanConfig{}, err
	}

	if len(roots) == 0 {
		roots = []string{ScanRootDefault}
	}

	hasWhole := false
	for _, r := range roots {
		if r == ScanRootWholeSource {
			hasWhole = true
			break
		}
	}
	if hasWhole && len(roots) > 1 {
		return ScanConfig{}, fmt.Errorf("scan root %q cannot be combined with other roots", ScanRootWholeSource)
	}

	for _, e := range exclude {
		if e == ScanRootWholeSource {
			return ScanConfig{}, fmt.Errorf("scan exclude cannot be %q", ScanRootWholeSource)
		}
	}

	return ScanConfig{Roots: roots, Exclude: exclude}, nil
}

func normalizeScanList(values []string) ([]string, error) {
	var out []string
	seen := make(map[string]bool)
	for _, raw := range values {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		clean, ok := NormalizeScanPath(trimmed)
		if !ok {
			return nil, fmt.Errorf("invalid scan path %q: must be a relative path inside the source", trimmed)
		}
		if seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	return out, nil
}

// Normalized returns the canonical scan scope, falling back to the default when invalid or empty.
func (c ScanConfig) Normalized() ScanConfig {
	n, err := NormalizeScanConfig(c)
	if err != nil {
		return DefaultScanConfig()
	}
	return n
}

// Stored returns the value to persist for this scope: nil when it equals the built-in
// default, otherwise a normalized copy. It reports invalid scopes instead of silently
// falling back to the default.
func (c ScanConfig) Stored() (*ScanConfig, error) {
	n, err := NormalizeScanConfig(c)
	if err != nil {
		return nil, err
	}
	if n.Mode() == ScanModeDefault {
		return nil, nil
	}
	return &n, nil
}

// Mode classifies the scan scope for display.
func (c ScanConfig) Mode() ScanMode {
	n := c.Normalized()
	if len(n.Roots) == 1 && n.Roots[0] == ScanRootWholeSource {
		return ScanModeWhole
	}
	if len(n.Roots) == 1 && n.Roots[0] == ScanRootDefault && len(n.Exclude) == 0 {
		return ScanModeDefault
	}
	return ScanModeCustom
}

// NormalizeScanPathForMatch canonicalizes a source-relative directory path for scope matching.
func NormalizeScanPathForMatch(relDir string) string {
	p := filepath.ToSlash(strings.TrimSpace(relDir))
	p = strings.Trim(p, "/")
	if p == "" || p == "." {
		return ""
	}
	return path.Clean(p)
}

// InScanScope reports whether a source-relative directory path falls inside the configured
// scan scope. Exclusion wins over inclusion, and entries match on path boundaries so
// "skills" never matches "skills-backup".
func InScanScope(relDir string, roots, exclude []string) bool {
	rel := NormalizeScanPathForMatch(relDir)
	if !scanListMatches(rel, roots) {
		return false
	}
	if scanListMatches(rel, exclude) {
		return false
	}
	return true
}

func scanListMatches(rel string, bases []string) bool {
	for _, raw := range bases {
		base := strings.TrimSpace(raw)
		base = strings.ReplaceAll(base, "\\", "/")
		base = strings.Trim(base, "/")
		if base == "" {
			continue
		}
		if base == ScanRootWholeSource {
			return true
		}
		base = path.Clean(base)
		if rel == base || strings.HasPrefix(rel, base+"/") {
			return true
		}
	}
	return false
}
