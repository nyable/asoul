package cmd

import (
	"fmt"


	"github.com/nyable/asoul/internal/config"
	"github.com/nyable/asoul/internal/output"
	"github.com/spf13/cobra"
)

// configCmd 配置管理命令
var configCmd = &cobra.Command{
	Use:     "config",
	Aliases: []string{"cfg"},
	Short:   "配置管理",
	Long:    `查看和修改 asoul 配置。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 默认显示配置
		return showConfig()
	},
}

// configSetCmd 设置配置
var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "设置配置项",
	Long: `设置配置项。

支持的配置项:
  skills_path  SKILLS 源目录路径

示例:
  asoul config set skills_path /path/to/skills`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		value := args[1]

		switch key {
		case "skills_path":
			cfg.SetSkillsPath(value)
		default:
			return fmt.Errorf("未知的配置项: %s", key)
		}

		if err := cfg.Save(); err != nil {
			return fmt.Errorf("保存配置失败: %w", err)
		}

		result := output.NewSuccess("配置已更新", map[string]string{
			"key":   key,
			"value": value,
		})

		return output.Default.Print(result, func() {
			output.PrintSuccess("配置项 '%s' 已设置为: %s\n", key, value)
		})
	},
}

func showConfig() error {
	configPath, _ := config.GetConfigPath()

	// 构建结构化数据
	result := output.ConfigInfo{
		SkillsPath: cfg.SkillsPath,
		GroupCount: len(cfg.Groups),
		ConfigFile: configPath,
	}

	return output.Default.Print(result, func() {
		output.PrintHeader("当前配置")
		fmt.Println()

		headers := []string{"配置项", "值"}
		rows := [][]string{
			{"skills_path", cfg.SkillsPath},
			{"分组数量", fmt.Sprintf("%d", len(cfg.Groups))},
			{"配置文件", configPath},
		}

		output.PrintTable(headers, rows)
	})
}

func init() {
	configCmd.AddCommand(configSetCmd)
	rootCmd.AddCommand(configCmd)
}
