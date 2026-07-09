package backpressure

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"burpvalve/internal/attestations"
)

func TestRunVerifierSubmitUpdatesOneCondition(t *testing.T) {
	root, plan, responsesPath := beginSubmitFixture(t)
	input := submitInputFor(t, plan, "dry")

	result, err := RunVerifierSubmit(context.Background(), SubmitVerifierOptions{
		Root:              root,
		ExplicitFeature:   "br-123",
		ConditionID:       "dry",
		ResponsesPath:     responsesPath,
		StagedPayloadHash: plan.StagedPayloadHash,
		ManifestHash:      plan.ManifestHash,
		ConditionFileHash: plan.ConditionFileHashes["dry"],
		Response:          input,
		Staged:            fixtureStaged(),
	})
	if err != nil {
		t.Fatalf("RunVerifierSubmit failed: %v", err)
	}
	if result.Status != StatusResponsesUpdated || result.Fatal {
		t.Fatalf("result = %#v", result)
	}
	responses := readResponsesFile(t, responsesPath)
	if !responses.Atomicity.OneFeatureOrFix || responses.Atomicity.Message != "Staged changes map only to br-123." {
		t.Fatalf("atomicity changed: %#v", responses.Atomicity)
	}
	if responses.Conditions[0].ConditionID != "lint-rules" || responses.Conditions[0].Verdict != attestations.VerdictUnknown {
		t.Fatalf("unrelated condition changed: %#v", responses.Conditions[0])
	}
	dry := responseByCondition(t, responses, "dry")
	if dry.Verdict != attestations.VerdictPass || dry.Verifier.Agent != "Verifier-dry" || len(dry.Evidence) == 0 {
		t.Fatalf("dry response not merged: %#v", dry)
	}
	if responseByCondition(t, responses, "anti-reward-hacking").Verdict != attestations.VerdictUnknown {
		t.Fatalf("later condition changed: %#v", responses.Conditions)
	}
}

func TestRunVerifierSubmitRejectsStaleBinding(t *testing.T) {
	root, plan, responsesPath := beginSubmitFixture(t)
	input := submitInputFor(t, plan, "dry")
	input.StagedPayloadHash = "sha256:old"

	result, err := RunVerifierSubmit(context.Background(), SubmitVerifierOptions{
		Root:              root,
		ExplicitFeature:   "br-123",
		ConditionID:       "dry",
		ResponsesPath:     responsesPath,
		StagedPayloadHash: plan.StagedPayloadHash,
		ManifestHash:      plan.ManifestHash,
		ConditionFileHash: plan.ConditionFileHashes["dry"],
		Response:          input,
		Staged:            fixtureStaged(),
	})
	if err == nil {
		t.Fatal("stale binding should block")
	}
	if result.Status != StatusBlocked || !strings.Contains(result.Message, "staged payload binding is stale") {
		t.Fatalf("result = %#v err=%v", result, err)
	}
}

func TestRunVerifierSubmitRequiresMatchingLaneBinding(t *testing.T) {
	root, plan, responsesPath := beginLaneSubmitFixture(t)
	bound := readResponsesFile(t, responsesPath).Binding.LaneBinding
	if bound == nil {
		t.Fatal("lane submit fixture missing lane binding")
	}

	input := submitInputFor(t, plan, "dry")
	input.LaneBinding = bound
	result, err := RunVerifierSubmit(context.Background(), SubmitVerifierOptions{
		Root:              root,
		ExplicitFeature:   "declared-lane-aj41",
		ConditionID:       "dry",
		ResponsesPath:     responsesPath,
		StagedPayloadHash: plan.StagedPayloadHash,
		ManifestHash:      plan.ManifestHash,
		ConditionFileHash: plan.ConditionFileHashes["dry"],
		Response:          input,
		Staged:            fixtureStaged(),
	})
	if err != nil {
		t.Fatalf("lane-bound submit failed: %v", err)
	}
	if result.Status != StatusResponsesUpdated || result.Fatal {
		t.Fatalf("result = %#v", result)
	}
	responses := readResponsesFile(t, responsesPath)
	if responses.Binding.LaneBinding == nil || responses.Binding.LaneBinding.LaneID != "declared-lane-aj41" {
		t.Fatalf("lane binding not preserved: %#v", responses.Binding.LaneBinding)
	}
	if got := responseByCondition(t, responses, "dry").Verdict; got != attestations.VerdictPass {
		t.Fatalf("dry verdict = %q", got)
	}

	tests := []struct {
		name   string
		mutate func(*SubmitVerifierInput)
		want   string
	}{
		{
			name: "missing lane binding",
			mutate: func(input *SubmitVerifierInput) {
				input.LaneBinding = nil
			},
			want: "requires matching submit lane_binding",
		},
		{
			name: "lane id mismatch",
			mutate: func(input *SubmitVerifierInput) {
				copyBinding := *bound
				copyBinding.LaneID = "other-lane"
				input.LaneBinding = &copyBinding
			},
			want: "lane id mismatch",
		},
		{
			name: "bead ids mismatch",
			mutate: func(input *SubmitVerifierInput) {
				copyBinding := *bound
				copyBinding.BeadIDs = []string{"burpvalve-aj41.3"}
				input.LaneBinding = &copyBinding
			},
			want: "lane bead ids mismatch",
		},
		{
			name: "authorization ref mismatch",
			mutate: func(input *SubmitVerifierInput) {
				copyBinding := *bound
				copyBinding.AuthorizationRef = "Agent Mail 4001"
				input.LaneBinding = &copyBinding
			},
			want: "authorization_ref mismatch",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := submitInputFor(t, plan, "dry")
			input.LaneBinding = bound
			tt.mutate(input)
			result, err := RunVerifierSubmit(context.Background(), SubmitVerifierOptions{
				Root:              root,
				ExplicitFeature:   "declared-lane-aj41",
				ConditionID:       "dry",
				ResponsesPath:     responsesPath,
				StagedPayloadHash: plan.StagedPayloadHash,
				ManifestHash:      plan.ManifestHash,
				ConditionFileHash: plan.ConditionFileHashes["dry"],
				Response:          input,
				Staged:            fixtureStaged(),
			})
			if err == nil {
				t.Fatal("lane mismatch should block")
			}
			if !strings.Contains(result.Message, tt.want) {
				t.Fatalf("message = %q, want contains %q", result.Message, tt.want)
			}
		})
	}
}

func TestRunVerifierSubmitRejectsUnexpectedLaneBinding(t *testing.T) {
	root, plan, responsesPath := beginSubmitFixture(t)
	input := submitInputFor(t, plan, "dry")
	input.LaneBinding = &attestations.LaneBinding{LaneID: "unexpected-lane"}

	result, err := RunVerifierSubmit(context.Background(), SubmitVerifierOptions{
		Root:              root,
		ExplicitFeature:   "br-123",
		ConditionID:       "dry",
		ResponsesPath:     responsesPath,
		StagedPayloadHash: plan.StagedPayloadHash,
		ManifestHash:      plan.ManifestHash,
		ConditionFileHash: plan.ConditionFileHashes["dry"],
		Response:          input,
		Staged:            fixtureStaged(),
	})
	if err == nil {
		t.Fatal("unexpected lane binding should block")
	}
	if !strings.Contains(result.Message, "not expected") {
		t.Fatalf("message = %q", result.Message)
	}
}

func TestRunVerifierSubmitRejectsMissingEvidenceAndAuthorizationOnlyEvidence(t *testing.T) {
	root, plan, responsesPath := beginSubmitFixture(t)
	tests := []struct {
		name   string
		mutate func(*SubmitVerifierInput)
		want   string
	}{
		{
			name: "pass evidence required",
			mutate: func(input *SubmitVerifierInput) {
				input.Evidence = nil
			},
			want: "without evidence",
		},
		{
			name: "non pass message required",
			mutate: func(input *SubmitVerifierInput) {
				input.Verdict = attestations.VerdictFail
				input.Message = ""
				input.Evidence = []string{"real failing evidence"}
				input.NextAction = "Fix it."
			},
			want: "without message",
		},
		{
			name: "authorization evidence rejected",
			mutate: func(input *SubmitVerifierInput) {
				input.Evidence = []string{VerifierAuthorizationText}
			},
			want: "authorization metadata",
		},
		{
			name: "provenance required",
			mutate: func(input *SubmitVerifierInput) {
				input.Verifier.Runtime = ""
			},
			want: "requires verifier runtime",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := submitInputFor(t, plan, "dry")
			tt.mutate(input)
			result, err := RunVerifierSubmit(context.Background(), SubmitVerifierOptions{
				Root:              root,
				ExplicitFeature:   "br-123",
				ConditionID:       "dry",
				ResponsesPath:     responsesPath,
				StagedPayloadHash: plan.StagedPayloadHash,
				ManifestHash:      plan.ManifestHash,
				ConditionFileHash: plan.ConditionFileHashes["dry"],
				Response:          input,
				Staged:            fixtureStaged(),
			})
			if err == nil {
				t.Fatal("submit should block")
			}
			if !strings.Contains(result.Message, tt.want) {
				t.Fatalf("message = %q, want contains %q", result.Message, tt.want)
			}
		})
	}
}

func TestRunVerifierSubmitMergesSupplementalAndAdjudication(t *testing.T) {
	root, plan, responsesPath := beginSubmitFixture(t)
	primary := submitInputFor(t, plan, "dry")
	if _, err := RunVerifierSubmit(context.Background(), SubmitVerifierOptions{
		Root:              root,
		ExplicitFeature:   "br-123",
		ConditionID:       "dry",
		ResponsesPath:     responsesPath,
		StagedPayloadHash: plan.StagedPayloadHash,
		ManifestHash:      plan.ManifestHash,
		ConditionFileHash: plan.ConditionFileHashes["dry"],
		Response:          primary,
		Staged:            fixtureStaged(),
	}); err != nil {
		t.Fatalf("primary submit failed: %v", err)
	}
	supplemental := &SubmitVerifierInput{
		StagedPayloadHash: plan.StagedPayloadHash,
		ManifestHash:      plan.ManifestHash,
		ConditionFileHash: plan.ConditionFileHashes["dry"],
		ResponseCondition: ResponseCondition{
			ConditionID:   "dry",
			ConditionFile: "backpressure/dry.md",
			Supplemental: []SupplementalVerifier{{
				StagedPayloadHash: plan.StagedPayloadHash,
				ManifestHash:      plan.ManifestHash,
				ConditionFileHash: plan.ConditionFileHashes["dry"],
				Verifier:          submitVerifier("SecondDry"),
				Verdict:           attestations.VerdictPass,
				Message:           "Supplemental dry verifier agrees.",
				Evidence:          []string{"second dry verifier inspected staged artifacts.go"},
			}},
			Adjudication: &ResponseAdjudication{
				Authority:    "RusticDog",
				Summary:      "Supplemental agreement accepted.",
				FinalVerdict: attestations.VerdictPass,
				AuditRef:     "Agent Mail 2700",
			},
		},
	}
	if _, err := RunVerifierSubmit(context.Background(), SubmitVerifierOptions{
		Root:              root,
		ExplicitFeature:   "br-123",
		ConditionID:       "dry",
		ResponsesPath:     responsesPath,
		StagedPayloadHash: plan.StagedPayloadHash,
		ManifestHash:      plan.ManifestHash,
		ConditionFileHash: plan.ConditionFileHashes["dry"],
		Response:          supplemental,
		Staged:            fixtureStaged(),
	}); err != nil {
		t.Fatalf("supplemental submit failed: %v", err)
	}
	dry := responseByCondition(t, readResponsesFile(t, responsesPath), "dry")
	if dry.Verifier.Agent != "Verifier-dry" || dry.Verdict != attestations.VerdictPass {
		t.Fatalf("primary was not preserved: %#v", dry)
	}
	if got := len(dry.Supplemental); got != 1 || dry.Supplemental[0].Verifier.Agent != "SecondDry" {
		t.Fatalf("supplemental not merged: %#v", dry.Supplemental)
	}
	if dry.Adjudication == nil || dry.Adjudication.AuditRef != "Agent Mail 2700" {
		t.Fatalf("adjudication not preserved: %#v", dry.Adjudication)
	}
	if responseByCondition(t, readResponsesFile(t, responsesPath), "lint-rules").Verdict != attestations.VerdictUnknown {
		t.Fatal("supplemental submit changed another condition")
	}
	replacement := *supplemental
	replacement.Supplemental[0].Evidence = []string{"replacement supplemental evidence"}
	result, err := RunVerifierSubmit(context.Background(), SubmitVerifierOptions{
		Root:              root,
		ExplicitFeature:   "br-123",
		ConditionID:       "dry",
		ResponsesPath:     responsesPath,
		StagedPayloadHash: plan.StagedPayloadHash,
		ManifestHash:      plan.ManifestHash,
		ConditionFileHash: plan.ConditionFileHashes["dry"],
		Response:          &replacement,
		Staged:            fixtureStaged(),
	})
	if err != nil {
		t.Fatalf("duplicate supplemental submit failed: %v", err)
	}
	if !warningsContain(result.Warnings, "replaced duplicate supplemental verifier") {
		t.Fatalf("duplicate warning missing: %#v", result.Warnings)
	}
	dry = responseByCondition(t, readResponsesFile(t, responsesPath), "dry")
	if len(dry.Supplemental) != 1 || dry.Supplemental[0].Evidence[0] != "replacement supplemental evidence" {
		t.Fatalf("duplicate supplemental was not deterministically replaced: %#v", dry.Supplemental)
	}

	adjudicationOnly := &SubmitVerifierInput{
		StagedPayloadHash: plan.StagedPayloadHash,
		ManifestHash:      plan.ManifestHash,
		ConditionFileHash: plan.ConditionFileHashes["dry"],
		ResponseCondition: ResponseCondition{
			ConditionID:   "dry",
			ConditionFile: "backpressure/dry.md",
			Adjudication: &ResponseAdjudication{
				Authority:    "RusticDog",
				Summary:      "Replacement ruling records the final audit trail.",
				FinalVerdict: attestations.VerdictPass,
				AuditRef:     "Agent Mail 2702",
			},
		},
	}
	result, err = RunVerifierSubmit(context.Background(), SubmitVerifierOptions{
		Root:              root,
		ExplicitFeature:   "br-123",
		ConditionID:       "dry",
		ResponsesPath:     responsesPath,
		StagedPayloadHash: plan.StagedPayloadHash,
		ManifestHash:      plan.ManifestHash,
		ConditionFileHash: plan.ConditionFileHashes["dry"],
		Response:          adjudicationOnly,
		Staged:            fixtureStaged(),
	})
	if err != nil {
		t.Fatalf("adjudication-only submit failed: %v", err)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "replaced adjudication") {
		t.Fatalf("adjudication replacement warning missing: %#v", result.Warnings)
	}
	dry = responseByCondition(t, readResponsesFile(t, responsesPath), "dry")
	if dry.Verifier.Agent != "Verifier-dry" || dry.Verdict != attestations.VerdictPass || len(dry.Supplemental) != 1 {
		t.Fatalf("adjudication-only submit should preserve primary and supplemental evidence: %#v", dry)
	}
	if dry.Adjudication == nil || dry.Adjudication.AuditRef != "Agent Mail 2702" {
		t.Fatalf("adjudication-only submit did not replace adjudication: %#v", dry.Adjudication)
	}
}

func warningsContain(warnings []string, needle string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, needle) {
			return true
		}
	}
	return false
}

func TestRunVerifierSubmitStoresTranscriptSummary(t *testing.T) {
	root, plan, responsesPath := beginSubmitFixture(t)
	writeFile(t, root, ".burpvalve.json", `{"schema_version":1,"defaults":{"verifier":{"transcript_dir":"log/custom-transcripts","transcripts":"summary"}}}`)
	input := submitInputFor(t, plan, "dry")
	longTranscript := strings.Repeat("line\n", 25)
	result, err := RunVerifierSubmit(context.Background(), SubmitVerifierOptions{
		Root:              root,
		ExplicitFeature:   "br-123",
		ConditionID:       "dry",
		ResponsesPath:     responsesPath,
		StagedPayloadHash: plan.StagedPayloadHash,
		ManifestHash:      plan.ManifestHash,
		ConditionFileHash: plan.ConditionFileHashes["dry"],
		Response:          input,
		TranscriptPath:    "-",
		TranscriptReader:  strings.NewReader(longTranscript),
		Staged:            fixtureStaged(),
	})
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	if result.TranscriptRef == "" || !strings.HasPrefix(result.TranscriptRef, "log/custom-transcripts/") {
		t.Fatalf("transcript ref = %q", result.TranscriptRef)
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.TranscriptRef)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "transcript summary truncated") {
		t.Fatalf("transcript was not summarized:\n%s", body)
	}
	if got := responseByCondition(t, readResponsesFile(t, responsesPath), "dry").Verifier.TranscriptRef; got != result.TranscriptRef {
		t.Fatalf("condition transcript ref = %q, want %q", got, result.TranscriptRef)
	}
}

func TestRunVerifierSubmitConcurrentPrimaryAndSupplemental(t *testing.T) {
	root, plan, responsesPath := beginSubmitFixture(t)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := RunVerifierSubmit(context.Background(), SubmitVerifierOptions{
			Root:              root,
			ExplicitFeature:   "br-123",
			ConditionID:       "dry",
			ResponsesPath:     responsesPath,
			StagedPayloadHash: plan.StagedPayloadHash,
			ManifestHash:      plan.ManifestHash,
			ConditionFileHash: plan.ConditionFileHashes["dry"],
			Response:          submitInputFor(t, plan, "dry"),
			Staged:            fixtureStaged(),
		})
		errs <- err
	}()
	go func() {
		defer wg.Done()
		input := &SubmitVerifierInput{
			StagedPayloadHash: plan.StagedPayloadHash,
			ManifestHash:      plan.ManifestHash,
			ConditionFileHash: plan.ConditionFileHashes["dry"],
			ResponseCondition: ResponseCondition{
				ConditionID:   "dry",
				ConditionFile: "backpressure/dry.md",
				Supplemental: []SupplementalVerifier{{
					Verifier: submitVerifier("ConcurrentSupplemental"),
					Verdict:  attestations.VerdictPass,
					Message:  "Supplemental verifier passed.",
					Evidence: []string{"supplemental concurrent evidence"},
				}},
				Adjudication: &ResponseAdjudication{Authority: "RusticDog", Summary: "No conflict.", AuditRef: "Agent Mail 2701"},
			},
		}
		_, err := RunVerifierSubmit(context.Background(), SubmitVerifierOptions{
			Root:              root,
			ExplicitFeature:   "br-123",
			ConditionID:       "dry",
			ResponsesPath:     responsesPath,
			StagedPayloadHash: plan.StagedPayloadHash,
			ManifestHash:      plan.ManifestHash,
			ConditionFileHash: plan.ConditionFileHashes["dry"],
			Response:          input,
			Staged:            fixtureStaged(),
		})
		errs <- err
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent submit failed: %v", err)
		}
	}
	var decoded map[string]any
	body, err := os.ReadFile(responsesPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("response file is partial/corrupt: %v\n%s", err, body)
	}
	dry := responseByCondition(t, readResponsesFile(t, responsesPath), "dry")
	if dry.Verdict != attestations.VerdictPass || len(dry.Supplemental) != 1 || dry.Adjudication == nil {
		t.Fatalf("lost concurrent update: %#v", dry)
	}
}

func beginSubmitFixture(t *testing.T) (string, Plan, string) {
	t.Helper()
	root := fixtureProject(t)
	result, err := RunVerifierBegin(context.Background(), BeginResponsesOptions{
		Root:             root,
		ExplicitFeature:  "br-123",
		OneFeature:       true,
		AtomicityMessage: "Staged changes map only to br-123.",
		Staged:           fixtureStaged(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return root, result.Plan, filepath.Join(root, filepath.FromSlash(result.ResponsesPath))
}

func beginLaneSubmitFixture(t *testing.T) (string, Plan, string) {
	t.Helper()
	root := fixtureProject(t)
	result, err := RunVerifierBegin(context.Background(), BeginResponsesOptions{
		Root:            root,
		ExplicitFeature: "declared-lane-aj41",
		Lane: LaneOptions{
			Enabled:           true,
			LaneID:            "declared-lane-aj41",
			BeadIDs:           []string{"burpvalve-aj41.3", "burpvalve-aj41.4"},
			Rationale:         "same orchestrator-authorized lane",
			AuthorizedBy:      "BronzeDeer",
			AuthorizationRef:  "Agent Mail 4000",
			AuthorizationKind: "orchestrator",
		},
		Staged: fixtureStaged(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return root, result.Plan, filepath.Join(root, filepath.FromSlash(result.ResponsesPath))
}

func submitInputFor(t *testing.T, plan Plan, conditionID string) *SubmitVerifierInput {
	t.Helper()
	condition, conditionHash, err := submitConditionSpec(plan, conditionID)
	if err != nil {
		t.Fatal(err)
	}
	return &SubmitVerifierInput{
		StagedPayloadHash: plan.StagedPayloadHash,
		ManifestHash:      plan.ManifestHash,
		ConditionFileHash: conditionHash,
		ResponseCondition: ResponseCondition{
			ConditionID:       condition.ID,
			ConditionFile:     condition.Path,
			VerifierPolicy:    normalizeConditionPolicy(condition),
			Verifier:          submitVerifier("Verifier-" + condition.ID),
			SubagentConfirmed: true,
			SubagentModel:     "gpt-5",
			Verdict:           attestations.VerdictPass,
			Message:           "Verifier passed " + condition.ID + ".",
			Evidence:          []string{"inspected staged payload for " + condition.ID},
			NextAction:        "",
		},
	}
}

func submitVerifier(agent string) attestations.Verifier {
	return attestations.Verifier{
		Kind:            attestations.VerifierIndependentSubagent,
		Agent:           agent,
		Model:           "gpt-5",
		Runtime:         "codex-cli",
		SeparateContext: true,
	}
}

func readResponsesFile(t *testing.T, path string) Responses {
	t.Helper()
	responses, err := LoadResponses(path)
	if err != nil {
		t.Fatal(err)
	}
	return *responses
}

func responseByCondition(t *testing.T, responses Responses, conditionID string) ResponseCondition {
	t.Helper()
	for _, response := range responses.Conditions {
		if response.ConditionID == conditionID {
			return response
		}
	}
	t.Fatalf("condition %s not found in %#v", conditionID, responses.Conditions)
	return ResponseCondition{}
}
