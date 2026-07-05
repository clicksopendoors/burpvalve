package scaffold

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"burpvalve/internal/cliui"
	"burpvalve/internal/ntm"
)

type CheckStatus string

const (
	StatusPresent     CheckStatus = "present"
	StatusMissing     CheckStatus = "missing"
	StatusUnavailable CheckStatus = "unavailable"
	StatusOK          CheckStatus = "ok"
	StatusDirty       CheckStatus = "dirty"
	StatusClean       CheckStatus = "clean"
	StatusConflict    CheckStatus = "conflict"
	StatusUnknown     CheckStatus = "unknown"
	StatusError       CheckStatus = "error"
)

type Report struct {
	SchemaVersion     int                   `json:"schema_version"`
	Command           string                `json:"command"`
	Status            string                `json:"status"`
	ReadinessSeverity string                `json:"readiness_severity"`
	TargetRoot        string                `json:"target_root"`
	CommandPath       string                `json:"command_path"`
	RepoBinPath       string                `json:"repo_bin_path"`
	HookCommandSource string                `json:"hook_command_source"`
	RepoLocalBinary   *RepoLocalBinaryFacts `json:"repo_local_binary,omitempty"`
	ClaudeRoute       *ClaudeRouteFacts     `json:"claude_route,omitempty"`
	Config            *ConfigSummary        `json:"config,omitempty"`
	Checks            []Check               `json:"checks"`
	PlannedChanges    []PlannedChange       `json:"planned_changes"`
	NextSteps         []RecoveryStep        `json:"next_steps"`
	Mutating          bool                  `json:"mutating"`
	Fatal             bool                  `json:"fatal"`
	PartialSuccess    bool                  `json:"partial_success"`
	Summary           Summary               `json:"summary"`
	Generated         time.Time             `json:"generated_at"`
}

type RecoveryStep struct {
	ID      string `json:"id"`
	Message string `json:"message"`
	Command string `json:"command,omitempty"`
	Fatal   bool   `json:"fatal"`
}

type Summary struct {
	Ready       bool `json:"ready"`
	Errors      int  `json:"errors"`
	Missing     int  `json:"missing"`
	Unavailable int  `json:"unavailable"`
	Conflicts   int  `json:"conflicts"`
}

type RepoLocalBinaryFacts struct {
	HookCommandSource string `json:"hook_command_source"`
	RepoLocalPath     string `json:"repo_local_path"`
	RepoLocalExists   bool   `json:"repo_local_exists"`
	RepoLocalIgnored  bool   `json:"repo_local_ignored"`
	PathCommand       string `json:"path_command"`
	FreshnessStatus   string `json:"freshness_status"`
	ComparisonBasis   string `json:"comparison_basis"`
	WarningCode       string `json:"warning_code"`
}

type ClaudeRouteFacts struct {
	Expected      string   `json:"expected"`
	Source        string   `json:"source,omitempty"`
	Detected      string   `json:"detected"`
	MissingPieces []string `json:"missing_pieces,omitempty"`
	Drift         []string `json:"drift,omitempty"`
	Conflicts     []string `json:"conflicts,omitempty"`
}

type ConfigSummary struct {
	GlobalPath   string          `json:"global_path"`
	GlobalFound  bool            `json:"global_found"`
	ProjectPath  string          `json:"project_path"`
	ProjectFound bool            `json:"project_found"`
	Sources      []ConfigSource  `json:"sources,omitempty"`
	Settings     []ConfigSetting `json:"settings,omitempty"`
}

type ConfigSource struct {
	Key    string `json:"key"`
	Source string `json:"source"`
}

type ConfigSetting struct {
	Key    string `json:"key"`
	Source string `json:"source"`
	Value  string `json:"value"`
}

type Check struct {
	ID       string      `json:"id"`
	Status   CheckStatus `json:"status"`
	Path     string      `json:"path,omitempty"`
	Required bool        `json:"required"`
	Message  string      `json:"message"`
	Detail   string      `json:"detail,omitempty"`
}

type PlannedChange struct {
	ID       string `json:"id"`
	Action   string `json:"action"`
	Path     string `json:"path,omitempty"`
	Reason   string `json:"reason"`
	Requires string `json:"requires,omitempty"`
}

type InspectOptions struct {
	Runner              Runner
	Looker              Looker
	Now                 func() time.Time
	SkipAgents          bool
	SkipClaude          bool
	SkipDocs            bool
	SkipPlans           bool
	SkipLog             bool
	SkipBackpressure    bool
	SkipAttestations    bool
	SkipBeads           bool
	SkipPreCommit       bool
	SkipHooksPath       bool
	RequireOrchestrator bool
	RequireRepoBin      bool
	ClaudeRoute         string
	SkipNTM             bool
}

type Runner interface {
	Run(ctx context.Context, dir string, name string, args ...string) (CommandResult, error)
}

type Looker interface {
	LookPath(file string) (string, error)
}

type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, dir string, name string, args ...string) (CommandResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := CommandResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if err == nil {
		return result, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, err
	}
	result.ExitCode = -1
	return result, err
}

type osLooker struct{}

func (osLooker) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

type inspectProbe struct {
	run func() []Check
}

func runInspectProbes(probes []inspectProbe) [][]Check {
	results := make([][]Check, len(probes))
	if len(probes) == 0 {
		return results
	}
	limit := len(probes)
	if limit > 4 {
		limit = 4
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i, probe := range probes {
		i, probe := i, probe
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = probe.run()
		}()
	}
	wg.Wait()
	return results
}

// Inspect builds a non-mutating readiness report for a target project root.
func Inspect(target string, opts InspectOptions) (Report, error) {
	root, err := filepath.Abs(target)
	if err != nil {
		return Report{}, err
	}
	if opts.Runner == nil {
		opts.Runner = execRunner{}
	}
	if opts.Looker == nil {
		opts.Looker = osLooker{}
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}

	report := Report{
		SchemaVersion: 1,
		Command:       "setup",
		TargetRoot:    root,
		Mutating:      false,
		Generated:     opts.Now().UTC(),
	}

	add := func(check Check) {
		report.Checks = append(report.Checks, check)
	}

	if !opts.SkipAgents {
		add(pathCheck(root, "agents", "AGENTS.md", true, "repo-local operating contract"))
	}
	if !opts.SkipClaude {
		route := effectiveInspectClaudeRoute(opts)
		report.ClaudeRoute = inspectClaudeRoute(root, route)
		add(claudeCheck(route, *report.ClaudeRoute))
	}
	if opts.RequireOrchestrator {
		add(pathCheck(root, "orchestrator", "ORCHESTRATOR.md", true, "repo-local orchestrator operating notes"))
	}
	if !opts.SkipDocs {
		add(pathCheck(root, "docs", "docs", true, "durable project knowledge directory"))
	}
	if !opts.SkipPlans {
		add(pathCheck(root, "plans", "plans", true, "implementation and strategy plans directory"))
	}
	if !opts.SkipLog {
		add(pathCheck(root, "log", "log", true, "agent/human work log directory"))
	}
	if !opts.SkipBackpressure {
		add(pathCheck(root, "backpressure", "backpressure", true, "backpressure condition directory"))
	}
	if !opts.SkipAttestations {
		add(pathCheck(root, "backpressure-attestations", "backpressure/attestations", true, "tracked passing attestation directory"))
	}
	if !opts.SkipBeads {
		add(pathCheck(root, "beads", ".beads", true, "local-first issue tracker state"))
		add(executableCheck("br", true, "br executable for local-first issue tracking", opts.Looker))
	}
	if !opts.SkipPreCommit {
		add(hookFileCheck(root))
	}
	preGitProbes := []inspectProbe{}
	if !opts.SkipHooksPath {
		preGitProbes = append(preGitProbes, inspectProbe{run: func() []Check {
			return []Check{gitHooksPathCheck(root, opts.Runner, opts.Looker)}
		}})
	}
	preGitProbes = append(preGitProbes, inspectProbe{run: func() []Check {
		toolCheck := burpvalveToolCheck(root, opts.Runner, opts.Looker)
		checks := []Check{toolCheck}
		if opts.RequireRepoBin || toolCheck.Status == StatusOK {
			if check, ok := repoLocalBurpvalveFallbackCheck(root, opts.Runner, opts.Looker); ok {
				if opts.RequireRepoBin {
					check.Required = true
					if check.ID == "backpressure-tool-fallback" && check.Status == StatusMissing {
						check.Message = "configured repo-local burpvalve fallback is missing"
					}
				}
				checks = append(checks, check)
			} else if opts.RequireRepoBin {
				check := repoLocalToolCheck(root)
				check.ID = "backpressure-tool-fallback"
				check.Required = true
				if check.Status == StatusMissing {
					check.Message = "configured repo-local burpvalve fallback is missing"
				}
				checks = append(checks, check)
			}
		}
		return checks
	}})
	for _, checks := range runInspectProbes(preGitProbes) {
		for _, check := range checks {
			add(check)
		}
	}
	add(gitRepoCheck(root))
	postGitProbes := []inspectProbe{{
		run: func() []Check {
			return []Check{gitDirtyCheck(root, opts.Runner, opts.Looker)}
		},
	}}
	if !opts.SkipNTM {
		postGitProbes = append(postGitProbes, inspectProbe{run: func() []Check {
			return []Check{ntmCheck(root, opts.Runner, opts.Looker)}
		}})
	}
	for _, checks := range runInspectProbes(postGitProbes) {
		for _, check := range checks {
			add(check)
		}
	}

	populateReadinessFacts(&report)
	report.RepoLocalBinary = repoLocalBinaryFacts(root, report, opts.Runner, opts.Looker)
	report.PlannedChanges = plannedChanges(report.Checks)
	report.Summary = summarize(report.Checks)
	report.Status = "ready"
	if !report.Summary.Ready {
		report.Status = "blocked"
	}
	report.Fatal = hasFatalSetupBlocker(report.Checks)
	report.PartialSuccess = !report.Summary.Ready && len(report.Checks) > report.Summary.Errors+report.Summary.Missing+report.Summary.Unavailable+report.Summary.Conflicts
	report.NextSteps = recoverySteps(report.Checks, report.PlannedChanges)
	report.NextSteps = append(report.NextSteps, repoLocalBinaryRecoveryStep(report)...)
	report.ReadinessSeverity = readinessSeverity(&report)
	return report, nil
}

func executableCheck(name string, required bool, message string, looker Looker) Check {
	if _, err := looker.LookPath(name); err != nil {
		return Check{ID: name + "-tool", Status: StatusUnavailable, Required: required, Message: message + " unavailable"}
	}
	return Check{ID: name + "-tool", Status: StatusOK, Required: required, Message: message + " available"}
}

func pathCheck(root, id, rel string, required bool, message string) Check {
	path := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(path)
	if err != nil {
		status := StatusMissing
		if !os.IsNotExist(err) {
			status = StatusError
			message = err.Error()
		}
		return Check{ID: id, Status: status, Path: rel, Required: required, Message: message}
	}
	if info.IsDir() || info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return Check{ID: id, Status: StatusPresent, Path: rel, Required: required, Message: message}
	}
	return Check{ID: id, Status: StatusUnknown, Path: rel, Required: required, Message: "path exists with unsupported file type"}
}

func effectiveInspectClaudeRoute(opts InspectOptions) string {
	route := strings.TrimSpace(opts.ClaudeRoute)
	switch route {
	case "", ClaudeRouteAgentSymlink:
		return ClaudeRouteAgentSymlink
	case ClaudeRouteOrchestratorSkill, ClaudeRouteNone:
		return route
	default:
		return route
	}
}

func inspectClaudeRoute(root, expected string) *ClaudeRouteFacts {
	facts := &ClaudeRouteFacts{Expected: expected, Detected: ClaudeRouteNone}
	detected, claudeMissing, claudeDrift, claudeConflicts := detectClaudeFileRoute(root)
	facts.Detected = detected
	facts.MissingPieces = append(facts.MissingPieces, claudeMissing...)
	facts.Drift = append(facts.Drift, claudeDrift...)
	facts.Conflicts = append(facts.Conflicts, claudeConflicts...)
	if expected == ClaudeRouteOrchestratorSkill || detected == ClaudeRouteOrchestratorSkill {
		missing, drift, conflicts := inspectClaudeSkillPackage(root)
		facts.MissingPieces = append(facts.MissingPieces, missing...)
		facts.Drift = append(facts.Drift, drift...)
		facts.Conflicts = append(facts.Conflicts, conflicts...)
	}
	return facts
}

func detectClaudeFileRoute(root string) (route string, missing []string, drift []string, conflicts []string) {
	rel := "CLAUDE.md"
	path := filepath.Join(root, rel)
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ClaudeRouteNone, []string{rel}, nil, nil
		}
		return ClaudeRouteNone, nil, nil, []string{rel + ": " + err.Error()}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		dest, err := os.Readlink(path)
		if err != nil {
			return ClaudeRouteNone, nil, nil, []string{rel + ": " + err.Error()}
		}
		if dest == "AGENTS.md" {
			return ClaudeRouteAgentSymlink, nil, nil, nil
		}
		return ClaudeRouteNone, nil, nil, []string{"CLAUDE.md symlink points to " + dest + ", not AGENTS.md"}
	}
	if info.Mode().IsRegular() {
		body, err := os.ReadFile(path)
		if err != nil {
			return ClaudeRouteNone, nil, nil, []string{rel + ": " + err.Error()}
		}
		want, err := embeddedTemplates.ReadFile("templates/CLAUDE.md.orchestrator.tmpl")
		if err != nil {
			return ClaudeRouteNone, nil, nil, []string{"embedded CLAUDE.md.orchestrator.tmpl: " + err.Error()}
		}
		if string(body) == string(want) {
			return ClaudeRouteOrchestratorSkill, nil, nil, nil
		}
		if isGeneratedClaudeOrchestratorBootstrap(body) {
			return ClaudeRouteOrchestratorSkill, nil, []string{rel}, nil
		}
		return ClaudeRouteNone, nil, nil, []string{"CLAUDE.md is an unmarked regular file"}
	}
	return ClaudeRouteNone, nil, nil, []string{"CLAUDE.md exists with unsupported file type"}
}

func inspectClaudeSkillPackage(root string) (missing []string, drift []string, conflicts []string) {
	files, err := claudeOrchestratorSkillTemplates()
	if err != nil {
		return nil, nil, []string{"embedded orchestrator skill templates: " + err.Error()}
	}
	for _, item := range files {
		path := filepath.Join(root, filepath.FromSlash(item.Target))
		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				missing = append(missing, item.Target)
				continue
			}
			conflicts = append(conflicts, item.Target+": "+err.Error())
			continue
		}
		if info.IsDir() {
			conflicts = append(conflicts, item.Target+" is a directory")
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			conflicts = append(conflicts, item.Target+": "+err.Error())
			continue
		}
		want, err := embeddedTemplates.ReadFile(item.Source)
		if err != nil {
			conflicts = append(conflicts, item.Source+": "+err.Error())
			continue
		}
		if string(body) != string(want) {
			drift = append(drift, item.Target)
		}
	}
	return missing, drift, conflicts
}

func claudeCheck(expected string, facts ClaudeRouteFacts) Check {
	const rel = "CLAUDE.md"
	required := expected != ClaudeRouteNone
	if len(facts.Conflicts) > 0 {
		return Check{ID: "claude", Status: StatusConflict, Path: rel, Required: true, Message: strings.Join(facts.Conflicts, "; "), Detail: claudeRouteDetail(facts)}
	}
	if expected == ClaudeRouteNone {
		if facts.Detected == ClaudeRouteNone {
			return Check{ID: "claude", Status: StatusPresent, Required: false, Message: "Claude route disabled", Detail: claudeRouteDetail(facts)}
		}
		return Check{ID: "claude", Status: StatusConflict, Path: rel, Required: true, Message: "Claude route exists but expected route is none", Detail: claudeRouteDetail(facts)}
	}
	if facts.Detected == ClaudeRouteNone || len(facts.MissingPieces) > 0 {
		return Check{ID: "claude", Status: StatusMissing, Path: rel, Required: required, Message: "Claude route pieces are missing", Detail: claudeRouteDetail(facts)}
	}
	if facts.Detected != expected {
		return Check{ID: "claude", Status: StatusConflict, Path: rel, Required: true, Message: "detected Claude route " + facts.Detected + " but expected " + expected, Detail: claudeRouteDetail(facts)}
	}
	if len(facts.Drift) > 0 {
		return Check{ID: "claude", Status: StatusConflict, Path: rel, Required: true, Message: "Claude route generated files drifted", Detail: claudeRouteDetail(facts)}
	}
	switch expected {
	case ClaudeRouteOrchestratorSkill:
		return Check{ID: "claude", Status: StatusPresent, Path: rel, Required: required, Message: "CLAUDE.md uses generated orchestrator skill route", Detail: claudeRouteDetail(facts)}
	default:
		return Check{ID: "claude", Status: StatusPresent, Path: rel, Required: required, Message: "CLAUDE.md symlinks to AGENTS.md", Detail: claudeRouteDetail(facts)}
	}
}

func claudeRouteDetail(facts ClaudeRouteFacts) string {
	parts := []string{"expected=" + facts.Expected, "detected=" + facts.Detected}
	if facts.Source != "" {
		parts = append(parts, "source="+facts.Source)
	}
	if len(facts.MissingPieces) > 0 {
		parts = append(parts, "missing="+strings.Join(facts.MissingPieces, ","))
	}
	if len(facts.Drift) > 0 {
		parts = append(parts, "drift="+strings.Join(facts.Drift, ","))
	}
	if len(facts.Conflicts) > 0 {
		parts = append(parts, "conflicts="+strings.Join(facts.Conflicts, ","))
	}
	return strings.Join(parts, "; ")
}

func gitRepoCheck(root string) Check {
	path := filepath.Join(root, ".git")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return Check{ID: "git-repo", Status: StatusMissing, Path: ".git", Required: true, Message: "target is not a git repository"}
		}
		return Check{ID: "git-repo", Status: StatusError, Path: ".git", Required: true, Message: err.Error()}
	}
	return Check{ID: "git-repo", Status: StatusPresent, Path: ".git", Required: true, Message: "target is a git repository"}
}

func hookFileCheck(root string) Check {
	rel := ".githooks/pre-commit"
	path := filepath.Join(root, rel)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Check{ID: "githook", Status: StatusMissing, Path: rel, Required: true, Message: "repo-local pre-commit hook"}
		}
		return Check{ID: "githook", Status: StatusError, Path: rel, Required: true, Message: err.Error()}
	}
	if info.IsDir() {
		return Check{ID: "githook", Status: StatusConflict, Path: rel, Required: true, Message: "pre-commit hook path is a directory"}
	}
	if info.Mode()&0o111 == 0 {
		return Check{ID: "githook", Status: StatusConflict, Path: rel, Required: true, Message: "pre-commit hook exists but is not executable"}
	}
	return Check{ID: "githook", Status: StatusPresent, Path: rel, Required: true, Message: "repo-local pre-commit hook is executable"}
}

func burpvalveToolCheck(root string, runner Runner, looker Looker) Check {
	if path, err := looker.LookPath("burpvalve"); err == nil {
		return Check{ID: "backpressure-tool", Status: StatusOK, Path: path, Required: true, Message: "burpvalve command is available on PATH"}
	}
	if check := repoLocalToolCheck(root); check.Status == StatusOK {
		if ignored, known, detail := gitIgnored(root, check.Path, runner, looker); known && ignored {
			return Check{ID: "backpressure-tool", Status: StatusConflict, Path: check.Path, Required: true, Message: "repo-local burpvalve fallback is executable but ignored by git", Detail: detail}
		}
		check.Status = StatusPresent
		check.Required = true
		check.Message = "repo-local burpvalve fallback is executable"
		return check
	} else if check.Status == StatusConflict || check.Status == StatusError {
		check.Required = true
		return check
	}
	return Check{ID: "backpressure-tool", Status: StatusUnavailable, Required: true, Message: "burpvalve is not on PATH; install the global command or opt into repo-local bin/burpvalve"}
}

func repoLocalBurpvalveFallbackCheck(root string, runner Runner, looker Looker) (Check, bool) {
	check := repoLocalToolCheck(root)
	check.ID = "backpressure-tool-fallback"
	check.Required = false
	switch check.Status {
	case StatusOK:
		if ignored, known, detail := gitIgnored(root, check.Path, runner, looker); known && ignored {
			check.Status = StatusConflict
			check.Message = "repo-local fallback is executable but ignored by git"
			check.Detail = detail
			return check, true
		}
		check.Status = StatusPresent
		check.Message = "repo-local fallback is executable"
	case StatusMissing:
		return Check{}, false
	case StatusConflict:
		check.Message = "repo-local burpvalve fallback conflict: " + check.Message
	}
	return check, true
}

func gitIgnored(root, rel string, runner Runner, looker Looker) (ignored bool, known bool, detail string) {
	if rel == "" {
		return false, false, ""
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return false, false, ""
	}
	if _, err := looker.LookPath("git"); err != nil {
		return false, false, ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := runner.Run(ctx, root, "git", "check-ignore", "-v", "--", rel)
	if err == nil {
		return true, true, strings.TrimSpace(result.Stdout)
	}
	if result.ExitCode == 1 {
		return false, true, ""
	}
	return false, false, strings.TrimSpace(firstNonEmpty(result.Stderr, err.Error()))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func repoLocalToolCheck(root string) Check {
	const rel = "bin/burpvalve"
	binPath := filepath.Join(root, filepath.FromSlash(rel))
	if info, err := os.Stat(binPath); err == nil {
		if info.IsDir() {
			return Check{ID: "backpressure-tool", Status: StatusConflict, Path: rel, Required: true, Message: "bin/burpvalve is a directory"}
		}
		if info.Mode().Perm()&0o111 == 0 {
			return Check{ID: "backpressure-tool", Status: StatusConflict, Path: rel, Required: true, Message: "bin/burpvalve exists but is not executable"}
		}
		return Check{ID: "backpressure-tool", Status: StatusOK, Path: rel, Required: true, Message: "runnable repo-local burpvalve binary"}
	} else if err != nil && !os.IsNotExist(err) {
		return Check{ID: "backpressure-tool", Status: StatusError, Path: rel, Required: true, Message: err.Error()}
	}
	return Check{ID: "backpressure-tool", Status: StatusMissing, Path: rel, Required: false, Message: "repo-local burpvalve fallback is not installed"}
}

func gitHooksPathCheck(root string, runner Runner, looker Looker) Check {
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return Check{ID: "git-hooks-path", Status: StatusUnavailable, Required: true, Message: "core.hooksPath check skipped because target is not a git repository"}
	}
	if _, err := looker.LookPath("git"); err != nil {
		return Check{ID: "git-hooks-path", Status: StatusUnavailable, Required: true, Message: "git executable unavailable for core.hooksPath check"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := runner.Run(ctx, root, "git", "config", "--get", "core.hooksPath")
	if err != nil {
		return Check{ID: "git-hooks-path", Status: StatusMissing, Required: true, Message: "git core.hooksPath is not configured", Detail: strings.TrimSpace(result.Stderr)}
	}
	value := strings.TrimSpace(result.Stdout)
	if value != ".githooks" {
		return Check{ID: "git-hooks-path", Status: StatusConflict, Required: true, Message: "git core.hooksPath should be .githooks", Detail: value}
	}
	return Check{ID: "git-hooks-path", Status: StatusOK, Required: true, Message: "git core.hooksPath is .githooks"}
}

func gitDirtyCheck(root string, runner Runner, looker Looker) Check {
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return Check{ID: "git-dirty", Status: StatusUnavailable, Required: false, Message: "git dirty check skipped because target is not a git repository"}
	}
	if _, err := looker.LookPath("git"); err != nil {
		return Check{ID: "git-dirty", Status: StatusUnavailable, Required: false, Message: "git executable unavailable"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := runner.Run(ctx, root, "git", "status", "--short")
	if err != nil {
		return Check{ID: "git-dirty", Status: StatusError, Required: false, Message: "git status --short failed", Detail: strings.TrimSpace(result.Stderr)}
	}
	if strings.TrimSpace(result.Stdout) == "" {
		return Check{ID: "git-dirty", Status: StatusClean, Required: false, Message: "working tree has no reported changes"}
	}
	return Check{ID: "git-dirty", Status: StatusDirty, Required: false, Message: "working tree has changes", Detail: strings.Trim(result.Stdout, "\n")}
}

func ntmCheck(root string, runner Runner, looker Looker) Check {
	report := ntm.Check(root, ntmRunner{runner: runner}, ntmLooker{looker: looker})
	detail := strings.Join(report.IntendedCommand, " ")
	switch report.Status {
	case ntm.StatusUnavailable:
		return Check{ID: "ntm", Status: StatusUnavailable, Required: false, Message: "ntm executable unavailable; setup should report this as a follow-up, not fake readiness", Detail: detail}
	case ntm.StatusReady:
		return Check{ID: "ntm", Status: StatusOK, Required: false, Message: "ntm --robot-capabilities succeeded; intended registration command is " + strings.Join(report.IntendedCommand, " "), Detail: detail}
	default:
		return Check{ID: "ntm", Status: StatusError, Required: false, Message: report.Blocker, Detail: detail}
	}
}

type ntmRunner struct {
	runner Runner
}

func (n ntmRunner) Run(ctx context.Context, dir string, name string, args ...string) (ntm.CommandResult, error) {
	result, err := n.runner.Run(ctx, dir, name, args...)
	return ntm.CommandResult(result), err
}

type ntmLooker struct {
	looker Looker
}

func (n ntmLooker) LookPath(file string) (string, error) {
	return n.looker.LookPath(file)
}

func summarize(checks []Check) Summary {
	summary := Summary{Ready: true}
	for _, check := range checks {
		switch check.Status {
		case StatusError:
			summary.Errors++
			if check.Required {
				summary.Ready = false
			}
		case StatusMissing:
			summary.Missing++
			if check.Required {
				summary.Ready = false
			}
		case StatusUnavailable:
			summary.Unavailable++
			if check.Required {
				summary.Ready = false
			}
		case StatusConflict:
			summary.Conflicts++
			if check.Required {
				summary.Ready = false
			}
		}
	}
	return summary
}

func populateReadinessFacts(report *Report) {
	if tool, ok := findCheck(report.Checks, "backpressure-tool"); ok {
		switch tool.Status {
		case StatusOK:
			report.CommandPath = tool.Path
			report.HookCommandSource = "path"
		case StatusPresent:
			report.RepoBinPath = tool.Path
			report.HookCommandSource = "repo-local"
		case StatusConflict:
			if tool.Path != "" {
				report.RepoBinPath = tool.Path
				report.HookCommandSource = "repo-local-conflict"
			}
		}
	}
	if fallback, ok := findCheck(report.Checks, "backpressure-tool-fallback"); ok && fallback.Path != "" {
		report.RepoBinPath = fallback.Path
	}
	if report.HookCommandSource == "" {
		report.HookCommandSource = "missing"
	}
}

func readinessSeverity(report *Report) string {
	if report.Fatal {
		return "blocked"
	}
	if report.RepoLocalBinary != nil && report.RepoLocalBinary.WarningCode != "" {
		return "warning"
	}
	for _, check := range report.Checks {
		if !check.Required && isBlockingStatus(check.Status) {
			return "warning"
		}
		if check.ID == "git-dirty" && check.Status == StatusDirty {
			return "warning"
		}
	}
	return "ready"
}

func repoLocalBinaryFacts(root string, report Report, runner Runner, looker Looker) *RepoLocalBinaryFacts {
	const rel = "bin/burpvalve"
	path := filepath.Join(root, filepath.FromSlash(rel))
	facts := &RepoLocalBinaryFacts{
		HookCommandSource: report.HookCommandSource,
		RepoLocalPath:     rel,
		PathCommand:       report.CommandPath,
		FreshnessStatus:   "not_applicable",
		ComparisonBasis:   "repo_local_missing",
	}
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		facts.FreshnessStatus = "unknown"
		facts.ComparisonBasis = "repo_local_stat_error: " + err.Error()
		facts.WarningCode = "repo_local_freshness_unknown"
		return facts
	}
	facts.RepoLocalExists = true
	if ignored, known, detail := repoLocalIgnoredFromChecks(report, rel); known {
		facts.RepoLocalIgnored = ignored
		if ignored {
			facts.ComparisonBasis = appendBasis(facts.ComparisonBasis, "repo_local_ignored: "+detail)
		}
	} else if ignored, known, detail := gitIgnored(root, rel, runner, looker); known && ignored {
		facts.RepoLocalIgnored = true
		facts.ComparisonBasis = appendBasis(facts.ComparisonBasis, "repo_local_ignored: "+detail)
	} else if !known && detail != "" {
		facts.ComparisonBasis = appendBasis(facts.ComparisonBasis, "repo_local_ignore_unknown: "+detail)
	}
	if report.HookCommandSource == "path" {
		facts.FreshnessStatus = "not_applicable"
		facts.ComparisonBasis = appendBasis(facts.ComparisonBasis, "hook_uses_path")
		return facts
	}
	if !strings.HasPrefix(report.HookCommandSource, "repo-local") {
		facts.FreshnessStatus = "not_applicable"
		facts.ComparisonBasis = appendBasis(facts.ComparisonBasis, "hook_not_repo_local")
		return facts
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		facts.FreshnessStatus = "unknown"
		facts.ComparisonBasis = appendBasis(facts.ComparisonBasis, "repo_local_not_regular")
		facts.WarningCode = warningCodeForRepoLocalFacts(facts)
		return facts
	}
	freshness, basis := repoLocalFreshness(root, path, info, report.CommandPath, runner, looker)
	facts.FreshnessStatus = freshness
	facts.ComparisonBasis = appendBasis(facts.ComparisonBasis, basis)
	facts.WarningCode = warningCodeForRepoLocalFacts(facts)
	return facts
}

func repoLocalIgnoredFromChecks(report Report, rel string) (bool, bool, string) {
	for _, id := range []string{"backpressure-tool", "backpressure-tool-fallback"} {
		check, ok := findCheck(report.Checks, id)
		if !ok || check.Path != rel {
			continue
		}
		if check.Status == StatusConflict && strings.Contains(check.Message, "ignored by git") {
			return true, true, check.Detail
		}
		if check.Status == StatusPresent || check.Status == StatusOK {
			return false, true, ""
		}
	}
	return false, false, ""
}

func warningCodeForRepoLocalFacts(facts *RepoLocalBinaryFacts) string {
	if facts.RepoLocalIgnored {
		return "repo_local_ignored"
	}
	switch facts.FreshnessStatus {
	case "stale":
		return "repo_local_stale"
	case "unknown":
		return "repo_local_freshness_unknown"
	case "fresh":
		return "repo_local_fallback_active"
	default:
		return ""
	}
}

func repoLocalFreshness(root, repoLocalPath string, repoLocalInfo os.FileInfo, pathCommand string, runner Runner, looker Looker) (string, string) {
	status := "fresh"
	var basis []string
	newestSource, sourceBasis, ok := newestTrackedSourceModTime(root, runner, looker)
	basis = append(basis, sourceBasis...)
	if !ok {
		status = "unknown"
	} else if repoLocalInfo.ModTime().Before(newestSource) {
		status = "stale"
		basis = append(basis, "repo_local_older_than_source")
	} else {
		basis = append(basis, "repo_local_newer_than_or_equal_source")
	}
	if pathCommand != "" {
		pathInfo, err := os.Lstat(pathCommand)
		if err != nil {
			status = unknownUnlessStale(status)
			basis = append(basis, "path_command_stat_unknown: "+err.Error())
		} else if pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() {
			status = unknownUnlessStale(status)
			basis = append(basis, "path_command_not_regular")
		} else if repoLocalInfo.ModTime().Before(pathInfo.ModTime()) {
			status = "stale"
			basis = append(basis, "repo_local_older_than_path_command")
		} else {
			basis = append(basis, "repo_local_newer_than_or_equal_path_command")
		}
		if versionStatus, versionBasis := compareBurpvalveVersions(root, repoLocalPath, pathCommand, runner); versionBasis != "" {
			basis = append(basis, versionBasis)
			if versionStatus == "stale" {
				status = "stale"
			} else if versionStatus == "unknown" {
				status = unknownUnlessStale(status)
			}
		}
	}
	return status, strings.Join(basis, "; ")
}

func unknownUnlessStale(status string) string {
	if status == "stale" {
		return status
	}
	return "unknown"
}

func newestTrackedSourceModTime(root string, runner Runner, looker Looker) (time.Time, []string, bool) {
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return time.Time{}, []string{"source_mtime_unknown: not_git_repo"}, false
	}
	if _, err := looker.LookPath("git"); err != nil {
		return time.Time{}, []string{"source_mtime_unknown: git_unavailable"}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := runner.Run(ctx, root, "git", "ls-files", "-z", "--", "cmd", "internal", "go.mod", "go.sum", "internal/scaffold/templates", "templates", "scripts", "install.sh")
	if err != nil {
		return time.Time{}, []string{"source_mtime_unknown: git_ls_files_failed"}, false
	}
	tracked := splitNUL(result.Stdout)
	trackedSet := map[string]bool{}
	var newest time.Time
	var newestPath string
	for _, rel := range tracked {
		if !sourcePathEligible(rel) {
			continue
		}
		trackedSet[filepath.ToSlash(rel)] = true
		info, statErr := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
		if statErr != nil || !info.Mode().IsRegular() {
			continue
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
			newestPath = filepath.ToSlash(rel)
		}
	}
	if newest.IsZero() {
		return time.Time{}, []string{"source_mtime_unknown: no_tracked_sources"}, false
	}
	if dirtyBasis, dirtyUnknown := dirtySourceBasis(root, runner, looker, trackedSet); dirtyUnknown {
		return newest, []string{"newest_tracked_source=" + newestPath, dirtyBasis}, false
	}
	return newest, []string{"newest_tracked_source=" + newestPath}, true
}

func dirtySourceBasis(root string, runner Runner, looker Looker, trackedSet map[string]bool) (string, bool) {
	if _, err := looker.LookPath("git"); err != nil {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := runner.Run(ctx, root, "git", "status", "--short", "--untracked-files=all")
	if err != nil {
		return "dirty_source_unknown: git_status_failed", true
	}
	for _, line := range strings.Split(strings.TrimSpace(result.Stdout), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		path := gitShortStatusPath(line)
		if path == "" || !sourcePathEligible(path) {
			continue
		}
		if strings.HasPrefix(line, "??") || !trackedSet[path] {
			return "dirty_source_unknown: " + path, true
		}
	}
	return "", false
}

func gitShortStatusPath(line string) string {
	if len(line) < 4 {
		return ""
	}
	path := strings.TrimSpace(line[3:])
	if before, after, ok := strings.Cut(path, " -> "); ok {
		_ = before
		path = after
	}
	return filepath.ToSlash(path)
}

func sourcePathEligible(rel string) bool {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" || strings.HasPrefix(rel, ".git/") || strings.HasPrefix(rel, "bin/") ||
		strings.HasPrefix(rel, "log/") || strings.HasPrefix(rel, "dist/") ||
		strings.HasPrefix(rel, "build/") || strings.HasPrefix(rel, "backpressure/attestations/") {
		return false
	}
	if rel == "go.mod" || rel == "go.sum" || rel == "install.sh" {
		return true
	}
	for _, prefix := range []string{"cmd/", "internal/", "templates/", "scripts/"} {
		if strings.HasPrefix(rel, prefix) {
			return true
		}
	}
	return false
}

func splitNUL(s string) []string {
	parts := strings.Split(s, "\x00")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, filepath.ToSlash(part))
		}
	}
	sort.Strings(out)
	return out
}

func compareBurpvalveVersions(root, repoLocalPath, pathCommand string, runner Runner) (string, string) {
	if pathCommand == "" {
		return "", ""
	}
	repoVersion, repoOK := commandVersion(root, runner, repoLocalPath)
	pathVersion, pathOK := commandVersion(root, runner, pathCommand)
	if !repoOK || !pathOK {
		return "unknown", "version_check_unknown"
	}
	if repoVersion == pathVersion {
		return "", "version_match"
	}
	return "unknown", "version_conflict"
}

func commandVersion(root string, runner Runner, command string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()
	result, err := runner.Run(ctx, root, command, "--version")
	if err != nil || result.ExitCode != 0 {
		return "", false
	}
	version := strings.TrimSpace(firstLine(result.Stdout))
	return version, version != ""
}

func appendBasis(current, extra string) string {
	extra = strings.TrimSpace(extra)
	if extra == "" {
		return current
	}
	if current == "" || current == "repo_local_missing" {
		return extra
	}
	return current + "; " + extra
}

func repoLocalBinaryRecoveryStep(report Report) []RecoveryStep {
	if report.RepoLocalBinary == nil || report.RepoLocalBinary.WarningCode == "" {
		return nil
	}
	return []RecoveryStep{{
		ID:      "repo-local-binary-provenance",
		Message: "Repo-local bin/burpvalve is the hook command or has stale-risk facts. Choose explicitly: run from source for this repo, install/use PATH burpvalve, or intentionally keep the repo-local fallback.",
		Command: "go run ./cmd/burpvalve setup, or install/use PATH burpvalve, or keep bin/burpvalve intentionally",
		Fatal:   false,
	}}
}

func plannedChanges(checks []Check) []PlannedChange {
	var changes []PlannedChange
	for _, check := range checks {
		switch check.Status {
		case StatusMissing:
			if check.Required {
				changes = append(changes, PlannedChange{
					ID:       check.ID,
					Action:   actionForMissing(check),
					Path:     check.Path,
					Reason:   check.Message,
					Requires: "init or repair mode",
				})
			}
		case StatusConflict:
			changes = append(changes, PlannedChange{
				ID:       check.ID,
				Action:   "manual review before repair",
				Path:     check.Path,
				Reason:   check.Message,
				Requires: "explicit confirmation",
			})
		case StatusUnavailable, StatusError:
			if check.Required {
				changes = append(changes, PlannedChange{
					ID:       check.ID,
					Action:   "resolve blocker",
					Path:     check.Path,
					Reason:   check.Message,
					Requires: "external dependency or manual repair",
				})
			}
		}
	}
	return changes
}

func hasFatalSetupBlocker(checks []Check) bool {
	for _, check := range checks {
		if check.Required && isBlockingStatus(check.Status) {
			return true
		}
	}
	return false
}

func isBlockingStatus(status CheckStatus) bool {
	switch status {
	case StatusMissing, StatusUnavailable, StatusConflict, StatusError:
		return true
	default:
		return false
	}
}

func recoverySteps(checks []Check, changes []PlannedChange) []RecoveryStep {
	steps := make([]RecoveryStep, 0, len(changes))
	for _, check := range checks {
		if !isBlockingStatus(check.Status) {
			continue
		}
		fatal := check.Required
		step := RecoveryStep{
			ID:      check.ID,
			Message: recoveryMessage(check),
			Command: recoveryCommand(check, changes),
			Fatal:   fatal,
		}
		steps = append(steps, step)
	}
	return steps
}

func recoveryMessage(check Check) string {
	switch check.ID {
	case "git-repo":
		return "Initialize Git before wiring commit hooks."
	case "git-hooks-path":
		if check.Status == StatusUnavailable {
			return "Git hook path cannot be checked until the target is a Git repository and git is available."
		}
		return "Configure this repo to use Burpvalve's .githooks directory."
	case "br-tool":
		return "Install br before enabling Beads-backed task tracking."
	case "backpressure-tool":
		return "Install Burpvalve on PATH, or explicitly opt into the repo-local fallback."
	case "ntm":
		return "NTM is optional; install it for coordination or skip it."
	default:
		return check.Message
	}
}

func recoveryCommand(check Check, changes []PlannedChange) string {
	switch check.ID {
	case "git-repo":
		return "git init"
	case "git-hooks-path":
		if check.Status == StatusUnavailable {
			return "git init && burpvalve repair --force --json hooks-path"
		}
		return "burpvalve repair --force --json hooks-path"
	case "githook":
		return "burpvalve repair --force --json precommit"
	case "br-tool":
		return "install br, then run burpvalve repair --force --json beads"
	case "beads":
		return "burpvalve repair --force --json beads"
	case "backpressure-tool":
		return "install-burpvalve, or run burpvalve repair --force --json bin/burpvalve"
	case "ntm":
		return "install ntm, or run burpvalve init --force --json --no-ntm"
	}
	for _, change := range changes {
		if change.ID == check.ID {
			target := repairTargetForCheckID(change.ID)
			switch change.Requires {
			case "init or repair mode":
				return "burpvalve repair --force --json " + target
			case "explicit confirmation":
				return "burpvalve repair " + target
			}
		}
	}
	return ""
}

func repairTargetForCheckID(id string) string {
	switch id {
	case "backpressure-attestations":
		return "attestations"
	case "backpressure-tool", "backpressure-tool-fallback":
		return "bin/burpvalve"
	case "git-hooks-path":
		return "hooks-path"
	case "githook":
		return "precommit"
	default:
		return id
	}
}

func actionForMissing(check Check) string {
	switch check.ID {
	case "agents":
		return "create AGENTS.md from template"
	case "claude":
		return "create CLAUDE.md symlink to AGENTS.md"
	case "githook":
		return "install executable .githooks/pre-commit"
	case "git-hooks-path":
		return "configure git core.hooksPath .githooks"
	case "backpressure-tool":
		return "install burpvalve on PATH or opt into repo-local bin/burpvalve"
	case "beads":
		return "run br init"
	default:
		if check.Path != "" {
			return "create missing scaffold path"
		}
		return "repair missing required check"
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

type TextOptions struct {
	Color bool
}

type reportRow struct {
	Left  string
	Right string
}

func (r Report) Text() string {
	return r.TextWithOptions(TextOptions{})
}

func (r Report) TextWithOptions(opts TextOptions) string {
	var b strings.Builder
	ui := cliui.New(opts.Color)
	fmt.Fprintf(&b, "%s\n", ui.Title("burpvalve setup report"))
	writeTwoColumnSection(&b, "summary", "field", "value", []reportRow{
		{Left: "target", Right: ui.Path(r.TargetRoot)},
		{Left: "edits files", Right: ui.Bool(r.Mutating)},
		{Left: "ready", Right: ui.Bool(r.Summary.Ready)},
		{Left: "severity", Right: emptyFallback(r.ReadinessSeverity, "unknown")},
		{Left: "hook command", Right: emptyFallback(r.HookCommandSource, "unknown")},
		{Left: "missing", Right: fmt.Sprint(r.Summary.Missing)},
		{Left: "unavailable", Right: fmt.Sprint(r.Summary.Unavailable)},
		{Left: "conflicts", Right: fmt.Sprint(r.Summary.Conflicts)},
		{Left: "errors", Right: fmt.Sprint(r.Summary.Errors)},
	}, opts)

	if r.Config != nil {
		writeConfigSummary(&b, *r.Config, opts)
	}
	if r.RepoLocalBinary != nil {
		writeRepoLocalBinaryFacts(&b, *r.RepoLocalBinary, opts)
	}

	var required []reportRow
	var optional []reportRow
	for _, check := range r.Checks {
		if check.ID == "git-dirty" {
			continue
		}
		row := reportRow{
			Left:  colorStatus(check.Status, opts.Color),
			Right: ui.Path(checkItem(check)) + ui.Muted(" - ") + checkPurpose(check),
		}
		if check.Detail != "" && !strings.Contains(check.Detail, "\n") {
			row.Right += " (" + check.Detail + ")"
		}
		if check.Required {
			required = append(required, row)
		} else {
			optional = append(optional, row)
		}
	}
	writeTwoColumnSection(&b, "required files and tools", "status", "item / purpose", required, opts)
	writeTwoColumnSection(&b, "optional context", "status", "item / purpose", optional, opts)

	if dirty, ok := findCheck(r.Checks, "git-dirty"); ok {
		writeGitChanges(&b, dirty, opts)
	}

	if len(r.NextSteps) > 0 {
		var steps []reportRow
		for _, step := range r.NextSteps {
			command := step.Command
			if command == "" {
				command = "decide"
			}
			message := step.Message
			if step.Fatal {
				message += " " + ui.Muted("(required)")
			} else {
				message += " " + ui.Muted("(optional)")
			}
			steps = append(steps, reportRow{Left: ui.Info(command), Right: message})
		}
		writeTwoColumnSection(&b, "next steps", "run", "why", steps, opts)
	}

	if len(r.PlannedChanges) == 0 {
		writeTwoColumnSection(&b, "planned changes", "run", "change", []reportRow{
			{Left: "-", Right: "none"},
		}, opts)
		return b.String()
	}
	var planned []reportRow
	for _, change := range r.PlannedChanges {
		target := change.Path
		if target == "" {
			target = change.ID
		}
		right := ui.Path(target) + ui.Muted(" - ") + change.Action
		if change.Requires != "" {
			right += ui.Muted(" (" + change.Requires + ")")
		}
		planned = append(planned, reportRow{Left: ui.Info("init/repair"), Right: right})
	}
	writeTwoColumnSection(&b, "planned changes", "run", "change", planned, opts)
	return b.String()
}

func writeRepoLocalBinaryFacts(b *strings.Builder, facts RepoLocalBinaryFacts, opts TextOptions) {
	ui := cliui.New(opts.Color)
	rows := []reportRow{
		{Left: "hook command", Right: emptyFallback(facts.HookCommandSource, "unknown")},
		{Left: "repo-local path", Right: ui.Path(facts.RepoLocalPath)},
		{Left: "repo-local exists", Right: ui.Bool(facts.RepoLocalExists)},
		{Left: "repo-local ignored", Right: ui.Bool(facts.RepoLocalIgnored)},
		{Left: "PATH command", Right: emptyFallback(facts.PathCommand, "unavailable")},
		{Left: "freshness", Right: facts.FreshnessStatus},
		{Left: "basis", Right: facts.ComparisonBasis},
	}
	if facts.WarningCode != "" {
		rows = append(rows, reportRow{Left: "warning", Right: facts.WarningCode})
	}
	writeTwoColumnSection(b, "repo-local binary provenance", "field", "value", rows, opts)
}

func writeConfigSummary(b *strings.Builder, config ConfigSummary, opts TextOptions) {
	ui := cliui.New(opts.Color)
	rows := []reportRow{
		{Left: "global", Right: ui.Path(config.GlobalPath) + " " + ui.Muted(foundLabel(config.GlobalFound))},
		{Left: "project", Right: ui.Path(config.ProjectPath) + " " + ui.Muted(foundLabel(config.ProjectFound))},
	}
	if len(config.Settings) > 0 {
		for _, setting := range config.Settings {
			rows = append(rows, reportRow{Left: setting.Source, Right: setting.Key + " = " + setting.Value})
		}
	} else {
		for _, source := range config.Sources {
			rows = append(rows, reportRow{Left: source.Source, Right: source.Key})
		}
	}
	writeTwoColumnSection(b, "config sources", "source", "value", rows, opts)
}

func foundLabel(found bool) string {
	if found {
		return "(found)"
	}
	return "(missing)"
}

func emptyFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func writeTwoColumnSection(b *strings.Builder, title, leftHeader, rightHeader string, rows []reportRow, opts TextOptions) {
	if len(rows) == 0 {
		return
	}
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	ui := cliui.New(opts.Color)
	if title != "" {
		b.WriteString(ui.Section(title))
		b.WriteByte('\n')
	}
	width := len(leftHeader)
	for _, row := range rows {
		if l := visibleLen(row.Left); l > width {
			width = l
		}
	}
	if width > 28 {
		width = 28
	}
	headerLeft := ui.Header(leftHeader)
	headerRight := ui.Header(rightHeader)
	fmt.Fprintf(b, "  %-*s  %s\n", width+cliui.ANSIPadding(headerLeft), headerLeft, headerRight)
	separatorLeft := ui.Muted(strings.Repeat("-", len(leftHeader)))
	separatorRight := ui.Muted(strings.Repeat("-", len(rightHeader)))
	fmt.Fprintf(b, "  %-*s  %s\n", width+cliui.ANSIPadding(separatorLeft), separatorLeft, separatorRight)
	for _, row := range rows {
		fmt.Fprintf(b, "  %-*s  %s\n", width+cliui.ANSIPadding(row.Left), row.Left, row.Right)
	}
}

func writeGitChanges(b *strings.Builder, check Check, opts TextOptions) {
	if strings.TrimSpace(check.Detail) == "" {
		writeTwoColumnSection(b, "git changes", "status", "file", []reportRow{
			{Left: colorStatus(check.Status, opts.Color), Right: check.Message},
		}, opts)
		return
	}
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	ui := cliui.New(opts.Color)
	b.WriteString(ui.Section("git changes"))
	b.WriteByte('\n')
	b.WriteString("  " + ui.Muted("Git shows two status columns before each file: staged | working tree.") + "\n")
	b.WriteString("  " + ui.Muted("M = modified, D = deleted, A = added, R = renamed, C = copied, U = unmerged, ? = untracked, - = no change.") + "\n")
	rows := gitStatusRows(check.Detail, opts)
	writeTwoColumnSection(b, "", "status", "file", rows, opts)
}

func gitStatusRows(detail string, opts TextOptions) []reportRow {
	var rows []reportRow
	for _, line := range strings.Split(strings.Trim(detail, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		status, path := parseGitStatusLine(line)
		rows = append(rows, reportRow{
			Left:  colorGitStatus(status, opts.Color),
			Right: cliui.New(opts.Color).Path(path),
		})
	}
	return rows
}

func parseGitStatusLine(line string) (string, string) {
	if strings.HasPrefix(line, "?? ") {
		return "??", strings.TrimSpace(line[3:])
	}
	if len(line) < 3 {
		return strings.TrimSpace(line), ""
	}
	status := line[:2]
	status = strings.ReplaceAll(status, " ", "-")
	return status, strings.TrimSpace(line[3:])
}

func findCheck(checks []Check, id string) (Check, bool) {
	for _, check := range checks {
		if check.ID == id {
			return check, true
		}
	}
	return Check{}, false
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func checkItem(check Check) string {
	switch check.ID {
	case "br-tool":
		return "br"
	case "git-hooks-path":
		return "git core.hooksPath"
	case "backpressure-tool":
		if check.Path != "" {
			return check.Path
		}
		return "burpvalve"
	case "backpressure-tool-fallback":
		if check.Path != "" {
			return check.Path
		}
		return "bin/burpvalve"
	case "git-repo":
		return ".git"
	case "ntm":
		return "ntm"
	default:
		if check.Path != "" {
			return check.Path
		}
		return check.ID
	}
}

func checkPurpose(check Check) string {
	switch check.ID {
	case "agents":
		return "shared instructions that agents and tools read before working"
	case "claude":
		return "points Claude Code at the same shared instructions"
	case "docs":
		return "durable project knowledge and decisions"
	case "plans":
		return "implementation plans; active task tracking belongs in Beads"
	case "log":
		return "work notes, debugging notes, and failed-attempt records"
	case "backpressure":
		return "rules a change must satisfy before commit"
	case "backpressure-attestations":
		return "tracked proof that a staged change passed the rules"
	case "beads":
		return "local issue and task graph"
	case "br-tool":
		return "command-line tool used to read and update Beads"
	case "githook":
		return "script Git runs before it creates a commit"
	case "git-hooks-path":
		return "Git setting that tells this repo to use .githooks"
	case "backpressure-tool":
		return "Burpvalve command the git hook runs before a commit"
	case "backpressure-tool-fallback":
		return "optional repo-local fallback for hook environments without PATH"
	case "git-repo":
		return "Git metadata for this repository"
	case "ntm":
		return "optional coordination tool; setup can register the repo session"
	default:
		return check.Message
	}
}

func colorStatus(status CheckStatus, color bool) string {
	return cliui.New(color).Status(string(status))
}

func colorGitStatus(status string, color bool) string {
	return cliui.New(color).GitStatus(status)
}

func visibleLen(s string) int {
	return cliui.VisibleLen(s)
}
