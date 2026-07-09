package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestPxpackDryRunJSONContract(t *testing.T) {
	out := filepath.Join(t.TempDir(), "packet")
	stdout, stderr, err := executeBurpvalveCommand(
		"pxpack",
		"--orchestrator",
		"--out", out,
		"--packet-id", "orchestrator-bootstrap-test",
		"--dry-run",
		"--json",
	)
	if err != nil {
		t.Fatalf("pxpack dry-run failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("pxpack dry-run wrote stderr: %s", stderr)
	}
	var got pxpackResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode pxpack dry-run: %v\n%s", err, stdout)
	}
	if got.Status != "planned" || got.Mode != "orchestrator" || got.PacketID != "orchestrator-bootstrap-test" {
		t.Fatalf("unexpected pxpack identity: %#v", got)
	}
	if got.PacketDir != out || got.ManifestPath != filepath.Join(out, "manifest.json") || got.FactsheetPath != filepath.Join(out, "factsheet.txt") {
		t.Fatalf("packet paths wrong: %#v", got)
	}
	if got.PxpipeRole != "image_lane_renderer_only" || got.FactsheetMode != "burpvalve_generated" || got.ManifestHashMode != "burpvalve_source_content_hashes" {
		t.Fatalf("prototype-derived boundaries missing: %#v", got)
	}
	if len(got.SourceInventory.Factsheet) == 0 || !containsPxpackString(got.SourceInventory.Factsheet, "templates/claude/skills/burpvalve-orchestrator/references/orchestrator-toolbox.md.tmpl") {
		t.Fatalf("default factsheet sources should include hcil toolbox: %#v", got.SourceInventory.Factsheet)
	}
	if len(got.PlannedPxpipeCommand) == 0 || !containsPxpackString(got.PlannedPxpipeCommand, "pxpipe-proxy") {
		t.Fatalf("planned pxpipe command missing renderer: %#v", got.PlannedPxpipeCommand)
	}
	if len(got.SourceHashes) == 0 || !containsPxpackSourceHash(got.SourceHashes, "templates/claude/skills/burpvalve-orchestrator/references/orchestrator-toolbox.md.tmpl", "factsheet") {
		t.Fatalf("source hashes should include factsheet sources: %#v", got.SourceHashes)
	}
}

func TestPxpackRobotsInputDryRun(t *testing.T) {
	out := filepath.Join(t.TempDir(), "robot-packet")
	input := `{
  "mode": "orchestrator",
  "out_dir": "` + escapeJSONForTest(out) + `",
  "packet_id": "robot-packet",
  "factsheet_sources": ["docs/ntm-bridge.md"],
  "image_sources": ["ORCHESTRATOR.md"],
  "live_sources": ["AGENTS.md"],
  "excludes": ["*.secret"],
  "max_pages": 4,
  "dry_run": true
}`
	stdout, stderr, err := executeBurpvalveCommandWithInput(input, "--robots", "pxpack")
	if err != nil {
		t.Fatalf("pxpack robots dry-run failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var got pxpackResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode pxpack robots result: %v\n%s", err, stdout)
	}
	if stderr != "" || got.Status != "planned" || got.PacketID != "robot-packet" {
		t.Fatalf("unexpected robots result: stderr=%q got=%#v", stderr, got)
	}
	if strings.Join(got.SourceInventory.Factsheet, ",") != "docs/ntm-bridge.md" || strings.Join(got.SourceInventory.Image, ",") != "ORCHESTRATOR.md" || strings.Join(got.SourceInventory.Live, ",") != "AGENTS.md" {
		t.Fatalf("robot inventory not applied: %#v", got.SourceInventory)
	}
	if !containsPxpackString(got.PlannedPxpipeCommand, "--exclude") || !containsPxpackString(got.PlannedPxpipeCommand, "*.secret") {
		t.Fatalf("exclude not reflected in planned renderer command: %#v", got.PlannedPxpipeCommand)
	}
}

func TestPxpackClassifiesGenericSourcesAndProtectsLiveOnly(t *testing.T) {
	dense := filepath.Join(t.TempDir(), "dense-notes.md")
	if err := os.WriteFile(dense, []byte("dense context"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "packet")
	stdout, stderr, err := executeBurpvalveCommand(
		"pxpack",
		"--orchestrator",
		"--out", out,
		"--source", dense,
		"--source", "AGENTS.md",
		"--image-source", "docs/dogfooding-findings-2026-07.md",
		"--image-source", "backpressure/README.md",
		"--factsheet-source", "docs/ntm-bridge.md",
		"--dry-run",
		"--json",
	)
	if err != nil {
		t.Fatalf("pxpack classified dry-run failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var got pxpackResult
	if decodeErr := json.Unmarshal([]byte(stdout), &got); decodeErr != nil {
		t.Fatalf("decode pxpack result: %v\n%s", decodeErr, stdout)
	}
	if got.Status != "planned" {
		t.Fatalf("unexpected status: %#v", got)
	}
	if !containsPxpackString(got.SourceInventory.Image, dense) || !containsPxpackString(got.SourceInventory.Image, "docs/dogfooding-findings-2026-07.md") {
		t.Fatalf("image lane missing dense sources: %#v", got.SourceInventory.Image)
	}
	if containsPxpackString(got.SourceInventory.Image, "backpressure/README.md") {
		t.Fatalf("live-only backpressure source should not remain in image lane: %#v", got.SourceInventory.Image)
	}
	if !containsPxpackString(got.SourceInventory.Live, "AGENTS.md") || !containsPxpackString(got.SourceInventory.Live, "backpressure/README.md") {
		t.Fatalf("live lane missing protected sources: %#v", got.SourceInventory.Live)
	}
	if !containsPxpackString(got.SourceInventory.Factsheet, "docs/ntm-bridge.md") {
		t.Fatalf("factsheet lane missing explicit source: %#v", got.SourceInventory.Factsheet)
	}
	if !containsPxpackString(got.PlannedPxpipeCommand, dense) || containsPxpackString(got.PlannedPxpipeCommand, "AGENTS.md") || containsPxpackString(got.PlannedPxpipeCommand, "backpressure/README.md") {
		t.Fatalf("planned pxpipe command should include image lane only: %#v", got.PlannedPxpipeCommand)
	}
	if !containsPxpackSubstring(got.Warnings, `image source "backpressure/README.md" is live-only`) {
		t.Fatalf("expected live-only warning, got %#v", got.Warnings)
	}
}

func TestPxpackRejectsMissingSource(t *testing.T) {
	out := filepath.Join(t.TempDir(), "packet")
	stdout, _, err := executeBurpvalveCommand(
		"pxpack",
		"--orchestrator",
		"--out", out,
		"--source", filepath.Join(t.TempDir(), "missing.md"),
		"--dry-run",
		"--json",
	)
	if err == nil {
		t.Fatalf("pxpack should reject missing sources\nstdout=%s", stdout)
	}
	var got pxpackResult
	if decodeErr := json.Unmarshal([]byte(stdout), &got); decodeErr != nil {
		t.Fatalf("missing-source blocker should emit JSON: %v\n%s", decodeErr, stdout)
	}
	if got.Status != "blocked" || !containsPxpackSubstring(got.NextSteps, "Use existing source paths") {
		t.Fatalf("missing-source blocker not actionable: %#v", got)
	}
}

func TestPxpackBlocksSensitivePathBeforePlanning(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	safe := filepath.Join(dir, "safe.md")
	if err := os.WriteFile(envFile, []byte("NOT_A_REAL_SECRET=placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(safe, []byte("safe context"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := executeBurpvalveCommand(
		"pxpack",
		"--orchestrator",
		"--out", filepath.Join(dir, "packet"),
		"--factsheet-source", safe,
		"--image-source", envFile,
		"--live-source", safe,
		"--dry-run",
		"--json",
	)
	if err == nil {
		t.Fatalf("pxpack should block sensitive path\nstdout=%s", stdout)
	}
	var got pxpackResult
	if decodeErr := json.Unmarshal([]byte(stdout), &got); decodeErr != nil {
		t.Fatalf("decode sensitive-path blocker: %v\n%s", decodeErr, stdout)
	}
	if got.Status != "blocked" || len(got.SensitiveFindings) == 0 {
		t.Fatalf("sensitive-path blocker missing findings: %#v", got)
	}
	if got.SensitiveFindings[0].Pattern != ".env" || got.SensitiveFindings[0].Lane != "image" {
		t.Fatalf("unexpected sensitive path finding: %#v", got.SensitiveFindings)
	}
}

func TestPxpackBlocksSensitiveContentInEveryLane(t *testing.T) {
	dir := t.TempDir()
	factsheet := filepath.Join(dir, "factsheet.md")
	image := filepath.Join(dir, "image.md")
	live := filepath.Join(dir, "live.md")
	if err := os.WriteFile(factsheet, []byte("client_secret = abcdefghijklmnop123456"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(image, []byte("host 192.168.10.20"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(live, []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nredacted\n-----END OPENSSH PRIVATE KEY-----"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := executeBurpvalveCommand(
		"pxpack",
		"--orchestrator",
		"--out", filepath.Join(dir, "packet"),
		"--factsheet-source", factsheet,
		"--image-source", image,
		"--live-source", live,
		"--dry-run",
		"--json",
	)
	if err == nil {
		t.Fatalf("pxpack should block sensitive content\nstdout=%s", stdout)
	}
	var got pxpackResult
	if decodeErr := json.Unmarshal([]byte(stdout), &got); decodeErr != nil {
		t.Fatalf("decode sensitive-content blocker: %v\n%s", decodeErr, stdout)
	}
	if got.Status != "blocked" || len(got.SensitiveFindings) != 3 {
		t.Fatalf("sensitive-content blocker missing lane findings: %#v", got.SensitiveFindings)
	}
	for _, want := range []string{"factsheet:token-assignment", "image:private-ipv4", "live:private-key-block"} {
		if !containsPxpackFinding(got.SensitiveFindings, want) {
			t.Fatalf("missing finding %s in %#v", want, got.SensitiveFindings)
		}
	}
}

func TestPxpackNonDryRunBlocksUntilWriterUnit(t *testing.T) {
	out := filepath.Join(t.TempDir(), "packet")
	t.Setenv("BURPVALVE_PXPACK_RENDERER", filepath.Join(t.TempDir(), "missing-renderer"))
	stdout, stderr, err := executeBurpvalveCommand("pxpack", "--orchestrator", "--out", out, "--json")
	if err == nil || !strings.Contains(err.Error(), "run PXPIPE renderer") {
		t.Fatalf("pxpack non-dry-run should require renderer without fake executable\nerr=%v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var got pxpackResult
	if decodeErr := json.Unmarshal([]byte(stdout), &got); decodeErr != nil {
		t.Fatalf("blocked pxpack should still emit JSON: %v\n%s", decodeErr, stdout)
	}
	if got.Status != "failed" || !containsPxpackSubstring(got.NextSteps, "Install PXPIPE") {
		t.Fatalf("blocked output not actionable: %#v", got)
	}
}

func TestPxpackRunsRendererAndPublishesOnlyRendererArtifacts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fake renderer is POSIX-only")
	}
	dir := t.TempDir()
	renderer := writeFakePxpackRenderer(t, dir, 0)
	factsheet := filepath.Join(dir, "factsheet.md")
	if err := os.WriteFile(factsheet, []byte("Exact command: burpvalve verifier begin --feature demo --one-feature --json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BURPVALVE_PXPACK_RENDERER", renderer)
	out := filepath.Join(dir, "packet")
	stdout, stderr, err := executeBurpvalveCommand(
		"pxpack",
		"--orchestrator",
		"--out", out,
		"--factsheet-source", factsheet,
		"--image-source", "ORCHESTRATOR.md",
		"--live-source", "AGENTS.md",
		"--json",
	)
	if err != nil {
		t.Fatalf("pxpack renderer run failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var got pxpackResult
	if decodeErr := json.Unmarshal([]byte(stdout), &got); decodeErr != nil {
		t.Fatalf("decode pxpack renderer result: %v\n%s", decodeErr, stdout)
	}
	if got.Status != "ok" || !got.Mutating || got.PageCount != 1 {
		t.Fatalf("unexpected renderer result: %#v", got)
	}
	for _, rel := range []string{"page-001.png", "factsheet.txt", "source-map.md", "manifest.json", "renderer/stdout.txt", "renderer/stderr.txt", "renderer/telemetry.json", "renderer/pxpipe-manifest.json"} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected %s in packet: %v", rel, err)
		}
	}
	var manifest pxpackManifest
	if err := json.Unmarshal([]byte(readTestFile(t, filepath.Join(out, "manifest.json"))), &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.PacketID != "packet" || manifest.PageCount != 1 || manifest.RendererManifestTrusted {
		t.Fatalf("unexpected manifest identity/boundary: %#v", manifest)
	}
	if !strings.Contains(manifest.NoEvidenceStatement, "not verifier evidence") {
		t.Fatalf("manifest missing no-evidence statement: %#v", manifest.NoEvidenceStatement)
	}
	if !containsPxpackSourceHash(manifest.SourceHashes, factsheet, "factsheet") || !containsPxpackOutputHash(manifest.OutputHashes, "factsheet.txt") || !containsPxpackOutputHash(manifest.OutputHashes, "page-001.png") {
		t.Fatalf("manifest missing source/output hashes: %#v", manifest)
	}
	factsheetBody := string(readTestFile(t, filepath.Join(out, "factsheet.txt")))
	if strings.Contains(factsheetBody, "auto factsheet should be discarded") {
		t.Fatalf("PXPIPE auto factsheet should not be kept:\n%s", factsheetBody)
	}
	if !strings.Contains(factsheetBody, "burpvalve verifier begin --feature demo --one-feature --json") {
		t.Fatalf("Burpvalve factsheet missing exact command:\n%s", factsheetBody)
	}
	sourceMap := string(readTestFile(t, filepath.Join(out, "source-map.md")))
	if !strings.Contains(sourceMap, "lane=`factsheet`") || !strings.Contains(sourceMap, "lane=`image`") || !strings.Contains(sourceMap, "lane=`live`") {
		t.Fatalf("source map missing lane inventory:\n%s", sourceMap)
	}
	if stderr != "" {
		t.Fatalf("pxpack command should not write stderr on successful JSON output: %s", stderr)
	}
}

func TestPxpackCheckDetectsFreshChangedSourceAndMissingOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fake renderer is POSIX-only")
	}
	dir := t.TempDir()
	renderer := writeFakePxpackRenderer(t, dir, 0)
	factsheet := filepath.Join(dir, "factsheet.md")
	if err := os.WriteFile(factsheet, []byte("Exact command: burpvalve verifier begin --feature demo --one-feature --json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BURPVALVE_PXPACK_RENDERER", renderer)
	out := filepath.Join(dir, "packet")
	stdout, stderr, err := executeBurpvalveCommand(
		"pxpack",
		"--orchestrator",
		"--out", out,
		"--factsheet-source", factsheet,
		"--image-source", "ORCHESTRATOR.md",
		"--live-source", "AGENTS.md",
		"--json",
	)
	if err != nil {
		t.Fatalf("pxpack renderer run failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	stdout, stderr, err = executeBurpvalveCommand("pxpack", "--orchestrator", "--check", out, "--json")
	if err != nil {
		t.Fatalf("pxpack check should pass for fresh packet: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	var fresh pxpackResult
	if decodeErr := json.Unmarshal([]byte(stdout), &fresh); decodeErr != nil {
		t.Fatalf("decode fresh check: %v\n%s", decodeErr, stdout)
	}
	if fresh.Status != "ok" || fresh.Stale || fresh.Mutating {
		t.Fatalf("fresh check should be ok/read-only: %#v", fresh)
	}
	if err := os.WriteFile(factsheet, []byte("Exact command changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, _, err = executeBurpvalveCommand("pxpack", "--orchestrator", "--check", out, "--json")
	if err == nil {
		t.Fatalf("pxpack check should fail after source change\nstdout=%s", stdout)
	}
	var changed pxpackResult
	if decodeErr := json.Unmarshal([]byte(stdout), &changed); decodeErr != nil {
		t.Fatalf("decode changed check: %v\n%s", decodeErr, stdout)
	}
	if changed.Status != "blocked" || !changed.Stale || !containsPxpackSubstring(changed.Warnings, "hash changed") {
		t.Fatalf("changed source should mark packet stale: %#v", changed)
	}
	if err := os.WriteFile(factsheet, []byte("Exact command: burpvalve verifier begin --feature demo --one-feature --json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(out, "page-001.png")); err != nil {
		t.Fatal(err)
	}
	stdout, _, err = executeBurpvalveCommand("pxpack", "--orchestrator", "--check", out, "--json")
	if err == nil {
		t.Fatalf("pxpack check should fail after output removal\nstdout=%s", stdout)
	}
	var missing pxpackResult
	if decodeErr := json.Unmarshal([]byte(stdout), &missing); decodeErr != nil {
		t.Fatalf("decode missing-output check: %v\n%s", decodeErr, stdout)
	}
	if missing.Status != "blocked" || !missing.Stale || !containsPxpackSubstring(missing.Warnings, "output page-001.png") {
		t.Fatalf("missing output should mark packet stale: %#v", missing)
	}
	if err := os.WriteFile(filepath.Join(out, "page-001.png"), []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "extra.txt"), []byte("unexpected"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, _, err = executeBurpvalveCommand("pxpack", "--orchestrator", "--check", out, "--json")
	if err == nil {
		t.Fatalf("pxpack check should fail after unexpected output appears\nstdout=%s", stdout)
	}
	var extra pxpackResult
	if decodeErr := json.Unmarshal([]byte(stdout), &extra); decodeErr != nil {
		t.Fatalf("decode unexpected-output check: %v\n%s", decodeErr, stdout)
	}
	if extra.Status != "blocked" || !extra.Stale || !containsPxpackSubstring(extra.Warnings, "output extra.txt") {
		t.Fatalf("unexpected output should mark packet stale: %#v", extra)
	}
}

func TestPxpackRendererFailureDoesNotPublishPacket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fake renderer is POSIX-only")
	}
	dir := t.TempDir()
	renderer := writeFakePxpackRenderer(t, dir, 23)
	t.Setenv("BURPVALVE_PXPACK_RENDERER", renderer)
	out := filepath.Join(dir, "packet")
	stdout, _, err := executeBurpvalveCommand(
		"pxpack",
		"--orchestrator",
		"--out", out,
		"--factsheet-source", "docs/ntm-bridge.md",
		"--image-source", "ORCHESTRATOR.md",
		"--live-source", "AGENTS.md",
		"--json",
	)
	if err == nil {
		t.Fatalf("pxpack should fail when renderer exits non-zero\nstdout=%s", stdout)
	}
	var got pxpackResult
	if decodeErr := json.Unmarshal([]byte(stdout), &got); decodeErr != nil {
		t.Fatalf("decode renderer failure result: %v\n%s", decodeErr, stdout)
	}
	if got.Status != "failed" || !containsPxpackSubstring(got.NextSteps, "Install PXPIPE") {
		t.Fatalf("renderer failure not actionable: %#v", got)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("failed renderer should not publish packet dir, stat err=%v", err)
	}
}

func TestPxpackCheckRequiresManifest(t *testing.T) {
	dir := t.TempDir()
	stdout, _, err := executeBurpvalveCommand("pxpack", "--orchestrator", "--check", dir, "--json")
	if err == nil {
		t.Fatalf("pxpack check should require Burpvalve-owned manifest\nstdout=%s", stdout)
	}
	var got pxpackResult
	if decodeErr := json.Unmarshal([]byte(stdout), &got); decodeErr != nil {
		t.Fatalf("check blocker should emit JSON: %v\n%s", decodeErr, stdout)
	}
	if got.Status != "blocked" || got.ManifestPath != filepath.Join(dir, "manifest.json") {
		t.Fatalf("check blocker paths wrong: %#v", got)
	}
}

func TestPxpackValidateSafeFixtureRecommends(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand(
		"pxpack",
		"validate",
		"--fixture", filepath.Join("testdata", "pxpack-validation-safe.json"),
		"--json",
	)
	if err != nil {
		t.Fatalf("pxpack validate should pass safe fixture: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("pxpack validate wrote stderr: %s", stderr)
	}
	var got pxpackValidationResult
	if decodeErr := json.Unmarshal([]byte(stdout), &got); decodeErr != nil {
		t.Fatalf("decode validation result: %v\n%s", decodeErr, stdout)
	}
	if got.Status != "passed" || !got.Recommended || got.Mutating {
		t.Fatalf("safe fixture should recommend read-only packet use: %#v", got)
	}
	if len(got.PacketScore.MissedExactStrings) != 0 || len(got.PacketScore.MissingFactsheetProof) != 0 {
		t.Fatalf("packet arm should preserve exact strings and prototype proof: %#v", got.PacketScore)
	}
	if len(got.PlainTextScore.MissedExactStrings) == 0 {
		t.Fatalf("fixture should prove packet improves exact-string recall over plain text: %#v", got.PlainTextScore)
	}
	if !containsPxpackSubstring(got.Benefits, "fewer cost units") {
		t.Fatalf("safe fixture should report a measured benefit: %#v", got.Benefits)
	}
}

func TestPxpackValidateBlocksUnsafePacketArm(t *testing.T) {
	dir := t.TempDir()
	fixture := pxpackValidationFixture{
		SchemaVersion: 1,
		Experiment:    "unsafe packet regression",
		PacketArm: pxpackValidationArm{
			Name:               "packet",
			CostUnits:          20,
			LatencyMs:          100,
			OperatorFocusScore: 4,
			Answers: map[string]string{
				"next_action": "submit verifier cells without the exact command",
				"gate_stop":   "continue even when a verifier is unknown",
			},
			SourceRereads: []string{"ORCHESTRATOR.md"},
			FactsheetText: "PXPIPE auto factsheet summary without the full command",
		},
		PlainTextArm: pxpackValidationArm{
			Name:               "plain_text",
			CostUnits:          30,
			LatencyMs:          120,
			OperatorFocusScore: 3,
			Answers: map[string]string{
				"next_action": "run burpvalve verifier begin --feature demo --one-feature --json",
				"gate_stop":   "stop on fail/unknown",
			},
			SourceRereads: []string{"ORCHESTRATOR.md", "templates/claude/skills/burpvalve-orchestrator/references/orchestrator-toolbox.md.tmpl"},
		},
		ExpectedExactStrings: []string{
			"burpvalve verifier begin --feature demo --one-feature --json",
		},
		RequiredSourceRereads: []string{
			"ORCHESTRATOR.md",
			"templates/claude/skills/burpvalve-orchestrator/references/orchestrator-toolbox.md.tmpl",
		},
		RequiredDecisionNeedles: []string{"stop on fail/unknown"},
		PrototypeDroppedCommands: []string{
			"burpvalve verifier begin --feature demo --one-feature --json",
		},
	}
	body, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "unsafe.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := executeBurpvalveCommand("pxpack", "validate", "--fixture", path, "--json")
	if err == nil {
		t.Fatalf("unsafe packet arm should block recommendation\nstdout=%s", stdout)
	}
	var got pxpackValidationResult
	if decodeErr := json.Unmarshal([]byte(stdout), &got); decodeErr != nil {
		t.Fatalf("decode blocked validation result: %v\n%s", decodeErr, stdout)
	}
	if got.Status != "blocked" || got.Recommended {
		t.Fatalf("unsafe fixture should be blocked: %#v", got)
	}
	for _, want := range []string{"missed more exact strings", "prototype-dropped command strings"} {
		if !containsPxpackSubstring(got.Failures, want) {
			t.Fatalf("missing validation failure %q in %#v", want, got.Failures)
		}
	}
}

func TestPxpackRobotsHelpDocumentsBoundaries(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("--robots", "pxpack", "-h")
	if err != nil {
		t.Fatalf("pxpack robots help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	for _, needle := range []string{
		`"command": "burpvalve pxpack"`,
		"factsheet_sources",
		"PXPIPE is only the image-lane renderer",
		"burpvalve_generated",
		"burpvalve_source_content_hashes",
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("pxpack robots help missing %q:\n%s", needle, stdout)
		}
	}
}

func TestPxpackValidateRobotsHelpDocumentsFixtureGate(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("--robots", "pxpack", "validate", "-h")
	if err != nil {
		t.Fatalf("pxpack validate robots help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	for _, needle := range []string{
		`"command": "burpvalve pxpack validate"`,
		"fixture",
		"missed exact",
		"invented facts",
		"source re-read",
		"operator-focus benefit",
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("pxpack validate robots help missing %q:\n%s", needle, stdout)
		}
	}
}

func escapeJSONForTest(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return replacer.Replace(value)
}

func containsPxpackString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func containsPxpackSubstring(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func containsPxpackFinding(findings []pxpackFinding, lanePattern string) bool {
	for _, finding := range findings {
		if finding.Lane+":"+finding.Pattern == lanePattern {
			return true
		}
	}
	return false
}

func containsPxpackSourceHash(refs []pxpackSourceRef, path, lane string) bool {
	for _, ref := range refs {
		if ref.Path == path && ref.Lane == lane && strings.HasPrefix(ref.Hash, "sha256:") {
			return true
		}
	}
	return false
}

func containsPxpackOutputHash(refs []pxpackOutputRef, path string) bool {
	for _, ref := range refs {
		if ref.Path == path && strings.HasPrefix(ref.Hash, "sha256:") {
			return true
		}
	}
	return false
}

func writeFakePxpackRenderer(t *testing.T, dir string, exitCode int) string {
	t.Helper()
	path := filepath.Join(dir, "fake-pxpipe")
	body := `#!/usr/bin/env bash
set -u
out=""
for ((i=1; i<=$#; i++)); do
  if [[ "${!i}" == "--out" ]]; then
    j=$((i+1))
    out="${!j}"
  fi
done
echo "fake renderer stdout"
echo "fake renderer stderr" >&2
if [[ ` + strconv.Itoa(exitCode) + ` -ne 0 ]]; then
  exit ` + strconv.Itoa(exitCode) + `
fi
mkdir -p "$out"
printf "png" > "$out/page-001.png"
printf "auto factsheet should be discarded" > "$out/factsheet.txt"
printf '{"renderer":"fake"}' > "$out/manifest.json"
`
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
