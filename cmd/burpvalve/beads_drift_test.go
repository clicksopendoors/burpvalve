package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBeadsDriftClosedDirtyReportsPossible(t *testing.T) {
	root := fixtureCleanDriftRepo(t)
	closedAt := time.Now().UTC().Format(time.RFC3339)
	installFakeBRList(t, `[{"id":"br-dirty","title":"Dirty closure","status":"closed","closed_at":"`+closedAt+`"}]`)
	writeCLIFile(t, root, "src/dirty.txt", "uncommitted\n")

	stdout, stderr, err := executeBurpvalveCommand("beads", "drift", "--root", root, "--json")
	if err != nil {
		t.Fatalf("beads drift is advisory and should not fail: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("beads drift should not write stderr: %s", stderr)
	}
	var got beadsDriftReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode drift report: %v\n%s", err, stdout)
	}
	if got.Status != "possible" || got.Fatal || got.Mutating || !got.DirtyTree {
		t.Fatalf("unexpected dirty drift status: %#v", got)
	}
	if len(got.Findings) != 1 || got.Findings[0].BeadID != "br-dirty" || got.Findings[0].Status != "possible" {
		t.Fatalf("possible finding missing: %#v", got.Findings)
	}
	if !containsString(got.DirtyPaths, "src/dirty.txt") {
		t.Fatalf("dirty path missing: %#v", got.DirtyPaths)
	}
	if len(got.NextSteps) == 0 || got.NextSteps[0].ID != "inspect-possible-drift" {
		t.Fatalf("next steps should advise inspection: %#v", got.NextSteps)
	}
}

func TestBeadsDriftAttestedClosureReportsClean(t *testing.T) {
	root := fixtureCleanDriftRepo(t)
	closedAt := time.Now().UTC().Format(time.RFC3339)
	installFakeBRList(t, `[{"id":"br-attested","title":"Attested closure","status":"closed","closed_at":"`+closedAt+`"}]`)
	writeDriftAttestation(t, root, "br-attested")
	writeCLIFile(t, root, "src/dirty.txt", "uncommitted\n")

	stdout, stderr, err := executeBurpvalveCommand("beads", "drift", "--root", root, "--json")
	if err != nil {
		t.Fatalf("attested closure should not fail: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var got beadsDriftReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode drift report: %v\n%s", err, stdout)
	}
	if got.Status != "clean" || !got.DirtyTree || len(got.Findings) != 0 {
		t.Fatalf("attested closure should be clean despite unrelated dirt: %#v", got)
	}
	if len(got.CheckedBeads) != 1 || got.CheckedBeads[0].Status != "attested" || !containsString(got.CheckedBeads[0].AttestationPaths, "backpressure/attestations/br-attested.json") {
		t.Fatalf("attested bead match missing: %#v", got.CheckedBeads)
	}
}

func TestBeadsDriftCleanTreeUnmatchedIsAdvisory(t *testing.T) {
	root := fixtureCleanDriftRepo(t)
	closedAt := time.Now().UTC().Format(time.RFC3339)
	installFakeBRList(t, `[{"id":"br-clean","title":"Clean closure","status":"closed","closed_at":"`+closedAt+`"}]`)

	stdout, stderr, err := executeBurpvalveCommand("beads", "drift", "--root", root, "--json")
	if err != nil {
		t.Fatalf("clean unmatched closure should be advisory: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var got beadsDriftReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode drift report: %v\n%s", err, stdout)
	}
	if got.Status != "clean" || got.DirtyTree {
		t.Fatalf("clean unmatched closure should stay clean: %#v", got)
	}
	if len(got.Findings) != 1 || got.Findings[0].Status != "unmatched_clean_tree" || got.Findings[0].Severity != "info" {
		t.Fatalf("clean-tree advisory missing: %#v", got.Findings)
	}
	if strings.Contains(got.Findings[0].Message, "dirty") && strings.Contains(got.Findings[0].Message, "possible") {
		t.Fatalf("clean-tree advisory overclaimed dirty drift: %#v", got.Findings[0])
	}
}

func TestBeadsDriftMalformedClosedAtWarns(t *testing.T) {
	root := fixtureCleanDriftRepo(t)
	installFakeBRList(t, `[{"id":"br-missing","title":"Missing closure","status":"closed"},{"id":"br-bad","title":"Bad closure","status":"closed","closed_at":"not-a-time"}]`)

	stdout, stderr, err := executeBurpvalveCommand("beads", "drift", "--root", root, "--json")
	if err != nil {
		t.Fatalf("malformed closed_at should warn, not fail: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var got beadsDriftReport
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode drift report: %v\n%s", err, stdout)
	}
	warnings := strings.Join(got.Warnings, "\n")
	if got.Status != "warning" || !strings.Contains(warnings, "br-missing missing closed_at") || !strings.Contains(warnings, `br-bad has malformed closed_at "not-a-time"`) {
		t.Fatalf("closed_at warnings missing: %#v", got)
	}
	if len(got.CheckedBeads) != 0 {
		t.Fatalf("malformed rows should be skipped from checked beads: %#v", got.CheckedBeads)
	}
}

func TestRobotsBeadsDriftHelpDocumentsReadOnlyAdvisory(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("--robots", "beads", "drift", "-h")
	if err != nil {
		t.Fatalf("robots beads drift help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var doc robotHelpDoc
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("decode robots help: %v\n%s", err, stdout)
	}
	if doc.Command != "burpvalve beads drift" {
		t.Fatalf("unexpected command: %#v", doc)
	}
	if !strings.Contains(mustJSONText(t, doc.RobotOutput), "dirty_tree") || !strings.Contains(mustJSONText(t, doc.RobotOutput), "next_steps") {
		t.Fatalf("robots output missing drift fields: %#v", doc.RobotOutput)
	}
	if !containsString(doc.Notes, "beads drift is read-only and advisory; it never closes beads, stages files, writes files, commits files, or blocks br") {
		t.Fatalf("robots notes missing read-only advisory boundary: %#v", doc.Notes)
	}
}

func fixtureCleanDriftRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run(t, root, "git", "init", "-q")
	run(t, root, "git", "config", "user.email", "michael-bltzr@users.noreply.github.com")
	run(t, root, "git", "config", "user.name", "Test User")
	writeCLIFile(t, root, "README.md", "baseline\n")
	run(t, root, "git", "add", "README.md")
	run(t, root, "git", "commit", "-q", "-m", "baseline")
	return root
}

func installFakeBRList(t *testing.T, listJSON string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "br")
	writeBeadsExecutable(t, path, `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "list" && "$2" == "--status" && "$3" == "closed" && "$4" == "--json" ]]; then
  cat <<'JSON'
`+listJSON+`
JSON
  exit 0
fi
echo "unexpected br args: $*" >&2
exit 2
`)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeDriftAttestation(t *testing.T, root, beadID string) {
	t.Helper()
	writeCLIFile(t, root, "backpressure/attestations/"+beadID+".json", `{
  "schema_version": 1,
  "tool": "burpvalve",
  "tool_version": "0.1.0",
  "artifact_kind": "passing",
  "staged_payload_hash": "sha256:payload",
  "manifest_hash": "sha256:manifest",
  "condition_order": ["lint-rules"],
  "generated_by": {"agent": "codex", "model": "gpt-5"},
  "git_head_before_commit": "abc123",
  "created_at": "2026-07-02T00:00:00Z",
  "feature": {"id": "`+beadID+`", "kind": "feature", "name": "drift test", "source_bead": "`+beadID+`", "bead_ids": ["`+beadID+`"]},
  "bead_ids": ["`+beadID+`"],
  "atomicity": {"one_feature_or_fix": true, "message": "Staged changes map only to `+beadID+`."},
  "conditions": [{
    "condition_id": "lint-rules",
    "condition_file": "backpressure/lint-rules.md",
    "condition_file_hash": "sha256:lint",
    "subagent_confirmed": true,
    "subagent_model": "test",
    "verdict": "pass",
    "evidence": ["go test ./..."],
    "timestamp": "2026-07-02T00:00:00Z"
  }]
}`)
}

func mustJSONText(t *testing.T, value any) string {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
