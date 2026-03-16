package cmd

import (
	"fmt"
	"strings"


	"github.com/nyable/asoul/internal/config"
	"github.com/nyable/asoul/internal/output"
	"github.com/spf13/cobra"
)

var (
	groupDescription string
	groupSkills      string
	groupAdd         string
	groupRemove      string
)

// groupCmd 分组管理命令
var groupCmd = &cobra.Command{
	Use:     "group",
	Aliases: []string{"g"},
	Short:   "分组管理",
	Long:    `管理 SKILLS 分组，支持创建、查看、修改和删除分组。`,
}

// groupListCmd 列出分组
var groupListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "列出所有分组",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 构建结构化数据
		result := output.GroupListResult{
			Groups: make([]output.GroupInfo, 0, len(cfg.Groups)),
			Count:  len(cfg.Groups),
		}

		for _, g := range cfg.Groups {
			result.Groups = append(result.Groups, output.GroupInfo{
				Name:        g.Name,
				Description: g.Description,
				Skills:      g.Skills,
				Count:       len(g.Skills),
			})
		}

		return output.Default.Print(result, func() {
			if len(cfg.Groups) == 0 {
				output.PrintWarning("没有分组")
				return
			}

			output.PrintHeader("分组列表")
			fmt.Println()

			headers := []string{"名称", "描述", "SKILLS"}
			var rows [][]string

			for _, g := range cfg.Groups {
				rows = append(rows, []string{
					g.Name,
					truncateStr(g.Description, 30),
					truncateStr(strings.Join(g.Skills, ", "), 40),
				})
			}

			output.PrintTable(headers, rows)
			fmt.Println()
			output.PrintInfo("共 %d 个分组\n", len(cfg.Groups))
		})
	},
}

// groupShowCmd 查看分组详情
var groupShowCmd = &cobra.Command{
	Use:     "show <name>",
	Aliases: []string{"info"},
	Short:   "查看分组详情",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		group := cfg.GetGroup(name)
		if group == nil {
			return fmt.Errorf("分组不存在: %s", name)
		}

		// 构建结构化数据
		result := output.GroupInfo{
			Name:        group.Name,
			Description: group.Description,
			Skills:      group.Skills,
			Count:       len(group.Skills),
		}

		return output.Default.Print(result, func() {
			output.PrintSection("分组: %s", group.Name)
			fmt.Println()
			output.PrintInfo("描述: %s\n", group.Description)
			output.PrintInfo("SKILLS (%d):\n", len(group.Skills))

			for _, s := range group.Skills {
				fmt.Printf("  - %s\n", s)
			}
		})
	},
}

// groupCreateCmd 创建分组
var groupCreateCmd = &cobra.Command{
	Use:     "create <name>",
	Aliases: []string{"new"},
	Short:   "创建分组",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		if cfg.GetGroup(name) != nil {
			return fmt.Errorf("分组已存在: %s", name)
		}

		skills := []string{}
		if groupSkills != "" {
			skills = strings.Split(groupSkills, ",")
			for i := range skills {
				skills[i] = strings.TrimSpace(skills[i])
			}
		}

		newGroup := config.Group{
			Name:        name,
			Description: groupDescription,
			Skills:      skills,
		}
		cfg.AddGroup(newGroup)

		if err := cfg.Save(); err != nil {
			return fmt.Errorf("保存配置失败: %w", err)
		}

		// 构建结构化数据
		result := output.NewSuccess("分组创建成功", output.GroupInfo{
			Name:        name,
			Description: groupDescription,
			Skills:      skills,
			Count:       len(skills),
		})

		return output.Default.Print(result, func() {
			output.PrintSuccess("分组 '%s' 创建成功\n", name)
		})
	},
}

// groupUpdateCmd 修改分组
var groupUpdateCmd = &cobra.Command{
	Use:     "update <name>",
	Aliases: []string{"edit"},
	Short:   "修改分组",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		group := cfg.GetGroup(name)
		if group == nil {
			return fmt.Errorf("分组不存在: %s", name)
		}

		// 更新描述
		if groupDescription != "" {
			group.Description = groupDescription
		}

		// 添加和移除 SKILLS
		var addSkills, removeSkills []string

		if groupAdd != "" {
			addSkills = strings.Split(groupAdd, ",")
			for i := range addSkills {
				addSkills[i] = strings.TrimSpace(addSkills[i])
			}
		}

		if groupRemove != "" {
			removeSkills = strings.Split(groupRemove, ",")
			for i := range removeSkills {
				removeSkills[i] = strings.TrimSpace(removeSkills[i])
			}
		}

		cfg.UpdateGroup(name, addSkills, removeSkills)

		if err := cfg.Save(); err != nil {
			return fmt.Errorf("保存配置失败: %w", err)
		}

		// 获取更新后的分组信息
		updatedGroup := cfg.GetGroup(name)
		result := output.NewSuccess("分组更新成功", output.GroupInfo{
			Name:        updatedGroup.Name,
			Description: updatedGroup.Description,
			Skills:      updatedGroup.Skills,
			Count:       len(updatedGroup.Skills),
		})

		return output.Default.Print(result, func() {
			output.PrintSuccess("分组 '%s' 更新成功\n", name)
		})
	},
}

// groupDeleteCmd 删除分组
var groupDeleteCmd = &cobra.Command{
	Use:     "delete <name>",
	Aliases: []string{"rm"},
	Short:   "删除分组",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		if !cfg.DeleteGroup(name) {
			return fmt.Errorf("分组不存在: %s", name)
		}

		if err := cfg.Save(); err != nil {
			return fmt.Errorf("保存配置失败: %w", err)
		}

		result := output.NewSuccess("分组已删除", map[string]string{"name": name})

		return output.Default.Print(result, func() {
			output.PrintSuccess("分组 '%s' 已删除\n", name)
		})
	},
}

func init() {
	// 添加子命令
	groupCmd.AddCommand(groupListCmd)
	groupCmd.AddCommand(groupShowCmd)
	groupCmd.AddCommand(groupCreateCmd)
	groupCmd.AddCommand(groupUpdateCmd)
	groupCmd.AddCommand(groupDeleteCmd)

	// 添加标志
	groupCreateCmd.Flags().StringVarP(&groupDescription, "description", "d", "", "分组描述")
	groupCreateCmd.Flags().StringVarP(&groupSkills, "skills", "s", "", "包含的 SKILLS（逗号分隔）")

	groupUpdateCmd.Flags().StringVarP(&groupDescription, "description", "d", "", "更新分组描述")
	groupUpdateCmd.Flags().StringVarP(&groupAdd, "add", "a", "", "添加 SKILLS（逗号分隔）")
	groupUpdateCmd.Flags().StringVarP(&groupRemove, "remove", "r", "", "移除 SKILLS（逗号分隔）")

	rootCmd.AddCommand(groupCmd)
}
