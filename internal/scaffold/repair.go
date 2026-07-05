package scaffold

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"burpvalve/internal/cliui"
)

type RepairResult struct {
	SchemaVersion     int                   `json:"schema_version"`
	Command           string                `json:"command"`
	Status            string                `json:"status"`
	TargetRoot        string                `json:"target_root"`
	Config            *ConfigSummary        `json:"config,omitempty"`
	RepoLocalBinary   *RepoLocalBinaryFacts `json:"repo_local_binary,omitempty"`
	ClaudeRoute       string                `json:"claude_route,omitempty"`
	ClaudeRouteSource string                `json:"claude_route_source,omitempty"`
	Mutating          bool                  `json:"mutating"`
	Implemented       bool                  `json:"implemented"`
	Fatal             bool                  `json:"fatal"`
	PartialSuccess    bool                  `json:"partial_success"`
	NextSteps         []RecoveryStep        `json:"next_steps,omitempty"`
	Created           []string              `json:"created"`
	Repaired          []string              `json:"repaired"`
	Preserved         []string              `json:"preserved"`
	Conflicts         []ApplyConflict       `json:"conflicts"`
	Commands          []string              `json:"commands"`
	Checks            []Check               `json:"checks"`
	PlannedChanges    []PlannedChange       `json:"planned_changes"`
	Summary           Summary               `json:"summary"`
	Note              string                `json:"note"`
	Generated         time.Time             `json:"generated_at"`
}

func ApplyRepair(target string) (RepairResult, error) {
	return ApplyRepairWithOptions(target, ApplyOptions{})
}

func ApplyRepairWithOptions(target string, opts ApplyOptions) (RepairResult, error) {
	root, err := filepath.Abs(target)
	if err != nil {
		return RepairResult{}, err
	}
	if opts.Runner == nil {
		opts.Runner = execRunner{}
	}
	if opts.Looker == nil {
		opts.Looker = osLooker{}
	}
	result := RepairResult{
		SchemaVersion: 1,
		Command:       "repair",
		Status:        "applied",
		TargetRoot:    root,
		ClaudeRoute:   effectiveRepairClaudeRoute(root, opts),
		Mutating:      true,
		Implemented:   true,
		Note:          "safe repair mode created missing selected pieces and preserved project-owned content",
		Generated:     time.Now().UTC(),
	}
	skips := scaffoldSkips(opts)
	targets := effectiveScaffoldTargets(opts.Targets, skips)
	activeTargets := activeScaffoldTargets(opts.Targets, skips)

	if targets.has(TargetAgents) {
		if opts.SkipAgents {
			result.Preserved = append(result.Preserved, "AGENTS.md skipped (--no-agents)")
		} else if err := repairAgentsFile(root, opts, &result); err != nil {
			return result, err
		}
	}
	if err := repairGeneratedFiles(root, targets, opts, &result); err != nil {
		return result, err
	}
	if targets.has(TargetLog) {
		if err := repairWithApplyResult(&result, func(apply *ApplyResult) error {
			return ensureLocalGitignore(root, false, apply)
		}); err != nil {
			return result, err
		}
	}
	if targets.has(TargetPreCommit) {
		if opts.SkipPreCommit {
			result.Preserved = append(result.Preserved, ".githooks/pre-commit skipped (--no-precommit)")
		} else if err := repairWithApplyResult(&result, func(apply *ApplyResult) error {
			return ensurePreCommitHook(root, apply)
		}); err != nil {
			return result, err
		}
	}
	if targets.has(TargetTool) {
		if opts.SkipTool {
			result.Preserved = append(result.Preserved, "bin/burpvalve skipped (--no-bin)")
		} else if err := repairWithApplyResult(&result, func(apply *ApplyResult) error {
			return ensureBackpressureTool(root, opts.BackpressureToolPath, apply)
		}); err != nil {
			return result, err
		}
	}
	if targets.has(TargetHooksPath) {
		if err := repairWithApplyResult(&result, func(apply *ApplyResult) error {
			return ensureGitForHooks(root, opts, apply)
		}); err != nil {
			return result, err
		}
		if opts.SkipHooksPath {
			result.Preserved = append(result.Preserved, "git core.hooksPath skipped (--no-hooks-path)")
		} else if err := repairWithApplyResult(&result, func(apply *ApplyResult) error {
			return configureHooksPath(root, opts.Runner, opts.Looker, apply)
		}); err != nil {
			return result, err
		}
	}
	if targets.has(TargetClaude) {
		if opts.SkipClaude {
			result.Preserved = append(result.Preserved, "CLAUDE.md skipped (--no-claude)")
		} else if err := repairClaudeRoute(root, opts, &result); err != nil {
			return result, err
		}
	}
	if targets.has(TargetBeads) {
		if opts.SkipBeads {
			result.Preserved = append(result.Preserved, ".beads skipped (--no-beads)")
		} else if err := repairWithApplyResult(&result, func(apply *ApplyResult) error {
			return ensureBeads(root, opts.Runner, opts.Looker, apply)
		}); err != nil {
			return result, err
		}
	}
	report, err := Inspect(root, InspectOptions{
		Runner:              opts.Runner,
		Looker:              opts.Looker,
		Now:                 func() time.Time { return result.Generated },
		RequireOrchestrator: targets.has(TargetOrchestrator),
	})
	if err == nil {
		result.Checks = report.Checks
		result.PlannedChanges = filterRepairPlannedChanges(report.PlannedChanges, activeTargets)
		result.NextSteps = report.NextSteps
		result.Summary = report.Summary
		result.RepoLocalBinary = report.RepoLocalBinary
	}
	if len(result.Conflicts) > 0 {
		result.Status = "partial_success"
		result.Fatal = true
		result.PartialSuccess = len(result.Created) > 0 || len(result.Repaired) > 0 || len(result.Preserved) > 0 || len(result.Commands) > 0
		return result, fmt.Errorf("repair encountered %d conflict(s)", len(result.Conflicts))
	}
	return result, err
}

func PreviewRepair(target string, opts InspectOptions) (RepairResult, error) {
	report, err := Inspect(target, opts)
	if err != nil {
		return RepairResult{}, err
	}
	return RepairResult{
		SchemaVersion:     1,
		Command:           "repair",
		Status:            report.Status,
		TargetRoot:        report.TargetRoot,
		Mutating:          false,
		Implemented:       false,
		Fatal:             report.Fatal,
		PartialSuccess:    report.PartialSuccess,
		NextSteps:         report.NextSteps,
		Checks:            report.Checks,
		PlannedChanges:    report.PlannedChanges,
		Summary:           report.Summary,
		RepoLocalBinary:   report.RepoLocalBinary,
		ClaudeRoute:       reportClaudeRouteExpected(report),
		ClaudeRouteSource: reportClaudeRouteSource(report),
		Note:              "repair preview is non-mutating; run repair mode to apply safe scaffold repairs",
		Generated:         report.Generated,
	}, nil
}

func reportClaudeRouteExpected(report Report) string {
	if report.ClaudeRoute == nil {
		return ""
	}
	return report.ClaudeRoute.Expected
}

func reportClaudeRouteSource(report Report) string {
	if report.ClaudeRoute == nil {
		return ""
	}
	return report.ClaudeRoute.Source
}

func (r RepairResult) Text() string {
	return r.TextWithOptions(TextOptions{})
}

func (r RepairResult) TextWithOptions(opts TextOptions) string {
	var b strings.Builder
	title := "Repair result"
	if !r.Mutating && !r.Implemented {
		title = "Repair preview"
	}
	ui := cliui.New(opts.Color)
	fmt.Fprintf(&b, "%s %s\n", ui.Title(title+" for"), ui.Path(r.TargetRoot))
	writeTwoColumnSection(&b, "summary", "field", "value", []reportRow{
		{Left: "mutating", Right: ui.Bool(r.Mutating)},
		{Left: "implemented", Right: ui.Bool(r.Implemented)},
		{Left: "claude route", Right: emptyFallback(r.ClaudeRoute, "none")},
		{Left: "route source", Right: emptyFallback(r.ClaudeRouteSource, "unknown")},
		{Left: "ready", Right: ui.Bool(r.Summary.Ready)},
		{Left: "missing", Right: fmt.Sprint(r.Summary.Missing)},
		{Left: "unavailable", Right: fmt.Sprint(r.Summary.Unavailable)},
		{Left: "conflicts", Right: fmt.Sprint(r.Summary.Conflicts)},
		{Left: "errors", Right: fmt.Sprint(r.Summary.Errors)},
		{Left: "note", Right: r.Note},
	}, opts)
	if r.Config != nil {
		writeConfigSummary(&b, *r.Config, opts)
	}
	if r.RepoLocalBinary != nil {
		writeRepoLocalBinaryFacts(&b, *r.RepoLocalBinary, opts)
	}
	writeListWithOptions(&b, "created", r.Created, opts)
	writeListWithOptions(&b, "repaired", r.Repaired, opts)
	writeListWithOptions(&b, "preserved", r.Preserved, opts)
	writeListWithOptions(&b, "commands", r.Commands, opts)
	if len(r.Conflicts) > 0 {
		var conflicts []reportRow
		for _, conflict := range r.Conflicts {
			conflicts = append(conflicts, reportRow{Left: ui.Error(conflict.Path), Right: conflict.Message})
		}
		writeTwoColumnSection(&b, "conflicts", "path", "message", conflicts, opts)
	}
	if len(r.PlannedChanges) == 0 {
		writeTwoColumnSection(&b, "planned repair changes", "target", "change", []reportRow{
			{Left: "-", Right: "none"},
		}, opts)
		return b.String()
	}
	var planned []reportRow
	for _, change := range r.PlannedChanges {
		target := change.ID
		if change.Path != "" {
			target = change.Path
		}
		right := change.Action + ui.Muted(" ("+change.Reason)
		if change.Requires != "" {
			right += ui.Muted("; requires " + change.Requires)
		}
		right += ui.Muted(")")
		planned = append(planned, reportRow{Left: ui.Path(target), Right: right})
	}
	writeTwoColumnSection(&b, "planned repair changes", "target", "change", planned, opts)
	return b.String()
}

const (
	agentsRepairStart = "<!-- burpvalve:repair-start -->"
	agentsRepairEnd   = "<!-- burpvalve:repair-end -->"
	claudeImportStart = "<!-- burpvalve:claude-import-start -->"
	claudeImportEnd   = "<!-- burpvalve:claude-import-end -->"
)

func repairAgentsFile(root string, opts ApplyOptions, result *RepairResult) error {
	const rel = "AGENTS.md"
	path := filepath.Join(root, rel)
	templateBody, err := renderAgentsTemplate(opts)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return repairWithApplyResult(result, func(apply *ApplyResult) error {
			return createFileIfMissing(root, rel, templateBody, 0o644, apply)
		})
	}
	if info.IsDir() {
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: rel, Message: "path exists as a directory"})
		return nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		result.Preserved = append(result.Preserved, rel)
		return nil
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	missing := missingAgentsSections(string(current), orderedAgentsRepairHeadings(opts))
	if len(missing) == 0 {
		result.Preserved = append(result.Preserved, rel)
		return nil
	}
	block, err := buildAgentsRepairBlock(string(templateBody), missing)
	if err != nil {
		return err
	}
	next := strings.TrimRight(string(current), "\n") + "\n\n" + block
	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	if err := os.WriteFile(path, []byte(next), mode); err != nil {
		return err
	}
	result.Repaired = append(result.Repaired, rel+" sections: "+strings.Join(missing, ", "))
	return nil
}

func repairClaudeLink(root string, opts ApplyOptions, result *RepairResult) error {
	return repairClaudeRouteAs(root, ClaudeRouteAgentSymlink, opts, result)
}

func repairClaudeRoute(root string, opts ApplyOptions, result *RepairResult) error {
	route := effectiveRepairClaudeRoute(root, opts)
	if route == ClaudeRouteNone {
		result.Preserved = append(result.Preserved, "Claude route disabled (claude_route=none)")
		return nil
	}
	return repairClaudeRouteAs(root, route, opts, result)
}

func effectiveRepairClaudeRoute(root string, opts ApplyOptions) string {
	route := strings.TrimSpace(opts.ClaudeRoute)
	if route != "" && route != ClaudeRepairRoutePreserve {
		return route
	}
	facts := inspectClaudeRoute(root, ClaudeRepairRoutePreserve)
	switch facts.Detected {
	case ClaudeRouteAgentSymlink, ClaudeRouteOrchestratorSkill:
		return facts.Detected
	default:
		return ClaudeRouteAgentSymlink
	}
}

func repairClaudeRouteAs(root, route string, opts ApplyOptions, result *RepairResult) error {
	if err := ensureAgentsForClaudeRepair(root, opts, result); err != nil {
		return err
	}
	if len(result.Conflicts) > 0 {
		return nil
	}
	switch route {
	case ClaudeRouteAgentSymlink:
		return repairClaudeLinkRoute(root, opts, result)
	case ClaudeRouteOrchestratorSkill:
		return repairClaudeOrchestratorRoute(root, opts, result)
	default:
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: "CLAUDE.md", Message: "unknown Claude route " + route})
		return nil
	}
}

func ensureAgentsForClaudeRepair(root string, opts ApplyOptions, result *RepairResult) error {
	if opts.SkipAgents && !fileExists(root, "AGENTS.md") {
		result.Conflicts = append(result.Conflicts, ApplyConflict{
			Path:    "AGENTS.md",
			Message: "active Claude route requires AGENTS.md; run `burpvalve repair AGENTS.md` first, omit --no-agents, or set claude_route=none",
		})
		return nil
	}
	if opts.SkipAgents {
		result.Preserved = append(result.Preserved, "AGENTS.md used (--no-agents)")
		return nil
	}
	return repairAgentsFile(root, opts, result)
}

func repairClaudeLinkRoute(root string, opts ApplyOptions, result *RepairResult) error {
	const rel = "CLAUDE.md"
	path := filepath.Join(root, rel)
	info, err := os.Lstat(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return repairWithApplyResult(result, func(apply *ApplyResult) error {
			return ensureClaudeLink(root, apply)
		})
	}
	if info.IsDir() {
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: rel, Message: "path exists as a directory"})
		return nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return repairWithApplyResult(result, func(apply *ApplyResult) error {
			return ensureClaudeLink(root, apply)
		})
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if isGeneratedClaudeOrchestratorBootstrap(current) {
		if err := os.Remove(path); err != nil {
			return err
		}
		if err := os.Symlink("AGENTS.md", path); err != nil {
			return err
		}
		result.Repaired = append(result.Repaired, "CLAUDE.md symlink")
		return nil
	}
	if !opts.AdoptClaude {
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: rel, Message: "regular file exists; rerun repair with explicit Claude adoption before replacing"})
		return nil
	}
	conflictsBefore := len(result.Conflicts)
	if err := importClaudeIntoAgents(root, current, result); err != nil {
		return err
	}
	if len(result.Conflicts) > conflictsBefore {
		return nil
	}
	backup := filepath.Join(root, ".CLAUDE.md.burpvalve-repair-backup")
	if _, err := os.Lstat(backup); err == nil {
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: ".CLAUDE.md.burpvalve-repair-backup", Message: "backup path exists; remove or review it before repairing CLAUDE.md"})
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(path, backup); err != nil {
		return err
	}
	if err := os.Symlink("AGENTS.md", path); err != nil {
		_ = os.Rename(backup, path)
		return err
	}
	if err := os.Remove(backup); err != nil {
		return err
	}
	result.Repaired = append(result.Repaired, "CLAUDE.md symlink")
	return nil
}

func repairClaudeOrchestratorRoute(root string, opts ApplyOptions, result *RepairResult) error {
	const rel = "CLAUDE.md"
	path := filepath.Join(root, rel)
	info, err := os.Lstat(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return repairWithApplyResult(result, func(apply *ApplyResult) error {
			if err := ensureClaudeOrchestratorBootstrap(root, true, apply); err != nil {
				return err
			}
			return ensureClaudeOrchestratorSkillPackage(root, true, apply)
		})
	}
	if info.IsDir() {
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: rel, Message: "path exists as a directory"})
		return nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		dest, readErr := os.Readlink(path)
		if readErr != nil {
			return readErr
		}
		if dest != "AGENTS.md" {
			result.Conflicts = append(result.Conflicts, ApplyConflict{Path: rel, Message: "symlink points to " + dest + ", not AGENTS.md"})
			return nil
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		body, err := embeddedTemplates.ReadFile("templates/CLAUDE.md.orchestrator.tmpl")
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, body, 0o644); err != nil {
			return err
		}
		result.Repaired = append(result.Repaired, "CLAUDE.md orchestrator bootstrap")
		return repairWithApplyResult(result, func(apply *ApplyResult) error {
			return ensureClaudeOrchestratorSkillPackage(root, true, apply)
		})
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !isGeneratedClaudeOrchestratorBootstrap(current) {
		if !opts.AdoptClaude {
			result.Conflicts = append(result.Conflicts, ApplyConflict{Path: rel, Message: "regular file exists; rerun repair with explicit Claude adoption before replacing"})
			return nil
		}
		conflictsBefore := len(result.Conflicts)
		if err := importClaudeIntoAgents(root, current, result); err != nil {
			return err
		}
		if len(result.Conflicts) > conflictsBefore {
			return nil
		}
	}
	return repairWithApplyResult(result, func(apply *ApplyResult) error {
		if err := ensureClaudeOrchestratorBootstrap(root, true, apply); err != nil {
			return err
		}
		return ensureClaudeOrchestratorSkillPackage(root, true, apply)
	})
}

func importClaudeIntoAgents(root string, claudeBody []byte, result *RepairResult) error {
	if strings.TrimSpace(string(claudeBody)) == "" {
		result.Preserved = append(result.Preserved, "CLAUDE.md empty before symlink")
		return nil
	}
	const rel = "AGENTS.md"
	path := filepath.Join(root, rel)
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: rel, Message: "path exists as a directory"})
		return nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: rel, Message: "AGENTS.md is a symlink; explicit review required before importing CLAUDE.md content"})
		return nil
	}
	agentsBody, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if strings.Contains(string(agentsBody), claudeImportStart) {
		result.Preserved = append(result.Preserved, "CLAUDE.md content already imported")
		return nil
	}
	next := strings.TrimRight(string(agentsBody), "\n") +
		"\n\n" + claudeImportStart + "\n" +
		"## Imported CLAUDE.md Notes\n\n" +
		"The content below came from CLAUDE.md before burpvalve repair replaced it with a symlink to AGENTS.md.\n\n" +
		strings.TrimRight(string(claudeBody), "\n") + "\n" +
		claudeImportEnd + "\n"
	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}
	if err := os.WriteFile(path, []byte(next), mode); err != nil {
		return err
	}
	result.Repaired = append(result.Repaired, "AGENTS.md imported CLAUDE.md content")
	return nil
}

func repairGeneratedFiles(root string, targets scaffoldTargetSet, opts ApplyOptions, result *RepairResult) error {
	for _, item := range scaffoldTemplates {
		if item.Target == "AGENTS.md" || item.Target == ".githooks/pre-commit" {
			continue
		}
		target, ok := scaffoldTargetForTemplate(item.Target)
		if !ok || !targets.has(target) {
			continue
		}
		if repairTemplateSkipped(target, item.Target, opts, result) {
			continue
		}
		body, err := embeddedTemplates.ReadFile(item.Source)
		if err != nil {
			return err
		}
		if err := repairWithApplyResult(result, func(apply *ApplyResult) error {
			return createFileIfMissing(root, item.Target, body, item.Mode, apply)
		}); err != nil {
			return err
		}
	}
	return nil
}

func repairTemplateSkipped(target ScaffoldTarget, rel string, opts ApplyOptions, result *RepairResult) bool {
	switch target {
	case TargetDocs:
		if opts.SkipDocs {
			result.Preserved = append(result.Preserved, rel+" skipped (--no-docs)")
			return true
		}
	case TargetPlans:
		if opts.SkipPlans {
			result.Preserved = append(result.Preserved, rel+" skipped (--no-plans)")
			return true
		}
	case TargetLog:
		if opts.SkipLog {
			result.Preserved = append(result.Preserved, rel+" skipped (--no-log)")
			return true
		}
	case TargetBackpressure:
		if opts.SkipBackpressure {
			result.Preserved = append(result.Preserved, rel+" skipped (--no-backpressure)")
			return true
		}
	case TargetAttestations:
		if opts.SkipAttestations {
			result.Preserved = append(result.Preserved, rel+" skipped (--no-attestations)")
			return true
		}
	case TargetToolDocs:
		if opts.SkipToolDocs {
			result.Preserved = append(result.Preserved, rel+" skipped (--no-tool-docs)")
			return true
		}
	}
	return false
}

func filterRepairPlannedChanges(plans []PlannedChange, targets scaffoldTargetSet) []PlannedChange {
	var filtered []PlannedChange
	for _, plan := range plans {
		target, ok := repairTargetForPlannedChange(plan)
		if !ok || targets.has(target) {
			filtered = append(filtered, plan)
		}
	}
	return filtered
}

func repairTargetForPlannedChange(plan PlannedChange) (ScaffoldTarget, bool) {
	switch plan.ID {
	case "agents":
		return TargetAgents, true
	case "claude":
		return TargetClaude, true
	case "docs":
		return TargetDocs, true
	case "plans":
		return TargetPlans, true
	case "log":
		return TargetLog, true
	case "backpressure":
		return TargetBackpressure, true
	case "backpressure-attestations":
		return TargetAttestations, true
	case "beads", "br-tool":
		return TargetBeads, true
	case "githook":
		return TargetPreCommit, true
	case "git-hooks-path":
		return TargetHooksPath, true
	case "backpressure-tool":
		return TargetTool, true
	default:
		target, ok := scaffoldTargetForTemplate(plan.Path)
		return target, ok
	}
}

func missingAgentsSections(body string, required []string) []string {
	var missing []string
	for _, heading := range required {
		if !hasMarkdownHeading(body, heading) {
			missing = append(missing, heading)
		}
	}
	return missing
}

func orderedAgentsRepairHeadings(opts ApplyOptions) []string {
	optional := map[string]bool{}
	for _, heading := range agentsRepairHeadings(opts) {
		optional[heading] = true
	}
	headings := []string{"Agent Startup"}
	if optional["Beads"] {
		headings = append(headings, "Beads")
	}
	headings = append(headings, "Atomic Work And Commits")
	if optional["NTM Session Naming"] {
		headings = append(headings, "NTM Session Naming")
	}
	if optional["Backpressure"] {
		headings = append(headings, "Backpressure")
	}
	headings = append(headings, "Definition Of Done")
	if optional["Docs, Plans, And Logs"] {
		headings = append(headings, "Docs, Plans, And Logs")
	}
	headings = append(headings, "Uncertainty", "File Coordination")
	return headings
}

func hasMarkdownHeading(body, heading string) bool {
	needle := "## " + heading
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == needle {
			return true
		}
	}
	return false
}

func buildAgentsRepairBlock(templateBody string, headings []string) (string, error) {
	var b strings.Builder
	b.WriteString(agentsRepairStart)
	b.WriteString("\n")
	b.WriteString("The following standard sections were appended by burpvalve repair. Preserve project-specific content above this block.\n\n")
	for i, heading := range headings {
		section, ok := extractMarkdownSection(templateBody, heading)
		if !ok {
			return "", fmt.Errorf("AGENTS.md template missing section %q", heading)
		}
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(section)
		b.WriteString("\n")
	}
	b.WriteString(agentsRepairEnd)
	b.WriteString("\n")
	return b.String(), nil
}

func extractMarkdownSection(body, heading string) (string, bool) {
	lines := strings.Split(body, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "## "+heading {
			start = i
			break
		}
	}
	if start == -1 {
		return "", false
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "## ") {
			end = i
			break
		}
	}
	return strings.TrimRight(strings.Join(lines[start:end], "\n"), "\n"), true
}

func repairWithApplyResult(result *RepairResult, fn func(*ApplyResult) error) error {
	var apply ApplyResult
	if err := fn(&apply); err != nil {
		mergeApplyIntoRepair(result, apply)
		return err
	}
	mergeApplyIntoRepair(result, apply)
	return nil
}

func mergeApplyIntoRepair(result *RepairResult, apply ApplyResult) {
	for _, created := range apply.Created {
		if strings.Contains(created, ".githooks/pre-commit") {
			result.Repaired = append(result.Repaired, created)
			continue
		}
		result.Created = append(result.Created, created)
	}
	result.Repaired = append(result.Repaired, apply.Repaired...)
	result.Preserved = append(result.Preserved, apply.Preserved...)
	result.Conflicts = append(result.Conflicts, apply.Conflicts...)
	result.Commands = append(result.Commands, apply.Commands...)
}
