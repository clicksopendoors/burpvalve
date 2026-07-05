package charmui

import (
	"image/color"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	defaultTUIWidth = 88
	maxTUIWidth     = 96
	minTUIWidth     = 32
)

type semanticTheme struct {
	titleFG    color.Color
	titleBG    color.Color
	border     color.Color
	section    color.Color
	text       color.Color
	muted      color.Color
	disabled   color.Color
	selectedFG color.Color
	selectedBG color.Color
	accent     color.Color
	success    color.Color
	warn       color.Color
	err        color.Color
}

func newSemanticTheme(hasDarkBackground bool) semanticTheme {
	pick := lipgloss.LightDark(hasDarkBackground)
	return semanticTheme{
		titleFG:    lipgloss.Color("#f8fafc"),
		titleBG:    pick(lipgloss.Color("#1d4ed8"), lipgloss.Color("#2563eb")),
		border:     pick(lipgloss.Color("#cbd5e1"), lipgloss.Color("#334155")),
		section:    pick(lipgloss.Color("#1d4ed8"), lipgloss.Color("#93c5fd")),
		text:       pick(lipgloss.Color("#0f172a"), lipgloss.Color("#f8fafc")),
		muted:      pick(lipgloss.Color("#475569"), lipgloss.Color("#94a3b8")),
		disabled:   pick(lipgloss.Color("#64748b"), lipgloss.Color("#64748b")),
		selectedFG: lipgloss.Color("#f8fafc"),
		selectedBG: pick(lipgloss.Color("#1e40af"), lipgloss.Color("#1d4ed8")),
		accent:     pick(lipgloss.Color("#0e7490"), lipgloss.Color("#67e8f9")),
		success:    pick(lipgloss.Color("#15803d"), lipgloss.Color("#86efac")),
		warn:       pick(lipgloss.Color("#a16207"), lipgloss.Color("#facc15")),
		err:        pick(lipgloss.Color("#b91c1c"), lipgloss.Color("#f87171")),
	}
}

func requestBackgroundColor(enabled bool) tea.Cmd {
	if !enabled {
		return nil
	}
	return tea.RequestBackgroundColor
}

func renderWidth(width int) int {
	if width <= 0 {
		return defaultTUIWidth
	}
	if width < minTUIWidth {
		return width
	}
	if width > maxTUIWidth {
		return maxTUIWidth
	}
	return width
}

func contentWidth(width int, framed bool) int {
	available := renderWidth(width)
	if framed {
		available -= 6
	}
	if available < 12 {
		return 12
	}
	return available
}

func applyTextInputStyles(input *textinput.Model, colorEnabled bool, hasDarkBackground bool, width int, framed bool) {
	input.SetWidth(contentWidth(width, framed))
	if !colorEnabled {
		return
	}
	theme := newSemanticTheme(hasDarkBackground)
	styles := textinput.DefaultDarkStyles()
	styles.Focused.Prompt = lipgloss.NewStyle().Bold(true).Foreground(theme.accent)
	styles.Focused.Text = lipgloss.NewStyle().Foreground(theme.text)
	styles.Focused.Placeholder = lipgloss.NewStyle().Foreground(theme.disabled)
	styles.Cursor.Color = theme.warn
	styles.Cursor.Shape = tea.CursorBar
	styles.Cursor.Blink = true
	input.SetStyles(styles)
}

func wrapText(value string, width int) string {
	if width <= 0 {
		return value
	}
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = wrapLine(line, width)
	}
	return strings.Join(lines, "\n")
}

func wrapLine(line string, width int) string {
	if lipgloss.Width(line) <= width {
		return line
	}
	words := strings.Fields(line)
	if len(words) == 0 {
		return line
	}
	var lines []string
	current := ""
	flush := func() {
		if current != "" {
			lines = append(lines, current)
			current = ""
		}
	}
	for _, word := range words {
		if lipgloss.Width(word) > width {
			flush()
			lines = append(lines, splitLongWord(word, width)...)
			continue
		}
		if current == "" {
			current = word
			continue
		}
		candidate := current + " " + word
		if lipgloss.Width(candidate) <= width {
			current = candidate
			continue
		}
		flush()
		current = word
	}
	flush()
	return strings.Join(lines, "\n")
}

func splitLongWord(word string, width int) []string {
	if width <= 0 || lipgloss.Width(word) <= width {
		return []string{word}
	}
	var chunks []string
	var current strings.Builder
	for _, r := range word {
		next := current.String() + string(r)
		if current.Len() > 0 && lipgloss.Width(next) > width {
			chunks = append(chunks, current.String())
			current.Reset()
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}

func joinHelpSegments(width int, separator string, segments []string) string {
	if width <= 0 {
		return strings.Join(segments, separator)
	}
	var lines []string
	current := ""
	for _, segment := range segments {
		if current == "" {
			current = segment
			continue
		}
		candidate := current + separator + segment
		if lipgloss.Width(candidate) <= width {
			current = candidate
			continue
		}
		lines = append(lines, current)
		current = segment
	}
	if current != "" {
		lines = append(lines, current)
	}
	return strings.Join(lines, "\n")
}
