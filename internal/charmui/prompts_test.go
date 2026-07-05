package charmui

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestConfirmPromptViewFitsWindowSize(t *testing.T) {
	model := confirmPromptModel{
		opts: ConfirmPrompt{
			Title: "Backpressure commit gate",
			Description: "Artifact: backpressure/attestations/very-long-artifact-name-that-should-wrap.json\n" +
				"Missing, failing, unknown, unconfirmed, malformed, or stale attestations block the commit.",
			Prompt: "Does the staged diff contain exactly one atomic feature or bug fix?",
			Color:  true,
		},
		value:             false,
		hasDarkBackground: true,
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	model = updated.(confirmPromptModel)
	assertMaxLineWidth(t, model.View().Content, 60)
}

func TestPromptUpdatesThemeFromBackgroundColor(t *testing.T) {
	model := confirmPromptModel{opts: ConfirmPrompt{Prompt: "Continue?", Color: true}, hasDarkBackground: true}
	updated, _ := model.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#ffffff")})
	model = updated.(confirmPromptModel)
	if model.hasDarkBackground {
		t.Fatal("white terminal background should switch prompt theme to light mode")
	}
}

func TestSemanticThemeHasLightAndDarkVariants(t *testing.T) {
	light := newSemanticTheme(false)
	dark := newSemanticTheme(true)
	for name, pair := range map[string][2]color.Color{
		"text":    {light.text, dark.text},
		"muted":   {light.muted, dark.muted},
		"section": {light.section, dark.section},
		"warn":    {light.warn, dark.warn},
	} {
		if sameColor(pair[0], pair[1]) {
			t.Fatalf("%s color should differ between light and dark themes", name)
		}
	}
}

func assertMaxLineWidth(t *testing.T, view string, maxWidth int) {
	t.Helper()
	for _, line := range strings.Split(view, "\n") {
		if width := lipgloss.Width(line); width > maxWidth {
			t.Fatalf("line width %d exceeds %d:\n%s", width, maxWidth, view)
		}
	}
}

func sameColor(left color.Color, right color.Color) bool {
	lr, lg, lb, la := left.RGBA()
	rr, rg, rb, ra := right.RGBA()
	return lr == rr && lg == rg && lb == rb && la == ra
}

func TestConfirmPromptViewUsesLipGlossWhenColorEnabled(t *testing.T) {
	model := confirmPromptModel{
		opts: ConfirmPrompt{
			Title:       "Confirm Burpvalve init",
			Description: "Target: .\nPieces: standard Burpvalve scaffold",
			Prompt:      "Apply these changes?",
			Color:       true,
		},
		value:             true,
		hasDarkBackground: true,
	}
	view := model.View().Content
	for _, want := range []string{"\x1b[", "Confirm Burpvalve init", "Apply these changes?", "[x] Yes", "[ ] No"} {
		if !strings.Contains(view, want) {
			t.Fatalf("styled confirm view missing %q:\n%s", want, view)
		}
	}
}

func TestTextPromptViewUsesLipGlossWhenColorEnabled(t *testing.T) {
	model := textPromptModel{
		opts: TextPrompt{
			Title:       "Feature for this commit",
			Description: "Pick one atomic feature.",
			Prompt:      "Feature id",
			Color:       true,
		},
		hasDarkBackground: true,
	}
	view := model.View().Content
	for _, want := range []string{"\x1b[", "Feature for this commit", "Feature id", "enter"} {
		if !strings.Contains(view, want) {
			t.Fatalf("styled text view missing %q:\n%s", want, view)
		}
	}
}

func TestSelectPromptViewUsesLipGlossWhenColorEnabled(t *testing.T) {
	model := selectPromptModel{
		opts: SelectPrompt{
			Title:     "Verifier verdict",
			Prompt:    "Choose verdict",
			Color:     true,
			Choices:   []Choice{{ID: "pass", Label: "pass", Description: "condition is satisfied"}, {ID: "fail", Label: "fail"}},
			DefaultID: "pass",
		},
		cursor:            0,
		hasDarkBackground: true,
	}
	view := model.View().Content
	for _, want := range []string{"\x1b[", "Verifier verdict", "Choose verdict", ">"} {
		if !strings.Contains(view, want) {
			t.Fatalf("styled select view missing %q:\n%s", want, view)
		}
	}
}

func TestPromptViewsStayPlainWhenColorDisabled(t *testing.T) {
	view := confirmPromptModel{
		opts:  ConfirmPrompt{Title: "Confirm", Prompt: "Continue?"},
		value: true,
	}.View().Content
	if strings.Contains(view, "\x1b[") {
		t.Fatalf("plain confirm view included ANSI:\n%s", view)
	}
	if strings.Contains(view, "\u256d") || strings.Contains(view, "\u2502") {
		t.Fatalf("plain confirm view included decorative border:\n%s", view)
	}
}
