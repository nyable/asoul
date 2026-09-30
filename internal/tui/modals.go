package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"asoul/internal/app"
	"asoul/internal/config"
	"asoul/internal/deploy"
	"asoul/internal/fsx"
	"asoul/internal/i18n"
	"asoul/internal/model"
	"asoul/internal/source/git"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/cellbuf"
	"github.com/muesli/reflow/truncate"
)

type DiscoveredTag int

const (
	DiscoveredTagNew DiscoveredTag = iota
	DiscoveredTagInstalled
	DiscoveredTagConflict
)

type AddModalFilter int

const (
	AddFilterAll AddModalFilter = iota
	AddFilterConflict
	AddFilterNew
	AddFilterInstalled
)

type DiscoveredItemStatus struct {
	Tag            DiscoveredTag
	ConflictSource string
}

// AddModalState manages the multi-step add skill workflow in TUI.
type AddModalState struct {
	Step             int // 0: input source, 1: select discovered skills
	SourceURL        string
	SourceRef        string
	SourceInput      textinput.Model
	PathInput        textinput.Model
	ActiveInput      int // 0: source, 1: path
	DiscoveredSkills []git.DiscoveredSkill
	SelectedIndices  map[int]bool
	DiscoveredCursor int
	ScrollOffset     int

	// Conflict resolution & rename support
	DiscoveredStatus map[int]DiscoveredItemStatus
	CustomAliases    map[string]string // subpath -> custom ID
	OverwriteSkills  map[string]bool   // subpath -> true (per-item overwrite)
	ReplaceSource    bool              // fallback/global if needed
	Force            bool              // toggle via [f]

	// Filtering
	CurrentFilter AddModalFilter
	SearchInput   textinput.Model
	Searching     bool

	// Upstream selection in Step 0
	FromUpstreamMode   bool
	UpstreamListCursor int
	Upstreams          []model.UpstreamInfo

	// Inline rename mode
	Renaming    bool
	RenameInput textinput.Model
}

// Counts returns total, conflicts, new, and installed counts in DiscoveredSkills.
func (s *AddModalState) Counts() (total, conflicts, news, installed int) {
	total = len(s.DiscoveredSkills)
	for i := range s.DiscoveredSkills {
		if st, ok := s.DiscoveredStatus[i]; ok {
			switch st.Tag {
			case DiscoveredTagConflict:
				conflicts++
			case DiscoveredTagNew:
				news++
			case DiscoveredTagInstalled:
				installed++
			}
		} else {
			news++
		}
	}
	return
}

// VisibleIndices returns the list of original indices matching CurrentFilter and SearchInput.
func (s *AddModalState) VisibleIndices() []int {
	var indices []int
	query := strings.TrimSpace(s.SearchInput.Value())
	lowerQuery := strings.ToLower(query)
	var re *regexp.Regexp
	if query != "" && strings.ContainsAny(query, ".*+?^$[]{}|()\\") {
		re, _ = regexp.Compile("(?i)" + query)
	}

	for i, sk := range s.DiscoveredSkills {
		tag := DiscoveredTagNew
		if st, ok := s.DiscoveredStatus[i]; ok {
			tag = st.Tag
		}

		matchesCategory := false
		switch s.CurrentFilter {
		case AddFilterConflict:
			matchesCategory = (tag == DiscoveredTagConflict)
		case AddFilterNew:
			matchesCategory = (tag == DiscoveredTagNew)
		case AddFilterInstalled:
			matchesCategory = (tag == DiscoveredTagInstalled)
		default: // AddFilterAll
			matchesCategory = true
		}
		if !matchesCategory {
			continue
		}

		if lowerQuery != "" {
			targetID := sk.ID
			if alias, ok := s.CustomAliases[sk.Path]; ok && alias != "" {
				targetID = alias
			}
			matched := false
			if re != nil && (re.MatchString(targetID) || re.MatchString(sk.ID)) {
				matched = true
			}
			if !matched {
				if strings.Contains(strings.ToLower(targetID), lowerQuery) ||
					strings.Contains(strings.ToLower(sk.ID), lowerQuery) ||
					strings.Contains(strings.ToLower(sk.Path), lowerQuery) ||
					strings.Contains(strings.ToLower(sk.Description), lowerQuery) {
					matched = true
				}
			}
			if !matched {
				continue
			}
		}

		indices = append(indices, i)
	}
	return indices
}

func newAddModal(upstreams ...model.UpstreamInfo) AddModalState {
	sInput := textinput.New()
	sInput.Placeholder = i18n.T("modal.add.source_placeholder")
	sInput.Focus()
	sInput.CharLimit = 256
	sInput.Width = 60

	pInput := textinput.New()
	pInput.Placeholder = i18n.T("modal.add.subpath_placeholder")
	pInput.CharLimit = 128
	pInput.Width = 60

	rInput := textinput.New()
	rInput.Placeholder = i18n.T("modal.new.placeholder")
	rInput.CharLimit = 64
	rInput.Width = 35

	srInput := textinput.New()
	srInput.Placeholder = i18n.T("search.placeholder")
	srInput.CharLimit = 64
	srInput.Width = 30

	fromUpstream := len(upstreams) > 0

	return AddModalState{
		Step:               0,
		SourceInput:        sInput,
		PathInput:          pInput,
		ActiveInput:        0,
		FromUpstreamMode:   fromUpstream,
		UpstreamListCursor: 0,
		Upstreams:          upstreams,
		SelectedIndices:    make(map[int]bool),
		DiscoveredStatus:   make(map[int]DiscoveredItemStatus),
		CustomAliases:      make(map[string]string),
		OverwriteSkills:    make(map[string]bool),
		RenameInput:        rInput,
		CurrentFilter:      AddFilterAll,
		SearchInput:        srInput,
		Searching:          false,
	}
}

// TargetModalState manages adding a new target.
type TargetModalState struct {
	NameInput   textinput.Model
	PathInput   textinput.Model
	ActiveInput int // 0: name, 1: path
}

func newTargetModal() TargetModalState {
	nInput := textinput.New()
	nInput.Placeholder = i18n.T("modal.target.name_placeholder")
	nInput.Focus()
	nInput.CharLimit = 64
	nInput.Width = 50

	pInput := textinput.New()
	pInput.Placeholder = i18n.T("modal.target.path_placeholder")
	pInput.CharLimit = 256
	pInput.Width = 50

	return TargetModalState{
		NameInput:   nInput,
		PathInput:   pInput,
		ActiveInput: 0,
	}
}

// ConfirmationModalState manages yes/no/force confirmations.
type ConfirmationModalState struct {
	Title       string
	Message     string
	Danger      bool
	AllowForce  bool
	Action      string // "remove-skill", "remove-skills-batch", "force-update", "force-deploy"
	SkillID     string
	SkillIDs    []string
	Deployments map[string][]string
	Target      string
}

// RenderAddModal renders the add skill modal with loading state, filter tabs, and error messages.
func RenderAddModal(state *AddModalState, box lipgloss.Style, loading bool, progressBlock string, err error, height int) string {
	var b strings.Builder

	if loading {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A29BFE")).Render(i18n.T("modal.add.loading_title")) + "\n\n")
		if progressBlock != "" {
			b.WriteString(progressBlock + "\n\n")
		}
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.add.cancel_hint")))
		return box.Render(b.String())
	}

	if state.Step == 0 {
		if len(state.Upstreams) > 0 {
			tabUpstream := i18n.T("modal.add.tab_upstream")
			tabCustom := i18n.T("modal.add.tab_custom")
			if state.FromUpstreamMode {
				tabUpstream = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#0984E3")).Bold(true).Padding(0, 1).Render(tabUpstream)
				tabCustom = lipgloss.NewStyle().Foreground(lipgloss.Color("#636E72")).Padding(0, 1).Render(tabCustom)
			} else {
				tabUpstream = lipgloss.NewStyle().Foreground(lipgloss.Color("#636E72")).Padding(0, 1).Render(tabUpstream)
				tabCustom = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#0984E3")).Bold(true).Padding(0, 1).Render(tabCustom)
			}
			b.WriteString(tabUpstream + "  " + tabCustom + "\n\n")

			if state.FromUpstreamMode {
				b.WriteString(i18n.T("modal.add.from_upstream_prompt"))
				for i, u := range state.Upstreams {
					cursor := "  "
					if i == state.UpstreamListCursor {
						cursor = "▶ "
					}
					nameStr := u.URL
					if u.Name != "" {
						nameStr = fmt.Sprintf("%s (%s)", u.Name, u.URL)
					}
					item := i18n.T("modal.add.upstream_item", cursor, nameStr, len(u.Skills))
					if i == state.UpstreamListCursor {
						item = lipgloss.NewStyle().Foreground(lipgloss.Color("#55EFC4")).Bold(true).Render(item)
					}
					b.WriteString(item + "\n")
				}
				if err != nil {
					b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D63031")).Render("❌ "+err.Error()) + "\n")
				}
				b.WriteString("\n" + lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.add.from_upstream_hint")))
				return box.Render(b.String())
			}
		}

		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A29BFE")).Render(i18n.T("modal.add.title")) + "\n\n")
		b.WriteString(i18n.T("modal.add.source_label") + "\n")
		b.WriteString(state.SourceInput.View() + "\n\n")
		b.WriteString(i18n.T("modal.add.subpath_label") + "\n")
		b.WriteString(state.PathInput.View() + "\n\n")

		if err != nil {
			b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D63031")).Render("❌ "+err.Error()) + "\n\n")
		}

		hint := i18n.T("modal.add.hint_step0")
		if len(state.Upstreams) > 0 {
			hint = i18n.T("modal.add.mode_prefix") + hint
		}
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(hint))
	} else {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A29BFE")).Render(fmt.Sprintf(i18n.T("modal.add.discovered_title"), len(state.DiscoveredSkills))) + "\n\n")

		// Render Filter Tab Bar
		totalCount, conflictCount, newCount, installedCount := state.Counts()
		tabAll := fmt.Sprintf("%s (%d)", i18n.T("filter.all"), totalCount)
		tabConflict := fmt.Sprintf("%s (%d)", i18n.T("filter.conflict"), conflictCount)
		tabNew := fmt.Sprintf("%s (%d)", i18n.T("filter.new"), newCount)
		tabInstalled := fmt.Sprintf("%s (%d)", i18n.T("filter.installed"), installedCount)

		styleActive := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#6C5CE7")).Padding(0, 1)
		styleInactive := lipgloss.NewStyle().Foreground(lipgloss.Color("#B2BEC3")).Padding(0, 1)

		renderTab := func(filter AddModalFilter, label string, keyNum string) string {
			text := fmt.Sprintf("[%s] %s", keyNum, label)
			if state.CurrentFilter == filter {
				return styleActive.Render(text)
			}
			return styleInactive.Render(text)
		}

		filterBar := fmt.Sprintf("  %s  %s  %s  %s\n\n",
			renderTab(AddFilterAll, tabAll, "1"),
			renderTab(AddFilterConflict, tabConflict, "2"),
			renderTab(AddFilterNew, tabNew, "3"),
			renderTab(AddFilterInstalled, tabInstalled, "4"),
		)
		b.WriteString(filterBar)

		if state.Searching || state.SearchInput.Value() != "" {
			b.WriteString("  🔍 " + state.SearchInput.View() + "\n\n")
		}

		visible := state.VisibleIndices()
		if len(visible) == 0 {
			emptyText := i18n.T("filter.empty")
			if state.SearchInput.Value() != "" {
				emptyText = i18n.T("filter.search_empty")
			}
			b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render(emptyText) + "\n\n")
		} else {
			maxVisible := height - 12
			if maxVisible < 5 {
				maxVisible = 5
			}
			if maxVisible > 12 {
				maxVisible = 12
			}

			if state.DiscoveredCursor >= len(visible) {
				state.DiscoveredCursor = len(visible) - 1
			}
			if state.DiscoveredCursor < 0 {
				state.DiscoveredCursor = 0
			}

			if state.DiscoveredCursor < state.ScrollOffset {
				state.ScrollOffset = state.DiscoveredCursor
			}
			if state.DiscoveredCursor >= state.ScrollOffset+maxVisible {
				state.ScrollOffset = state.DiscoveredCursor - maxVisible + 1
			}
			maxOffset := len(visible) - maxVisible
			if maxOffset < 0 {
				maxOffset = 0
			}
			if state.ScrollOffset > maxOffset {
				state.ScrollOffset = maxOffset
			}
			if state.ScrollOffset < 0 {
				state.ScrollOffset = 0
			}

			start := state.ScrollOffset
			end := start + maxVisible
			if end > len(visible) {
				end = len(visible)
			}

			if start > 0 {
				b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(i18n.T("more.skills.above"), start)) + "\n")
			}

			for vIdx := start; vIdx < end; vIdx++ {
				realIdx := visible[vIdx]
				sk := state.DiscoveredSkills[realIdx]
				cursor := "  "
				if vIdx == state.DiscoveredCursor {
					cursor = "▶ "
				}
				checked := "[ ] "
				if state.SelectedIndices[realIdx] {
					checked = "[✓] "
				}
				desc := sk.Description
				if len(desc) > 30 {
					desc = desc[:27] + "..."
				}

				targetID := sk.ID
				var tagStr string
				if alias, ok := state.CustomAliases[sk.Path]; ok && alias != "" {
					targetID = alias
					tagStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#55EFC4")).Bold(true).Render(fmt.Sprintf("[%s: %s]", i18n.T("tag.renamed"), alias))
				} else if state.OverwriteSkills[sk.Path] {
					tagStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF7675")).Bold(true).Render(i18n.T("tag.overwrite"))
				} else if st, ok := state.DiscoveredStatus[realIdx]; ok {
					switch st.Tag {
					case DiscoveredTagNew:
						tagStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#00B894")).Render(i18n.T("tag.new"))
					case DiscoveredTagInstalled:
						tagStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#636E72")).Render(i18n.T("tag.installed"))
					case DiscoveredTagConflict:
						tagStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FDCB6E")).Bold(true).Render(fmt.Sprintf(i18n.T("tag.conflict"), st.ConflictSource))
					}
				}

				line := fmt.Sprintf("%s%s%-18s %s", cursor, checked, targetID, tagStr)
				if desc != "" {
					line += fmt.Sprintf(" - %s", desc)
				}
				if vIdx == state.DiscoveredCursor {
					line = lipgloss.NewStyle().Foreground(lipgloss.Color("#74B9FF")).Bold(true).Render(line)
				}
				b.WriteString(line + "\n")
			}

			if end < len(visible) {
				b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(i18n.T("more.skills.below"), len(visible)-end)) + "\n")
			}
		}

		if state.Renaming {
			b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FDCB6E")).Render(i18n.T("modal.add.rename_prompt")) + " ")
			b.WriteString(state.RenameInput.View() + "\n")
			b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.add.rename_hint")) + "\n")
		}

		if err != nil {
			b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D63031")).Render("❌ "+err.Error()) + "\n")
		}

		b.WriteString("\n" + lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.add.hint_step1")))
	}

	return box.Render(b.String())
}

// RenderNewModal renders the new skill creation modal.
func RenderNewModal(input textinput.Model, box lipgloss.Style, loading bool, progressBlock string, err error) string {
	var b strings.Builder
	if loading {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#55EFC4")).Render("⏳ "+i18n.T("modal.new.title")) + "\n\n")
		if progressBlock != "" {
			b.WriteString(progressBlock + "\n\n")
		}
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.loading.cancel_hint")))
		return box.Render(b.String())
	}
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#55EFC4")).Render(i18n.T("modal.new.title")) + "\n\n")
	b.WriteString(i18n.T("modal.new.label") + "\n")
	b.WriteString(input.View() + "\n\n")
	if err != nil {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D63031")).Render("❌ "+err.Error()) + "\n\n")
	}
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.new.hint")))
	return box.Render(b.String())
}

// RenderTargetModal renders the add target modal.
func RenderTargetModal(state *TargetModalState, box lipgloss.Style, loading bool, progressBlock string, err error) string {
	var b strings.Builder
	if loading {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#74B9FF")).Render("⏳ "+i18n.T("modal.target.title")) + "\n\n")
		if progressBlock != "" {
			b.WriteString(progressBlock + "\n\n")
		}
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.loading.cancel_hint")))
		return box.Render(b.String())
	}
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#74B9FF")).Render(i18n.T("modal.target.title")) + "\n\n")
	b.WriteString(i18n.T("modal.target.name_label") + "\n")
	b.WriteString(state.NameInput.View() + "\n\n")
	b.WriteString(i18n.T("modal.target.path_label") + "\n")
	b.WriteString(state.PathInput.View() + "\n\n")
	if err != nil {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D63031")).Render("❌ "+err.Error()) + "\n\n")
	}
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.target.hint")))
	return box.Render(b.String())
}

// RenderConfirmationModal renders warning or confirmation dialogs.
func RenderConfirmationModal(state *ConfirmationModalState, box lipgloss.Style, loading bool, progressBlock string) string {
	var b strings.Builder
	if loading {
		title := "⏳ " + i18n.T("modal.loading.title")
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A29BFE")).Render(title) + "\n\n")
		if progressBlock != "" {
			b.WriteString(progressBlock + "\n\n")
		}
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.loading.cancel_hint")))
		return box.Render(b.String())
	}

	titleColor := "#FDCB6E"
	if state.Danger {
		titleColor = "#FF7675"
	}

	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(titleColor)).Render(state.Title) + "\n\n")
	b.WriteString(state.Message + "\n\n")

	if state.Action == "remove-skills-batch" && len(state.SkillIDs) > 0 {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#DFE6E9")).Render(i18n.T("modal.remove_batch.list_title")) + "\n")
		maxShow := 8
		for i, id := range state.SkillIDs {
			if i >= maxShow {
				b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.remove_batch.more_skills", len(state.SkillIDs)-maxShow)) + "\n")
				break
			}
			b.WriteString(fmt.Sprintf("  • %s\n", id))
		}
		b.WriteString("\n")
	}

	if len(state.Deployments) > 0 {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF7675")).Render(i18n.T("confirm.remove_skills_batch.deployed_warning")) + "\n")
		depCount := 0
		for id, targets := range state.Deployments {
			if depCount >= 5 {
				b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.remove_batch.more_deployments", len(state.Deployments))) + "\n")
				break
			}
			b.WriteString(fmt.Sprintf("  • %s ➜ %s\n", id, strings.Join(targets, ", ")))
			depCount++
		}
		b.WriteString("\n")
	}

	actions := hintText(i18n.T("confirm.actions.normal"))
	if state.AllowForce {
		actions = hintText(i18n.T("confirm.actions.force"))
	}
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(actions))

	return box.Render(b.String())
}

// ProjectModalState manages adding a new project directory.
type ProjectModalState struct {
	PathInput textinput.Model
}

func newProjectModal() ProjectModalState {
	pInput := textinput.New()
	pInput.Placeholder = i18n.T("modal.project.placeholder")
	pInput.Focus()
	pInput.CharLimit = 256
	pInput.Width = 60
	return ProjectModalState{PathInput: pInput}
}

func RenderProjectModal(state *ProjectModalState, box lipgloss.Style, loading bool, progressBlock string, err error) string {
	var b strings.Builder
	if loading {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#74B9FF")).Render("⏳ "+i18n.T("modal.project.title")) + "\n\n")
		if progressBlock != "" {
			b.WriteString(progressBlock + "\n\n")
		}
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.loading.cancel_hint")))
		return box.Render(b.String())
	}
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#74B9FF")).Render(i18n.T("modal.project.title")) + "\n\n")
	b.WriteString(i18n.T("modal.project.label") + "\n")
	b.WriteString(state.PathInput.View() + "\n\n")
	if err != nil {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D63031")).Render("❌ "+err.Error()) + "\n\n")
	}
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.project.hint")))
	return box.Render(b.String())
}

// GroupModalState manages adding a new skill group.
type GroupModalState struct {
	NameInput textinput.Model
}

func newGroupModal() GroupModalState {
	nInput := textinput.New()
	nInput.Placeholder = i18n.T("modal.group.name_placeholder")
	nInput.Focus()
	nInput.CharLimit = 64
	nInput.Width = 50
	return GroupModalState{NameInput: nInput}
}

func RenderGroupModal(state *GroupModalState, box lipgloss.Style, loading bool, progressBlock string, err error) string {
	var b strings.Builder
	if loading {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FDCB6E")).Render("⏳ "+i18n.T("modal.group.title")) + "\n\n")
		if progressBlock != "" {
			b.WriteString(progressBlock + "\n\n")
		}
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.loading.cancel_hint")))
		return box.Render(b.String())
	}
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FDCB6E")).Render(i18n.T("modal.group.title")) + "\n\n")
	b.WriteString(i18n.T("modal.group.name_label") + "\n")
	b.WriteString(state.NameInput.View() + "\n\n")
	if err != nil {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D63031")).Render("❌ "+err.Error()) + "\n\n")
	}
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.group.hint")))
	return box.Render(b.String())
}

// GroupSkillsModalState manages assigning skills to a group.
type GroupSkillsModalState struct {
	GroupName      string
	Skills         []model.SkillStatus
	SelectedSkills map[string]bool
	Cursor         int
	ScrollOffset   int
	SearchInput    textinput.Model
	Searching      bool
}

func (s *GroupSkillsModalState) VisibleSkills() []model.SkillStatus {
	query := strings.TrimSpace(s.SearchInput.Value())
	if query == "" {
		return s.Skills
	}
	lowerQuery := strings.ToLower(query)
	var re *regexp.Regexp
	if strings.ContainsAny(query, ".*+?^$[]{}|()\\") {
		re, _ = regexp.Compile("(?i)" + query)
	}
	var res []model.SkillStatus
	for _, sk := range s.Skills {
		matched := false
		if re != nil && re.MatchString(sk.ID) {
			matched = true
		}
		if !matched {
			if strings.Contains(strings.ToLower(sk.ID), lowerQuery) ||
				strings.Contains(strings.ToLower(string(sk.Source.Type)), lowerQuery) ||
				strings.Contains(strings.ToLower(sk.Source.URL), lowerQuery) ||
				strings.Contains(strings.ToLower(sk.Source.Path), lowerQuery) {
				matched = true
			}
		}
		if matched {
			res = append(res, sk)
		}
	}
	return res
}

func newGroupSkillsModal(groupName string, allSkills []model.SkillStatus, currentGroupSkills []string) GroupSkillsModalState {
	selected := make(map[string]bool)
	for _, id := range currentGroupSkills {
		selected[id] = true
	}
	sInput := textinput.New()
	sInput.Placeholder = i18n.T("search.placeholder")
	sInput.CharLimit = 64
	sInput.Width = 30

	return GroupSkillsModalState{
		GroupName:      groupName,
		Skills:         allSkills,
		SelectedSkills: selected,
		Cursor:         0,
		ScrollOffset:   0,
		SearchInput:    sInput,
		Searching:      false,
	}
}

func RenderGroupSkillsModal(state *GroupSkillsModalState, box lipgloss.Style, height int, loading bool, progressBlock string, err error) string {
	var b strings.Builder
	if loading {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FDCB6E")).Render(
			fmt.Sprintf("⏳ "+i18n.T("modal.group_skills.title"), state.GroupName),
		) + "\n\n")
		if progressBlock != "" {
			b.WriteString(progressBlock + "\n\n")
		}
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.loading.cancel_hint")))
		return box.Render(b.String())
	}
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FDCB6E")).Render(
		fmt.Sprintf(i18n.T("modal.group_skills.title"), state.GroupName),
	) + "\n\n")

	if len(state.Skills) == 0 {
		b.WriteString(i18n.T("modal.group_skills.no_skills") + "\n")
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.group_skills.hint")))
		return box.Render(b.String())
	}

	if state.Searching || state.SearchInput.Value() != "" {
		b.WriteString("  🔍 " + state.SearchInput.View() + "\n\n")
	}

	visible := state.VisibleSkills()
	if len(visible) == 0 {
		b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render(i18n.T("filter.search_empty")) + "\n\n")
	} else {
		maxVisible := height - 14
		if maxVisible < 5 {
			maxVisible = 5
		}
		if maxVisible > 14 {
			maxVisible = 14
		}

		if state.Cursor < state.ScrollOffset {
			state.ScrollOffset = state.Cursor
		}
		if state.Cursor >= state.ScrollOffset+maxVisible {
			state.ScrollOffset = state.Cursor - maxVisible + 1
		}
		maxOffset := len(visible) - maxVisible
		if maxOffset < 0 {
			maxOffset = 0
		}
		if state.ScrollOffset > maxOffset {
			state.ScrollOffset = maxOffset
		}
		if state.ScrollOffset < 0 {
			state.ScrollOffset = 0
		}

		start := state.ScrollOffset
		end := start + maxVisible
		if end > len(visible) {
			end = len(visible)
		}

		if start > 0 {
			b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(i18n.T("more.skills.above"), start)) + "\n")
		}

		for i := start; i < end; i++ {
			sk := visible[i]
			cursor := "  "
			if i == state.Cursor {
				cursor = "▶ "
			}
			checked := "[ ] "
			if state.SelectedSkills[sk.ID] {
				checked = "[✓] "
			}
			line := i18n.T("deploy.skill_line", cursor, checked, sk.ID, sk.Source.Type)
			if i == state.Cursor {
				line = lipgloss.NewStyle().Foreground(lipgloss.Color("#74B9FF")).Bold(true).Render(line)
			}
			b.WriteString(line + "\n")
		}

		if end < len(visible) {
			b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(i18n.T("more.skills.below"), len(visible)-end)) + "\n")
		}
	}

	var selCount int
	for _, v := range state.SelectedSkills {
		if v {
			selCount++
		}
	}
	b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#00CEC9")).Render(
		fmt.Sprintf(i18n.T("modal.group_skills.count"), selCount, len(state.Skills)),
	) + "\n")

	if err != nil {
		b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D63031")).Render("❌ "+err.Error()) + "\n")
	}

	b.WriteString("\n" + lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.group_skills.hint")))
	return box.Render(b.String())
}

type DeploySubMode int

const (
	DeploySubModeSkills DeploySubMode = iota // 0: 手动多选技能 (默认)
	DeploySubModeGroup                       // 1: 按分组批量
)

type DeployTargetTab int

const (
	DeployTargetTabAgent   DeployTargetTab = iota // 0: Agent 渠道
	DeployTargetTabProject                        // 1: 项目工程
)

type DeployStep int

const (
	DeployStepSelectSkills DeployStep = iota // 0: 选择技能 (默认直接进入手动多选，可切分组)
	DeployStepSelectTarget                   // 1: 选择分发目标 (支持 Agent 渠道 / 项目工程 Tab 切换)
	DeployStepSelectFormat                   // 2: 选择项目分发格式
)

type SkillGroupItem struct {
	Name        string
	Description string
	SkillIDs    []string
}

type DeployModalState struct {
	IsUndeploy bool
	Step       DeployStep
	SubMode    DeploySubMode // 0: Skills list, 1: Groups list

	// Locked target (when deploy is triggered directly from a specific target)
	LockedTargetName  string
	LockedTargetType  string // "agent" or "project"
	LockedProjectPath string

	// Skill multi-selection
	Skills         []model.SkillStatus
	SkillsCursor   int
	SelectedSkills map[string]bool
	ScrollOffset   int

	// Group selection
	Groups      []SkillGroupItem
	GroupCursor int

	// Target selection
	TargetTab        DeployTargetTab // 0: Agent, 1: Project
	AgentTargets     []string
	AgentTargetPaths map[string]string
	AgentCursor      int
	AgentScroll      int

	ProjectTargets     []string
	ProjectTargetPaths map[string]string
	ProjectCursor      int
	ProjectScroll      int

	// Legacy / Helper map for compatibility
	TargetPaths     map[string]string
	TargetIsProject map[string]string // target name -> project root

	// Format selection (for project deployment)
	FormatIDs       []string
	FormatCursor    int
	SelectedFormats map[string]bool
	SelectedProject string

	// Resolved skill IDs
	ResolvedSkillIDs []string

	// Search filter
	SearchInput textinput.Model
	Searching   bool
}

func (s *DeployModalState) VisibleSkills() []model.SkillStatus {
	query := strings.TrimSpace(s.SearchInput.Value())
	if query == "" {
		return s.Skills
	}
	lowerQuery := strings.ToLower(query)
	var re *regexp.Regexp
	if strings.ContainsAny(query, ".*+?^$[]{}|()\\") {
		re, _ = regexp.Compile("(?i)" + query)
	}
	var res []model.SkillStatus
	for _, sk := range s.Skills {
		matched := false
		if re != nil && re.MatchString(sk.ID) {
			matched = true
		}
		if !matched {
			if strings.Contains(strings.ToLower(sk.ID), lowerQuery) ||
				strings.Contains(strings.ToLower(string(sk.Source.Type)), lowerQuery) ||
				strings.Contains(strings.ToLower(sk.Source.URL), lowerQuery) ||
				strings.Contains(strings.ToLower(sk.Source.Path), lowerQuery) {
				matched = true
			}
		}
		if matched {
			res = append(res, sk)
		}
	}
	return res
}

func (s *DeployModalState) VisibleGroups() []SkillGroupItem {
	query := strings.TrimSpace(s.SearchInput.Value())
	if query == "" {
		return s.Groups
	}
	lowerQuery := strings.ToLower(query)
	var res []SkillGroupItem
	for _, g := range s.Groups {
		matched := strings.Contains(strings.ToLower(g.Name), lowerQuery) ||
			strings.Contains(strings.ToLower(g.Description), lowerQuery)
		if !matched {
			for _, id := range g.SkillIDs {
				if strings.Contains(strings.ToLower(id), lowerQuery) {
					matched = true
					break
				}
			}
		}
		if matched {
			res = append(res, g)
		}
	}
	return res
}

func (s *DeployModalState) VisibleAgentTargets() []string {
	query := strings.TrimSpace(s.SearchInput.Value())
	if query == "" {
		return s.AgentTargets
	}
	lowerQuery := strings.ToLower(query)
	var res []string
	for _, t := range s.AgentTargets {
		if strings.Contains(strings.ToLower(t), lowerQuery) ||
			strings.Contains(strings.ToLower(s.AgentTargetPaths[t]), lowerQuery) {
			res = append(res, t)
		}
	}
	return res
}

func (s *DeployModalState) VisibleProjectTargets() []string {
	query := strings.TrimSpace(s.SearchInput.Value())
	if query == "" {
		return s.ProjectTargets
	}
	lowerQuery := strings.ToLower(query)
	var res []string
	for _, t := range s.ProjectTargets {
		if strings.Contains(strings.ToLower(t), lowerQuery) ||
			strings.Contains(strings.ToLower(s.ProjectTargetPaths[t]), lowerQuery) {
			res = append(res, t)
		}
	}
	return res
}

// WithLockedTarget sets a pre-bound deployment target, skipping the target selection step.
func (s DeployModalState) WithLockedTarget(name, targetType, projPath string) DeployModalState {
	s.LockedTargetName = name
	s.LockedTargetType = targetType
	s.LockedProjectPath = projPath
	return s
}

func newDeployModal(isUndeploy bool, allSkills []model.SkillStatus, currentSkillID string, targetNames []string, targets map[string]model.TargetConfig, profiles map[string]model.ProfileConfig, recentProjects []string, isProjEnabled ...func(string) bool) DeployModalState {
	var checkProjEnabled func(string) bool
	if len(isProjEnabled) > 0 {
		checkProjEnabled = isProjEnabled[0]
	}

	selected := make(map[string]bool)
	if currentSkillID != "" {
		selected[currentSkillID] = true
	}

	targetPaths := make(map[string]string)
	targetIsProject := make(map[string]string)

	// Agent targets: Loaded from user configured targets (enabled ones, or all targets if undeploying)
	var agentTargets []string
	agentPaths := make(map[string]string)

	for _, name := range targetNames {
		if tgt, ok := targets[name]; ok {
			if tgt.Type != model.TargetTypeProject && (tgt.IsEnabled() || isUndeploy) {
				skillsDir := tgt.SkillsDir()
				agentTargets = append(agentTargets, name)
				agentPaths[name] = skillsDir
				targetPaths[name] = skillsDir
			}
		}
	}
	if len(agentTargets) == 0 && len(targets) > 0 {
		var names []string
		for name, tgt := range targets {
			if tgt.Type != model.TargetTypeProject && (tgt.IsEnabled() || isUndeploy) {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			tgt := targets[name]
			skillsDir := tgt.SkillsDir()
			agentTargets = append(agentTargets, name)
			agentPaths[name] = skillsDir
			targetPaths[name] = skillsDir
		}
	}

	// Project targets: Configured project targets + Recent projects (deduplicated by CanonicalPath)
	var projectTargets []string
	projectPaths := make(map[string]string)
	seenProjPaths := make(map[string]bool)

	// Add configured project targets
	for _, name := range targetNames {
		if tgt, ok := targets[name]; ok && tgt.Type == model.TargetTypeProject {
			pDir := tgt.ResolveConfigDir()
			if pDir == "" {
				pDir = tgt.SkillsDir()
			}
			canon := fsx.CanonicalPath(pDir)
			if !tgt.IsEnabled() && !isUndeploy {
				seenProjPaths[canon] = true // Block disabled project target from being added via recentProjects
				continue
			}
			if !seenProjPaths[canon] {
				if isUndeploy || checkProjEnabled == nil || checkProjEnabled(pDir) {
					seenProjPaths[canon] = true
					display := fmt.Sprintf("%s (%s)", name, pDir)
					projectTargets = append(projectTargets, display)
					projectPaths[display] = pDir
					targetPaths[display] = pDir
					targetIsProject[display] = pDir
				}
			}
		}
	}

	for _, p := range recentProjects {
		if p == "" {
			continue
		}
		canon := fsx.CanonicalPath(p)
		if seenProjPaths[canon] {
			continue
		}
		if !isUndeploy && checkProjEnabled != nil && !checkProjEnabled(p) {
			continue
		}
		seenProjPaths[canon] = true
		projectTargets = append(projectTargets, p)
		projectPaths[p] = p
		targetPaths[p] = p
		targetIsProject[p] = p
	}

	formatIDs := model.AvailableFormatIDs()
	selectedFormats := make(map[string]bool)
	selectedFormats[model.FormatAuto] = true

	var groups []SkillGroupItem

	// 1. User defined Custom Groups (Profiles) FIRST!
	var profNames []string
	for name := range profiles {
		profNames = append(profNames, name)
	}
	sort.Strings(profNames)
	for _, name := range profNames {
		prof := profiles[name]
		if len(prof.Skills) > 0 {
			groups = append(groups, SkillGroupItem{
				Name:        fmt.Sprintf(i18n.T("deploy.group.profile_prefix"), name),
				Description: fmt.Sprintf(i18n.T("deploy.group.profile_desc"), len(prof.Skills)),
				SkillIDs:    prof.Skills,
			})
		}
	}

	// 2. Git repos
	repoMap := make(map[string][]string)
	var localIDs []string
	var managedIDs []string

	for _, s := range allSkills {
		switch s.Source.Type {
		case model.SourceTypeGit:
			repoMap[s.Source.URL] = append(repoMap[s.Source.URL], s.ID)
		case model.SourceTypeLocal:
			localIDs = append(localIDs, s.ID)
		case model.SourceTypeManaged:
			managedIDs = append(managedIDs, s.ID)
		}
	}

	var urls []string
	for url := range repoMap {
		urls = append(urls, url)
	}
	sort.Strings(urls)
	for _, url := range urls {
		ids := repoMap[url]
		shortName := url
		parts := strings.Split(strings.TrimSuffix(url, ".git"), "/")
		if len(parts) >= 2 {
			shortName = parts[len(parts)-2] + "/" + parts[len(parts)-1]
		}
		groups = append(groups, SkillGroupItem{
			Name:        fmt.Sprintf(i18n.T("deploy.group.git_prefix"), shortName),
			Description: fmt.Sprintf(i18n.T("deploy.group.git_desc"), url, len(ids)),
			SkillIDs:    ids,
		})
	}

	if len(localIDs) > 0 {
		groups = append(groups, SkillGroupItem{
			Name:        i18n.T("deploy.group.local_title"),
			Description: fmt.Sprintf(i18n.T("deploy.group.local_desc"), len(localIDs)),
			SkillIDs:    localIDs,
		})
	}

	if len(managedIDs) > 0 {
		groups = append(groups, SkillGroupItem{
			Name:        i18n.T("deploy.group.managed_title"),
			Description: fmt.Sprintf(i18n.T("deploy.group.managed_desc"), len(managedIDs)),
			SkillIDs:    managedIDs,
		})
	}

	skillCursor := 0
	if currentSkillID != "" {
		for idx, sk := range allSkills {
			if sk.ID == currentSkillID {
				skillCursor = idx
				break
			}
		}
	}

	sInput := textinput.New()
	sInput.Placeholder = i18n.T("search.placeholder")
	sInput.CharLimit = 64
	sInput.Width = 30

	return DeployModalState{
		IsUndeploy:         isUndeploy,
		Step:               DeployStepSelectSkills,
		SubMode:            DeploySubModeSkills,
		Skills:             allSkills,
		SkillsCursor:       skillCursor,
		SelectedSkills:     selected,
		Groups:             groups,
		TargetTab:          DeployTargetTabAgent,
		AgentTargets:       agentTargets,
		AgentTargetPaths:   agentPaths,
		ProjectTargets:     projectTargets,
		ProjectTargetPaths: projectPaths,
		TargetPaths:        targetPaths,
		TargetIsProject:    targetIsProject,
		FormatIDs:          formatIDs,
		SelectedFormats:    selectedFormats,
		SearchInput:        sInput,
		Searching:          false,
	}
}

// RenderDeployModal renders the deployment / undeployment modal.
func RenderDeployModal(state *DeployModalState, box lipgloss.Style, height int, loading bool, progressBlock string, err error) string {
	var b strings.Builder
	actionName := i18n.T("deploy.verb_deploy")
	actionEmoji := "🚀"
	if state.IsUndeploy {
		actionName = i18n.T("deploy.verb_undeploy")
		actionEmoji = "🗑"
	}

	if loading {
		title := i18n.T("deploy.loading_title", actionEmoji, actionName)
		if state.LockedTargetName != "" {
			title = i18n.T("deploy.loading_title_target", actionEmoji, actionName, state.LockedTargetName)
		}
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00CEC9")).Render(title) + "\n\n")
		if progressBlock != "" {
			b.WriteString(progressBlock + "\n\n")
		}
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.loading.cancel_hint")))
		return box.Render(b.String())
	}

	switch state.Step {
	case DeployStepSelectSkills:
		title := i18n.T("deploy.select_title", actionEmoji, actionName)
		if state.LockedTargetName != "" {
			if state.IsUndeploy {
				title = fmt.Sprintf(i18n.T("deploy.locked_undeploy_title"), actionEmoji, state.LockedTargetName)
			} else {
				title = fmt.Sprintf(i18n.T("deploy.locked_deploy_title"), actionEmoji, state.LockedTargetName)
			}
		}
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00CEC9")).Render(title) + "\n")
		if state.LockedTargetName != "" {
			var hint string
			if state.LockedTargetType == "project" {
				hint = fmt.Sprintf(i18n.T("deploy.locked_target_hint_proj"), state.LockedTargetName)
			} else {
				hint = fmt.Sprintf(i18n.T("deploy.locked_target_hint_agent"), state.LockedTargetName)
			}
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#55EFC4")).Render("  • "+hint) + "\n\n")
		} else {
			b.WriteString("\n")
		}

		// Sub-mode tabs: [ 1. 技能列表 (手动多选) ]   [ 2. 按分组批量 ]
		tabManual := i18n.T("deploy.tab_manual")
		tabGroup := i18n.T("deploy.tab_group")
		if state.SubMode == DeploySubModeSkills {
			tabManual = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#6C5CE7")).Bold(true).Padding(0, 1).Render(tabManual)
			tabGroup = lipgloss.NewStyle().Foreground(lipgloss.Color("#636E72")).Padding(0, 1).Render(tabGroup)
		} else {
			tabManual = lipgloss.NewStyle().Foreground(lipgloss.Color("#636E72")).Padding(0, 1).Render(tabManual)
			tabGroup = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#6C5CE7")).Bold(true).Padding(0, 1).Render(tabGroup)
		}
		b.WriteString(tabManual + "  " + tabGroup + "\n\n")

		if state.Searching || state.SearchInput.Value() != "" {
			b.WriteString("  🔍 " + state.SearchInput.View() + "\n\n")
		}

		if state.SubMode == DeploySubModeSkills {
			visibleSkills := state.VisibleSkills()
			if len(visibleSkills) == 0 {
				emptyText := i18n.T("empty.skills")
				if state.SearchInput.Value() != "" {
					emptyText = i18n.T("filter.search_empty")
				}
				b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render(emptyText) + "\n\n")
			} else {
				maxVisible := height - 15
				if maxVisible < 5 {
					maxVisible = 5
				}
				if maxVisible > 12 {
					maxVisible = 12
				}

				if state.SkillsCursor < state.ScrollOffset {
					state.ScrollOffset = state.SkillsCursor
				}
				if state.SkillsCursor >= state.ScrollOffset+maxVisible {
					state.ScrollOffset = state.SkillsCursor - maxVisible + 1
				}
				maxOffset := len(visibleSkills) - maxVisible
				if maxOffset < 0 {
					maxOffset = 0
				}
				if state.ScrollOffset > maxOffset {
					state.ScrollOffset = maxOffset
				}
				if state.ScrollOffset < 0 {
					state.ScrollOffset = 0
				}

				start := state.ScrollOffset
				end := start + maxVisible
				if end > len(visibleSkills) {
					end = len(visibleSkills)
				}

				if start > 0 {
					b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(i18n.T("more.skills.above"), start)) + "\n")
				}

				for i := start; i < end; i++ {
					s := visibleSkills[i]
					cursor := "  "
					if i == state.SkillsCursor {
						cursor = "▶ "
					}
					checked := "[ ] "
					if state.SelectedSkills[s.ID] {
						checked = "[✓] "
					}
					line := i18n.T("deploy.skill_line", cursor, checked, s.ID, s.Source.Type)
					if i == state.SkillsCursor {
						line = lipgloss.NewStyle().Foreground(lipgloss.Color("#74B9FF")).Bold(true).Render(line)
					}
					b.WriteString(line + "\n")
				}

				if end < len(visibleSkills) {
					b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(i18n.T("more.skills.below"), len(visibleSkills)-end)) + "\n")
				}
			}

			var selectedCount int
			for _, v := range state.SelectedSkills {
				if v {
					selectedCount++
				}
			}
			b.WriteString(fmt.Sprintf("\n"+i18n.T("deploy.selected_count")+"\n", selectedCount, len(state.Skills)))

			if err != nil {
				b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D63031")).Render("❌ "+err.Error()) + "\n")
			}

			b.WriteString("\n" + lipgloss.NewStyle().Faint(true).Render(i18n.T("deploy.step1_submode_hint")))
		} else {
			// SubMode == DeploySubModeGroup
			visibleGroups := state.VisibleGroups()
			if len(visibleGroups) == 0 {
				if len(state.Groups) == 0 {
					b.WriteString(i18n.T("deploy.no_groups"))
				} else {
					b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render(i18n.T("filter.search_empty")) + "\n\n")
				}
			} else {
				for i, g := range visibleGroups {
					cursor := "  "
					if i == state.GroupCursor {
						cursor = "▶ "
					}
					line := fmt.Sprintf("%s%-25s - %s", cursor, g.Name, g.Description)
					if i == state.GroupCursor {
						line = lipgloss.NewStyle().Foreground(lipgloss.Color("#74B9FF")).Bold(true).Render(line)
					}
					b.WriteString(line + "\n")
				}
			}

			if err != nil {
				b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D63031")).Render("❌ "+err.Error()) + "\n")
			}

			b.WriteString("\n" + lipgloss.NewStyle().Faint(true).Render(i18n.T("deploy.step1_group_hint")))
		}

	case DeployStepSelectTarget:
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00CEC9")).Render(
			fmt.Sprintf(i18n.T("deploy.select_target_title"), actionEmoji, actionName, len(state.ResolvedSkillIDs)),
		) + "\n\n")

		preview := strings.Join(state.ResolvedSkillIDs, ", ")
		if len(preview) > 50 {
			preview = preview[:47] + "..."
		}
		b.WriteString(fmt.Sprintf(i18n.T("deploy.skills_preview"), lipgloss.NewStyle().Foreground(lipgloss.Color("#A29BFE")).Render(preview)))

		// Target channel tabs: [ 1. Agent 渠道 (%d) ]   [ 2. 项目工程 (%d) ]
		tabAgent := fmt.Sprintf(i18n.T("deploy.tab_target_agent"), len(state.AgentTargets))
		tabProj := fmt.Sprintf(i18n.T("deploy.tab_target_project"), len(state.ProjectTargets))
		if state.TargetTab == DeployTargetTabAgent {
			tabAgent = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#0984E3")).Bold(true).Padding(0, 1).Render(tabAgent)
			tabProj = lipgloss.NewStyle().Foreground(lipgloss.Color("#636E72")).Padding(0, 1).Render(tabProj)
		} else {
			tabAgent = lipgloss.NewStyle().Foreground(lipgloss.Color("#636E72")).Padding(0, 1).Render(tabAgent)
			tabProj = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#0984E3")).Bold(true).Padding(0, 1).Render(tabProj)
		}
		b.WriteString(tabAgent + "  " + tabProj + "\n\n")

		if state.Searching || state.SearchInput.Value() != "" {
			b.WriteString("  🔍 " + state.SearchInput.View() + "\n\n")
		}

		maxVisible := height - 16
		if maxVisible < 5 {
			maxVisible = 5
		}
		if maxVisible > 10 {
			maxVisible = 10
		}

		if state.TargetTab == DeployTargetTabAgent {
			visibleAgents := state.VisibleAgentTargets()
			if len(visibleAgents) == 0 {
				if len(state.AgentTargets) == 0 {
					b.WriteString(i18n.T("empty.targets_agent"))
				} else {
					b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render(i18n.T("filter.search_empty")) + "\n")
				}
			} else {
				if state.AgentCursor < state.AgentScroll {
					state.AgentScroll = state.AgentCursor
				}
				if state.AgentCursor >= state.AgentScroll+maxVisible {
					state.AgentScroll = state.AgentCursor - maxVisible + 1
				}
				maxOffset := len(visibleAgents) - maxVisible
				if maxOffset < 0 {
					maxOffset = 0
				}
				if state.AgentScroll > maxOffset {
					state.AgentScroll = maxOffset
				}
				if state.AgentScroll < 0 {
					state.AgentScroll = 0
				}
				start := state.AgentScroll
				end := start + maxVisible
				if end > len(visibleAgents) {
					end = len(visibleAgents)
				}
				for i := start; i < end; i++ {
					name := visibleAgents[i]
					cursor := "  "
					if i == state.AgentCursor {
						cursor = "▶ "
					}
					line := fmt.Sprintf("%s%-18s (%s)", cursor, name, state.AgentTargetPaths[name])
					if i == state.AgentCursor {
						line = lipgloss.NewStyle().Foreground(lipgloss.Color("#74B9FF")).Bold(true).Render(line)
					}
					b.WriteString(line + "\n")
				}
			}
		} else {
			// Project targets
			visibleProjs := state.VisibleProjectTargets()
			if len(visibleProjs) == 0 {
				if len(state.ProjectTargets) == 0 {
					b.WriteString(i18n.T("empty.targets_project"))
				} else {
					b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render(i18n.T("filter.search_empty")) + "\n")
				}
			} else {
				if state.ProjectCursor < state.ProjectScroll {
					state.ProjectScroll = state.ProjectCursor
				}
				if state.ProjectCursor >= state.ProjectScroll+maxVisible {
					state.ProjectScroll = state.ProjectCursor - maxVisible + 1
				}
				maxOffset := len(visibleProjs) - maxVisible
				if maxOffset < 0 {
					maxOffset = 0
				}
				if state.ProjectScroll > maxOffset {
					state.ProjectScroll = maxOffset
				}
				if state.ProjectScroll < 0 {
					state.ProjectScroll = 0
				}
				start := state.ProjectScroll
				end := start + maxVisible
				if end > len(visibleProjs) {
					end = len(visibleProjs)
				}
				for i := start; i < end; i++ {
					p := visibleProjs[i]
					cursor := "  "
					if i == state.ProjectCursor {
						cursor = "▶ "
					}
					line := fmt.Sprintf("%s%-24s", cursor, p)
					if i == state.ProjectCursor {
						line = lipgloss.NewStyle().Foreground(lipgloss.Color("#74B9FF")).Bold(true).Render(line)
					}
					b.WriteString(line + "\n")
				}
			}
		}

		if err != nil {
			b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D63031")).Render("❌ "+err.Error()) + "\n")
		}

		b.WriteString("\n" + lipgloss.NewStyle().Faint(true).Render(i18n.T("deploy.step2_target_tab_hint")))

	case DeployStepSelectFormat:
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00CEC9")).Render(
			fmt.Sprintf(i18n.T("deploy.select_format_title"), actionEmoji, actionName, state.SelectedProject),
		) + "\n\n")

		for i, fmtID := range state.FormatIDs {
			cursor := "  "
			if i == state.FormatCursor {
				cursor = "▶ "
			}
			boxIcon := "[ ]"
			if state.SelectedFormats[fmtID] {
				boxIcon = "[✓]"
			}

			desc := ""
			if fmtID == model.FormatAuto {
				desc = i18n.T("deploy.format.auto_detect")
			} else if def, ok := model.LookupFormat(fmtID); ok {
				desc = def.Description
			}

			line := fmt.Sprintf("%s%s %-10s - %s", cursor, boxIcon, fmtID, desc)
			if i == state.FormatCursor {
				line = lipgloss.NewStyle().Foreground(lipgloss.Color("#74B9FF")).Bold(true).Render(line)
			}
			b.WriteString(line + "\n")
		}

		if err != nil {
			b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D63031")).Render("❌ "+err.Error()) + "\n")
		}

		b.WriteString("\n" + lipgloss.NewStyle().Faint(true).Render(i18n.T("deploy.step3_format_hint")))
	}

	return box.Render(b.String())
}

// EditSettingModalState manages editing a key-value setting.
type EditSettingModalState struct {
	SettingKey string // e.g. "default_root", "workspace_root"
	Title      string
	Label      string
	Input      textinput.Model
}

func newEditSettingModal(key, title, label, initialValue string) EditSettingModalState {
	input := textinput.New()
	input.SetValue(initialValue)
	input.Focus()
	input.CharLimit = 256
	input.Width = 60
	return EditSettingModalState{
		SettingKey: key,
		Title:      title,
		Label:      label,
		Input:      input,
	}
}

// RenderEditSettingModal renders the textinput modal for editing setting values.
func RenderEditSettingModal(state *EditSettingModalState, box lipgloss.Style, err error) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#74B9FF")).Render(state.Title) + "\n\n")
	b.WriteString(state.Label + "\n")
	b.WriteString(state.Input.View() + "\n\n")
	if err != nil {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D63031")).Render("❌ "+err.Error()) + "\n\n")
	}
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.edit_setting.hint")))
	return box.Render(b.String())
}

// LanguageOption represents an interface language selection option.
type LanguageOption struct {
	Code  string
	Label string
}

// SelectLanguageModalState manages interface language selection.
type SelectLanguageModalState struct {
	Options []LanguageOption
	Cursor  int
}

func newSelectLanguageModal(currentCode string) SelectLanguageModalState {
	options := []LanguageOption{
		{Code: "zh-CN", Label: i18n.T("settings.lang.zh_cn")},
		{Code: "en", Label: i18n.T("settings.lang.en")},
		{Code: "auto", Label: i18n.T("settings.lang.auto")},
	}
	cursor := 0
	for i, opt := range options {
		if strings.EqualFold(opt.Code, currentCode) {
			cursor = i
			break
		}
	}
	return SelectLanguageModalState{
		Options: options,
		Cursor:  cursor,
	}
}

// RenderSelectLanguageModal renders the language selection modal.
func RenderSelectLanguageModal(state *SelectLanguageModalState, box lipgloss.Style, currentCode string) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A29BFE")).Render(i18n.T("modal.language.title")) + "\n\n")
	for i, opt := range state.Options {
		cursor := "  "
		if i == state.Cursor {
			cursor = "▶ "
		}
		checked := "○ "
		if strings.EqualFold(opt.Code, currentCode) {
			checked = "● "
		}
		line := fmt.Sprintf("%s%s%d. %s", cursor, checked, i+1, opt.Label)
		if i == state.Cursor {
			line = lipgloss.NewStyle().Foreground(lipgloss.Color("#74B9FF")).Bold(true).Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.language.hint")))
	return box.Render(b.String())
}

// WorkspaceOption represents a registered workspace option in the selector.
type WorkspaceOption struct {
	Path        string
	Initialized bool
	Active      bool
}

// SelectWorkspaceModalState manages workspace selection, switching, and deletion.
type SelectWorkspaceModalState struct {
	Options []WorkspaceOption
	Cursor  int
}

func newSelectWorkspaceModal(workspaces []string, activePath string, checkInit func(string) bool) SelectWorkspaceModalState {
	var options []WorkspaceOption
	cursor := 0
	for _, ws := range workspaces {
		ws = fsx.NormalizeWorkspacePath(ws)
		already := false
		for _, opt := range options {
			if fsx.SamePath(opt.Path, ws) {
				already = true
				break
			}
		}
		if already {
			continue
		}
		isInit := false
		if checkInit != nil {
			isInit = checkInit(ws)
		}
		isActive := false
		if fsx.SamePath(ws, activePath) {
			isActive = true
			cursor = len(options)
		}
		options = append(options, WorkspaceOption{
			Path:        ws,
			Initialized: isInit,
			Active:      isActive,
		})
	}
	if cursor >= len(options) && len(options) > 0 {
		cursor = 0
	}
	return SelectWorkspaceModalState{
		Options: options,
		Cursor:  cursor,
	}
}

// RenderSelectWorkspaceModal renders the workspace selection modal.
func RenderSelectWorkspaceModal(state *SelectWorkspaceModalState, box lipgloss.Style, err error) string {
	var b strings.Builder
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A29BFE")).Render(i18n.T("modal.workspaces.title"))
	b.WriteString(title + "\n\n")

	if len(state.Options) == 0 {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.workspaces.empty")) + "\n")
	} else {
		for i, opt := range state.Options {
			cursor := "  "
			if i == state.Cursor {
				cursor = "▶ "
			}
			activeMark := "  "
			if opt.Active {
				activeMark = lipgloss.NewStyle().Foreground(lipgloss.Color("#55EFC4")).Bold(true).Render("● ")
			}
			statusBadge := lipgloss.NewStyle().Foreground(lipgloss.Color("#00B894")).Render(i18n.T("settings.status.initialized"))
			if !opt.Initialized {
				statusBadge = lipgloss.NewStyle().Foreground(lipgloss.Color("#FDCB6E")).Render(i18n.T("settings.status.uninitialized"))
			}
			pathStr := opt.Path
			if i == state.Cursor {
				pathStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#74B9FF")).Bold(true).Render(pathStr)
			}
			line := fmt.Sprintf("%s%s%-36s  %s", cursor, activeMark, pathStr, statusBadge)
			b.WriteString(line + "\n")
		}
	}

	if err != nil {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#FF7675")).Render("✖ "+err.Error()) + "\n")
	}

	b.WriteString("\n" + lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.workspaces.hint")))
	return box.Render(b.String())
}

// AddWorkspaceModalState manages adding a new workspace.
type AddWorkspaceModalState struct {
	Input textinput.Model
}

func newAddWorkspaceModal() AddWorkspaceModalState {
	ti := textinput.New()
	ti.Placeholder = config.DefaultWorkspacePathCompact()
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 50
	return AddWorkspaceModalState{Input: ti}
}

// RenderAddWorkspaceModal renders the prompt to add a new workspace.
func RenderAddWorkspaceModal(state *AddWorkspaceModalState, box lipgloss.Style, err error) string {
	var b strings.Builder
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A29BFE")).Render(i18n.T("modal.add_workspace.title"))
	b.WriteString(title + "\n\n")

	b.WriteString(i18n.T("modal.add_workspace.label") + "\n")
	b.WriteString(state.Input.View() + "\n\n")

	if err != nil {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF7675")).Render("✖ "+err.Error()) + "\n\n")
	}

	b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.add_workspace.hint")))
	return box.Render(b.String())
}

// TargetDetailModalState manages target detail view.
type TargetDetailModalState struct {
	Name           string
	Type           string // "agent" or "project"
	Channel        string
	ConfigDir      string
	ResolvedDir    string
	SkillsDir      string
	Paths          map[string]string
	PathExists     bool
	SkillsExists   bool
	Formats        string
	DeployedSkills []string
	Enabled        bool
	Viewport       viewport.Model
	Ready          bool
}

func newTargetDetailModal(name, tgtType, channel, configDir, resolvedDir, skillsDir string, paths map[string]string, pathExists, skillsExists bool, formats string, deployedSkills []string, enabled bool) TargetDetailModalState {
	vp := viewport.New(70, 10)
	vp.GotoTop()
	return TargetDetailModalState{
		Name:           name,
		Type:           tgtType,
		Channel:        channel,
		ConfigDir:      configDir,
		ResolvedDir:    resolvedDir,
		SkillsDir:      skillsDir,
		Paths:          paths,
		PathExists:     pathExists,
		SkillsExists:   skillsExists,
		Formats:        formats,
		DeployedSkills: deployedSkills,
		Enabled:        enabled,
		Viewport:       vp,
		Ready:          true,
	}
}

// RenderTargetDetailModal renders comprehensive details for an agent or project target.
func RenderTargetDetailModal(state *TargetDetailModalState, box lipgloss.Style, termWidth, termHeight int) string {
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
		if termHeight > 0 && termHeight < 8 {
			maxOuterHeight = termHeight
		} else {
			maxOuterHeight = 8
		}
	}

	maxInnerHeight := maxOuterHeight - box.GetVerticalFrameSize()
	if maxInnerHeight < 4 {
		maxInnerHeight = 4
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A29BFE"))

	fullTitle := fmt.Sprintf(i18n.T("modal.target_detail.title"), state.Name)
	if lipgloss.Width(fullTitle) > contentWidth {
		fullTitle = truncate.StringWithTail(fullTitle, uint(contentWidth), "…")
	}
	headerStr := titleStyle.Render(fullTitle) + "\n\n"
	headerLines := lipgloss.Height(headerStr)

	// Build the scrollable body content
	bodyContent := buildTargetDetailBody(state, contentWidth)
	bodyHeight := lipgloss.Height(bodyContent)

	// Tentative footer to check if scrolling is needed
	tentativeFooter := renderDetailFooter(i18n.T("modal.target_detail.hint"), contentWidth)
	tentativeFooterLines := lipgloss.Height(tentativeFooter) + 1
	availVpH := maxInnerHeight - headerLines - tentativeFooterLines
	if availVpH < 3 {
		availVpH = 3
	}

	isScrollable := bodyHeight > availVpH

	var footerText string
	if isScrollable {
		var scrollBadge string
		if state.Viewport.AtTop() {
			scrollBadge = "[TOP]"
		} else if state.Viewport.AtBottom() {
			scrollBadge = "[END]"
		} else {
			scrollBadge = fmt.Sprintf("[%2.0f%%]", state.Viewport.ScrollPercent()*100)
		}
		scrollHint := i18n.T("modal.target_detail.scroll_hint")
		footerText = fmt.Sprintf("%s %s • %s", scrollBadge, scrollHint, i18n.T("modal.target_detail.hint"))
	} else {
		footerText = i18n.T("modal.target_detail.hint")
	}

	footerRendered := renderDetailFooter(footerText, contentWidth)
	actualFooterLines := lipgloss.Height(footerRendered) + 1

	finalMaxVpH := maxInnerHeight - headerLines - actualFooterLines
	if finalMaxVpH < 3 {
		finalMaxVpH = 3
	}

	vpH := bodyHeight
	if vpH > finalMaxVpH {
		vpH = finalMaxVpH
	}

	if !state.Ready || state.Viewport.Width != contentWidth || state.Viewport.Height != vpH {
		currOffset := state.Viewport.YOffset
		state.Viewport.Width = contentWidth
		state.Viewport.Height = vpH
		state.Viewport.SetContent(bodyContent)
		state.Viewport.YOffset = currOffset
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

func buildTargetDetailBody(state *TargetDetailModalState, contentWidth int) string {
	var b strings.Builder

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#55EFC4"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#B2BEC3"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#74B9FF"))
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00B894")).Bold(true)
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FAB1A0"))

	// 1. Basic info
	typeDesc := i18n.T("target.type.agent_desc")
	if state.Type == "project" {
		typeDesc = i18n.T("target.type.project_desc")
	}
	renderDetailField(&b, i18n.T("modal.target_detail.name"), state.Name, labelStyle, valueStyle, contentWidth)
	renderDetailField(&b, i18n.T("modal.target_detail.type"), typeDesc, labelStyle, valueStyle, contentWidth)

	statusBadge := okStyle.Render("✓ " + i18n.T("target.status.enabled"))
	if !state.Enabled {
		statusBadge = warnStyle.Render("✗ " + i18n.T("target.status.disabled"))
	}
	b.WriteString(fmt.Sprintf("  • %s %s\n", labelStyle.Render(i18n.T("modal.target_detail.status")+":"), statusBadge))

	if state.Channel != "" {
		renderDetailField(&b, i18n.T("modal.target_detail.channel"), state.Channel, labelStyle, valueStyle, contentWidth)
	}
	if state.Formats != "" {
		renderDetailField(&b, i18n.T("modal.target_detail.formats"), state.Formats, labelStyle, valueStyle, contentWidth)
	}
	b.WriteString("\n")

	// 2. Base config directory
	b.WriteString(sectionStyle.Render(i18n.T("modal.target_detail.sec_config_dir")) + "\n")
	renderDetailField(&b, i18n.T("modal.target_detail.configured_path"), fsx.CompactUser(state.ConfigDir), labelStyle, pathStyle, contentWidth)

	statusText := okStyle.Render("✓ " + i18n.T("modal.target_detail.dir_exists"))
	if !state.PathExists {
		statusText = warnStyle.Render("✗ " + i18n.T("modal.target_detail.dir_not_exists"))
	}
	compactResolved := fsx.CompactUser(state.ResolvedDir)
	resolvedPrefix := fmt.Sprintf("  • %s: ", i18n.T("modal.target_detail.resolved_path"))
	if lipgloss.Width(resolvedPrefix)+lipgloss.Width(compactResolved)+lipgloss.Width(statusText)+4 <= contentWidth {
		b.WriteString(fmt.Sprintf("  • %s %s  (%s)\n\n", labelStyle.Render(i18n.T("modal.target_detail.resolved_path")+":"), pathStyle.Render(compactResolved), statusText))
	} else {
		renderDetailField(&b, i18n.T("modal.target_detail.resolved_path"), compactResolved, labelStyle, pathStyle, contentWidth)
		b.WriteString(fmt.Sprintf("      (%s)\n\n", statusText))
	}

	// 3. Artifact mappings (for agent targets)
	if state.Type != "project" {
		b.WriteString(sectionStyle.Render(i18n.T("modal.target_detail.sec_mappings")) + "\n")
		if len(state.Paths) > 0 {
			var keys []string
			for k := range state.Paths {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				tpl := state.Paths[k]
				resolved := tpl
				if state.ConfigDir != "" {
					resolved = strings.ReplaceAll(resolved, "${config_dir}", state.ConfigDir)
				}
				expResolved, _ := fsx.ExpandUser(resolved)
				ex := fsx.DirExists(expResolved) || fsx.FileExists(expResolved)
				exBadge := okStyle.Render("✓ " + i18n.T("modal.target_detail.exists"))
				if !ex {
					exBadge = warnStyle.Render("✗ " + i18n.T("modal.target_detail.not_created"))
				}
				compactExp := fsx.CompactUser(expResolved)
				b.WriteString(fmt.Sprintf("  • %s %s\n",
					valueStyle.Render(k+":"),
					lipgloss.NewStyle().Faint(true).Render(tpl),
				))
				arrowLine := fmt.Sprintf("      ➔ %s (%s)\n", pathStyle.Render(compactExp), exBadge)
				if lipgloss.Width(arrowLine) <= contentWidth {
					b.WriteString(arrowLine)
				} else {
					b.WriteString("      ➔ " + pathStyle.Render(compactExp) + "\n")
					b.WriteString(fmt.Sprintf("        (%s)\n", exBadge))
				}
			}
		} else {
			exBadge := okStyle.Render("✓ " + i18n.T("modal.target_detail.exists"))
			if !state.SkillsExists {
				exBadge = warnStyle.Render("✗ " + i18n.T("modal.target_detail.not_created"))
			}
			b.WriteString(fmt.Sprintf("  • %s %s  (%s)\n",
				valueStyle.Render(i18n.T("modal.target_detail.skills_label")),
				pathStyle.Render(fsx.CompactUser(state.SkillsDir)),
				exBadge,
			))
		}
		b.WriteString("\n")
	}

	// 4. Deployed skills
	b.WriteString(sectionStyle.Render(i18n.T("modal.target_detail.sec_deployed")) + "\n")
	if len(state.DeployedSkills) > 0 {
		b.WriteString(renderDetailSkillList(i18n.T("modal.target_detail.deployed_skills"), len(state.DeployedSkills), state.DeployedSkills, 0, valueStyle, labelStyle, contentWidth))
	} else {
		b.WriteString("  • " + lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.target_detail.no_deployed")) + "\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// DeployConflictModalState holds state for resolving deployment conflicts when target files differ.
type DeployConflictModalState struct {
	SkillID        string
	TargetName     string
	ProjectPath    string
	Format         string
	TargetDir      string
	IsUntracked    bool
	ErrMessage     string
	BatchDeployed  int
	BatchTotal     int
	RemainingQueue []*deploy.TargetConflictError
}

// RenderDeployConflictModal renders the conflict resolution dialog.
func RenderDeployConflictModal(state *DeployConflictModalState, box lipgloss.Style, height int, loading bool, progressBlock string) string {
	var b strings.Builder

	title := i18n.T("modal.conflict.title")
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF7675")).Render(title) + "\n\n")

	if loading {
		if progressBlock != "" {
			b.WriteString(progressBlock + "\n\n")
		}
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.loading.cancel_hint")))
		return box.Render(b.String())
	}

	if state.BatchTotal > 1 {
		progress := fmt.Sprintf(i18n.T("modal.conflict.batch_progress"), state.BatchDeployed, state.BatchTotal)
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#55EFC4")).Render("  ✓ "+progress) + "\n\n")
	}

	var desc string
	if state.IsUntracked {
		desc = i18n.T("modal.conflict.untracked_desc")
	} else {
		desc = i18n.T("modal.conflict.modified_desc")
	}
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FAB1A0")).Render("  "+desc) + "\n\n")

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#74B9FF"))
	valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#DFE6E9"))
	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00CEC9"))

	targetDisplay := state.TargetName
	if state.ProjectPath != "" {
		targetDisplay = fsx.CompactUser(state.ProjectPath)
	}

	b.WriteString(fmt.Sprintf("  • %-10s %s\n", labelStyle.Render(i18n.T("modal.conflict.skill")+":"), valStyle.Render(state.SkillID)))
	b.WriteString(fmt.Sprintf("  • %-10s %s\n", labelStyle.Render(i18n.T("modal.conflict.target")+":"), valStyle.Render(targetDisplay)))
	if state.Format != "" {
		b.WriteString(fmt.Sprintf("  • %-10s %s\n", labelStyle.Render(i18n.T("modal.conflict.format")+":"), valStyle.Render(state.Format)))
	}
	b.WriteString(fmt.Sprintf("  • %-10s %s\n\n", labelStyle.Render(i18n.T("modal.conflict.path")+":"), pathStyle.Render(fsx.CompactUser(state.TargetDir))))

	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A29BFE")).Render("  "+i18n.T("modal.conflict.options_title")) + "\n")
	optStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	optKeyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FDCB6E"))

	b.WriteString("    " + optKeyStyle.Render("[v]") + " " + optStyle.Render(i18n.T("modal.conflict.opt_diff")) + "\n")
	b.WriteString("    " + optKeyStyle.Render("[f]") + " " + optStyle.Render(i18n.T("modal.conflict.opt_force")) + "\n")
	b.WriteString("    " + optKeyStyle.Render("[b]") + " " + optStyle.Render(i18n.T("modal.conflict.opt_adopt")) + "\n")

	cancelText := i18n.T("modal.conflict.opt_cancel")
	if state.BatchTotal > 1 || len(state.RemainingQueue) > 0 {
		cancelText = i18n.T("modal.conflict.opt_skip")
	}
	b.WriteString("    " + optKeyStyle.Render("[Esc]") + " " + optStyle.Render(cancelText) + "\n\n")

	hintStyle := lipgloss.NewStyle().Faint(true)
	b.WriteString(hintStyle.Render("  " + i18n.T("modal.conflict.hint")))

	return box.Render(b.String())
}

// BatchRemoveFilterMode represents the active filter mode for batch removing skills.
type BatchRemoveFilterMode int

const (
	BatchRemoveByRegex   BatchRemoveFilterMode = iota // 0: 按名称/ID正则匹配
	BatchRemoveBySource                               // 1: 按来源仓库/本地目录
	BatchRemoveByChannel                              // 2: 按部署目标渠道
)

// BatchRemoveModalState manages filtering and batch removal by regex, source, or target channel.
type BatchRemoveModalState struct {
	Mode          BatchRemoveFilterMode
	RegexInput    textinput.Model
	SourceCursor  int
	Sources       []string // unique sources, e.g. "https://github.com/microsoft/skills" or "local"
	ChannelCursor int
	Channels      []string // unique targets/channels, e.g. "opencode", "codex", project paths

	MatchedSkills []model.SkillStatus
	Deployments   map[string][]string // skillID -> targets deployed
	RegexError    string
	Force         bool
}

func newBatchRemoveModal() BatchRemoveModalState {
	ti := textinput.New()
	ti.Placeholder = i18n.T("modal.batch_remove.regex_placeholder")
	ti.Focus()
	ti.CharLimit = 128
	ti.Width = 50

	return BatchRemoveModalState{
		Mode:        BatchRemoveByRegex,
		RegexInput:  ti,
		Deployments: make(map[string][]string),
	}
}

// RenderBatchRemoveModal renders the batch removal modal with tabs for regex, source, and channel.
func RenderBatchRemoveModal(state *BatchRemoveModalState, box lipgloss.Style, height int) string {
	var b strings.Builder

	// Title
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF7675")).Render(i18n.T("modal.batch_remove.title")) + "\n\n")

	// Filter Mode Tabs: [ 1. 按技能正则 ]   [ 2. 按来源仓库 ]   [ 3. 按部署渠道 ]
	activeTabStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#6C5CE7")).Padding(0, 1)
	inactiveTabStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#B2BEC3")).Background(lipgloss.Color("#2D3436")).Padding(0, 1)

	tab1 := i18n.T("modal.batch_remove.tab_regex")
	tab2 := i18n.T("modal.batch_remove.tab_source")
	tab3 := i18n.T("modal.batch_remove.tab_channel")

	if state.Mode == BatchRemoveByRegex {
		b.WriteString(activeTabStyle.Render("[1] "+tab1) + "  " + inactiveTabStyle.Render("[2] "+tab2) + "  " + inactiveTabStyle.Render("[3] "+tab3) + "\n\n")
	} else if state.Mode == BatchRemoveBySource {
		b.WriteString(inactiveTabStyle.Render("[1] "+tab1) + "  " + activeTabStyle.Render("[2] "+tab2) + "  " + inactiveTabStyle.Render("[3] "+tab3) + "\n\n")
	} else {
		b.WriteString(inactiveTabStyle.Render("[1] "+tab1) + "  " + inactiveTabStyle.Render("[2] "+tab2) + "  " + activeTabStyle.Render("[3] "+tab3) + "\n\n")
	}

	// Filter Controls
	switch state.Mode {
	case BatchRemoveByRegex:
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#74B9FF")).Render(i18n.T("modal.batch_remove.regex_label")) + "\n")
		b.WriteString(state.RegexInput.View() + "\n")
		if state.RegexError != "" {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF7675")).Render("❌ "+state.RegexError) + "\n")
		} else {
			b.WriteString("\n")
		}

	case BatchRemoveBySource:
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#74B9FF")).Render(i18n.T("modal.batch_remove.source_label")) + "\n")
		if len(state.Sources) == 0 {
			b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.batch_remove.no_sources")))
		} else {
			maxShow := 4
			start := 0
			if state.SourceCursor >= maxShow {
				start = state.SourceCursor - maxShow + 1
			}
			end := start + maxShow
			if end > len(state.Sources) {
				end = len(state.Sources)
			}
			for i := start; i < end; i++ {
				src := state.Sources[i]
				cursor := "  "
				lineStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#DFE6E9"))
				if i == state.SourceCursor {
					cursor = "▶ "
					lineStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#55EFC4"))
				}
				b.WriteString(cursor + lineStyle.Render(src) + "\n")
			}
			b.WriteString("\n")
		}

	case BatchRemoveByChannel:
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#74B9FF")).Render(i18n.T("modal.batch_remove.channel_label")) + "\n")
		if len(state.Channels) == 0 {
			b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.batch_remove.no_channels")))
		} else {
			maxShow := 4
			start := 0
			if state.ChannelCursor >= maxShow {
				start = state.ChannelCursor - maxShow + 1
			}
			end := start + maxShow
			if end > len(state.Channels) {
				end = len(state.Channels)
			}
			for i := start; i < end; i++ {
				ch := state.Channels[i]
				cursor := "  "
				lineStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#DFE6E9"))
				if i == state.ChannelCursor {
					cursor = "▶ "
					lineStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#55EFC4"))
				}
				b.WriteString(cursor + lineStyle.Render(ch) + "\n")
			}
			b.WriteString("\n")
		}
	}

	// Matched Skills Preview
	matchedCount := len(state.MatchedSkills)
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FDCB6E"))
	b.WriteString(headerStyle.Render(fmt.Sprintf(i18n.T("modal.batch_remove.matched_title"), matchedCount)) + "\n")

	if matchedCount == 0 {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("  "+i18n.T("modal.batch_remove.no_match")) + "\n\n")
	} else {
		maxVisible := 5
		for i, s := range state.MatchedSkills {
			if i >= maxVisible {
				b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.batch_remove.and_more", matchedCount-maxVisible)) + "\n")
				break
			}
			depInfo := ""
			if deps := state.Deployments[s.ID]; len(deps) > 0 {
				depInfo = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF7675")).Render(i18n.T("modal.batch_remove.deployed_tag", strings.Join(deps, ", ")))
			}
			b.WriteString(fmt.Sprintf("  • %-20s (%s)%s\n", s.ID, s.Source.Type, depInfo))
		}
		b.WriteString("\n")
	}

	// Bottom action bar
	actions := hintText(i18n.T("modal.batch_remove.actions"))
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#DFE6E9")).Render(actions))

	return box.Render(b.String())
}

// Upstream scan mode values used by the add/edit modal tab.
const (
	upstreamScanModeDefault = iota
	upstreamScanModeCustom
	upstreamScanModeWhole
)

// upstreamScanForm holds the skill scan scope fields shared by the add and edit upstream modals.
type upstreamScanForm struct {
	RootsInput   textarea.Model
	ExcludeInput textarea.Model
	Mode         int
}

func upstreamScanModes() []int {
	return []int{upstreamScanModeDefault, upstreamScanModeCustom, upstreamScanModeWhole}
}

func newUpstreamScanForm(scan model.ScanConfig) upstreamScanForm {
	norm := scan.Normalized()
	form := upstreamScanForm{Mode: upstreamScanModeDefault}
	switch norm.Mode() {
	case model.ScanModeWhole:
		form.Mode = upstreamScanModeWhole
	case model.ScanModeCustom:
		form.Mode = upstreamScanModeCustom
	}

	roots := norm.Roots
	if form.Mode == upstreamScanModeWhole {
		roots = nil
	}
	form.RootsInput = newScanTextarea(strings.Join(roots, "\n"), i18n.T("modal.upstream.scan.roots_placeholder"))
	form.ExcludeInput = newScanTextarea(strings.Join(norm.Exclude, "\n"), i18n.T("modal.upstream.scan.exclude_placeholder"))
	return form
}

func newScanTextarea(value, placeholder string) textarea.Model {
	ta := textarea.New()
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	ta.Placeholder = placeholder
	ta.SetWidth(56)
	ta.SetHeight(3)
	ta.SetValue(value)
	ta.Blur()
	return ta
}

func scanModeLabel(mode int) string {
	switch mode {
	case upstreamScanModeDefault:
		return i18n.T("modal.upstream.scan.mode_default")
	case upstreamScanModeWhole:
		return i18n.T("modal.upstream.scan.mode_whole")
	default:
		return i18n.T("modal.upstream.scan.mode_custom")
	}
}

func (f *upstreamScanForm) cycleMode(delta int) {
	modes := upstreamScanModes()
	idx := 0
	for i, m := range modes {
		if m == f.Mode {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(modes)) % len(modes)
	f.Mode = modes[idx]
}

// config validates the form into a scan scope. It returns a localized message on failure.
func (f upstreamScanForm) config() (model.ScanConfig, string) {
	var roots []string
	switch f.Mode {
	case upstreamScanModeDefault:
		roots = nil
	case upstreamScanModeWhole:
		roots = []string{model.ScanRootWholeSource}
	default:
		for _, line := range splitScanLines(f.RootsInput.Value()) {
			clean, ok := model.NormalizeScanPath(line)
			if !ok {
				return model.ScanConfig{}, i18n.T("error.upstream_scan_path_invalid", line)
			}
			roots = append(roots, clean)
		}
		if len(roots) == 0 {
			return model.ScanConfig{}, i18n.T("error.upstream_scan_roots_empty")
		}
	}

	var exclude []string
	for _, line := range splitScanLines(f.ExcludeInput.Value()) {
		clean, ok := model.NormalizeScanPath(line)
		if !ok {
			return model.ScanConfig{}, i18n.T("error.upstream_scan_path_invalid", line)
		}
		if clean == model.ScanRootWholeSource {
			return model.ScanConfig{}, i18n.T("error.upstream_scan_exclude_whole")
		}
		exclude = append(exclude, clean)
	}
	return model.ScanConfig{Roots: roots, Exclude: exclude}, ""
}

func splitScanLines(value string) []string {
	var out []string
	for _, line := range strings.Split(value, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

func upstreamFieldLabel(text string, focused bool) string {
	if focused {
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A29BFE")).Render(text)
	}
	return lipgloss.NewStyle().Bold(true).Render(text)
}

func renderUpstreamTabBar(tab int, focused bool) string {
	labels := []string{i18n.T("modal.upstream.tab.basic"), i18n.T("modal.upstream.tab.scan")}
	rendered := make([]string, 0, len(labels))
	for i, l := range labels {
		var style lipgloss.Style
		switch {
		case i == tab && focused:
			style = subTabFocusedStyle
		case i == tab:
			style = subTabSelectedStyle
		case focused:
			style = ancestorTabStyle
		default:
			style = subTabInactiveStyle
		}
		rendered = append(rendered, style.Render(l))
	}
	return strings.Join(rendered, " ")
}

// renderUpstreamScanForm renders the scan tab body. active is the focused field group
// (1 = mode, 2 = roots, 3 = exclude); 0 means focus is on the tab bar.
func renderUpstreamScanForm(f *upstreamScanForm, active int) string {
	var b strings.Builder

	b.WriteString(upstreamFieldLabel(i18n.T("modal.upstream.scan.mode_label"), active == 1) + "\n")
	var pills []string
	for _, m := range upstreamScanModes() {
		var style lipgloss.Style
		switch {
		case m == f.Mode && active == 1:
			style = subTabFocusedStyle
		case m == f.Mode:
			style = subTabSelectedStyle
		default:
			style = subTabInactiveStyle
		}
		pills = append(pills, style.Render(scanModeLabel(m)))
	}
	b.WriteString("  " + strings.Join(pills, " ") + "\n")
	if f.Mode == upstreamScanModeWhole {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("  "+i18n.T("modal.upstream.scan.whole_warning")) + "\n")
	}
	b.WriteString("\n")

	b.WriteString(upstreamFieldLabel(i18n.T("modal.upstream.scan.roots_label"), active == 2) + "\n")
	if f.Mode == upstreamScanModeCustom {
		b.WriteString(f.RootsInput.View() + "\n\n")
	} else {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("  "+scanModeLabel(f.Mode)) + "\n\n")
	}

	b.WriteString(upstreamFieldLabel(i18n.T("modal.upstream.scan.exclude_label"), active == 3) + "\n")
	b.WriteString(f.ExcludeInput.View() + "\n\n")

	b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.upstream.scan.note")))
	return b.String()
}

// AddUpstreamModalState holds state for adding an upstream source.
type AddUpstreamModalState struct {
	URLInput  textinput.Model
	RefInput  textinput.Model
	NameInput textinput.Model
	Scan      upstreamScanForm
	Tab       int // 0: basic information, 1: skill scan
	Active    int // 0: tab bar; 1..3: field groups within the current tab
}

func newAddUpstreamModal() AddUpstreamModalState {
	u := textinput.New()
	u.Placeholder = i18n.T("modal.upstream.url_placeholder")
	u.CharLimit = 256
	u.Width = 60
	u.Focus()

	r := textinput.New()
	r.Placeholder = i18n.T("modal.upstream.ref_placeholder")
	r.CharLimit = 128
	r.Width = 60

	n := textinput.New()
	n.Placeholder = i18n.T("modal.upstream.name_placeholder")
	n.CharLimit = 64
	n.Width = 60

	return AddUpstreamModalState{
		URLInput:  u,
		RefInput:  r,
		NameInput: n,
		Scan:      newUpstreamScanForm(model.DefaultScanConfig()),
		Tab:       0,
		Active:    1,
	}
}

// validFieldGroups returns the focusable groups for the current tab in tab order.
func (am AddUpstreamModalState) validFieldGroups() []int {
	if am.Tab == 1 && am.Scan.Mode != upstreamScanModeCustom {
		return []int{0, 1, 3}
	}
	return []int{0, 1, 2, 3}
}

// focusIsMultiline reports whether the focused field uses a multi-line textarea.
func (am AddUpstreamModalState) focusIsMultiline() bool {
	return am.Tab == 1 && (am.Active == 2 || am.Active == 3)
}

func (am *AddUpstreamModalState) cycleFocus(back bool) {
	groups := am.validFieldGroups()
	idx := 0
	for i, g := range groups {
		if g == am.Active {
			idx = i
			break
		}
	}
	if back {
		idx = (idx - 1 + len(groups)) % len(groups)
	} else {
		idx = (idx + 1) % len(groups)
	}
	am.Active = groups[idx]
}

// RenderAddUpstreamModal renders the modal for adding an upstream source.
func RenderAddUpstreamModal(state *AddUpstreamModalState, box lipgloss.Style, err error) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A29BFE")).Render(i18n.T("modal.upstream.add.title")) + "\n\n")
	b.WriteString(renderUpstreamTabBar(state.Tab, state.Active == 0) + "\n\n")

	if state.Tab == 0 {
		b.WriteString(upstreamFieldLabel(i18n.T("modal.upstream.add.url_label"), state.Active == 1) + "\n")
		b.WriteString(state.URLInput.View() + "\n\n")

		b.WriteString(upstreamFieldLabel(i18n.T("modal.upstream.add.ref_label"), state.Active == 2) + "\n")
		b.WriteString(state.RefInput.View() + "\n\n")

		b.WriteString(upstreamFieldLabel(i18n.T("modal.upstream.add.name_label"), state.Active == 3) + "\n")
		b.WriteString(state.NameInput.View() + "\n\n")
	} else {
		b.WriteString(renderUpstreamScanForm(&state.Scan, state.Active) + "\n\n")
	}

	if err != nil {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D63031")).Render("❌ "+err.Error()) + "\n\n")
	}

	b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.upstream.add.hint")))
	return box.Render(b.String())
}

// EditUpstreamModalState holds state for editing an upstream source configuration.
type EditUpstreamModalState struct {
	URL       string
	RefInput  textinput.Model
	NameInput textinput.Model
	Scan      upstreamScanForm
	Tab       int
	Active    int
}

func newEditUpstreamModal(url, ref, name string, scan model.ScanConfig) EditUpstreamModalState {
	r := textinput.New()
	r.SetValue(ref)
	r.Placeholder = i18n.T("modal.upstream.ref_placeholder_edit")
	r.CharLimit = 128
	r.Width = 60
	r.Focus()

	n := textinput.New()
	n.SetValue(name)
	n.Placeholder = i18n.T("modal.upstream.alias_placeholder")
	n.CharLimit = 64
	n.Width = 60

	return EditUpstreamModalState{
		URL:       url,
		RefInput:  r,
		NameInput: n,
		Scan:      newUpstreamScanForm(scan),
		Tab:       0,
		Active:    1,
	}
}

func (em EditUpstreamModalState) validFieldGroups() []int {
	if em.Tab == 1 && em.Scan.Mode != upstreamScanModeCustom {
		return []int{0, 1, 3}
	}
	return []int{0, 1, 2, 3}
}

// focusIsMultiline reports whether the focused field uses a multi-line textarea.
func (em EditUpstreamModalState) focusIsMultiline() bool {
	return em.Tab == 1 && (em.Active == 2 || em.Active == 3)
}

func (em *EditUpstreamModalState) cycleFocus(back bool) {
	groups := em.validFieldGroups()
	idx := 0
	for i, g := range groups {
		if g == em.Active {
			idx = i
			break
		}
	}
	if back {
		idx = (idx - 1 + len(groups)) % len(groups)
	} else {
		idx = (idx + 1) % len(groups)
	}
	em.Active = groups[idx]
}

// RenderEditUpstreamModal renders the modal for editing an upstream source.
func RenderEditUpstreamModal(state *EditUpstreamModalState, box lipgloss.Style, err error) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A29BFE")).Render(i18n.T("modal.upstream.edit.title")) + "\n\n")
	b.WriteString(renderUpstreamTabBar(state.Tab, state.Active == 0) + "\n\n")
	b.WriteString(i18n.T("modal.upstream.edit.url_label") + lipgloss.NewStyle().Foreground(lipgloss.Color("#DFE6E9")).Bold(true).Render(state.URL) + "\n\n")

	if state.Tab == 0 {
		b.WriteString(upstreamFieldLabel(i18n.T("modal.upstream.edit.ref_label"), state.Active == 1) + "\n")
		b.WriteString(state.RefInput.View() + "\n\n")

		b.WriteString(upstreamFieldLabel(i18n.T("modal.upstream.edit.name_label"), state.Active == 2) + "\n")
		b.WriteString(state.NameInput.View() + "\n\n")
	} else {
		b.WriteString(renderUpstreamScanForm(&state.Scan, state.Active) + "\n\n")
	}

	if err != nil {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D63031")).Render("❌ "+err.Error()) + "\n\n")
	}

	b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.upstream.edit.hint")))
	return box.Render(b.String())
}

// ConfirmUpstreamRemoveModalState holds state for confirming upstream removal.
type ConfirmUpstreamRemoveModalState struct {
	URL          string
	LinkedSkills []string
}

func newConfirmUpstreamRemoveModal(url string, linkedSkills []string) ConfirmUpstreamRemoveModalState {
	return ConfirmUpstreamRemoveModalState{
		URL:          url,
		LinkedSkills: linkedSkills,
	}
}

// RenderConfirmUpstreamRemoveModal renders the modal for confirming upstream removal.
func RenderConfirmUpstreamRemoveModal(state *ConfirmUpstreamRemoveModalState, box lipgloss.Style) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF7675")).Render(i18n.T("modal.upstream.remove.title")) + "\n\n")

	b.WriteString(i18n.T("modal.upstream.remove.url_label") + lipgloss.NewStyle().Bold(true).Render(state.URL) + "\n\n")

	if len(state.LinkedSkills) > 0 {
		b.WriteString(i18n.T("modal.upstream.remove.linked", len(state.LinkedSkills)))
		for _, s := range state.LinkedSkills {
			b.WriteString(fmt.Sprintf("  • %s\n", s))
		}
		b.WriteString(i18n.T("modal.upstream.remove.choose"))
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FDCB6E")).Render(i18n.T("modal.upstream.remove.opt_keep")) + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#D63031")).Render(i18n.T("modal.upstream.remove.opt_delete")) + "\n")
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.upstream.remove.cancel")) + "\n")
	} else {
		b.WriteString(i18n.T("modal.upstream.remove.none"))
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FDCB6E")).Render(i18n.T("modal.upstream.remove.confirm")) + "\n")
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.upstream.remove.cancel")) + "\n")
	}

	return box.Render(b.String())
}

// UpstreamDetailModalState holds state for viewing upstream source details.
type UpstreamDetailModalState struct {
	Upstream model.UpstreamInfo
	Viewport viewport.Model
	Ready    bool
}

func newUpstreamDetailModal(u model.UpstreamInfo) UpstreamDetailModalState {
	vp := viewport.New(70, 10)
	vp.GotoTop()
	return UpstreamDetailModalState{
		Upstream: u,
		Viewport: vp,
		Ready:    true,
	}
}

// RenderUpstreamDetailModal renders comprehensive details for an upstream source.
func RenderUpstreamDetailModal(state *UpstreamDetailModalState, box lipgloss.Style, termWidth, termHeight int) string {
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
	// Max outer height must leave at least 3 rows (tabs bar at rows 0-1, plus 1 row margin)
	maxOuterHeight := termHeight - 3
	if maxOuterHeight < 8 {
		if termHeight > 0 && termHeight < 8 {
			maxOuterHeight = termHeight
		} else {
			maxOuterHeight = 8
		}
	}

	maxInnerHeight := maxOuterHeight - box.GetVerticalFrameSize()
	if maxInnerHeight < 4 {
		maxInnerHeight = 4
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A29BFE"))

	u := state.Upstream
	titleText := i18n.T("modal.upstream_detail.title")
	if u.Name != "" {
		titleText = fmt.Sprintf("%s (%s)", titleText, u.Name)
	}
	fullTitle := "📦 " + titleText
	if lipgloss.Width(fullTitle) > contentWidth {
		fullTitle = truncate.StringWithTail(fullTitle, uint(contentWidth), "…")
	}
	headerStr := titleStyle.Render(fullTitle) + "\n\n"
	headerLines := lipgloss.Height(headerStr)

	// Build the scrollable body content
	bodyContent := buildUpstreamDetailBody(u, contentWidth)
	bodyHeight := lipgloss.Height(bodyContent)

	// Tentative footer to check if scrolling is needed
	tentativeFooter := renderDetailFooter(i18n.T("modal.upstream_detail.footer"), contentWidth)
	tentativeFooterLines := lipgloss.Height(tentativeFooter) + 1
	availVpH := maxInnerHeight - headerLines - tentativeFooterLines
	if availVpH < 3 {
		availVpH = 3
	}

	isScrollable := bodyHeight > availVpH

	var footerText string
	if isScrollable {
		var scrollBadge string
		if state.Viewport.AtTop() {
			scrollBadge = "[TOP]"
		} else if state.Viewport.AtBottom() {
			scrollBadge = "[END]"
		} else {
			scrollBadge = fmt.Sprintf("[%2.0f%%]", state.Viewport.ScrollPercent()*100)
		}
		scrollHint := i18n.T("modal.upstream_detail.scroll_hint")
		footerText = fmt.Sprintf("%s %s • %s", scrollBadge, scrollHint, i18n.T("modal.upstream_detail.footer"))
	} else {
		footerText = i18n.T("modal.upstream_detail.footer")
	}

	footerRendered := renderDetailFooter(footerText, contentWidth)
	actualFooterLines := lipgloss.Height(footerRendered) + 1

	finalMaxVpH := maxInnerHeight - headerLines - actualFooterLines
	if finalMaxVpH < 3 {
		finalMaxVpH = 3
	}

	vpH := bodyHeight
	if vpH > finalMaxVpH {
		vpH = finalMaxVpH
	}

	if !state.Ready || state.Viewport.Width != contentWidth || state.Viewport.Height != vpH {
		currOffset := state.Viewport.YOffset
		state.Viewport.Width = contentWidth
		state.Viewport.Height = vpH
		state.Viewport.SetContent(bodyContent)
		state.Viewport.YOffset = currOffset
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

func buildUpstreamDetailBody(u model.UpstreamInfo, contentWidth int) string {
	var b strings.Builder

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#55EFC4"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#B2BEC3"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#74B9FF"))
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00B894")).Bold(true)
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FAB1A0"))

	// 1. Basic info
	b.WriteString(sectionStyle.Render(i18n.T("modal.upstream_detail.sec_basic")) + "\n")
	if u.Name != "" {
		renderDetailField(&b, i18n.T("modal.upstream_detail.name"), u.Name, labelStyle, valueStyle, contentWidth)
	}
	renderDetailField(&b, i18n.T("modal.upstream_detail.type"), string(u.Type), labelStyle, valueStyle, contentWidth)
	renderDetailField(&b, i18n.T("modal.upstream_detail.url"), u.URL, labelStyle, pathStyle, contentWidth)

	refStr := u.Ref
	if refStr == "" {
		refStr = "-"
	}
	renderDetailField(&b, i18n.T("modal.upstream_detail.ref"), refStr, labelStyle, valueStyle, contentWidth)

	var statusBadge string
	switch u.Status {
	case model.UpstreamUpToDate:
		statusBadge = okStyle.Render("✓ " + i18n.T("upstream.status.up_to_date"))
	case model.UpstreamUpdateAvailable:
		statusBadge = lipgloss.NewStyle().Foreground(lipgloss.Color("#FDCB6E")).Bold(true).Render("⬆ " + i18n.T("upstream.status.update_available"))
	case model.UpstreamSourceChanged:
		statusBadge = warnStyle.Render("! " + i18n.T("upstream.status.source_changed"))
	case model.UpstreamUnreachable:
		statusBadge = lipgloss.NewStyle().Foreground(lipgloss.Color("#D63031")).Bold(true).Render("✗ " + i18n.T("upstream.status.unreachable"))
	case model.UpstreamInvalid:
		statusBadge = lipgloss.NewStyle().Foreground(lipgloss.Color("#D63031")).Bold(true).Render("✗ " + i18n.T("upstream.status.invalid"))
	default:
		statusBadge = valueStyle.Render("-")
	}
	b.WriteString(fmt.Sprintf("  • %s %s\n", labelStyle.Render(i18n.T("modal.upstream_detail.status")+":"), statusBadge))

	createdStr := u.CreatedAt
	if createdStr == "" {
		createdStr = "-"
	}
	b.WriteString(fmt.Sprintf("  • %s %s\n", labelStyle.Render(i18n.T("modal.upstream_detail.created_at")+":"), valueStyle.Render(createdStr)))

	updatedStr := u.UpdatedAt
	if updatedStr == "" {
		updatedStr = "-"
	}
	b.WriteString(fmt.Sprintf("  • %s %s\n\n", labelStyle.Render(i18n.T("modal.upstream_detail.updated_at")+":"), valueStyle.Render(updatedStr)))

	// 2. Local cache & version
	b.WriteString(sectionStyle.Render(i18n.T("modal.upstream_detail.sec_cache")) + "\n")
	cachePathStr := u.CachePath
	if cachePathStr == "" {
		cachePathStr = "-"
	} else {
		cachePathStr = fsx.CompactUser(cachePathStr)
	}
	renderDetailField(&b, i18n.T("modal.upstream_detail.cache_path"), cachePathStr, labelStyle, pathStyle, contentWidth)

	var cacheStatusBadge string
	if u.CacheExists {
		cacheStatusBadge = okStyle.Render("✓ " + i18n.T("modal.upstream_detail.cache_cached"))
	} else {
		cacheStatusBadge = warnStyle.Render("✗ " + i18n.T("modal.upstream_detail.cache_uncached"))
	}
	b.WriteString(fmt.Sprintf("  • %s %s\n", labelStyle.Render(i18n.T("modal.upstream_detail.cache_status")+":"), cacheStatusBadge))

	if u.Commit != "" {
		renderDetailField(&b, i18n.T("modal.upstream_detail.commit"), u.Commit, labelStyle, valueStyle, contentWidth)
	}
	if u.NewCommit != "" {
		newCommitStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FDCB6E")).Bold(true)
		renderDetailField(&b, i18n.T("modal.upstream_detail.new_commit"), u.NewCommit, labelStyle, newCommitStyle, contentWidth)
	}
	b.WriteString("\n")

	// 3. Skill scan scope
	b.WriteString(sectionStyle.Render(i18n.T("modal.upstream_detail.sec_scan")) + "\n")
	renderDetailField(&b, i18n.T("modal.upstream_detail.scan_mode"), i18n.RenderScanScope(u.ScanRoots, u.ScanExclude), labelStyle, valueStyle, contentWidth)
	if scanNorm := (model.ScanConfig{Roots: u.ScanRoots, Exclude: u.ScanExclude}).Normalized(); len(scanNorm.Exclude) > 0 {
		renderDetailField(&b, i18n.T("modal.upstream_detail.scan_exclude"), strings.Join(scanNorm.Exclude, ", "), labelStyle, pathStyle, contentWidth)
	}
	b.WriteString("\n")

	// 4. Skills overview
	b.WriteString(sectionStyle.Render(i18n.T("modal.upstream_detail.sec_skills")) + "\n")
	var introText string
	if u.ScanError != "" {
		introText = i18n.T("error.upstream_scan", u.URL, u.ScanError)
	} else if u.Scanned || u.CacheExists {
		introText = fmt.Sprintf(i18n.T("modal.upstream_detail.skills_summary_intro"), len(u.AvailableSkills), len(u.Skills))
	} else {
		introText = fmt.Sprintf(i18n.T("modal.upstream_detail.skills_uncached_intro"), len(u.Skills))
	}
	introStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#DFE6E9"))
	b.WriteString(wrapIndentedText(introText, "  ", contentWidth, introStyle))

	if len(u.AvailableSkills) > 0 {
		b.WriteString(renderDetailSkillList(i18n.T("modal.upstream_detail.avail_skills"), len(u.AvailableSkills), u.AvailableSkills, 0, valueStyle, labelStyle, contentWidth))
	} else if u.Scanned || (u.CacheExists && u.ScanError == "") {
		b.WriteString(fmt.Sprintf("  • %s: %s\n",
			labelStyle.Render(i18n.T("modal.upstream_detail.avail_skills")),
			lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.upstream_detail.none")),
		))
	} else {
		b.WriteString(fmt.Sprintf("  • %s: %s\n",
			labelStyle.Render(i18n.T("modal.upstream_detail.avail_skills")),
			lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.upstream_detail.cache_uncached")),
		))
	}

	if len(u.Skills) > 0 {
		b.WriteString(renderDetailSkillList(i18n.T("modal.upstream_detail.managed_skills"), len(u.Skills), u.Skills, 0, okStyle, labelStyle, contentWidth))
	} else {
		b.WriteString(fmt.Sprintf("  • %s: %s\n",
			labelStyle.Render(i18n.T("modal.upstream_detail.managed_skills")),
			lipgloss.NewStyle().Faint(true).Render(i18n.T("modal.upstream_detail.none")),
		))
	}

	return strings.TrimRight(b.String(), "\n")
}

func renderDetailField(b *strings.Builder, label, val string, labelStyle, valStyle lipgloss.Style, contentWidth int) {
	if val == "" {
		val = "-"
	}
	prefix := fmt.Sprintf("  • %s: ", label)
	styledLabel := labelStyle.Render(label + ":")
	if lipgloss.Width(prefix)+lipgloss.Width(val) <= contentWidth {
		b.WriteString(fmt.Sprintf("  • %s %s\n", styledLabel, valStyle.Render(val)))
		return
	}
	b.WriteString(fmt.Sprintf("  • %s\n", styledLabel))
	b.WriteString(wrapIndentedText(val, "      ", contentWidth, valStyle))
}

func wrapIndentedText(text, indent string, maxW int, style lipgloss.Style) string {
	if maxW <= 10 {
		maxW = 10
	}
	availW := maxW - lipgloss.Width(indent)
	if availW <= 10 {
		availW = 10
	}
	wrapped := cellbuf.Wrap(text, availW, "")
	lines := strings.Split(wrapped, "\n")
	var b strings.Builder
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		b.WriteString(indent)
		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}
	return b.String()
}

func renderDetailSkillList(label string, count int, skills []string, maxShow int, style, labelStyle lipgloss.Style, contentWidth int) string {
	if maxShow <= 0 {
		maxShow = 20
	}
	items := skills
	more := 0
	if len(skills) > maxShow {
		items = skills[:maxShow]
		more = len(skills) - maxShow
	}

	prefix := fmt.Sprintf("  • %s (%d): ", label, count)
	listStr := strings.Join(items, ", ")
	if more > 0 {
		listStr += fmt.Sprintf(" ... (+%d)", more)
	}

	if lipgloss.Width(prefix)+lipgloss.Width(listStr) <= contentWidth {
		return fmt.Sprintf("  • %s (%d): %s\n", labelStyle.Render(label), count, style.Render(listStr))
	}

	var lines []string
	var curLine []string
	curW := 0
	commaW := lipgloss.Width(", ")
	availWidth := contentWidth - 6
	if availWidth < 20 {
		availWidth = 20
	}

	for i, item := range items {
		itemW := lipgloss.Width(item)
		addW := itemW
		if len(curLine) > 0 {
			addW += commaW
		}

		extraW := 0
		if i == len(items)-1 && more > 0 {
			extraW = lipgloss.Width(fmt.Sprintf(" ... (+%d)", more))
		}

		if len(curLine) == 0 || curW+addW+extraW <= availWidth {
			curLine = append(curLine, item)
			curW += addW
		} else {
			lines = append(lines, strings.Join(curLine, ", "))
			curLine = []string{item}
			curW = itemW
		}
	}
	if len(curLine) > 0 {
		lastStr := strings.Join(curLine, ", ")
		if more > 0 {
			lastStr += fmt.Sprintf(" ... (+%d)", more)
		}
		lines = append(lines, lastStr)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("  • %s (%d):\n", labelStyle.Render(label), count))
	for _, l := range lines {
		b.WriteString("      " + style.Render(l) + "\n")
	}
	return b.String()
}

func renderDetailFooter(footer string, maxWidth int) string {
	parts := strings.Split(footer, " • ")
	if len(parts) <= 1 {
		return lipgloss.NewStyle().Faint(true).Render(footer)
	}

	var lines []string
	var curLine []string
	curW := 0
	sepW := lipgloss.Width(hintSeparator)

	for _, part := range parts {
		partW := lipgloss.Width(part)
		if len(curLine) == 0 {
			curLine = append(curLine, part)
			curW = partW
			continue
		}
		if curW+sepW+partW <= maxWidth {
			curLine = append(curLine, part)
			curW += sepW + partW
		} else {
			lines = append(lines, strings.Join(curLine, hintSeparator))
			curLine = []string{part}
			curW = partW
		}
	}
	if len(curLine) > 0 {
		lines = append(lines, strings.Join(curLine, hintSeparator))
	}

	styledLines := make([]string, len(lines))
	faint := lipgloss.NewStyle().Faint(true)
	for i, l := range lines {
		styledLines[i] = faint.Render(l)
	}
	return strings.Join(styledLines, "\n")
}

// ModelDetailModalState holds state for viewing model item details.
type ModelDetailModalState struct {
	Item     app.AgentModelItem
	Viewport viewport.Model
	Ready    bool
}

func newModelDetailModal(item app.AgentModelItem) ModelDetailModalState {
	vp := viewport.New(70, 10)
	vp.GotoTop()
	return ModelDetailModalState{
		Item:     item,
		Viewport: vp,
		Ready:    true,
	}
}

// RenderModelDetailModal renders comprehensive details for an agent model item.
func RenderModelDetailModal(state *ModelDetailModalState, box lipgloss.Style, termWidth, termHeight int) string {
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
		if termHeight > 0 && termHeight < 8 {
			maxOuterHeight = termHeight
		} else {
			maxOuterHeight = 8
		}
	}

	maxInnerHeight := maxOuterHeight - box.GetVerticalFrameSize()
	if maxInnerHeight < 4 {
		maxInnerHeight = 4
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A29BFE"))

	it := state.Item
	titleText := i18n.T("modal.model_detail.title")
	if it.ModelID != "" {
		titleText = fmt.Sprintf("%s (%s)", titleText, it.ModelID)
	}
	fullTitle := "🤖 " + titleText
	if lipgloss.Width(fullTitle) > contentWidth {
		fullTitle = truncate.StringWithTail(fullTitle, uint(contentWidth), "…")
	}
	headerStr := titleStyle.Render(fullTitle) + "\n\n"
	headerLines := lipgloss.Height(headerStr)

	bodyContent := buildModelDetailBody(it, contentWidth)
	bodyHeight := lipgloss.Height(bodyContent)

	tentativeFooter := renderDetailFooter(i18n.T("modal.model_detail.footer"), contentWidth)
	tentativeFooterLines := lipgloss.Height(tentativeFooter) + 1
	availVpH := maxInnerHeight - headerLines - tentativeFooterLines
	if availVpH < 3 {
		availVpH = 3
	}

	isScrollable := bodyHeight > availVpH

	var footerText string
	if isScrollable {
		var scrollBadge string
		if state.Viewport.AtTop() {
			scrollBadge = "[TOP]"
		} else if state.Viewport.AtBottom() {
			scrollBadge = "[END]"
		} else {
			scrollBadge = fmt.Sprintf("[%2.0f%%]", state.Viewport.ScrollPercent()*100)
		}
		scrollHint := i18n.T("modal.model_detail.scroll_hint")
		footerText = fmt.Sprintf("%s %s • %s", scrollBadge, scrollHint, i18n.T("modal.model_detail.footer"))
	} else {
		footerText = i18n.T("modal.model_detail.footer")
	}

	footerRendered := renderDetailFooter(footerText, contentWidth)
	actualFooterLines := lipgloss.Height(footerRendered) + 1

	finalMaxVpH := maxInnerHeight - headerLines - actualFooterLines
	if finalMaxVpH < 3 {
		finalMaxVpH = 3
	}

	vpH := bodyHeight
	if vpH > finalMaxVpH {
		vpH = finalMaxVpH
	}

	if !state.Ready || state.Viewport.Width != contentWidth || state.Viewport.Height != vpH {
		currOffset := state.Viewport.YOffset
		state.Viewport.Width = contentWidth
		state.Viewport.Height = vpH
		state.Viewport.SetContent(bodyContent)
		state.Viewport.YOffset = currOffset
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

func buildModelDetailBody(it app.AgentModelItem, contentWidth int) string {
	var b strings.Builder

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#55EFC4"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#B2BEC3"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#74B9FF"))
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00B894")).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#636E72"))

	// 1. 基本信息
	b.WriteString(sectionStyle.Render(i18n.T("modal.model_detail.sec_basic")) + "\n")
	renderDetailField(&b, i18n.T("modal.model_detail.id"), it.ModelID, labelStyle, valueStyle, contentWidth)
	if it.Name != "" {
		renderDetailField(&b, i18n.T("modal.model_detail.name"), it.Name, labelStyle, valueStyle, contentWidth)
	}
	renderDetailField(&b, i18n.T("modal.model_detail.channel"), it.ProviderID, labelStyle, pathStyle, contentWidth)
	if it.SourceProvider != "" {
		renderDetailField(&b, i18n.T("modal.model_detail.source_provider"), it.SourceProvider, labelStyle, valueStyle, contentWidth)
	}
	var statusBadge string
	if it.IsEnriched {
		statusBadge = okStyle.Render("✓ " + i18n.T("model.status.enriched"))
	} else {
		statusBadge = lipgloss.NewStyle().Foreground(lipgloss.Color("#FDCB6E")).Bold(true).Render("⏳ " + i18n.T("model.status.pending"))
	}
	renderDetailField(&b, i18n.T("modal.model_detail.status"), statusBadge, labelStyle, valueStyle, contentWidth)
	b.WriteString("\n")

	// 2. 上下文与令牌限制
	b.WriteString(sectionStyle.Render(i18n.T("modal.model_detail.sec_limits")) + "\n")
	ctxStr := "-"
	if it.ContextLimit > 0 {
		ctxStr = fmt.Sprintf("%d tokens (%dk)", it.ContextLimit, it.ContextLimit/1000)
	}
	renderDetailField(&b, i18n.T("modal.model_detail.context_limit"), ctxStr, labelStyle, valueStyle, contentWidth)

	if it.InputLimit > 0 {
		inStr := fmt.Sprintf("%d tokens (%dk)", it.InputLimit, it.InputLimit/1000)
		renderDetailField(&b, i18n.T("modal.model_detail.input_limit"), inStr, labelStyle, valueStyle, contentWidth)
	}
	outStr := "-"
	if it.OutputLimit > 0 {
		outStr = fmt.Sprintf("%d tokens (%dk)", it.OutputLimit, it.OutputLimit/1000)
	}
	renderDetailField(&b, i18n.T("modal.model_detail.output_limit"), outStr, labelStyle, valueStyle, contentWidth)
	b.WriteString("\n")

	// 3. 价格与计费 (每 1M tokens)
	b.WriteString(sectionStyle.Render(i18n.T("modal.model_detail.sec_pricing")) + "\n")
	if it.PriceInput == 0 && it.PriceOutput == 0 && it.PriceCacheRead == 0 && it.PriceCacheWrite == 0 {
		renderDetailField(&b, i18n.T("modal.model_detail.price_input"), i18n.T("modal.model_detail.free"), labelStyle, dimStyle, contentWidth)
	} else {
		renderDetailField(&b, i18n.T("modal.model_detail.price_input"), fmt.Sprintf("$%g / 1M tokens", it.PriceInput), labelStyle, valueStyle, contentWidth)
		renderDetailField(&b, i18n.T("modal.model_detail.price_output"), fmt.Sprintf("$%g / 1M tokens", it.PriceOutput), labelStyle, valueStyle, contentWidth)
		if it.PriceCacheRead > 0 {
			renderDetailField(&b, i18n.T("modal.model_detail.price_cache_read"), fmt.Sprintf("$%g / 1M tokens", it.PriceCacheRead), labelStyle, valueStyle, contentWidth)
		}
		if it.PriceCacheWrite > 0 {
			renderDetailField(&b, i18n.T("modal.model_detail.price_cache_write"), fmt.Sprintf("$%g / 1M tokens", it.PriceCacheWrite), labelStyle, valueStyle, contentWidth)
		}
	}
	b.WriteString("\n")

	// 4. 支持能力与特性
	b.WriteString(sectionStyle.Render(i18n.T("modal.model_detail.sec_caps")) + "\n")
	formatCap := func(supported bool) string {
		if supported {
			return okStyle.Render("✓ " + i18n.T("model.cap.supported"))
		}
		return dimStyle.Render("✕ " + i18n.T("model.cap.unsupported"))
	}
	renderDetailField(&b, i18n.T("modal.model_detail.cap_reasoning"), formatCap(it.Reasoning), labelStyle, valueStyle, contentWidth)
	renderDetailField(&b, i18n.T("modal.model_detail.cap_tool_call"), formatCap(it.ToolCall), labelStyle, valueStyle, contentWidth)
	renderDetailField(&b, i18n.T("modal.model_detail.cap_attachment"), formatCap(it.Attachment), labelStyle, valueStyle, contentWidth)
	renderDetailField(&b, i18n.T("modal.model_detail.cap_structured"), formatCap(it.StructuredOutput), labelStyle, valueStyle, contentWidth)
	renderDetailField(&b, i18n.T("modal.model_detail.cap_temperature"), formatCap(it.Temperature), labelStyle, valueStyle, contentWidth)

	if len(it.ModalitiesInput) > 0 {
		renderDetailField(&b, i18n.T("modal.model_detail.modalities_input"), strings.Join(it.ModalitiesInput, ", "), labelStyle, valueStyle, contentWidth)
	}
	if len(it.ModalitiesOutput) > 0 {
		renderDetailField(&b, i18n.T("modal.model_detail.modalities_output"), strings.Join(it.ModalitiesOutput, ", "), labelStyle, valueStyle, contentWidth)
	}

	// 5. 变体
	if len(it.Variants) > 0 {
		b.WriteString("\n" + sectionStyle.Render(i18n.T("modal.model_detail.sec_variants")) + "\n")
		for _, v := range it.Variants {
			b.WriteString("  • " + valueStyle.Render(v) + "\n")
		}
	}

	// 6. 原始 JSON 配置
	if it.RawJSON != "" {
		b.WriteString("\n" + sectionStyle.Render(i18n.T("modal.model_detail.sec_raw_json")) + "\n")
		jsonStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#DFE6E9"))
		rawLines := strings.Split(it.RawJSON, "\n")
		for _, line := range rawLines {
			b.WriteString("  " + jsonStyle.Render(line) + "\n")
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

// ModelConfigFileModalState holds state for viewing full model configuration file.
type ModelConfigFileModalState struct {
	FilePath string
	Content  string
	Err      error
	Viewport viewport.Model
	Ready    bool
}

func newModelConfigFileModal(filePath string) ModelConfigFileModalState {
	vp := viewport.New(76, 12)
	vp.GotoTop()
	state := ModelConfigFileModalState{
		FilePath: filePath,
		Viewport: vp,
		Ready:    true,
	}
	if filePath == "" {
		state.Content = i18n.T("modal.model_config.empty")
		return state
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		state.Err = err
		state.Content = i18n.T("modal.model_config.read_failed", err)
		return state
	}
	lines := strings.Split(string(data), "\n")
	numLines := len(lines)
	numWidth := len(fmt.Sprintf("%d", numLines))
	if numWidth < 2 {
		numWidth = 2
	}
	numStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#636E72"))
	var sb strings.Builder
	for i, line := range lines {
		lineNum := fmt.Sprintf("%*d", numWidth, i+1)
		sb.WriteString(numStyle.Render(lineNum + " │ "))
		sb.WriteString(line)
		if i < numLines-1 {
			sb.WriteByte('\n')
		}
	}
	state.Content = sb.String()
	return state
}

// RenderModelConfigFileModal renders the full model configuration file modal with line numbers.
func RenderModelConfigFileModal(state *ModelConfigFileModalState, box lipgloss.Style, termWidth, termHeight int) string {
	maxOuterWidth := 80
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
		if termHeight > 0 && termHeight < 8 {
			maxOuterHeight = termHeight
		} else {
			maxOuterHeight = 8
		}
	}

	maxInnerHeight := maxOuterHeight - box.GetVerticalFrameSize()
	if maxInnerHeight < 4 {
		maxInnerHeight = 4
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A29BFE"))
	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#74B9FF"))
	statsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#B2BEC3"))

	baseName := filepath.Base(state.FilePath)
	if baseName == "" || baseName == "." {
		baseName = "opencode.json"
	}
	fullTitle := fmt.Sprintf("📄 %s (%s)", i18n.T("modal.model_config.title"), baseName)
	if lipgloss.Width(fullTitle) > contentWidth {
		fullTitle = truncate.StringWithTail(fullTitle, uint(contentWidth), "…")
	}
	headerStr := titleStyle.Render(fullTitle) + "\n"

	// File info subheader
	var infoStr string
	if state.FilePath != "" {
		infoStr = pathStyle.Render(state.FilePath)
		if fi, err := os.Stat(state.FilePath); err == nil {
			sizeKB := float64(fi.Size()) / 1024.0
			linesCount := strings.Count(state.Content, "\n") + 1
			infoStr += statsStyle.Render(i18n.T("modal.model_config.file_stats", sizeKB, linesCount))
		}
		if lipgloss.Width(infoStr) > contentWidth {
			infoStr = truncate.StringWithTail(infoStr, uint(contentWidth), "…")
		}
	}
	if infoStr != "" {
		headerStr += infoStr + "\n\n"
	} else {
		headerStr += "\n"
	}
	headerLines := lipgloss.Height(headerStr)

	bodyContent := state.Content
	bodyHeight := lipgloss.Height(bodyContent)

	tentativeFooter := renderDetailFooter(i18n.T("modal.model_config.footer"), contentWidth)
	tentativeFooterLines := lipgloss.Height(tentativeFooter) + 1
	availVpH := maxInnerHeight - headerLines - tentativeFooterLines
	if availVpH < 3 {
		availVpH = 3
	}

	isScrollable := bodyHeight > availVpH

	var footerText string
	if isScrollable {
		var scrollBadge string
		if state.Viewport.AtTop() {
			scrollBadge = "[TOP]"
		} else if state.Viewport.AtBottom() {
			scrollBadge = "[END]"
		} else {
			scrollBadge = fmt.Sprintf("[%2.0f%%]", state.Viewport.ScrollPercent()*100)
		}
		scrollHint := i18n.T("modal.model_config.scroll_hint")
		footerText = fmt.Sprintf("%s %s • %s", scrollBadge, scrollHint, i18n.T("modal.model_config.footer"))
	} else {
		footerText = i18n.T("modal.model_config.footer")
	}

	footerRendered := renderDetailFooter(footerText, contentWidth)
	actualFooterLines := lipgloss.Height(footerRendered) + 1

	finalMaxVpH := maxInnerHeight - headerLines - actualFooterLines
	if finalMaxVpH < 3 {
		finalMaxVpH = 3
	}

	vpH := bodyHeight
	if vpH > finalMaxVpH {
		vpH = finalMaxVpH
	}

	if !state.Ready || state.Viewport.Width != contentWidth || state.Viewport.Height != vpH {
		currOffset := state.Viewport.YOffset
		state.Viewport.Width = contentWidth
		state.Viewport.Height = vpH
		state.Viewport.SetContent(bodyContent)
		state.Viewport.YOffset = currOffset
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
