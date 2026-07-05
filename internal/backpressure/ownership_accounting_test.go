package backpressure

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type fakeUntrackedReader struct {
	files []OwnershipUntrackedFile
	err   error
}

type fakeOwnershipBeadsReader struct {
	context OwnershipBeadsContext
	err     error
}

func (f fakeUntrackedReader) UntrackedFiles(context.Context, string) ([]OwnershipUntrackedFile, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]OwnershipUntrackedFile(nil), f.files...), nil
}

func (f fakeOwnershipBeadsReader) ActiveBeads(context.Context, string) (OwnershipBeadsContext, error) {
	if f.err != nil {
		return OwnershipBeadsContext{}, f.err
	}
	return f.context, nil
}

func TestRunOwnershipAccountingReportsOwnedStagedPath(t *testing.T) {
	result := runOwnershipAccountingForTest(t, OwnershipInput{Records: []OwnershipRecord{{
		UnitID:        "ifr2-c2",
		Path:          "cmd/burpvalve/main.go",
		OwnershipKind: OwnershipKindWholePath,
		Source:        "stdin",
	}}}, []StagedPayloadFile{{Path: "cmd/burpvalve/main.go", Status: "modified"}}, nil)

	got := singleOwnershipResult(t, result.Staged, "cmd/burpvalve/main.go")
	if got.Status != OwnershipStatusOwned || !reflect.DeepEqual(got.OwnerUnitIDs, []string{"ifr2-c2"}) || got.Source != "stdin" {
		t.Fatalf("owned result = %#v", got)
	}
}

func TestRunOwnershipAccountingReportsUnownedStagedPath(t *testing.T) {
	result := runOwnershipAccountingForTest(t, OwnershipInput{}, []StagedPayloadFile{{Path: "cmd/burpvalve/main.go", Status: "modified"}}, nil)
	got := singleOwnershipResult(t, result.Staged, "cmd/burpvalve/main.go")
	if got.Status != OwnershipStatusUnowned || len(got.Owners) != 0 {
		t.Fatalf("unowned result = %#v", got)
	}
}

func TestRunOwnershipAccountingReportsConflictForIncompatibleOwners(t *testing.T) {
	result := runOwnershipAccountingForTest(t, OwnershipInput{Records: []OwnershipRecord{{
		UnitID:        "ifr2-c2",
		Path:          "cmd/burpvalve/main.go",
		OwnershipKind: OwnershipKindWholePath,
		Source:        "plan",
		Rationale:     "shared cli surface",
	}, {
		UnitID:        "cos-route",
		Path:          "cmd/burpvalve/main.go",
		OwnershipKind: OwnershipKindWholePath,
		Source:        "plan",
		Rationale:     "shared cli surface",
	}}}, []StagedPayloadFile{{Path: "cmd/burpvalve/main.go", Status: "modified"}}, nil)

	got := singleOwnershipResult(t, result.Staged, "cmd/burpvalve/main.go")
	if got.Status != OwnershipStatusConflict || !reflect.DeepEqual(got.OwnerUnitIDs, []string{"cos-route", "ifr2-c2"}) {
		t.Fatalf("conflict result = %#v", got)
	}
}

func TestRunOwnershipAccountingReportsSharedDeclaredForSplitFile(t *testing.T) {
	result := runOwnershipAccountingForTest(t, OwnershipInput{Records: []OwnershipRecord{{
		UnitID:        "ifr2-c2",
		Path:          "cmd/burpvalve/main.go",
		OwnershipKind: OwnershipKindFunction,
		Symbol:        "newAccountPayloadCommand",
		Source:        "plan",
		Rationale:     "declared split-file ownership",
	}, {
		UnitID:        "cos-route",
		Path:          "cmd/burpvalve/main.go",
		OwnershipKind: OwnershipKindTest,
		TestPattern:   "Route",
		Source:        "plan",
		Rationale:     "declared split-file ownership",
	}}}, []StagedPayloadFile{{Path: "cmd/burpvalve/main.go", Status: "modified"}}, nil)

	got := singleOwnershipResult(t, result.Staged, "cmd/burpvalve/main.go")
	if got.Status != OwnershipStatusSharedDeclared || len(got.Owners) != 2 {
		t.Fatalf("shared result = %#v", got)
	}
}

func TestRunOwnershipAccountingReportsGeneratedEvidenceException(t *testing.T) {
	result := runOwnershipAccountingForTest(t, OwnershipInput{}, []StagedPayloadFile{{Path: "backpressure/attestations/generated.json", Status: "added"}}, nil)
	got := singleOwnershipResult(t, result.Staged, "backpressure/attestations/generated.json")
	if got.Status != OwnershipStatusGeneratedException || !got.Generated {
		t.Fatalf("generated result = %#v", got)
	}
}

func TestRunOwnershipAccountingReportsUntrackedStatuses(t *testing.T) {
	result := runOwnershipAccountingForTest(t, OwnershipInput{}, nil, []OwnershipUntrackedFile{
		{Path: "scratch/ignored.tmp", Ignored: true},
		{Path: "src/new.go"},
		{Path: "log/backpressure/failed/generated.json", Ignored: true},
	})

	if got := singleOwnershipResult(t, result.Untracked, "scratch/ignored.tmp"); got.Status != OwnershipStatusIgnoredUntracked || !got.Ignored {
		t.Fatalf("ignored result = %#v", got)
	}
	if got := singleOwnershipResult(t, result.Untracked, "src/new.go"); got.Status != OwnershipStatusUnowned || got.Ignored {
		t.Fatalf("untracked source result = %#v", got)
	}
	if got := singleOwnershipResult(t, result.Untracked, "log/backpressure/failed/generated.json"); got.Status != OwnershipStatusGeneratedException || !got.Generated || !got.Ignored {
		t.Fatalf("ignored generated result = %#v", got)
	}
}

func TestRunOwnershipAccountingReportsCoveredUntrackedException(t *testing.T) {
	result := runOwnershipAccountingForTest(t, OwnershipInput{Records: []OwnershipRecord{{
		UnitID:        "ifr2-c2",
		Path:          "tmp/manual-note.txt",
		OwnershipKind: OwnershipKindException,
		Source:        "review-packet",
		Rationale:     "operator scratch note is intentionally outside payload",
	}}}, nil, []OwnershipUntrackedFile{{Path: "tmp/manual-note.txt"}})

	got := singleOwnershipResult(t, result.Untracked, "tmp/manual-note.txt")
	if got.Status != OwnershipStatusCoveredException || got.Rationale == "" {
		t.Fatalf("covered exception result = %#v", got)
	}
}

func TestRunOwnershipAccountingDoesNotRequireBeadsOrMutate(t *testing.T) {
	result := runOwnershipAccountingForTest(t, OwnershipInput{}, []StagedPayloadFile{{Path: "README.md", Status: "modified"}}, nil)
	if result.Mutating || result.Command != "account payload" || result.Status != "completed" {
		t.Fatalf("result should be read-only and completed: %#v", result)
	}
}

func TestRunOwnershipAccountingBeadsContextIsDisplayOnly(t *testing.T) {
	result, err := RunOwnershipAccounting(context.Background(), OwnershipAccountingOptions{
		Ownership: OwnershipInput{Records: []OwnershipRecord{{
			UnitID:        "burpvalve-ifr2-c3-beads-enrichment-docs-pe7s",
			BeadID:        "burpvalve-ifr2-c3-beads-enrichment-docs-pe7s",
			Path:          "docs/result-contract.md",
			OwnershipKind: OwnershipKindWholePath,
			Source:        "stdin",
		}}},
		IncludeBeads: true,
		Staged: &fakeStagedEntries{entries: []StagedPayloadFile{
			{Path: "README.md", Status: "modified"},
			{Path: "docs/result-contract.md", Status: "modified"},
		}},
		Beads: fakeOwnershipBeadsReader{context: OwnershipBeadsContext{
			Available: true,
			Active: []OwnershipBeadContext{{
				ID:     "burpvalve-ifr2-c3-beads-enrichment-docs-pe7s",
				Title:  "ifr2-C3",
				Status: "in_progress",
			}, {
				ID:     "unreferenced-active",
				Title:  "other active bead",
				Status: "open",
			}},
		}},
	})
	if err != nil {
		t.Fatalf("RunOwnershipAccounting error = %v", err)
	}
	if result.Beads == nil || !result.Beads.DisplayOnly || len(result.Beads.Active) != 2 {
		t.Fatalf("beads context missing or not display-only: %#v", result.Beads)
	}
	owned := singleOwnershipResult(t, result.Staged, "docs/result-contract.md")
	if owned.Status != OwnershipStatusOwned || len(owned.BeadsContext) != 1 || owned.BeadsContext[0].ID != "burpvalve-ifr2-c3-beads-enrichment-docs-pe7s" {
		t.Fatalf("explicitly owned path did not receive display context: %#v", owned)
	}
	unowned := singleOwnershipResult(t, result.Staged, "README.md")
	if unowned.Status != OwnershipStatusUnowned || len(unowned.BeadsContext) != 0 || len(unowned.Owners) != 0 {
		t.Fatalf("unreferenced active bead must not create ownership: %#v", unowned)
	}
}

func TestFileOwnershipBeadsReaderFiltersActiveStatuses(t *testing.T) {
	root := t.TempDir()
	writeOwnershipAccountingTestFile(t, root, ".beads/issues.jsonl", strings.Join([]string{
		`{"id":"open-bead","title":"Open","status":"open","priority":1,"issue_type":"task","labels":["z","a"]}`,
		`{"id":"progress-bead","title":"Progress","status":"in_progress","priority":2}`,
		`{"id":"closed-bead","title":"Closed","status":"closed","priority":1}`,
		`{"id":"deferred-bead","title":"Deferred","status":"deferred","priority":1}`,
		`{"id":"tombstoned-bead","title":"Tombstoned","status":"tombstoned","priority":1}`,
	}, "\n")+"\n")

	context, err := (FileOwnershipBeadsReader{}).ActiveBeads(context.Background(), root)
	if err != nil {
		t.Fatalf("ActiveBeads error = %v", err)
	}
	if !context.Available || !context.DisplayOnly {
		t.Fatalf("unexpected context flags: %#v", context)
	}
	if got := beadIDs(context.Active); !reflect.DeepEqual(got, []string{"open-bead", "progress-bead"}) {
		t.Fatalf("active beads = %#v", got)
	}
	if context.Active[0].Type != "task" {
		t.Fatalf("issue_type should map into display type: %#v", context.Active[0])
	}
	if !reflect.DeepEqual(context.Active[0].Labels, []string{"a", "z"}) {
		t.Fatalf("labels should be sorted: %#v", context.Active[0].Labels)
	}
}

func TestFileOwnershipBeadsReaderUnavailableOutsideBeadsRepo(t *testing.T) {
	context, err := (FileOwnershipBeadsReader{}).ActiveBeads(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("ActiveBeads error = %v", err)
	}
	if context.Available || !context.DisplayOnly || len(context.Warnings) == 0 {
		t.Fatalf("non-Beads repo should degrade with warning: %#v", context)
	}
}

func TestRunOwnershipAccountingPropagatesUntrackedReaderErrors(t *testing.T) {
	_, err := RunOwnershipAccounting(context.Background(), OwnershipAccountingOptions{
		IncludeUntracked: true,
		Staged:           &fakeStagedEntries{},
		Untracked:        fakeUntrackedReader{err: errors.New("git unavailable")},
	})
	if err == nil || err.Error() != "inspect untracked paths: git unavailable" {
		t.Fatalf("RunOwnershipAccounting error = %v", err)
	}
}

func runOwnershipAccountingForTest(t *testing.T, input OwnershipInput, staged []StagedPayloadFile, untracked []OwnershipUntrackedFile) OwnershipAccountingResult {
	t.Helper()
	result, err := RunOwnershipAccounting(context.Background(), OwnershipAccountingOptions{
		Ownership:        input,
		IncludeUntracked: untracked != nil,
		Staged:           &fakeStagedEntries{entries: staged},
		Untracked:        fakeUntrackedReader{files: untracked},
	})
	if err != nil {
		t.Fatalf("RunOwnershipAccounting error = %v", err)
	}
	return result
}

func beadIDs(beads []OwnershipBeadContext) []string {
	ids := make([]string, 0, len(beads))
	for _, bead := range beads {
		ids = append(ids, bead.ID)
	}
	return ids
}

func writeOwnershipAccountingTestFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func singleOwnershipResult(t *testing.T, results []OwnershipPathResult, path string) OwnershipPathResult {
	t.Helper()
	for _, result := range results {
		if result.Path == path {
			return result
		}
	}
	t.Fatalf("path %q not found in %#v", path, results)
	return OwnershipPathResult{}
}
