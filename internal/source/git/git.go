package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"asoul/internal/cache"
	"asoul/internal/fsx"
	"asoul/internal/gitx"
	"asoul/internal/hash"
	"asoul/internal/model"
	"asoul/internal/progress"
	"asoul/internal/skill"
	"asoul/internal/source"
)

// DiscoveredSkill contains information about a skill found in a repository.
type DiscoveredSkill struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

// Source handles git repository sources via git cache and system git client.
type Source struct {
	client       *gitx.Client
	cacheManager *cache.Manager
}

// New creates a new git source handler.
func New(client *gitx.Client, cacheManager *cache.Manager) *Source {
	return &Source{
		client:       client,
		cacheManager: cacheManager,
	}
}

// EnsureRepo ensures that the bare clone exists and is up to date.
func (s *Source) EnsureRepo(ctx context.Context, url string, fetch bool, onProgress ...progress.Func) (string, error) {
	repoDir := s.cacheManager.RepoDir(url)
	headFile := filepath.Join(repoDir, "HEAD")
	if !fsx.DirExists(repoDir) || !fsx.FileExists(headFile) {
		_ = os.RemoveAll(repoDir)
		if err := s.client.CloneMirror(ctx, url, repoDir, onProgress...); err != nil {
			_ = os.RemoveAll(repoDir)
			return "", fmt.Errorf("git clone failed for %s: %w", url, err)
		}
	} else if fetch {
		if err := s.client.Fetch(ctx, repoDir, onProgress...); err != nil {
			return repoDir, fmt.Errorf("git fetch failed for %s: %w", url, err)
		}
	}
	_ = s.cacheManager.TouchRepo(url)
	return repoDir, nil
}

// DiscoverSkills inspects a git repository at the given ref and finds all valid skills,
// prioritizing the local bare cache without network, only cloning if no cache exists.
func (s *Source) DiscoverSkills(ctx context.Context, url, ref string, onProgress ...progress.Func) ([]DiscoveredSkill, error) {
	return s.DiscoverSkillsWithFetch(ctx, url, ref, false, onProgress...)
}

// DiscoverSkillsWithFetch inspects a git repository at the given ref and finds all valid skills,
// optionally forcing a network fetch first.
func (s *Source) DiscoverSkillsWithFetch(ctx context.Context, url, ref string, fetch bool, onProgress ...progress.Func) ([]DiscoveredSkill, error) {
	repoDir, err := s.EnsureRepo(ctx, url, fetch, onProgress...)
	if err != nil {
		return nil, err
	}

	commit, err := s.client.ResolveRef(ctx, repoDir, ref)
	if err != nil {
		// If ref resolution failed on existing cache and fetch was false, try fetching remote once
		if !fetch {
			if fetchErr := s.client.Fetch(ctx, repoDir, onProgress...); fetchErr == nil {
				commit, err = s.client.ResolveRef(ctx, repoDir, ref)
			}
		}
		if err != nil {
			return nil, fmt.Errorf("failed to resolve ref %q: %w", ref, err)
		}
	}

	return s.discoverAtCommit(ctx, repoDir, commit, onProgress...)
}

// DiscoverSkillsCached never clones or fetches, even when the ref cannot be resolved.
func (s *Source) DiscoverSkillsCached(ctx context.Context, url, ref string) ([]DiscoveredSkill, error) {
	repoDir := s.cacheManager.RepoDir(url)
	commit, err := s.client.ResolveRef(ctx, repoDir, ref)
	if err != nil {
		return nil, err
	}
	return s.discoverAtCommit(ctx, repoDir, commit)
}

func (s *Source) discoverAtCommit(ctx context.Context, repoDir, commit string, onProgress ...progress.Func) ([]DiscoveredSkill, error) {
	files, err := s.client.ListTree(ctx, repoDir, commit, "")
	if err != nil {
		return nil, fmt.Errorf("failed to list repository tree: %w", err)
	}

	var skillMDPaths []string
	for _, f := range files {
		clean := filepath.ToSlash(f)
		if filepath.Base(clean) == "SKILL.md" {
			skillMDPaths = append(skillMDPaths, clean)
		}
	}

	if len(skillMDPaths) == 0 {
		return []DiscoveredSkill{}, nil
	}

	results := []DiscoveredSkill{}
	var failures []error
	for i, mdPath := range skillMDPaths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		progress.Send(progress.First(onProgress...), progress.Update{Phase: progress.PhaseScan, Current: i + 1, Total: len(skillMDPaths), Text: mdPath})
		subDir := filepath.Dir(mdPath)
		data, err := s.client.ShowFile(ctx, repoDir, commit, mdPath)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", mdPath, err))
			continue
		}
		meta, _, parseErr := skill.ParseSkillContent(data)
		if parseErr != nil {
			failures = append(failures, fmt.Errorf("%s: %w", mdPath, parseErr))
			continue
		}
		if err := skill.ValidateID(meta.Name); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", mdPath, err))
			continue
		}
		if strings.TrimSpace(meta.Description) == "" || (subDir != "." && filepath.Base(subDir) != meta.Name) {
			failures = append(failures, fmt.Errorf("%s: missing description or name does not match directory", mdPath))
			continue
		}
		results = append(results, DiscoveredSkill{ID: meta.Name, Path: subDir, Description: meta.Description})
	}

	return results, errors.Join(failures...)
}

// ResolveCommit resolves a git reference to a full commit hash in the cached repo.
func (s *Source) ResolveCommit(ctx context.Context, url, ref string) (string, error) {
	repoDir := s.cacheManager.RepoDir(url)
	if !s.cacheManager.RepoExists(url) {
		return "", fmt.Errorf("repository %s not cached", url)
	}
	return s.client.ResolveRef(ctx, repoDir, ref)
}

// ResolveBatch extracts multiple skills from the repository commit into candidate directories in one archive operation.
func (s *Source) ResolveBatch(ctx context.Context, spec model.SourceSpec, subpaths []string, fetch bool, onProgress ...progress.Func) ([]*source.Candidate, map[string]error, error) {
	if spec.URL == "" {
		return nil, nil, fmt.Errorf("git source spec requires a url")
	}

	repoDir, err := s.EnsureRepo(ctx, spec.URL, fetch, onProgress...)
	if err != nil {
		return nil, nil, err
	}

	commit, err := s.client.ResolveRef(ctx, repoDir, spec.Ref)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve git ref %q for %s: %w", spec.Ref, spec.URL, err)
	}

	progress.Send(progress.First(onProgress...), progress.Update{Phase: progress.PhaseExtract})

	extractDir, err := os.MkdirTemp("", "asoul-batch-raw-*")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create temporary extraction directory: %w", err)
	}
	defer os.RemoveAll(extractDir)

	if err := s.client.ArchiveExtractPaths(ctx, repoDir, commit, subpaths, extractDir); err != nil {
		return nil, nil, fmt.Errorf("failed to extract git archive: %w", err)
	}

	progress.Send(progress.First(onProgress...), progress.Update{Phase: progress.PhaseValidate})

	type itemResult struct {
		subpath string
		cand    *source.Candidate
		err     error
	}

	results := make([]itemResult, len(subpaths))
	var wg sync.WaitGroup

	for i, sp := range subpaths {
		wg.Add(1)
		go func(idx int, sub string) {
			defer wg.Done()

			cleanSub := strings.Trim(filepath.ToSlash(sub), "/")
			itemDir := extractDir
			if cleanSub != "" && cleanSub != "." {
				itemDir = filepath.Join(extractDir, filepath.FromSlash(cleanSub))
			}

			if !fsx.DirExists(itemDir) {
				results[idx] = itemResult{subpath: sub, err: fmt.Errorf("skill directory %s does not exist in archive", sub)}
				return
			}

			skillMDPath := filepath.Join(itemDir, "SKILL.md")
			if !fsx.FileExists(skillMDPath) {
				results[idx] = itemResult{subpath: sub, err: fmt.Errorf("SKILL.md not found in %s", sub)}
				return
			}

			meta, _, err := skill.ParseSkillMD(skillMDPath)
			if err != nil {
				results[idx] = itemResult{subpath: sub, err: fmt.Errorf("invalid SKILL.md in %s: %w", sub, err)}
				return
			}

			if err := skill.ValidateID(meta.Name); err != nil {
				results[idx] = itemResult{subpath: sub, err: fmt.Errorf("invalid skill name %q in %s: %w", meta.Name, sub, err)}
				return
			}

			candParent, err := os.MkdirTemp("", "asoul-cand-*")
			if err != nil {
				results[idx] = itemResult{subpath: sub, err: err}
				return
			}

			candDir := filepath.Join(candParent, meta.Name)
			copyErr := fsx.CopyDir(itemDir, candDir)

			if copyErr != nil {
				_ = os.RemoveAll(candParent)
				results[idx] = itemResult{subpath: sub, err: fmt.Errorf("failed to stage candidate for %s: %w", sub, copyErr)}
				return
			}

			if _, err := skill.ValidateSkillDir(candDir, meta.Name); err != nil {
				_ = os.RemoveAll(candParent)
				results[idx] = itemResult{subpath: sub, err: fmt.Errorf("extracted skill %q is invalid: %w", meta.Name, err)}
				return
			}

			h, err := hash.HashDir(candDir)
			if err != nil {
				_ = os.RemoveAll(candParent)
				results[idx] = itemResult{subpath: sub, err: fmt.Errorf("failed to compute content hash for %q: %w", meta.Name, err)}
				return
			}

			cleanup := func() {
				_ = os.RemoveAll(candParent)
			}

			results[idx] = itemResult{
				subpath: sub,
				cand: &source.Candidate{
					TempDir: candDir,
					SkillID: meta.Name,
					Commit:  commit,
					Hash:    h,
					Subpath: sub,
					Cleanup: cleanup,
				},
			}
		}(i, sp)
	}

	wg.Wait()

	var candidates []*source.Candidate
	failures := make(map[string]error)

	for _, res := range results {
		if res.err != nil {
			failures[res.subpath] = res.err
			continue
		}
		if res.cand != nil {
			candidates = append(candidates, res.cand)
		}
	}

	return candidates, failures, nil
}

// Resolve extracts the skill content from the repository commit into a candidate directory.
func (s *Source) Resolve(ctx context.Context, spec model.SourceSpec, onProgress ...progress.Func) (*source.Candidate, error) {
	if spec.URL == "" {
		return nil, fmt.Errorf("git source spec requires a url")
	}

	candidates, failures, err := s.ResolveBatch(ctx, spec, []string{spec.Path}, true, onProgress...)
	if err != nil {
		return nil, err
	}
	if len(failures) > 0 {
		for _, fErr := range failures {
			return nil, fErr
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no skill candidate could be resolved from %s (path: %s)", spec.URL, spec.Path)
	}
	return candidates[0], nil
}

// Check checks if the git upstream has updates.
func (s *Source) Check(ctx context.Context, spec model.SourceSpec, resolved model.Resolved) (*source.SourceStatus, error) {
	repoDir := s.cacheManager.RepoDir(spec.URL)
	if !fsx.DirExists(repoDir) {
		// Bare repo not cached yet
		return &source.SourceStatus{
			Upstream: model.UpstreamUnreachable,
			Error:    "cache repository missing",
		}, nil
	}

	// Fetch upstream to get latest commits
	if err := s.client.Fetch(ctx, repoDir); err != nil {
		return &source.SourceStatus{
			Upstream: model.UpstreamUnreachable,
			Error:    err.Error(),
		}, nil
	}
	return s.CheckCached(ctx, spec, resolved)
}

// CheckCached compares a source against an already refreshed repository cache.
func (s *Source) CheckCached(ctx context.Context, spec model.SourceSpec, resolved model.Resolved) (*source.SourceStatus, error) {
	repoDir := s.cacheManager.RepoDir(spec.URL)
	if !fsx.DirExists(repoDir) {
		return &source.SourceStatus{Upstream: model.UpstreamUnreachable, Error: "cache repository missing"}, nil
	}
	commit, err := s.client.ResolveRef(ctx, repoDir, spec.Ref)
	if err != nil {
		return &source.SourceStatus{
			Upstream: model.UpstreamInvalid,
			Error:    err.Error(),
		}, nil
	}

	if commit == resolved.Commit {
		return &source.SourceStatus{
			Upstream: model.UpstreamUpToDate,
			Commit:   commit,
			Hash:     resolved.ContentHash,
		}, nil
	}

	return &source.SourceStatus{
		Upstream: model.UpstreamUpdateAvailable,
		Commit:   commit,
	}, nil
}
