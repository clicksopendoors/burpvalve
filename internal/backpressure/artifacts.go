package backpressure

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"burpvalve/internal/attestations"
	"burpvalve/internal/charmui"
	"burpvalve/internal/cliui"
)

type Responses struct {
	Atomicity  attestations.Atomicity `json:"atomicity"`
	Binding    ResponseBinding        `json:"binding,omitempty"`
	Conditions []ResponseCondition    `json:"conditions"`
}

type ResponseBinding struct {
	StagedPayloadHash string                     `json:"staged_payload_hash"`
	ManifestHash      string                     `json:"manifest_hash"`
	Conditions        []ResponseConditionBinding `json:"conditions"`
}

type ResponseConditionBinding struct {
	ConditionID       string `json:"condition_id"`
	ConditionFile     string `json:"condition_file"`
	ConditionFileHash string `json:"condition_file_hash"`
}

type ResponseCondition struct {
	ConditionID       string                      `json:"condition_id"`
	ConditionFile     string                      `json:"condition_file,omitempty"`
	VerifierPolicy    attestations.VerifierPolicy `json:"verifier_policy,omitempty"`
	Verifier          attestations.Verifier       `json:"verifier,omitempty"`
	SubagentConfirmed bool                        `json:"subagent_confirmed"`
	SubagentModel     string                      `json:"subagent_model,omitempty"`
	Verdict           attestations.Verdict        `json:"verdict"`
	Message           string                      `json:"message"`
	Evidence          []string                    `json:"evidence"`
	NextAction        string                      `json:"next_action"`
	Supplemental      []SupplementalVerifier      `json:"supplemental_verifiers,omitempty"`
	Adjudication      *ResponseAdjudication       `json:"adjudication,omitempty"`
}

type PreCommitOptions struct {
	Root            string
	ExplicitFeature string
	BeadIDs         []string
	BeadRationale   string
	ResponsesPath   string
	Agent           string
	Model           string
	ColorMode       string
	Now             func() time.Time
	Staged          StagedReader
	Prompt          *PromptIO
}

const (
	hookContextEnv       = "BURPVALVE_HOOK_CONTEXT"
	hookCommandSourceEnv = "BURPVALVE_HOOK_COMMAND_SOURCE"
)

type PreCommitResult struct {
	SchemaVersion     int      `json:"schema_version"`
	Command           string   `json:"command"`
	Status            string   `json:"status"`
	Message           string   `json:"message"`
	Fatal             bool     `json:"fatal"`
	Warnings          []string `json:"warnings,omitempty"`
	NextSteps         []string `json:"next_steps,omitempty"`
	ArtifactPath      string   `json:"artifact_path,omitempty"`
	BlockedReportPath string   `json:"blocked_report_path,omitempty"`
	ResponsesPath     string   `json:"responses_path,omitempty"`
	Plan              Plan     `json:"plan"`
}

type BeginResponsesOptions struct {
	Root             string
	ExplicitFeature  string
	OneFeature       bool
	AtomicityMessage string
	Staged           StagedReader
}

type BeginResponsesResult struct {
	SchemaVersion     int      `json:"schema_version"`
	Command           string   `json:"command"`
	Status            string   `json:"status"`
	Message           string   `json:"message"`
	Fatal             bool     `json:"fatal"`
	NextSteps         []string `json:"next_steps,omitempty"`
	ResponsesPath     string   `json:"responses_path,omitempty"`
	StagedPayloadHash string   `json:"staged_payload_hash,omitempty"`
	ManifestHash      string   `json:"manifest_hash,omitempty"`
	Plan              Plan     `json:"plan"`
}

const (
	StatusPassed             = "passed"
	StatusBlocked            = "blocked"
	StatusAttestationWritten = "attestation_written"
	StatusResponsesWritten   = "responses_written"
)

func RunPreCommit(ctx context.Context, opts PreCommitOptions) (PreCommitResult, error) {
	root, err := filepath.Abs(defaultRoot(opts.Root))
	if err != nil {
		return PreCommitResult{}, err
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
	if planErr != nil && strings.TrimSpace(opts.ExplicitFeature) == "" && strings.TrimSpace(plan.BlockingReason) != "" {
		if feature, err := promptForFeature(plan, planErr, opts); err == nil {
			opts.ExplicitFeature = feature
			plan, planErr = BuildPlan(ctx, Options{
				Root:            root,
				Mode:            "pre-commit",
				ExplicitFeature: opts.ExplicitFeature,
				Staged:          staged,
			})
		} else if errors.Is(err, charmui.ErrCancelled) {
			planErr = fmt.Errorf("%w; feature prompt cancelled", planErr)
		}
	}
	result := PreCommitResult{
		SchemaVersion: 1,
		Command:       "commit",
		Status:        StatusBlocked,
		Fatal:         true,
		ArtifactPath:  AttestationPath(plan.StagedPayloadHash),
		Plan:          plan,
	}
	if planErr != nil {
		report, path, writeErr := writeBlockedReport(root, plan, nil, opts, "feature detection blocked: "+planErr.Error())
		result.BlockedReportPath = path
		result.Message = report.Atomicity.Message
		result.NextSteps = hookAwareNextSteps([]string{"Stage one atomic feature or pass --feature with the bead or feature id, then rerun burpvalve commit."})
		return result, firstErr(writeErr, planErr)
	}

	expected := ExpectedBinding(plan)
	if isStaged(plan, result.ArtifactPath) {
		body, err := staged.StagedFileContent(ctx, root, result.ArtifactPath)
		if err == nil {
			var artifact attestations.Artifact
			if json.Unmarshal(body, &artifact) == nil && artifact.ValidatePassing(expected) == nil {
				result.Status = StatusPassed
				result.Message = "staged backpressure attestation is valid"
				result.Fatal = false
				return result, nil
			}
		}
	}
	if strings.TrimSpace(opts.ResponsesPath) == "" && opts.Prompt == nil {
		if stale := stagedAttestationPaths(plan, result.ArtifactPath); len(stale) > 0 {
			message := "staged backpressure attestation is stale or invalid for the current payload: " + strings.Join(stale, ", ")
			report, path, writeErr := writeBlockedReport(root, plan, nil, opts, message)
			result.BlockedReportPath = path
			result.Message = report.Atomicity.Message
			result.NextSteps = hookAwareNextSteps([]string{
				"Rerun burpvalve commit with current verifier responses for this staged payload.",
				"Stage the newly written attestation for " + result.ArtifactPath + " before committing.",
			})
			return result, firstErr(writeErr, errors.New(message))
		}
	}
	responses, promptOut, responseInfo, err := responsesForPreCommit(root, plan, opts)
	result.ResponsesPath = responseInfo.Path
	result.Warnings = append(result.Warnings, responseInfo.Warnings...)
	if err != nil {
		if promptOut != nil {
			WriteResponseSummaryWithOptions(promptOut, plan, responses, TextOptions{Color: promptColor(opts)})
		}
		report, path, writeErr := writeBlockedReport(root, plan, responses, opts, err.Error())
		result.BlockedReportPath = path
		result.Message = report.Atomicity.Message
		result.NextSteps = hookAwareNextSteps(responseErrorNextSteps(err, responseInfo.Path))
		return result, firstErr(writeErr, err)
	}
	if promptOut != nil {
		WriteResponseSummaryWithOptions(promptOut, plan, responses, TextOptions{Color: promptColor(opts)})
	}

	if err := validateResponses(plan, responses); err != nil {
		report, path, writeErr := writeBlockedReport(root, plan, responses, opts, err.Error())
		result.BlockedReportPath = path
		result.Message = report.Atomicity.Message
		result.NextSteps = hookAwareNextSteps([]string{"Fix the verifier responses so every required feature x condition cell has acceptable evidence, then rerun burpvalve commit."})
		return result, firstErr(writeErr, err)
	}

	artifact := BuildArtifact(plan, responses, opts, attestations.ArtifactPassing)
	if err := artifact.ValidatePassing(expected); err != nil {
		report, path, writeErr := writeBlockedReport(root, plan, responses, opts, "matrix is not accepted: "+err.Error())
		result.BlockedReportPath = path
		result.Message = report.Atomicity.Message
		result.NextSteps = hookAwareNextSteps([]string{"Fix verifier policy or response evidence for the blocked cells, then rerun burpvalve commit."})
		return result, firstErr(writeErr, err)
	}

	if err := writeArtifact(root, result.ArtifactPath, artifact); err != nil {
		result.NextSteps = []string{"Fix the filesystem error that prevented writing the attestation, then rerun burpvalve commit."}
		return result, err
	}
	result.Status = StatusAttestationWritten
	result.Message = "wrote passing attestation; stage it with: git add " + result.ArtifactPath
	result.NextSteps = hookAwareNextSteps([]string{"git add " + result.ArtifactPath, "rerun git commit"})
	return result, errors.New(result.Message)
}

func hookAwareNextSteps(base []string) []string {
	if os.Getenv(hookContextEnv) != "pre-commit" {
		return base
	}
	source := hookCommandSourceLabel(os.Getenv(hookCommandSourceEnv))
	steps := []string{
		"Pre-commit hook context: Burpvalve was invoked by the git hook using " + source + ".",
		"Keep the current staged payload intact while collecting verifier evidence; do not treat this hook failure as evidence that lint or verifier checks passed.",
	}
	steps = append(steps, base...)
	if containsStep(base, "git add ") || containsStep(base, "Stage the newly written attestation") {
		steps = append(steps, "After staging the attestation, rerun git commit so the hook can revalidate the final payload.")
	} else {
		steps = append(steps, "After the response file is current for this staged payload, rerun git commit so the hook can revalidate and then run lint.")
	}
	return steps
}

func hookCommandSourceLabel(source string) string {
	switch source {
	case "source":
		return "the source checkout"
	case "path":
		return "the PATH burpvalve command"
	case "repo-local":
		return "the repo-local fallback binary"
	default:
		return "an unknown command source"
	}
}

func containsStep(steps []string, needle string) bool {
	for _, step := range steps {
		if strings.Contains(step, needle) {
			return true
		}
	}
	return false
}

type preCommitResponseInfo struct {
	Path     string
	Warnings []string
}

func responsesForPreCommit(root string, plan Plan, opts PreCommitOptions) (*Responses, io.Writer, preCommitResponseInfo, error) {
	if strings.TrimSpace(opts.ResponsesPath) != "" {
		responses, err := LoadResponses(opts.ResponsesPath)
		info := preCommitResponseInfo{Path: opts.ResponsesPath}
		if err == nil && !responsesHasBinding(responses) {
			info.Warnings = append(info.Warnings, "legacy unbound responses file accepted; prefer `burpvalve verifier begin` and `burpvalve verifier submit` so commit can auto-discover hash-bound responses")
		}
		return responses, nil, info, err
	}
	autoPath := ResponsesPath(plan.StagedPayloadHash)
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(autoPath))); err == nil {
		responses, loadErr := LoadResponses(filepath.Join(root, filepath.FromSlash(autoPath)))
		return responses, nil, preCommitResponseInfo{Path: autoPath}, loadErr
	} else if err != nil && !os.IsNotExist(err) {
		return nil, nil, preCommitResponseInfo{Path: autoPath}, fmt.Errorf("inspect auto-discovered responses %s: %w", autoPath, err)
	}
	if stale := staleResponsePaths(root, autoPath); len(stale) > 0 {
		return nil, nil, preCommitResponseInfo{Path: autoPath}, fmt.Errorf("responses exist for a different staged payload: %s; rerun burpvalve verifier begin for the current staged payload", strings.Join(stale, ", "))
	}
	prompt := opts.Prompt
	var closePrompt func() error
	if prompt == nil {
		var err error
		prompt, closePrompt, err = openTTYPrompt(opts.ColorMode)
		if err != nil {
			return nil, nil, preCommitResponseInfo{Path: autoPath}, fmt.Errorf("no /dev/tty available; run burpvalve verifier begin, collect verifier submit responses, then rerun burpvalve commit, or pass --responses <file>")
		}
		defer closePrompt()
	}
	out := prompt.Out
	if out == nil {
		out = io.Discard
	}
	responses, err := CollectPromptResponses(plan, PromptIO{In: prompt.In, Out: out, TUI: prompt.TUI, Color: prompt.Color})
	return responses, out, preCommitResponseInfo{Path: autoPath}, err
}

func responseErrorNextSteps(err error, responsesPath string) []string {
	message := ""
	if err != nil {
		message = err.Error()
	}
	switch {
	case strings.Contains(message, "different staged payload"):
		return []string{
			"Rerun burpvalve verifier begin for the current staged payload.",
			"Resend verifier packets and record fresh verifier submit responses.",
			"Rerun burpvalve commit after " + defaultString(responsesPath, "the hash-bound response file") + " matches the staged payload.",
		}
	case strings.Contains(message, "no /dev/tty"):
		return []string{
			"Run burpvalve verifier begin for this staged payload.",
			"Collect real verifier responses with burpvalve verifier submit.",
			"Rerun burpvalve commit, or pass --responses for a legacy response file.",
		}
	default:
		return []string{"Provide verifier responses with --responses, rerun in an interactive terminal, or inspect the blocked report before trying again."}
	}
}

func staleResponsePaths(root, expected string) []string {
	matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash("log/backpressure/responses/*.json")))
	if err != nil {
		return nil
	}
	expectedAbs := filepath.Clean(filepath.Join(root, filepath.FromSlash(expected)))
	var stale []string
	for _, match := range matches {
		if filepath.Clean(match) == expectedAbs {
			continue
		}
		if rel, err := filepath.Rel(root, match); err == nil {
			stale = append(stale, filepath.ToSlash(rel))
		}
	}
	sort.Strings(stale)
	return stale
}

func openTTYPrompt(colorMode string) (*PromptIO, func() error, error) {
	file, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	return &PromptIO{In: file, Out: file, TUI: shouldUseTUI(), Color: shouldUsePromptColor(colorMode)}, file.Close, nil
}

func shouldUseTUI() bool {
	return os.Getenv("NO_TUI") == "" && os.Getenv("CI") == "" && os.Getenv("TERM") != "dumb"
}

func shouldUsePromptColor(colorMode string) bool {
	switch colorMode {
	case "always":
		return true
	case "never":
		return false
	}
	return os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
}

func promptColor(opts PreCommitOptions) bool {
	if opts.Prompt == nil {
		return shouldUsePromptColor(opts.ColorMode)
	}
	return opts.Prompt.Color
}

func ttyAvailable() bool {
	file, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	file.Close()
	return true
}

func promptForFeature(plan Plan, planErr error, opts PreCommitOptions) (string, error) {
	prompt := opts.Prompt
	var closePrompt func() error
	if prompt == nil {
		var err error
		prompt, closePrompt, err = openTTYPrompt(opts.ColorMode)
		if err != nil {
			return "", err
		}
		defer closePrompt()
	}
	description := "Burpvalve could not infer one atomic feature from the staged paths.\n" +
		"Reason: " + planErr.Error()
	if len(plan.StagedPayloadPaths) > 0 {
		description += "\n\nStaged paths:\n- " + strings.Join(plan.StagedPayloadPaths, "\n- ")
	}
	if !prompt.TUI {
		out := prompt.Out
		if out == nil {
			out = io.Discard
		}
		if prompt.In == nil {
			return "", fmt.Errorf("interactive prompt input is unavailable")
		}
		ui := cliui.New(prompt.Color)
		fmt.Fprintln(out, ui.Title("Feature for this commit"))
		fmt.Fprintln(out, description)
		return askRequired(out, bufio.NewScanner(prompt.In), "Feature, bug fix, or bead id for this staged commit: ", prompt.Color)
	}
	feature, err := charmui.AskText(prompt.In, prompt.Out, charmui.TextPrompt{
		Title:       "Feature for this commit",
		Description: description,
		Prompt:      "What feature, bug fix, or bead id describes this staged commit?",
		Placeholder: "br-123 or scaffold-init",
		Required:    true,
		Color:       prompt.Color,
	})
	if err != nil {
		return "", err
	}
	return feature, nil
}

func LoadResponses(path string) (*Responses, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("missing matrix responses; run the verifier interactively or pass --responses <file>")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read responses: %w", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse responses: %w", err)
	}
	conditions, ok := raw["conditions"]
	if !ok {
		return nil, fmt.Errorf("responses file must contain top-level conditions array")
	}
	var conditionProbe []json.RawMessage
	if err := json.Unmarshal(conditions, &conditionProbe); err != nil {
		return nil, fmt.Errorf("responses file conditions must be an array: %w", err)
	}
	var responses Responses
	if err := json.Unmarshal(body, &responses); err != nil {
		return nil, fmt.Errorf("parse responses: %w", err)
	}
	return &responses, nil
}

func BuildResponsesTemplate(plan Plan) Responses {
	return BuildBoundResponsesTemplate(plan, attestations.Atomicity{
		OneFeatureOrFix: false,
		Message:         "Describe why the staged diff is exactly one feature or bug fix.",
	})
}

func BuildBoundResponsesTemplate(plan Plan, atomicity attestations.Atomicity) Responses {
	template := Responses{
		Atomicity: atomicity,
		Binding:   BuildResponseBinding(plan),
	}
	for _, condition := range plan.Matrix.Conditions {
		template.Conditions = append(template.Conditions, ResponseCondition{
			ConditionID:       condition.ID,
			ConditionFile:     condition.Path,
			VerifierPolicy:    normalizeConditionPolicy(condition),
			Verifier:          responseTemplateVerifier(),
			SubagentConfirmed: false,
			SubagentModel:     "",
			Verdict:           attestations.VerdictUnknown,
			Message:           "Replace with verifier result for " + condition.ID + ".",
			Evidence:          []string{},
			NextAction:        "Replace with next action when verdict is fail or unknown.",
		})
	}
	return template
}

func BuildResponseBinding(plan Plan) ResponseBinding {
	binding := ResponseBinding{
		StagedPayloadHash: plan.StagedPayloadHash,
		ManifestHash:      plan.ManifestHash,
	}
	for _, condition := range plan.Matrix.Conditions {
		binding.Conditions = append(binding.Conditions, ResponseConditionBinding{
			ConditionID:       condition.ID,
			ConditionFile:     condition.Path,
			ConditionFileHash: plan.ConditionFileHashes[condition.ID],
		})
	}
	return binding
}

func RunVerifierBegin(ctx context.Context, opts BeginResponsesOptions) (BeginResponsesResult, error) {
	root, err := filepath.Abs(defaultRoot(opts.Root))
	if err != nil {
		return BeginResponsesResult{}, err
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
	result := BeginResponsesResult{
		SchemaVersion: 1,
		Command:       "verifier begin",
		Status:        StatusBlocked,
		Fatal:         true,
		Plan:          plan,
	}
	if planErr != nil {
		result.Message = planErr.Error()
		result.NextSteps = []string{"Stage one atomic feature or pass --feature with the bead or feature id, then rerun burpvalve verifier begin."}
		return result, planErr
	}
	result.StagedPayloadHash = plan.StagedPayloadHash
	result.ManifestHash = plan.ManifestHash
	result.ResponsesPath = ResponsesPath(plan.StagedPayloadHash)
	if !opts.OneFeature {
		result.Message = "atomicity not confirmed: pass --one-feature and --atomicity-message"
		result.NextSteps = []string{"Rerun burpvalve verifier begin with --one-feature --atomicity-message \"why this staged payload is exactly one feature or bug fix\"."}
		return result, errors.New(result.Message)
	}
	if strings.TrimSpace(opts.AtomicityMessage) == "" {
		result.Message = "atomicity message is required"
		result.NextSteps = []string{"Provide --atomicity-message describing why this staged payload is exactly one feature or bug fix."}
		return result, errors.New(result.Message)
	}
	responses := BuildBoundResponsesTemplate(plan, attestations.Atomicity{
		OneFeatureOrFix: true,
		Message:         strings.TrimSpace(opts.AtomicityMessage),
	})
	if err := writeResponses(root, result.ResponsesPath, responses); err != nil {
		result.Message = err.Error()
		result.NextSteps = []string{"Fix the filesystem error that prevented writing the response file, then rerun burpvalve verifier begin."}
		return result, err
	}
	result.Status = StatusResponsesWritten
	result.Message = "wrote bound verifier responses file: " + result.ResponsesPath
	result.Fatal = false
	result.NextSteps = []string{"Send verifier packets, then record each verdict into " + result.ResponsesPath + " before running burpvalve commit."}
	return result, nil
}

func validateResponses(plan Plan, responses *Responses) error {
	if responses == nil {
		return fmt.Errorf("missing matrix responses")
	}
	if !responses.Atomicity.OneFeatureOrFix {
		return fmt.Errorf("atomicity not confirmed: %s", responses.Atomicity.Message)
	}
	bound := responsesHasBinding(responses)
	if bound {
		if err := validateResponsesBinding(plan, responses); err != nil {
			return err
		}
	}
	expected := map[string]bool{}
	for _, condition := range plan.Matrix.Conditions {
		expected[condition.ID] = true
	}
	seen := map[string]ResponseCondition{}
	for _, response := range responses.Conditions {
		if !expected[response.ConditionID] {
			return fmt.Errorf("unexpected condition response %q", response.ConditionID)
		}
		if _, ok := seen[response.ConditionID]; ok {
			return fmt.Errorf("duplicate condition response %q", response.ConditionID)
		}
		seen[response.ConditionID] = response
	}
	for _, condition := range plan.Matrix.Conditions {
		response, ok := seen[condition.ID]
		if !ok {
			return fmt.Errorf("missing condition response %q", condition.ID)
		}
		if err := validateResponseCondition(condition, response, bound); err != nil {
			return err
		}
	}
	return nil
}

func validateResponseCondition(condition ConditionSpec, response ResponseCondition, bound bool) error {
	conditionID := condition.ID
	if !attestations.ValidVerifierKind(effectiveResponseVerifierKind(response)) {
		return fmt.Errorf("condition %q has invalid verifier kind %q", conditionID, response.Verifier.Kind)
	}
	policy := normalizeConditionPolicy(condition)
	if !attestations.VerifierPolicyAllows(policy, effectiveResponseVerifierKind(response)) {
		if strings.TrimSpace(response.Message) == "" {
			return fmt.Errorf("condition %q verifier policy %q does not allow verifier kind %q without a message", conditionID, policy, effectiveResponseVerifierKind(response))
		}
		return fmt.Errorf("condition %q verifier policy %q does not allow verifier kind %q: %s", conditionID, policy, effectiveResponseVerifierKind(response), attestations.VerifierPolicyRecovery(policy, conditionID))
	}
	switch response.Verdict {
	case attestations.VerdictPass:
		if bound && len(nonEmptyEvidence(response.Evidence)) == 0 {
			return fmt.Errorf("condition %q has pass verdict without evidence", conditionID)
		}
		return nil
	case attestations.VerdictNotApplicable:
		if strings.TrimSpace(response.Message) == "" {
			return fmt.Errorf("condition %q is not_applicable without a message", conditionID)
		}
		return nil
	case attestations.VerdictFail, attestations.VerdictUnknown:
		if strings.TrimSpace(response.Message) == "" {
			return fmt.Errorf("condition %q has %s verdict without blocker message", conditionID, response.Verdict)
		}
		if len(response.Evidence) == 0 {
			return fmt.Errorf("condition %q has %s verdict without evidence", conditionID, response.Verdict)
		}
		if strings.TrimSpace(response.NextAction) == "" {
			return fmt.Errorf("condition %q has %s verdict without next action", conditionID, response.Verdict)
		}
		return fmt.Errorf("condition %q has blocking verdict %q", conditionID, response.Verdict)
	default:
		return fmt.Errorf("condition %q has invalid verdict %q", conditionID, response.Verdict)
	}
}

func responsesHasBinding(responses *Responses) bool {
	return responses != nil &&
		(strings.TrimSpace(responses.Binding.StagedPayloadHash) != "" ||
			strings.TrimSpace(responses.Binding.ManifestHash) != "" ||
			len(responses.Binding.Conditions) > 0)
}

func validateResponsesBinding(plan Plan, responses *Responses) error {
	if strings.TrimSpace(responses.Binding.StagedPayloadHash) == "" {
		return fmt.Errorf("bound responses missing binding.staged_payload_hash")
	}
	if responses.Binding.StagedPayloadHash != plan.StagedPayloadHash {
		return fmt.Errorf("bound responses staged payload binding is stale: got %q want %q", responses.Binding.StagedPayloadHash, plan.StagedPayloadHash)
	}
	if strings.TrimSpace(responses.Binding.ManifestHash) == "" {
		return fmt.Errorf("bound responses missing binding.manifest_hash")
	}
	if responses.Binding.ManifestHash != plan.ManifestHash {
		return fmt.Errorf("bound responses manifest binding is stale: got %q want %q", responses.Binding.ManifestHash, plan.ManifestHash)
	}
	expected := map[string]string{}
	for _, condition := range plan.Matrix.Conditions {
		expected[condition.ID] = plan.ConditionFileHashes[condition.ID]
	}
	seen := map[string]bool{}
	for _, binding := range responses.Binding.Conditions {
		hash, ok := expected[binding.ConditionID]
		if !ok {
			return fmt.Errorf("bound responses contain unexpected condition binding %q", binding.ConditionID)
		}
		if binding.ConditionFileHash == "" {
			return fmt.Errorf("bound responses condition %q missing condition_file_hash", binding.ConditionID)
		}
		if binding.ConditionFileHash != hash {
			return fmt.Errorf("bound responses condition %q file hash is stale: got %q want %q", binding.ConditionID, binding.ConditionFileHash, hash)
		}
		seen[binding.ConditionID] = true
	}
	for conditionID := range expected {
		if !seen[conditionID] {
			return fmt.Errorf("bound responses missing condition binding %q", conditionID)
		}
	}
	return nil
}

func BuildArtifact(plan Plan, responses *Responses, opts PreCommitOptions, kind attestations.ArtifactKind) attestations.Artifact {
	now := time.Now().UTC()
	if opts.Now != nil {
		now = opts.Now().UTC()
	}
	artifact := attestations.Artifact{
		SchemaVersion:        1,
		Tool:                 attestations.ToolName,
		ToolVersion:          attestations.ToolVersion,
		ArtifactKind:         kind,
		StagedPayloadHash:    plan.StagedPayloadHash,
		ManifestHash:         plan.ManifestHash,
		ConditionOrder:       append([]string(nil), plan.ConditionOrder...),
		GeneratedBy:          attestations.Generator{Agent: defaultString(opts.Agent, "codex"), Model: defaultString(opts.Model, "unspecified")},
		GitHeadBeforeCommit:  gitHead(opts.Root),
		CreatedAt:            now,
		Feature:              artifactFeature(plan),
		BeadIDs:              cleanBeadIDs(opts.BeadIDs),
		CoupledWorkRationale: strings.TrimSpace(opts.BeadRationale),
		Atomicity: attestations.Atomicity{
			OneFeatureOrFix: false,
			Message:         "Missing matrix responses.",
		},
	}
	if len(artifact.BeadIDs) > 0 {
		artifact.Feature.BeadIDs = append([]string(nil), artifact.BeadIDs...)
		if len(artifact.BeadIDs) == 1 {
			artifact.Feature.SourceBead = artifact.BeadIDs[0]
		} else {
			artifact.Feature.SourceBead = ""
			if artifact.Feature.DiffCluster == "" {
				artifact.Feature.DiffCluster = artifact.Feature.ID
			}
		}
	}
	if responses != nil {
		artifact.Atomicity = responses.Atomicity
	}
	responseByCondition := map[string]ResponseCondition{}
	if responses != nil {
		for _, response := range responses.Conditions {
			responseByCondition[response.ConditionID] = response
		}
	}
	for _, condition := range plan.Matrix.Conditions {
		response, ok := responseByCondition[condition.ID]
		cell := attestations.Condition{
			ConditionID:       condition.ID,
			ConditionFile:     condition.Path,
			ConditionFileHash: plan.ConditionFileHashes[condition.ID],
			VerifierPolicy:    normalizeConditionPolicy(condition),
			Verifier:          attestations.Verifier{Kind: attestations.VerifierNone},
			SubagentConfirmed: false,
			Verdict:           attestations.VerdictUnknown,
			Message:           "Missing response for condition " + condition.ID + ".",
			Evidence:          []string{"backpressure matrix response missing"},
			NextAction:        "Spawn a verifier subagent for " + condition.ID + " and rerun burpvalve commit.",
			Timestamp:         now,
		}
		if ok {
			cell.Verifier = responseVerifier(response, now)
			cell.SubagentConfirmed = response.SubagentConfirmed || cell.Verifier.Kind == attestations.VerifierIndependentSubagent
			cell.SubagentModel = defaultString(response.SubagentModel, cell.Verifier.Model)
			cell.Verdict = response.Verdict
			cell.Message = response.Message
			cell.Evidence = append([]string(nil), response.Evidence...)
			cell.NextAction = response.NextAction
			cell.Supplemental = append([]attestations.SupplementalVerifier(nil), response.Supplemental...)
			if response.Adjudication != nil {
				adjudication := *response.Adjudication
				cell.Adjudication = &adjudication
			}
		}
		artifact.Conditions = append(artifact.Conditions, cell)
	}
	return artifact
}

func cleanBeadIDs(ids []string) []string {
	seen := map[string]bool{}
	var clean []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		clean = append(clean, id)
	}
	return clean
}

func AttestationPath(payloadHash string) string {
	hash := strings.TrimPrefix(payloadHash, HashPrefix)
	if hash == "" {
		hash = "unknown"
	}
	return "backpressure/attestations/" + hash + ".json"
}

func ResponsesPath(payloadHash string) string {
	hash := strings.TrimPrefix(payloadHash, HashPrefix)
	if hash == "" {
		hash = "unknown"
	}
	return "log/backpressure/responses/" + hash + ".json"
}

func BlockedReportPath(now time.Time) string {
	return "log/backpressure/failed/" + now.UTC().Format("20060102T150405Z") + "-blocked.json"
}

func writeResponses(root, rel string, responses Responses) error {
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(responses, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}

func writeBlockedReport(root string, plan Plan, responses *Responses, opts PreCommitOptions, message string) (attestations.Artifact, string, error) {
	now := time.Now().UTC()
	if opts.Now != nil {
		now = opts.Now().UTC()
	}
	report := BuildArtifact(plan, normalizeBlockedResponses(plan, responses, message), opts, attestations.ArtifactBlocked)
	report.Atomicity.OneFeatureOrFix = false
	report.Atomicity.Message = message
	report.NextSteps = blockedReportNextSteps(report)
	path := BlockedReportPath(now)
	err := writeArtifact(root, path, report)
	return report, path, err
}

func normalizeBlockedResponses(plan Plan, responses *Responses, reason string) *Responses {
	normalized := &Responses{
		Atomicity: attestations.Atomicity{
			OneFeatureOrFix: false,
			Message:         reason,
		},
	}
	if responses != nil && strings.TrimSpace(responses.Atomicity.Message) != "" {
		normalized.Atomicity.Message = responses.Atomicity.Message
	}
	byCondition := map[string]ResponseCondition{}
	if responses != nil {
		for _, response := range responses.Conditions {
			if _, ok := byCondition[response.ConditionID]; !ok {
				byCondition[response.ConditionID] = response
			}
		}
	}
	for _, condition := range plan.Matrix.Conditions {
		response, ok := byCondition[condition.ID]
		if !ok {
			normalized.Conditions = append(normalized.Conditions, ResponseCondition{
				ConditionID:       condition.ID,
				VerifierPolicy:    normalizeConditionPolicy(condition),
				Verifier:          attestations.Verifier{Kind: attestations.VerifierNone},
				SubagentConfirmed: false,
				Verdict:           attestations.VerdictUnknown,
				Message:           "Missing response for condition " + condition.ID + ".",
				Evidence:          []string{"backpressure matrix response missing"},
				NextAction:        attestations.VerifierPolicyRecovery(normalizeConditionPolicy(condition), condition.ID) + ", then rerun burpvalve commit.",
			})
			continue
		}
		normalized.Conditions = append(normalized.Conditions, normalizeBlockedCondition(condition, response, reason))
	}
	return normalized
}

func normalizeBlockedCondition(condition ConditionSpec, response ResponseCondition, reason string) ResponseCondition {
	conditionID := condition.ID
	if response.ConditionID == "" {
		response.ConditionID = conditionID
	}
	response.VerifierPolicy = normalizeConditionPolicy(condition)
	if response.Verifier.Kind == "" && !response.SubagentConfirmed {
		response.Verifier.Kind = attestations.VerifierNone
	}
	if !attestations.ValidVerifierKind(effectiveResponseVerifierKind(response)) || !attestations.VerifierPolicyAllows(response.VerifierPolicy, effectiveResponseVerifierKind(response)) {
		response.Verdict = attestations.VerdictUnknown
		response.Message = defaultString(response.Message, "Verifier evidence did not satisfy policy for "+conditionID+": "+reason)
		response.Evidence = defaultEvidence(response.Evidence)
		response.NextAction = defaultString(response.NextAction, attestations.VerifierPolicyRecovery(response.VerifierPolicy, conditionID)+", then rerun burpvalve commit.")
		return response
	}
	switch response.Verdict {
	case attestations.VerdictPass:
		return response
	case attestations.VerdictNotApplicable:
		if strings.TrimSpace(response.Message) == "" {
			response.Verdict = attestations.VerdictUnknown
			response.Message = "not_applicable verdict for " + conditionID + " is missing a reason: " + reason
			response.Evidence = defaultEvidence(response.Evidence)
			response.NextAction = defaultString(response.NextAction, "Rerun verifier and provide a not_applicable reason or a passing/failing verdict.")
		}
		return response
	case attestations.VerdictFail, attestations.VerdictUnknown:
		response.Message = defaultString(response.Message, "Blocking verdict for "+conditionID+": "+reason)
		response.Evidence = defaultEvidence(response.Evidence)
		response.NextAction = defaultString(response.NextAction, "Fix the blocker or rerun verifier with complete evidence and next action.")
		return response
	default:
		invalid := response.Verdict
		response.Verdict = attestations.VerdictUnknown
		response.Message = "Invalid verdict for " + conditionID + ": " + string(invalid)
		response.Evidence = defaultEvidence(response.Evidence)
		response.NextAction = "Rerun verifier and enter pass, not_applicable, fail, or unknown."
		return response
	}
}

func normalizeConditionPolicy(condition ConditionSpec) attestations.VerifierPolicy {
	return attestations.NormalizeVerifierPolicy(condition.VerifierPolicy)
}

func responseTemplateVerifier() attestations.Verifier {
	createdAt := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	return attestations.Verifier{
		Kind:            attestations.VerifierUnknown,
		Agent:           "replace-with-verifier-agent",
		Model:           "replace-with-verifier-model",
		Runtime:         "replace-with-runtime",
		SeparateContext: true,
		TranscriptRef:   "optional-transcript-path-or-hash",
		EvidenceRef:     "optional-evidence-path-or-hash",
		CreatedAt:       &createdAt,
	}
}

func effectiveResponseVerifierKind(response ResponseCondition) attestations.VerifierKind {
	return attestations.EffectiveVerifierKind(response.Verifier, response.SubagentConfirmed)
}

func responseVerifier(response ResponseCondition, now time.Time) attestations.Verifier {
	verifier := response.Verifier
	if verifier.Kind == "" && response.SubagentConfirmed {
		verifier.Kind = attestations.VerifierIndependentSubagent
		verifier.SeparateContext = true
	}
	if verifier.Kind == "" {
		verifier.Kind = attestations.VerifierNone
	}
	if verifier.Model == "" {
		verifier.Model = response.SubagentModel
	}
	if verifier.Kind == attestations.VerifierIndependentSubagent && verifier.CreatedAt == nil {
		createdAt := now
		verifier.CreatedAt = &createdAt
	}
	return verifier
}

func blockedReportNextSteps(report attestations.Artifact) []string {
	steps := []string{
		"The valve (the fail-closed commit gate) burped this work unit back (refused the atomic change being checked); fix the blocker below before rerunning.",
	}
	seen := map[string]bool{}
	seen[steps[0]] = true
	for _, condition := range report.Conditions {
		step := strings.TrimSpace(condition.NextAction)
		if step == "" || seen[step] {
			continue
		}
		seen[step] = true
		steps = append(steps, step)
	}
	return steps
}

func defaultEvidence(evidence []string) []string {
	if len(evidence) > 0 {
		return append([]string(nil), evidence...)
	}
	return []string{"backpressure prompt or response validation failed"}
}

func writeArtifact(root, rel string, artifact attestations.Artifact) error {
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}

func isStaged(plan Plan, rel string) bool {
	for _, path := range append(append([]string(nil), plan.StagedPayloadPaths...), plan.ExcludedStagedPaths...) {
		if path == rel {
			return true
		}
	}
	return false
}

func stagedAttestationPaths(plan Plan, expected string) []string {
	var paths []string
	for _, path := range plan.ExcludedStagedPaths {
		if path == expected {
			paths = append(paths, path)
			continue
		}
		if strings.HasPrefix(path, "backpressure/attestations/") && strings.HasSuffix(path, ".json") {
			paths = append(paths, path)
		}
	}
	return paths
}

func artifactFeature(plan Plan) attestations.Feature {
	if len(plan.Features) == 0 {
		return attestations.Feature{ID: "unknown", Kind: "unknown", Name: "unknown", DiffCluster: "unknown"}
	}
	feature := plan.Features[0]
	return attestations.Feature{
		ID:          feature.ID,
		Kind:        feature.Kind,
		Name:        feature.Name,
		SourceBead:  feature.SourceBead,
		DiffCluster: feature.DiffCluster,
	}
}

func gitHead(root string) string {
	abs, err := filepath.Abs(defaultRoot(root))
	if err != nil {
		return "unknown"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "HEAD")
	cmd.Dir = abs
	output, err := cmd.Output()
	if err != nil {
		return "unborn"
	}
	return strings.TrimSpace(string(output))
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func firstErr(primary, fallback error) error {
	if primary != nil {
		return primary
	}
	return fallback
}
