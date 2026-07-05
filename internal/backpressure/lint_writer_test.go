package backpressure

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"burpvalve/internal/lintconfig"
)

func TestLintManifestWriterPreservesExistingManifestContent(t *testing.T) {
	root := fixtureProject(t)
	writeFile(t, root, ManifestPath, `# project-owned manifest comment
conditions:
  - id: dry
    path: backpressure/dry.md
    enabled: true
lint_commands:
  - id: existing
    command: "go test ./..."
    required: true
    paths: ["."]
    timeout_seconds: 30
`)
	result, err := WriteLintManifestUpdate(root, LintManifestUpdate{
		Commands: []LintCommandProposal{{
			Command: lintconfig.Command{
				ID:             "node-lint",
				Command:        "npm run lint",
				Required:       false,
				Paths:          []string{"apps/web"},
				TimeoutSeconds: 60,
				RunDirectory:   "apps/web",
				Serial:         true,
			},
		}},
		Coverage: lintconfig.Coverage{DeclinedRoots: []string{"legacy/app"}, DeclinedAt: "2026-07-02"},
	})
	if err != nil {
		t.Fatalf("WriteLintManifestUpdate returned error: %v", err)
	}
	if !result.Changed || strings.Join(result.Added, ",") != "node-lint" {
		t.Fatalf("writer result = %#v", result)
	}
	body := readFile(t, root, ManifestPath)
	for _, want := range []string{
		"# project-owned manifest comment",
		"conditions:",
		"id: dry",
		"id: existing",
		"id: node-lint",
		"run_directory: apps/web",
		"serial: true",
		"lint_coverage:",
		"declined_roots:",
		"- legacy/app",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("manifest missing %q:\n%s", want, body)
		}
	}
	if strings.Index(body, "conditions:") > strings.Index(body, "lint_commands:") {
		t.Fatalf("conditions should stay before lint_commands:\n%s", body)
	}
}

func TestLintManifestWriterIdempotentRerunZeroDiff(t *testing.T) {
	root := fixtureProject(t)
	writeFile(t, root, ManifestPath, "conditions: []\nlint_commands: []\n")
	update := LintManifestUpdate{
		Commands: []LintCommandProposal{{
			Command: lintconfig.Command{
				ID:             "go-test",
				Command:        "go test ./...",
				Required:       true,
				Paths:          []string{"."},
				TimeoutSeconds: 120,
			},
		}},
	}
	first, err := WriteLintManifestUpdate(root, update)
	if err != nil {
		t.Fatalf("first write returned error: %v", err)
	}
	second, err := WriteLintManifestUpdate(root, update)
	if err != nil {
		t.Fatalf("second write returned error: %v", err)
	}
	if !first.Changed || second.Changed {
		t.Fatalf("idempotency result first=%#v second=%#v", first, second)
	}
	if first.After != second.After {
		t.Fatalf("rerun should produce zero diff")
	}
}

func TestLintManifestWriterRejectsDuplicateProposalIDsBeforeWrite(t *testing.T) {
	root := fixtureProject(t)
	original := "conditions: []\nlint_commands: []\n"
	writeFile(t, root, ManifestPath, original)
	_, err := WriteLintManifestUpdate(root, LintManifestUpdate{
		Commands: []LintCommandProposal{
			{Command: lintconfig.Command{ID: "dup", Command: "true", Required: true, Paths: []string{"."}, TimeoutSeconds: 5}},
			{Command: lintconfig.Command{ID: "dup", Command: "go test ./...", Required: true, Paths: []string{"."}, TimeoutSeconds: 5}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate lint command proposal id") {
		t.Fatalf("error = %v", err)
	}
	if got := readFile(t, root, ManifestPath); got != original {
		t.Fatalf("manifest changed after rejected duplicate:\n%s", got)
	}
}

func TestLintManifestWriterRejectsDuplicateExistingCommandIDs(t *testing.T) {
	root := fixtureProject(t)
	original := `conditions: []
lint_commands:
  - id: dup
    command: "true"
    required: true
    paths: ["."]
    timeout_seconds: 5
  - id: dup
    command: "go test ./..."
    required: true
    paths: ["."]
    timeout_seconds: 30
`
	writeFile(t, root, ManifestPath, original)
	_, err := WriteLintManifestUpdate(root, LintManifestUpdate{
		Commands: []LintCommandProposal{{
			Command: lintconfig.Command{ID: "new-command", Command: "go test ./...", Required: true, Paths: []string{"."}, TimeoutSeconds: 30},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate existing lint command id") {
		t.Fatalf("error = %v", err)
	}
	if got := readFile(t, root, ManifestPath); got != original {
		t.Fatalf("manifest changed after duplicate existing IDs:\n%s", got)
	}
}

func TestLintManifestWriterConflictSkipUpdateRename(t *testing.T) {
	root := fixtureProject(t)
	writeFile(t, root, ManifestPath, `conditions: []
lint_commands:
  - id: go-test
    command: "go test ./..."
    required: true
    paths: ["."]
    timeout_seconds: 30
`)
	skipped, err := WriteLintManifestUpdate(root, LintManifestUpdate{
		Commands: []LintCommandProposal{{
			Command: lintconfig.Command{ID: "go-test", Command: "go test ./internal/...", Required: true, Paths: []string{"internal"}, TimeoutSeconds: 60},
		}},
	})
	if err != nil {
		t.Fatalf("skip write returned error: %v", err)
	}
	if skipped.Changed || strings.Join(skipped.Skipped, ",") != "go-test" {
		t.Fatalf("default conflict should skip with zero diff: %#v", skipped)
	}
	updated, err := WriteLintManifestUpdate(root, LintManifestUpdate{
		Commands: []LintCommandProposal{{
			Command:    lintconfig.Command{ID: "go-test", Command: "go test ./internal/...", Required: true, Paths: []string{"internal"}, TimeoutSeconds: 60},
			OnConflict: LintCommandConflictUpdate,
		}},
	})
	if err != nil {
		t.Fatalf("update write returned error: %v", err)
	}
	body := readFile(t, root, ManifestPath)
	if !updated.Changed || !strings.Contains(body, "go test ./internal/...") || strings.Contains(body, "timeout_seconds: 30") {
		t.Fatalf("update not applied result=%#v body=\n%s", updated, body)
	}
	renamed, err := WriteLintManifestUpdate(root, LintManifestUpdate{
		Commands: []LintCommandProposal{{
			Command:    lintconfig.Command{ID: "go-test", Command: "go test ./cmd/...", Required: false, Paths: []string{"cmd"}, TimeoutSeconds: 45},
			OnConflict: LintCommandConflictRename,
			RenameID:   "go-test-cmd",
		}},
	})
	if err != nil {
		t.Fatalf("rename write returned error: %v", err)
	}
	body = readFile(t, root, ManifestPath)
	if !renamed.Changed || strings.Join(renamed.Renamed, ",") != "go-test->go-test-cmd" || !strings.Contains(body, "id: go-test-cmd") {
		t.Fatalf("rename not applied result=%#v body=\n%s", renamed, body)
	}
}

func TestLintManifestWriterRejectsRobotUnknownConflictAction(t *testing.T) {
	root := fixtureProject(t)
	original := "conditions: []\nlint_commands: []\n"
	writeFile(t, root, ManifestPath, original)
	_, err := WriteLintManifestUpdate(root, LintManifestUpdate{
		Commands: []LintCommandProposal{{
			Command:    lintconfig.Command{ID: "go-test", Command: "go test ./...", Required: true, Paths: []string{"."}, TimeoutSeconds: 30},
			OnConflict: LintCommandConflictAction("merge"),
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported on_conflict") {
		t.Fatalf("error = %v", err)
	}
	if got := readFile(t, root, ManifestPath); got != original {
		t.Fatalf("manifest changed after rejected conflict action:\n%s", got)
	}
}

func TestLintRulesRecommendationsReplacementIsIdempotent(t *testing.T) {
	root := fixtureProject(t)
	writeFile(t, root, LintRulesPath, `# Lint Rules

Project-owned guidance stays here.

<!-- burpvalve:lint-init recommendations -->
## Lint Init Recommendations

- Old recommendation.
<!-- /burpvalve:lint-init recommendations -->
`)
	recommendations := []string{
		"Consider adding a real Go test command before claiming Go lint enforcement.",
		"Structural rules remain prose until a user enables an executable analyzer.",
	}
	first, err := WriteLintRulesRecommendationsUpdate(root, recommendations)
	if err != nil {
		t.Fatalf("recommendation write returned error: %v", err)
	}
	second, err := WriteLintRulesRecommendationsUpdate(root, recommendations)
	if err != nil {
		t.Fatalf("recommendation rerun returned error: %v", err)
	}
	body := readFile(t, root, LintRulesPath)
	if !first.Changed || second.Changed {
		t.Fatalf("recommendation idempotency first=%#v second=%#v", first, second)
	}
	if strings.Count(body, LintInitRecommendationsStartMarker) != 1 || strings.Contains(body, "Old recommendation") {
		t.Fatalf("recommendation section not replaced cleanly:\n%s", body)
	}
	if !strings.Contains(body, "Project-owned guidance stays here.") || !strings.Contains(body, "Structural rules remain prose") {
		t.Fatalf("recommendation body lost content:\n%s", body)
	}
}

func TestLintManifestWriterDoesNotLeavePartialManifestOnAtomicFailure(t *testing.T) {
	root := fixtureProject(t)
	path := filepath.Join(root, filepath.FromSlash(ManifestPath))
	original := "conditions: []\nlint_commands: []\n"
	writeFile(t, root, ManifestPath, original)
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove manifest file: %v", err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("replace manifest with dir: %v", err)
	}
	_, err := WriteLintManifestUpdate(root, LintManifestUpdate{
		Commands: []LintCommandProposal{{
			Command: lintconfig.Command{ID: "go-test", Command: "go test ./...", Required: true, Paths: []string{"."}, TimeoutSeconds: 30},
		}},
	})
	if err == nil {
		t.Fatalf("expected read/write failure with manifest path as directory")
	}
	entries, readErr := os.ReadDir(filepath.Join(root, "backpressure"))
	if readErr != nil {
		t.Fatalf("read backpressure dir: %v", readErr)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Fatalf("atomic writer left temp file %s after failure", entry.Name())
		}
	}
}

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(body)
}
