package modelrules

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"asoul/internal/agentmodel"
	"asoul/internal/fsx"
	"asoul/internal/jsonc"
)

// RuleFile represents a channel-specific rule configuration file.
// The channel is selected by the file name (<channel>.json|jsonc) or by an
// explicit --rules path; it is not declared inside the file.
type RuleFile struct {
	Schema string `json:"$schema,omitempty"`
	Rules  []Rule `json:"rules"`
}

// Rule defines a pattern and a set of transformations to inject for matching models.
//
// The four operations are independent and may all be configured on the same
// rule. They run in the order given by Order, or in the default order
// fill -> merge -> override -> remove when Order is omitted.
type Rule struct {
	Name        string         `json:"name"`
	Pattern     string         `json:"pattern"` // Regex pattern matching model name
	Description string         `json:"description,omitempty"`
	Order       []string       `json:"order,omitempty"`
	Fill        map[string]any `json:"fill,omitempty"`     // set only missing keys
	Merge       map[string]any `json:"merge,omitempty"`    // recursive merge, arrays unioned
	Override    map[string]any `json:"override,omitempty"` // recursive merge, values win
	Remove      []string       `json:"remove,omitempty"`   // dot-separated paths to delete
}

// Context provides runtime values for variable injection.
type Context struct {
	ModelID    string
	ProviderID string
	ConfigDir  string
	Workspace  string
	Now        time.Time
}

var (
	validRuleOps     = map[string]bool{"fill": true, "merge": true, "override": true, "remove": true}
	defaultRuleOrder = []string{"fill", "merge", "override", "remove"}
	templatePattern  = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)
)

// LoadRuleFile reads and parses a rule file using jsonc to support comments.
// Unknown fields are rejected so that stale or misspelled keys fail loudly.
func LoadRuleFile(path string) (*RuleFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read rule file: %w", err)
	}

	clean := jsonc.StripCommentsAndTrailingCommas(data)
	dec := json.NewDecoder(bytes.NewReader(clean))
	dec.DisallowUnknownFields()

	var rf RuleFile
	if err := dec.Decode(&rf); err != nil {
		return nil, fmt.Errorf("failed to parse rule file %s: %w", path, err)
	}

	return &rf, nil
}

// FindRuleFile searches for an agent rule file in workspace, user config, or explicit path.
func FindRuleFile(agent string, explicitPath string, workspaceRoot string) (string, error) {
	if explicitPath != "" {
		expanded, err := fsx.ExpandUser(explicitPath)
		if err != nil {
			return "", err
		}
		if !fsx.FileExists(expanded) {
			return "", fmt.Errorf("specified rule file not found: %s", explicitPath)
		}
		return expanded, nil
	}

	// Check workspace level: <workspace>/rules/<agent>.json or <agent>.jsonc
	if workspaceRoot != "" {
		wsJSON := filepath.Join(workspaceRoot, "rules", agent+".json")
		if fsx.FileExists(wsJSON) {
			return wsJSON, nil
		}
		wsJSONC := filepath.Join(workspaceRoot, "rules", agent+".jsonc")
		if fsx.FileExists(wsJSONC) {
			return wsJSONC, nil
		}
	}

	// Check user config level: ~/.config/asoul/rules/<agent>.json or .jsonc
	userConfigDir, err := os.UserConfigDir()
	if err == nil {
		cfgJSON := filepath.Join(userConfigDir, "asoul", "rules", agent+".json")
		if fsx.FileExists(cfgJSON) {
			return cfgJSON, nil
		}
		cfgJSONC := filepath.Join(userConfigDir, "asoul", "rules", agent+".jsonc")
		if fsx.FileExists(cfgJSONC) {
			return cfgJSONC, nil
		}
	}

	return "", nil
}

// ApplyRules applies matching rules to a model definition.
func ApplyRules(modelName string, rules []Rule, base map[string]any, ctx Context) (map[string]any, []string, error) {
	result := base
	var matchedRules []string

	if ctx.ModelID == "" {
		ctx.ModelID = modelName
	}

	for _, rule := range rules {
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			continue
		}
		if !re.MatchString(modelName) {
			continue
		}

		order, err := resolveOrder(rule)
		if err != nil {
			return nil, nil, err
		}
		vars := buildVars(ctx, re, modelName)

		for _, op := range order {
			switch op {
			case "fill":
				result = agentmodel.FillMissing(result, renderMap(rule.Fill, vars))
			case "merge":
				result = agentmodel.MergeUnion(result, renderMap(rule.Merge, vars))
			case "override":
				result = agentmodel.DeepMerge(result, renderMap(rule.Override, vars))
			case "remove":
				for _, path := range rule.Remove {
					path = renderString(path, vars)
					agentmodel.RemovePath(result, path)
				}
			}
		}
		matchedRules = append(matchedRules, rule.Name)
	}

	return result, matchedRules, nil
}

func resolveOrder(rule Rule) ([]string, error) {
	present := map[string]bool{}
	if len(rule.Fill) > 0 {
		present["fill"] = true
	}
	if len(rule.Merge) > 0 {
		present["merge"] = true
	}
	if len(rule.Override) > 0 {
		present["override"] = true
	}
	if len(rule.Remove) > 0 {
		present["remove"] = true
	}

	if len(rule.Order) == 0 {
		var order []string
		for _, op := range defaultRuleOrder {
			if present[op] {
				order = append(order, op)
			}
		}
		return order, nil
	}

	seen := map[string]bool{}
	for _, op := range rule.Order {
		if !validRuleOps[op] {
			return nil, fmt.Errorf("rule %q: unknown operation %q in order (valid: fill, merge, override, remove)", rule.Name, op)
		}
		if seen[op] {
			return nil, fmt.Errorf("rule %q: duplicate operation %q in order", rule.Name, op)
		}
		seen[op] = true
	}
	for op := range present {
		if !seen[op] {
			return nil, fmt.Errorf("rule %q: operation %q is configured but missing from order", rule.Name, op)
		}
	}
	return rule.Order, nil
}

func buildVars(ctx Context, re *regexp.Regexp, modelName string) map[string]string {
	now := ctx.Now
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()

	vars := map[string]string{
		"model_id":    ctx.ModelID,
		"provider_id": ctx.ProviderID,
		"config_dir":  ctx.ConfigDir,
		"workspace":   ctx.Workspace,
		"date":        now.Format("2006-01-02"),
		"time":        now.Format("15:04:05"),
		"datetime":    now.Format(time.RFC3339),
		"timestamp":   strconv.FormatInt(now.Unix(), 10),
		"year":        now.Format("2006"),
		"month":       now.Format("01"),
		"day":         now.Format("02"),
	}
	if vars["model_id"] == "" {
		vars["model_id"] = modelName
	}

	matches := re.FindStringSubmatch(modelName)
	names := re.SubexpNames()
	for i, sub := range matches {
		if i == 0 {
			continue
		}
		vars[strconv.Itoa(i)] = sub
		if i < len(names) && names[i] != "" {
			vars[names[i]] = sub
		}
	}
	return vars
}

func renderMap(m map[string]any, vars map[string]string) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = renderValue(v, vars)
	}
	return out
}

func renderValue(v any, vars map[string]string) any {
	switch t := v.(type) {
	case string:
		return renderString(t, vars)
	case map[string]any:
		return renderMap(t, vars)
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = renderValue(item, vars)
		}
		return out
	default:
		return v
	}
}

func renderString(s string, vars map[string]string) string {
	if !strings.Contains(s, "${") {
		return s
	}
	return templatePattern.ReplaceAllStringFunc(s, func(token string) string {
		key := token[2 : len(token)-1]
		if val, ok := vars[key]; ok {
			return val
		}
		return token
	})
}
