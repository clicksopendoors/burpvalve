package backpressure

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseOwnershipInputRejectsMissingUnitID(t *testing.T) {
	_, err := ParseOwnershipInput(strings.NewReader(`[{
		"path": "cmd/burpvalve/main.go",
		"ownership_kind": "whole_path",
		"source": "stdin"
	}]`))
	if err == nil || !strings.Contains(err.Error(), "unit_id is required") {
		t.Fatalf("ParseOwnershipInput error = %v, want missing unit_id", err)
	}
}

func TestParseOwnershipInputRejectsMissingPath(t *testing.T) {
	_, err := ParseOwnershipInput(strings.NewReader(`[{
		"unit_id": "ifr2-c1",
		"ownership_kind": "whole_path",
		"source": "stdin"
	}]`))
	if err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("ParseOwnershipInput error = %v, want missing path", err)
	}
}

func TestParseOwnershipInputRejectsInvalidOwnershipKind(t *testing.T) {
	_, err := ParseOwnershipInput(strings.NewReader(`[{
		"unit_id": "ifr2-c1",
		"path": "internal/backpressure/ownership.go",
		"ownership_kind": "everything",
		"source": "stdin"
	}]`))
	if err == nil || !strings.Contains(err.Error(), "invalid ownership_kind") {
		t.Fatalf("ParseOwnershipInput error = %v, want invalid ownership_kind", err)
	}
}

func TestParseOwnershipInputRejectsInvalidExpiresOrScope(t *testing.T) {
	_, err := ParseOwnershipInput(strings.NewReader(`[{
		"unit_id": "ifr2-c1",
		"path": "internal/backpressure/ownership.go",
		"ownership_kind": "whole_path",
		"source": "stdin",
		"expires_or_scope": "eventually"
	}]`))
	if err == nil || !strings.Contains(err.Error(), "invalid expires_or_scope") {
		t.Fatalf("ParseOwnershipInput error = %v, want invalid expires_or_scope", err)
	}
}

func TestParseOwnershipInputAcceptsStructuredExpirationScopes(t *testing.T) {
	for _, scope := range []OwnershipExpirationScope{
		OwnershipScopeSingleBead,
		OwnershipScopePlanRound,
		OwnershipScopeUntilCommit,
	} {
		t.Run(string(scope), func(t *testing.T) {
			input, err := ParseOwnershipInput(strings.NewReader(`{
				"records": [{
					"unit_id": "ifr2-c1",
					"path": "internal/backpressure/ownership.go",
					"ownership_kind": "whole_path",
					"source": "plan",
					"expires_or_scope": "` + string(scope) + `",
					"expires_note": "valid until the named boundary"
				}]
			}`))
			if err != nil {
				t.Fatalf("ParseOwnershipInput error = %v", err)
			}
			got := input.Records[0]
			if got.ExpiresOrScope != scope || got.ExpiresNote == "" {
				t.Fatalf("expiration fields = %#v", got)
			}
		})
	}
}

func TestParseOwnershipInputRejectsExceptionWithoutRationale(t *testing.T) {
	_, err := ParseOwnershipInput(strings.NewReader(`[{
		"unit_id": "ifr2-c1",
		"path": "backpressure/attestations/example.json",
		"ownership_kind": "exception",
		"source": "review-packet"
	}]`))
	if err == nil || !strings.Contains(err.Error(), "rationale is required") {
		t.Fatalf("ParseOwnershipInput error = %v, want rationale requirement", err)
	}
}

func TestParseOwnershipInputRejectsSharedPathWithoutRationale(t *testing.T) {
	_, err := ParseOwnershipInput(strings.NewReader(`[{
		"unit_id": "ifr2-c1",
		"path": "cmd/burpvalve/main.go",
		"ownership_kind": "function",
		"symbol": "runAccountPayload",
		"source": "plan"
	}, {
		"unit_id": "cos-route",
		"path": "cmd/burpvalve/main.go",
		"ownership_kind": "function",
		"symbol": "runInit",
		"source": "plan",
		"rationale": "shared CLI surface"
	}]`))
	if err == nil || !strings.Contains(err.Error(), "rationale is required") {
		t.Fatalf("ParseOwnershipInput error = %v, want shared path rationale requirement", err)
	}
}

func TestParseOwnershipInputNormalizesRepoRelativePaths(t *testing.T) {
	input, err := ParseOwnershipInput(strings.NewReader(`[{
		"unit_id": "ifr2-c1",
		"path": "internal/./backpressure//ownership.go",
		"ownership_kind": "whole_path",
		"source": "stdin"
	}]`))
	if err != nil {
		t.Fatalf("ParseOwnershipInput error = %v", err)
	}
	if got := input.Records[0].Path; got != "internal/backpressure/ownership.go" {
		t.Fatalf("normalized path = %q", got)
	}
}

func TestParseOwnershipInputRejectsAbsoluteAndTraversalPaths(t *testing.T) {
	for _, path := range []string{"/example/file", "../outside", "internal/../outside", `internal\..\outside`} {
		t.Run(path, func(t *testing.T) {
			_, err := ParseOwnershipInput(strings.NewReader(`[{
				"unit_id": "ifr2-c1",
				"path": "` + strings.ReplaceAll(path, `\`, `\\`) + `",
				"ownership_kind": "whole_path",
				"source": "stdin"
			}]`))
			if err == nil || !strings.Contains(err.Error(), "path") {
				t.Fatalf("ParseOwnershipInput error = %v, want path rejection", err)
			}
		})
	}
}

func TestParseOwnershipInputPreservesSourceForAudit(t *testing.T) {
	input, err := ParseOwnershipInput(strings.NewReader(`[{
		"unit_id": "ifr2-c1",
		"bead_id": "burpvalve-ifr2-c1-ownership-schema-contract-vr1",
		"path": "internal/backpressure/ownership.go",
		"ownership_kind": "whole_path",
		"source": "plans/issue-followups-round-2.md"
	}]`))
	if err != nil {
		t.Fatalf("ParseOwnershipInput error = %v", err)
	}
	got := input.Records[0]
	if got.Source != "plans/issue-followups-round-2.md" || got.BeadID == "" {
		t.Fatalf("audit fields not preserved: %#v", got)
	}
}

func TestMergeOwnershipInputsLetsStdinOverrideSameClaim(t *testing.T) {
	fileInput := OwnershipInput{Records: []OwnershipRecord{{
		UnitID:        "ifr2-c1",
		Path:          "internal/backpressure/ownership.go",
		OwnershipKind: OwnershipKindWholePath,
		Source:        "ownership-file",
		Rationale:     "file claim",
	}}}
	stdinInput := OwnershipInput{Records: []OwnershipRecord{{
		UnitID:        "ifr2-c1",
		Path:          "internal/backpressure/ownership.go",
		OwnershipKind: OwnershipKindWholePath,
		Source:        "stdin",
		Rationale:     "stdin claim",
	}}}
	merged := MergeOwnershipInputs(fileInput, stdinInput)
	if len(merged.Records) != 1 || merged.Records[0].Source != "stdin" {
		t.Fatalf("merged records = %#v, want stdin override", merged.Records)
	}
}

func TestOwnershipStatusesAreRoundOneContract(t *testing.T) {
	want := []OwnershipStatus{
		OwnershipStatusOwned,
		OwnershipStatusSharedDeclared,
		OwnershipStatusConflict,
		OwnershipStatusUnowned,
		OwnershipStatusGeneratedException,
		OwnershipStatusIgnoredUntracked,
		OwnershipStatusCoveredException,
	}
	if got := OwnershipStatuses(); !reflect.DeepEqual(got, want) {
		t.Fatalf("OwnershipStatuses = %#v, want %#v", got, want)
	}
}
