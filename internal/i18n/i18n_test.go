package i18n_test

import (
	"os"
	"testing"

	"asoul/internal/i18n"
)

func TestI18nInitAndSwitch(t *testing.T) {
	// Test English init
	i18n.Init("en")
	if i18n.Current() != i18n.LangEn {
		t.Fatalf("expected LangEn, got %s", i18n.Current())
	}
	if got := i18n.T("app.title"); got != "asoul - Agent Skills Local Hub" {
		t.Fatalf("expected English title, got %q", got)
	}

	// Test Chinese init
	i18n.Init("zh-CN")
	if i18n.Current() != i18n.LangZhCN {
		t.Fatalf("expected LangZhCN, got %s", i18n.Current())
	}
	if got := i18n.T("app.title"); got != "asoul - Agent Skills 本地管理中心" {
		t.Fatalf("expected Chinese title, got %q", got)
	}

	// Test format arguments
	formatted := i18n.T("more.skills.above", 5)
	if formatted != "  ▲ (上方还有 5 个技能...)" {
		t.Fatalf("unexpected formatted string: %q", formatted)
	}

	// Test fallback to English when key missing in zh-CN
	// Let's test SetLang
	i18n.SetLang(i18n.LangEn)
	formattedEn := i18n.T("more.skills.above", 3)
	if formattedEn != "  ▲ (3 more skills above...)" {
		t.Fatalf("unexpected formatted English string: %q", formattedEn)
	}

	// Test completely unknown key
	unknown := i18n.T("non.existent.key")
	if unknown != "non.existent.key" {
		t.Fatalf("expected key returned for unknown key, got %q", unknown)
	}

	// Test auto-detect with LANG=zh_CN.UTF-8
	_ = os.Setenv("LANG", "zh_CN.UTF-8")
	i18n.Init("auto")
	if i18n.Current() != i18n.LangZhCN {
		t.Fatalf("expected auto to detect LangZhCN with LANG=zh_CN.UTF-8, got %s", i18n.Current())
	}

	// Test auto-detect with LANG=en_US.UTF-8
	_ = os.Setenv("LANG", "en_US.UTF-8")
	_ = os.Unsetenv("LC_ALL")
	_ = os.Unsetenv("LC_MESSAGES")
	i18n.Init("")
	if i18n.Current() != i18n.LangEn {
		t.Fatalf("expected empty/auto to detect LangEn with LANG=en_US.UTF-8, got %s", i18n.Current())
	}
}
