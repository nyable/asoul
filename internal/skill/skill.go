package skill

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Skill 表示一个 SKILL
type Skill struct {
	FolderName  string // 文件夹名称
	Name        string // SKILL.md 中的 name
	Description string // SKILL.md 中的 description
	Path        string // 完整路径
}

// DisplayName 返回显示名称
// 如果 FolderName == Name，返回 FolderName
// 否则返回 FolderName(Name)
func (s *Skill) DisplayName() string {
	if s.FolderName == s.Name {
		return s.FolderName
	}
	return s.FolderName + "(" + s.Name + ")"
}

// ID 返回用于标识的 ID（使用文件夹名）
func (s *Skill) ID() string {
	return s.FolderName
}

// ScanSkills 扫描指定目录下的所有 SKILLS
func ScanSkills(skillsPath string) ([]Skill, error) {
	var skills []Skill

	entries, err := os.ReadDir(skillsPath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillPath := filepath.Join(skillsPath, entry.Name())
		skillMdPath := filepath.Join(skillPath, "SKILL.md")

		// 检查 SKILL.md 是否存在
		if _, err := os.Stat(skillMdPath); os.IsNotExist(err) {
			continue
		}

		// 解析 SKILL.md
		name, description, err := parseSkillMd(skillMdPath)
		if err != nil {
			continue // 跳过无法解析的
		}

		// 必须有 name 和 description
		if name == "" || description == "" {
			continue
		}

		skills = append(skills, Skill{
			FolderName:  entry.Name(),
			Name:        name,
			Description: description,
			Path:        skillPath,
		})
	}

	return skills, nil
}

// parseSkillMd 解析 SKILL.md 文件的 YAML frontmatter
func parseSkillMd(path string) (name, description string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	inFrontmatter := false
	frontmatterLines := 0

	for scanner.Scan() {
		line := scanner.Text()

		// 检测 frontmatter 开始/结束
		if strings.TrimSpace(line) == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			} else {
				// frontmatter 结束
				break
			}
		}

		if !inFrontmatter {
			continue
		}

		frontmatterLines++

		// 简单解析 key: value 格式
		if strings.HasPrefix(line, "name:") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			name = strings.Trim(name, "\"'")
		} else if strings.HasPrefix(line, "description:") {
			description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			description = strings.Trim(description, "\"'")
		}
	}

	return name, description, scanner.Err()
}

// GetSkillByID 根据 ID（文件夹名）获取 SKILL
func GetSkillByID(skillsPath, id string) (*Skill, error) {
	skills, err := ScanSkills(skillsPath)
	if err != nil {
		return nil, err
	}

	for _, s := range skills {
		if s.ID() == id {
			return &s, nil
		}
	}

	return nil, nil
}

// GetSkillsByIDs 根据多个 ID 获取 SKILLS
func GetSkillsByIDs(skillsPath string, ids []string) ([]Skill, []string, error) {
	allSkills, err := ScanSkills(skillsPath)
	if err != nil {
		return nil, nil, err
	}

	skillMap := make(map[string]Skill)
	for _, s := range allSkills {
		skillMap[s.ID()] = s
	}

	var found []Skill
	var notFound []string

	for _, id := range ids {
		if s, ok := skillMap[id]; ok {
			found = append(found, s)
		} else {
			notFound = append(notFound, id)
		}
	}

	return found, notFound, nil
}
