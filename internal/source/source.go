package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"asoul/internal/hash"
	"asoul/internal/model"
	"asoul/internal/skill"
)

// Candidate represents a validated skill ready to be installed into the managed store.
type Candidate struct {
	TempDir string
	SkillID string
	Commit  string
	Hash    string
	Subpath string
	Cleanup func()
}

// OverrideID renames candidate skill to a new ID, updating SKILL.md, folder name, and content hash.
func (c *Candidate) OverrideID(newID string) error {
	if err := skill.ValidateID(newID); err != nil {
		return err
	}
	if c.SkillID == newID {
		return nil
	}

	skillMDPath := filepath.Join(c.TempDir, "SKILL.md")
	if err := skill.RewriteSkillMDName(skillMDPath, newID); err != nil {
		return fmt.Errorf("failed to rewrite SKILL.md name: %w", err)
	}

	candParent := filepath.Dir(c.TempDir)
	newCandDir := filepath.Join(candParent, newID)
	if err := os.Rename(c.TempDir, newCandDir); err != nil {
		return fmt.Errorf("failed to rename candidate directory: %w", err)
	}
	c.TempDir = newCandDir
	c.SkillID = newID

	h, err := hash.HashDir(newCandDir)
	if err != nil {
		return fmt.Errorf("failed to recompute content hash: %w", err)
	}
	c.Hash = h
	return nil
}

// SourceStatus represents the upstream check result for a source.
type SourceStatus struct {
	Upstream model.UpstreamStatus
	Commit   string
	Hash     string
	Error    string
}

// Source is the abstraction for any skill source provider.
type Source interface {
	Resolve(ctx context.Context, spec model.SourceSpec) (*Candidate, error)
	Check(ctx context.Context, spec model.SourceSpec, resolved model.Resolved) (*SourceStatus, error)
}
