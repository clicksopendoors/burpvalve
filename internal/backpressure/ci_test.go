package backpressure

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"burpvalve/internal/attestations"
)

func TestRunCIValidatesStagedPassingArtifact(t *testing.T) {
	root := fixtureProject(t)
	staged := fixtureStaged()
	plan, err := BuildPlan(context.Background(), Options{
		Root:            root,
		Mode:            "ci",
		ExplicitFeature: "br-123",
		Staged:          staged,
	})
	if err != nil {
		t.Fatal(err)
	}
	artifact := BuildArtifact(plan, passingResponses(t), PreCommitOptions{
		Root: root,
		Now:  func() time.Time { return artifactTestTime },
	}, attestations.ArtifactPassing)
	body, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	artifactPath := AttestationPath(plan.StagedPayloadHash)
	staged.paths = append(staged.paths, artifactPath)
	staged.content[artifactPath] = string(body)

	result, err := RunCI(context.Background(), CIOptions{
		Root:            root,
		ExplicitFeature: "br-123",
		Staged:          staged,
	})
	if err != nil {
		t.Fatalf("RunCI returned error: %v", err)
	}
	if result.Status != StatusPassed {
		t.Fatalf("status = %q", result.Status)
	}
	if len(result.ArtifactPaths) != 1 || result.ArtifactPaths[0] != artifactPath {
		t.Fatalf("artifact paths = %#v, want %q", result.ArtifactPaths, artifactPath)
	}
	if result.Attestation.Path != artifactPath || result.Attestation.FeatureID != "br-123" {
		t.Fatalf("attestation provenance = %#v", result.Attestation)
	}
	if len(result.ConditionProvenance) != len(plan.ConditionOrder) {
		t.Fatalf("condition provenance count = %d, want %d", len(result.ConditionProvenance), len(plan.ConditionOrder))
	}
}

func TestRunCIRejectsMissingOrInvalidArtifact(t *testing.T) {
	root := fixtureProject(t)
	t.Run("missing", func(t *testing.T) {
		result, err := RunCI(context.Background(), CIOptions{
			Root:            root,
			ExplicitFeature: "br-123",
			Staged:          fixtureStaged(),
		})
		if err == nil || !strings.Contains(result.Message, "missing staged or committed attestation") {
			t.Fatalf("missing artifact should fail, result=%#v err=%v", result, err)
		}
	})

	t.Run("stale", func(t *testing.T) {
		staged := fixtureStaged()
		plan, err := BuildPlan(context.Background(), Options{
			Root:            root,
			Mode:            "ci",
			ExplicitFeature: "br-123",
			Staged:          staged,
		})
		if err != nil {
			t.Fatal(err)
		}
		artifact := BuildArtifact(plan, passingResponses(t), PreCommitOptions{
			Root: root,
			Now:  func() time.Time { return artifactTestTime },
		}, attestations.ArtifactPassing)
		artifact.Conditions[0].Verdict = attestations.VerdictUnknown
		artifact.Conditions[0].Message = "unknown"
		body, err := json.Marshal(artifact)
		if err != nil {
			t.Fatal(err)
		}
		artifactPath := AttestationPath(plan.StagedPayloadHash)
		staged.paths = append(staged.paths, artifactPath)
		staged.content[artifactPath] = string(body)
		result, err := RunCI(context.Background(), CIOptions{
			Root:            root,
			ExplicitFeature: "br-123",
			Staged:          staged,
		})
		if err == nil || !strings.Contains(result.Message, "non-passing verdict") {
			t.Fatalf("invalid artifact should fail, result=%#v err=%v", result, err)
		}
	})
}
