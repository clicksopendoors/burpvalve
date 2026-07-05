package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"burpvalve/internal/attestations"
	"burpvalve/internal/backpressure"
	"burpvalve/internal/scaffold"
)

type explainResponse struct {
	SchemaVersion  int    `json:"schema_version"`
	Command        string `json:"command"`
	InputType      string `json:"input_type"`
	Status         string `json:"status"`
	OriginalStatus string `json:"original_status"`
	Summary        string `json:"summary"`
	WhyItMatters   string `json:"why_it_matters"`
	Fatal          bool   `json:"fatal"`
	Config         *struct {
		ProjectFound bool `json:"project_found"`
		Sources      []struct {
			Key    string `json:"key"`
			Source string `json:"source"`
		} `json:"sources"`
	} `json:"config"`
	FeatureIDs []string `json:"feature_ids"`
	BeadIDs    []string `json:"bead_ids"`
	NextSteps  []string `json:"next_steps"`
	Blockers   []struct {
		ID              string `json:"id"`
		Status          string `json:"status"`
		Message         string `json:"message"`
		Command         string `json:"command"`
		Fatal           bool   `json:"fatal"`
		ConditionFile   string `json:"condition_file"`
		VerifierPolicy  string `json:"verifier_policy"`
		VerifierKind    string `json:"verifier_kind"`
		Verdict         string `json:"verdict"`
		NextAction      string `json:"next_action"`
		EvidenceMissing bool   `json:"evidence_missing"`
		Supplemental    bool   `json:"supplemental"`
		AdjudicationRef string `json:"adjudication_ref"`
	} `json:"blockers"`
}

func TestExplainSetupJSONIncludesRecoveryAndConfigSources(t *testing.T) {
	report := scaffold.Report{
		SchemaVersion:     1,
		Command:           "setup",
		Status:            "blocked",
		ReadinessSeverity: "blocked",
		Fatal:             true,
		Config: &scaffold.ConfigSummary{
			ProjectFound: true,
			Sources: []scaffold.ConfigSource{
				{Key: "defaults.init.repo_bin", Source: "project"},
			},
		},
		NextSteps: []scaffold.RecoveryStep{{
			ID:      "git-repo",
			Message: "Initialize Git before hooks can be installed.",
			Command: "git init",
			Fatal:   true,
		}},
	}
	stdout, stderr, err := executeBurpvalveCommandWithInput(mustJSONString(t, report), "explain", "--json", "-")
	if err != nil {
		t.Fatalf("explain setup failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("explain setup wrote stderr: %s", stderr)
	}
	got := decodeExplain(t, stdout)
	if got.InputType != "setup" || !got.Fatal || got.OriginalStatus != "blocked" {
		t.Fatalf("unexpected setup explanation: %#v", got)
	}
	if got.Config == nil || !got.Config.ProjectFound || !explainSourceContains(got.Config.Sources, "defaults.init.repo_bin", "project") {
		t.Fatalf("setup config source details missing: %#v", got.Config)
	}
	if len(got.NextSteps) != 1 || got.NextSteps[0] != "git init" {
		t.Fatalf("setup next steps wrong: %#v", got.NextSteps)
	}
	if len(got.Blockers) != 1 || got.Blockers[0].ID != "git-repo" || got.Blockers[0].Command != "git init" {
		t.Fatalf("setup blocker wrong: %#v", got.Blockers)
	}

	human, stderr, err := executeBurpvalveCommandWithInput(mustJSONString(t, report), "explain", "--color", "never", "-")
	if err != nil {
		t.Fatalf("human explain setup failed: %v\nstdout=%s\nstderr=%s", err, human, stderr)
	}
	for _, needle := range []string{"Burpvalve explain", "Input: setup", "Fatal: yes", "Config:", "Next steps", "git init"} {
		if !strings.Contains(human, needle) {
			t.Fatalf("human setup explanation missing %q:\n%s", needle, human)
		}
	}
}

func TestExplainLintNoopIsMissingEvidence(t *testing.T) {
	lint := backpressure.LintResult{
		SchemaVersion:    1,
		Command:          "lint",
		Status:           backpressure.StatusPassed,
		Message:          "no executable lint commands declared; lint-rules wishlist skipped",
		Enforced:         false,
		CommandCount:     0,
		EvidenceStrength: "none",
		EnforcementLevel: "scaffold-only",
		NextSteps:        []string{"Add exact executable lint_commands entries to backpressure/manifest.yaml when this project is ready for deterministic lint enforcement."},
	}
	stdout, stderr, err := executeBurpvalveCommandWithInput(mustJSONString(t, lint), "explain", "--json", "-")
	if err != nil {
		t.Fatalf("explain lint no-op failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("explain lint no-op wrote stderr: %s", stderr)
	}
	got := decodeExplain(t, stdout)
	if got.InputType != "lint" || got.Fatal {
		t.Fatalf("unexpected lint explanation: %#v", got)
	}
	if !strings.Contains(got.Summary, "not enforcing") || !strings.Contains(got.WhyItMatters, "not proof") {
		t.Fatalf("lint no-op should be explained as missing evidence: %#v", got)
	}
	if len(got.Blockers) != 1 || got.Blockers[0].ID != "lint_commands" || got.Blockers[0].Status != "missing" {
		t.Fatalf("lint no-op blocker wrong: %#v", got.Blockers)
	}
}

func TestExplainInitAndRepairJSON(t *testing.T) {
	for _, command := range []string{"init", "repair"} {
		t.Run(command, func(t *testing.T) {
			input := map[string]any{
				"schema_version":   1,
				"command":          command,
				"status":           "partial_success",
				"fatal":            true,
				"partial_success":  true,
				"mutating":         true,
				"created":          []string{"log/README.md"},
				"preserved":        []string{"AGENTS.md"},
				"next_steps":       []scaffold.RecoveryStep{{ID: "AGENTS.md", Message: "Review existing AGENTS.md before appending generated sections.", Command: "burpvalve repair AGENTS.md", Fatal: true}},
				"conflicts":        []scaffold.ApplyConflict{{Path: "AGENTS.md", Message: "existing project instructions preserved"}},
				"generated_at":     time.Now().UTC(),
				"target_root":      t.TempDir(),
				"schema_version_v": 1,
			}
			stdout, stderr, err := executeBurpvalveCommandWithInput(mustJSONString(t, input), "explain", "--json", "-")
			if err != nil {
				t.Fatalf("explain %s failed: %v\nstdout=%s\nstderr=%s", command, err, stdout, stderr)
			}
			got := decodeExplain(t, stdout)
			if got.InputType != command || !got.Fatal || got.OriginalStatus != "partial_success" {
				t.Fatalf("unexpected %s explanation: %#v", command, got)
			}
			if len(got.Blockers) < 2 || got.NextSteps[0] != "burpvalve repair AGENTS.md" {
				t.Fatalf("%s explanation should include next steps and conflicts: %#v", command, got)
			}
		})
	}
}

func TestExplainConsumesRealLintJSONPipe(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)

	lintStdout, lintStderr, err := runBurpvalve(t, repoRoot, "lint", "--root", target, "--json")
	if err != nil {
		t.Fatalf("lint --json failed: %v\nstdout=%s\nstderr=%s", err, lintStdout, lintStderr)
	}
	if len(lintStderr) != 0 {
		t.Fatalf("lint --json should not write human summary to stderr:\n%s", lintStderr)
	}
	stdout, stderr, err := runBurpvalveWithInput(t, repoRoot, string(lintStdout), "explain", "--json", "-")
	if err != nil {
		t.Fatalf("explain real lint JSON failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	got := decodeExplain(t, string(stdout))
	if got.InputType != "lint" || len(got.Blockers) != 1 || got.Blockers[0].ID != "lint_commands" {
		t.Fatalf("real lint pipe explanation wrong: %#v", got)
	}
}

func TestExplainBlockedReportPathAndVerifierPolicyFailure(t *testing.T) {
	root := t.TempDir()
	created := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	artifact := artifactFixture(attestations.ArtifactBlocked, "policyhash", "br-policy", created)
	artifact.NextSteps = []string{"Spawn a read-only verifier for dry, then rerun burpvalve commit."}
	artifact.Conditions[0].Verdict = attestations.VerdictPass
	artifact.Conditions[0].VerifierPolicy = attestations.VerifierPolicyIndependentRequired
	artifact.Conditions[0].Verifier = attestations.Verifier{
		Kind:            attestations.VerifierMainAgent,
		Agent:           "codex",
		Model:           "test",
		Runtime:         "go-test",
		SeparateContext: false,
	}
	artifact.Conditions[0].Evidence = []string{"main agent checked this"}
	artifact.Conditions[0].Message = "main-agent evidence does not satisfy independent verifier policy"
	artifact.Conditions[0].NextAction = "spawn an independent read-only verifier"
	writeArtifactFixture(t, root, "log/backpressure/failed/policyhash.json", artifact)

	stdout, stderr, err := executeBurpvalveCommand("explain", "--root", root, "--json", "log/backpressure/failed/policyhash.json")
	if err != nil {
		t.Fatalf("explain blocked report failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("explain blocked report wrote stderr: %s", stderr)
	}
	got := decodeExplain(t, stdout)
	if got.InputType != "blocked_report" || !got.Fatal {
		t.Fatalf("unexpected blocked report explanation: %#v", got)
	}
	if len(got.Blockers) != 1 {
		t.Fatalf("expected policy blocker: %#v", got.Blockers)
	}
	blocker := got.Blockers[0]
	if blocker.VerifierPolicy != string(attestations.VerifierPolicyIndependentRequired) ||
		blocker.VerifierKind != string(attestations.VerifierMainAgent) ||
		blocker.Verdict != string(attestations.VerdictPass) ||
		blocker.EvidenceMissing {
		t.Fatalf("policy failure details wrong: %#v", blocker)
	}
	if !strings.Contains(blocker.NextAction, "independent") {
		t.Fatalf("policy failure next action missing: %#v", blocker)
	}
	if got.NextSteps[0] != "Spawn a read-only verifier for dry, then rerun burpvalve commit." {
		t.Fatalf("blocked report next steps wrong: %#v", got.NextSteps)
	}
}

func TestExplainPassingAttestationPath(t *testing.T) {
	root := t.TempDir()
	writeArtifactFixture(t, root, "backpressure/attestations/passhash.json", artifactFixture(attestations.ArtifactPassing, "passhash", "br-pass", time.Now().UTC()))

	stdout, stderr, err := executeBurpvalveCommand("explain", "--root", root, "--json", "passhash")
	if err != nil {
		t.Fatalf("explain passing attestation failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	got := decodeExplain(t, stdout)
	if got.InputType != "passing_attestation" || got.Fatal || len(got.Blockers) != 0 {
		t.Fatalf("passing attestation should not have blockers: %#v", got)
	}
	if len(got.NextSteps) != 1 || got.NextSteps[0] != "No artifact recovery step is required." {
		t.Fatalf("passing attestation next steps wrong: %#v", got.NextSteps)
	}
}

func TestExplainPassingAttestationSurfacesSupplementalDisagreement(t *testing.T) {
	root := t.TempDir()
	created := time.Date(2026, 7, 3, 15, 0, 0, 0, time.UTC)
	artifact := artifactFixture(attestations.ArtifactPassing, "supplementalhash", "br-supplemental", created)
	artifact.Conditions[0].Supplemental = []attestations.SupplementalVerifier{{
		Verifier: attestations.Verifier{
			Kind:            attestations.VerifierIndependentSubagent,
			Agent:           "ScarletMarsh",
			Model:           "gpt-5-codex",
			Runtime:         "codex-cli",
			SeparateContext: true,
			CreatedAt:       &created,
		},
		Verdict:       attestations.VerdictFail,
		Message:       "Supplemental verifier found a disagreement.",
		Evidence:      []string{"supplemental evidence"},
		TranscriptRef: "Agent Mail 3131",
		NextAction:    "Hold and escalate to RusticDog.",
	}}
	artifact.Conditions[0].Adjudication = &attestations.ResponseAdjudication{
		Authority:    "RusticDog",
		Summary:      "Adjudication is recorded for audit only.",
		FinalVerdict: attestations.VerdictPass,
		AuditRef:     "Agent Mail 3135",
	}
	writeArtifactFixture(t, root, "backpressure/attestations/supplementalhash.json", artifact)

	stdout, stderr, err := executeBurpvalveCommand("explain", "--root", root, "--json", "supplementalhash")
	if err != nil {
		t.Fatalf("explain passing attestation failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	got := decodeExplain(t, stdout)
	if got.InputType != "passing_attestation" || got.Fatal {
		t.Fatalf("supplemental disagreement should not change artifact fatality: %#v", got)
	}
	if len(got.Blockers) != 1 || !got.Blockers[0].Supplemental || got.Blockers[0].Status != "supplemental_fail" {
		t.Fatalf("supplemental disagreement blocker missing: %#v", got.Blockers)
	}
	if got.Blockers[0].AdjudicationRef != "Agent Mail 3135" || !strings.Contains(got.Blockers[0].NextAction, "Hold and escalate") {
		t.Fatalf("supplemental blocker should carry adjudication ref and hold instruction: %#v", got.Blockers[0])
	}
	if len(got.NextSteps) != 1 || !strings.Contains(got.NextSteps[0], "adjudication final_verdict is audit metadata only") {
		t.Fatalf("supplemental disagreement next steps wrong: %#v", got.NextSteps)
	}
}

func TestExplainCommitJSONPointsToBlockedReport(t *testing.T) {
	result := backpressure.PreCommitResult{
		SchemaVersion:     1,
		Command:           "commit",
		Status:            backpressure.StatusBlocked,
		Message:           "missing verifier responses",
		Fatal:             true,
		BlockedReportPath: "log/backpressure/failed/blocked.json",
		NextSteps:         []string{"Fill responses.json with verifier evidence, then rerun burpvalve commit."},
	}
	stdout, stderr, err := executeBurpvalveCommandWithInput(mustJSONString(t, result), "explain", "--json", "-")
	if err != nil {
		t.Fatalf("explain commit failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	got := decodeExplain(t, stdout)
	if got.InputType != "commit" || !got.Fatal || len(got.Blockers) != 1 {
		t.Fatalf("commit explanation wrong: %#v", got)
	}
	if got.Blockers[0].Command != "burpvalve explain log/backpressure/failed/blocked.json" {
		t.Fatalf("commit blocker should point to explain blocked report: %#v", got.Blockers[0])
	}
}

func TestExplainMalformedAndUnknownInput(t *testing.T) {
	for name, input := range map[string]string{
		"malformed":             `{"command":`,
		"unknown":               `{"command":"wat"}`,
		"missing_artifact_kind": `{"tool":"burpvalve"}`,
		"unknown_artifact_kind": `{"tool":"burpvalve","artifact_kind":"weird"}`,
		"incomplete_artifact":   `{"schema_version":1,"tool":"burpvalve","artifact_kind":"passing"}`,
	} {
		t.Run(name, func(t *testing.T) {
			stdout, stderr, err := executeBurpvalveCommandWithInput(input, "explain", "--json", "-")
			if err == nil {
				t.Fatalf("expected explain error for %s\nstdout=%s\nstderr=%s", name, stdout, stderr)
			}
			got := decodeExplain(t, stdout)
			if got.Status != "error" || !got.Fatal {
				t.Fatalf("expected JSON error explanation: %#v", got)
			}
			if !strings.Contains(got.WhyItMatters, "structured JSON") || len(got.NextSteps) == 0 {
				t.Fatalf("error explanation should include recovery guidance: %#v", got)
			}
			if stderr != "" {
				t.Fatalf("explain error should stay on stdout for JSON mode, stderr=%s", stderr)
			}
		})
	}
}

func TestExplainUnknownArtifactKindPathIsMalformed(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		"weird": `{
  "schema_version": 1,
  "tool": "burpvalve",
  "artifact_kind": "weird",
  "staged_payload_hash": "weird"
}`,
		"incomplete": `{
  "schema_version": 1,
  "tool": "burpvalve",
  "artifact_kind": "passing"
}`,
	} {
		t.Run(name, func(t *testing.T) {
			writeCmdTestFile(t, filepath.Join(root, "backpressure/attestations/"+name+".json"), body)
			stdout, stderr, err := executeBurpvalveCommand("explain", "--root", root, "--json", name)
			if err == nil {
				t.Fatalf("malformed artifact path should fail\nstdout=%s\nstderr=%s", stdout, stderr)
			}
			got := decodeExplain(t, stdout)
			if got.Status != "error" || !strings.Contains(got.Summary, "malformed") {
				t.Fatalf("malformed artifact path should be a malformed error: %#v", got)
			}
		})
	}
}

func TestExplainRobotsHelpDocumentsReadOnlyJSON(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("--robots", "explain", "-h")
	if err != nil {
		t.Fatalf("robots explain help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	for _, needle := range []string{"schema_version", "blockers", "read-only", "setup --json", "scraping human text"} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("robots explain help missing %q:\n%s", needle, stdout)
		}
	}
	if stderr != "" {
		t.Fatalf("robots explain help wrote stderr: %s", stderr)
	}
}

func TestLintHelpDocumentsJSONForExplainPipes(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("lint", "-h")
	if err != nil {
		t.Fatalf("lint help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "--json") || !strings.Contains(stdout, "machine-readable JSON") {
		t.Fatalf("lint help should document JSON mode for explain pipes:\n%s", stdout)
	}
}

func decodeExplain(t *testing.T, body string) explainResponse {
	t.Helper()
	var got explainResponse
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode explain response: %v\n%s", err, body)
	}
	return got
}

func mustJSONString(t *testing.T, value any) string {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func executeBurpvalveCommandWithInput(input string, args ...string) (string, string, error) {
	cmd := newRootCommand()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetIn(strings.NewReader(input))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func explainSourceContains(sources []struct {
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

func artifactFixture(kind attestations.ArtifactKind, payloadHash string, featureID string, created time.Time) attestations.Artifact {
	return attestations.Artifact{
		SchemaVersion:       1,
		Tool:                attestations.ToolName,
		ToolVersion:         attestations.ToolVersion,
		ArtifactKind:        kind,
		StagedPayloadHash:   "sha256:" + payloadHash,
		ManifestHash:        "sha256:manifest",
		ConditionOrder:      []string{"dry"},
		GeneratedBy:         attestations.Generator{Agent: "test", Model: "unit"},
		GitHeadBeforeCommit: "abc123",
		CreatedAt:           created,
		Feature: attestations.Feature{
			ID:         featureID,
			Kind:       "bead",
			Name:       featureID,
			SourceBead: featureID,
		},
		Atomicity: attestations.Atomicity{OneFeatureOrFix: kind == attestations.ArtifactPassing, Message: "one feature"},
		Conditions: []attestations.Condition{{
			ConditionID:       "dry",
			ConditionFile:     "backpressure/dry.md",
			ConditionFileHash: "sha256:dry",
			VerifierPolicy:    attestations.VerifierPolicyIndependentRequired,
			Verifier: attestations.Verifier{
				Kind:            attestations.VerifierIndependentSubagent,
				Agent:           "Verifier",
				Model:           "gpt-test",
				Runtime:         "go-test",
				SeparateContext: true,
				CreatedAt:       &created,
			},
			SubagentConfirmed: true,
			SubagentModel:     "gpt-test",
			Verdict:           attestations.VerdictPass,
			Message:           "dry passed",
			Evidence:          []string{"dry evidence"},
			Timestamp:         created,
		}},
	}
}

func writeArtifactFixture(t *testing.T, root string, rel string, artifact attestations.Artifact) {
	t.Helper()
	body, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExplainAttestationJSONFromStdin(t *testing.T) {
	artifact := artifactFixture(attestations.ArtifactBlocked, "stdinblocked", "br-stdin", time.Now().UTC())
	artifact.Conditions[0].Evidence = nil
	artifact.Conditions[0].Message = "verifier evidence is missing"
	stdout, stderr, err := executeBurpvalveCommandWithInput(mustJSONString(t, artifact), "explain", "--json", "-")
	if err != nil {
		t.Fatalf("explain stdin artifact failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	got := decodeExplain(t, stdout)
	if got.InputType != "blocked_report" || len(got.Blockers) != 1 || !got.Blockers[0].EvidenceMissing {
		t.Fatalf("stdin blocked artifact explanation wrong: %#v", got)
	}
}
