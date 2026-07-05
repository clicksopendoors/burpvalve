package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"burpvalve/internal/scaffold"
)

func TestBeadsCloseDeliveryStopsAfterGateWithoutConfirmation(t *testing.T) {
	root := fixtureGitRepo(t)
	responses := writeBeadsCloseResponses(t, root)
	logPath := installFakeCloseBR(t, "in_progress")

	stdout, stderr, err := executeBurpvalveCommand("beads", "close", "--root", root, "--json", "--reason", "Complete br-delivery", "--responses", responses, "br-delivery")
	if err != nil {
		t.Fatalf("beads close should stop nonfatally before commit: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stderr, "br close br-delivery --reason") && !strings.Contains(stderr, "/br close br-delivery --reason") {
		t.Fatalf("br close command was not printed before execution:\n%s", stderr)
	}
	if !strings.Contains(stderr, "running: git add .beads/issues.jsonl") {
		t.Fatalf("mutating commands were not printed before execution:\n%s", stderr)
	}
	var got beadsCloseResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode beads close result: %v\n%s", err, stdout)
	}
	if got.Status != "awaiting_commit_confirmation" || got.Fatal || !got.Partial {
		t.Fatalf("unexpected close result: %#v", got)
	}
	if got.JournalPath != "log/backpressure/closures/br-delivery.json" {
		t.Fatalf("journal path = %q", got.JournalPath)
	}
	if !hasCloseStep(got.Steps, "commit-gate", "waiting") || !hasCloseStep(got.Steps, "stage-attestation", "done") || !hasCloseStep(got.Steps, "commit-gate-revalidate", "done") {
		t.Fatalf("attestation bounce/revalidate steps missing: %#v", got.Steps)
	}
	if !hasRecoveryCommand(got.NextSteps, "git commit -m") {
		t.Fatalf("missing exact git commit next step: %#v", got.NextSteps)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(got.JournalPath))); err != nil {
		t.Fatalf("journal not written: %v", err)
	}
	staged := runOutput(t, root, "git", "diff", "--cached", "--name-only")
	if !strings.Contains(staged, ".beads/issues.jsonl") || !strings.Contains(staged, "backpressure/attestations/") {
		t.Fatalf("expected .beads and attestation staged, got:\n%s", staged)
	}
	brLog := readTestFile(t, logPath)
	if strings.Contains(brLog, "--force") || strings.Contains(brLog, "--bypass") {
		t.Fatalf("br command used forbidden bypass flag:\n%s", brLog)
	}
}

func TestBeadsCloseMissingBRBlocksWithResumeStep(t *testing.T) {
	root := fixtureGitRepo(t)
	t.Setenv("PATH", t.TempDir())

	stdout, stderr, err := executeBurpvalveCommand("beads", "close", "--root", root, "--json", "--reason", "Complete br-delivery", "br-delivery")
	if err == nil {
		t.Fatalf("missing br should block\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	var got beadsCloseResult
	if decodeErr := json.Unmarshal([]byte(stdout), &got); decodeErr != nil {
		t.Fatalf("decode missing br result: %v\n%s", decodeErr, stdout)
	}
	if got.Status != "blocked" || !got.Fatal || !hasRecoveryCommand(got.NextSteps, "burpvalve beads close br-delivery --resume") {
		t.Fatalf("missing br result wrong: %#v", got)
	}
}

func TestRobotsBeadsCloseHelpDocumentsStateMachine(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("--robots", "beads", "close", "-h")
	if err != nil {
		t.Fatalf("robot help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var got robotHelpDoc
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode robot help: %v\n%s", err, stdout)
	}
	if !robotHelpHasFlag(got.Flags, "--reason") || !robotHelpHasFlag(got.Flags, "--responses") || !robotHelpHasFlag(got.Flags, "--resume") || !robotHelpHasFlag(got.Flags, "--yes") {
		t.Fatalf("beads close robot help missing flags: %#v", got.Flags)
	}
	body := stdout
	for _, want := range []string{"commit_message", "attestation-written bounce", "partial_success", "journal_path"} {
		if !strings.Contains(body, want) {
			t.Fatalf("robot help missing %q:\n%s", want, body)
		}
	}
}

func TestRobotsBeadsCloseReadsBeadIDsFromStdin(t *testing.T) {
	root := fixtureGitRepo(t)
	responses := writeBeadsCloseResponses(t, root)
	installFakeCloseBR(t, "in_progress")
	input := `{"root":` + strconv.Quote(root) + `,"bead_ids":["br-delivery"],"reason":"Complete br-delivery","responses_path":` + strconv.Quote(responses) + `}`

	previousColorMode := colorMode
	previousRobotsMode := robotsMode
	colorMode = "auto"
	robotsMode = false
	defer func() {
		colorMode = previousColorMode
		robotsMode = previousRobotsMode
	}()

	stdout, stderr, err := executeBurpvalveCommandWithInput(input, "--robots", "beads", "close")
	if err != nil {
		t.Fatalf("robot stdin close should use bead_ids without positional args: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var got beadsCloseResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode robot close result: %v\n%s", err, stdout)
	}
	if got.Status != "awaiting_commit_confirmation" || got.Fatal || !got.Partial {
		t.Fatalf("unexpected robot close result: %#v", got)
	}
	if len(got.BeadIDs) != 1 || got.BeadIDs[0] != "br-delivery" {
		t.Fatalf("robot bead ids = %#v", got.BeadIDs)
	}
}

func TestBeadsCloseCloseSucceededSyncFailed(t *testing.T) {
	root := fixtureGitRepo(t)
	responses := writeBeadsCloseResponses(t, root)
	installFakeCloseBRScript(t, `#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  show) printf '[{"id":"%s","title":"Delivery bead","status":"in_progress"}]' "$2" ;;
  close) exit 0 ;;
  sync) echo sync blocked >&2; exit 7 ;;
esac
`)

	stdout, stderr, err := runBeadsCloseJSON(root, responses, "br-delivery")
	if err == nil {
		t.Fatalf("sync failure should fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	got := requireBeadsCloseState(t, stdout, "failed", true, true)
	requireCloseStep(t, got, "br-close-br-delivery", "done")
	requireCloseStep(t, got, "br-sync", "failed")
	requireStructuredResume(t, got, "burpvalve beads close br-delivery --resume")
	requireJournalStep(t, root, got.JournalPath, "br-sync", "failed")
}

func TestBeadsCloseSyncSucceededGateBlocked(t *testing.T) {
	root := fixtureGitRepo(t)
	installFakeCloseBR(t, "in_progress")
	responses := filepath.Join(root, "responses.json")
	writeCLIFile(t, root, "responses.json", `{
  "atomicity": {"one_feature_or_fix": true, "message": "Staged changes map only to br-delivery."},
  "conditions": [
    {"condition_id": "dry", "subagent_confirmed": true, "verdict": "fail", "evidence": ["dry failed"]},
    {"condition_id": "scope-control", "subagent_confirmed": true, "verdict": "pass", "evidence": ["scope ok"]}
  ]
}`)

	stdout, stderr, err := runBeadsCloseJSON(root, responses, "br-delivery")
	if err == nil {
		t.Fatalf("gate failure should fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	got := requireBeadsCloseState(t, stdout, "failed", true, true)
	requireCloseStep(t, got, "br-sync", "done")
	requireCloseStep(t, got, "stage-beads", "done")
	requireCloseStep(t, got, "commit-gate", "failed")
	requireStructuredResume(t, got, "burpvalve beads close br-delivery --resume")
	requireJournalStep(t, root, got.JournalPath, "commit-gate", "failed")
}

func TestBeadsCloseAttestationWrittenUnstagedNonfatal(t *testing.T) {
	root := fixtureGitRepo(t)
	responses := writeBeadsCloseResponses(t, root)
	installFakeCloseBR(t, "in_progress")

	stdout, stderr, err := runBeadsCloseJSON(root, responses, "br-delivery")
	if err != nil {
		t.Fatalf("attestation bounce should be nonfatal\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	got := requireBeadsCloseState(t, stdout, "awaiting_commit_confirmation", false, true)
	requireCloseStep(t, got, "commit-gate", "waiting")
	requireCloseStep(t, got, "stage-attestation", "done")
	requireCloseStep(t, got, "commit-gate-revalidate", "done")
	requireStructuredResume(t, got, "burpvalve beads close br-delivery --resume")
	requireJournalStep(t, root, got.JournalPath, "commit-gate", "waiting")
}

func TestBeadsCloseAttestationStagedPayloadChanged(t *testing.T) {
	root := fixtureGitRepo(t)
	responses := writeBeadsCloseResponses(t, root)
	installFakeCloseBR(t, "in_progress")
	stageStaleBeadsCloseAttestation(t, root, responses)

	stdout, stderr, err := runBeadsCloseJSON(root, responses, "br-delivery")
	if err == nil {
		t.Fatalf("stale attestation should fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	got := requireBeadsCloseState(t, stdout, "failed", true, true)
	requireCloseStep(t, got, "commit-gate", "failed")
	if !strings.Contains(strings.Join(got.Warnings, "\n"), "regenerated") {
		t.Fatalf("stale attestation warning should mention regeneration: %#v", got.Warnings)
	}
	requireStructuredResume(t, got, "burpvalve beads close br-delivery --resume")
}

func TestBeadsCloseGitCommitFailed(t *testing.T) {
	root := fixtureGitRepo(t)
	responses := writeBeadsCloseResponses(t, root)
	installFakeCloseBR(t, "in_progress")
	run(t, root, "git", "config", "user.email", "michael-bltzr@users.noreply.github.com")
	run(t, root, "git", "config", "user.name", "Test User")
	writeCLIFile(t, root, ".git/hooks/pre-commit", "#!/usr/bin/env bash\necho commit blocked >&2\nexit 9\n")
	if err := os.Chmod(filepath.Join(root, ".git/hooks/pre-commit"), 0o755); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := executeBurpvalveCommand("beads", "close", "--root", root, "--json", "--reason", "Complete br-delivery", "--responses", responses, "--yes", "--message", "Close br-delivery", "br-delivery")
	if err == nil {
		t.Fatalf("commit failure should fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	got := requireBeadsCloseState(t, stdout, "failed", true, true)
	requireCloseStep(t, got, "commit-gate-revalidate", "done")
	requireCloseStep(t, got, "git-commit", "failed")
	if !strings.Contains(strings.Join(got.Warnings, "\n"), "commit blocked") {
		t.Fatalf("commit blocker missing from warnings: %#v", got.Warnings)
	}
	requireStructuredResume(t, got, "burpvalve beads close br-delivery --resume")
}

func TestBeadsCloseAlreadyClosedSkipsBRClose(t *testing.T) {
	root := fixtureGitRepo(t)
	responses := writeBeadsCloseResponses(t, root)
	logPath := installFakeCloseBR(t, "closed")

	stdout, stderr, err := runBeadsCloseJSON(root, responses, "br-delivery")
	if err != nil {
		t.Fatalf("already closed bead should continue\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	got := requireBeadsCloseState(t, stdout, "awaiting_commit_confirmation", false, true)
	requireCloseStep(t, got, "br-close:br-delivery", "skipped")
	requireCloseStep(t, got, "br-sync", "done")
	brLog := readTestFile(t, logPath)
	if strings.Contains(brLog, "close br-delivery") {
		t.Fatalf("already closed bead should not run br close:\n%s", brLog)
	}
}

func TestBeadsCloseMissingBRResumeFixture(t *testing.T) {
	root := fixtureGitRepo(t)
	t.Setenv("PATH", t.TempDir())

	stdout, stderr, err := executeBurpvalveCommand("beads", "close", "--root", root, "--json", "--reason", "Complete br-delivery", "br-delivery")
	if err == nil {
		t.Fatalf("missing br should fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	got := requireBeadsCloseState(t, stdout, "blocked", true, false)
	requireCloseStep(t, got, "preflight", "failed")
	requireStructuredResume(t, got, "burpvalve beads close br-delivery --resume")
	if !strings.Contains(strings.Join(got.Warnings, "\n"), "br executable not found") {
		t.Fatalf("missing br warning absent: %#v", got.Warnings)
	}
}

func TestBeadsCloseUnrelatedDirtyDoesNotStage(t *testing.T) {
	root := fixtureGitRepo(t)
	responses := writeBeadsCloseResponses(t, root)
	installFakeCloseBR(t, "in_progress")
	writeCLIFile(t, root, "notes/unrelated.txt", "do not stage me\n")

	stdout, stderr, err := runBeadsCloseJSON(root, responses, "br-delivery")
	if err != nil {
		t.Fatalf("unrelated dirty file should warn but continue\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	got := requireBeadsCloseState(t, stdout, "awaiting_commit_confirmation", false, true)
	if !strings.Contains(strings.Join(got.Preflight.Warnings, "\n"), "notes/unrelated.txt") {
		t.Fatalf("unrelated dirty file missing from preflight warnings: %#v", got.Preflight.Warnings)
	}
	staged := runOutput(t, root, "git", "diff", "--cached", "--name-only")
	if strings.Contains(staged, "notes/unrelated.txt") {
		t.Fatalf("unrelated dirty file was staged:\n%s", staged)
	}
}

func TestBeadsCloseAdminOnlyBatchSkipsVerifierEvidence(t *testing.T) {
	root := fixtureGitRepo(t)
	logPath := installFakeCloseBR(t, "in_progress")
	run(t, root, "git", "config", "user.email", "michael-bltzr@users.noreply.github.com")
	run(t, root, "git", "config", "user.name", "Test User")
	run(t, root, "git", "commit", "-m", "Initial delivery payload")

	stdout, stderr, err := executeBurpvalveCommand("beads", "close", "--root", root, "--admin-only", "--json", "--reason", "Batch admin tracker cleanup", "--yes", "--message", "Close admin beads", "admin-one", "admin-two")
	if err != nil {
		t.Fatalf("admin-only batch should close and commit without verifier evidence: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	got := requireBeadsCloseState(t, stdout, "completed", false, true)
	if !got.Preflight.AdminOnly || got.Preflight.Classification != "admin" {
		t.Fatalf("admin close preflight wrong: %#v", got.Preflight)
	}
	requireCloseStep(t, got, "br-close-admin-one", "done")
	requireCloseStep(t, got, "br-close-admin-two", "done")
	requireCloseStep(t, got, "br-sync", "done")
	requireCloseStep(t, got, "stage-beads", "done")
	requireCloseStep(t, got, "git-commit", "done")
	if hasCloseStep(got.Steps, "commit-gate", "done") || hasCloseStep(got.Steps, "commit-gate", "waiting") || hasCloseStep(got.Steps, "stage-attestation", "done") {
		t.Fatalf("admin-only close should not run code verifier gate or attestation steps: %#v", got.Steps)
	}
	staged := runOutput(t, root, "git", "diff", "--cached", "--name-only")
	if strings.TrimSpace(staged) != "" {
		t.Fatalf("admin-only committed flow should leave index clean, got:\n%s", staged)
	}
	if entries := runOutput(t, root, "git", "log", "--oneline", "-1"); !strings.Contains(entries, "Close admin beads") {
		t.Fatalf("admin-only commit missing:\n%s", entries)
	}
	brLog := readTestFile(t, logPath)
	if strings.Contains(brLog, "--force") || strings.Contains(brLog, "--bypass") {
		t.Fatalf("admin br command used forbidden bypass flag:\n%s", brLog)
	}
}

func TestBeadsCloseAdminOnlyRejectsDeliveryPayload(t *testing.T) {
	root := fixtureGitRepo(t)
	writeCLIFile(t, root, "docs/admin.md", "not tracker-only\n")
	run(t, root, "git", "add", "docs/admin.md")
	installFakeCloseBR(t, "in_progress")

	stdout, stderr, err := executeBurpvalveCommand("beads", "close", "--root", root, "--admin-only", "--json", "--reason", "Admin cleanup", "admin-one")
	if err == nil {
		t.Fatalf("admin-only delivery payload should fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	got := requireBeadsCloseState(t, stdout, "blocked", true, false)
	requireCloseStep(t, got, "preflight", "failed")
	if !strings.Contains(strings.Join(got.Warnings, "\n"), "docs/admin.md") {
		t.Fatalf("admin-only failure should name non-Beads staged path: %#v", got.Warnings)
	}
}

func TestBeadsCloseMultiBeadDeliveryRecordsRationale(t *testing.T) {
	root := fixtureGitRepo(t)
	responses := writeBeadsCloseResponses(t, root)
	logPath := installFakeCloseBR(t, "in_progress")

	stdout, stderr, err := executeBurpvalveCommand("beads", "close", "--root", root, "--json", "--reason", "Complete coupled delivery", "--bead-rationale", "same staged payload", "--responses", responses, "br-one", "br-two")
	if err != nil {
		t.Fatalf("multi-bead delivery should stop after verified gate without commit confirmation: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	got := requireBeadsCloseState(t, stdout, "awaiting_commit_confirmation", false, true)
	if got.Preflight.CoupledWorkRationale != "same staged payload" || len(got.BeadIDs) != 2 {
		t.Fatalf("multi-bead rationale missing: %#v", got)
	}
	requireCloseStep(t, got, "br-close-br-one", "done")
	requireCloseStep(t, got, "br-close-br-two", "done")
	requireCloseStep(t, got, "br-sync", "done")
	requireCloseStep(t, got, "commit-gate-revalidate", "done")
	body := readOnlyAttestation(t, root)
	for _, want := range []string{`"bead_ids"`, `"br-one"`, `"br-two"`, `"coupled_work_rationale"`, "same staged payload"} {
		if !strings.Contains(body, want) {
			t.Fatalf("multi-bead attestation missing %q:\n%s", want, body)
		}
	}
	brLog := readTestFile(t, logPath)
	closeOne := strings.Index(brLog, "close br-one")
	closeTwo := strings.Index(brLog, "close br-two")
	syncAt := strings.Index(brLog, "sync --flush-only")
	if closeOne < 0 || closeTwo < 0 || syncAt < 0 || closeOne > syncAt || closeTwo > syncAt {
		t.Fatalf("multi-bead close-before-sync order wrong:\n%s", brLog)
	}
	if strings.Contains(brLog, "--force") || strings.Contains(brLog, "--bypass") {
		t.Fatalf("multi-bead br command used forbidden bypass flag:\n%s", brLog)
	}
}

func TestBeadsCloseMultiBeadDeliveryRequiresRationale(t *testing.T) {
	root := fixtureGitRepo(t)
	installFakeCloseBR(t, "in_progress")

	stdout, stderr, err := executeBurpvalveCommand("beads", "close", "--root", root, "--json", "--reason", "Complete coupled delivery", "br-one", "br-two")
	if err == nil {
		t.Fatalf("multi-bead delivery without rationale should fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	got := requireBeadsCloseState(t, stdout, "blocked", true, false)
	if !strings.Contains(strings.Join(got.Warnings, "\n"), "--bead-rationale is required") {
		t.Fatalf("multi-bead rationale warning missing: %#v", got.Warnings)
	}
}

func writeBeadsCloseResponses(t *testing.T, root string) string {
	t.Helper()
	responses := filepath.Join(root, "responses.json")
	writeCLIFile(t, root, "responses.json", `{
  "atomicity": {"one_feature_or_fix": true, "message": "Staged changes map only to br-delivery."},
  "conditions": [
    {"condition_id": "dry", "subagent_confirmed": true, "verdict": "pass", "evidence": ["dry ok"]},
    {"condition_id": "scope-control", "subagent_confirmed": true, "verdict": "pass", "evidence": ["scope ok"]}
  ]
}`)
	return responses
}

func installFakeCloseBR(t *testing.T, status string) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "br.log")
	path := filepath.Join(dir, "br")
	writeBeadsExecutable(t, path, `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> `+shellQuote(logPath)+`
case "$1" in
  show)
    printf '[{"id":"%s","title":"Delivery bead","status":"`+status+`"}]' "$2"
    ;;
  close)
    exit 0
    ;;
  sync)
    mkdir -p .beads
    printf '{"id":"br-delivery","status":"closed"}\n' > .beads/issues.jsonl
    ;;
  *)
    echo "unexpected br args: $*" >&2
    exit 2
    ;;
esac
`)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func installFakeCloseBRScript(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "br")
	writeBeadsExecutable(t, path, script)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return path
}

func runBeadsCloseJSON(root, responses, bead string) (string, string, error) {
	return executeBurpvalveCommand("beads", "close", "--root", root, "--json", "--reason", "Complete "+bead, "--responses", responses, bead)
}

func requireBeadsCloseState(t *testing.T, stdout, status string, fatal, partial bool) beadsCloseResult {
	t.Helper()
	var got beadsCloseResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode beads close result: %v\n%s", err, stdout)
	}
	if got.Status != status || got.Fatal != fatal || got.Partial != partial {
		t.Fatalf("unexpected close result: status=%s fatal=%t partial=%t full=%#v", got.Status, got.Fatal, got.Partial, got)
	}
	return got
}

func requireCloseStep(t *testing.T, got beadsCloseResult, id, status string) {
	t.Helper()
	if !hasCloseStep(got.Steps, id, status) {
		t.Fatalf("missing close step %s=%s in %#v", id, status, got.Steps)
	}
}

func requireStructuredResume(t *testing.T, got beadsCloseResult, commandPart string) {
	t.Helper()
	if len(got.NextSteps) == 0 {
		t.Fatalf("missing structured next steps")
	}
	if !hasRecoveryCommand(got.NextSteps, commandPart) {
		t.Fatalf("missing resume command containing %q: %#v", commandPart, got.NextSteps)
	}
	for _, step := range got.NextSteps {
		if strings.TrimSpace(step.ID) == "" || strings.TrimSpace(step.Message) == "" || strings.TrimSpace(step.Command) == "" {
			t.Fatalf("next step is not structured: %#v", step)
		}
	}
}

func requireJournalStep(t *testing.T, root, journalPath, id, status string) {
	t.Helper()
	body := readTestFile(t, filepath.Join(root, filepath.FromSlash(journalPath)))
	var journal beadsCloseJournal
	if err := json.Unmarshal([]byte(body), &journal); err != nil {
		t.Fatalf("decode journal %s: %v\n%s", journalPath, err, body)
	}
	if !hasCloseStep(journal.Steps, id, status) {
		t.Fatalf("journal missing step %s=%s: %#v", id, status, journal.Steps)
	}
}

func stageStaleBeadsCloseAttestation(t *testing.T, root, responses string) {
	t.Helper()
	stdout, _, err := executeBurpvalveCommand("commit", "--root", root, "--feature", "br-delivery", "--bead", "br-delivery", "--responses", responses, "--agent", "test", "--model", "test")
	if err == nil {
		t.Fatalf("commit should request attestation staging before stale setup:\n%s", stdout)
	}
	matches, err := filepath.Glob(filepath.Join(root, "backpressure", "attestations", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one generated attestation, got %d: %#v", len(matches), matches)
	}
	rel, err := filepath.Rel(root, matches[0])
	if err != nil {
		t.Fatal(err)
	}
	run(t, root, "git", "add", filepath.ToSlash(rel))
	writeCLIFile(t, root, "src/app.go", "package app\n\nconst ChangedAfterAttestation = true\n")
	run(t, root, "git", "add", "src/app.go")
}

func readOnlyAttestation(t *testing.T, root string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, "backpressure", "attestations", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one generated attestation, got %d: %#v", len(matches), matches)
	}
	return readTestFile(t, matches[0])
}

func hasCloseStep(steps []beadsCloseJournalStep, id, status string) bool {
	for _, step := range steps {
		if step.ID == id && step.Status == status {
			return true
		}
	}
	return false
}

func hasRecoveryCommand(steps []scaffold.RecoveryStep, want string) bool {
	for _, step := range steps {
		if strings.Contains(step.Command, want) {
			return true
		}
	}
	return false
}
