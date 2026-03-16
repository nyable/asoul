package output

// Result 通用命令执行结果
type Result struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// NewSuccess 创建成功结果
func NewSuccess(message string, data interface{}) Result {
	return Result{
		Success: true,
		Message: message,
		Data:    data,
	}
}

// NewError 创建错误结果
func NewError(err error) Result {
	return Result{
		Success: false,
		Error:   err.Error(),
	}
}

// SkillInfo SKILL 信息（用于 JSON 输出）
type SkillInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path,omitempty"`
}

// ListResult list 命令的结果
type ListResult struct {
	Skills []SkillInfo `json:"skills"`
	Count  int         `json:"count"`
	Path   string      `json:"skills_path"`
}

// InstallResult install 命令的结果
type InstallResult struct {
	Installed []string          `json:"installed"`
	Failed    map[string]string `json:"failed,omitempty"`
	Mode      string            `json:"mode"`
	Target    string            `json:"target"`
}

// UninstallResult uninstall 命令的结果
type UninstallResult struct {
	Removed []string          `json:"removed"`
	Failed  map[string]string `json:"failed,omitempty"`
	Target  string            `json:"target"`
}

// GroupInfo 分组信息
type GroupInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Skills      []string `json:"skills"`
	Count       int      `json:"count"`
}

// GroupListResult group list 命令的结果
type GroupListResult struct {
	Groups []GroupInfo `json:"groups"`
	Count  int         `json:"count"`
}

// ConfigInfo 配置信息
type ConfigInfo struct {
	SkillsPath string `json:"skills_path"`
	GroupCount int    `json:"group_count"`
	ConfigFile string `json:"config_file"`
}
