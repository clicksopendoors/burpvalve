package attestations

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	ToolName = "burpvalve"
	// ToolVersion identifies the attestation schema/tooling contract, not the
	// Burpvalve binary release. Do not bump it for package releases like v0.1.2.
	ToolVersion = "0.1.0"
)

// Verdict is the final state for a feature/condition verification cell.
type Verdict string

const (
	VerdictPass          Verdict = "pass"
	VerdictNotApplicable Verdict = "not_applicable"
	VerdictFail          Verdict = "fail"
	VerdictUnknown       Verdict = "unknown"
)

type ArtifactKind string

const (
	ArtifactPassing ArtifactKind = "passing"
	ArtifactBlocked ArtifactKind = "blocked"
)

type VerifierKind string

const (
	VerifierIndependentSubagent VerifierKind = "independent_subagent"
	VerifierMainAgent           VerifierKind = "main_agent"
	VerifierCI                  VerifierKind = "ci"
	VerifierHuman               VerifierKind = "human"
	VerifierNone                VerifierKind = "none"
	VerifierUnknown             VerifierKind = "unknown"
)

type VerifierPolicy string

const (
	VerifierPolicyIndependentRequired VerifierPolicy = "independent_required"
	VerifierPolicyMainAgentAllowed    VerifierPolicy = "main_agent_allowed"
	VerifierPolicyCIAllowed           VerifierPolicy = "ci_allowed"
	VerifierPolicyHumanAllowed        VerifierPolicy = "human_allowed"
	VerifierPolicyOptional            VerifierPolicy = "optional"
)

type AtomicityMode string

const (
	AtomicityModeSingle AtomicityMode = "single"
	AtomicityModeLane   AtomicityMode = "lane"
)

const LaneAuthorizationKindOrchestrator = "orchestrator"

type Artifact struct {
	SchemaVersion        int          `json:"schema_version"`
	Tool                 string       `json:"tool"`
	ToolVersion          string       `json:"tool_version"`
	ArtifactKind         ArtifactKind `json:"artifact_kind"`
	StagedPayloadHash    string       `json:"staged_payload_hash"`
	ManifestHash         string       `json:"manifest_hash"`
	ConditionOrder       []string     `json:"condition_order"`
	GeneratedBy          Generator    `json:"generated_by"`
	GitHeadBeforeCommit  string       `json:"git_head_before_commit"`
	CreatedAt            time.Time    `json:"created_at"`
	Feature              Feature      `json:"feature"`
	BeadIDs              []string     `json:"bead_ids,omitempty"`
	CoupledWorkRationale string       `json:"coupled_work_rationale,omitempty"`
	Atomicity            Atomicity    `json:"atomicity"`
	Conditions           []Condition  `json:"conditions"`
	NextSteps            []string     `json:"next_steps,omitempty"`
}

type Generator struct {
	Agent string `json:"agent"`
	Model string `json:"model"`
}

type Feature struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	SourceBead  string   `json:"source_bead,omitempty"`
	BeadIDs     []string `json:"bead_ids,omitempty"`
	DiffCluster string   `json:"diff_cluster,omitempty"`
}

type Atomicity struct {
	Mode            AtomicityMode `json:"mode,omitempty"`
	OneFeatureOrFix bool          `json:"one_feature_or_fix"`
	Message         string        `json:"message"`
	Lane            *LaneBinding  `json:"lane,omitempty"`
}

type LaneBinding struct {
	LaneID            string     `json:"lane_id"`
	BeadIDs           []string   `json:"bead_ids"`
	Rationale         string     `json:"rationale"`
	AuthorizedBy      string     `json:"authorized_by"`
	AuthorizationRef  string     `json:"authorization_ref"`
	AuthorizationKind string     `json:"authorization_kind"`
	CreatedAt         *time.Time `json:"created_at,omitempty"`
}

type Verifier struct {
	Kind            VerifierKind `json:"kind,omitempty"`
	Agent           string       `json:"agent,omitempty"`
	Model           string       `json:"model,omitempty"`
	Runtime         string       `json:"runtime,omitempty"`
	SeparateContext bool         `json:"separate_context"`
	TranscriptRef   string       `json:"transcript_ref,omitempty"`
	EvidenceRef     string       `json:"evidence_ref,omitempty"`
	CreatedAt       *time.Time   `json:"created_at,omitempty"`
}

type SupplementalVerifier struct {
	StagedPayloadHash string   `json:"staged_payload_hash,omitempty"`
	ManifestHash      string   `json:"manifest_hash,omitempty"`
	ConditionFileHash string   `json:"condition_file_hash,omitempty"`
	Verifier          Verifier `json:"verifier"`
	SubagentConfirmed bool     `json:"subagent_confirmed,omitempty"`
	SubagentModel     string   `json:"subagent_model,omitempty"`
	Verdict           Verdict  `json:"verdict"`
	Message           string   `json:"message"`
	Evidence          []string `json:"evidence"`
	TranscriptRef     string   `json:"transcript_ref,omitempty"`
	NextAction        string   `json:"next_action,omitempty"`
}

type ResponseAdjudication struct {
	Authority    string  `json:"authority"`
	Summary      string  `json:"summary"`
	FinalVerdict Verdict `json:"final_verdict,omitempty"`
	AuditRef     string  `json:"audit_ref"`
}

type Condition struct {
	ConditionID       string                 `json:"condition_id"`
	ConditionFile     string                 `json:"condition_file"`
	ConditionFileHash string                 `json:"condition_file_hash"`
	VerifierPolicy    VerifierPolicy         `json:"verifier_policy,omitempty"`
	Verifier          Verifier               `json:"verifier,omitempty"`
	SubagentConfirmed bool                   `json:"subagent_confirmed"`
	SubagentModel     string                 `json:"subagent_model,omitempty"`
	Verdict           Verdict                `json:"verdict"`
	Message           string                 `json:"message"`
	Evidence          []string               `json:"evidence"`
	NextAction        string                 `json:"next_action"`
	Supplemental      []SupplementalVerifier `json:"supplemental_verifiers,omitempty"`
	Adjudication      *ResponseAdjudication  `json:"adjudication,omitempty"`
	Timestamp         time.Time              `json:"timestamp"`
}

type ExpectedBinding struct {
	StagedPayloadHash string
	ManifestHash      string
	ConditionHashes   map[string]string
	ConditionOrder    []string
}

func (a Artifact) ValidatePassing(expected ExpectedBinding) error {
	if err := a.validateCommon(expected); err != nil {
		return err
	}
	if a.ArtifactKind != ArtifactPassing {
		return fmt.Errorf("artifact kind %q is not %q", a.ArtifactKind, ArtifactPassing)
	}
	if err := a.validatePassingAtomicity(); err != nil {
		return err
	}
	if len(a.ConditionOrder) == 0 {
		return errors.New("condition order is required")
	}
	if len(a.Conditions) != len(a.ConditionOrder) {
		return fmt.Errorf("condition cell count %d does not match condition order count %d", len(a.Conditions), len(a.ConditionOrder))
	}
	seen := map[string]bool{}
	for _, condition := range a.Conditions {
		if seen[condition.ConditionID] {
			return fmt.Errorf("duplicate condition cell %q", condition.ConditionID)
		}
		seen[condition.ConditionID] = true
		if err := condition.validatePassing(expected); err != nil {
			return err
		}
	}
	for _, id := range a.ConditionOrder {
		if !seen[id] {
			return fmt.Errorf("missing condition cell %q", id)
		}
	}
	return nil
}

func (a Artifact) validatePassingAtomicity() error {
	switch a.Atomicity.Mode {
	case "", AtomicityModeSingle:
		if !a.Atomicity.OneFeatureOrFix {
			return errors.New("passing artifact must confirm one atomic feature or bug fix")
		}
		if a.Atomicity.Lane != nil {
			return errors.New("single-work-unit artifact must not include atomicity lane")
		}
		if a.Feature.Kind == "lane" {
			return errors.New("single-work-unit artifact must not use lane feature kind")
		}
		return nil
	case AtomicityModeLane:
		return a.validatePassingLaneAtomicity()
	default:
		return fmt.Errorf("passing artifact has invalid atomicity mode %q", a.Atomicity.Mode)
	}
}

func (a Artifact) validatePassingLaneAtomicity() error {
	lane := a.Atomicity.Lane
	if a.Atomicity.OneFeatureOrFix {
		return errors.New("lane artifact must not confirm one_feature_or_fix")
	}
	if lane == nil {
		return errors.New("lane artifact must include atomicity.lane")
	}
	if strings.TrimSpace(lane.LaneID) == "" {
		return errors.New("lane artifact requires lane_id")
	}
	if len(nonEmptyStrings(lane.BeadIDs)) < 2 {
		return errors.New("lane artifact requires at least two bead_ids")
	}
	if !sameStrings(nonEmptyStrings(a.BeadIDs), nonEmptyStrings(lane.BeadIDs)) {
		return errors.New("lane artifact bead_ids must match atomicity lane bead_ids")
	}
	if !sameStrings(nonEmptyStrings(a.Feature.BeadIDs), nonEmptyStrings(lane.BeadIDs)) {
		return errors.New("lane feature bead_ids must match atomicity lane bead_ids")
	}
	if a.Feature.Kind != "lane" {
		return errors.New("lane artifact requires feature kind lane")
	}
	if a.Feature.ID != lane.LaneID {
		return fmt.Errorf("lane artifact feature id %q does not match lane_id %q", a.Feature.ID, lane.LaneID)
	}
	if a.Feature.DiffCluster != "lane:"+lane.LaneID {
		return fmt.Errorf("lane artifact feature diff_cluster %q does not match lane %q", a.Feature.DiffCluster, lane.LaneID)
	}
	if strings.TrimSpace(a.CoupledWorkRationale) == "" || strings.TrimSpace(a.CoupledWorkRationale) != strings.TrimSpace(lane.Rationale) {
		return errors.New("lane artifact coupled_work_rationale must match atomicity lane rationale")
	}
	if strings.TrimSpace(lane.Rationale) == "" {
		return errors.New("lane artifact requires lane rationale")
	}
	if strings.TrimSpace(lane.AuthorizedBy) == "" {
		return errors.New("lane artifact requires lane authorized_by")
	}
	if strings.TrimSpace(lane.AuthorizationRef) == "" {
		return errors.New("lane artifact requires lane authorization_ref")
	}
	if strings.TrimSpace(lane.AuthorizationKind) == "" {
		return errors.New("lane artifact requires lane authorization_kind")
	}
	if strings.TrimSpace(lane.AuthorizationKind) != LaneAuthorizationKindOrchestrator {
		return fmt.Errorf("lane artifact authorization_kind %q is not %q", lane.AuthorizationKind, LaneAuthorizationKindOrchestrator)
	}
	if strings.TrimSpace(a.Atomicity.Message) == "" {
		return errors.New("lane artifact requires atomicity message")
	}
	return nil
}

func (a Artifact) ValidateBlocked(expected ExpectedBinding) error {
	if err := a.validateCommon(expected); err != nil {
		return err
	}
	if a.ArtifactKind != ArtifactBlocked {
		return fmt.Errorf("artifact kind %q is not %q", a.ArtifactKind, ArtifactBlocked)
	}
	if len(a.Conditions) == 0 {
		return errors.New("blocked artifact must include at least one condition cell")
	}
	for _, condition := range a.Conditions {
		if err := condition.validateBlocked(expected); err != nil {
			return err
		}
	}
	return nil
}

// ValidateShape checks that an evidence artifact is structurally usable for
// query/explain surfaces without requiring it to pass the current commit gate.
func (a Artifact) ValidateShape() error {
	switch a.ArtifactKind {
	case ArtifactPassing:
		return a.ValidatePassing(ExpectedBinding{})
	case ArtifactBlocked:
		if err := a.validateCommon(ExpectedBinding{}); err != nil {
			return err
		}
		if len(a.Conditions) == 0 {
			return errors.New("blocked artifact must include at least one condition cell")
		}
		for _, condition := range a.Conditions {
			if err := condition.validateCommon(ExpectedBinding{}); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown artifact_kind %q", a.ArtifactKind)
	}
}

func (a Artifact) validateCommon(expected ExpectedBinding) error {
	if a.SchemaVersion != 1 {
		return fmt.Errorf("unsupported schema_version %d", a.SchemaVersion)
	}
	if a.Tool != ToolName {
		return fmt.Errorf("unexpected tool %q", a.Tool)
	}
	if a.ToolVersion == "" {
		return errors.New("tool_version is required")
	}
	if a.StagedPayloadHash == "" {
		return errors.New("staged_payload_hash is required")
	}
	if expected.StagedPayloadHash != "" && a.StagedPayloadHash != expected.StagedPayloadHash {
		return fmt.Errorf("staged payload hash is stale: got %s want %s", a.StagedPayloadHash, expected.StagedPayloadHash)
	}
	if a.ManifestHash == "" {
		return errors.New("manifest_hash is required")
	}
	if expected.ManifestHash != "" && a.ManifestHash != expected.ManifestHash {
		return fmt.Errorf("manifest hash is stale: got %s want %s", a.ManifestHash, expected.ManifestHash)
	}
	if len(expected.ConditionOrder) > 0 && !sameStrings(a.ConditionOrder, expected.ConditionOrder) {
		return errors.New("condition order is stale")
	}
	if a.GeneratedBy.Agent == "" {
		return errors.New("generated_by.agent is required")
	}
	if a.GitHeadBeforeCommit == "" {
		return errors.New("git_head_before_commit is required")
	}
	if a.CreatedAt.IsZero() {
		return errors.New("created_at timestamp is required")
	}
	if a.Feature.ID == "" || a.Feature.Kind == "" || a.Feature.Name == "" {
		return errors.New("feature id, kind, and name are required")
	}
	if a.Feature.SourceBead == "" && a.Feature.DiffCluster == "" {
		return errors.New("feature must include source_bead or diff_cluster")
	}
	return nil
}

func (c Condition) validatePassing(expected ExpectedBinding) error {
	if err := c.validateCommon(expected); err != nil {
		return err
	}
	if err := c.validateVerifierPolicyForPassing(); err != nil {
		return err
	}
	switch c.Verdict {
	case VerdictPass:
		return nil
	case VerdictNotApplicable:
		if strings.TrimSpace(c.Message) == "" {
			return fmt.Errorf("condition %q is not_applicable without message", c.ConditionID)
		}
		return nil
	case VerdictFail, VerdictUnknown:
		return fmt.Errorf("condition %q has non-passing verdict %q", c.ConditionID, c.Verdict)
	default:
		return fmt.Errorf("condition %q has invalid verdict %q", c.ConditionID, c.Verdict)
	}
}

func (c Condition) validateBlocked(expected ExpectedBinding) error {
	if err := c.validateCommon(expected); err != nil {
		return err
	}
	if !c.VerifierPolicyAccepted() && strings.TrimSpace(c.Message) == "" {
		return fmt.Errorf("condition %q missing message for unacceptable verifier evidence", c.ConditionID)
	}
	switch c.Verdict {
	case VerdictPass:
		if !c.VerifierPolicyAccepted() {
			return fmt.Errorf("condition %q cannot pass with verifier policy %q and verifier kind %q", c.ConditionID, c.NormalizedVerifierPolicy(), c.EffectiveVerifierKind())
		}
	case VerdictNotApplicable:
		if strings.TrimSpace(c.Message) == "" {
			return fmt.Errorf("condition %q is not_applicable without message", c.ConditionID)
		}
	case VerdictFail, VerdictUnknown:
		if strings.TrimSpace(c.Message) == "" {
			return fmt.Errorf("condition %q missing failure or unknown message", c.ConditionID)
		}
		if len(c.Evidence) == 0 {
			return fmt.Errorf("condition %q missing evidence for blocked verdict", c.ConditionID)
		}
		if strings.TrimSpace(c.NextAction) == "" {
			return fmt.Errorf("condition %q missing next action for blocked verdict", c.ConditionID)
		}
	default:
		return fmt.Errorf("condition %q has invalid verdict %q", c.ConditionID, c.Verdict)
	}
	return nil
}

func (c Condition) validateCommon(expected ExpectedBinding) error {
	if c.ConditionID == "" {
		return errors.New("condition_id is required")
	}
	if c.ConditionFile == "" {
		return fmt.Errorf("condition %q missing condition_file", c.ConditionID)
	}
	if c.ConditionFileHash == "" {
		return fmt.Errorf("condition %q missing condition_file_hash", c.ConditionID)
	}
	if expected.ConditionHashes != nil {
		want, ok := expected.ConditionHashes[c.ConditionID]
		if !ok {
			return fmt.Errorf("condition %q not present in expected condition hashes", c.ConditionID)
		}
		if c.ConditionFileHash != want {
			return fmt.Errorf("condition %q file hash is stale: got %s want %s", c.ConditionID, c.ConditionFileHash, want)
		}
	}
	if c.Timestamp.IsZero() {
		return fmt.Errorf("condition %q timestamp is required", c.ConditionID)
	}
	if !ValidVerifierPolicy(c.NormalizedVerifierPolicy()) {
		return fmt.Errorf("condition %q has invalid verifier_policy %q", c.ConditionID, c.VerifierPolicy)
	}
	if !ValidVerifierKind(c.EffectiveVerifierKind()) {
		return fmt.Errorf("condition %q has invalid verifier kind %q", c.ConditionID, c.Verifier.Kind)
	}
	for i, supplemental := range c.Supplemental {
		if err := supplemental.validate(c.ConditionID); err != nil {
			return fmt.Errorf("condition %q supplemental_verifiers[%d]: %w", c.ConditionID, i, err)
		}
	}
	if c.Adjudication != nil {
		if err := c.Adjudication.validate(); err != nil {
			return fmt.Errorf("condition %q adjudication: %w", c.ConditionID, err)
		}
	}
	return nil
}

func (s SupplementalVerifier) validate(conditionID string) error {
	kind := EffectiveVerifierKind(s.Verifier, s.SubagentConfirmed)
	if !ValidVerifierKind(kind) || kind == VerifierNone || kind == VerifierUnknown {
		return errors.New("requires verifier provenance kind")
	}
	if strings.TrimSpace(s.Verifier.Model) == "" && strings.TrimSpace(s.SubagentModel) == "" {
		return errors.New("requires verifier model")
	}
	if strings.TrimSpace(s.Verifier.Runtime) == "" {
		return errors.New("requires verifier runtime")
	}
	if !s.Verifier.SeparateContext {
		return errors.New("requires verifier separate_context=true")
	}
	switch s.Verdict {
	case VerdictPass, VerdictNotApplicable, VerdictFail, VerdictUnknown:
	default:
		return fmt.Errorf("has invalid verdict %q", s.Verdict)
	}
	if len(nonEmptyStrings(s.Evidence)) == 0 {
		return fmt.Errorf("has %s verdict without evidence", s.Verdict)
	}
	switch s.Verdict {
	case VerdictNotApplicable, VerdictFail, VerdictUnknown:
		if strings.TrimSpace(s.Message) == "" {
			return fmt.Errorf("has %s verdict without message", s.Verdict)
		}
	}
	switch s.Verdict {
	case VerdictFail, VerdictUnknown:
		if strings.TrimSpace(s.NextAction) == "" {
			return fmt.Errorf("has %s verdict without next_action", s.Verdict)
		}
	}
	if conditionID == "" {
		return errors.New("missing parent condition id")
	}
	return nil
}

func (a ResponseAdjudication) validate() error {
	if strings.TrimSpace(a.Authority) == "" {
		return errors.New("authority is required")
	}
	if strings.TrimSpace(a.Summary) == "" {
		return errors.New("summary is required")
	}
	if strings.TrimSpace(a.AuditRef) == "" {
		return errors.New("audit_ref is required")
	}
	if a.FinalVerdict != "" {
		switch a.FinalVerdict {
		case VerdictPass, VerdictNotApplicable, VerdictFail, VerdictUnknown:
		default:
			return fmt.Errorf("final_verdict is invalid: %q", a.FinalVerdict)
		}
	}
	return nil
}

func nonEmptyStrings(values []string) []string {
	var out []string
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}

func (c Condition) validateVerifierPolicyForPassing() error {
	if c.VerifierPolicyAccepted() {
		return nil
	}
	policy := c.NormalizedVerifierPolicy()
	kind := c.EffectiveVerifierKind()
	return fmt.Errorf("condition %q verifier policy %q does not allow verifier kind %q: %s", c.ConditionID, policy, kind, VerifierPolicyRecovery(policy, c.ConditionID))
}

func (c Condition) NormalizedVerifierPolicy() VerifierPolicy {
	return NormalizeVerifierPolicy(c.VerifierPolicy)
}

func (c Condition) EffectiveVerifierKind() VerifierKind {
	return EffectiveVerifierKind(c.Verifier, c.SubagentConfirmed)
}

func (c Condition) VerifierPolicyAccepted() bool {
	return VerifierPolicyAllows(c.NormalizedVerifierPolicy(), c.EffectiveVerifierKind())
}

func NormalizeVerifierPolicy(policy VerifierPolicy) VerifierPolicy {
	if policy == "" {
		return VerifierPolicyIndependentRequired
	}
	return policy
}

func EffectiveVerifierKind(verifier Verifier, legacySubagentConfirmed bool) VerifierKind {
	if verifier.Kind != "" {
		return verifier.Kind
	}
	if legacySubagentConfirmed {
		return VerifierIndependentSubagent
	}
	return VerifierNone
}

func VerifierPolicyAllows(policy VerifierPolicy, kind VerifierKind) bool {
	switch NormalizeVerifierPolicy(policy) {
	case VerifierPolicyIndependentRequired:
		return kind == VerifierIndependentSubagent
	case VerifierPolicyMainAgentAllowed:
		return kind == VerifierIndependentSubagent || kind == VerifierMainAgent
	case VerifierPolicyCIAllowed:
		return kind == VerifierIndependentSubagent || kind == VerifierCI
	case VerifierPolicyHumanAllowed:
		return kind == VerifierIndependentSubagent || kind == VerifierHuman
	case VerifierPolicyOptional:
		return ValidVerifierKind(kind) && kind != VerifierUnknown
	default:
		return false
	}
}

func ValidVerifierPolicy(policy VerifierPolicy) bool {
	switch NormalizeVerifierPolicy(policy) {
	case VerifierPolicyIndependentRequired, VerifierPolicyMainAgentAllowed, VerifierPolicyCIAllowed, VerifierPolicyHumanAllowed, VerifierPolicyOptional:
		return true
	default:
		return false
	}
}

func ValidVerifierKind(kind VerifierKind) bool {
	switch kind {
	case VerifierIndependentSubagent, VerifierMainAgent, VerifierCI, VerifierHuman, VerifierNone, VerifierUnknown:
		return true
	default:
		return false
	}
}

func VerifierPolicyRecovery(policy VerifierPolicy, conditionID string) string {
	switch NormalizeVerifierPolicy(policy) {
	case VerifierPolicyIndependentRequired:
		return "spawn an independent read-only verifier for " + conditionID + ", provide that verifier evidence, or change the manifest policy intentionally"
	case VerifierPolicyMainAgentAllowed:
		return "provide main-agent or independent verifier evidence for " + conditionID + ", or accept a blocked report"
	case VerifierPolicyCIAllowed:
		return "provide CI or independent verifier evidence for " + conditionID + ", or accept a blocked report"
	case VerifierPolicyHumanAllowed:
		return "provide human or independent verifier evidence for " + conditionID + ", or accept a blocked report"
	case VerifierPolicyOptional:
		return "provide verifier evidence or leave verifier.kind as none when this condition is intentionally optional"
	default:
		return "set verifier_policy to independent_required, main_agent_allowed, ci_allowed, human_allowed, or optional"
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
