package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"burpvalve/internal/backpressure"
	"burpvalve/internal/charmui"
	"burpvalve/internal/lintconfig"
	"burpvalve/internal/scaffold"
)

type lintInitOptions struct {
	root         string
	jsonOutput   bool
	detect       bool
	write        bool
	force        bool
	preset       string
	jobs         int
	robotConfirm bool
	wizardPlan   *lintInitWizardPlan
	now          func() time.Time
}

type lintInitResult struct {
	SchemaVersion        int                        `json:"schema_version"`
	Command              string                     `json:"command"`
	Status               string                     `json:"status"`
	Message              string                     `json:"message"`
	Fatal                bool                       `json:"fatal"`
	Mutating             bool                       `json:"mutating"`
	Root                 string                     `json:"root"`
	Preset               string                     `json:"preset"`
	Jobs                 int                        `json:"jobs,omitempty"`
	Detection            backpressure.LintDetection `json:"detection"`
	Coverage             lintInitCoverage           `json:"coverage"`
	ProposedCommands     []lintconfig.Command       `json:"proposed_commands,omitempty"`
	ManifestUpdate       lintInitFileUpdate         `json:"manifest_update"`
	Recommendations      []string                   `json:"recommendations,omitempty"`
	LintRulesUpdate      lintInitFileUpdate         `json:"lint_rules_update"`
	ConfirmationRequired bool                       `json:"confirmation_required"`
	Warnings             []string                   `json:"warnings,omitempty"`
	NextSteps            []scaffold.RecoveryStep    `json:"next_steps,omitempty"`
}

type lintInitCoverage struct {
	Status           string   `json:"status"`
	MultiRoot        bool     `json:"multi_root"`
	NeedsScopedSetup bool     `json:"needs_scoped_setup"`
	CoveredRoots     []string `json:"covered_roots,omitempty"`
	UncheckedRoots   []string `json:"unchecked_roots,omitempty"`
	DeclinedRoots    []string `json:"declined_roots,omitempty"`
	DeclinedAt       string   `json:"declined_at,omitempty"`
}

type lintInitFileUpdate struct {
	Path    string   `json:"path"`
	Changed bool     `json:"changed"`
	Added   []string `json:"added,omitempty"`
	Updated []string `json:"updated,omitempty"`
	Renamed []string `json:"renamed,omitempty"`
	Skipped []string `json:"skipped,omitempty"`
	Before  string   `json:"before,omitempty"`
	After   string   `json:"after,omitempty"`
}

type lintInitWizardPlan struct {
	ScopedSetup   bool
	SelectedIDs   map[string]bool
	Commands      map[string]lintconfig.Command
	DeclinedRoots []string
	DeclinedAt    string
}

type lintInitPromptIO struct {
	confirm func(charmui.ConfirmPrompt) (bool, error)
	text    func(charmui.TextPrompt) (string, error)
}

var lintInitPrompts = lintInitPromptIO{
	confirm: func(prompt charmui.ConfirmPrompt) (bool, error) {
		return charmui.AskConfirm(os.Stdin, os.Stdout, prompt)
	},
	text: func(prompt charmui.TextPrompt) (string, error) {
		return charmui.AskText(os.Stdin, os.Stdout, prompt)
	},
}

func runLintInit(opts lintInitOptions) error {
	if opts.jobs <= 0 {
		return fail(2, "lint init --jobs must be positive")
	}
	if shouldRunLintInitWizard(opts) {
		wizardOpts, err := runLintInitWizard(opts)
		if err != nil {
			return err
		}
		opts = wizardOpts
	}
	result, err := buildLintInitResult(opts)
	if err != nil {
		return err
	}
	shouldWrite := opts.write && !opts.detect
	if shouldWrite {
		confirmed := opts.force || opts.robotConfirm
		if !confirmed && !opts.jsonOutput && isInteractiveTerminal(os.Stdin, os.Stdout) {
			if err := confirmLintInitMutation(opts, result); err != nil {
				return err
			}
			confirmed = true
		}
		if !confirmed {
			result.Status = "confirmation_required"
			result.Message = "lint init --write requires confirmation before changing files"
			result.Fatal = true
			result.ConfirmationRequired = true
			result.NextSteps = []scaffold.RecoveryStep{{
				ID:      "confirm-lint-init-write",
				Message: "Review the proposed manifest and lint-rules changes, then confirm the write.",
				Command: "burpvalve lint init --write --force --preset " + result.Preset,
				Fatal:   true,
			}}
			if encodeErr := writeLintInitResult(os.Stdout, os.Stderr, result, opts.jsonOutput); encodeErr != nil {
				return encodeErr
			}
			return exitCode(2)
		}
		if err := applyLintInitWrites(opts); err != nil {
			return err
		}
		result, err = buildLintInitResult(opts)
		if err != nil {
			return err
		}
		result.Status = "written"
		result.Message = "lint setup recommendations written"
		result.Mutating = true
	}
	if encodeErr := writeLintInitResult(os.Stdout, os.Stderr, result, opts.jsonOutput); encodeErr != nil {
		return encodeErr
	}
	return err
}

func buildLintInitResult(opts lintInitOptions) (lintInitResult, error) {
	root, err := filepath.Abs(defaultCLIRoot(opts.root))
	if err != nil {
		return lintInitResult{}, err
	}
	preset := normalizeLintInitPreset(opts.preset)
	detection, err := backpressure.DetectLintSetup(backpressure.LintDetectionOptions{Root: root})
	if err != nil {
		return lintInitResult{}, err
	}
	commands := lintInitWizardCommands(lintInitPresetCommands(detection.CandidateCommands, preset), opts.wizardPlan)
	coverageData := lintInitCoverageData(opts.wizardPlan)
	update := backpressure.LintManifestUpdate{Commands: lintInitCommandProposals(commands), Coverage: coverageData}
	manifestUpdate, err := backpressure.PlanLintManifestUpdate(root, update)
	if err != nil {
		return lintInitResult{}, err
	}
	recommendations := lintInitRecommendations(detection, commands)
	rulesUpdate, err := backpressure.PlanLintRulesRecommendationsUpdate(root, recommendations)
	if err != nil {
		return lintInitResult{}, err
	}
	result := lintInitResult{
		SchemaVersion:    1,
		Command:          "lint init",
		Status:           "detected",
		Message:          "lint setup detected; no files changed",
		Root:             root,
		Preset:           preset,
		Jobs:             opts.jobs,
		Detection:        detection,
		Coverage:         lintInitDetectionCoverage(detection, commands, coverageData),
		ProposedCommands: commands,
		ManifestUpdate:   lintInitFileUpdateFromResult(manifestUpdate),
		Recommendations:  recommendations,
		LintRulesUpdate:  lintInitFileUpdateFromResult(rulesUpdate),
	}
	if opts.force && !opts.write {
		result.Warnings = append(result.Warnings, "--force without --write is read-only and did not change files")
	}
	if opts.detect && opts.write {
		result.Warnings = append(result.Warnings, "--detect is read-only and ignored --write")
	}
	if len(coverageData.DeclinedRoots) > 0 {
		result.Warnings = append(result.Warnings, "scoped lint setup declined; unchecked roots: "+strings.Join(coverageData.DeclinedRoots, ", "))
	}
	if len(commands) == 0 {
		result.Warnings = append(result.Warnings, "no executable Go or Node/Astro lint commands were detected for the selected preset")
	}
	return result, nil
}

func applyLintInitWrites(opts lintInitOptions) error {
	root, err := filepath.Abs(defaultCLIRoot(opts.root))
	if err != nil {
		return err
	}
	preset := normalizeLintInitPreset(opts.preset)
	detection, err := backpressure.DetectLintSetup(backpressure.LintDetectionOptions{Root: root})
	if err != nil {
		return err
	}
	commands := lintInitWizardCommands(lintInitPresetCommands(detection.CandidateCommands, preset), opts.wizardPlan)
	update := backpressure.LintManifestUpdate{Commands: lintInitCommandProposals(commands), Coverage: lintInitCoverageData(opts.wizardPlan)}
	if _, err := backpressure.WriteLintManifestUpdate(root, update); err != nil {
		return err
	}
	if _, err := backpressure.WriteLintRulesRecommendationsUpdate(root, lintInitRecommendations(detection, commands)); err != nil {
		return err
	}
	return nil
}

func normalizeLintInitPreset(preset string) string {
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case "", "auto", "all":
		return "auto"
	case "go", "node", "astro":
		return strings.ToLower(strings.TrimSpace(preset))
	default:
		return strings.ToLower(strings.TrimSpace(preset))
	}
}

func lintInitPresetCommands(commands []lintconfig.Command, preset string) []lintconfig.Command {
	preset = normalizeLintInitPreset(preset)
	var selected []lintconfig.Command
	for _, command := range commands {
		if lintInitCommandMatchesPreset(command, preset) {
			selected = append(selected, command)
		}
	}
	return selected
}

func lintInitCommandMatchesPreset(command lintconfig.Command, preset string) bool {
	if preset == "auto" {
		return true
	}
	if preset == "go" {
		return strings.HasPrefix(command.ID, "go-")
	}
	if preset == "node" || preset == "astro" {
		return strings.HasPrefix(command.ID, "npm-") ||
			strings.HasPrefix(command.ID, "pnpm-") ||
			strings.HasPrefix(command.ID, "yarn-") ||
			strings.HasPrefix(command.ID, "bun-")
	}
	return false
}

func lintInitCommandProposals(commands []lintconfig.Command) []backpressure.LintCommandProposal {
	proposals := make([]backpressure.LintCommandProposal, 0, len(commands))
	for _, command := range commands {
		proposals = append(proposals, backpressure.LintCommandProposal{
			Command:    command,
			OnConflict: backpressure.LintCommandConflictSkip,
		})
	}
	return proposals
}

func shouldRunLintInitWizard(opts lintInitOptions) bool {
	return !opts.jsonOutput && !opts.detect && !opts.write && !opts.force && !opts.robotConfirm && isInteractiveTerminal(os.Stdin, os.Stdout)
}

func runLintInitWizard(opts lintInitOptions) (lintInitOptions, error) {
	result, err := buildLintInitResult(opts)
	if err != nil {
		return opts, err
	}
	plan, confirmed, err := collectLintInitWizardPlan(result, opts)
	if err != nil {
		if strings.Contains(err.Error(), "cancel") {
			return opts, fail(2, "lint init cancelled; no files changed")
		}
		return opts, err
	}
	if !confirmed {
		return opts, fail(2, "lint init cancelled; no files changed")
	}
	opts.write = true
	opts.wizardPlan = &plan
	opts.robotConfirm = true
	return opts, nil
}

func collectLintInitWizardPlan(result lintInitResult, opts lintInitOptions) (lintInitWizardPlan, bool, error) {
	plan := lintInitWizardPlan{
		ScopedSetup: true,
		SelectedIDs: map[string]bool{},
		Commands:    map[string]lintconfig.Command{},
	}
	for _, command := range result.ProposedCommands {
		plan.SelectedIDs[command.ID] = true
		plan.Commands[command.ID] = command
	}
	if result.Detection.NeedsScopedSetup {
		ok, err := lintInitPrompts.confirm(charmui.ConfirmPrompt{
			Title:       "Configure scoped lint commands",
			Description: lintInitScopedSetupDescription(result),
			Prompt:      "Set up commands for each detected root?",
			Default:     true,
			Color:       shouldColor(os.Stdout),
		})
		if err != nil {
			return plan, false, err
		}
		plan.ScopedSetup = ok
		if !ok {
			plan.DeclinedRoots = lintInitUncheckedRoots(result.Detection, lintInitRepoRootCommands(result.ProposedCommands))
			plan.DeclinedAt = lintInitNow(opts).Format(time.RFC3339)
			plan.SelectedIDs = map[string]bool{}
			plan.Commands = map[string]lintconfig.Command{}
			for _, command := range lintInitRepoRootCommands(result.ProposedCommands) {
				plan.SelectedIDs[command.ID] = true
				plan.Commands[command.ID] = command
			}
		}
	}
	if plan.ScopedSetup {
		for _, command := range result.ProposedCommands {
			selected, err := lintInitPrompts.confirm(charmui.ConfirmPrompt{
				Title:       "Select lint command",
				Description: lintInitCommandDescription(command),
				Prompt:      "Include " + command.ID + "?",
				Default:     plan.SelectedIDs[command.ID],
				Color:       shouldColor(os.Stdout),
			})
			if err != nil {
				return plan, false, err
			}
			if !selected {
				delete(plan.SelectedIDs, command.ID)
				continue
			}
			edited, err := collectLintInitCommandSettings(command)
			if err != nil {
				return plan, false, err
			}
			plan.SelectedIDs[command.ID] = true
			plan.Commands[command.ID] = edited
		}
	}
	preview := result
	commands := lintInitWizardCommands(result.ProposedCommands, &plan)
	coverageData := lintInitCoverageData(&plan)
	preview.ProposedCommands = commands
	preview.Coverage = lintInitDetectionCoverage(result.Detection, commands, coverageData)
	manifestUpdate, err := backpressure.PlanLintManifestUpdate(result.Root, backpressure.LintManifestUpdate{Commands: lintInitCommandProposals(commands), Coverage: coverageData})
	if err != nil {
		return plan, false, err
	}
	recommendations := lintInitRecommendations(result.Detection, commands)
	rulesUpdate, err := backpressure.PlanLintRulesRecommendationsUpdate(result.Root, recommendations)
	if err != nil {
		return plan, false, err
	}
	preview.ManifestUpdate = lintInitFileUpdateFromResult(manifestUpdate)
	preview.Recommendations = recommendations
	preview.LintRulesUpdate = lintInitFileUpdateFromResult(rulesUpdate)
	confirmed, err := lintInitPrompts.confirm(charmui.ConfirmPrompt{
		Title:       "Confirm Burpvalve lint init",
		Description: lintInitPreviewDescription(preview, plan),
		Prompt:      "Write these lint setup changes?",
		Default:     false,
		Color:       shouldColor(os.Stdout),
	})
	return plan, confirmed, err
}

func collectLintInitCommandSettings(command lintconfig.Command) (lintconfig.Command, error) {
	required, err := lintInitPrompts.confirm(charmui.ConfirmPrompt{
		Title:       "Lint command requirement",
		Description: lintInitCommandDescription(command),
		Prompt:      "Should " + command.ID + " be required?",
		Default:     command.Required,
		Color:       shouldColor(os.Stdout),
	})
	if err != nil {
		return command, err
	}
	command.Required = required
	timeout, err := lintInitPrompts.text(charmui.TextPrompt{
		Title:       "Lint command timeout",
		Description: lintInitCommandDescription(command),
		Prompt:      "Timeout seconds for " + command.ID,
		Default:     strconv.Itoa(command.TimeoutSeconds),
		Placeholder: strconv.Itoa(command.TimeoutSeconds),
		Color:       shouldColor(os.Stdout),
	})
	if err != nil {
		return command, err
	}
	if strings.TrimSpace(timeout) != "" {
		parsed, parseErr := strconv.Atoi(strings.TrimSpace(timeout))
		if parseErr != nil || parsed <= 0 {
			return command, fail(2, "lint init timeout for %s must be a positive integer", command.ID)
		}
		command.TimeoutSeconds = parsed
	}
	paths, err := lintInitPrompts.text(charmui.TextPrompt{
		Title:       "Lint command scope",
		Description: lintInitCommandDescription(command),
		Prompt:      "Scope paths for " + command.ID + " (comma-separated)",
		Default:     strings.Join(command.Paths, ","),
		Placeholder: strings.Join(command.Paths, ","),
		Color:       shouldColor(os.Stdout),
	})
	if err != nil {
		return command, err
	}
	if parsed := splitLintInitCSV(paths); len(parsed) > 0 {
		command.Paths = parsed
	}
	runDir, err := lintInitPrompts.text(charmui.TextPrompt{
		Title:       "Lint command run directory",
		Description: lintInitCommandDescription(command),
		Prompt:      "Run directory for " + command.ID,
		Default:     command.RunDirectory,
		Placeholder: command.RunDirectory,
		Color:       shouldColor(os.Stdout),
	})
	if err != nil {
		return command, err
	}
	if strings.TrimSpace(runDir) != "" {
		command.RunDirectory = strings.TrimSpace(runDir)
	}
	serial, err := lintInitPrompts.confirm(charmui.ConfirmPrompt{
		Title:       "Lint command scheduling",
		Description: "Serial commands run after the bounded parallel batch and preserve manifest order relative to other serial commands.",
		Prompt:      "Run " + command.ID + " serially?",
		Default:     command.Serial,
		Color:       shouldColor(os.Stdout),
	})
	if err != nil {
		return command, err
	}
	command.Serial = serial
	return command, nil
}

func lintInitWizardCommands(commands []lintconfig.Command, plan *lintInitWizardPlan) []lintconfig.Command {
	if plan == nil {
		return commands
	}
	var selected []lintconfig.Command
	for _, command := range commands {
		if !plan.SelectedIDs[command.ID] {
			continue
		}
		if edited, ok := plan.Commands[command.ID]; ok {
			selected = append(selected, edited)
		} else {
			selected = append(selected, command)
		}
	}
	return selected
}

func lintInitCoverageData(plan *lintInitWizardPlan) lintconfig.Coverage {
	if plan == nil {
		return lintconfig.Coverage{}
	}
	return lintconfig.Coverage{DeclinedRoots: plan.DeclinedRoots, DeclinedAt: plan.DeclinedAt}
}

func lintInitRecommendations(detection backpressure.LintDetection, commands []lintconfig.Command) []string {
	var recommendations []string
	if len(commands) > 0 {
		recommendations = append(recommendations, fmt.Sprintf("Review and run %d proposed executable lint command(s) before treating lint coverage as enforced.", len(commands)))
	} else {
		recommendations = append(recommendations, "No executable Go or Node/Astro lint commands were detected; add explicit lint_commands when deterministic lint coverage is ready.")
	}
	if detection.NeedsScopedSetup {
		recommendations = append(recommendations, "This repository has multiple detected lint roots; keep scoped setup explicit and record any declined roots as reduced coverage.")
	}
	if len(detection.GoRoots) > 0 && !detection.GoAvailable {
		recommendations = append(recommendations, "Go roots were detected but go is unavailable on PATH, so Go commands were not proposed.")
	}
	return recommendations
}

func lintInitDetectionCoverage(detection backpressure.LintDetection, commands []lintconfig.Command, coverageData lintconfig.Coverage) lintInitCoverage {
	covered := map[string]bool{}
	for _, command := range commands {
		for _, path := range command.Paths {
			covered[path] = true
		}
	}
	var coveredRoots []string
	var uncheckedRoots []string
	for _, root := range detection.GoRoots {
		if covered[root.Path] {
			coveredRoots = append(coveredRoots, root.Path)
		} else {
			uncheckedRoots = append(uncheckedRoots, root.Path)
		}
	}
	for _, root := range detection.NodeRoots {
		if covered[root.Path] {
			coveredRoots = append(coveredRoots, root.Path)
		} else {
			uncheckedRoots = append(uncheckedRoots, root.Path)
		}
	}
	status := "full"
	if len(uncheckedRoots) > 0 {
		status = "partial"
	}
	if len(coveredRoots) == 0 && len(uncheckedRoots) == 0 {
		status = "none"
	}
	if len(coverageData.DeclinedRoots) > 0 {
		uncheckedRoots = coverageData.DeclinedRoots
		status = "partial"
	}
	return lintInitCoverage{
		Status:           status,
		MultiRoot:        detection.MultiRoot,
		NeedsScopedSetup: detection.NeedsScopedSetup,
		CoveredRoots:     coveredRoots,
		UncheckedRoots:   uncheckedRoots,
		DeclinedRoots:    coverageData.DeclinedRoots,
		DeclinedAt:       coverageData.DeclinedAt,
	}
}

func lintInitRepoRootCommands(commands []lintconfig.Command) []lintconfig.Command {
	var rootCommands []lintconfig.Command
	for _, command := range commands {
		if command.RunDirectory == "." || lintInitContainsString(command.Paths, ".") {
			rootCommands = append(rootCommands, command)
		}
	}
	return rootCommands
}

func lintInitUncheckedRoots(detection backpressure.LintDetection, commands []lintconfig.Command) []string {
	coverage := lintInitDetectionCoverage(detection, commands, lintconfig.Coverage{})
	return coverage.UncheckedRoots
}

func lintInitNow(opts lintInitOptions) time.Time {
	if opts.now != nil {
		return opts.now().UTC()
	}
	return time.Now().UTC()
}

func lintInitScopedSetupDescription(result lintInitResult) string {
	var lines []string
	lines = append(lines, "Detected multiple lint roots.")
	for _, root := range result.Detection.GoRoots {
		lines = append(lines, "Go: "+root.Path)
	}
	for _, root := range result.Detection.NodeRoots {
		lines = append(lines, "Node/Astro: "+root.Path+" ("+root.PackageManager+")")
	}
	lines = append(lines, "Declining scoped setup records reduced coverage instead of blocking.")
	return strings.Join(lines, "\n")
}

func lintInitCommandDescription(command lintconfig.Command) string {
	return fmt.Sprintf("Command: %s\nRun directory: %s\nScope paths: %s\nRequired: %t\nTimeout seconds: %d\nSerial: %t",
		command.Command,
		command.RunDirectory,
		strings.Join(command.Paths, ", "),
		command.Required,
		command.TimeoutSeconds,
		command.Serial,
	)
}

func lintInitPreviewDescription(result lintInitResult, plan lintInitWizardPlan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Root: %s\n", result.Root)
	fmt.Fprintf(&b, "Coverage: %s\n", result.Coverage.Status)
	if len(plan.DeclinedRoots) > 0 {
		fmt.Fprintf(&b, "Declined roots: %s\n", strings.Join(plan.DeclinedRoots, ", "))
	}
	if len(result.ProposedCommands) == 0 {
		b.WriteString("Selected commands: none\n")
	} else {
		b.WriteString("Selected commands:\n")
		for _, command := range result.ProposedCommands {
			fmt.Fprintf(&b, "- %s: %s (run: %s, paths: %s, required: %t, timeout: %ds, serial: %t)\n",
				command.ID,
				command.Command,
				command.RunDirectory,
				strings.Join(command.Paths, ","),
				command.Required,
				command.TimeoutSeconds,
				command.Serial,
			)
		}
	}
	fmt.Fprintf(&b, "Manifest changed: %t\n", result.ManifestUpdate.Changed)
	if strings.TrimSpace(result.ManifestUpdate.Before) != "" {
		b.WriteString("Manifest before:\n")
		b.WriteString(result.ManifestUpdate.Before)
		if !strings.HasSuffix(result.ManifestUpdate.Before, "\n") {
			b.WriteString("\n")
		}
	}
	if strings.TrimSpace(result.ManifestUpdate.After) != "" {
		b.WriteString("Manifest after:\n")
		b.WriteString(result.ManifestUpdate.After)
		if !strings.HasSuffix(result.ManifestUpdate.After, "\n") {
			b.WriteString("\n")
		}
	}
	fmt.Fprintf(&b, "Lint rules changed: %t\n", result.LintRulesUpdate.Changed)
	if strings.TrimSpace(result.LintRulesUpdate.Before) != "" {
		b.WriteString("Recommendations before:\n")
		b.WriteString(result.LintRulesUpdate.Before)
		if !strings.HasSuffix(result.LintRulesUpdate.Before, "\n") {
			b.WriteString("\n")
		}
	}
	if strings.TrimSpace(result.LintRulesUpdate.After) != "" {
		b.WriteString("Recommendations after:\n")
		b.WriteString(result.LintRulesUpdate.After)
	}
	b.WriteString("\nDefault is No.")
	return b.String()
}

func splitLintInitCSV(value string) []string {
	var result []string
	for _, part := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func lintInitContainsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func lintInitFileUpdateFromResult(result backpressure.LintFileUpdateResult) lintInitFileUpdate {
	return lintInitFileUpdate{
		Path:    result.Path,
		Changed: result.Changed,
		Added:   result.Added,
		Updated: result.Updated,
		Renamed: result.Renamed,
		Skipped: result.Skipped,
		Before:  result.Before,
		After:   result.After,
	}
}

func writeLintInitResult(stdout io.Writer, stderr io.Writer, result lintInitResult, jsonOutput bool) error {
	if err := encodeJSON(stdout, result, "encode lint init result"); err != nil {
		return err
	}
	if jsonOutput {
		return nil
	}
	fmt.Fprintf(stderr, "Lint init: %s\n", result.Message)
	fmt.Fprintf(stderr, "Preset: %s\n", result.Preset)
	fmt.Fprintf(stderr, "Coverage: %s\n", result.Coverage.Status)
	if len(result.ProposedCommands) > 0 {
		fmt.Fprintf(stderr, "Proposed commands: %d\n", len(result.ProposedCommands))
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(stderr, "Warning: %s\n", warning)
	}
	return nil
}

func confirmLintInitMutation(opts lintInitOptions, result lintInitResult) error {
	if opts.force || opts.robotConfirm {
		return nil
	}
	if opts.jsonOutput {
		return fail(2, "lint init --json will not change files without confirmation; rerun with: burpvalve lint init --write --force --json")
	}
	if !isInteractiveTerminal(os.Stdin, os.Stdout) {
		return fail(2, "lint init --write requires confirmation before changing files; run in a terminal or rerun with: burpvalve lint init --write --force")
	}
	confirmed, err := charmui.AskConfirm(os.Stdin, os.Stdout, charmui.ConfirmPrompt{
		Title: "Confirm Burpvalve lint init",
		Description: fmt.Sprintf(
			"Target: %s\nManifest changed: %t\nLint rules changed: %t\nDefault is No.",
			result.Root,
			result.ManifestUpdate.Changed,
			result.LintRulesUpdate.Changed,
		),
		Prompt:  "Apply these changes?",
		Default: false,
		Color:   shouldColor(os.Stdout),
	})
	if err != nil {
		if strings.Contains(err.Error(), "cancel") {
			return fail(2, "lint init cancelled; no files changed")
		}
		return err
	}
	if !confirmed {
		return fail(2, "lint init cancelled; no files changed")
	}
	return nil
}
