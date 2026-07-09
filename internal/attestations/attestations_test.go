package attestations

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var testTime = time.Date(2026, 6, 20, 2, 0, 0, 0, time.UTC)

func TestValidatePassingAcceptsPassAndNotApplicable(t *testing.T) {
	artifact := validPassingArtifact()
	artifact.Conditions = append(artifact.Conditions, Condition{
		ConditionID:       "dry",
		ConditionFile:     "backpressure/dry.md",
		ConditionFileHash: "sha256:dry",
		SubagentConfirmed: true,
		Verdict:           VerdictNotApplicable,
		Message:           "Documentation-only change has no duplicate logic surface.",
		Timestamp:         testTime,
	})
	artifact.ConditionOrder = []string{"lint-rules", "dry"}
	expected := expectedBinding()
	expected.ConditionOrder = []string{"lint-rules", "dry"}
	expected.ConditionHashes["dry"] = "sha256:dry"

	if err := artifact.ValidatePassing(expected); err != nil {
		t.Fatalf("ValidatePassing returned error: %v", err)
	}
}

func TestValidatePassingRejectsMissingCell(t *testing.T) {
	artifact := validPassingArtifact()
	artifact.ConditionOrder = []string{"lint-rules", "dry"}
	expected := expectedBinding()
	expected.ConditionOrder = []string{"lint-rules", "dry"}
	expected.ConditionHashes["dry"] = "sha256:dry"

	assertValidationError(t, artifact.ValidatePassing(expected), "cell count")
}

func TestValidatePassingRejectsFailUnknownAndUnconfirmed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Artifact)
		want   string
	}{
		{
			name: "fail",
			mutate: func(a *Artifact) {
				a.Conditions[0].Verdict = VerdictFail
				a.Conditions[0].Message = "failed"
			},
			want: "non-passing verdict",
		},
		{
			name: "unknown",
			mutate: func(a *Artifact) {
				a.Conditions[0].Verdict = VerdictUnknown
				a.Conditions[0].Message = "unknown"
			},
			want: "non-passing verdict",
		},
		{
			name: "unconfirmed",
			mutate: func(a *Artifact) {
				a.Conditions[0].SubagentConfirmed = false
			},
			want: "verifier policy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifact := validPassingArtifact()
			tt.mutate(&artifact)
			assertValidationError(t, artifact.ValidatePassing(expectedBinding()), tt.want)
		})
	}
}

func TestValidatePassingVerifierPolicyKinds(t *testing.T) {
	tests := []struct {
		name   string
		policy VerifierPolicy
		kind   VerifierKind
		wantOK bool
		want   string
	}{
		{
			name:   "independent verifier passes default policy",
			kind:   VerifierIndependentSubagent,
			wantOK: true,
		},
		{
			name:   "legacy subagent confirmed passes default policy",
			wantOK: true,
		},
		{
			name:   "main agent allowed",
			policy: VerifierPolicyMainAgentAllowed,
			kind:   VerifierMainAgent,
			wantOK: true,
		},
		{
			name: "main agent blocked by independent default",
			kind: VerifierMainAgent,
			want: "independent_required",
		},
		{
			name:   "ci allowed",
			policy: VerifierPolicyCIAllowed,
			kind:   VerifierCI,
			wantOK: true,
		},
		{
			name:   "human allowed",
			policy: VerifierPolicyHumanAllowed,
			kind:   VerifierHuman,
			wantOK: true,
		},
		{
			name:   "optional no verifier",
			policy: VerifierPolicyOptional,
			kind:   VerifierNone,
			wantOK: true,
		},
		{
			name:   "optional unknown verifier blocked",
			policy: VerifierPolicyOptional,
			kind:   VerifierUnknown,
			want:   "verifier kind \"unknown\"",
		},
		{
			name: "missing verifier metadata blocked",
			want: "verifier kind \"none\"",
		},
		{
			name: "unknown verifier kind blocked",
			kind: VerifierUnknown,
			want: "verifier kind \"unknown\"",
		},
		{
			name: "invalid verifier kind rejected",
			kind: VerifierKind("robot"),
			want: "invalid verifier kind",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifact := validPassingArtifact()
			condition := &artifact.Conditions[0]
			condition.VerifierPolicy = tt.policy
			condition.SubagentConfirmed = false
			condition.SubagentModel = ""
			condition.Verifier = Verifier{}
			if tt.kind != "" {
				condition.Verifier = Verifier{Kind: tt.kind, Model: "verifier-model", SeparateContext: tt.kind == VerifierIndependentSubagent}
			}
			if tt.kind == "" && tt.wantOK {
				condition.SubagentConfirmed = true
				condition.SubagentModel = "sonnet"
			}
			err := artifact.ValidatePassing(expectedBinding())
			if tt.wantOK {
				if err != nil {
					t.Fatalf("ValidatePassing returned error: %v", err)
				}
				return
			}
			assertValidationError(t, err, tt.want)
		})
	}
}

func TestValidatePassingRejectsNotApplicableWithoutMessage(t *testing.T) {
	artifact := validPassingArtifact()
	artifact.Conditions[0].Verdict = VerdictNotApplicable
	artifact.Conditions[0].Message = ""

	assertValidationError(t, artifact.ValidatePassing(expectedBinding()), "not_applicable without message")
}

func TestValidatePassingPreservesSupplementalAndAdjudicationMetadata(t *testing.T) {
	artifact := validPassingArtifact()
	artifact.Conditions[0].Supplemental = []SupplementalVerifier{{
		Verifier: Verifier{
			Kind:            VerifierIndependentSubagent,
			Agent:           "ScarletMarsh",
			Model:           "gpt-5-codex",
			Runtime:         "codex-cli",
			SeparateContext: true,
		},
		Verdict:       VerdictFail,
		Message:       "Supplemental verifier found a disagreement.",
		Evidence:      []string{"Agent Mail 3131: supplemental disagreement"},
		TranscriptRef: "Agent Mail 3131",
		NextAction:    "Hold and escalate to the owner.",
	}}
	artifact.Conditions[0].Adjudication = &ResponseAdjudication{
		Authority:    "RusticDog",
		Summary:      "Primary pass accepted for commit; supplemental disagreement remains audit metadata.",
		FinalVerdict: VerdictPass,
		AuditRef:     "Agent Mail 3135",
	}

	if err := artifact.ValidatePassing(expectedBinding()); err != nil {
		t.Fatalf("ValidatePassing returned error: %v", err)
	}
	if got := artifact.Conditions[0].Supplemental[0].TranscriptRef; got != "Agent Mail 3131" {
		t.Fatalf("supplemental transcript ref = %q", got)
	}
}

func TestValidatePassingRejectsPrimaryFailDespiteAdjudicationFinalPass(t *testing.T) {
	artifact := validPassingArtifact()
	artifact.Conditions[0].Verdict = VerdictFail
	artifact.Conditions[0].Message = "Primary verifier failed this cell."
	artifact.Conditions[0].NextAction = "Fix the blocker."
	artifact.Conditions[0].Adjudication = &ResponseAdjudication{
		Authority:    "RusticDog",
		Summary:      "Adjudication is audit metadata only.",
		FinalVerdict: VerdictPass,
		AuditRef:     "Agent Mail ruling",
	}

	assertValidationError(t, artifact.ValidatePassing(expectedBinding()), "non-passing verdict")
}

func TestValidatePassingRejectsStaleHashesAndMissingIdentity(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Artifact)
		want   string
	}{
		{
			name: "stale payload",
			mutate: func(a *Artifact) {
				a.StagedPayloadHash = "sha256:old"
			},
			want: "staged payload hash is stale",
		},
		{
			name: "stale manifest",
			mutate: func(a *Artifact) {
				a.ManifestHash = "sha256:old"
			},
			want: "manifest hash is stale",
		},
		{
			name: "stale condition",
			mutate: func(a *Artifact) {
				a.Conditions[0].ConditionFileHash = "sha256:old"
			},
			want: "file hash is stale",
		},
		{
			name: "missing condition file hash",
			mutate: func(a *Artifact) {
				a.Conditions[0].ConditionFileHash = ""
			},
			want: "missing condition_file_hash",
		},
		{
			name: "malformed artifact timestamp",
			mutate: func(a *Artifact) {
				a.CreatedAt = time.Time{}
			},
			want: "created_at timestamp is required",
		},
		{
			name: "malformed condition timestamp",
			mutate: func(a *Artifact) {
				a.Conditions[0].Timestamp = time.Time{}
			},
			want: "timestamp is required",
		},
		{
			name: "missing feature id",
			mutate: func(a *Artifact) {
				a.Feature.ID = ""
			},
			want: "feature id, kind, and name are required",
		},
		{
			name: "missing source",
			mutate: func(a *Artifact) {
				a.Feature.SourceBead = ""
				a.Feature.DiffCluster = ""
			},
			want: "source_bead or diff_cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifact := validPassingArtifact()
			tt.mutate(&artifact)
			assertValidationError(t, artifact.ValidatePassing(expectedBinding()), tt.want)
		})
	}
}

func TestValidateBlockedAllowsFailUnknownAndMissingSubagentWithMessages(t *testing.T) {
	artifact := validPassingArtifact()
	artifact.ArtifactKind = ArtifactBlocked
	artifact.Atomicity.OneFeatureOrFix = false
	artifact.Conditions[0].SubagentConfirmed = false
	artifact.Conditions[0].Verdict = VerdictUnknown
	artifact.Conditions[0].Message = "No dedicated verifier was available for lint-rules on br-123."
	artifact.Conditions[0].Evidence = []string{"staged diff unavailable"}
	artifact.Conditions[0].NextAction = "Spawn verifier and retry."

	if err := artifact.ValidateBlocked(expectedBinding()); err != nil {
		t.Fatalf("ValidateBlocked returned error: %v", err)
	}
}

func TestValidateBlockedRejectsBareFailure(t *testing.T) {
	artifact := validPassingArtifact()
	artifact.ArtifactKind = ArtifactBlocked
	artifact.Conditions[0].Verdict = VerdictFail
	artifact.Conditions[0].Message = ""
	artifact.Conditions[0].Evidence = nil
	artifact.Conditions[0].NextAction = ""

	assertValidationError(t, artifact.ValidateBlocked(expectedBinding()), "missing failure or unknown message")
}

func TestArtifactSchemaRoundTripsLaneAtomicityAndLegacySingleMode(t *testing.T) {
	artifact := validPassingArtifact()
	artifact.Feature = Feature{
		ID:          "declared-lane-aj41-95t2",
		Kind:        "lane",
		Name:        "declared-lane-aj41-95t2",
		BeadIDs:     []string{"burpvalve-aj41", "burpvalve-95t2"},
		DiffCluster: "lane:declared-lane-aj41-95t2",
	}
	artifact.BeadIDs = []string{"burpvalve-aj41", "burpvalve-95t2"}
	artifact.CoupledWorkRationale = "one orchestrator ruling plus tracker export"
	artifact.Atomicity = Atomicity{
		Mode:            AtomicityModeLane,
		OneFeatureOrFix: false,
		Message:         "Orchestrator-authorized lane commit naming every bead id.",
		Lane: &LaneBinding{
			LaneID:            "declared-lane-aj41-95t2",
			BeadIDs:           []string{"burpvalve-aj41", "burpvalve-95t2"},
			Rationale:         "one orchestrator ruling plus tracker export",
			AuthorizedBy:      "BronzeDeer",
			AuthorizationRef:  "Agent Mail 1234 / ORCHESTRATOR.md ruling",
			AuthorizationKind: "orchestrator",
		},
	}

	body, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"kind":"lane"`, `"mode":"lane"`, `"lane":`, `"bead_ids":["burpvalve-aj41","burpvalve-95t2"]`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("artifact JSON missing %s: %s", want, string(body))
		}
	}
	var decoded Artifact
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Atomicity.Mode != AtomicityModeLane || decoded.Atomicity.Lane == nil {
		t.Fatalf("lane atomicity not preserved: %#v", decoded.Atomicity)
	}
	if decoded.Feature.Kind != "lane" || decoded.Feature.DiffCluster != "lane:declared-lane-aj41-95t2" {
		t.Fatalf("lane feature not preserved: %#v", decoded.Feature)
	}
	if got := decoded.Atomicity.Lane.AuthorizationKind; got != "orchestrator" {
		t.Fatalf("authorization_kind = %q", got)
	}

	var legacy Artifact
	if err := json.Unmarshal([]byte(`{"atomicity":{"one_feature_or_fix":true,"message":"single bead"}}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Atomicity.Mode != "" || legacy.Atomicity.Lane != nil || !legacy.Atomicity.OneFeatureOrFix {
		t.Fatalf("legacy atomicity changed: %#v", legacy.Atomicity)
	}
}

func TestValidatePassingAcceptsCompleteLaneAtomicity(t *testing.T) {
	artifact := validPassingArtifact()
	artifact.Feature = Feature{
		ID:          "declared-lane-aj41-95t2",
		Kind:        "lane",
		Name:        "declared-lane-aj41-95t2",
		BeadIDs:     []string{"burpvalve-aj41", "burpvalve-95t2"},
		DiffCluster: "lane:declared-lane-aj41-95t2",
	}
	artifact.BeadIDs = []string{"burpvalve-aj41", "burpvalve-95t2"}
	artifact.CoupledWorkRationale = "one orchestrator ruling plus tracker export"
	artifact.Atomicity = Atomicity{
		Mode:            AtomicityModeLane,
		OneFeatureOrFix: false,
		Message:         "Orchestrator-authorized lane commit naming every bead id.",
		Lane: &LaneBinding{
			LaneID:            "declared-lane-aj41-95t2",
			BeadIDs:           []string{"burpvalve-aj41", "burpvalve-95t2"},
			Rationale:         "one orchestrator ruling plus tracker export",
			AuthorizedBy:      "BronzeDeer",
			AuthorizationRef:  "Agent Mail 1234 / ORCHESTRATOR.md ruling",
			AuthorizationKind: "orchestrator",
		},
	}
	if err := artifact.ValidatePassing(expectedBinding()); err != nil {
		t.Fatalf("lane artifact should validate: %v", err)
	}
}

func TestValidatePassingRejectsIncompleteLaneAtomicity(t *testing.T) {
	base := validPassingArtifact()
	base.Feature = Feature{
		ID:          "declared-lane-aj41-95t2",
		Kind:        "lane",
		Name:        "declared-lane-aj41-95t2",
		BeadIDs:     []string{"burpvalve-aj41", "burpvalve-95t2"},
		DiffCluster: "lane:declared-lane-aj41-95t2",
	}
	base.BeadIDs = []string{"burpvalve-aj41", "burpvalve-95t2"}
	base.CoupledWorkRationale = "one orchestrator ruling plus tracker export"
	base.Atomicity = Atomicity{
		Mode:    AtomicityModeLane,
		Message: "Orchestrator-authorized lane commit naming every bead id.",
		Lane: &LaneBinding{
			LaneID:            "declared-lane-aj41-95t2",
			BeadIDs:           []string{"burpvalve-aj41", "burpvalve-95t2"},
			Rationale:         "one orchestrator ruling plus tracker export",
			AuthorizedBy:      "BronzeDeer",
			AuthorizationRef:  "Agent Mail 1234 / ORCHESTRATOR.md ruling",
			AuthorizationKind: "orchestrator",
		},
	}
	tests := []struct {
		name   string
		mutate func(*Artifact)
		want   string
	}{
		{
			name: "one feature flag set",
			mutate: func(a *Artifact) {
				a.Atomicity.OneFeatureOrFix = true
			},
			want: "one_feature_or_fix",
		},
		{
			name: "missing lane",
			mutate: func(a *Artifact) {
				a.Atomicity.Lane = nil
			},
			want: "atomicity.lane",
		},
		{
			name: "bead mismatch",
			mutate: func(a *Artifact) {
				a.BeadIDs = []string{"burpvalve-aj41", "other"}
			},
			want: "bead_ids must match",
		},
		{
			name: "feature kind mismatch",
			mutate: func(a *Artifact) {
				a.Feature.Kind = "feature"
			},
			want: "feature kind lane",
		},
		{
			name: "authorization missing",
			mutate: func(a *Artifact) {
				a.Atomicity.Lane.AuthorizationRef = ""
			},
			want: "authorization_ref",
		},
		{
			name: "authorization kind missing",
			mutate: func(a *Artifact) {
				a.Atomicity.Lane.AuthorizationKind = ""
			},
			want: "authorization_kind",
		},
		{
			name: "authorization kind not orchestrator",
			mutate: func(a *Artifact) {
				a.Atomicity.Lane.AuthorizationKind = "human"
			},
			want: `not "orchestrator"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifact := base
			lane := *base.Atomicity.Lane
			artifact.Atomicity.Lane = &lane
			tt.mutate(&artifact)
			assertValidationError(t, artifact.ValidatePassing(expectedBinding()), tt.want)
		})
	}
}

func validPassingArtifact() Artifact {
	return Artifact{
		SchemaVersion:       1,
		Tool:                ToolName,
		ToolVersion:         ToolVersion,
		ArtifactKind:        ArtifactPassing,
		StagedPayloadHash:   "sha256:payload",
		ManifestHash:        "sha256:manifest",
		ConditionOrder:      []string{"lint-rules"},
		GeneratedBy:         Generator{Agent: "codex", Model: "gpt-5"},
		GitHeadBeforeCommit: "abc123",
		CreatedAt:           testTime,
		Feature: Feature{
			ID:         "br-123",
			Kind:       "feature",
			Name:       "checkout discounts",
			SourceBead: "br-123",
		},
		Atomicity: Atomicity{
			OneFeatureOrFix: true,
			Message:         "Staged changes map only to br-123.",
		},
		Conditions: []Condition{{
			ConditionID:       "lint-rules",
			ConditionFile:     "backpressure/lint-rules.md",
			ConditionFileHash: "sha256:lint",
			SubagentConfirmed: true,
			SubagentModel:     "sonnet",
			Verdict:           VerdictPass,
			Evidence:          []string{"go test ./..."},
			Timestamp:         testTime,
		}},
	}
}

func expectedBinding() ExpectedBinding {
	return ExpectedBinding{
		StagedPayloadHash: "sha256:payload",
		ManifestHash:      "sha256:manifest",
		ConditionOrder:    []string{"lint-rules"},
		ConditionHashes: map[string]string{
			"lint-rules": "sha256:lint",
		},
	}
}

func assertValidationError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected validation error containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("validation error = %q, want substring %q", err.Error(), want)
	}
}
