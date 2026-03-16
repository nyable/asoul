package cmd

import (
	"fmt"
	"strings"


	"github.com/nyable/asoul/internal/installer"
	"github.com/nyable/asoul/internal/output"
	"github.com/spf13/cobra"
)

var (
	installTarget string
	installGroup  string
	installLink   bool
)

// installCmd 安装命令
var installCmd = &cobra.Command{
	Use:     "install [skills...]",
	Aliases: []string{"i"},
	Short:   "安装 SKILLS 到目标项目",
	Long: `安装一个或多个 SKILLS 到目标项目。

示例:
  asoul install docx -t ./my-project
  asoul i docx pdf xlsx -t ./my-project
  asoul i -g 文档处理 -t ./my-project
  asoul i docx -t ./my-project -l  # 使用软链接模式`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if installTarget == "" {
			return fmt.Errorf("必须指定目标路径 (-t/--target)")
		}

		mode := installer.ModeCopy
		if installLink {
			mode = installer.ModeLink
		}

		inst := installer.NewInstaller(cfg.SkillsPath, installTarget, mode)

		var installed []string
		var failed map[string]error

		if installGroup != "" {
			// 安装分组
			installed, failed = inst.InstallGroup(cfg, installGroup)
		} else if len(args) > 0 {
			// 安装指定的 SKILLS
			installed, failed = inst.Install(args)
		} else {
			return fmt.Errorf("必须指定要安装的 SKILLS 或使用 -g 指定分组")
		}

		// 构建结构化数据
		modeStr := "copy"
		if mode == installer.ModeLink {
			modeStr = "link"
		}

		result := output.InstallResult{
			Installed: installed,
			Failed:    make(map[string]string),
			Mode:      modeStr,
			Target:    installTarget,
		}

		for id, err := range failed {
			result.Failed[id] = err.Error()
		}

		// 统一输出
		return output.Default.Print(result, func() {
			if len(installed) > 0 {
				output.PrintSuccess("成功安装: %s\n", strings.Join(installed, ", "))
			}

			if len(failed) > 0 {
				for id, err := range failed {
					output.PrintError("安装失败 [%s]: %v\n", id, err)
				}
			}

			if len(installed) > 0 {
				modeDisplay := "复制"
				if mode == installer.ModeLink {
					modeDisplay = "软链接"
				}
				output.PrintInfo("安装模式: %s\n", modeDisplay)
				output.PrintInfo("目标路径: %s\n", installTarget)
			}
		})
	},
}

func init() {
	installCmd.Flags().StringVarP(&installTarget, "target", "t", "", "目标项目路径")
	installCmd.Flags().StringVarP(&installGroup, "group", "g", "", "安装指定分组的所有 SKILLS")
	installCmd.Flags().BoolVarP(&installLink, "link", "l", false, "使用软链接模式（默认为复制模式）")

	rootCmd.AddCommand(installCmd)
}
