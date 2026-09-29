package modelsdev

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"asoul/internal/fsx"
)

const (
	DefaultAPIURL   = "https://models.dev/api.json"
	DefaultCacheTTL = 24 * time.Hour
	UserAgent       = "Mozilla/5.0 (compatible; asoul/0.1.0; +https://github.com/asoul)"
)

// Client fetches and caches models.dev catalog data.
type Client struct {
	apiURL     string
	cachePath  string
	cacheTTL   time.Duration
	httpClient *http.Client
}

// Option configures Client.
type Option func(*Client)

// WithAPIURL overrides default API URL.
func WithAPIURL(url string) Option {
	return func(c *Client) { c.apiURL = url }
}

// WithCachePath overrides cache file path.
func WithCachePath(p string) Option {
	return func(c *Client) { c.cachePath = p }
}

// WithCacheTTL overrides cache expiration duration.
func WithCacheTTL(ttl time.Duration) Option {
	return func(c *Client) { c.cacheTTL = ttl }
}

// WithHTTPClient overrides default http.Client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) { c.httpClient = client }
}

// NewClient creates a new models.dev client.
func NewClient(opts ...Option) (*Client, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	defaultCache := filepath.Join(cacheDir, "asoul", "models_dev.json")

	c := &Client{
		apiURL:     DefaultAPIURL,
		cachePath:  defaultCache,
		cacheTTL:   DefaultCacheTTL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// FetchCatalog loads the models.dev catalog from cache or network.
func (c *Client) FetchCatalog(ctx context.Context, refresh bool) (Catalog, error) {
	// Try cache first if not refreshing
	if !refresh && c.isCacheValid() {
		if cat, err := c.readCache(); err == nil {
			return cat, nil
		}
	}

	// Fetch from remote
	cat, fetchErr := c.fetchRemote(ctx)
	if fetchErr == nil {
		_ = c.writeCache(cat)
		return cat, nil
	}

	// Network failed: fallback to expired cache if available
	if cat, err := c.readCache(); err == nil {
		return cat, nil
	}

	return nil, fmt.Errorf("failed to fetch models.dev catalog: %w", fetchErr)
}

func (c *Client) isCacheValid() bool {
	if c.cachePath == "" {
		return false
	}
	info, err := os.Stat(c.cachePath)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < c.cacheTTL
}

// ReadCachedCatalog returns the cached catalog from disk without network requests.
func (c *Client) ReadCachedCatalog() (Catalog, error) {
	return c.readCache()
}

func (c *Client) readCache() (Catalog, error) {
	data, err := os.ReadFile(c.cachePath)
	if err != nil {
		return nil, err
	}
	var cat Catalog
	if err := json.Unmarshal(data, &cat); err != nil {
		return nil, err
	}
	return cat, nil
}

func (c *Client) writeCache(cat Catalog) error {
	if c.cachePath == "" {
		return nil
	}
	dir := filepath.Dir(c.cachePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		return err
	}
	return fsx.AtomicWriteFile(c.cachePath, data, 0644)
}

func (c *Client) fetchRemote(ctx context.Context) (Catalog, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error %d %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var cat Catalog
	if err := json.Unmarshal(body, &cat); err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}
	return cat, nil
}
