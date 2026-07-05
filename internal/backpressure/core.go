package backpressure

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"burpvalve/internal/attestations"
	"burpvalve/internal/gitindex"
	"burpvalve/internal/lintconfig"

	"gopkg.in/yaml.v3"
)

const (
	ManifestPath = "backpressure/manifest.yaml"
	HashPrefix   = "sha256:"
)

var errNoStagedPayloadPaths = errors.New("no staged payload paths; stage changes or pass --feature explicitly")

type Manifest struct {
	Conditions   []ConditionSpec      `yaml:"conditions" json:"conditions"`
	LintCommands []lintconfig.Command `yaml:"lint_commands" json:"lint_commands"`
	LintCoverage lintconfig.Coverage  `yaml:"lint_coverage" json:"lint_coverage,omitempty"`
}

type ConditionSpec struct {
	ID             string                      `yaml:"id" json:"id"`
	Path           string                      `yaml:"path" json:"path"`
	Enabled        bool                        `yaml:"enabled" json:"enabled"`
	VerifierPolicy attestations.VerifierPolicy `yaml:"verifier_policy,omitempty" json:"verifier_policy,omitempty"`
}

type Feature struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	SourceBead  string   `json:"source_bead,omitempty"`
	DiffCluster string   `json:"diff_cluster,omitempty"`
	Paths       []string `json:"paths,omitempty"`
}

type MatrixCell struct {
	FeatureID     string `json:"feature_id"`
	ConditionID   string `json:"condition_id"`
	ConditionPath string `json:"condition_path"`
}

type Matrix struct {
	Features   []Feature       `json:"features"`
	Conditions []ConditionSpec `json:"conditions"`
	Cells      []MatrixCell    `json:"cells"`
}

type Plan struct {
	Mode                string               `json:"mode"`
	ManifestHash        string               `json:"manifest_hash"`
	ConditionOrder      []string             `json:"condition_order"`
	ConditionFileHashes map[string]string    `json:"condition_file_hashes"`
	StagedPayloadHash   string               `json:"staged_payload_hash"`
	StagedPayloadPaths  []string             `json:"staged_payload_paths"`
	StagedPayloadFiles  []StagedPayloadFile  `json:"staged_payload_files,omitempty"`
	ExcludedStagedPaths []string             `json:"excluded_staged_paths"`
	Features            []Feature            `json:"features"`
	Matrix              Matrix               `json:"matrix"`
	LintCommands        []lintconfig.Command `json:"lint_commands"`
	BlockingReason      string               `json:"blocking_reason,omitempty"`
}

type Options struct {
	Root            string
	Mode            string
	ExplicitFeature string
	Staged          StagedReader
}

type StagedReader interface {
	StagedFiles(ctx context.Context, root string) ([]string, error)
	StagedFileContent(ctx context.Context, root, path string) ([]byte, error)
}

type StagedEntryReader interface {
	StagedEntries(ctx context.Context, root string) ([]StagedPayloadFile, error)
}

type StagedPayloadFile struct {
	Path      string `json:"path"`
	OldPath   string `json:"old_path,omitempty"`
	Status    string `json:"status"`
	GitStatus string `json:"git_status,omitempty"`
}

type GitStagedReader struct{}

func (GitStagedReader) StagedFiles(ctx context.Context, root string) ([]string, error) {
	entries, err := (GitStagedReader{}).StagedEntries(ctx, root)
	if err != nil {
		return nil, err
	}
	return stagedEntryPaths(entries), nil
}

func (GitStagedReader) StagedEntries(ctx context.Context, root string) ([]StagedPayloadFile, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--name-status", "-M", "-z")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseGitNameStatus(output), nil
}

func (GitStagedReader) StagedFileContent(ctx context.Context, root, path string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", "show", ":"+path)
	cmd.Dir = root
	return cmd.Output()
}

type GitCommittedReader struct {
	Commit string
}

func (r GitCommittedReader) StagedFiles(ctx context.Context, root string) ([]string, error) {
	entries, err := r.StagedEntries(ctx, root)
	if err != nil {
		return nil, err
	}
	return stagedEntryPaths(entries), nil
}

func (r GitCommittedReader) StagedEntries(ctx context.Context, root string) ([]StagedPayloadFile, error) {
	commit := r.targetCommit()
	base := commit + "^1"
	if err := gitRevParse(ctx, root, base); err != nil {
		base = gitEmptyTreeHash
	}
	cmd := exec.CommandContext(ctx, "git", "diff-tree", "--no-commit-id", "--name-status", "-M", "-r", "-z", base, commit)
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseGitNameStatus(output), nil
}

func parseGitNameStatus(output []byte) []StagedPayloadFile {
	parts := bytes.Split(output, []byte{0})
	var entries []StagedPayloadFile
	for i := 0; i < len(parts); {
		if len(parts[i]) == 0 {
			i++
			continue
		}
		gitStatus := string(parts[i])
		i++
		if i >= len(parts) || len(parts[i]) == 0 {
			break
		}
		entry := StagedPayloadFile{
			Status:    normalizeGitStatus(gitStatus),
			GitStatus: gitStatus,
		}
		if strings.HasPrefix(gitStatus, "R") || strings.HasPrefix(gitStatus, "C") {
			entry.OldPath = filepath.ToSlash(string(parts[i]))
			i++
			if i >= len(parts) || len(parts[i]) == 0 {
				break
			}
		}
		entry.Path = filepath.ToSlash(string(parts[i]))
		i++
		entries = append(entries, entry)
	}
	sortStagedEntries(entries)
	return entries
}

func normalizeGitStatus(status string) string {
	switch {
	case strings.HasPrefix(status, "A"):
		return "added"
	case strings.HasPrefix(status, "C"):
		return "copied"
	case strings.HasPrefix(status, "D"):
		return "deleted"
	case strings.HasPrefix(status, "M"):
		return "modified"
	case strings.HasPrefix(status, "R"):
		return "renamed"
	case strings.HasPrefix(status, "T"):
		return "type_changed"
	default:
		return "unknown"
	}
}

func stagedEntryPaths(entries []StagedPayloadFile) []string {
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.Path)
	}
	sort.Strings(paths)
	return paths
}

func (r GitCommittedReader) StagedFileContent(ctx context.Context, root, path string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", "show", r.targetCommit()+":"+path)
	cmd.Dir = root
	return cmd.Output()
}

func (r GitCommittedReader) targetCommit() string {
	if strings.TrimSpace(r.Commit) == "" {
		return "HEAD"
	}
	return strings.TrimSpace(r.Commit)
}

const gitEmptyTreeHash = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

func gitRevParse(ctx context.Context, root, rev string) error {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", rev)
	cmd.Dir = root
	return cmd.Run()
}

func BuildPlan(ctx context.Context, opts Options) (Plan, error) {
	root, err := filepath.Abs(defaultRoot(opts.Root))
	if err != nil {
		return Plan{}, err
	}
	if opts.Staged == nil {
		opts.Staged = GitStagedReader{}
	}
	manifest, manifestBody, err := LoadManifest(root)
	if err != nil {
		return Plan{}, err
	}
	enabled := EnabledConditions(manifest)
	conditionHashes, err := HashConditionFiles(root, enabled)
	if err != nil {
		return Plan{}, err
	}
	payload, err := HashStagedPayload(ctx, root, opts.Staged)
	if err != nil {
		return Plan{}, err
	}
	features, featureErr := DetectFeatures(FeatureOptions{
		ExplicitFeature: opts.ExplicitFeature,
		StagedPaths:     payload.IncludedPaths,
	})
	matrix := BuildMatrix(features, enabled)
	plan := Plan{
		Mode:                opts.Mode,
		ManifestHash:        HashBytes(manifestBody),
		ConditionOrder:      ConditionOrder(enabled),
		ConditionFileHashes: conditionHashes,
		StagedPayloadHash:   payload.Hash,
		StagedPayloadPaths:  payload.IncludedPaths,
		StagedPayloadFiles:  payload.IncludedFiles,
		ExcludedStagedPaths: payload.ExcludedPaths,
		Features:            features,
		Matrix:              matrix,
		LintCommands:        manifest.LintCommands,
	}
	if featureErr != nil {
		plan.BlockingReason = featureErr.Error()
		return plan, featureErr
	}
	return plan, nil
}

func LoadManifest(root string) (Manifest, []byte, error) {
	path := filepath.Join(root, filepath.FromSlash(ManifestPath))
	body, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, nil, err
	}
	var manifest Manifest
	if err := yaml.Unmarshal(body, &manifest); err != nil {
		return Manifest{}, nil, fmt.Errorf("parse %s: %w", ManifestPath, err)
	}
	if err := validateManifestShape(body); err != nil {
		return Manifest{}, nil, err
	}
	if err := normalizeManifestLintFields(&manifest); err != nil {
		return Manifest{}, nil, err
	}
	for i, condition := range manifest.Conditions {
		if strings.TrimSpace(condition.ID) == "" {
			return Manifest{}, nil, fmt.Errorf("condition %d missing id", i)
		}
		if strings.TrimSpace(condition.Path) == "" {
			return Manifest{}, nil, fmt.Errorf("condition %q missing path", condition.ID)
		}
		if !attestations.ValidVerifierPolicy(attestations.NormalizeVerifierPolicy(condition.VerifierPolicy)) {
			return Manifest{}, nil, fmt.Errorf("condition %q has invalid verifier_policy %q", condition.ID, condition.VerifierPolicy)
		}
	}
	return manifest, body, nil
}

func EnabledConditions(manifest Manifest) []ConditionSpec {
	var enabled []ConditionSpec
	for _, condition := range manifest.Conditions {
		if condition.Enabled {
			enabled = append(enabled, condition)
		}
	}
	return enabled
}

func ConditionOrder(conditions []ConditionSpec) []string {
	order := make([]string, 0, len(conditions))
	for _, condition := range conditions {
		order = append(order, condition.ID)
	}
	return order
}

func HashConditionFiles(root string, conditions []ConditionSpec) (map[string]string, error) {
	hashes := make(map[string]string, len(conditions))
	for _, condition := range conditions {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(condition.Path)))
		if err != nil {
			return nil, fmt.Errorf("read condition %s: %w", condition.ID, err)
		}
		hashes[condition.ID] = HashBytes(body)
	}
	return hashes, nil
}

type PayloadHash struct {
	Hash          string
	IncludedPaths []string
	IncludedFiles []StagedPayloadFile
	ExcludedPaths []string
}

func HashStagedPayload(ctx context.Context, root string, staged StagedReader) (PayloadHash, error) {
	entries, err := stagedPayloadEntries(ctx, root, staged)
	if err != nil {
		return PayloadHash{}, err
	}
	var included []string
	var includedFiles []StagedPayloadFile
	var excluded []string
	h := sha256.New()
	for _, entry := range entries {
		entry.Path = filepath.ToSlash(entry.Path)
		entry.OldPath = filepath.ToSlash(entry.OldPath)
		if entry.Status == "" {
			entry.Status = "modified"
		}
		if isGeneratedPath(entry.Path) {
			excluded = append(excluded, entry.Path)
			continue
		}
		included = append(included, entry.Path)
		includedFiles = append(includedFiles, entry)
		fmt.Fprintf(h, "path:%s\nstatus:%s\nold_path:%s\ngit_status:%s\n", entry.Path, entry.Status, entry.OldPath, entry.GitStatus)
		if entry.Status == "deleted" {
			continue
		}
		body, err := staged.StagedFileContent(ctx, root, entry.Path)
		if err != nil {
			return PayloadHash{}, fmt.Errorf("read staged %s: %w", entry.Path, err)
		}
		fmt.Fprintf(h, "size:%d\n", len(body))
		h.Write(body)
		h.Write([]byte{'\n'})
	}
	return PayloadHash{
		Hash:          HashPrefix + hex.EncodeToString(h.Sum(nil)),
		IncludedPaths: included,
		IncludedFiles: includedFiles,
		ExcludedPaths: excluded,
	}, nil
}

func stagedPayloadEntries(ctx context.Context, root string, staged StagedReader) ([]StagedPayloadFile, error) {
	if reader, ok := staged.(StagedEntryReader); ok {
		entries, err := reader.StagedEntries(ctx, root)
		if err != nil {
			return nil, err
		}
		sortStagedEntries(entries)
		return entries, nil
	}
	paths, err := staged.StagedFiles(ctx, root)
	if err != nil {
		return nil, err
	}
	entries := make([]StagedPayloadFile, 0, len(paths))
	for _, path := range paths {
		entries = append(entries, StagedPayloadFile{Path: filepath.ToSlash(path), Status: "modified"})
	}
	sortStagedEntries(entries)
	return entries, nil
}

func sortStagedEntries(entries []StagedPayloadFile) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Path != entries[j].Path {
			return entries[i].Path < entries[j].Path
		}
		if entries[i].OldPath != entries[j].OldPath {
			return entries[i].OldPath < entries[j].OldPath
		}
		return entries[i].Status < entries[j].Status
	})
}

func isGeneratedPath(path string) bool {
	return gitindex.IsGeneratedEvidencePath(path)
}

type FeatureOptions struct {
	ExplicitFeature string
	StagedPaths     []string
}

func DetectFeatures(opts FeatureOptions) ([]Feature, error) {
	if feature := strings.TrimSpace(opts.ExplicitFeature); feature != "" {
		return []Feature{{
			ID:          feature,
			Kind:        "feature",
			Name:        feature,
			DiffCluster: "explicit:" + feature,
			Paths:       append([]string(nil), opts.StagedPaths...),
		}}, nil
	}
	if len(opts.StagedPaths) == 0 {
		return nil, errNoStagedPayloadPaths
	}
	clusters := pathClusters(opts.StagedPaths)
	if len(clusters) != 1 {
		names := make([]string, 0, len(clusters))
		for name := range clusters {
			names = append(names, name)
		}
		sort.Strings(names)
		return nil, fmt.Errorf("staged changes map to multiple diff clusters (%s); split commit or pass --feature explicitly", strings.Join(names, ", "))
	}
	for cluster, paths := range clusters {
		sort.Strings(paths)
		return []Feature{{
			ID:          "diff:" + cluster,
			Kind:        "feature",
			Name:        "staged changes in " + cluster,
			DiffCluster: cluster,
			Paths:       paths,
		}}, nil
	}
	return nil, errors.New("could not detect feature from staged paths")
}

func pathClusters(paths []string) map[string][]string {
	clusters := map[string][]string{}
	for _, path := range paths {
		cluster := firstPathSegment(path)
		clusters[cluster] = append(clusters[cluster], path)
	}
	return clusters
}

func firstPathSegment(path string) string {
	path = strings.Trim(filepath.ToSlash(path), "/")
	if path == "" {
		return "."
	}
	segment, _, ok := strings.Cut(path, "/")
	if !ok {
		return "."
	}
	return segment
}

func BuildMatrix(features []Feature, conditions []ConditionSpec) Matrix {
	matrix := Matrix{
		Features:   append([]Feature(nil), features...),
		Conditions: append([]ConditionSpec(nil), conditions...),
	}
	for _, feature := range features {
		for _, condition := range conditions {
			matrix.Cells = append(matrix.Cells, MatrixCell{
				FeatureID:     feature.ID,
				ConditionID:   condition.ID,
				ConditionPath: condition.Path,
			})
		}
	}
	return matrix
}

func ExpectedBinding(plan Plan) attestations.ExpectedBinding {
	return attestations.ExpectedBinding{
		StagedPayloadHash: plan.StagedPayloadHash,
		ManifestHash:      plan.ManifestHash,
		ConditionHashes:   plan.ConditionFileHashes,
		ConditionOrder:    plan.ConditionOrder,
	}
}

func HashBytes(body []byte) string {
	sum := sha256.Sum256(body)
	return HashPrefix + hex.EncodeToString(sum[:])
}

func defaultRoot(root string) string {
	if strings.TrimSpace(root) == "" {
		return "."
	}
	return root
}

func PrintPlanJSON(plan Plan) ([]byte, error) {
	return json.MarshalIndent(plan, "", "  ")
}

func WithTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 30*time.Second)
}
