package cmd

import (
	"fmt"
	"os"

	"github.com/nyable/asoul/internal/output"
	"github.com/spf13/cobra"
	"github.com/nyable/asoul/internal/config"
)

// initCmd 初始化命令
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "初始化 asoul 配置目录",
	Long: `创建 asoul 所需的配置目录和默认配置文件。

默认会创建:
  ~/.asoul/config.yaml  配置文件
  ~/.asoul/skills/      SKILLS 目录`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 获取配置目录
		configDir, err := config.GetConfigDir()
		if err != nil {
			return err
		}

		// 获取 SKILLS 目录
		skillsPath, err := config.GetDefaultSkillsPath()
		if err != nil {
			return err
		}

		// 创建配置目录
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return fmt.Errorf("创建配置目录失败: %w", err)
		}
		output.PrintSuccess("配置目录: %s\n", configDir)

		// 创建 SKILLS 目录
		if err := os.MkdirAll(skillsPath, 0755); err != nil {
			return fmt.Errorf("创建 SKILLS 目录失败: %w", err)
		}
		output.PrintSuccess("SKILLS 目录: %s\n", skillsPath)

		// 创建默认配置文件（如果不存在）
		configPath, _ := config.GetConfigPath()
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			defaultCfg, err := config.DefaultConfig()
			if err != nil {
				return err
			}
			if err := defaultCfg.Save(); err != nil {
				return fmt.Errorf("创建配置文件失败: %w", err)
			}
			output.PrintSuccess("配置文件: %s\n", configPath)
		} else {
			output.PrintInfo("配置文件已存在: %s\n", configPath)
		}

		fmt.Println()
		output.PrintInfo("初始化完成！")
		output.PrintInfo("你可以使用以下命令设置 SKILLS 路径:")
		output.PrintInfo("  asoul cfg set skills_path <path>\n")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
