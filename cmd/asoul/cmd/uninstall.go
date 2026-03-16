package cmd

import (
	"fmt"
	"strings"


	"github.com/nyable/asoul/internal/installer"
	"github.com/nyable/asoul/internal/output"
	"github.com/spf13/cobra"
)

var (
	uninstallTarget string
	uninstallGroup  string
)

// uninstallCmd 卸载命令
var uninstallCmd = &cobra.Command{
	Use:     "uninstall [skills...]",
	Aliases: []string{"rm"},
	Short:   "卸载 SKILLS",
	Long: `从目标项目卸载一个或多个 SKILLS。

示例:
  asoul uninstall docx -t ./my-project
  asoul rm docx pdf -t ./my-project
  asoul rm -g 文档处理 -t ./my-project`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if uninstallTarget == "" {
			return fmt.Errorf("必须指定目标路径 (-t/--target)")
		}

		inst := installer.NewInstaller(cfg.SkillsPath, uninstallTarget, installer.ModeCopy)
		var removed, skipped []string
		var failed map[string]error

		if uninstallGroup != "" {
			// 按分组卸载
			removed, skipped, failed = inst.UninstallGroup(cfg, uninstallGroup)
		} else if len(args) > 0 {
			// 按名称卸载
			removed, failed = inst.Uninstall(args)
		} else {
			return fmt.Errorf("必须指定要卸载的 SKILLS 或使用 -g 指定分组")
		}

		// 构建结构化数据
		result := output.UninstallResult{
			Removed: removed,
			Failed:  make(map[string]string),
			Target:  uninstallTarget,
		}

		for id, err := range failed {
			result.Failed[id] = err.Error()
		}

		// 统一输出
		return output.Default.Print(result, func() {
			if len(removed) > 0 {
				output.PrintSuccess("成功卸载: %s\n", strings.Join(removed, ", "))
			}

			if len(skipped) > 0 {
				output.PrintWarning("跳过(未安装): %s\n", strings.Join(skipped, ", "))
			}

			if len(failed) > 0 {
				for id, err := range failed {
					output.PrintError("卸载失败 [%s]: %v\n", id, err)
				}
			}

			if len(removed) == 0 && len(failed) == 0 && len(skipped) == 0 {
				output.PrintInfo("没有进行任何操作")
			}
		})
	},
}

func init() {
	uninstallCmd.Flags().StringVarP(&uninstallTarget, "target", "t", "", "目标项目路径")
	uninstallCmd.Flags().StringVarP(&uninstallGroup, "group", "g", "", "卸载指定分组的所有 SKILLS")

	rootCmd.AddCommand(uninstallCmd)
}
