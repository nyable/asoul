package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"asoul/internal/fsx"
)

// DefaultCacheDir returns user cache directory: ~/.cache/asoul/git (or OS equivalent)
func DefaultCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("unable to determine user cache directory: %w", err)
	}
	return filepath.Join(base, "asoul", "git"), nil
}

// RepoMeta stores cached repository information.
type RepoMeta struct {
	URL        string    `json:"url"`
	Key        string    `json:"key"`
	LastUsedAt time.Time `json:"lastUsedAt"`
}

// CacheEntry represents an item for asoul cache list.
type CacheEntry struct {
	Key        string    `json:"key"`
	URL        string    `json:"url"`
	Path       string    `json:"path"`
	SizeBytes  int64     `json:"sizeBytes"`
	LastUsedAt time.Time `json:"lastUsedAt"`
}

// Manager manages git bare repositories cached on disk.
type Manager struct {
	cacheDir string
}

// NewManager creates a cache manager.
func NewManager(customCacheDir string) (*Manager, error) {
	if customCacheDir != "" {
		expanded, err := fsx.ExpandUser(customCacheDir)
		if err != nil {
			return nil, err
		}
		return &Manager{cacheDir: expanded}, nil
	}
	defDir, err := DefaultCacheDir()
	if err != nil {
		return nil, err
	}
	return &Manager{cacheDir: defDir}, nil
}

// CacheDir returns the base cache directory.
func (m *Manager) CacheDir() string {
	return m.cacheDir
}

// KeyForURL generates a deterministic key for a git repository URL.
func KeyForURL(url string) string {
	cleanURL := strings.TrimSpace(url)
	cleanURL = strings.TrimSuffix(cleanURL, ".git")
	cleanURL = strings.ToLower(cleanURL)

	h := sha256.Sum256([]byte(cleanURL))
	shortHash := hex.EncodeToString(h[:])[:12]

	parts := strings.Split(cleanURL, "/")
	slug := "repo"
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		if last != "" {
			// clean non-alphanumeric
			slug = strings.Map(func(r rune) rune {
				if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
					return r
				}
				return '_'
			}, last)
		}
	}
	return fmt.Sprintf("%s-%s.git", slug, shortHash)
}

// RepoDir returns the bare repository path for a URL.
func (m *Manager) RepoDir(url string) string {
	key := KeyForURL(url)
	return filepath.Join(m.cacheDir, key)
}

// RepoExists returns whether a valid bare repository cache exists for the URL.
func (m *Manager) RepoExists(url string) bool {
	repoDir := m.RepoDir(url)
	headFile := filepath.Join(repoDir, "HEAD")
	return fsx.DirExists(repoDir) && fsx.FileExists(headFile)
}

// GetRepoMeta returns the cached repository metadata if available.
func (m *Manager) GetRepoMeta(url string) *RepoMeta {
	repoDir := m.RepoDir(url)
	metaPath := filepath.Join(repoDir, "asoul-meta.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil
	}
	var meta RepoMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil
	}
	return &meta
}

// TouchRepo updates the last used metadata for a repo cache.
func (m *Manager) TouchRepo(url string) error {
	repoDir := m.RepoDir(url)
	metaPath := filepath.Join(repoDir, "asoul-meta.json")
	meta := RepoMeta{
		URL:        url,
		Key:        filepath.Base(repoDir),
		LastUsedAt: time.Now().UTC(),
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return fsx.AtomicWriteFile(metaPath, data, 0644)
}

// List returns all cached repositories with their sizes and metadata.
func (m *Manager) List() ([]CacheEntry, error) {
	if !fsx.DirExists(m.cacheDir) {
		return nil, nil
	}

	entries, err := os.ReadDir(m.cacheDir)
	if err != nil {
		return nil, err
	}

	var results []CacheEntry
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasSuffix(entry.Name(), ".git") {
			continue
		}

		repoPath := filepath.Join(m.cacheDir, entry.Name())
		var size int64
		_ = filepath.Walk(repoPath, func(_ string, info os.FileInfo, err error) error {
			if err == nil && info != nil && !info.IsDir() {
				size += info.Size()
			}
			return nil
		})

		meta := RepoMeta{Key: entry.Name()}
		metaData, err := os.ReadFile(filepath.Join(repoPath, "asoul-meta.json"))
		if err == nil {
			_ = json.Unmarshal(metaData, &meta)
		}

		results = append(results, CacheEntry{
			Key:        entry.Name(),
			URL:        meta.URL,
			Path:       repoPath,
			SizeBytes:  size,
			LastUsedAt: meta.LastUsedAt,
		})
	}

	return results, nil
}

// Prune removes caches that are not in the activeKeys set.
func (m *Manager) Prune(activeKeys map[string]bool) (int, int64, error) {
	entries, err := m.List()
	if err != nil {
		return 0, 0, err
	}

	var removedCount int
	var freedBytes int64

	for _, entry := range entries {
		if !activeKeys[entry.Key] {
			if err := os.RemoveAll(entry.Path); err == nil {
				removedCount++
				freedBytes += entry.SizeBytes
			}
		}
	}

	return removedCount, freedBytes, nil
}

// Clean removes all cached repositories.
func (m *Manager) Clean() (int, int64, error) {
	return m.Prune(map[string]bool{})
}
