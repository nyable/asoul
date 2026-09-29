package skill

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"asoul/internal/fsx"
	"asoul/internal/model"

	"gopkg.in/yaml.v3"
)

var idRegex = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// ValidateID checks if the skill ID matches the naming convention.
func ValidateID(id string) error {
	if len(id) == 0 {
		return fmt.Errorf("skill ID cannot be empty")
	}
	if len(id) > 64 {
		return fmt.Errorf("skill ID too long (max 64 characters): %s", id)
	}
	if !idRegex.MatchString(id) {
		return fmt.Errorf("invalid skill ID %q: must match lowercase alphanumeric and dashes (%s)", id, `^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	}
	return nil
}

// ParseSkillContent parses the YAML frontmatter and markdown body from byte content.
func ParseSkillContent(data []byte) (*model.SkillMetadata, string, error) {
	content := string(data)
	if !strings.HasPrefix(content, "---") {
		return nil, "", fmt.Errorf("missing YAML frontmatter (must start with '---')")
	}

	// Split frontmatter and body
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return nil, "", fmt.Errorf("invalid frontmatter delimiters")
	}

	frontmatterStr := parts[1]
	body := strings.TrimSpace(parts[2])

	var meta model.SkillMetadata
	if err := yaml.Unmarshal([]byte(frontmatterStr), &meta); err != nil {
		return nil, "", fmt.Errorf("failed to parse YAML frontmatter: %w", err)
	}

	return &meta, body, nil
}

// ParseSkillMD parses the YAML frontmatter and markdown body from SKILL.md.
func ParseSkillMD(filePath string) (*model.SkillMetadata, string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read %s: %w", filePath, err)
	}

	meta, body, err := ParseSkillContent(data)
	if err != nil {
		return nil, "", fmt.Errorf("in %s: %w", filePath, err)
	}

	meta.Path = filePath
	return meta, body, nil
}

// ValidateSkillDir validates that a directory is a valid skill directory according to the specification.
func ValidateSkillDir(dirPath string, expectedID string) (*model.SkillMetadata, error) {
	info, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("skill directory not found: %s", dirPath)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("skill path is not a directory: %s", dirPath)
	}

	// 1. Check for symlinks and special files
	if err := fsx.ValidateNoSymlinks(dirPath); err != nil {
		return nil, err
	}

	// 2. Check SKILL.md exists
	skillMDPath := filepath.Join(dirPath, "SKILL.md")
	if !fsx.FileExists(skillMDPath) {
		return nil, fmt.Errorf("SKILL.md not found in %s", dirPath)
	}

	// 3. Parse and validate frontmatter
	meta, _, err := ParseSkillMD(skillMDPath)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(meta.Name) == "" {
		return nil, fmt.Errorf("SKILL.md in %s is missing required 'name' field", dirPath)
	}

	if strings.TrimSpace(meta.Description) == "" {
		return nil, fmt.Errorf("SKILL.md in %s is missing required 'description' field", dirPath)
	}

	if err := ValidateID(meta.Name); err != nil {
		return nil, fmt.Errorf("invalid skill name in SKILL.md: %w", err)
	}

	// 4. Verify name matches directory name
	dirName := filepath.Base(dirPath)
	if meta.Name != dirName {
		return nil, fmt.Errorf("skill name in SKILL.md (%q) does not match directory name (%q)", meta.Name, dirName)
	}

	// 5. If an expected ID was provided, verify it matches
	if expectedID != "" && meta.Name != expectedID {
		return nil, fmt.Errorf("skill name %q does not match expected ID %q", meta.Name, expectedID)
	}

	return meta, nil
}

// GenerateSkillMD generates a template SKILL.md content.
func GenerateSkillMD(name, description string) []byte {
	if description == "" {
		description = "Skill description for " + name
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.WriteString(fmt.Sprintf("name: %s\n", name))
	buf.WriteString(fmt.Sprintf("description: %s\n", description))
	buf.WriteString("---\n\n")
	buf.WriteString(fmt.Sprintf("# %s\n\n", name))
	buf.WriteString("TODO: Document skill usage, prompts, and instructions here.\n")
	return buf.Bytes()
}

// RewriteSkillMDName reads a SKILL.md file, replaces the 'name' field in its YAML frontmatter with newName,
// and preserves other frontmatter fields and the markdown body.
func RewriteSkillMDName(filePath, newName string) error {
	if err := ValidateID(newName); err != nil {
		return fmt.Errorf("invalid skill name: %w", err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", filePath, err)
	}

	content := string(data)
	if !strings.HasPrefix(content, "---") {
		return fmt.Errorf("missing YAML frontmatter in %s", filePath)
	}

	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return fmt.Errorf("invalid frontmatter delimiters in %s", filePath)
	}

	var rawMap map[string]interface{}
	if err := yaml.Unmarshal([]byte(parts[1]), &rawMap); err != nil {
		return fmt.Errorf("failed to parse YAML frontmatter: %w", err)
	}
	if rawMap == nil {
		rawMap = make(map[string]interface{})
	}
	rawMap["name"] = newName

	newFrontmatter, err := yaml.Marshal(rawMap)
	if err != nil {
		return fmt.Errorf("failed to marshal updated YAML frontmatter: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(newFrontmatter)
	buf.WriteString("---\n\n")
	buf.WriteString(strings.TrimSpace(parts[2]))
	buf.WriteString("\n")

	info, err := os.Stat(filePath)
	perm := os.FileMode(0644)
	if err == nil {
		perm = info.Mode()
	}

	return fsx.AtomicWriteFile(filePath, buf.Bytes(), perm)
}
