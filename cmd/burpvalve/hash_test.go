package main

import (
	"encoding/json"
	"strings"
	"testing"

	"burpvalve/internal/backpressure"
)

func TestHashStagedCommandMatchesVerifierPromptHash(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)
	writeCLIFile(t, target, "src/hash.go", "package app\n\nconst Hash = true\n")
	writeCLIFile(t, target, "backpressure/attestations/README.md", "attestation docs stay hash included\n")
	writeCLIFile(t, target, "backpressure/attestations/generated.json", "{}\n")
	run(t, target, "git", "add", "src/hash.go", "backpressure/attestations/README.md", "backpressure/attestations/generated.json")

	stdout, stderr, err := runBurpvalve(t, repoRoot, "hash", "--staged", "--root", target, "--json")
	if err != nil {
		t.Fatalf("hash --staged failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if len(stderr) != 0 {
		t.Fatalf("hash --staged --json wrote stderr:\n%s", stderr)
	}
	var hashResult stagedHashResult
	if err := json.Unmarshal(stdout, &hashResult); err != nil {
		t.Fatalf("decode hash result: %v\n%s", err, stdout)
	}
	if hashResult.Command != "hash" || hashResult.Status != "completed" || hashResult.StagedPayloadHash == "" {
		t.Fatalf("unexpected hash result metadata: %#v", hashResult)
	}
	if !payloadHasPath(hashResult.StagedPayload, "src/hash.go") ||
		!payloadHasPath(hashResult.StagedPayload, "backpressure/attestations/README.md") {
		t.Fatalf("hash-included staged payload missing paths: %#v", hashResult.StagedPayload)
	}
	if !payloadHasPath(hashResult.HashExcludedStagedPayload, "backpressure/attestations/generated.json") ||
		payloadHasPath(hashResult.HashExcludedStagedPayload, "backpressure/attestations/README.md") {
		t.Fatalf("hash-excluded generated payload wrong: %#v", hashResult.HashExcludedStagedPayload)
	}
	assertHashStagedPayloadUnion(t, hashResult.StagedPayload, hashResult.HashExcludedStagedPayload, []string{
		"backpressure/attestations/README.md",
		"backpressure/attestations/generated.json",
		"src/hash.go",
	})
	if !strings.Contains(hashResult.Warning, "Naive git diff hashing is not equivalent") {
		t.Fatalf("warning missing naive diff text: %q", hashResult.Warning)
	}

	promptOut, promptErr, err := runBurpvalve(t, repoRoot, "verifier", "prompts", "--root", target, "--feature", "br-hash", "--condition", "dry", "--json")
	if err != nil {
		t.Fatalf("verifier prompts failed: %v\nstdout=%s\nstderr=%s", err, promptOut, promptErr)
	}
	var prompts backpressure.VerifierPromptSet
	if err := json.Unmarshal(promptOut, &prompts); err != nil {
		t.Fatalf("decode verifier prompts: %v\n%s", err, promptOut)
	}
	if prompts.StagedPayloadHash != hashResult.StagedPayloadHash {
		t.Fatalf("hash helper and verifier prompts disagree: hash=%s prompts=%s", hashResult.StagedPayloadHash, prompts.StagedPayloadHash)
	}
}

func TestHashStagedCommandHumanOutputAndRequiredFlag(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := fixtureGitRepo(t)
	writeCLIFile(t, target, "src/hash_human.go", "package app\n\nconst Human = true\n")
	run(t, target, "git", "add", "src/hash_human.go")

	human, stderr, err := runBurpvalve(t, repoRoot, "hash", "--staged", "--root", target, "--color", "never")
	if err != nil {
		t.Fatalf("hash --staged human failed: %v\nstdout=%s\nstderr=%s", err, human, stderr)
	}
	text := string(human)
	for _, needle := range []string{
		"Burpvalve staged payload hash",
		"Hash:",
		"Hash-included staged paths:",
		"src/hash_human.go",
		"Hash-excluded generated evidence paths:",
		"Generated evidence prefixes:",
		"Warning: Naive git diff hashing is not equivalent",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("human hash output missing %q:\n%s", needle, text)
		}
	}

	stdout, stderr, err := runBurpvalve(t, repoRoot, "hash", "--root", target, "--json")
	if err == nil {
		t.Fatalf("hash without --staged should fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if !strings.Contains(string(stderr), "hash currently supports only --staged") {
		t.Fatalf("missing --staged error not explained:\nstdout=%s\nstderr=%s", stdout, stderr)
	}
}

func TestHashRobotsHelpDocumentsStagedPayloadContract(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("--robots", "hash", "-h")
	if err != nil {
		t.Fatalf("robots hash help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("robots hash help wrote stderr: %s", stderr)
	}
	for _, needle := range []string{
		`"staged_payload_hash"`,
		`"staged_payload"`,
		`"hash_excluded_staged_payload"`,
		`"generated_path_prefixes"`,
		`exactly once across staged_payload and hash_excluded_staged_payload`,
		`HashStagedPayload`,
		`git diff --cached --binary | sha256sum`,
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("robots hash help missing %q:\n%s", needle, stdout)
		}
	}
}

func assertHashStagedPayloadUnion(t *testing.T, included []backpressure.StagedPayloadFile, excluded []backpressure.StagedPayloadFile, want []string) {
	t.Helper()
	seen := map[string]int{}
	for _, file := range included {
		seen[file.Path]++
	}
	for _, file := range excluded {
		seen[file.Path]++
	}
	for path, count := range seen {
		if count != 1 {
			t.Fatalf("staged path %q appears %d times across included/excluded payloads; included=%#v excluded=%#v", path, count, included, excluded)
		}
	}
	for _, path := range want {
		if seen[path] != 1 {
			t.Fatalf("staged path %q appears %d times across included/excluded payloads; included=%#v excluded=%#v", path, seen[path], included, excluded)
		}
	}
}
