package main

import (
	"fmt"
	"image/color"
	"os"
	"strings"
	"time"

	"burpvalve/internal/attestations"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
)

type attestationBrowseOptions struct {
	root    string
	status  string
	limit   int
	feature string
	bead    string
}

type attestationBrowserModel struct {
	records           []attestations.Record
	filtered          []int
	cursor            int
	detail            bool
	searching         bool
	showHelp          bool
	query             string
	width             int
	height            int
	color             bool
	hasDarkBackground bool
}

type attestationBrowserStyles struct {
	width    int
	listW    int
	detailW  int
	title    lipgloss.Style
	section  lipgloss.Style
	selected lipgloss.Style
	status   lipgloss.Style
	muted    lipgloss.Style
	warn     lipgloss.Style
	err      lipgloss.Style
	path     lipgloss.Style
	panel    lipgloss.Style
}

func newAttestationsBrowseCommand(parent *attestationQueryOptions) *cobra.Command {
	opts := attestationBrowseOptions{
		root:    parent.root,
		status:  parent.status,
		limit:   parent.limit,
		feature: parent.feature,
		bead:    parent.bead,
	}
	cmd := &cobra.Command{
		Use:   "browse",
		Short: "Open a read-only attestation browser TUI.",
		Long: `Open an interactive read-only browser for passing attestations and blocked reports.
Agents and scripts should use attestations list/show/latest --json instead.`,
		Example: `  burpvalve attestations browse
  burpvalve attestations browse --status blocked
  burpvalve attestations list --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAttestationsBrowse(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.root, "root", ".", "repository root")
	cmd.Flags().StringVar(&opts.status, "status", "all", "filter status: all, pass, blocked, or malformed")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum records to return; 0 means no limit")
	cmd.Flags().StringVar(&opts.feature, "feature", "", "filter by feature id")
	cmd.Flags().StringVar(&opts.bead, "bead", "", "filter by bead id when present")
	return cmd
}

func runAttestationsBrowse(cmd *cobra.Command, opts attestationBrowseOptions) error {
	if err := validateAttestationStatus(opts.status); err != nil {
		return err
	}
	if !isInteractiveTerminal(os.Stdin, os.Stdout) {
		return fail(2, "attestations browse requires an interactive terminal; use burpvalve attestations list --json for agents and scripts")
	}
	records, err := attestations.List(opts.root, attestations.QueryOptions{
		Status:  opts.status,
		Limit:   opts.limit,
		Feature: opts.feature,
		Bead:    opts.bead,
	})
	if err != nil {
		return fail(2, "browse attestations: %v", err)
	}
	model := newAttestationBrowserModel(records, shouldColorWriter(cmd.OutOrStdout()))
	program := tea.NewProgram(model, tea.WithInput(os.Stdin), tea.WithOutput(os.Stdout))
	_, err = program.Run()
	return err
}

func newAttestationBrowserModel(records []attestations.Record, color bool) attestationBrowserModel {
	m := attestationBrowserModel{
		records: records,
		width:   100,
		height:  28,
		color:   color,
	}
	m.applyFilter()
	return m
}

func (m attestationBrowserModel) Init() tea.Cmd {
	if !m.color {
		return nil
	}
	return tea.RequestBackgroundColor
}

func (m attestationBrowserModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.BackgroundColorMsg:
		m.hasDarkBackground = msg.IsDark()
	case tea.KeyPressMsg:
		key := msg.String()
		if m.searching {
			switch key {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.searching = false
			case "enter":
				m.searching = false
			case "backspace":
				if len(m.query) > 0 {
					m.query = m.query[:len(m.query)-1]
					m.applyFilter()
				}
			default:
				if text := msg.Key().Text; text != "" {
					m.query += text
					m.applyFilter()
				}
			}
			return m, nil
		}
		switch key {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			if m.detail {
				m.detail = false
				return m, nil
			}
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.filtered) > 0 {
				m.detail = !m.detail
			}
		case "tab":
			m.detail = !m.detail
		case "/":
			m.searching = true
		case "?":
			m.showHelp = !m.showHelp
		}
	}
	return m, nil
}

func (m attestationBrowserModel) View() tea.View {
	return tea.NewView(m.render())
}

func (m attestationBrowserModel) render() string {
	styles := newAttestationBrowserStyles(m.width, m.color, m.hasDarkBackground)
	if m.detail {
		styles.detailW = max(36, styles.width-4)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", styles.title.Render("Burpvalve attestations"))
	search := "search: " + emptyDefault(m.query, "all")
	if m.searching {
		search = "searching: " + m.query
	}
	fmt.Fprintf(&b, "%s  %s\n\n", styles.muted.Render(search), styles.muted.Render(fmt.Sprintf("%d records", len(m.filtered))))
	if len(m.records) == 0 {
		b.WriteString(styles.panel.Width(max(20, styles.listW+styles.detailW+3)).Render("No attestation artifacts found.\n\nUse burpvalve commit to create passing attestations or blocked reports."))
		b.WriteString("\n\n")
		b.WriteString(m.helpLine(styles))
		return b.String()
	}
	left := m.renderList(styles)
	right := m.renderDetail(styles)
	if m.width < 86 || m.detail {
		if m.detail {
			b.WriteString(right)
		} else {
			b.WriteString(left)
		}
	} else {
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right))
	}
	b.WriteString("\n")
	if m.showHelp {
		b.WriteString(styles.panel.Width(max(20, styles.listW+styles.detailW+3)).Render(strings.Join([]string{
			"q/esc exits",
			"up/down or j/k moves",
			"enter toggles detail",
			"/ filters by status, feature, bead, path, hash, condition, verifier, or warning",
			"? toggles this help",
			"Equivalent JSON: burpvalve attestations list --json; show details with burpvalve attestations show <path> --json",
		}, "\n")))
		b.WriteString("\n")
	}
	b.WriteString(m.helpLine(styles))
	return b.String()
}

func (m attestationBrowserModel) renderList(styles attestationBrowserStyles) string {
	var b strings.Builder
	b.WriteString(styles.section.Render("Evidence"))
	b.WriteString("\n")
	if len(m.filtered) == 0 {
		b.WriteString(styles.muted.Render("No records match " + shellQuote(m.query)))
		return styles.panel.Width(styles.listW).Render(b.String())
	}
	visible := min(len(m.filtered), max(4, m.height-8))
	start := 0
	if m.cursor >= visible {
		start = m.cursor - visible + 1
	}
	for row := 0; row < visible && start+row < len(m.filtered); row++ {
		i := start + row
		record := m.records[m.filtered[i]]
		line := fmt.Sprintf("%-8s %-12s %-16s %s",
			record.Status,
			attestationShortValue(firstNonEmptyString(record.PayloadHash, record.ID), 12),
			attestationShortValue(firstNonEmptyString(strings.Join(record.BeadIDs, ","), strings.Join(record.FeatureIDs, ",")), 16),
			attestationShortValue(record.Path, max(16, styles.listW-43)),
		)
		if i == m.cursor {
			line = styles.selected.Render(trimToWidth("> "+line, styles.listW-4))
		} else {
			line = "  " + trimToWidth(line, styles.listW-4)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return styles.panel.Width(styles.listW).Render(strings.TrimRight(b.String(), "\n"))
}

func (m attestationBrowserModel) renderDetail(styles attestationBrowserStyles) string {
	var b strings.Builder
	if len(m.filtered) == 0 {
		b.WriteString(styles.section.Render("Detail"))
		b.WriteString("\n")
		b.WriteString(styles.muted.Render("No selected record."))
		return styles.panel.Width(styles.detailW).Render(b.String())
	}
	record := m.records[m.filtered[m.cursor]]
	b.WriteString(styles.section.Render("Detail"))
	b.WriteString("\n")
	writeTUIKV(&b, styles, "status", record.Status)
	writeTUIKV(&b, styles, "type", record.ArtifactType)
	writeTUIKV(&b, styles, "path", record.Path)
	writeTUIKV(&b, styles, "payload", attestationShortValue(record.PayloadHash, 24))
	writeTUIKV(&b, styles, "feature", strings.Join(record.FeatureIDs, ", "))
	writeTUIKV(&b, styles, "beads", strings.Join(record.BeadIDs, ", "))
	if record.CreatedAt != nil {
		writeTUIKV(&b, styles, "created", record.CreatedAt.Format(time.RFC3339))
	}
	if len(record.ConditionVerdicts) > 0 {
		b.WriteString("\n")
		b.WriteString(styles.section.Render("Conditions"))
		b.WriteString("\n")
		for _, condition := range record.ConditionVerdicts {
			line := fmt.Sprintf("%s %s %s",
				condition.ConditionID,
				condition.Verdict,
				condition.VerifierPolicy,
			)
			b.WriteString(trimToWidth(line, styles.detailW-4))
			b.WriteString("\n")
			verifier := string(condition.VerifierKind)
			if condition.VerifierAgent != "" || condition.VerifierModel != "" {
				verifier += " " + strings.TrimSpace(condition.VerifierAgent+" "+condition.VerifierModel)
			}
			b.WriteString(trimToWidth("  verifier "+verifier, styles.detailW-4))
			b.WriteString("\n")
		}
	}
	if len(record.ParseWarnings) > 0 || len(record.Warnings) > 0 {
		b.WriteString("\n")
		b.WriteString(styles.warn.Render("Warnings"))
		b.WriteString("\n")
		for _, warning := range append(record.ParseWarnings, record.Warnings...) {
			b.WriteString(trimToWidth("- "+warning, styles.detailW-4))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	writeTUIKV(&b, styles, "show JSON", "burpvalve attestations show "+shellQuote(record.Path)+" --json")
	if record.Status == "blocked" {
		writeTUIKV(&b, styles, "explain", "burpvalve explain "+shellQuote(record.Path))
	}
	return styles.panel.Width(styles.detailW).Render(strings.TrimRight(b.String(), "\n"))
}

func (m *attestationBrowserModel) applyFilter() {
	m.filtered = m.filtered[:0]
	query := strings.ToLower(strings.TrimSpace(m.query))
	for i, record := range m.records {
		if query == "" || strings.Contains(strings.ToLower(recordSearchText(record)), query) {
			m.filtered = append(m.filtered, i)
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

func (m attestationBrowserModel) helpLine(styles attestationBrowserStyles) string {
	return styles.muted.Render("j/k move | enter detail | / search | ? help | q quit")
}

func newAttestationBrowserStyles(width int, color bool, hasDarkBackground bool) attestationBrowserStyles {
	if width <= 0 {
		width = 100
	}
	total := max(44, min(width, 132))
	listW := min(58, max(36, total/2-1))
	detailW := max(36, total-listW-3)
	if total < 86 {
		listW = max(36, total-4)
		detailW = listW
	}
	theme := attestationBrowserTheme(hasDarkBackground)
	styles := attestationBrowserStyles{
		width:   total,
		listW:   listW,
		detailW: detailW,
		title:   lipgloss.NewStyle().Bold(true),
		section: lipgloss.NewStyle().Bold(true),
		panel:   lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1),
	}
	if !color {
		styles.title = lipgloss.NewStyle()
		styles.section = lipgloss.NewStyle()
		styles.selected = lipgloss.NewStyle()
		styles.status = lipgloss.NewStyle()
		styles.muted = lipgloss.NewStyle()
		styles.warn = lipgloss.NewStyle()
		styles.err = lipgloss.NewStyle()
		styles.path = lipgloss.NewStyle()
		return styles
	}
	styles.title = styles.title.Foreground(theme.title).Background(theme.titleBG).Padding(0, 1)
	styles.section = styles.section.Foreground(theme.section)
	styles.selected = lipgloss.NewStyle().Foreground(theme.selectedFG).Background(theme.selectedBG).Bold(true)
	styles.status = lipgloss.NewStyle().Foreground(theme.status)
	styles.muted = lipgloss.NewStyle().Foreground(theme.muted)
	styles.warn = lipgloss.NewStyle().Foreground(theme.warn).Bold(true)
	styles.err = lipgloss.NewStyle().Foreground(theme.err).Bold(true)
	styles.path = lipgloss.NewStyle().Foreground(theme.path)
	styles.panel = styles.panel.BorderForeground(theme.border)
	return styles
}

type attestationBrowserPalette struct {
	title      color.Color
	titleBG    color.Color
	section    color.Color
	selectedFG color.Color
	selectedBG color.Color
	status     color.Color
	muted      color.Color
	warn       color.Color
	err        color.Color
	path       color.Color
	border     color.Color
}

func attestationBrowserTheme(hasDarkBackground bool) attestationBrowserPalette {
	pick := lipgloss.LightDark(hasDarkBackground)
	return attestationBrowserPalette{
		title:      lipgloss.Color("#f8fafc"),
		titleBG:    pick(lipgloss.Color("#0e7490"), lipgloss.Color("#155e75")),
		section:    pick(lipgloss.Color("#0f766e"), lipgloss.Color("#5eead4")),
		selectedFG: lipgloss.Color("#f8fafc"),
		selectedBG: pick(lipgloss.Color("#0f766e"), lipgloss.Color("#0e7490")),
		status:     pick(lipgloss.Color("#166534"), lipgloss.Color("#86efac")),
		muted:      pick(lipgloss.Color("#475569"), lipgloss.Color("#94a3b8")),
		warn:       pick(lipgloss.Color("#a16207"), lipgloss.Color("#facc15")),
		err:        pick(lipgloss.Color("#b91c1c"), lipgloss.Color("#f87171")),
		path:       pick(lipgloss.Color("#1d4ed8"), lipgloss.Color("#93c5fd")),
		border:     pick(lipgloss.Color("#cbd5e1"), lipgloss.Color("#334155")),
	}
}

func writeTUIKV(b *strings.Builder, styles attestationBrowserStyles, key string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	fmt.Fprintf(b, "%-10s %s\n", key, trimToWidth(value, styles.detailW-15))
}

func recordSearchText(record attestations.Record) string {
	var parts []string
	parts = append(parts, record.Status, record.ArtifactType, record.Path, record.ID, record.PayloadHash, record.ManifestHash)
	parts = append(parts, record.FeatureIDs...)
	parts = append(parts, record.BeadIDs...)
	parts = append(parts, record.Warnings...)
	parts = append(parts, record.ParseWarnings...)
	for _, condition := range record.ConditionVerdicts {
		parts = append(parts,
			condition.ConditionID,
			condition.ConditionFile,
			string(condition.Verdict),
			string(condition.VerifierPolicy),
			string(condition.VerifierKind),
			condition.VerifierAgent,
			condition.VerifierModel,
		)
	}
	if record.CreatedAt != nil {
		parts = append(parts, record.CreatedAt.Format(time.RFC3339))
	}
	return strings.Join(parts, " ")
}

func emptyDefault(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func attestationShortValue(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}

func attestationShortTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}

func trimToWidth(value string, width int) string {
	if width <= 0 || lipgloss.Width(value) <= width {
		return value
	}
	if width <= 1 {
		return "…"
	}
	runes := []rune(value)
	for len(runes) > 0 && lipgloss.Width(string(runes)+"…") > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}
