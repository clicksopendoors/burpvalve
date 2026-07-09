package attestations

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type QueryOptions struct {
	Status  string
	Limit   int
	Feature string
	Bead    string
}

type Record struct {
	SchemaVersion        int               `json:"schema_version"`
	ArtifactType         string            `json:"artifact_type"`
	Status               string            `json:"status"`
	Path                 string            `json:"path"`
	ID                   string            `json:"id,omitempty"`
	FeatureIDs           []string          `json:"feature_ids,omitempty"`
	BeadIDs              []string          `json:"bead_ids,omitempty"`
	LaneID               string            `json:"lane_id,omitempty"`
	LaneAuthorizationRef string            `json:"lane_authorization_ref,omitempty"`
	PayloadHash          string            `json:"payload_hash,omitempty"`
	ManifestHash         string            `json:"manifest_hash,omitempty"`
	ConditionVerdicts    []ConditionRecord `json:"condition_verdicts,omitempty"`
	GeneratedBy          Generator         `json:"generated_by,omitempty"`
	CreatedAt            *time.Time        `json:"created_at,omitempty"`
	Warnings             []string          `json:"warnings,omitempty"`
	ParseWarnings        []string          `json:"parse_warnings,omitempty"`
}

type ConditionRecord struct {
	ConditionID     string                 `json:"condition_id"`
	ConditionFile   string                 `json:"condition_file,omitempty"`
	Verdict         Verdict                `json:"verdict,omitempty"`
	VerifierPolicy  VerifierPolicy         `json:"verifier_policy,omitempty"`
	VerifierKind    VerifierKind           `json:"verifier_kind,omitempty"`
	VerifierAgent   string                 `json:"verifier_agent,omitempty"`
	VerifierModel   string                 `json:"verifier_model,omitempty"`
	Supplemental    []SupplementalVerifier `json:"supplemental_verifiers,omitempty"`
	Adjudication    *ResponseAdjudication  `json:"adjudication,omitempty"`
	HasDisagreement bool                   `json:"has_disagreement,omitempty"`
}

type ShowError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ShowError) Error() string {
	return e.Message
}

func List(root string, opts QueryOptions) ([]Record, error) {
	records, err := scan(root)
	if err != nil {
		return nil, err
	}
	records = filterRecords(records, opts)
	sortRecords(records)
	if opts.Limit > 0 && len(records) > opts.Limit {
		records = records[:opts.Limit]
	}
	return records, nil
}

func Latest(root string, opts QueryOptions) (Record, error) {
	if opts.Limit <= 0 {
		opts.Limit = 1
	}
	records, err := List(root, opts)
	if err != nil {
		return Record{}, err
	}
	for _, record := range records {
		if record.Status != "malformed" {
			return record, nil
		}
	}
	return Record{}, &ShowError{Code: "not_found", Message: "no attestation artifacts found"}
}

func Show(root string, ref string) (Record, error) {
	record, _, err := ShowArtifact(root, ref)
	return record, err
}

func ShowArtifact(root string, ref string) (Record, Artifact, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return Record{}, Artifact{}, &ShowError{Code: "missing_ref", Message: "attestation reference is required"}
	}
	records, err := scan(root)
	if err != nil {
		return Record{}, Artifact{}, err
	}
	matches := matchingRecords(root, records, ref)
	if len(matches) == 0 {
		return Record{}, Artifact{}, &ShowError{Code: "not_found", Message: fmt.Sprintf("no attestation artifact matches %q", ref)}
	}
	if len(matches) > 1 {
		paths := make([]string, 0, len(matches))
		for _, match := range matches {
			paths = append(paths, match.Path)
		}
		sort.Strings(paths)
		return Record{}, Artifact{}, &ShowError{Code: "ambiguous_ref", Message: fmt.Sprintf("attestation reference %q is ambiguous: %s", ref, strings.Join(paths, ", "))}
	}
	record := matches[0]
	if len(record.ParseWarnings) > 0 {
		return Record{}, Artifact{}, &ShowError{Code: "malformed", Message: fmt.Sprintf("attestation artifact %s is malformed: %s", record.Path, strings.Join(record.ParseWarnings, "; "))}
	}
	artifact, err := readArtifact(filepath.Join(defaultRoot(root), filepath.FromSlash(record.Path)))
	if err != nil {
		return Record{}, Artifact{}, &ShowError{Code: "malformed", Message: fmt.Sprintf("attestation artifact %s is malformed: %s", record.Path, err)}
	}
	return record, artifact, nil
}

func scan(root string) ([]Record, error) {
	abs, err := filepath.Abs(defaultRoot(root))
	if err != nil {
		return nil, err
	}
	var records []Record
	for _, spec := range []struct {
		dir          string
		artifactType string
		status       string
	}{
		{dir: "backpressure/attestations", artifactType: "passing_attestation", status: "pass"},
		{dir: "log/backpressure/failed", artifactType: "blocked_report", status: "blocked"},
	} {
		base := filepath.Join(abs, filepath.FromSlash(spec.dir))
		if _, err := os.Stat(base); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				return nil
			}
			rel, err := filepath.Rel(abs, path)
			if err != nil {
				return err
			}
			records = append(records, readRecord(path, filepath.ToSlash(rel), spec.artifactType, spec.status))
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return records, nil
}

func readRecord(path string, rel string, artifactType string, status string) Record {
	record := Record{
		SchemaVersion: 1,
		ArtifactType:  artifactType,
		Status:        status,
		Path:          rel,
		ID:            idFromPath(rel),
	}
	body, err := os.ReadFile(path)
	if err != nil {
		record.Status = "malformed"
		record.ParseWarnings = []string{err.Error()}
		return record
	}
	var artifact Artifact
	if err := json.Unmarshal(body, &artifact); err != nil {
		record.Status = "malformed"
		record.ParseWarnings = []string{err.Error()}
		return record
	}
	if err := validateRecordShape(artifact); err != nil {
		record.Status = "malformed"
		record.ParseWarnings = []string{err.Error()}
		return record
	}
	record.SchemaVersion = artifact.SchemaVersion
	switch artifact.ArtifactKind {
	case ArtifactPassing:
		record.ArtifactType = "passing_attestation"
		record.Status = "pass"
	case ArtifactBlocked:
		record.ArtifactType = "blocked_report"
		record.Status = "blocked"
	default:
		record.Status = "malformed"
		record.ParseWarnings = append(record.ParseWarnings, "unknown artifact_kind "+string(artifact.ArtifactKind))
		return record
	}
	record.PayloadHash = artifact.StagedPayloadHash
	record.ManifestHash = artifact.ManifestHash
	record.GeneratedBy = artifact.GeneratedBy
	if !artifact.CreatedAt.IsZero() {
		created := artifact.CreatedAt
		record.CreatedAt = &created
	}
	if artifact.Feature.ID != "" {
		record.FeatureIDs = append(record.FeatureIDs, artifact.Feature.ID)
	}
	record.BeadIDs = uniqueStrings(append(record.BeadIDs, artifact.BeadIDs...))
	record.BeadIDs = uniqueStrings(append(record.BeadIDs, artifact.Feature.BeadIDs...))
	if artifact.Feature.SourceBead != "" && looksLikeLegacyBeadID(artifact.Feature.SourceBead) {
		record.BeadIDs = uniqueStrings(append(record.BeadIDs, artifact.Feature.SourceBead))
	}
	if artifact.Atomicity.Lane != nil {
		record.LaneID = strings.TrimSpace(artifact.Atomicity.Lane.LaneID)
		record.LaneAuthorizationRef = strings.TrimSpace(artifact.Atomicity.Lane.AuthorizationRef)
		record.BeadIDs = uniqueStrings(append(record.BeadIDs, artifact.Atomicity.Lane.BeadIDs...))
	}
	for _, condition := range artifact.Conditions {
		record.ConditionVerdicts = append(record.ConditionVerdicts, ConditionRecord{
			ConditionID:     condition.ConditionID,
			ConditionFile:   condition.ConditionFile,
			Verdict:         condition.Verdict,
			VerifierPolicy:  condition.VerifierPolicy,
			VerifierKind:    condition.EffectiveVerifierKind(),
			VerifierAgent:   condition.Verifier.Agent,
			VerifierModel:   condition.Verifier.Model,
			Supplemental:    append([]SupplementalVerifier(nil), condition.Supplemental...),
			Adjudication:    copyAdjudication(condition.Adjudication),
			HasDisagreement: hasSupplementalDisagreement(condition),
		})
	}
	record.Warnings = append(record.Warnings, artifactWarnings(artifact)...)
	return record
}

func copyAdjudication(adjudication *ResponseAdjudication) *ResponseAdjudication {
	if adjudication == nil {
		return nil
	}
	copy := *adjudication
	return &copy
}

func hasSupplementalDisagreement(condition Condition) bool {
	for _, supplemental := range condition.Supplemental {
		if supplemental.Verdict == VerdictFail ||
			supplemental.Verdict == VerdictUnknown ||
			(condition.Verdict != "" && supplemental.Verdict != "" && supplemental.Verdict != condition.Verdict) {
			return true
		}
	}
	return false
}

func validateRecordShape(artifact Artifact) error {
	if artifact.SchemaVersion != 1 {
		return fmt.Errorf("unsupported schema_version %d", artifact.SchemaVersion)
	}
	if artifact.Tool != "" && artifact.Tool != ToolName {
		return fmt.Errorf("unexpected tool %q", artifact.Tool)
	}
	switch artifact.ArtifactKind {
	case ArtifactPassing, ArtifactBlocked:
	default:
		return fmt.Errorf("unknown artifact_kind %q", artifact.ArtifactKind)
	}
	if artifact.StagedPayloadHash == "" {
		return errors.New("staged_payload_hash is required")
	}
	if artifact.ManifestHash == "" {
		return errors.New("manifest_hash is required")
	}
	if artifact.Feature.ID == "" {
		return errors.New("feature.id is required")
	}
	return nil
}

func looksLikeLegacyBeadID(id string) bool {
	id = strings.TrimSpace(id)
	return strings.HasPrefix(id, "br-") || strings.HasPrefix(id, "bd-")
}

func readArtifact(path string) (Artifact, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Artifact{}, err
	}
	var artifact Artifact
	if err := json.Unmarshal(body, &artifact); err != nil {
		return Artifact{}, err
	}
	return artifact, nil
}

func artifactWarnings(artifact Artifact) []string {
	var warnings []string
	seen := map[string]bool{}
	for _, condition := range artifact.Conditions {
		if condition.ConditionID == "" {
			warnings = append(warnings, "condition missing id")
			continue
		}
		if seen[condition.ConditionID] {
			warnings = append(warnings, "duplicate condition "+condition.ConditionID)
		}
		seen[condition.ConditionID] = true
		if condition.ConditionFileHash == "" {
			warnings = append(warnings, "condition "+condition.ConditionID+" missing condition_file_hash")
		}
	}
	for _, id := range artifact.ConditionOrder {
		if !seen[id] {
			warnings = append(warnings, "condition_order includes missing condition "+id)
		}
	}
	return warnings
}

func filterRecords(records []Record, opts QueryOptions) []Record {
	status := strings.ToLower(strings.TrimSpace(opts.Status))
	if status == "" {
		status = "all"
	}
	feature := strings.TrimSpace(opts.Feature)
	bead := strings.TrimSpace(opts.Bead)
	filtered := records[:0]
	for _, record := range records {
		if status != "all" && record.Status != status {
			continue
		}
		if feature != "" && !containsString(record.FeatureIDs, feature) {
			continue
		}
		if bead != "" && !containsString(record.BeadIDs, bead) {
			continue
		}
		filtered = append(filtered, record)
	}
	return filtered
}

func matchingRecords(root string, records []Record, ref string) []Record {
	ref = filepath.ToSlash(ref)
	if abs, err := filepath.Abs(defaultRoot(root)); err == nil {
		if rel, err := filepath.Rel(abs, ref); err == nil && !strings.HasPrefix(rel, "..") {
			ref = filepath.ToSlash(rel)
		}
	}
	ref = strings.TrimPrefix(ref, "./")
	var matches []Record
	for _, record := range records {
		for _, candidate := range []string{record.Path, filepath.Base(record.Path), record.ID, record.PayloadHash} {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			if candidate == ref || strings.HasPrefix(candidate, ref) {
				matches = append(matches, record)
				break
			}
		}
	}
	return matches
}

func sortRecords(records []Record) {
	sort.SliceStable(records, func(i, j int) bool {
		left, right := records[i], records[j]
		if left.CreatedAt == nil && right.CreatedAt != nil {
			return false
		}
		if left.CreatedAt != nil && right.CreatedAt == nil {
			return true
		}
		if left.CreatedAt != nil && right.CreatedAt != nil && !left.CreatedAt.Equal(*right.CreatedAt) {
			return left.CreatedAt.After(*right.CreatedAt)
		}
		return left.Path < right.Path
	})
}

func idFromPath(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return base
}

func containsString(values []string, value string) bool {
	for _, existing := range values {
		if existing == value {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var unique []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	return unique
}

func defaultRoot(root string) string {
	if strings.TrimSpace(root) == "" {
		return "."
	}
	return root
}

func IsShowError(err error) bool {
	var showErr *ShowError
	return errors.As(err, &showErr)
}
