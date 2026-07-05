package cliui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

type Styles struct {
	Color          bool
	DarkBackground bool
}

func New(color bool) Styles {
	return NewWithDarkBackground(color, true)
}

func NewWithDarkBackground(color bool, darkBackground bool) Styles {
	return Styles{Color: color, DarkBackground: darkBackground}
}

type theme struct {
	title   color.Color
	section color.Color
	header  color.Color
	label   color.Color
	path    color.Color
	muted   color.Color
	success color.Color
	warn    color.Color
	err     color.Color
	info    color.Color
}

func (s Styles) theme() theme {
	pick := lipgloss.LightDark(s.DarkBackground)
	return theme{
		title:   pick(lipgloss.Color("#0f172a"), lipgloss.Color("#f8fafc")),
		section: pick(lipgloss.Color("#1d4ed8"), lipgloss.Color("#93c5fd")),
		header:  pick(lipgloss.Color("#334155"), lipgloss.Color("#cbd5e1")),
		label:   pick(lipgloss.Color("#334155"), lipgloss.Color("#cbd5e1")),
		path:    pick(lipgloss.Color("#0e7490"), lipgloss.Color("#67e8f9")),
		muted:   pick(lipgloss.Color("#475569"), lipgloss.Color("#94a3b8")),
		success: pick(lipgloss.Color("#15803d"), lipgloss.Color("#86efac")),
		warn:    pick(lipgloss.Color("#a16207"), lipgloss.Color("#facc15")),
		err:     pick(lipgloss.Color("#b91c1c"), lipgloss.Color("#f87171")),
		info:    pick(lipgloss.Color("#0e7490"), lipgloss.Color("#67e8f9")),
	}
}

func (s Styles) Title(value string) string {
	return s.render(lipgloss.NewStyle().Bold(true).Foreground(s.theme().title), value)
}

func (s Styles) Section(value string) string {
	return s.render(lipgloss.NewStyle().Bold(true).Foreground(s.theme().section), value)
}

func (s Styles) Header(value string) string {
	return s.render(lipgloss.NewStyle().Bold(true).Foreground(s.theme().header), value)
}

func (s Styles) Label(value string) string {
	return s.render(lipgloss.NewStyle().Foreground(s.theme().label), value)
}

func (s Styles) Path(value string) string {
	return s.render(lipgloss.NewStyle().Foreground(s.theme().path), value)
}

func (s Styles) Muted(value string) string {
	return s.render(lipgloss.NewStyle().Foreground(s.theme().muted), value)
}

func (s Styles) Success(value string) string {
	return s.render(lipgloss.NewStyle().Bold(true).Foreground(s.theme().success), value)
}

func (s Styles) Warn(value string) string {
	return s.render(lipgloss.NewStyle().Bold(true).Foreground(s.theme().warn), value)
}

func (s Styles) Error(value string) string {
	return s.render(lipgloss.NewStyle().Bold(true).Foreground(s.theme().err), value)
}

func (s Styles) Info(value string) string {
	return s.render(lipgloss.NewStyle().Bold(true).Foreground(s.theme().info), value)
}

func (s Styles) Bool(value bool) string {
	if value {
		return s.Success("yes")
	}
	return s.Warn("no")
}

func (s Styles) Status(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ok", "present", "clean", "passed", "ready", "valid":
		return s.Success(value)
	case "missing", "unavailable", "dirty", "unknown", "skipped", "timeout":
		return s.Warn(value)
	case "conflict", "error", "failed", "blocked":
		return s.Error(value)
	default:
		return s.Info(value)
	}
}

func (s Styles) GitStatus(value string) string {
	switch {
	case strings.Contains(value, "D") || strings.Contains(value, "U"):
		return s.Error(value)
	case strings.Contains(value, "M"):
		return s.Warn(value)
	case strings.Contains(value, "A") || strings.Contains(value, "R") || strings.Contains(value, "C"):
		return s.Success(value)
	case strings.Contains(value, "?"):
		return s.Info(value)
	default:
		return s.Muted(value)
	}
}

func (s Styles) render(style lipgloss.Style, value string) string {
	if !s.Color || value == "" {
		return value
	}
	return style.Render(value)
}

func VisibleLen(value string) int {
	return len(StripANSI(value))
}

func ANSIPadding(value string) int {
	return len(value) - VisibleLen(value)
}

func StripANSI(value string) string {
	for {
		start := strings.IndexByte(value, '\x1b')
		if start == -1 {
			return value
		}
		end := strings.IndexByte(value[start:], 'm')
		if end == -1 {
			return value[:start]
		}
		value = value[:start] + value[start+end+1:]
	}
}
