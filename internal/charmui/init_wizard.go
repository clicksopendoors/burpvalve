package charmui

import (
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type InitWizardOptions struct {
	Target   string
	Color    bool
	Skip     InitWizardResult
	Defaults *InitWizardResult
}

type RepairWizardOptions struct {
	Target   string
	Color    bool
	Skip     InitWizardResult
	Defaults *InitWizardResult
}

type InitWizardResult struct {
	Target       string
	Agents       bool
	Claude       bool
	ClaudeRoute  string
	Docs         bool
	Plans        bool
	Log          bool
	Backpressure bool
	Attestations bool
	Hooks        bool
	HooksPath    bool
	Bin          bool
	ToolDocs     bool
	Beads        bool
	NTM          bool
	Orchestrator bool
}

func DefaultInitWizardResult(target string) InitWizardResult {
	if strings.TrimSpace(target) == "" {
		target = "."
	}
	return InitWizardResult{
		Target:       target,
		Agents:       true,
		Claude:       true,
		ClaudeRoute:  "agent-symlink",
		Docs:         true,
		Plans:        true,
		Log:          true,
		Backpressure: true,
		Attestations: true,
		Hooks:        true,
		HooksPath:    true,
		Bin:          false,
		ToolDocs:     true,
		Beads:        true,
		NTM:          true,
	}
}

func RunInitWizard(in io.Reader, out io.Writer, opts InitWizardOptions) (InitWizardResult, error) {
	return runScaffoldWizard(in, out, scaffoldWizardConfig{
		title:        "Burpvalve init",
		description:  "Choose the repo and the pieces Burpvalve should install.",
		targetPrompt: "Install into which repo directory?",
		itemPrompt:   "Which pieces should Burpvalve set up?",
		runHelp:      "enter runs init",
		target:       opts.Target,
		color:        opts.Color,
		skip:         opts.Skip,
		defaults:     opts.Defaults,
		includeNTM:   true,
	})
}

func RunRepairWizard(in io.Reader, out io.Writer, opts RepairWizardOptions) (InitWizardResult, error) {
	return runScaffoldWizard(in, out, scaffoldWizardConfig{
		title:        "Burpvalve repair",
		description:  "Choose the repo and only the pieces Burpvalve should repair.",
		targetPrompt: "Repair which repo directory?",
		itemPrompt:   "Which pieces should Burpvalve repair?",
		runHelp:      "enter runs repair",
		target:       opts.Target,
		color:        opts.Color,
		skip:         opts.Skip,
		defaults:     opts.Defaults,
		includeNTM:   false,
	})
}

type scaffoldWizardConfig struct {
	title        string
	description  string
	targetPrompt string
	itemPrompt   string
	runHelp      string
	target       string
	color        bool
	skip         InitWizardResult
	defaults     *InitWizardResult
	includeNTM   bool
}

func (c scaffoldWizardConfig) routePrompt() string {
	if strings.Contains(strings.ToLower(c.title), "repair") {
		return "How should Claude Code use this repo after repair?"
	}
	return "How should Claude Code use this repo?"
}

type claudeRouteChoice struct {
	value       string
	label       string
	description string
}

func claudeRouteChoices() []claudeRouteChoice {
	return []claudeRouteChoice{
		{
			value:       "agent-symlink",
			label:       "Ordinary agent",
			description: "CLAUDE.md stays a symlink to AGENTS.md for Claude as a regular repo agent",
		},
		{
			value:       "orchestrator-skill",
			label:       "Orchestrator",
			description: "CLAUDE.md becomes a bootstrap file and .claude/skills/burpvalve-orchestrator/ is installed",
		},
		{
			value:       "none",
			label:       "No Claude route",
			description: "Burpvalve leaves CLAUDE.md and the Claude orchestrator skill route disabled",
		},
	}
}

func defaultClaudeRoute(claude bool, route string) string {
	route = strings.TrimSpace(route)
	if !claude {
		return "none"
	}
	switch route {
	case "", "preserve":
		return "agent-symlink"
	case "agent-symlink", "orchestrator-skill", "none":
		return route
	default:
		return "agent-symlink"
	}
}

func routeChoiceIndex(route string) int {
	route = defaultClaudeRoute(true, route)
	for i, choice := range claudeRouteChoices() {
		if choice.value == route {
			return i
		}
	}
	return 0
}

func runScaffoldWizard(in io.Reader, out io.Writer, config scaffoldWizardConfig) (InitWizardResult, error) {
	result := DefaultInitWizardResult(config.target)
	if config.defaults != nil {
		result = *config.defaults
		if strings.TrimSpace(result.Target) == "" {
			result.Target = config.target
		}
		if strings.TrimSpace(result.Target) == "" {
			result.Target = "."
		}
		result.ClaudeRoute = defaultClaudeRoute(result.Claude, result.ClaudeRoute)
	}
	applyWizardSkips(&result, config.skip)
	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = "."
	input.SetValue(result.Target)
	applyTextInputStyles(&input, config.color, true, 0, false)
	input.CursorEnd()
	focusCmd := input.Focus()
	items := scaffoldWizardItems(config.skip, config.includeNTM)
	model := initWizardModel{
		config:            resultConfig(config),
		result:            result,
		input:             input,
		items:             items,
		routeChoices:      claudeRouteChoices(),
		routeCursor:       routeChoiceIndex(result.ClaudeRoute),
		hasDarkBackground: true,
	}
	final, err := runPrompt(in, out, model, focusCmd)
	if err != nil {
		return InitWizardResult{}, err
	}
	done := final.(initWizardModel)
	if done.cancelled {
		return InitWizardResult{}, ErrCancelled
	}
	return done.result, nil
}

func scaffoldWizardItems(skip InitWizardResult, includeNTM bool) []initWizardItem {
	var items []initWizardItem
	addWizardItem := func(skip bool, item initWizardItem) {
		if !skip {
			items = append(items, item)
		}
	}
	addWizardItem(skip.Agents, initWizardItem{label: "AGENTS.md operating contract", description: "repo instructions for agents", get: func(r InitWizardResult) bool { return r.Agents }, set: func(r *InitWizardResult, v bool) { r.Agents = v }})
	addWizardItem(skip.Claude, initWizardItem{
		label:       "Claude Code route",
		description: "ordinary agent symlink, orchestrator skill, or no Claude route",
		get:         func(r InitWizardResult) bool { return r.Claude },
		set: func(r *InitWizardResult, v bool) {
			r.Claude = v
			if v && (strings.TrimSpace(r.ClaudeRoute) == "" || r.ClaudeRoute == "none") {
				r.ClaudeRoute = "agent-symlink"
			} else {
				r.ClaudeRoute = defaultClaudeRoute(v, r.ClaudeRoute)
			}
		},
	})
	addWizardItem(skip.Docs, initWizardItem{label: "docs/", description: "durable project knowledge", get: func(r InitWizardResult) bool { return r.Docs }, set: func(r *InitWizardResult, v bool) { r.Docs = v }})
	addWizardItem(skip.Plans, initWizardItem{label: "plans/", description: "implementation plans and indexes", get: func(r InitWizardResult) bool { return r.Plans }, set: func(r *InitWizardResult, v bool) { r.Plans = v }})
	addWizardItem(skip.Log, initWizardItem{label: "log/", description: "work logs and failed backpressure reports", get: func(r InitWizardResult) bool { return r.Log }, set: func(r *InitWizardResult, v bool) { r.Log = v }})
	addWizardItem(skip.Backpressure, initWizardItem{label: "backpressure/", description: "conditions and manifest", get: func(r InitWizardResult) bool { return r.Backpressure }, set: func(r *InitWizardResult, v bool) { r.Backpressure = v }})
	addWizardItem(skip.Attestations, initWizardItem{label: "backpressure attestations", description: "tracked passing evidence", get: func(r InitWizardResult) bool { return r.Attestations }, set: func(r *InitWizardResult, v bool) { r.Attestations = v }})
	addWizardItem(skip.Hooks, initWizardItem{label: "pre-commit hook", description: "runs commit gate and lint before git commit", get: func(r InitWizardResult) bool { return r.Hooks }, set: func(r *InitWizardResult, v bool) { r.Hooks = v }})
	addWizardItem(skip.HooksPath, initWizardItem{label: "git hooksPath", description: "points Git at .githooks/", get: func(r InitWizardResult) bool { return r.HooksPath }, set: func(r *InitWizardResult, v bool) { r.HooksPath = v }})
	addWizardItem(skip.Bin, initWizardItem{label: "repo-local bin/burpvalve", description: "optional hook fallback", get: func(r InitWizardResult) bool { return r.Bin }, set: func(r *InitWizardResult, v bool) { r.Bin = v }})
	addWizardItem(skip.ToolDocs, initWizardItem{label: "tools/burpvalve docs", description: "local replacement notes", get: func(r InitWizardResult) bool { return r.ToolDocs }, set: func(r *InitWizardResult, v bool) { r.ToolDocs = v }})
	addWizardItem(skip.Beads, initWizardItem{label: ".beads", description: "optional local task graph", get: func(r InitWizardResult) bool { return r.Beads }, set: func(r *InitWizardResult, v bool) { r.Beads = v }})
	if includeNTM && !skip.NTM {
		items = append(items, initWizardItem{label: "NTM bridge", description: "optional coordination snapshot", get: func(r InitWizardResult) bool { return r.NTM }, set: func(r *InitWizardResult, v bool) { r.NTM = v }})
	}
	if includeNTM && !skip.Orchestrator {
		items = append(items, initWizardItem{
			label:       "ORCHESTRATOR.md",
			description: "optional orchestrator operating notes; never installed silently",
			get:         func(r InitWizardResult) bool { return r.Orchestrator },
			set:         func(r *InitWizardResult, v bool) { r.Orchestrator = v },
			visible:     func(r InitWizardResult) bool { return r.Beads || r.NTM },
		})
	}
	return items
}

func applyWizardSkips(result *InitWizardResult, skip InitWizardResult) {
	if skip.Agents {
		result.Agents = false
	}
	if skip.Claude {
		result.Claude = false
		result.ClaudeRoute = "none"
	}
	if skip.Docs {
		result.Docs = false
	}
	if skip.Plans {
		result.Plans = false
	}
	if skip.Log {
		result.Log = false
	}
	if skip.Backpressure {
		result.Backpressure = false
	}
	if skip.Attestations {
		result.Attestations = false
	}
	if skip.Hooks {
		result.Hooks = false
	}
	if skip.HooksPath {
		result.HooksPath = false
	}
	if skip.Bin {
		result.Bin = false
	}
	if skip.ToolDocs {
		result.ToolDocs = false
	}
	if skip.Beads {
		result.Beads = false
	}
	if skip.NTM {
		result.NTM = false
	}
	if skip.Orchestrator {
		result.Orchestrator = false
	}
}

func resultConfig(config scaffoldWizardConfig) scaffoldWizardConfig {
	if strings.TrimSpace(config.title) == "" {
		config.title = "Burpvalve"
	}
	if strings.TrimSpace(config.runHelp) == "" {
		config.runHelp = "enter runs"
	}
	return config
}

type initWizardItem struct {
	label       string
	description string
	get         func(InitWizardResult) bool
	set         func(*InitWizardResult, bool)
	visible     func(InitWizardResult) bool
}

type initWizardModel struct {
	config            scaffoldWizardConfig
	step              int
	cursor            int
	routeCursor       int
	width             int
	hasDarkBackground bool
	input             textinput.Model
	items             []initWizardItem
	routeChoices      []claudeRouteChoice
	result            InitWizardResult
	err               string
	cancelled         bool
}

func (m *initWizardModel) clampCursor() {
	visible := m.visibleItems()
	if len(visible) == 0 {
		m.cursor = 0
		return
	}
	if m.cursor >= len(visible) {
		m.cursor = len(visible) - 1
	}
}

func (m *initWizardModel) normalizeHiddenSelections() {
	if !m.result.Beads && !m.result.NTM {
		m.result.Orchestrator = false
	}
	m.result.ClaudeRoute = defaultClaudeRoute(m.result.Claude, m.result.ClaudeRoute)
	if len(m.routeChoices) > 0 {
		m.routeCursor = routeChoiceIndex(m.result.ClaudeRoute)
	}
}

func (m initWizardModel) shouldAskClaudeRoute() bool {
	return m.result.Claude && len(m.routeChoices) > 0
}

func (m initWizardModel) visibleItems() []initWizardItem {
	visible := make([]initWizardItem, 0, len(m.items))
	for _, item := range m.items {
		if item.isVisible(m.result) {
			visible = append(visible, item)
		}
	}
	return visible
}

func (i initWizardItem) isVisible(result InitWizardResult) bool {
	return i.visible == nil || i.visible(result)
}

func (m initWizardModel) Init() tea.Cmd {
	return requestBackgroundColor(m.config.color)
}

func (m initWizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		applyTextInputStyles(&m.input, m.config.color, m.hasDarkBackground, m.width, false)
		return m, nil
	case tea.BackgroundColorMsg:
		m.hasDarkBackground = msg.IsDark()
		applyTextInputStyles(&m.input, m.config.color, m.hasDarkBackground, m.width, false)
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		}
		if m.step == 0 {
			switch msg.String() {
			case "enter":
				target := strings.TrimSpace(m.input.Value())
				if target == "" {
					m.err = "target directory is required"
					return m, nil
				}
				m.result.Target = target
				m.step = 1
				m.err = ""
				return m, nil
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		if m.step == 2 {
			switch msg.String() {
			case "up", "k":
				if len(m.routeChoices) > 0 && m.routeCursor > 0 {
					m.routeCursor--
				}
			case "down", "j":
				if len(m.routeChoices) > 0 && m.routeCursor < len(m.routeChoices)-1 {
					m.routeCursor++
				}
			case "space", "enter":
				if len(m.routeChoices) > 0 {
					m.result.Claude = true
					m.result.ClaudeRoute = m.routeChoices[m.routeCursor].value
					if m.result.ClaudeRoute == "none" {
						m.result.Claude = true
					}
				}
				return m, tea.Quit
			}
			return m, nil
		}
		visible := m.visibleItems()
		switch msg.String() {
		case "up", "k":
			if len(visible) > 0 && m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if len(visible) > 0 && m.cursor < len(visible)-1 {
				m.cursor++
			}
		case "space":
			if len(visible) == 0 {
				return m, nil
			}
			item := visible[m.cursor]
			item.set(&m.result, !item.get(m.result))
			m.normalizeHiddenSelections()
			m.clampCursor()
		case "a":
			for _, item := range m.items {
				if item.isVisible(m.result) {
					item.set(&m.result, true)
				}
			}
			m.normalizeHiddenSelections()
			m.clampCursor()
		case "n":
			for _, item := range m.items {
				item.set(&m.result, false)
			}
			m.normalizeHiddenSelections()
			m.clampCursor()
		case "enter":
			if m.shouldAskClaudeRoute() {
				m.routeCursor = routeChoiceIndex(m.result.ClaudeRoute)
				m.step = 2
				return m, nil
			}
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m initWizardModel) View() tea.View {
	var b strings.Builder
	style := newWizardStyles(m.config.color, m.width, m.hasDarkBackground)
	writeWizardHeader(&b, style, m.config.title, m.config.description)
	if m.step == 0 {
		b.WriteString(style.section("Target repository"))
		b.WriteString("\n")
		b.WriteString(style.promptBlock(m.config.targetPrompt))
		b.WriteString("\n")
		b.WriteString(m.input.View())
		if m.err != "" {
			b.WriteString("\n")
			b.WriteString(style.err(m.err))
		}
		b.WriteString("\n\n")
		b.WriteString(style.helpLine([]string{"enter"}, "accepts", []string{"esc"}, "cancels"))
		return tea.NewView(b.String())
	}
	b.WriteString(style.section("Target"))
	b.WriteString(" ")
	b.WriteString(style.path(m.result.Target))
	b.WriteString("\n\n")
	if m.step == 2 {
		b.WriteString(style.promptBlock(m.config.routePrompt()))
		b.WriteString("\n\n")
		for i, choice := range m.routeChoices {
			cursor := " "
			label := choice.label
			if i == m.routeCursor {
				cursor = style.cursor(">")
				label = style.selectedLabel(label)
			} else {
				label = style.label(label)
			}
			b.WriteString(fmt.Sprintf("%s %s", cursor, label))
			if choice.description != "" {
				b.WriteString(style.muted(" - " + wrapLine(choice.description, max(12, style.textWidth()-lipgloss.Width(choice.label)-5))))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(style.helpLine([]string{"up/down"}, "moves", []string{"enter"}, strings.TrimPrefix(m.config.runHelp, "enter "), []string{"esc"}, "cancels"))
		return tea.NewView(b.String())
	}
	b.WriteString(style.promptBlock(m.config.itemPrompt))
	b.WriteString("\n\n")
	visibleItems := m.visibleItems()
	if len(visibleItems) == 0 {
		b.WriteString(style.muted("All pieces were skipped by flags."))
		b.WriteString("\n\n")
		b.WriteString(style.helpLine([]string{"enter"}, strings.TrimPrefix(m.config.runHelp, "enter "), []string{"esc"}, "cancels"))
		return tea.NewView(b.String())
	}
	for i, item := range visibleItems {
		selected := i == m.cursor
		cursor := " "
		if i == m.cursor {
			cursor = style.cursor(">")
		}
		checked := style.disabled("[ ]")
		if item.get(m.result) {
			checked = style.enabled("[x]")
		}
		label := item.label
		if selected {
			label = style.selectedLabel(label)
		} else if !item.get(m.result) {
			label = style.disabled(label)
		} else {
			label = style.label(label)
		}
		b.WriteString(fmt.Sprintf("%s %s %s", cursor, checked, label))
		if item.description != "" {
			b.WriteString(style.muted(" - " + wrapLine(item.description, max(12, style.textWidth()-lipgloss.Width(item.label)-8))))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(style.helpLine([]string{"space"}, "toggles", []string{"a"}, "selects all", []string{"n"}, "selects none", []string{"enter"}, strings.TrimPrefix(m.config.runHelp, "enter "), []string{"esc"}, "cancels"))
	return tea.NewView(b.String())
}

type wizardStyles struct {
	color             bool
	width             int
	hasDarkBackground bool
	theme             semanticTheme
}

func newWizardStyles(color bool, width int, hasDarkBackground bool) wizardStyles {
	return wizardStyles{
		color:             color,
		width:             width,
		hasDarkBackground: hasDarkBackground,
		theme:             newSemanticTheme(hasDarkBackground),
	}
}

func writeWizardHeader(b *strings.Builder, style wizardStyles, title, description string) {
	if title != "" {
		b.WriteString(style.titleBlock(title))
		b.WriteString("\n")
	}
	if description != "" {
		b.WriteString(style.mutedBlock(description))
		b.WriteString("\n")
	}
	if title != "" || description != "" {
		b.WriteString("\n")
	}
}

func (s wizardStyles) section(value string) string {
	return s.render(lipgloss.NewStyle().Bold(true).Foreground(s.theme.section), value)
}

func (s wizardStyles) titleBlock(value string) string {
	return s.renderBlock(lipgloss.NewStyle().Bold(true).Foreground(s.theme.titleFG).Background(s.theme.titleBG).Padding(0, 1), value)
}

func (s wizardStyles) promptBlock(value string) string {
	return s.renderBlock(lipgloss.NewStyle().Bold(true).Foreground(s.theme.text), value)
}

func (s wizardStyles) path(value string) string {
	return s.render(lipgloss.NewStyle().Foreground(s.theme.accent), value)
}

func (s wizardStyles) muted(value string) string {
	return s.render(lipgloss.NewStyle().Foreground(s.theme.muted), value)
}

func (s wizardStyles) mutedBlock(value string) string {
	return s.renderBlock(lipgloss.NewStyle().Foreground(s.theme.muted), value)
}

func (s wizardStyles) cursor(value string) string {
	return s.render(lipgloss.NewStyle().Bold(true).Foreground(s.theme.accent), value)
}

func (s wizardStyles) enabled(value string) string {
	return s.render(lipgloss.NewStyle().Bold(true).Foreground(s.theme.success), value)
}

func (s wizardStyles) disabled(value string) string {
	return s.render(lipgloss.NewStyle().Foreground(s.theme.disabled), value)
}

func (s wizardStyles) selectedLabel(value string) string {
	return s.render(lipgloss.NewStyle().Bold(true).Foreground(s.theme.selectedFG).Background(s.theme.selectedBG).Padding(0, 1), value)
}

func (s wizardStyles) label(value string) string {
	return s.render(lipgloss.NewStyle().Foreground(s.theme.text), value)
}

func (s wizardStyles) key(value string) string {
	return s.render(lipgloss.NewStyle().Bold(true).Foreground(s.theme.warn), value)
}

func (s wizardStyles) err(value string) string {
	return s.render(lipgloss.NewStyle().Bold(true).Foreground(s.theme.err), value)
}

func (s wizardStyles) helpLine(parts ...any) string {
	var out []string
	for i := 0; i < len(parts); i += 2 {
		keys, _ := parts[i].([]string)
		action, _ := parts[i+1].(string)
		if len(keys) == 0 || action == "" {
			continue
		}
		out = append(out, s.key(strings.Join(keys, "/"))+" "+s.muted(action))
	}
	return joinHelpSegments(s.textWidth(), s.muted(" · "), out)
}

func (s wizardStyles) render(style lipgloss.Style, value string) string {
	if !s.color || value == "" {
		return value
	}
	return style.Render(value)
}

func (s wizardStyles) renderBlock(style lipgloss.Style, value string) string {
	value = wrapText(value, s.textWidth())
	if !s.color || value == "" {
		return value
	}
	return style.Render(value)
}

func (s wizardStyles) textWidth() int {
	return contentWidth(s.width, false)
}
