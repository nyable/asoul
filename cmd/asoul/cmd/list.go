package cmd

import (
	"fmt"


	"github.com/nyable/asoul/internal/installer"
	"github.com/nyable/asoul/internal/output"
	"github.com/nyable/asoul/internal/skill"
	"github.com/spf13/cobra"
)

var listTarget string

// listCmd 列出 SKILLS
var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "列出所有可用的 SKILLS",
	RunE: func(cmd *cobra.Command, args []string) error {
		var skills []skill.Skill
		var err error

		if listTarget != "" {
			inst := installer.NewInstaller(cfg.SkillsPath, listTarget, installer.ModeCopy)
			var ids []string
			ids, err = inst.ListInstalled()
			if err == nil {
				var notFound []string
				skills, notFound, err = skill.GetSkillsByIDs(cfg.SkillsPath, ids)

				// 添加未找到详情的 SKILLS
				for _, id := range notFound {
					skills = append(skills, skill.Skill{
						FolderName:  id,
						Name:        id,
						Description: "（本地已安装，但在源中未找到详情）",
					})
				}
			}
		} else {
			skills, err = skill.ScanSkills(cfg.SkillsPath)
		}

		if err != nil {
			return fmt.Errorf("获取 SKILLS 列表失败: %w", err)
		}

		// 构建结构化数据
		result := output.ListResult{
			Skills: make([]output.SkillInfo, 0, len(skills)),
			Count:  len(skills),
			Path:   cfg.SkillsPath,
		}

		for _, s := range skills {
			result.Skills = append(result.Skills, output.SkillInfo{
				ID:          s.ID(),
				Name:        s.DisplayName(),
				Description: s.Description,
				Path:        s.Path,
			})
		}

		// 统一输出
		return output.Default.Print(result, func() {
			if len(skills) == 0 {
				if listTarget != "" {
					output.PrintWarning("目标项目没有安装任何 SKILLS")
				} else {
					output.PrintWarning("没有可用的 SKILLS")
					output.PrintInfo("SKILLS 路径: %s\n", cfg.SkillsPath)
				}
				return
			}

			if listTarget != "" {
				output.PrintHeader("已安装 SKILLS (目标: %s)\n", listTarget)
			} else {
				output.PrintHeader("可用的 SKILLS")
			}
			fmt.Println()

			headers := []string{"ID", "名称", "描述"}
			var rows [][]string

			for _, s := range skills {
				rows = append(rows, []string{
					s.ID(),
					s.DisplayName(),
					truncateStr(s.Description, 60),
				})
			}

			output.PrintTable(headers, rows)
			fmt.Println()
			output.PrintInfo("共 %d 个 SKILLS\n", len(skills))
		})
	},
}

func init() {
	listCmd.Flags().StringVarP(&listTarget, "target", "t", "", "从目标项目列出已安装的 SKILLS")
	rootCmd.AddCommand(listCmd)
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
