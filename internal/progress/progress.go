// Package progress defines transport-neutral progress reporting shared by the
// git layer, the application service and the TUI/CLI presentation layers.
//
// It deliberately does not import internal/i18n: the application layer must not
// localize text, so callers emit numeric counts plus a non-localized phase enum
// and the presentation layer is responsible for rendering localized labels.
package progress

import (
	"regexp"
	"strconv"
	"strings"
)

// Phase identifies which stage of a long-running operation is reporting.
type Phase int

const (
	PhaseUnknown Phase = iota
	PhaseClone
	PhaseFetch
	PhaseCounting
	PhaseCompressing
	PhaseReceiving
	PhaseResolving
	PhaseExtract
	PhaseValidate
	PhaseInstall
	PhaseCheck
	PhaseRemove
	PhaseDeploy
	PhaseUpdate
	PhaseScan
)

// Update is a single progress report. Current/Total are meaningful only when
// Total > 0. Text carries an optional non-localized detail such as a skill ID,
// repository URL or raw tool line.
type Update struct {
	Phase   Phase
	Current int
	Total   int
	Text    string
}

// Determinate reports whether the update carries a reliable completion ratio.
func (u Update) Determinate() bool {
	return u.Total > 0 && u.Current >= 0
}

// I18nKey returns the dictionary key for a phase label, or "" when the phase
// has no dedicated label. Keeping the key here lets both presentation layers
// localize without the application layer importing internal/i18n.
func (p Phase) I18nKey() string {
	switch p {
	case PhaseClone:
		return "progress.phase.clone"
	case PhaseFetch:
		return "progress.phase.fetch"
	case PhaseCounting:
		return "progress.phase.counting"
	case PhaseCompressing:
		return "progress.phase.compressing"
	case PhaseReceiving:
		return "progress.phase.receiving"
	case PhaseResolving:
		return "progress.phase.resolving"
	case PhaseExtract:
		return "progress.phase.extract"
	case PhaseValidate:
		return "progress.phase.validate"
	case PhaseInstall:
		return "progress.phase.install"
	case PhaseCheck:
		return "progress.phase.check"
	case PhaseRemove:
		return "progress.phase.remove"
	case PhaseDeploy:
		return "progress.phase.deploy"
	case PhaseUpdate:
		return "progress.phase.update"
	case PhaseScan:
		return "progress.phase.scan"
	default:
		return ""
	}
}

// Func receives progress updates. A nil Func is always safe to call.
type Func func(Update)

// Send invokes fn when it is non-nil.
func Send(fn Func, u Update) {
	if fn != nil {
		fn(u)
	}
}

// First returns the first non-nil callback from the optional variadic list, or
// nil when none was supplied.
func First(fns ...Func) Func {
	for _, fn := range fns {
		if fn != nil {
			return fn
		}
	}
	return nil
}

var gitProgressRe = regexp.MustCompile(`(?i)^(?:\s*remote:\s*)?([a-z][a-z ]*?):\s*(\d+)%\s*\((\d+)/(\d+)\)`)

// ParseGitLine parses a single git stderr progress line (for example
// "Receiving objects:  45% (100/200), 1.2 MiB"). It returns false when the line
// does not carry parseable progress so callers can fall back to raw text.
func ParseGitLine(line string) (Update, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return Update{}, false
	}

	if m := gitProgressRe.FindStringSubmatch(line); m != nil {
		phase := gitPhase(strings.ToLower(strings.TrimSpace(m[1])))
		pct, _ := strconv.Atoi(m[2])
		cur, _ := strconv.Atoi(m[3])
		total, _ := strconv.Atoi(m[4])
		// Respect git's own percentage when rounding diverges.
		if total > 0 && pct > 0 && cur == 0 {
			cur = total * pct / 100
		}
		return Update{Phase: phase, Current: cur, Total: total, Text: line}, true
	}

	lower := strings.ToLower(line)
	switch {
	case strings.HasPrefix(lower, "cloning into"):
		return Update{Phase: PhaseClone, Text: line}, true
	case strings.HasPrefix(lower, "from "), strings.HasPrefix(lower, "remote: from "):
		return Update{Phase: PhaseFetch, Text: line}, true
	}

	return Update{}, false
}

func gitPhase(name string) Phase {
	switch {
	case strings.Contains(name, "counting"):
		return PhaseCounting
	case strings.Contains(name, "compressing"):
		return PhaseCompressing
	case strings.Contains(name, "receiving"):
		return PhaseReceiving
	case strings.Contains(name, "resolving"):
		return PhaseResolving
	default:
		return PhaseUnknown
	}
}
