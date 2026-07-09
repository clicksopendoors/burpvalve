package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"burpvalve/internal/attestations"
	"burpvalve/internal/backpressure"
)

func TestModesRouteThroughCore(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)

	t.Run("pre-commit", func(t *testing.T) {
		responses := filepath.Join(target, "responses.json")
		writeCLIFile(t, target, "responses.json", `{
  "atomicity": {"one_feature_or_fix": true, "message": "Staged changes map only to br-123."},
  "conditions": [
    {"condition_id": "dry", "subagent_confirmed": true, "verdict": "pass", "evidence": ["dry ok"]},
    {"condition_id": "scope-control", "subagent_confirmed": true, "verdict": "pass", "evidence": ["scope ok"]}
  ]
}`)
		stdout, stderr, err := runBurpvalve(t, repoRoot, "commit", "--root", target, "--feature", "br-123", "--responses", responses)
		if err == nil {
			t.Fatal("pre-commit should block after writing an unstaged attestation")
		}
		var result backpressure.PreCommitResult
		if err := json.Unmarshal(stdout, &result); err != nil {
			t.Fatalf("decode pre-commit result: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		if result.Status != backpressure.StatusAttestationWritten {
			t.Fatalf("status = %q", result.Status)
		}
		if !strings.Contains(string(stderr), "git add "+result.ArtifactPath) {
			t.Fatalf("stderr missing staging instruction:\n%s", stderr)
		}
		staged := runOutput(t, target, "git", "diff", "--cached", "--name-only", "--", result.ArtifactPath)
		if strings.TrimSpace(staged) != "" {
			t.Fatalf("pre-commit auto-staged attestation %s", result.ArtifactPath)
		}
	})

	t.Run("ci", func(t *testing.T) {
		stageValidAttestation(t, repoRoot, target)
		stdout, stderr, err := runBurpvalve(t, repoRoot, "ci", "--root", target, "--feature", "br-123")
		if err != nil {
			t.Fatalf("ci mode failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		var result backpressure.CIResult
		if err := json.Unmarshal(stdout, &result); err != nil {
			t.Fatalf("decode ci result: %v\n%s", err, stdout)
		}
		if result.Status != backpressure.StatusPassed {
			t.Fatalf("ci status = %q", result.Status)
		}
		if result.Plan.Features[0].ID != "br-123" {
			t.Fatalf("explicit feature was not routed through core: %#v", result.Plan.Features)
		}
		if len(result.Plan.Matrix.Cells) != 2 {
			t.Fatalf("matrix cells = %d, want 2", len(result.Plan.Matrix.Cells))
		}
		if !containsPath(result.Plan.StagedPayloadPaths, "src/app.go") {
			t.Fatalf("unexpected staged payload paths: %#v", result.Plan.StagedPayloadPaths)
		}
	})

	t.Run("lint", func(t *testing.T) {
		stdout, stderr, err := runBurpvalve(t, repoRoot, "lint", "--root", target)
		if err != nil {
			t.Fatalf("lint mode failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
		}
		var result backpressure.LintResult
		if err := json.Unmarshal(stdout, &result); err != nil {
			t.Fatalf("decode lint result: %v\n%s", err, stdout)
		}
		if result.Status != backpressure.LintStatusNotEnforced {
			t.Fatalf("lint status = %q", result.Status)
		}
		if !strings.Contains(string(stderr), "Skipped wishlist or placeholders") {
			t.Fatalf("lint stderr missing skipped wishlist section:\n%s", stderr)
		}
	})
}

func TestPreCommitFailsClosedWithoutTTY(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)
	stdout, stderr, err := runBurpvalve(t, repoRoot, "commit", "--root", target, "--feature", "br-123")
	if err == nil {
		t.Fatalf("pre-commit without responses and without TTY should fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	var result backpressure.PreCommitResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode pre-commit result: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(result.Message, "no /dev/tty available") || !strings.Contains(string(stderr), "--responses") {
		t.Fatalf("missing no-TTY guidance, result=%#v stderr=%s", result, stderr)
	}
}

func TestPreCommitAutoDiscoversBoundResponses(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)
	templateStdout, templateStderr, templateErr := runBurpvalve(t, repoRoot, "commit", "--root", target, "--feature", "br-auto", "--responses-template")
	if templateErr != nil {
		t.Fatalf("responses template failed: %v\nstdout=%s\nstderr=%s", templateErr, templateStdout, templateStderr)
	}
	responses := passingBoundCLIResponses(t, templateStdout)
	writeCLIFile(t, target, backpressure.ResponsesPath(responses.Binding.StagedPayloadHash), mustJSONBytes(t, responses))

	stdout, stderr, err := runBurpvalve(t, repoRoot, "commit", "--root", target, "--feature", "br-auto")
	if err == nil {
		t.Fatal("auto-discovered responses should write unstaged attestation and block once")
	}
	var result backpressure.PreCommitResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode auto-discovered result: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if result.Status != backpressure.StatusAttestationWritten || result.ResponsesPath != backpressure.ResponsesPath(responses.Binding.StagedPayloadHash) {
		t.Fatalf("auto-discovery result = %#v stderr=%s", result, stderr)
	}
	if strings.Contains(string(stderr), "legacy unbound") {
		t.Fatalf("bound auto-discovery should not warn as legacy:\n%s", stderr)
	}
}

func TestPreCommitExplicitLegacyResponsesOverrideAutoDiscovery(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)
	writeCLIFile(t, target, "log/backpressure/responses/stale.json", `{"atomicity":{"one_feature_or_fix":true,"message":"stale"},"binding":{"staged_payload_hash":"sha256:stale"},"conditions":[]}`)
	legacy := filepath.Join(target, "responses.json")
	writeCLIFile(t, target, "responses.json", `{
  "atomicity": {"one_feature_or_fix": true, "message": "Staged changes map only to br-legacy."},
  "conditions": [
    {"condition_id": "dry", "subagent_confirmed": true, "subagent_model": "test", "verdict": "pass", "evidence": ["dry ok"]},
    {"condition_id": "scope-control", "subagent_confirmed": true, "subagent_model": "test", "verdict": "pass", "evidence": ["scope ok"]}
  ]
}`)

	stdout, stderr, err := runBurpvalve(t, repoRoot, "commit", "--root", target, "--feature", "br-legacy", "--responses", legacy)
	if err == nil {
		t.Fatal("legacy responses should write unstaged attestation and block once")
	}
	var result backpressure.PreCommitResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode legacy override result: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if result.Status != backpressure.StatusAttestationWritten || result.ResponsesPath != legacy {
		t.Fatalf("legacy override result = %#v stderr=%s", result, stderr)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "legacy unbound responses") || !strings.Contains(string(stderr), "legacy unbound responses") {
		t.Fatalf("legacy warning missing result=%#v stderr=%s", result, stderr)
	}
}

func TestPreCommitReportsStaleAutoDiscoveredResponses(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)
	writeCLIFile(t, target, "log/backpressure/responses/stale.json", `{"atomicity":{"one_feature_or_fix":true,"message":"stale"},"binding":{"staged_payload_hash":"sha256:stale"},"conditions":[]}`)

	stdout, stderr, err := runBurpvalve(t, repoRoot, "commit", "--root", target, "--feature", "br-stale")
	if err == nil {
		t.Fatal("stale auto-discovered responses should block")
	}
	var result backpressure.PreCommitResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode stale result: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(result.Message, "different staged payload") || len(result.NextSteps) == 0 || !strings.Contains(string(stderr), "different staged payload") {
		t.Fatalf("stale response guidance missing result=%#v stderr=%s", result, stderr)
	}
}

func TestCIModeValidatesCommittedAttestationInCleanCheckout(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureCommittedPayloadRepo(t, repoRoot)
	if dirty := runOutput(t, target, "git", "status", "--short"); strings.TrimSpace(dirty) != "" {
		t.Fatalf("fixture repo should be clean after commit:\n%s", dirty)
	}

	stdout, stderr, err := runBurpvalve(t, repoRoot, "ci", "--root", target)
	if err != nil {
		t.Fatalf("ci mode should validate committed attestation: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var result backpressure.CIResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode ci result: %v\n%s", err, stdout)
	}
	if result.Status != backpressure.StatusPassed {
		t.Fatalf("ci status = %q", result.Status)
	}
	if result.Plan.Features[0].ID != "diff:src" {
		t.Fatalf("ci should infer committed payload feature, got %#v", result.Plan.Features)
	}
	if !containsPath(result.Plan.StagedPayloadPaths, "src/app.go") {
		t.Fatalf("unexpected committed payload paths: %#v", result.Plan.StagedPayloadPaths)
	}
}

func TestCICommitAuditsSpecifiedCommit(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureCommittedPayloadRepo(t, repoRoot)
	featureCommit := strings.TrimSpace(runOutput(t, target, "git", "rev-parse", "HEAD"))

	writeCLIFile(t, target, "notes.txt", "later commit without attestation\n")
	run(t, target, "git", "add", "notes.txt")
	run(t, target, "git", "commit", "-q", "--no-verify", "-m", "later")

	stdout, stderr, err := runBurpvalve(t, repoRoot, "ci", "--root", target, "--commit", featureCommit, "--feature", "br-123")
	if err != nil {
		t.Fatalf("ci --commit should validate target commit: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var result backpressure.CIResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode ci result: %v\n%s", err, stdout)
	}
	if result.Status != backpressure.StatusPassed || result.AuditCommit != featureCommit {
		t.Fatalf("unexpected ci --commit result: %#v", result)
	}
	if result.Attestation.FeatureID != "br-123" || len(result.ConditionProvenance) != 2 {
		t.Fatalf("missing audit provenance: %#v", result)
	}

	stdout, stderr, err = runBurpvalve(t, repoRoot, "ci", "--root", target, "--commit", featureCommit, "--feature", "wrong-feature")
	if err == nil || !strings.Contains(string(stderr), "feature assertion mismatch") {
		t.Fatalf("ci --commit should reject feature assertion mismatch\nerr=%v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}

	input := `{"root":` + strconv.Quote(target) + `,"commit":` + strconv.Quote(featureCommit) + `,"feature":"br-123"}`
	stdout, stderr, err = runBurpvalveWithInput(t, repoRoot, input, "--robots", "ci")
	if err != nil {
		t.Fatalf("robot ci --commit should validate target commit: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
}

func TestCICommitExplicitFeatureAuditsMultiClusterCommit(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureCommittedMultiClusterPayloadRepo(t, repoRoot)
	featureCommit := strings.TrimSpace(runOutput(t, target, "git", "rev-parse", "HEAD"))

	stdout, stderr, err := runBurpvalve(t, repoRoot, "ci", "--root", target, "--commit", featureCommit, "--feature", "br-123")
	if err != nil {
		t.Fatalf("ci --commit with explicit feature should validate multi-cluster target commit: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var result backpressure.CIResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode ci result: %v\n%s", err, stdout)
	}
	if result.Status != backpressure.StatusPassed || result.AuditCommit != featureCommit {
		t.Fatalf("unexpected ci --commit result: %#v", result)
	}
	if result.Plan.Features[0].ID != "br-123" || result.Plan.Features[0].DiffCluster != "explicit:br-123" {
		t.Fatalf("explicit feature should scope committed audit plan, got %#v", result.Plan.Features)
	}
	if !containsPath(result.Plan.StagedPayloadPaths, "src/app.go") || !containsPath(result.Plan.StagedPayloadPaths, "docs/readme.md") {
		t.Fatalf("expected both committed payload clusters, got %#v", result.Plan.StagedPayloadPaths)
	}

	stdout, stderr, err = runBurpvalve(t, repoRoot, "ci", "--root", target, "--commit", featureCommit, "--feature", "wrong-feature")
	if err == nil || !strings.Contains(string(stderr), "feature assertion mismatch") {
		t.Fatalf("ci --commit should reject feature assertion mismatch for multi-cluster commit\nerr=%v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
}

func passingBoundCLIResponses(t *testing.T, template []byte) backpressure.Responses {
	t.Helper()
	var responses backpressure.Responses
	if err := json.Unmarshal(template, &responses); err != nil {
		t.Fatalf("decode response template: %v\n%s", err, template)
	}
	now := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
	responses.Atomicity.OneFeatureOrFix = true
	responses.Atomicity.Message = "Staged changes map only to this test feature."
	for i := range responses.Conditions {
		responses.Conditions[i].Verifier = attestations.Verifier{
			Kind:            attestations.VerifierIndependentSubagent,
			Agent:           "Verifier",
			Model:           "test",
			Runtime:         "go-test",
			SeparateContext: true,
			EvidenceRef:     "test transcript",
			CreatedAt:       &now,
		}
		responses.Conditions[i].SubagentConfirmed = true
		responses.Conditions[i].SubagentModel = "test"
		responses.Conditions[i].Verdict = attestations.VerdictPass
		responses.Conditions[i].Message = "Verifier accepted " + responses.Conditions[i].ConditionID + "."
		responses.Conditions[i].Evidence = []string{"evidence for " + responses.Conditions[i].ConditionID}
		responses.Conditions[i].NextAction = ""
	}
	return responses
}

func mustJSONBytes(t *testing.T, value any) string {
	t.Helper()
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestPreCommitHandlesStagedDeletion(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)
	run(t, target, "git", "config", "user.email", "michael-bltzr@users.noreply.github.com")
	run(t, target, "git", "config", "user.name", "Test User")
	run(t, target, "git", "commit", "-q", "--no-verify", "-m", "baseline")

	if err := os.Remove(filepath.Join(target, "src/app.go")); err != nil {
		t.Fatal(err)
	}
	run(t, target, "git", "add", "-u", "src/app.go")
	responses := filepath.Join(target, "responses-delete.json")
	writeCLIFile(t, target, "responses-delete.json", `{
  "atomicity": {"one_feature_or_fix": true, "message": "Staged changes map only to br-delete."},
  "conditions": [
    {"condition_id": "dry", "subagent_confirmed": true, "verdict": "pass", "evidence": ["delete ok"]},
    {"condition_id": "scope-control", "subagent_confirmed": true, "verdict": "pass", "evidence": ["scope ok"]}
  ]
}`)

	stdout, stderr, err := runBurpvalve(t, repoRoot, "commit", "--root", target, "--feature", "br-delete", "--responses", responses)
	if err == nil {
		t.Fatal("pre-commit should ask to stage generated attestation")
	}
	var result backpressure.PreCommitResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode pre-commit result: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if result.Status != backpressure.StatusAttestationWritten {
		t.Fatalf("staged deletion should reach attestation writing, result=%#v stderr=%s", result, stderr)
	}
	if !containsPath(result.Plan.StagedPayloadPaths, "src/app.go") {
		t.Fatalf("deleted file missing from staged payload paths: %#v", result.Plan.StagedPayloadPaths)
	}
	if len(result.Plan.StagedPayloadFiles) != 1 || result.Plan.StagedPayloadFiles[0].Status != "deleted" {
		t.Fatalf("deleted file status not exposed in plan: %#v", result.Plan.StagedPayloadFiles)
	}
}

func TestPreCommitRecordsBeadMetadataAndQueryFilters(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)
	responses := filepath.Join(target, "responses.json")
	writeCLIFile(t, target, "responses.json", `{
  "atomicity": {"one_feature_or_fix": true, "message": "Staged changes map to one delivery bead."},
  "conditions": [
    {"condition_id": "dry", "subagent_confirmed": true, "verdict": "pass", "evidence": ["dry ok"]},
    {"condition_id": "scope-control", "subagent_confirmed": true, "verdict": "pass", "evidence": ["scope ok"]}
  ]
}`)
	stdout, stderr, err := runBurpvalve(t, repoRoot, "commit", "--root", target, "--feature", "delivery-feature", "--bead", "br-delivery", "--responses", responses)
	if err == nil {
		t.Fatal("pre-commit should ask to stage generated attestation")
	}
	var result backpressure.PreCommitResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode pre-commit result: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	body := readTestFile(t, filepath.Join(target, filepath.FromSlash(result.ArtifactPath)))
	if !strings.Contains(body, `"bead_ids":`) || !strings.Contains(body, `"br-delivery"`) {
		t.Fatalf("attestation missing bead metadata:\n%s", body)
	}
	listStdout, listStderr, listErr := runBurpvalve(t, repoRoot, "attestations", "list", "--root", target, "--bead", "br-delivery", "--json")
	if listErr != nil {
		t.Fatalf("attestation query by bead failed: %v\nstdout=%s\nstderr=%s", listErr, listStdout, listStderr)
	}
	if !strings.Contains(string(listStdout), `"bead_ids"`) || !strings.Contains(string(listStdout), `"br-delivery"`) {
		t.Fatalf("attestation list did not expose bead id:\n%s", listStdout)
	}
}

func TestCommitMultipleBeadsRequireRationale(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("commit", "--bead", "br-one", "--bead", "br-two")
	if err == nil || !strings.Contains(err.Error(), "--bead-rationale is required") {
		t.Fatalf("multiple bead commit should require rationale\nerr=%v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
}

func TestCommitLaneAssertionMustMatchBoundResponses(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)
	writeCLIFile(t, target, "src/lane.go", "package app\n\nconst Lane = true\n")
	run(t, target, "git", "add", "src/lane.go")

	stdout, stderr, err := runBurpvalve(t, repoRoot, "verifier", "begin",
		"--root", target,
		"--lane",
		"--lane-id", "lane-aj41",
		"--bead", "burpvalve-aj41.3",
		"--bead", "burpvalve-aj41.4",
		"--lane-rationale", "authorized lane payload",
		"--lane-authorization-ref", "ORCH-2026-07-08",
		"--authorized-by", "BronzeDeer",
		"--json")
	if err != nil {
		t.Fatalf("lane verifier begin failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var begin backpressure.BeginResponsesResult
	if err := json.Unmarshal(stdout, &begin); err != nil {
		t.Fatalf("decode begin result: %v\nstdout=%s", err, stdout)
	}
	responsesPath := filepath.Join(target, filepath.FromSlash(begin.ResponsesPath))
	var responses backpressure.Responses
	if err := json.Unmarshal([]byte(readTestFile(t, responsesPath)), &responses); err != nil {
		t.Fatalf("decode begin responses: %v", err)
	}
	for i := range responses.Conditions {
		responses.Conditions[i].Verifier.Kind = "independent_subagent"
		responses.Conditions[i].SubagentConfirmed = true
		responses.Conditions[i].Verdict = "pass"
		responses.Conditions[i].Message = "lane assertion test pass"
		responses.Conditions[i].Evidence = []string{"test evidence"}
		responses.Conditions[i].NextAction = ""
	}
	if err := os.WriteFile(responsesPath, []byte(mustJSONBytes(t, responses)), 0o644); err != nil {
		t.Fatalf("rewrite responses: %v", err)
	}

	stdout, stderr, err = runBurpvalve(t, repoRoot, "commit",
		"--root", target,
		"--feature", "lane-aj41",
		"--lane", "wrong-lane",
		"--bead", "burpvalve-aj41.3",
		"--bead", "burpvalve-aj41.4",
		"--lane-rationale", "authorized lane payload",
		"--lane-authorization-ref", "ORCH-2026-07-08",
		"--authorized-by", "BronzeDeer",
		"--responses", responsesPath)
	if err == nil {
		t.Fatalf("commit should reject lane mismatch\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if !strings.Contains(string(stderr), `commit lane id mismatch`) {
		t.Fatalf("stderr missing lane mismatch:\n%s", stderr)
	}
}

func TestNormalizeCommitBeadsAcceptsRepeatedAndCSVAlias(t *testing.T) {
	beads, rationale, warnings, err := normalizeCommitBeads(
		[]string{" br-one ", "br-one"},
		[]string{"br-two, br-one", "br-three"},
		" same staged payload ",
	)
	if err != nil {
		t.Fatalf("normalizeCommitBeads returned error: %v", err)
	}
	if got, want := strings.Join(beads, ","), "br-one,br-two,br-three"; got != want {
		t.Fatalf("beads = %q, want %q", got, want)
	}
	if rationale != "same staged payload" {
		t.Fatalf("rationale = %q", rationale)
	}
	if len(warnings) != 2 || !strings.Contains(strings.Join(warnings, "\n"), `duplicate bead ID "br-one"`) {
		t.Fatalf("duplicate warning missing: %#v", warnings)
	}
}

func TestNormalizeCommitBeadsRejectsCommaInRepeatedBead(t *testing.T) {
	_, _, _, err := normalizeCommitBeads([]string{"br-one,br-two"}, nil, "")
	if err == nil || !strings.Contains(err.Error(), "must not contain commas") {
		t.Fatalf("comma in --bead should be rejected, err=%v", err)
	}
}

func TestNormalizeCommitBeadsRejectsEmptyCSVTokens(t *testing.T) {
	_, _, _, err := normalizeCommitBeads(nil, []string{"br-one,,br-two"}, "")
	if err == nil || !strings.Contains(err.Error(), "empty tokens") {
		t.Fatalf("empty --beads token should be rejected, err=%v", err)
	}
}

func TestLintModeHonorsDeclaredTimeoutAboveGlobalCoreTimeout(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)
	writeCLIFile(t, target, "backpressure/manifest.yaml", `conditions:
  - id: dry
    path: backpressure/dry.md
    enabled: true
lint_commands:
  - id: long-timeout
    command: "true"
    required: true
    paths: ["."]
    timeout_seconds: 60
`)

	stdout, stderr, err := runBurpvalve(t, repoRoot, "lint", "--root", target)
	if err != nil {
		t.Fatalf("lint mode should honor declared timeout above core timeout: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var result backpressure.LintResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode lint result: %v\n%s", err, stdout)
	}
	if len(result.Commands) != 1 || result.Commands[0].TimeoutSeconds != 60 || result.Commands[0].Status != backpressure.LintStatusPassed {
		t.Fatalf("declared timeout/result not preserved: %#v", result.Commands)
	}
}

func TestLintModeMalformedManifestReturnsBlockedJSON(t *testing.T) {
	repoRoot := findRepoRoot(t)
	tests := []struct {
		name     string
		commands string
		want     string
	}{
		{
			name: "scalar command",
			commands: `lint_commands:
  - ./scripts/check-structure.sh
`,
			want: "lint_commands[0] must be an object",
		},
		{
			name: "unsupported key",
			commands: `lint_commands:
  - id: extra-field
    command: "true"
    required: true
    paths: ["."]
    timeout_seconds: 5
    run_dir: src
`,
			want: "run_dir is not supported",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := fixtureGitRepo(t)
			writeCLIFile(t, target, "backpressure/manifest.yaml", `conditions:
  - id: dry
    path: backpressure/dry.md
    enabled: true
`+tt.commands)

			stdout, stderr, err := runBurpvalve(t, repoRoot, "lint", "--root", target)
			if err == nil {
				t.Fatal("malformed lint_commands should fail")
			}
			var result backpressure.LintResult
			if err := json.Unmarshal(stdout, &result); err != nil {
				t.Fatalf("decode lint result: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
			}
			if result.Status != backpressure.StatusBlocked || !strings.Contains(result.Message, tt.want) {
				t.Fatalf("malformed lint result = %#v, want %q", result, tt.want)
			}
			if !strings.Contains(string(stderr), tt.want) {
				t.Fatalf("stderr missing schema error %q:\n%s", tt.want, stderr)
			}
		})
	}
}

func TestCommitResponsesTemplateEmitsCurrentMatrix(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)

	stdout, stderr, err := runBurpvalve(t, repoRoot, "commit", "--root", target, "--feature", "br-123", "--responses-template")
	if err != nil {
		t.Fatalf("responses template failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != nil && len(stderr) != 0 {
		t.Fatalf("responses template should not write stderr:\n%s", stderr)
	}
	var responses backpressure.Responses
	if err := json.Unmarshal(stdout, &responses); err != nil {
		t.Fatalf("decode responses template: %v\n%s", err, stdout)
	}
	if responses.Atomicity.OneFeatureOrFix {
		t.Fatal("responses template should not accidentally pass atomicity")
	}
	if got, want := len(responses.Conditions), 2; got != want {
		t.Fatalf("template conditions = %d, want %d", got, want)
	}
	if responses.Conditions[0].ConditionID != "dry" || responses.Conditions[0].ConditionFile != "backpressure/dry.md" {
		t.Fatalf("first condition template = %#v", responses.Conditions[0])
	}
	if responses.Conditions[1].ConditionID != "scope-control" || responses.Conditions[1].ConditionFile != "backpressure/scope-control.md" {
		t.Fatalf("second condition template = %#v", responses.Conditions[1])
	}
	for _, condition := range responses.Conditions {
		if condition.VerifierPolicy != "independent_required" {
			t.Fatalf("condition %s verifier policy = %q", condition.ConditionID, condition.VerifierPolicy)
		}
		if condition.Verifier.Kind != "unknown" || condition.Verifier.Agent == "" || condition.Verifier.Model == "" || condition.Verifier.Runtime == "" {
			t.Fatalf("condition %s missing verifier placeholders: %#v", condition.ConditionID, condition.Verifier)
		}
	}
}

func containsPath(paths []string, needle string) bool {
	for _, path := range paths {
		if path == needle {
			return true
		}
	}
	return false
}

func fixtureGitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run(t, root, "git", "init", "-q")
	writeCLIFile(t, root, "backpressure/manifest.yaml", `conditions:
  - id: dry
    path: backpressure/dry.md
    enabled: true
  - id: scope-control
    path: backpressure/scope-control.md
    enabled: true
lint_commands: []
`)
	writeCLIFile(t, root, "backpressure/dry.md", "# DRY\n")
	writeCLIFile(t, root, "backpressure/scope-control.md", "# Scope\n")
	writeCLIFile(t, root, "src/app.go", "package app\n")
	run(t, root, "git", "add", "backpressure/manifest.yaml", "backpressure/dry.md", "backpressure/scope-control.md", "src/app.go")
	return root
}

func fixtureCommittedPayloadRepo(t *testing.T, repoRoot string) string {
	t.Helper()
	target := fixtureGitRepo(t)
	run(t, target, "git", "config", "user.email", "michael-bltzr@users.noreply.github.com")
	run(t, target, "git", "config", "user.name", "Test User")
	run(t, target, "git", "commit", "-q", "--no-verify", "-m", "baseline")

	writeCLIFile(t, target, "src/app.go", "package app\n\nconst Version = 1\n")
	run(t, target, "git", "add", "src/app.go")
	stageValidAttestation(t, repoRoot, target)
	if err := os.Remove(filepath.Join(target, "responses-ci.json")); err != nil {
		t.Fatal(err)
	}
	run(t, target, "git", "commit", "-q", "--no-verify", "-m", "feature")
	return target
}

func fixtureCommittedMultiClusterPayloadRepo(t *testing.T, repoRoot string) string {
	t.Helper()
	target := fixtureGitRepo(t)
	run(t, target, "git", "config", "user.email", "michael-bltzr@users.noreply.github.com")
	run(t, target, "git", "config", "user.name", "Test User")
	run(t, target, "git", "commit", "-q", "--no-verify", "-m", "baseline")

	writeCLIFile(t, target, "src/app.go", "package app\n\nconst Version = 1\n")
	writeCLIFile(t, target, "docs/readme.md", "# Docs\n")
	run(t, target, "git", "add", "src/app.go", "docs/readme.md")
	stageValidAttestation(t, repoRoot, target)
	if err := os.Remove(filepath.Join(target, "responses-ci.json")); err != nil {
		t.Fatal(err)
	}
	run(t, target, "git", "commit", "-q", "--no-verify", "-m", "feature")
	return target
}

func stageValidAttestation(t *testing.T, repoRoot, target string) {
	t.Helper()
	responses := filepath.Join(target, "responses-ci.json")
	writeCLIFile(t, target, "responses-ci.json", `{
  "atomicity": {"one_feature_or_fix": true, "message": "Staged changes map only to br-123."},
  "conditions": [
    {"condition_id": "dry", "subagent_confirmed": true, "verdict": "pass", "evidence": ["dry ok"]},
    {"condition_id": "scope-control", "subagent_confirmed": true, "verdict": "pass", "evidence": ["scope ok"]}
  ]
}`)
	stdout, _, err := runBurpvalve(t, repoRoot, "commit", "--root", target, "--feature", "br-123", "--responses", responses)
	if err == nil {
		t.Fatal("pre-commit should ask to stage generated attestation")
	}
	var result backpressure.PreCommitResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode pre-commit result: %v\n%s", err, stdout)
	}
	run(t, target, "git", "add", result.ArtifactPath)
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
	}
}

func writeCLIFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, output)
	}
}

func runOutput(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, output)
	}
	return string(output)
}

func runBurpvalve(t *testing.T, repoRoot string, args ...string) ([]byte, []byte, error) {
	t.Helper()
	cmd := exec.Command("go", append([]string{"run", "./cmd/burpvalve"}, args...)...)
	cmd.Dir = repoRoot
	cmd.Env = testCommandEnv(t)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func runBurpvalveWithInput(t *testing.T, repoRoot string, input string, args ...string) ([]byte, []byte, error) {
	t.Helper()
	cmd := exec.Command("go", append([]string{"run", "./cmd/burpvalve"}, args...)...)
	cmd.Dir = repoRoot
	cmd.Env = testCommandEnv(t)
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func testCommandEnv(t *testing.T) []string {
	t.Helper()
	dir := t.TempDir()
	writeExecutable(t, filepath.Join(dir, "burpvalve"), "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo test-burpvalve; fi\nexit 0\n")
	return append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
