package model

import (
	"fmt"
	"strings"
)

// Supported format identifiers
const (
	FormatAuto     = "auto"
	FormatStandard = "standard"
	FormatClaude   = "claude"
	FormatCline    = "cline"
	FormatCursor   = "cursor"
	FormatGemini   = "gemini"
	FormatWindsurf = "windsurf"
	FormatCopilot  = "copilot"
	FormatOpencode = "opencode"
	FormatRoo      = "roo"
	FormatPi       = "pi"
	FormatFactory  = "factory"
)

// FormatDef describes a project-level distribution format.
type FormatDef struct {
	ID          string   `json:"id"`
	Subpath     string   `json:"subpath"`     // relative path in project, e.g. ".agents/skills"
	ProbeDirs   []string `json:"probeDirs"`   // project dirs that indicate this agent is used
	Description string   `json:"description"` // human readable description
}

// BuiltinFormats defines format registry for project-level distribution
var BuiltinFormats = []FormatDef{
	{
		ID:          FormatStandard,
		Subpath:     ".agents/skills",
		ProbeDirs:   []string{".agents", ".agents/skills"},
		Description: "Standard Agent Skills (.agents/skills/) - Codex, Antigravity, Cursor, Windsurf, Copilot, etc.",
	},
	{
		ID:          FormatClaude,
		Subpath:     ".claude/skills",
		ProbeDirs:   []string{".claude", ".claude/skills"},
		Description: "Claude Code (.claude/skills/)",
	},
	{
		ID:          FormatCline,
		Subpath:     ".cline/skills",
		ProbeDirs:   []string{".cline", ".cline/skills"},
		Description: "Cline (.cline/skills/)",
	},
	{
		ID:          FormatCursor,
		Subpath:     ".cursor/skills",
		ProbeDirs:   []string{".cursor", ".cursor/skills"},
		Description: "Cursor Native (.cursor/skills/)",
	},
	{
		ID:          FormatGemini,
		Subpath:     ".gemini/skills",
		ProbeDirs:   []string{".gemini", ".gemini/skills"},
		Description: "Gemini CLI Native (.gemini/skills/)",
	},
	{
		ID:          FormatWindsurf,
		Subpath:     ".windsurf/skills",
		ProbeDirs:   []string{".windsurf", ".windsurf/skills"},
		Description: "Windsurf Native (.windsurf/skills/)",
	},
	{
		ID:          FormatCopilot,
		Subpath:     ".github/skills",
		ProbeDirs:   []string{".github", ".github/skills"},
		Description: "GitHub Copilot Native (.github/skills/)",
	},
	{
		ID:          FormatOpencode,
		Subpath:     ".opencode/skills",
		ProbeDirs:   []string{".opencode", ".opencode/skills"},
		Description: "OpenCode Native (.opencode/skills/)",
	},
	{
		ID:          FormatRoo,
		Subpath:     ".roo/skills",
		ProbeDirs:   []string{".roo", ".roo/skills"},
		Description: "Roo Code Native (.roo/skills/)",
	},
	{
		ID:          FormatPi,
		Subpath:     ".pi/skills",
		ProbeDirs:   []string{".pi", ".pi/skills"},
		Description: "Pi Native (.pi/skills/)",
	},
	{
		ID:          FormatFactory,
		Subpath:     ".factory/skills",
		ProbeDirs:   []string{".factory", ".factory/skills"},
		Description: "Factory Droid Native (.factory/skills/)",
	},
}

// LookupFormat finds the FormatDef by ID.
func LookupFormat(id string) (*FormatDef, bool) {
	for i := range BuiltinFormats {
		if strings.EqualFold(BuiltinFormats[i].ID, id) {
			return &BuiltinFormats[i], true
		}
	}
	return nil, false
}

// AvailableFormatIDs returns all valid project format IDs including auto.
func AvailableFormatIDs() []string {
	ids := []string{FormatAuto}
	for _, f := range BuiltinFormats {
		ids = append(ids, f.ID)
	}
	return ids
}

// GlobalProgramDef describes a global program's default directory.
type GlobalProgramDef struct {
	ID          string `json:"id"`
	DefaultPath string `json:"defaultPath"`
	Description string `json:"description"`
}

// BuiltinGlobalPrograms defines known global agent programs.
var BuiltinGlobalPrograms = []GlobalProgramDef{
	{
		ID:          "standard",
		DefaultPath: "~/.agents/skills",
		Description: "Standard Global Agent Skills (~/.agents/skills)",
	},
	{
		ID:          "codex",
		DefaultPath: "~/.agents/skills",
		Description: "OpenAI Codex (~/.agents/skills)",
	},
	{
		ID:          "antigravity-2.0",
		DefaultPath: "~/.gemini/config/skills",
		Description: "Google Antigravity 2.0 (~/.gemini/config/skills)",
	},
	{
		ID:          "antigravity-cli",
		DefaultPath: "~/.gemini/antigravity-cli/skills",
		Description: "Google Antigravity CLI (~/.gemini/antigravity-cli/skills)",
	},
	{
		ID:          "gemini",
		DefaultPath: "~/.gemini/skills",
		Description: "Google Gemini CLI (~/.gemini/skills)",
	},
	{
		ID:          "cursor",
		DefaultPath: "~/.cursor/skills",
		Description: "Cursor (~/.cursor/skills)",
	},
	{
		ID:          "windsurf",
		DefaultPath: "~/.codeium/windsurf/skills",
		Description: "Windsurf / Cascade (~/.codeium/windsurf/skills)",
	},
	{
		ID:          "copilot",
		DefaultPath: "~/.copilot/skills",
		Description: "GitHub Copilot (~/.copilot/skills)",
	},
	{
		ID:          "opencode",
		DefaultPath: "~/.config/opencode/skills",
		Description: "OpenCode (~/.config/opencode/skills)",
	},
	{
		ID:          "claude",
		DefaultPath: "~/.claude/skills",
		Description: "Claude Code (~/.claude/skills)",
	},
	{
		ID:          "cline",
		DefaultPath: "~/.cline/skills",
		Description: "Cline (~/.cline/skills)",
	},
	{
		ID:          "roo",
		DefaultPath: "~/.roo/skills",
		Description: "Roo Code (~/.roo/skills)",
	},
	{
		ID:          "amp",
		DefaultPath: "~/.config/agents/skills",
		Description: "Amp (~/.config/agents/skills)",
	},
	{
		ID:          "pi",
		DefaultPath: "~/.pi/agent/skills",
		Description: "Pi (~/.pi/agent/skills)",
	},
	{
		ID:          "factory",
		DefaultPath: "~/.factory/skills",
		Description: "Factory Droid (~/.factory/skills)",
	},
}

// LookupGlobalProgram finds the GlobalProgramDef by ID.
func LookupGlobalProgram(id string) (*GlobalProgramDef, bool) {
	for i := range BuiltinGlobalPrograms {
		if strings.EqualFold(BuiltinGlobalPrograms[i].ID, id) {
			return &BuiltinGlobalPrograms[i], true
		}
	}
	return nil, false
}

// AvailableGlobalProgramIDs returns all valid global program IDs.
func AvailableGlobalProgramIDs() []string {
	var ids []string
	for _, p := range BuiltinGlobalPrograms {
		ids = append(ids, p.ID)
	}
	return ids
}

// ValidateGlobalProgram checks if a program ID is supported.
func ValidateGlobalProgram(id string) error {
	if _, ok := LookupGlobalProgram(id); !ok {
		return fmt.Errorf("unsupported global program %q. Available programs: %s", id, strings.Join(AvailableGlobalProgramIDs(), ", "))
	}
	return nil
}

// DefaultTargetPresets generates standard preset TargetConfig entries to write into config.jsonc on initialization.
func DefaultTargetPresets() map[string]TargetConfig {
	presets := make(map[string]TargetConfig)
	for _, p := range BuiltinGlobalPrograms {
		cfgDir := strings.TrimSuffix(p.DefaultPath, "/skills")
		paths := map[string]string{
			"skills": "${config_dir}/skills",
		}
		if p.ID == "opencode" {
			paths["rules"] = "${config_dir}/rules"
		}
		presets[p.ID] = TargetConfig{
			Type:      TargetTypeAgent,
			Channel:   p.ID,
			ConfigDir: cfgDir,
			Paths:     paths,
		}
	}
	return presets
}
