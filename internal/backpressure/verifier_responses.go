package backpressure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"burpvalve/internal/attestations"
	bvconfig "burpvalve/internal/config"
)

type SupplementalVerifier = attestations.SupplementalVerifier

type ResponseAdjudication = attestations.ResponseAdjudication

type SubmitVerifierInput struct {
	ResponseCondition
	StagedPayloadHash string `json:"staged_payload_hash"`
	ManifestHash      string `json:"manifest_hash"`
	ConditionFileHash string `json:"condition_file_hash"`
}

type SubmitVerifierOptions struct {
	Root              string
	ExplicitFeature   string
	ConditionID       string
	ResponsesPath     string
	StagedPayloadHash string
	ManifestHash      string
	ConditionFileHash string
	Response          *SubmitVerifierInput
	ResponseReader    io.Reader
	TranscriptPath    string
	TranscriptReader  io.Reader
	Staged            StagedReader
	LockTimeout       time.Duration
}

type SubmitVerifierResult struct {
	SchemaVersion     int      `json:"schema_version"`
	Command           string   `json:"command"`
	Status            string   `json:"status"`
	Message           string   `json:"message"`
	Fatal             bool     `json:"fatal"`
	NextSteps         []string `json:"next_steps,omitempty"`
	Warnings          []string `json:"warnings,omitempty"`
	ResponsesPath     string   `json:"responses_path,omitempty"`
	ConditionID       string   `json:"condition_id,omitempty"`
	StagedPayloadHash string   `json:"staged_payload_hash,omitempty"`
	ManifestHash      string   `json:"manifest_hash,omitempty"`
	TranscriptRef     string   `json:"transcript_ref,omitempty"`
	Plan              Plan     `json:"plan"`
}

const (
	StatusResponsesUpdated = "responses_updated"

	defaultTranscriptDir = "log/backpressure/transcripts"
)

func RunVerifierSubmit(ctx context.Context, opts SubmitVerifierOptions) (SubmitVerifierResult, error) {
	root, err := filepath.Abs(defaultRoot(opts.Root))
	if err != nil {
		return SubmitVerifierResult{}, err
	}
	staged := opts.Staged
	if staged == nil {
		staged = GitStagedReader{}
	}
	plan, planErr := BuildPlan(ctx, Options{
		Root:            root,
		Mode:            "pre-commit",
		ExplicitFeature: opts.ExplicitFeature,
		Staged:          staged,
	})
	result := SubmitVerifierResult{
		SchemaVersion: 1,
		Command:       "verifier submit",
		Status:        StatusBlocked,
		Fatal:         true,
		ConditionID:   strings.TrimSpace(opts.ConditionID),
		Plan:          plan,
	}
	if planErr != nil {
		result.Message = planErr.Error()
		result.NextSteps = []string{"Stage the same payload used by verifier begin, or pass --feature with the bead or feature id, then rerun burpvalve verifier submit."}
		return result, planErr
	}
	result.StagedPayloadHash = plan.StagedPayloadHash
	result.ManifestHash = plan.ManifestHash
	responsesPath := strings.TrimSpace(opts.ResponsesPath)
	if responsesPath == "" {
		responsesPath = ResponsesPath(plan.StagedPayloadHash)
	}
	result.ResponsesPath = filepath.ToSlash(responsesPath)

	input, err := submitInput(opts)
	if err != nil {
		result.Message = err.Error()
		result.NextSteps = []string{"Pass verifier result JSON on stdin or through SubmitVerifierOptions.Response."}
		return result, err
	}
	if err := validateSubmitBinding(plan, opts, input); err != nil {
		result.Message = err.Error()
		result.NextSteps = []string{"Use the current packet submit command and response file for this staged payload, then rerun verifier submit."}
		return result, err
	}
	normalizeSubmitBindings(opts, input)
	condition, conditionHash, err := submitConditionSpec(plan, opts.ConditionID)
	if err != nil {
		result.Message = err.Error()
		result.NextSteps = []string{"Choose one enabled condition id from the current manifest."}
		return result, err
	}
	if err := validateSubmitInput(condition, conditionHash, input); err != nil {
		result.Message = err.Error()
		result.NextSteps = []string{"Fix the verifier JSON so it includes binding, provenance, verdict, message/evidence, and any supplemental/adjudication fields required by policy."}
		return result, err
	}
	transcriptRef, err := storeSubmitTranscript(root, plan, opts, input)
	if err != nil {
		result.Message = err.Error()
		result.NextSteps = []string{"Fix the transcript path or omit --transcript, then rerun verifier submit."}
		return result, err
	}
	if transcriptRef != "" {
		result.TranscriptRef = transcriptRef
		if input.Verifier.TranscriptRef == "" {
			input.Verifier.TranscriptRef = transcriptRef
		}
	}
	warnings, err := mergeVerifierResponse(root, responsesPath, condition, input, lockTimeout(opts.LockTimeout))
	if err != nil {
		result.Message = err.Error()
		result.NextSteps = []string{"Resolve the response-file conflict or stale binding, then rerun verifier submit."}
		return result, err
	}
	result.Status = StatusResponsesUpdated
	result.Message = "updated verifier response for " + condition.ID + " in " + result.ResponsesPath
	result.Fatal = false
	result.Warnings = warnings
	result.NextSteps = []string{"Continue collecting verifier responses, then run burpvalve commit with --responses " + result.ResponsesPath + "."}
	return result, nil
}

func submitInput(opts SubmitVerifierOptions) (*SubmitVerifierInput, error) {
	if opts.Response != nil {
		copy := *opts.Response
		return &copy, nil
	}
	if opts.ResponseReader == nil {
		return nil, errors.New("missing verifier response JSON")
	}
	body, err := io.ReadAll(opts.ResponseReader)
	if err != nil {
		return nil, fmt.Errorf("read verifier response JSON: %w", err)
	}
	var input SubmitVerifierInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, fmt.Errorf("parse verifier response JSON: %w", err)
	}
	return &input, nil
}

func validateSubmitBinding(plan Plan, opts SubmitVerifierOptions, input *SubmitVerifierInput) error {
	if input == nil {
		return errors.New("missing verifier response JSON")
	}
	if got, want := firstNonEmpty(input.StagedPayloadHash, opts.StagedPayloadHash), plan.StagedPayloadHash; got == "" || got != want {
		return fmt.Errorf("staged payload binding is stale: got %q want %q", got, want)
	}
	if got, want := firstNonEmpty(input.ManifestHash, opts.ManifestHash), plan.ManifestHash; got == "" || got != want {
		return fmt.Errorf("manifest binding is stale: got %q want %q", got, want)
	}
	_, wantConditionHash, err := submitConditionSpec(plan, opts.ConditionID)
	if err != nil {
		return err
	}
	if got := firstNonEmpty(input.ConditionFileHash, opts.ConditionFileHash); got == "" || got != wantConditionHash {
		return fmt.Errorf("condition file binding is stale for %s: got %q want %q", opts.ConditionID, got, wantConditionHash)
	}
	return nil
}

func normalizeSubmitBindings(opts SubmitVerifierOptions, input *SubmitVerifierInput) {
	if input.StagedPayloadHash == "" {
		input.StagedPayloadHash = strings.TrimSpace(opts.StagedPayloadHash)
	}
	if input.ManifestHash == "" {
		input.ManifestHash = strings.TrimSpace(opts.ManifestHash)
	}
	if input.ConditionFileHash == "" {
		input.ConditionFileHash = strings.TrimSpace(opts.ConditionFileHash)
	}
}

func submitConditionSpec(plan Plan, id string) (ConditionSpec, string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ConditionSpec{}, "", errors.New("condition id is required")
	}
	for _, condition := range plan.Matrix.Conditions {
		if condition.ID == id {
			return condition, plan.ConditionFileHashes[condition.ID], nil
		}
	}
	return ConditionSpec{}, "", fmt.Errorf("unknown condition id %q", id)
}

func validateSubmitInput(condition ConditionSpec, conditionHash string, input *SubmitVerifierInput) error {
	if strings.TrimSpace(input.ConditionID) == "" {
		input.ConditionID = condition.ID
	}
	if input.ConditionID != condition.ID {
		return fmt.Errorf("condition id mismatch: got %q want %q", input.ConditionID, condition.ID)
	}
	input.ConditionFile = defaultString(input.ConditionFile, condition.Path)
	if input.ConditionFile != condition.Path {
		return fmt.Errorf("condition file mismatch for %s: got %q want %q", condition.ID, input.ConditionFile, condition.Path)
	}
	input.VerifierPolicy = normalizeConditionPolicy(condition)
	if isPrimarySubmit(input.ResponseCondition) {
		if err := validateSubmittedCondition(condition.ID, normalizeConditionPolicy(condition), input.ResponseCondition); err != nil {
			return err
		}
	}
	for i, supplemental := range input.Supplemental {
		if err := validateSupplementalBinding(condition.ID, conditionHash, input, supplemental); err != nil {
			return fmt.Errorf("supplemental_verifiers[%d]: %w", i, err)
		}
		if err := validateSupplementalVerifier(condition.ID, normalizeConditionPolicy(condition), supplemental); err != nil {
			return fmt.Errorf("supplemental_verifiers[%d]: %w", i, err)
		}
	}
	if input.Adjudication != nil {
		if err := validateAdjudication(*input.Adjudication); err != nil {
			return err
		}
	}
	if !isPrimarySubmit(input.ResponseCondition) && len(input.Supplemental) == 0 && input.Adjudication == nil {
		return fmt.Errorf("condition %q submit contains no primary verdict, supplemental verifier, or adjudication", condition.ID)
	}
	return nil
}

func validateSubmittedCondition(conditionID string, policy attestations.VerifierPolicy, response ResponseCondition) error {
	if err := validateVerifierProvenance(conditionID, response.Verifier, response.SubagentConfirmed, response.SubagentModel); err != nil {
		return err
	}
	if !attestations.VerifierPolicyAllows(policy, effectiveResponseVerifierKind(response)) {
		return fmt.Errorf("condition %q verifier policy %q does not allow verifier kind %q: %s", conditionID, policy, effectiveResponseVerifierKind(response), attestations.VerifierPolicyRecovery(policy, conditionID))
	}
	return validateVerdictEvidence(conditionID, response.Verdict, response.Message, response.Evidence, response.NextAction)
}

func validateSupplementalVerifier(conditionID string, policy attestations.VerifierPolicy, supplemental SupplementalVerifier) error {
	if err := validateVerifierProvenance(conditionID, supplemental.Verifier, supplemental.SubagentConfirmed, supplemental.SubagentModel); err != nil {
		return err
	}
	kind := attestations.EffectiveVerifierKind(supplemental.Verifier, supplemental.SubagentConfirmed)
	if !attestations.VerifierPolicyAllows(policy, kind) {
		return fmt.Errorf("condition %q verifier policy %q does not allow supplemental verifier kind %q: %s", conditionID, policy, kind, attestations.VerifierPolicyRecovery(policy, conditionID))
	}
	return validateVerdictEvidence(conditionID, supplemental.Verdict, supplemental.Message, supplemental.Evidence, supplemental.NextAction)
}

func validateVerifierProvenance(conditionID string, verifier attestations.Verifier, legacyConfirmed bool, legacyModel string) error {
	kind := attestations.EffectiveVerifierKind(verifier, legacyConfirmed)
	if !attestations.ValidVerifierKind(kind) || kind == attestations.VerifierNone || kind == attestations.VerifierUnknown {
		return fmt.Errorf("condition %q requires verifier provenance kind", conditionID)
	}
	if strings.TrimSpace(verifier.Model) == "" && strings.TrimSpace(legacyModel) == "" {
		return fmt.Errorf("condition %q requires verifier model", conditionID)
	}
	if strings.TrimSpace(verifier.Runtime) == "" {
		return fmt.Errorf("condition %q requires verifier runtime", conditionID)
	}
	if !verifier.SeparateContext {
		return fmt.Errorf("condition %q requires verifier separate_context=true", conditionID)
	}
	return nil
}

func validateVerdictEvidence(conditionID string, verdict attestations.Verdict, message string, evidence []string, nextAction string) error {
	switch verdict {
	case attestations.VerdictPass, attestations.VerdictNotApplicable, attestations.VerdictFail, attestations.VerdictUnknown:
	default:
		return fmt.Errorf("condition %q has invalid verdict %q", conditionID, verdict)
	}
	if len(nonEmptyEvidence(evidence)) == 0 {
		return fmt.Errorf("condition %q has %s verdict without evidence", conditionID, verdict)
	}
	if evidenceOnlyAuthorization(evidence) {
		return fmt.Errorf("condition %q evidence is only authorization metadata or D7 boilerplate", conditionID)
	}
	switch verdict {
	case attestations.VerdictNotApplicable, attestations.VerdictFail, attestations.VerdictUnknown:
		if strings.TrimSpace(message) == "" {
			return fmt.Errorf("condition %q has %s verdict without message", conditionID, verdict)
		}
	}
	switch verdict {
	case attestations.VerdictFail, attestations.VerdictUnknown:
		if strings.TrimSpace(nextAction) == "" {
			return fmt.Errorf("condition %q has %s verdict without next_action", conditionID, verdict)
		}
	}
	return nil
}

func validateSupplementalBinding(conditionID, conditionHash string, input *SubmitVerifierInput, supplemental SupplementalVerifier) error {
	if supplemental.StagedPayloadHash != "" && supplemental.StagedPayloadHash != input.StagedPayloadHash {
		return fmt.Errorf("staged payload binding is stale for %s: got %q want %q", conditionID, supplemental.StagedPayloadHash, input.StagedPayloadHash)
	}
	if supplemental.ManifestHash != "" && supplemental.ManifestHash != input.ManifestHash {
		return fmt.Errorf("manifest binding is stale for %s: got %q want %q", conditionID, supplemental.ManifestHash, input.ManifestHash)
	}
	if supplemental.ConditionFileHash != "" && supplemental.ConditionFileHash != conditionHash {
		return fmt.Errorf("condition file binding is stale for %s: got %q want %q", conditionID, supplemental.ConditionFileHash, conditionHash)
	}
	return nil
}

func validateAdjudication(adjudication ResponseAdjudication) error {
	if strings.TrimSpace(adjudication.Authority) == "" {
		return errors.New("adjudication authority is required")
	}
	if strings.TrimSpace(adjudication.Summary) == "" {
		return errors.New("adjudication summary is required")
	}
	if strings.TrimSpace(adjudication.AuditRef) == "" {
		return errors.New("adjudication audit_ref is required")
	}
	if adjudication.FinalVerdict != "" {
		switch adjudication.FinalVerdict {
		case attestations.VerdictPass, attestations.VerdictNotApplicable, attestations.VerdictFail, attestations.VerdictUnknown:
		default:
			return fmt.Errorf("adjudication final_verdict is invalid: %q", adjudication.FinalVerdict)
		}
	}
	return nil
}

func mergeVerifierResponse(root, rel string, condition ConditionSpec, input *SubmitVerifierInput, timeout time.Duration) ([]string, error) {
	path := responseFilePath(root, rel)
	var warnings []string
	if err := withFileLock(path+".lock", timeout, func() error {
		responses, err := LoadResponses(path)
		if err != nil {
			return err
		}
		if err := validateResponseFileBinding(responses, input); err != nil {
			return err
		}
		index := -1
		for i := range responses.Conditions {
			if responses.Conditions[i].ConditionID == condition.ID {
				index = i
				break
			}
		}
		if index < 0 {
			return fmt.Errorf("response file is missing condition %q", condition.ID)
		}
		current := responses.Conditions[index]
		merged := current
		if isPrimarySubmit(input.ResponseCondition) {
			if current.Verdict == attestations.VerdictPass || current.Verdict == attestations.VerdictNotApplicable {
				warnings = append(warnings, "replacing already-populated condition "+condition.ID)
			}
			merged = input.ResponseCondition
			merged.Supplemental = current.Supplemental
			merged.Adjudication = current.Adjudication
		}
		merged.ConditionID = condition.ID
		merged.ConditionFile = condition.Path
		merged.VerifierPolicy = normalizeConditionPolicy(condition)
		if len(input.Supplemental) > 0 {
			var supplementalWarnings []string
			merged.Supplemental, supplementalWarnings = mergeSupplementalVerifiers(merged.Supplemental, input.Supplemental)
			warnings = append(warnings, supplementalWarnings...)
		}
		if input.Adjudication != nil {
			if merged.Adjudication != nil {
				warnings = append(warnings, "replaced adjudication for "+condition.ID)
			}
			merged.Adjudication = input.Adjudication
		}
		responses.Conditions[index] = merged
		if err := writeResponsesAtomic(path, *responses); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return warnings, nil
}

func responseFilePath(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, filepath.FromSlash(path))
}

func validateResponseFileBinding(responses *Responses, input *SubmitVerifierInput) error {
	if responses.Binding.StagedPayloadHash == "" {
		return errors.New("response file is missing binding.staged_payload_hash; rerun verifier begin")
	}
	if responses.Binding.StagedPayloadHash != input.StagedPayloadHash {
		return fmt.Errorf("response file staged payload binding is stale: got %q want %q", responses.Binding.StagedPayloadHash, input.StagedPayloadHash)
	}
	if responses.Binding.ManifestHash != input.ManifestHash {
		return fmt.Errorf("response file manifest binding is stale: got %q want %q", responses.Binding.ManifestHash, input.ManifestHash)
	}
	for _, binding := range responses.Binding.Conditions {
		if binding.ConditionID == input.ConditionID {
			if binding.ConditionFileHash != input.ConditionFileHash {
				return fmt.Errorf("response file condition binding is stale for %s: got %q want %q", input.ConditionID, binding.ConditionFileHash, input.ConditionFileHash)
			}
			return nil
		}
	}
	return fmt.Errorf("response file binding is missing condition %q", input.ConditionID)
}

func mergeSupplementalVerifiers(existing, incoming []SupplementalVerifier) ([]SupplementalVerifier, []string) {
	merged := append([]SupplementalVerifier(nil), existing...)
	var warnings []string
	for _, next := range incoming {
		key := supplementalKey(next)
		replaced := false
		for i, current := range merged {
			if supplementalKey(current) == key {
				merged[i] = next
				replaced = true
				warnings = append(warnings, "replaced duplicate supplemental verifier "+key)
				break
			}
		}
		if !replaced {
			merged = append(merged, next)
		}
	}
	return merged, warnings
}

func supplementalKey(s SupplementalVerifier) string {
	parts := []string{
		string(attestations.EffectiveVerifierKind(s.Verifier, s.SubagentConfirmed)),
		strings.TrimSpace(s.Verifier.Agent),
		strings.TrimSpace(s.Verifier.Model),
		strings.TrimSpace(s.Verifier.Runtime),
	}
	return strings.Join(parts, "/")
}

func isPrimarySubmit(response ResponseCondition) bool {
	return response.Verdict != "" ||
		len(response.Evidence) > 0 ||
		strings.TrimSpace(response.Message) != "" ||
		response.Verifier.Kind != "" ||
		response.SubagentConfirmed
}

func withFileLock(path string, timeout time.Duration, fn func() error) error {
	deadline := time.Now().Add(timeout)
	for {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			file.Close()
			defer os.Remove(path)
			return fn()
		}
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("acquire response lock: %w", err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for response lock %s", filepath.ToSlash(path))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func writeResponsesAtomic(path string, responses Responses) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(responses, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func storeSubmitTranscript(root string, plan Plan, opts SubmitVerifierOptions, input *SubmitVerifierInput) (string, error) {
	if strings.TrimSpace(opts.TranscriptPath) == "" {
		return "", nil
	}
	body, err := readTranscript(root, opts)
	if err != nil {
		return "", err
	}
	mode, dir := transcriptConfig(root)
	conditionID := strings.TrimSpace(opts.ConditionID)
	if conditionID == "" {
		conditionID = input.ConditionID
	}
	rel := filepath.ToSlash(filepath.Join(dir, transcriptFileName(plan.StagedPayloadHash, conditionID)))
	content := body
	if mode == "summary" || mode == "" {
		content = summarizeTranscript(body)
	}
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return rel, nil
}

func readTranscript(root string, opts SubmitVerifierOptions) (string, error) {
	if opts.TranscriptPath == "-" {
		if opts.TranscriptReader == nil {
			return "", errors.New("--transcript - requires transcript stdin")
		}
		body, err := io.ReadAll(opts.TranscriptReader)
		if err != nil {
			return "", fmt.Errorf("read transcript stdin: %w", err)
		}
		return string(body), nil
	}
	path := filepath.Join(root, filepath.FromSlash(opts.TranscriptPath))
	if filepath.IsAbs(opts.TranscriptPath) {
		path = opts.TranscriptPath
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read transcript: %w", err)
	}
	return string(body), nil
}

func transcriptConfig(root string) (string, string) {
	effective, err := bvconfig.Load(root)
	if err != nil {
		return "summary", defaultTranscriptDir
	}
	defaults := effective.File.Defaults.Verifier
	mode := strings.TrimSpace(defaults.Transcripts)
	if mode == "" {
		mode = "summary"
	}
	dir := strings.TrimSpace(defaults.TranscriptDir)
	if dir == "" {
		dir = defaultTranscriptDir
	}
	return mode, dir
}

func transcriptFileName(payloadHash, conditionID string) string {
	hash := strings.TrimPrefix(payloadHash, HashPrefix)
	if len(hash) > 12 {
		hash = hash[:12]
	}
	if hash == "" {
		hash = "unknown"
	}
	conditionID = strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(conditionID)
	return hash + "-" + conditionID + ".md"
}

func summarizeTranscript(body string) string {
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) > 20 {
		lines = lines[:20]
		lines = append(lines, "(transcript summary truncated)")
	}
	return strings.Join(lines, "\n") + "\n"
}

func nonEmptyEvidence(evidence []string) []string {
	var out []string
	for _, entry := range evidence {
		if strings.TrimSpace(entry) != "" {
			out = append(out, entry)
		}
	}
	return out
}

func evidenceOnlyAuthorization(evidence []string) bool {
	evidence = nonEmptyEvidence(evidence)
	if len(evidence) == 0 {
		return false
	}
	for _, entry := range evidence {
		lower := strings.ToLower(entry)
		if !strings.Contains(lower, "authorization is never per-cell evidence") &&
			!strings.Contains(lower, "standing verifier authorization") &&
			!strings.Contains(lower, "authorization permits spawning") &&
			!strings.Contains(lower, "d7") {
			return false
		}
	}
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func lockTimeout(timeout time.Duration) time.Duration {
	if timeout > 0 {
		return timeout
	}
	return 5 * time.Second
}
