package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Group 表示一个 SKILL 分组
type Group struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Skills      []string `yaml:"skills"`
}

// Config 应用配置
type Config struct {
	SkillsPath string  `yaml:"skills_path"`
	Groups     []Group `yaml:"groups"`
}

// 默认配置目录名
const configDirName = ".asoul"
const configFileName = "config.yaml"
const skillsDirName = "skills"

// GetConfigDir 获取配置目录路径
func GetConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, configDirName), nil
}

// GetConfigPath 获取配置文件路径
func GetConfigPath() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, configFileName), nil
}

// GetDefaultSkillsPath 获取默认 SKILLS 目录路径
func GetDefaultSkillsPath() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, skillsDirName), nil
}

// EnsureConfigDir 确保配置目录存在
func EnsureConfigDir() error {
	configDir, err := GetConfigDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(configDir, 0755)
}

// Load 加载配置文件
func Load() (*Config, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	// 如果配置文件不存在，返回默认配置
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return DefaultConfig()
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// DefaultConfig 返回默认配置
func DefaultConfig() (*Config, error) {
	skillsPath, err := GetDefaultSkillsPath()
	if err != nil {
		return nil, err
	}

	return &Config{
		SkillsPath: skillsPath,
		Groups:     []Group{},
	}, nil
}

// Save 保存配置到文件
func (c *Config) Save() error {
	if err := EnsureConfigDir(); err != nil {
		return err
	}

	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

// SetSkillsPath 设置 SKILLS 路径
func (c *Config) SetSkillsPath(path string) {
	c.SkillsPath = path
}

// AddGroup 添加分组
func (c *Config) AddGroup(group Group) {
	c.Groups = append(c.Groups, group)
}

// GetGroup 获取指定名称的分组
func (c *Config) GetGroup(name string) *Group {
	for i := range c.Groups {
		if c.Groups[i].Name == name {
			return &c.Groups[i]
		}
	}
	return nil
}

// UpdateGroup 更新分组
func (c *Config) UpdateGroup(name string, addSkills, removeSkills []string) bool {
	group := c.GetGroup(name)
	if group == nil {
		return false
	}

	// 移除指定的 skills
	if len(removeSkills) > 0 {
		removeSet := make(map[string]bool)
		for _, s := range removeSkills {
			removeSet[s] = true
		}
		newSkills := []string{}
		for _, s := range group.Skills {
			if !removeSet[s] {
				newSkills = append(newSkills, s)
			}
		}
		group.Skills = newSkills
	}

	// 添加新的 skills（去重）
	if len(addSkills) > 0 {
		existSet := make(map[string]bool)
		for _, s := range group.Skills {
			existSet[s] = true
		}
		for _, s := range addSkills {
			if !existSet[s] {
				group.Skills = append(group.Skills, s)
				existSet[s] = true
			}
		}
	}

	return true
}

// DeleteGroup 删除分组
func (c *Config) DeleteGroup(name string) bool {
	for i := range c.Groups {
		if c.Groups[i].Name == name {
			c.Groups = append(c.Groups[:i], c.Groups[i+1:]...)
			return true
		}
	}
	return false
}
