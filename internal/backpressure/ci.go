package backpressure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"burpvalve/internal/attestations"
)

type CIOptions struct {
	Root            string
	ExplicitFeature string
	Commit          string
	Staged          StagedReader
}

type CIResult struct {
	Status              string                  `json:"status"`
	Message             string                  `json:"message"`
	ArtifactPath        string                  `json:"artifact_path"`
	ArtifactPaths       []string                `json:"artifact_paths,omitempty"`
	AuditCommit         string                  `json:"audit_commit,omitempty"`
	Attestation         CIProvenance            `json:"attestation,omitempty"`
	ConditionProvenance []CIConditionProvenance `json:"condition_provenance,omitempty"`
	Plan                Plan                    `json:"plan"`
}

type CIProvenance struct {
	Path              string `json:"path"`
	ArtifactKind      string `json:"artifact_kind,omitempty"`
	Tool              string `json:"tool,omitempty"`
	ToolVersion       string `json:"tool_version,omitempty"`
	StagedPayloadHash string `json:"staged_payload_hash,omitempty"`
	ManifestHash      string `json:"manifest_hash,omitempty"`
	FeatureID         string `json:"feature_id,omitempty"`
}

type CIConditionProvenance struct {
	ConditionID       string `json:"condition_id"`
	ConditionFile     string `json:"condition_file,omitempty"`
	ConditionFileHash string `json:"condition_file_hash,omitempty"`
	Verdict           string `json:"verdict,omitempty"`
	VerifierPolicy    string `json:"verifier_policy,omitempty"`
	VerifierKind      string `json:"verifier_kind,omitempty"`
	VerifierAgent     string `json:"verifier_agent,omitempty"`
	VerifierModel     string `json:"verifier_model,omitempty"`
}

func RunCI(ctx context.Context, opts CIOptions) (CIResult, error) {
	root, err := filepath.Abs(defaultRoot(opts.Root))
	if err != nil {
		return CIResult{}, err
	}
	staged := opts.Staged
	if staged == nil {
		staged = GitStagedReader{}
	}
	if strings.TrimSpace(opts.Commit) != "" && opts.Staged == nil {
		staged = GitCommittedReader{Commit: opts.Commit}
	}
	plan, err := BuildPlan(ctx, Options{
		Root:            root,
		Mode:            "ci",
		ExplicitFeature: explicitFeatureForPlan(opts),
		Staged:          staged,
	})
	if strings.TrimSpace(opts.Commit) == "" && opts.Staged == nil && ((err == nil && len(plan.StagedPayloadPaths) == 0) || errors.Is(err, errNoStagedPayloadPaths)) {
		staged = GitCommittedReader{}
		plan, err = BuildPlan(ctx, Options{
			Root:            root,
			Mode:            "ci",
			ExplicitFeature: opts.ExplicitFeature,
			Staged:          staged,
		})
	}
	result := CIResult{
		Status:        StatusBlocked,
		ArtifactPath:  AttestationPath(plan.StagedPayloadHash),
		ArtifactPaths: []string{AttestationPath(plan.StagedPayloadHash)},
		AuditCommit:   strings.TrimSpace(opts.Commit),
		Plan:          plan,
	}
	if err != nil {
		result.Message = err.Error()
		return result, err
	}
	if len(plan.Features) != 1 {
		result.Message = fmt.Sprintf("expected exactly one atomic feature, got %d", len(plan.Features))
		return result, errors.New(result.Message)
	}
	body, err := staged.StagedFileContent(ctx, root, result.ArtifactPath)
	if err != nil {
		result.Message = "missing staged or committed attestation artifact " + result.ArtifactPath
		return result, errors.New(result.Message)
	}
	var artifact attestations.Artifact
	if err := json.Unmarshal(body, &artifact); err != nil {
		result.Message = "parse attestation artifact: " + err.Error()
		return result, errors.New(result.Message)
	}
	result.Attestation = ciProvenance(result.ArtifactPath, artifact)
	result.ConditionProvenance = ciConditionProvenance(artifact)
	if strings.TrimSpace(opts.Commit) != "" {
		if err := assertCommitFeature(opts.ExplicitFeature, artifact); err != nil {
			result.Message = err.Error()
			return result, err
		}
	}
	if err := artifact.ValidatePassing(ExpectedBinding(plan)); err != nil {
		result.Message = "attestation validation failed: " + err.Error()
		return result, errors.New(result.Message)
	}
	result.Status = StatusPassed
	result.Message = "ci attestation validation passed"
	return result, nil
}

func explicitFeatureForPlan(opts CIOptions) string {
	return opts.ExplicitFeature
}

func assertCommitFeature(explicit string, artifact attestations.Artifact) error {
	explicit = strings.TrimSpace(explicit)
	if explicit == "" {
		return nil
	}
	if artifact.Feature.ID != explicit {
		return fmt.Errorf("feature assertion mismatch: --feature %q does not match committed attestation feature %q", explicit, artifact.Feature.ID)
	}
	return nil
}

func ciProvenance(path string, artifact attestations.Artifact) CIProvenance {
	return CIProvenance{
		Path:              path,
		ArtifactKind:      string(artifact.ArtifactKind),
		Tool:              artifact.Tool,
		ToolVersion:       artifact.ToolVersion,
		StagedPayloadHash: artifact.StagedPayloadHash,
		ManifestHash:      artifact.ManifestHash,
		FeatureID:         artifact.Feature.ID,
	}
}

func ciConditionProvenance(artifact attestations.Artifact) []CIConditionProvenance {
	provenance := make([]CIConditionProvenance, 0, len(artifact.Conditions))
	for _, condition := range artifact.Conditions {
		provenance = append(provenance, CIConditionProvenance{
			ConditionID:       condition.ConditionID,
			ConditionFile:     condition.ConditionFile,
			ConditionFileHash: condition.ConditionFileHash,
			Verdict:           string(condition.Verdict),
			VerifierPolicy:    string(condition.VerifierPolicy),
			VerifierKind:      string(condition.EffectiveVerifierKind()),
			VerifierAgent:     condition.Verifier.Agent,
			VerifierModel:     condition.Verifier.Model,
		})
	}
	return provenance
}
