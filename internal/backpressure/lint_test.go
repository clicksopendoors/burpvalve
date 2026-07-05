package backpressure

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRunLintSkipsWishlistWhenNoExecutableCommandsDeclared(t *testing.T) {
	root := lintFixtureProject(t, "lint_commands: []\n")
	result, err := RunLint(context.Background(), LintOptions{Root: root})
	if err != nil {
		t.Fatalf("RunLint returned error: %v", err)
	}
	if result.Status != LintStatusNotEnforced {
		t.Fatalf("status = %q", result.Status)
	}
	if result.Enforced || result.CommandCount != 0 || result.EvidenceStrength != "none" || result.EnforcementLevel != "scaffold-only" {
		t.Fatalf("no-op lint should not look enforced: %#v", result)
	}
	if len(result.Commands) != 0 {
		t.Fatalf("wishlist should not execute commands: %#v", result.Commands)
	}
	if !hasSkipped(result, "lint-rules-wishlist") {
		t.Fatalf("wishlist skip not reported: %#v", result.Skipped)
	}
	summary := PrintLintSummary(result)
	if strings.Contains(summary, "burpvalve lint passed") || !strings.Contains(summary, "burpvalve lint not enforced") {
		t.Fatalf("no-op summary should not claim lint passed:\n%s", summary)
	}
	if !strings.Contains(summary, "Skipped wishlist or placeholders") {
		t.Fatalf("summary did not print skipped section:\n%s", summary)
	}
}

func TestRunLintExecutesPassingDeclaredCommandWithPathContext(t *testing.T) {
	root := lintFixtureProject(t, `lint_commands:
  - id: env-paths
    command: "test \"$BACKPRESSURE_LINT_PATHS\" = \"src:tests\""
    required: true
    paths: ["src", "tests"]
    timeout_seconds: 5
`)
	result, err := RunLint(context.Background(), LintOptions{Root: root})
	if err != nil {
		t.Fatalf("RunLint returned error: %v result=%#v", err, result)
	}
	if len(result.Commands) != 1 {
		t.Fatalf("commands = %#v", result.Commands)
	}
	if !result.Enforced || result.CommandCount != 1 || result.EvidenceStrength != "command-output" || result.EnforcementLevel != "required-and-optional-commands" {
		t.Fatalf("declared command should be enforced evidence: %#v", result)
	}
	if result.Commands[0].Status != LintStatusPassed {
		t.Fatalf("command status = %#v", result.Commands[0])
	}
	if result.Commands[0].Paths[0] != "src" || result.Commands[0].Paths[1] != "tests" {
		t.Fatalf("paths not preserved: %#v", result.Commands[0].Paths)
	}
}

func TestRunLintCommandUsesRunDirectoryAndRootRelativePaths(t *testing.T) {
	root := lintFixtureProject(t, `lint_commands:
  - id: web-lint
    run_directory: apps/web
    command: "test \"$(basename \"$PWD\")\" = web && test \"$BACKPRESSURE_LINT_PATHS\" = apps/web"
    required: true
    paths: ["apps/web"]
    timeout_seconds: 5
`)
	writeFile(t, root, "apps/web/package.json", "{}\n")
	result, err := RunLint(context.Background(), LintOptions{Root: root})
	if err != nil {
		t.Fatalf("RunLint returned error: %v result=%#v", err, result)
	}
	if len(result.Commands) != 1 {
		t.Fatalf("commands = %#v", result.Commands)
	}
	got := result.Commands[0]
	if got.Status != LintStatusPassed || got.RunDirectory != "apps/web" {
		t.Fatalf("run_directory command result = %#v", got)
	}
}

func TestRunLintMissingRunDirectoryFailsCommand(t *testing.T) {
	root := lintFixtureProject(t, `lint_commands:
  - id: missing-dir
    run_directory: apps/missing
    command: "true"
    required: true
    paths: ["apps/missing"]
    timeout_seconds: 5
`)
	result, err := RunLint(context.Background(), LintOptions{Root: root})
	if err == nil {
		t.Fatalf("missing run_directory should fail lint mode: %#v", result)
	}
	if len(result.Commands) != 1 || result.Commands[0].Status != LintStatusFailed || !strings.Contains(result.Commands[0].Error, "run_directory \"apps/missing\" does not exist") {
		t.Fatalf("missing run_directory not reported clearly: %#v", result)
	}
}

func TestRunLintPreservesLegacyCdCommandsAlongsideRunDirectory(t *testing.T) {
	root := lintFixtureProject(t, `lint_commands:
  - id: legacy-cd
    command: "cd apps/web && test -f package.json"
    required: true
    paths: ["apps/web"]
    timeout_seconds: 5
  - id: scoped
    run_directory: apps/web
    command: "test -f package.json"
    required: true
    paths: ["apps/web"]
    timeout_seconds: 5
`)
	writeFile(t, root, "apps/web/package.json", "{}\n")
	result, err := RunLint(context.Background(), LintOptions{Root: root})
	if err != nil {
		t.Fatalf("RunLint returned error: %v result=%#v", err, result)
	}
	if len(result.Commands) != 2 || result.Commands[0].Status != LintStatusPassed || result.Commands[1].Status != LintStatusPassed {
		t.Fatalf("legacy and scoped commands should both pass: %#v", result.Commands)
	}
}

func TestRunLintOptionalFailureDoesNotFail(t *testing.T) {
	root := lintFixtureProject(t, `lint_commands:
  - id: optional-style
    command: "echo optional >&2; exit 7"
    required: false
    paths: ["."]
    timeout_seconds: 5
`)
	result, err := RunLint(context.Background(), LintOptions{Root: root})
	if err != nil {
		t.Fatalf("optional failure should not fail lint mode: %v result=%#v", err, result)
	}
	if result.Status != StatusPassed || result.Commands[0].Status != LintStatusFailed || result.Commands[0].ExitCode != 7 {
		t.Fatalf("optional failure not reported correctly: %#v", result)
	}
	if strings.Join(result.AdvisoryFailures, ",") != "optional-style" {
		t.Fatalf("optional failure should be advisory: %#v", result.AdvisoryFailures)
	}
	summary := PrintLintSummary(result)
	if !strings.Contains(summary, "advisory") || !strings.Contains(summary, "optional-style failed (does not block)") {
		t.Fatalf("summary missing advisory failure:\n%s", summary)
	}
}

func TestRunLintOptionalPassDoesNotReportAdvisoryFailure(t *testing.T) {
	root := lintFixtureProject(t, `lint_commands:
  - id: optional-style
    command: "true"
    required: false
    paths: ["."]
    timeout_seconds: 5
`)
	result, err := RunLint(context.Background(), LintOptions{Root: root})
	if err != nil {
		t.Fatalf("optional pass should not fail lint mode: %v result=%#v", err, result)
	}
	if result.Status != StatusPassed || len(result.AdvisoryFailures) != 0 {
		t.Fatalf("optional pass should not report advisory failures: %#v", result)
	}
}

func TestRunLintRequiredFailureFails(t *testing.T) {
	root := lintFixtureProject(t, `lint_commands:
  - id: required-style
    command: "echo required >&2; exit 9"
    required: true
    paths: ["."]
    timeout_seconds: 5
`)
	result, err := RunLint(context.Background(), LintOptions{Root: root})
	if err == nil {
		t.Fatalf("required failure should fail lint mode: %#v", result)
	}
	if result.Status != StatusBlocked || !strings.Contains(result.Message, "required-style") {
		t.Fatalf("required failure not reported correctly: %#v", result)
	}
	if !strings.Contains(PrintLintSummary(result), "Declared command results") {
		t.Fatalf("summary did not print command results:\n%s", PrintLintSummary(result))
	}
}

func TestRunLintRequiredTimeoutFails(t *testing.T) {
	root := lintFixtureProject(t, `lint_commands:
  - id: slow-required
    command: "sleep 2"
    required: true
    paths: ["."]
    timeout_seconds: 1
`)
	result, err := RunLint(context.Background(), LintOptions{Root: root})
	if err == nil {
		t.Fatalf("required timeout should fail lint mode: %#v", result)
	}
	if result.Commands[0].Status != LintStatusTimeout || !strings.Contains(result.Commands[0].Error, "timeout after 1s") {
		t.Fatalf("timeout not reported correctly: %#v", result.Commands[0])
	}
}

func TestRunLintParallelResultsRemainInManifestOrder(t *testing.T) {
	root := lintFixtureProject(t, `lint_commands:
  - id: slow
    command: "sleep 0.2; echo slow"
    required: true
    paths: ["."]
    timeout_seconds: 5
  - id: fast
    command: "echo fast"
    required: true
    paths: ["."]
    timeout_seconds: 5
`)
	result, err := RunLint(context.Background(), LintOptions{Root: root, Jobs: 2})
	if err != nil {
		t.Fatalf("RunLint returned error: %v result=%#v", err, result)
	}
	if got := lintCommandIDs(result.Commands); strings.Join(got, ",") != "slow,fast" {
		t.Fatalf("commands not in manifest order: %#v", got)
	}
	if !strings.Contains(result.Commands[0].Stdout, "slow") || !strings.Contains(result.Commands[1].Stdout, "fast") {
		t.Fatalf("stdout not preserved per command: %#v", result.Commands)
	}
}

func TestRunLintParallelReportsMultipleFailures(t *testing.T) {
	root := lintFixtureProject(t, `lint_commands:
  - id: fail-one
    command: "echo one >&2; exit 11"
    required: true
    paths: ["."]
    timeout_seconds: 5
  - id: fail-two
    command: "echo two >&2; exit 12"
    required: true
    paths: ["."]
    timeout_seconds: 5
`)
	result, err := RunLint(context.Background(), LintOptions{Root: root, Jobs: 2})
	if err == nil {
		t.Fatalf("required failures should fail lint mode: %#v", result)
	}
	if len(result.Commands) != 2 || result.Commands[0].ExitCode != 11 || result.Commands[1].ExitCode != 12 {
		t.Fatalf("multiple failures not preserved: %#v", result.Commands)
	}
	if !strings.Contains(result.Message, "fail-one") || !strings.Contains(result.Message, "fail-two") {
		t.Fatalf("message should include both failures: %q", result.Message)
	}
}

func TestRunLintParallelTimeoutDoesNotCancelOtherCommands(t *testing.T) {
	root := lintFixtureProject(t, `lint_commands:
  - id: slow-timeout
    command: "sleep 2"
    required: false
    paths: ["."]
    timeout_seconds: 1
  - id: fast-pass
    command: "sleep 1.2; echo done"
    required: true
    paths: ["."]
    timeout_seconds: 5
`)
	result, err := RunLint(context.Background(), LintOptions{Root: root, Jobs: 2})
	if err != nil {
		t.Fatalf("optional timeout should not fail lint mode: %v result=%#v", err, result)
	}
	if result.Commands[0].Status != LintStatusTimeout || result.Commands[1].Status != LintStatusPassed {
		t.Fatalf("timeout should not cancel sibling command: %#v", result.Commands)
	}
	if !strings.Contains(result.Commands[1].Stdout, "done") {
		t.Fatalf("sibling command output missing: %#v", result.Commands[1])
	}
}

func TestRunLintSerialCommandsRunAfterParallelBatch(t *testing.T) {
	root := lintFixtureProject(t, "")
	marker := filepath.Join(root, "order.txt")
	writeFile(t, root, ManifestPath, `conditions:
  - id: lint-rules
    path: backpressure/lint-rules.md
    enabled: true
lint_commands:
  - id: serial-first
    command: 'test -f `+strconv.Quote(marker)+` && printf s >> `+strconv.Quote(marker)+`'
    required: true
    paths: ["."]
    timeout_seconds: 5
    serial: true
  - id: parallel-second
    command: 'sleep 0.1; printf p >> `+strconv.Quote(marker)+`'
    required: true
    paths: ["."]
    timeout_seconds: 5
`)
	result, err := RunLint(context.Background(), LintOptions{Root: root, Jobs: 2})
	if err != nil {
		t.Fatalf("RunLint returned error: %v result=%#v", err, result)
	}
	body, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if string(body) != "ps" {
		t.Fatalf("serial command should run after parallel batch, marker=%q result=%#v", body, result.Commands)
	}
	if got := lintCommandIDs(result.Commands); strings.Join(got, ",") != "serial-first,parallel-second" {
		t.Fatalf("results should remain in manifest order: %#v", got)
	}
}

func TestRunLintJobsOneRunsSerially(t *testing.T) {
	root := lintFixtureProject(t, "")
	marker := filepath.Join(root, "serial.txt")
	writeFile(t, root, ManifestPath, `conditions:
  - id: lint-rules
    path: backpressure/lint-rules.md
    enabled: true
lint_commands:
  - id: first
    command: 'sleep 0.1; printf 1 >> `+strconv.Quote(marker)+`'
    required: true
    paths: ["."]
    timeout_seconds: 5
  - id: second
    command: 'test "$(cat `+strconv.Quote(marker)+`)" = 1 && printf 2 >> `+strconv.Quote(marker)+`'
    required: true
    paths: ["."]
    timeout_seconds: 5
`)
	result, err := RunLint(context.Background(), LintOptions{Root: root, Jobs: 1})
	if err != nil {
		t.Fatalf("RunLint returned error: %v result=%#v", err, result)
	}
	body, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if string(body) != "12" {
		t.Fatalf("jobs=1 should run commands serially in manifest order, marker=%q", body)
	}
}

func TestRunLintDefaultParallelismIsBounded(t *testing.T) {
	root := lintFixtureProject(t, `lint_commands:
  - id: one
    command: "sleep 0.35"
    required: true
    paths: ["."]
    timeout_seconds: 5
  - id: two
    command: "sleep 0.35"
    required: true
    paths: ["."]
    timeout_seconds: 5
`)
	start := time.Now()
	result, err := RunLint(context.Background(), LintOptions{Root: root})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("RunLint returned error: %v result=%#v", err, result)
	}
	if elapsed > 900*time.Millisecond {
		t.Fatalf("default jobs should run independent commands concurrently, elapsed=%s", elapsed)
	}
}

func TestRunLintRejectsMalformedConfig(t *testing.T) {
	tests := []struct {
		name     string
		commands string
		want     string
	}{
		{
			name: "scalar lint command",
			commands: `lint_commands:
  - ./scripts/check-structure.sh
`,
			want: "lint_commands[0] must be an object",
		},
		{
			name: "number lint command",
			commands: `lint_commands:
  - 42
`,
			want: "lint_commands[0] must be an object",
		},
		{
			name: "null lint command",
			commands: `lint_commands:
  - null
`,
			want: "lint_commands[0] must be an object",
		},
		{
			name: "missing id",
			commands: `lint_commands:
  - command: "true"
    required: true
    paths: ["."]
    timeout_seconds: 5
`,
			want: "missing id",
		},
		{
			name: "string paths",
			commands: `lint_commands:
  - id: bad-paths
    command: "true"
    required: true
    paths: "."
    timeout_seconds: 5
`,
			want: "paths must be a list",
		},
		{
			name: "string required",
			commands: `lint_commands:
  - id: bad-required
    command: "true"
    required: maybe
    paths: ["."]
    timeout_seconds: 5
`,
			want: "required must be a boolean",
		},
		{
			name: "string timeout",
			commands: `lint_commands:
  - id: bad-timeout
    command: "true"
    required: true
    paths: ["."]
    timeout_seconds: "soon"
`,
			want: "timeout_seconds must be an integer",
		},
		{
			name: "unknown key",
			commands: `lint_commands:
  - id: extra-field
    command: "true"
    required: true
    paths: ["."]
    timeout_seconds: 5
    run_dir: subdir
`,
			want: "run_dir is not supported",
		},
		{
			name: "run directory must be string",
			commands: `lint_commands:
  - id: bad-run-dir
    command: "true"
    required: true
    paths: ["."]
    timeout_seconds: 5
    run_directory: 12
`,
			want: "run_directory must be a string",
		},
		{
			name: "serial must be bool",
			commands: `lint_commands:
  - id: bad-serial
    command: "true"
    required: true
    paths: ["."]
    timeout_seconds: 5
    serial: maybe
`,
			want: "serial must be a boolean",
		},
		{
			name: "absolute run directory",
			commands: `lint_commands:
  - id: abs-run-dir
    command: "true"
    required: true
    paths: ["."]
    timeout_seconds: 5
    run_directory: /tmp
`,
			want: "must be repo-relative",
		},
		{
			name: "traversing run directory",
			commands: `lint_commands:
  - id: traversal-run-dir
    command: "true"
    required: true
    paths: ["."]
    timeout_seconds: 5
    run_directory: ../outside
`,
			want: "must stay inside the repository",
		},
		{
			name: "missing paths",
			commands: `lint_commands:
  - id: no-paths
    command: "true"
    required: true
    timeout_seconds: 5
`,
			want: "missing paths",
		},
		{
			name: "bad timeout",
			commands: `lint_commands:
  - id: bad-timeout
    command: "true"
    required: true
    paths: ["."]
    timeout_seconds: 0
`,
			want: "timeout_seconds must be positive",
		},
		{
			name: "lint coverage unknown key",
			commands: `lint_commands: []
lint_coverage:
  declined_roots: ["apps/web"]
  reason: "later"
`,
			want: "lint_coverage.reason is not supported",
		},
		{
			name: "lint coverage declined roots must be list",
			commands: `lint_commands: []
lint_coverage:
  declined_roots: apps/web
`,
			want: "lint_coverage.declined_roots must be a list",
		},
		{
			name: "lint coverage declined root traversal",
			commands: `lint_commands: []
lint_coverage:
  declined_roots: ["../apps"]
`,
			want: "must stay inside the repository",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := lintFixtureProject(t, tt.commands)
			result, err := RunLint(context.Background(), LintOptions{Root: root})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q result=%#v", err, tt.want, result)
			}
			if result.Status != StatusBlocked || result.Message == "" {
				t.Fatalf("malformed config should return blocked result with message: %#v", result)
			}
			if !strings.Contains(result.Message, ManifestPath) && strings.Contains(tt.want, "lint_commands") {
				t.Fatalf("schema error should name manifest path: %#v", result)
			}
		})
	}
}

func TestRunLintReportsPartialCoverage(t *testing.T) {
	root := lintFixtureProject(t, `lint_commands: []
lint_coverage:
  declined_roots: ["apps/web", "services/api"]
  declined_at: "2026-07-02"
`)
	result, err := RunLint(context.Background(), LintOptions{Root: root})
	if err != nil {
		t.Fatalf("RunLint returned error: %v", err)
	}
	if result.Coverage != "partial" || strings.Join(result.UncheckedRoots, ",") != "apps/web,services/api" {
		t.Fatalf("partial coverage not reported: %#v", result)
	}
	summary := PrintLintSummary(result)
	if !strings.Contains(summary, "coverage") || !strings.Contains(summary, "apps/web,services/api") {
		t.Fatalf("summary missing partial coverage:\n%s", summary)
	}
}

func TestRunLintPartialCoverageDoesNotWeakenCommandEvidence(t *testing.T) {
	root := lintFixtureProject(t, `lint_commands:
  - id: go-test
    command: "true"
    required: true
    paths: ["."]
    timeout_seconds: 5
lint_coverage:
  declined_roots: ["apps/web"]
  declined_at: "2026-07-02"
`)
	result, err := RunLint(context.Background(), LintOptions{Root: root})
	if err != nil {
		t.Fatalf("RunLint returned error: %v result=%#v", err, result)
	}
	if result.Status != StatusPassed || !result.Enforced || result.EvidenceStrength != "command-output" {
		t.Fatalf("partial coverage should not weaken command evidence: %#v", result)
	}
	if result.Coverage != "partial" || strings.Join(result.UncheckedRoots, ",") != "apps/web" {
		t.Fatalf("partial coverage missing: %#v", result)
	}
	summary := PrintLintSummary(result)
	if !strings.Contains(summary, "coverage") || !strings.Contains(summary, "apps/web") {
		t.Fatalf("summary missing partial coverage:\n%s", summary)
	}
}

func TestRunLintSkipsPlaceholderCommand(t *testing.T) {
	root := lintFixtureProject(t, `lint_commands:
  - id: placeholder
    command: "TBD"
    required: false
    paths: ["."]
    timeout_seconds: 5
`)
	result, err := RunLint(context.Background(), LintOptions{Root: root})
	if err != nil {
		t.Fatalf("placeholder should be skipped, not executed: %v", err)
	}
	if len(result.Commands) != 0 || !hasSkipped(result, "placeholder") {
		t.Fatalf("placeholder command not skipped: %#v", result)
	}
}

func lintFixtureProject(t *testing.T, commands string) string {
	t.Helper()
	root := fixtureProject(t)
	writeFile(t, root, ManifestPath, `conditions:
  - id: lint-rules
    path: backpressure/lint-rules.md
    enabled: true
`+commands)
	return root
}

func hasSkipped(result LintResult, id string) bool {
	for _, skipped := range result.Skipped {
		if skipped.ID == id {
			return true
		}
	}
	return false
}

func lintCommandIDs(commands []LintCommandResult) []string {
	ids := make([]string, len(commands))
	for i, command := range commands {
		ids[i] = command.ID
	}
	return ids
}
