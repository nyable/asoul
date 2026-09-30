package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"asoul/internal/agent"
	"asoul/internal/app"
	"asoul/internal/cache"
	"asoul/internal/catalog"
	"asoul/internal/config"
	"asoul/internal/deploy"
	"asoul/internal/diffview"
	"asoul/internal/fsx"
	"asoul/internal/i18n"
	"asoul/internal/model"
	"asoul/internal/progress"
	"asoul/internal/skill"
	"asoul/internal/source/git"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/cellbuf"
	"github.com/muesli/reflow/truncate"
)

type activeTab int

const (
	tabDashboard activeTab = iota
	tabUpstreams
	tabSkills
	tabChannels
	tabProjects
	tabSystem
	tabSettings
)

type currentView int

const (
	viewList currentView = iota
	viewDetail
	viewDiff
	viewDeploy
	viewUndeploy
	viewHelp
	viewModalAdd
	viewModalNew
	viewModalAddTarget
	viewModalAddProject
	viewModalAddGroup
	viewModalGroupSkills
	viewModalConfirm
	viewModalBatchRemove
	viewSearch
	viewModalEditSetting
	viewModalSelectLanguage
	viewModalSelectWorkspace
	viewModalAddWorkspace
	viewModalTargetDetail
	viewModalDeployConflict
	viewTargetDiff
	viewModalAddUpstream
	viewModalEditUpstream
	viewModalConfirmUpstreamRemove
	viewModalUpstreamDetail
	viewModalModelDetail
	viewModalModelConfigFile
	viewModalProviderDetail
	viewModalEditField
	viewModalEditJSON
)

type skillSubTab int

const (
	skillTabSkills skillSubTab = iota
	skillTabGroups
)

type systemSubTab int

const (
	systemTabCache systemSubTab = iota
	systemTabDoctor
)

type targetFilterMode int

const (
	targetFilterEnabledOnly targetFilterMode = iota
	targetFilterDisabledOnly
	targetFilterAll
)

type agentTargetItem struct {
	Name        string
	Channel     string
	ConfigDir   string
	ResolvedDir string
	SkillsDir   string
	Paths       map[string]string
	Features    string
	SkillCount  int
	Enabled     bool
}

type projectTargetItem struct {
	Name       string
	Path       string
	RawPath    string
	Formats    string
	SkillCount int
	Enabled    bool
}

// channelEntry couples a configured agent deployment target with its optional
// model-config capability. One entry per configured agent target (names are
// unique); paths may repeat.
type channelEntry struct {
	Name      string
	Channel   string
	Target    agentTargetItem
	Supported bool
}

// Async messages
type reloadStatusMsg []model.SkillStatus
type upstreamReloadMsg []model.UpstreamInfo
type asyncNoticeMsg string
type discoveredSkillsMsg struct {
	url    string
	ref    string
	skills []git.DiscoveredSkill
}
type doctorReportMsg *app.DoctorReport
type cacheListMsg []cache.CacheEntry
type reloadCompleteMsg struct{ snapshot *Model }
type configChangedMsg struct{}
type targetToggledMsg struct {
	enabled bool
	notice  string
}
type targetDetailMsg struct {
	modal TargetDetailModalState
}
type targetDetailSkillsMsg []string
type progressMsg struct {
	phase   progress.Phase
	current int
	total   int
	detail  string
}

type progressTickMsg time.Time

type diffLoadedMsg string
type deployConflictMsg struct {
	Conflict      *deploy.TargetConflictError
	Queue         []*deploy.TargetConflictError
	DeployedCount int
	TotalCount    int
}
type targetDiffLoadedMsg struct {
	diff string
	err  error
}
type modelsEnrichDryRunMsg struct {
	summary *agent.EnrichSummary
}

// pendingConfigEdit describes a single agent-config field change awaiting
// confirmation in the diff view.
type pendingConfigEdit struct {
	channel      string
	cfgFile      string
	fieldPath    []string
	value        any
	remove       bool
	originalHash string
	candidate    []byte
}

type configEditPreviewMsg struct {
	edit *app.AgentConfigEdit
}

type errMsg struct{ err error }

func (e errMsg) Error() string { return e.err.Error() }

type settingItem struct {
	Key         string
	Title       string
	Value       string
	Description string
	ActionHint  string
}

// Model represents the Bubble Tea TUI state.
type Model struct {
	ctx                context.Context
	service            *app.Service
	activeTab          activeTab
	focusDepth         int
	view               currentView
	skills             []model.SkillStatus
	filtered           []model.SkillStatus
	deployments        map[string][]string
	cursor             int
	targets            map[string]model.TargetConfig
	targetNames        []string
	targetCursor       int
	caches             []cache.CacheEntry
	cacheCursor        int
	doctorReport       *app.DoctorReport
	doctorCursor       int
	doctorScrollOffset int

	// Upstreams
	upstreams            []model.UpstreamInfo
	upstreamCursor       int
	upstreamScrollOffset int
	upstreamOperation    uint64
	upstreamScanActive   bool

	// Channel / project deployment targets
	targetFilter        targetFilterMode
	agentTargets        []agentTargetItem
	channelEntries      []channelEntry
	channelCursor       int
	projectTargets      []projectTargetItem
	projectCursor       int
	projectScrollOffset int

	// Skill / group sub-tabs
	skillSubTab skillSubTab

	// Group management
	groupNames        []string
	groups            map[string]model.ProfileConfig
	groupCursor       int
	groupScrollOffset int

	// System sub-tabs
	systemSubTab systemSubTab

	// Settings management
	settings             []settingItem
	settingsCursor       int
	settingsScrollOffset int
	editSettingModal     EditSettingModalState
	languageModal        SelectLanguageModalState
	selectWorkspaceModal SelectWorkspaceModalState
	addWorkspaceModal    AddWorkspaceModalState
	targetDetailModal    TargetDetailModalState

	// Model management
	models               []app.AgentModelItem
	modelCursor          int
	modelRowCursor       int
	modelScrollOffset    int
	modelConfigFile      string
	modelOverride        bool
	isModelsDiffConfirm  bool
	modelsDiffRulesOnly  bool
	modelsDiffSummary    *agent.EnrichSummary
	agentProviders       []app.AgentProviderItem
	collapsedProviders   map[string]bool
	modelDetailModal     ModelDetailModalState
	modelConfigFileModal ModelConfigFileModalState
	providerDetailModal  ProviderDetailModalState
	editFieldModal       FieldEditModalState
	editJSONModal        JSONEditModalState
	pendingConfigEdit    *pendingConfigEdit
	editorOperation      uint64

	// Sub-states
	addModal                   AddModalState
	addUpstreamModal           AddUpstreamModalState
	editUpstreamModal          EditUpstreamModalState
	confirmUpstreamRemoveModal ConfirmUpstreamRemoveModalState
	upstreamDetailModal        UpstreamDetailModalState
	targetModal                TargetModalState
	projectModal               ProjectModalState
	groupModal                 GroupModalState
	groupSkillsModal           GroupSkillsModalState
	confirmModal               ConfirmationModalState
	batchRemoveModal           BatchRemoveModalState
	selectedSkills             map[string]bool
	deployModal                DeployModalState
	deployConflictModal        DeployConflictModalState
	newInput                   textinput.Model
	searchInput                textinput.Model
	skillScrollOffset          int
	targetScrollOffset         int
	cacheScrollOffset          int

	notice            string
	gitProgress       string
	progressCurrent   int
	progressTotal     int
	progressPhase     progress.Phase
	progressFrame     int
	program           *tea.Program
	cancelOp          context.CancelFunc
	detailContent     string
	diffContent       string
	diffSummary       diffview.Summary
	diffChangesOnly   bool
	diffRendered      string
	diffSearch        textinput.Model
	diffSearching     bool
	diffPrefix        string
	diffLastJump      int
	detailSkill       *model.SkillStatus
	detailMeta        *model.SkillMetadata
	detailPath        string
	detailViewport    viewport.Model
	diffViewport      viewport.Model
	dashboardViewport viewport.Model
	helpViewport      viewport.Model
	width             int
	height            int
	err               error
	loading           bool
}

// Styles
var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 2)

	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#6C5CE7")).
			Padding(0, 2)

	ancestorTabStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#D6CCFF")).
				Background(lipgloss.Color("#3A2F6B")).
				Padding(0, 2)

	subTabFocusedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#6C5CE7")).
				Padding(0, 1)

	subTabSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#0984E3")).
				Padding(0, 1)

	subTabInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#636E72")).
				Padding(0, 1)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#A0A0A0")).
				Background(lipgloss.Color("#2D3436")).
				Padding(0, 2)

	selectedRowStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#4834D4"))

	statusCleanStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00B894")).
				Bold(true)

	statusModifiedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FDCB6E")).
				Bold(true)

	statusErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#D63031")).
				Bold(true)

	statusNoticeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#0984E3")).
				Bold(true)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#B2BEC3")).
			Background(lipgloss.Color("#2D3436")).
			Padding(0, 1)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#6C5CE7")).
			Padding(1, 2)

	detailCardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#6C5CE7")).
			Padding(0, 1)

	detailNameStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#A29BFE"))

	detailValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FAFAFA"))

	detailDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#B2BEC3"))

	detailPathStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#74B9FF"))

	detailDividerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6C5CE7")).
				Bold(true)
)

// Run starts the Bubble Tea program.
func Run(ctx context.Context, svc *app.Service) error {
	m := NewModel(ctx, svc)
	p := tea.NewProgram(m, tea.WithAltScreen())
	m.program = p
	_, err := p.Run()
	return err
}

// NewModel creates an initial TUI model.
func NewModel(ctx context.Context, svc *app.Service) *Model {
	if svc != nil {
		if cfg, err := svc.Config(); err == nil && cfg != nil && cfg.Language != "" {
			i18n.Init(cfg.Language)
		}
	}

	nInput := textinput.New()
	nInput.Placeholder = i18n.T("modal.new.placeholder")
	nInput.CharLimit = 64
	nInput.Width = 40

	sInput := textinput.New()
	sInput.Placeholder = i18n.T("search.placeholder")
	sInput.CharLimit = 64
	sInput.Width = 30

	m := &Model{
		ctx:               ctx,
		service:           svc,
		activeTab:         tabDashboard,
		view:              viewList,
		width:             120,
		height:            24,
		addModal:          newAddModal(),
		addUpstreamModal:  newAddUpstreamModal(),
		targetModal:       newTargetModal(),
		projectModal:      newProjectModal(),
		groupModal:        newGroupModal(),
		selectedSkills:    make(map[string]bool),
		newInput:          nInput,
		searchInput:       sInput,
		detailViewport:    viewport.New(76, 10),
		diffViewport:      viewport.New(76, 14),
		dashboardViewport: viewport.New(76, 14),
		helpViewport:      viewport.New(76, 20),
	}
	m.reloadLocal()
	m.reloadTargets()
	m.reloadUpstreams()
	m.reloadGroups()
	m.reloadSettings()
	m.updateViewportSizes()
	return m
}

func (m *Model) updateViewportSizes() {
	vpW := m.width - 4
	if vpW < 20 {
		vpW = 20
	}
	// Fixed elements height in detail:
	// top title (2) + tabs (2) + compact detail card (5) + footer (1) = 10 lines
	vpH := m.height - 10
	if vpH < 5 {
		vpH = 5
	}
	m.detailViewport.Width = vpW
	m.detailViewport.Height = vpH

	diffH := m.height - 8
	if diffH < 5 {
		diffH = 5
	}
	m.diffViewport.Width = vpW
	m.diffViewport.Height = diffH

	// Dashboard viewport height is computed at render time from the shared
	// chrome budget (see renderBaseLayout).
	m.dashboardViewport.Width = m.width
	if m.dashboardViewport.Width < 20 {
		m.dashboardViewport.Width = 20
	}

	if m.diffContent != "" && (m.view == viewDiff || m.view == viewTargetDiff) {
		m.renderDiffView()
	}
}

// renderDiffView re-renders the current diff honoring the changes-only toggle.
func (m *Model) renderDiffView() {
	rendered := diffview.Render(m.diffContent, diffview.RenderOptions{
		Color:          true,
		IncludeSummary: true,
		ChangesOnly:    m.diffChangesOnly,
		LineNumbers:    true,
		Width:          m.diffViewport.Width,
	})
	m.diffViewport.SetContent(rendered)
	m.diffRendered = rendered
}

// toggleChangesOnly flips between the full diff and a changes-only view.
func (m *Model) toggleChangesOnly() {
	m.diffChangesOnly = !m.diffChangesOnly
	if m.diffChangesOnly {
		m.notice = i18n.T("notice.diff_changes_only")
	} else {
		m.notice = i18n.T("notice.diff_full")
	}
	m.renderDiffView()
	m.diffViewport.GotoTop()
	m.diffLastJump = -1
}

func (m *Model) reloadLocal() {
	if m.service == nil || m.service.WorkspaceRoot() == "" {
		m.notice = i18n.T("notice.no_workspace")
		return
	}
	statuses, err := m.service.Status(m.ctx, false)
	if err != nil {
		m.err = err
		return
	}
	m.skills = statuses
	m.deployments = m.service.AllSkillDeployments()
	m.applyFilter()
	m.clampSkillCursor()
	if m.selectedSkills != nil {
		validIDs := make(map[string]bool, len(m.skills))
		for _, s := range m.skills {
			validIDs[s.ID] = true
		}
		for id := range m.selectedSkills {
			if !validIDs[id] {
				delete(m.selectedSkills, id)
			}
		}
	}

	targets, err := m.service.TargetList()
	if err == nil {
		m.targets = targets
		m.targetNames = make([]string, 0, len(targets))
		for name := range targets {
			m.targetNames = append(m.targetNames, name)
		}
	}

	caches, err := m.service.CacheList()
	if err == nil {
		m.caches = caches
	}

	m.reloadUpstreams()
}

// reloadAsync runs every full-state reload off the Bubble Tea main loop and
// returns a snapshot applied by the reloadCompleteMsg handler. Running these
// reloads on the main loop would block rendering on Git access, network calls
// and full-directory scans.
func (m *Model) reloadAsync() tea.Cmd {
	snapshot := *m
	if m.selectedSkills != nil {
		sel := make(map[string]bool, len(m.selectedSkills))
		for k, v := range m.selectedSkills {
			sel[k] = v
		}
		snapshot.selectedSkills = sel
	}
	return func() tea.Msg {
		s := &snapshot
		s.reloadLocal()
		s.reloadTargets()
		s.reloadUpstreams()
		s.reloadGroups()
		s.reloadSettings()
		s.reloadModels()
		if s.service != nil && s.targetDetailModal.Name != "" {
			if s.targetDetailModal.Type == "project" {
				s.targetDetailModal.DeployedSkills = s.service.ProjectDeployedSkills(s.targetDetailModal.ConfigDir)
			} else {
				s.targetDetailModal.DeployedSkills = s.service.TargetDeployedSkills(s.targetDetailModal.Name)
			}
		}
		return reloadCompleteMsg{snapshot: s}
	}
}

// reloadUpstreamsCmd refreshes the tracked upstream list off the main loop.
func (m *Model) reloadUpstreamsCmd() tea.Cmd {
	svc := m.service
	ctx := m.ctx
	return func() tea.Msg {
		if svc == nil {
			return nil
		}
		upstreams, err := svc.UpstreamList(ctx)
		if err != nil {
			return nil
		}
		return upstreamReloadMsg(upstreams)
	}
}

// asyncWrite runs a mutating service operation off the Bubble Tea main loop.
// On success it delivers the notice via asyncNoticeMsg, which also triggers an
// async reload of all cached state.
func (m *Model) asyncWrite(notice string, fn func(ctx context.Context) error) tea.Cmd {
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancelOp = cancel
	m.loading = true
	m.notice = notice
	return func() tea.Msg {
		if ctx.Err() != nil {
			return nil
		}
		if err := fn(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return errMsg{err: err}
		}
		if ctx.Err() != nil {
			return nil
		}
		return asyncNoticeMsg(notice)
	}
}

// toggleEnabledCmd sets a target/project enabled flag off the main loop and
// reports the new state so an open detail modal can stay in sync.
func (m *Model) toggleEnabledCmd(isProject bool, key string, enabled bool, notice string) tea.Cmd {
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancelOp = cancel
	m.loading = true
	m.notice = notice
	return func() tea.Msg {
		var err error
		if isProject {
			err = m.service.SetProjectEnabled(key, enabled)
		} else {
			err = m.service.SetTargetEnabled(key, enabled)
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return errMsg{err: err}
		}
		if ctx.Err() != nil {
			return nil
		}
		return targetToggledMsg{enabled: enabled, notice: notice}
	}
}

// configSetCmd persists a single config key off the main loop and asks the
// main loop for a state refresh once the write has landed.
func (m *Model) configSetCmd(key, val string) tea.Cmd {
	svc := m.service
	if svc == nil {
		return nil
	}
	return func() tea.Msg {
		_ = svc.ConfigSet(key, val)
		return configChangedMsg{}
	}
}

func (m *Model) reloadUpstreams() {
	if m.service == nil {
		return
	}
	upstreams, err := m.service.UpstreamList(m.ctx)
	if err == nil {
		m.upstreams = upstreams
		m.clampUpstreamCursor()
	}
}

func (m *Model) reloadCache() {
	if m.service == nil {
		return
	}
	caches, err := m.service.CacheList()
	if err == nil {
		m.caches = caches
	}
}

func (m *Model) reloadTargets() {
	if m.service == nil {
		return
	}
	// 1. Agent targets: strictly from user configured targets
	var agents []agentTargetItem
	targets, err := m.service.TargetList()
	if err == nil {
		m.targets = targets
		m.targetNames = make([]string, 0, len(targets))
		var agentNames []string
		for name, tgt := range targets {
			if tgt.Type != model.TargetTypeProject {
				agentNames = append(agentNames, name)
			}
		}
		sort.Strings(agentNames)
		for _, name := range agentNames {
			m.targetNames = append(m.targetNames, name)
			tgt := targets[name]
			deployed := m.service.TargetDeployedSkills(name)
			agents = append(agents, agentTargetItem{
				Name:        name,
				Channel:     tgt.Channel,
				ConfigDir:   tgt.ConfigDir,
				ResolvedDir: tgt.ResolveConfigDir(),
				SkillsDir:   tgt.SkillsDir(),
				Paths:       tgt.Paths,
				Features:    tgt.FeaturesString(),
				SkillCount:  len(deployed),
				Enabled:     tgt.IsEnabled(),
			})
		}
	}
	m.agentTargets = agents

	// 2. Project targets: configured projects + current directory + recent projects, deduplicated by CanonicalPath
	var projs []projectTargetItem
	seenProjects := make(map[string]bool)

	// Add configured project targets
	if targets != nil {
		var projNames []string
		for name, tgt := range targets {
			if tgt.Type == model.TargetTypeProject {
				projNames = append(projNames, name)
			}
		}
		sort.Strings(projNames)
		for _, name := range projNames {
			tgt := targets[name]
			pDir := tgt.ResolveConfigDir()
			if pDir == "" {
				pDir = tgt.SkillsDir()
			}
			canon := fsx.CanonicalPath(pDir)
			if seenProjects[canon] {
				continue
			}
			seenProjects[canon] = true

			fmts := deploy.DetectProjectFormats(pDir)
			deployed := m.service.ProjectDeployedSkills(pDir)
			sc := len(deployed)
			fmtStr := strings.Join(fmts, ", ")
			if fmtStr == "" {
				fmtStr = "none"
			}
			enabled := tgt.IsEnabled() && m.service.IsProjectEnabled(pDir)
			projs = append(projs, projectTargetItem{
				Name:       name,
				Path:       fsx.CompactUser(pDir),
				RawPath:    pDir,
				Formats:    fmtStr,
				SkillCount: sc,
				Enabled:    enabled,
			})
		}
	}

	recent, _ := m.service.ProjectList()
	for _, p := range recent {
		if p == "" {
			continue
		}
		canon := fsx.CanonicalPath(p)
		if seenProjects[canon] {
			continue
		}
		seenProjects[canon] = true

		fmts := deploy.DetectProjectFormats(p)
		deployed := m.service.ProjectDeployedSkills(p)
		sc := len(deployed)
		fmtStr := strings.Join(fmts, ", ")
		if fmtStr == "" {
			fmtStr = "none"
		}
		name := filepath.Base(p)
		if name == "" || name == "/" || name == "." {
			name = p
		}
		projs = append(projs, projectTargetItem{
			Name:       name,
			Path:       fsx.CompactUser(p),
			RawPath:    p,
			Formats:    fmtStr,
			SkillCount: sc,
			Enabled:    m.service.IsProjectEnabled(p),
		})
	}
	m.projectTargets = projs
	m.clampTargetCursors()
	m.reloadChannels()
}

// reloadChannels rebuilds the channel entries from the configured agent targets.
func (m *Model) reloadChannels() {
	entries := make([]channelEntry, 0, len(m.agentTargets))
	for _, item := range m.agentTargets {
		ch := item.Channel
		if ch == "" {
			ch = item.Name
		}
		entries = append(entries, channelEntry{
			Name:      item.Name,
			Channel:   ch,
			Target:    item,
			Supported: agent.ChannelSupported(ch),
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	m.channelEntries = entries
	shown := m.displayedChannels()
	if m.channelCursor >= len(shown) {
		m.channelCursor = max(0, len(shown)-1)
	}
}

func (m *Model) displayedProjectTargets() []projectTargetItem {
	var base []projectTargetItem
	if m.targetFilter == targetFilterAll {
		base = m.projectTargets
	} else {
		for _, p := range m.projectTargets {
			if (m.targetFilter == targetFilterEnabledOnly && p.Enabled) ||
				(m.targetFilter == targetFilterDisabledOnly && !p.Enabled) {
				base = append(base, p)
			}
		}
	}
	query := strings.TrimSpace(m.searchInput.Value())
	if query == "" {
		return base
	}
	lowerQuery := strings.ToLower(query)
	var re *regexp.Regexp
	if strings.ContainsAny(query, ".*+?^$[]{}|()\\") {
		re, _ = regexp.Compile("(?i)" + query)
	}
	var res []projectTargetItem
	for _, p := range base {
		matched := false
		if re != nil && re.MatchString(p.Name) {
			matched = true
		}
		if !matched {
			if strings.Contains(strings.ToLower(p.Name), lowerQuery) ||
				strings.Contains(strings.ToLower(p.Path), lowerQuery) ||
				strings.Contains(strings.ToLower(p.RawPath), lowerQuery) ||
				strings.Contains(strings.ToLower(p.Formats), lowerQuery) {
				matched = true
			}
		}
		if matched {
			res = append(res, p)
		}
	}
	return res
}

func (m *Model) displayedGroupNames() []string {
	query := strings.TrimSpace(m.searchInput.Value())
	if query == "" {
		return m.groupNames
	}
	lowerQuery := strings.ToLower(query)
	var res []string
	for _, name := range m.groupNames {
		matched := strings.Contains(strings.ToLower(name), lowerQuery)
		if !matched {
			if grp, ok := m.groups[name]; ok {
				for _, sk := range grp.Skills {
					if strings.Contains(strings.ToLower(sk), lowerQuery) {
						matched = true
						break
					}
				}
			}
		}
		if matched {
			res = append(res, name)
		}
	}
	return res
}

func (m *Model) displayedCaches() []cache.CacheEntry {
	query := strings.TrimSpace(m.searchInput.Value())
	if query == "" {
		return m.caches
	}
	lowerQuery := strings.ToLower(query)
	var res []cache.CacheEntry
	for _, c := range m.caches {
		if strings.Contains(strings.ToLower(c.Key), lowerQuery) ||
			strings.Contains(strings.ToLower(c.URL), lowerQuery) {
			res = append(res, c)
		}
	}
	return res
}

func (m *Model) displayedSettings() []settingItem {
	query := strings.TrimSpace(m.searchInput.Value())
	if query == "" {
		return m.settings
	}
	lowerQuery := strings.ToLower(query)
	var res []settingItem
	for _, s := range m.settings {
		if strings.Contains(strings.ToLower(s.Key), lowerQuery) ||
			strings.Contains(strings.ToLower(s.Title), lowerQuery) ||
			strings.Contains(strings.ToLower(s.Description), lowerQuery) ||
			strings.Contains(strings.ToLower(s.Value), lowerQuery) {
			res = append(res, s)
		}
	}
	return res
}

// displayedChannels returns the configured agent channels after applying the
// enabled/disabled status filter.
func (m *Model) displayedChannels() []channelEntry {
	query := strings.ToLower(strings.TrimSpace(m.searchInput.Value()))
	var res []channelEntry
	for _, e := range m.channelEntries {
		statusOK := m.targetFilter == targetFilterAll ||
			(m.targetFilter == targetFilterEnabledOnly && e.Target.Enabled) ||
			(m.targetFilter == targetFilterDisabledOnly && !e.Target.Enabled)
		if !statusOK {
			continue
		}
		if query != "" &&
			!strings.Contains(strings.ToLower(e.Name), query) &&
			!strings.Contains(strings.ToLower(e.Channel), query) &&
			!strings.Contains(strings.ToLower(e.Target.ConfigDir), query) {
			continue
		}
		res = append(res, e)
	}
	return res
}

// currentChannelEntry returns the selected channel entry, clamped to range.
func (m *Model) currentChannelEntry() (channelEntry, bool) {
	entries := m.displayedChannels()
	if len(entries) == 0 {
		return channelEntry{}, false
	}
	if m.channelCursor < 0 || m.channelCursor >= len(entries) {
		m.channelCursor = 0
	}
	return entries[m.channelCursor], true
}

// currentAgentChannel returns the channel identifier used for model config.
func (m *Model) currentAgentChannel() string {
	if e, ok := m.currentChannelEntry(); ok {
		return e.Channel
	}
	if len(m.channelEntries) > 0 {
		return m.channelEntries[0].Channel
	}
	return "opencode"
}

// switchChannel moves to the adjacent configured channel and reloads its models.
// It reports whether the channel actually changed.
func (m *Model) switchChannel(delta int) bool {
	entries := m.displayedChannels()
	n := len(entries)
	if n == 0 {
		return false
	}
	next := (m.channelCursor + delta) % n
	if next < 0 {
		next += n
	}
	if next == m.channelCursor {
		return false
	}
	m.channelCursor = next
	m.modelCursor = 0
	m.modelRowCursor = 0
	m.modelScrollOffset = 0
	m.collapsedProviders = make(map[string]bool)
	m.reloadModels()
	return true
}

// hasSubTabs reports whether the active tab exposes a secondary level that Tab
// can descend into.
func (m *Model) hasSubTabs() bool {
	switch m.activeTab {
	case tabSkills, tabSystem:
		return true
	case tabChannels:
		return len(m.displayedChannels()) > 1
	}
	return false
}

// cycleFocusDepth wraps the focused level around the page's available levels.
// The hierarchy currently has two levels (0 primary, 1 secondary), so cycling
// is a toggle and Tab / Shift+Tab behave identically; pages without a secondary
// level ignore the key.
func (m *Model) cycleFocusDepth() {
	if !m.hasSubTabs() {
		return
	}
	m.focusDepth = (m.focusDepth + 1) % 2
}

// setActiveTab is the single entry point for changing the top-level tab. It
// always resets focus to the primary level and rewinds the secondary selection
// to its first item so entering a page is deterministic.
func (m *Model) setActiveTab(t activeTab) tea.Cmd {
	m.activeTab = t
	m.focusDepth = 0
	m.cursor = 0
	switch t {
	case tabSkills:
		m.skillSubTab = skillTabSkills
	case tabChannels:
		m.channelCursor = 0
		m.modelCursor = 0
		m.modelRowCursor = 0
		m.modelScrollOffset = 0
	case tabProjects:
		m.projectCursor = 0
	case tabSystem:
		m.systemSubTab = systemTabCache
		m.cacheCursor = 0
		m.doctorCursor = 0
		m.doctorScrollOffset = 0
	case tabSettings:
		m.settingsCursor = 0
	}
	return m.onTabChanged()
}

// cycleActiveTab moves to the previous/next top-level tab, wrapping around.
func (m *Model) cycleActiveTab(delta int) tea.Cmd {
	const n = 7
	next := (int(m.activeTab) + delta) % n
	if next < 0 {
		next += n
	}
	return m.setActiveTab(activeTab(next))
}

// cycleSubTab moves within the active tab's secondary level, wrapping around.
func (m *Model) cycleSubTab(delta int) tea.Cmd {
	switch m.activeTab {
	case tabChannels:
		m.switchChannel(delta)
	case tabSkills:
		if m.skillSubTab == skillTabSkills {
			m.skillSubTab = skillTabGroups
			m.reloadGroups()
		} else {
			m.skillSubTab = skillTabSkills
		}
	case tabSystem:
		if m.systemSubTab == systemTabCache {
			m.systemSubTab = systemTabDoctor
			m.doctorCursor = 0
			m.doctorScrollOffset = 0
			return m.doctorCmd()
		}
		m.systemSubTab = systemTabCache
		return m.reloadCacheCmd()
	}
	return nil
}

// moveFocusHorizontal implements the ←/→ invariant: the key stays inside the
// currently focused level and never crosses levels.
func (m *Model) moveFocusHorizontal(delta int) tea.Cmd {
	if m.focusDepth == 0 {
		return m.cycleActiveTab(delta)
	}
	return m.cycleSubTab(delta)
}

func (m *Model) displayedModels() []app.AgentModelItem {
	query := strings.TrimSpace(m.searchInput.Value())
	lowerQuery := strings.ToLower(query)

	var res []app.AgentModelItem
	for _, it := range m.models {
		if query != "" {
			if !strings.Contains(strings.ToLower(it.ModelID), lowerQuery) &&
				!strings.Contains(strings.ToLower(it.Name), lowerQuery) &&
				!strings.Contains(strings.ToLower(it.ProviderID), lowerQuery) &&
				!strings.Contains(strings.ToLower(it.SourceProvider), lowerQuery) {
				continue
			}
		}
		res = append(res, it)
	}
	return res
}

func (m *Model) clampModelCursor() {
	models := m.displayedModels()
	if m.modelCursor >= len(models) {
		m.modelCursor = max(0, len(models)-1)
	}
	if m.modelCursor < 0 {
		m.modelCursor = 0
	}
	m.syncModelRowCursor()
}

// clampModelRowCursor keeps the row cursor inside the current display rows and
// synchronizes the model cursor when the cursor sits on a model row.
func (m *Model) clampModelRowCursor() {
	rows := m.getModelDisplayRows()
	if len(rows) == 0 {
		m.modelRowCursor = 0
		return
	}
	if m.modelRowCursor >= len(rows) {
		m.modelRowCursor = len(rows) - 1
	}
	if m.modelRowCursor < 0 {
		m.modelRowCursor = 0
	}
	if !rows[m.modelRowCursor].isHeader {
		m.modelCursor = rows[m.modelRowCursor].modelIndex
	}
}

func (m *Model) clampTargetCursors() {
	projs := m.displayedProjectTargets()
	if m.projectCursor >= len(projs) {
		m.projectCursor = max(0, len(projs)-1)
	}
}

func (m *Model) reloadGroups() {
	if m.service == nil {
		return
	}
	groups, err := m.service.GroupList()
	if err == nil {
		m.groups = groups
		m.groupNames = make([]string, 0, len(groups))
		for name := range groups {
			m.groupNames = append(m.groupNames, name)
		}
		sort.Strings(m.groupNames)
		if m.groupCursor >= len(m.groupNames) {
			m.groupCursor = max(0, len(m.groupNames)-1)
		}
	}
}

func (m *Model) reloadSettings() {
	if m.service == nil {
		return
	}

	cfg, _ := m.service.Config()
	if cfg == nil {
		cfg = &model.Config{Version: 1}
	}

	// 1. Language
	currLang := cfg.Language
	if currLang == "" {
		currLang = "auto"
	}
	langVal := i18n.T("settings.lang.auto")
	switch strings.ToLower(currLang) {
	case "zh-cn", "zh", "chinese", "cn":
		langVal = i18n.T("settings.lang.zh_cn")
	case "en", "english":
		langVal = i18n.T("settings.lang.en")
	}

	// 2. Default Root
	defRoot := cfg.DefaultRoot
	if defRoot == "" {
		defRoot = config.DefaultWorkspacePathCompact()
	}

	// 2. Workspace Root
	wsRoot := fsx.NormalizeWorkspacePath(m.service.WorkspaceRoot())
	wsStatus := i18n.T("settings.status.uninitialized")
	if m.service.WorkspaceInitialized() {
		wsStatus = i18n.T("settings.status.initialized")
	}
	wsDisplay := wsRoot
	if wsDisplay == "" {
		wsDisplay = fsx.NormalizeWorkspacePath(defRoot)
	}
	wsDisplay = wsDisplay + " " + wsStatus
	allWS := m.service.Workspaces()
	if len(allWS) > 1 {
		countLabel := i18n.T("settings.workspace_saved_count", len(allWS))
		wsDisplay = wsDisplay + countLabel
	}

	// 3. Config Path
	cfgPath := m.service.ConfigPath()
	cfgStatus := i18n.T("settings.status.saved")
	if !m.service.ConfigExists() {
		cfgStatus = i18n.T("settings.status.not_created")
	}
	cfgDisplay := cfgPath + " " + cfgStatus

	// 4. Targets summary
	tgtCount := len(cfg.Targets)
	tgtDisplay := fmt.Sprintf("%d channels", tgtCount)
	if tgtCount > 0 {
		var names []string
		for n := range cfg.Targets {
			names = append(names, n)
		}
		sort.Strings(names)
		tgtDisplay = fmt.Sprintf("%d (%s)", tgtCount, strings.Join(names, ", "))
	}

	// 5. Groups summary
	grpCount := len(cfg.Profiles)
	grpDisplay := fmt.Sprintf("%d groups", grpCount)
	if grpCount > 0 {
		var names []string
		for n := range cfg.Profiles {
			names = append(names, n)
		}
		sort.Strings(names)
		grpDisplay = fmt.Sprintf("%d (%s)", grpCount, strings.Join(names, ", "))
	}

	// 6. Git Cache
	cacheDisplay := fmt.Sprintf("%d repos", len(m.caches))
	if len(m.caches) > 0 {
		var totalBytes int64
		for _, c := range m.caches {
			totalBytes += c.SizeBytes
		}
		cacheDisplay = fmt.Sprintf("%d repos (%.2f MB)", len(m.caches), float64(totalBytes)/(1024*1024))
	}

	// 7. Backup max versions
	backupMax := cfg.GetBackupMaxVersions()
	backupDisplay := i18n.T("settings.backup.unlimited")
	if backupMax != nil {
		if *backupMax == 0 {
			backupDisplay = i18n.T("settings.backup.disabled")
		} else {
			backupDisplay = fmt.Sprintf(i18n.T("settings.backup.limit"), *backupMax)
		}
	}

	m.settings = []settingItem{
		{
			Key:         "language",
			Title:       i18n.T("settings.item.language"),
			Value:       langVal,
			Description: i18n.T("settings.desc.language"),
			ActionHint:  i18n.T("settings.action.language"),
		},
		{
			Key:         "workspace",
			Title:       i18n.T("settings.item.workspace"),
			Value:       wsDisplay,
			Description: i18n.T("settings.desc.workspace"),
			ActionHint:  i18n.T("settings.action.workspace"),
		},
		{
			Key:         "config_path",
			Title:       i18n.T("settings.item.config_path"),
			Value:       cfgDisplay,
			Description: i18n.T("settings.desc.config_path"),
			ActionHint:  i18n.T("settings.action.config_path"),
		},
		{
			Key:         "backup_max_versions",
			Title:       i18n.T("settings.item.backup_max_versions"),
			Value:       backupDisplay,
			Description: i18n.T("settings.desc.backup_max_versions"),
			ActionHint:  i18n.T("settings.action.backup_max_versions"),
		},
		{
			Key:         "targets",
			Title:       i18n.T("settings.item.targets"),
			Value:       tgtDisplay,
			Description: i18n.T("settings.desc.targets"),
			ActionHint:  i18n.T("settings.action.targets"),
		},
		{
			Key:         "groups",
			Title:       i18n.T("settings.item.groups"),
			Value:       grpDisplay,
			Description: i18n.T("settings.desc.groups"),
			ActionHint:  i18n.T("settings.action.groups"),
		},
		{
			Key:         "cache",
			Title:       i18n.T("settings.item.cache"),
			Value:       cacheDisplay,
			Description: i18n.T("settings.desc.cache"),
			ActionHint:  i18n.T("settings.action.cache"),
		},
		{
			Key:         "reset_defaults",
			Title:       i18n.T("settings.item.reset_defaults"),
			Value:       i18n.T("settings.val.reset_defaults"),
			Description: i18n.T("settings.desc.reset_defaults"),
			ActionHint:  i18n.T("settings.action.reset_defaults"),
		},
	}
	if m.settingsCursor >= len(m.settings) {
		m.settingsCursor = max(0, len(m.settings)-1)
	}

	m.modelOverride = cfg.GetModelEnrichMode() == model.ModelEnrichModeRemote
}

func (m *Model) applyLanguage(code string) tea.Cmd {
	i18n.Init(code)
	m.newInput.Placeholder = i18n.T("modal.new.placeholder")
	m.searchInput.Placeholder = i18n.T("search.placeholder")
	m.reloadSettings()
	return m.configSetCmd("language", code)
}

// nextLanguage returns the language code that cycleLanguage would switch to.
func (m *Model) nextLanguage() string {
	cfg, _ := m.service.Config()
	curr := "auto"
	if cfg != nil && cfg.Language != "" {
		curr = cfg.Language
	}
	switch strings.ToLower(curr) {
	case "zh-cn", "zh", "chinese", "cn":
		return "en"
	case "en", "english":
		return "auto"
	default:
		return "zh-CN"
	}
}

func languageLabel(code string) string {
	switch strings.ToLower(code) {
	case "zh-cn", "zh", "chinese", "cn":
		return i18n.T("settings.lang.zh_cn")
	case "en", "english":
		return i18n.T("settings.lang.en")
	default:
		return i18n.T("settings.lang.auto")
	}
}

func (m *Model) handleSettingsEnter() (tea.Model, tea.Cmd) {
	settings := m.displayedSettings()
	if m.settingsCursor < 0 || m.settingsCursor >= len(settings) {
		return m, nil
	}
	item := settings[m.settingsCursor]
	switch item.Key {
	case "language":
		cfg, _ := m.service.Config()
		currLang := "auto"
		if cfg != nil && cfg.Language != "" {
			currLang = cfg.Language
		}
		m.languageModal = newSelectLanguageModal(currLang)
		m.view = viewModalSelectLanguage
		return m, nil

	case "workspace", "default_root", "workspace_root":
		return m.openSelectWorkspaceModal()

	case "config_path":
		m.reloadSettings()
		m.notice = i18n.T("notice.settings_reloaded")
		return m, nil

	case "targets":
		return m, m.setActiveTab(tabChannels)

	case "groups":
		cmd := m.setActiveTab(tabSkills)
		m.skillSubTab = skillTabGroups
		m.reloadGroups()
		return m, cmd

	case "cache":
		return m, m.setActiveTab(tabSystem)

	case "reset_defaults":
		m.confirmModal = ConfirmationModalState{
			Title:   i18n.T("confirm.reset_defaults.title"),
			Message: i18n.T("confirm.reset_defaults.msg"),
			Action:  "reset-defaults",
		}
		m.view = viewModalConfirm
		return m, nil
	}
	return m, nil
}

func (m *Model) handleInitWorkspace() (tea.Model, tea.Cmd) {
	if m.service.WorkspaceInitialized() {
		m.notice = i18n.T("notice.workspace_already_initialized")
		return m, nil
	}
	target := m.service.WorkspaceRoot()
	if target == "" {
		target = "."
	}
	return m, m.asyncWrite(i18n.T("notice.workspace_initialized"), func(ctx context.Context) error {
		_, err := m.service.InitWorkspace(ctx, target)
		return err
	})
}

func (m *Model) handleModalEditSettingKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = viewList
		m.err = nil
		return m, nil

	case "enter":
		val := strings.TrimSpace(m.editSettingModal.Input.Value())
		if m.editSettingModal.SettingKey == "workspace" || m.editSettingModal.SettingKey == "default_root" || m.editSettingModal.SettingKey == "workspace_root" {
			if val == "" {
				m.err = fmt.Errorf("%s", i18n.T("error.workspace_root_empty"))
				return m, nil
			}
			return m, m.asyncWrite(fmt.Sprintf(i18n.T("notice.workspace_updated"), val), func(ctx context.Context) error {
				if err := m.service.ConfigSet("default_root", val); err != nil {
					return err
				}
				return m.service.SwitchWorkspaceRoot(val)
			})
		}
		m.view = viewList
		return m, nil

	default:
		var cmd tea.Cmd
		m.editSettingModal.Input, cmd = m.editSettingModal.Input.Update(msg)
		return m, cmd
	}
}

func (m *Model) handleModalSelectLanguageKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = viewList
		return m, nil

	case "up", "k":
		if m.languageModal.Cursor > 0 {
			m.languageModal.Cursor--
		}
		return m, nil

	case "down", "j":
		if m.languageModal.Cursor < len(m.languageModal.Options)-1 {
			m.languageModal.Cursor++
		}
		return m, nil

	case "1":
		opt := m.languageModal.Options[0]
		cmd := m.applyLanguage(opt.Code)
		m.view = viewList
		m.notice = fmt.Sprintf(i18n.T("notice.language_changed"), opt.Label)
		return m, cmd

	case "2":
		opt := m.languageModal.Options[1]
		cmd := m.applyLanguage(opt.Code)
		m.view = viewList
		m.notice = fmt.Sprintf(i18n.T("notice.language_changed"), opt.Label)
		return m, cmd

	case "3":
		opt := m.languageModal.Options[2]
		cmd := m.applyLanguage(opt.Code)
		m.view = viewList
		m.notice = fmt.Sprintf(i18n.T("notice.language_changed"), opt.Label)
		return m, cmd

	case "enter", " ":
		if m.languageModal.Cursor >= 0 && m.languageModal.Cursor < len(m.languageModal.Options) {
			opt := m.languageModal.Options[m.languageModal.Cursor]
			cmd := m.applyLanguage(opt.Code)
			m.view = viewList
			m.notice = fmt.Sprintf(i18n.T("notice.language_changed"), opt.Label)
			return m, cmd
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) openSelectWorkspaceModal() (tea.Model, tea.Cmd) {
	workspaces := m.service.Workspaces()
	activePath := m.service.WorkspaceRoot()
	m.selectWorkspaceModal = newSelectWorkspaceModal(workspaces, activePath, m.service.WorkspacePathInitialized)
	m.view = viewModalSelectWorkspace
	m.err = nil
	return m, nil
}

func (m *Model) openAddWorkspaceModal() (tea.Model, tea.Cmd) {
	m.addWorkspaceModal = newAddWorkspaceModal()
	m.view = viewModalAddWorkspace
	m.err = nil
	return m, nil
}

func (m *Model) handleModalSelectWorkspaceKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.view = viewList
		m.err = nil
		return m, nil

	case "up", "k":
		if m.selectWorkspaceModal.Cursor > 0 {
			m.selectWorkspaceModal.Cursor--
		}
		return m, nil

	case "down", "j":
		if m.selectWorkspaceModal.Cursor < len(m.selectWorkspaceModal.Options)-1 {
			m.selectWorkspaceModal.Cursor++
		}
		return m, nil

	case "enter":
		if len(m.selectWorkspaceModal.Options) == 0 {
			return m, nil
		}
		target := m.selectWorkspaceModal.Options[m.selectWorkspaceModal.Cursor].Path
		return m, m.asyncWrite(fmt.Sprintf(i18n.T("notice.workspace_switched"), target), func(ctx context.Context) error {
			return m.service.SelectWorkspace(target)
		})

	case "a", "+":
		return m.openAddWorkspaceModal()

	case "x", "d", "delete":
		if len(m.selectWorkspaceModal.Options) <= 1 {
			m.err = fmt.Errorf("%s", i18n.T("error.cannot_remove_only_workspace"))
			return m, nil
		}
		target := m.selectWorkspaceModal.Options[m.selectWorkspaceModal.Cursor].Path
		return m, m.asyncWrite(fmt.Sprintf(i18n.T("notice.workspace_removed"), target), func(ctx context.Context) error {
			return m.service.RemoveWorkspace(target)
		})

	case "i":
		if len(m.selectWorkspaceModal.Options) == 0 {
			return m, nil
		}
		target := m.selectWorkspaceModal.Options[m.selectWorkspaceModal.Cursor].Path
		if m.service.WorkspacePathInitialized(target) {
			m.notice = i18n.T("notice.workspace_already_initialized")
			return m, nil
		}
		return m, m.asyncWrite(fmt.Sprintf(i18n.T("notice.workspace_initialized_path"), target), func(ctx context.Context) error {
			return m.service.InitWorkspacePath(target)
		})
	}
	return m, nil
}

func (m *Model) handleModalAddWorkspaceKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = viewModalSelectWorkspace
		m.err = nil
		return m, nil

	case "enter":
		val := strings.TrimSpace(m.addWorkspaceModal.Input.Value())
		if val == "" {
			m.err = fmt.Errorf("%s", i18n.T("error.workspace_path_empty"))
			return m, nil
		}
		return m, m.asyncWrite(fmt.Sprintf(i18n.T("notice.workspace_added_and_switched"), val), func(ctx context.Context) error {
			if err := m.service.AddWorkspace(val); err != nil {
				return err
			}
			return m.service.SelectWorkspace(val)
		})

	default:
		var cmd tea.Cmd
		m.addWorkspaceModal.Input, cmd = m.addWorkspaceModal.Input.Update(msg)
		return m, cmd
	}
}

func (m *Model) applyFilter() {
	query := strings.TrimSpace(m.searchInput.Value())
	if query == "" {
		m.filtered = m.skills
	} else {
		lowerQuery := strings.ToLower(query)
		var re *regexp.Regexp
		if strings.ContainsAny(query, ".*+?^$[]{}|()\\") {
			re, _ = regexp.Compile("(?i)" + query)
		}

		var deployments map[string][]string
		if m.service != nil {
			deployments = m.deployments
		}
		var res []model.SkillStatus
		for _, s := range m.skills {
			matched := false
			if re != nil && re.MatchString(s.ID) {
				matched = true
			}
			if !matched {
				if strings.Contains(strings.ToLower(s.ID), lowerQuery) ||
					strings.Contains(strings.ToLower(string(s.Source.Type)), lowerQuery) ||
					strings.Contains(strings.ToLower(string(s.ManagedStatus)), lowerQuery) ||
					strings.Contains(strings.ToLower(s.Source.URL), lowerQuery) ||
					strings.Contains(strings.ToLower(s.Source.Path), lowerQuery) {
					matched = true
				}
			}
			if !matched && deployments != nil {
				for _, dep := range deployments[s.ID] {
					if strings.Contains(strings.ToLower(dep), lowerQuery) {
						matched = true
						break
					}
				}
			}
			if matched {
				res = append(res, s)
			}
		}
		m.filtered = res
	}
	if m.cursor >= len(m.visibleSkills()) {
		m.cursor = max(0, len(m.visibleSkills())-1)
	}
	m.clampTargetCursors()
	groups := m.displayedGroupNames()
	if m.groupCursor >= len(groups) {
		m.groupCursor = max(0, len(groups)-1)
	}
	caches := m.displayedCaches()
	if m.cacheCursor >= len(caches) {
		m.cacheCursor = max(0, len(caches)-1)
	}
	settings := m.displayedSettings()
	if m.settingsCursor >= len(settings) {
		m.settingsCursor = max(0, len(settings)-1)
	}
	m.clampModelCursor()
	m.clampUpstreamCursor()
}

func (m *Model) Init() tea.Cmd {
	// Start the loading animation timer; it reschedules itself for the life of
	// the program but only advances the frame while an operation is running.
	return progressTickCmd()
}

// finishLoading clears every loading-related field so a completed or cancelled
// operation cannot leak progress state into the next one.
func (m *Model) finishLoading() {
	m.loading = false
	m.gitProgress = ""
	m.progressCurrent = 0
	m.progressTotal = 0
	m.progressPhase = progress.PhaseUnknown
	m.cancelOp = nil
	m.upstreamScanActive = false
}

func progressTickCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(t time.Time) tea.Msg {
		return progressTickMsg(t)
	})
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle modal text input updates first if in modal
	var cmd tea.Cmd
	switch m.view {
	case viewModalAdd:
		if m.addModal.Step == 0 && !m.addModal.FromUpstreamMode {
			if m.addModal.ActiveInput == 0 {
				m.addModal.SourceInput, cmd = m.addModal.SourceInput.Update(msg)
			} else {
				m.addModal.PathInput, cmd = m.addModal.PathInput.Update(msg)
			}
		}
	case viewModalAddUpstream:
		cmd = m.updateAddUpstreamInputs(msg)
	case viewModalEditUpstream:
		cmd = m.updateEditUpstreamInputs(msg)
	case viewModalNew:
		m.newInput, cmd = m.newInput.Update(msg)
	case viewModalAddTarget:
		if m.targetModal.ActiveInput == 0 {
			m.targetModal.NameInput, cmd = m.targetModal.NameInput.Update(msg)
		} else {
			m.targetModal.PathInput, cmd = m.targetModal.PathInput.Update(msg)
		}
	case viewModalAddProject:
		m.projectModal.PathInput, cmd = m.projectModal.PathInput.Update(msg)
	case viewModalAddGroup:
		m.groupModal.NameInput, cmd = m.groupModal.NameInput.Update(msg)
	case viewSearch:
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.applyFilter()
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateViewportSizes()
		return m, nil

	case editorPreparedMsg:
		return m.handleEditorPrepared(msg)
	case editorFinishedMsg:
		return m.handleEditorFinished(msg)
	case configDraftErrorMsg:
		m.finishLoading()
		if m.view == viewModalEditJSON {
			m.editJSONModal.Err = i18n.T("error.editor_failed", msg.err.Error())
		} else {
			m.err = fmt.Errorf("%s", i18n.T("error.editor_failed", msg.err.Error()))
		}
		return m, nil
	case draftPreviewMsg:
		if msg.id != m.editorOperation || !m.loading {
			return m, nil
		}
		if msg.err != nil {
			return m.Update(configDraftErrorMsg{err: msg.err})
		}
		m.pendingConfigEdit = msg.pending
		return m.Update(configEditPreviewMsg{edit: msg.edit})
	case draftWriteMsg:
		if msg.id != m.editorOperation || !m.loading {
			return m, nil
		}
		if msg.err != nil {
			return m.Update(configDraftErrorMsg{err: msg.err})
		}
		return m.Update(asyncNoticeMsg(msg.notice))

	case progressMsg:
		// Unknown-phase updates only carry a detail line; they must not reset
		// an in-flight determinate bar. Every other phase reports its own
		// counts (0 total meaning "indeterminate from here on").
		if msg.phase != progress.PhaseUnknown {
			m.progressPhase = msg.phase
			m.progressCurrent = msg.current
			m.progressTotal = msg.total
		}
		m.gitProgress = msg.detail
		return m, nil

	case progressTickMsg:
		// The animation timer runs for the lifetime of the program; the frame
		// only advances while an operation owns the UI.
		if m.loading {
			m.progressFrame++
		}
		return m, progressTickCmd()

	case reloadStatusMsg:
		m.finishLoading()
		m.skills = msg
		m.applyFilter()
		if m.activeTab == tabSkills {
			m.clampSkillCursor()
		}
		m.notice = i18n.T("notice.upstream_completed")
		return m, m.reloadUpstreamsCmd()

	case upstreamReloadMsg:
		m.finishLoading()
		m.upstreams = msg
		m.clampUpstreamCursor()
		m.notice = i18n.T("notice.upstream_completed")
		return m, nil

	case upstreamAddedMsg:
		if msg.id != m.upstreamOperation || !m.loading {
			return m, nil
		}
		m.finishLoading()
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.addRegisteredUpstream(msg)
		m.view = viewList
		m.notice = i18n.T("notice.upstream_added", msg.url)
		return m, m.startUpstreamScan(msg.url, msg.ref)

	case upstreamProgressMsg:
		if msg.id != m.upstreamOperation || !m.loading {
			return m, nil
		}
		return m.Update(progressMsg{phase: msg.update.Phase, current: msg.update.Current, total: msg.update.Total, detail: msg.update.Text})

	case upstreamScanMsg:
		if msg.id != m.upstreamOperation || !m.loading {
			return m, nil
		}
		m.finishLoading()
		m.updateScannedUpstream(msg)
		if msg.err != nil {
			m.err = fmt.Errorf("%s", i18n.T("error.upstream_scan", msg.url, msg.err.Error()))
		}
		if len(msg.skills) == 0 {
			m.view = viewList
			if msg.err == nil {
				m.notice = i18n.T("notice.upstream_scan_empty", msg.url)
			}
			return m, m.reloadAsync()
		}
		_, _ = m.Update(discoveredSkillsMsg{url: msg.url, ref: msg.ref, skills: msg.skills})
		m.notice = i18n.T("notice.upstream_scan_complete", len(msg.skills))
		return m, m.reloadAsync()

	case discoveredSkillsMsg:
		m.finishLoading()
		m.view = viewModalAdd
		m.addModal.Step = 1
		m.addModal.SourceURL = msg.url
		m.addModal.SourceRef = msg.ref
		m.addModal.SourceInput.SetValue(msg.url)
		m.addModal.DiscoveredSkills = msg.skills
		m.addModal.DiscoveredCursor = 0
		m.addModal.ScrollOffset = 0
		m.addModal.SelectedIndices = make(map[int]bool)
		m.addModal.DiscoveredStatus = make(map[int]DiscoveredItemStatus)
		m.addModal.CustomAliases = make(map[string]string)
		m.addModal.OverwriteSkills = make(map[string]bool)
		m.addModal.ReplaceSource = false
		m.addModal.Force = false
		m.addModal.Renaming = false

		installedMap := make(map[string]model.SkillStatus)
		for _, s := range m.skills {
			installedMap[s.ID] = s
		}

		seenInBatch := make(map[string]int)
		conflictsCount := 0
		for i, sk := range msg.skills {
			if existing, found := installedMap[sk.ID]; found {
				sameSource := existing.Source.Type == model.SourceTypeGit &&
					(strings.TrimSuffix(existing.Source.URL, ".git") == strings.TrimSuffix(msg.url, ".git"))
				if sameSource {
					m.addModal.DiscoveredStatus[i] = DiscoveredItemStatus{
						Tag: DiscoveredTagInstalled,
					}
				} else {
					conflictSrc := existing.Source.URL
					if conflictSrc == "" {
						conflictSrc = string(existing.Source.Type)
					}
					m.addModal.DiscoveredStatus[i] = DiscoveredItemStatus{
						Tag:            DiscoveredTagConflict,
						ConflictSource: conflictSrc,
					}
					conflictsCount++
				}
				m.addModal.SelectedIndices[i] = false
			} else if prevIdx, seen := seenInBatch[sk.ID]; seen {
				m.addModal.DiscoveredStatus[i] = DiscoveredItemStatus{
					Tag:            DiscoveredTagConflict,
					ConflictSource: msg.skills[prevIdx].Path,
				}
				m.addModal.SelectedIndices[i] = false
				conflictsCount++
			} else {
				m.addModal.DiscoveredStatus[i] = DiscoveredItemStatus{
					Tag: DiscoveredTagNew,
				}
				m.addModal.SelectedIndices[i] = true
				seenInBatch[sk.ID] = i
			}
		}

		if conflictsCount > 0 {
			m.addModal.CurrentFilter = AddFilterConflict
		} else {
			m.addModal.CurrentFilter = AddFilterAll
		}
		return m, nil

	case doctorReportMsg:
		m.finishLoading()
		m.doctorReport = msg
		m.notice = i18n.T("notice.doctor_completed")
		if m.doctorReport != nil && len(m.doctorReport.Items) > 0 {
			if m.doctorCursor >= len(m.doctorReport.Items) {
				m.doctorCursor = len(m.doctorReport.Items) - 1
			}
		} else {
			m.doctorCursor = 0
		}
		m.doctorScrollOffset = 0
		return m, nil

	case cacheListMsg:
		m.finishLoading()
		m.caches = msg
		return m, nil

	case diffLoadedMsg:
		m.finishLoading()
		diff := string(msg)
		if diff == "" {
			diff = diffview.NoDifferences
		}
		m.openDiff(diff)
		return m, nil

	case deployConflictMsg:
		m.finishLoading()
		m.err = nil
		m.deployConflictModal = DeployConflictModalState{
			SkillID:        msg.Conflict.SkillID,
			TargetName:     msg.Conflict.Target,
			Format:         msg.Conflict.Format,
			TargetDir:      msg.Conflict.TargetDir,
			IsUntracked:    msg.Conflict.IsUntracked,
			BatchDeployed:  msg.DeployedCount,
			BatchTotal:     msg.TotalCount,
			RemainingQueue: msg.Queue,
		}
		if msg.Conflict.IsProject {
			m.deployConflictModal.ProjectPath = msg.Conflict.Target
		}
		m.view = viewModalDeployConflict
		return m, nil

	case targetDiffLoadedMsg:
		m.finishLoading()
		if msg.err != nil {
			m.err = msg.err
			m.view = viewModalDeployConflict
			return m, nil
		}
		diff := msg.diff
		if strings.TrimSpace(diff) == "" {
			diff = diffview.NoDifferencesTarget
		}
		m.openDiff(diff)
		m.view = viewTargetDiff
		return m, nil

	case modelsEnrichDryRunMsg:
		m.finishLoading()
		if msg.summary == nil || !msg.summary.Modified {
			m.notice = i18n.T("notice.model_config_up_to_date")
			return m, nil
		}
		m.isModelsDiffConfirm = true
		m.modelsDiffSummary = msg.summary
		if len(msg.summary.Candidate) > 0 {
			m.pendingConfigEdit = &pendingConfigEdit{channel: m.currentAgentChannel(), cfgFile: msg.summary.ConfigFile, originalHash: msg.summary.OriginalHash, candidate: append([]byte(nil), msg.summary.Candidate...)}
		}
		m.openDiff(msg.summary.DiffText)
		return m, nil

	case configEditPreviewMsg:
		m.finishLoading()
		if msg.edit == nil || !msg.edit.Modified {
			m.pendingConfigEdit = nil
			m.isModelsDiffConfirm = false
			m.editJSONModal.OriginalHash = ""
			m.notice = i18n.T("notice.model_config_unchanged")
			return m, nil
		}
		m.isModelsDiffConfirm = true
		if m.pendingConfigEdit != nil {
			m.pendingConfigEdit.originalHash = msg.edit.OriginalHash
			m.pendingConfigEdit.cfgFile = msg.edit.ConfigFile
			if m.view == viewModalEditJSON {
				m.editJSONModal.OriginalHash = msg.edit.OriginalHash
				m.editJSONModal.CfgFile = msg.edit.ConfigFile
			}
		}
		m.modelsDiffRulesOnly = false
		m.openDiff(msg.edit.DiffText)
		return m, nil

	case asyncNoticeMsg:
		m.finishLoading()
		m.isModelsDiffConfirm = false
		m.modelsDiffRulesOnly = false
		m.modelsDiffSummary = nil
		m.pendingConfigEdit = nil
		m.notice = string(msg)
		m.err = nil
		m.view = viewList
		m.addModal = newAddModal()
		m.targetModal = newTargetModal()
		m.projectModal = newProjectModal()
		m.groupModal = newGroupModal()
		m.newInput.SetValue("")
		return m, m.reloadAsync()

	case configChangedMsg:
		return m, m.reloadAsync()

	case targetToggledMsg:
		m.finishLoading()
		m.targetDetailModal.Enabled = msg.enabled
		m.notice = msg.notice
		if m.activeTab == tabChannels || m.activeTab == tabProjects {
			m.reloadTargets()
			if m.activeTab == tabChannels {
				m.reloadModels()
			}
		}
		return m, m.reloadAsync()

	case targetDetailMsg:
		m.finishLoading()
		m.targetDetailModal = msg.modal
		m.view = viewModalTargetDetail
		return m, nil

	case targetDetailSkillsMsg:
		m.finishLoading()
		m.targetDetailModal.DeployedSkills = []string(msg)
		m.notice = i18n.T("notice.targets_reloaded")
		return m, nil

	case reloadCompleteMsg:
		s := msg.snapshot
		m.skills = s.skills
		m.deployments = s.deployments
		m.targets = s.targets
		m.targetNames = s.targetNames
		m.agentTargets = s.agentTargets
		m.channelEntries = s.channelEntries
		if m.channelCursor >= len(m.displayedChannels()) {
			m.channelCursor = max(0, len(m.displayedChannels())-1)
		}
		m.projectTargets = s.projectTargets
		m.groups = s.groups
		m.groupNames = s.groupNames
		m.settings = s.settings
		m.caches = s.caches
		m.upstreams = s.upstreams
		for _, u := range m.upstreams {
			if u.URL == m.upstreamDetailModal.Upstream.URL && u.Ref == m.upstreamDetailModal.Upstream.Ref {
				m.upstreamDetailModal.Upstream = u
				break
			}
		}
		m.models = s.models
		m.modelConfigFile = s.modelConfigFile
		m.agentProviders = s.agentProviders
		m.modelOverride = s.modelOverride
		if s.err != nil {
			m.err = s.err
		}
		if m.targetDetailModal.Name != "" {
			m.targetDetailModal.DeployedSkills = s.targetDetailModal.DeployedSkills
		}
		if m.selectedSkills != nil {
			valid := make(map[string]bool, len(m.skills))
			for _, sk := range m.skills {
				valid[sk.ID] = true
			}
			for id := range m.selectedSkills {
				if !valid[id] {
					delete(m.selectedSkills, id)
				}
			}
		}
		m.applyFilter()
		if m.groupCursor >= len(m.groupNames) {
			m.groupCursor = max(0, len(m.groupNames)-1)
		}
		if m.cacheCursor >= len(m.caches) {
			m.cacheCursor = max(0, len(m.caches)-1)
		}
		if m.settingsCursor >= len(m.settings) {
			m.settingsCursor = max(0, len(m.settings)-1)
		}
		return m, nil

	case errMsg:
		m.finishLoading()
		m.isModelsDiffConfirm = false
		m.modelsDiffRulesOnly = false
		m.modelsDiffSummary = nil
		m.pendingConfigEdit = nil
		m.err = msg.err
		return m, nil

	case tea.MouseMsg:
		if m.view == viewModalUpstreamDetail {
			var cmd tea.Cmd
			m.upstreamDetailModal.Viewport, cmd = m.upstreamDetailModal.Viewport.Update(msg)
			return m, cmd
		}
		if m.view == viewModalTargetDetail {
			var cmd tea.Cmd
			m.targetDetailModal.Viewport, cmd = m.targetDetailModal.Viewport.Update(msg)
			return m, cmd
		}
		if m.view == viewModalModelDetail {
			var cmd tea.Cmd
			m.modelDetailModal.Viewport, cmd = m.modelDetailModal.Viewport.Update(msg)
			return m, cmd
		}
		if m.view == viewModalModelConfigFile {
			var cmd tea.Cmd
			m.modelConfigFileModal.Viewport, cmd = m.modelConfigFileModal.Viewport.Update(msg)
			return m, cmd
		}
		if m.view == viewDiff || m.view == viewTargetDiff {
			var cmd tea.Cmd
			m.diffViewport, cmd = m.diffViewport.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		// Intercept Esc to cancel loading, q/ctrl+c to quit, and block all other keys during loading
		if m.loading {
			if msg.String() == "ctrl+c" || msg.String() == "q" {
				if m.cancelOp != nil {
					m.cancelOp()
				}
				return m, tea.Quit
			}
			if msg.String() == "esc" {
				wasScanning := m.upstreamScanActive
				wasEditing := m.view == viewModalEditJSON
				if m.cancelOp != nil {
					m.cancelOp()
				}
				m.upstreamOperation++
				m.editorOperation++
				m.finishLoading()
				m.notice = i18n.T("notice.cancelled")
				if wasScanning {
					m.notice = i18n.T("notice.upstream_scan_cancelled")
				}
				m.err = nil
				m.view = viewList
				if wasEditing {
					m.view = viewModalEditJSON
				}
				m.addModal = newAddModal()
				m.targetModal = newTargetModal()
				m.projectModal = newProjectModal()
				m.groupModal = newGroupModal()
				return m, m.reloadAsync()
			}
			// Block all other operations during loading
			return m, nil
		}

		// Handle specific views
		switch m.view {
		case viewModalAdd:
			return m.handleModalAddKeys(msg)
		case viewModalNew:
			return m.handleModalNewKeys(msg)
		case viewModalAddTarget:
			return m.handleModalAddTargetKeys(msg)
		case viewModalAddProject:
			return m.handleModalAddProjectKeys(msg)
		case viewModalAddGroup:
			return m.handleModalAddGroupKeys(msg)
		case viewModalGroupSkills:
			return m.handleModalGroupSkillsKeys(msg)
		case viewModalConfirm:
			return m.handleModalConfirmKeys(msg)
		case viewModalBatchRemove:
			return m.handleBatchRemoveModalKeys(msg)
		case viewModalEditSetting:
			return m.handleModalEditSettingKeys(msg)
		case viewModalSelectLanguage:
			return m.handleModalSelectLanguageKeys(msg)
		case viewModalSelectWorkspace:
			return m.handleModalSelectWorkspaceKeys(msg)
		case viewModalAddWorkspace:
			return m.handleModalAddWorkspaceKeys(msg)
		case viewModalTargetDetail:
			return m.handleModalTargetDetailKeys(msg)
		case viewModalDeployConflict:
			return m.handleDeployConflictKeys(msg)
		case viewTargetDiff:
			return m.handleTargetDiffKeys(msg)
		case viewModalAddUpstream:
			return m.handleModalAddUpstreamKeys(msg)
		case viewModalEditUpstream:
			return m.handleModalEditUpstreamKeys(msg)
		case viewModalConfirmUpstreamRemove:
			return m.handleModalConfirmUpstreamRemoveKeys(msg)
		case viewModalUpstreamDetail:
			return m.handleModalUpstreamDetailKeys(msg)
		case viewModalModelDetail:
			return m.handleModalModelDetailKeys(msg)
		case viewModalModelConfigFile:
			return m.handleModalModelConfigFileKeys(msg)
		case viewModalProviderDetail:
			return m.handleModalProviderDetailKeys(msg)
		case viewModalEditField:
			return m.handleModalEditFieldKeys(msg)
		case viewModalEditJSON:
			return m.handleModalEditJSONKeys(msg)
		case viewDeploy, viewUndeploy:
			return m.handleDeployKeys(msg)
		case viewDetail:
			return m.handleDetailKeys(msg)
		case viewDiff:
			return m.handleDiffKeys(msg)
		case viewHelp:
			return m.handleHelpKeys(msg)
		case viewSearch:
			if msg.String() == "enter" {
				m.view = viewList
				return m, nil
			}
			if msg.String() == "esc" {
				m.view = viewList
				m.searchInput.SetValue("")
				m.applyFilter()
				return m, nil
			}
			if msg.String() == "up" || msg.String() == "down" {
				m.view = viewList
			} else {
				return m, cmd
			}
		}

		// Global keys in main view (viewList)
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "esc":
			if m.focusDepth == 1 {
				m.focusDepth = 0
				return m, nil
			}
			if m.searchInput.Value() != "" {
				m.searchInput.SetValue("")
				m.applyFilter()
				return m, nil
			}
			if m.activeTab == tabSkills && len(m.selectedSkills) > 0 {
				m.selectedSkills = make(map[string]bool)
				return m, nil
			}
			return m, nil

		case "ctrl+a":
			if m.activeTab == tabSkills {
				vis := m.visibleSkills()
				if len(vis) > 0 {
					if m.selectedSkills == nil {
						m.selectedSkills = make(map[string]bool)
					}
					allSelected := true
					for _, sk := range vis {
						if !m.selectedSkills[sk.ID] {
							allSelected = false
							break
						}
					}
					if allSelected {
						for _, sk := range vis {
							delete(m.selectedSkills, sk.ID)
						}
					} else {
						for _, sk := range vis {
							m.selectedSkills[sk.ID] = true
						}
					}
					return m, nil
				}
			}

		case "tab", "shift+tab":
			m.cycleFocusDepth()
			return m, nil

		case "right", "l":
			return m, m.moveFocusHorizontal(1)

		case "left", "h":
			return m, m.moveFocusHorizontal(-1)

		case "]":
			if m.focusDepth == 0 {
				return m, m.cycleActiveTab(1)
			}
			return m, nil

		case "[":
			if m.focusDepth == 0 {
				return m, m.cycleActiveTab(-1)
			}
			return m, nil

		case "1":
			return m, m.setActiveTab(tabDashboard)
		case "2":
			return m, m.setActiveTab(tabUpstreams)
		case "3":
			return m, m.setActiveTab(tabSkills)
		case "4", "m":
			return m, m.setActiveTab(tabChannels)
		case "5":
			return m, m.setActiveTab(tabProjects)
		case "6":
			return m, m.setActiveTab(tabSystem)
		case "7":
			return m, m.setActiveTab(tabSettings)
		case "s":
			if m.activeTab == tabUpstreams {
				return m.handleUpstreamDiscover()
			}
			return m, m.setActiveTab(tabSettings)

		case "up", "k":
			m.moveCursorUp()
		case "down", "j":
			m.moveCursorDown()

		case "pgup", "ctrl+u":
			if m.activeTab == tabDashboard {
				m.dashboardViewport.HalfViewUp()
			} else if m.activeTab == tabSystem && m.systemSubTab == systemTabDoctor {
				m.moveDoctorCursor(-m.listRows(4))
			}
		case "pgdown", "ctrl+d":
			if m.activeTab == tabDashboard {
				m.dashboardViewport.HalfViewDown()
			} else if m.activeTab == tabSystem && m.systemSubTab == systemTabDoctor {
				m.moveDoctorCursor(m.listRows(4))
			}
		case "g", "home":
			if m.activeTab == tabDashboard {
				m.dashboardViewport.GotoTop()
			} else if m.activeTab == tabSystem && m.systemSubTab == systemTabDoctor {
				m.doctorCursor = 0
			}
		case "G", "end":
			if m.activeTab == tabDashboard {
				m.dashboardViewport.GotoBottom()
			} else if m.activeTab == tabSystem && m.systemSubTab == systemTabDoctor && m.doctorReport != nil {
				if n := len(m.doctorReport.Items); n > 0 {
					m.doctorCursor = n - 1
				}
			}

		case "/": // Search
			m.view = viewSearch
			m.searchInput.Focus()

		case " ", "t": // Space/t toggles settings/targets, collapses provider headers, or selects skills
			if m.activeTab == tabChannels {
				if msg.String() == "t" {
					return m.toggleChannelEnabled()
				}
				return m.handleModelsSpace()
			}
			if m.activeTab == tabProjects {
				return m.toggleProjectEnabled()
			}
			if m.activeTab == tabSkills && m.skillSubTab == skillTabSkills && msg.String() == " " {
				vis := m.visibleSkills()
				if len(vis) > 0 && m.cursor < len(vis) {
					sk := vis[m.cursor]
					if m.selectedSkills == nil {
						m.selectedSkills = make(map[string]bool)
					}
					if m.selectedSkills[sk.ID] {
						delete(m.selectedSkills, sk.ID)
					} else {
						m.selectedSkills[sk.ID] = true
					}
				}
				return m, nil
			}
			settings := m.displayedSettings()
			if m.activeTab == tabSettings && len(settings) > 0 && m.settingsCursor < len(settings) && settings[m.settingsCursor].Key == "language" {
				nextLang := m.nextLanguage()
				langCmd := m.applyLanguage(nextLang)
				m.notice = fmt.Sprintf(i18n.T("notice.language_changed"), languageLabel(nextLang))
				return m, langCmd
			}

		case "f": // Filter channels/projects by enabled/disabled status
			if m.activeTab == tabChannels || m.activeTab == tabProjects {
				m.targetFilter = (m.targetFilter + 1) % 3
				if m.activeTab == tabChannels {
					shown := m.displayedChannels()
					if m.channelCursor >= len(shown) {
						m.channelCursor = max(0, len(shown)-1)
					}
					if len(shown) <= 1 && m.focusDepth == 1 {
						m.focusDepth = 0
					}
					m.reloadModels()
				} else {
					m.clampTargetCursors()
				}
				return m, nil
			}

		case "e": // Edit group skills, setting, upstream, or enrich models
			if m.activeTab == tabUpstreams {
				displayed := m.displayedUpstreams()
				if len(displayed) > 0 && m.upstreamCursor < len(displayed) {
					u := displayed[m.upstreamCursor]
					m.view = viewModalEditUpstream
					m.editUpstreamModal = newEditUpstreamModal(u.URL, u.Ref, u.Name, model.ScanConfig{Roots: u.ScanRoots, Exclude: u.ScanExclude})
					return m, nil
				}
			} else if m.activeTab == tabChannels {
				return m.handleModelsDiff()
			} else if m.activeTab == tabSkills && m.skillSubTab == skillTabGroups {
				return m.openGroupSkillsModal()
			} else if m.activeTab == tabSettings {
				settings := m.displayedSettings()
				if len(settings) > 0 && m.settingsCursor < len(settings) {
					if settings[m.settingsCursor].Key == "workspace" {
						return m.openAddWorkspaceModal()
					}
					return m.handleSettingsEnter()
				}
			}

		case "w": // Switch/modify workspace in settings
			if m.activeTab == tabSettings {
				return m.openSelectWorkspaceModal()
			}

		case "i": // Unified detail key: open the selected object's detail
			return m.handleDetailKey()

		case "I": // Page-context detail: current channel, or initialize workspace in settings
			if m.activeTab == tabChannels {
				return m.handleOpenChannelDetail()
			}
			if m.activeTab == tabSettings {
				return m.handleInitWorkspace()
			}

		case "R": // Reset defaults in settings, or regenerate models from rules
			if m.activeTab == tabSettings {
				m.confirmModal = ConfirmationModalState{
					Title:   i18n.T("confirm.reset_defaults.title"),
					Message: i18n.T("confirm.reset_defaults.msg"),
					Action:  "reset-defaults",
				}
				m.view = viewModalConfirm
				return m, nil
			}
			if m.activeTab == tabChannels {
				return m.handleModelsRulesOnly()
			}

		case "a", "+": // Add skill, target, project, group, upstream, or workspace
			if m.activeTab == tabUpstreams {
				m.view = viewModalAddUpstream
				m.addUpstreamModal = newAddUpstreamModal()
				return m, nil
			} else if m.activeTab == tabChannels {
				m.view = viewModalAddTarget
				m.targetModal = newTargetModal()
			} else if m.activeTab == tabProjects {
				m.view = viewModalAddProject
				m.projectModal = newProjectModal()
			} else if m.activeTab == tabSkills && m.skillSubTab == skillTabGroups {
				m.view = viewModalAddGroup
				m.groupModal = newGroupModal()
			} else if m.activeTab == tabSettings {
				return m.openAddWorkspaceModal()
			} else {
				m.view = viewModalAdd
				m.addModal = newAddModal(m.upstreams...)
				return m, nil
			}

		case "n": // New skill
			if m.activeTab == tabSkills && m.skillSubTab == skillTabSkills {
				m.view = viewModalNew
				m.newInput.SetValue("")
				m.newInput.Focus()
			}

		case "A": // Adopt unmanaged skill
			if m.activeTab == tabSkills && m.skillSubTab == skillTabSkills {
				selected, ok := m.selectedSkill()
				if !ok || selected.ManagedStatus != model.StatusUnmanaged {
					break
				}
				skID := selected.ID
				ctx, cancel := context.WithCancel(m.ctx)
				m.cancelOp = cancel
				m.loading = true
				return m, func() tea.Msg {
					err := m.service.Adopt(ctx, skID)
					if err != nil {
						if ctx.Err() != nil {
							return nil
						}
						return errMsg{err: err}
					}
					if ctx.Err() != nil {
						return nil
					}
					return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.adopted"), skID))
				}
			}

		case "x", "delete": // Remove
			if m.activeTab == tabUpstreams {
				displayed := m.displayedUpstreams()
				if len(displayed) > 0 && m.upstreamCursor < len(displayed) {
					u := displayed[m.upstreamCursor]
					m.confirmUpstreamRemoveModal = newConfirmUpstreamRemoveModal(u.URL, u.Skills)
					m.view = viewModalConfirmUpstreamRemove
					return m, nil
				}
				return m, nil
			} else if m.activeTab == tabChannels {
				if e, ok := m.currentChannelEntry(); ok {
					m.confirmModal = ConfirmationModalState{
						Title:   i18n.T("confirm.remove_target.title"),
						Message: fmt.Sprintf(i18n.T("confirm.remove_target.msg"), e.Name),
						Action:  "remove-target",
						Target:  e.Name,
					}
					m.view = viewModalConfirm
				}
				return m, nil
			} else if m.activeTab == tabProjects {
				projs := m.displayedProjectTargets()
				if len(projs) > 0 && m.projectCursor < len(projs) {
					proj := projs[m.projectCursor]
					m.confirmModal = ConfirmationModalState{
						Title:   i18n.T("confirm.remove_project.title"),
						Message: fmt.Sprintf(i18n.T("confirm.remove_project.msg"), proj.Path),
						Action:  "remove-project",
						Target:  proj.RawPath,
					}
					m.view = viewModalConfirm
				}
				return m, nil
			} else if m.activeTab == tabSkills && m.skillSubTab == skillTabGroups {
				groups := m.displayedGroupNames()
				if len(groups) > 0 && m.groupCursor < len(groups) {
					gName := groups[m.groupCursor]
					m.confirmModal = ConfirmationModalState{
						Title:   i18n.T("confirm.remove_group.title"),
						Message: fmt.Sprintf(i18n.T("confirm.remove_group.msg"), gName),
						Action:  "remove-group",
						Target:  gName,
					}
					m.view = viewModalConfirm
				}
				return m, nil
			} else if m.activeTab == tabSkills {
				var selectedIDs []string
				if len(m.selectedSkills) > 0 {
					for _, sk := range m.skills {
						if m.selectedSkills[sk.ID] {
							selectedIDs = append(selectedIDs, sk.ID)
						}
					}
				}

				if len(selectedIDs) > 0 {
					allDeps := m.deployments
					depMap := make(map[string][]string)
					hasModified := false
					for _, id := range selectedIDs {
						if deps, ok := allDeps[id]; ok && len(deps) > 0 {
							depMap[id] = deps
						}
						for _, sk := range m.skills {
							if sk.ID == id && sk.ManagedStatus == model.StatusModified {
								hasModified = true
								break
							}
						}
					}

					m.confirmModal = ConfirmationModalState{
						Title:       fmt.Sprintf(i18n.T("confirm.remove_skills_batch.title"), len(selectedIDs)),
						Message:     fmt.Sprintf(i18n.T("confirm.remove_skills_batch.msg"), len(selectedIDs)),
						Danger:      true,
						AllowForce:  hasModified,
						Action:      "remove-skills-batch",
						SkillIDs:    selectedIDs,
						Deployments: depMap,
					}
					m.view = viewModalConfirm
					return m, nil
				}

				sk, ok := m.selectedSkill()
				if !ok {
					return m, nil
				}
				deps := m.deployments[sk.ID]
				msg := fmt.Sprintf(i18n.T("confirm.remove_skill.msg"), sk.ID)
				depMap := make(map[string][]string)
				if len(deps) > 0 {
					depMap[sk.ID] = deps
					msg += "\n\n" + fmt.Sprintf(i18n.T("confirm.remove_skill.deployed_warning"), strings.Join(deps, ", "))
				}
				m.confirmModal = ConfirmationModalState{
					Title:       i18n.T("confirm.remove_skill.title"),
					Message:     msg,
					Danger:      true,
					AllowForce:  sk.ManagedStatus == model.StatusModified,
					Action:      "remove-skill",
					SkillID:     sk.ID,
					Deployments: depMap,
				}
				m.view = viewModalConfirm
			}

		case "X": // Batch removal modal (by regex, source, or channel)
			if m.activeTab == tabSkills && m.skillSubTab == skillTabSkills {
				return m.openBatchRemoveModal()
			}

		case "r": // Refresh
			if m.activeTab == tabProjects {
				m.notice = i18n.T("notice.targets_reloaded")
				m.reloadTargets()
				return m, nil
			}
			if m.activeTab == tabSkills && m.skillSubTab == skillTabGroups {
				m.notice = i18n.T("notice.groups_reloaded")
				m.reloadGroups()
				return m, nil
			}
			if m.activeTab == tabSystem && m.systemSubTab == systemTabCache {
				m.notice = i18n.T("notice.cache_reloaded")
				return m, m.reloadCacheCmd()
			}
			if m.activeTab == tabChannels {
				m.reloadTargets()
				entry, ok := m.currentChannelEntry()
				if !ok || !entry.Supported {
					m.reloadModels()
					m.notice = i18n.T("notice.targets_reloaded")
					return m, nil
				}
				ctx, cancel := context.WithCancel(m.ctx)
				m.cancelOp = cancel
				m.loading = true
				m.notice = i18n.T("notice.refreshing_modelsdev")
				channel := m.currentAgentChannel()
				cfgFile := m.modelConfigFile
				return m, func() tea.Msg {
					_, err := m.service.EnrichModelConfig(ctx, channel, agent.EnrichOptions{
						FilePath:     cfgFile,
						RefreshCache: true,
						DryRun:       true,
					})
					if err != nil {
						if ctx.Err() != nil {
							return nil
						}
						return errMsg{err: err}
					}
					if ctx.Err() != nil {
						return nil
					}
					return asyncNoticeMsg(i18n.T("notice.modelsdev_refreshed"))
				}
			}
			if m.activeTab == tabSettings {
				m.notice = i18n.T("notice.settings_reloaded")
				return m, m.reloadAsync()
			}
			if m.activeTab == tabSystem && m.systemSubTab == systemTabDoctor {
				return m, m.doctorCmd()
			}
			if m.activeTab == tabUpstreams {
				m.notice = i18n.T("notice.upstreams_cache_refreshed")
				return m, m.reloadAsync()
			}
			if m.activeTab == tabDashboard {
				m.notice = i18n.T("notice.dashboard_refreshed")
				return m, m.reloadAsync()
			}
			ctx, cancel := context.WithCancel(m.ctx)
			m.cancelOp = cancel
			m.loading = true
			m.notice = i18n.T("notice.checking_upstream")
			return m, func() tea.Msg {
				res, err := m.service.Check(ctx, nil, m.progressCallback())
				if err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return errMsg{err: err}
				}
				if ctx.Err() != nil {
					return nil
				}
				return reloadStatusMsg(res)
			}

		case "u": // Update single or multi-selected
			if m.activeTab == tabDashboard {
				updatable := m.updatableSkills()
				if len(updatable) == 0 {
					m.notice = i18n.T("notice.all_skills_up_to_date")
					return m, nil
				}
				ctx, cancel := context.WithCancel(m.ctx)
				m.cancelOp = cancel
				m.loading = true
				m.notice = i18n.T("notice.updating_all_skills", len(updatable))
				return m, func() tea.Msg {
					var count int
					for i, sk := range updatable {
						if ctx.Err() != nil {
							return nil
						}
						m.sendProgress(progress.PhaseUpdate, i+1, len(updatable), sk.ID)
						if _, err := m.service.Update(ctx, sk.ID, false, false); err == nil {
							count++
						}
					}
					if ctx.Err() != nil {
						return nil
					}
					return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.updated_all_skills"), count))
				}
			}
			if m.activeTab == tabUpstreams {
				displayed := m.displayedUpstreams()
				if len(displayed) > 0 && m.upstreamCursor < len(displayed) {
					selectedUp := displayed[m.upstreamCursor]
					if len(selectedUp.Skills) == 0 {
						m.notice = i18n.T("notice.upstream_no_imported_skills")
						return m, nil
					}
					ctx, cancel := context.WithCancel(m.ctx)
					m.cancelOp = cancel
					m.loading = true
					m.notice = fmt.Sprintf(i18n.T("notice.updating_upstream"), selectedUp.URL)
					return m, func() tea.Msg {
						var count int
						total := len(selectedUp.Skills)
						for i, skID := range selectedUp.Skills {
							if ctx.Err() != nil {
								return nil
							}
							m.sendProgress(progress.PhaseUpdate, i+1, total, skID)
							if _, err := m.service.Update(ctx, skID, false, false); err == nil {
								count++
							}
						}
						if ctx.Err() != nil {
							return nil
						}
						return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.updated_all_skills"), count))
					}
				}
				return m, nil
			}
			if m.activeTab == tabSkills && m.skillSubTab == skillTabSkills {
				var toUpdate []string
				if len(m.selectedSkills) > 0 {
					for _, sk := range m.skills {
						if m.selectedSkills[sk.ID] {
							toUpdate = append(toUpdate, sk.ID)
						}
					}
				} else if sk, ok := m.selectedSkill(); ok {
					toUpdate = append(toUpdate, sk.ID)
				}

				if len(toUpdate) == 1 {
					skID := toUpdate[0]
					var curSk *model.SkillStatus
					for i := range m.skills {
						if m.skills[i].ID == skID {
							curSk = &m.skills[i]
							break
						}
					}
					if curSk != nil && curSk.ManagedStatus == model.StatusModified {
						m.confirmModal = ConfirmationModalState{
							Title:      i18n.T("confirm.local_mods.title"),
							Message:    fmt.Sprintf(i18n.T("confirm.local_mods.msg"), skID),
							Danger:     true,
							AllowForce: true,
							Action:     "update-skill",
							SkillID:    skID,
						}
						m.view = viewModalConfirm
						return m, nil
					}
					ctx, cancel := context.WithCancel(m.ctx)
					m.cancelOp = cancel
					m.loading = true
					m.notice = i18n.T("notice.updating_skill", skID)
					return m, func() tea.Msg {
						_, err := m.service.Update(ctx, skID, false, false, m.progressCallback())
						if err != nil {
							if ctx.Err() != nil {
								return nil
							}
							return errMsg{err: err}
						}
						if ctx.Err() != nil {
							return nil
						}
						return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.updated_skill"), skID))
					}
				} else if len(toUpdate) > 1 {
					ctx, cancel := context.WithCancel(m.ctx)
					m.cancelOp = cancel
					m.loading = true
					m.notice = i18n.T("notice.updating_selected_skills", len(toUpdate))
					return m, func() tea.Msg {
						var count int
						total := len(toUpdate)
						for i, skID := range toUpdate {
							if ctx.Err() != nil {
								return nil
							}
							m.sendProgress(progress.PhaseUpdate, i+1, total, skID)
							if _, err := m.service.Update(ctx, skID, false, false); err == nil {
								count++
							}
						}
						if ctx.Err() != nil {
							return nil
						}
						return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.updated_all_skills"), count))
					}
				}
			}

		case "U": // Update all
			ctx, cancel := context.WithCancel(m.ctx)
			m.cancelOp = cancel
			m.loading = true
			m.notice = i18n.T("notice.updating_all_pending")
			return m, func() tea.Msg {
				cb := m.progressCallback()
				statuses, err := m.service.Check(ctx, nil, cb)
				if err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return errMsg{err: err}
				}
				var pending []string
				for _, st := range statuses {
					if st.Upstream == model.UpstreamUpdateAvailable || st.Upstream == model.UpstreamSourceChanged {
						if st.ManagedStatus != model.StatusModified {
							pending = append(pending, st.ID)
						}
					}
				}
				var count int
				for i, id := range pending {
					if ctx.Err() != nil {
						return nil
					}
					m.sendProgress(progress.PhaseUpdate, i+1, len(pending), id)
					if _, err := m.service.Update(ctx, id, false, false); err == nil {
						count++
					} else if ctx.Err() != nil {
						return nil
					}
				}
				if ctx.Err() != nil {
					return nil
				}
				return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.updated_all_skills"), count))
			}

		case "d": // Deploy
			if m.activeTab != tabSkills && m.activeTab != tabChannels && m.activeTab != tabProjects {
				return m, nil
			}
			if len(m.skills) == 0 {
				m.notice = i18n.T("notice.no_skills_prompt")
				return m, nil
			}
			recentProjects, _ := m.service.ProjectList()
			if m.activeTab == tabSkills && m.skillSubTab == skillTabGroups {
				groups := m.displayedGroupNames()
				if len(groups) > 0 && m.groupCursor < len(groups) {
					grpName := groups[m.groupCursor]
					grp := m.groups[grpName]
					m.deployModal = newDeployModal(false, m.skills, "", m.targetNames, m.targets, m.service.Profiles(), recentProjects, m.service.IsProjectEnabled)
					m.deployModal.SelectedSkills = make(map[string]bool)
					for _, skID := range grp.Skills {
						m.deployModal.SelectedSkills[skID] = true
					}
					m.view = viewDeploy
					m.err = nil
					return m, nil
				}
			}
			if m.activeTab == tabChannels {
				e, ok := m.currentChannelEntry()
				if ok {
					if !e.Target.Enabled {
						m.notice = fmt.Sprintf(i18n.T("notice.target_disabled_cannot_deploy"), e.Name)
						return m, nil
					}
					m.deployModal = newDeployModal(false, m.skills, "", m.targetNames, m.targets, m.service.Profiles(), recentProjects, m.service.IsProjectEnabled).
						WithLockedTarget(e.Name, "agent", "")
					m.view = viewDeploy
					m.err = nil
					return m, nil
				}
			}
			if m.activeTab == tabProjects {
				projs := m.displayedProjectTargets()
				if len(projs) > 0 && m.projectCursor < len(projs) {
					p := projs[m.projectCursor]
					if !p.Enabled {
						m.notice = fmt.Sprintf(i18n.T("notice.target_disabled_cannot_deploy"), p.Name)
						return m, nil
					}
					m.deployModal = newDeployModal(false, m.skills, "", m.targetNames, m.targets, m.service.Profiles(), recentProjects, m.service.IsProjectEnabled).
						WithLockedTarget(p.Name, "project", p.RawPath)
					m.view = viewDeploy
					m.err = nil
					return m, nil
				}
			}
			var curID string
			if selected, ok := m.selectedSkill(); ok && m.activeTab == tabSkills && m.skillSubTab == skillTabSkills {
				curID = selected.ID
			}
			m.deployModal = newDeployModal(false, m.skills, curID, m.targetNames, m.targets, m.service.Profiles(), recentProjects, m.service.IsProjectEnabled)
			if len(m.selectedSkills) > 0 && m.activeTab == tabSkills && m.skillSubTab == skillTabSkills {
				m.deployModal.SelectedSkills = make(map[string]bool)
				for k, v := range m.selectedSkills {
					if v {
						m.deployModal.SelectedSkills[k] = true
					}
				}
			}
			m.view = viewDeploy
			m.err = nil

		case "D": // Undeploy
			if m.activeTab != tabSkills && m.activeTab != tabChannels && m.activeTab != tabProjects {
				return m, nil
			}
			if len(m.skills) == 0 {
				m.notice = i18n.T("notice.no_skills_prompt")
				return m, nil
			}
			recentProjects, _ := m.service.ProjectList()
			if m.activeTab == tabSkills && m.skillSubTab == skillTabGroups {
				groups := m.displayedGroupNames()
				if len(groups) > 0 && m.groupCursor < len(groups) {
					grpName := groups[m.groupCursor]
					grp := m.groups[grpName]
					m.deployModal = newDeployModal(true, m.skills, "", m.targetNames, m.targets, m.service.Profiles(), recentProjects, m.service.IsProjectEnabled)
					m.deployModal.SelectedSkills = make(map[string]bool)
					for _, skID := range grp.Skills {
						m.deployModal.SelectedSkills[skID] = true
					}
					m.view = viewUndeploy
					m.err = nil
					return m, nil
				}
			}
			if m.activeTab == tabChannels {
				e, ok := m.currentChannelEntry()
				if ok {
					deployed := m.service.TargetDeployedSkills(e.Name)
					if len(deployed) == 0 {
						m.notice = fmt.Sprintf(i18n.T("notice.no_deployed_skills_in_target"), e.Name)
						return m, nil
					}
					targetSkills := m.skillsForTargetUndeploy(deployed)
					m.deployModal = newDeployModal(true, targetSkills, "", m.targetNames, m.targets, m.service.Profiles(), recentProjects, m.service.IsProjectEnabled).
						WithLockedTarget(e.Name, "agent", "")
					m.view = viewUndeploy
					m.err = nil
					return m, nil
				}
			}
			if m.activeTab == tabProjects {
				projs := m.displayedProjectTargets()
				if len(projs) > 0 && m.projectCursor < len(projs) {
					p := projs[m.projectCursor]
					deployed := m.service.ProjectDeployedSkills(p.RawPath)
					if len(deployed) == 0 {
						m.notice = fmt.Sprintf(i18n.T("notice.no_deployed_skills_in_target"), p.Name)
						return m, nil
					}
					targetSkills := m.skillsForTargetUndeploy(deployed)
					m.deployModal = newDeployModal(true, targetSkills, "", m.targetNames, m.targets, m.service.Profiles(), recentProjects, m.service.IsProjectEnabled).
						WithLockedTarget(p.Name, "project", p.RawPath)
					m.view = viewUndeploy
					m.err = nil
					return m, nil
				}
			}
			var curID string
			if selected, ok := m.selectedSkill(); ok && m.activeTab == tabSkills && m.skillSubTab == skillTabSkills {
				curID = selected.ID
			}
			m.deployModal = newDeployModal(true, m.skills, curID, m.targetNames, m.targets, m.service.Profiles(), recentProjects, m.service.IsProjectEnabled)
			if len(m.selectedSkills) > 0 && m.activeTab == tabSkills && m.skillSubTab == skillTabSkills {
				m.deployModal.SelectedSkills = make(map[string]bool)
				for k, v := range m.selectedSkills {
					if v {
						m.deployModal.SelectedSkills[k] = true
					}
				}
			}
			m.view = viewUndeploy
			m.err = nil

		case "v": // Skill diff
			if sk, ok := m.selectedSkill(); ok && m.activeTab == tabSkills && m.skillSubTab == skillTabSkills {
				skID := sk.ID
				ctx, cancel := context.WithCancel(m.ctx)
				m.cancelOp = cancel
				m.loading = true
				m.notice = i18n.T("notice.loading_diff_skill", skID)
				return m, func() tea.Msg {
					diff, err := m.service.Diff(ctx, skID)
					if err != nil {
						if ctx.Err() != nil {
							return nil
						}
						return errMsg{err: err}
					}
					return diffLoadedMsg(diff)
				}
			}

		case "o": // Toggle model enrich mode
			if m.activeTab == tabChannels {
				m.modelOverride = !m.modelOverride
				mode := model.ModelEnrichModeIncremental
				if m.modelOverride {
					mode = model.ModelEnrichModeRemote
					m.notice = i18n.T("notice.model_mode_remote")
				} else {
					m.notice = i18n.T("notice.model_mode_incremental")
				}
				return m, m.configSetCmd("model_enrich_mode", mode)
			}

		case "p": // Prune Cache
			if m.activeTab == tabSystem && m.systemSubTab == systemTabCache {
				ctx, cancel := context.WithCancel(m.ctx)
				m.cancelOp = cancel
				m.loading = true
				return m, func() tea.Msg {
					count, bytesFreed, err := m.service.CachePrune(ctx)
					if err != nil {
						if ctx.Err() != nil {
							return nil
						}
						return errMsg{err: err}
					}
					if ctx.Err() != nil {
						return nil
					}
					return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.pruned_cache"), count, float64(bytesFreed)/(1024*1024)))
				}
			}

		case "c", "C": // Clean Cache, Clean Leftovers, Check Upstreams, or View Model Config
			if m.activeTab == tabUpstreams {
				ctx, cancel := context.WithCancel(m.ctx)
				m.cancelOp = cancel
				m.loading = true
				m.notice = i18n.T("notice.checking_upstream")
				return m, func() tea.Msg {
					res, err := m.service.UpstreamCheck(ctx, "", m.progressCallback())
					if err != nil {
						if ctx.Err() != nil {
							return nil
						}
						return errMsg{err: err}
					}
					if ctx.Err() != nil {
						return nil
					}
					return upstreamReloadMsg(res)
				}
			} else if m.activeTab == tabSystem && m.systemSubTab == systemTabCache {
				m.confirmModal = ConfirmationModalState{
					Title:   i18n.T("confirm.clean_cache.title"),
					Message: i18n.T("confirm.clean_cache.msg"),
					Danger:  true,
					Action:  "clean-cache",
				}
				m.view = viewModalConfirm
			} else if m.activeTab == tabSystem && m.systemSubTab == systemTabDoctor {
				return m, func() tea.Msg {
					cleaned, _ := m.service.CleanLeftovers()
					return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.clean_leftovers"), cleaned))
				}
			} else if m.activeTab == tabChannels {
				return m.handleOpenModelConfigFile()
			}

		case "?":
			if m.view == viewHelp {
				m.view = viewList
			} else {
				m.view = viewHelp
				m.helpViewport.GotoTop()
			}
		}
	}

	return m, cmd
}

func (m *Model) onTabChanged() tea.Cmd {
	m.applyFilter()
	switch m.activeTab {
	case tabDashboard:
		// Hashing every managed skill is expensive; keep it off the main loop.
		return m.reloadAsync()
	case tabSkills:
		m.reloadGroups()
		return m.reloadAsync()
	case tabChannels:
		m.reloadTargets()
		m.reloadModels()
	case tabProjects:
		m.reloadTargets()
	case tabSettings:
		m.reloadSettings()
	case tabSystem:
		return m.reloadCacheCmd()
	}
	return nil
}

// reloadCacheCmd computes cache sizes off the main loop.
func (m *Model) reloadCacheCmd() tea.Cmd {
	return func() tea.Msg {
		if m.service == nil {
			return nil
		}
		caches, err := m.service.CacheList()
		if err != nil {
			return nil
		}
		return cacheListMsg(caches)
	}
}

// doctorCmd runs the diagnostic report off the main loop.
func (m *Model) doctorCmd() tea.Cmd {
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancelOp = cancel
	m.loading = true
	m.notice = i18n.T("notice.running_doctor")
	return func() tea.Msg {
		rep, err := m.service.Doctor(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return errMsg{err: err}
		}
		if ctx.Err() != nil {
			return nil
		}
		return doctorReportMsg(rep)
	}
}

func (m *Model) moveCursorUp() {
	switch m.activeTab {
	case tabDashboard:
		m.dashboardViewport.LineUp(1)
	case tabUpstreams:
		if m.upstreamCursor > 0 {
			m.upstreamCursor--
		}
	case tabChannels:
		m.moveModelRow(-1)
	case tabProjects:
		if m.projectCursor > 0 {
			m.projectCursor--
		}
	case tabSkills:
		if m.skillSubTab == skillTabGroups {
			if m.groupCursor > 0 {
				m.groupCursor--
			}
		} else if m.cursor > 0 {
			m.cursor--
		}
	case tabSystem:
		if m.systemSubTab == systemTabCache {
			if m.cacheCursor > 0 {
				m.cacheCursor--
			}
		} else if m.doctorCursor > 0 {
			m.doctorCursor--
		}
	case tabSettings:
		if m.settingsCursor > 0 {
			m.settingsCursor--
		}
	}
}

func (m *Model) toggleChannelEnabled() (tea.Model, tea.Cmd) {
	e, ok := m.currentChannelEntry()
	if !ok {
		return m, nil
	}
	newVal := !e.Target.Enabled
	notice := fmt.Sprintf(i18n.T("notice.target_disabled"), e.Name)
	if newVal {
		notice = fmt.Sprintf(i18n.T("notice.target_enabled"), e.Name)
	}
	return m, m.asyncWrite(notice, func(ctx context.Context) error {
		return m.service.SetTargetEnabled(e.Name, newVal)
	})
}

func (m *Model) toggleProjectEnabled() (tea.Model, tea.Cmd) {
	projs := m.displayedProjectTargets()
	if len(projs) == 0 || m.projectCursor >= len(projs) {
		return m, nil
	}
	p := projs[m.projectCursor]
	newVal := !p.Enabled
	notice := fmt.Sprintf(i18n.T("notice.project_disabled"), p.Name)
	if newVal {
		notice = fmt.Sprintf(i18n.T("notice.project_enabled"), p.Name)
	}
	return m, m.asyncWrite(notice, func(ctx context.Context) error {
		return m.service.SetProjectEnabled(p.RawPath, newVal)
	})
}

func (m *Model) moveCursorDown() {
	switch m.activeTab {
	case tabDashboard:
		m.dashboardViewport.LineDown(1)
	case tabUpstreams:
		upstreams := m.displayedUpstreams()
		if m.upstreamCursor < len(upstreams)-1 {
			m.upstreamCursor++
		}
	case tabChannels:
		m.moveModelRow(1)
	case tabProjects:
		projs := m.displayedProjectTargets()
		if m.projectCursor < len(projs)-1 {
			m.projectCursor++
		}
	case tabSkills:
		if m.skillSubTab == skillTabGroups {
			groups := m.displayedGroupNames()
			if m.groupCursor < len(groups)-1 {
				m.groupCursor++
			}
		} else if m.cursor < len(m.visibleSkills())-1 {
			m.cursor++
		}
	case tabSystem:
		if m.systemSubTab == systemTabCache {
			caches := m.displayedCaches()
			if m.cacheCursor < len(caches)-1 {
				m.cacheCursor++
			}
		} else if m.doctorReport != nil && m.doctorCursor < len(m.doctorReport.Items)-1 {
			m.doctorCursor++
		}
	case tabSettings:
		settings := m.displayedSettings()
		if m.settingsCursor < len(settings)-1 {
			m.settingsCursor++
		}
	}
}

// moveDoctorCursor moves the diagnostics selection by delta, clamped to range.
func (m *Model) moveDoctorCursor(delta int) {
	if m.doctorReport == nil || len(m.doctorReport.Items) == 0 {
		m.doctorCursor = 0
		return
	}
	next := m.doctorCursor + delta
	if next < 0 {
		next = 0
	}
	if next >= len(m.doctorReport.Items) {
		next = len(m.doctorReport.Items) - 1
	}
	m.doctorCursor = next
}

// moveModelRow moves the row cursor across provider headers and model rows.
func (m *Model) moveModelRow(delta int) {
	rows := m.getModelDisplayRows()
	if len(rows) == 0 {
		m.modelRowCursor = 0
		return
	}
	next := m.modelRowCursor + delta
	if next < 0 {
		next = 0
	}
	if next >= len(rows) {
		next = len(rows) - 1
	}
	m.modelRowCursor = next
	if !rows[next].isHeader {
		m.modelCursor = rows[next].modelIndex
	}
}

// handleDetailKey opens the detail view for the currently selected object.
// It is the single detail entry point for all main list pages, bound to "i".
func (m *Model) handleDetailKey() (tea.Model, tea.Cmd) {
	switch m.activeTab {
	case tabUpstreams:
		displayed := m.displayedUpstreams()
		if len(displayed) > 0 && m.upstreamCursor < len(displayed) {
			u := displayed[m.upstreamCursor]
			m.upstreamDetailModal = newUpstreamDetailModal(u)
			m.view = viewModalUpstreamDetail
		}
		return m, nil

	case tabSkills:
		if m.skillSubTab != skillTabSkills {
			return m, nil
		}
		selected, ok := m.selectedSkill()
		if !ok {
			return m, nil
		}
		st, md, err := m.service.Show(m.ctx, selected.ID)
		if err != nil {
			m.err = err
		} else {
			m.openSkillDetail(st, md)
		}
		return m, nil

	case tabChannels:
		return m.handleChannelRowDetail()

	case tabProjects:
		return m.handleOpenProjectDetail()
	}
	return m, nil
}

// handleChannelRowDetail opens the detail for the highlighted model-table row:
// provider headers open the provider detail, model rows open the model detail.
func (m *Model) handleChannelRowDetail() (tea.Model, tea.Cmd) {
	row, ok := m.currentModelRow()
	if !ok {
		return m, nil
	}
	if row.isHeader {
		return m.handleOpenProviderDetail(row.providerID)
	}
	return m.handleOpenModelDetail()
}

// openGroupSkillsModal opens the skill-assignment modal for the selected group.
func (m *Model) openGroupSkillsModal() (tea.Model, tea.Cmd) {
	groups := m.displayedGroupNames()
	if len(groups) == 0 || m.groupCursor >= len(groups) {
		return m, nil
	}
	name := groups[m.groupCursor]
	currSkills := m.groups[name].Skills
	m.groupSkillsModal = newGroupSkillsModal(name, m.skills, currSkills)
	m.view = viewModalGroupSkills
	return m, nil
}

func (m *Model) handleUpstreamDiscover() (tea.Model, tea.Cmd) {
	displayed := m.displayedUpstreams()
	if len(displayed) > 0 && m.upstreamCursor < len(displayed) {
		selectedUp := displayed[m.upstreamCursor]
		return m, m.startUpstreamScan(selectedUp.URL, selectedUp.Ref)
	}
	return m, nil
}

func (m *Model) handleOpenChannelDetail() (tea.Model, tea.Cmd) {
	e, ok := m.currentChannelEntry()
	if !ok {
		return m, nil
	}
	tgtItem := e.Target
	svc := m.service
	return m, func() tea.Msg {
		cfgTgt, skillsDir, err := svc.TargetShow(tgtItem.Name)
		if err != nil {
			return errMsg{err: err}
		}
		resolvedDir := cfgTgt.ResolveConfigDir()
		modal := newTargetDetailModal(
			tgtItem.Name,
			string(cfgTgt.Type),
			cfgTgt.Channel,
			cfgTgt.ConfigDir,
			resolvedDir,
			skillsDir,
			cfgTgt.Paths,
			fsx.DirExists(resolvedDir),
			fsx.DirExists(skillsDir),
			"",
			svc.TargetDeployedSkills(tgtItem.Name),
			tgtItem.Enabled,
		)
		return targetDetailMsg{modal: modal}
	}
}

func (m *Model) handleOpenProjectDetail() (tea.Model, tea.Cmd) {
	projs := m.displayedProjectTargets()
	if len(projs) == 0 || m.projectCursor >= len(projs) {
		return m, nil
	}
	proj := projs[m.projectCursor]
	pDir := proj.RawPath
	svc := m.service
	return m, func() tea.Msg {
		modal := newTargetDetailModal(
			proj.Name,
			"project",
			"",
			pDir,
			pDir,
			"",
			nil,
			fsx.DirExists(pDir),
			false,
			proj.Formats,
			svc.ProjectDeployedSkills(pDir),
			proj.Enabled,
		)
		return targetDetailMsg{modal: modal}
	}
}

func (m *Model) handleModalTargetDetailKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter", "q":
		m.view = viewList
		return m, nil

	case "r":
		if m.targetDetailModal.Name == "" {
			return m, m.reloadAsync()
		}
		name := m.targetDetailModal.Name
		isProject := m.targetDetailModal.Type == "project"
		dir := m.targetDetailModal.ConfigDir
		svc := m.service
		return m, func() tea.Msg {
			if isProject {
				return targetDetailSkillsMsg(svc.ProjectDeployedSkills(dir))
			}
			return targetDetailSkillsMsg(svc.TargetDeployedSkills(name))
		}

	case " ", "t":
		if m.targetDetailModal.Type == "project" {
			pDir := m.targetDetailModal.ConfigDir
			newVal := !m.targetDetailModal.Enabled
			notice := fmt.Sprintf(i18n.T("notice.project_disabled"), m.targetDetailModal.Name)
			if newVal {
				notice = fmt.Sprintf(i18n.T("notice.project_enabled"), m.targetDetailModal.Name)
			}
			return m, m.toggleEnabledCmd(true, pDir, newVal, notice)
		}
		name := m.targetDetailModal.Name
		newVal := !m.targetDetailModal.Enabled
		notice := fmt.Sprintf(i18n.T("notice.target_disabled"), name)
		if newVal {
			notice = fmt.Sprintf(i18n.T("notice.target_enabled"), name)
		}
		return m, m.toggleEnabledCmd(false, name, newVal, notice)

	case "d":
		if !m.targetDetailModal.Enabled {
			m.notice = fmt.Sprintf(i18n.T("notice.target_disabled_cannot_deploy"), m.targetDetailModal.Name)
			return m, nil
		}
		recentProjects, _ := m.service.ProjectList()
		if m.targetDetailModal.Type == "project" {
			m.deployModal = newDeployModal(false, m.skills, "", m.targetNames, m.targets, m.service.Profiles(), recentProjects, m.service.IsProjectEnabled).
				WithLockedTarget(m.targetDetailModal.Name, "project", m.targetDetailModal.ConfigDir)
		} else {
			m.deployModal = newDeployModal(false, m.skills, "", m.targetNames, m.targets, m.service.Profiles(), recentProjects, m.service.IsProjectEnabled).
				WithLockedTarget(m.targetDetailModal.Name, "agent", "")
		}
		m.view = viewDeploy
		return m, nil

	case "D":
		recentProjects, _ := m.service.ProjectList()
		if m.targetDetailModal.Type == "project" {
			deployed := m.service.ProjectDeployedSkills(m.targetDetailModal.ConfigDir)
			if len(deployed) == 0 {
				m.notice = fmt.Sprintf(i18n.T("notice.no_deployed_skills_in_target"), m.targetDetailModal.Name)
				return m, nil
			}
			targetSkills := m.skillsForTargetUndeploy(deployed)
			m.deployModal = newDeployModal(true, targetSkills, "", m.targetNames, m.targets, m.service.Profiles(), recentProjects, m.service.IsProjectEnabled).
				WithLockedTarget(m.targetDetailModal.Name, "project", m.targetDetailModal.ConfigDir)
		} else {
			deployed := m.service.TargetDeployedSkills(m.targetDetailModal.Name)
			if len(deployed) == 0 {
				m.notice = fmt.Sprintf(i18n.T("notice.no_deployed_skills_in_target"), m.targetDetailModal.Name)
				return m, nil
			}
			targetSkills := m.skillsForTargetUndeploy(deployed)
			m.deployModal = newDeployModal(true, targetSkills, "", m.targetNames, m.targets, m.service.Profiles(), recentProjects, m.service.IsProjectEnabled).
				WithLockedTarget(m.targetDetailModal.Name, "agent", "")
		}
		m.view = viewUndeploy
		return m, nil

	case "x":
		if m.targetDetailModal.Type != "project" {
			name := m.targetDetailModal.Name
			m.confirmModal = ConfirmationModalState{
				Title:   i18n.T("confirm.remove_target.title"),
				Message: fmt.Sprintf(i18n.T("confirm.remove_target.msg"), name),
				Action:  "remove-target",
				Target:  name,
			}
			m.view = viewModalConfirm
			return m, nil
		} else {
			proj := m.targetDetailModal.ConfigDir
			m.confirmModal = ConfirmationModalState{
				Title:   i18n.T("confirm.remove_project.title"),
				Message: fmt.Sprintf(i18n.T("confirm.remove_project.msg"), proj),
				Action:  "remove-project",
				Target:  proj,
			}
			m.view = viewModalConfirm
			return m, nil
		}

	case "up", "k":
		m.targetDetailModal.Viewport.LineUp(1)
		return m, nil

	case "down", "j":
		m.targetDetailModal.Viewport.LineDown(1)
		return m, nil

	case "pgup", "b", "ctrl+u":
		m.targetDetailModal.Viewport.HalfViewUp()
		return m, nil

	case "pgdown", "f", "ctrl+d":
		m.targetDetailModal.Viewport.HalfViewDown()
		return m, nil

	case "g", "home":
		m.targetDetailModal.Viewport.GotoTop()
		return m, nil

	case "G", "end":
		m.targetDetailModal.Viewport.GotoBottom()
		return m, nil

	default:
		var cmd tea.Cmd
		m.targetDetailModal.Viewport, cmd = m.targetDetailModal.Viewport.Update(msg)
		return m, cmd
	}
}

func (m *Model) handleDeployConflictKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		m.view = viewList
		m.err = nil
		return m, nil

	case "esc":
		if len(m.deployConflictModal.RemainingQueue) > 0 {
			next := m.deployConflictModal.RemainingQueue[0]
			return m, func() tea.Msg {
				return deployConflictMsg{
					Conflict:      next,
					Queue:         m.deployConflictModal.RemainingQueue[1:],
					DeployedCount: m.deployConflictModal.BatchDeployed,
					TotalCount:    m.deployConflictModal.BatchTotal,
				}
			}
		}
		m.view = viewList
		m.err = nil
		if m.deployConflictModal.BatchDeployed > 0 {
			m.notice = i18n.T("notice.deployed_skipped_conflicts", m.deployConflictModal.BatchDeployed, m.deployConflictModal.TargetName)
		}
		return m, nil

	case "v": // View Diff
		m.loading = true
		m.notice = i18n.T("notice.loading_diff_workspace_target")
		ctx, cancel := context.WithCancel(m.ctx)
		m.cancelOp = cancel
		st := m.deployConflictModal
		return m, func() tea.Msg {
			diff, err := m.service.DiffTarget(ctx, st.SkillID, st.TargetName, st.ProjectPath, st.Format)
			return targetDiffLoadedMsg{diff: diff, err: err}
		}

	case "f": // Force deploy
		m.loading = true
		m.notice = i18n.T("notice.force_deploying", m.deployConflictModal.SkillID)
		ctx, cancel := context.WithCancel(m.ctx)
		m.cancelOp = cancel
		st := m.deployConflictModal
		return m, func() tea.Msg {
			var err error
			if st.ProjectPath != "" {
				_, _, err = m.service.DeployProject(ctx, st.SkillID, st.ProjectPath, []string{st.Format}, true)
			} else {
				err = m.service.Deploy(ctx, st.SkillID, st.TargetName, true)
			}
			if err != nil {
				return errMsg{err: err}
			}
			if len(st.RemainingQueue) > 0 {
				next := st.RemainingQueue[0]
				return deployConflictMsg{
					Conflict:      next,
					Queue:         st.RemainingQueue[1:],
					DeployedCount: st.BatchDeployed + 1,
					TotalCount:    st.BatchTotal,
				}
			}
			if st.BatchTotal > 1 {
				return asyncNoticeMsg(i18n.T("notice.deployed_partial", st.BatchDeployed+1, st.BatchTotal, st.TargetName))
			}
			return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.force_deployed"), st.SkillID, st.TargetName))
		}

	case "b": // Adopt modifications back to workspace
		m.loading = true
		m.notice = i18n.T("notice.adopting", m.deployConflictModal.SkillID)
		ctx, cancel := context.WithCancel(m.ctx)
		m.cancelOp = cancel
		st := m.deployConflictModal
		return m, func() tea.Msg {
			err := m.service.AdoptFromTarget(ctx, st.SkillID, st.TargetName, st.ProjectPath, st.Format)
			if err != nil {
				return errMsg{err: err}
			}
			if len(st.RemainingQueue) > 0 {
				next := st.RemainingQueue[0]
				return deployConflictMsg{
					Conflict:      next,
					Queue:         st.RemainingQueue[1:],
					DeployedCount: st.BatchDeployed + 1,
					TotalCount:    st.BatchTotal,
				}
			}
			if st.BatchTotal > 1 {
				return asyncNoticeMsg(i18n.T("notice.deployed_partial_adopted", st.BatchDeployed+1, st.BatchTotal, st.TargetName, st.SkillID))
			}
			return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.target_adopted"), st.SkillID, st.TargetName))
		}
	}
	return m, nil
}

func (m *Model) handleTargetDiffKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if handled, cmd := m.handleDiffNavigation(msg); handled {
		return m, cmd
	}
	switch msg.String() {
	case "esc", "q":
		m.view = viewModalDeployConflict
		m.err = nil
		return m, nil
	case "t", "T":
		m.toggleChangesOnly()
		return m, nil
	case "up", "k":
		m.diffViewport.LineUp(1)
	case "down", "j":
		m.diffViewport.LineDown(1)
	case "pgup", "b", "ctrl+u":
		m.diffViewport.HalfViewUp()
	case "pgdown", "f", " ", "ctrl+d":
		m.diffViewport.HalfViewDown()
	case "g", "home":
		m.diffViewport.GotoTop()
	case "G", "end":
		m.diffViewport.GotoBottom()
	default:
		var cmd tea.Cmd
		m.diffViewport, cmd = m.diffViewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) openSkillDetail(st *model.SkillStatus, rawMD string) {
	m.detailSkill = st
	if m.service != nil && m.service.WorkspaceRoot() != "" {
		m.detailPath = filepath.Join(catalog.SkillDir(m.service.WorkspaceRoot(), st.ID), "SKILL.md")
	} else {
		m.detailPath = filepath.Join("skills", st.ID, "SKILL.md")
	}

	meta, body, err := skill.ParseSkillContent([]byte(rawMD))
	if err != nil {
		meta = &model.SkillMetadata{
			Name:        st.ID,
			Description: "-",
			Path:        m.detailPath,
		}
		body = rawMD
	} else {
		if meta.Name == "" {
			meta.Name = st.ID
		}
		if meta.Description == "" {
			meta.Description = "-"
		}
		meta.Path = m.detailPath
	}
	m.detailMeta = meta

	contentWidth := m.width - 6
	if contentWidth < 30 {
		contentWidth = 30
	}
	renderer, rErr := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(contentWidth),
	)
	var rendered string
	if rErr == nil {
		rendered, rErr = renderer.Render(body)
	}
	if rErr != nil || strings.TrimSpace(rendered) == "" {
		rendered = body
	}

	m.updateViewportSizes()
	m.detailViewport.SetContent(strings.TrimSpace(rendered))
	m.detailViewport.GotoTop()
	m.view = viewDetail
}

func (m *Model) openDiff(diff string) {
	m.diffContent = diff
	m.diffSummary = diffview.Parse(diff)
	m.diffChangesOnly = false
	m.diffSearching = false
	m.diffSearch = textinput.New()
	m.diffPrefix = ""
	m.diffLastJump = -1
	m.view = viewDiff
	m.updateViewportSizes()
	m.diffViewport.GotoTop()
	m.diffViewport.SetXOffset(0)
}

func (m *Model) handleModalAddKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.addModal.Step == 0 {
		if len(m.addModal.Upstreams) > 0 {
			switch msg.String() {
			case "esc":
				m.view = viewList
				m.err = nil
				m.addModal = newAddModal(m.upstreams...)
				return m, nil
			case "left":
				m.addModal.FromUpstreamMode = true
				m.addModal.SourceInput.Blur()
				m.addModal.PathInput.Blur()
				return m, nil
			case "right":
				m.addModal.FromUpstreamMode = false
				m.addModal.ActiveInput = 0
				m.addModal.SourceInput.Focus()
				m.addModal.PathInput.Blur()
				return m, nil
			}

			if m.addModal.FromUpstreamMode {
				switch msg.String() {
				case "tab":
					m.addModal.FromUpstreamMode = false
					m.addModal.ActiveInput = 0
					m.addModal.SourceInput.Focus()
					m.addModal.PathInput.Blur()
					return m, nil
				case "shift+tab":
					m.addModal.FromUpstreamMode = false
					m.addModal.ActiveInput = 1
					m.addModal.SourceInput.Blur()
					m.addModal.PathInput.Focus()
					return m, nil
				case "up", "k":
					if m.addModal.UpstreamListCursor > 0 {
						m.addModal.UpstreamListCursor--
					}
					return m, nil
				case "down", "j":
					if m.addModal.UpstreamListCursor < len(m.addModal.Upstreams)-1 {
						m.addModal.UpstreamListCursor++
					}
					return m, nil
				case "enter":
					if m.loading || len(m.addModal.Upstreams) == 0 {
						return m, nil
					}
					selectedUp := m.addModal.Upstreams[m.addModal.UpstreamListCursor]
					ctx, cancel := context.WithCancel(m.ctx)
					m.cancelOp = cancel
					m.loading = true
					m.notice = fmt.Sprintf(i18n.T("notice.discovering_upstream"), selectedUp.URL)
					return m, func() tea.Msg {
						progressCb := m.progressCallback()
						discovered, err := m.service.UpstreamDiscover(ctx, selectedUp.URL, selectedUp.Ref, progressCb)
						if err != nil {
							if ctx.Err() != nil {
								return nil
							}
							return errMsg{err: err}
						}
						if ctx.Err() != nil {
							return nil
						}
						return discoveredSkillsMsg{
							url:    selectedUp.URL,
							ref:    selectedUp.Ref,
							skills: discovered,
						}
					}
				}
				return m, nil
			}
		}

		switch msg.String() {
		case "esc":
			m.view = viewList
			m.err = nil
			m.addModal = newAddModal(m.upstreams...)
			return m, nil

		case "tab":
			if len(m.addModal.Upstreams) > 0 && m.addModal.ActiveInput == 1 {
				m.addModal.FromUpstreamMode = true
				m.addModal.PathInput.Blur()
				return m, nil
			}
			m.addModal.ActiveInput = (m.addModal.ActiveInput + 1) % 2
			if m.addModal.ActiveInput == 0 {
				m.addModal.SourceInput.Focus()
				m.addModal.PathInput.Blur()
			} else {
				m.addModal.SourceInput.Blur()
				m.addModal.PathInput.Focus()
			}
			return m, nil

		case "shift+tab":
			if len(m.addModal.Upstreams) > 0 && m.addModal.ActiveInput == 0 {
				m.addModal.FromUpstreamMode = true
				m.addModal.SourceInput.Blur()
				return m, nil
			}
			m.addModal.ActiveInput = (m.addModal.ActiveInput + 1) % 2
			if m.addModal.ActiveInput == 0 {
				m.addModal.SourceInput.Focus()
				m.addModal.PathInput.Blur()
			} else {
				m.addModal.SourceInput.Blur()
				m.addModal.PathInput.Focus()
			}
			return m, nil

		case "enter":
			if m.loading {
				return m, nil
			}
			m.err = nil

			src := strings.TrimSpace(m.addModal.SourceInput.Value())
			subpath := strings.TrimSpace(m.addModal.PathInput.Value())
			if src == "" {
				return m, nil
			}

			// Automatically prepend https:// if user inputs github.com/...
			if strings.HasPrefix(src, "github.com/") || strings.HasPrefix(src, "gitlab.com/") || strings.HasPrefix(src, "gitee.com/") {
				src = "https://" + src
			}

			isGit := strings.HasPrefix(src, "http://") ||
				strings.HasPrefix(src, "https://") ||
				strings.HasPrefix(src, "git@") ||
				strings.HasPrefix(src, "ssh://") ||
				strings.HasSuffix(src, ".git")

			if !isGit {
				// Local directory add
				ctx, cancel := context.WithCancel(m.ctx)
				m.cancelOp = cancel
				m.loading = true
				m.notice = i18n.T("notice.importing_local_dir", src)
				return m, func() tea.Msg {
					err := m.service.AddLocal(ctx, src, false)
					if err != nil {
						if ctx.Err() != nil {
							return nil
						}
						return errMsg{err: err}
					}
					if ctx.Err() != nil {
						return nil
					}
					return asyncNoticeMsg(i18n.T("notice.added_local_skill", src))
				}
			}

			// Git repository
			progressCb := m.progressCallback()

			if subpath != "" {
				// Specific subpath provided, add directly
				ctx, cancel := context.WithCancel(m.ctx)
				m.cancelOp = cancel
				m.loading = true
				m.notice = i18n.T("notice.cloning_extracting", src, subpath)
				m.progressPhase = progress.PhaseFetch
				return m, func() tea.Msg {
					err := m.service.AddGit(ctx, src, "", subpath, false, progressCb)
					if err != nil {
						if ctx.Err() != nil {
							return nil
						}
						return errMsg{err: err}
					}
					return asyncNoticeMsg(i18n.T("notice.added_git_skill", src, subpath))
				}
			}

			// Auto-discover in Git repo
			ctx, cancel := context.WithCancel(m.ctx)
			m.cancelOp = cancel
			m.loading = true
			m.notice = i18n.T("notice.connecting_inspect", src)
			m.progressPhase = progress.PhaseFetch
			return m, func() tea.Msg {
				discovered, err := m.service.DiscoverGit(ctx, src, "", progressCb)
				if err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return errMsg{err: err}
				}
				if len(discovered) == 0 {
					return errMsg{err: fmt.Errorf("%s", i18n.T("error.no_valid_skill_md", src))}
				}
				if len(discovered) == 1 {
					hasConflict := false
					cat, err := catalog.Load(m.service.WorkspaceRoot())
					if err == nil && cat != nil {
						if _, exists := cat.Skills[discovered[0].ID]; exists {
							hasConflict = true
						}
					}
					if !hasConflict {
						// Single skill with no conflict, add directly!
						res, err := m.service.AddGitBatch(ctx, src, "", []string{discovered[0].Path}, false, false, false, progressCb)
						if err != nil {
							if ctx.Err() != nil {
								return nil
							}
							return errMsg{err: err}
						}
						if len(res.Failed) > 0 {
							for _, fErr := range res.Failed {
								return errMsg{err: fmt.Errorf("%s", fErr)}
							}
						}
						return asyncNoticeMsg(i18n.T("notice.added_skill_from", discovered[0].ID, src))
					}
				}
				return discoveredSkillsMsg{url: src, skills: discovered}
			}
		}
	} else {
		// Step 1: multi-select discovered skills
		visible := m.addModal.VisibleIndices()

		if m.addModal.Renaming {
			switch msg.String() {
			case "esc":
				m.addModal.Renaming = false
				m.addModal.RenameInput.Blur()
				m.err = nil
				return m, nil

			case "enter":
				val := strings.TrimSpace(m.addModal.RenameInput.Value())
				if val == "" {
					m.err = fmt.Errorf("%s", i18n.T("error.skill_id_empty"))
					return m, nil
				}
				if err := skill.ValidateID(val); err != nil {
					m.err = err
					return m, nil
				}
				for _, s := range m.skills {
					if s.ID == val {
						m.err = fmt.Errorf("%s", i18n.T("error.skill_id_in_use", val))
						return m, nil
					}
				}
				if len(visible) == 0 {
					m.addModal.Renaming = false
					return m, nil
				}
				curIdx := m.addModal.DiscoveredCursor
				if curIdx >= len(visible) {
					curIdx = len(visible) - 1
				}
				realIdx := visible[curIdx]
				for j, other := range m.addModal.DiscoveredSkills {
					if j == realIdx {
						continue
					}
					otherID := other.ID
					if al, ok := m.addModal.CustomAliases[other.Path]; ok && al != "" {
						otherID = al
					}
					if otherID == val {
						m.err = fmt.Errorf("%s", i18n.T("error.skill_id_conflict_batch", val, other.Path))
						return m, nil
					}
				}
				curSk := m.addModal.DiscoveredSkills[realIdx]
				m.addModal.CustomAliases[curSk.Path] = val
				m.addModal.SelectedIndices[realIdx] = true
				delete(m.addModal.OverwriteSkills, curSk.Path)
				m.addModal.Renaming = false
				m.addModal.RenameInput.Blur()
				m.err = nil
				return m, nil

			default:
				var cmd tea.Cmd
				m.addModal.RenameInput, cmd = m.addModal.RenameInput.Update(msg)
				return m, cmd
			}
		}

		if m.addModal.Searching {
			switch msg.String() {
			case "esc":
				m.addModal.Searching = false
				m.addModal.SearchInput.Blur()
				return m, nil
			case "enter":
				m.addModal.Searching = false
				m.addModal.SearchInput.Blur()
				return m, nil
			case "up", "down":
				m.addModal.Searching = false
				m.addModal.SearchInput.Blur()
			default:
				var cmd tea.Cmd
				m.addModal.SearchInput, cmd = m.addModal.SearchInput.Update(msg)
				vis := m.addModal.VisibleIndices()
				if m.addModal.DiscoveredCursor >= len(vis) {
					m.addModal.DiscoveredCursor = max(0, len(vis)-1)
				}
				return m, cmd
			}
		}

		switch msg.String() {
		case "/":
			m.addModal.Searching = true
			m.addModal.SearchInput.Focus()
			return m, textinput.Blink
		case "esc":
			if m.addModal.SearchInput.Value() != "" {
				m.addModal.SearchInput.SetValue("")
				vis := m.addModal.VisibleIndices()
				if m.addModal.DiscoveredCursor >= len(vis) {
					m.addModal.DiscoveredCursor = max(0, len(vis)-1)
				}
				return m, nil
			}
			m.view = viewList
			m.err = nil
			m.addModal = newAddModal()
			return m, nil
		case "q":
			m.view = viewList
			m.err = nil
			m.addModal = newAddModal()
			return m, nil

		// Filter selection is a secondary tab row: ←/→ cycle it, digits jump.
		case "right", "l":
			m.addModal.CurrentFilter = (m.addModal.CurrentFilter + 1) % 4
			m.addModal.DiscoveredCursor = 0
			m.addModal.ScrollOffset = 0
			m.err = nil
			return m, nil

		case "left", "h":
			m.addModal.CurrentFilter = (m.addModal.CurrentFilter + 3) % 4
			m.addModal.DiscoveredCursor = 0
			m.addModal.ScrollOffset = 0
			m.err = nil
			return m, nil
		case "1":
			m.addModal.CurrentFilter = AddFilterAll
			m.addModal.DiscoveredCursor = 0
			m.addModal.ScrollOffset = 0
			m.err = nil
			return m, nil
		case "2":
			m.addModal.CurrentFilter = AddFilterConflict
			m.addModal.DiscoveredCursor = 0
			m.addModal.ScrollOffset = 0
			m.err = nil
			return m, nil
		case "3":
			m.addModal.CurrentFilter = AddFilterNew
			m.addModal.DiscoveredCursor = 0
			m.addModal.ScrollOffset = 0
			m.err = nil
			return m, nil
		case "4":
			m.addModal.CurrentFilter = AddFilterInstalled
			m.addModal.DiscoveredCursor = 0
			m.addModal.ScrollOffset = 0
			m.err = nil
			return m, nil

		case "up", "k":
			if m.addModal.DiscoveredCursor > 0 {
				m.addModal.DiscoveredCursor--
			}
		case "down", "j":
			if m.addModal.DiscoveredCursor < len(visible)-1 {
				m.addModal.DiscoveredCursor++
			}
		case " ":
			if len(visible) > 0 {
				idx := m.addModal.DiscoveredCursor
				if idx >= len(visible) {
					idx = len(visible) - 1
				}
				realIdx := visible[idx]
				m.addModal.SelectedIndices[realIdx] = !m.addModal.SelectedIndices[realIdx]
			}
		case "a":
			if len(visible) > 0 {
				allSelected := true
				for _, rIdx := range visible {
					if !m.addModal.SelectedIndices[rIdx] {
						allSelected = false
						break
					}
				}
				for _, rIdx := range visible {
					m.addModal.SelectedIndices[rIdx] = !allSelected
				}
			}
		case "s":
			for i, sk := range m.addModal.DiscoveredSkills {
				isNew := false
				if alias, ok := m.addModal.CustomAliases[sk.Path]; ok && alias != "" {
					isNew = true
				} else if st, ok := m.addModal.DiscoveredStatus[i]; ok && st.Tag == DiscoveredTagNew {
					isNew = true
				}
				m.addModal.SelectedIndices[i] = isNew
			}
		case "o":
			if len(visible) == 0 {
				return m, nil
			}

			// Check if any conflict/installed items in current visible view are selected
			var selectedConflictIndices []int
			for _, rIdx := range visible {
				if m.addModal.SelectedIndices[rIdx] {
					st, ok := m.addModal.DiscoveredStatus[rIdx]
					if ok && (st.Tag == DiscoveredTagConflict || st.Tag == DiscoveredTagInstalled) {
						selectedConflictIndices = append(selectedConflictIndices, rIdx)
					}
				}
			}

			if len(selectedConflictIndices) > 0 {
				// Batch toggle overwrite for all selected conflict/installed items in visible view
				allOverwritten := true
				for _, rIdx := range selectedConflictIndices {
					sk := m.addModal.DiscoveredSkills[rIdx]
					if !m.addModal.OverwriteSkills[sk.Path] {
						allOverwritten = false
						break
					}
				}

				newOverwriteState := !allOverwritten
				for _, rIdx := range selectedConflictIndices {
					sk := m.addModal.DiscoveredSkills[rIdx]
					if newOverwriteState {
						m.addModal.OverwriteSkills[sk.Path] = true
						delete(m.addModal.CustomAliases, sk.Path)
					} else {
						delete(m.addModal.OverwriteSkills, sk.Path)
					}
				}
				m.err = nil
				return m, nil
			}

			// Single-item fallback: when no conflict/installed items are currently selected in visible view,
			// toggle overwrite for the item under the cursor.
			idx := m.addModal.DiscoveredCursor
			if idx >= len(visible) {
				idx = len(visible) - 1
			}
			realIdx := visible[idx]
			sk := m.addModal.DiscoveredSkills[realIdx]
			st, ok := m.addModal.DiscoveredStatus[realIdx]
			if !ok || (st.Tag != DiscoveredTagConflict && st.Tag != DiscoveredTagInstalled) {
				m.err = fmt.Errorf("%s", i18n.T("error.skill_no_conflict", sk.ID))
				return m, nil
			}
			m.addModal.OverwriteSkills[sk.Path] = !m.addModal.OverwriteSkills[sk.Path]
			if m.addModal.OverwriteSkills[sk.Path] {
				m.addModal.SelectedIndices[realIdx] = true
				delete(m.addModal.CustomAliases, sk.Path)
			} else {
				delete(m.addModal.OverwriteSkills, sk.Path)
			}
			m.err = nil
			return m, nil
		case "f":
			m.addModal.Force = !m.addModal.Force
		case "e":
			if len(visible) > 0 {
				idx := m.addModal.DiscoveredCursor
				if idx >= len(visible) {
					idx = len(visible) - 1
				}
				realIdx := visible[idx]
				sk := m.addModal.DiscoveredSkills[realIdx]
				m.addModal.Renaming = true
				initialVal := sk.ID
				if alias, ok := m.addModal.CustomAliases[sk.Path]; ok && alias != "" {
					initialVal = alias
				}
				m.addModal.RenameInput.SetValue(initialVal)
				m.addModal.RenameInput.Focus()
				m.err = nil
				return m, nil
			}
		case "enter":
			url := strings.TrimSpace(m.addModal.SourceURL)
			if url == "" {
				url = strings.TrimSpace(m.addModal.SourceInput.Value())
			}
			if strings.HasPrefix(url, "github.com/") || strings.HasPrefix(url, "gitlab.com/") || strings.HasPrefix(url, "gitee.com/") {
				url = "https://" + url
			}
			ref := m.addModal.SourceRef
			if url == "" {
				m.err = fmt.Errorf("%s", i18n.T("error.source_url_missing"))
				return m, nil
			}
			var toAdd []git.DiscoveredSkill
			for i, sk := range m.addModal.DiscoveredSkills {
				if m.addModal.SelectedIndices[i] {
					toAdd = append(toAdd, sk)
				}
			}
			if len(toAdd) == 0 {
				m.err = fmt.Errorf("%s", i18n.T("error.no_skills_selected"))
				return m, nil
			}

			var conflictWithoutResolution []string
			for i, sk := range m.addModal.DiscoveredSkills {
				if m.addModal.SelectedIndices[i] {
					if st, ok := m.addModal.DiscoveredStatus[i]; ok && st.Tag == DiscoveredTagConflict {
						hasAlias := m.addModal.CustomAliases[sk.Path] != ""
						hasOverwrite := m.addModal.OverwriteSkills[sk.Path]
						if !hasAlias && !hasOverwrite && !m.addModal.ReplaceSource {
							conflictWithoutResolution = append(conflictWithoutResolution, sk.ID)
						}
					}
				}
			}
			if len(conflictWithoutResolution) > 0 {
				m.err = fmt.Errorf(i18n.T("modal.add.unresolved_conflict"), strings.Join(conflictWithoutResolution, ", "))
				return m, nil
			}

			ctx, cancel := context.WithCancel(m.ctx)
			m.cancelOp = cancel
			m.loading = true
			m.notice = i18n.T("notice.importing_selected", len(toAdd))
			m.gitProgress = ""

			aliasesCopy := make(map[string]string)
			for k, v := range m.addModal.CustomAliases {
				aliasesCopy[k] = v
			}
			overwriteCopy := make(map[string]bool)
			for k, v := range m.addModal.OverwriteSkills {
				overwriteCopy[k] = v
			}
			replaceSource := m.addModal.ReplaceSource
			force := m.addModal.Force

			return m, func() tea.Msg {
				var toAddPaths []string
				for _, sk := range toAdd {
					toAddPaths = append(toAddPaths, sk.Path)
				}
				progressCb := m.progressCallback()
				res, err := m.service.AddGitBatchWithOptions(ctx, url, ref, toAddPaths, aliasesCopy, overwriteCopy, replaceSource, force, false, progressCb)
				if err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return errMsg{err: err}
				}
				if ctx.Err() != nil {
					return nil
				}
				msg := fmt.Sprintf("Added %d skills from %s", len(res.Added), url)
				if len(res.Skipped) > 0 {
					msg += fmt.Sprintf(" (%d skipped)", len(res.Skipped))
				}
				if len(res.Failed) > 0 {
					var failedNames []string
					for id := range res.Failed {
						failedNames = append(failedNames, id)
					}
					sort.Strings(failedNames)
					if len(failedNames) <= 3 {
						msg += fmt.Sprintf(" (%d failed: %s)", len(res.Failed), strings.Join(failedNames, ", "))
					} else {
						msg += fmt.Sprintf(" (%d failed: %s...)", len(res.Failed), strings.Join(failedNames[:3], ", "))
					}
				}
				return asyncNoticeMsg(msg)
			}
		}
	}
	return m, nil
}

func (m *Model) handleModalNewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.view = viewList
		m.err = nil
		m.newInput.SetValue("")
		return m, nil
	}
	if msg.String() == "enter" {
		id := strings.TrimSpace(m.newInput.Value())
		if id == "" {
			return m, nil
		}
		m.loading = true
		m.notice = i18n.T("notice.creating_skill", id)
		ctx, cancel := context.WithCancel(m.ctx)
		m.cancelOp = cancel
		return m, func() tea.Msg {
			err := m.service.NewSkill(ctx, id)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return errMsg{err: err}
			}
			return asyncNoticeMsg(i18n.T("notice.created_managed_skill", id))
		}
	}
	return m, nil
}

func (m *Model) handleModalAddTargetKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.view = viewList
		m.err = nil
		m.targetModal = newTargetModal()
		return m, nil
	}
	switch msg.String() {
	case "tab", "shift+tab":
		m.targetModal.ActiveInput = (m.targetModal.ActiveInput + 1) % 2
		if m.targetModal.ActiveInput == 0 {
			m.targetModal.NameInput.Focus()
			m.targetModal.PathInput.Blur()
		} else {
			m.targetModal.NameInput.Blur()
			m.targetModal.PathInput.Focus()
		}
	case "enter":
		name := strings.TrimSpace(m.targetModal.NameInput.Value())
		path := strings.TrimSpace(m.targetModal.PathInput.Value())
		if name == "" || path == "" {
			return m, nil
		}
		m.loading = true
		m.notice = i18n.T("notice.adding_target", name)
		ctx, cancel := context.WithCancel(m.ctx)
		m.cancelOp = cancel
		return m, func() tea.Msg {
			if ctx.Err() != nil {
				return nil
			}
			err := m.service.TargetAdd(name, path)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return errMsg{err: err}
			}
			if ctx.Err() != nil {
				return nil
			}
			return asyncNoticeMsg(i18n.T("notice.added_target", name, path))
		}
	}
	return m, nil
}

func (m *Model) handleModalAddProjectKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.view = viewList
		m.err = nil
		m.projectModal = newProjectModal()
		return m, nil
	}
	if msg.String() == "enter" {
		path := strings.TrimSpace(m.projectModal.PathInput.Value())
		if path == "" {
			return m, nil
		}
		m.loading = true
		m.notice = i18n.T("notice.adding_project", path)
		ctx, cancel := context.WithCancel(m.ctx)
		m.cancelOp = cancel
		return m, func() tea.Msg {
			if ctx.Err() != nil {
				return nil
			}
			err := m.service.ProjectAdd(path)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return errMsg{err: err}
			}
			if ctx.Err() != nil {
				return nil
			}
			return asyncNoticeMsg(i18n.T("notice.added_project_dir", path))
		}
	}
	return m, nil
}

func (m *Model) handleModalAddGroupKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.view = viewList
		m.err = nil
		m.groupModal = newGroupModal()
		return m, nil
	}
	if msg.String() == "enter" {
		name := strings.TrimSpace(m.groupModal.NameInput.Value())
		if name == "" {
			return m, nil
		}
		m.loading = true
		m.notice = i18n.T("notice.creating_group", name)
		ctx, cancel := context.WithCancel(m.ctx)
		m.cancelOp = cancel
		return m, func() tea.Msg {
			if ctx.Err() != nil {
				return nil
			}
			err := m.service.GroupCreate(name)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return errMsg{err: err}
			}
			if ctx.Err() != nil {
				return nil
			}
			return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.group_created"), name))
		}
	}
	return m, nil
}

func (m *Model) handleModalGroupSkillsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	st := &m.groupSkillsModal
	if st.Searching {
		switch msg.String() {
		case "esc":
			st.Searching = false
			st.SearchInput.Blur()
			return m, nil
		case "enter":
			st.Searching = false
			st.SearchInput.Blur()
			return m, nil
		case "up", "down":
			st.Searching = false
			st.SearchInput.Blur()
		default:
			var cmd tea.Cmd
			st.SearchInput, cmd = st.SearchInput.Update(msg)
			vis := st.VisibleSkills()
			if st.Cursor >= len(vis) {
				st.Cursor = max(0, len(vis)-1)
			}
			return m, cmd
		}
	}

	switch msg.String() {
	case "/":
		st.Searching = true
		st.SearchInput.Focus()
		return m, textinput.Blink
	case "esc":
		if st.SearchInput.Value() != "" {
			st.SearchInput.SetValue("")
			vis := st.VisibleSkills()
			if st.Cursor >= len(vis) {
				st.Cursor = max(0, len(vis)-1)
			}
			return m, nil
		}
		m.view = viewList
		m.err = nil
		return m, nil
	case "q":
		m.view = viewList
		m.err = nil
		return m, nil
	case "up", "k":
		if st.Cursor > 0 {
			st.Cursor--
		}
	case "down", "j":
		vis := st.VisibleSkills()
		if st.Cursor < len(vis)-1 {
			st.Cursor++
		}
	case " ":
		vis := st.VisibleSkills()
		if len(vis) > 0 && st.Cursor < len(vis) {
			skID := vis[st.Cursor].ID
			st.SelectedSkills[skID] = !st.SelectedSkills[skID]
		}
	case "a":
		vis := st.VisibleSkills()
		if len(vis) > 0 {
			allSelected := true
			for _, s := range vis {
				if !st.SelectedSkills[s.ID] {
					allSelected = false
					break
				}
			}
			for _, s := range vis {
				st.SelectedSkills[s.ID] = !allSelected
			}
		}
	case "enter":
		var chosen []string
		for _, s := range st.Skills {
			if st.SelectedSkills[s.ID] {
				chosen = append(chosen, s.ID)
			}
		}
		grpName := st.GroupName
		m.loading = true
		m.notice = i18n.T("notice.saving_group", grpName)
		ctx, cancel := context.WithCancel(m.ctx)
		m.cancelOp = cancel
		return m, func() tea.Msg {
			if ctx.Err() != nil {
				return nil
			}
			err := m.service.GroupSetSkills(grpName, chosen)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return errMsg{err: err}
			}
			if ctx.Err() != nil {
				return nil
			}
			return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.group_saved"), grpName, len(chosen)))
		}
	}
	return m, nil
}

func (m *Model) handleModalConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		m.loading = true
		action := m.confirmModal.Action
		skillID := m.confirmModal.SkillID
		target := m.confirmModal.Target
		ctx, cancel := context.WithCancel(m.ctx)
		m.cancelOp = cancel

		switch action {
		case "remove-skill":
			m.notice = i18n.T("notice.removing_skill_one", skillID)
		case "remove-skills-batch":
			m.notice = i18n.T("notice.removing_skills_many", len(m.confirmModal.SkillIDs))
		case "remove-target":
			m.notice = i18n.T("notice.removing_target", target)
		case "remove-project":
			m.notice = i18n.T("notice.removing_project", target)
		case "remove-group":
			m.notice = i18n.T("notice.deleting_group", target)
		case "clean-cache":
			m.notice = i18n.T("notice.cleaning_cache")
		case "update-skill":
			m.notice = i18n.T("notice.updating_skill", skillID)
		case "force-deploy":
			m.notice = i18n.T("notice.force_deploying_to", skillID, target)
		case "reset-defaults":
			m.notice = i18n.T("notice.resetting_defaults")
		default:
			m.notice = i18n.T("notice.processing")
		}

		return m, func() tea.Msg {
			if ctx.Err() != nil {
				return nil
			}
			switch action {
			case "remove-skill":
				if err := m.service.Remove(ctx, skillID, false); err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return errMsg{err: err}
				}
				return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.removed_skill"), skillID))
			case "remove-skills-batch":
				skillIDs := m.confirmModal.SkillIDs
				res, err := m.service.RemoveBatch(ctx, skillIDs, false, m.progressCallback())
				if err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return errMsg{err: err}
				}
				if len(res.Failed) > 0 {
					return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.removed_skills_batch_partial"), len(res.Removed), len(res.Failed)))
				}
				return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.removed_skills_batch"), len(res.Removed)))
			case "remove-target":
				if err := m.service.TargetRemove(target); err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return errMsg{err: err}
				}
				return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.removed_target"), target))
			case "remove-project":
				if ctx.Err() != nil {
					return nil
				}
				if err := m.service.ProjectRemove(target); err != nil {
					return errMsg{err: err}
				}
				return asyncNoticeMsg(i18n.T("notice.removed_project", target))
			case "remove-group":
				if err := m.service.GroupDelete(target); err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return errMsg{err: err}
				}
				return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.group_deleted"), target))
			case "clean-cache":
				count, _, _ := m.service.CacheClean()
				return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.clean_cache"), count))
			case "update-skill":
				if _, err := m.service.Update(ctx, skillID, false, false); err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return errMsg{err: err}
				}
				return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.updated_skill"), skillID))
			case "force-deploy":
				if err := m.service.Deploy(ctx, skillID, target, true); err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return errMsg{err: err}
				}
				return asyncNoticeMsg(i18n.T("notice.force_deployed", skillID, target))
			case "reset-defaults":
				if ctx.Err() != nil {
					return nil
				}
				if err := m.service.ConfigResetDefaults(); err != nil {
					return errMsg{err: err}
				}
				if ctx.Err() != nil {
					return nil
				}
				return asyncNoticeMsg(i18n.T("notice.reset_defaults_completed"))
			}
			return nil
		}

	case "f":
		if m.confirmModal.AllowForce {
			m.loading = true
			skillID := m.confirmModal.SkillID
			action := m.confirmModal.Action
			target := m.confirmModal.Target
			ctx, cancel := context.WithCancel(m.ctx)
			m.cancelOp = cancel

			switch action {
			case "remove-skill":
				m.notice = i18n.T("notice.force_removing_skill", skillID)
			case "remove-skills-batch":
				m.notice = i18n.T("notice.force_removing_skills", len(m.confirmModal.SkillIDs))
			case "update-skill":
				m.notice = i18n.T("notice.force_updating_skill", skillID)
			case "deploy-skill", "force-deploy":
				m.notice = i18n.T("notice.force_deploying_to", skillID, target)
			default:
				m.notice = i18n.T("notice.force_executing")
			}

			return m, func() tea.Msg {
				if ctx.Err() != nil {
					return nil
				}
				if action == "remove-skill" {
					if err := m.service.Remove(ctx, skillID, true); err != nil {
						if ctx.Err() != nil {
							return nil
						}
						return errMsg{err: err}
					}
					return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.force_removed_skill"), skillID))
				} else if action == "remove-skills-batch" {
					skillIDs := m.confirmModal.SkillIDs
					res, err := m.service.RemoveBatch(ctx, skillIDs, true, m.progressCallback())
					if err != nil {
						if ctx.Err() != nil {
							return nil
						}
						return errMsg{err: err}
					}
					if len(res.Failed) > 0 {
						return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.removed_skills_batch_partial"), len(res.Removed), len(res.Failed)))
					}
					return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.force_removed_skills_batch"), len(res.Removed)))
				} else if action == "update-skill" {
					if _, err := m.service.Update(ctx, skillID, true, false); err != nil {
						if ctx.Err() != nil {
							return nil
						}
						return errMsg{err: err}
					}
					return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.force_updated_skill"), skillID))
				} else if action == "deploy-skill" || action == "force-deploy" {
					if err := m.service.Deploy(ctx, skillID, target, true); err != nil {
						if ctx.Err() != nil {
							return nil
						}
						return errMsg{err: err}
					}
					return asyncNoticeMsg(i18n.T("notice.force_deployed", skillID, target))
				}
				return nil
			}
		}

	case "n", "esc", "q":
		m.view = viewList
		return m, nil
	}
	return m, nil
}

// upstreamModalNavKey reports whether the key is reserved for upstream modal navigation
// and must not reach the focused text control.
func upstreamModalNavKey(msg tea.Msg) bool {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return false
	}
	switch key.String() {
	case "tab", "shift+tab", "esc", "ctrl+s":
		return true
	}
	return false
}

func (m *Model) updateAddUpstreamInputs(msg tea.Msg) tea.Cmd {
	am := &m.addUpstreamModal
	if am.Active == 0 || upstreamModalNavKey(msg) {
		return nil
	}
	var cmd tea.Cmd
	switch {
	case am.Tab == 0 && am.Active == 1:
		am.URLInput, cmd = am.URLInput.Update(msg)
	case am.Tab == 0 && am.Active == 2:
		am.RefInput, cmd = am.RefInput.Update(msg)
	case am.Tab == 0 && am.Active == 3:
		am.NameInput, cmd = am.NameInput.Update(msg)
	case am.Tab == 1 && am.Active == 2:
		am.Scan.RootsInput, cmd = am.Scan.RootsInput.Update(msg)
	case am.Tab == 1 && am.Active == 3:
		am.Scan.ExcludeInput, cmd = am.Scan.ExcludeInput.Update(msg)
	}
	return cmd
}

func (m *Model) updateEditUpstreamInputs(msg tea.Msg) tea.Cmd {
	em := &m.editUpstreamModal
	if em.Active == 0 || upstreamModalNavKey(msg) {
		return nil
	}
	var cmd tea.Cmd
	switch {
	case em.Tab == 0 && em.Active == 1:
		em.RefInput, cmd = em.RefInput.Update(msg)
	case em.Tab == 0 && em.Active == 2:
		em.NameInput, cmd = em.NameInput.Update(msg)
	case em.Tab == 1 && em.Active == 2:
		em.Scan.RootsInput, cmd = em.Scan.RootsInput.Update(msg)
	case em.Tab == 1 && em.Active == 3:
		em.Scan.ExcludeInput, cmd = em.Scan.ExcludeInput.Update(msg)
	}
	return cmd
}

func (m *Model) handleModalAddUpstreamKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = viewList
		m.err = nil
		m.addUpstreamModal = newAddUpstreamModal()
		return m, nil

	case "tab", "shift+tab":
		m.addUpstreamModal.cycleFocus(msg.String() == "shift+tab")
		m.updateAddUpstreamFocus()
		return m, nil

	case "left", "right":
		if m.addUpstreamModal.Active == 0 {
			m.addUpstreamModal.Tab = 1 - m.addUpstreamModal.Tab
			m.addUpstreamModal.Active = 1
			m.updateAddUpstreamFocus()
			return m, nil
		}
		if m.addUpstreamModal.Tab == 1 && m.addUpstreamModal.Active == 1 {
			delta := 1
			if msg.String() == "left" {
				delta = -1
			}
			m.addUpstreamModal.Scan.cycleMode(delta)
			m.updateAddUpstreamFocus()
			return m, nil
		}
		// Otherwise the focused text control handles the arrow (cursor movement).

	case "enter", "ctrl+s":
		if msg.String() == "enter" && m.addUpstreamModal.focusIsMultiline() {
			return m, nil
		}
		return m.submitAddUpstream()
	}
	return m, nil
}

func (m *Model) submitAddUpstream() (tea.Model, tea.Cmd) {
	url := strings.TrimSpace(m.addUpstreamModal.URLInput.Value())
	if url == "" {
		return m, nil
	}
	scan, scanErrMsg := m.addUpstreamModal.Scan.config()
	if scanErrMsg != "" {
		m.err = fmt.Errorf("%s", scanErrMsg)
		return m, nil
	}
	m.err = nil
	ref := strings.TrimSpace(m.addUpstreamModal.RefInput.Value())
	name := strings.TrimSpace(m.addUpstreamModal.NameInput.Value())

	ctx, cancel := context.WithCancel(m.ctx)
	m.cancelOp = cancel
	m.loading = true
	m.notice = i18n.T("notice.adding_upstream", url)
	m.upstreamOperation++
	id := m.upstreamOperation
	svc := m.service
	return m, func() tea.Msg {
		if ctx.Err() != nil {
			return nil
		}
		err := svc.UpstreamAdd(ctx, url, ref, name, scan)
		result := upstreamAddedMsg{id: id, url: url, ref: ref, name: name, sourceType: model.SourceTypeGit, err: err}
		if err == nil {
			expanded, expandErr := fsx.ExpandUser(url)
			cfg, configErr := svc.Config()
			if configErr == nil {
				for _, u := range cfg.Upstreams {
					if u.URL == url || (expandErr == nil && u.URL == expanded) {
						result.url, result.ref, result.name, result.sourceType = u.URL, u.Ref, u.Name, u.Type
						break
					}
				}
			}
		}
		return result
	}
}

func (m *Model) updateAddUpstreamFocus() {
	am := &m.addUpstreamModal
	am.URLInput.Blur()
	am.RefInput.Blur()
	am.NameInput.Blur()
	am.Scan.RootsInput.Blur()
	am.Scan.ExcludeInput.Blur()
	if am.Active == 0 {
		return
	}
	switch {
	case am.Tab == 0 && am.Active == 1:
		am.URLInput.Focus()
	case am.Tab == 0 && am.Active == 2:
		am.RefInput.Focus()
	case am.Tab == 0 && am.Active == 3:
		am.NameInput.Focus()
	case am.Tab == 1 && am.Active == 2:
		am.Scan.RootsInput.Focus()
	case am.Tab == 1 && am.Active == 3:
		am.Scan.ExcludeInput.Focus()
	}
}

func (m *Model) handleModalEditUpstreamKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = viewList
		m.err = nil
		return m, nil

	case "tab", "shift+tab":
		m.editUpstreamModal.cycleFocus(msg.String() == "shift+tab")
		m.updateEditUpstreamFocus()
		return m, nil

	case "left", "right":
		if m.editUpstreamModal.Active == 0 {
			m.editUpstreamModal.Tab = 1 - m.editUpstreamModal.Tab
			m.editUpstreamModal.Active = 1
			m.updateEditUpstreamFocus()
			return m, nil
		}
		if m.editUpstreamModal.Tab == 1 && m.editUpstreamModal.Active == 1 {
			delta := 1
			if msg.String() == "left" {
				delta = -1
			}
			m.editUpstreamModal.Scan.cycleMode(delta)
			m.updateEditUpstreamFocus()
			return m, nil
		}

	case "enter", "ctrl+s":
		if msg.String() == "enter" && m.editUpstreamModal.focusIsMultiline() {
			return m, nil
		}
		return m.submitEditUpstream()
	}
	return m, nil
}

func (m *Model) updateEditUpstreamFocus() {
	em := &m.editUpstreamModal
	em.RefInput.Blur()
	em.NameInput.Blur()
	em.Scan.RootsInput.Blur()
	em.Scan.ExcludeInput.Blur()
	if em.Active == 0 {
		return
	}
	switch {
	case em.Tab == 0 && em.Active == 1:
		em.RefInput.Focus()
	case em.Tab == 0 && em.Active == 2:
		em.NameInput.Focus()
	case em.Tab == 1 && em.Active == 2:
		em.Scan.RootsInput.Focus()
	case em.Tab == 1 && em.Active == 3:
		em.Scan.ExcludeInput.Focus()
	}
}

func (m *Model) submitEditUpstream() (tea.Model, tea.Cmd) {
	scan, scanErrMsg := m.editUpstreamModal.Scan.config()
	if scanErrMsg != "" {
		m.err = fmt.Errorf("%s", scanErrMsg)
		return m, nil
	}
	m.err = nil
	ref := strings.TrimSpace(m.editUpstreamModal.RefInput.Value())
	name := strings.TrimSpace(m.editUpstreamModal.NameInput.Value())
	url := m.editUpstreamModal.URL

	ctx, cancel := context.WithCancel(m.ctx)
	m.cancelOp = cancel
	m.loading = true
	m.notice = i18n.T("notice.editing_upstream", url)
	return m, func() tea.Msg {
		if ctx.Err() != nil {
			return nil
		}
		err := m.service.UpstreamEdit(ctx, url, ref, name, scan)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return errMsg{err: err}
		}
		return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.upstream_edited"), url))
	}
}

func (m *Model) handleModalConfirmUpstreamRemoveKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "n", "N":
		m.view = viewList
		m.err = nil
		return m, nil

	case "enter", "y", "Y":
		url := m.confirmUpstreamRemoveModal.URL
		removeSkills := false
		ctx, cancel := context.WithCancel(m.ctx)
		m.cancelOp = cancel
		m.loading = true
		m.view = viewList
		m.notice = i18n.T("notice.removing_upstream", url)
		return m, func() tea.Msg {
			if ctx.Err() != nil {
				return nil
			}
			err := m.service.UpstreamRemove(ctx, url, removeSkills)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return errMsg{err: err}
			}
			return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.upstream_removed"), url))
		}

	case "s", "S":
		url := m.confirmUpstreamRemoveModal.URL
		removeSkills := true
		ctx, cancel := context.WithCancel(m.ctx)
		m.cancelOp = cancel
		m.loading = true
		m.view = viewList
		m.notice = i18n.T("notice.removing_upstream_with_skills", url)
		return m, func() tea.Msg {
			if ctx.Err() != nil {
				return nil
			}
			err := m.service.UpstreamRemove(ctx, url, removeSkills)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return errMsg{err: err}
			}
			return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.upstream_removed"), url))
		}
	}
	return m, nil
}

func (m *Model) handleModalUpstreamDetailKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", " ":
		m.view = viewList
		return m, nil

	case "enter", "s", "S":
		// Discover skills from this upstream and open add modal
		m.view = viewList
		u := m.upstreamDetailModal.Upstream
		return m, m.startUpstreamScan(u.URL, u.Ref)

	case "u", "U":
		// Update installed skills for this upstream
		u := m.upstreamDetailModal.Upstream
		if len(u.Skills) == 0 {
			m.notice = i18n.T("notice.upstream_no_imported_skills")
			return m, nil
		}
		m.view = viewList
		ctx, cancel := context.WithCancel(m.ctx)
		m.cancelOp = cancel
		m.loading = true
		m.notice = fmt.Sprintf(i18n.T("notice.updating_upstream"), u.URL)
		return m, func() tea.Msg {
			var count int
			for _, skID := range u.Skills {
				if ctx.Err() != nil {
					return nil
				}
				if _, err := m.service.Update(ctx, skID, false, false); err == nil {
					count++
				}
			}
			if ctx.Err() != nil {
				return nil
			}
			return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.updated_all_skills"), count))
		}

	case "r", "R":
		// Refresh from local cache
		m.notice = i18n.T("notice.upstreams_cache_refreshed")
		return m, m.reloadAsync()

	case "up", "k":
		m.upstreamDetailModal.Viewport.LineUp(1)
		return m, nil

	case "down", "j":
		m.upstreamDetailModal.Viewport.LineDown(1)
		return m, nil

	case "pgup", "b", "ctrl+u":
		m.upstreamDetailModal.Viewport.HalfViewUp()
		return m, nil

	case "pgdown", "f", "ctrl+d":
		m.upstreamDetailModal.Viewport.HalfViewDown()
		return m, nil

	case "g", "home":
		m.upstreamDetailModal.Viewport.GotoTop()
		return m, nil

	case "G", "end":
		m.upstreamDetailModal.Viewport.GotoBottom()
		return m, nil

	default:
		var cmd tea.Cmd
		m.upstreamDetailModal.Viewport, cmd = m.upstreamDetailModal.Viewport.Update(msg)
		return m, cmd
	}
}

func (m *Model) openBatchRemoveModal() (tea.Model, tea.Cmd) {
	m.view = viewModalBatchRemove
	m.batchRemoveModal = newBatchRemoveModal()

	// 1. Collect unique sources from m.skills
	srcMap := make(map[string]bool)
	for _, sk := range m.skills {
		srcName := ""
		switch sk.Source.Type {
		case model.SourceTypeGit:
			if sk.Source.URL != "" {
				srcName = sk.Source.URL
			}
		case model.SourceTypeLocal:
			if sk.Source.Path != "" {
				srcName = sk.Source.Path
			}
		case model.SourceTypeManaged:
			srcName = "managed"
		}
		if srcName != "" {
			srcMap[srcName] = true
		}
	}
	sources := make([]string, 0, len(srcMap))
	for src := range srcMap {
		sources = append(sources, src)
	}
	sort.Strings(sources)
	m.batchRemoveModal.Sources = sources

	// 2. Collect unique channels from deployments and configured targets
	m.batchRemoveModal.Deployments = m.deployments
	chMap := make(map[string]bool)
	for _, deps := range m.batchRemoveModal.Deployments {
		for _, d := range deps {
			chMap[d] = true
		}
	}
	for name := range m.targets {
		chMap[fmt.Sprintf("%s (agent)", name)] = true
	}
	channels := make([]string, 0, len(chMap))
	for ch := range chMap {
		channels = append(channels, ch)
	}
	sort.Strings(channels)
	m.batchRemoveModal.Channels = channels

	m.updateBatchRemoveMatches()
	return m, nil
}

func (m *Model) updateBatchRemoveMatches() {
	switch m.batchRemoveModal.Mode {
	case BatchRemoveByRegex:
		pat := strings.TrimSpace(m.batchRemoveModal.RegexInput.Value())
		if pat == "" {
			m.batchRemoveModal.MatchedSkills = nil
			m.batchRemoveModal.RegexError = ""
			return
		}
		_, err := regexp.Compile(pat)
		if err != nil {
			m.batchRemoveModal.RegexError = err.Error()
			m.batchRemoveModal.MatchedSkills = nil
			return
		}
		m.batchRemoveModal.RegexError = ""
		matched, _ := m.service.FilterSkills(m.ctx, app.SkillFilterOptions{Pattern: pat})
		m.batchRemoveModal.MatchedSkills = matched

	case BatchRemoveBySource:
		m.batchRemoveModal.RegexError = ""
		if len(m.batchRemoveModal.Sources) == 0 || m.batchRemoveModal.SourceCursor >= len(m.batchRemoveModal.Sources) {
			m.batchRemoveModal.MatchedSkills = nil
			return
		}
		chosenSrc := m.batchRemoveModal.Sources[m.batchRemoveModal.SourceCursor]
		var matched []model.SkillStatus
		for _, s := range m.skills {
			if s.Source.URL == chosenSrc || s.Source.Path == chosenSrc || (chosenSrc == "managed" && s.Source.Type == model.SourceTypeManaged) {
				matched = append(matched, s)
			}
		}
		m.batchRemoveModal.MatchedSkills = matched

	case BatchRemoveByChannel:
		m.batchRemoveModal.RegexError = ""
		if len(m.batchRemoveModal.Channels) == 0 || m.batchRemoveModal.ChannelCursor >= len(m.batchRemoveModal.Channels) {
			m.batchRemoveModal.MatchedSkills = nil
			return
		}
		chosenCh := m.batchRemoveModal.Channels[m.batchRemoveModal.ChannelCursor]
		var matched []model.SkillStatus
		for _, s := range m.skills {
			deps := m.batchRemoveModal.Deployments[s.ID]
			for _, d := range deps {
				if d == chosenCh {
					matched = append(matched, s)
					break
				}
			}
		}
		m.batchRemoveModal.MatchedSkills = matched
	}
}

func (m *Model) handleBatchRemoveModalKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "right", "l":
		if m.batchRemoveModal.Mode != BatchRemoveByRegex || m.batchRemoveModal.RegexInput.Value() == "" {
			m.batchRemoveModal.Mode = (m.batchRemoveModal.Mode + 1) % 3
			if m.batchRemoveModal.Mode == BatchRemoveByRegex {
				m.batchRemoveModal.RegexInput.Focus()
			} else {
				m.batchRemoveModal.RegexInput.Blur()
			}
			m.updateBatchRemoveMatches()
			return m, nil
		}

	case "left", "h":
		if m.batchRemoveModal.Mode != BatchRemoveByRegex || m.batchRemoveModal.RegexInput.Value() == "" {
			m.batchRemoveModal.Mode = (m.batchRemoveModal.Mode + 2) % 3
			if m.batchRemoveModal.Mode == BatchRemoveByRegex {
				m.batchRemoveModal.RegexInput.Focus()
			} else {
				m.batchRemoveModal.RegexInput.Blur()
			}
			m.updateBatchRemoveMatches()
			return m, nil
		}

	case "1":
		if m.batchRemoveModal.Mode != BatchRemoveByRegex {
			m.batchRemoveModal.Mode = BatchRemoveByRegex
			m.batchRemoveModal.RegexInput.Focus()
			m.updateBatchRemoveMatches()
			return m, nil
		}
	case "2":
		if m.batchRemoveModal.Mode != BatchRemoveByRegex {
			m.batchRemoveModal.Mode = BatchRemoveBySource
			m.batchRemoveModal.RegexInput.Blur()
			m.updateBatchRemoveMatches()
			return m, nil
		}
	case "3":
		if m.batchRemoveModal.Mode != BatchRemoveByRegex {
			m.batchRemoveModal.Mode = BatchRemoveByChannel
			m.batchRemoveModal.RegexInput.Blur()
			m.updateBatchRemoveMatches()
			return m, nil
		}

	case "up", "k":
		if m.batchRemoveModal.Mode == BatchRemoveBySource {
			if m.batchRemoveModal.SourceCursor > 0 {
				m.batchRemoveModal.SourceCursor--
				m.updateBatchRemoveMatches()
			}
			return m, nil
		} else if m.batchRemoveModal.Mode == BatchRemoveByChannel {
			if m.batchRemoveModal.ChannelCursor > 0 {
				m.batchRemoveModal.ChannelCursor--
				m.updateBatchRemoveMatches()
			}
			return m, nil
		}

	case "down", "j":
		if m.batchRemoveModal.Mode == BatchRemoveBySource {
			if m.batchRemoveModal.SourceCursor < len(m.batchRemoveModal.Sources)-1 {
				m.batchRemoveModal.SourceCursor++
				m.updateBatchRemoveMatches()
			}
			return m, nil
		} else if m.batchRemoveModal.Mode == BatchRemoveByChannel {
			if m.batchRemoveModal.ChannelCursor < len(m.batchRemoveModal.Channels)-1 {
				m.batchRemoveModal.ChannelCursor++
				m.updateBatchRemoveMatches()
			}
			return m, nil
		}

	case "ctrl+s":
		if len(m.batchRemoveModal.MatchedSkills) == 0 {
			m.notice = i18n.T("modal.batch_remove.no_match")
			return m, nil
		}
		if m.selectedSkills == nil {
			m.selectedSkills = make(map[string]bool)
		}
		for _, sk := range m.batchRemoveModal.MatchedSkills {
			m.selectedSkills[sk.ID] = true
		}
		count := len(m.batchRemoveModal.MatchedSkills)
		m.view = viewList
		m.notice = fmt.Sprintf(i18n.T("notice.batch_remove_selected"), count)
		return m, nil

	case "s", "S":
		if m.batchRemoveModal.Mode != BatchRemoveByRegex {
			if len(m.batchRemoveModal.MatchedSkills) == 0 {
				m.notice = i18n.T("modal.batch_remove.no_match")
				return m, nil
			}
			if m.selectedSkills == nil {
				m.selectedSkills = make(map[string]bool)
			}
			for _, sk := range m.batchRemoveModal.MatchedSkills {
				m.selectedSkills[sk.ID] = true
			}
			count := len(m.batchRemoveModal.MatchedSkills)
			m.view = viewList
			m.notice = fmt.Sprintf(i18n.T("notice.batch_remove_selected"), count)
			return m, nil
		}

	case "enter":
		if len(m.batchRemoveModal.MatchedSkills) == 0 {
			m.notice = i18n.T("modal.batch_remove.no_match")
			return m, nil
		}
		var ids []string
		depMap := make(map[string][]string)
		hasModified := false
		for _, sk := range m.batchRemoveModal.MatchedSkills {
			ids = append(ids, sk.ID)
			if deps := m.batchRemoveModal.Deployments[sk.ID]; len(deps) > 0 {
				depMap[sk.ID] = deps
			}
			if sk.ManagedStatus == model.StatusModified {
				hasModified = true
			}
		}

		m.confirmModal = ConfirmationModalState{
			Title:       fmt.Sprintf(i18n.T("confirm.remove_skills_batch.title"), len(ids)),
			Message:     fmt.Sprintf(i18n.T("confirm.remove_skills_batch.msg"), len(ids)),
			Danger:      true,
			AllowForce:  hasModified || m.batchRemoveModal.Force,
			Action:      "remove-skills-batch",
			SkillIDs:    ids,
			Deployments: depMap,
		}
		m.view = viewModalConfirm
		return m, nil

	case "ctrl+f":
		m.batchRemoveModal.Force = !m.batchRemoveModal.Force
		return m, nil

	case "f":
		if m.batchRemoveModal.Mode != BatchRemoveByRegex {
			m.batchRemoveModal.Force = !m.batchRemoveModal.Force
			return m, nil
		}

	case "esc":
		m.view = viewList
		return m, nil
	}

	if m.batchRemoveModal.Mode == BatchRemoveByRegex {
		var cmd tea.Cmd
		m.batchRemoveModal.RegexInput, cmd = m.batchRemoveModal.RegexInput.Update(msg)
		m.updateBatchRemoveMatches()
		return m, cmd
	}

	return m, nil
}

func (m *Model) clampDeployCursors() {
	st := &m.deployModal
	switch st.Step {
	case DeployStepSelectSkills:
		if st.SubMode == DeploySubModeSkills {
			vis := st.VisibleSkills()
			if st.SkillsCursor >= len(vis) {
				st.SkillsCursor = max(0, len(vis)-1)
			}
		} else {
			vis := st.VisibleGroups()
			if st.GroupCursor >= len(vis) {
				st.GroupCursor = max(0, len(vis)-1)
			}
		}
	case DeployStepSelectTarget:
		if st.TargetTab == DeployTargetTabAgent {
			vis := st.VisibleAgentTargets()
			if st.AgentCursor >= len(vis) {
				st.AgentCursor = max(0, len(vis)-1)
			}
		} else {
			vis := st.VisibleProjectTargets()
			if st.ProjectCursor >= len(vis) {
				st.ProjectCursor = max(0, len(vis)-1)
			}
		}
	}
}

func (m *Model) moveDeployCursorUp() {
	st := &m.deployModal
	switch st.Step {
	case DeployStepSelectSkills:
		if st.SubMode == DeploySubModeSkills {
			if st.SkillsCursor > 0 {
				st.SkillsCursor--
			}
		} else {
			if st.GroupCursor > 0 {
				st.GroupCursor--
			}
		}
	case DeployStepSelectTarget:
		if st.TargetTab == DeployTargetTabAgent {
			if st.AgentCursor > 0 {
				st.AgentCursor--
			}
		} else {
			if st.ProjectCursor > 0 {
				st.ProjectCursor--
			}
		}
	case DeployStepSelectFormat:
		if st.FormatCursor > 0 {
			st.FormatCursor--
		}
	}
}

func (m *Model) moveDeployCursorDown() {
	st := &m.deployModal
	switch st.Step {
	case DeployStepSelectSkills:
		if st.SubMode == DeploySubModeSkills {
			vis := st.VisibleSkills()
			if st.SkillsCursor < len(vis)-1 {
				st.SkillsCursor++
			}
		} else {
			vis := st.VisibleGroups()
			if st.GroupCursor < len(vis)-1 {
				st.GroupCursor++
			}
		}
	case DeployStepSelectTarget:
		if st.TargetTab == DeployTargetTabAgent {
			vis := st.VisibleAgentTargets()
			if st.AgentCursor < len(vis)-1 {
				st.AgentCursor++
			}
		} else {
			vis := st.VisibleProjectTargets()
			if st.ProjectCursor < len(vis)-1 {
				st.ProjectCursor++
			}
		}
	case DeployStepSelectFormat:
		if st.FormatCursor < len(st.FormatIDs)-1 {
			st.FormatCursor++
		}
	}
}

func (m *Model) handleDeployKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	st := &m.deployModal

	switch st.Step {
	case DeployStepSelectSkills:
		if st.Searching {
			switch msg.String() {
			case "esc":
				st.Searching = false
				st.SearchInput.Blur()
				return m, nil
			case "enter":
				st.Searching = false
				st.SearchInput.Blur()
				return m, nil
			case "up", "down":
				st.Searching = false
				st.SearchInput.Blur()
			default:
				var cmd tea.Cmd
				st.SearchInput, cmd = st.SearchInput.Update(msg)
				m.clampDeployCursors()
				return m, cmd
			}
		}

		// SubMode switcher
		switch msg.String() {
		case "/":
			st.Searching = true
			st.SearchInput.Focus()
			return m, textinput.Blink
		case "left", "right", "h", "l":
			if st.SubMode == DeploySubModeSkills {
				st.SubMode = DeploySubModeGroup
			} else {
				st.SubMode = DeploySubModeSkills
			}
			m.clampDeployCursors()
			return m, nil
		case "1":
			st.SubMode = DeploySubModeSkills
			m.clampDeployCursors()
			return m, nil
		case "2":
			st.SubMode = DeploySubModeGroup
			m.clampDeployCursors()
			return m, nil
		case "esc":
			if st.SearchInput.Value() != "" {
				st.SearchInput.SetValue("")
				m.clampDeployCursors()
				return m, nil
			}
			m.view = viewList
			m.err = nil
			return m, nil
		case "q":
			m.view = viewList
			m.err = nil
			return m, nil
		}

		if st.SubMode == DeploySubModeSkills {
			switch msg.String() {
			case "up", "k":
				m.moveDeployCursorUp()
			case "down", "j":
				m.moveDeployCursorDown()
			case " ":
				vis := st.VisibleSkills()
				if len(vis) > 0 && st.SkillsCursor < len(vis) {
					id := vis[st.SkillsCursor].ID
					st.SelectedSkills[id] = !st.SelectedSkills[id]
				}
			case "a":
				vis := st.VisibleSkills()
				if len(vis) > 0 {
					allSelected := true
					for _, s := range vis {
						if !st.SelectedSkills[s.ID] {
							allSelected = false
							break
						}
					}
					for _, s := range vis {
						st.SelectedSkills[s.ID] = !allSelected
					}
				}
			case "enter":
				var selected []string
				for _, s := range st.Skills {
					if st.SelectedSkills[s.ID] {
						selected = append(selected, s.ID)
					}
				}
				if len(selected) == 0 {
					if st.IsUndeploy {
						m.err = fmt.Errorf("%s", i18n.T("error.select_one_skill_undeploy"))
					} else {
						m.err = fmt.Errorf("%s", i18n.T("error.select_one_skill_deploy"))
					}
					return m, nil
				}
				m.err = nil
				st.ResolvedSkillIDs = selected
				return m.proceedFromSkillSelection(st)
			}
		} else {
			// SubMode == DeploySubModeGroup
			switch msg.String() {
			case "up", "k":
				m.moveDeployCursorUp()
			case "down", "j":
				m.moveDeployCursorDown()
			case "enter":
				vis := st.VisibleGroups()
				if len(vis) == 0 {
					return m, nil
				}
				if st.GroupCursor >= len(vis) {
					st.GroupCursor = len(vis) - 1
				}
				g := vis[st.GroupCursor]
				if len(g.SkillIDs) == 0 {
					m.err = fmt.Errorf("%s", i18n.T("error.group_no_skills", g.Name))
					return m, nil
				}
				st.ResolvedSkillIDs = g.SkillIDs
				m.err = nil
				return m.proceedFromSkillSelection(st)
			}
		}

	case DeployStepSelectTarget:
		if st.Searching {
			switch msg.String() {
			case "esc":
				st.Searching = false
				st.SearchInput.Blur()
				return m, nil
			case "enter":
				st.Searching = false
				st.SearchInput.Blur()
				return m, nil
			case "up", "down":
				st.Searching = false
				st.SearchInput.Blur()
			default:
				var cmd tea.Cmd
				st.SearchInput, cmd = st.SearchInput.Update(msg)
				m.clampDeployCursors()
				return m, cmd
			}
		}

		switch msg.String() {
		case "/":
			st.Searching = true
			st.SearchInput.Focus()
			return m, textinput.Blink
		case "left", "right", "h", "l":
			if st.TargetTab == DeployTargetTabAgent {
				st.TargetTab = DeployTargetTabProject
			} else {
				st.TargetTab = DeployTargetTabAgent
			}
			m.clampDeployCursors()
			return m, nil
		case "1":
			st.TargetTab = DeployTargetTabAgent
			m.clampDeployCursors()
			return m, nil
		case "2":
			st.TargetTab = DeployTargetTabProject
			m.clampDeployCursors()
			return m, nil

		case "up", "k":
			m.moveDeployCursorUp()
		case "down", "j":
			m.moveDeployCursorDown()
		case "enter":
			if st.TargetTab == DeployTargetTabProject {
				vis := st.VisibleProjectTargets()
				if len(vis) == 0 {
					m.err = fmt.Errorf("%s", i18n.T("error.no_project_targets"))
					return m, nil
				}
				if st.ProjectCursor >= len(vis) {
					st.ProjectCursor = len(vis) - 1
				}
				projItem := vis[st.ProjectCursor]
				projPath := st.ProjectTargetPaths[projItem]
				if projPath == "" {
					projPath = projItem
				}
				st.SelectedProject = projPath
				st.Step = DeployStepSelectFormat
				return m, nil
			}

			// DeployTargetTabAgent
			vis := st.VisibleAgentTargets()
			if len(vis) == 0 {
				m.err = fmt.Errorf("%s", i18n.T("error.no_agent_targets"))
				return m, nil
			}
			if st.AgentCursor >= len(vis) {
				st.AgentCursor = len(vis) - 1
			}
			targetName := vis[st.AgentCursor]
			return m.executeAgentDeploy(targetName, st.ResolvedSkillIDs, st.IsUndeploy)

		case "esc":
			if st.SearchInput.Value() != "" {
				st.SearchInput.SetValue("")
				m.clampDeployCursors()
				return m, nil
			}
			m.err = nil
			st.Step = DeployStepSelectSkills
			st.Searching = false
			st.SearchInput.Blur()
			st.SearchInput.SetValue("")
			m.clampDeployCursors()
		case "q":
			m.err = nil
			st.Step = DeployStepSelectSkills
			st.Searching = false
			st.SearchInput.Blur()
			st.SearchInput.SetValue("")
			m.clampDeployCursors()
		}

	case DeployStepSelectFormat:
		switch msg.String() {
		case "up", "k":
			m.moveDeployCursorUp()
		case "down", "j":
			m.moveDeployCursorDown()
		case " ": // toggle format
			if st.FormatCursor < len(st.FormatIDs) {
				fmtID := st.FormatIDs[st.FormatCursor]
				st.SelectedFormats[fmtID] = !st.SelectedFormats[fmtID]
			}
		case "enter":
			var chosenFormats []string
			for _, fmtID := range st.FormatIDs {
				if st.SelectedFormats[fmtID] {
					chosenFormats = append(chosenFormats, fmtID)
				}
			}
			if len(chosenFormats) == 0 {
				chosenFormats = []string{model.FormatStandard}
			}

			toAction := st.ResolvedSkillIDs
			isUndeploy := st.IsUndeploy
			projPath := st.SelectedProject

			m.loading = true
			actionWord := i18n.T("deploy.verb_deploying")
			if isUndeploy {
				actionWord = i18n.T("deploy.verb_undeploying")
			}
			m.notice = i18n.T("notice.deploying_progress_project", actionWord, len(toAction), projPath)
			ctx, cancel := context.WithCancel(m.ctx)
			m.cancelOp = cancel

			return m, func() tea.Msg {
				var count int
				var failed []string
				var conflicts []*deploy.TargetConflictError
				for i, id := range toAction {
					if ctx.Err() != nil {
						return nil
					}
					if m.program != nil {
						m.program.Send(progressMsg{phase: progress.PhaseDeploy, current: i + 1, total: len(toAction), detail: id})
					}
					var err error
					if isUndeploy {
						_, err = m.service.UndeployProject(ctx, id, projPath, chosenFormats, false)
					} else {
						_, _, err = m.service.DeployProject(ctx, id, projPath, chosenFormats, false)
					}
					if err != nil {
						var conf *deploy.TargetConflictError
						if errors.As(err, &conf) {
							conflicts = append(conflicts, conf)
							failed = append(failed, i18n.T("deploy.failed_modified", id))
						} else {
							failed = append(failed, fmt.Sprintf("%s (%v)", id, err))
						}
					} else {
						count++
					}
				}
				if ctx.Err() != nil {
					return nil
				}
				if len(conflicts) > 0 {
					return deployConflictMsg{
						Conflict:      conflicts[0],
						Queue:         conflicts[1:],
						DeployedCount: count,
						TotalCount:    len(toAction),
					}
				}
				if len(failed) > 0 {
					return asyncNoticeMsg(i18n.T("notice.deploy_result_failed", actionWord, count, projPath, len(failed), strings.Join(failed, "; ")))
				}
				actionDone := i18n.T("deploy.past_deployed")
				if isUndeploy {
					actionDone = i18n.T("deploy.past_undeployed")
				}
				return asyncNoticeMsg(i18n.T("notice.deploy_result_ok_formats", actionDone, count, projPath, strings.Join(chosenFormats, ", ")))
			}

		case "esc":
			m.err = nil
			if st.LockedTargetName != "" {
				st.Step = DeployStepSelectSkills
			} else {
				st.Step = DeployStepSelectTarget
			}
			st.Searching = false
			st.SearchInput.Blur()
			st.SearchInput.SetValue("")
			m.clampDeployCursors()
		case "q":
			m.err = nil
			if st.LockedTargetName != "" {
				st.Step = DeployStepSelectSkills
			} else {
				st.Step = DeployStepSelectTarget
			}
			st.Searching = false
			st.SearchInput.Blur()
			st.SearchInput.SetValue("")
			m.clampDeployCursors()
		}
	}
	return m, nil
}

func (m *Model) proceedFromSkillSelection(st *DeployModalState) (tea.Model, tea.Cmd) {
	st.Searching = false
	st.SearchInput.Blur()
	st.SearchInput.SetValue("")
	m.clampDeployCursors()
	if st.LockedTargetName != "" {
		if st.LockedTargetType == "project" {
			st.SelectedProject = st.LockedProjectPath
			st.Step = DeployStepSelectFormat
			return m, nil
		}
		return m.executeAgentDeploy(st.LockedTargetName, st.ResolvedSkillIDs, st.IsUndeploy)
	}
	st.Step = DeployStepSelectTarget
	return m, nil
}

func (m *Model) executeAgentDeploy(actualTarget string, toAction []string, isUndeploy bool) (tea.Model, tea.Cmd) {
	m.loading = true
	actionWord := i18n.T("deploy.verb_deploying")
	if isUndeploy {
		actionWord = i18n.T("deploy.verb_undeploying")
	}
	m.notice = i18n.T("notice.deploying_progress_target", actionWord, len(toAction), actualTarget)
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancelOp = cancel

	return m, func() tea.Msg {
		var count int
		var failed []string
		var conflicts []*deploy.TargetConflictError
		for i, id := range toAction {
			if ctx.Err() != nil {
				return nil
			}
			if m.program != nil {
				m.program.Send(progressMsg{phase: progress.PhaseDeploy, current: i + 1, total: len(toAction), detail: id})
			}
			var err error
			if isUndeploy {
				err = m.service.Undeploy(ctx, id, actualTarget, false)
			} else {
				err = m.service.Deploy(ctx, id, actualTarget, false)
			}
			if err != nil {
				var conf *deploy.TargetConflictError
				if errors.As(err, &conf) {
					conflicts = append(conflicts, conf)
					failed = append(failed, i18n.T("deploy.failed_modified", id))
				} else {
					failed = append(failed, fmt.Sprintf("%s (%v)", id, err))
				}
			} else {
				count++
			}
		}
		if ctx.Err() != nil {
			return nil
		}
		if len(conflicts) > 0 {
			return deployConflictMsg{
				Conflict:      conflicts[0],
				Queue:         conflicts[1:],
				DeployedCount: count,
				TotalCount:    len(toAction),
			}
		}
		if len(failed) > 0 {
			return asyncNoticeMsg(i18n.T("notice.deploy_result_failed", actionWord, count, actualTarget, len(failed), strings.Join(failed, "; ")))
		}
		actionDone := i18n.T("deploy.past_deployed")
		if isUndeploy {
			actionDone = i18n.T("deploy.past_undeployed")
		}
		return asyncNoticeMsg(i18n.T("notice.deploy_result_ok", actionDone, count, actualTarget))
	}
}

// primaryTabStyle returns the style for a top-level tab item: focused (purple)
// when the primary level is active, or the dimmer ancestor tone when the user
// has descended into the secondary level.
func (m *Model) primaryTabStyle(selected bool) lipgloss.Style {
	if !selected {
		return inactiveTabStyle
	}
	if m.focusDepth == 1 {
		return ancestorTabStyle
	}
	return activeTabStyle
}

// subTabStyle returns the style for a secondary tab item: focused (purple) when
// the secondary level is active, selected (blue) when the secondary is merely
// displayed, and inactive otherwise.
func (m *Model) subTabStyle(selected bool) lipgloss.Style {
	if !selected {
		return subTabInactiveStyle
	}
	if m.focusDepth == 1 {
		return subTabFocusedStyle
	}
	return subTabSelectedStyle
}

func (m *Model) renderTabsBar() string {
	names := []string{
		i18n.T("tab.dashboard"),
		i18n.T("tab.upstreams"),
		i18n.T("tab.skills"),
		i18n.T("tab.channels"),
		i18n.T("tab.projects"),
		i18n.T("tab.system"),
		i18n.T("tab.settings"),
	}
	tabs := make([]string, len(names))
	for i, name := range names {
		tabs[i] = fmt.Sprintf("%d. %s", i+1, name)
	}
	var renderedTabs []string
	if m.width < 110 {
		idx := int(m.activeTab)
		if idx < 0 || idx >= len(tabs) {
			idx = 0
		}
		renderedTabs = append(renderedTabs,
			inactiveTabStyle.Render("◀"),
			m.primaryTabStyle(true).Render(tabs[idx]),
			inactiveTabStyle.Render("▶"),
		)
	} else {
		for i, t := range tabs {
			renderedTabs = append(renderedTabs, m.primaryTabStyle(activeTab(i) == m.activeTab).Render(t))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, renderedTabs...) + "\n\n"
}

// splitLines splits s into visual lines, ignoring a single trailing newline
// that only acts as a separator before the following fragment.
func splitLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// searchActive reports whether the search bar is currently rendered.
func (m *Model) searchActive() bool {
	return m.view == viewSearch || m.searchInput.Value() != ""
}

// renderHeader renders the fixed top chrome: tab bar and optional search bar.
func (m *Model) renderHeader() string {
	var b strings.Builder
	b.WriteString(m.renderTabsBar())
	if m.searchActive() {
		b.WriteString("  🔍 " + m.searchInput.View() + "\n\n")
	}
	return b.String()
}

// renderTail renders the fixed bottom chrome: notice/error line and footer.
func (m *Model) renderTail() string {
	var b strings.Builder
	if !m.isModalView() && !m.loading {
		if m.err != nil {
			b.WriteString("\n" + statusErrorStyle.Render("Error: "+m.err.Error()))
		} else if m.notice != "" {
			b.WriteString("\n" + statusNoticeStyle.Render("ℹ "+m.notice))
		}
	}
	b.WriteString("\n" + m.renderFooter())
	return b.String()
}

// chromeLines returns the number of fixed lines consumed by the header and
// tail of the base layout.
func (m *Model) chromeLines() int {
	return len(splitLines(m.renderHeader())) + len(splitLines(m.renderTail()))
}

// listRows returns the number of scrollable rows available in the content area
// after reserving the fixed chrome and a page's own fixed lines.
func (m *Model) listRows(fixed int) int {
	if m.height <= 0 {
		return 20
	}
	n := m.height - m.chromeLines() - fixed
	if n < 3 {
		n = 3
	}
	return n
}

// listMax returns the visible row budget for a list page, reserving one line
// for the "more above/below" indicator when the list overflows.
func (m *Model) listMax(fixed, total int) int {
	n := m.listRows(fixed)
	if total > n && n > 1 {
		n--
	}
	return n
}

func (m *Model) renderBaseLayout() string {
	header := m.renderHeader()

	var body string
	switch m.activeTab {
	case tabDashboard:
		if m.height > 0 {
			m.dashboardViewport.Height = m.listRows(0)
		}
		m.dashboardViewport.SetContent(m.renderDashboard())
		body = m.dashboardViewport.View()
	case tabUpstreams:
		body = m.renderUpstreamsList()
	case tabSkills:
		body = m.renderSkillsTab()
	case tabChannels:
		body = m.renderChannelsList()
	case tabProjects:
		body = m.renderProjectsList()
	case tabSystem:
		body = m.renderSystemTab()
	case tabSettings:
		body = m.renderSettingsList()
	default:
		if m.height > 0 {
			m.dashboardViewport.Height = m.listRows(0)
		}
		m.dashboardViewport.SetContent(m.renderDashboard())
		body = m.dashboardViewport.View()
	}

	tail := m.renderTail()

	// Pin the header and tail; clip the content area to the remaining height so
	// the top-level tab bar can never scroll off the top of the terminal.
	if m.height > 0 {
		avail := m.height - len(splitLines(header)) - len(splitLines(tail))
		if avail < 1 {
			avail = 1
		}
		lines := splitLines(body)
		if len(lines) > avail {
			body = strings.Join(lines[:avail], "\n") + "\n"
		}
	}

	return header + body + tail
}

func (m *Model) isModalView() bool {
	switch m.view {
	case viewModalAdd,
		viewModalNew,
		viewModalAddTarget,
		viewModalAddProject,
		viewModalAddGroup,
		viewModalGroupSkills,
		viewModalEditSetting,
		viewModalSelectLanguage,
		viewModalSelectWorkspace,
		viewModalAddWorkspace,
		viewModalTargetDetail,
		viewModalDeployConflict,
		viewModalConfirm,
		viewModalBatchRemove,
		viewModalAddUpstream,
		viewModalEditUpstream,
		viewModalConfirmUpstreamRemove,
		viewModalUpstreamDetail,
		viewModalModelDetail,
		viewModalModelConfigFile,
		viewModalProviderDetail,
		viewModalEditField,
		viewModalEditJSON,
		viewDeploy,
		viewUndeploy,
		viewHelp:
		return true
	default:
		return false
	}
}

func (m *Model) renderActiveModal() string {
	modalH := m.height - 4
	if modalH < 12 {
		modalH = 12
	}

	switch m.view {
	case viewModalAdd:
		return RenderAddModal(&m.addModal, boxStyle, m.loading, m.progressBlock(), m.err, m.height)

	case viewModalNew:
		return RenderNewModal(m.newInput, boxStyle, m.loading, m.progressBlock(), m.err)

	case viewModalAddTarget:
		return RenderTargetModal(&m.targetModal, boxStyle, m.loading, m.progressBlock(), m.err)

	case viewModalAddProject:
		return RenderProjectModal(&m.projectModal, boxStyle, m.loading, m.progressBlock(), m.err)

	case viewModalAddGroup:
		return RenderGroupModal(&m.groupModal, boxStyle, m.loading, m.progressBlock(), m.err)

	case viewModalGroupSkills:
		return RenderGroupSkillsModal(&m.groupSkillsModal, boxStyle, modalH, m.loading, m.progressBlock(), m.err)

	case viewModalEditSetting:
		return RenderEditSettingModal(&m.editSettingModal, boxStyle, m.err)

	case viewModalSelectLanguage:
		cfg, _ := m.service.Config()
		currLang := "auto"
		if cfg != nil && cfg.Language != "" {
			currLang = cfg.Language
		}
		return RenderSelectLanguageModal(&m.languageModal, boxStyle, currLang)

	case viewModalSelectWorkspace:
		return RenderSelectWorkspaceModal(&m.selectWorkspaceModal, boxStyle, m.err)

	case viewModalAddWorkspace:
		return RenderAddWorkspaceModal(&m.addWorkspaceModal, boxStyle, m.err)

	case viewModalTargetDetail:
		return RenderTargetDetailModal(&m.targetDetailModal, boxStyle, m.width, m.height)

	case viewModalDeployConflict:
		return RenderDeployConflictModal(&m.deployConflictModal, boxStyle, modalH, m.loading, m.progressBlock())

	case viewModalConfirm:
		return RenderConfirmationModal(&m.confirmModal, boxStyle, m.loading, m.progressBlock())

	case viewModalBatchRemove:
		return RenderBatchRemoveModal(&m.batchRemoveModal, boxStyle, modalH)

	case viewModalAddUpstream:
		return RenderAddUpstreamModal(&m.addUpstreamModal, boxStyle, m.err)

	case viewModalEditUpstream:
		return RenderEditUpstreamModal(&m.editUpstreamModal, boxStyle, m.err)

	case viewModalConfirmUpstreamRemove:
		return RenderConfirmUpstreamRemoveModal(&m.confirmUpstreamRemoveModal, boxStyle)

	case viewModalUpstreamDetail:
		return RenderUpstreamDetailModal(&m.upstreamDetailModal, boxStyle, m.width, m.height)

	case viewModalModelDetail:
		return RenderModelDetailModal(&m.modelDetailModal, boxStyle, m.width, m.height)

	case viewModalModelConfigFile:
		return RenderModelConfigFileModal(&m.modelConfigFileModal, boxStyle, m.width, m.height)

	case viewModalProviderDetail:
		return RenderProviderDetailModal(&m.providerDetailModal, boxStyle, m.width, m.height)

	case viewModalEditField:
		return RenderFieldEditModal(&m.editFieldModal, boxStyle)

	case viewModalEditJSON:
		return RenderJSONEditModal(&m.editJSONModal, boxStyle, m.width, m.height)

	case viewDeploy, viewUndeploy:
		return RenderDeployModal(&m.deployModal, boxStyle, modalH, m.loading, m.progressBlock(), m.err)

	case viewHelp:
		return m.renderHelpModal()
	}
	return ""
}

func (m *Model) overlayModal(bg, modal string) string {
	mW := lipgloss.Width(modal)
	mH := lipgloss.Height(modal)
	bgW := lipgloss.Width(bg)
	bgH := lipgloss.Height(bg)

	w := m.width
	if w < mW {
		w = mW
	}
	if w < bgW {
		w = bgW
	}
	if w <= 0 {
		w = 120
	}

	// When terminal height is initialized (m.height > 0), do not expand buffer beyond
	// m.height, as rendering more lines than the terminal height causes terminal scrolling
	// and pushes the top of the UI off-screen.
	h := m.height
	if h <= 0 {
		h = 24
		if h < mH+3 {
			h = mH + 3
		}
		if h < bgH {
			h = bgH
		}
	}

	buf := cellbuf.NewBuffer(w, h)
	cellbuf.SetContent(buf, bg)

	// Horizontal centering
	x := (w - mW) / 2
	if x < 0 {
		x = 0
	}

	// Vertically center in the content area BELOW the tabs bar (rows 0-1 are reserved for tabs)
	contentH := h - 2
	y := 2
	if contentH > mH {
		y = 2 + (contentH-mH)/2
	}

	rect := cellbuf.Rect(x, y, mW, mH)
	cellbuf.SetContentRect(buf, modal, rect)

	rendered := cellbuf.Render(buf)
	rendered = strings.ReplaceAll(rendered, "\r\n", "\n")
	return strings.TrimRight(rendered, "\n") + "\n"
}

func (m *Model) renderLoadingModal() string {
	var loadCard strings.Builder
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A29BFE"))
	hintStyle := lipgloss.NewStyle().Faint(true)

	loadCard.WriteString(titleStyle.Render(i18n.T("loading.in_progress")) + "\n\n")
	block := m.progressBlock()
	loadCard.WriteString(block)
	loadCard.WriteString("\n\n" + hintStyle.Render(hintText(i18n.T("loading.lock_hint"))))

	cardWidth := progressBarWidth(m.width) + 6
	for _, line := range strings.Split(block, "\n") {
		if w := lipgloss.Width(line) + 6; w > cardWidth {
			cardWidth = w
		}
	}
	if titleW := lipgloss.Width(i18n.T("loading.in_progress")) + 6; titleW > cardWidth {
		cardWidth = titleW
	}
	if m.width > 0 && cardWidth > m.width-8 {
		cardWidth = m.width - 8
	}
	if cardWidth < 20 {
		cardWidth = 20
	}
	return boxStyle.Width(cardWidth).Render(loadCard.String())
}

func (m *Model) View() string {
	switch m.view {
	case viewDetail:
		var b strings.Builder
		b.WriteString(m.renderTabsBar())
		b.WriteString(m.renderSkillDetail())
		if m.loading {
			return m.overlayModal(b.String(), m.renderLoadingModal())
		}
		return b.String()

	case viewDiff, viewTargetDiff:
		var b strings.Builder
		b.WriteString(m.renderTabsBar())
		b.WriteString(m.renderDiffDetail())
		if m.loading {
			return m.overlayModal(b.String(), m.renderLoadingModal())
		}
		return b.String()
	}

	if m.isModalView() {
		bg := m.renderBaseLayout()
		modal := m.renderActiveModal()
		return m.overlayModal(bg, modal)
	}

	if m.loading {
		bg := m.renderBaseLayout()
		modal := m.renderLoadingModal()
		return m.overlayModal(bg, modal)
	}

	return m.renderBaseLayout()
}

func (m *Model) renderSkillsList() string {
	if len(m.filtered) == 0 {
		if len(m.skills) == 0 {
			return i18n.T("empty.skills")
		}
		return i18n.T("empty.skills_search")
	}

	nameW, statW, locHashW, sourceW, srcVerW := computeSkillColWidths(m.width)
	totalWidth := 6 + nameW + 1 + statW + 1 + locHashW + 1 + sourceW + 1 + srcVerW

	var b strings.Builder
	header := "      " +
		padCell(i18n.T("table.header.name"), nameW) + " " +
		padCell(i18n.T("table.header.status"), statW) + " " +
		padCell(i18n.T("table.header.local_hash"), locHashW) + " " +
		padCell(i18n.T("table.header.source"), sourceW) + " " +
		padCell(i18n.T("table.header.source_version"), srcVerW)
	b.WriteString(tableHeaderStyle.Render(header) + "\n")
	b.WriteString(tableSeparatorStyle.Render(strings.Repeat("─", totalWidth)) + "\n")

	maxVisible := m.listMax(4, len(m.filtered))

	if m.cursor < m.skillScrollOffset {
		m.skillScrollOffset = m.cursor
	}
	if m.cursor >= m.skillScrollOffset+maxVisible {
		m.skillScrollOffset = m.cursor - maxVisible + 1
	}
	maxOffset := len(m.filtered) - maxVisible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.skillScrollOffset > maxOffset {
		m.skillScrollOffset = maxOffset
	}
	if m.skillScrollOffset < 0 {
		m.skillScrollOffset = 0
	}

	start := m.skillScrollOffset
	end := start + maxVisible
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	if start > 0 {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(i18n.T("more.skills.above"), start)) + "\n")
	}

	for i := start; i < end; i++ {
		s := m.filtered[i]
		isSelected := (i == m.cursor)
		cursor := "  "
		if isSelected {
			cursor = "▶ "
		}

		chk := "[ ] "
		if m.selectedSkills[s.ID] {
			chk = "[✓] "
		}

		statusBadge := formatStatusBadge(s.ManagedStatus, isSelected)
		locHash := formatLocalHash(s)
		sourceCombined := formatSourceCombined(s.Source, isSelected)
		srcVer := formatSourceVersion(s, isSelected)

		row := cursor + chk +
			padCell(s.ID, nameW) + " " +
			padCell(statusBadge, statW) + " " +
			padCell(locHash, locHashW) + " " +
			padCell(sourceCombined, sourceW) + " " +
			padCell(srcVer, srcVerW)

		if isSelected {
			row = selectedRowStyle.Width(totalWidth).Render(row)
		}
		b.WriteString(row + "\n")
	}

	if end < len(m.filtered) {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(i18n.T("more.skills.below"), len(m.filtered)-end)) + "\n")
	}
	return b.String()
}

// renderSubTabBar renders a horizontal secondary-level tab row.
func (m *Model) renderSubTabBar(names []string, current int) string {
	rendered := make([]string, len(names))
	for i, n := range names {
		rendered[i] = m.subTabStyle(i == current).Render(n)
	}
	return strings.Join(rendered, "  ")
}

// filterStateLabel returns the localized label of the active status filter.
func filterStateLabel(mode targetFilterMode) string {
	switch mode {
	case targetFilterEnabledOnly:
		return i18n.T("target.filter.enabled")
	case targetFilterDisabledOnly:
		return i18n.T("target.filter.disabled")
	default:
		return i18n.T("target.filter.all")
	}
}

// renderFilterBar renders the compact enabled/disabled status filter row.
func (m *Model) renderFilterBar() string {
	line := fmt.Sprintf(i18n.T("target.filter.line"), filterStateLabel(m.targetFilter))
	return "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("#B2BEC3")).Render(line)
}

// renderChannelsList renders the channel page: an inline status filter in the
// channel selector line, a compact one-line channel summary and, for channels
// that support data sync, the provider/model table.
func (m *Model) renderChannelsList() string {
	var b strings.Builder

	entries := m.displayedChannels()
	if len(m.channelEntries) == 0 {
		b.WriteString(i18n.T("empty.channels"))
		return b.String()
	}

	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#B2BEC3"))
	channelLabel := fmt.Sprintf(i18n.T("model.channel.label_filter"), filterStateLabel(m.targetFilter))

	if len(entries) == 0 {
		b.WriteString("  " + labelStyle.Render(channelLabel) + "\n")
		b.WriteString(i18n.T("empty.targets_filtered_agent"))
		return b.String()
	}

	if m.channelCursor >= len(entries) {
		m.channelCursor = 0
	}

	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}
	b.WriteString("  " + labelStyle.Render(channelLabel) + ": " + m.renderSubTabBar(names, m.channelCursor) + "\n")

	e := entries[m.channelCursor]

	// Compact one-line channel summary: enabled dot, config dir, skill count and
	// (for channels with data sync) the current sync mode.
	dot := statusCleanStyle.Render("●")
	if !e.Target.Enabled {
		dot = statusErrorStyle.Render("●")
	}
	cfg := e.Target.ConfigDir
	if cfg == "" {
		cfg = "-"
	}
	summary := dot + " " + detailPathStyle.Render(fsx.CompactUser(cfg)) +
		"  " + fmt.Sprintf(i18n.T("channels.summary.skills"), e.Target.SkillCount)
	if e.Supported {
		modeKey := "model.mode.incremental"
		if m.modelOverride {
			modeKey = "model.mode.remote"
		}
		summary += "  " + statusNoticeStyle.Render(fmt.Sprintf(i18n.T("channels.summary.sync"), i18n.T(modeKey)))
	}
	b.WriteString("  " + summary + "\n\n")

	if !e.Supported {
		return b.String()
	}

	preLines := len(splitLines(b.String()))
	b.WriteString(m.renderModelsBody(preLines))
	return b.String()
}

func (m *Model) renderProjectsList() string {
	var b strings.Builder

	b.WriteString(m.renderFilterBar() + "\n\n")

	{
		// Project targets
		if len(m.projectTargets) == 0 {
			b.WriteString(i18n.T("empty.targets_project"))
			return b.String()
		}

		displayedProjs := m.displayedProjectTargets()
		if len(displayedProjs) == 0 {
			b.WriteString(i18n.T("empty.targets_filtered_project"))
			return b.String()
		}

		maxVisible := m.listMax(4, len(displayedProjs))

		statusW, nameW, pathW, featW, skillsW := computeProjectColWidths(m.width)
		totalWidth := 2 + statusW + 1 + nameW + 1 + pathW + 1 + featW + 1 + skillsW

		header := "  " +
			padCell(i18n.T("table.header.target_status"), statusW) + " " +
			padCell(i18n.T("table.header.project_name"), nameW) + " " +
			padCell(i18n.T("table.header.project_path"), pathW) + " " +
			padCell(i18n.T("table.header.project_features"), featW) + " " +
			padCell(i18n.T("table.header.project_skills"), skillsW)
		b.WriteString(tableHeaderStyle.Render(header) + "\n")
		b.WriteString(tableSeparatorStyle.Render(strings.Repeat("─", totalWidth)) + "\n")

		if m.projectCursor < m.projectScrollOffset {
			m.projectScrollOffset = m.projectCursor
		}
		if m.projectCursor >= m.projectScrollOffset+maxVisible {
			m.projectScrollOffset = m.projectCursor - maxVisible + 1
		}
		maxOffset := len(displayedProjs) - maxVisible
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.projectScrollOffset > maxOffset {
			m.projectScrollOffset = maxOffset
		}
		if m.projectScrollOffset < 0 {
			m.projectScrollOffset = 0
		}

		start := m.projectScrollOffset
		end := start + maxVisible
		if end > len(displayedProjs) {
			end = len(displayedProjs)
		}

		if start > 0 {
			b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(i18n.T("more.targets.above"), start)) + "\n")
		}

		for i := start; i < end; i++ {
			p := displayedProjs[i]
			isSelected := (i == m.projectCursor)
			cursor := "  "
			if isSelected {
				cursor = "▶ "
			}
			pathDisp := p.Path
			if pathDisp == "" {
				pathDisp = "-"
			}
			featDisp := p.Formats
			if featDisp == "" {
				featDisp = "-"
			}
			skDisp := fmt.Sprintf("%d", p.SkillCount)
			statusBadge := formatTargetEnabledBadge(p.Enabled, isSelected)

			row := cursor +
				padCell(statusBadge, statusW) + " " +
				padCell(p.Name, nameW) + " " +
				padCell(pathDisp, pathW) + " " +
				padCell(featDisp, featW) + " " +
				padCell(skDisp, skillsW)
			if isSelected {
				row = selectedRowStyle.Width(totalWidth).Render(row)
			} else if !p.Enabled {
				row = lipgloss.NewStyle().Faint(true).Render(row)
			}
			b.WriteString(row + "\n")
		}

		if end < len(displayedProjs) {
			b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(i18n.T("more.targets.below"), len(displayedProjs)-end)) + "\n")
		}
	}

	return b.String()
}

// renderSkillsTab renders the Skills/Group secondary-level tab row and content.
func (m *Model) renderSkillsTab() string {
	var b strings.Builder
	names := []string{
		fmt.Sprintf(i18n.T("skills.subtab.skills"), len(m.skills)),
		fmt.Sprintf(i18n.T("skills.subtab.groups"), len(m.groupNames)),
	}
	b.WriteString("  " + m.renderSubTabBar(names, int(m.skillSubTab)) + "\n\n")
	if m.skillSubTab == skillTabGroups {
		b.WriteString(m.renderGroupsList())
	} else {
		b.WriteString(m.renderSkillsList())
	}
	return b.String()
}

// renderSystemTab renders the Cache/Diagnostics secondary-level tab row and content.
func (m *Model) renderSystemTab() string {
	var b strings.Builder
	names := []string{
		fmt.Sprintf(i18n.T("system.subtab.cache"), len(m.caches)),
		i18n.T("system.subtab.doctor"),
	}
	b.WriteString("  " + m.renderSubTabBar(names, int(m.systemSubTab)) + "\n\n")
	if m.systemSubTab == systemTabDoctor {
		b.WriteString(m.renderDoctorList())
	} else {
		b.WriteString(m.renderCacheList())
	}
	return b.String()
}

func (m *Model) renderGroupsList() string {
	if len(m.groupNames) == 0 {
		return i18n.T("empty.groups")
	}

	displayed := m.displayedGroupNames()
	if len(displayed) == 0 {
		return i18n.T("filter.search_empty")
	}

	nameW, countW, skillsW := computeGroupColWidths(m.width)
	totalWidth := 2 + nameW + 1 + countW + 1 + skillsW

	var b strings.Builder
	header := "  " +
		padCell(i18n.T("table.header.group_name"), nameW) + " " +
		padCell(i18n.T("table.header.group_count"), countW) + " " +
		padCell(i18n.T("table.header.group_skills"), skillsW)
	b.WriteString(tableHeaderStyle.Render(header) + "\n")
	b.WriteString(tableSeparatorStyle.Render(strings.Repeat("─", totalWidth)) + "\n")

	maxVisible := m.listMax(4, len(displayed))

	if m.groupCursor < m.groupScrollOffset {
		m.groupScrollOffset = m.groupCursor
	}
	if m.groupCursor >= m.groupScrollOffset+maxVisible {
		m.groupScrollOffset = m.groupCursor - maxVisible + 1
	}
	maxOffset := len(displayed) - maxVisible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.groupScrollOffset > maxOffset {
		m.groupScrollOffset = maxOffset
	}
	if m.groupScrollOffset < 0 {
		m.groupScrollOffset = 0
	}

	start := m.groupScrollOffset
	end := start + maxVisible
	if end > len(displayed) {
		end = len(displayed)
	}

	if start > 0 {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(i18n.T("more.groups.above"), start)) + "\n")
	}

	for i := start; i < end; i++ {
		name := displayed[i]
		grp := m.groups[name]
		isSelected := (i == m.groupCursor)
		cursor := "  "
		if isSelected {
			cursor = "▶ "
		}
		skillsStr := strings.Join(grp.Skills, ", ")
		if skillsStr == "" {
			skillsStr = "-"
		}
		row := cursor +
			padCell(name, nameW) + " " +
			padCell(fmt.Sprintf("%d", len(grp.Skills)), countW) + " " +
			padCell(skillsStr, skillsW)
		if isSelected {
			row = selectedRowStyle.Width(totalWidth).Render(row)
		}
		b.WriteString(row + "\n")
	}

	if end < len(displayed) {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(i18n.T("more.groups.below"), len(displayed)-end)) + "\n")
	}
	return b.String()
}

func (m *Model) renderUpstreamsList() string {
	if len(m.upstreams) == 0 {
		return i18n.T("empty.upstreams")
	}

	displayed := m.displayedUpstreams()
	if len(displayed) == 0 {
		return i18n.T("filter.search_empty")
	}

	typeW, statusW, skillsW, refW, urlW := computeUpstreamsColWidths(m.width)
	totalWidth := 2 + typeW + 1 + statusW + 1 + skillsW + 1 + refW + 1 + urlW

	var b strings.Builder
	header := "  " +
		padCell(i18n.T("table.header.upstream_type"), typeW) + " " +
		padCell(i18n.T("table.header.upstream_status"), statusW) + " " +
		padCell(i18n.T("table.header.upstream_skills"), skillsW) + " " +
		padCell(i18n.T("table.header.upstream_ref"), refW) + " " +
		padCell(i18n.T("table.header.upstream_url"), urlW)
	b.WriteString(tableHeaderStyle.Render(header) + "\n")
	b.WriteString(tableSeparatorStyle.Render(strings.Repeat("─", totalWidth)) + "\n")

	maxVisible := m.listMax(4, len(displayed))

	if m.upstreamCursor < m.upstreamScrollOffset {
		m.upstreamScrollOffset = m.upstreamCursor
	}
	if m.upstreamCursor >= m.upstreamScrollOffset+maxVisible {
		m.upstreamScrollOffset = m.upstreamCursor - maxVisible + 1
	}
	maxOffset := len(displayed) - maxVisible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.upstreamScrollOffset > maxOffset {
		m.upstreamScrollOffset = maxOffset
	}
	if m.upstreamScrollOffset < 0 {
		m.upstreamScrollOffset = 0
	}

	start := m.upstreamScrollOffset
	end := start + maxVisible
	if end > len(displayed) {
		end = len(displayed)
	}

	if start > 0 {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(i18n.T("more.skills.above"), start)) + "\n")
	}

	for i := start; i < end; i++ {
		u := displayed[i]
		isSelected := (i == m.upstreamCursor)
		cursor := "  "
		if isSelected {
			cursor = "▶ "
		}

		typeStr := string(u.Type)

		var statusStr string
		switch u.Status {
		case model.UpstreamUpToDate:
			statusStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#00B894")).Render("[" + i18n.T("upstream.status.up_to_date") + "]")
		case model.UpstreamUpdateAvailable:
			statusStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FDCB6E")).Bold(true).Render("[" + i18n.T("upstream.status.update_available") + "]")
		case model.UpstreamSourceChanged:
			statusStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#E17055")).Bold(true).Render("[" + i18n.T("upstream.status.source_changed") + "]")
		case model.UpstreamUnreachable:
			statusStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#D63031")).Render("[" + i18n.T("upstream.status.unreachable") + "]")
		case model.UpstreamInvalid:
			statusStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#D63031")).Render("[" + i18n.T("upstream.status.invalid") + "]")
		default:
			statusStr = "-"
		}

		var skillsSummary string
		if u.ScanError != "" {
			skillsSummary = i18n.T("upstream.skills_summary.failed", len(u.Skills))
		} else if u.Scanned || u.CacheExists {
			skillsSummary = fmt.Sprintf(i18n.T("upstream.skills_summary.cached"), len(u.AvailableSkills), len(u.Skills))
		} else {
			skillsSummary = fmt.Sprintf(i18n.T("upstream.skills_summary.uncached"), len(u.Skills))
		}

		refStr := u.Ref
		if refStr == "" {
			refStr = "-"
		}

		urlStr := u.URL
		if len(urlStr) > urlW {
			urlStr = urlStr[:urlW-3] + "..."
		}

		row := cursor +
			padCell(typeStr, typeW) + " " +
			padCell(statusStr, statusW) + " " +
			padCell(skillsSummary, skillsW) + " " +
			padCell(refStr, refW) + " " +
			padCell(urlStr, urlW)

		if isSelected {
			row = selectedRowStyle.Width(totalWidth).Render(row)
		}
		b.WriteString(row + "\n")
	}

	if end < len(displayed) {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(i18n.T("more.skills.below"), len(displayed)-end)) + "\n")
	}

	if len(displayed) > 0 {
		idx := m.upstreamCursor
		if idx < 0 || idx >= len(displayed) {
			idx = 0
		}
		sel := displayed[idx]
		summary := fmt.Sprintf(i18n.T("upstream.scan.summary"), i18n.RenderScanScopeWithExclude(sel.ScanRoots, sel.ScanExclude))
		if m.width > 4 {
			summary = truncate.StringWithTail(summary, uint(m.width-4), "…")
		}
		b.WriteString("\n" + lipgloss.NewStyle().Faint(true).Render("  "+summary))
	}

	return b.String()
}

func (m *Model) renderCacheList() string {
	if len(m.caches) == 0 {
		return i18n.T("empty.cache")
	}

	displayed := m.displayedCaches()
	if len(displayed) == 0 {
		return i18n.T("filter.search_empty")
	}

	repoW, sizeW, urlW := computeCacheColWidths(m.width)
	totalWidth := 2 + repoW + 1 + sizeW + 1 + urlW

	var b strings.Builder
	header := "  " +
		padCell(i18n.T("table.header.cache_repo"), repoW) + " " +
		padCell(i18n.T("table.header.size"), sizeW) + " " +
		padCell(i18n.T("table.header.original_url"), urlW)
	b.WriteString(tableHeaderStyle.Render(header) + "\n")
	b.WriteString(tableSeparatorStyle.Render(strings.Repeat("─", totalWidth)) + "\n")

	maxVisible := m.listMax(4, len(displayed))

	if m.cacheCursor < m.cacheScrollOffset {
		m.cacheScrollOffset = m.cacheCursor
	}
	if m.cacheCursor >= m.cacheScrollOffset+maxVisible {
		m.cacheScrollOffset = m.cacheCursor - maxVisible + 1
	}
	maxOffset := len(displayed) - maxVisible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.cacheScrollOffset > maxOffset {
		m.cacheScrollOffset = maxOffset
	}
	if m.cacheScrollOffset < 0 {
		m.cacheScrollOffset = 0
	}

	start := m.cacheScrollOffset
	end := start + maxVisible
	if end > len(displayed) {
		end = len(displayed)
	}

	if start > 0 {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(i18n.T("more.caches.above"), start)) + "\n")
	}

	for i := start; i < end; i++ {
		c := displayed[i]
		isSelected := (i == m.cacheCursor)
		cursor := "  "
		if isSelected {
			cursor = "▶ "
		}
		sizeMB := fmt.Sprintf("%.2f MB", float64(c.SizeBytes)/(1024*1024))
		row := cursor +
			padCell(c.Key, repoW) + " " +
			padCell(sizeMB, sizeW) + " " +
			padCell(c.URL, urlW)
		if isSelected {
			row = selectedRowStyle.Width(totalWidth).Render(row)
		}
		b.WriteString(row + "\n")
	}

	if end < len(displayed) {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(i18n.T("more.caches.below"), len(displayed)-end)) + "\n")
	}
	return b.String()
}

func (m *Model) renderDoctorList() string {
	if m.doctorReport == nil || len(m.doctorReport.Items) == 0 {
		return i18n.T("empty.doctor")
	}
	items := m.doctorReport.Items

	stW, itemW, msgW := computeDoctorColWidths(m.width)
	var b strings.Builder

	header := "  " +
		padCell(i18n.T("table.header.st"), stW) + " " +
		padCell(i18n.T("table.header.check_item"), itemW) + " " +
		padCell(i18n.T("table.header.diagnosis_msg"), msgW)
	b.WriteString(tableHeaderStyle.Render(header) + "\n")
	b.WriteString(tableSeparatorStyle.Render(strings.Repeat("─", 2+stW+1+itemW+1+msgW)) + "\n")

	maxVisible := m.listMax(4, len(items))

	if m.doctorCursor >= len(items) {
		m.doctorCursor = len(items) - 1
	}
	if m.doctorCursor < 0 {
		m.doctorCursor = 0
	}
	if m.doctorCursor < m.doctorScrollOffset {
		m.doctorScrollOffset = m.doctorCursor
	}
	if m.doctorCursor >= m.doctorScrollOffset+maxVisible {
		m.doctorScrollOffset = m.doctorCursor - maxVisible + 1
	}
	maxOffset := len(items) - maxVisible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.doctorScrollOffset > maxOffset {
		m.doctorScrollOffset = maxOffset
	}
	if m.doctorScrollOffset < 0 {
		m.doctorScrollOffset = 0
	}

	start := m.doctorScrollOffset
	end := start + maxVisible
	if end > len(items) {
		end = len(items)
	}

	if start > 0 {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(i18n.T("more.doctor.above"), start)) + "\n")
	}

	for i := start; i < end; i++ {
		item := items[i]
		isSelected := i == m.doctorCursor
		cursor := "  "
		if isSelected {
			cursor = "▶ "
		}
		icon := statusCleanStyle.Render("✓ ")
		if item.Status == app.CheckWarn {
			icon = statusModifiedStyle.Render("! ")
		} else if item.Status == app.CheckErr {
			icon = statusErrorStyle.Render("✗ ")
		}
		row := cursor +
			padCell(icon, stW) + " " +
			padCell(item.DisplayName(i18n.T), itemW) + " " +
			padCell(item.DisplayMessage(i18n.T), msgW)
		if isSelected {
			row = selectedRowStyle.Render(row)
		}
		b.WriteString(row + "\n")
	}

	if end < len(items) {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(i18n.T("more.doctor.below"), len(items)-end)) + "\n")
	}
	return b.String()
}

func (m *Model) renderSettingsList() string {
	if len(m.settings) == 0 {
		return i18n.T("empty.settings")
	}

	displayed := m.displayedSettings()
	if len(displayed) == 0 {
		return i18n.T("filter.search_empty")
	}

	itemW := computeSettingsColWidths(m.width)
	totalWidth := 2 + itemW

	cardWidth := totalWidth
	if cardWidth < 50 {
		cardWidth = 50
	}
	var cardStr string
	if m.settingsCursor >= 0 && m.settingsCursor < len(displayed) {
		curr := displayed[m.settingsCursor]
		cardContent := fmt.Sprintf("⚙ %s\n  %s: %s\n  %s: %s\n  %s: %s",
			detailNameStyle.Render(curr.Title),
			i18n.T("settings.card.value"),
			detailValueStyle.Render(curr.Value),
			i18n.T("settings.card.desc"),
			detailDescStyle.Render(curr.Description),
			i18n.T("settings.card.action"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#55EFC4")).Bold(true).Render(curr.ActionHint),
		)
		cardStr = boxStyle.Width(cardWidth).Render(cardContent)
	}
	cardLines := len(splitLines(cardStr))

	var b strings.Builder
	header := "  " + padCell(i18n.T("table.header.setting_item"), itemW)
	b.WriteString(tableHeaderStyle.Render(header) + "\n")
	b.WriteString(tableSeparatorStyle.Render(strings.Repeat("─", totalWidth)) + "\n")

	maxVisible := m.listMax(2+cardLines, len(displayed))

	if m.settingsCursor < m.settingsScrollOffset {
		m.settingsScrollOffset = m.settingsCursor
	}
	if m.settingsCursor >= m.settingsScrollOffset+maxVisible {
		m.settingsScrollOffset = m.settingsCursor - maxVisible + 1
	}
	maxOffset := len(displayed) - maxVisible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.settingsScrollOffset > maxOffset {
		m.settingsScrollOffset = maxOffset
	}
	if m.settingsScrollOffset < 0 {
		m.settingsScrollOffset = 0
	}

	start := m.settingsScrollOffset
	end := start + maxVisible
	if end > len(displayed) {
		end = len(displayed)
	}

	if start > 0 {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(i18n.T("more.settings.above"), start)) + "\n")
	}

	for i := start; i < end; i++ {
		s := displayed[i]
		isSelected := (i == m.settingsCursor)
		cursor := "  "
		if isSelected {
			cursor = "▶ "
		}

		row := cursor + padCell(s.Title, itemW)
		if isSelected {
			row = selectedRowStyle.Width(totalWidth).Render(row)
		}
		b.WriteString(row + "\n")
	}

	if end < len(displayed) {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(i18n.T("more.settings.below"), len(displayed)-end)) + "\n")
	}

	// Selected setting card at bottom
	if cardStr != "" {
		b.WriteString("\n" + cardStr)
	}

	return b.String()
}

// hintSeparator is the visual glyph placed between footer/hint items at render
// time. Dictionary strings store items joined by " • "; rendering swaps in this
// lighter separator so the footer reads less cluttered.
const hintSeparator = " · "

// hintText swaps the dictionary's " • " item separator for the lighter display
// glyph, for hints that are rendered directly instead of via a wrapping helper.
func hintText(s string) string {
	return strings.ReplaceAll(s, " • ", hintSeparator)
}

func (m *Model) renderWrappedFooter(footer string) string {
	availWidth := m.width - footerStyle.GetHorizontalFrameSize()
	if availWidth <= 0 {
		availWidth = 80
	}

	parts := strings.Split(footer, " • ")
	if len(parts) <= 1 {
		if availWidth > 0 && lipgloss.Width(footer) > availWidth {
			footer = truncate.StringWithTail(footer, uint(availWidth), "…")
		}
		return footerStyle.Render(footer)
	}

	targetWidth := availWidth

	var lines []string
	var currentLine []string
	currentLen := 0
	sepWidth := lipgloss.Width(hintSeparator)

	for _, part := range parts {
		partWidth := lipgloss.Width(part)
		if len(currentLine) == 0 {
			currentLine = append(currentLine, part)
			currentLen = partWidth
			continue
		}
		if currentLen+sepWidth+partWidth <= targetWidth {
			currentLine = append(currentLine, part)
			currentLen += sepWidth + partWidth
		} else {
			lines = append(lines, strings.Join(currentLine, hintSeparator))
			currentLine = []string{part}
			currentLen = partWidth
		}
	}
	if len(currentLine) > 0 {
		lines = append(lines, strings.Join(currentLine, hintSeparator))
	}

	var rendered []string
	for _, l := range lines {
		if availWidth > 0 && lipgloss.Width(l) > availWidth {
			l = truncate.StringWithTail(l, uint(availWidth), "…")
		}
		rendered = append(rendered, footerStyle.Render(l))
	}
	return strings.Join(rendered, "\n")
}

// helpTabKey maps the active top-level tab to its dictionary name key.
func helpTabKey(t activeTab) string {
	switch t {
	case tabDashboard:
		return "tab.dashboard"
	case tabUpstreams:
		return "tab.upstreams"
	case tabSkills:
		return "tab.skills"
	case tabChannels:
		return "tab.channels"
	case tabProjects:
		return "tab.projects"
	case tabSystem:
		return "tab.system"
	case tabSettings:
		return "tab.settings"
	default:
		return "tab.dashboard"
	}
}

// helpPageKey maps the active top-level tab to its page-specific help key.
func helpPageKey(t activeTab) string {
	switch t {
	case tabDashboard:
		return "help.page.overview"
	case tabUpstreams:
		return "help.page.upstreams"
	case tabSkills:
		return "help.page.skills"
	case tabChannels:
		return "help.page.channels"
	case tabProjects:
		return "help.page.projects"
	case tabSystem:
		return "help.page.system"
	case tabSettings:
		return "help.page.settings"
	default:
		return "help.page.overview"
	}
}

// styledHelpBody builds the scrollable help content for the active tab: the
// shared navigation block followed by the page-specific shortcut list, with
// section headers emphasized.
func (m *Model) styledHelpBody() string {
	content := i18n.T("help.nav") + "\n\n" + i18n.T(helpPageKey(m.activeTab))
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00CEC9"))
	var b strings.Builder
	for _, line := range splitLines(content) {
		trimmed := strings.TrimRight(line, " \t")
		if trimmed != "" && (strings.HasSuffix(trimmed, ":") || strings.HasSuffix(trimmed, "：")) {
			b.WriteString(headerStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// renderHelpModal renders the per-page, scrollable help screen.
func (m *Model) renderHelpModal() string {
	box := boxStyle
	termWidth := m.width
	termHeight := m.height
	if termHeight <= 0 {
		termHeight = 24
	}

	maxOuterWidth := 84
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
	if contentWidth < 24 {
		contentWidth = 24
	}
	box = box.Width(contentWidth + box.GetHorizontalPadding())

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

	title := fmt.Sprintf(i18n.T("help.title"), i18n.T(helpTabKey(m.activeTab)))
	if lipgloss.Width(title) > contentWidth {
		title = truncate.StringWithTail(title, uint(contentWidth), "…")
	}
	headerStr := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A29BFE")).Render(title) + "\n\n"
	headerLines := lipgloss.Height(headerStr)

	bodyContent := m.styledHelpBody()
	bodyHeight := lipgloss.Height(bodyContent)

	tentativeFooter := renderDetailFooter(i18n.T("help.footer"), contentWidth)
	availVpH := maxInnerHeight - headerLines - (lipgloss.Height(tentativeFooter) + 1)
	if availVpH < 3 {
		availVpH = 3
	}
	isScrollable := bodyHeight > availVpH

	footerText := i18n.T("help.footer")
	if isScrollable {
		var scrollBadge string
		if m.helpViewport.AtTop() {
			scrollBadge = "[TOP]"
		} else if m.helpViewport.AtBottom() {
			scrollBadge = "[END]"
		} else {
			scrollBadge = fmt.Sprintf("[%2.0f%%]", m.helpViewport.ScrollPercent()*100)
		}
		footerText = scrollBadge + " " + footerText
	}
	footerRendered := renderDetailFooter(footerText, contentWidth)

	finalMaxVpH := maxInnerHeight - headerLines - (lipgloss.Height(footerRendered) + 1)
	if finalMaxVpH < 3 {
		finalMaxVpH = 3
	}
	vpH := bodyHeight
	if vpH > finalMaxVpH {
		vpH = finalMaxVpH
	}

	if m.helpViewport.Width != contentWidth || m.helpViewport.Height != vpH {
		offset := m.helpViewport.YOffset
		m.helpViewport.Width = contentWidth
		m.helpViewport.Height = vpH
		m.helpViewport.SetContent(bodyContent)
		m.helpViewport.YOffset = offset
	} else {
		m.helpViewport.SetContent(bodyContent)
	}

	var b strings.Builder
	b.WriteString(headerStr)
	b.WriteString(m.helpViewport.View())
	b.WriteString("\n\n" + footerRendered)
	return box.Render(b.String())
}

func (m *Model) renderFooter() string {
	if m.loading {
		return m.renderWrappedFooter(i18n.T("footer.loading"))
	}
	var footer string
	switch m.activeTab {
	case tabDashboard:
		footer = i18n.T("footer.dashboard")
	case tabUpstreams:
		footer = i18n.T("footer.upstreams")
	case tabSkills:
		if m.skillSubTab == skillTabGroups {
			footer = i18n.T("footer.groups")
		} else if len(m.selectedSkills) > 0 {
			footer = fmt.Sprintf(i18n.T("footer.skills_selected"), len(m.selectedSkills))
		} else {
			footer = i18n.T("footer.skills")
		}
	case tabChannels:
		footer = i18n.T("footer.channels")
		if e, ok := m.currentChannelEntry(); ok && e.Supported {
			footer += " • " + i18n.T("footer.channels.models")
		}
		footer += " • " + i18n.T("footer.help_entry") + " • " + i18n.T("footer.quit_entry")
	case tabProjects:
		footer = i18n.T("footer.projects")
	case tabSystem:
		if m.systemSubTab == systemTabDoctor {
			footer = i18n.T("footer.doctor")
		} else {
			footer = i18n.T("footer.cache")
		}
	case tabSettings:
		footer = i18n.T("footer.settings")
	default:
		footer = i18n.T("footer.dashboard")
	}
	return m.renderWrappedFooter(footer)
}

func (m *Model) updatableSkills() []model.SkillStatus {
	var items []model.SkillStatus
	for _, s := range m.skills {
		if s.Upstream == model.UpstreamUpdateAvailable || s.Upstream == model.UpstreamSourceChanged {
			items = append(items, s)
		}
	}
	return items
}

func (m *Model) clampUpstreamCursor() {
	items := m.displayedUpstreams()
	if len(items) == 0 {
		m.upstreamCursor = 0
		m.upstreamScrollOffset = 0
		return
	}
	if m.upstreamCursor >= len(items) {
		m.upstreamCursor = len(items) - 1
	}
	if m.upstreamCursor < 0 {
		m.upstreamCursor = 0
	}
}

func (m *Model) displayedUpstreams() []model.UpstreamInfo {
	query := strings.TrimSpace(m.searchInput.Value())
	if query == "" {
		return m.upstreams
	}
	lowerQuery := strings.ToLower(query)
	var re *regexp.Regexp
	if strings.ContainsAny(query, ".*+?^$[]{}|()\\") {
		re, _ = regexp.Compile("(?i)" + query)
	}

	var res []model.UpstreamInfo
	for _, u := range m.upstreams {
		matched := false
		if re != nil && (re.MatchString(u.URL) || re.MatchString(string(u.Type)) || re.MatchString(string(u.Status))) {
			matched = true
		}
		if !matched {
			if strings.Contains(strings.ToLower(u.URL), lowerQuery) ||
				strings.Contains(strings.ToLower(string(u.Type)), lowerQuery) ||
				strings.Contains(strings.ToLower(string(u.Status)), lowerQuery) ||
				strings.Contains(strings.ToLower(u.Ref), lowerQuery) ||
				strings.Contains(strings.ToLower(u.Commit), lowerQuery) ||
				strings.Contains(strings.ToLower(u.NewCommit), lowerQuery) ||
				strings.Contains(strings.ToLower(u.NewHash), lowerQuery) {
				matched = true
			}
			if !matched {
				for _, sk := range u.Skills {
					if strings.Contains(strings.ToLower(sk), lowerQuery) {
						matched = true
						break
					}
				}
			}
		}
		if matched {
			res = append(res, u)
		}
	}
	return res
}

func (m *Model) visibleSkills() []model.SkillStatus {
	return m.filtered
}

func (m *Model) selectedSkill() (model.SkillStatus, bool) {
	items := m.visibleSkills()
	if m.cursor < 0 || m.cursor >= len(items) {
		return model.SkillStatus{}, false
	}
	return items[m.cursor], true
}

func (m *Model) skillsForTargetUndeploy(deployedIDs []string) []model.SkillStatus {
	var result []model.SkillStatus
	skillMap := make(map[string]model.SkillStatus)
	for _, s := range m.skills {
		skillMap[s.ID] = s
	}
	for _, id := range deployedIDs {
		if s, ok := skillMap[id]; ok {
			result = append(result, s)
		} else {
			result = append(result, model.SkillStatus{
				ID:            id,
				ManagedStatus: model.StatusClean,
				Source: model.SourceSpec{
					Type: model.SourceType("deployed"),
				},
			})
		}
	}
	return result
}

func (m *Model) clampSkillCursor() {
	items := m.visibleSkills()
	if m.cursor >= len(items) {
		m.cursor = max(0, len(items)-1)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m *Model) renderSkillDetail() string {
	var b strings.Builder

	// 1. Fixed Header Info Card
	b.WriteString(m.renderSkillInfoCard() + "\n")

	// 2. Viewport for SKILL.md
	b.WriteString(m.detailViewport.View() + "\n")

	// 3. Footer with dynamic scroll indicator
	var scrollHint string
	if m.detailViewport.AtTop() {
		scrollHint = i18n.T("detail.scroll_top")
	} else if m.detailViewport.AtBottom() {
		scrollHint = i18n.T("detail.scroll_end")
	} else {
		scrollHint = fmt.Sprintf("%2.0f%%", m.detailViewport.ScrollPercent()*100)
	}
	footerText := fmt.Sprintf("[%s] • %s", scrollHint, i18n.T("footer.detail"))
	b.WriteString(m.renderWrappedFooter(footerText))
	return b.String()
}

func (m *Model) renderSkillInfoCard() string {
	if m.detailSkill == nil || m.detailMeta == nil {
		return ""
	}
	cardW := m.width - 4
	if cardW < 40 {
		cardW = 40
	}
	innerW := cardW - 4

	st := m.detailSkill
	meta := m.detailMeta

	// Source description
	srcDesc := string(st.Source.Type)
	if st.Source.URL != "" {
		srcDesc = st.Source.URL
		if st.Source.Path != "" {
			srcDesc += "#" + st.Source.Path
		}
	} else if st.Source.Path != "" {
		srcDesc = st.Source.Path
	}

	// Local path
	locPath := meta.Path
	if locPath == "" {
		locPath = m.detailPath
	}

	// Status badge with optional upstream indicator
	statBadge := formatStatusBadge(st.ManagedStatus, false)
	statusLine := statBadge
	if st.Upstream != "" && st.Upstream != model.UpstreamNone {
		statusLine += " " + formatUpstreamStatus(st.Upstream, false)
	}

	// Arrow separator
	arrow := lipgloss.NewStyle().Foreground(lipgloss.Color("#636E72")).Render(" -> ")

	// Truncate source and path if needed to ensure the line fits in card width
	fixedW := lipgloss.Width(statusLine) + 2 + lipgloss.Width(arrow)
	availForPaths := innerW - fixedW
	if availForPaths < 16 {
		availForPaths = 16
	}

	srcRunes := []rune(srcDesc)
	pathRunes := []rune(locPath)
	if len(srcRunes)+len(pathRunes) > availForPaths {
		half := availForPaths / 2
		var targetSrcW, targetPathW int
		if len(srcRunes) < half {
			targetSrcW = len(srcRunes)
			targetPathW = availForPaths - len(srcRunes)
		} else if len(pathRunes) < half {
			targetPathW = len(pathRunes)
			targetSrcW = availForPaths - len(pathRunes)
		} else {
			targetSrcW = half
			targetPathW = availForPaths - half
		}

		if len(srcRunes) > targetSrcW && targetSrcW > 4 {
			srcDesc = string(srcRunes[:targetSrcW-3]) + "..."
		}
		if len(pathRunes) > targetPathW && targetPathW > 4 {
			startIdx := len(pathRunes) - (targetPathW - 3)
			if startIdx < 0 {
				startIdx = 0
			}
			locPath = "..." + string(pathRunes[startIdx:])
		}
	}

	// Name: prominent styled title without field label
	name := meta.Name
	if name == "" {
		name = st.ID
	}
	nameRunes := []rune(name)
	if len(nameRunes) > innerW && innerW > 10 {
		name = string(nameRunes[:innerW-3]) + "..."
	}
	valName := detailNameStyle.Render(name)

	// Description: secondary styled text without field label
	desc := meta.Description
	if desc == "" {
		desc = "-"
	}
	descRunes := []rune(desc)
	if len(descRunes) > innerW && innerW > 10 {
		desc = string(descRunes[:innerW-3]) + "..."
	}
	valDesc := detailDescStyle.Render(desc)

	valSrc := detailValueStyle.Render(srcDesc)
	valPath := detailPathStyle.Render(locPath)

	pipelineLine := fmt.Sprintf("%s  %s%s%s", statusLine, valSrc, arrow, valPath)

	content := fmt.Sprintf("%s\n%s\n%s",
		valName,
		valDesc,
		pipelineLine,
	)
	if st.Error != "" {
		content += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#FF7675")).Render("⚠ "+st.Error)
	}

	return detailCardStyle.Width(cardW).Render(content)
}

func (m *Model) renderDiffDetail() string {
	var b strings.Builder
	totalW := m.width - 4
	if totalW < 40 {
		totalW = 40
	}

	badge := diffview.FormatBadge(m.diffSummary, true)
	prefix := detailDividerStyle.Render("── Unified Diff ── ")
	leftW := lipgloss.Width(prefix)
	badgeW := lipgloss.Width(badge)

	var header string
	if leftW+badgeW+2 < totalW {
		right := detailDividerStyle.Render(" " + strings.Repeat("─", totalW-leftW-badgeW-1))
		header = prefix + badge + right
	} else {
		header = prefix + badge
	}

	b.WriteString(header + "\n")
	footerKey := "footer.diff"
	if m.isModelsDiffConfirm {
		footerKey = "footer.models_confirm"
	}
	if m.view == viewTargetDiff {
		footerKey = "footer.target_diff"
	}
	footer := m.renderWrappedFooter(i18n.T(footerKey))
	feedback := ""
	if m.err != nil {
		feedback = statusErrorStyle.Render(truncateToWidth(m.err.Error(), totalW))
	} else if m.notice != "" {
		feedback = statusNoticeStyle.Render(truncateToWidth(m.notice, totalW))
	}
	if m.height > 0 {
		reserved := lipgloss.Height(m.renderTabsBar()) + 1 + 2 + lipgloss.Height(footer)
		if feedback != "" {
			reserved++
		}
		if m.diffSearching {
			reserved++
		}
		m.diffViewport.Height = max(1, m.height-reserved)
	}
	b.WriteString(m.diffViewport.View() + "\n\n")
	if m.diffSearching {
		b.WriteString(m.diffSearch.View() + "\n")
	}
	if feedback != "" {
		b.WriteString(feedback + "\n")
	}
	b.WriteString(footer)
	return b.String()
}

func (m *Model) handleDetailKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.view = viewList
		m.err = nil
		return m, nil

	case "up", "k":
		m.detailViewport.LineUp(1)
	case "down", "j":
		m.detailViewport.LineDown(1)
	case "pgup", "b", "ctrl+u":
		m.detailViewport.HalfViewUp()
	case "pgdown", "f", " ", "ctrl+d":
		m.detailViewport.HalfViewDown()
	case "g", "home":
		m.detailViewport.GotoTop()
	case "G", "end":
		m.detailViewport.GotoBottom()

	case "u": // Update current skill
		if m.detailSkill != nil {
			sk := *m.detailSkill
			if sk.ManagedStatus == model.StatusModified {
				m.confirmModal = ConfirmationModalState{
					Title:      i18n.T("confirm.local_mods.title"),
					Message:    fmt.Sprintf(i18n.T("confirm.local_mods.msg"), sk.ID),
					Danger:     true,
					AllowForce: true,
					Action:     "update-skill",
					SkillID:    sk.ID,
				}
				m.view = viewModalConfirm
				return m, nil
			}
			m.loading = true
			m.notice = i18n.T("notice.updating_skill", sk.ID)
			return m, func() tea.Msg {
				_, err := m.service.Update(m.ctx, sk.ID, false, false)
				if err != nil {
					return errMsg{err: err}
				}
				return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.updated_skill"), sk.ID))
			}
		}

	case "d": // Deploy current skill
		if m.detailSkill != nil {
			recentProjects, _ := m.service.ProjectList()
			m.deployModal = newDeployModal(false, m.skills, m.detailSkill.ID, m.targetNames, m.targets, m.service.Profiles(), recentProjects, m.service.IsProjectEnabled)
			m.view = viewDeploy
			m.err = nil
		}

	default:
		var cmd tea.Cmd
		m.detailViewport, cmd = m.detailViewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) handleDiffKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if handled, cmd := m.handleDiffNavigation(msg); handled {
		return m, cmd
	}
	if m.isModelsDiffConfirm {
		switch msg.String() {
		case "e":
			if edit := m.pendingConfigEdit; edit != nil && !edit.remove {
				if m.editJSONModal.CfgFile != edit.cfgFile || m.editJSONModal.OriginalHash != edit.originalHash || strings.Join(m.editJSONModal.Path, "\x00") != strings.Join(edit.fieldPath, "\x00") {
					m.editJSONModal = newJSONEditModal(i18n.T("modal.edit_json.title", strings.Join(edit.fieldPath, ".")), edit.channel, edit.cfgFile, edit.fieldPath, edit.value)
					if edit.candidate != nil {
						m.editJSONModal.setDraft(string(edit.candidate))
					}
				}
				m.editJSONModal.OriginalHash = edit.originalHash
				m.isModelsDiffConfirm = false
				m.view = viewModalEditJSON
				return m, nil
			}
			if summary := m.modelsDiffSummary; summary != nil && len(summary.Candidate) > 0 {
				m.editJSONModal = newJSONEditModal(i18n.T("modal.edit_json.title", filepath.Base(summary.ConfigFile)), m.currentAgentChannel(), summary.ConfigFile, nil, nil)
				m.editJSONModal.setDraft(string(summary.Candidate))
				m.editJSONModal.OriginalHash = summary.OriginalHash
				m.isModelsDiffConfirm = false
				m.view = viewModalEditJSON
				return m, nil
			}
		case "esc", "q", "n":
			m.view = viewList
			m.isModelsDiffConfirm = false
			m.modelsDiffRulesOnly = false
			m.modelsDiffSummary = nil
			m.pendingConfigEdit = nil
			m.err = nil
			m.notice = i18n.T("notice.config_write_cancelled")
			return m, nil

		case "enter", "y":
			if m.pendingConfigEdit != nil {
				return m.applyPendingConfigEdit()
			}
			ctx, cancel := context.WithCancel(m.ctx)
			m.cancelOp = cancel
			m.loading = true
			rulesOnly := m.modelsDiffRulesOnly
			if rulesOnly {
				m.notice = i18n.T("notice.model_config_writing_custom_rules")
			} else {
				m.notice = i18n.T("notice.model_config_writing_rules")
			}
			channel := m.currentAgentChannel()
			cfgFile := m.modelConfigFile
			override := m.modelOverride
			return m, func() tea.Msg {
				summary, err := m.service.EnrichModelConfig(ctx, channel, agent.EnrichOptions{
					FilePath:  cfgFile,
					Override:  override,
					RulesOnly: rulesOnly,
					DryRun:    false,
				})
				if err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return errMsg{err: err}
				}
				if ctx.Err() != nil {
					return nil
				}
				msg := i18n.T("notice.model_config_enriched")
				if rulesOnly {
					msg = i18n.T("notice.model_config_regenerated")
				}
				if summary != nil {
					var providers []string
					pSeen := make(map[string]bool)
					for _, r := range summary.Results {
						if r.MatchedProvider != "" && !pSeen[r.MatchedProvider] {
							pSeen[r.MatchedProvider] = true
							providers = append(providers, r.MatchedProvider)
						}
					}
					if len(providers) > 0 && !rulesOnly {
						msg = i18n.T("notice.model_config_enriched_source", strings.Join(providers, ", "))
					}
					if summary.BackupFile != "" {
						relBackup, err := filepath.Rel(filepath.Dir(summary.ConfigFile), summary.BackupFile)
						if err != nil {
							relBackup = filepath.Base(summary.BackupFile)
						}
						msg += i18n.T("notice.backup_saved", relBackup)
					}
				}
				return asyncNoticeMsg(msg)
			}
		}
	}

	switch msg.String() {
	case "esc", "q":
		m.view = viewList
		m.err = nil
		return m, nil
	case "u":
		if m.isModelsDiffConfirm {
			return m, nil
		}
		if sk, ok := m.selectedSkill(); ok {
			skID := sk.ID
			ctx, cancel := context.WithCancel(m.ctx)
			m.cancelOp = cancel
			m.loading = true
			m.view = viewList
			m.notice = i18n.T("notice.updating_skill", skID)
			return m, func() tea.Msg {
				_, err := m.service.Update(ctx, skID, false, false)
				if err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return errMsg{err: err}
				}
				if ctx.Err() != nil {
					return nil
				}
				return asyncNoticeMsg(fmt.Sprintf(i18n.T("notice.updated_skill"), skID))
			}
		}

	case "t", "T":
		m.toggleChangesOnly()
		return m, nil

	case "up", "k":
		m.diffViewport.LineUp(1)
	case "down", "j":
		m.diffViewport.LineDown(1)
	case "pgup", "b", "ctrl+u":
		m.diffViewport.HalfViewUp()
	case "pgdown", "f", " ", "ctrl+d":
		m.diffViewport.HalfViewDown()
	case "g", "home":
		m.diffViewport.GotoTop()
	case "G", "end":
		m.diffViewport.GotoBottom()

	default:
		var cmd tea.Cmd
		m.diffViewport, cmd = m.diffViewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) handleHelpKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "?", "q":
		m.view = viewList
		m.err = nil
		return m, nil
	case "up", "k":
		m.helpViewport.LineUp(1)
	case "down", "j":
		m.helpViewport.LineDown(1)
	case "pgup", "b", "ctrl+u":
		m.helpViewport.HalfViewUp()
	case "pgdown", "f", "ctrl+d":
		m.helpViewport.HalfViewDown()
	case "g", "home":
		m.helpViewport.GotoTop()
	case "G", "end":
		m.helpViewport.GotoBottom()
	}
	return m, nil
}

func (m *Model) reloadModels() {
	if m.service == nil {
		return
	}
	entry, ok := m.currentChannelEntry()
	if !ok || !entry.Supported {
		m.models = nil
		m.agentProviders = nil
		m.modelConfigFile = ""
		m.err = nil
		m.modelCursor = 0
		m.modelRowCursor = 0
		m.modelScrollOffset = 0
		return
	}
	channel := entry.Channel
	items, cfgFile, err := m.service.ListAgentModels(m.ctx, channel)
	if err != nil {
		m.err = err
		return
	}
	m.models = items
	m.modelConfigFile = cfgFile
	if m.collapsedProviders == nil {
		m.collapsedProviders = make(map[string]bool)
	}
	if provs, _, err := m.service.ListAgentProviders(m.ctx, channel); err == nil {
		m.agentProviders = provs
	}
	m.syncModelRowCursor()
	m.clampModelCursor()
}

// syncModelRowCursor points the row cursor at the current model row, falling
// back to a clamped row when the model is not visible.
func (m *Model) syncModelRowCursor() {
	for i, r := range m.getModelDisplayRows() {
		if !r.isHeader && r.modelIndex == m.modelCursor {
			m.modelRowCursor = i
			return
		}
	}
	m.clampModelRowCursor()
}

func (m *Model) handleOpenModelDetail() (tea.Model, tea.Cmd) {
	displayed := m.displayedModels()
	if len(displayed) == 0 || m.modelCursor >= len(displayed) {
		return m, nil
	}
	item := displayed[m.modelCursor]
	m.modelDetailModal = newModelDetailModal(item)
	m.view = viewModalModelDetail
	return m, nil
}

func (m *Model) currentModelRow() (modelDisplayRow, bool) {
	rows := m.getModelDisplayRows()
	if len(rows) == 0 || m.modelRowCursor < 0 || m.modelRowCursor >= len(rows) {
		return modelDisplayRow{}, false
	}
	return rows[m.modelRowCursor], true
}

// handleModelsSpace toggles provider collapse when a provider header is
// highlighted. Detail views are opened with "i" (see handleChannelRowDetail).
func (m *Model) handleModelsSpace() (tea.Model, tea.Cmd) {
	row, ok := m.currentModelRow()
	if !ok {
		return m, nil
	}
	if row.isHeader {
		m.toggleProviderCollapse(row.providerID)
	}
	return m, nil
}

func (m *Model) toggleProviderCollapse(providerID string) {
	if m.collapsedProviders == nil {
		m.collapsedProviders = make(map[string]bool)
	}
	m.collapsedProviders[providerID] = !m.collapsedProviders[providerID]
	for i, r := range m.getModelDisplayRows() {
		if r.isHeader && r.providerID == providerID {
			m.modelRowCursor = i
			return
		}
	}
}

func (m *Model) handleOpenProviderDetail(providerID string) (tea.Model, tea.Cmd) {
	var provider app.AgentProviderItem
	found := false
	for _, p := range m.agentProviders {
		if p.ProviderID == providerID {
			provider = p
			found = true
			break
		}
	}
	if !found {
		provider = app.AgentProviderItem{ProviderID: providerID}
		for _, it := range m.displayedModels() {
			if it.ProviderID == providerID {
				provider.ModelCount++
			}
		}
	}
	m.providerDetailModal = newProviderDetailModal(provider)
	m.view = viewModalProviderDetail
	return m, nil
}

func (m *Model) handleOpenModelConfigFile() (tea.Model, tea.Cmd) {
	m.modelConfigFileModal = newModelConfigFileModal(m.modelConfigFile)
	m.view = viewModalModelConfigFile
	return m, nil
}

func (m *Model) handleModalModelDetailKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", " ":
		m.view = viewList
		return m, nil

	case "e", "E":
		item := m.modelDetailModal.Item
		m.editFieldModal = newFieldEditModal(
			fmt.Sprintf(i18n.T("modal.edit_field.title"), item.ModelID),
			m.currentAgentChannel(),
			m.modelConfigFile,
			modelFields(item),
		)
		m.view = viewModalEditField
		return m, nil

	case "up", "k":
		m.modelDetailModal.Viewport.LineUp(1)
		return m, nil

	case "down", "j":
		m.modelDetailModal.Viewport.LineDown(1)
		return m, nil

	case "pgup", "b", "ctrl+u":
		m.modelDetailModal.Viewport.HalfViewUp()
		return m, nil

	case "pgdown", "f", "ctrl+d":
		m.modelDetailModal.Viewport.HalfViewDown()
		return m, nil

	case "g", "home":
		m.modelDetailModal.Viewport.GotoTop()
		return m, nil

	case "G", "end":
		m.modelDetailModal.Viewport.GotoBottom()
		return m, nil
	}
	return m, nil
}

func (m *Model) handleModalModelConfigFileKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.view = viewList
		return m, nil

	case "up", "k":
		m.modelConfigFileModal.Viewport.LineUp(1)
		return m, nil

	case "down", "j":
		m.modelConfigFileModal.Viewport.LineDown(1)
		return m, nil

	case "pgup", "b", "ctrl+u":
		m.modelConfigFileModal.Viewport.HalfViewUp()
		return m, nil

	case "pgdown", "f", "ctrl+d":
		m.modelConfigFileModal.Viewport.HalfViewDown()
		return m, nil

	case "g", "home":
		m.modelConfigFileModal.Viewport.GotoTop()
		return m, nil

	case "G", "end":
		m.modelConfigFileModal.Viewport.GotoBottom()
		return m, nil
	}
	return m, nil
}

func (m *Model) handleModalProviderDetailKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.view = viewList
		return m, nil

	case "e", "E":
		provider := m.providerDetailModal.Provider
		m.editFieldModal = newFieldEditModal(
			fmt.Sprintf(i18n.T("modal.edit_field.title_provider"), provider.ProviderID),
			m.currentAgentChannel(),
			m.modelConfigFile,
			providerFields(provider),
		)
		m.view = viewModalEditField
		return m, nil

	case "up", "k":
		m.providerDetailModal.Viewport.LineUp(1)
		return m, nil

	case "down", "j":
		m.providerDetailModal.Viewport.LineDown(1)
		return m, nil

	case "pgup", "b", "ctrl+u":
		m.providerDetailModal.Viewport.HalfViewUp()
		return m, nil

	case "pgdown", "f", "ctrl+d":
		m.providerDetailModal.Viewport.HalfViewDown()
		return m, nil

	case "g", "home":
		m.providerDetailModal.Viewport.GotoTop()
		return m, nil

	case "G", "end":
		m.providerDetailModal.Viewport.GotoBottom()
		return m, nil
	}
	return m, nil
}

func (m *Model) handleModalEditFieldKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.editFieldModal.Editing {
		switch msg.String() {
		case "esc":
			m.editFieldModal.Editing = false
			m.editFieldModal.Err = ""
			return m, nil
		case "enter":
			field, ok := m.editFieldModal.selected()
			if !ok {
				m.editFieldModal.Editing = false
				return m, nil
			}
			raw := strings.TrimSpace(m.editFieldModal.Input.Value())
			value, err := coerceFieldValue(field.Kind, raw)
			if err != nil {
				m.editFieldModal.Err = err.Error()
				return m, nil
			}
			m.editFieldModal.Editing = false
			m.editFieldModal.Err = ""
			return m.previewConfigEdit(m.editFieldModal.Channel, m.editFieldModal.CfgFile, field.Path, value, false)
		default:
			var cmd tea.Cmd
			m.editFieldModal.Input, cmd = m.editFieldModal.Input.Update(msg)
			return m, cmd
		}
	}

	switch msg.String() {
	case "esc", "q":
		m.view = viewList
		return m, nil
	case "up", "k":
		if m.editFieldModal.Cursor > 0 {
			m.editFieldModal.Cursor--
		}
		return m, nil
	case "down", "j":
		if m.editFieldModal.Cursor < len(m.editFieldModal.Fields)-1 {
			m.editFieldModal.Cursor++
		}
		return m, nil
	case " ", "t":
		field, ok := m.editFieldModal.selected()
		if !ok || field.Kind != "bool" {
			return m, nil
		}
		newVal := field.Value != "true"
		return m.previewConfigEdit(m.editFieldModal.Channel, m.editFieldModal.CfgFile, field.Path, newVal, false)
	case "enter":
		field, ok := m.editFieldModal.selected()
		if !ok {
			return m, nil
		}
		if field.Kind == "json" {
			m.editJSONModal = newJSONEditModal(
				fmt.Sprintf(i18n.T("modal.edit_json.title"), strings.Join(field.Path, ".")),
				m.editFieldModal.Channel,
				m.editFieldModal.CfgFile,
				field.Path,
				m.rawValueForField(field),
			)
			m.view = viewModalEditJSON
			return m, nil
		}
		m.editFieldModal.Editing = true
		m.editFieldModal.Err = ""
		m.editFieldModal.Input.SetValue(field.Value)
		m.editFieldModal.Input.CursorEnd()
		m.editFieldModal.Input.Focus()
		return m, nil
	case "d", "D":
		field, ok := m.editFieldModal.selected()
		if !ok || field.Kind == "json" {
			return m, nil
		}
		return m.previewConfigEdit(m.editFieldModal.Channel, m.editFieldModal.CfgFile, field.Path, nil, true)
	}
	return m, nil
}

// rawValueForField returns the parsed JSON value backing a raw-JSON field.
func (m *Model) rawValueForField(field EditableField) any {
	var v any
	if err := json.Unmarshal([]byte(field.Value), &v); err == nil {
		return v
	}
	return map[string]any{}
}

func (m *Model) handleModalEditJSONKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.modelsDiffSummary != nil || m.pendingConfigEdit != nil {
			m.isModelsDiffConfirm = true
			m.view = viewDiff
		} else {
			m.view = viewModalEditField
		}
		m.editJSONModal.Err = ""
		return m, nil
	case "ctrl+s":
		return m.previewJSONDraft()
	case "ctrl+e":
		return m.openExternalEditor()
	default:
		var cmd tea.Cmd
		if m.editJSONModal.ReadOnly {
			return m, nil
		}
		m.editJSONModal.Input, cmd = m.editJSONModal.Input.Update(msg)
		return m, cmd
	}
}

// previewConfigEdit computes the diff for a pending edit and opens the confirm view.
func (m *Model) previewConfigEdit(channel, cfgFile string, path []string, value any, remove bool) (tea.Model, tea.Cmd) {
	m.editorOperation++
	id := m.editorOperation
	pending := &pendingConfigEdit{
		channel:   channel,
		cfgFile:   cfgFile,
		fieldPath: path,
		value:     value,
		remove:    remove,
	}
	if m.view == viewModalEditJSON {
		pending.originalHash = m.editJSONModal.OriginalHash
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancelOp = cancel
	m.loading = true
	m.notice = i18n.T("notice.generating_config_diff")
	expectedHash := pending.originalHash
	return m, func() tea.Msg {
		edit, err := m.service.PreviewAgentConfigValue(ctx, channel, cfgFile, path, value, remove, expectedHash)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return draftPreviewMsg{id: id, err: err}
		}
		if ctx.Err() != nil {
			return nil
		}
		return draftPreviewMsg{id: id, edit: edit, pending: pending}
	}
}

// applyPendingConfigEdit writes the confirmed config edit and creates a backup.
func (m *Model) applyPendingConfigEdit() (tea.Model, tea.Cmd) {
	edit := m.pendingConfigEdit
	if edit == nil {
		return m, nil
	}
	m.editorOperation++
	id := m.editorOperation
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancelOp = cancel
	m.loading = true
	m.notice = i18n.T("notice.writing_config_backup")
	channel := edit.channel
	cfgFile := edit.cfgFile
	fieldPath := append([]string(nil), edit.fieldPath...)
	value := edit.value
	remove := edit.remove
	expectedHash := edit.originalHash
	candidate := append([]byte(nil), edit.candidate...)
	return m, func() tea.Msg {
		var result *app.AgentConfigEdit
		var err error
		if candidate != nil {
			result, err = m.service.EditAgentConfigDraft(ctx, channel, cfgFile, candidate, expectedHash, false)
		} else {
			result, err = m.service.ApplyAgentConfigValue(ctx, channel, cfgFile, fieldPath, value, remove, expectedHash)
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return draftWriteMsg{id: id, err: err}
		}
		if ctx.Err() != nil {
			return nil
		}
		msg := i18n.T("notice.config_updated")
		if result != nil && result.BackupFile != "" {
			msg += i18n.T("notice.backup_saved_comma", filepath.Base(result.BackupFile))
		}
		return draftWriteMsg{id: id, notice: msg}
	}
}

// coerceFieldValue converts a raw text input into the typed JSON value.
func coerceFieldValue(kind, raw string) (any, error) {
	switch kind {
	case "number":
		if raw == "" {
			return nil, fmt.Errorf("value cannot be empty")
		}
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number: %s", raw)
		}
		return f, nil
	case "bool":
		switch strings.ToLower(raw) {
		case "true", "1", "yes", "y":
			return true, nil
		case "false", "0", "no", "n":
			return false, nil
		}
		return nil, fmt.Errorf("invalid boolean: %s", raw)
	default:
		return raw, nil
	}
}

func (m *Model) handleModelsDiff() (tea.Model, tea.Cmd) {
	if len(m.models) == 0 {
		return m, nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancelOp = cancel
	m.loading = true
	m.notice = i18n.T("notice.generating_model_diff")
	m.modelsDiffRulesOnly = false
	channel := m.currentAgentChannel()
	cfgFile := m.modelConfigFile
	override := m.modelOverride
	return m, func() tea.Msg {
		summary, err := m.service.EnrichModelConfig(ctx, channel, agent.EnrichOptions{
			FilePath: cfgFile,
			Override: override,
			DryRun:   true,
		})
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return errMsg{err: err}
		}
		if ctx.Err() != nil {
			return nil
		}
		return modelsEnrichDryRunMsg{summary: summary}
	}
}

// handleModelsRulesOnly regenerates the config from custom rules only, without
// consulting models.dev, and previews the diff before confirmation.
func (m *Model) handleModelsRulesOnly() (tea.Model, tea.Cmd) {
	if len(m.models) == 0 {
		return m, nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancelOp = cancel
	m.loading = true
	m.notice = i18n.T("notice.generating_model_diff_custom_rules")
	m.modelsDiffRulesOnly = true
	channel := m.currentAgentChannel()
	cfgFile := m.modelConfigFile
	return m, func() tea.Msg {
		summary, err := m.service.EnrichModelConfig(ctx, channel, agent.EnrichOptions{
			FilePath:  cfgFile,
			RulesOnly: true,
			DryRun:    true,
		})
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return errMsg{err: err}
		}
		if ctx.Err() != nil {
			return nil
		}
		return modelsEnrichDryRunMsg{summary: summary}
	}
}

type modelDisplayRow struct {
	isHeader   bool
	collapsed  bool
	providerID string
	modelCount int
	modelIndex int
	item       app.AgentModelItem
}

func (m *Model) getModelDisplayRows() []modelDisplayRow {
	models := m.displayedModels()
	if len(models) == 0 {
		return nil
	}

	var rows []modelDisplayRow
	currentProvider := ""

	for i, item := range models {
		if item.ProviderID != currentProvider {
			currentProvider = item.ProviderID
			count := 0
			for _, m2 := range models[i:] {
				if m2.ProviderID == currentProvider {
					count++
				} else {
					break
				}
			}
			collapsed := m.collapsedProviders[currentProvider]
			rows = append(rows, modelDisplayRow{
				isHeader:   true,
				collapsed:  collapsed,
				providerID: currentProvider,
				modelCount: count,
				modelIndex: -1,
			})
		}
		if m.collapsedProviders[currentProvider] {
			continue
		}
		rows = append(rows, modelDisplayRow{
			isHeader:   false,
			modelIndex: i,
			item:       item,
		})
	}
	return rows
}

// currentProviderLabel describes the provider of the currently selected model.
// It is rendered as a fixed line so the provider context stays visible even
// when the inline provider group headers scroll out of view.
func (m *Model) currentProviderLabel() string {
	models := m.displayedModels()
	if len(models) == 0 {
		return ""
	}
	idx := m.modelCursor
	if idx >= len(models) {
		idx = len(models) - 1
	}
	if idx < 0 {
		return ""
	}
	current := models[idx].ProviderID

	var order []string
	seen := make(map[string]bool)
	for _, it := range models {
		if !seen[it.ProviderID] {
			seen[it.ProviderID] = true
			order = append(order, it.ProviderID)
		}
	}
	providerPos := 1
	for i, p := range order {
		if p == current {
			providerPos = i + 1
			break
		}
	}

	posInProvider, totalInProvider := 0, 0
	for _, it := range models {
		if it.ProviderID != current {
			continue
		}
		totalInProvider++
		if it.ModelID == models[idx].ModelID {
			posInProvider = totalInProvider
		}
	}

	return fmt.Sprintf(i18n.T("model.current_provider"), current, posInProvider, totalInProvider, providerPos, len(order))
}

// renderModelsBody renders the provider/model table for the current channel.
// fixed is the number of lines already consumed above this block on the
// Channels page; the compact channel summary now carries the config path and
// sync mode, so the table starts directly with the current-provider line.
func (m *Model) renderModelsBody(fixed int) string {
	var b strings.Builder

	if len(m.models) == 0 {
		b.WriteString(i18n.T("empty.models"))
		return b.String()
	}

	displayRows := m.getModelDisplayRows()
	if len(displayRows) == 0 {
		b.WriteString(i18n.T("filter.search_empty"))
		return b.String()
	}

	// Lines inside this block before the first data row: current-provider
	// label, table header and separator.
	maxVisible := m.listMax(fixed+3, len(displayRows))

	cursorRowIdx := m.modelRowCursor
	if cursorRowIdx < 0 || cursorRowIdx >= len(displayRows) {
		cursorRowIdx = 0
	}

	if cursorRowIdx < m.modelScrollOffset {
		m.modelScrollOffset = cursorRowIdx
	}
	if cursorRowIdx >= m.modelScrollOffset+maxVisible {
		m.modelScrollOffset = cursorRowIdx - maxVisible + 1
	}
	maxOffset := len(displayRows) - maxVisible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.modelScrollOffset > maxOffset {
		m.modelScrollOffset = maxOffset
	}
	if m.modelScrollOffset < 0 {
		m.modelScrollOffset = 0
	}

	end := m.modelScrollOffset + maxVisible
	if end > len(displayRows) {
		end = len(displayRows)
	}

	modelW, ctxW, priceW, capsW, varW, statW := computeModelColWidths(m.width)
	sepWidth := 4 + modelW + 1 + ctxW + 1 + priceW + 1 + capsW + 1 + varW + 1 + statW

	if label := m.currentProviderLabel(); label != "" {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(lipgloss.Color("#00CEC9")).Bold(true).Render(label) + "\n")
	}

	headerRow := "    " +
		padCell(i18n.T("table.header.model_id"), modelW) + " " +
		padCell(i18n.T("table.header.model_context"), ctxW) + " " +
		padCell(i18n.T("table.header.model_pricing"), priceW) + " " +
		padCell(i18n.T("table.header.model_caps"), capsW) + " " +
		padCell(i18n.T("table.header.model_variants"), varW) + " " +
		padCell(i18n.T("table.header.model_status"), statW)

	b.WriteString(tableHeaderStyle.Render(headerRow) + "\n")
	b.WriteString("  " + tableSeparatorStyle.Render(strings.Repeat("─", sepWidth)) + "\n")

	if m.modelScrollOffset > 0 {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(i18n.T("more.models.above"), m.modelScrollOffset)) + "\n")
	}

	for i := m.modelScrollOffset; i < end; i++ {
		row := displayRows[i]
		if row.isHeader {
			marker := "▼"
			if row.collapsed {
				marker = "▶"
			}
			headerText := fmt.Sprintf("%s %s", marker, fmt.Sprintf(i18n.T("model.provider_group"), row.providerID, row.modelCount))
			style := lipgloss.NewStyle().Foreground(lipgloss.Color("#00CEC9")).Bold(true)
			if i == m.modelRowCursor {
				style = style.Background(lipgloss.Color("#0984E3")).Foreground(lipgloss.Color("#FFFFFF"))
			}
			b.WriteString(style.Render("  "+headerText) + "\n")
			continue
		}

		item := row.item
		isSelected := (i == m.modelRowCursor)

		ctxStr := "-"
		if item.ContextLimit > 0 {
			cStr := fmt.Sprintf("%dk", item.ContextLimit/1000)
			if item.ContextLimit < 1000 {
				cStr = fmt.Sprintf("%d", item.ContextLimit)
			}
			if item.OutputLimit > 0 {
				oStr := fmt.Sprintf("%dk", item.OutputLimit/1000)
				if item.OutputLimit < 1000 {
					oStr = fmt.Sprintf("%d", item.OutputLimit)
				}
				if ctxW >= lipgloss.Width(cStr+" / "+oStr) {
					ctxStr = cStr + " / " + oStr
				} else {
					ctxStr = cStr + "/" + oStr
				}
			} else {
				ctxStr = cStr
			}
		}

		priceStr := "-"
		if item.PriceInput > 0 || item.PriceOutput > 0 {
			fullPrice := fmt.Sprintf("$%.2f / $%.2f", item.PriceInput, item.PriceOutput)
			if lipgloss.Width(fullPrice) <= priceW {
				priceStr = fullPrice
			} else {
				priceStr = fmt.Sprintf("$%g/$%g", item.PriceInput, item.PriceOutput)
			}
		}

		var caps []string
		if item.Reasoning {
			caps = append(caps, i18n.T("model.cap.reasoning"))
		}
		if item.ToolCall {
			caps = append(caps, i18n.T("model.cap.tools"))
		}
		if item.Attachment {
			caps = append(caps, i18n.T("model.cap.vision"))
		}
		capsStr := "-"
		if len(caps) > 0 {
			fullCaps := strings.Join(caps, ",")
			if lipgloss.Width(fullCaps) <= capsW {
				capsStr = fullCaps
			} else {
				var shortCaps []string
				for _, c := range caps {
					shortCaps = append(shortCaps, string(c[0]))
				}
				capsStr = strings.Join(shortCaps, ",")
			}
		}

		varStr := "-"
		if len(item.Variants) > 0 {
			varStr = i18n.T("model.variants_count", len(item.Variants))
		}

		var statusBadge string
		if item.IsEnriched {
			txt := i18n.T("model.enrich.done")
			if item.SourceProvider != "" {
				txt = i18n.T("model.enrich.done_source", item.SourceProvider)
			} else {
				txt = i18n.T("model.enrich.done_local")
			}
			if isSelected {
				statusBadge = lipgloss.NewStyle().Foreground(lipgloss.Color("#55EFC4")).Bold(true).Render(txt)
			} else {
				statusBadge = statusCleanStyle.Render(txt)
			}
		} else {
			txt := i18n.T("model.enrich.pending")
			if item.SourceProvider != "" {
				txt = i18n.T("model.enrich.pending_source", item.SourceProvider)
			} else {
				txt = i18n.T("model.enrich.pending_unmatched")
			}
			if isSelected {
				statusBadge = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFEAA7")).Bold(true).Render(txt)
			} else {
				statusBadge = statusModifiedStyle.Render(txt)
			}
		}

		prefix := "    "
		if isSelected {
			prefix = "  > "
		}

		line := prefix +
			padCell(item.ModelID, modelW) + " " +
			padCell(ctxStr, ctxW) + " " +
			padCell(priceStr, priceW) + " " +
			padCell(capsStr, capsW) + " " +
			padCell(varStr, varW) + " " +
			padCell(statusBadge, statW)

		if isSelected {
			line = selectedRowStyle.Width(sepWidth + 2).Render(line)
		}
		b.WriteString(line + "\n")
	}

	if end < len(displayRows) {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(i18n.T("more.models.below"), len(displayRows)-end)) + "\n")
	}

	return b.String()
}
