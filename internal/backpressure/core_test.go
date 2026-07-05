package backpressure

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type fakeStaged struct {
	paths   []string
	content map[string]string
}

func (f fakeStaged) StagedFiles(context.Context, string) ([]string, error) {
	return append([]string(nil), f.paths...), nil
}

func (f fakeStaged) StagedFileContent(_ context.Context, _ string, path string) ([]byte, error) {
	body, ok := f.content[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return []byte(body), nil
}

type fakeStagedEntries struct {
	entries []StagedPayloadFile
	content map[string]string
	reads   []string
}

func (f *fakeStagedEntries) StagedFiles(context.Context, string) ([]string, error) {
	var paths []string
	for _, entry := range f.entries {
		paths = append(paths, entry.Path)
	}
	return paths, nil
}

func (f *fakeStagedEntries) StagedEntries(context.Context, string) ([]StagedPayloadFile, error) {
	return append([]StagedPayloadFile(nil), f.entries...), nil
}

func (f *fakeStagedEntries) StagedFileContent(_ context.Context, _ string, path string) ([]byte, error) {
	f.reads = append(f.reads, path)
	body, ok := f.content[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return []byte(body), nil
}

func TestLoadManifestAndConditionOrder(t *testing.T) {
	root := fixtureProject(t)
	manifest, body, err := LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	enabled := EnabledConditions(manifest)
	got := ConditionOrder(enabled)
	want := []string{"lint-rules", "dry", "anti-reward-hacking"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("condition order = %#v, want %#v", got, want)
	}
	if HashBytes(body) == "" {
		t.Fatal("manifest hash should be non-empty")
	}
	if len(manifest.LintCommands) != 1 || manifest.LintCommands[0].ID != "go-test" {
		t.Fatalf("lint commands not loaded: %#v", manifest.LintCommands)
	}
}

func TestLoadManifestRejectsInvalidVerifierPolicy(t *testing.T) {
	root := fixtureProject(t)
	writeFile(t, root, ManifestPath, `conditions:
  - id: lint-rules
    path: backpressure/lint-rules.md
    enabled: true
    verifier_policy: almost_independent
`)
	_, _, err := LoadManifest(root)
	if err == nil || !strings.Contains(err.Error(), "invalid verifier_policy") {
		t.Fatalf("LoadManifest error = %v, want invalid verifier_policy", err)
	}
}

func TestLoadManifestLoadsLintRunDirectoryAndCoverage(t *testing.T) {
	root := fixtureProject(t)
	writeFile(t, root, ManifestPath, `conditions:
  - id: lint-rules
    path: backpressure/lint-rules.md
    enabled: true
lint_commands:
  - id: web-lint
    command: "npm run lint"
    required: true
    paths: ["apps/web"]
    timeout_seconds: 120
    run_directory: "apps/./web"
    serial: true
lint_coverage:
  declined_roots: ["services/./api"]
  declined_at: "2026-07-02"
`)
	manifest, _, err := LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := manifest.LintCommands[0].RunDirectory; got != "apps/web" {
		t.Fatalf("run_directory = %q", got)
	}
	if !manifest.LintCommands[0].Serial {
		t.Fatalf("serial flag not loaded: %#v", manifest.LintCommands[0])
	}
	if got := manifest.LintCoverage.DeclinedRoots; !reflect.DeepEqual(got, []string{"services/api"}) {
		t.Fatalf("declined roots = %#v", got)
	}
}

func TestLoadManifestLeavesAbsentLintRunDirectoryEmpty(t *testing.T) {
	root := fixtureProject(t)
	writeFile(t, root, ManifestPath, `conditions:
  - id: lint-rules
    path: backpressure/lint-rules.md
    enabled: true
lint_commands:
  - id: repo-lint
    command: "go test ./..."
    required: true
    paths: ["."]
    timeout_seconds: 120
`)
	manifest, _, err := LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := manifest.LintCommands[0].RunDirectory; got != "" {
		t.Fatalf("absent run_directory = %q, want empty", got)
	}
}

func TestLoadManifestRejectsUnknownLintCommandKey(t *testing.T) {
	root := fixtureProject(t)
	writeFile(t, root, ManifestPath, `conditions:
  - id: lint-rules
    path: backpressure/lint-rules.md
    enabled: true
lint_commands:
  - id: typo
    command: "true"
    required: true
    paths: ["."]
    timeout_seconds: 5
    run_dir: "."
`)
	_, _, err := LoadManifest(root)
	if err == nil || !strings.Contains(err.Error(), "run_dir is not supported") {
		t.Fatalf("LoadManifest error = %v, want unsupported run_dir", err)
	}
}

func TestLoadManifestRejectsUnknownConditionKey(t *testing.T) {
	root := fixtureProject(t)
	writeFile(t, root, ManifestPath, `conditions:
  - id: lint-rules
    path: backpressure/lint-rules.md
    enabled: true
    owner: platform
`)
	_, _, err := LoadManifest(root)
	if err == nil || !strings.Contains(err.Error(), "owner is not supported") {
		t.Fatalf("LoadManifest error = %v, want unsupported owner", err)
	}
}

func TestHashStagedPayloadExcludesGeneratedArtifacts(t *testing.T) {
	staged := fakeStaged{
		paths: []string{
			"backpressure/attestations/README.md",
			"backpressure/attestations/hash.json",
			"cmd/app/main.go",
			"log/backpressure/failed/README.md",
			"log/backpressure/failed/blocked.json",
		},
		content: map[string]string{
			"backpressure/attestations/README.md": "tracked passing attestation docs\n",
			"cmd/app/main.go":                     "package main\n",
			"log/backpressure/failed/README.md":   "tracked blocked report docs\n",
		},
	}
	payload, err := HashStagedPayload(context.Background(), "/repo", staged)
	if err != nil {
		t.Fatal(err)
	}
	wantIncluded := []string{
		"backpressure/attestations/README.md",
		"cmd/app/main.go",
		"log/backpressure/failed/README.md",
	}
	if !reflect.DeepEqual(payload.IncludedPaths, wantIncluded) {
		t.Fatalf("included paths = %#v, want %#v", payload.IncludedPaths, wantIncluded)
	}
	wantExcluded := []string{"backpressure/attestations/hash.json", "log/backpressure/failed/blocked.json"}
	if !reflect.DeepEqual(payload.ExcludedPaths, wantExcluded) {
		t.Fatalf("excluded paths = %#v, want %#v", payload.ExcludedPaths, wantExcluded)
	}
	changed, err := HashStagedPayload(context.Background(), "/repo", fakeStaged{
		paths: []string{
			"backpressure/attestations/README.md",
			"backpressure/attestations/other.json",
			"cmd/app/main.go",
			"log/backpressure/failed/README.md",
		},
		content: map[string]string{
			"backpressure/attestations/README.md": "tracked passing attestation docs\n",
			"cmd/app/main.go":                     "package main\n",
			"log/backpressure/failed/README.md":   "tracked blocked report docs\n",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if payload.Hash != changed.Hash {
		t.Fatalf("generated artifacts changed payload hash: %s vs %s", payload.Hash, changed.Hash)
	}
}

func TestHashStagedPayloadIncludesGeneratedDirectoryReadmes(t *testing.T) {
	base, err := HashStagedPayload(context.Background(), "/repo", fakeStaged{
		paths: []string{"cmd/app/main.go"},
		content: map[string]string{
			"cmd/app/main.go": "package main\n",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	withReadmes, err := HashStagedPayload(context.Background(), "/repo", fakeStaged{
		paths: []string{
			"backpressure/attestations/README.md",
			"cmd/app/main.go",
			"log/backpressure/failed/README.md",
		},
		content: map[string]string{
			"backpressure/attestations/README.md": "tracked passing attestation docs\n",
			"cmd/app/main.go":                     "package main\n",
			"log/backpressure/failed/README.md":   "tracked blocked report docs\n",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if base.Hash == withReadmes.Hash {
		t.Fatal("tracked README files under generated evidence directories should affect payload hash")
	}
	for _, path := range []string{"backpressure/attestations/README.md", "log/backpressure/failed/README.md"} {
		if !containsString(withReadmes.IncludedPaths, path) {
			t.Fatalf("%s missing from included paths: %#v", path, withReadmes.IncludedPaths)
		}
		if containsString(withReadmes.ExcludedPaths, path) {
			t.Fatalf("%s should not be hash-excluded: %#v", path, withReadmes.ExcludedPaths)
		}
	}
}

func TestHashStagedPayloadHandlesDeletedRenamedAndShellSensitivePaths(t *testing.T) {
	staged := &fakeStagedEntries{
		entries: []StagedPayloadFile{
			{Path: "cmd/deleted.go", Status: "deleted", GitStatus: "D"},
			{Path: "cmd/new name.go", OldPath: "cmd/old name.go", Status: "renamed", GitStatus: "R100"},
			{Path: "cmd/--flag.go", Status: "modified", GitStatus: "M"},
			{Path: "cmd/tab\tname.go", Status: "modified", GitStatus: "M"},
			{Path: "cmd/line\nbreak.go", Status: "added", GitStatus: "A"},
		},
		content: map[string]string{
			"cmd/new name.go":    "package cmd\nconst Renamed = true\n",
			"cmd/--flag.go":      "package cmd\nconst Flag = true\n",
			"cmd/tab\tname.go":   "package cmd\nconst Tab = true\n",
			"cmd/line\nbreak.go": "package cmd\nconst Line = true\n",
		},
	}
	payload, err := HashStagedPayload(context.Background(), "/repo", staged)
	if err != nil {
		t.Fatal(err)
	}
	if containsString(staged.reads, "cmd/deleted.go") {
		t.Fatalf("deleted path was read from git index: %#v", staged.reads)
	}
	wantPaths := []string{"cmd/--flag.go", "cmd/deleted.go", "cmd/line\nbreak.go", "cmd/new name.go", "cmd/tab\tname.go"}
	if !reflect.DeepEqual(payload.IncludedPaths, wantPaths) {
		t.Fatalf("included paths = %#v, want %#v", payload.IncludedPaths, wantPaths)
	}
	var renamed StagedPayloadFile
	for _, file := range payload.IncludedFiles {
		if file.Status == "renamed" {
			renamed = file
			break
		}
	}
	if renamed.OldPath != "cmd/old name.go" || renamed.Path != "cmd/new name.go" {
		t.Fatalf("rename metadata not preserved: %#v", payload.IncludedFiles)
	}

	withoutDelete := &fakeStagedEntries{
		entries: []StagedPayloadFile{
			{Path: "cmd/new name.go", OldPath: "cmd/old name.go", Status: "renamed", GitStatus: "R100"},
			{Path: "cmd/--flag.go", Status: "modified", GitStatus: "M"},
			{Path: "cmd/tab\tname.go", Status: "modified", GitStatus: "M"},
			{Path: "cmd/line\nbreak.go", Status: "added", GitStatus: "A"},
		},
		content: staged.content,
	}
	changed, err := HashStagedPayload(context.Background(), "/repo", withoutDelete)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Hash == changed.Hash {
		t.Fatal("deleted path metadata should affect payload hash")
	}
}

func TestGitStagedReaderHandlesRealRenameAndSpecialPaths(t *testing.T) {
	first := fixtureGitRenameAndSpecialPaths(t)
	second := fixtureGitRenameAndSpecialPaths(t)

	firstPayload, err := HashStagedPayload(context.Background(), first, GitStagedReader{})
	if err != nil {
		t.Fatal(err)
	}
	secondPayload, err := HashStagedPayload(context.Background(), second, GitStagedReader{})
	if err != nil {
		t.Fatal(err)
	}
	if firstPayload.Hash != secondPayload.Hash {
		t.Fatalf("same staged rename payload should hash stably:\n%s\n%s", firstPayload.Hash, secondPayload.Hash)
	}
	assertPayloadFile(t, firstPayload.IncludedFiles, "src/new name.go", "renamed", "src/old name.go")
	assertPayloadFile(t, firstPayload.IncludedFiles, "src/--flag.go", "modified", "")
	assertPayloadFile(t, firstPayload.IncludedFiles, "src/tab\tname.go", "modified", "")
	assertPayloadFile(t, firstPayload.IncludedFiles, "src/line\nbreak.go", "added", "")
}

func TestGitCommittedReaderHandlesDeleteAndRename(t *testing.T) {
	root := fixtureCommittedDeleteAndRename(t)
	payload, err := HashStagedPayload(context.Background(), root, GitCommittedReader{})
	if err != nil {
		t.Fatal(err)
	}
	assertPayloadFile(t, payload.IncludedFiles, "src/new.go", "renamed", "src/old.go")
	assertPayloadFile(t, payload.IncludedFiles, "src/delete.go", "deleted", "")
}

func TestGitCommittedReaderTargetsSpecificCommit(t *testing.T) {
	root := fixtureCommittedSequence(t)
	first := gitOutputCore(t, root, "rev-parse", "HEAD~1")
	payload, err := HashStagedPayload(context.Background(), root, GitCommittedReader{Commit: first})
	if err != nil {
		t.Fatal(err)
	}
	assertPayloadFile(t, payload.IncludedFiles, "src/one.go", "added", "")
	if containsString(payload.IncludedPaths, "src/two.go") {
		t.Fatalf("targeted reader included later commit path: %#v", payload.IncludedPaths)
	}
	body, err := (GitCommittedReader{Commit: first}).StagedFileContent(context.Background(), root, "src/one.go")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "package src\nconst One = true\n" {
		t.Fatalf("targeted file content = %q", body)
	}
}

func TestGitCommittedReaderRootCommitUsesEmptyTree(t *testing.T) {
	root := fixtureCommittedSequence(t)
	rootCommit := gitOutputCore(t, root, "rev-list", "--max-parents=0", "HEAD")
	payload, err := HashStagedPayload(context.Background(), root, GitCommittedReader{Commit: rootCommit})
	if err != nil {
		t.Fatal(err)
	}
	assertPayloadFile(t, payload.IncludedFiles, "README.md", "added", "")
}

func TestGitCommittedReaderMergeCommitUsesFirstParent(t *testing.T) {
	root := fixtureMergeCommit(t)
	payload, err := HashStagedPayload(context.Background(), root, GitCommittedReader{Commit: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	assertPayloadFile(t, payload.IncludedFiles, "src/branch.go", "added", "")
	if containsString(payload.IncludedPaths, "src/main.go") {
		t.Fatalf("merge reader should use first parent, got paths %#v", payload.IncludedPaths)
	}
}

func TestHashConditionFiles(t *testing.T) {
	root := fixtureProject(t)
	manifest, _, err := LoadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	hashes, err := HashConditionFiles(root, EnabledConditions(manifest))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"lint-rules", "dry", "anti-reward-hacking"} {
		if !strings.HasPrefix(hashes[id], HashPrefix) {
			t.Fatalf("missing hash for %s in %#v", id, hashes)
		}
	}
}

func TestDetectFeaturesExplicitFlag(t *testing.T) {
	features, err := DetectFeatures(FeatureOptions{
		ExplicitFeature: "burpvalve-prs-09a",
		StagedPaths:     []string{"cmd/burpvalve/main.go", "internal/backpressure/core.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := features[0]; got.ID != "burpvalve-prs-09a" || got.SourceBead != "" || got.DiffCluster != "explicit:burpvalve-prs-09a" {
		t.Fatalf("explicit feature not preserved: %#v", got)
	}
}

func TestDetectFeaturesBlocksAmbiguousOrMultipleClusters(t *testing.T) {
	if _, err := DetectFeatures(FeatureOptions{}); err == nil || !strings.Contains(err.Error(), "no staged payload paths") {
		t.Fatalf("empty staged paths should block, got %v", err)
	}
	_, err := DetectFeatures(FeatureOptions{StagedPaths: []string{"cmd/app/main.go", "internal/app/app.go"}})
	if err == nil || !strings.Contains(err.Error(), "multiple diff clusters") {
		t.Fatalf("multiple clusters should block, got %v", err)
	}
}

func TestBuildMatrixSize(t *testing.T) {
	features := []Feature{
		{ID: "feature-a", Name: "A"},
		{ID: "feature-b", Name: "B"},
	}
	conditions := []ConditionSpec{
		{ID: "dry", Path: "backpressure/dry.md", Enabled: true},
		{ID: "scope-control", Path: "backpressure/scope-control.md", Enabled: true},
		{ID: "data-integrity", Path: "backpressure/data-integrity.md", Enabled: true},
	}
	matrix := BuildMatrix(features, conditions)
	if got, want := len(matrix.Cells), 6; got != want {
		t.Fatalf("matrix cells = %d, want %d", got, want)
	}
	if matrix.Cells[0].FeatureID != "feature-a" || matrix.Cells[0].ConditionID != "dry" {
		t.Fatalf("matrix order changed: %#v", matrix.Cells)
	}
}

func TestBuildPlanRoutesCore(t *testing.T) {
	root := fixtureProject(t)
	plan, err := BuildPlan(context.Background(), Options{
		Root:            root,
		Mode:            "pre-commit",
		ExplicitFeature: "br-123",
		Staged: fakeStaged{
			paths: []string{"cmd/app/main.go", "backpressure/attestations/generated.json"},
			content: map[string]string{
				"cmd/app/main.go": "package main\n",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != "pre-commit" || plan.Features[0].ID != "br-123" {
		t.Fatalf("unexpected plan identity: %#v", plan)
	}
	if len(plan.Matrix.Cells) != 3 {
		t.Fatalf("matrix cells = %d, want 3", len(plan.Matrix.Cells))
	}
	if plan.StagedPayloadPaths[0] != "cmd/app/main.go" || plan.ExcludedStagedPaths[0] != "backpressure/attestations/generated.json" {
		t.Fatalf("payload paths wrong: %#v excluded %#v", plan.StagedPayloadPaths, plan.ExcludedStagedPaths)
	}
	if ExpectedBinding(plan).StagedPayloadHash != plan.StagedPayloadHash {
		t.Fatal("expected binding did not use plan payload hash")
	}
}

func fixtureGitRenameAndSpecialPaths(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGitCore(t, root, "init", "-q")
	runGitCore(t, root, "config", "user.email", "michael-bltzr@users.noreply.github.com")
	runGitCore(t, root, "config", "user.name", "Test User")
	writeFile(t, root, "src/old name.go", "package src\nconst Rename = true\n")
	writeFile(t, root, "src/--flag.go", "package src\nconst Flag = false\n")
	writeFile(t, root, "src/tab\tname.go", "package src\nconst Tab = false\n")
	runGitCore(t, root, "add", "--", "src/old name.go", "src/--flag.go", "src/tab\tname.go")
	runGitCore(t, root, "commit", "-q", "--no-verify", "-m", "baseline")

	runGitCore(t, root, "mv", "--", "src/old name.go", "src/new name.go")
	writeFile(t, root, "src/--flag.go", "package src\nconst Flag = true\n")
	writeFile(t, root, "src/tab\tname.go", "package src\nconst Tab = true\n")
	writeFile(t, root, "src/line\nbreak.go", "package src\nconst Line = true\n")
	runGitCore(t, root, "add", "--", "src/--flag.go", "src/tab\tname.go", "src/line\nbreak.go")
	return root
}

func fixtureCommittedDeleteAndRename(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGitCore(t, root, "init", "-q")
	runGitCore(t, root, "config", "user.email", "michael-bltzr@users.noreply.github.com")
	runGitCore(t, root, "config", "user.name", "Test User")
	writeFile(t, root, "src/old.go", "package src\nconst Rename = true\n")
	writeFile(t, root, "src/delete.go", "package src\nconst Delete = true\n")
	runGitCore(t, root, "add", "--", "src/old.go", "src/delete.go")
	runGitCore(t, root, "commit", "-q", "--no-verify", "-m", "baseline")

	runGitCore(t, root, "mv", "--", "src/old.go", "src/new.go")
	if err := os.Remove(filepath.Join(root, "src/delete.go")); err != nil {
		t.Fatal(err)
	}
	runGitCore(t, root, "add", "-A", "--", "src")
	runGitCore(t, root, "commit", "-q", "--no-verify", "-m", "rename and delete")
	return root
}

func fixtureCommittedSequence(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGitCore(t, root, "init", "-q")
	runGitCore(t, root, "config", "user.email", "michael-bltzr@users.noreply.github.com")
	runGitCore(t, root, "config", "user.name", "Test User")
	writeFile(t, root, "README.md", "baseline\n")
	runGitCore(t, root, "add", "README.md")
	runGitCore(t, root, "commit", "-q", "--no-verify", "-m", "root")
	writeFile(t, root, "src/one.go", "package src\nconst One = true\n")
	runGitCore(t, root, "add", "src/one.go")
	runGitCore(t, root, "commit", "-q", "--no-verify", "-m", "one")
	writeFile(t, root, "src/two.go", "package src\nconst Two = true\n")
	runGitCore(t, root, "add", "src/two.go")
	runGitCore(t, root, "commit", "-q", "--no-verify", "-m", "two")
	return root
}

func fixtureMergeCommit(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGitCore(t, root, "init", "-q")
	runGitCore(t, root, "config", "user.email", "michael-bltzr@users.noreply.github.com")
	runGitCore(t, root, "config", "user.name", "Test User")
	writeFile(t, root, "README.md", "baseline\n")
	runGitCore(t, root, "add", "README.md")
	runGitCore(t, root, "commit", "-q", "--no-verify", "-m", "baseline")
	mainBranch := gitOutputCore(t, root, "rev-parse", "--abbrev-ref", "HEAD")
	runGitCore(t, root, "checkout", "-q", "-b", "feature")
	writeFile(t, root, "src/branch.go", "package src\nconst Branch = true\n")
	runGitCore(t, root, "add", "src/branch.go")
	runGitCore(t, root, "commit", "-q", "--no-verify", "-m", "branch")
	runGitCore(t, root, "checkout", "-q", mainBranch)
	writeFile(t, root, "src/main.go", "package src\nconst Main = true\n")
	runGitCore(t, root, "add", "src/main.go")
	runGitCore(t, root, "commit", "-q", "--no-verify", "-m", "main")
	runGitCore(t, root, "merge", "--no-ff", "--no-edit", "feature")
	return root
}

func assertPayloadFile(t *testing.T, files []StagedPayloadFile, path, status, oldPath string) {
	t.Helper()
	for _, file := range files {
		if file.Path == path {
			if file.Status != status || file.OldPath != oldPath {
				t.Fatalf("payload file %q = %#v, want status=%q old_path=%q", path, file, status, oldPath)
			}
			return
		}
	}
	t.Fatalf("payload file %q missing from %#v", path, files)
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func runGitCore(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func gitOutputCore(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s failed: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(output))
}

func fixtureProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, ManifestPath, `conditions:
  - id: lint-rules
    path: backpressure/lint-rules.md
    enabled: true
  - id: disabled-condition
    path: backpressure/disabled.md
    enabled: false
  - id: dry
    path: backpressure/dry.md
    enabled: true
  - id: anti-reward-hacking
    path: backpressure/anti-reward-hacking.md
    enabled: true
lint_commands:
  - id: go-test
    command: "go test ./..."
    required: true
    paths: ["."]
    timeout_seconds: 120
`)
	writeFile(t, root, "backpressure/lint-rules.md", "# Lint\n")
	writeFile(t, root, "backpressure/dry.md", "# DRY\n")
	writeFile(t, root, "backpressure/anti-reward-hacking.md", "# Anti\n")
	return root
}

func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
