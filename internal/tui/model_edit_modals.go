package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"asoul/internal/app"
	"asoul/internal/i18n"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// ── Provider detail ────────────────────────────────────────────────────────

// ProviderDetailModalState holds state for viewing provider details.
type ProviderDetailModalState struct {
	Provider app.AgentProviderItem
	Viewport viewport.Model
	Ready    bool
}

func newProviderDetailModal(p app.AgentProviderItem) ProviderDetailModalState {
	vp := viewport.New(70, 10)
	vp.GotoTop()
	return ProviderDetailModalState{Provider: p, Viewport: vp, Ready: true}
}

// RenderProviderDetailModal renders provider key information.
func RenderProviderDetailModal(state *ProviderDetailModalState, box lipgloss.Style, termWidth, termHeight int) string {
	maxOuterWidth := 76
	targetOuterWidth := maxOuterWidth
	if termWidth > 0 && termWidth-4 < targetOuterWidth {
		targetOuterWidth = termWidth - 4
	}
	if targetOuterWidth < 40 {
		if termWidth > 0 && termWidth < 40 {
			targetOuterWidth = termWidth
		} else {
			targetOuterWidth = 40
		}
	}
	contentWidth := targetOuterWidth - box.GetHorizontalFrameSize()
	if contentWidth < 20 {
		contentWidth = 20
	}
	box = box.Width(contentWidth + box.GetHorizontalPadding())

	if termHeight <= 0 {
		termHeight = 24
	}
	maxOuterHeight := termHeight - 3
	if maxOuterHeight < 8 {
		maxOuterHeight = 8
	}
	maxInnerHeight := maxOuterHeight - box.GetVerticalFrameSize()
	if maxInnerHeight < 4 {
		maxInnerHeight = 4
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A29BFE"))
	title := "📦 " + i18n.T("modal.provider_detail.title")
	if state.Provider.ProviderID != "" {
		title = fmt.Sprintf("%s (%s)", title, state.Provider.ProviderID)
	}
	headerStr := titleStyle.Render(truncateToWidth(title, contentWidth)) + "\n\n"
	headerLines := lipgloss.Height(headerStr)

	bodyContent := buildProviderDetailBody(state.Provider, contentWidth)
	bodyHeight := lipgloss.Height(bodyContent)

	footerText := i18n.T("modal.provider_detail.footer")
	footerRendered := renderDetailFooter(footerText, contentWidth)
	footerLines := lipgloss.Height(footerRendered) + 1

	vpH := maxInnerHeight - headerLines - footerLines
	if vpH < 3 {
		vpH = 3
	}
	if bodyHeight < vpH {
		vpH = bodyHeight
	}

	if !state.Ready || state.Viewport.Width != contentWidth || state.Viewport.Height != vpH {
		offset := state.Viewport.YOffset
		state.Viewport.Width = contentWidth
		state.Viewport.Height = vpH
		state.Viewport.SetContent(bodyContent)
		state.Viewport.YOffset = offset
		state.Ready = true
	} else {
		state.Viewport.SetContent(bodyContent)
	}

	var b strings.Builder
	b.WriteString(headerStr)
	b.WriteString(state.Viewport.View())
	b.WriteString("\n\n" + footerRendered)
	return box.Render(b.String())
}

func buildProviderDetailBody(p app.AgentProviderItem, contentWidth int) string {
	var b strings.Builder
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#55EFC4"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#B2BEC3"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)

	b.WriteString(sectionStyle.Render(i18n.T("modal.provider_detail.sec_basic")) + "\n")
	renderDetailField(&b, i18n.T("modal.provider_detail.id"), p.ProviderID, labelStyle, valueStyle, contentWidth)
	if p.Name != "" {
		renderDetailField(&b, i18n.T("modal.provider_detail.name"), p.Name, labelStyle, valueStyle, contentWidth)
	}
	if p.NPM != "" {
		renderDetailField(&b, i18n.T("modal.provider_detail.npm"), p.NPM, labelStyle, valueStyle, contentWidth)
	}
	renderDetailField(&b, i18n.T("modal.provider_detail.model_count"), strconv.Itoa(p.ModelCount), labelStyle, valueStyle, contentWidth)
	b.WriteString("\n")

	if len(p.Options) > 0 {
		b.WriteString(sectionStyle.Render(i18n.T("modal.provider_detail.sec_options")) + "\n")
		for _, k := range sortedMapKeys(p.Options) {
			renderDetailField(&b, k, formatFieldValue(p.Options[k], k), labelStyle, valueStyle, contentWidth)
		}
		b.WriteString("\n")
	}

	if p.RawJSON != "" {
		b.WriteString(sectionStyle.Render(i18n.T("modal.provider_detail.sec_raw_json")) + "\n")
		jsonStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#DFE6E9"))
		for _, line := range strings.Split(p.RawJSON, "\n") {
			b.WriteString("  " + jsonStyle.Render(line) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// ── Field edit ─────────────────────────────────────────────────────────────

// EditableField describes a single editable config field.
type EditableField struct {
	Label string
	Path  []string
	Kind  string // string | number | bool | json
	Value string
}

// FieldEditModalState manages editing a set of config fields.
type FieldEditModalState struct {
	Title   string
	Channel string
	CfgFile string
	Fields  []EditableField
	Cursor  int
	Editing bool
	Input   textinput.Model
	Err     string
}

func newFieldEditModal(title, channel, cfgFile string, fields []EditableField) FieldEditModalState {
	input := textinput.New()
	input.CharLimit = 512
	input.Width = 60
	return FieldEditModalState{
		Title:   title,
		Channel: channel,
		CfgFile: cfgFile,
		Fields:  fields,
		Input:   input,
	}
}

func (s *FieldEditModalState) selected() (EditableField, bool) {
	if len(s.Fields) == 0 || s.Cursor < 0 || s.Cursor >= len(s.Fields) {
		return EditableField{}, false
	}
	return s.Fields[s.Cursor], true
}

// RenderFieldEditModal renders the field list editor.
func RenderFieldEditModal(state *FieldEditModalState, box lipgloss.Style) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#74B9FF"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#B2BEC3"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	selStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#0984E3")).Bold(true)

	var b strings.Builder
	b.WriteString(titleStyle.Render(state.Title) + "\n\n")

	for i, f := range state.Fields {
		shown := f
		if f.Kind == "json" {
			shown.Value = i18n.T("modal.edit_field.json_value")
		} else if isSecretKey(f.Label) {
			shown.Value = maskSecret(f.Value)
		}
		value := shown.Value
		if f.Kind == "bool" {
			if value == "true" {
				value = "✓ true"
			} else {
				value = "✗ false"
			}
		}
		if value == "" {
			value = lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.edit_field.empty"))
		}
		var line string
		if i == state.Cursor {
			line = selStyle.Render(fmt.Sprintf("▶ %-20s %s", f.Label, plainValue(shown)))
		} else {
			line = "  " + fmt.Sprintf("%-22s %s", labelStyle.Render(f.Label), valueStyle.Render(value))
		}
		b.WriteString(line + "\n")
	}

	b.WriteString("\n")
	if state.Editing {
		f, _ := state.selected()
		b.WriteString(labelStyle.Render(fmt.Sprintf(i18n.T("modal.edit_field.editing"), f.Label)) + "\n")
		b.WriteString(state.Input.View() + "\n\n")
		if state.Err != "" {
			b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D63031")).Render("❌ "+state.Err) + "\n\n")
		}
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.edit_field.edit_hint")))
	} else {
		hint := i18n.T("modal.edit_field.hint")
		if f, ok := state.selected(); ok && f.Kind == "bool" {
			hint = i18n.T("modal.edit_field.hint_bool")
		}
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(hint))
	}
	return box.Render(b.String())
}

func plainValue(f EditableField) string {
	if f.Kind == "bool" {
		if f.Value == "true" {
			return "✓ true"
		}
		return "✗ false"
	}
	if f.Value == "" {
		return "—"
	}
	return f.Value
}

// ── Raw JSON edit ──────────────────────────────────────────────────────────

// JSONEditModalState manages raw JSON editing of a config object.
type JSONEditModalState struct {
	Title   string
	Channel string
	CfgFile string
	Path    []string
	Input   textarea.Model
	Err     string
}

func newJSONEditModal(title, channel, cfgFile string, path []string, value any) JSONEditModalState {
	ta := textarea.New()
	ta.CharLimit = 0
	ta.ShowLineNumbers = true
	ta.SetWidth(70)
	ta.SetHeight(14)
	if b, err := json.MarshalIndent(value, "", "  "); err == nil {
		ta.SetValue(string(b))
	}
	ta.Focus()
	return JSONEditModalState{
		Title:   title,
		Channel: channel,
		CfgFile: cfgFile,
		Path:    path,
		Input:   ta,
	}
}

// RenderJSONEditModal renders the raw JSON textarea editor.
func RenderJSONEditModal(state *JSONEditModalState, box lipgloss.Style, termWidth, termHeight int) string {
	width := 78
	if termWidth > 0 && termWidth-6 < width {
		width = termWidth - 6
	}
	if width < 30 {
		width = 30
	}
	height := 16
	if termHeight > 0 && termHeight-8 < height {
		height = termHeight - 8
	}
	if height < 6 {
		height = 6
	}
	state.Input.SetWidth(width - 4)
	state.Input.SetHeight(height)

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#74B9FF")).Render(state.Title) + "\n\n")
	b.WriteString(state.Input.View() + "\n")
	if state.Err != "" {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D63031")).Render("❌ "+state.Err) + "\n")
	}
	b.WriteString("\n" + lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.edit_json.hint")))
	return box.Render(b.String())
}

// ── Field builders ─────────────────────────────────────────────────────────

func joinPath(base []string, keys ...string) []string {
	out := make([]string, 0, len(base)+len(keys))
	out = append(out, base...)
	out = append(out, keys...)
	return out
}

func fieldKind(v any) string {
	switch v.(type) {
	case bool:
		return "bool"
	case string:
		return "string"
	case float64, float32, int, int64:
		return "number"
	default:
		return "json"
	}
}

func formatFieldValue(v any, key string) string {
	if v == nil {
		return ""
	}
	if isSecretKey(key) {
		if s, ok := v.(string); ok {
			return maskSecret(s)
		}
	}
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return formatNumber(t)
	case float32:
		return formatNumber(float64(t))
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	default:
		if b, err := json.Marshal(t); err == nil {
			return string(b)
		}
		return fmt.Sprintf("%v", t)
	}
}

func formatFieldInput(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return formatNumber(t)
	case float32:
		return formatNumber(float64(t))
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case nil:
		return ""
	default:
		if b, err := json.MarshalIndent(t, "", "  "); err == nil {
			return string(b)
		}
		return fmt.Sprintf("%v", t)
	}
}

func formatNumber(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func isSecretKey(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "apikey") || strings.Contains(lower, "api_key") || strings.Contains(lower, "token")
}

func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "…" + s[len(s)-2:]
}

func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// providerFields builds the editable field list for a provider.
func providerFields(p app.AgentProviderItem) []EditableField {
	base := []string{"provider", p.ProviderID}
	fields := []EditableField{
		{Label: "name", Path: joinPath(base, "name"), Kind: "string", Value: p.Name},
		{Label: "npm", Path: joinPath(base, "npm"), Kind: "string", Value: p.NPM},
	}

	optionKeys := make([]string, 0, len(p.Options)+2)
	for _, k := range []string{"baseURL", "apiKey"} {
		if _, ok := p.Options[k]; !ok {
			optionKeys = append(optionKeys, k)
		}
	}
	optionKeys = append(optionKeys, sortedMapKeys(p.Options)...)
	for _, k := range optionKeys {
		v := p.Options[k]
		fields = append(fields, EditableField{
			Label: "options." + k,
			Path:  joinPath(base, "options", k),
			Kind:  fieldKind(v),
			Value: formatFieldInput(v),
		})
	}

	fields = append(fields, EditableField{
		Label: i18n.T("modal.edit_field.raw_json"),
		Path:  base,
		Kind:  "json",
		Value: p.RawJSON,
	})
	return fields
}

// modelFields builds the editable field list for a model.
func modelFields(it app.AgentModelItem) []EditableField {
	base := []string{"provider", it.ProviderID, "models", it.ModelID}
	fields := []EditableField{
		{Label: "name", Path: joinPath(base, "name"), Kind: "string", Value: it.Name},
	}
	if it.ContextLimit > 0 {
		fields = append(fields, EditableField{Label: "limit.context", Path: joinPath(base, "limit", "context"), Kind: "number", Value: strconv.Itoa(it.ContextLimit)})
	} else {
		fields = append(fields, EditableField{Label: "limit.context", Path: joinPath(base, "limit", "context"), Kind: "number"})
	}
	if it.InputLimit > 0 {
		fields = append(fields, EditableField{Label: "limit.input", Path: joinPath(base, "limit", "input"), Kind: "number", Value: strconv.Itoa(it.InputLimit)})
	}
	if it.OutputLimit > 0 {
		fields = append(fields, EditableField{Label: "limit.output", Path: joinPath(base, "limit", "output"), Kind: "number", Value: strconv.Itoa(it.OutputLimit)})
	}
	if it.PriceInput > 0 {
		fields = append(fields, EditableField{Label: "cost.input", Path: joinPath(base, "cost", "input"), Kind: "number", Value: formatNumber(it.PriceInput)})
	}
	if it.PriceOutput > 0 {
		fields = append(fields, EditableField{Label: "cost.output", Path: joinPath(base, "cost", "output"), Kind: "number", Value: formatNumber(it.PriceOutput)})
	}
	if it.PriceCacheRead > 0 {
		fields = append(fields, EditableField{Label: "cost.cache_read", Path: joinPath(base, "cost", "cache_read"), Kind: "number", Value: formatNumber(it.PriceCacheRead)})
	}
	if it.PriceCacheWrite > 0 {
		fields = append(fields, EditableField{Label: "cost.cache_write", Path: joinPath(base, "cost", "cache_write"), Kind: "number", Value: formatNumber(it.PriceCacheWrite)})
	}
	fields = append(fields, EditableField{
		Label: i18n.T("modal.edit_field.raw_json"),
		Path:  base,
		Kind:  "json",
		Value: it.RawJSON,
	})
	return fields
}

func truncateToWidth(s string, width int) string {
	if width <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}
