package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/nyable/asoul/internal/config"
	"github.com/nyable/asoul/internal/skill"
)

// RenderSkillDetail 渲染统一的 SKILL 详情视图
func RenderSkillDetail(s skill.Skill, width int) string {
	if width <= 0 {
		width = 80 // 默认宽度
	}

	// 内容宽度留出边框(2)和padding(4)
	contentWidth := width - 8
	if contentWidth < 40 {
		contentWidth = 40
	}

	descStyle := lipgloss.NewStyle().Width(contentWidth)

	// 使用现有样式
	content := titleStyle.Render("SKILL 详情") + "\n\n"
	content += fmt.Sprintf("ID:   %s\n", s.ID())
	content += fmt.Sprintf("名称: %s\n", s.DisplayName())
	content += fmt.Sprintf("路径: %s\n\n", s.Path)
	content += "描述:\n"
	content += descStyle.Render(s.Description) + "\n"

	// 限制边框宽度
	return detailBorderStyle.Width(width - 2).Render(content)
}

// RenderGroupDetail 渲染统一的分组详情视图
func RenderGroupDetail(g config.Group, width int) string {
	if width <= 0 {
		width = 80 // 默认宽度
	}

	// 内容宽度留出边框(2)和padding(4)
	contentWidth := width - 8
	if contentWidth < 40 {
		contentWidth = 40
	}

	descStyle := lipgloss.NewStyle().Width(contentWidth)

	content := titleStyle.Render("分组详情") + "\n\n"
	content += fmt.Sprintf("名称: %s\n", g.Name)
	content += "描述:\n"
	content += descStyle.Render(g.Description) + "\n\n"
	content += fmt.Sprintf("包含 SKILLS (%d):\n", len(g.Skills))

	if len(g.Skills) == 0 {
		content += "(无)"
	} else {
		// 限制显示的 skills 数量或者长度
		skillsList := strings.Join(g.Skills, ", ")
		content += descStyle.Render(skillsList)
	}
	content += "\n"

	// 限制边框宽度
	return detailBorderStyle.Width(width - 2).Render(content)
}
