package tui

import (
	"fmt"
	"strings"

	"asoul/internal/i18n"
	"asoul/internal/model"

	"github.com/charmbracelet/lipgloss"
)

var (
	dashTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#A29BFE"))

	dashNumStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#55EFC4"))

	dashSubtextStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#B2BEC3"))
)

func (m *Model) renderDashboard() string {
	var b strings.Builder

	// Top workspace & config information
	wsPath := ""
	if m.service != nil {
		wsPath = m.service.WorkspaceRoot()
	}
	if len(wsPath) > 50 {
		wsPath = "..." + wsPath[len(wsPath)-47:]
	}
	cfgPath := ""
	if m.service != nil {
		cfgPath = m.service.ConfigPath()
	}
	if len(cfgPath) > 40 {
		cfgPath = "..." + cfgPath[len(cfgPath)-37:]
	}

	infoLine := lipgloss.NewStyle().Foreground(lipgloss.Color("#A29BFE")).Bold(true).Render(i18n.T("dashboard.workspace_label")) +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#DFE6E9")).Render(wsPath) +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#636E72")).Render("   |   ") +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#A29BFE")).Bold(true).Render(i18n.T("dashboard.config_label")) +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#DFE6E9")).Render(cfgPath)
	b.WriteString("  " + infoLine + "\n\n")

	// Compute metrics
	totalSkills := len(m.skills)
	gitSkills, localSkills, managedSkills := 0, 0, 0
	for _, rec := range m.skills {
		switch rec.Source.Type {
		case model.SourceTypeGit:
			gitSkills++
		case model.SourceTypeLocal:
			localSkills++
		default:
			managedSkills++
		}
	}

	totalUpstreams := len(m.upstreams)
	upToDateUpstreams, updateAvailUpstreams := 0, 0
	for _, u := range m.upstreams {
		if u.Status == model.UpstreamUpdateAvailable {
			updateAvailUpstreams++
		} else {
			upToDateUpstreams++
		}
	}

	agentCount := len(m.agentTargets)
	projectCount := len(m.projectTargets)
	deployedSkills := 0
	if m.service != nil {
		allDeps := m.service.AllSkillDeployments()
		for _, deps := range allDeps {
			deployedSkills += len(deps)
		}
	}

	updatable := m.updatableSkills()
	pendingCount := len(updatable)

	warnCount := 0
	for _, st := range m.skills {
		if st.ManagedStatus == model.StatusInvalid || st.ManagedStatus == model.StatusMissing {
			warnCount++
		}
	}

	// Render Cards
	cardW := 21
	if m.width > 120 {
		cardW = (m.width - 16) / 5
		if cardW > 28 {
			cardW = 28
		}
	}

	createCard := func(title, bigVal, subtext string, isAlert bool) string {
		borderColor := lipgloss.Color("#6C5CE7")
		if isAlert {
			borderColor = lipgloss.Color("#FDCB6E")
		}
		cardStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Padding(0, 1).
			Width(cardW)

		cContent := dashTitleStyle.Render(title) + "\n" +
			dashNumStyle.Render(bigVal) + "\n" +
			dashSubtextStyle.Render(subtext)
		return cardStyle.Render(cContent)
	}

	c1 := createCard(i18n.T("dashboard.card.skills_title"), i18n.T("dashboard.card.skills_value", totalSkills), i18n.T("dashboard.card.skills_breakdown", gitSkills, localSkills, managedSkills), false)
	c2 := createCard(i18n.T("dashboard.card.upstreams_title"), i18n.T("dashboard.card.upstreams_value", totalUpstreams), i18n.T("dashboard.card.upstreams_breakdown", upToDateUpstreams, updateAvailUpstreams), updateAvailUpstreams > 0)
	c3 := createCard(i18n.T("dashboard.card.deploy_title"), i18n.T("dashboard.card.deploy_value", deployedSkills), i18n.T("dashboard.card.deploy_breakdown", agentCount, projectCount), false)

	updateHint := i18n.T("dashboard.updates.latest")
	if pendingCount > 0 {
		updateHint = i18n.T("dashboard.updates.update_all")
	}
	c4 := createCard(i18n.T("dashboard.card.pending_title"), i18n.T("dashboard.card.pending_value", pendingCount), updateHint, pendingCount > 0)

	healthVal := i18n.T("dashboard.health.ok")
	healthSub := i18n.T("dashboard.health.ok_hint")
	if warnCount > 0 {
		healthVal = i18n.T("dashboard.health.warn", warnCount)
		healthSub = i18n.T("dashboard.health.warn_hint")
	}
	c5 := createCard(i18n.T("dashboard.card.health_title"), healthVal, healthSub, warnCount > 0)

	if m.width >= 115 {
		cardsRow := lipgloss.JoinHorizontal(lipgloss.Top, c1, " ", c2, " ", c3, " ", c4, " ", c5)
		b.WriteString(cardsRow + "\n\n")
	} else {
		row1 := lipgloss.JoinHorizontal(lipgloss.Top, c1, " ", c2, " ", c3)
		row2 := lipgloss.JoinHorizontal(lipgloss.Top, c4, " ", c5)
		b.WriteString(row1 + "\n" + row2 + "\n\n")
	}

	// Panel 1: Pending Updates
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#DFE6E9")).Render(i18n.T("dashboard.panel.updates_title")) + "\n")
	if pendingCount > 0 {
		for i, sk := range updatable {
			if i >= 3 {
				b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("dashboard.panel.updates_more", pendingCount-3)) + "\n")
				break
			}
			newCommit := sk.NewCommit
			if newCommit == "" {
				newCommit = "-"
			} else if len(newCommit) > 7 {
				newCommit = newCommit[:7]
			}
			b.WriteString(i18n.T("dashboard.panel.update_line",
				lipgloss.NewStyle().Foreground(lipgloss.Color("#55EFC4")).Bold(true).Render(sk.ID),
				newCommit,
				lipgloss.NewStyle().Foreground(lipgloss.Color("#FDCB6E")).Render(i18n.T("dashboard.badge.updatable")),
			))
		}
		b.WriteString("\n")
	} else {
		b.WriteString("    " + lipgloss.NewStyle().Foreground(lipgloss.Color("#00B894")).Render(i18n.T("dashboard.panel.updates_none")) + "\n\n")
	}

	// Panel 2: Upstream Sources
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#DFE6E9")).Render(i18n.T("dashboard.panel.sources_title")) + "\n")
	if totalUpstreams > 0 {
		for i, u := range m.upstreams {
			if i >= 3 {
				b.WriteString(lipgloss.NewStyle().Faint(true).Render(i18n.T("dashboard.panel.sources_more", totalUpstreams)) + "\n")
				break
			}
			stBadge := lipgloss.NewStyle().Foreground(lipgloss.Color("#00B894")).Render(i18n.T("dashboard.badge.latest"))
			if u.Status == model.UpstreamUpdateAvailable {
				stBadge = lipgloss.NewStyle().Foreground(lipgloss.Color("#FDCB6E")).Bold(true).Render(i18n.T("dashboard.badge.update_available"))
			}
			displayName := u.URL
			if u.Name != "" {
				displayName = fmt.Sprintf("%s (%s)", u.Name, u.URL)
			}
			if len(displayName) > 55 {
				displayName = displayName[:52] + "..."
			}
			b.WriteString(fmt.Sprintf("    • %-55s  %-10s  %s\n",
				displayName,
				i18n.T("dashboard.sources.skill_count", len(u.Skills)),
				stBadge,
			))
		}
		b.WriteString("\n")
	} else {
		b.WriteString("    " + lipgloss.NewStyle().Faint(true).Render(i18n.T("dashboard.panel.sources_none")) + "\n\n")
	}

	return b.String()
}
