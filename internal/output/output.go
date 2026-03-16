// Package output 提供统一的命令行输出格式化功能
package output

import (
	"encoding/json"
	"io"
	"os"
)

// Format 输出格式类型
type Format string

const (
	FormatText Format = "text" // 人类可读的文本格式（默认）
	FormatJSON Format = "json" // JSON 格式
	FormatYAML Format = "yaml" // YAML 格式（预留）
)

// Output 统一输出管理器
type Output struct {
	format Format
	writer io.Writer
}

// 默认全局输出实例
var Default = New(FormatText)

// New 创建新的 Output 实例
func New(format Format) *Output {
	return &Output{
		format: format,
		writer: os.Stdout,
	}
}

// SetFormat 设置输出格式
func (o *Output) SetFormat(format string) {
	switch format {
	case "json":
		o.format = FormatJSON
	case "yaml":
		o.format = FormatYAML
	default:
		o.format = FormatText
	}
}

// GetFormat 获取当前输出格式
func (o *Output) GetFormat() Format {
	return o.format
}

// IsJSON 判断是否为 JSON 格式
func (o *Output) IsJSON() bool {
	return o.format == FormatJSON
}

// IsText 判断是否为文本格式
func (o *Output) IsText() bool {
	return o.format == FormatText
}

// Print 智能输出
// data: 用于 JSON/YAML 输出的结构化数据
// textFn: 用于 text 格式的自定义输出函数
func (o *Output) Print(data interface{}, textFn func()) error {
	switch o.format {
	case FormatJSON:
		return o.printJSON(data)
	case FormatYAML:
		return o.printYAML(data)
	default:
		if textFn != nil {
			textFn()
		}
		return nil
	}
}

// PrintJSON 仅输出 JSON 数据
func (o *Output) PrintJSON(data interface{}) error {
	return o.printJSON(data)
}

// printJSON 内部 JSON 输出实现
func (o *Output) printJSON(data interface{}) error {
	encoder := json.NewEncoder(o.writer)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(data)
}

// printYAML YAML 输出（预留实现）
func (o *Output) printYAML(data interface{}) error {
	// TODO: 如需 YAML 支持，可引入 gopkg.in/yaml.v3
	// 目前回退到 JSON
	return o.printJSON(data)
}

// SetWriter 设置输出写入器（用于测试）
func (o *Output) SetWriter(w io.Writer) {
	o.writer = w
}
