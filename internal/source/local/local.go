package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"asoul/internal/fsx"
	"asoul/internal/hash"
	"asoul/internal/model"
	"asoul/internal/skill"
	"asoul/internal/source"
)

// Source handles local directory sources outside the workspace.
type Source struct {
	workspaceRoot string
}

// New creates a local source handler.
func New(workspaceRoot string) *Source {
	return &Source{workspaceRoot: workspaceRoot}
}

// resolveLocalPath resolves relative or user-expanded paths relative to the caller's current directory.
func (s *Source) resolveLocalPath(srcPath string) (string, error) {
	expanded, err := fsx.ExpandUser(srcPath)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(expanded) {
		return filepath.Clean(expanded), nil
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

// Resolve validates the external local directory, computes its hash, and stages it.
func (s *Source) Resolve(ctx context.Context, spec model.SourceSpec) (*source.Candidate, error) {
	absPath, err := s.resolveLocalPath(spec.Path)
	if err != nil {
		return nil, err
	}

	meta, err := skill.ValidateSkillDir(absPath, "")
	if err != nil {
		return nil, fmt.Errorf("local source validation failed at %s: %w", absPath, err)
	}

	h, err := hash.HashDir(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to compute hash for local source %s: %w", absPath, err)
	}

	// Create temporary copy for candidate
	tmpDir, err := os.MkdirTemp("", "asoul-candidate-local-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory for candidate: %w", err)
	}

	candDir := filepath.Join(tmpDir, meta.Name)
	if err := fsx.CopyDir(absPath, candDir); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("failed to copy local source to candidate: %w", err)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return &source.Candidate{
		TempDir: candDir,
		SkillID: meta.Name,
		Hash:    h,
		Cleanup: cleanup,
	}, nil
}

// Check compares current hash of the external local directory with the recorded hash.
func (s *Source) Check(ctx context.Context, spec model.SourceSpec, resolved model.Resolved) (*source.SourceStatus, error) {
	absPath, err := s.resolveLocalPath(spec.Path)
	if err != nil {
		return &source.SourceStatus{
			Upstream: model.UpstreamInvalid,
			Error:    err.Error(),
		}, nil
	}

	if !fsx.DirExists(absPath) {
		return &source.SourceStatus{
			Upstream: model.UpstreamUnreachable,
			Error:    fmt.Sprintf("source directory does not exist: %s", absPath),
		}, nil
	}

	currentHash, err := hash.HashDir(absPath)
	if err != nil {
		return &source.SourceStatus{
			Upstream: model.UpstreamInvalid,
			Error:    err.Error(),
		}, nil
	}

	if currentHash == resolved.ContentHash {
		return &source.SourceStatus{
			Upstream: model.UpstreamUpToDate,
			Hash:     currentHash,
		}, nil
	}

	return &source.SourceStatus{
		Upstream: model.UpstreamSourceChanged,
		Hash:     currentHash,
	}, nil
}
