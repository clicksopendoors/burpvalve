package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"burpvalve/internal/attestations"
	"burpvalve/internal/backpressure"
)

func TestGateRunDryRunJSONContract(t *testing.T) {
	handoff := writeGateRunHandoff(t, t.TempDir(), "unsafe/../qhqa-run")
	stdout, stderr, err := executeBurpvalveCommand(
		"gate", "run",
		"--handoff", handoff,
		"--dry-run",
		"--json",
	)
	if err != nil {
		t.Fatalf("gate run dry-run failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("gate run dry-run wrote stderr: %s", stderr)
	}
	var got gateRunResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode gate run result: %v\n%s", err, stdout)
	}
	if got.Status != "planned" || got.Phase != "validated" || got.RunID != "qhqa-run" {
		t.Fatalf("unexpected gate run identity: %#v", got)
	}
	if got.CanonicalHandoffPath != filepath.Join("log", "backpressure", "gate-runs", "qhqa-run-handoff.json") {
		t.Fatalf("canonical handoff path wrong: %#v", got)
	}
	if got.JournalPath != filepath.Join("log", "backpressure", "gate-runs", "qhqa-run-journal.json") {
		t.Fatalf("journal path wrong: %#v", got)
	}
	if strings.Join(got.PlannedPushCommand, " ") != "git push origin main" {
		t.Fatalf("planned push command wrong: %#v", got.PlannedPushCommand)
	}
	if got.Mutating {
		t.Fatalf("dry-run must be read-only: %#v", got)
	}
}

func TestGateRunRobotsInlineHandoff(t *testing.T) {
	input := `{
  "dry_run": true,
  "confirm": true,
  "remote": "upstream",
  "branch": "release",
  "agent": "PurpleStream",
  "model": "gpt-5",
  "handoff": ` + gateRunHandoffJSON("robot-run") + `
}`
	stdout, stderr, err := executeBurpvalveCommandWithInput(input, "--robots", "gate", "run")
	if err != nil {
		t.Fatalf("robots gate run failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("robots gate run wrote stderr: %s", stderr)
	}
	var got gateRunResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode robots gate run: %v\n%s", err, stdout)
	}
	if got.Status != "planned" || got.Agent != "PurpleStream" || got.Model != "gpt-5" {
		t.Fatalf("unexpected robots gate run result: %#v", got)
	}
	if strings.Join(got.PlannedPushCommand, " ") != "git push upstream release" {
		t.Fatalf("robot remote/branch overrides not applied: %#v", got.PlannedPushCommand)
	}
}

func TestGateRunNonDryRunBlocksUntilExecutionUnits(t *testing.T) {
	handoff := writeGateRunHandoff(t, t.TempDir(), "qhqa-run")
	stdout, stderr, err := executeBurpvalveCommand("gate", "run", "--handoff", handoff, "--json")
	if err == nil {
		t.Fatalf("gate run execution should require confirmation\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	var got gateRunResult
	if decodeErr := json.Unmarshal([]byte(stdout), &got); decodeErr != nil {
		t.Fatalf("blocked gate run should still emit JSON: %v\n%s", decodeErr, stdout)
	}
	if got.Status != "blocked" || got.Phase != "confirmation_required" {
		t.Fatalf("unexpected blocked status: %#v", got)
	}
	if !containsGateRunString(got.NextSteps, "Run with --yes or robots confirm=true to execute the mutating gate-run ceremony.") {
		t.Fatalf("blocked result not actionable: %#v", got.NextSteps)
	}
}

func TestGateRunYesWritesCanonicalHandoffAndJournal(t *testing.T) {
	runInTempDir(t)
	handoff := writeGateRunHandoff(t, t.TempDir(), "unsafe/../journal-run")
	stdout, stderr, err := executeBurpvalveCommand("gate", "run", "--handoff", handoff, "--yes", "--json")
	if err == nil {
		t.Fatalf("gate run should stop on missing git HEAD\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("gate run wrote stderr: %s", stderr)
	}
	var got gateRunResult
	if decodeErr := json.Unmarshal([]byte(stdout), &got); decodeErr != nil {
		t.Fatalf("decode gate run result: %v\n%s", decodeErr, stdout)
	}
	if got.Status != "blocked" || got.Phase != "head_mismatch" || !got.PartialSuccess || !got.Mutating {
		t.Fatalf("unexpected journaled stop: %#v", got)
	}
	if got.HandoffHash == "" || got.JournalHash == "" {
		t.Fatalf("content hashes should be reported: %#v", got)
	}
	if _, statErr := os.Stat(got.CanonicalHandoffPath); statErr != nil {
		t.Fatalf("canonical handoff not written: %v", statErr)
	}
	body, readErr := os.ReadFile(got.JournalPath)
	if readErr != nil {
		t.Fatalf("journal not written: %v", readErr)
	}
	var journal gateRunJournal
	if err := json.Unmarshal(body, &journal); err != nil {
		t.Fatalf("decode journal: %v\n%s", err, body)
	}
	if journal.RunID != "journal-run" || journal.HandoffHash != got.HandoffHash || len(journal.Steps) != 3 {
		t.Fatalf("unexpected journal: %#v", journal)
	}
	if journal.Steps[2].ID != "git_head" || journal.Steps[2].Status != "blocked" {
		t.Fatalf("missing git-head stop in journal: %#v", journal.Steps)
	}
}

func TestGateRunYesStagesExactPathsAndHashesPayload(t *testing.T) {
	runInTempDir(t)
	initGateRunGitRepo(t)
	writeGateRunWorktreeFile(t, "cmd/burpvalve/main.go", "package main\n")
	writeGateRunWorktreeFile(t, "cmd/burpvalve/gate_run.go", "package main\nvar gateRun = true\n")
	head := runGateRunGit(t, "rev-parse", "HEAD")
	handoff := writeGateRunHandoffWithHead(t, t.TempDir(), "stage-run", head)
	stdout, stderr, err := executeBurpvalveCommand("gate", "run", "--handoff", handoff, "--yes", "--json")
	if err == nil {
		t.Fatalf("gate run should still stop before verifier dispatch\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("gate run wrote stderr: %s", stderr)
	}
	var got gateRunResult
	if decodeErr := json.Unmarshal([]byte(stdout), &got); decodeErr != nil {
		t.Fatalf("decode gate run result: %v\n%s", decodeErr, stdout)
	}
	if got.Status != "blocked" || got.Phase != "verifier_responses" || got.StagedPayloadHash == "" {
		t.Fatalf("unexpected gate-run preflight result: %#v", got)
	}
	if got.ResponsesPath == "" || got.VerifierPromptsPath == "" || len(got.PendingConditions) == 0 {
		t.Fatalf("verifier handoff details missing: %#v", got)
	}
	if got.ExecutableConditions == nil ||
		got.ExecutableConditions.Status != "skipped" ||
		got.ExecutableConditions.Mode != "serial" ||
		got.ExecutableConditions.CommandCount != 0 ||
		got.ExecutableConditions.Parallelism != 1 {
		t.Fatalf("executable condition seam should be skipped serial no-op by default: %#v", got.ExecutableConditions)
	}
	if strings.Join(got.StagedPaths, ",") != "cmd/burpvalve/gate_run.go,cmd/burpvalve/main.go" {
		t.Fatalf("staged paths not exact or sorted: %#v", got.StagedPaths)
	}
	if staged := runGateRunGit(t, "diff", "--cached", "--name-only"); strings.TrimSpace(staged) != strings.Join(got.StagedPaths, "\n") {
		t.Fatalf("git index does not match result staged paths:\n%s\n%#v", staged, got.StagedPaths)
	}
	body, readErr := os.ReadFile(got.JournalPath)
	if readErr != nil {
		t.Fatalf("journal not written: %v", readErr)
	}
	var journal gateRunJournal
	if err := json.Unmarshal(body, &journal); err != nil {
		t.Fatalf("decode journal: %v\n%s", err, body)
	}
	if journal.StagedPayloadHash != got.StagedPayloadHash ||
		!containsGateRunJournalStep(journal.Steps, "exact_staging", "completed") ||
		!containsGateRunJournalStep(journal.Steps, "executable_conditions", "skipped") ||
		!containsGateRunJournalStep(journal.Steps, "verifier_prompts", "completed") ||
		!containsGateRunJournalStep(journal.Steps, "verifier_responses", "blocked") {
		t.Fatalf("journal did not record exact staging: %#v", journal)
	}
}

func TestGateRunValidatedResponsesCommitAfterAttestationLoop(t *testing.T) {
	runInTempDir(t)
	initGateRunGitRepo(t)
	writeGateRunWorktreeFile(t, "cmd/burpvalve/main.go", "package main\n")
	writeGateRunWorktreeFile(t, "cmd/burpvalve/gate_run.go", "package main\nvar gateRun = true\n")
	head := runGateRunGit(t, "rev-parse", "HEAD")
	handoff := writeGateRunHandoffWithHead(t, t.TempDir(), "verified-run", head)
	stdout, _, err := executeBurpvalveCommand("gate", "run", "--handoff", handoff, "--yes", "--json")
	if err == nil {
		t.Fatalf("first gate run should stop for verifier cells\nstdout=%s", stdout)
	}
	var first gateRunResult
	if decodeErr := json.Unmarshal([]byte(stdout), &first); decodeErr != nil {
		t.Fatalf("decode first gate run result: %v\n%s", decodeErr, stdout)
	}
	if first.Phase != "verifier_responses" || first.ResponsesPath == "" {
		t.Fatalf("first run should write responses and block for cells: %#v", first)
	}
	completeGateRunResponses(t, first.ResponsesPath)
	stdout, stderr, err := executeBurpvalveCommand("gate", "run", "--handoff", handoff, "--resume", "--yes", "--json")
	if err != nil {
		t.Fatalf("verified gate run should commit after attestation loop: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("gate run wrote stderr: %s", stderr)
	}
	var got gateRunResult
	if decodeErr := json.Unmarshal([]byte(stdout), &got); decodeErr != nil {
		t.Fatalf("decode verified gate run result: %v\n%s", decodeErr, stdout)
	}
	if got.Status != "committed" || got.Phase != "post_commit" || len(got.PendingConditions) != 0 || len(got.BlockingConditions) != 0 {
		t.Fatalf("completed responses should commit locally: %#v", got)
	}
	if got.ExecutableConditions == nil ||
		got.ExecutableConditions.Status != "skipped" ||
		got.ExecutableConditions.Mode != "serial" ||
		got.ExecutableConditions.CommandCount != 0 {
		t.Fatalf("executable condition phase should stay serial no-op through commit: %#v", got.ExecutableConditions)
	}
	if !got.PushJournaled || strings.Join(got.PlannedPushCommand, " ") != "git push origin main" {
		t.Fatalf("push should be journaled but not executed: %#v", got)
	}
	if got.ReservationRelease.Status != "mcp_instruction" ||
		got.ReservationRelease.AgentMailIdentity != "PurpleStream" ||
		got.ReservationRelease.MCPTool != "release_file_reservations" ||
		!strings.Contains(got.ReservationRelease.Instruction, "project_key") {
		t.Fatalf("reservation release instruction missing: %#v", got.ReservationRelease)
	}
	if got.WakeRef != "burpvalve-qhqa.1" || !containsGateRunString(got.WakeInstructions, "burpvalve-qhqa.1") {
		t.Fatalf("wake instruction missing: %#v", got)
	}
	if len(got.AttestationPaths) != 1 || !strings.HasPrefix(got.AttestationPaths[0], "backpressure/attestations/") {
		t.Fatalf("generated attestation path missing: %#v", got.AttestationPaths)
	}
	if got.CurrentHead == "" || got.CurrentHead == head {
		t.Fatalf("current head should advance after commit: old=%s result=%#v", head, got)
	}
	if subject := runGateRunGit(t, "log", "-1", "--pretty=%s"); subject != "gate: add run command schema" {
		t.Fatalf("unexpected commit subject: %q", subject)
	}
	if staged := runGateRunGit(t, "diff", "--cached", "--name-only"); strings.TrimSpace(staged) != "" {
		t.Fatalf("index should be empty after local commit:\n%s", staged)
	}
	committedNames := runGateRunGit(t, "show", "--name-only", "--pretty=format:", "HEAD")
	for _, path := range []string{"cmd/burpvalve/main.go", "cmd/burpvalve/gate_run.go", got.AttestationPaths[0]} {
		if !strings.Contains(committedNames, path) {
			t.Fatalf("commit missing %s:\n%s", path, committedNames)
		}
	}
	body, readErr := os.ReadFile(got.JournalPath)
	if readErr != nil {
		t.Fatalf("journal not written: %v", readErr)
	}
	var journal gateRunJournal
	if err := json.Unmarshal(body, &journal); err != nil {
		t.Fatalf("decode journal: %v\n%s", err, body)
	}
	for _, step := range []string{"executable_conditions", "verifier_responses", "commit_gate", "stage_attestation", "commit_gate_revalidate", "git_commit", "push_journal", "release_reservations", "wake_handoff"} {
		if !containsGateRunJournalStep(journal.Steps, step, "completed") && step != "commit_gate" {
			if step == "executable_conditions" && containsGateRunJournalStep(journal.Steps, step, "skipped") {
				continue
			}
			t.Fatalf("journal did not record %s completion: %#v", step, journal.Steps)
		}
		if step == "commit_gate" && !containsGateRunJournalStep(journal.Steps, step, "waiting") && !containsGateRunJournalStep(journal.Steps, step, "completed") {
			t.Fatalf("journal did not record commit gate bounce/pass: %#v", journal.Steps)
		}
	}
	if !journal.PushJournaled ||
		strings.Join(journal.PlannedPushCommand, " ") != "git push origin main" ||
		journal.ReservationRelease.Status != "mcp_instruction" ||
		journal.WakeRef != "burpvalve-qhqa.1" {
		t.Fatalf("journal did not record post-commit handoff fields: %#v", journal)
	}
}

func TestGateRunBlocksDirtyIndexOutsideStagePaths(t *testing.T) {
	runInTempDir(t)
	initGateRunGitRepo(t)
	writeGateRunWorktreeFile(t, "cmd/burpvalve/main.go", "package main\n")
	writeGateRunWorktreeFile(t, "notes/unrelated.txt", "do not stage\n")
	runGateRunGit(t, "add", "notes/unrelated.txt")
	head := runGateRunGit(t, "rev-parse", "HEAD")
	handoff := writeGateRunHandoffWithHead(t, t.TempDir(), "dirty-index-run", head)
	stdout, _, err := executeBurpvalveCommand("gate", "run", "--handoff", handoff, "--yes", "--json")
	if err == nil {
		t.Fatalf("dirty index should block\nstdout=%s", stdout)
	}
	var got gateRunResult
	if decodeErr := json.Unmarshal([]byte(stdout), &got); decodeErr != nil {
		t.Fatalf("decode dirty-index result: %v\n%s", decodeErr, stdout)
	}
	if got.Phase != "dirty_index" || !containsGateRunString(got.DirtyIndexPaths, "notes/unrelated.txt") {
		t.Fatalf("unexpected dirty-index result: %#v", got)
	}
}

func TestGateRunBeadsCloseStagesIssuesBeforeHash(t *testing.T) {
	runInTempDir(t)
	initGateRunGitRepo(t)
	installFakeGateRunBR(t)
	writeGateRunWorktreeFile(t, "cmd/burpvalve/main.go", "package main\n")
	writeGateRunWorktreeFile(t, "cmd/burpvalve/gate_run.go", "package main\nvar gateRun = true\n")
	head := runGateRunGit(t, "rev-parse", "HEAD")
	body := strings.Replace(gateRunHandoffJSON("beads-run"), `"expected_head": "abc123"`, `"expected_head": "`+escapeJSONForTest(head)+`"`, 1)
	body = strings.Replace(body, `"bead_ids": ["burpvalve-qhqa.1"]`, `"bead_ids": ["br-delivery"]`, 1)
	body = strings.Replace(body, `"feature": "burpvalve-qhqa.1"`, `"feature": "br-delivery"`, 1)
	body = strings.Replace(body, `"close": false`, `"close": true`, 1)
	handoff := writeGateRunHandoffBody(t, t.TempDir(), body)
	stdout, _, err := executeBurpvalveCommand("gate", "run", "--handoff", handoff, "--yes", "--json")
	if err == nil {
		t.Fatalf("gate run should still stop before verifier dispatch\nstdout=%s", stdout)
	}
	var got gateRunResult
	if decodeErr := json.Unmarshal([]byte(stdout), &got); decodeErr != nil {
		t.Fatalf("decode gate run result: %v\n%s", decodeErr, stdout)
	}
	if got.Status != "blocked" || got.Phase != "verifier_responses" || got.StagedPayloadHash == "" {
		t.Fatalf("unexpected gate-run beads result: %#v", got)
	}
	if !containsGateRunString(got.StagedPaths, ".beads/issues.jsonl") {
		t.Fatalf("beads metadata was not staged before hash: %#v", got.StagedPaths)
	}
	if !containsGateRunString(got.Warnings, ".beads/issues.jsonl") {
		t.Fatalf("result should explain why beads metadata entered the payload: %#v", got.Warnings)
	}
	bodyBytes, readErr := os.ReadFile(got.JournalPath)
	if readErr != nil {
		t.Fatalf("journal not written: %v", readErr)
	}
	var journal gateRunJournal
	if err := json.Unmarshal(bodyBytes, &journal); err != nil {
		t.Fatalf("decode journal: %v\n%s", err, bodyBytes)
	}
	if !containsGateRunJournalStep(journal.Steps, "beads_close", "completed") {
		t.Fatalf("journal did not record beads close: %#v", journal.Steps)
	}
	staged := runGateRunGit(t, "diff", "--cached", "--name-only")
	if !strings.Contains(staged, ".beads/issues.jsonl") {
		t.Fatalf("git index missing beads metadata:\n%s", staged)
	}
}

func TestGateRunResumeReconcilesExistingJournal(t *testing.T) {
	runInTempDir(t)
	handoff := writeGateRunHandoff(t, t.TempDir(), "resume-run")
	if stdout, _, err := executeBurpvalveCommand("gate", "run", "--handoff", handoff, "--yes", "--json"); err == nil {
		t.Fatalf("initial gate run should stop after writing journal\nstdout=%s", stdout)
	}
	stdout, stderr, err := executeBurpvalveCommand("gate", "run", "--handoff", handoff, "--resume", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("resume dry-run failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("resume dry-run wrote stderr: %s", stderr)
	}
	var got gateRunResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode resume result: %v\n%s", err, stdout)
	}
	if !got.Resumed || got.PreviousJournalHash == "" || got.HandoffHash == "" {
		t.Fatalf("resume did not report reconciled journal state: %#v", got)
	}
	if !containsGateRunString(got.Warnings, "resume reconciled existing journal") {
		t.Fatalf("resume warning missing: %#v", got.Warnings)
	}
}

func TestGateRunRejectsUnsafeStagePath(t *testing.T) {
	handoff := gateRunHandoffJSON("bad-stage")
	handoff = strings.Replace(handoff, `"cmd/burpvalve/main.go"`, `"../outside"`, 1)
	input := `{"dry_run":true,"handoff":` + handoff + `}`
	stdout, _, err := executeBurpvalveCommandWithInput(input, "--robots", "gate", "run")
	if err == nil {
		t.Fatalf("unsafe stage path should block\nstdout=%s", stdout)
	}
	var got gateRunResult
	if decodeErr := json.Unmarshal([]byte(stdout), &got); decodeErr != nil {
		t.Fatalf("decode unsafe path blocker: %v\n%s", decodeErr, stdout)
	}
	if got.Status != "blocked" || got.Phase != "handoff_validation" || !containsGateRunString(got.NextSteps, "invalid path") {
		t.Fatalf("unexpected unsafe path blocker: %#v", got)
	}
}

func TestGateRunRobotsHelpDocumentsSchema(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("--robots", "gate", "run", "-h")
	if err != nil {
		t.Fatalf("gate run robots help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	for _, needle := range []string{
		`"command": "burpvalve gate run"`,
		"handoff_path",
		"inline handoff",
		"journal-push",
		"gate run stages only handoff-declared paths and the generated attestation path",
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("gate run robots help missing %q:\n%s", needle, stdout)
		}
	}
}

func writeGateRunHandoff(t *testing.T, dir, runID string) string {
	t.Helper()
	path := filepath.Join(dir, "handoff.json")
	if err := os.WriteFile(path, []byte(gateRunHandoffJSON(runID)), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeGateRunHandoffWithHead(t *testing.T, dir, runID, head string) string {
	t.Helper()
	body := strings.Replace(gateRunHandoffJSON(runID), `"expected_head": "abc123"`, `"expected_head": "`+escapeJSONForTest(head)+`"`, 1)
	return writeGateRunHandoffBody(t, dir, body)
}

func writeGateRunHandoffBody(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "handoff.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func completeGateRunResponses(t *testing.T, path string) {
	t.Helper()
	responses, err := backpressure.LoadResponses(path)
	if err != nil {
		t.Fatalf("load gate run responses: %v", err)
	}
	for i := range responses.Conditions {
		responses.Conditions[i].Verifier = attestations.Verifier{
			Kind:            attestations.VerifierIndependentSubagent,
			Agent:           "VerifierPeak",
			Model:           "unit-model",
			Runtime:         "unit test separate context",
			SeparateContext: true,
		}
		responses.Conditions[i].SubagentConfirmed = true
		responses.Conditions[i].SubagentModel = "unit-model"
		responses.Conditions[i].Verdict = attestations.VerdictPass
		responses.Conditions[i].Message = "verified in unit test"
		responses.Conditions[i].Evidence = []string{"focused unit-test evidence"}
		responses.Conditions[i].NextAction = "proceed"
	}
	body, err := json.MarshalIndent(responses, "", "  ")
	if err != nil {
		t.Fatalf("encode gate run responses: %v", err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		t.Fatalf("write gate run responses: %v", err)
	}
}

func gateRunHandoffJSON(runID string) string {
	return `{
  "schema_version": 1,
  "run_id": "` + escapeJSONForTest(runID) + `",
  "work_unit": {
    "kind": "single",
    "feature": "burpvalve-qhqa.1",
    "lane_id": "",
    "bead_ids": ["burpvalve-qhqa.1"],
    "rationale": ""
  },
  "authorization": {
    "kind": "orchestrator",
    "authority": "BronzeDeer",
    "audit_ref": "standing orders"
  },
  "git": {
    "expected_head": "abc123",
    "stage_paths": ["cmd/burpvalve/main.go", "cmd/burpvalve/gate_run.go"],
    "allow_untracked": false,
    "commit_message": "gate: add run command schema",
    "publish_after_commit": true,
    "remote": "origin",
    "branch": "main"
  },
  "verification": {
    "feature": "burpvalve-qhqa.1",
    "responses_path": "log/backpressure/responses/hash.json",
    "begin_if_missing": true,
    "prompt_profile": "native",
    "required_verdicts": ["pass", "not_applicable"]
  },
  "beads": {
    "close": false,
    "reason": "Complete burpvalve-qhqa.1",
    "admin_only": false,
    "sync": true
  },
  "release": {
    "agent_mail_identity": "PurpleStream",
    "release_reservations": true,
    "agent_mail_mcp": "available",
    "wake_ref": "burpvalve-qhqa.1"
  }
}`
}

func containsGateRunString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle || strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func containsGateRunJournalStep(steps []gateRunJournalStep, id, status string) bool {
	for _, step := range steps {
		if step.ID == id && step.Status == status {
			return true
		}
	}
	return false
}

func runInTempDir(t *testing.T) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatal(err)
		}
	})
}

func initGateRunGitRepo(t *testing.T) {
	t.Helper()
	runGateRunGit(t, "init")
	runGateRunGit(t, "config", "user.email", "test@example.invalid")
	runGateRunGit(t, "config", "user.name", "Burpvalve Test")
	writeGateRunWorktreeFile(t, "README.md", "initial\n")
	writeGateRunWorktreeFile(t, "backpressure/manifest.yaml", `conditions:
  - id: lint-rules
    path: backpressure/lint-rules.md
    enabled: true
    verifier_policy: independent_required
  - id: scope-control
    path: backpressure/scope-control.md
    enabled: true
    verifier_policy: independent_required
`)
	writeGateRunWorktreeFile(t, "backpressure/lint-rules.md", "# Lint rules\n")
	writeGateRunWorktreeFile(t, "backpressure/scope-control.md", "# Scope control\n")
	runGateRunGit(t, "add", "README.md", "backpressure/manifest.yaml", "backpressure/lint-rules.md", "backpressure/scope-control.md")
	runGateRunGit(t, "commit", "-m", "initial")
}

func writeGateRunWorktreeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runGateRunGit(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func installFakeGateRunBR(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "br")
	script := `#!/bin/sh
set -eu
case "$1" in
  show)
    printf '[{"id":"%s","title":"Delivery","status":"open","issue_type":"task"}]\n' "$2"
    ;;
  close)
    mkdir -p .beads
    printf '{"id":"%s","status":"closed"}\n' "$2" > .beads/issues.jsonl
    ;;
  sync)
    mkdir -p .beads
    test -f .beads/issues.jsonl || printf '{"id":"br-delivery","status":"closed"}\n' > .beads/issues.jsonl
    ;;
  *)
    echo "unexpected br command: $*" >&2
    exit 2
    ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
