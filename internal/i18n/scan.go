package i18n

import (
	"fmt"
	"strings"

	"asoul/internal/model"
)

// RenderScanScope renders a localized one-line description of an upstream skill scan scope.
func RenderScanScope(roots, exclude []string) string {
	scan := model.ScanConfig{Roots: roots, Exclude: exclude}
	switch scan.Mode() {
	case model.ScanModeWhole:
		return T("upstream.scan.mode.whole")
	case model.ScanModeDefault:
		return T("upstream.scan.mode.default")
	default:
		return fmt.Sprintf(T("upstream.scan.mode.custom"), strings.Join(scan.Normalized().Roots, ", "))
	}
}

// RenderScanScopeWithExclude renders the scan scope and appends excluded directories when present.
func RenderScanScopeWithExclude(roots, exclude []string) string {
	text := RenderScanScope(roots, exclude)
	if normalized := (model.ScanConfig{Roots: roots, Exclude: exclude}).Normalized(); len(normalized.Exclude) > 0 {
		text += " • " + fmt.Sprintf(T("upstream.scan.exclude_inline"), strings.Join(normalized.Exclude, ", "))
	}
	return text
}
