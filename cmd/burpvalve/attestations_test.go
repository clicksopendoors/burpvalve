package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"burpvalve/internal/attestations"
)

type attestationListRecord struct {
	Status               string   `json:"status"`
	ArtifactType         string   `json:"artifact_type"`
	Path                 string   `json:"path"`
	FeatureIDs           []string `json:"feature_ids"`
	BeadIDs              []string `json:"bead_ids"`
	LaneID               string   `json:"lane_id"`
	LaneAuthorizationRef string   `json:"lane_authorization_ref"`
	PayloadHash          string   `json:"payload_hash"`
	ParseWarnings        []string `json:"parse_warnings"`
	Conditions           []struct {
		ConditionID     string                              `json:"condition_id"`
		VerifierKind    string                              `json:"verifier_kind"`
		Supplemental    []attestations.SupplementalVerifier `json:"supplemental_verifiers"`
		Adjudication    *attestations.ResponseAdjudication  `json:"adjudication"`
		HasDisagreement bool                                `json:"has_disagreement"`
	} `json:"condition_verdicts"`
}

func TestAttestationsListShowLatestJSONAndHuman(t *testing.T) {
	root := t.TempDir()
	newer := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	older := time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC)
	passing := attestationQueryArtifactFixture(attestations.ArtifactPassing, "abcdef1234567890", "br-123", newer)
	passing.Conditions[0].Supplemental = []attestations.SupplementalVerifier{{
		Verifier: attestations.Verifier{
			Kind:            attestations.VerifierIndependentSubagent,
			Agent:           "ScarletMarsh",
			Model:           "gpt-5-codex",
			Runtime:         "codex-cli",
			SeparateContext: true,
		},
		Verdict:       attestations.VerdictFail,
		Message:       "Supplemental verifier disagreed.",
		Evidence:      []string{"supplemental query evidence"},
		TranscriptRef: "Agent Mail 3131",
		NextAction:    "Hold and escalate.",
	}}
	passing.Conditions[0].Adjudication = &attestations.ResponseAdjudication{Authority: "RusticDog", Summary: "Audit ruling.", FinalVerdict: attestations.VerdictPass, AuditRef: "Agent Mail 3135"}
	writeAttestationQueryFixture(t, root, "backpressure/attestations/abcdef1234567890.json", passing)
	writeAttestationQueryFixture(t, root, "log/backpressure/failed/blocked-report.json", attestationQueryArtifactFixture(attestations.ArtifactBlocked, "fedcba9876543210", "br-blocked", older))
	writeCmdTestFile(t, filepath.Join(root, "backpressure/attestations/bad.json"), `{"not valid"`)

	stdout, stderr, err := executeBurpvalveCommand("attestations", "list", "--root", root, "--json")
	if err != nil {
		t.Fatalf("attestations list failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("attestations list wrote stderr: %s", stderr)
	}
	var list struct {
		SchemaVersion int                     `json:"schema_version"`
		Records       []attestationListRecord `json:"records"`
	}
	if err := json.Unmarshal([]byte(stdout), &list); err != nil {
		t.Fatalf("decode list: %v\n%s", err, stdout)
	}
	if list.SchemaVersion != 1 || len(list.Records) != 3 {
		t.Fatalf("unexpected list response: %#v", list)
	}
	if list.Records[0].Status != "pass" || list.Records[0].PayloadHash != "abcdef1234567890" {
		t.Fatalf("newest passing artifact should sort first: %#v", list.Records)
	}
	if list.Records[0].FeatureIDs[0] != "br-123" || list.Records[0].BeadIDs[0] != "br-123" {
		t.Fatalf("feature/bead ids missing: %#v", list.Records[0])
	}
	if list.Records[0].Conditions[0].VerifierKind != "independent_subagent" {
		t.Fatalf("verifier provenance missing: %#v", list.Records[0].Conditions)
	}
	if !list.Records[0].Conditions[0].HasDisagreement ||
		len(list.Records[0].Conditions[0].Supplemental) != 1 ||
		list.Records[0].Conditions[0].Adjudication == nil ||
		list.Records[0].Conditions[0].Adjudication.AuditRef != "Agent Mail 3135" {
		t.Fatalf("supplemental/adjudication query metadata missing: %#v", list.Records[0].Conditions[0])
	}
	if malformed := findAttestationQueryRecord(list.Records, "malformed"); malformed == nil || len(malformed.ParseWarnings) == 0 {
		t.Fatalf("malformed artifact should remain listed with parse warnings: %#v", list.Records)
	}

	human, stderr, err := executeBurpvalveCommand("attestations", "list", "--root", root, "--color", "never")
	if err != nil {
		t.Fatalf("human attestations list failed: %v\nstdout=%s\nstderr=%s", err, human, stderr)
	}
	for _, needle := range []string{"Burpvalve attestations", "STATUS", "pass", "blocked", "malformed", "backpressure/attestations/abcdef1234567890.json"} {
		if !strings.Contains(human, needle) {
			t.Fatalf("human list missing %q:\n%s", needle, human)
		}
	}

	showPath := "backpressure/attestations/abcdef1234567890.json"
	shown, stderr, err := executeBurpvalveCommand("attestations", "show", "--root", root, "--json", showPath)
	if err != nil {
		t.Fatalf("show path failed: %v\nstdout=%s\nstderr=%s", err, shown, stderr)
	}
	var shownRecord struct {
		Status      string   `json:"status"`
		FeatureIDs  []string `json:"feature_ids"`
		BeadIDs     []string `json:"bead_ids"`
		PayloadHash string   `json:"payload_hash"`
	}
	if err := json.Unmarshal([]byte(shown), &shownRecord); err != nil {
		t.Fatalf("decode show: %v\n%s", err, shown)
	}
	if shownRecord.Status != "pass" || shownRecord.PayloadHash != "abcdef1234567890" || shownRecord.BeadIDs[0] != "br-123" {
		t.Fatalf("unexpected shown record: %#v", shownRecord)
	}

	prefix, stderr, err := executeBurpvalveCommand("attestations", "show", "--root", root, "--json", "abcdef")
	if err != nil {
		t.Fatalf("show prefix failed: %v\nstdout=%s\nstderr=%s", err, prefix, stderr)
	}
	if !strings.Contains(prefix, `"payload_hash": "abcdef1234567890"`) {
		t.Fatalf("prefix show returned wrong artifact:\n%s", prefix)
	}

	humanShow, stderr, err := executeBurpvalveCommand("attestations", "show", "--root", root, "--color", "never", showPath)
	if err != nil {
		t.Fatalf("human show failed: %v\nstdout=%s\nstderr=%s", err, humanShow, stderr)
	}
	for _, needle := range []string{"Burpvalve attestation", "Status:", "pass", "Conditions", "dry", "independent_subagent"} {
		if !strings.Contains(humanShow, needle) {
			t.Fatalf("human show missing %q:\n%s", needle, humanShow)
		}
	}

	latest, stderr, err := executeBurpvalveCommand("attestations", "latest", "--root", root, "--json")
	if err != nil {
		t.Fatalf("latest failed: %v\nstdout=%s\nstderr=%s", err, latest, stderr)
	}
	if !strings.Contains(latest, `"payload_hash": "abcdef1234567890"`) {
		t.Fatalf("latest returned wrong artifact:\n%s", latest)
	}

	humanLatest, stderr, err := executeBurpvalveCommand("attestations", "latest", "--root", root, "--color", "never")
	if err != nil {
		t.Fatalf("human latest failed: %v\nstdout=%s\nstderr=%s", err, humanLatest, stderr)
	}
	for _, needle := range []string{"Burpvalve attestation", "Payload:", "abcdef1234567890", "Feature:", "br-123"} {
		if !strings.Contains(humanLatest, needle) {
			t.Fatalf("human latest missing %q:\n%s", needle, humanLatest)
		}
	}

	blocked, stderr, err := executeBurpvalveCommand("attestations", "list", "--root", root, "--json", "--status", "blocked")
	if err != nil {
		t.Fatalf("blocked list failed: %v\nstdout=%s\nstderr=%s", err, blocked, stderr)
	}
	if !strings.Contains(blocked, `"status": "blocked"`) || strings.Contains(blocked, `"status": "pass"`) {
		t.Fatalf("blocked filter wrong:\n%s", blocked)
	}

	feature, stderr, err := executeBurpvalveCommand("attestations", "list", "--root", root, "--json", "--feature", "br-123")
	if err != nil {
		t.Fatalf("feature filter failed: %v\nstdout=%s\nstderr=%s", err, feature, stderr)
	}
	if !strings.Contains(feature, `"payload_hash": "abcdef1234567890"`) || strings.Contains(feature, `"payload_hash": "fedcba9876543210"`) {
		t.Fatalf("feature filter wrong:\n%s", feature)
	}

	bead, stderr, err := executeBurpvalveCommand("attestations", "list", "--root", root, "--json", "--bead", "br-blocked")
	if err != nil {
		t.Fatalf("bead filter failed: %v\nstdout=%s\nstderr=%s", err, bead, stderr)
	}
	if !strings.Contains(bead, `"payload_hash": "fedcba9876543210"`) || strings.Contains(bead, `"payload_hash": "abcdef1234567890"`) {
		t.Fatalf("bead filter wrong:\n%s", bead)
	}

	limited, stderr, err := executeBurpvalveCommand("attestations", "list", "--root", root, "--json", "--limit", "1")
	if err != nil {
		t.Fatalf("limit filter failed: %v\nstdout=%s\nstderr=%s", err, limited, stderr)
	}
	var limitedList struct {
		Records []attestationListRecord `json:"records"`
	}
	if err := json.Unmarshal([]byte(limited), &limitedList); err != nil {
		t.Fatalf("decode limited list: %v\n%s", err, limited)
	}
	if len(limitedList.Records) != 1 || limitedList.Records[0].PayloadHash != "abcdef1234567890" {
		t.Fatalf("limit should return the newest single record: %#v", limitedList.Records)
	}
}

func TestAttestationsExposeLaneMetadata(t *testing.T) {
	root := t.TempDir()
	created := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	artifact := attestationQueryArtifactFixture(attestations.ArtifactPassing, "laneabcdef123456", "declared-lane-aj41", created)
	artifact.BeadIDs = []string{"burpvalve-aj41.3", "burpvalve-aj41.4"}
	artifact.Feature.BeadIDs = []string{"burpvalve-aj41.3", "burpvalve-aj41.4"}
	artifact.Feature.DiffCluster = "lane:declared-lane-aj41"
	artifact.Atomicity = attestations.Atomicity{
		Mode:            attestations.AtomicityModeLane,
		OneFeatureOrFix: false,
		Message:         "Orchestrator-authorized lane commit naming every bead id.",
		Lane: &attestations.LaneBinding{
			LaneID:            "declared-lane-aj41",
			BeadIDs:           []string{"burpvalve-aj41.3", "burpvalve-aj41.4"},
			Rationale:         "same authorized lane payload",
			AuthorizedBy:      "BronzeDeer",
			AuthorizationRef:  "ORCH-2026-07-08",
			AuthorizationKind: attestations.LaneAuthorizationKindOrchestrator,
			CreatedAt:         &created,
		},
	}
	writeAttestationQueryFixture(t, root, "backpressure/attestations/laneabcdef123456.json", artifact)

	stdout, stderr, err := executeBurpvalveCommand("attestations", "list", "--root", root, "--json")
	if err != nil {
		t.Fatalf("attestations list failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var list struct {
		Records []attestationListRecord `json:"records"`
	}
	if err := json.Unmarshal([]byte(stdout), &list); err != nil {
		t.Fatalf("decode list: %v\n%s", err, stdout)
	}
	if len(list.Records) != 1 {
		t.Fatalf("expected one lane record, got %#v", list.Records)
	}
	record := list.Records[0]
	if record.LaneID != "declared-lane-aj41" || record.LaneAuthorizationRef != "ORCH-2026-07-08" {
		t.Fatalf("lane metadata missing from JSON record: %#v", record)
	}
	for _, beadID := range []string{"burpvalve-aj41.3", "burpvalve-aj41.4"} {
		if !containsString(record.BeadIDs, beadID) {
			t.Fatalf("lane bead id %q missing from query bead_ids: %#v", beadID, record.BeadIDs)
		}
	}

	human, stderr, err := executeBurpvalveCommand("attestations", "show", "--root", root, "--color", "never", "laneabcdef")
	if err != nil {
		t.Fatalf("human show failed: %v\nstdout=%s\nstderr=%s", err, human, stderr)
	}
	for _, needle := range []string{"Lane:", "declared-lane-aj41", "Lane auth:", "ORCH-2026-07-08"} {
		if !strings.Contains(human, needle) {
			t.Fatalf("human show missing %q:\n%s", needle, human)
		}
	}
}

func TestAttestationsShowErrorsAndRobotHelp(t *testing.T) {
	root := t.TempDir()
	created := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	writeAttestationQueryFixture(t, root, "backpressure/attestations/abc111.json", attestationQueryArtifactFixture(attestations.ArtifactPassing, "abc111", "br-one", created))
	writeAttestationQueryFixture(t, root, "backpressure/attestations/abc222.json", attestationQueryArtifactFixture(attestations.ArtifactPassing, "abc222", "br-two", created.Add(time.Minute)))
	writeCmdTestFile(t, filepath.Join(root, "log/backpressure/failed/bad.json"), `not json`)

	stdout, _, err := executeBurpvalveCommand("attestations", "show", "--root", root, "--json", "missing")
	if err == nil || !strings.Contains(stdout, `"code": "not_found"`) {
		t.Fatalf("missing show should return JSON not_found, err=%v stdout=%s", err, stdout)
	}
	stdout, _, err = executeBurpvalveCommand("attestations", "show", "--root", root, "--json", "abc")
	if err == nil || !strings.Contains(stdout, `"code": "ambiguous_ref"`) {
		t.Fatalf("ambiguous show should return JSON ambiguous_ref, err=%v stdout=%s", err, stdout)
	}
	stdout, _, err = executeBurpvalveCommand("attestations", "show", "--root", root, "--json", "log/backpressure/failed/bad.json")
	if err == nil || !strings.Contains(stdout, `"code": "malformed"`) {
		t.Fatalf("malformed show should return JSON malformed, err=%v stdout=%s", err, stdout)
	}

	help, stderr, err := executeBurpvalveCommand("--robots", "attestations", "list", "-h")
	if err != nil {
		t.Fatalf("robot attestations help failed: %v\nstdout=%s\nstderr=%s", err, help, stderr)
	}
	if !strings.Contains(help, "condition_verdicts") || !strings.Contains(help, "parse_warnings") || !strings.Contains(help, "instead of scraping human tables") {
		t.Fatalf("robot help should document attestation query schema:\n%s", help)
	}
}

func writeAttestationQueryFixture(t *testing.T, root string, rel string, artifact attestations.Artifact) {
	t.Helper()
	body, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeCmdTestFile(t, filepath.Join(root, filepath.FromSlash(rel)), string(body)+"\n")
}

func attestationQueryArtifactFixture(kind attestations.ArtifactKind, payloadHash string, featureID string, created time.Time) attestations.Artifact {
	status := attestations.VerdictPass
	if kind == attestations.ArtifactBlocked {
		status = attestations.VerdictUnknown
	}
	return attestations.Artifact{
		SchemaVersion:       1,
		Tool:                attestations.ToolName,
		ToolVersion:         attestations.ToolVersion,
		ArtifactKind:        kind,
		StagedPayloadHash:   payloadHash,
		ManifestHash:        "manifest-" + payloadHash,
		ConditionOrder:      []string{"dry"},
		GeneratedBy:         attestations.Generator{Agent: "codex", Model: "test"},
		GitHeadBeforeCommit: "HEAD",
		CreatedAt:           created,
		Feature: attestations.Feature{
			ID:         featureID,
			Kind:       "feature",
			Name:       featureID,
			SourceBead: featureID,
		},
		Atomicity: attestations.Atomicity{OneFeatureOrFix: kind == attestations.ArtifactPassing, Message: "fixture"},
		Conditions: []attestations.Condition{{
			ConditionID:       "dry",
			ConditionFile:     "backpressure/dry.md",
			ConditionFileHash: "sha256:dry",
			VerifierPolicy:    attestations.VerifierPolicyIndependentRequired,
			Verifier: attestations.Verifier{
				Kind:            attestations.VerifierIndependentSubagent,
				Agent:           "verifier",
				Model:           "test",
				Runtime:         "go-test",
				SeparateContext: true,
			},
			SubagentConfirmed: true,
			SubagentModel:     "test",
			Verdict:           status,
			Message:           "fixture condition",
			Evidence:          []string{"fixture evidence"},
			NextAction:        "rerun after fixing blocker",
			Timestamp:         created,
		}},
	}
}

func findAttestationQueryRecord(records []attestationListRecord, status string) *attestationListRecord {
	for i := range records {
		if records[i].Status == status {
			return &records[i]
		}
	}
	return nil
}
