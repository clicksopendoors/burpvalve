package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"burpvalve/internal/attestations"
	"burpvalve/internal/backpressure"
	"burpvalve/internal/scaffold"
)

type e2eHarness struct {
	t        *testing.T
	repoRoot string
	workDir  string
	binDir   string
	binary   string
	home     string
	xdg      string
	data     string
	path     string

	transcript []e2eCommandResult
}

type e2eCommandResult struct {
	Dir      string
	Name     string
	Args     []string
	Env      map[string]string
	ExitCode int
	Stdout   string
	Stderr   string
}

func TestE2EHarnessInitSetupAndRealPreCommitHook(t *testing.T) {
	h := newE2EHarness(t)
	repo := h.newRepo("project")
	h.write(repo, ".burpvalve.json", `{
  "schema_version": 1,
  "defaults": {
    "init": {
      "beads": false,
      "ntm": false
    }
  }
}`)

	init := h.burpvalve(repo, nil, "init", "--target", repo, "--force", "--json", "--git-init", "--no-beads", "--no-ntm")
	h.requireExit(init, 0)
	h.requireExit(h.git(repo, nil, "config", "user.email", "michael-bltzr@users.noreply.github.com"), 0)
	h.requireExit(h.git(repo, nil, "config", "user.name", "Test User"), 0)
	h.git(repo, nil, "add", ".")
	h.git(repo, nil, "commit", "-q", "--no-verify", "-m", "baseline")

	setup := h.burpvalve(repo, nil, "setup", "--target", repo, "--json")
	h.requireExit(setup, 0)
	var setupReport struct {
		Status  string `json:"status"`
		Summary struct {
			Ready bool `json:"ready"`
		} `json:"summary"`
	}
	h.decodeJSON(setup.Stdout, &setupReport)
	if setupReport.Status != "ready" || !setupReport.Summary.Ready {
		t.Fatalf("setup should report ready after baseline scaffold, got %#v\nstdout=%s", setupReport, setup.Stdout)
	}

	h.write(repo, "src/app.go", "package app\n\nconst Version = 1\n")
	h.git(repo, nil, "add", "src/app.go")

	missing := h.git(repo, map[string]string{"BURPVALVE_FEATURE": "br-e2e"}, "commit", "-m", "feature without attestation")
	h.requireExit(missing, 1)
	h.assertOutputContains(missing, "--responses")
	blockedReports := h.glob(repo, "log/backpressure/failed/*.json")
	if len(blockedReports) == 0 {
		t.Fatalf("blocked hook should write a blocked report\n%s", missing.String())
	}
	h.git(repo, nil, append([]string{"add"}, blockedReports...)...)

	begin := h.burpvalve(repo, nil, "verifier", "begin", "--root", repo, "--feature", "br-e2e", "--one-feature", "--atomicity-message", "E2E staged payload is one feature.", "--json")
	h.requireExit(begin, 0)
	var beginResult backpressure.BeginResponsesResult
	h.decodeJSON(begin.Stdout, &beginResult)
	for _, condition := range beginResult.Plan.Matrix.Conditions {
		transcriptPath := filepath.ToSlash(filepath.Join("log/backpressure/e2e-source-transcripts", condition.ID+".md"))
		h.write(repo, transcriptPath, "E2E verifier transcript for "+condition.ID+"\n")
		submitBody := verifierSubmitBody(t, beginResult, condition.ID)
		submit := h.burpvalveInput(repo, submitBody, nil,
			"verifier", "submit",
			"--root", repo,
			"--feature", "br-e2e",
			"--condition", condition.ID,
			"--staged-payload-hash", beginResult.StagedPayloadHash,
			"--manifest-hash", beginResult.ManifestHash,
			"--condition-file-hash", beginResult.Plan.ConditionFileHashes[condition.ID],
			"--transcript", transcriptPath,
			"--json",
		)
		h.requireExit(submit, 0)
	}

	written := h.burpvalve(repo, nil, "commit", "--root", repo, "--feature", "br-e2e")
	h.requireExit(written, 2)
	var preCommit backpressure.PreCommitResult
	h.decodeJSON(written.Stdout, &preCommit)
	if preCommit.Status != backpressure.StatusAttestationWritten || preCommit.ArtifactPath == "" {
		t.Fatalf("expected attestation_written with artifact path, got %#v", preCommit)
	}
	if preCommit.ResponsesPath != beginResult.ResponsesPath {
		t.Fatalf("commit did not auto-discover begin response file: %#v begin=%s", preCommit, beginResult.ResponsesPath)
	}
	h.assertPathExists(repo, preCommit.ArtifactPath)
	h.assertArtifactBound(repo, preCommit)
	bodyBytes, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(preCommit.ArtifactPath)))
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	if !strings.Contains(body, `"transcript_ref": "log/backpressure/transcripts/`) {
		t.Fatalf("auto-discovered attestation missing transcript refs:\n%s", body)
	}
	h.git(repo, nil, "add", preCommit.ArtifactPath)

	commit := h.git(repo, map[string]string{"BURPVALVE_FEATURE": "br-e2e"}, "commit", "-m", "feature with burpvalve evidence")
	h.requireExit(commit, 0)
	h.removeAll(repo, "log/backpressure/e2e-source-transcripts")
	h.removeAll(repo, "log/backpressure/responses")
	h.removeAll(repo, "log/backpressure/transcripts")
	h.assertGitClean(repo)
	h.assertNoUnexpectedRootUntracked(repo)
}

func TestE2ECommitPayloadDeleteRenameAndVerifierPolicy(t *testing.T) {
	h := newE2EHarness(t)
	repo := h.newBackpressureRepo("payload-policy")
	h.write(repo, "src/old.go", `package app

const Name = "old"
const FeatureEnabled = true
const RetryLimit = 3
const TimeoutSeconds = 30
const EvidenceMode = "strict"
`)
	h.write(repo, "src/delete.go", "package app\n\nconst Delete = true\n")
	h.git(repo, nil, "add", ".")
	h.git(repo, nil, "commit", "-q", "--no-verify", "-m", "baseline")

	h.git(repo, nil, "mv", "src/old.go", "src/new.go")
	h.write(repo, "src/new.go", `package app

const Name = "new"
const FeatureEnabled = true
const RetryLimit = 3
const TimeoutSeconds = 30
const EvidenceMode = "strict"
`)
	if err := os.Remove(filepath.Join(repo, "src/delete.go")); err != nil {
		t.Fatal(err)
	}
	h.git(repo, nil, "add", "-A")

	template := h.burpvalve(repo, nil, "commit", "--root", repo, "--feature", "br-payload", "--responses-template")
	h.requireExit(template, 0)
	blockedResponses := passingResponses(t, template.Stdout)
	for i := range blockedResponses.Conditions {
		blockedResponses.Conditions[i].Verifier = attestations.Verifier{
			Kind:            attestations.VerifierMainAgent,
			Agent:           "committer",
			Model:           "test",
			Runtime:         "go-test",
			SeparateContext: false,
		}
		blockedResponses.Conditions[i].SubagentConfirmed = false
		blockedResponses.Conditions[i].Message = "main-agent evidence is intentionally insufficient for policy"
	}
	blockedPath := filepath.Join(h.workDir, "blocked-responses.json")
	h.writeAbs(blockedPath, mustJSON(t, blockedResponses))
	blocked := h.burpvalve(repo, nil, "commit", "--root", repo, "--feature", "br-payload", "--responses", blockedPath)
	h.requireExit(blocked, 2)
	var blockedResult backpressure.PreCommitResult
	h.decodeJSON(blocked.Stdout, &blockedResult)
	if blockedResult.Status != backpressure.StatusBlocked || blockedResult.BlockedReportPath == "" {
		t.Fatalf("policy failure should produce blocked report: %#v", blockedResult)
	}
	h.assertPathExists(repo, blockedResult.BlockedReportPath)

	passing := passingResponses(t, template.Stdout)
	passingPath := filepath.Join(h.workDir, "passing-responses.json")
	h.writeAbs(passingPath, mustJSON(t, passing))
	written := h.burpvalve(repo, nil, "commit", "--root", repo, "--feature", "br-payload", "--responses", passingPath)
	h.requireExit(written, 2)
	var writtenResult backpressure.PreCommitResult
	h.decodeJSON(written.Stdout, &writtenResult)
	if writtenResult.Status != backpressure.StatusAttestationWritten {
		t.Fatalf("expected attestation_written, got %#v", writtenResult)
	}
	if !hasStagedFile(writtenResult.Plan.StagedPayloadFiles, stagedFileWant{path: "src/delete.go", status: "deleted", gitStatus: "D"}) ||
		!hasStagedFile(writtenResult.Plan.StagedPayloadFiles, stagedFileWant{path: "src/new.go", oldPath: "src/old.go", status: "renamed", gitStatusPrefix: "R"}) {
		t.Fatalf("delete/rename statuses missing from plan: %#v", writtenResult.Plan.StagedPayloadFiles)
	}
	h.assertArtifactBound(repo, writtenResult)
}

func TestE2ELintTruthfulnessAndEnforcement(t *testing.T) {
	h := newE2EHarness(t)

	noOp := h.newBackpressureRepo("lint-noop")
	noOpStatus := h.gitStatus(noOp)
	noOpResult := h.burpvalve(noOp, nil, "lint", "--root", noOp, "--json")
	h.requireExit(noOpResult, 0)
	var noOpLint backpressure.LintResult
	h.decodeJSON(noOpResult.Stdout, &noOpLint)
	if noOpLint.Status != backpressure.LintStatusNotEnforced || noOpLint.Enforced || noOpLint.EvidenceStrength != "none" || len(noOpLint.NextSteps) == 0 {
		t.Fatalf("lint no-op should report missing enforcement, not a normal pass: %#v", noOpLint)
	}
	h.assertGitStatusUnchanged(noOp, noOpStatus)

	pass := h.newBackpressureRepo("lint-pass")
	h.write(pass, "backpressure/manifest.yaml", `conditions:
  - id: dry
    path: backpressure/dry.md
    enabled: true
lint_commands:
  - id: env-paths
    command: "test \"$BACKPRESSURE_LINT_PATHS\" = \"src\""
    required: true
    paths: ["src"]
    timeout_seconds: 5
`)
	passStatus := h.gitStatus(pass)
	passResult := h.burpvalve(pass, nil, "lint", "--root", pass, "--json")
	h.requireExit(passResult, 0)
	var passLint backpressure.LintResult
	h.decodeJSON(passResult.Stdout, &passLint)
	if !passLint.Enforced || passLint.CommandCount != 1 || passLint.Commands[0].Status != backpressure.LintStatusPassed {
		t.Fatalf("lint pass should report enforced command output: %#v", passLint)
	}
	h.assertGitStatusUnchanged(pass, passStatus)

	fail := h.newBackpressureRepo("lint-fail")
	h.write(fail, "backpressure/manifest.yaml", `conditions:
  - id: dry
    path: backpressure/dry.md
    enabled: true
lint_commands:
  - id: required-style
    command: "echo required failure >&2; exit 7"
    required: true
    paths: ["."]
    timeout_seconds: 5
`)
	failStatus := h.gitStatus(fail)
	failResult := h.burpvalve(fail, nil, "lint", "--root", fail, "--json")
	h.requireExit(failResult, 2)
	var failLint backpressure.LintResult
	h.decodeJSON(failResult.Stdout, &failLint)
	if failLint.Status != backpressure.StatusBlocked || !failLint.Fatal || failLint.Commands[0].ExitCode != 7 {
		t.Fatalf("lint failure should be fatal with command evidence: %#v", failLint)
	}
	h.assertGitStatusUnchanged(fail, failStatus)
}

func TestE2ENonBeadsCoreWorksWithoutBR(t *testing.T) {
	h := newE2EHarness(t)
	repo := h.newBackpressureRepo("without-br")
	noBREnv := h.envWithoutBR()

	brProbe := h.run(repo, noBREnv, "sh", "-c", "command -v br")
	if brProbe.ExitCode == 0 {
		t.Fatalf("no-br test path unexpectedly found br:\n%s", brProbe.String())
	}

	h.write(repo, "src/app.go", "package app\n\nconst Version = 1\n")
	h.git(repo, noBREnv, "add", ".")
	h.git(repo, noBREnv, "commit", "-q", "--no-verify", "-m", "baseline without beads")

	setupResult := h.burpvalve(repo, noBREnv, "setup", "--target", repo, "--json", "--no-beads", "--no-ntm")
	h.requireExit(setupResult, 0)
	var setupReport scaffold.Report
	h.decodeJSON(setupResult.Stdout, &setupReport)
	for _, id := range []string{"beads", "br-tool", "ntm"} {
		if e2eCheckByID(setupReport.Checks, id) != nil {
			t.Fatalf("setup --no-beads --no-ntm should not require %s when br is absent: %#v", id, setupReport.Checks)
		}
	}

	lintResult := h.burpvalve(repo, noBREnv, "lint", "--root", repo, "--json")
	h.requireExit(lintResult, 0)
	var lintReport backpressure.LintResult
	h.decodeJSON(lintResult.Stdout, &lintReport)
	if lintReport.Status != backpressure.LintStatusNotEnforced || lintReport.Enforced {
		t.Fatalf("lint should remain usable without br and report no-op truthfully: %#v", lintReport)
	}

	h.write(repo, "src/app.go", "package app\n\nconst Version = 2\n")
	h.git(repo, noBREnv, "add", "src/app.go")
	template := h.burpvalve(repo, noBREnv, "commit", "--root", repo, "--feature", "no-beads-feature", "--responses-template")
	h.requireExit(template, 0)
	responsesPath := filepath.Join(h.workDir, "without-br-responses.json")
	h.writeAbs(responsesPath, mustJSON(t, passingResponses(t, template.Stdout)))
	written := h.burpvalve(repo, noBREnv, "commit", "--root", repo, "--feature", "no-beads-feature", "--responses", responsesPath)
	h.requireExit(written, 2)
	var writtenResult backpressure.PreCommitResult
	h.decodeJSON(written.Stdout, &writtenResult)
	artifact := h.readArtifact(repo, writtenResult.ArtifactPath)
	if len(artifact.BeadIDs) != 0 || len(artifact.Feature.BeadIDs) != 0 || artifact.Feature.SourceBead != "" {
		t.Fatalf("commit without br should not invent Beads metadata: %#v", artifact)
	}

	listResult := h.burpvalve(repo, noBREnv, "attestations", "list", "--root", repo, "--json")
	h.requireExit(listResult, 0)
	var list struct {
		Records []attestations.Record `json:"records"`
	}
	h.decodeJSON(listResult.Stdout, &list)
	if len(list.Records) != 1 || len(list.Records[0].BeadIDs) != 0 || !containsString(list.Records[0].FeatureIDs, "no-beads-feature") {
		t.Fatalf("attestations should remain queryable without br: %#v", list.Records)
	}

	explainResult := h.burpvalveInput(repo, written.Stdout, noBREnv, "explain", "--json", "-")
	h.requireExit(explainResult, 0)
	explained := decodeExplain(t, explainResult.Stdout)
	if explained.InputType != "commit" ||
		explained.OriginalStatus != "attestation_written" ||
		!explained.Fatal ||
		!containsString(explained.NextSteps, "rerun git commit") {
		t.Fatalf("explain should translate commit output without br: %#v", explained)
	}

	h.git(repo, noBREnv, "add", "src/app.go", writtenResult.ArtifactPath)
	h.git(repo, noBREnv, "commit", "-q", "--no-verify", "-m", "feature without beads")
	h.assertGitClean(repo)
	h.assertNoUnexpectedRootUntracked(repo)
}

func TestE2EBeadDeliveryAttestationQueryAndDrift(t *testing.T) {
	h := newE2EHarness(t)

	single := h.newBackpressureRepo("single-bead")
	h.write(single, "src/app.go", "package app\n\nconst Version = 1\n")
	h.git(single, nil, "add", ".")
	h.git(single, nil, "commit", "-q", "--no-verify", "-m", "baseline")
	h.write(single, "src/app.go", "package app\n\nconst Version = 2\n")
	h.git(single, nil, "add", "src/app.go")

	singleTemplate := h.burpvalve(single, nil, "commit", "--root", single, "--feature", "delivery-feature", "--responses-template")
	h.requireExit(singleTemplate, 0)
	singleResponsesPath := filepath.Join(h.workDir, "single-responses.json")
	h.writeAbs(singleResponsesPath, mustJSON(t, passingResponses(t, singleTemplate.Stdout)))
	singleWritten := h.burpvalve(single, nil, "commit", "--root", single, "--feature", "delivery-feature", "--bead", "br-delivery", "--responses", singleResponsesPath)
	h.requireExit(singleWritten, 2)
	var singleResult backpressure.PreCommitResult
	h.decodeJSON(singleWritten.Stdout, &singleResult)
	if singleResult.Status != backpressure.StatusAttestationWritten {
		t.Fatalf("single bead should write attestation, got %#v", singleResult)
	}
	singleArtifact := h.readArtifact(single, singleResult.ArtifactPath)
	if !containsString(singleArtifact.BeadIDs, "br-delivery") ||
		!containsString(singleArtifact.Feature.BeadIDs, "br-delivery") ||
		singleArtifact.Feature.SourceBead != "br-delivery" ||
		singleArtifact.Feature.ID != "delivery-feature" {
		t.Fatalf("single bead metadata missing from artifact: %#v", singleArtifact)
	}
	if singleArtifact.Conditions[0].EffectiveVerifierKind() != attestations.VerifierIndependentSubagent ||
		singleArtifact.Conditions[0].Verifier.Agent != "e2e-verifier" ||
		singleArtifact.StagedPayloadHash != singleResult.Plan.StagedPayloadHash {
		t.Fatalf("verifier provenance or payload binding missing: %#v", singleArtifact)
	}

	singleList := h.burpvalve(single, nil, "attestations", "list", "--root", single, "--json", "--bead", "br-delivery")
	h.requireExit(singleList, 0)
	var singleListResult struct {
		Records []attestations.Record `json:"records"`
	}
	h.decodeJSON(singleList.Stdout, &singleListResult)
	if len(singleListResult.Records) != 1 ||
		singleListResult.Records[0].Status != "pass" ||
		!containsString(singleListResult.Records[0].BeadIDs, "br-delivery") ||
		!containsString(singleListResult.Records[0].FeatureIDs, "delivery-feature") {
		t.Fatalf("attestation list should expose single bead metadata: %#v", singleListResult.Records)
	}
	singleShow := h.burpvalve(single, nil, "attestations", "show", "--root", single, "--json", singleResult.ArtifactPath)
	h.requireExit(singleShow, 0)
	var singleShowRecord attestations.Record
	h.decodeJSON(singleShow.Stdout, &singleShowRecord)
	if singleShowRecord.PayloadHash != singleResult.Plan.StagedPayloadHash || !containsString(singleShowRecord.BeadIDs, "br-delivery") {
		t.Fatalf("attestation show should expose payload and bead metadata: %#v", singleShowRecord)
	}
	singleLatest := h.burpvalve(single, nil, "attestations", "latest", "--root", single, "--json", "--bead", "br-delivery")
	h.requireExit(singleLatest, 0)
	var singleLatestRecord attestations.Record
	h.decodeJSON(singleLatest.Stdout, &singleLatestRecord)
	if singleLatestRecord.Path != singleResult.ArtifactPath {
		t.Fatalf("attestation latest returned wrong artifact: %#v", singleLatestRecord)
	}
	singleHuman := h.burpvalve(single, nil, "attestations", "list", "--root", single, "--color", "never", "--bead", "br-delivery")
	h.requireExit(singleHuman, 0)
	h.assertOutputContains(singleHuman, "Burpvalve attestations", "pass", "br-delivery")
	h.git(single, nil, "add", singleResult.ArtifactPath)
	singleGate := h.burpvalve(single, nil, "commit", "--root", single, "--feature", "delivery-feature", "--bead", "br-delivery")
	h.requireExit(singleGate, 0)
	var singleGateResult backpressure.PreCommitResult
	h.decodeJSON(singleGate.Stdout, &singleGateResult)
	if singleGateResult.Status != backpressure.StatusPassed {
		t.Fatalf("staged matching attestation should pass: %#v", singleGateResult)
	}
	h.git(single, nil, "commit", "-q", "--no-verify", "-m", "single bead delivery")
	h.assertGitClean(single)

	coupled := h.newBackpressureRepo("coupled-beads")
	h.write(coupled, "src/app.go", "package app\n\nconst Version = 1\n")
	h.git(coupled, nil, "add", ".")
	h.git(coupled, nil, "commit", "-q", "--no-verify", "-m", "baseline")
	h.write(coupled, "src/app.go", "package app\n\nconst Version = 2\nconst Coupled = true\n")
	h.git(coupled, nil, "add", "src/app.go")
	coupledTemplate := h.burpvalve(coupled, nil, "commit", "--root", coupled, "--feature", "coupled-feature", "--responses-template")
	h.requireExit(coupledTemplate, 0)
	coupledResponsesPath := filepath.Join(h.workDir, "coupled-responses.json")
	h.writeAbs(coupledResponsesPath, mustJSON(t, passingResponses(t, coupledTemplate.Stdout)))
	coupledWritten := h.burpvalve(coupled, nil,
		"commit", "--root", coupled,
		"--feature", "coupled-feature",
		"--bead", "br-one",
		"--bead", "br-two",
		"--bead-rationale", "same staged payload",
		"--responses", coupledResponsesPath,
	)
	h.requireExit(coupledWritten, 2)
	var coupledResult backpressure.PreCommitResult
	h.decodeJSON(coupledWritten.Stdout, &coupledResult)
	coupledArtifact := h.readArtifact(coupled, coupledResult.ArtifactPath)
	if !containsString(coupledArtifact.BeadIDs, "br-one") ||
		!containsString(coupledArtifact.BeadIDs, "br-two") ||
		coupledArtifact.CoupledWorkRationale != "same staged payload" {
		t.Fatalf("coupled bead metadata missing: %#v", coupledArtifact)
	}
	for _, beadID := range []string{"br-one", "br-two"} {
		list := h.burpvalve(coupled, nil, "attestations", "list", "--root", coupled, "--json", "--bead", beadID)
		h.requireExit(list, 0)
		var got struct {
			Records []attestations.Record `json:"records"`
		}
		h.decodeJSON(list.Stdout, &got)
		if len(got.Records) != 1 || !containsString(got.Records[0].BeadIDs, beadID) {
			t.Fatalf("coupled bead %s should be queryable: %#v", beadID, got.Records)
		}
	}
	h.git(coupled, nil, "add", coupledResult.ArtifactPath)
	h.git(coupled, nil, "commit", "-q", "--no-verify", "-m", "coupled bead delivery")
	h.assertGitClean(coupled)

	drift := h.newBackpressureRepo("stale-drift")
	h.write(drift, "src/app.go", "package app\n\nconst Version = 1\n")
	h.git(drift, nil, "add", ".")
	h.git(drift, nil, "commit", "-q", "--no-verify", "-m", "baseline")
	h.write(drift, "src/app.go", "package app\n\nconst Version = 2\n")
	h.git(drift, nil, "add", "src/app.go")
	driftTemplate := h.burpvalve(drift, nil, "commit", "--root", drift, "--feature", "drift-feature", "--responses-template")
	h.requireExit(driftTemplate, 0)
	driftResponsesPath := filepath.Join(h.workDir, "drift-responses.json")
	h.writeAbs(driftResponsesPath, mustJSON(t, passingResponses(t, driftTemplate.Stdout)))
	driftWritten := h.burpvalve(drift, nil, "commit", "--root", drift, "--feature", "drift-feature", "--bead", "br-drift", "--responses", driftResponsesPath)
	h.requireExit(driftWritten, 2)
	var driftWrittenResult backpressure.PreCommitResult
	h.decodeJSON(driftWritten.Stdout, &driftWrittenResult)
	h.git(drift, nil, "add", driftWrittenResult.ArtifactPath)
	h.write(drift, "src/app.go", "package app\n\nconst Version = 3\n")
	h.git(drift, nil, "add", "src/app.go")
	driftBlocked := h.burpvalve(drift, nil, "commit", "--root", drift, "--feature", "drift-feature", "--bead", "br-drift")
	h.requireExit(driftBlocked, 2)
	var driftBlockedResult backpressure.PreCommitResult
	h.decodeJSON(driftBlocked.Stdout, &driftBlockedResult)
	if driftBlockedResult.Status != backpressure.StatusBlocked ||
		!strings.Contains(driftBlockedResult.Message, "stale or invalid") ||
		!strings.Contains(strings.Join(driftBlockedResult.NextSteps, "\n"), "Rerun burpvalve commit") {
		t.Fatalf("stale payload should block with rerun guidance: %#v", driftBlockedResult)
	}
	driftReport := h.readArtifact(drift, driftBlockedResult.BlockedReportPath)
	if driftReport.ArtifactKind != attestations.ArtifactBlocked || driftReport.StagedPayloadHash != driftBlockedResult.Plan.StagedPayloadHash {
		t.Fatalf("drift blocked report should be formatter-safe and bound to current payload: %#v", driftReport)
	}
	driftList := h.burpvalve(drift, nil, "attestations", "list", "--root", drift, "--json", "--status", "blocked")
	h.requireExit(driftList, 0)
	var driftListResult struct {
		Records []attestations.Record `json:"records"`
	}
	h.decodeJSON(driftList.Stdout, &driftListResult)
	if len(driftListResult.Records) != 1 || driftListResult.Records[0].Status != "blocked" || !containsString(driftListResult.Records[0].BeadIDs, "br-drift") {
		t.Fatalf("blocked report should be queryable by status and bead: %#v", driftListResult.Records)
	}
	driftStatus := h.git(drift, nil, "status", "--short")
	h.requireExit(driftStatus, 0)
	for _, needle := range []string{"M  src/app.go", "A  " + driftWrittenResult.ArtifactPath, "?? log/"} {
		if !strings.Contains(driftStatus.Stdout, needle) {
			t.Fatalf("drift scenario dirty state missing %q:\n%s", needle, driftStatus.Stdout)
		}
	}

	noBeads := h.newBackpressureRepo("no-beads")
	h.write(noBeads, "src/app.go", "package app\n\nconst Version = 1\n")
	h.git(noBeads, nil, "add", ".")
	h.git(noBeads, nil, "commit", "-q", "--no-verify", "-m", "baseline")
	h.write(noBeads, "src/app.go", "package app\n\nconst Version = 2\n")
	h.git(noBeads, nil, "add", "src/app.go")
	noBeadsFeature := "br-shaped-feature"
	noBeadsTemplate := h.burpvalve(noBeads, nil, "commit", "--root", noBeads, "--feature", noBeadsFeature, "--responses-template")
	h.requireExit(noBeadsTemplate, 0)
	noBeadsResponsesPath := filepath.Join(h.workDir, "no-beads-responses.json")
	h.writeAbs(noBeadsResponsesPath, mustJSON(t, passingResponses(t, noBeadsTemplate.Stdout)))
	noBeadsWritten := h.burpvalve(noBeads, nil, "commit", "--root", noBeads, "--feature", noBeadsFeature, "--responses", noBeadsResponsesPath)
	h.requireExit(noBeadsWritten, 2)
	var noBeadsResult backpressure.PreCommitResult
	h.decodeJSON(noBeadsWritten.Stdout, &noBeadsResult)
	noBeadsArtifact := h.readArtifact(noBeads, noBeadsResult.ArtifactPath)
	if len(noBeadsArtifact.BeadIDs) != 0 ||
		len(noBeadsArtifact.Feature.BeadIDs) != 0 ||
		noBeadsArtifact.Feature.SourceBead != "" ||
		noBeadsArtifact.Feature.DiffCluster != "explicit:"+noBeadsFeature {
		t.Fatalf("non-Beads attestation should not invent bead metadata: %#v", noBeadsArtifact)
	}
	noBeadsList := h.burpvalve(noBeads, nil, "attestations", "list", "--root", noBeads, "--json")
	h.requireExit(noBeadsList, 0)
	var noBeadsListResult struct {
		Records []attestations.Record `json:"records"`
	}
	h.decodeJSON(noBeadsList.Stdout, &noBeadsListResult)
	if len(noBeadsListResult.Records) != 1 ||
		len(noBeadsListResult.Records[0].BeadIDs) != 0 ||
		!containsString(noBeadsListResult.Records[0].FeatureIDs, noBeadsFeature) {
		t.Fatalf("attestation query should work without Beads: %#v", noBeadsListResult.Records)
	}
	h.git(noBeads, nil, "add", noBeadsResult.ArtifactPath)
	h.git(noBeads, nil, "commit", "-q", "--no-verify", "-m", "plain delivery")
	h.assertGitClean(noBeads)
}

func TestE2EExplainRecoveryScenarios(t *testing.T) {
	h := newE2EHarness(t)

	setupRepo := h.newRepo("explain-setup")
	setupJSON := h.burpvalve(setupRepo, nil, "setup", "--target", setupRepo, "--json", "--no-beads", "--no-ntm")
	h.requireExit(setupJSON, 0)
	setupExplain := h.burpvalveInput(setupRepo, setupJSON.Stdout, nil, "explain", "--json", "-")
	h.requireExit(setupExplain, 0)
	setupExplanation := decodeExplain(t, setupExplain.Stdout)
	if setupExplanation.InputType != "setup" ||
		setupExplanation.OriginalStatus != "blocked" ||
		!setupExplanation.Fatal ||
		!hasExplainBlocker(setupExplanation, "git-repo") ||
		!containsString(setupExplanation.NextSteps, "git init") {
		t.Fatalf("setup explanation should preserve structured blocker and recovery command: %#v", setupExplanation)
	}
	setupHuman := h.burpvalveInput(setupRepo, setupJSON.Stdout, nil, "explain", "--color", "never", "-")
	h.requireExit(setupHuman, 0)
	h.assertOutputContains(setupHuman, "Burpvalve explain", "Input: setup", "Fatal: yes", "git init")

	lintRepo := h.newBackpressureRepo("explain-lint")
	h.git(lintRepo, nil, "add", ".")
	h.git(lintRepo, nil, "commit", "-q", "--no-verify", "-m", "baseline")
	lintJSON := h.burpvalve(lintRepo, nil, "lint", "--root", lintRepo, "--json")
	h.requireExit(lintJSON, 0)
	lintExplain := h.burpvalveInput(lintRepo, lintJSON.Stdout, nil, "explain", "--json", "-")
	h.requireExit(lintExplain, 0)
	lintExplanation := decodeExplain(t, lintExplain.Stdout)
	if lintExplanation.InputType != "lint" ||
		lintExplanation.OriginalStatus != backpressure.LintStatusNotEnforced ||
		lintExplanation.Fatal ||
		!strings.Contains(lintExplanation.Summary, "not enforcing") ||
		!strings.Contains(lintExplanation.WhyItMatters, "not proof") ||
		!hasExplainBlocker(lintExplanation, "lint_commands") {
		t.Fatalf("lint no-op explanation should be truthful missing evidence: %#v", lintExplanation)
	}
	lintHuman := h.burpvalveInput(lintRepo, lintJSON.Stdout, nil, "explain", "--color", "never", "-")
	h.requireExit(lintHuman, 0)
	h.assertOutputContains(lintHuman, "Input: lint", "not enforcing", "not proof")
	h.assertGitClean(lintRepo)

	completionRepo := h.newRepo("explain-completion")
	completionPath := filepath.Join(h.workDir, "missing-completion", "burpvalve.fish")
	missingBin := filepath.Join(h.workDir, "missing-bin")
	completionJSON := h.burpvalve(completionRepo, nil,
		"completion", "verify",
		"--target", completionRepo,
		"--shell", "fish",
		"--path", completionPath,
		"--bin-dir", missingBin,
		"--json",
	)
	h.requireExit(completionJSON, 0)
	completionExplain := h.burpvalveInput(completionRepo, completionJSON.Stdout, nil, "explain", "--json", "-")
	h.requireExit(completionExplain, 0)
	completionExplanation := decodeExplain(t, completionExplain.Stdout)
	if completionExplanation.InputType != "completion_verify" ||
		completionExplanation.OriginalStatus != "action_needed" ||
		completionExplanation.Fatal ||
		!hasExplainBlocker(completionExplanation, "completion_script") ||
		!strings.Contains(strings.Join(completionExplanation.NextSteps, "\n"), "completion install --shell fish") {
		t.Fatalf("completion verify explanation should name missing completion recovery: %#v", completionExplanation)
	}
	completionHuman := h.burpvalveInput(completionRepo, completionJSON.Stdout, nil, "explain", "--color", "never", "-")
	h.requireExit(completionHuman, 0)
	h.assertOutputContains(completionHuman, "Input: completion_verify", "Shell completion setup needs action", "completion install --shell fish")

	plain := h.newBackpressureRepo("explain-plain-commit")
	h.write(plain, "src/app.go", "package app\n\nconst Version = 1\n")
	h.git(plain, nil, "add", ".")
	h.git(plain, nil, "commit", "-q", "--no-verify", "-m", "baseline")
	h.write(plain, "src/app.go", "package app\n\nconst Version = 2\n")
	h.git(plain, nil, "add", "src/app.go")
	plainBlocked := h.burpvalve(plain, nil, "commit", "--root", plain, "--feature", "plain-explain")
	h.requireExit(plainBlocked, 2)
	var plainBlockedResult backpressure.PreCommitResult
	h.decodeJSON(plainBlocked.Stdout, &plainBlockedResult)
	plainExplain := h.burpvalveInput(plain, plainBlocked.Stdout, nil, "explain", "--json", "-")
	h.requireExit(plainExplain, 0)
	plainExplanation := decodeExplain(t, plainExplain.Stdout)
	if plainExplanation.InputType != "commit" ||
		!plainExplanation.Fatal ||
		!hasExplainBlocker(plainExplanation, "blocked_report") ||
		!strings.Contains(plainExplanation.Blockers[0].Command, plainBlockedResult.BlockedReportPath) {
		t.Fatalf("commit result explanation should point to blocked report: %#v", plainExplanation)
	}
	plainStatus := h.git(plain, nil, "status", "--short")
	h.requireExit(plainStatus, 0)
	if !strings.Contains(plainStatus.Stdout, "M  src/app.go") || !strings.Contains(plainStatus.Stdout, "?? log/") {
		t.Fatalf("plain blocked scenario should leave exact expected dirty state:\n%s", plainStatus.Stdout)
	}

	beads := h.newBackpressureRepo("explain-bead-blocked")
	h.write(beads, "src/app.go", "package app\n\nconst Version = 1\n")
	h.git(beads, nil, "add", ".")
	h.git(beads, nil, "commit", "-q", "--no-verify", "-m", "baseline")
	h.write(beads, "src/app.go", "package app\n\nconst Version = 2\n")
	h.git(beads, nil, "add", "src/app.go")
	beadBlocked := h.burpvalve(beads, nil, "commit", "--root", beads, "--feature", "bead-explain", "--bead", "br-explain")
	h.requireExit(beadBlocked, 2)
	var beadBlockedResult backpressure.PreCommitResult
	h.decodeJSON(beadBlocked.Stdout, &beadBlockedResult)
	beadPathExplain := h.burpvalve(beads, nil, "explain", "--root", beads, "--json", beadBlockedResult.BlockedReportPath)
	h.requireExit(beadPathExplain, 0)
	beadExplanation := decodeExplain(t, beadPathExplain.Stdout)
	if beadExplanation.InputType != "blocked_report" ||
		!beadExplanation.Fatal ||
		!containsString(beadExplanation.BeadIDs, "br-explain") ||
		!containsString(beadExplanation.FeatureIDs, "bead-explain") ||
		!hasExplainBlocker(beadExplanation, "dry") ||
		!strings.Contains(strings.Join(beadExplanation.NextSteps, "\n"), "verifier evidence") {
		t.Fatalf("blocked report path explanation should preserve Beads-specific traceability: %#v", beadExplanation)
	}
	beadHuman := h.burpvalve(beads, nil, "explain", "--root", beads, "--color", "never", beadBlockedResult.BlockedReportPath)
	h.requireExit(beadHuman, 0)
	h.assertOutputContains(beadHuman, "Input: blocked_report", "Beads: br-explain", "spawn an independent read-only verifier")
	beadStatus := h.git(beads, nil, "status", "--short")
	h.requireExit(beadStatus, 0)
	if !strings.Contains(beadStatus.Stdout, "M  src/app.go") || !strings.Contains(beadStatus.Stdout, "?? log/") {
		t.Fatalf("bead blocked scenario should leave exact expected dirty state:\n%s", beadStatus.Stdout)
	}

	pass := h.newBackpressureRepo("explain-passing-artifact")
	h.write(pass, "src/app.go", "package app\n\nconst Version = 1\n")
	h.git(pass, nil, "add", ".")
	h.git(pass, nil, "commit", "-q", "--no-verify", "-m", "baseline")
	h.write(pass, "src/app.go", "package app\n\nconst Version = 2\n")
	h.git(pass, nil, "add", "src/app.go")
	passTemplate := h.burpvalve(pass, nil, "commit", "--root", pass, "--feature", "pass-explain", "--responses-template")
	h.requireExit(passTemplate, 0)
	passResponsesPath := filepath.Join(h.workDir, "explain-pass-responses.json")
	h.writeAbs(passResponsesPath, mustJSON(t, passingResponses(t, passTemplate.Stdout)))
	passWritten := h.burpvalve(pass, nil, "commit", "--root", pass, "--feature", "pass-explain", "--responses", passResponsesPath)
	h.requireExit(passWritten, 2)
	var passResult backpressure.PreCommitResult
	h.decodeJSON(passWritten.Stdout, &passResult)
	passExplain := h.burpvalve(pass, nil, "explain", "--root", pass, "--json", passResult.ArtifactPath)
	h.requireExit(passExplain, 0)
	passExplanation := decodeExplain(t, passExplain.Stdout)
	if passExplanation.InputType != "passing_attestation" ||
		passExplanation.Fatal ||
		len(passExplanation.Blockers) != 0 ||
		!containsString(passExplanation.FeatureIDs, "pass-explain") {
		t.Fatalf("passing attestation path explanation should be nonfatal: %#v", passExplanation)
	}
	h.git(pass, nil, "add", "src/app.go", passResult.ArtifactPath)
	h.git(pass, nil, "commit", "-q", "--no-verify", "-m", "passing explained artifact")
	h.assertGitClean(pass)

	for name, input := range map[string]string{
		"malformed": `{"command":`,
		"unknown":   `{"command":"unknown-burpvalve-command"}`,
	} {
		explain := h.burpvalveInput(setupRepo, input, nil, "explain", "--json", "-")
		h.requireExit(explain, 2)
		got := decodeExplain(t, explain.Stdout)
		if got.Status != "error" || !got.Fatal || !strings.Contains(got.WhyItMatters, "structured JSON") || len(got.NextSteps) == 0 {
			t.Fatalf("%s explain error should be structured and actionable: %#v", name, got)
		}
	}
}

func TestE2ESetupConfigInstallAndCompletionFirstRun(t *testing.T) {
	h := newE2EHarness(t)
	repo := h.newRepo("first-run")
	globalCompletion := filepath.Join(h.workDir, "global-completions", "burpvalve.bash")
	projectCompletion := filepath.Join(repo, ".config", "fish", "burpvalve.fish")
	globalBin := filepath.Join(h.workDir, "global-bin")
	skillsDir := filepath.Join(h.workDir, "skills")
	h.writeAbs(filepath.Join(h.xdg, "burpvalve", "config.json"), `{
  "schema_version": 1,
  "defaults": {
    "shell": "bash",
    "bin_dir": "`+escapeJSONPath(globalBin)+`",
    "completion": {
      "path": "`+escapeJSONPath(globalCompletion)+`"
    },
    "init": {
      "beads": false,
      "ntm": false
    }
  }
}`)
	h.write(repo, ".burpvalve.json", `{
  "schema_version": 1,
  "defaults": {
    "shell": "fish",
    "completion": {
      "path": "`+escapeJSONPath(projectCompletion)+`"
    }
  }
}`)

	before := h.burpvalve(repo, nil, "setup", "--target", repo, "--json", "--no-beads", "--no-ntm")
	h.requireExit(before, 0)
	var beforeSetup scaffold.Report
	h.decodeJSON(before.Stdout, &beforeSetup)
	if beforeSetup.Status != "blocked" || beforeSetup.Summary.Ready || !hasSetupCheck(beforeSetup, "git-repo", scaffold.StatusMissing) {
		t.Fatalf("first setup should report missing git repo, got %#v", beforeSetup)
	}

	init := h.burpvalve(repo, nil, "init", "--target", repo, "--force", "--json", "--git-init", "--no-beads", "--no-ntm")
	h.requireExit(init, 0)
	h.git(repo, nil, "config", "user.email", "michael-bltzr@users.noreply.github.com")
	h.git(repo, nil, "config", "user.name", "Test User")
	h.assertPathExists(repo, ".git")
	h.assertPathExists(repo, ".githooks/pre-commit")
	hooksPath := h.git(repo, nil, "config", "--get", "core.hooksPath")
	h.requireExit(hooksPath, 0)
	if strings.TrimSpace(hooksPath.Stdout) != ".githooks" {
		t.Fatalf("init should configure hooks after git init, got %q", hooksPath.Stdout)
	}
	if _, err := os.Stat(filepath.Join(h.home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatalf("project init should not mutate global shell rc, stat=%v", err)
	}

	after := h.burpvalve(repo, nil, "setup", "--target", repo, "--json", "--no-beads", "--no-ntm")
	h.requireExit(after, 0)
	var afterSetup scaffold.Report
	h.decodeJSON(after.Stdout, &afterSetup)
	if afterSetup.CommandPath != h.binary || afterSetup.RepoBinPath != "" || afterSetup.HookCommandSource != "path" {
		t.Fatalf("setup should distinguish global PATH command from missing repo-local fallback: command=%q repo=%q hook=%q", afterSetup.CommandPath, afterSetup.RepoBinPath, afterSetup.HookCommandSource)
	}

	config := h.burpvalve(repo, nil, "config", "--target", repo, "--json")
	h.requireExit(config, 0)
	var configView struct {
		GlobalFound  bool `json:"global_found"`
		ProjectFound bool `json:"project_found"`
		Defaults     struct {
			Shell      string `json:"shell"`
			Completion struct {
				Path string `json:"path"`
			} `json:"completion"`
		} `json:"defaults"`
		Sources []struct {
			Key    string `json:"key"`
			Source string `json:"source"`
		} `json:"sources"`
	}
	h.decodeJSON(config.Stdout, &configView)
	if !configView.GlobalFound || !configView.ProjectFound || configView.Defaults.Shell != "fish" || configView.Defaults.Completion.Path != projectCompletion {
		t.Fatalf("project config should override global config: %#v", configView)
	}
	if !hasConfigSource(configView.Sources, "defaults.shell", "project") || !hasConfigSource(configView.Sources, "defaults.bin_dir", "global") {
		t.Fatalf("config sources should show project/global precedence: %#v", configView.Sources)
	}

	guide := h.burpvalve(repo, nil, "completion")
	h.requireExit(guide, 0)
	if !strings.Contains(guide.Stdout, "Configured shell:") || !strings.Contains(guide.Stdout, "fish") || !strings.Contains(guide.Stdout, projectCompletion) {
		t.Fatalf("completion guide should use project config defaults:\n%s", guide.Stdout)
	}
	if strings.Contains(guide.Stdout, "complete -c burpvalve") {
		t.Fatalf("no-arg completion guide should not dump raw fish script:\n%s", guide.Stdout)
	}

	installCompletion := h.burpvalve(repo, nil, "completion", "install", "--force", "--no-path")
	h.requireExit(installCompletion, 0)
	h.assertPathExists(repo, ".config/fish/burpvalve.fish")
	if strings.Contains(installCompletion.Stdout, "command shim") || strings.Contains(installCompletion.Stdout, "PATH startup") {
		t.Fatalf("completion --no-path should not plan command PATH writes:\n%s", installCompletion.Stdout)
	}
	verifyCompletion := h.burpvalve(repo, nil, "completion", "verify", "--target", repo, "--no-path", "--json")
	h.requireExit(verifyCompletion, 0)
	var completionReport completionVerifyReport
	h.decodeJSON(verifyCompletion.Stdout, &completionReport)
	if completionReport.Status != "ready" || !completionReport.Verified || completionReport.Shell != "fish" || completionReport.CompletionPath != projectCompletion {
		t.Fatalf("completion verify should use project config and require no real shell sourcing: %#v", completionReport)
	}

	archive := writeMinimalInstallArchive(t)
	stdout, stderr, err := runInstallScript(t, h.repoRoot,
		"--from-archive", archive,
		"--skills-dir", skillsDir,
		"--bin-dir", globalBin,
		"--yes",
	)
	if err != nil {
		t.Fatalf("archive install failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stderr, "Burpvalve install plan") || !strings.Contains(stdout, "Verify with: "+filepath.Join(globalBin, "burpvalve")+" --version") {
		t.Fatalf("archive install should preview writes and print verification guidance\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "burpvalve", "SKILL.md")); err != nil {
		t.Fatalf("skill-shaped archive should install SKILL.md: %v", err)
	}
	commandPath := filepath.Join(globalBin, "burpvalve")
	if info, err := os.Lstat(commandPath); err != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode()&0o111 == 0 {
		t.Fatalf("install command should be executable file, not symlink: info=%v err=%v", info, err)
	}
	movedSkills := skillsDir + "-moved"
	if err := os.Rename(skillsDir, movedSkills); err != nil {
		t.Fatal(err)
	}
	version := exec.Command(commandPath, "--version")
	version.Env = append(os.Environ(), "PATH=/usr/bin:/bin")
	if out, err := version.CombinedOutput(); err != nil || !strings.Contains(string(out), "burpvalve-test") {
		t.Fatalf("installed command should survive skills dir move: out=%q err=%v", string(out), err)
	}

	h.requireExit(h.git(repo, nil, "add", "."), 0)
	h.requireExit(h.git(repo, nil, "commit", "-q", "--no-verify", "-m", "first run scaffold"), 0)
	h.assertGitClean(repo)
	h.assertNoUnexpectedRootUntracked(repo)
}

func newE2EHarness(t *testing.T) *e2eHarness {
	t.Helper()
	repoRoot := findRepoRoot(t)
	workDir := t.TempDir()
	binDir := filepath.Join(workDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(binDir, "burpvalve")
	build := exec.Command("go", "build", "-trimpath", "-o", binary, "./cmd/burpvalve")
	build.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	build.Stdout = &stdout
	build.Stderr = &stderr
	if err := build.Run(); err != nil {
		t.Fatalf("build temp burpvalve binary: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	h := &e2eHarness{
		t:        t,
		repoRoot: repoRoot,
		workDir:  workDir,
		binDir:   binDir,
		binary:   binary,
		home:     filepath.Join(workDir, "home"),
		xdg:      filepath.Join(workDir, "xdg-config"),
		data:     filepath.Join(workDir, "xdg-data"),
		path:     binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	}
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("e2e binary: %s", h.binary)
			for _, result := range h.transcript {
				t.Log(result.String())
			}
		}
	})
	return h
}

func (h *e2eHarness) newRepo(name string) string {
	h.t.Helper()
	repo := filepath.Join(h.workDir, name)
	if err := os.MkdirAll(repo, 0o755); err != nil {
		h.t.Fatal(err)
	}
	return repo
}

func (h *e2eHarness) newBackpressureRepo(name string) string {
	h.t.Helper()
	repo := h.newRepo(name)
	h.git(repo, nil, "init", "-q")
	h.git(repo, nil, "config", "user.email", "michael-bltzr@users.noreply.github.com")
	h.git(repo, nil, "config", "user.name", "Test User")
	h.write(repo, "backpressure/manifest.yaml", `conditions:
  - id: dry
    path: backpressure/dry.md
    enabled: true
  - id: scope-control
    path: backpressure/scope-control.md
    enabled: true
lint_commands: []
`)
	h.write(repo, "backpressure/dry.md", "# DRY\n")
	h.write(repo, "backpressure/scope-control.md", "# Scope\n")
	return repo
}

type stagedFileWant struct {
	path            string
	oldPath         string
	status          string
	gitStatus       string
	gitStatusPrefix string
}

func hasStagedFile(files []backpressure.StagedPayloadFile, want stagedFileWant) bool {
	for _, file := range files {
		if file.Path != want.path || file.Status != want.status {
			continue
		}
		if want.oldPath != "" && file.OldPath != want.oldPath {
			continue
		}
		if want.gitStatus != "" && file.GitStatus != want.gitStatus {
			continue
		}
		if want.gitStatusPrefix != "" && !strings.HasPrefix(file.GitStatus, want.gitStatusPrefix) {
			continue
		}
		return true
	}
	return false
}

func (h *e2eHarness) burpvalve(dir string, extraEnv map[string]string, args ...string) e2eCommandResult {
	h.t.Helper()
	return h.run(dir, extraEnv, h.binary, args...)
}

func (h *e2eHarness) burpvalveInput(dir string, input string, extraEnv map[string]string, args ...string) e2eCommandResult {
	h.t.Helper()
	return h.runInput(dir, input, extraEnv, h.binary, args...)
}

func (h *e2eHarness) git(dir string, extraEnv map[string]string, args ...string) e2eCommandResult {
	h.t.Helper()
	return h.run(dir, extraEnv, "git", args...)
}

func (h *e2eHarness) run(dir string, extraEnv map[string]string, name string, args ...string) e2eCommandResult {
	h.t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	env, relevant := h.env(extraEnv)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := e2eCommandResult{
		Dir:      dir,
		Name:     name,
		Args:     append([]string(nil), args...),
		Env:      relevant,
		ExitCode: 0,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
	if err != nil {
		result.ExitCode = -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
	}
	h.transcript = append(h.transcript, result)
	return result
}

func (h *e2eHarness) runInput(dir string, input string, extraEnv map[string]string, name string, args ...string) e2eCommandResult {
	h.t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	env, relevant := h.env(extraEnv)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := e2eCommandResult{
		Dir:      dir,
		Name:     name,
		Args:     append([]string(nil), args...),
		Env:      relevant,
		ExitCode: 0,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
	if err != nil {
		result.ExitCode = -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
	}
	h.transcript = append(h.transcript, result)
	return result
}

func (h *e2eHarness) env(extra map[string]string) ([]string, map[string]string) {
	values := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	values["HOME"] = h.home
	values["XDG_CONFIG_HOME"] = h.xdg
	values["XDG_DATA_HOME"] = h.data
	values["PATH"] = h.path
	values["GIT_TERMINAL_PROMPT"] = "0"
	values["NO_COLOR"] = "1"
	relevant := map[string]string{
		"HOME":                values["HOME"],
		"XDG_CONFIG_HOME":     values["XDG_CONFIG_HOME"],
		"XDG_DATA_HOME":       values["XDG_DATA_HOME"],
		"PATH_PREFIX":         h.binDir,
		"GIT_TERMINAL_PROMPT": values["GIT_TERMINAL_PROMPT"],
		"NO_COLOR":            values["NO_COLOR"],
		"BURPVALVE_BINARY":    h.binary,
	}
	for key, value := range extra {
		values[key] = value
		relevant[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	return env, relevant
}

func (h *e2eHarness) envWithoutBR() map[string]string {
	h.t.Helper()
	toolDir := filepath.Join(h.workDir, "path-without-br")
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		h.t.Fatal(err)
	}
	for _, name := range []string{"git", "sh"} {
		target, err := exec.LookPath(name)
		if err != nil {
			h.t.Fatalf("find %s for no-br test path: %v", name, err)
		}
		link := filepath.Join(toolDir, name)
		if err := os.Symlink(target, link); err != nil && !os.IsExist(err) {
			h.t.Fatalf("link %s into no-br test path: %v", name, err)
		}
	}
	return map[string]string{
		"PATH": h.binDir + string(os.PathListSeparator) + toolDir,
	}
}

func (h *e2eHarness) write(root, rel, body string) {
	h.t.Helper()
	h.writeAbs(filepath.Join(root, filepath.FromSlash(rel)), body)
}

func (h *e2eHarness) writeAbs(path, body string) {
	h.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

func (h *e2eHarness) removeAll(root, rel string) {
	h.t.Helper()
	if err := os.RemoveAll(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		h.t.Fatal(err)
	}
}

func (h *e2eHarness) glob(root, pattern string) []string {
	h.t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
	if err != nil {
		h.t.Fatal(err)
	}
	rel := make([]string, 0, len(matches))
	for _, match := range matches {
		path, err := filepath.Rel(root, match)
		if err != nil {
			h.t.Fatal(err)
		}
		rel = append(rel, filepath.ToSlash(path))
	}
	sort.Strings(rel)
	return rel
}

func (h *e2eHarness) decodeJSON(body string, dst any) {
	h.t.Helper()
	if err := json.Unmarshal([]byte(body), dst); err != nil {
		h.t.Fatalf("decode JSON: %v\n%s", err, body)
	}
}

func (h *e2eHarness) requireExit(result e2eCommandResult, want int) {
	h.t.Helper()
	if result.ExitCode != want {
		h.t.Fatalf("exit = %d, want %d\n%s", result.ExitCode, want, result.String())
	}
}

func (h *e2eHarness) gitStatus(root string) string {
	h.t.Helper()
	status := h.git(root, nil, "status", "--short", "--untracked-files=all")
	h.requireExit(status, 0)
	return status.Stdout
}

func (h *e2eHarness) assertGitStatusUnchanged(root string, before string) {
	h.t.Helper()
	after := h.gitStatus(root)
	if after != before {
		h.t.Fatalf("git status changed unexpectedly\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func (h *e2eHarness) assertOutputContains(result e2eCommandResult, needles ...string) {
	h.t.Helper()
	output := result.Stdout + result.Stderr
	for _, needle := range needles {
		if !strings.Contains(output, needle) {
			h.t.Fatalf("command output missing %q\n%s", needle, result.String())
		}
	}
}

func (h *e2eHarness) assertPathExists(root, rel string) {
	h.t.Helper()
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		h.t.Fatalf("expected path %s to exist: %v", rel, err)
	}
}

func (h *e2eHarness) readArtifact(root, rel string) attestations.Artifact {
	h.t.Helper()
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		h.t.Fatal(err)
	}
	assertE2EFormatterSafeArtifactJSON(h.t, body)
	var artifact attestations.Artifact
	if err := json.Unmarshal(body, &artifact); err != nil {
		h.t.Fatalf("decode artifact: %v\n%s", err, body)
	}
	return artifact
}

func assertE2EFormatterSafeArtifactJSON(t *testing.T, body []byte) {
	t.Helper()
	if !bytes.HasSuffix(body, []byte("\n")) {
		t.Fatalf("artifact JSON missing trailing newline:\n%s", string(body))
	}
	withoutFinalNewline := bytes.TrimSuffix(body, []byte("\n"))
	if bytes.HasSuffix(withoutFinalNewline, []byte("\n")) {
		t.Fatalf("artifact JSON has more than one trailing newline:\n%s", string(body))
	}
	if !bytes.Contains(body, []byte("\n  \"")) {
		t.Fatalf("artifact JSON is not pretty-printed with two-space indentation:\n%s", string(body))
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("artifact JSON is invalid: %v\n%s", err, string(body))
	}
}

func (h *e2eHarness) assertArtifactBound(root string, result backpressure.PreCommitResult) {
	h.t.Helper()
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.ArtifactPath)))
	if err != nil {
		h.t.Fatal(err)
	}
	var artifact attestations.Artifact
	if err := json.Unmarshal(body, &artifact); err != nil {
		h.t.Fatalf("decode artifact: %v\n%s", err, body)
	}
	if artifact.ArtifactKind != attestations.ArtifactPassing {
		h.t.Fatalf("artifact kind = %q", artifact.ArtifactKind)
	}
	if artifact.StagedPayloadHash == "" || artifact.StagedPayloadHash != result.Plan.StagedPayloadHash {
		h.t.Fatalf("artifact payload hash %q does not match result %q", artifact.StagedPayloadHash, result.Plan.StagedPayloadHash)
	}
	wantFeature := ""
	if len(result.Plan.Features) > 0 {
		wantFeature = result.Plan.Features[0].ID
	}
	if artifact.Feature.ID != wantFeature {
		h.t.Fatalf("artifact feature = %#v, want %q", artifact.Feature, wantFeature)
	}
}

func (h *e2eHarness) assertGitClean(root string) {
	h.t.Helper()
	status := h.git(root, nil, "status", "--short")
	h.requireExit(status, 0)
	if strings.TrimSpace(status.Stdout) != "" {
		h.t.Fatalf("git status should be clean after successful hook commit:\n%s", status.Stdout)
	}
}

func (h *e2eHarness) assertNoUnexpectedRootUntracked(root string) {
	h.t.Helper()
	status := h.git(root, nil, "status", "--short", "--untracked-files=all")
	h.requireExit(status, 0)
	for _, line := range strings.Split(strings.TrimSpace(status.Stdout), "\n") {
		if strings.HasPrefix(line, "?? ") {
			h.t.Fatalf("unexpected untracked file after successful hook commit: %s\n%s", line, status.Stdout)
		}
	}
}

func e2eCheckByID(checks []scaffold.Check, id string) *scaffold.Check {
	for i := range checks {
		if checks[i].ID == id {
			return &checks[i]
		}
	}
	return nil
}

func passingResponses(t *testing.T, template string) backpressure.Responses {
	t.Helper()
	var responses backpressure.Responses
	if err := json.Unmarshal([]byte(template), &responses); err != nil {
		t.Fatalf("decode response template: %v\n%s", err, template)
	}
	now := time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC)
	responses.Atomicity.OneFeatureOrFix = true
	responses.Atomicity.Message = "E2E staged payload is one feature."
	for i := range responses.Conditions {
		responses.Conditions[i].Verifier = attestations.Verifier{
			Kind:            attestations.VerifierIndependentSubagent,
			Agent:           "e2e-verifier",
			Model:           "test",
			Runtime:         "go-test",
			SeparateContext: true,
			EvidenceRef:     "e2e transcript",
			CreatedAt:       &now,
		}
		responses.Conditions[i].SubagentConfirmed = true
		responses.Conditions[i].SubagentModel = "test"
		responses.Conditions[i].Verdict = attestations.VerdictPass
		responses.Conditions[i].Message = "E2E verifier accepted this condition."
		responses.Conditions[i].Evidence = []string{"E2E fixture evidence for " + responses.Conditions[i].ConditionID}
		responses.Conditions[i].NextAction = ""
	}
	return responses
}

func verifierSubmitBody(t *testing.T, begin backpressure.BeginResponsesResult, conditionID string) string {
	t.Helper()
	now := time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC)
	return mustJSON(t, backpressure.SubmitVerifierInput{
		ResponseCondition: backpressure.ResponseCondition{
			ConditionID:    conditionID,
			ConditionFile:  conditionPath(begin, conditionID),
			VerifierPolicy: attestations.VerifierPolicyIndependentRequired,
			Verifier: attestations.Verifier{
				Kind:            attestations.VerifierIndependentSubagent,
				Agent:           "e2e-verifier",
				Model:           "test",
				Runtime:         "go-test",
				SeparateContext: true,
				EvidenceRef:     "e2e transcript",
				CreatedAt:       &now,
			},
			SubagentConfirmed: true,
			SubagentModel:     "test",
			Verdict:           attestations.VerdictPass,
			Message:           "E2E verifier accepted " + conditionID + ".",
			Evidence:          []string{"E2E fixture evidence for " + conditionID},
		},
		StagedPayloadHash: begin.StagedPayloadHash,
		ManifestHash:      begin.ManifestHash,
		ConditionFileHash: begin.Plan.ConditionFileHashes[conditionID],
	})
}

func conditionPath(begin backpressure.BeginResponsesResult, conditionID string) string {
	for _, condition := range begin.Plan.Matrix.Conditions {
		if condition.ID == conditionID {
			return condition.Path
		}
	}
	return ""
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(body) + "\n"
}

func hasSetupCheck(report scaffold.Report, id string, status scaffold.CheckStatus) bool {
	for _, check := range report.Checks {
		if check.ID == id && check.Status == status {
			return true
		}
	}
	return false
}

func hasConfigSource(sources []struct {
	Key    string `json:"key"`
	Source string `json:"source"`
}, key string, source string) bool {
	for _, item := range sources {
		if item.Key == key && item.Source == source {
			return true
		}
	}
	return false
}

func hasExplainBlocker(got explainResponse, id string) bool {
	for _, blocker := range got.Blockers {
		if blocker.ID == id {
			return true
		}
	}
	return false
}

func escapeJSONPath(path string) string {
	return strings.ReplaceAll(path, `\`, `\\`)
}

func (r e2eCommandResult) String() string {
	envKeys := make([]string, 0, len(r.Env))
	for key := range r.Env {
		envKeys = append(envKeys, key)
	}
	sort.Strings(envKeys)
	env := make([]string, 0, len(envKeys))
	for _, key := range envKeys {
		env = append(env, key+"="+r.Env[key])
	}
	return fmt.Sprintf("$ (cd %s && %s %s)\nenv: %s\nexit=%d\nstdout:\n%s\nstderr:\n%s", r.Dir, r.Name, strings.Join(r.Args, " "), strings.Join(env, " "), r.ExitCode, r.Stdout, r.Stderr)
}
