package installer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/nyable/asoul/internal/config"
	"github.com/nyable/asoul/internal/skill"
)

// Mode 安装模式
type Mode string

const (
	ModeCopy Mode = "copy" // 覆盖复制
	ModeLink Mode = "link" // 软链接
)

// Installer 安装器
type Installer struct {
	SourcePath string // SKILLS 源目录
	TargetPath string // 目标项目路径
	Mode       Mode   // 安装模式
}

// NewInstaller 创建安装器
func NewInstaller(sourcePath, targetPath string, mode Mode) *Installer {
	return &Installer{
		SourcePath: sourcePath,
		TargetPath: targetPath,
		Mode:       mode,
	}
}

// getTargetSkillsDir 获取目标项目的 .agent/skills 目录
func (i *Installer) getTargetSkillsDir() string {
	return filepath.Join(i.TargetPath, ".agent", "skills")
}

// EnsureTargetDir 确保目标目录存在
func (i *Installer) EnsureTargetDir() error {
	return os.MkdirAll(i.getTargetSkillsDir(), 0755)
}

// Install 安装指定的 SKILLS
func (i *Installer) Install(skillIDs []string) (installed []string, failed map[string]error) {
	failed = make(map[string]error)

	if err := i.EnsureTargetDir(); err != nil {
		for _, id := range skillIDs {
			failed[id] = fmt.Errorf("无法创建目标目录: %w", err)
		}
		return nil, failed
	}

	skills, notFound, err := skill.GetSkillsByIDs(i.SourcePath, skillIDs)
	if err != nil {
		for _, id := range skillIDs {
			failed[id] = err
		}
		return nil, failed
	}

	// 标记未找到的
	for _, id := range notFound {
		failed[id] = fmt.Errorf("SKILL 不存在")
	}

	// 安装找到的
	for _, s := range skills {
		if err := i.installOne(s); err != nil {
			failed[s.ID()] = err
		} else {
			installed = append(installed, s.ID())
		}
	}

	return installed, failed
}

// installOne 安装单个 SKILL
func (i *Installer) installOne(s skill.Skill) error {
	targetDir := filepath.Join(i.getTargetSkillsDir(), s.FolderName)

	// 先删除已存在的目录或链接
	if err := os.RemoveAll(targetDir); err != nil {
		return fmt.Errorf("无法删除已存在的目录: %w", err)
	}

	switch i.Mode {
	case ModeLink:
		return i.createSymlink(s.Path, targetDir)
	case ModeCopy:
		fallthrough
	default:
		return i.copyDir(s.Path, targetDir)
	}
}

// createSymlink 创建软链接
func (i *Installer) createSymlink(source, target string) error {
	return os.Symlink(source, target)
}

// copyDir 递归复制目录
func (i *Installer) copyDir(source, target string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 计算相对路径
		relPath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(target, relPath)

		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}

		return copyFile(path, targetPath, info.Mode())
	})
}

// copyFile 复制单个文件
func copyFile(source, target string, mode os.FileMode) error {
	srcFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// InstallGroup 安装整个分组
func (i *Installer) InstallGroup(cfg *config.Config, groupName string) (installed []string, failed map[string]error) {
	group := cfg.GetGroup(groupName)
	if group == nil {
		return nil, map[string]error{groupName: fmt.Errorf("分组不存在")}
	}

	return i.Install(group.Skills)
}

// UninstallGroup 按分组卸载 SKILLS（不存在则忽略）
func (i *Installer) UninstallGroup(cfg *config.Config, groupName string) (removed []string, skipped []string, failed map[string]error) {
	group := cfg.GetGroup(groupName)
	if group == nil {
		return nil, nil, map[string]error{groupName: fmt.Errorf("分组不存在")}
	}

	failed = make(map[string]error)
	targetDir := i.getTargetSkillsDir()

	for _, id := range group.Skills {
		skillPath := filepath.Join(targetDir, id)

		// 检查是否存在，不存在则跳过
		if _, err := os.Lstat(skillPath); os.IsNotExist(err) {
			skipped = append(skipped, id)
			continue
		}

		// 删除（无论是目录还是软链接）
		if err := os.RemoveAll(skillPath); err != nil {
			failed[id] = err
		} else {
			removed = append(removed, id)
		}
	}

	return removed, skipped, failed
}

// Uninstall 卸载指定的 SKILLS
func (i *Installer) Uninstall(skillIDs []string) (removed []string, failed map[string]error) {
	failed = make(map[string]error)
	targetDir := i.getTargetSkillsDir()

	for _, id := range skillIDs {
		skillPath := filepath.Join(targetDir, id)

		// 检查是否存在
		if _, err := os.Lstat(skillPath); os.IsNotExist(err) {
			failed[id] = fmt.Errorf("SKILL 未安装")
			continue
		}

		// 删除（无论是目录还是软链接）
		if err := os.RemoveAll(skillPath); err != nil {
			failed[id] = err
		} else {
			removed = append(removed, id)
		}
	}

	return removed, failed
}

// ListInstalled 列出已安装的 SKILLS
func (i *Installer) ListInstalled() ([]string, error) {
	targetDir := i.getTargetSkillsDir()

	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		return []string{}, nil
	}

	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return nil, err
	}

	var installed []string
	for _, entry := range entries {
		installed = append(installed, entry.Name())
	}

	return installed, nil
}
