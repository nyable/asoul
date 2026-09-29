package managed

import (
	"context"
	"fmt"
	"path/filepath"

	"asoul/internal/catalog"
	"asoul/internal/hash"
	"asoul/internal/model"
	"asoul/internal/skill"
	"asoul/internal/source"
)

// Source handles managed skills that live entirely inside workspace.
type Source struct {
	workspaceRoot string
}

// New creates a managed source handler.
func New(workspaceRoot string) *Source {
	return &Source{workspaceRoot: workspaceRoot}
}

// Resolve validates the managed skill within the workspace.
func (s *Source) Resolve(ctx context.Context, spec model.SourceSpec) (*source.Candidate, error) {
	skillDir := filepath.Join(catalog.SkillsDir(s.workspaceRoot), spec.Path)
	meta, err := skill.ValidateSkillDir(skillDir, spec.Path)
	if err != nil {
		return nil, fmt.Errorf("managed skill validation failed: %w", err)
	}

	h, err := hash.HashDir(skillDir)
	if err != nil {
		return nil, err
	}

	return &source.Candidate{
		TempDir: skillDir,
		SkillID: meta.Name,
		Hash:    h,
		Cleanup: func() {},
	}, nil
}

// Check for managed source always returns UpstreamNone since there is no upstream.
func (s *Source) Check(ctx context.Context, spec model.SourceSpec, resolved model.Resolved) (*source.SourceStatus, error) {
	return &source.SourceStatus{
		Upstream: model.UpstreamNone,
		Hash:     resolved.ContentHash,
	}, nil
}
