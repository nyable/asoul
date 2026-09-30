package tui

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"asoul/internal/app"
	"asoul/internal/editor"
	"asoul/internal/i18n"
	"asoul/internal/jsonc"

	tea "github.com/charmbracelet/bubbletea"
)

type editorPreparedMsg struct {
	id           uint64
	draft        *editor.Draft
	originalHash string
	cfgFile      string
	err          error
}

type editorFinishedMsg struct {
	id                  uint64
	content             string
	readErr, processErr error
}

func (m *Model) openExternalEditor() (tea.Model, tea.Cmd) {
	m.editorOperation++
	id := m.editorOperation
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancelOp = cancel
	m.loading = true
	m.notice = i18n.T("notice.editor_preparing")
	content, cfgFile, originalHash := m.editJSONModal.draftContent(), m.editJSONModal.CfgFile, m.editJSONModal.OriginalHash
	svc := m.service
	return m, func() tea.Msg {
		cfg, err := svc.Config()
		if err != nil {
			return editorPreparedMsg{id: id, err: err}
		}
		cfgFile, err = filepath.EvalSymlinks(cfgFile)
		if err != nil {
			return editorPreparedMsg{id: id, err: err}
		}
		if originalHash == "" {
			raw, err := os.ReadFile(cfgFile)
			if err != nil {
				return editorPreparedMsg{id: id, err: err}
			}
			originalHash = fmt.Sprintf("%x", sha256.Sum256(raw))
		}
		draft, err := editor.Prepare(ctx, cfg.Editor, content)
		if ctx.Err() != nil {
			if draft != nil {
				draft.Cleanup()
			}
			return nil
		}
		return editorPreparedMsg{id: id, draft: draft, cfgFile: cfgFile, originalHash: originalHash, err: err}
	}
}

func (m *Model) handleEditorPrepared(msg editorPreparedMsg) (tea.Model, tea.Cmd) {
	if msg.id != m.editorOperation || !m.loading {
		if msg.draft != nil {
			return m, func() tea.Msg { msg.draft.Cleanup(); return nil }
		}
		return m, nil
	}
	m.finishLoading()
	m.view = viewModalEditJSON
	m.editJSONModal.Err = ""
	if msg.err != nil {
		if errors.Is(msg.err, editor.ErrUnavailable) {
			m.editJSONModal.Err = i18n.T("error.editor_unavailable")
		} else {
			m.editJSONModal.Err = i18n.T("error.editor_failed", msg.err.Error())
		}
		return m, nil
	}
	m.editJSONModal.OriginalHash = msg.originalHash
	m.editJSONModal.CfgFile = msg.cfgFile
	m.notice = i18n.T("notice.editor_running")
	return m, tea.ExecProcess(msg.draft.Command, func(processErr error) tea.Msg {
		defer msg.draft.Cleanup()
		content, readErr := msg.draft.Read()
		return editorFinishedMsg{id: msg.id, content: content, readErr: readErr, processErr: processErr}
	})
}

func (m *Model) handleEditorFinished(msg editorFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.id != m.editorOperation {
		return m, nil
	}
	m.view = viewModalEditJSON
	before := m.editJSONModal.draftContent()
	if msg.readErr == nil {
		m.editJSONModal.setDraft(msg.content)
	}
	if err := errors.Join(msg.processErr, msg.readErr); err != nil {
		m.editJSONModal.Err = i18n.T("error.editor_failed", err.Error())
		return m, nil
	}
	if before == msg.content {
		m.notice = i18n.T("notice.model_config_unchanged")
		return m, nil
	}
	return m.previewJSONDraft()
}

func (m *Model) previewJSONDraft() (tea.Model, tea.Cmd) {
	var value any
	if err := jsonc.Unmarshal([]byte(m.editJSONModal.draftContent()), &value); err != nil {
		m.editJSONModal.Err = i18n.T("error.editor_invalid_json", err.Error())
		return m, nil
	}
	m.editJSONModal.Err = ""
	if len(m.editJSONModal.Path) == 0 {
		m.editorOperation++
		id := m.editorOperation
		draft := m.editJSONModal
		candidate := []byte(draft.draftContent())
		pending := &pendingConfigEdit{channel: draft.Channel, cfgFile: draft.CfgFile, originalHash: draft.OriginalHash, candidate: candidate}
		ctx, cancel := context.WithCancel(m.ctx)
		m.cancelOp = cancel
		m.loading = true
		m.notice = i18n.T("notice.generating_config_diff")
		return m, func() tea.Msg {
			edit, err := m.service.EditAgentConfigDraft(ctx, draft.Channel, draft.CfgFile, candidate, draft.OriginalHash, true)
			if ctx.Err() != nil {
				return nil
			}
			return draftPreviewMsg{id: id, edit: edit, err: err, pending: pending}
		}
	}
	return m.previewConfigEdit(m.editJSONModal.Channel, m.editJSONModal.CfgFile, m.editJSONModal.Path, value, false)
}

type configDraftErrorMsg struct{ err error }

type draftWriteMsg struct {
	id     uint64
	notice string
	err    error
}

type draftPreviewMsg struct {
	id      uint64
	edit    *app.AgentConfigEdit
	err     error
	pending *pendingConfigEdit
}
