package backpressure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"burpvalve/internal/attestations"
)

var artifactTestTime = time.Date(2026, 6, 20, 3, 0, 0, 0, time.UTC)

func TestRunPreCommitWritesPassingArtifactAndRequiresStaging(t *testing.T) {
	root := fixtureProject(t)
	responsesPath := passingResponsesFile(t, root)
	result, err := RunPreCommit(context.Background(), PreCommitOptions{
		Root:            root,
		ExplicitFeature: "br-123",
		ResponsesPath:   responsesPath,
		Now:             func() time.Time { return artifactTestTime },
		Staged:          fixtureStaged(),
	})
	if err == nil {
		t.Fatal("unstaged passing artifact should block once")
	}
	if result.Status != StatusAttestationWritten {
		t.Fatalf("status = %q, want %q", result.Status, StatusAttestationWritten)
	}
	if !strings.Contains(result.Message, "git add "+result.ArtifactPath) {
		t.Fatalf("missing exact staging instruction: %q", result.Message)
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.ArtifactPath)))
	if err != nil {
		t.Fatalf("passing artifact not written: %v", err)
	}
	var artifact attestations.Artifact
	if err := json.Unmarshal(body, &artifact); err != nil {
		t.Fatal(err)
	}
	if err := artifact.ValidatePassing(ExpectedBinding(result.Plan)); err != nil {
		t.Fatalf("written artifact is not valid passing artifact: %v", err)
	}
	assertFormatterSafeArtifactJSON(t, body)
}

func TestRunPreCommitPreservesSupplementalAndAdjudication(t *testing.T) {
	root := fixtureProject(t)
	responses := passingResponses(t)
	responses.Conditions[0].Supplemental = []SupplementalVerifier{{
		Verifier: attestations.Verifier{
			Kind:            attestations.VerifierIndependentSubagent,
			Agent:           "ScarletMarsh",
			Model:           "gpt-5-codex",
			Runtime:         "codex-cli",
			SeparateContext: true,
		},
		Verdict:       attestations.VerdictFail,
		Message:       "Supplemental verifier disagreed with the primary pass.",
		Evidence:      []string{"Agent Mail 3131: supplemental disagreement"},
		TranscriptRef: "Agent Mail 3131",
		NextAction:    "Hold and escalate the disagreement.",
	}}
	responses.Conditions[0].Adjudication = &ResponseAdjudication{
		Authority:    "RusticDog",
		Summary:      "Adjudication records the review trail but does not override the primary verdict.",
		FinalVerdict: attestations.VerdictPass,
		AuditRef:     "Agent Mail 3135",
	}
	responsesPath := responsesFile(t, root, responses)

	result, err := RunPreCommit(context.Background(), PreCommitOptions{
		Root:            root,
		ExplicitFeature: "br-123",
		ResponsesPath:   responsesPath,
		Now:             func() time.Time { return artifactTestTime },
		Staged:          fixtureStaged(),
	})
	if err == nil {
		t.Fatal("unstaged passing artifact should block once")
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.ArtifactPath)))
	if err != nil {
		t.Fatalf("passing artifact not written: %v", err)
	}
	var artifact attestations.Artifact
	if err := json.Unmarshal(body, &artifact); err != nil {
		t.Fatal(err)
	}
	first := artifact.Conditions[0]
	if len(first.Supplemental) != 1 || first.Supplemental[0].TranscriptRef != "Agent Mail 3131" {
		t.Fatalf("supplemental verifier not preserved: %#v", first.Supplemental)
	}
	if first.Adjudication == nil || first.Adjudication.AuditRef != "Agent Mail 3135" {
		t.Fatalf("adjudication not preserved: %#v", first.Adjudication)
	}
	if err := artifact.ValidatePassing(ExpectedBinding(result.Plan)); err != nil {
		t.Fatalf("artifact should validate with supplemental metadata: %v", err)
	}
}

func TestRunPreCommitStagedValidArtifactExitsZero(t *testing.T) {
	root := fixtureProject(t)
	staged := fixtureStaged()
	plan, err := BuildPlan(context.Background(), Options{
		Root:            root,
		Mode:            "pre-commit",
		ExplicitFeature: "br-123",
		Staged:          staged,
	})
	if err != nil {
		t.Fatal(err)
	}
	artifact := BuildArtifact(plan, passingResponses(t), PreCommitOptions{
		Root: root,
		Now:  func() time.Time { return artifactTestTime },
	}, attestations.ArtifactPassing)
	body, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	artifactPath := AttestationPath(plan.StagedPayloadHash)
	staged.paths = append(staged.paths, artifactPath)
	staged.content[artifactPath] = string(body)

	result, err := RunPreCommit(context.Background(), PreCommitOptions{
		Root:            root,
		ExplicitFeature: "br-123",
		Staged:          staged,
	})
	if err != nil {
		t.Fatalf("staged valid artifact should pass: %v", err)
	}
	if result.Status != StatusPassed {
		t.Fatalf("status = %q, want %q", result.Status, StatusPassed)
	}
}

func TestRunPreCommitMissingResponsesWritesBlockedReport(t *testing.T) {
	root := fixtureProject(t)
	result, err := RunPreCommit(context.Background(), PreCommitOptions{
		Root:            root,
		ExplicitFeature: "br-123",
		Now:             func() time.Time { return artifactTestTime },
		Staged:          fixturePayloadOnlyStaged(),
	})
	if err == nil {
		t.Fatal("missing responses should block")
	}
	if result.Status != StatusBlocked {
		t.Fatalf("status = %q, want %q", result.Status, StatusBlocked)
	}
	if result.BlockedReportPath != "log/backpressure/failed/20260620T030000Z-blocked.json" {
		t.Fatalf("blocked report path = %q", result.BlockedReportPath)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(result.BlockedReportPath))); err != nil {
		t.Fatalf("blocked report not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(result.ArtifactPath))); !os.IsNotExist(err) {
		t.Fatalf("passing artifact should not be written on missing responses, stat err=%v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.BlockedReportPath)))
	if err != nil {
		t.Fatalf("blocked report not readable: %v", err)
	}
	assertFormatterSafeArtifactJSON(t, body)
	var artifact attestations.Artifact
	if err := json.Unmarshal(body, &artifact); err != nil {
		t.Fatal(err)
	}
	joinedSteps := strings.Join(artifact.NextSteps, "\n")
	for _, needle := range []string{
		"The valve (the fail-closed commit gate) burped this work unit back",
		"refused the atomic change being checked",
		"spawn an independent read-only verifier",
	} {
		if !strings.Contains(joinedSteps, needle) {
			t.Fatalf("blocked report next steps missing %q: %#v", needle, artifact.NextSteps)
		}
	}
}

func TestRunPreCommitDirectMissingResponsesKeepsGenericNextSteps(t *testing.T) {
	t.Setenv(hookContextEnv, "")
	t.Setenv(hookCommandSourceEnv, "")
	root := fixtureProject(t)
	result, err := RunPreCommit(context.Background(), PreCommitOptions{
		Root:            root,
		ExplicitFeature: "br-123",
		Now:             func() time.Time { return artifactTestTime },
		Staged:          fixturePayloadOnlyStaged(),
	})
	if err == nil {
		t.Fatal("missing responses should block")
	}
	joined := strings.Join(result.NextSteps, "\n")
	if strings.Contains(joined, "Pre-commit hook context") {
		t.Fatalf("direct commit should not include hook context next steps: %#v", result.NextSteps)
	}
	if !strings.Contains(joined, "verifier begin") {
		t.Fatalf("direct missing responses should still tell caller to begin verifier flow: %#v", result.NextSteps)
	}
}

func TestRunPreCommitHookMissingResponsesNamesHookContextAndSource(t *testing.T) {
	t.Setenv(hookContextEnv, "pre-commit")
	t.Setenv(hookCommandSourceEnv, "path")
	root := fixtureProject(t)
	result, err := RunPreCommit(context.Background(), PreCommitOptions{
		Root:            root,
		ExplicitFeature: "br-123",
		Now:             func() time.Time { return artifactTestTime },
		Staged:          fixturePayloadOnlyStaged(),
	})
	if err == nil {
		t.Fatal("missing responses should block")
	}
	joined := strings.Join(result.NextSteps, "\n")
	for _, needle := range []string{
		"Pre-commit hook context",
		"PATH burpvalve command",
		"Keep the current staged payload intact",
		"do not treat this hook failure as evidence that lint or verifier checks passed",
		"After the response file is current for this staged payload, rerun git commit",
	} {
		if !strings.Contains(joined, needle) {
			t.Fatalf("hook-aware next steps missing %q: %#v", needle, result.NextSteps)
		}
	}
}

func TestRunPreCommitHookAttestationWrittenNamesRerunDiscipline(t *testing.T) {
	t.Setenv(hookContextEnv, "pre-commit")
	t.Setenv(hookCommandSourceEnv, "repo-local")
	root := fixtureProject(t)
	responsesPath := passingResponsesFile(t, root)
	result, err := RunPreCommit(context.Background(), PreCommitOptions{
		Root:            root,
		ExplicitFeature: "br-123",
		ResponsesPath:   responsesPath,
		Now:             func() time.Time { return artifactTestTime },
		Staged:          fixtureStaged(),
	})
	if err == nil {
		t.Fatal("unstaged passing artifact should block once")
	}
	joined := strings.Join(result.NextSteps, "\n")
	for _, needle := range []string{
		"repo-local fallback binary",
		"git add " + result.ArtifactPath,
		"After staging the attestation, rerun git commit so the hook can revalidate the final payload.",
	} {
		if !strings.Contains(joined, needle) {
			t.Fatalf("attestation next steps missing %q: %#v", needle, result.NextSteps)
		}
	}
	if containsBurpLanguage(result.Message, result.NextSteps...) {
		t.Fatalf("attestation-written bounce is not a valve refusal and must not use burp language: message=%q next=%#v", result.Message, result.NextSteps)
	}
}

func TestRunPreCommitAutoDiscoversBoundResponses(t *testing.T) {
	root := fixtureProject(t)
	staged := fixturePayloadOnlyStaged()
	plan, err := BuildPlan(context.Background(), Options{
		Root:            root,
		Mode:            "pre-commit",
		ExplicitFeature: "br-123",
		Staged:          staged,
	})
	if err != nil {
		t.Fatal(err)
	}
	writeBoundResponses(t, root, plan, passingBoundResponses(t, plan))

	result, err := RunPreCommit(context.Background(), PreCommitOptions{
		Root:            root,
		ExplicitFeature: "br-123",
		Now:             func() time.Time { return artifactTestTime },
		Staged:          staged,
	})
	if err == nil {
		t.Fatal("auto-discovered responses should write unstaged attestation and block once")
	}
	if result.Status != StatusAttestationWritten || result.ResponsesPath != ResponsesPath(plan.StagedPayloadHash) {
		t.Fatalf("auto-discovery result = %#v err=%v", result, err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("bound auto-discovery should not warn: %#v", result.Warnings)
	}
}

func TestRunPreCommitReportsStaleAutoDiscoveredResponses(t *testing.T) {
	root := fixtureProject(t)
	staged := fixturePayloadOnlyStaged()
	plan, err := BuildPlan(context.Background(), Options{
		Root:            root,
		Mode:            "pre-commit",
		ExplicitFeature: "br-123",
		Staged:          staged,
	})
	if err != nil {
		t.Fatal(err)
	}
	stalePath := filepath.Join(root, filepath.FromSlash("log/backpressure/responses/stale.json"))
	if err := os.MkdirAll(filepath.Dir(stalePath), 0o755); err != nil {
		t.Fatal(err)
	}
	stale := passingBoundResponses(t, plan)
	stale.Binding.StagedPayloadHash = "sha256:stale"
	if body, err := json.Marshal(stale); err != nil {
		t.Fatal(err)
	} else if err := os.WriteFile(stalePath, body, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := RunPreCommit(context.Background(), PreCommitOptions{
		Root:            root,
		ExplicitFeature: "br-123",
		Now:             func() time.Time { return artifactTestTime },
		Staged:          staged,
	})
	if err == nil || !strings.Contains(err.Error(), "different staged payload") {
		t.Fatalf("expected stale response blocker, err=%v result=%#v", err, result)
	}
	if result.ResponsesPath != ResponsesPath(plan.StagedPayloadHash) || len(result.NextSteps) == 0 {
		t.Fatalf("stale response result missing recovery: %#v", result)
	}
}

func TestRunPreCommitExplicitResponsesOverrideAutoDiscoveryAndWarnForLegacy(t *testing.T) {
	root := fixtureProject(t)
	staged := fixtureStaged()
	plan, err := BuildPlan(context.Background(), Options{
		Root:            root,
		Mode:            "pre-commit",
		ExplicitFeature: "br-123",
		Staged:          staged,
	})
	if err != nil {
		t.Fatal(err)
	}
	stale := passingBoundResponses(t, plan)
	stale.Binding.StagedPayloadHash = "sha256:stale"
	writeBoundResponses(t, root, plan, stale)
	legacyPath := responsesFile(t, root, passingResponses(t))

	result, err := RunPreCommit(context.Background(), PreCommitOptions{
		Root:            root,
		ExplicitFeature: "br-123",
		ResponsesPath:   legacyPath,
		Now:             func() time.Time { return artifactTestTime },
		Staged:          staged,
	})
	if err == nil {
		t.Fatal("explicit legacy responses should still write unstaged attestation and block once")
	}
	if result.Status != StatusAttestationWritten || result.ResponsesPath != legacyPath {
		t.Fatalf("explicit responses did not override auto-discovery: %#v err=%v", result, err)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "legacy unbound responses") {
		t.Fatalf("legacy notice missing: %#v", result.Warnings)
	}
}

func assertFormatterSafeArtifactJSON(t *testing.T, body []byte) {
	t.Helper()
	if !bytes.HasSuffix(body, []byte("\n")) {
		t.Fatalf("artifact JSON missing trailing newline:\n%s", string(body))
	}
	if bytes.HasSuffix(bytes.TrimSuffix(body, []byte("\n")), []byte("\n")) {
		t.Fatalf("artifact JSON has more than one trailing newline:\n%q", string(body[len(body)-min(len(body), 8):]))
	}
	if !bytes.Contains(body, []byte("\n  \"")) {
		t.Fatalf("artifact JSON is not pretty-printed with two-space indentation:\n%s", string(body))
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("artifact JSON is invalid: %v\n%s", err, string(body))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func containsBurpLanguage(message string, steps ...string) bool {
	text := strings.ToLower(message + "\n" + strings.Join(steps, "\n"))
	text = strings.ReplaceAll(text, "burpvalve", "")
	return strings.Contains(text, "burp") || strings.Contains(text, "burped")
}

func TestPromptForFeatureUsesPlainPromptWhenTUIDisabled(t *testing.T) {
	var out bytes.Buffer
	feature, err := promptForFeature(Plan{
		BlockingReason:     "multiple possible features",
		StagedPayloadPaths: []string{"cmd/app/main.go"},
	}, errors.New("could not infer feature"), PreCommitOptions{
		Prompt: &PromptIO{
			In:  strings.NewReader("docs-example\n"),
			Out: &out,
			TUI: false,
		},
	})
	if err != nil {
		t.Fatalf("plain feature prompt failed: %v", err)
	}
	if feature != "docs-example" {
		t.Fatalf("feature = %q, want docs-example", feature)
	}
	output := out.String()
	for _, needle := range []string{
		"Feature for this commit",
		"could not infer feature",
		"Feature, bug fix, or bead id for this staged commit:",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("plain feature prompt missing %q:\n%s", needle, output)
		}
	}
}

func TestPromptColorHonorsColorMode(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "dumb")

	if !shouldUsePromptColor("always") {
		t.Fatal("color mode always should force prompt color")
	}
	if shouldUsePromptColor("never") {
		t.Fatal("color mode never should disable prompt color")
	}
	if shouldUsePromptColor("auto") {
		t.Fatal("auto prompt color should respect NO_COLOR and TERM=dumb")
	}
	if promptColor(PreCommitOptions{ColorMode: "never", Prompt: &PromptIO{Color: true}}) != true {
		t.Fatal("explicit PromptIO color should override PreCommitOptions color mode")
	}
}

func TestRunPreCommitFailingResponsesWriteBlockedReport(t *testing.T) {
	root := fixtureProject(t)
	responses := passingResponses(t)
	responses.Conditions[1].Verdict = attestations.VerdictFail
	responses.Conditions[1].Message = "DRY verifier found duplicated setup logic."
	responses.Conditions[1].Evidence = []string{"internal/scaffold/apply.go"}
	responses.Conditions[1].NextAction = "Extract shared helper."
	responsesPath := responsesFile(t, root, responses)

	result, err := RunPreCommit(context.Background(), PreCommitOptions{
		Root:            root,
		ExplicitFeature: "br-123",
		ResponsesPath:   responsesPath,
		Now:             func() time.Time { return artifactTestTime },
		Staged:          fixtureStaged(),
	})
	if err == nil {
		t.Fatal("failing responses should block")
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.BlockedReportPath)))
	if err != nil {
		t.Fatalf("blocked report not written: %v", err)
	}
	var artifact attestations.Artifact
	if err := json.Unmarshal(body, &artifact); err != nil {
		t.Fatal(err)
	}
	if artifact.ArtifactKind != attestations.ArtifactBlocked {
		t.Fatalf("artifact kind = %q", artifact.ArtifactKind)
	}
	if !strings.Contains(artifact.Atomicity.Message, "blocking verdict") {
		t.Fatalf("blocked message = %q", artifact.Atomicity.Message)
	}
}

func TestRunPreCommitAcceptsMainAgentWhenManifestAllows(t *testing.T) {
	root := fixtureProject(t)
	writeFile(t, root, ManifestPath, `conditions:
  - id: lint-rules
    path: backpressure/lint-rules.md
    enabled: true
    verifier_policy: main_agent_allowed
  - id: dry
    path: backpressure/dry.md
    enabled: true
  - id: anti-reward-hacking
    path: backpressure/anti-reward-hacking.md
    enabled: true
`)
	responses := passingResponses(t)
	responses.Conditions[0].SubagentConfirmed = false
	responses.Conditions[0].SubagentModel = ""
	responses.Conditions[0].Verifier = attestations.Verifier{
		Kind:    attestations.VerifierMainAgent,
		Agent:   "codex",
		Model:   "gpt-5",
		Runtime: "codex-cli",
	}
	responsesPath := responsesFile(t, root, responses)

	result, err := RunPreCommit(context.Background(), PreCommitOptions{
		Root:            root,
		ExplicitFeature: "br-123",
		ResponsesPath:   responsesPath,
		Now:             func() time.Time { return artifactTestTime },
		Staged:          fixtureStaged(),
	})
	if err == nil {
		t.Fatal("unstaged passing artifact should block once")
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.ArtifactPath)))
	if err != nil {
		t.Fatalf("passing artifact not written: %v", err)
	}
	var artifact attestations.Artifact
	if err := json.Unmarshal(body, &artifact); err != nil {
		t.Fatal(err)
	}
	first := artifact.Conditions[0]
	if first.VerifierPolicy != attestations.VerifierPolicyMainAgentAllowed {
		t.Fatalf("verifier policy = %q", first.VerifierPolicy)
	}
	if first.Verifier.Kind != attestations.VerifierMainAgent || first.SubagentConfirmed {
		t.Fatalf("main agent verifier not preserved without legacy subagent confirmation: %#v", first)
	}
	if err := artifact.ValidatePassing(ExpectedBinding(result.Plan)); err != nil {
		t.Fatalf("artifact should validate with main_agent_allowed policy: %v", err)
	}
}

func TestRunPreCommitBlocksMainAgentUnderIndependentPolicy(t *testing.T) {
	root := fixtureProject(t)
	responses := passingResponses(t)
	responses.Conditions[0].SubagentConfirmed = false
	responses.Conditions[0].SubagentModel = ""
	responses.Conditions[0].Verifier = attestations.Verifier{
		Kind:  attestations.VerifierMainAgent,
		Agent: "codex",
		Model: "gpt-5",
	}
	responses.Conditions[0].Message = "Main agent checked the condition without an independent verifier."
	responsesPath := responsesFile(t, root, responses)

	result, err := RunPreCommit(context.Background(), PreCommitOptions{
		Root:            root,
		ExplicitFeature: "br-123",
		ResponsesPath:   responsesPath,
		Now:             func() time.Time { return artifactTestTime },
		Staged:          fixtureStaged(),
	})
	if err == nil || !strings.Contains(err.Error(), "independent_required") {
		t.Fatalf("error = %v, want independent_required", err)
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.BlockedReportPath)))
	if err != nil {
		t.Fatalf("blocked report not written: %v", err)
	}
	var artifact attestations.Artifact
	if err := json.Unmarshal(body, &artifact); err != nil {
		t.Fatal(err)
	}
	first := artifact.Conditions[0]
	if first.Verifier.Kind != attestations.VerifierMainAgent || first.Verdict != attestations.VerdictUnknown {
		t.Fatalf("blocked verifier policy cell not preserved as unknown: %#v", first)
	}
	if !strings.Contains(first.NextAction, "independent read-only verifier") {
		t.Fatalf("next action does not explain independent verifier recovery: %#v", first)
	}
}

func TestRunPreCommitPromptFlowWritesSummaryBeforeArtifact(t *testing.T) {
	root := fixtureProject(t)
	staged := fixtureStaged()
	plan, err := BuildPlan(context.Background(), Options{
		Root:            root,
		Mode:            "pre-commit",
		ExplicitFeature: "br-123",
		Staged:          staged,
	})
	if err != nil {
		t.Fatal(err)
	}
	artifactPath := AttestationPath(plan.StagedPayloadHash)
	promptOut := &summaryBeforeArtifactWriter{t: t, root: root, artifactPath: artifactPath}

	result, err := RunPreCommit(context.Background(), PreCommitOptions{
		Root:            root,
		ExplicitFeature: "br-123",
		Now:             func() time.Time { return artifactTestTime },
		Staged:          staged,
		Prompt: &PromptIO{
			In: strings.NewReader(strings.Join([]string{
				"y",
				"y", "sonnet", "pass", "lint verifier passed",
				"y", "sonnet", "pass", "dry verifier passed",
				"y", "sonnet", "not_applicable", "No reward path changed.", "anti-reward verifier reviewed staged diff",
				"",
			}, "\n")),
			Out: promptOut,
		},
	})
	if err == nil {
		t.Fatal("unstaged passing artifact should block once")
	}
	if result.Status != StatusAttestationWritten {
		t.Fatalf("status = %q", result.Status)
	}
	if !promptOut.artifactAbsentAtSummary {
		t.Fatalf("summary was not observed before artifact write; output:\n%s", promptOut.String())
	}
	output := promptOut.String()
	for _, needle := range []string{
		"Backpressure commit gate",
		"Atomicity: does the staged diff contain exactly one atomic feature or bug fix?",
		"Matrix cell 1/3",
		"Dedicated subagent checked this exact condition for this exact feature?",
		"Verdict [pass|not_applicable|fail|unknown]",
		"Summary",
		"Artifact step follows this summary.",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("prompt output missing %q:\n%s", needle, output)
		}
	}
	if got := strings.Count(output, "Matrix cell "); got != 3 {
		t.Fatalf("prompted %d matrix cells, want 3:\n%s", got, output)
	}
}

func TestRunPreCommitPromptFlowBlocksMissingSubagentAsUnknown(t *testing.T) {
	root := fixtureProject(t)
	var promptOut bytes.Buffer
	result, err := RunPreCommit(context.Background(), PreCommitOptions{
		Root:            root,
		ExplicitFeature: "br-123",
		Now:             func() time.Time { return artifactTestTime },
		Staged:          fixtureStaged(),
		Prompt: &PromptIO{
			In: strings.NewReader(strings.Join([]string{
				"y",
				"n",
				"No dedicated verifier was spawned for lint-rules.",
				"cmd/app/main.go",
				"Spawn lint-rules verifier and retry.",
				"",
			}, "\n")),
			Out: &promptOut,
		},
	})
	if err == nil {
		t.Fatal("missing subagent confirmation should block")
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.BlockedReportPath)))
	if err != nil {
		t.Fatalf("blocked report not written: %v", err)
	}
	var artifact attestations.Artifact
	if err := json.Unmarshal(body, &artifact); err != nil {
		t.Fatal(err)
	}
	first := artifact.Conditions[0]
	if first.SubagentConfirmed || first.Verdict != attestations.VerdictUnknown {
		t.Fatalf("missing subagent should write unknown blocked cell: %#v", first)
	}
	if !strings.Contains(first.Message, "No dedicated verifier") {
		t.Fatalf("missing-subagent message not preserved: %#v", first)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(result.ArtifactPath))); !os.IsNotExist(err) {
		t.Fatalf("passing artifact should not be written, stat err=%v", err)
	}
}

func TestCollectPromptResponsesRequiresCellDetails(t *testing.T) {
	root := fixtureProject(t)
	plan, err := BuildPlan(context.Background(), Options{
		Root:            root,
		Mode:            "pre-commit",
		ExplicitFeature: "br-123",
		Staged:          fixtureStaged(),
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "not applicable message",
			input: strings.Join([]string{
				"y",
				"y", "sonnet", "not_applicable", "",
			}, "\n"),
			want: "Not-applicable reason",
		},
		{
			name: "fail evidence",
			input: strings.Join([]string{
				"y",
				"y", "sonnet", "fail", "lint-rules failed.", "",
			}, "\n"),
			want: "Evidence and files/commands involved",
		},
		{
			name: "unknown next action",
			input: strings.Join([]string{
				"y",
				"y", "sonnet", "unknown", "Verifier output was missing.", "verifier output missing", "",
			}, "\n"),
			want: "Next action",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CollectPromptResponses(plan, PromptIO{In: strings.NewReader(tt.input), Out: &bytes.Buffer{}})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestRunPreCommitNormalizesContradictoryResponsesToUnknownBlockedCell(t *testing.T) {
	root := fixtureProject(t)
	responses := passingResponses(t)
	responses.Conditions[0].SubagentConfirmed = false
	responses.Conditions[0].Verdict = attestations.VerdictPass
	responses.Conditions[0].Message = "No subagent checked lint-rules."
	responsesPath := responsesFile(t, root, responses)

	result, err := RunPreCommit(context.Background(), PreCommitOptions{
		Root:            root,
		ExplicitFeature: "br-123",
		ResponsesPath:   responsesPath,
		Now:             func() time.Time { return artifactTestTime },
		Staged:          fixtureStaged(),
	})
	if err == nil {
		t.Fatal("contradictory responses should block")
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.BlockedReportPath)))
	if err != nil {
		t.Fatalf("blocked report not written: %v", err)
	}
	var artifact attestations.Artifact
	if err := json.Unmarshal(body, &artifact); err != nil {
		t.Fatal(err)
	}
	first := artifact.Conditions[0]
	if first.SubagentConfirmed || first.Verdict != attestations.VerdictUnknown {
		t.Fatalf("contradictory pass should be normalized to unknown: %#v", first)
	}
}

func TestRunPreCommitRequiresResponseMessagesEvidenceAndNextAction(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Responses)
		want   string
	}{
		{
			name: "not applicable message",
			mutate: func(r *Responses) {
				r.Conditions[0].Verdict = attestations.VerdictNotApplicable
				r.Conditions[0].Message = ""
			},
			want: "not_applicable without a message",
		},
		{
			name: "fail evidence",
			mutate: func(r *Responses) {
				r.Conditions[0].Verdict = attestations.VerdictFail
				r.Conditions[0].Message = "lint-rules failed."
				r.Conditions[0].Evidence = nil
				r.Conditions[0].NextAction = "Fix lint rule failure."
			},
			want: "without evidence",
		},
		{
			name: "unknown next action",
			mutate: func(r *Responses) {
				r.Conditions[0].Verdict = attestations.VerdictUnknown
				r.Conditions[0].Message = "Verifier output was missing."
				r.Conditions[0].Evidence = []string{"missing verifier output"}
				r.Conditions[0].NextAction = ""
			},
			want: "without next action",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := fixtureProject(t)
			responses := passingResponses(t)
			tt.mutate(responses)
			result, err := RunPreCommit(context.Background(), PreCommitOptions{
				Root:            root,
				ExplicitFeature: "br-123",
				ResponsesPath:   responsesFile(t, root, responses),
				Now:             func() time.Time { return artifactTestTime },
				Staged:          fixtureStaged(),
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q result=%#v", err, tt.want, result)
			}
		})
	}
}

func TestLoadResponsesRequiresTopLevelConditionsArray(t *testing.T) {
	root := fixtureProject(t)
	path := filepath.Join(root, "responses.json")
	if err := os.WriteFile(path, []byte(`{"atomicity":{"one_feature_or_fix":true},"responses":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadResponses(path)
	if err == nil || !strings.Contains(err.Error(), "top-level conditions array") {
		t.Fatalf("LoadResponses error = %v", err)
	}

	if err := os.WriteFile(path, []byte(`{"atomicity":{"one_feature_or_fix":true},"conditions":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = LoadResponses(path)
	if err == nil || !strings.Contains(err.Error(), "conditions must be an array") {
		t.Fatalf("LoadResponses object conditions error = %v", err)
	}
}

func TestResponsesSchemaRoundTripsLaneBindingAndLegacyAtomicity(t *testing.T) {
	createdAt := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	responses := Responses{
		Atomicity: attestations.Atomicity{
			Mode:            attestations.AtomicityModeLane,
			OneFeatureOrFix: false,
			Message:         "Orchestrator-authorized lane commit naming every bead id.",
		},
		Binding: ResponseBinding{
			StagedPayloadHash: "sha256:payload",
			ManifestHash:      "sha256:manifest",
			Conditions: []ResponseConditionBinding{{
				ConditionID:       "dry",
				ConditionFile:     "backpressure/dry.md",
				ConditionFileHash: "sha256:dry",
			}},
			LaneBinding: &attestations.LaneBinding{
				LaneID:            "declared-lane-aj41-95t2",
				BeadIDs:           []string{"burpvalve-aj41", "burpvalve-95t2"},
				Rationale:         "one orchestrator ruling plus tracker export",
				AuthorizedBy:      "BronzeDeer",
				AuthorizationRef:  "Agent Mail 1234 / ORCHESTRATOR.md ruling",
				AuthorizationKind: "orchestrator",
				CreatedAt:         &createdAt,
			},
		},
		Conditions: []ResponseCondition{{
			ConditionID:    "dry",
			ConditionFile:  "backpressure/dry.md",
			VerifierPolicy: attestations.VerifierPolicyIndependentRequired,
			Verifier:       attestations.Verifier{Kind: attestations.VerifierUnknown},
			Verdict:        attestations.VerdictUnknown,
			Message:        "Replace with verifier result for dry.",
			Evidence:       []string{},
			NextAction:     "Replace with next action when verdict is fail or unknown.",
		}},
	}

	body, err := json.Marshal(responses)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`"mode":"lane"`)) || !bytes.Contains(body, []byte(`"lane_binding"`)) {
		t.Fatalf("lane response schema fields missing: %s", string(body))
	}
	var decoded Responses
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Atomicity.Mode != attestations.AtomicityModeLane {
		t.Fatalf("atomicity mode = %q", decoded.Atomicity.Mode)
	}
	if decoded.Binding.LaneBinding == nil || decoded.Binding.LaneBinding.LaneID != "declared-lane-aj41-95t2" {
		t.Fatalf("lane binding not preserved: %#v", decoded.Binding.LaneBinding)
	}
	if got := decoded.Binding.LaneBinding.BeadIDs; len(got) != 2 || got[0] != "burpvalve-aj41" || got[1] != "burpvalve-95t2" {
		t.Fatalf("lane bead ids = %#v", got)
	}
	if decoded.Binding.LaneBinding.CreatedAt == nil || !decoded.Binding.LaneBinding.CreatedAt.Equal(createdAt) {
		t.Fatalf("lane created_at = %#v", decoded.Binding.LaneBinding.CreatedAt)
	}

	var legacy Responses
	if err := json.Unmarshal([]byte(`{"atomicity":{"one_feature_or_fix":true,"message":"single bead"},"conditions":[]}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Atomicity.Mode != "" || !legacy.Atomicity.OneFeatureOrFix {
		t.Fatalf("legacy atomicity changed: %#v", legacy.Atomicity)
	}
	if legacy.Binding.LaneBinding != nil {
		t.Fatalf("legacy response should not synthesize lane binding: %#v", legacy.Binding.LaneBinding)
	}
}

func TestValidateResponsesChecksBoundBindingAndPassEvidence(t *testing.T) {
	root := fixtureProject(t)
	plan, err := BuildPlan(context.Background(), Options{
		Root:            root,
		Mode:            "pre-commit",
		ExplicitFeature: "br-123",
		Staged:          fixtureStaged(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("stale staged payload hash", func(t *testing.T) {
		responses := passingBoundResponses(t, plan)
		responses.Binding.StagedPayloadHash = "sha256:stale"
		err := validateResponses(plan, responses)
		if err == nil || !strings.Contains(err.Error(), "staged payload binding is stale") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("missing condition file hash", func(t *testing.T) {
		responses := passingBoundResponses(t, plan)
		responses.Binding.Conditions[0].ConditionFileHash = ""
		err := validateResponses(plan, responses)
		if err == nil || !strings.Contains(err.Error(), "missing condition_file_hash") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("pass evidence mandatory for bound responses", func(t *testing.T) {
		responses := passingBoundResponses(t, plan)
		responses.Conditions[0].Evidence = nil
		err := validateResponses(plan, responses)
		if err == nil || !strings.Contains(err.Error(), "pass verdict without evidence") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("blank pass evidence rejected for bound responses", func(t *testing.T) {
		responses := passingBoundResponses(t, plan)
		responses.Conditions[0].Evidence = []string{"", " \t\n"}
		err := validateResponses(plan, responses)
		if err == nil || !strings.Contains(err.Error(), "pass verdict without evidence") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestValidateCommitLaneAssertions(t *testing.T) {
	lane := attestations.LaneBinding{
		LaneID:            "declared-lane-aj41",
		BeadIDs:           []string{"burpvalve-aj41.3", "burpvalve-aj41.4"},
		Rationale:         "same orchestrator-authorized lane",
		AuthorizedBy:      "BronzeDeer",
		AuthorizationRef:  "Agent Mail 4000",
		AuthorizationKind: "orchestrator",
	}
	responses := &Responses{Binding: ResponseBinding{LaneBinding: &lane}}
	if err := validateCommitLaneAssertions(responses, PreCommitOptions{Lane: LaneOptions{
		Enabled:           true,
		LaneID:            "declared-lane-aj41",
		BeadIDs:           []string{"burpvalve-aj41.3", "burpvalve-aj41.4"},
		Rationale:         "same orchestrator-authorized lane",
		AuthorizedBy:      "BronzeDeer",
		AuthorizationRef:  "Agent Mail 4000",
		AuthorizationKind: "orchestrator",
	}}); err != nil {
		t.Fatalf("matching lane assertions blocked: %v", err)
	}
	tests := []struct {
		name string
		opts PreCommitOptions
		want string
	}{
		{
			name: "missing lane binding",
			opts: PreCommitOptions{Lane: LaneOptions{Enabled: true, LaneID: "declared-lane-aj41"}},
			want: "binding.lane_binding",
		},
		{
			name: "id mismatch",
			opts: PreCommitOptions{Lane: LaneOptions{Enabled: true, LaneID: "other"}},
			want: "lane id mismatch",
		},
		{
			name: "bead mismatch",
			opts: PreCommitOptions{Lane: LaneOptions{Enabled: true, LaneID: "declared-lane-aj41", BeadIDs: []string{"burpvalve-aj41.3", "other"}}},
			want: "bead ids mismatch",
		},
		{
			name: "authorization mismatch",
			opts: PreCommitOptions{Lane: LaneOptions{
				Enabled:           true,
				LaneID:            "declared-lane-aj41",
				BeadIDs:           []string{"burpvalve-aj41.3", "burpvalve-aj41.4"},
				Rationale:         "same orchestrator-authorized lane",
				AuthorizationRef:  "other",
				AuthorizedBy:      "BronzeDeer",
				AuthorizationKind: "orchestrator",
			}},
			want: "authorization ref mismatch",
		},
		{
			name: "authorization kind mismatch",
			opts: PreCommitOptions{Lane: LaneOptions{
				Enabled:           true,
				LaneID:            "declared-lane-aj41",
				BeadIDs:           []string{"burpvalve-aj41.3", "burpvalve-aj41.4"},
				Rationale:         "same orchestrator-authorized lane",
				AuthorizationRef:  "Agent Mail 4000",
				AuthorizedBy:      "BronzeDeer",
				AuthorizationKind: "human",
			}},
			want: "authorization kind mismatch",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotResponses := responses
			if tt.name == "missing lane binding" {
				gotResponses = &Responses{}
			}
			err := validateCommitLaneAssertions(gotResponses, tt.opts)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestRunPreCommitWritesLaneAttestation(t *testing.T) {
	root := fixtureProject(t)
	staged := fixtureStaged()
	plan, err := BuildPlan(context.Background(), Options{
		Root:            root,
		Mode:            "pre-commit",
		ExplicitFeature: "declared-lane-aj41",
		Staged:          staged,
	})
	if err != nil {
		t.Fatal(err)
	}
	lane := attestations.LaneBinding{
		LaneID:            "declared-lane-aj41",
		BeadIDs:           []string{"burpvalve-aj41.3", "burpvalve-aj41.4"},
		Rationale:         "same orchestrator-authorized lane",
		AuthorizedBy:      "BronzeDeer",
		AuthorizationRef:  "Agent Mail 4000",
		AuthorizationKind: "orchestrator",
	}
	responses := passingBoundResponses(t, plan)
	responses.Atomicity = attestations.Atomicity{
		Mode:            attestations.AtomicityModeLane,
		OneFeatureOrFix: false,
		Message:         "Orchestrator-authorized lane declared-lane-aj41.",
		Lane:            &lane,
	}
	responses.Binding.LaneBinding = &lane
	responsesPath := writeBoundResponses(t, root, plan, responses)

	result, err := RunPreCommit(context.Background(), PreCommitOptions{
		Root:            root,
		ExplicitFeature: "declared-lane-aj41",
		ResponsesPath:   responsesPath,
		Lane: LaneOptions{
			Enabled:           true,
			LaneID:            "declared-lane-aj41",
			BeadIDs:           []string{"burpvalve-aj41.3", "burpvalve-aj41.4"},
			Rationale:         "same orchestrator-authorized lane",
			AuthorizedBy:      "BronzeDeer",
			AuthorizationRef:  "Agent Mail 4000",
			AuthorizationKind: "orchestrator",
		},
		Now:    func() time.Time { return artifactTestTime },
		Staged: staged,
	})
	if err == nil {
		t.Fatal("unstaged passing artifact should block once")
	}
	if result.Status != StatusAttestationWritten {
		t.Fatalf("status = %q, want %q", result.Status, StatusAttestationWritten)
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.ArtifactPath)))
	if err != nil {
		t.Fatalf("lane artifact not written: %v", err)
	}
	var artifact attestations.Artifact
	if err := json.Unmarshal(body, &artifact); err != nil {
		t.Fatal(err)
	}
	if err := artifact.ValidatePassing(ExpectedBinding(result.Plan)); err != nil {
		t.Fatalf("lane artifact should validate: %v", err)
	}
	if artifact.Feature.Kind != "lane" || artifact.Feature.ID != "declared-lane-aj41" || artifact.Feature.DiffCluster != "lane:declared-lane-aj41" {
		t.Fatalf("lane feature metadata wrong: %#v", artifact.Feature)
	}
	if got := strings.Join(artifact.BeadIDs, ","); got != "burpvalve-aj41.3,burpvalve-aj41.4" {
		t.Fatalf("artifact bead ids = %q", got)
	}
	if artifact.CoupledWorkRationale != "same orchestrator-authorized lane" {
		t.Fatalf("coupled rationale = %q", artifact.CoupledWorkRationale)
	}
}

func TestRunPreCommitRejectsLaneBoundResponsesWithoutLaneAssertions(t *testing.T) {
	root := fixtureProject(t)
	staged := fixtureStaged()
	plan, err := BuildPlan(context.Background(), Options{
		Root:            root,
		Mode:            "pre-commit",
		ExplicitFeature: "declared-lane-aj41",
		Staged:          staged,
	})
	if err != nil {
		t.Fatal(err)
	}
	lane := attestations.LaneBinding{
		LaneID:            "declared-lane-aj41",
		BeadIDs:           []string{"burpvalve-aj41.3", "burpvalve-aj41.4"},
		Rationale:         "same orchestrator-authorized lane",
		AuthorizedBy:      "BronzeDeer",
		AuthorizationRef:  "Agent Mail 4000",
		AuthorizationKind: "orchestrator",
	}
	responses := passingBoundResponses(t, plan)
	responses.Atomicity = attestations.Atomicity{
		Mode:            attestations.AtomicityModeLane,
		OneFeatureOrFix: false,
		Message:         "Orchestrator-authorized lane declared-lane-aj41.",
		Lane:            &lane,
	}
	responses.Binding.LaneBinding = &lane
	responsesPath := writeBoundResponses(t, root, plan, responses)

	result, err := RunPreCommit(context.Background(), PreCommitOptions{
		Root:            root,
		ExplicitFeature: "declared-lane-aj41",
		ResponsesPath:   responsesPath,
		Now:             func() time.Time { return artifactTestTime },
		Staged:          staged,
	})
	if err == nil || !strings.Contains(err.Error(), "lane-bound responses require --lane") {
		t.Fatalf("missing lane assertions should block, err=%v result=%#v", err, result)
	}
	if result.Status != StatusBlocked || result.BlockedReportPath == "" {
		t.Fatalf("missing lane assertions should write blocked report: %#v", result)
	}
}

func TestValidateResponsesRequiresLaneBindingMatchesAtomicity(t *testing.T) {
	root := fixtureProject(t)
	plan, err := BuildPlan(context.Background(), Options{
		Root:            root,
		Mode:            "pre-commit",
		ExplicitFeature: "declared-lane-aj41",
		Staged:          fixtureStaged(),
	})
	if err != nil {
		t.Fatal(err)
	}
	lane := attestations.LaneBinding{
		LaneID:            "declared-lane-aj41",
		BeadIDs:           []string{"burpvalve-aj41.3", "burpvalve-aj41.4"},
		Rationale:         "same orchestrator-authorized lane",
		AuthorizedBy:      "BronzeDeer",
		AuthorizationRef:  "Agent Mail 4000",
		AuthorizationKind: "orchestrator",
	}
	responses := passingBoundResponses(t, plan)
	responses.Atomicity = attestations.Atomicity{
		Mode:    attestations.AtomicityModeLane,
		Message: "Orchestrator-authorized lane declared-lane-aj41.",
		Lane:    &lane,
	}
	responses.Binding.LaneBinding = &lane
	if err := validateResponses(plan, responses); err != nil {
		t.Fatalf("matching lane responses should validate: %v", err)
	}
	contradictory := *responses
	contradictory.Atomicity.OneFeatureOrFix = true
	if err := validateResponses(plan, &contradictory); err == nil || !strings.Contains(err.Error(), "one_feature_or_fix") {
		t.Fatalf("lane one_feature_or_fix should block, err=%v", err)
	}
	missingKind := *responses
	missingKindLane := *responses.Atomicity.Lane
	missingKindLane.AuthorizationKind = ""
	missingKind.Atomicity.Lane = &missingKindLane
	missingKind.Binding.LaneBinding = &missingKindLane
	if err := validateResponses(plan, &missingKind); err == nil || !strings.Contains(err.Error(), "authorization metadata") {
		t.Fatalf("missing lane authorization kind should block, err=%v", err)
	}
	wrongKind := *responses
	wrongKindLane := *responses.Atomicity.Lane
	wrongKindLane.AuthorizationKind = "human"
	wrongKind.Atomicity.Lane = &wrongKindLane
	wrongKind.Binding.LaneBinding = &wrongKindLane
	if err := validateResponses(plan, &wrongKind); err == nil || !strings.Contains(err.Error(), `not "orchestrator"`) {
		t.Fatalf("non-orchestrator lane authorization kind should block, err=%v", err)
	}
	mismatched := *responses
	bound := *responses.Binding.LaneBinding
	bound.AuthorizationRef = "Agent Mail 4001"
	mismatched.Binding.LaneBinding = &bound
	if err := validateResponses(plan, &mismatched); err == nil || !strings.Contains(err.Error(), "authorization metadata") {
		t.Fatalf("mismatched lane binding should block, err=%v", err)
	}
}

func TestBuildResponsesTemplateIncludesEveryCondition(t *testing.T) {
	root := fixtureProject(t)
	plan, err := BuildPlan(context.Background(), Options{
		Root:            root,
		Mode:            "pre-commit",
		ExplicitFeature: "br-123",
		Staged:          fixtureStaged(),
	})
	if err != nil {
		t.Fatal(err)
	}
	template := BuildResponsesTemplate(plan)
	templateBody, err := json.Marshal(template)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(templateBody, []byte("0001-01-01")) {
		t.Fatalf("template should not emit zero verifier timestamps: %s", string(templateBody))
	}
	if template.Atomicity.OneFeatureOrFix {
		t.Fatal("template should not accidentally confirm atomicity")
	}
	if template.Binding.StagedPayloadHash != plan.StagedPayloadHash {
		t.Fatalf("binding staged payload hash = %q, want %q", template.Binding.StagedPayloadHash, plan.StagedPayloadHash)
	}
	if template.Binding.ManifestHash != plan.ManifestHash {
		t.Fatalf("binding manifest hash = %q, want %q", template.Binding.ManifestHash, plan.ManifestHash)
	}
	if got, want := len(template.Binding.Conditions), len(plan.Matrix.Conditions); got != want {
		t.Fatalf("binding conditions = %d, want %d", got, want)
	}
	if got, want := len(template.Conditions), len(plan.Matrix.Conditions); got != want {
		t.Fatalf("template conditions = %d, want %d", got, want)
	}
	for i, condition := range plan.Matrix.Conditions {
		if template.Binding.Conditions[i].ConditionID != condition.ID ||
			template.Binding.Conditions[i].ConditionFile != condition.Path ||
			template.Binding.Conditions[i].ConditionFileHash != plan.ConditionFileHashes[condition.ID] {
			t.Fatalf("condition %d binding = %#v, want %s/%s/%s", i, template.Binding.Conditions[i], condition.ID, condition.Path, plan.ConditionFileHashes[condition.ID])
		}
		if template.Conditions[i].ConditionID != condition.ID || template.Conditions[i].ConditionFile != condition.Path {
			t.Fatalf("condition %d template = %#v, want %s/%s", i, template.Conditions[i], condition.ID, condition.Path)
		}
		if template.Conditions[i].SubagentConfirmed {
			t.Fatalf("template should not confirm subagent by default: %#v", template.Conditions[i])
		}
		if template.Conditions[i].VerifierPolicy != attestations.VerifierPolicyIndependentRequired {
			t.Fatalf("template verifier policy = %q, want independent_required", template.Conditions[i].VerifierPolicy)
		}
		verifier := template.Conditions[i].Verifier
		if verifier.Kind != attestations.VerifierUnknown ||
			verifier.Agent == "" ||
			verifier.Model == "" ||
			verifier.Runtime == "" ||
			verifier.TranscriptRef == "" ||
			verifier.EvidenceRef == "" ||
			verifier.CreatedAt == nil {
			t.Fatalf("template should include verifier provenance placeholders: %#v", template.Conditions[i])
		}
	}
}

func TestRunVerifierBeginWritesBoundResponses(t *testing.T) {
	root := fixtureProject(t)
	result, err := RunVerifierBegin(context.Background(), BeginResponsesOptions{
		Root:             root,
		ExplicitFeature:  "br-123",
		OneFeature:       true,
		AtomicityMessage: "Staged changes map only to br-123.",
		Staged:           fixtureStaged(),
	})
	if err != nil {
		t.Fatalf("RunVerifierBegin failed: %v", err)
	}
	if result.Status != StatusResponsesWritten {
		t.Fatalf("status = %q, want %q", result.Status, StatusResponsesWritten)
	}
	if result.ResponsesPath != ResponsesPath(result.StagedPayloadHash) {
		t.Fatalf("responses path = %q, want %q", result.ResponsesPath, ResponsesPath(result.StagedPayloadHash))
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.ResponsesPath)))
	if err != nil {
		t.Fatalf("responses file not written: %v", err)
	}
	assertFormatterSafeArtifactJSON(t, body)
	var responses Responses
	if err := json.Unmarshal(body, &responses); err != nil {
		t.Fatal(err)
	}
	if !responses.Atomicity.OneFeatureOrFix || responses.Atomicity.Message != "Staged changes map only to br-123." {
		t.Fatalf("atomicity not preserved: %#v", responses.Atomicity)
	}
	if responses.Binding.StagedPayloadHash != result.Plan.StagedPayloadHash || responses.Binding.ManifestHash != result.Plan.ManifestHash {
		t.Fatalf("binding = %#v, plan hashes = %s/%s", responses.Binding, result.Plan.StagedPayloadHash, result.Plan.ManifestHash)
	}
	for i, condition := range result.Plan.Matrix.Conditions {
		if responses.Conditions[i].ConditionID != condition.ID || responses.Conditions[i].Verdict != attestations.VerdictUnknown {
			t.Fatalf("condition %d response = %#v, want unknown for %s", i, responses.Conditions[i], condition.ID)
		}
		if responses.Binding.Conditions[i].ConditionFileHash != result.Plan.ConditionFileHashes[condition.ID] {
			t.Fatalf("condition %d binding hash = %q, want %q", i, responses.Binding.Conditions[i].ConditionFileHash, result.Plan.ConditionFileHashes[condition.ID])
		}
	}
}

func TestRunVerifierBeginWritesLaneBoundResponses(t *testing.T) {
	root := fixtureProject(t)
	result, err := RunVerifierBegin(context.Background(), BeginResponsesOptions{
		Root:            root,
		ExplicitFeature: "declared-lane-aj41",
		Lane: LaneOptions{
			Enabled:          true,
			LaneID:           "declared-lane-aj41",
			BeadIDs:          []string{"burpvalve-aj41.3", "burpvalve-aj41.4"},
			Rationale:        "same orchestrator-authorized lane",
			AuthorizationRef: "Agent Mail 4000",
			AuthorizedBy:     "BronzeDeer",
		},
		Staged: fixtureStaged(),
	})
	if err != nil {
		t.Fatalf("RunVerifierBegin lane failed: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.ResponsesPath)))
	if err != nil {
		t.Fatalf("responses file not written: %v", err)
	}
	var responses Responses
	if err := json.Unmarshal(body, &responses); err != nil {
		t.Fatalf("decode lane responses: %v\n%s", err, body)
	}
	if responses.Atomicity.Mode != attestations.AtomicityModeLane || responses.Atomicity.OneFeatureOrFix {
		t.Fatalf("lane atomicity wrong: %#v", responses.Atomicity)
	}
	if responses.Binding.LaneBinding == nil || responses.Binding.LaneBinding.LaneID != "declared-lane-aj41" {
		t.Fatalf("lane binding missing: %#v", responses.Binding.LaneBinding)
	}
	if got := responses.Binding.LaneBinding.BeadIDs; len(got) != 2 || got[0] != "burpvalve-aj41.3" || got[1] != "burpvalve-aj41.4" {
		t.Fatalf("lane bead ids = %#v", got)
	}
}

func TestRunVerifierBeginRejectsLaneTrackerExportOutsideDeclaredBeads(t *testing.T) {
	root := fixtureProject(t)
	gitOutputCore(t, root, "init", "-q")
	gitOutputCore(t, root, "config", "user.email", "tests@example.invalid")
	gitOutputCore(t, root, "config", "user.name", "Burpvalve Tests")

	baseline := `{"id":"burpvalve-aj41.7","status":"open"}
{"id":"burpvalve-aj41.8","status":"open"}
{"id":"burpvalve-aj41.9","status":"open"}
`
	writeFile(t, root, ".beads/issues.jsonl", baseline)
	gitOutputCore(t, root, "add", ".")
	gitOutputCore(t, root, "commit", "-qm", "baseline")

	withOutside := `{"id":"burpvalve-aj41.7","status":"closed"}
{"id":"burpvalve-aj41.8","status":"closed"}
{"id":"burpvalve-aj41.9","status":"closed"}
`
	writeFile(t, root, ".beads/issues.jsonl", withOutside)
	gitOutputCore(t, root, "add", ".beads/issues.jsonl")

	lane := LaneOptions{
		Enabled:          true,
		LaneID:           "declared-lane-aj41-docs",
		BeadIDs:          []string{"burpvalve-aj41.7", "burpvalve-aj41.8"},
		Rationale:        "same orchestrator-authorized lane",
		AuthorizationRef: "Agent Mail 4000",
		AuthorizedBy:     "BronzeDeer",
	}
	result, err := RunVerifierBegin(context.Background(), BeginResponsesOptions{
		Root: root,
		Lane: lane,
	})
	if err == nil {
		t.Fatal("lane tracker export with extra bead id should block")
	}
	if !strings.Contains(result.Message, "outside declared lane") || !strings.Contains(result.Message, "burpvalve-aj41.9") {
		t.Fatalf("message = %q, want outside lane bead id", result.Message)
	}

	withinLane := `{"id":"burpvalve-aj41.7","status":"closed"}
{"id":"burpvalve-aj41.8","status":"closed"}
{"id":"burpvalve-aj41.9","status":"open"}
`
	writeFile(t, root, ".beads/issues.jsonl", withinLane)
	gitOutputCore(t, root, "add", ".beads/issues.jsonl")
	result, err = RunVerifierBegin(context.Background(), BeginResponsesOptions{
		Root: root,
		Lane: lane,
	})
	if err != nil {
		t.Fatalf("lane tracker export limited to declared beads should pass: %v", err)
	}
	if result.Status != StatusResponsesWritten {
		t.Fatalf("status = %q, want %q", result.Status, StatusResponsesWritten)
	}
}

func TestRunVerifierBeginRejectsInvalidLaneFlags(t *testing.T) {
	root := fixtureProject(t)
	tests := []struct {
		name string
		opts BeginResponsesOptions
		want string
	}{
		{
			name: "one feature conflict",
			opts: BeginResponsesOptions{
				Root:       root,
				OneFeature: true,
				Lane:       LaneOptions{Enabled: true, LaneID: "lane", BeadIDs: []string{"a", "b"}, Rationale: "r", AuthorizationRef: "ref", AuthorizedBy: "BronzeDeer"},
			},
			want: "mutually exclusive",
		},
		{
			name: "feature mismatch",
			opts: BeginResponsesOptions{
				Root:            root,
				ExplicitFeature: "other",
				Lane:            LaneOptions{Enabled: true, LaneID: "lane", BeadIDs: []string{"a", "b"}, Rationale: "r", AuthorizationRef: "ref", AuthorizedBy: "BronzeDeer"},
			},
			want: "must match --lane-id",
		},
		{
			name: "missing authorization",
			opts: BeginResponsesOptions{
				Root: root,
				Lane: LaneOptions{Enabled: true, LaneID: "lane", BeadIDs: []string{"a", "b"}, Rationale: "r"},
			},
			want: "--lane-authorization-ref",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.opts.Staged = fixtureStaged()
			result, err := RunVerifierBegin(context.Background(), tt.opts)
			if err == nil {
				t.Fatal("invalid lane options should block")
			}
			if !strings.Contains(result.Message, tt.want) {
				t.Fatalf("message = %q, want %q", result.Message, tt.want)
			}
		})
	}
}

func TestRunVerifierBeginRequiresExplicitAtomicity(t *testing.T) {
	root := fixtureProject(t)
	result, err := RunVerifierBegin(context.Background(), BeginResponsesOptions{
		Root:            root,
		ExplicitFeature: "br-123",
		Staged:          fixtureStaged(),
	})
	if err == nil {
		t.Fatal("missing atomicity should block")
	}
	if result.Status != StatusBlocked || len(result.NextSteps) == 0 {
		t.Fatalf("result = %#v", result)
	}
	if result.ResponsesPath == "" {
		t.Fatal("blocked result should still expose the intended response path for the staged payload")
	}
	if _, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(result.ResponsesPath))); !os.IsNotExist(statErr) {
		t.Fatalf("responses file should not be written without atomicity, stat err=%v", statErr)
	}
}

func TestRunVerifierBeginRefusesEmptyStagedPayload(t *testing.T) {
	root := fixtureProject(t)
	result, err := RunVerifierBegin(context.Background(), BeginResponsesOptions{
		Root:             root,
		OneFeature:       true,
		AtomicityMessage: "No payload.",
		Staged:           fakeStaged{},
	})
	if err == nil {
		t.Fatal("empty staged payload should block")
	}
	if result.Status != StatusBlocked || len(result.NextSteps) == 0 {
		t.Fatalf("result = %#v", result)
	}
	if result.ResponsesPath != "" {
		t.Fatalf("empty payload should not have response path, got %q", result.ResponsesPath)
	}
}

func TestRunPreCommitDoesNotAcceptStaleOrMalformedStagedAttestations(t *testing.T) {
	root := fixtureProject(t)
	staged := fixtureStaged()
	plan, err := BuildPlan(context.Background(), Options{
		Root:            root,
		Mode:            "pre-commit",
		ExplicitFeature: "br-123",
		Staged:          staged,
	})
	if err != nil {
		t.Fatal(err)
	}
	artifactPath := AttestationPath(plan.StagedPayloadHash)
	stale := BuildArtifact(plan, passingResponses(t), PreCommitOptions{
		Root: root,
		Now:  func() time.Time { return artifactTestTime },
	}, attestations.ArtifactPassing)
	stale.StagedPayloadHash = "sha256:old"
	staleBody, err := json.Marshal(stale)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed", body: "{"},
		{name: "stale", body: string(staleBody)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			staged := fixtureStaged()
			staged.paths = append(staged.paths, artifactPath)
			staged.content[artifactPath] = tt.body
			result, err := RunPreCommit(context.Background(), PreCommitOptions{
				Root:            root,
				ExplicitFeature: "br-123",
				Now:             func() time.Time { return artifactTestTime },
				Staged:          staged,
			})
			if err == nil || !strings.Contains(err.Error(), "stale or invalid") {
				t.Fatalf("%s staged attestation error = %v, result=%#v", tt.name, err, result)
			}
			if result.Status == StatusPassed {
				t.Fatalf("%s staged attestation passed unexpectedly", tt.name)
			}
			if result.BlockedReportPath == "" || !strings.Contains(result.Message, "stale or invalid") {
				t.Fatalf("%s staged attestation should write stale blocked report, result=%#v", tt.name, result)
			}
		})
	}
}

type summaryBeforeArtifactWriter struct {
	t                       *testing.T
	root                    string
	artifactPath            string
	artifactAbsentAtSummary bool
	bytes.Buffer
}

func (w *summaryBeforeArtifactWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "Summary") && !w.artifactAbsentAtSummary {
		_, err := os.Stat(filepath.Join(w.root, filepath.FromSlash(w.artifactPath)))
		if os.IsNotExist(err) {
			w.artifactAbsentAtSummary = true
		} else if err != nil {
			w.t.Fatalf("stat artifact at summary: %v", err)
		} else {
			w.t.Fatalf("artifact existed before summary was printed")
		}
	}
	return w.Buffer.Write(p)
}

func fixtureStaged() fakeStaged {
	return fakeStaged{
		paths: []string{
			"backpressure/attestations/old.json",
			"cmd/app/main.go",
			"log/backpressure/failed/old-blocked.json",
		},
		content: map[string]string{
			"cmd/app/main.go": "package main\n",
		},
	}
}

func fixturePayloadOnlyStaged() fakeStaged {
	return fakeStaged{
		paths: []string{"cmd/app/main.go"},
		content: map[string]string{
			"cmd/app/main.go": "package main\n",
		},
	}
}

func passingResponsesFile(t *testing.T, root string) string {
	t.Helper()
	return responsesFile(t, root, passingResponses(t))
}

func passingResponses(t *testing.T) *Responses {
	t.Helper()
	return &Responses{
		Atomicity: attestations.Atomicity{
			OneFeatureOrFix: true,
			Message:         "Staged changes map only to br-123.",
		},
		Conditions: []ResponseCondition{
			{
				ConditionID:       "lint-rules",
				SubagentConfirmed: true,
				SubagentModel:     "sonnet",
				Verdict:           attestations.VerdictPass,
				Evidence:          []string{"lint verifier passed"},
			},
			{
				ConditionID:       "dry",
				SubagentConfirmed: true,
				SubagentModel:     "sonnet",
				Verdict:           attestations.VerdictPass,
				Evidence:          []string{"dry verifier passed"},
			},
			{
				ConditionID:       "anti-reward-hacking",
				SubagentConfirmed: true,
				SubagentModel:     "sonnet",
				Verdict:           attestations.VerdictNotApplicable,
				Message:           "No success metric or reward path changed.",
				Evidence:          []string{"anti-reward verifier passed"},
			},
		},
	}
}

func passingBoundResponses(t *testing.T, plan Plan) *Responses {
	t.Helper()
	responses := BuildBoundResponsesTemplate(plan, attestations.Atomicity{
		OneFeatureOrFix: true,
		Message:         "Staged changes map only to br-123.",
	})
	now := time.Date(2026, 6, 20, 4, 0, 0, 0, time.UTC)
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
	return &responses
}

func writeBoundResponses(t *testing.T, root string, plan Plan, responses *Responses) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(ResponsesPath(plan.StagedPayloadHash)))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(responses)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func responsesFile(t *testing.T, root string, responses *Responses) string {
	t.Helper()
	body, err := json.Marshal(responses)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "responses.json")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
