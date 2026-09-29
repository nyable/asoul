package modelsdev

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMatcher(t *testing.T) {
	cat := Catalog{
		"anthropic": {
			ID:   "anthropic",
			Name: "Anthropic",
			Models: map[string]ModelData{
				"anthropic/claude-3-7-sonnet": {
					ID:        "anthropic/claude-3-7-sonnet",
					Name:      "Claude 3.7 Sonnet",
					Reasoning: true,
					Limit:     &LimitData{Context: 200000, Output: 64000},
				},
				"anthropic/claude-3-5-sonnet-20241022": {
					ID:    "anthropic/claude-3-5-sonnet-20241022",
					Name:  "Claude 3.5 Sonnet",
					Limit: &LimitData{Context: 200000, Output: 8192},
				},
			},
		},
		"openai": {
			ID:   "openai",
			Name: "OpenAI",
			Models: map[string]ModelData{
				"gpt-4o": {
					ID:    "gpt-4o",
					Name:  "GPT-4o",
					Limit: &LimitData{Context: 128000, Output: 16384},
				},
			},
		},
	}

	matcher := NewMatcher(cat)

	// 1. Exact match with prefix
	m, ok := matcher.FindModel("anthropic/claude-3-7-sonnet")
	if !ok || m.Name != "Claude 3.7 Sonnet" {
		t.Fatalf("expected Claude 3.7 Sonnet, got %v", m)
	}

	// 2. Stripped prefix query
	m, ok = matcher.FindModel("claude-3-7-sonnet")
	if !ok || m.Name != "Claude 3.7 Sonnet" {
		t.Fatalf("expected Claude 3.7 Sonnet from short query, got %v", m)
	}

	// 3. Short query matching dated model
	m, ok = matcher.FindModel("claude-3-5-sonnet")
	if !ok || m.Name != "Claude 3.5 Sonnet" {
		t.Fatalf("expected Claude 3.5 Sonnet from query without date, got %v", m)
	}

	// 4. Query with extra prefix
	m, ok = matcher.FindModel("my-provider/gpt-4o")
	if !ok || m.Name != "GPT-4o" {
		t.Fatalf("expected GPT-4o, got %v", m)
	}

	// 5. Non-existent model
	_, ok = matcher.FindModel("unknown-model-xyz")
	if ok {
		t.Fatalf("expected false for unknown model")
	}
}

func TestMatcherAuthoritativePriority(t *testing.T) {
	cat := Catalog{
		"302ai": {
			ID:   "302ai",
			Name: "302.AI",
			Models: map[string]ModelData{
				"gpt-4o": {
					ID:   "302ai/gpt-4o",
					Name: "GPT-4o from 302",
					Cost: &CostData{Input: 5.0, Output: 15.0},
				},
				"claude-3-7": {
					ID:   "302ai/claude-3-7",
					Name: "Claude 3.7 from 302",
				},
				"o3-mini": {
					ID:   "302ai/o3-mini",
					Name: "o3-mini from 302",
				},
			},
		},
		"openai": {
			ID:   "openai",
			Name: "OpenAI",
			Models: map[string]ModelData{
				"gpt-4o": {
					ID:   "openai/gpt-4o",
					Name: "Official GPT-4o",
					Cost: &CostData{Input: 2.5, Output: 10.0, CacheRead: 1.25},
				},
				"o3-mini": {
					ID:   "openai/o3-mini",
					Name: "Official o3-mini",
				},
			},
		},
		"anthropic": {
			ID:   "anthropic",
			Name: "Anthropic",
			Models: map[string]ModelData{
				"claude-3-7": {
					ID:   "anthropic/claude-3-7",
					Name: "Official Claude 3.7",
				},
			},
		},
	}

	officialMap := map[string][]string{
		"openai":    {"(?i)^gpt-", "(?i)^o[134]-"},
		"anthropic": {"(?i)^claude-"},
	}

	matcher := NewMatcher(cat, officialMap)

	// 1. Short query matches official provider despite alphabetical order
	m, ok := matcher.FindModel("gpt-4o")
	if !ok {
		t.Fatal("expected gpt-4o to be matched")
	}
	if m.ID != "openai/gpt-4o" {
		t.Fatalf("expected official openai/gpt-4o, got %s (from %s)", m.ID, m.Name)
	}
	if m.ProviderID != "openai" {
		t.Fatalf("expected provider openai, got %s", m.ProviderID)
	}

	// 2. Regex pattern (?i)^o[134]- matches o3-mini from official OpenAI
	m, ok = matcher.FindModel("o3-mini")
	if !ok {
		t.Fatal("expected o3-mini to be matched")
	}
	if m.ID != "openai/o3-mini" {
		t.Fatalf("expected official openai/o3-mini, got %s (from %s)", m.ID, m.Name)
	}
	if m.ProviderID != "openai" {
		t.Fatalf("expected provider openai, got %s", m.ProviderID)
	}

	// 3. Short query matches official anthropic
	m, ok = matcher.FindModel("claude-3-7")
	if !ok {
		t.Fatal("expected claude-3-7 to be matched")
	}
	if m.ID != "anthropic/claude-3-7" {
		t.Fatalf("expected official anthropic/claude-3-7, got %s (from %s)", m.ID, m.Name)
	}
	if m.ProviderID != "anthropic" {
		t.Fatalf("expected provider anthropic, got %s", m.ProviderID)
	}

	// 4. Explicit channel query still routes directly to channel vendor
	m, ok = matcher.FindModel("302ai/gpt-4o")
	if !ok {
		t.Fatal("expected 302ai/gpt-4o to be matched")
	}
	if m.ID != "302ai/gpt-4o" {
		t.Fatalf("expected 302ai/gpt-4o, got %s", m.ID)
	}
	if m.ProviderID != "302ai" {
		t.Fatalf("expected provider 302ai, got %s", m.ProviderID)
	}
}

func TestClientCacheAndFallback(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "modelsdev-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	cacheFile := filepath.Join(tempDir, "cache.json")

	// Create test HTTP server
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"test": {
				"id": "test",
				"name": "Test Provider",
				"models": {
					"test-m1": { "id": "test-m1", "name": "Model 1" }
				}
			}
		}`))
	}))
	defer server.Close()

	client, err := NewClient(
		WithAPIURL(server.URL),
		WithCachePath(cacheFile),
		WithCacheTTL(1*time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Fetch 1: should hit network
	cat, err := client.FetchCatalog(context.Background(), false)
	if err != nil {
		t.Fatalf("fetch 1 failed: %v", err)
	}
	if requests != 1 {
		t.Errorf("expected 1 network request, got %d", requests)
	}
	if cat["test"].Name != "Test Provider" {
		t.Errorf("unexpected provider name: %s", cat["test"].Name)
	}

	// Fetch 2: should hit cache
	cat2, err := client.FetchCatalog(context.Background(), false)
	if err != nil {
		t.Fatalf("fetch 2 failed: %v", err)
	}
	if requests != 1 {
		t.Errorf("expected 1 network request (cached), got %d", requests)
	}
	if cat2["test"].Models["test-m1"].Name != "Model 1" {
		t.Errorf("unexpected model name: %s", cat2["test"].Models["test-m1"].Name)
	}

	// Fetch 3: with refresh = true
	_, err = client.FetchCatalog(context.Background(), true)
	if err != nil {
		t.Fatalf("fetch 3 failed: %v", err)
	}
	if requests != 2 {
		t.Errorf("expected 2 network requests with refresh=true, got %d", requests)
	}
}
