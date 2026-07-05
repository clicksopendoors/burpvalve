package backpressure

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"burpvalve/internal/attestations"
	"burpvalve/internal/cliui"
	"burpvalve/internal/lintconfig"
	"gopkg.in/yaml.v3"
)

type LintOptions struct {
	Root string
	Jobs int
}

type LintResult struct {
	SchemaVersion    int                 `json:"schema_version"`
	Command          string              `json:"command"`
	Status           string              `json:"status"`
	Message          string              `json:"message"`
	Fatal            bool                `json:"fatal"`
	NextSteps        []string            `json:"next_steps,omitempty"`
	Enforced         bool                `json:"enforced"`
	CommandCount     int                 `json:"command_count"`
	EvidenceStrength string              `json:"evidence_strength"`
	EnforcementLevel string              `json:"enforcement_level"`
	Coverage         string              `json:"coverage"`
	UncheckedRoots   []string            `json:"unchecked_roots,omitempty"`
	AdvisoryFailures []string            `json:"advisory_failures,omitempty"`
	Commands         []LintCommandResult `json:"commands"`
	Skipped          []LintSkipped       `json:"skipped"`
}

type LintCommandResult struct {
	ID             string   `json:"id"`
	Command        string   `json:"command"`
	Required       bool     `json:"required"`
	Paths          []string `json:"paths"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	RunDirectory   string   `json:"run_directory,omitempty"`
	Serial         bool     `json:"serial,omitempty"`
	Status         string   `json:"status"`
	ExitCode       int      `json:"exit_code,omitempty"`
	DurationMS     int64    `json:"duration_ms"`
	Stdout         string   `json:"stdout,omitempty"`
	Stderr         string   `json:"stderr,omitempty"`
	Error          string   `json:"error,omitempty"`
}

type LintSkipped struct {
	ID      string `json:"id"`
	Reason  string `json:"reason"`
	Context string `json:"context,omitempty"`
}

type TextOptions struct {
	Color bool
}

const (
	LintStatusPassed      = "passed"
	LintStatusFailed      = "failed"
	LintStatusSkipped     = "skipped"
	LintStatusTimeout     = "timeout"
	LintStatusNotEnforced = "not_enforced"
)

func RunLint(ctx context.Context, opts LintOptions) (LintResult, error) {
	root, err := filepath.Abs(defaultRoot(opts.Root))
	if err != nil {
		return LintResult{}, err
	}
	result := LintResult{
		SchemaVersion:    1,
		Command:          "lint",
		Status:           LintStatusNotEnforced,
		Message:          "no executable lint commands declared; lint-rules wishlist skipped",
		EvidenceStrength: "none",
		EnforcementLevel: "scaffold-only",
		Coverage:         "full",
		Skipped: []LintSkipped{{
			ID:      "lint-rules-wishlist",
			Reason:  "policy wishlist is not executable",
			Context: "backpressure/lint-rules.md rules are skipped unless represented by exact lint_commands entries in backpressure/manifest.yaml",
		}},
	}
	if err := validateLintManifestShape(root); err != nil {
		result.Status = StatusBlocked
		result.Message = err.Error()
		result.Fatal = true
		result.NextSteps = []string{"Fix backpressure/manifest.yaml so lint_commands is a list of executable command objects, then rerun burpvalve lint."}
		return result, err
	}
	manifest, _, err := LoadManifest(root)
	if err != nil {
		result.Status = StatusBlocked
		result.Message = err.Error()
		result.Fatal = true
		result.NextSteps = []string{"Fix backpressure/manifest.yaml so lint_commands is a list of executable command objects, then rerun burpvalve lint."}
		return result, err
	}
	applyLintCoverage(&result, manifest.LintCoverage)

	executable, skipped, err := executableLintCommands(manifest.LintCommands)
	result.Skipped = append(result.Skipped, skipped...)
	if err != nil {
		result.Status = StatusBlocked
		result.Message = err.Error()
		result.Fatal = true
		result.NextSteps = []string{"Fix backpressure/manifest.yaml lint_commands, then rerun burpvalve lint."}
		return result, err
	}
	result.CommandCount = len(executable)
	if len(executable) == 0 {
		result.NextSteps = []string{"Add exact executable lint_commands entries to backpressure/manifest.yaml when this project is ready for deterministic lint enforcement."}
		return result, nil
	}
	result.Status = StatusPassed
	result.Message = "declared lint commands passed"
	result.Enforced = true
	result.EvidenceStrength = "command-output"
	result.EnforcementLevel = "required-and-optional-commands"

	result.Commands = runLintCommands(ctx, root, executable, opts.Jobs)
	var failures []string
	for _, commandResult := range result.Commands {
		if commandResult.Status == LintStatusPassed {
			continue
		}
		if !commandResult.Required {
			result.AdvisoryFailures = append(result.AdvisoryFailures, commandResult.ID)
			continue
		}
		failures = append(failures, commandResult.ID+": "+commandResult.Error)
	}
	if len(failures) > 0 {
		result.Status = StatusBlocked
		result.Message = "required lint command failures: " + strings.Join(failures, "; ")
		result.Fatal = true
		result.NextSteps = []string{"Fix the failing required lint command output, then rerun burpvalve lint."}
		return result, errors.New(result.Message)
	}
	result.Message = "all required lint commands passed"
	return result, nil
}

func validateLintManifestShape(root string) error {
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ManifestPath)))
	if err != nil {
		return err
	}
	return validateManifestShape(body)
}

func validateManifestShape(body []byte) error {
	var doc yaml.Node
	if err := yaml.Unmarshal(body, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", ManifestPath, err)
	}
	if len(doc.Content) == 0 {
		return nil
	}
	rootNode := doc.Content[0]
	if rootNode.Kind != yaml.MappingNode {
		return nil
	}
	conditions := lintManifestMappingValue(rootNode, "conditions")
	if conditions != nil {
		if err := validateManifestConditionsShape(conditions); err != nil {
			return err
		}
	}
	lintCommands := lintManifestMappingValue(rootNode, "lint_commands")
	if lintCommands != nil {
		if lintCommands.Kind != yaml.SequenceNode {
			return fmt.Errorf("%s: lint_commands must be a list of objects", ManifestPath)
		}
		for i, item := range lintCommands.Content {
			if item.Kind != yaml.MappingNode {
				return fmt.Errorf("%s: lint_commands[%d] must be an object with id, command, required, paths, timeout_seconds, run_directory, and serial", ManifestPath, i)
			}
			if err := validateLintCommandShape(i, item); err != nil {
				return err
			}
		}
	}
	lintCoverage := lintManifestMappingValue(rootNode, "lint_coverage")
	if lintCoverage != nil {
		if err := validateLintCoverageShape(lintCoverage); err != nil {
			return err
		}
	}
	return nil
}

func validateManifestConditionsShape(node *yaml.Node) error {
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("%s: conditions must be a list of objects", ManifestPath)
	}
	for i, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			return fmt.Errorf("%s: conditions[%d] must be an object with id, path, enabled, and verifier_policy", ManifestPath, i)
		}
		for n := 0; n < len(item.Content)-1; n += 2 {
			key := item.Content[n].Value
			value := item.Content[n+1]
			switch key {
			case "id", "path", "verifier_policy":
				if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
					return fmt.Errorf("%s: conditions[%d].%s must be a string", ManifestPath, i, key)
				}
			case "enabled":
				if value.Kind != yaml.ScalarNode || value.Tag != "!!bool" {
					return fmt.Errorf("%s: conditions[%d].enabled must be a boolean", ManifestPath, i)
				}
			default:
				return fmt.Errorf("%s: conditions[%d].%s is not supported; expected id, path, enabled, and verifier_policy", ManifestPath, i, key)
			}
		}
	}
	return nil
}

func validateLintCommandShape(i int, item *yaml.Node) error {
	requiredOrder := []string{"id", "command", "required", "paths", "timeout_seconds"}
	required := map[string]bool{
		"id":              false,
		"command":         false,
		"required":        false,
		"paths":           false,
		"timeout_seconds": false,
	}
	for j := 0; j < len(item.Content)-1; j += 2 {
		key := item.Content[j].Value
		value := item.Content[j+1]
		switch key {
		case "id", "command":
			required[key] = true
			if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
				return fmt.Errorf("%s: lint_commands[%d].%s must be a string", ManifestPath, i, key)
			}
		case "required":
			required[key] = true
			if value.Kind != yaml.ScalarNode || value.Tag != "!!bool" {
				return fmt.Errorf("%s: lint_commands[%d].required must be a boolean", ManifestPath, i)
			}
		case "paths":
			required[key] = true
			if value.Kind != yaml.SequenceNode {
				return fmt.Errorf("%s: lint_commands[%d].paths must be a list of relative paths", ManifestPath, i)
			}
			for p, path := range value.Content {
				if path.Kind != yaml.ScalarNode || path.Tag != "!!str" {
					return fmt.Errorf("%s: lint_commands[%d].paths[%d] must be a string", ManifestPath, i, p)
				}
			}
		case "timeout_seconds":
			required[key] = true
			if value.Kind != yaml.ScalarNode || value.Tag != "!!int" {
				return fmt.Errorf("%s: lint_commands[%d].timeout_seconds must be an integer", ManifestPath, i)
			}
		case "run_directory":
			if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
				return fmt.Errorf("%s: lint_commands[%d].run_directory must be a string", ManifestPath, i)
			}
		case "serial":
			if value.Kind != yaml.ScalarNode || value.Tag != "!!bool" {
				return fmt.Errorf("%s: lint_commands[%d].serial must be a boolean", ManifestPath, i)
			}
		default:
			return fmt.Errorf("%s: lint_commands[%d].%s is not supported; expected id, command, required, paths, timeout_seconds, run_directory, and serial", ManifestPath, i, key)
		}
	}
	for _, key := range requiredOrder {
		if !required[key] {
			return fmt.Errorf("%s: lint_commands[%d] missing %s", ManifestPath, i, key)
		}
	}
	return nil
}

func validateLintCoverageShape(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("%s: lint_coverage must be an object", ManifestPath)
	}
	for i := 0; i < len(node.Content)-1; i += 2 {
		key := node.Content[i].Value
		value := node.Content[i+1]
		switch key {
		case "declined_roots":
			if value.Kind != yaml.SequenceNode {
				return fmt.Errorf("%s: lint_coverage.declined_roots must be a list of relative paths", ManifestPath)
			}
			for p, path := range value.Content {
				if path.Kind != yaml.ScalarNode || path.Tag != "!!str" {
					return fmt.Errorf("%s: lint_coverage.declined_roots[%d] must be a string", ManifestPath, p)
				}
			}
		case "declined_at":
			if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
				return fmt.Errorf("%s: lint_coverage.declined_at must be a string", ManifestPath)
			}
		default:
			return fmt.Errorf("%s: lint_coverage.%s is not supported; expected declined_roots and declined_at", ManifestPath, key)
		}
	}
	return nil
}

func lintManifestMappingValue(node *yaml.Node, key string) *yaml.Node {
	for i := 0; i < len(node.Content)-1; i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func executableLintCommands(commands []lintconfig.Command) ([]lintconfig.Command, []LintSkipped, error) {
	var executable []lintconfig.Command
	var skipped []LintSkipped
	seen := map[string]bool{}
	for i, command := range commands {
		id := strings.TrimSpace(command.ID)
		if id == "" {
			return nil, skipped, fmt.Errorf("lint_commands[%d] missing id", i)
		}
		if seen[id] {
			return nil, skipped, fmt.Errorf("duplicate lint command id %q", id)
		}
		seen[id] = true
		command.ID = id
		command.Command = strings.TrimSpace(command.Command)
		if isPlaceholderLintCommand(command.Command) {
			skipped = append(skipped, LintSkipped{
				ID:      command.ID,
				Reason:  "placeholder command is not executable",
				Context: "replace command \"TBD\" with an exact lint/format/static-analysis command to enforce it",
			})
			continue
		}
		if command.Command == "" {
			return nil, skipped, fmt.Errorf("lint command %q missing command", command.ID)
		}
		if len(command.Paths) == 0 {
			return nil, skipped, fmt.Errorf("lint command %q missing paths", command.ID)
		}
		for _, path := range command.Paths {
			if strings.TrimSpace(path) == "" {
				return nil, skipped, fmt.Errorf("lint command %q has empty path", command.ID)
			}
			if filepath.IsAbs(path) {
				return nil, skipped, fmt.Errorf("lint command %q path %q must be relative", command.ID, path)
			}
		}
		runDirectory, err := normalizeRepoRelativePath(command.RunDirectory, ".")
		if err != nil {
			return nil, skipped, fmt.Errorf("lint command %q run_directory %q is invalid: %w", command.ID, command.RunDirectory, err)
		}
		command.RunDirectory = runDirectory
		if command.TimeoutSeconds <= 0 {
			return nil, skipped, fmt.Errorf("lint command %q timeout_seconds must be positive", command.ID)
		}
		executable = append(executable, command)
	}
	return executable, skipped, nil
}

func runLintCommands(ctx context.Context, root string, commands []lintconfig.Command, jobs int) []LintCommandResult {
	results := make([]LintCommandResult, len(commands))
	if len(commands) == 0 {
		return results
	}
	if jobs <= 0 || jobs > len(commands) {
		jobs = len(commands)
		if jobs > 4 {
			jobs = 4
		}
	}
	if jobs == 1 {
		for i, command := range commands {
			results[i] = runLintCommand(ctx, root, command)
		}
		return results
	}

	type indexedCommand struct {
		index   int
		command lintconfig.Command
	}
	parallel := make(chan indexedCommand)
	var wg sync.WaitGroup
	for range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range parallel {
				results[item.index] = runLintCommand(ctx, root, item.command)
			}
		}()
	}
	for i, command := range commands {
		if command.Serial {
			continue
		}
		parallel <- indexedCommand{index: i, command: command}
	}
	close(parallel)
	wg.Wait()

	for i, command := range commands {
		if command.Serial {
			results[i] = runLintCommand(ctx, root, command)
		}
	}
	return results
}

func isPlaceholderLintCommand(command string) bool {
	switch strings.ToUpper(strings.TrimSpace(command)) {
	case "TBD", "TODO":
		return true
	default:
		return false
	}
}

func runLintCommand(parent context.Context, root string, command lintconfig.Command) LintCommandResult {
	result := LintCommandResult{
		ID:             command.ID,
		Command:        command.Command,
		Required:       command.Required,
		Paths:          append([]string(nil), command.Paths...),
		TimeoutSeconds: command.TimeoutSeconds,
		RunDirectory:   command.RunDirectory,
		Serial:         command.Serial,
		Status:         LintStatusPassed,
	}
	if result.RunDirectory == "" {
		result.RunDirectory = "."
	}
	runDir, err := lintCommandRunDirectory(root, result.RunDirectory)
	if err != nil {
		result.Status = LintStatusFailed
		result.Error = err.Error()
		return result
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(command.TimeoutSeconds)*time.Second)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, "sh", "-c", command.Command)
	cmd.Dir = runDir
	cmd.Env = append(os.Environ(), "BACKPRESSURE_LINT_PATHS="+strings.Join(command.Paths, string(os.PathListSeparator)))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	result.DurationMS = time.Since(start).Milliseconds()
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	if ctx.Err() == context.DeadlineExceeded {
		result.Status = LintStatusTimeout
		result.Error = fmt.Sprintf("timeout after %ds", command.TimeoutSeconds)
		return result
	}
	if err != nil {
		result.Status = LintStatusFailed
		result.Error = err.Error()
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		}
		return result
	}
	return result
}

func PrintLintSummary(result LintResult) string {
	return PrintLintSummaryWithOptions(result, TextOptions{})
}

func PrintLintSummaryWithOptions(result LintResult, opts TextOptions) string {
	var b strings.Builder
	ui := cliui.New(opts.Color)
	displayStatus := result.Status
	if result.Status == LintStatusNotEnforced || (result.Status == StatusPassed && !result.Enforced) {
		displayStatus = "not enforced"
	}
	fmt.Fprintf(&b, "%s %s\n", ui.Title(attestations.ToolName+" lint"), ui.Status(displayStatus))
	fmt.Fprintf(&b, "  %s %s\n", ui.Header("message"), result.Message)
	if result.Coverage == "partial" {
		fmt.Fprintf(&b, "  %s partial - unchecked roots: %s\n", ui.Header("coverage"), strings.Join(result.UncheckedRoots, ","))
	}
	for _, id := range result.AdvisoryFailures {
		fmt.Fprintf(&b, "  %s %s failed (does not block)\n", ui.Header("advisory"), id)
	}
	if len(result.Skipped) > 0 {
		fmt.Fprintf(&b, "\n%s\n", ui.Section("Skipped wishlist or placeholders"))
		fmt.Fprintf(&b, "  %s  %s\n", padStyled(ui.Header("id"), 24), ui.Header("reason"))
		fmt.Fprintf(&b, "  %s  %s\n", padStyled(ui.Muted("--"), 24), ui.Muted("------"))
		for _, skipped := range result.Skipped {
			reason := skipped.Reason
			if skipped.Context != "" {
				reason += ui.Muted(" (" + skipped.Context + ")")
			}
			fmt.Fprintf(&b, "  %s  %s\n", padStyled(ui.Warn(skipped.ID), 24), reason)
		}
	}
	if len(result.Commands) > 0 {
		fmt.Fprintf(&b, "\n%s\n", ui.Section("Declared command results"))
		fmt.Fprintf(&b, "  %s  %s  %s  %s\n", padStyled(ui.Header("id"), 24), padStyled(ui.Header("status"), 10), padStyled(ui.Header("required"), 8), ui.Header("paths"))
		fmt.Fprintf(&b, "  %s  %s  %s  %s\n", padStyled(ui.Muted("--"), 24), padStyled(ui.Muted("------"), 10), padStyled(ui.Muted("--------"), 8), ui.Muted("-----"))
		for _, command := range result.Commands {
			required := "no"
			if command.Required {
				required = "yes"
			}
			fmt.Fprintf(&b, "  %s  %s  %-8s  %s", padStyled(ui.Path(command.ID), 24), padStyled(ui.Status(command.Status), 10), required, strings.Join(command.Paths, ","))
			if command.Error != "" {
				fmt.Fprintf(&b, "  %s", ui.Error(command.Error))
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func applyLintCoverage(result *LintResult, coverage lintconfig.Coverage) {
	if len(coverage.DeclinedRoots) == 0 {
		result.Coverage = "full"
		result.UncheckedRoots = nil
		return
	}
	result.Coverage = "partial"
	result.UncheckedRoots = append([]string(nil), coverage.DeclinedRoots...)
}

func normalizeManifestLintFields(manifest *Manifest) error {
	for i := range manifest.LintCommands {
		if strings.TrimSpace(manifest.LintCommands[i].RunDirectory) == "" {
			continue
		}
		runDirectory, err := normalizeRepoRelativePath(manifest.LintCommands[i].RunDirectory, ".")
		if err != nil {
			return fmt.Errorf("lint command %q run_directory %q is invalid: %w", manifest.LintCommands[i].ID, manifest.LintCommands[i].RunDirectory, err)
		}
		manifest.LintCommands[i].RunDirectory = runDirectory
	}
	for i, root := range manifest.LintCoverage.DeclinedRoots {
		clean, err := normalizeRepoRelativePath(root, "")
		if err != nil {
			return fmt.Errorf("lint_coverage.declined_roots[%d] %q is invalid: %w", i, root, err)
		}
		manifest.LintCoverage.DeclinedRoots[i] = clean
	}
	return nil
}

func normalizeRepoRelativePath(path, defaultPath string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		if defaultPath == "" {
			return "", fmt.Errorf("must not be empty")
		}
		trimmed = defaultPath
	}
	if filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("must be repo-relative")
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(trimmed)))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("must stay inside the repository")
	}
	return clean, nil
}

func lintCommandRunDirectory(root, runDirectory string) (string, error) {
	clean, err := normalizeRepoRelativePath(runDirectory, ".")
	if err != nil {
		return "", fmt.Errorf("run_directory %q is invalid: %w", runDirectory, err)
	}
	dir := filepath.Join(root, filepath.FromSlash(clean))
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("run_directory %q does not exist", clean)
		}
		return "", fmt.Errorf("stat run_directory %q: %w", clean, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("run_directory %q is not a directory", clean)
	}
	return dir, nil
}

func padStyled(value string, width int) string {
	padding := width - cliui.VisibleLen(value)
	if padding <= 0 {
		return value
	}
	return value + strings.Repeat(" ", padding)
}
