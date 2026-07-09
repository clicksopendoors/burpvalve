package backpressure

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"burpvalve/internal/attestations"
)

const expectedVerifierAuthorizationText = "Standing verifier authorization permits spawning read-only verifier subagents for backpressure checks when recorded in defaults.verifier. Authorization is policy metadata only and is never per-cell verification evidence. Do not fabricate subagent confirmation."

func TestBuildVerifierPromptsIncludesBoundContract(t *testing.T) {
	root := fixtureVerifierPromptProject(t)
	writeFile(t, root, ".burpvalve.json", `{"schema_version":1,"defaults":{"verifier":{"authorized":true,"authorized_at":"2026-07-02T12:00:00Z","authorization_scope":"repo:`+root+`","spawn_method":"native","condition_models":{"missing-condition":"gpt-x"}}}}`)
	staged := &fakeStagedEntries{
		entries: []StagedPayloadFile{
			{Path: "backpressure/attestations/README.md", Status: "modified", GitStatus: "M"},
			{Path: "backpressure/attestations/hash.json", Status: "added", GitStatus: "A"},
			{Path: "src/new.go", OldPath: "src/old.go", Status: "renamed", GitStatus: "R100"},
			{Path: "src/delete.go", Status: "deleted", GitStatus: "D"},
			{Path: "src/edit.go", Status: "modified", GitStatus: "M"},
		},
		content: map[string]string{
			"backpressure/attestations/README.md": "tracked passing attestation docs\n",
			"backpressure/attestations/hash.json": "{}\n",
			"src/new.go":                          "package src\nconst New = true\n",
			"src/edit.go":                         "package src\nconst Edit = true\n",
		},
	}

	set, err := BuildVerifierPrompts(context.Background(), VerifierPromptOptions{
		Root:      root,
		Feature:   "br-123",
		Condition: "dry",
		Profile:   "native",
		Staged:    staged,
	})
	if err != nil {
		t.Fatal(err)
	}
	if set.ManifestHash == "" || set.StagedPayloadHash == "" {
		t.Fatalf("missing binding hashes: %#v", set)
	}
	packet := set.Packets[0]
	if packet.ConditionID != "dry" || packet.ConditionFileHash == "" || packet.ManifestHash != set.ManifestHash || packet.StagedPayloadHash != set.StagedPayloadHash {
		t.Fatalf("wrong packet bindings: %#v", packet)
	}
	if packet.VerifierPolicy != attestations.VerifierPolicyCIAllowed {
		t.Fatalf("verifier policy = %q", packet.VerifierPolicy)
	}
	if VerifierAuthorizationText != expectedVerifierAuthorizationText {
		t.Fatalf("approved verifier authorization text changed:\n%s", VerifierAuthorizationText)
	}
	if !packet.Authorization.Recorded || !packet.Authorization.Authorized || packet.Authorization.AuthorizationScope != "repo:"+root {
		t.Fatalf("recorded authorization missing: %#v", packet.Authorization)
	}
	if len(set.Warnings) != 1 || !strings.Contains(set.Warnings[0], "missing-condition") {
		t.Fatalf("condition model warning missing: %#v", set.Warnings)
	}
	for _, needle := range []string{
		expectedVerifierAuthorizationText,
		"Staged payload hash:",
		"Manifest hash:",
		"Condition file hash:",
		"Hash-included staged payload:",
		"- modified backpressure/attestations/README.md",
		"- renamed src/new.go (from src/old.go)",
		"Hash-excluded generated staged paths:",
		"- added backpressure/attestations/hash.json (generated/hash-excluded)",
		"Condition contents:",
		"# DRY",
		"Hash reproduction:",
		"after sorting by path, old_path, and normalized status",
		"raw git status",
		"git diff --cached --binary | sha256sum",
		"not_applicable means the staged payload contains no surface governed by this condition",
		"pass means the staged payload contains a governed surface",
		"Response JSON schema:",
		"supplemental_verifiers",
		"adjudication",
		"Submit command:",
		"burpvalve verifier submit",
		"Return JSON matching the inline schema",
	} {
		if !strings.Contains(packet.Prompt, needle) {
			t.Fatalf("prompt missing %q:\n%s", needle, packet.Prompt)
		}
	}
	if len(packet.HashExcludedStagedPayload) != 1 || packet.HashExcludedStagedPayload[0].Path != "backpressure/attestations/hash.json" {
		t.Fatalf("hash-excluded staged paths missing: %#v", packet.HashExcludedStagedPayload)
	}
	if packet.ResponseSchema.ConditionID != "dry" ||
		packet.ResponseSchema.ConditionFile != "backpressure/dry.md" ||
		packet.ResponseSchema.VerifierPolicy != attestations.VerifierPolicyCIAllowed ||
		packet.ResponseSchema.Verifier.Kind != attestations.VerifierUnknown ||
		!packet.ResponseSchema.Verifier.SeparateContext ||
		packet.ResponseSchema.Verdict != attestations.VerdictUnknown {
		t.Fatalf("response schema does not match commit response fields: %#v", packet.ResponseSchema)
	}
	assertPayloadFile(t, set.StagedPayload, "src/new.go", "renamed", "src/old.go")
	assertPayloadFile(t, set.StagedPayload, "backpressure/attestations/README.md", "modified", "")
	assertPayloadFile(t, set.HashExcludedStagedPayload, "backpressure/attestations/hash.json", "added", "")
	assertVerifierPromptStagedPayloadUnion(t, set.StagedPayload, set.HashExcludedStagedPayload, []string{
		"backpressure/attestations/README.md",
		"backpressure/attestations/hash.json",
		"src/delete.go",
		"src/edit.go",
		"src/new.go",
	})
	for _, detail := range set.StagedPayloadDetails {
		if detail.Path == "backpressure/attestations/README.md" && (!detail.HashIncluded || detail.Generated) {
			t.Fatalf("README detail should be hash-included and not generated: %#v", detail)
		}
	}
}

func TestBuildVerifierPromptsProfilesAndAuthorizationWarning(t *testing.T) {
	root := fixtureVerifierPromptProject(t)
	staged := fakeStaged{paths: []string{"src/app.go"}, content: map[string]string{"src/app.go": "package src\n"}}

	set, err := BuildVerifierPrompts(context.Background(), VerifierPromptOptions{Root: root, Feature: "br-ntm", Profile: "NTM", Staged: staged})
	if err != nil {
		t.Fatal(err)
	}
	if set.Profile != "ntm" || len(set.Packets) != 2 {
		t.Fatalf("profile/packet count = %q/%d", set.Profile, len(set.Packets))
	}
	if !strings.Contains(strings.Join(set.Notes, "\n"), "docs/ntm-bridge.md") {
		t.Fatalf("ntm profile should mention bridge policy: %#v", set.Notes)
	}
	if set.Authorization.Recorded || set.Authorization.Authorized {
		t.Fatalf("authorization should not be recorded: %#v", set.Authorization)
	}
	for _, needle := range []string{"Verifier authorization is not recorded as granted", "burpvalve config init", "Authorization is never per-cell evidence"} {
		if !strings.Contains(set.Packets[0].Prompt, needle) {
			t.Fatalf("authorization warning missing %q:\n%s", needle, set.Packets[0].Prompt)
		}
	}

	manual, err := BuildVerifierPrompts(context.Background(), VerifierPromptOptions{Root: root, Feature: "br-manual", Condition: "scope-control", Profile: "manual", Staged: staged})
	if err != nil {
		t.Fatal(err)
	}
	if manual.Profile != "manual" || len(manual.Packets) != 1 || manual.Packets[0].ConditionID != "scope-control" {
		t.Fatalf("condition filtering failed: %#v", manual)
	}
}

func TestBuildVerifierPromptsRendersLaneBinding(t *testing.T) {
	root := fixtureVerifierPromptProject(t)
	staged := fakeStaged{paths: []string{"src/app.go"}, content: map[string]string{"src/app.go": "package src\n"}}

	set, err := BuildVerifierPrompts(context.Background(), VerifierPromptOptions{
		Root:      root,
		Feature:   "declared-lane-aj41",
		Condition: "scope-control",
		Profile:   "native",
		Lane: LaneOptions{
			Enabled:          true,
			LaneID:           "declared-lane-aj41",
			BeadIDs:          []string{"burpvalve-aj41.3", "burpvalve-aj41.4"},
			Rationale:        "same orchestrator-authorized lane",
			AuthorizationRef: "Agent Mail 4000",
			AuthorizedBy:     "BronzeDeer",
		},
		Staged: staged,
	})
	if err != nil {
		t.Fatal(err)
	}
	if set.LaneBinding == nil || set.LaneBinding.LaneID != "declared-lane-aj41" {
		t.Fatalf("set lane binding missing: %#v", set.LaneBinding)
	}
	if set.Feature.ID != "declared-lane-aj41" {
		t.Fatalf("feature id = %q", set.Feature.ID)
	}
	packet := set.Packets[0]
	if packet.LaneBinding == nil || packet.LaneBinding.AuthorizationRef != "Agent Mail 4000" {
		t.Fatalf("packet lane binding missing: %#v", packet.LaneBinding)
	}
	if got := strings.Join(packet.LaneBinding.BeadIDs, ","); got != "burpvalve-aj41.3,burpvalve-aj41.4" {
		t.Fatalf("lane bead ids = %q", got)
	}
	for _, needle := range []string{
		`"lane_binding"`,
		`"lane_id": "declared-lane-aj41"`,
		`"bead_ids": [`,
		`"burpvalve-aj41.3"`,
		`"authorization_ref": "Agent Mail 4000"`,
	} {
		if !strings.Contains(packet.ResponseSchemaJSON, needle) {
			t.Fatalf("lane response schema missing %q:\n%s", needle, packet.ResponseSchemaJSON)
		}
	}
	if packet.ConditionFileHashes["dry"] == "" || packet.ConditionFileHashes["scope-control"] == "" {
		t.Fatalf("packet missing full condition hash map: %#v", packet.ConditionFileHashes)
	}
	for _, needle := range []string{
		"Lane binding:",
		"Lane id: declared-lane-aj41",
		"Bead ids: burpvalve-aj41.3, burpvalve-aj41.4",
		"Rationale: same orchestrator-authorized lane",
		"Authorized by: BronzeDeer",
		"Authorization ref: Agent Mail 4000",
		"judge whether the staged payload stays inside this declared lane boundary",
		"All condition file hashes:",
		"dry: sha256:",
		"scope-control: sha256:",
	} {
		if !strings.Contains(packet.Prompt, needle) {
			t.Fatalf("lane prompt missing %q:\n%s", needle, packet.Prompt)
		}
	}
}

func TestBuildVerifierPromptsAntiRewardHackingScopeText(t *testing.T) {
	root := fixtureVerifierPromptProject(t)
	writeFile(t, root, ManifestPath, `conditions:
  - id: anti-reward-hacking
    path: backpressure/anti-reward-hacking.md
    enabled: true
`)
	writeFile(t, root, "backpressure/anti-reward-hacking.md", "# Anti reward hacking\n")
	set, err := BuildVerifierPrompts(context.Background(), VerifierPromptOptions{
		Root:      root,
		Feature:   "br-arh",
		Condition: "anti-reward-hacking",
		Staged:    fakeStaged{paths: []string{"docs/decision.md"}, content: map[string]string{"docs/decision.md": "decision text\n"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"staged payload's consistency with its own claims", "not merely the absence of shortcut-authorization language", "land-04", "contradicted its own standard"} {
		if !strings.Contains(set.Packets[0].SuccessCriteria, needle) {
			t.Fatalf("anti-reward-hacking text missing %q:\n%s", needle, set.Packets[0].SuccessCriteria)
		}
	}
	if strings.Contains(set.Packets[0].SuccessCriteria, "->") {
		t.Fatalf("anti-reward-hacking rationale should avoid bare arrow-chain prose:\n%s", set.Packets[0].SuccessCriteria)
	}
}

func TestBuildVerifierPromptsHashContractMatchesStagedPayload(t *testing.T) {
	root := fixtureVerifierPromptProject(t)
	runGit(t, root, "init", "-q")
	writeFile(t, root, "src/keep.go", "package src\nconst Keep = 1\n")
	writeFile(t, root, "src/delete.go", "package src\nconst Delete = 1\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "config", "user.email", "michael-bltzr@users.noreply.github.com")
	runGit(t, root, "config", "user.name", "Test User")
	runGit(t, root, "commit", "-q", "--no-verify", "-m", "baseline")

	writeFile(t, root, "src/keep.go", "package src\nconst Keep = 2\n")
	if err := os.Remove(filepath.Join(root, "src/delete.go")); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "-A")

	payload, err := HashStagedPayload(context.Background(), root, GitStagedReader{})
	if err != nil {
		t.Fatal(err)
	}
	set, err := BuildVerifierPrompts(context.Background(), VerifierPromptOptions{Root: root, Feature: "br-hash", Condition: "dry"})
	if err != nil {
		t.Fatal(err)
	}
	if set.StagedPayloadHash != payload.Hash || set.Packets[0].StagedPayloadHash != payload.Hash {
		t.Fatalf("prompt hash does not match HashStagedPayload: prompt=%s packet=%s want=%s", set.StagedPayloadHash, set.Packets[0].StagedPayloadHash, payload.Hash)
	}
	for _, needle := range []string{"src/keep.go", "src/delete.go", "Deleted files contribute only their metadata", "staged content size"} {
		if !strings.Contains(set.Packets[0].Prompt, needle) {
			t.Fatalf("hash contract prompt missing %q:\n%s", needle, set.Packets[0].Prompt)
		}
	}
}

func TestBuildVerifierPromptsErrorsAreActionable(t *testing.T) {
	root := fixtureVerifierPromptProject(t)
	_, err := BuildVerifierPrompts(context.Background(), VerifierPromptOptions{Root: root, Feature: "br-123", Profile: "swarm", Staged: fakeStaged{paths: []string{"src/app.go"}, content: map[string]string{"src/app.go": "package src\n"}}})
	if err == nil || !strings.Contains(err.Error(), "expected native, ntm, hermes, or manual") {
		t.Fatalf("unsupported profile error = %v", err)
	}
	_, err = BuildVerifierPrompts(context.Background(), VerifierPromptOptions{Root: root, Feature: "br-123", Staged: fakeStaged{}})
	if err == nil || !strings.Contains(err.Error(), "no staged payload") {
		t.Fatalf("no staged payload error = %v", err)
	}
	_, err = BuildVerifierPrompts(context.Background(), VerifierPromptOptions{
		Root:   root,
		Staged: fakeStaged{paths: []string{"cmd/app/main.go", "internal/app/app.go"}, content: map[string]string{"cmd/app/main.go": "package main\n", "internal/app/app.go": "package app\n"}},
	})
	if err == nil || !strings.Contains(err.Error(), "multiple diff clusters") {
		t.Fatalf("missing feature/ambiguous payload error = %v", err)
	}
	_, err = BuildVerifierPrompts(context.Background(), VerifierPromptOptions{Root: root, Feature: "br-123", Condition: "missing", Staged: fakeStaged{paths: []string{"src/app.go"}, content: map[string]string{"src/app.go": "package src\n"}}})
	if err == nil || !strings.Contains(err.Error(), `condition "missing" not found`) {
		t.Fatalf("missing condition error = %v", err)
	}
}

func fixtureVerifierPromptProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, ManifestPath, `conditions:
  - id: dry
    path: backpressure/dry.md
    enabled: true
    verifier_policy: ci_allowed
  - id: scope-control
    path: backpressure/scope-control.md
    enabled: true
    verifier_policy: independent_required
  - id: disabled
    path: backpressure/disabled.md
    enabled: false
`)
	writeFile(t, root, "backpressure/dry.md", "# DRY\n")
	writeFile(t, root, "backpressure/scope-control.md", "# Scope\n")
	writeFile(t, root, "backpressure/disabled.md", "# Disabled\n")
	return root
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func assertVerifierPromptStagedPayloadUnion(t *testing.T, included []StagedPayloadFile, excluded []StagedPayloadFile, want []string) {
	t.Helper()
	seen := map[string]int{}
	for _, file := range included {
		seen[file.Path]++
	}
	for _, file := range excluded {
		seen[file.Path]++
	}
	for _, path := range want {
		if seen[path] != 1 {
			t.Fatalf("staged path %q appears %d times across included/excluded payloads; included=%#v excluded=%#v", path, seen[path], included, excluded)
		}
		delete(seen, path)
	}
	if len(seen) != 0 {
		t.Fatalf("unexpected staged paths across included/excluded payloads: %#v", seen)
	}
}
