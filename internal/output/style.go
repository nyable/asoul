package output

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	// StyleSuccess 成功信息样式 (绿色)
	StyleSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	
	// StyleError 错误信息样式 (红色)
	StyleError = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	
	// StyleWarning 警告信息样式 (黄色)
	StyleWarning = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	
	// StyleInfo 提示信息样式 (蓝色)
	StyleInfo = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	
	// StyleHeader 标题样式 (加粗，紫色)
	StyleHeader = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))

	// StyleSection 章节标题样式 (加粗，青色)
	StyleSection = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
)

// PrintSuccess 打印成功信息
func PrintSuccess(format string, a ...interface{}) {
	fmt.Println(StyleSuccess.Render("[SUCCESS] ") + fmt.Sprintf(format, a...))
}

// PrintError 打印错误信息
func PrintError(format string, a ...interface{}) {
	fmt.Println(StyleError.Render("[ERROR] ") + fmt.Sprintf(format, a...))
}

// PrintWarning 打印警告信息
func PrintWarning(format string, a ...interface{}) {
	fmt.Println(StyleWarning.Render("[WARNING] ") + fmt.Sprintf(format, a...))
}

// PrintInfo 打印提示信息
func PrintInfo(format string, a ...interface{}) {
	fmt.Println(StyleInfo.Render("[INFO] ") + fmt.Sprintf(format, a...))
}

// PrintHeader 打印标题
func PrintHeader(format string, a ...interface{}) {
	fmt.Println(StyleHeader.Render(fmt.Sprintf(format, a...)))
}

// PrintSection 打印章节标题
func PrintSection(format string, a ...interface{}) {
	fmt.Println(StyleSection.Render(fmt.Sprintf(format, a...)))
}

// PrintTable 打印简单表格
func PrintTable(headers []string, rows [][]string) {
	if len(headers) == 0 && len(rows) == 0 {
		return
	}
	
	// 计算每列的最大宽度
	colWidths := make([]int, 0)
	if len(headers) > 0 {
		for _, h := range headers {
			colWidths = append(colWidths, lipgloss.Width(h))
		}
	} else if len(rows) > 0 {
		for range rows[0] {
			colWidths = append(colWidths, 0)
		}
	}

	for _, row := range rows {
		for i, cell := range row {
			w := lipgloss.Width(cell)
			if i < len(colWidths) {
				if w > colWidths[i] {
					colWidths[i] = w
				}
			} else {
				colWidths = append(colWidths, w)
			}
		}
	}

	// 至少保持一定宽度
	for i := range colWidths {
		if colWidths[i] < 4 {
			colWidths[i] = 4
		}
		colWidths[i] += 2 // Padding
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("43"))
	
	// 渲染表头
	if len(headers) > 0 {
		var headerRow string
		for i, h := range headers {
			width := colWidths[i]
			headerRow += headerStyle.Render(lipgloss.PlaceHorizontal(width, lipgloss.Left, " "+h+" "))
		}
		fmt.Println(headerRow)
		
		// 分隔线
		var separator string
		for _, w := range colWidths {
			separator += strings.Repeat("─", w)
		}
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(separator))
	}

	// 渲染数据
	for _, row := range rows {
		var rowStr string
		for i, cell := range row {
			if i < len(colWidths) {
				width := colWidths[i]
				rowStr += lipgloss.PlaceHorizontal(width, lipgloss.Left, " "+cell+" ")
			}
		}
		fmt.Println(rowStr)
	}
}
