package ui

import (
	"os"
)

// truncate 截断字符串
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}

// ensureDir 确保目录存在
func ensureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// errorMessage 返回错误消息（如果存在）
func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	return "错误: " + err.Error()
}

// ViewSkillItem 用于 List 显示的通用 Item
type ViewSkillItem struct {
	IDStr   string // ID (为了避免与 list.Item 的 methods 冲突，改名)
	NameStr string
	DescStr string
}

func (i ViewSkillItem) Title() string       { return i.NameStr }
func (i ViewSkillItem) Description() string { return i.DescStr }
func (i ViewSkillItem) FilterValue() string { return i.NameStr + " " + i.DescStr }
