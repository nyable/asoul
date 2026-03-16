package cmd

import (
	"github.com/nyable/asoul/internal/config"
	"github.com/nyable/asoul/internal/output"
	"github.com/nyable/asoul/internal/ui"
	"github.com/spf13/cobra"
)

var (
	cfg          *config.Config
	interactive  bool
	outputFormat string
)

// rootCmd 根命令
var rootCmd = &cobra.Command{
	Use:   "asoul",
	Short: "SKILLS 管理工具",
	Long:  `asoul 是一个用于管理 AI Agent SKILLS 的命令行工具，支持安装、卸载、分组管理等功能。`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// 设置输出格式
		if outputFormat != "" {
			output.Default.SetFormat(outputFormat)
		}

		var err error
		cfg, err = config.Load()
		return err
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if interactive {
			return ui.NewInteractive(cfg).Run()
		}
		return cmd.Help()
	},
}

func init() {
	rootCmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "进入交互模式")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "", "输出格式: json, text (默认: text)")
}

// Execute 执行根命令
func Execute() error {
	return rootCmd.Execute()
}

// GetConfig 获取配置
func GetConfig() *config.Config {
	return cfg
}
