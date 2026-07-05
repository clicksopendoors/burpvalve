package scaffold

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"burpvalve/internal/cliui"
	"burpvalve/internal/ntm"
)

//go:embed templates/* templates/claude/skills/burpvalve-orchestrator/.MAINTAINER-CHECKS.md.tmpl
var embeddedTemplates embed.FS

type ApplyResult struct {
	SchemaVersion     int             `json:"schema_version"`
	Command           string          `json:"command"`
	Status            string          `json:"status"`
	TargetRoot        string          `json:"target_root"`
	Config            *ConfigSummary  `json:"config,omitempty"`
	ClaudeRoute       string          `json:"claude_route,omitempty"`
	ClaudeRouteSource string          `json:"claude_route_source,omitempty"`
	Mutating          bool            `json:"mutating"`
	Fatal             bool            `json:"fatal"`
	PartialSuccess    bool            `json:"partial_success"`
	NextSteps         []RecoveryStep  `json:"next_steps,omitempty"`
	Created           []string        `json:"created"`
	Repaired          []string        `json:"repaired,omitempty"`
	Preserved         []string        `json:"preserved"`
	Skipped           []string        `json:"skipped,omitempty"`
	Conflicts         []ApplyConflict `json:"conflicts"`
	Commands          []string        `json:"commands"`
	NTM               ntm.Report      `json:"ntm"`
	Generated         time.Time       `json:"generated_at"`
}

type ApplyConflict struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type templateFile struct {
	Source string
	Target string
	Mode   os.FileMode
}

var scaffoldTemplates = []templateFile{
	{Source: "templates/AGENTS.md.tmpl", Target: "AGENTS.md", Mode: 0o644},
	{Source: "templates/ORCHESTRATOR.md.tmpl", Target: "ORCHESTRATOR.md", Mode: 0o644},
	{Source: "templates/docs/README.md", Target: "docs/README.md", Mode: 0o644},
	{Source: "templates/docs/ntm-bridge.md", Target: "docs/ntm-bridge.md", Mode: 0o644},
	{Source: "templates/plans/README.md", Target: "plans/README.md", Mode: 0o644},
	{Source: "templates/log/README.md", Target: "log/README.md", Mode: 0o644},
	{Source: "templates/log/backpressure/failed/README.md", Target: "log/backpressure/failed/README.md", Mode: 0o644},
	{Source: "templates/backpressure/manifest.yaml", Target: "backpressure/manifest.yaml", Mode: 0o644},
	{Source: "templates/backpressure/README.md", Target: "backpressure/README.md", Mode: 0o644},
	{Source: "templates/backpressure/attestations/README.md", Target: "backpressure/attestations/README.md", Mode: 0o644},
	{Source: "templates/backpressure/lint-rules.md", Target: "backpressure/lint-rules.md", Mode: 0o644},
	{Source: "templates/backpressure/dry.md", Target: "backpressure/dry.md", Mode: 0o644},
	{Source: "templates/backpressure/anti-reward-hacking.md", Target: "backpressure/anti-reward-hacking.md", Mode: 0o644},
	{Source: "templates/backpressure/one-function-one-test.md", Target: "backpressure/one-function-one-test.md", Mode: 0o644},
	{Source: "templates/backpressure/definition-of-done.md", Target: "backpressure/definition-of-done.md", Mode: 0o644},
	{Source: "templates/backpressure/evidence-log.md", Target: "backpressure/evidence-log.md", Mode: 0o644},
	{Source: "templates/backpressure/scope-control.md", Target: "backpressure/scope-control.md", Mode: 0o644},
	{Source: "templates/backpressure/destructive-operations.md", Target: "backpressure/destructive-operations.md", Mode: 0o644},
	{Source: "templates/backpressure/data-integrity.md", Target: "backpressure/data-integrity.md", Mode: 0o644},
	{Source: "templates/backpressure/security-boundaries.md", Target: "backpressure/security-boundaries.md", Mode: 0o644},
	{Source: "templates/backpressure/visual-regression.md", Target: "backpressure/visual-regression.md", Mode: 0o644},
	{Source: "templates/backpressure/performance-budget.md", Target: "backpressure/performance-budget.md", Mode: 0o644},
	{Source: "templates/backpressure/autonomy-boundary.md", Target: "backpressure/autonomy-boundary.md", Mode: 0o644},
	{Source: "templates/githooks/pre-commit", Target: ".githooks/pre-commit", Mode: 0o755},
	{Source: "templates/tools/burpvalve/README.md", Target: "tools/burpvalve/README.md", Mode: 0o644},
}

func ApplyInit(target string) (ApplyResult, error) {
	return ApplyInitWithOptions(target, ApplyOptions{})
}

type ApplyOptions struct {
	Runner               Runner
	Looker               Looker
	BackpressureToolPath string
	Targets              []ScaffoldTarget
	ClaudeRoute          string
	AdoptClaude          bool
	SkipAgents           bool
	SkipClaude           bool
	SkipBeads            bool
	SkipNTM              bool
	SkipDocs             bool
	SkipPlans            bool
	SkipLog              bool
	SkipBackpressure     bool
	SkipAttestations     bool
	SkipPreCommit        bool
	SkipHooksPath        bool
	SkipTool             bool
	SkipToolDocs         bool
	VerifierConfigured   bool
	Dogfood              bool
	GitInit              bool
}

func ApplyInitWithOptions(target string, opts ApplyOptions) (ApplyResult, error) {
	root, err := filepath.Abs(target)
	if err != nil {
		return ApplyResult{}, err
	}
	if opts.Runner == nil {
		opts.Runner = execRunner{}
	}
	if opts.Looker == nil {
		opts.Looker = osLooker{}
	}
	result := ApplyResult{
		SchemaVersion: 1,
		Command:       "init",
		TargetRoot:    root,
		ClaudeRoute:   effectiveInitClaudeRoute(opts),
		Mutating:      true,
		Generated:     time.Now().UTC(),
	}
	targets := effectiveScaffoldTargets(opts.Targets, scaffoldSkips(opts))
	if err := preflightClaudeRoute(root, targets, opts, &result); err != nil {
		finalizeApplyResult(&result)
		return result, err
	}

	for _, item := range scaffoldTemplates {
		if item.Target == ".githooks/pre-commit" {
			continue
		}
		target, ok := scaffoldTargetForTemplate(item.Target)
		if !ok || !targets.has(target) {
			continue
		}
		if opts.SkipAgents && target == TargetAgents {
			result.Skipped = append(result.Skipped, item.Target+" (--no-agents)")
			continue
		}
		if opts.SkipDocs && target == TargetDocs {
			result.Skipped = append(result.Skipped, item.Target+" (--no-docs)")
			continue
		}
		if opts.SkipPlans && target == TargetPlans {
			result.Skipped = append(result.Skipped, item.Target+" (--no-plans)")
			continue
		}
		if opts.SkipLog && target == TargetLog {
			result.Skipped = append(result.Skipped, item.Target+" (--no-log)")
			continue
		}
		if opts.SkipBackpressure && target == TargetBackpressure {
			result.Skipped = append(result.Skipped, item.Target+" (--no-backpressure)")
			continue
		}
		if opts.SkipAttestations && target == TargetAttestations {
			result.Skipped = append(result.Skipped, item.Target+" (--no-attestations)")
			continue
		}
		if opts.SkipToolDocs && target == TargetToolDocs {
			result.Skipped = append(result.Skipped, item.Target+" (--no-tool-docs)")
			continue
		}
		body, err := scaffoldTemplateBody(item.Source, opts)
		if err != nil {
			return result, err
		}
		if item.Target == "AGENTS.md" {
			if err := ensureAgentsFile(root, body, item.Mode, opts, &result); err != nil {
				return result, err
			}
			continue
		}
		if err := createFileIfMissing(root, item.Target, body, item.Mode, &result); err != nil {
			return result, err
		}
	}
	if targets.has(TargetLog) {
		if err := ensureLocalGitignore(root, false, &result); err != nil {
			return result, err
		}
	}
	if !targets.has(TargetBeads) {
		// Not requested.
	} else if opts.SkipBeads {
		result.Skipped = append(result.Skipped, ".beads (--no-beads)")
	} else {
		if err := ensureBeads(root, opts.Runner, opts.Looker, &result); err != nil {
			return result, err
		}
	}
	if !targets.has(TargetClaude) {
		// Not requested.
	} else if opts.SkipClaude {
		result.Skipped = append(result.Skipped, "CLAUDE.md (--no-claude)")
	} else {
		if err := ensureClaudeInitRoute(root, opts, &result); err != nil {
			return result, err
		}
	}
	if targets.has(TargetPreCommit) {
		if opts.SkipPreCommit {
			result.Skipped = append(result.Skipped, ".githooks/pre-commit (--no-precommit)")
		} else if err := ensurePreCommitHook(root, &result); err != nil {
			return result, err
		}
	}
	if targets.has(TargetTool) {
		if opts.SkipTool {
			result.Skipped = append(result.Skipped, "bin/burpvalve (--no-bin)")
		} else if err := ensureBackpressureTool(root, opts.BackpressureToolPath, &result); err != nil {
			return result, err
		}
	}
	if targets.has(TargetHooksPath) {
		if err := ensureGitForHooks(root, opts, &result); err != nil {
			return result, err
		}
		if opts.SkipHooksPath {
			result.Skipped = append(result.Skipped, "git core.hooksPath (--no-hooks-path)")
		} else if err := configureHooksPath(root, opts.Runner, opts.Looker, &result); err != nil {
			return result, err
		}
	}
	if !targets.has(TargetNTM) {
		// Not requested.
	} else if opts.SkipNTM {
		result.Skipped = append(result.Skipped, "ntm quick (--no-ntm)")
		result.NTM = ntm.Report{
			Status:          ntm.StatusSkipped,
			BaseSessionName: ntm.BaseSessionName(root),
			Blocker:         "skipped by --no-ntm",
		}
	} else {
		registerNTM(root, opts.Runner, opts.Looker, &result)
	}
	finalizeApplyResult(&result)
	if len(result.Conflicts) > 0 {
		return result, fmt.Errorf("init encountered %d conflict(s)", len(result.Conflicts))
	}
	return result, nil
}

func finalizeApplyResult(result *ApplyResult) {
	result.Status = "applied"
	if len(result.Conflicts) > 0 {
		result.Status = "partial_success"
		result.Fatal = true
		result.PartialSuccess = len(result.Created) > 0 || len(result.Repaired) > 0 || len(result.Preserved) > 0 || len(result.Commands) > 0
		for _, conflict := range result.Conflicts {
			result.NextSteps = append(result.NextSteps, RecoveryStep{
				ID:      conflict.Path,
				Message: conflict.Message,
				Command: "burpvalve setup --json",
				Fatal:   true,
			})
		}
	}
}

func ensureGitForHooks(root string, opts ApplyOptions, result *ApplyResult) error {
	if opts.SkipHooksPath {
		return nil
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
		return nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !opts.GitInit {
		result.Conflicts = append(result.Conflicts, ApplyConflict{
			Path:    ".git",
			Message: "git repository is required before installing commit hooks; rerun with --git-init or skip hooks with --no-hooks",
		})
		return nil
	}
	if _, err := exec.LookPath("git"); err != nil {
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: ".git", Message: "git executable unavailable; cannot run git init"})
		return nil
	}
	cmd := exec.Command("git", "init")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: ".git", Message: "git init failed: " + strings.TrimSpace(string(output))})
		return nil
	}
	result.Created = append(result.Created, "git init")
	return nil
}

func scaffoldTemplateBody(source string, opts ApplyOptions) ([]byte, error) {
	if source == "templates/AGENTS.md.tmpl" {
		return renderAgentsTemplate(opts)
	}
	if source == "templates/ORCHESTRATOR.md.tmpl" {
		return renderOrchestratorTemplate(opts)
	}
	return embeddedTemplates.ReadFile(source)
}

func preflightClaudeRoute(root string, targets scaffoldTargetSet, opts ApplyOptions, result *ApplyResult) error {
	if !targets.has(TargetClaude) || opts.SkipClaude || effectiveInitClaudeRoute(opts) == ClaudeRouteNone {
		return nil
	}
	if opts.SkipAgents && !fileExists(root, "AGENTS.md") {
		result.Conflicts = append(result.Conflicts, ApplyConflict{
			Path:    "AGENTS.md",
			Message: "active Claude route requires AGENTS.md; omit --no-agents, create AGENTS.md first, or set claude_route=none",
		})
		return fmt.Errorf("init encountered %d conflict(s)", len(result.Conflicts))
	}
	return nil
}

func fileExists(root, rel string) bool {
	_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

func registerNTM(root string, runner Runner, looker Looker, result *ApplyResult) {
	report := ntm.Quick(root, ntmRunner{runner: runner}, ntmLooker{looker: looker})
	result.NTM = report
	switch report.Status {
	case ntm.StatusRegistered:
		result.Commands = append(result.Commands, "ntm quick "+report.BaseSessionName, "ntm --robot-snapshot")
	case ntm.StatusUnavailable:
		result.Preserved = append(result.Preserved, "ntm unavailable: "+report.Blocker)
	default:
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: "ntm", Message: report.Blocker})
	}
}

func createDirIfMissing(root, rel string, result *ApplyResult) error {
	path := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(path)
	if err == nil {
		if !info.IsDir() {
			result.Conflicts = append(result.Conflicts, ApplyConflict{Path: rel, Message: "path exists but is not a directory"})
			return nil
		}
		result.Preserved = append(result.Preserved, rel)
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	result.Created = append(result.Created, rel)
	return nil
}

func createFileIfMissing(root, rel string, body []byte, mode os.FileMode, result *ApplyResult) error {
	path := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(path)
	if err == nil {
		if info.IsDir() {
			result.Conflicts = append(result.Conflicts, ApplyConflict{Path: rel, Message: "path exists as a directory"})
			return nil
		}
		result.Preserved = append(result.Preserved, rel)
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, body, mode); err != nil {
		return err
	}
	result.Created = append(result.Created, rel)
	return nil
}

func ensureAgentsFile(root string, body []byte, mode os.FileMode, opts ApplyOptions, result *ApplyResult) error {
	const rel = "AGENTS.md"
	path := filepath.Join(root, rel)
	info, err := os.Lstat(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, body, mode); err != nil {
			return err
		}
		result.Created = append(result.Created, rel)
		return nil
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
	block, err := buildAgentsRepairBlock(string(body), missing)
	if err != nil {
		return err
	}
	next := strings.TrimRight(string(current), "\n") + "\n\n" + block
	fileMode := info.Mode().Perm()
	if fileMode == 0 {
		fileMode = mode
	}
	if err := os.WriteFile(path, []byte(next), fileMode); err != nil {
		return err
	}
	result.Repaired = append(result.Repaired, rel+" sections: "+strings.Join(missing, ", "))
	return nil
}

func ensureBackpressureTool(root, sourcePath string, result *ApplyResult) error {
	if check := repoLocalToolCheck(root); check.Status == StatusOK {
		result.Preserved = append(result.Preserved, check.Path)
		return nil
	}
	source, err := resolveBackpressureToolSource(sourcePath)
	if err != nil {
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: "bin/burpvalve", Message: err.Error()})
		return nil
	}
	const rel = "bin/burpvalve"
	dest := filepath.Join(root, filepath.FromSlash(rel))
	if info, err := os.Lstat(dest); err == nil {
		if info.IsDir() {
			result.Conflicts = append(result.Conflicts, ApplyConflict{Path: rel, Message: "path exists as a directory"})
			return nil
		}
		if info.Mode().Perm()&0o111 == 0 {
			if err := os.Chmod(dest, info.Mode().Perm()|0o755); err != nil {
				return err
			}
			result.Created = append(result.Created, rel+" executable bit")
		} else {
			result.Preserved = append(result.Preserved, rel)
		}
		return ensureLocalGitignore(root, true, result)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := copyFile(source, dest, 0o755); err != nil {
		return err
	}
	result.Created = append(result.Created, rel)
	return ensureLocalGitignore(root, true, result)
}

func resolveBackpressureToolSource(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return executableFile(explicit)
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot locate burpvalve executable: %w", err)
	}
	return executableFile(exe)
}

func executableFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("runnable burpvalve not found; build burpvalve with `make build` and rerun setup: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("runnable burpvalve source %s is a directory", path)
	}
	if info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("runnable burpvalve source %s is not executable", path)
	}
	return path, nil
}

func copyFile(src, dest string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dest, mode)
}

func ensureLocalGitignore(root string, includeBin bool, result *ApplyResult) error {
	const rel = ".gitignore"
	path := filepath.Join(root, rel)
	want := []string{"log/backpressure/failed/*.json"}
	if includeBin {
		want = append([]string{"bin/"}, want...)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		content := "# Local burpvalve outputs\n" + strings.Join(want, "\n") + "\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
		result.Created = append(result.Created, rel)
		return nil
	}
	text := string(body)
	var missing []string
	for _, entry := range want {
		if !gitignoreContains(text, entry) {
			missing = append(missing, entry)
		}
	}
	if len(missing) == 0 {
		result.Preserved = append(result.Preserved, rel)
		return nil
	}
	next := strings.TrimRight(text, "\n")
	if next != "" {
		next += "\n"
	}
	next += strings.Join(missing, "\n") + "\n"
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		return err
	}
	result.Created = append(result.Created, rel+" entries")
	return nil
}

func gitignoreContains(body, entry string) bool {
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == entry {
			return true
		}
	}
	return false
}

func effectiveInitClaudeRoute(opts ApplyOptions) string {
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

func ensureClaudeInitRoute(root string, opts ApplyOptions, result *ApplyResult) error {
	route := effectiveInitClaudeRoute(opts)
	switch route {
	case ClaudeRouteNone:
		result.Skipped = append(result.Skipped, "Claude route disabled (claude_route=none)")
		return nil
	case ClaudeRouteAgentSymlink:
		if opts.SkipAgents {
			result.Preserved = append(result.Preserved, "AGENTS.md used (--no-agents)")
		}
		return ensureClaudeLink(root, result)
	case ClaudeRouteOrchestratorSkill:
		if opts.SkipAgents {
			result.Preserved = append(result.Preserved, "AGENTS.md used (--no-agents)")
		}
		if err := ensureClaudeOrchestratorBootstrap(root, false, result); err != nil {
			return err
		}
		return ensureClaudeOrchestratorSkillPackage(root, false, result)
	default:
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: "CLAUDE.md", Message: "unknown Claude route " + route})
		return nil
	}
}

func ensureClaudeLink(root string, result *ApplyResult) error {
	const rel = "CLAUDE.md"
	path := filepath.Join(root, rel)
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			dest, readErr := os.Readlink(path)
			if readErr != nil {
				return readErr
			}
			if dest == "AGENTS.md" {
				result.Preserved = append(result.Preserved, rel)
				return nil
			}
			result.Conflicts = append(result.Conflicts, ApplyConflict{Path: rel, Message: "symlink points to " + dest + ", not AGENTS.md"})
			return nil
		}
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: rel, Message: "regular file exists; explicit confirmation required before replacing"})
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Symlink("AGENTS.md", path); err != nil {
		return err
	}
	result.Created = append(result.Created, rel)
	return nil
}

func ensureClaudeOrchestratorBootstrap(root string, repairDrift bool, result *ApplyResult) error {
	body, err := embeddedTemplates.ReadFile("templates/CLAUDE.md.orchestrator.tmpl")
	if err != nil {
		return err
	}
	const rel = "CLAUDE.md"
	path := filepath.Join(root, rel)
	info, err := os.Lstat(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.WriteFile(path, body, 0o644); err != nil {
			return err
		}
		result.Created = append(result.Created, rel)
		return nil
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
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: rel, Message: "existing symlink route points to " + dest + "; repair with claude_route=orchestrator-skill to convert"})
		return nil
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(current) == string(body) {
		result.Preserved = append(result.Preserved, rel)
		return nil
	}
	if isGeneratedClaudeOrchestratorBootstrap(current) && repairDrift {
		mode := info.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(path, body, mode); err != nil {
			return err
		}
		result.Repaired = append(result.Repaired, rel+" orchestrator bootstrap")
		return nil
	}
	result.Conflicts = append(result.Conflicts, ApplyConflict{Path: rel, Message: "regular file exists; explicit repair adoption required before replacing"})
	return nil
}

func ensureClaudeOrchestratorSkillPackage(root string, repairDrift bool, result *ApplyResult) error {
	files, err := claudeOrchestratorSkillTemplates()
	if err != nil {
		return err
	}
	for _, item := range files {
		body, err := embeddedTemplates.ReadFile(item.Source)
		if err != nil {
			return err
		}
		path := filepath.Join(root, filepath.FromSlash(item.Target))
		info, err := os.Lstat(path)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, body, item.Mode); err != nil {
				return err
			}
			result.Created = append(result.Created, item.Target)
			continue
		}
		if info.IsDir() {
			result.Conflicts = append(result.Conflicts, ApplyConflict{Path: item.Target, Message: "path exists as a directory"})
			continue
		}
		current, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if string(current) == string(body) {
			result.Preserved = append(result.Preserved, item.Target)
			continue
		}
		if repairDrift {
			mode := info.Mode().Perm()
			if mode == 0 {
				mode = item.Mode
			}
			if err := os.WriteFile(path, body, mode); err != nil {
				return err
			}
			result.Repaired = append(result.Repaired, item.Target)
			continue
		}
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: item.Target, Message: "generated skill file differs; run repair before init rerun or review manually"})
	}
	return nil
}

func claudeOrchestratorSkillTemplates() ([]templateFile, error) {
	const prefix = "templates/claude/skills/burpvalve-orchestrator"
	var files []templateFile
	err := fs.WalkDir(embeddedTemplates, prefix, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		target := strings.TrimPrefix(path, "templates/claude/")
		target = ".claude/" + strings.TrimSuffix(target, ".tmpl")
		mode := os.FileMode(0o644)
		files = append(files, templateFile{Source: path, Target: target, Mode: mode})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Target < files[j].Target })
	return files, nil
}

func isGeneratedClaudeOrchestratorBootstrap(body []byte) bool {
	text := string(body)
	return strings.Contains(text, "# Claude Orchestrator Route") &&
		strings.Contains(text, "Burpvalve Claude orchestrator route") &&
		strings.Contains(text, ".claude/skills/burpvalve-orchestrator/SKILL.md")
}

func ensurePreCommitHook(root string, result *ApplyResult) error {
	body, err := embeddedTemplates.ReadFile("templates/githooks/pre-commit")
	if err != nil {
		return err
	}
	const rel = ".githooks/pre-commit"
	path := filepath.Join(root, rel)
	info, err := os.Lstat(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := preserveLegacyGitHook(root, result); err != nil {
			return err
		}
		return createFileIfMissing(root, rel, body, 0o755, result)
	}
	if info.IsDir() {
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: rel, Message: "path exists as a directory"})
		return nil
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(existing) == string(body) {
		if err := preserveLegacyGitHook(root, result); err != nil {
			return err
		}
		if info.Mode().Perm()&0o111 == 0 {
			if err := os.Chmod(path, info.Mode().Perm()|0o755); err != nil {
				return err
			}
			result.Created = append(result.Created, rel+" executable bit")
			return nil
		}
		result.Preserved = append(result.Preserved, rel)
		return nil
	}
	if isGeneratedHook(existing) {
		if err := preserveLegacyGitHook(root, result); err != nil {
			return err
		}
		if err := os.WriteFile(path, body, 0o755); err != nil {
			return err
		}
		if err := os.Chmod(path, 0o755); err != nil {
			return err
		}
		result.Created = append(result.Created, rel+" dispatcher")
		return nil
	}
	userRel := ".githooks/pre-commit.user"
	userPath := filepath.Join(root, userRel)
	if _, err := os.Lstat(userPath); err == nil {
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: userRel, Message: "preserved user hook path already exists; explicit confirmation required before wrapping"})
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(userPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(path, userPath); err != nil {
		return err
	}
	if err := os.Chmod(userPath, info.Mode().Perm()|0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, body, 0o755); err != nil {
		return err
	}
	result.Created = append(result.Created, rel+" dispatcher")
	result.Preserved = append(result.Preserved, userRel)
	return nil
}

func preserveLegacyGitHook(root string, result *ApplyResult) error {
	legacyRel := ".git/hooks/pre-commit"
	legacyPath := filepath.Join(root, legacyRel)
	info, err := os.Lstat(legacyPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: legacyRel, Message: "legacy git hook path is a directory"})
		return nil
	}
	userRel := ".githooks/pre-commit.user"
	userPath := filepath.Join(root, userRel)
	if _, err := os.Lstat(userPath); err == nil {
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: userRel, Message: "legacy .git/hooks/pre-commit exists but preserved user hook path already exists"})
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(userPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(legacyPath, userPath); err != nil {
		return err
	}
	if err := os.Chmod(userPath, info.Mode().Perm()|0o755); err != nil {
		return err
	}
	result.Preserved = append(result.Preserved, userRel)
	return nil
}

func isGeneratedHook(body []byte) bool {
	text := string(body)
	return strings.Contains(text, "BURPVALVE=(") &&
		strings.Contains(text, "\"${BURPVALVE[@]}\" commit") &&
		strings.Contains(text, "\"${BURPVALVE[@]}\" lint")
}

func configureHooksPath(root string, runner Runner, looker Looker, result *ApplyResult) error {
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return nil
	}
	if legacyGitHookPresent(root) {
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: ".git/hooks/pre-commit", Message: "legacy hook still exists; preserve it before enabling .githooks"})
		return nil
	}
	if check := burpvalveToolCheck(root, runner, looker); check.Status != StatusOK && check.Status != StatusPresent {
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: "burpvalve", Message: "runnable burpvalve is required before enabling .githooks: " + check.Message})
		return nil
	}
	if _, err := exec.LookPath("git"); err != nil {
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: ".git/config", Message: "git executable unavailable; cannot configure core.hooksPath"})
		return nil
	}
	current := exec.Command("git", "config", "--get", "core.hooksPath")
	current.Dir = root
	output, err := current.Output()
	value := strings.TrimSpace(string(output))
	if err == nil && value == ".githooks" {
		result.Preserved = append(result.Preserved, "git config core.hooksPath")
		return nil
	}
	if err == nil && value != "" && value != ".githooks" {
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: ".git/config", Message: "core.hooksPath already set to " + value})
		return nil
	}
	cmd := exec.Command("git", "config", "core.hooksPath", ".githooks")
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git config core.hooksPath .githooks: %w: %s", err, strings.TrimSpace(string(output)))
	}
	result.Created = append(result.Created, "git config core.hooksPath")
	return nil
}

func legacyGitHookPresent(root string) bool {
	_, err := os.Lstat(filepath.Join(root, ".git/hooks/pre-commit"))
	return err == nil
}

func (r ApplyResult) Text() string {
	return r.TextWithOptions(TextOptions{})
}

func (r ApplyResult) TextWithOptions(opts TextOptions) string {
	var b strings.Builder
	ui := cliui.New(opts.Color)
	fmt.Fprintf(&b, "%s %s\n", ui.Title("Init result for"), ui.Path(r.TargetRoot))
	writeTwoColumnSection(&b, "summary", "field", "value", []reportRow{
		{Left: "mutating", Right: ui.Bool(r.Mutating)},
		{Left: "claude route", Right: emptyFallback(r.ClaudeRoute, "none")},
		{Left: "route source", Right: emptyFallback(r.ClaudeRouteSource, "unknown")},
		{Left: "created", Right: fmt.Sprint(len(r.Created))},
		{Left: "preserved", Right: fmt.Sprint(len(r.Preserved))},
		{Left: "skipped", Right: fmt.Sprint(len(r.Skipped))},
		{Left: "conflicts", Right: fmt.Sprint(len(r.Conflicts))},
	}, opts)
	if r.Config != nil {
		writeConfigSummary(&b, *r.Config, opts)
	}
	writeListWithOptions(&b, "created", r.Created, opts)
	writeListWithOptions(&b, "repaired", r.Repaired, opts)
	writeListWithOptions(&b, "preserved", r.Preserved, opts)
	writeListWithOptions(&b, "skipped", r.Skipped, opts)
	if len(r.Conflicts) == 0 {
		writeTwoColumnSection(&b, "conflicts", "path", "message", []reportRow{
			{Left: "-", Right: "none"},
		}, opts)
		return b.String()
	}
	var conflicts []reportRow
	for _, conflict := range r.Conflicts {
		conflicts = append(conflicts, reportRow{Left: ui.Error(conflict.Path), Right: conflict.Message})
	}
	writeTwoColumnSection(&b, "conflicts", "path", "message", conflicts, opts)
	return b.String()
}

func writeList(b *strings.Builder, title string, values []string) {
	writeListWithOptions(b, title, values, TextOptions{})
}

func writeListWithOptions(b *strings.Builder, title string, values []string, opts TextOptions) {
	ui := cliui.New(opts.Color)
	if len(values) == 0 {
		writeTwoColumnSection(b, strings.ToLower(title), "item", "status", []reportRow{
			{Left: "-", Right: "none"},
		}, opts)
		return
	}
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	fmt.Fprintf(b, "%s\n", ui.Section(strings.ToLower(title)))
	for _, value := range values {
		fmt.Fprintf(b, "  %s %s\n", ui.Muted("-"), ui.Path(value))
	}
}
