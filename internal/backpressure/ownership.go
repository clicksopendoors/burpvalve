package backpressure

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

type OwnershipKind string

const (
	OwnershipKindWholePath OwnershipKind = "whole_path"
	OwnershipKindFunction  OwnershipKind = "function"
	OwnershipKindTest      OwnershipKind = "test"
	OwnershipKindHunk      OwnershipKind = "hunk"
	OwnershipKindGenerated OwnershipKind = "generated"
	OwnershipKindAdmin     OwnershipKind = "admin"
	OwnershipKindException OwnershipKind = "exception"
)

type OwnershipExpirationScope string

const (
	OwnershipScopeSingleBead  OwnershipExpirationScope = "single_bead"
	OwnershipScopePlanRound   OwnershipExpirationScope = "plan_round"
	OwnershipScopeUntilCommit OwnershipExpirationScope = "until_commit"
)

type OwnershipStatus string

const (
	OwnershipStatusOwned              OwnershipStatus = "owned"
	OwnershipStatusSharedDeclared     OwnershipStatus = "shared_declared"
	OwnershipStatusConflict           OwnershipStatus = "conflict"
	OwnershipStatusUnowned            OwnershipStatus = "unowned"
	OwnershipStatusGeneratedException OwnershipStatus = "generated_exception"
	OwnershipStatusIgnoredUntracked   OwnershipStatus = "ignored_untracked"
	OwnershipStatusCoveredException   OwnershipStatus = "covered_exception"
)

type OwnershipRecord struct {
	UnitID         string                   `json:"unit_id"`
	BeadID         string                   `json:"bead_id,omitempty"`
	Path           string                   `json:"path"`
	OwnershipKind  OwnershipKind            `json:"ownership_kind"`
	Symbol         string                   `json:"symbol,omitempty"`
	TestPattern    string                   `json:"test_pattern,omitempty"`
	HunkLabel      string                   `json:"hunk_label,omitempty"`
	Source         string                   `json:"source"`
	Rationale      string                   `json:"rationale,omitempty"`
	ExpiresOrScope OwnershipExpirationScope `json:"expires_or_scope,omitempty"`
	ExpiresNote    string                   `json:"expires_note,omitempty"`
}

type OwnershipInput struct {
	Records []OwnershipRecord `json:"records"`
}

type OwnershipPathResult struct {
	Path         string                 `json:"path"`
	Status       OwnershipStatus        `json:"status"`
	PathState    string                 `json:"path_state,omitempty"`
	GitStatus    string                 `json:"git_status,omitempty"`
	Ignored      bool                   `json:"ignored,omitempty"`
	Generated    bool                   `json:"generated,omitempty"`
	Owners       []OwnershipRecord      `json:"owners,omitempty"`
	OwnerUnitIDs []string               `json:"owner_unit_ids,omitempty"`
	BeadsContext []OwnershipBeadContext `json:"beads_context,omitempty"`
	Rationale    string                 `json:"rationale,omitempty"`
	Source       string                 `json:"source,omitempty"`
}

func ParseOwnershipInput(r io.Reader) (OwnershipInput, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return OwnershipInput{}, fmt.Errorf("read ownership input: %w", err)
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return OwnershipInput{}, fmt.Errorf("ownership input is empty")
	}

	var input OwnershipInput
	switch body[0] {
	case '[':
		var records []OwnershipRecord
		if err := decodeStrict(body, &records); err != nil {
			return OwnershipInput{}, err
		}
		input.Records = records
	case '{':
		if err := decodeStrict(body, &input); err != nil {
			return OwnershipInput{}, err
		}
	default:
		return OwnershipInput{}, fmt.Errorf("ownership input must be a JSON object or array")
	}
	if err := ValidateOwnershipInput(&input); err != nil {
		return OwnershipInput{}, err
	}
	return input, nil
}

func ValidateOwnershipInput(input *OwnershipInput) error {
	if input == nil {
		return fmt.Errorf("ownership input is nil")
	}
	normalized := make([]OwnershipRecord, len(input.Records))
	for i, record := range input.Records {
		if err := validateOwnershipRecord(record); err != nil {
			return fmt.Errorf("records[%d]: %w", i, err)
		}
		path, err := NormalizeOwnershipPath(record.Path)
		if err != nil {
			return fmt.Errorf("records[%d]: %w", i, err)
		}
		record.Path = path
		normalized[i] = record
	}
	for i, record := range normalized {
		if needsOwnershipRationale(record, normalized) && strings.TrimSpace(record.Rationale) == "" {
			return fmt.Errorf("records[%d]: rationale is required for %s ownership on shared or exception paths", i, record.OwnershipKind)
		}
	}
	input.Records = normalized
	return nil
}

func MergeOwnershipInputs(fileInput, stdinInput OwnershipInput) OwnershipInput {
	merged := make(map[ownershipClaimKey]OwnershipRecord)
	order := make([]ownershipClaimKey, 0, len(fileInput.Records)+len(stdinInput.Records))
	appendRecord := func(record OwnershipRecord) {
		key := ownershipClaimKeyFor(record)
		if _, ok := merged[key]; !ok {
			order = append(order, key)
		}
		merged[key] = record
	}
	for _, record := range fileInput.Records {
		appendRecord(record)
	}
	for _, record := range stdinInput.Records {
		appendRecord(record)
	}
	records := make([]OwnershipRecord, 0, len(order))
	for _, key := range order {
		records = append(records, merged[key])
	}
	return OwnershipInput{Records: records}
}

func NormalizeOwnershipPath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("path is required")
	}
	slashed := strings.ReplaceAll(trimmed, "\\", "/")
	if filepath.IsAbs(trimmed) || strings.HasPrefix(slashed, "/") {
		return "", fmt.Errorf("path %q must be repo-relative", path)
	}
	parts := strings.Split(slashed, "/")
	for _, part := range parts {
		if part == ".." {
			return "", fmt.Errorf("path %q must not contain traversal", path)
		}
	}
	cleaned := filepath.ToSlash(filepath.Clean(slashed))
	if cleaned == "." || cleaned == "" {
		return "", fmt.Errorf("path is required")
	}
	if strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", fmt.Errorf("path %q must not contain traversal", path)
	}
	return cleaned, nil
}

func OwnershipKindValid(kind OwnershipKind) bool {
	switch kind {
	case OwnershipKindWholePath, OwnershipKindFunction, OwnershipKindTest, OwnershipKindHunk, OwnershipKindGenerated, OwnershipKindAdmin, OwnershipKindException:
		return true
	default:
		return false
	}
}

func OwnershipExpirationScopeValid(scope OwnershipExpirationScope) bool {
	switch scope {
	case "", OwnershipScopeSingleBead, OwnershipScopePlanRound, OwnershipScopeUntilCommit:
		return true
	default:
		return false
	}
}

func OwnershipStatuses() []OwnershipStatus {
	return []OwnershipStatus{
		OwnershipStatusOwned,
		OwnershipStatusSharedDeclared,
		OwnershipStatusConflict,
		OwnershipStatusUnowned,
		OwnershipStatusGeneratedException,
		OwnershipStatusIgnoredUntracked,
		OwnershipStatusCoveredException,
	}
}

func validateOwnershipRecord(record OwnershipRecord) error {
	if strings.TrimSpace(record.UnitID) == "" {
		return fmt.Errorf("unit_id is required")
	}
	if strings.TrimSpace(record.Path) == "" {
		return fmt.Errorf("path is required")
	}
	if !OwnershipKindValid(record.OwnershipKind) {
		return fmt.Errorf("invalid ownership_kind %q", record.OwnershipKind)
	}
	if strings.TrimSpace(record.Source) == "" {
		return fmt.Errorf("source is required")
	}
	if !OwnershipExpirationScopeValid(record.ExpiresOrScope) {
		return fmt.Errorf("invalid expires_or_scope %q", record.ExpiresOrScope)
	}
	return nil
}

func needsOwnershipRationale(record OwnershipRecord, records []OwnershipRecord) bool {
	if record.OwnershipKind == OwnershipKindException {
		return true
	}
	for _, other := range records {
		if other.Path == record.Path && other.UnitID != record.UnitID {
			return true
		}
	}
	return false
}

type ownershipClaimKey struct {
	unitID string
	path   string
	kind   OwnershipKind
}

func ownershipClaimKeyFor(record OwnershipRecord) ownershipClaimKey {
	return ownershipClaimKey{
		unitID: strings.TrimSpace(record.UnitID),
		path:   record.Path,
		kind:   record.OwnershipKind,
	}
}

func decodeStrict(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode ownership input: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("decode ownership input: multiple JSON values are not supported")
	}
	return nil
}
