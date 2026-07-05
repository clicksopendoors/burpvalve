package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"burpvalve/internal/backpressure"
)

func TestVerifierPromptsCommandJSONAndHumanOutput(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)
	writeCLIFile(t, target, "backpressure/attestations/README.md", "tracked attestation docs\n")
	writeCLIFile(t, target, "backpressure/attestations/generated.json", "{}\n")
	run(t, target, "git", "add", "backpressure/attestations/README.md", "backpressure/attestations/generated.json")

	stdout, stderr, err := runBurpvalve(t, repoRoot, "verifier", "prompts", "--root", target, "--feature", "br-123", "--condition", "dry", "--json")
	if err != nil {
		t.Fatalf("verifier prompts json failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if len(stderr) != 0 {
		t.Fatalf("json verifier prompts should not write stderr:\n%s", stderr)
	}
	var set backpressure.VerifierPromptSet
	if err := json.Unmarshal(stdout, &set); err != nil {
		t.Fatalf("decode verifier prompt set: %v\n%s", err, stdout)
	}
	if set.Command != "verifier prompts" || set.Profile != "native" || set.Feature.ID != "br-123" {
		t.Fatalf("unexpected prompt metadata: %#v", set)
	}
	if len(set.Packets) != 1 || set.Packets[0].ConditionID != "dry" {
		t.Fatalf("condition filter failed: %#v", set.Packets)
	}
	if !strings.Contains(set.Packets[0].Authorization.Message, "Verifier authorization is not recorded as granted") ||
		!strings.Contains(set.Packets[0].Authorization.Message, "never per-cell evidence") {
		t.Fatalf("authorization warning missing: %#v", set.Packets[0].Authorization)
	}
	if set.Packets[0].ResponseSchema.ConditionID != "dry" || set.Packets[0].ResponseSchema.Verifier.Kind != "unknown" {
		t.Fatalf("response schema missing verifier metadata: %#v", set.Packets[0].ResponseSchema)
	}
	if set.Packets[0].StagedPayloadHash == "" || set.Packets[0].ManifestHash == "" || set.Packets[0].ConditionFileHash == "" {
		t.Fatalf("binding hashes missing: %#v", set.Packets[0])
	}
	if !payloadHasPath(set.Packets[0].StagedPayload, "backpressure/attestations/README.md") {
		t.Fatalf("tracked README under generated dir should be hash-included: %#v", set.Packets[0].StagedPayload)
	}
	if !payloadHasPath(set.Packets[0].HashExcludedStagedPayload, "backpressure/attestations/generated.json") ||
		payloadHasPath(set.Packets[0].HashExcludedStagedPayload, "backpressure/attestations/README.md") {
		t.Fatalf("generated exclusion list wrong: %#v", set.Packets[0].HashExcludedStagedPayload)
	}
	if !strings.Contains(set.Packets[0].ResponseSchemaJSON, "supplemental_verifiers") ||
		!strings.Contains(set.Packets[0].ResponseSchemaJSON, "adjudication") {
		t.Fatalf("response schema JSON missing shared-cell fields:\n%s", set.Packets[0].ResponseSchemaJSON)
	}
	if !strings.Contains(set.Packets[0].HashReproduction, "HashStagedPayload") ||
		!strings.Contains(set.Packets[0].HashReproduction, "git diff --cached --binary | sha256sum") {
		t.Fatalf("hash reproduction contract missing:\n%s", set.Packets[0].HashReproduction)
	}

	human, stderr, err := runBurpvalve(t, repoRoot, "verifier", "prompts", "--root", target, "--feature", "br-123", "--profile", "manual", "--color", "never")
	if err != nil {
		t.Fatalf("verifier prompts human failed: %v\nstdout=%s\nstderr=%s", err, human, stderr)
	}
	if len(stderr) != 0 {
		t.Fatalf("human verifier prompts should not write stderr:\n%s", stderr)
	}
	for _, needle := range []string{
		"Burpvalve verifier prompts",
		"Profile:",
		"manual",
		"Packet:",
		"Condition:",
		"Read-only expectation:",
		"Verifier authorization is not recorded as granted",
		"Staged payload hash:",
		"Manifest hash:",
		"Condition file hash:",
		"Hash reproduction:",
		"Response JSON schema:",
		"supplemental_verifiers",
		"adjudication",
		"Submit command:",
		"burpvalve verifier submit",
		"Do not fabricate subagent confirmation.",
	} {
		if !strings.Contains(string(human), needle) {
			t.Fatalf("human verifier prompts missing %q:\n%s", needle, human)
		}
	}
}

func payloadHasPath(files []backpressure.StagedPayloadFile, path string) bool {
	for _, file := range files {
		if file.Path == path {
			return true
		}
	}
	return false
}

func TestVerifierBeginCommandWritesBoundResponses(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)
	writeCLIFile(t, target, "cmd/app/main.go", "package main\n")
	run(t, target, "git", "add", "cmd/app/main.go")

	stdout, stderr, err := runBurpvalve(t, repoRoot, "verifier", "begin", "--root", target, "--feature", "br-123", "--one-feature", "--atomicity-message", "Staged changes map only to br-123.", "--json")
	if err != nil {
		t.Fatalf("verifier begin failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if len(stderr) != 0 {
		t.Fatalf("verifier begin json should not write stderr:\n%s", stderr)
	}
	var result backpressure.BeginResponsesResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode verifier begin result: %v\n%s", err, stdout)
	}
	if result.Command != "verifier begin" || result.Status != backpressure.StatusResponsesWritten || result.ResponsesPath == "" {
		t.Fatalf("unexpected begin result: %#v", result)
	}
	body, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(result.ResponsesPath)))
	if err != nil {
		t.Fatalf("responses file not written: %v", err)
	}
	var responses backpressure.Responses
	if err := json.Unmarshal(body, &responses); err != nil {
		t.Fatalf("decode responses file: %v\n%s", err, body)
	}
	if !responses.Atomicity.OneFeatureOrFix || responses.Atomicity.Message != "Staged changes map only to br-123." {
		t.Fatalf("atomicity not preserved: %#v", responses.Atomicity)
	}
	if responses.Binding.StagedPayloadHash != result.StagedPayloadHash || responses.Binding.ManifestHash != result.ManifestHash {
		t.Fatalf("binding = %#v, result hashes = %s/%s", responses.Binding, result.StagedPayloadHash, result.ManifestHash)
	}
	if len(responses.Conditions) == 0 || len(responses.Binding.Conditions) != len(responses.Conditions) {
		t.Fatalf("response conditions/binding mismatch: %#v", responses)
	}
	for i, condition := range responses.Conditions {
		if condition.Verdict != "unknown" {
			t.Fatalf("condition %d verdict = %q, want unknown", i, condition.Verdict)
		}
		if responses.Binding.Conditions[i].ConditionID != condition.ConditionID || responses.Binding.Conditions[i].ConditionFileHash == "" {
			t.Fatalf("condition %d binding = %#v response=%#v", i, responses.Binding.Conditions[i], condition)
		}
	}
}

func TestVerifierBeginCommandRequiresAtomicity(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)
	writeCLIFile(t, target, "cmd/app/main.go", "package main\n")
	run(t, target, "git", "add", "cmd/app/main.go")

	stdout, stderr, err := runBurpvalve(t, repoRoot, "verifier", "begin", "--root", target, "--feature", "br-123", "--json")
	if err == nil {
		t.Fatalf("verifier begin without atomicity should fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if !strings.Contains(string(stderr), "atomicity not confirmed") {
		t.Fatalf("stderr missing atomicity blocker:\n%s", stderr)
	}
	var result backpressure.BeginResponsesResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode blocked begin result: %v\n%s", err, stdout)
	}
	if result.Status != backpressure.StatusBlocked || len(result.NextSteps) == 0 || result.ResponsesPath == "" {
		t.Fatalf("blocked result missing recovery/path: %#v", result)
	}
}

func TestVerifierDoctorCommandJSONAndHumanOutput(t *testing.T) {
	target := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeCLIFile(t, target, ".claude/settings.json", `{"subagents":{"max_subagents":4,"max_depth":2}}`)
	writeCLIFile(t, target, ".codex/config.toml", "max_parallel_verifiers = 3\n")

	stdout, stderr, err := executeBurpvalveCommand("verifier", "doctor", "--root", target, "--json")
	if err != nil {
		t.Fatalf("verifier doctor json failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if len(stderr) != 0 {
		t.Fatalf("verifier doctor json should not write stderr:\n%s", stderr)
	}
	var result backpressure.VerifierDoctorResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode verifier doctor result: %v\n%s", err, stdout)
	}
	if result.Command != "verifier doctor" || !result.ReportOnly || len(result.NextSteps) == 0 {
		t.Fatalf("doctor result contract missing: %#v", result)
	}
	if !doctorCLIRuntime(result, "claude-code").Supported || !doctorCLIRuntime(result, "codex").Supported {
		t.Fatalf("doctor did not report supported configs: %#v", result.Checks)
	}

	human, stderr, err := executeBurpvalveCommand("verifier", "doctor", "--root", target, "--color", "never")
	if err != nil {
		t.Fatalf("verifier doctor human failed: %v\nstdout=%s\nstderr=%s", err, human, stderr)
	}
	if len(stderr) != 0 {
		t.Fatalf("verifier doctor human should not write stderr:\n%s", stderr)
	}
	for _, needle := range []string{
		"Burpvalve verifier doctor",
		"Report only:",
		"true",
		"claude-code",
		"codex",
		"ntm",
		"subagent_limit:",
		"depth_limit:",
		"without writing files",
		"per-cell verifier evidence",
	} {
		if !strings.Contains(string(human), needle) {
			t.Fatalf("human verifier doctor missing %q:\n%s", needle, human)
		}
	}
}

func TestVerifierDoctorCommandReportsUnsupportedMalformedConfig(t *testing.T) {
	target := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLIFile(t, target, ".claude/settings.json", `{"max_subagents":`)

	stdout, stderr, err := executeBurpvalveCommand("verifier", "doctor", "--root", target, "--json")
	if err != nil {
		t.Fatalf("verifier doctor malformed config should report, not fail: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var result backpressure.VerifierDoctorResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode verifier doctor result: %v\n%s", err, stdout)
	}
	claude := doctorCLIRuntime(result, "claude-code")
	if claude.Supported {
		t.Fatalf("malformed config should have supported=false: %#v", claude)
	}
	if len(claude.Paths) == 0 || claude.Paths[0].Supported || !strings.Contains(claude.Paths[0].Message, "unsupported JSON format") {
		t.Fatalf("malformed path should be unsupported with message: %#v", claude.Paths)
	}
	for _, step := range result.NextSteps {
		if strings.Contains(strings.ToLower(step), "set ") || strings.Contains(strings.ToLower(step), "write ") {
			t.Fatalf("doctor should not invent exact unsupported edits: %#v", result.NextSteps)
		}
	}
}

func TestVerifierSubmitCommandUpdatesBoundResponses(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target, begin := beginVerifierSubmitFixture(t, repoRoot)
	dryHash := conditionHash(t, begin, "dry")
	body := verifierSubmitJSON(t, begin, "dry", dryHash, map[string]any{
		"condition_id":       "dry",
		"condition_file":     "backpressure/dry.md",
		"subagent_confirmed": true,
		"subagent_model":     "gpt-5",
		"verifier": map[string]any{
			"kind":             "independent_subagent",
			"agent":            "DryVerifier",
			"model":            "gpt-5",
			"runtime":          "codex-cli",
			"separate_context": true,
		},
		"verdict":     "pass",
		"message":     "Dry passed.",
		"evidence":    []string{"DryVerifier inspected the staged payload."},
		"next_action": "",
	})

	stdout, stderr, err := runBurpvalveWithInput(t, repoRoot, body, "verifier", "submit",
		"--root", target,
		"--feature", "br-123",
		"--condition", "dry",
		"--staged-payload-hash", begin.StagedPayloadHash,
		"--manifest-hash", begin.ManifestHash,
		"--condition-file-hash", dryHash,
		"--json",
	)
	if err != nil {
		t.Fatalf("verifier submit failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if len(stderr) != 0 {
		t.Fatalf("submit json wrote stderr:\n%s", stderr)
	}
	var result backpressure.SubmitVerifierResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode submit result: %v\n%s", err, stdout)
	}
	if result.Status != backpressure.StatusResponsesUpdated || result.ConditionID != "dry" {
		t.Fatalf("unexpected submit result: %#v", result)
	}
	responses := readCLIResponses(t, filepath.Join(target, filepath.FromSlash(begin.ResponsesPath)))
	dry := cliResponseCondition(t, responses, "dry")
	if dry.Verdict != "pass" || dry.Verifier.Agent != "DryVerifier" || len(dry.Evidence) == 0 {
		t.Fatalf("dry response not written: %#v", dry)
	}
	if scope := cliResponseCondition(t, responses, "scope-control"); scope.Verdict != "unknown" {
		t.Fatalf("unrelated condition changed: %#v", scope)
	}
}

func doctorCLIRuntime(result backpressure.VerifierDoctorResult, runtime string) backpressure.VerifierDoctorRuntimeCheck {
	for _, check := range result.Checks {
		if check.Runtime == runtime {
			return check
		}
	}
	return backpressure.VerifierDoctorRuntimeCheck{}
}

func TestVerifierSubmitCommandRejectsMissingEvidence(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target, begin := beginVerifierSubmitFixture(t, repoRoot)
	dryHash := conditionHash(t, begin, "dry")
	body := verifierSubmitJSON(t, begin, "dry", dryHash, map[string]any{
		"condition_id":       "dry",
		"condition_file":     "backpressure/dry.md",
		"subagent_confirmed": true,
		"subagent_model":     "gpt-5",
		"verifier": map[string]any{
			"kind":             "independent_subagent",
			"agent":            "DryVerifier",
			"model":            "gpt-5",
			"runtime":          "codex-cli",
			"separate_context": true,
		},
		"verdict":  "pass",
		"message":  "Dry passed.",
		"evidence": []string{},
	})

	stdout, stderr, err := runBurpvalveWithInput(t, repoRoot, body, "verifier", "submit",
		"--root", target,
		"--feature", "br-123",
		"--condition", "dry",
		"--staged-payload-hash", begin.StagedPayloadHash,
		"--manifest-hash", begin.ManifestHash,
		"--condition-file-hash", dryHash,
		"--json",
	)
	if err == nil {
		t.Fatalf("submit without evidence should fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if !strings.Contains(string(stderr), "without evidence") {
		t.Fatalf("stderr missing evidence blocker:\n%s", stderr)
	}
}

func TestVerifierSubmitCommandStoresTranscript(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target, begin := beginVerifierSubmitFixture(t, repoRoot)
	writeCLIFile(t, target, ".burpvalve.json", `{"schema_version":1,"defaults":{"verifier":{"transcript_dir":"log/verifier-transcripts","transcripts":"summary"}}}`)
	writeCLIFile(t, target, "transcript.md", strings.Repeat("line\n", 25))
	dryHash := conditionHash(t, begin, "dry")
	body := verifierSubmitJSON(t, begin, "dry", dryHash, map[string]any{
		"condition_id":       "dry",
		"condition_file":     "backpressure/dry.md",
		"subagent_confirmed": true,
		"subagent_model":     "gpt-5",
		"verifier": map[string]any{
			"kind":             "independent_subagent",
			"agent":            "DryVerifier",
			"model":            "gpt-5",
			"runtime":          "codex-cli",
			"separate_context": true,
		},
		"verdict":  "pass",
		"message":  "Dry passed.",
		"evidence": []string{"DryVerifier inspected the staged payload."},
	})
	stdout, stderr, err := runBurpvalveWithInput(t, repoRoot, body, "verifier", "submit",
		"--root", target,
		"--feature", "br-123",
		"--condition", "dry",
		"--staged-payload-hash", begin.StagedPayloadHash,
		"--manifest-hash", begin.ManifestHash,
		"--condition-file-hash", dryHash,
		"--transcript", "transcript.md",
		"--json",
	)
	if err != nil {
		t.Fatalf("submit transcript failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var result backpressure.SubmitVerifierResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode submit result: %v\n%s", err, stdout)
	}
	if result.TranscriptRef == "" || !strings.HasPrefix(result.TranscriptRef, "log/verifier-transcripts/") {
		t.Fatalf("transcript ref = %q", result.TranscriptRef)
	}
	transcript, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(result.TranscriptRef)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(transcript), "transcript summary truncated") {
		t.Fatalf("transcript not summarized:\n%s", transcript)
	}
}

func TestVerifierPromptsCommandErrorsAndRobotHelp(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)

	stdout, stderr, err := runBurpvalve(t, repoRoot, "verifier", "prompts", "--root", target, "--feature", "br-123", "--profile", "SWARM", "--json")
	if err == nil {
		t.Fatalf("unsupported profile should fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if !strings.Contains(string(stderr), "expected native, ntm, hermes, or manual") {
		t.Fatalf("stderr missing supported profiles:\n%s", stderr)
	}

	help, helpStderr, err := executeBurpvalveCommand("--robots", "verifier", "prompts", "-h")
	if err != nil {
		t.Fatalf("robot verifier help failed: %v\nstdout=%s\nstderr=%s", err, help, helpStderr)
	}
	if helpStderr != "" {
		t.Fatalf("robot help wrote stderr: %s", helpStderr)
	}
	for _, needle := range []string{
		`"command": "burpvalve verifier prompts"`,
		`"profile"`,
		`"response_schema"`,
		`"response_schema_json"`,
		`"submit_command"`,
		`"hash_reproduction"`,
		`"hash_excluded_staged_payload"`,
		`"staged_payload"`,
		"binding hashes",
		"never per-cell evidence",
		"does not spawn subagents",
		"Do not fabricate subagent confirmation",
	} {
		if !strings.Contains(help, needle) {
			t.Fatalf("robot verifier help missing %q:\n%s", needle, help)
		}
	}
}

func TestVerifierBeginRobotHelp(t *testing.T) {
	help, helpStderr, err := executeBurpvalveCommand("--robots", "verifier", "begin", "-h")
	if err != nil {
		t.Fatalf("robot verifier begin help failed: %v\nstdout=%s\nstderr=%s", err, help, helpStderr)
	}
	if helpStderr != "" {
		t.Fatalf("robot begin help wrote stderr: %s", helpStderr)
	}
	for _, needle := range []string{
		`"command": "burpvalve verifier begin"`,
		`"one-feature"`,
		`"atomicity-message"`,
		`"responses_path"`,
		`"staged_payload_hash"`,
		`"response_file_schema"`,
		"must not rewrite atomicity",
	} {
		if !strings.Contains(help, needle) {
			t.Fatalf("robot verifier begin help missing %q:\n%s", needle, help)
		}
	}
}

func TestVerifierSubmitRobotHelp(t *testing.T) {
	help, helpStderr, err := executeBurpvalveCommand("--robots", "verifier", "submit", "-h")
	if err != nil {
		t.Fatalf("robot verifier submit help failed: %v\nstdout=%s\nstderr=%s", err, help, helpStderr)
	}
	if helpStderr != "" {
		t.Fatalf("robot submit help wrote stderr: %s", helpStderr)
	}
	for _, needle := range []string{
		`"command": "burpvalve verifier submit"`,
		`"condition"`,
		`"responses"`,
		`"staged-payload-hash"`,
		`"manifest-hash"`,
		`"condition-file-hash"`,
		`"transcript"`,
		"supplemental_verifiers",
		"adjudication",
		"authorization text alone is rejected",
	} {
		if !strings.Contains(help, needle) {
			t.Fatalf("robot verifier submit help missing %q:\n%s", needle, help)
		}
	}
}

func beginVerifierSubmitFixture(t *testing.T, repoRoot string) (string, backpressure.BeginResponsesResult) {
	t.Helper()
	target := fixtureGitRepo(t)
	writeCLIFile(t, target, "cmd/app/main.go", "package main\n")
	run(t, target, "git", "add", "cmd/app/main.go")
	stdout, stderr, err := runBurpvalve(t, repoRoot, "verifier", "begin", "--root", target, "--feature", "br-123", "--one-feature", "--atomicity-message", "Staged changes map only to br-123.", "--json")
	if err != nil {
		t.Fatalf("verifier begin failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var result backpressure.BeginResponsesResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode begin result: %v\n%s", err, stdout)
	}
	return target, result
}

func conditionHash(t *testing.T, result backpressure.BeginResponsesResult, conditionID string) string {
	t.Helper()
	hash := result.Plan.ConditionFileHashes[conditionID]
	if hash == "" {
		t.Fatalf("missing condition hash for %s", conditionID)
	}
	return hash
}

func verifierSubmitJSON(t *testing.T, begin backpressure.BeginResponsesResult, conditionID, conditionHash string, fields map[string]any) string {
	t.Helper()
	fields["staged_payload_hash"] = begin.StagedPayloadHash
	fields["manifest_hash"] = begin.ManifestHash
	fields["condition_file_hash"] = conditionHash
	body, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func readCLIResponses(t *testing.T, path string) backpressure.Responses {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var responses backpressure.Responses
	if err := json.Unmarshal(body, &responses); err != nil {
		t.Fatalf("decode responses: %v\n%s", err, body)
	}
	return responses
}

func cliResponseCondition(t *testing.T, responses backpressure.Responses, conditionID string) backpressure.ResponseCondition {
	t.Helper()
	for _, condition := range responses.Conditions {
		if condition.ConditionID == conditionID {
			return condition
		}
	}
	t.Fatalf("condition %s not found", conditionID)
	return backpressure.ResponseCondition{}
}
