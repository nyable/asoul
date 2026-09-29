package tui

import (
	"asoul/internal/app"
	"asoul/internal/deploy"
	"asoul/internal/model"
	"asoul/internal/progress"
	"asoul/internal/source/git"

	tea "github.com/charmbracelet/bubbletea"
)

// NewDiscoveredSkillsMsgForTest creates a discoveredSkillsMsg for testing.
func NewDiscoveredSkillsMsgForTest(url, ref string, skillIDs []string) tea.Msg {
	var skills []git.DiscoveredSkill
	for _, id := range skillIDs {
		skills = append(skills, git.DiscoveredSkill{
			ID:          id,
			Path:        "skills/" + id,
			Description: "Test skill " + id,
		})
	}
	return discoveredSkillsMsg{
		url:    url,
		ref:    ref,
		skills: skills,
	}
}

// NewReloadStatusMsgForTest creates a status refresh message for interaction tests.
func NewReloadStatusMsgForTest(statuses []model.SkillStatus) tea.Msg {
	return reloadStatusMsg(statuses)
}

// NewAsyncNoticeMsgForTest creates an asyncNoticeMsg for testing.
func NewAsyncNoticeMsgForTest(notice string) tea.Msg {
	return asyncNoticeMsg(notice)
}

// NewDeployConflictMsgForTest creates a deployConflictMsg for testing.
func NewDeployConflictMsgForTest(conflict *deploy.TargetConflictError) tea.Msg {
	return deployConflictMsg{Conflict: conflict}
}

// NewDiffLoadedMsgForTest creates a diffLoadedMsg for testing.
func NewDiffLoadedMsgForTest(diff string) tea.Msg {
	return diffLoadedMsg(diff)
}

// NewProgressMsgForTest creates a progressMsg for testing.
func NewProgressMsgForTest(phase progress.Phase, current, total int, detail string) tea.Msg {
	return progressMsg{phase: phase, current: current, total: total, detail: detail}
}

// BeginLoadingForTest seeds the loading state without starting a real operation.
func (m *Model) BeginLoadingForTest(notice string) {
	m.loading = true
	m.notice = notice
	m.gitProgress = ""
}

// CurrentViewForTest returns the current view state as an int.
func (m *Model) CurrentViewForTest() int {
	return int(m.view)
}

const (
	TabDashboardForTest = int(tabDashboard)
	TabUpstreamsForTest = int(tabUpstreams)
	TabSkillsForTest    = int(tabSkills)
	TabChannelsForTest  = int(tabChannels)
	TabProjectsForTest  = int(tabProjects)
	TabSystemForTest    = int(tabSystem)
	TabSettingsForTest  = int(tabSettings)

	ViewListForTest                       = int(viewList)
	ViewModalDeployConflictForTest        = int(viewModalDeployConflict)
	ViewTargetDiffForTest                 = int(viewTargetDiff)
	ViewModalAddForTest                   = int(viewModalAdd)
	ViewDeployForTest                     = int(viewDeploy)
	ViewModalConfirmForTest               = int(viewModalConfirm)
	ViewDiffForTest                       = int(viewDiff)
	ViewModalAddUpstreamForTest           = int(viewModalAddUpstream)
	ViewModalEditUpstreamForTest          = int(viewModalEditUpstream)
	ViewModalConfirmUpstreamRemoveForTest = int(viewModalConfirmUpstreamRemove)
	ViewModalUpstreamDetailForTest        = int(viewModalUpstreamDetail)
	ViewModalTargetDetailForTest          = int(viewModalTargetDetail)
	ViewModalModelDetailForTest           = int(viewModalModelDetail)
	ViewModalModelConfigFileForTest       = int(viewModalModelConfigFile)
)

func (m *Model) ActiveTabForTest() int {
	return int(m.activeTab)
}

func (m *Model) SelectTabForTest(tab int) {
	m.PumpForTest(m.setActiveTab(activeTab(tab)))
}

// SetFocusDepthForTest seeds the focus level for tests that need to start on
// the secondary level.
func (m *Model) SetFocusDepthForTest(depth int) {
	m.focusDepth = depth
}

// PumpForTest drains a command synchronously, mirroring the Bubble Tea event
// loop, so tests can observe state after async reloads complete.
func (m *Model) PumpForTest(cmd tea.Cmd) {
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			break
		}
		_, next := m.Update(msg)
		cmd = next
	}
}

const (
	DiscoveredTagNewForTest       = int(DiscoveredTagNew)
	DiscoveredTagInstalledForTest = int(DiscoveredTagInstalled)
	DiscoveredTagConflictForTest  = int(DiscoveredTagConflict)
)

func (m *Model) AddModalStepForTest() int {
	return m.addModal.Step
}

// AddModalFocusForTest reports the Add modal's current focus group:
// whether the upstream list mode is active and which input (0/1) is focused.
func (m *Model) AddModalFocusForTest() (fromUpstream bool, activeInput int) {
	return m.addModal.FromUpstreamMode, m.addModal.ActiveInput
}

func (m *Model) AddModalIsSelectedForTest(idx int) bool {
	return m.addModal.SelectedIndices[idx]
}

func (m *Model) AddModalTagForTest(idx int) (int, string) {
	st := m.addModal.DiscoveredStatus[idx]
	return int(st.Tag), st.ConflictSource
}

func (m *Model) AddModalCustomAliasForTest(path string) string {
	return m.addModal.CustomAliases[path]
}

func (m *Model) AddModalReplaceSourceForTest() bool {
	return m.addModal.ReplaceSource
}

func (m *Model) AddModalRenamingForTest() bool {
	return m.addModal.Renaming
}

func (m *Model) AddModalErrorForTest() string {
	if m.err != nil {
		return m.err.Error()
	}
	return ""
}

func (m *Model) AddModalSourceURLForTest() string {
	return m.addModal.SourceURL
}

func (m *Model) AddModalSourceRefForTest() string {
	return m.addModal.SourceRef
}

const (
	AddFilterAllForTest       = int(AddFilterAll)
	AddFilterConflictForTest  = int(AddFilterConflict)
	AddFilterNewForTest       = int(AddFilterNew)
	AddFilterInstalledForTest = int(AddFilterInstalled)
)

func (m *Model) AddModalFilterForTest() int {
	return int(m.addModal.CurrentFilter)
}

func (m *Model) AddModalVisibleCountForTest() int {
	return len(m.addModal.VisibleIndices())
}

func (m *Model) AddModalVisibleIndicesForTest() []int {
	return m.addModal.VisibleIndices()
}

func (m *Model) AddModalIsOverwriteForTest(path string) bool {
	return m.addModal.OverwriteSkills[path]
}

func (m *Model) SelectedSkillsCountForTest() int {
	return len(m.selectedSkills)
}

func (m *Model) IsSkillSelectedForTest(id string) bool {
	return m.selectedSkills[id]
}

func (m *Model) SetSearchQueryForTest(q string) {
	m.searchInput.SetValue(q)
	m.applyFilter()
}

func (m *Model) GroupSkillsModalVisibleCountForTest() int {
	return len(m.groupSkillsModal.VisibleSkills())
}

func (m *Model) GroupSkillsModalIsSkillSelectedForTest(id string) bool {
	return m.groupSkillsModal.SelectedSkills[id]
}

func (m *Model) DeployModalVisibleSkillsCountForTest() int {
	return len(m.deployModal.VisibleSkills())
}

func (m *Model) DeployModalVisibleGroupsCountForTest() int {
	return len(m.deployModal.VisibleGroups())
}

func (m *Model) DeployModalVisibleAgentTargetsCountForTest() int {
	return len(m.deployModal.VisibleAgentTargets())
}

func (m *Model) DeployModalVisibleProjectTargetsCountForTest() int {
	return len(m.deployModal.VisibleProjectTargets())
}

func (m *Model) DeployModalIsSkillSelectedForTest(id string) bool {
	return m.deployModal.SelectedSkills[id]
}

func (m *Model) UpstreamsCountForTest() int {
	return len(m.upstreams)
}

func (m *Model) DisplayedUpstreamsCountForTest() int {
	return len(m.displayedUpstreams())
}

func (m *Model) UpstreamCursorForTest() int {
	return m.upstreamCursor
}

func NewUpstreamReloadMsgForTest(upstreams []model.UpstreamInfo) tea.Msg {
	return upstreamReloadMsg(upstreams)
}

func (m *Model) UpstreamDetailModalForTest() (model.UpstreamInfo, bool) {
	return m.upstreamDetailModal.Upstream, m.view == viewModalUpstreamDetail
}

func (m *Model) NoticeForTest() string {
	return m.notice
}

func RenderUpstreamDetailModalForTest(u model.UpstreamInfo, termWidth, termHeight int) string {
	state := newUpstreamDetailModal(u)
	return RenderUpstreamDetailModal(&state, boxStyle, termWidth, termHeight)
}

func (m *Model) UpstreamDetailModalViewportOffsetForTest() int {
	return m.upstreamDetailModal.Viewport.YOffset
}

func (m *Model) TargetDetailModalForTest() (TargetDetailModalState, bool) {
	return m.targetDetailModal, m.view == viewModalTargetDetail
}

func RenderTargetDetailModalForTest(state TargetDetailModalState, termWidth, termHeight int) string {
	return RenderTargetDetailModal(&state, boxStyle, termWidth, termHeight)
}

func (m *Model) TargetDetailModalViewportOffsetForTest() int {
	return m.targetDetailModal.Viewport.YOffset
}

func NewTargetDetailModalForTest(name, tgtType, channel, configDir, resolvedDir, skillsDir string, paths map[string]string, pathExists, skillsExists bool, formats string, deployedSkills []string, enabled bool) TargetDetailModalState {
	return newTargetDetailModal(name, tgtType, channel, configDir, resolvedDir, skillsDir, paths, pathExists, skillsExists, formats, deployedSkills, enabled)
}

func (m *Model) ModelDetailModalForTest() (app.AgentModelItem, bool) {
	return m.modelDetailModal.Item, m.view == viewModalModelDetail
}

func (m *Model) ModelConfigFileModalForTest() (string, bool) {
	return m.modelConfigFileModal.FilePath, m.view == viewModalModelConfigFile
}

func (m *Model) ChannelCursorForTest() int {
	return m.channelCursor
}

func (m *Model) FocusDepthForTest() int {
	return m.focusDepth
}

func (m *Model) SkillSubTabForTest() int {
	return int(m.skillSubTab)
}

func (m *Model) SystemSubTabForTest() int {
	return int(m.systemSubTab)
}

func (m *Model) ChannelNamesForTest() []string {
	entries := m.displayedChannels()
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name)
	}
	return names
}

func (m *Model) ChannelEntriesForTest() []channelEntry {
	return m.channelEntries
}

func (m *Model) SetDoctorReportForTest(items []app.CheckItem) {
	m.doctorReport = &app.DoctorReport{Items: items}
}

func (m *Model) DoctorCursorForTest() int {
	return m.doctorCursor
}

func (m *Model) DoctorScrollOffsetForTest() int {
	return m.doctorScrollOffset
}

func (m *Model) CurrentProviderLabelForTest() string {
	return m.currentProviderLabel()
}

func (m *Model) ModelsDiffRulesOnlyForTest() bool {
	return m.modelsDiffRulesOnly
}

func (m *Model) DiffChangesOnlyForTest() bool {
	return m.diffChangesOnly
}

func (m *Model) EditFieldModalForTest() (bool, int, int) {
	return m.editFieldModal.Editing, m.editFieldModal.Cursor, len(m.editFieldModal.Fields)
}

func (m *Model) SetEditFieldInputForTest(v string) {
	m.editFieldModal.Input.SetValue(v)
}

func (m *Model) ProviderDetailOpenForTest() bool {
	return m.view == viewModalProviderDetail
}

func (m *Model) ModelsDiffConfirmForTest() bool {
	return m.isModelsDiffConfirm
}

func (m *Model) ModelOverrideForTest() bool {
	return m.modelOverride
}

func (m *Model) ProviderDetailNameForTest() string {
	return m.providerDetailModal.Provider.Name
}

func RenderModelDetailModalForTest(item app.AgentModelItem, termWidth, termHeight int) string {
	state := newModelDetailModal(item)
	return RenderModelDetailModal(&state, boxStyle, termWidth, termHeight)
}

func RenderModelConfigFileModalForTest(filePath string, termWidth, termHeight int) string {
	state := newModelConfigFileModal(filePath)
	return RenderModelConfigFileModal(&state, boxStyle, termWidth, termHeight)
}
