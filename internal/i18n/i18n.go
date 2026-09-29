package i18n

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// Lang represents supported languages.
type Lang string

const (
	LangEn   Lang = "en"
	LangZhCN Lang = "zh-CN"
)

var (
	mu          sync.RWMutex
	currentLang = LangEn
)

// Init initializes the active language according to configuration or system environment.
// Resolution order:
// 1. If cfgLang is explicitly "zh-CN", "zh", etc., use LangZhCN.
// 2. If cfgLang is "en", "english", etc., use LangEn.
// 3. If cfgLang is "auto" or empty, detect from LC_ALL, LC_MESSAGES, or LANG.
// 4. Fallback to LangEn.
func Init(cfgLang string) {
	mu.Lock()
	defer mu.Unlock()

	norm := strings.ToLower(strings.TrimSpace(cfgLang))
	switch norm {
	case "zh", "zh-cn", "zh_cn", "chinese", "cn":
		currentLang = LangZhCN
		return
	case "en", "en-us", "en_us", "english":
		currentLang = LangEn
		return
	}

	// Auto-detect from environment
	for _, envKey := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		v := strings.ToLower(os.Getenv(envKey))
		if strings.HasPrefix(v, "zh") || strings.Contains(v, "zh_cn") || strings.Contains(v, "zh-cn") {
			currentLang = LangZhCN
			return
		}
	}

	currentLang = LangEn
}

// SetLang explicitly sets the current language.
func SetLang(l Lang) {
	mu.Lock()
	defer mu.Unlock()
	currentLang = l
}

// Current returns the current language.
func Current() Lang {
	mu.RLock()
	defer mu.RUnlock()
	return currentLang
}

// T returns the translated string for key, formatting it with args if provided.
// If the key is not found in the current language dictionary, it falls back to English.
// If still not found, it returns the key itself.
func T(key string, args ...any) string {
	mu.RLock()
	lang := currentLang
	mu.RUnlock()

	var dict map[string]string
	if lang == LangZhCN {
		dict = zhCNDict
	} else {
		dict = enDict
	}

	val, ok := dict[key]
	if !ok {
		// Fallback to English dictionary
		val, ok = enDict[key]
		if !ok {
			val = key
		}
	}

	if len(args) > 0 {
		return fmt.Sprintf(val, args...)
	}
	return val
}
