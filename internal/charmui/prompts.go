package charmui

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var ErrCancelled = errors.New("prompt cancelled")

type TextPrompt struct {
	Title       string
	Description string
	Prompt      string
	Placeholder string
	Default     string
	Required    bool
	Color       bool
}

type ConfirmPrompt struct {
	Title       string
	Description string
	Prompt      string
	Default     bool
	Color       bool
}

type Choice struct {
	ID          string
	Label       string
	Description string
}

type SelectPrompt struct {
	Title       string
	Description string
	Prompt      string
	Choices     []Choice
	DefaultID   string
	Color       bool
}

func AskText(in io.Reader, out io.Writer, opts TextPrompt) (string, error) {
	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = opts.Placeholder
	input.SetValue(opts.Default)
	applyTextInputStyles(&input, opts.Color, true, 0, true)
	input.CursorEnd()
	focusCmd := input.Focus()
	model := textPromptModel{opts: opts, input: input, hasDarkBackground: true}
	final, err := runPrompt(in, out, model, focusCmd)
	if err != nil {
		return "", err
	}
	result := final.(textPromptModel)
	if result.cancelled {
		return "", ErrCancelled
	}
	value := strings.TrimSpace(result.value)
	if opts.Required && value == "" {
		return "", fmt.Errorf("%s is required", promptName(opts.Prompt))
	}
	return value, nil
}

func AskConfirm(in io.Reader, out io.Writer, opts ConfirmPrompt) (bool, error) {
	model := confirmPromptModel{opts: opts, value: opts.Default, hasDarkBackground: true}
	final, err := runPrompt(in, out, model, nil)
	if err != nil {
		return false, err
	}
	result := final.(confirmPromptModel)
	if result.cancelled {
		return false, ErrCancelled
	}
	return result.value, nil
}

func AskSelect(in io.Reader, out io.Writer, opts SelectPrompt) (Choice, error) {
	if len(opts.Choices) == 0 {
		return Choice{}, fmt.Errorf("%s has no choices", promptName(opts.Prompt))
	}
	cursor := 0
	if opts.DefaultID != "" {
		for i, choice := range opts.Choices {
			if choice.ID == opts.DefaultID {
				cursor = i
				break
			}
		}
	}
	model := selectPromptModel{opts: opts, cursor: cursor, hasDarkBackground: true}
	final, err := runPrompt(in, out, model, nil)
	if err != nil {
		return Choice{}, err
	}
	result := final.(selectPromptModel)
	if result.cancelled {
		return Choice{}, ErrCancelled
	}
	return opts.Choices[result.cursor], nil
}

func runPrompt(in io.Reader, out io.Writer, model tea.Model, initCmd tea.Cmd) (tea.Model, error) {
	if in == nil {
		return nil, fmt.Errorf("prompt input is unavailable")
	}
	if out == nil {
		out = io.Discard
	}
	if initCmd != nil {
		model = initWrapper{Model: model, cmd: initCmd}
	}
	p := tea.NewProgram(model, tea.WithInput(in), tea.WithOutput(out))
	final, err := p.Run()
	if wrapped, ok := final.(initWrapper); ok {
		final = wrapped.Model
	}
	return final, err
}

type initWrapper struct {
	tea.Model
	cmd tea.Cmd
}

func (m initWrapper) Init() tea.Cmd {
	return tea.Batch(m.Model.Init(), m.cmd)
}

type promptStyles struct {
	color             bool
	width             int
	hasDarkBackground bool
	theme             semanticTheme
}

func newPromptStyles(color bool, width int, hasDarkBackground bool) promptStyles {
	return promptStyles{
		color:             color,
		width:             width,
		hasDarkBackground: hasDarkBackground,
		theme:             newSemanticTheme(hasDarkBackground),
	}
}

type textPromptModel struct {
	opts              TextPrompt
	input             textinput.Model
	width             int
	hasDarkBackground bool
	value             string
	err               string
	cancelled         bool
}

func (m textPromptModel) Init() tea.Cmd {
	return requestBackgroundColor(m.opts.Color)
}

func (m textPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		applyTextInputStyles(&m.input, m.opts.Color, m.hasDarkBackground, m.width, true)
		return m, nil
	case tea.BackgroundColorMsg:
		m.hasDarkBackground = msg.IsDark()
		applyTextInputStyles(&m.input, m.opts.Color, m.hasDarkBackground, m.width, true)
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "enter":
			value := strings.TrimSpace(m.input.Value())
			if m.opts.Required && value == "" {
				m.err = promptName(m.opts.Prompt) + " is required"
				return m, nil
			}
			m.value = value
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m textPromptModel) View() tea.View {
	var b strings.Builder
	style := newPromptStyles(m.opts.Color, m.width, m.hasDarkBackground)
	writePromptHeader(&b, style, m.opts.Title, m.opts.Description)
	if m.opts.Prompt != "" {
		b.WriteString(style.questionBlock(m.opts.Prompt))
		b.WriteString("\n")
	}
	b.WriteString(m.input.View())
	if m.err != "" {
		b.WriteString("\n")
		b.WriteString(style.err(m.err))
	}
	b.WriteString("\n\n")
	b.WriteString(style.helpLine([]string{"enter"}, "accepts", []string{"esc"}, "cancels"))
	return tea.NewView(style.panel(b.String()))
}

type confirmPromptModel struct {
	opts              ConfirmPrompt
	width             int
	hasDarkBackground bool
	value             bool
	cancelled         bool
}

func (m confirmPromptModel) Init() tea.Cmd {
	return requestBackgroundColor(m.opts.Color)
}

func (m confirmPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case tea.BackgroundColorMsg:
		m.hasDarkBackground = msg.IsDark()
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "left", "h", "right", "l", "tab":
			m.value = !m.value
		case "y":
			m.value = true
			return m, tea.Quit
		case "n":
			m.value = false
			return m, tea.Quit
		case "enter":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m confirmPromptModel) View() tea.View {
	var b strings.Builder
	style := newPromptStyles(m.opts.Color, m.width, m.hasDarkBackground)
	writePromptHeader(&b, style, m.opts.Title, m.opts.Description)
	b.WriteString(style.questionBlock(m.opts.Prompt))
	b.WriteString("\n\n")
	b.WriteString(style.binaryChoice("Yes", m.value))
	b.WriteString("  ")
	b.WriteString(style.binaryChoice("No", !m.value))
	b.WriteString("\n\n")
	b.WriteString(style.helpLine([]string{"left", "right"}, "toggles", []string{"y", "n"}, "answers", []string{"enter"}, "accepts", []string{"esc"}, "cancels"))
	return tea.NewView(style.panel(b.String()))
}

type selectPromptModel struct {
	opts              SelectPrompt
	width             int
	hasDarkBackground bool
	cursor            int
	cancelled         bool
}

func (m selectPromptModel) Init() tea.Cmd {
	return requestBackgroundColor(m.opts.Color)
}

func (m selectPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case tea.BackgroundColorMsg:
		m.hasDarkBackground = msg.IsDark()
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.opts.Choices)-1 {
				m.cursor++
			}
		case "enter":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m selectPromptModel) View() tea.View {
	var b strings.Builder
	style := newPromptStyles(m.opts.Color, m.width, m.hasDarkBackground)
	writePromptHeader(&b, style, m.opts.Title, m.opts.Description)
	if m.opts.Prompt != "" {
		b.WriteString(style.questionBlock(m.opts.Prompt))
		b.WriteString("\n\n")
	}
	for i, choice := range m.opts.Choices {
		cursor := " "
		if i == m.cursor {
			cursor = style.cursor(">")
		}
		label := style.choice(choice.Label, i == m.cursor)
		b.WriteString(fmt.Sprintf("%s %s", cursor, label))
		if choice.Description != "" {
			b.WriteString(style.description(" - " + wrapLine(choice.Description, max(12, style.textWidth()-lipgloss.Width(choice.Label)-6))))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(style.helpLine([]string{"up", "down"}, "moves", []string{"enter"}, "selects", []string{"esc"}, "cancels"))
	return tea.NewView(style.panel(b.String()))
}

func writePromptHeader(b *strings.Builder, style promptStyles, title, description string) {
	if title != "" {
		b.WriteString(style.titleBlock(title))
		b.WriteString("\n")
	}
	if description != "" {
		b.WriteString(style.descriptionBlock(description))
		b.WriteString("\n")
	}
	if title != "" || description != "" {
		b.WriteString("\n")
	}
}

func (s promptStyles) panel(value string) string {
	value = strings.TrimRight(value, "\n")
	if !s.color || value == "" {
		return value
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.theme.border).
		Padding(1, 2).
		Width(renderWidth(s.width)).
		Render(value)
}

func (s promptStyles) titleBlock(value string) string {
	return s.renderBlock(lipgloss.NewStyle().Bold(true).Foreground(s.theme.titleFG).Background(s.theme.titleBG).Padding(0, 1), value)
}

func (s promptStyles) description(value string) string {
	return s.render(lipgloss.NewStyle().Foreground(s.theme.muted), value)
}

func (s promptStyles) descriptionBlock(value string) string {
	return s.renderBlock(lipgloss.NewStyle().Foreground(s.theme.muted), value)
}

func (s promptStyles) questionBlock(value string) string {
	return s.renderBlock(lipgloss.NewStyle().Bold(true).Foreground(s.theme.text), value)
}

func (s promptStyles) cursor(value string) string {
	return s.render(lipgloss.NewStyle().Bold(true).Foreground(s.theme.accent), value)
}

func (s promptStyles) choice(label string, selected bool) string {
	if selected {
		return s.render(lipgloss.NewStyle().Bold(true).Foreground(s.theme.selectedFG).Background(s.theme.selectedBG).Padding(0, 1), label)
	}
	return s.render(lipgloss.NewStyle().Foreground(s.theme.text), label)
}

func (s promptStyles) binaryChoice(label string, selected bool) string {
	if selected {
		return s.render(lipgloss.NewStyle().Bold(true).Foreground(s.theme.selectedFG).Background(s.theme.selectedBG).Padding(0, 1), "[x] "+label)
	}
	return s.render(lipgloss.NewStyle().Foreground(s.theme.disabled), "[ ] "+label)
}

func (s promptStyles) key(value string) string {
	return s.render(lipgloss.NewStyle().Bold(true).Foreground(s.theme.warn), value)
}

func (s promptStyles) err(value string) string {
	return s.render(lipgloss.NewStyle().Bold(true).Foreground(s.theme.err), value)
}

func (s promptStyles) helpLine(parts ...any) string {
	var out []string
	for i := 0; i < len(parts); i += 2 {
		keys, _ := parts[i].([]string)
		action, _ := parts[i+1].(string)
		if len(keys) == 0 || action == "" {
			continue
		}
		out = append(out, s.key(strings.Join(keys, "/"))+" "+s.description(action))
	}
	return joinHelpSegments(s.textWidth(), s.description(" · "), out)
}

func (s promptStyles) render(style lipgloss.Style, value string) string {
	if !s.color || value == "" {
		return value
	}
	return style.Render(value)
}

func (s promptStyles) renderBlock(style lipgloss.Style, value string) string {
	value = wrapText(value, s.textWidth())
	if !s.color || value == "" {
		return value
	}
	return style.Render(value)
}

func (s promptStyles) textWidth() int {
	return contentWidth(s.width, s.color)
}

func promptName(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	prompt = strings.TrimSuffix(prompt, ":")
	if prompt == "" {
		return "answer"
	}
	return prompt
}
