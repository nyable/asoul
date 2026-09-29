package tui_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"asoul/internal/agent/opencode"
	"asoul/internal/app"
	"asoul/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

func setupModelTestEnvironment(t *testing.T) (*app.Service, string, func()) {
	t.Helper()
	svc, wsRoot, cleanup := setupTestService(t)

	tmpCache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpCache)
	asoulCacheDir := filepath.Join(tmpCache, "asoul")
	_ = os.MkdirAll(asoulCacheDir, 0755)

	mockCatalog := []byte(`{
		"anthropic": {
			"id": "anthropic",
			"name": "Anthropic",
			"models": {
				"claude-3-7-sonnet": {
					"id": "anthropic/claude-3-7-sonnet",
					"name": "Claude 3.7 Sonnet",
					"limit": {"context": 200000, "input": 128000, "output": 8192},
					"cost": {"input": 3.0, "output": 15.0, "cache_read": 0.3, "cache_write": 3.75},
					"reasoning": true,
					"tool_call": true,
					"attachment": true,
					"structured_output": true,
					"temperature": true,
					"modalities": {"input": ["text", "image"], "output": ["text"]}
				},
				"claude-3-5-haiku": {
					"id": "anthropic/claude-3-5-haiku",
					"name": "Claude 3.5 Haiku",
					"limit": {"context": 200000, "output": 4096},
					"cost": {"input": 0.8, "output": 4.0},
					"tool_call": true
				}
			}
		},
		"openai": {
			"id": "openai",
			"name": "OpenAI",
			"models": {
				"gpt-4o": {
					"id": "openai/gpt-4o",
					"name": "GPT-4o",
					"limit": {"context": 128000, "output": 4096},
					"cost": {"input": 2.5, "output": 10.0},
					"tool_call": true,
					"attachment": true
				},
				"gpt-5": {
					"id": "openai/gpt-5",
					"name": "GPT-5",
					"limit": {"context": 400000, "output": 128000},
					"cost": {"input": 5.0, "output": 15.0},
					"reasoning": true
				}
			}
		}
	}`)
	_ = os.WriteFile(filepath.Join(asoulCacheDir, "models_dev.json"), mockCatalog, 0644)

	multiProviderConfig := []byte(`{
		"provider": {
			"anthropic": {
				"models": {
					"claude-3-7-sonnet": {
						"name": "Claude 3.7 Sonnet (Custom)",
						"limit": {"context": 200000, "input": 128000, "output": 8192},
						"cost": {"input": 3.0, "output": 15.0, "cache_read": 0.3, "cache_write": 3.75},
						"reasoning": true,
						"tool_call": true,
						"attachment": true,
						"structured_output": true,
						"temperature": true,
						"modalities": {"input": ["text", "image"], "output": ["text"]},
						"variants": {
							"thinking": {"reasoning": true},
							"fast": {"reasoning": false}
						}
					},
					"claude-3-5-haiku": {
						"name": "Claude 3.5 Haiku"
					}
				}
			},
			"openai": {
				"models": {
					"gpt-4o": {
						"name": "GPT-4o Omnimodel",
						"limit": {"context": 128000, "output": 4096},
						"cost": {"input": 2.5, "output": 10.0},
						"tool_call": true,
						"attachment": true
					},
					"gpt-5": {
						"name": "GPT-5 Next Gen"
					}
				}
			}
		}
	}`)
	opencodePath := filepath.Join(wsRoot, "opencode.json")
	_ = os.WriteFile(opencodePath, multiProviderConfig, 0644)
	_ = svc.TargetAdd("opencode", wsRoot)

	return svc, opencodePath, func() {
		cleanup()
	}
}

func TestTUIModelsTab_AgentChannelGrouping(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupModelTestEnvironment(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)
	m = step(m, tea.WindowSizeMsg{Width: 120, Height: 60})

	// Jump to Tab 4 (Channels)
	mTab4, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	modelModel := mTab4.(*tui.Model)

	channels := modelModel.ChannelNamesForTest()
	if len(channels) != 1 || channels[0] != "opencode" {
		t.Fatalf("expected agent channels [opencode], got %v", channels)
	}
	if modelModel.ChannelCursorForTest() != 0 {
		t.Fatalf("expected default channel cursor 0, got %d", modelModel.ChannelCursorForTest())
	}

	// The opencode channel groups all providers and their models together.
	view := modelModel.View()
	if !strings.Contains(view, "opencode") {
		t.Fatalf("expected view to show the agent channel bar, got:\n%s", view)
	}
	if !strings.Contains(view, "claude-3-7-sonnet") || !strings.Contains(view, "gpt-4o") {
		t.Fatalf("expected view to show models from all providers, got:\n%s", view)
	}
	if !strings.Contains(view, "anthropic") || !strings.Contains(view, "openai") {
		t.Fatalf("expected view to show provider group headers, got:\n%s", view)
	}
	// Provider group header should be rendered clearly with its model count.
	if !strings.Contains(view, "个模型") && !strings.Contains(view, "models") {
		t.Fatalf("expected provider group header label, got:\n%s", view)
	}
}

func TestTUIModelsTab_DetailModal_IKey(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupModelTestEnvironment(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)
	mTab9, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	modelModel := mTab9.(*tui.Model)

	// claude-3-5-haiku is index 0, press Down to select claude-3-7-sonnet (index 1)
	mDown, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyDown})
	modelModel = mDown.(*tui.Model)

	// 1. Press i to open single model detail modal
	mEnter, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	modelModel = mEnter.(*tui.Model)

	item, isOpen := modelModel.ModelDetailModalForTest()
	if !isOpen {
		t.Fatal("expected ModelDetailModal to be open after i")
	}
	if item.ModelID != "claude-3-7-sonnet" {
		t.Fatalf("expected detail modal for 'claude-3-7-sonnet', got %s", item.ModelID)
	}

	// Check full rendered modal body for all sections and details without truncation
	fullRender := tui.RenderModelDetailModalForTest(item, 80, 60)
	expectedSnippets := []string{
		"claude-3-7-sonnet",
		"Claude 3.7 Sonnet (Custom)",
		"anthropic",
		"200000 tokens (200k)",
		"128000 tokens (128k)",
		"8192 tokens (8k)",
		"$3 / 1M tokens",
		"$15 / 1M tokens",
		"$0.3 / 1M tokens",
		"$3.75 / 1M tokens",
		"thinking",
		"fast",
		"text, image",
	}
	for _, snip := range expectedSnippets {
		if !strings.Contains(fullRender, snip) {
			t.Fatalf("expected full detail modal to contain '%s', got:\n%s", snip, fullRender)
		}
	}

	// 2. Initial view in standard viewport should show [TOP] badge
	viewTop := modelModel.View()
	if !strings.Contains(viewTop, "[TOP]") {
		t.Fatalf("expected view to have [TOP] scroll badge, got:\n%s", viewTop)
	}

	// 3. Test scrolling keys (j, k, down, up, G, g)
	mScrollDown, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	modelModel = mScrollDown.(*tui.Model)

	mScrollBottom, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	modelModel = mScrollBottom.(*tui.Model)
	viewBottom := modelModel.View()
	if !strings.Contains(viewBottom, "[END]") {
		t.Fatalf("expected view to have [END] scroll badge after G, got:\n%s", viewBottom)
	}
	if !strings.Contains(viewBottom, "thinking") {
		t.Fatalf("expected view at bottom to display variants 'thinking', got:\n%s", viewBottom)
	}

	mScrollTop, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	modelModel = mScrollTop.(*tui.Model)
	if !strings.Contains(modelModel.View(), "[TOP]") {
		t.Fatalf("expected view to return to [TOP] after g, got:\n%s", modelModel.View())
	}

	// 3. Press Esc to close
	mEsc, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyEscape})
	modelModel = mEsc.(*tui.Model)
	if _, isOpen := modelModel.ModelDetailModalForTest(); isOpen {
		t.Fatal("expected ModelDetailModal to be closed after Esc")
	}

	// 4. Press i to open detail modal
	mSpace, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	modelModel = mSpace.(*tui.Model)
	if _, isOpen := modelModel.ModelDetailModalForTest(); !isOpen {
		t.Fatal("expected ModelDetailModal to be open after i")
	}

	// 5. Press 'q' to close modal
	mQ, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	modelModel = mQ.(*tui.Model)
	if _, isOpen := modelModel.ModelDetailModalForTest(); isOpen {
		t.Fatal("expected ModelDetailModal to be closed after 'q'")
	}
}

func TestTUIModelsTab_ConfigFileModal_KeyC(t *testing.T) {
	ctx := context.Background()
	svc, opencodePath, cleanup := setupModelTestEnvironment(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)
	mTab9, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	modelModel := mTab9.(*tui.Model)

	// 1. Press 'c' to open config file modal
	mC, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	modelModel = mC.(*tui.Model)

	path, isOpen := modelModel.ModelConfigFileModalForTest()
	if !isOpen {
		t.Fatal("expected ModelConfigFileModal to be open after 'c'")
	}
	if path != opencodePath {
		t.Fatalf("expected config file path %s, got %s", opencodePath, path)
	}

	viewConfig := modelModel.View()
	if !strings.Contains(viewConfig, "opencode.json") {
		t.Fatalf("expected config view to display opencode.json filename, got:\n%s", viewConfig)
	}
	// Line numbers should be formatted with "│"
	if !strings.Contains(viewConfig, "│") {
		t.Fatalf("expected config view to contain line number separator '│', got:\n%s", viewConfig)
	}
	if !strings.Contains(viewConfig, "claude-3-7-sonnet") {
		t.Fatalf("expected config view to show file content, got:\n%s", viewConfig)
	}

	// 2. Test scrolling inside config modal
	mDown, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyDown})
	modelModel = mDown.(*tui.Model)

	mUp, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyUp})
	modelModel = mUp.(*tui.Model)

	// 3. Press 'q' to close
	mClose, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	modelModel = mClose.(*tui.Model)
	if _, isOpen := modelModel.ModelConfigFileModalForTest(); isOpen {
		t.Fatal("expected ModelConfigFileModal to be closed after 'q'")
	}
}

func TestTUIModelsTab_FullDiffWithoutTruncation(t *testing.T) {
	// Generate two large JSON documents with > 150 different lines
	var origLines []string
	var updLines []string
	origLines = append(origLines, "{", `  "items": [`)
	updLines = append(updLines, "{", `  "items": [`)

	for i := 1; i <= 200; i++ {
		origLines = append(origLines, fmt.Sprintf(`    {"id": "model-%d", "version": 1},`, i))
		updLines = append(updLines, fmt.Sprintf(`    {"id": "model-%d", "version": 2, "enriched": true},`, i))
	}
	origLines = append(origLines, "  ]", "}")
	updLines = append(updLines, "  ]", "}")

	origText := strings.Join(origLines, "\n")
	updText := strings.Join(updLines, "\n")

	diff := opencode.BuildDiff(origText, updText)

	// Verify diff is not empty
	if diff == "" {
		t.Fatal("expected non-empty diff")
	}

	// Crucial check: verify that truncation text is NOT present
	if strings.Contains(diff, "more differences") {
		t.Fatalf("diff contains truncation message 'more differences', expected full diff:\n%s", diff)
	}

	// Verify that model-150 through model-200 are present in the diff
	for i := 150; i <= 200; i++ {
		key := fmt.Sprintf("model-%d", i)
		if !strings.Contains(diff, key) {
			t.Fatalf("expected diff to contain %s, got truncated diff", key)
		}
	}
}
