package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"asoul/internal/catalog"
	"asoul/internal/fsx"
	"asoul/internal/model"
)

// CreateDefaultConfig creates a default config.jsonc without interactive prompts.
func CreateDefaultConfig(m *Manager) error {
	return m.Update(func(cfg *model.Config) error {
		cfg.Version = 1
		cfg.Language = "auto"
		cfg.DefaultRoot = DefaultWorkspacePathCompact()
		cfg.Workspaces = []string{DefaultWorkspacePathCompact()}
		cfg.Targets = model.DefaultTargetPresets()
		if cfg.Model == nil {
			cfg.Model = &model.ModelConfig{}
		}
		cfg.Model.OfficialProviders = model.DefaultOfficialProviders()
		return nil
	})
}

// Exists checks if the config file exists on disk.
func (m *Manager) Exists() bool {
	return fsx.FileExists(m.configPath)
}

// RunInteractiveWizard prompts the user to configure asoul on first run.
func RunInteractiveWizard(m *Manager) (*model.Config, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("==================================================================")
	fmt.Println("  👋 Welcome to asoul - Agent Skills Local Hub!")
	fmt.Println("  First time setup: configuring asoul environment.")
	fmt.Printf("  Config location: %s\n", m.configPath)
	fmt.Println("==================================================================")
	fmt.Println()

	// 1. Language choice
	chosenLang := "auto"
	fmt.Printf("1. Choose Interface Language / 选择界面语言:\n")
	fmt.Printf("   1) English\n")
	fmt.Printf("   2) 简体中文 (Simplified Chinese)\n")
	fmt.Printf("   3) Auto-detect (跟随系统语言)\n")
	fmt.Printf("   [default: 3]: ")
	langInput, _ := reader.ReadString('\n')
	langInput = strings.TrimSpace(langInput)
	switch langInput {
	case "1":
		chosenLang = "en"
		fmt.Println("   ✓ Language set to English")
	case "2":
		chosenLang = "zh-CN"
		fmt.Println("   ✓ 语言已设置为简体中文")
	default:
		chosenLang = "auto"
		fmt.Println("   ✓ Set to auto-detect / 自动跟随系统语言")
	}
	fmt.Println()

	// 2. Ask for default workspace root
	defWorkspace := DefaultWorkspacePathCompact()
	fmt.Printf("2. Where would you like to store your agent skills workspace?\n")
	fmt.Printf("   [default: %s]: ", defWorkspace)
	wsInput, _ := reader.ReadString('\n')
	wsInput = strings.TrimSpace(wsInput)
	if wsInput == "" {
		wsInput = defWorkspace
	}

	expandedWS, err := fsx.ExpandUser(wsInput)
	if err != nil {
		expandedWS = wsInput
	}

	// If workspace directory doesn't exist, ask to initialize it now
	if !fsx.FileExists(catalog.CatalogPath(expandedWS)) {
		fmt.Printf("   Directory %s is not yet initialized as an asoul workspace.\n", wsInput)
		fmt.Print("   Initialize workspace there now? [Y/n]: ")
		initInput, _ := reader.ReadString('\n')
		initInput = strings.TrimSpace(strings.ToLower(initInput))
		if initInput == "" || initInput == "y" || initInput == "yes" {
			if err := catalog.InitWorkspace(expandedWS); err != nil {
				fmt.Printf("   Notice: Could not initialize workspace: %v\n", err)
			} else {
				fmt.Printf("   ✓ Initialized workspace at %s\n", expandedWS)
			}
		}
	}

	// 2. Ask for common agent targets
	targets := make(map[string]model.TargetConfig)
	fmt.Println()
	fmt.Println("2. Common Agent Skill Targets:")
	fmt.Println("   • codex:    ~/.codex/skills")
	fmt.Println("   • opencode: ~/.config/opencode/skills")
	fmt.Println("   • claude:   ~/.claude/skills")
	fmt.Print("   Enable standard agent targets (codex, opencode, claude)? [Y/n]: ")

	tgtInput, _ := reader.ReadString('\n')
	tgtInput = strings.TrimSpace(strings.ToLower(tgtInput))
	if tgtInput == "" || tgtInput == "y" || tgtInput == "yes" {
		targets = model.DefaultTargetPresets()
		fmt.Println("   ✓ Configured standard agent targets (codex, opencode, claude, cursor, etc.)")
	}

	// Ask if user wants to add a custom target
	fmt.Print("\n   Would you like to add an additional custom target? [y/N]: ")
	customInput, _ := reader.ReadString('\n')
	customInput = strings.TrimSpace(strings.ToLower(customInput))
	if customInput == "y" || customInput == "yes" {
		fmt.Print("   Target Name (e.g. my-project): ")
		cName, _ := reader.ReadString('\n')
		cName = strings.TrimSpace(cName)
		if cName != "" {
			fmt.Print("   Target Config Directory Path (e.g. ~/.my-agent or /path/to/project): ")
			cPath, _ := reader.ReadString('\n')
			cPath = strings.TrimSpace(cPath)
			if cPath != "" {
				targets[cName] = model.TargetConfig{
					Type:      model.TargetTypeAgent,
					Channel:   cName,
					ConfigDir: cPath,
					Paths: map[string]string{
						"skills": "${config_dir}/skills",
					},
				}
				fmt.Printf("   ✓ Added custom target %q (%s)\n", cName, cPath)
			}
		}
	}

	cfg := &model.Config{
		Version:     1,
		Language:    chosenLang,
		DefaultRoot: wsInput,
		Workspaces:  []string{wsInput},
		Targets:     targets,
		Model: &model.ModelConfig{
			OfficialProviders: model.DefaultOfficialProviders(),
		},
	}

	// Ensure config directory exists
	cfgDir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory %s: %w", cfgDir, err)
	}

	if err := m.Save(cfg); err != nil {
		return nil, fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Println()
	fmt.Printf("✓ Configuration successfully saved to: %s\n", m.configPath)
	fmt.Println("==================================================================")
	fmt.Println()

	return cfg, nil
}
