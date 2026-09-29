package i18n

import "testing"

// TestDictionariesHaveMatchingKeys guards against a key being added to one
// language dictionary but forgotten in the other, which silently leaks the raw
// key into the UI (i18n.T returns the key when it is missing everywhere).
func TestDictionariesHaveMatchingKeys(t *testing.T) {
	for k := range enDict {
		if _, ok := zhCNDict[k]; !ok {
			t.Errorf("zh-CN dictionary missing key %q", k)
		}
	}
	for k := range zhCNDict {
		if _, ok := enDict[k]; !ok {
			t.Errorf("en dictionary missing key %q", k)
		}
	}
}

// TestDoctorKeysLocalized ensures the health-diagnostics surface has keys in
// both languages and that they are actually translated.
func TestDoctorKeysLocalized(t *testing.T) {
	keys := []string{
		"empty.doctor",
		"table.header.st",
		"table.header.check_item",
		"table.header.diagnosis_msg",
		"doctor.check.git",
		"doctor.check.workspace",
		"doctor.check.catalog",
		"doctor.check.skills_dir",
		"doctor.check.leftovers",
		"doctor.check.consistency",
		"doctor.check.cache",
		"doctor.check.cached_project",
		"doctor.check.project_named",
		"doctor.check.projects",
		"doctor.check.deployments",
		"doctor.msg.no_workspace",
		"doctor.msg.projects_none",
		"doctor.msg.consistency_summary",
		"doctor.cli.title",
		"doctor.cli.error",
	}
	for _, k := range keys {
		if enDict[k] == "" {
			t.Errorf("en dictionary missing or empty doctor key %q", k)
		}
		if zhCNDict[k] == "" {
			t.Errorf("zh-CN dictionary missing or empty doctor key %q", k)
		}
	}
	for _, k := range []string{"doctor.check.git", "doctor.check.workspace", "table.header.check_item"} {
		if zhCNDict[k] == enDict[k] {
			t.Errorf("key %q is not translated in zh-CN", k)
		}
	}
}
