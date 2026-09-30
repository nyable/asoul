package tui

import (
	"context"

	"asoul/internal/i18n"
	"asoul/internal/model"
	"asoul/internal/progress"
	"asoul/internal/source/git"

	tea "github.com/charmbracelet/bubbletea"
)

// Each result belongs to one cancellable operation, including results already
// queued before Esc. A cancelled operation may have persisted registration.
type upstreamAddedMsg struct {
	id         uint64
	url, ref   string
	name       string
	sourceType model.SourceType
	err        error
}

type upstreamProgressMsg struct {
	id     uint64
	update progress.Update
}

type upstreamScanMsg struct {
	id       uint64
	url, ref string
	skills   []git.DiscoveredSkill
	err      error
}

func (m *Model) startUpstreamScan(url, ref string) tea.Cmd {
	m.upstreamOperation++
	id := m.upstreamOperation
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancelOp = cancel
	m.loading = true
	m.upstreamScanActive = true
	m.err = nil
	m.notice = i18n.T("notice.discovering_upstream", url)
	svc, program := m.service, m.program
	cb := func(u progress.Update) {
		if program != nil && ctx.Err() == nil {
			program.Send(upstreamProgressMsg{id: id, update: u})
		}
	}
	return func() tea.Msg {
		skills, err := svc.UpstreamDiscover(ctx, url, ref, cb)
		return upstreamScanMsg{id: id, url: url, ref: ref, skills: skills, err: err}
	}
}

func (m *Model) updateScannedUpstream(msg upstreamScanMsg) {
	for i := range m.upstreams {
		u := &m.upstreams[i]
		if u.URL != msg.url || u.Ref != msg.ref {
			continue
		}
		u.Scanned = true
		u.CacheExists = u.Type == model.SourceTypeGit || u.Type == model.SourceTypeLocal
		u.ScanError = ""
		if msg.err != nil {
			u.ScanError = msg.err.Error()
		}
		u.AvailableSkills = []string{}
		for _, sk := range msg.skills {
			u.AvailableSkills = append(u.AvailableSkills, sk.ID)
		}
	}
}

func (m *Model) addRegisteredUpstream(msg upstreamAddedMsg) {
	for _, u := range m.upstreams {
		if u.URL == msg.url && u.Ref == msg.ref {
			return
		}
	}
	m.upstreams = append(m.upstreams, model.UpstreamInfo{URL: msg.url, Ref: msg.ref, Name: msg.name, Type: msg.sourceType, Skills: []string{}})
}
