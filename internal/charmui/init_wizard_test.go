package charmui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestScaffoldWizardSkipsOmitQuestionsAndDisableResults(t *testing.T) {
	skip := InitWizardResult{
		Hooks:     true,
		HooksPath: true,
		Bin:       true,
		Beads:     true,
		NTM:       true,
	}
	result := DefaultInitWizardResult(".")
	applyWizardSkips(&result, skip)

	if result.Hooks || result.HooksPath || result.Bin || result.Beads || result.NTM {
		t.Fatalf("skipped pieces should be disabled in result: %#v", result)
	}

	items := scaffoldWizardItems(skip, true)
	labels := wizardItemLabels(items)
	for _, forbidden := range []string{".beads", "NTM bridge", "pre-commit hook", "git hooksPath", "repo-local bin/burpvalve"} {
		if containsWizardLabel(labels, forbidden) {
			t.Fatalf("skipped question %q should not be shown: %#v", forbidden, labels)
		}
	}
	for _, want := range []string{"AGENTS.md operating contract", "Claude Code route", "backpressure/"} {
		if !containsWizardLabel(labels, want) {
			t.Fatalf("unskipped question %q should still be shown: %#v", want, labels)
		}
	}
}

func TestScaffoldWizardClaudeRouteStep(t *testing.T) {
	result := DefaultInitWizardResult(".")
	model := initWizardModel{
		config: scaffoldWizardConfig{
			title:      "Burpvalve init",
			itemPrompt: "Which pieces should Burpvalve set up?",
			runHelp:    "enter runs init",
		},
		step:         1,
		result:       result,
		items:        scaffoldWizardItems(InitWizardResult{}, true),
		routeChoices: claudeRouteChoices(),
		routeCursor:  routeChoiceIndex(result.ClaudeRoute),
	}

	updated, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd != nil {
		t.Fatal("enter from piece step should move to route step, not quit")
	}
	model = updated.(initWizardModel)
	if model.step != 2 {
		t.Fatalf("step = %d, want route step", model.step)
	}
	content := model.View().Content
	for _, want := range []string{"How should Claude Code use this repo?", "Ordinary agent", "Orchestrator", "No Claude route", ".claude/skills/burpvalve-orchestrator/"} {
		if !strings.Contains(content, want) {
			t.Fatalf("route prompt missing %q:\n%s", want, content)
		}
	}
}

func TestScaffoldWizardRouteSelection(t *testing.T) {
	model := initWizardModel{
		config:       scaffoldWizardConfig{runHelp: "enter runs init"},
		step:         2,
		result:       DefaultInitWizardResult("."),
		items:        scaffoldWizardItems(InitWizardResult{}, true),
		routeChoices: claudeRouteChoices(),
		routeCursor:  routeChoiceIndex("orchestrator-skill"),
	}

	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	model = updated.(initWizardModel)
	if model.result.ClaudeRoute != "orchestrator-skill" {
		t.Fatalf("selected route = %q", model.result.ClaudeRoute)
	}
}

func TestScaffoldWizardSkippedClaudeSetsRouteNone(t *testing.T) {
	result := DefaultInitWizardResult(".")
	applyWizardSkips(&result, InitWizardResult{Claude: true})
	if result.Claude || result.ClaudeRoute != "none" {
		t.Fatalf("skipping Claude should disable the route: %#v", result)
	}
}

func TestRepairWizardDoesNotShowNTMQuestion(t *testing.T) {
	labels := wizardItemLabels(scaffoldWizardItems(InitWizardResult{}, false))
	if containsWizardLabel(labels, "NTM bridge") {
		t.Fatalf("repair wizard should not show NTM question: %#v", labels)
	}
	if containsWizardLabel(labels, "ORCHESTRATOR.md") {
		t.Fatalf("repair wizard should not show orchestrator question: %#v", labels)
	}
}

func TestInitWizardShowsUncheckedOrchestratorOnlyWithBeadsOrNTM(t *testing.T) {
	result := DefaultInitWizardResult(".")
	items := scaffoldWizardItems(InitWizardResult{}, true)
	model := initWizardModel{result: result, items: items}
	if !containsWizardLabel(wizardItemLabels(model.visibleItems()), "ORCHESTRATOR.md") {
		t.Fatalf("init wizard should show orchestrator when Beads/NTM are selected: %#v", wizardItemLabels(model.visibleItems()))
	}
	if result.Orchestrator {
		t.Fatalf("orchestrator should default unchecked: %#v", result)
	}

	result.Beads = false
	result.NTM = false
	model.result = result
	if containsWizardLabel(wizardItemLabels(model.visibleItems()), "ORCHESTRATOR.md") {
		t.Fatalf("orchestrator should hide when Beads and NTM are both unselected: %#v", wizardItemLabels(model.visibleItems()))
	}
}

func TestInitWizardClearsHiddenOrchestratorSelection(t *testing.T) {
	result := DefaultInitWizardResult(".")
	result.Orchestrator = true
	model := initWizardModel{
		result: result,
		items:  scaffoldWizardItems(InitWizardResult{}, true),
	}

	model.result.Beads = false
	model.result.NTM = false
	model.normalizeHiddenSelections()

	if model.result.Orchestrator {
		t.Fatalf("hidden orchestrator selection should be cleared: %#v", model.result)
	}
	if containsWizardLabel(wizardItemLabels(model.visibleItems()), "ORCHESTRATOR.md") {
		t.Fatalf("orchestrator should remain hidden when Beads and NTM are false: %#v", wizardItemLabels(model.visibleItems()))
	}
}

func TestSkippedQuestionsAreAbsentFromRenderedWizard(t *testing.T) {
	skip := InitWizardResult{Beads: true, NTM: true}
	result := DefaultInitWizardResult(".")
	applyWizardSkips(&result, skip)
	model := initWizardModel{
		config: scaffoldWizardConfig{
			title:      "Burpvalve init",
			itemPrompt: "Which pieces should Burpvalve set up?",
		},
		step:   1,
		result: result,
		items:  scaffoldWizardItems(skip, true),
	}

	content := model.View().Content
	for _, forbidden := range []string{".beads", "NTM bridge"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("rendered wizard should omit skipped question %q:\n%s", forbidden, content)
		}
	}
	if !strings.Contains(content, "AGENTS.md operating contract") {
		t.Fatalf("rendered wizard should keep unskipped questions:\n%s", content)
	}
}

func TestScaffoldWizardViewFitsWindowSize(t *testing.T) {
	result := DefaultInitWizardResult(".")
	model := initWizardModel{
		config: scaffoldWizardConfig{
			title:       "Burpvalve init",
			description: "Choose the repo and the pieces Burpvalve should install.",
			itemPrompt:  "Which pieces should Burpvalve set up?",
			runHelp:     "enter runs init",
			color:       true,
		},
		step:              1,
		result:            result,
		items:             scaffoldWizardItems(InitWizardResult{}, true),
		hasDarkBackground: true,
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model = updated.(initWizardModel)
	for _, line := range strings.Split(model.View().Content, "\n") {
		if width := lipgloss.Width(line); width > 80 {
			t.Fatalf("line width %d exceeds 80:\n%s", width, model.View().Content)
		}
	}
}

func TestScaffoldWizardUpdatesThemeFromBackgroundColor(t *testing.T) {
	model := initWizardModel{
		config:            scaffoldWizardConfig{color: true},
		hasDarkBackground: true,
	}
	updated, _ := model.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#ffffff")})
	model = updated.(initWizardModel)
	if model.hasDarkBackground {
		t.Fatal("white terminal background should switch wizard theme to light mode")
	}
}

func wizardItemLabels(items []initWizardItem) []string {
	labels := make([]string, 0, len(items))
	for _, item := range items {
		labels = append(labels, item.label)
	}
	return labels
}

func containsWizardLabel(labels []string, want string) bool {
	for _, label := range labels {
		if label == want {
			return true
		}
	}
	return false
}
