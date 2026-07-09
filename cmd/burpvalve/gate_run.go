package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"burpvalve/internal/attestations"
	"burpvalve/internal/backpressure"

	"github.com/spf13/cobra"
)

type gateRunOptions struct {
	handoffPath string
	handoff     *gateRunHandoff
	resume      bool
	dryRun      bool
	yes         bool
	journalPush bool
	remote      string
	branch      string
	message     string
	agent       string
	model       string
	jsonOutput  bool
}

type robotGateRunInput struct {
	HandoffPath string          `json:"handoff_path"`
	Handoff     *gateRunHandoff `json:"handoff"`
	Confirm     bool            `json:"confirm"`
	Resume      bool            `json:"resume"`
	DryRun      bool            `json:"dry_run"`
	JournalPush bool            `json:"journal_push"`
	Remote      string          `json:"remote"`
	Branch      string          `json:"branch"`
	Message     string          `json:"message"`
	Agent       string          `json:"agent"`
	Model       string          `json:"model"`
}

type gateRunHandoff struct {
	SchemaVersion int                    `json:"schema_version"`
	RunID         string                 `json:"run_id"`
	WorkUnit      gateRunWorkUnit        `json:"work_unit"`
	Authorization gateRunAuthorization   `json:"authorization"`
	Git           gateRunGit             `json:"git"`
	Verification  gateRunVerification    `json:"verification"`
	Beads         gateRunBeads           `json:"beads"`
	Release       gateRunRelease         `json:"release"`
	Extra         map[string]interface{} `json:"-"`
}

type gateRunWorkUnit struct {
	Kind      string   `json:"kind"`
	Feature   string   `json:"feature"`
	LaneID    string   `json:"lane_id"`
	BeadIDs   []string `json:"bead_ids"`
	Rationale string   `json:"rationale"`
}

type gateRunAuthorization struct {
	Kind      string `json:"kind"`
	Authority string `json:"authority"`
	AuditRef  string `json:"audit_ref"`
}

type gateRunGit struct {
	ExpectedHead         string   `json:"expected_head"`
	StagePaths           []string `json:"stage_paths"`
	ExpectedStagedHash   string   `json:"expected_staged_hash"`
	AllowUntracked       bool     `json:"allow_untracked"`
	ProtectedPaths       []string `json:"protected_paths"`
	CommitMessage        string   `json:"commit_message"`
	PublishAfterCommit   bool     `json:"publish_after_commit"`
	Remote               string   `json:"remote"`
	Branch               string   `json:"branch"`
	AllowMessageOverride bool     `json:"allow_message_override"`
}

type gateRunVerification struct {
	Feature          string   `json:"feature"`
	ResponsesPath    string   `json:"responses_path"`
	BeginIfMissing   bool     `json:"begin_if_missing"`
	PromptProfile    string   `json:"prompt_profile"`
	RequiredVerdicts []string `json:"required_verdicts"`
}

type gateRunBeads struct {
	Close     bool   `json:"close"`
	Reason    string `json:"reason"`
	AdminOnly bool   `json:"admin_only"`
	Sync      bool   `json:"sync"`
}

type gateRunRelease struct {
	AgentMailIdentity   string `json:"agent_mail_identity"`
	ReleaseReservations bool   `json:"release_reservations"`
	AgentMailMCP        string `json:"agent_mail_mcp"`
	WakeRef             string `json:"wake_ref"`
}

type gateRunResult struct {
	SchemaVersion        int                              `json:"schema_version"`
	Command              string                           `json:"command"`
	Status               string                           `json:"status"`
	Phase                string                           `json:"phase"`
	PartialSuccess       bool                             `json:"partial_success"`
	Mutating             bool                             `json:"mutating"`
	RunID                string                           `json:"run_id"`
	WorkUnit             gateRunWorkUnit                  `json:"work_unit"`
	HandoffPath          string                           `json:"handoff_path"`
	CanonicalHandoffPath string                           `json:"canonical_handoff_path"`
	JournalPath          string                           `json:"journal_path"`
	HandoffHash          string                           `json:"handoff_hash,omitempty"`
	JournalHash          string                           `json:"journal_hash,omitempty"`
	CurrentHead          string                           `json:"current_head,omitempty"`
	Resumed              bool                             `json:"resumed"`
	PreviousJournalHash  string                           `json:"previous_journal_hash,omitempty"`
	ExpectedHead         string                           `json:"expected_head"`
	ExpectedStagedHash   string                           `json:"expected_staged_hash,omitempty"`
	StagedPayloadHash    string                           `json:"staged_payload_hash,omitempty"`
	StagePaths           []string                         `json:"stage_paths"`
	StagedPaths          []string                         `json:"staged_paths,omitempty"`
	DirtyIndexPaths      []string                         `json:"dirty_index_paths,omitempty"`
	PeerDirtPaths        []string                         `json:"peer_dirt_paths,omitempty"`
	AttestationPaths     []string                         `json:"attestation_paths,omitempty"`
	CommitMessage        string                           `json:"commit_message"`
	ResponsesPath        string                           `json:"responses_path"`
	VerifierPromptsPath  string                           `json:"verifier_prompts_path,omitempty"`
	PendingConditions    []string                         `json:"pending_conditions,omitempty"`
	BlockingConditions   []string                         `json:"blocking_conditions,omitempty"`
	ExecutableConditions *gateRunExecutableConditionPhase `json:"executable_conditions,omitempty"`
	PlannedPushCommand   []string                         `json:"planned_push_command,omitempty"`
	PushJournaled        bool                             `json:"push_journaled,omitempty"`
	ReservationRelease   gateRunReleaseOutput             `json:"reservation_release,omitempty"`
	WakeRef              string                           `json:"wake_ref,omitempty"`
	WakeInstructions     []string                         `json:"wake_instructions,omitempty"`
	Agent                string                           `json:"agent,omitempty"`
	Model                string                           `json:"model,omitempty"`
	Warnings             []string                         `json:"warnings"`
	NextSteps            []string                         `json:"next_steps"`
}

type gateRunReleaseOutput struct {
	AgentMailIdentity   string `json:"agent_mail_identity,omitempty"`
	ReleaseReservations bool   `json:"release_reservations"`
	AgentMailMCP        string `json:"agent_mail_mcp,omitempty"`
	Status              string `json:"status,omitempty"`
	ProjectKey          string `json:"project_key,omitempty"`
	MCPTool             string `json:"mcp_tool,omitempty"`
	Instruction         string `json:"instruction,omitempty"`
}

type gateRunJournal struct {
	SchemaVersion        int                              `json:"schema_version"`
	Command              string                           `json:"command"`
	RunID                string                           `json:"run_id"`
	Status               string                           `json:"status"`
	Phase                string                           `json:"phase"`
	UpdatedAt            string                           `json:"updated_at"`
	SourceHandoffPath    string                           `json:"source_handoff_path,omitempty"`
	CanonicalHandoffPath string                           `json:"canonical_handoff_path"`
	JournalPath          string                           `json:"journal_path"`
	HandoffHash          string                           `json:"handoff_hash"`
	PreviousJournalHash  string                           `json:"previous_journal_hash,omitempty"`
	CurrentHead          string                           `json:"current_head,omitempty"`
	ExpectedHead         string                           `json:"expected_head,omitempty"`
	ExpectedStagedHash   string                           `json:"expected_staged_hash,omitempty"`
	StagedPayloadHash    string                           `json:"staged_payload_hash,omitempty"`
	StagePaths           []string                         `json:"stage_paths"`
	StagedPaths          []string                         `json:"staged_paths,omitempty"`
	PeerDirtPaths        []string                         `json:"peer_dirt_paths,omitempty"`
	AttestationPaths     []string                         `json:"attestation_paths,omitempty"`
	VerifierPromptsPath  string                           `json:"verifier_prompts_path,omitempty"`
	PendingConditions    []string                         `json:"pending_conditions,omitempty"`
	BlockingConditions   []string                         `json:"blocking_conditions,omitempty"`
	ExecutableConditions *gateRunExecutableConditionPhase `json:"executable_conditions,omitempty"`
	PlannedPushCommand   []string                         `json:"planned_push_command,omitempty"`
	PushJournaled        bool                             `json:"push_journaled,omitempty"`
	ReservationRelease   gateRunReleaseOutput             `json:"reservation_release,omitempty"`
	WakeRef              string                           `json:"wake_ref,omitempty"`
	WakeInstructions     []string                         `json:"wake_instructions,omitempty"`
	Steps                []gateRunJournalStep             `json:"steps"`
}

type gateRunJournalStep struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}

type gateRunExecutableConditionPhase struct {
	Enabled      bool   `json:"enabled"`
	Status       string `json:"status"`
	Mode         string `json:"mode"`
	CommandCount int    `json:"command_count"`
	Parallelism  int    `json:"parallelism"`
	Message      string `json:"message"`
}

type gateRunPhaseRunner interface {
	RunExecutableConditions(context.Context, gateRunHandoff, gateRunResult) (gateRunExecutableConditionPhase, error)
}

type serialNoopGatePhaseRunner struct{}

func (serialNoopGatePhaseRunner) RunExecutableConditions(context.Context, gateRunHandoff, gateRunResult) (gateRunExecutableConditionPhase, error) {
	return gateRunExecutableConditionPhase{
		Enabled:      false,
		Status:       "skipped",
		Mode:         "serial",
		CommandCount: 0,
		Parallelism:  1,
		Message:      "no executable gate-run conditions configured; v1 remains serial and deterministic",
	}, nil
}

func newGateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gate",
		Short: "Run fail-closed commit-gate workflows from prepared handoffs.",
		Long:  "Run fail-closed gate workflows from authored, hash-bound handoffs.",
	}
	cmd.AddCommand(newGateRunCommand())
	return cmd
}

func newGateRunCommand() *cobra.Command {
	opts := gateRunOptions{journalPush: true}
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a prepared gate-run handoff.",
		Long: `Run a prepared gate-run handoff for the fail-closed commit ceremony.

Use --dry-run to inspect the canonical handoff and journal paths without
mutation. Mutating runs require --yes or robots confirm:true, stage only the
handoff-declared payload plus the generated attestation, and journal push,
reservation release, and wake handoff output after the local commit.`,
		Example: `  burpvalve gate run --handoff log/backpressure/gate-runs/qhqa-handoff.json --dry-run --json
  printf '{"handoff_path":"log/backpressure/gate-runs/qhqa-handoff.json","dry_run":true}' | burpvalve --robots gate run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fail(2, "gate run does not accept positional arguments; use --handoff or --robots")
			}
			if robotsMode {
				return runGateRunRobots(cmd, opts)
			}
			return runGateRun(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.handoffPath, "handoff", "", "prepared handoff JSON path")
	cmd.Flags().BoolVar(&opts.resume, "resume", false, "resume from the gate-run journal after recomputing reality")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "validate and print the gate-run plan without mutation")
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "confirm mutation when later gate-run execution units are implemented")
	cmd.Flags().BoolVar(&opts.journalPush, "journal-push", true, "journal the exact push command instead of pushing")
	cmd.Flags().StringVar(&opts.remote, "remote", "", "publication remote recorded in the journaled push command")
	cmd.Flags().StringVar(&opts.branch, "branch", "", "publication branch recorded in the journaled push command")
	cmd.Flags().StringVar(&opts.message, "message", "", "commit message override when the handoff permits it")
	cmd.Flags().StringVar(&opts.agent, "agent", "", "agent identity recorded in future generated artifacts")
	cmd.Flags().StringVar(&opts.model, "model", "", "model identity recorded in future generated artifacts")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "emit machine-readable result JSON")
	return cmd
}

func runGateRunRobots(cmd *cobra.Command, opts gateRunOptions) error {
	var input robotGateRunInput
	if err := decodeRobotInput(cmd.InOrStdin(), &input); err != nil {
		return err
	}
	if input.HandoffPath != "" {
		opts.handoffPath = input.HandoffPath
	}
	if input.Handoff != nil {
		opts.handoff = input.Handoff
	}
	opts.resume = opts.resume || input.Resume
	opts.dryRun = opts.dryRun || input.DryRun
	opts.yes = opts.yes || input.Confirm
	opts.journalPush = opts.journalPush || input.JournalPush
	if input.Remote != "" {
		opts.remote = input.Remote
	}
	if input.Branch != "" {
		opts.branch = input.Branch
	}
	if input.Message != "" {
		opts.message = input.Message
	}
	if input.Agent != "" {
		opts.agent = input.Agent
	}
	if input.Model != "" {
		opts.model = input.Model
	}
	opts.jsonOutput = true
	return runGateRun(cmd, opts)
}

func runGateRun(cmd *cobra.Command, opts gateRunOptions) error {
	result, err := planGateRun(cmd, opts)
	if opts.jsonOutput || robotsMode {
		if encodeErr := encodeJSON(cmd.OutOrStdout(), result, "encode gate run result"); encodeErr != nil {
			return encodeErr
		}
	}
	if err != nil {
		return err
	}
	if !opts.jsonOutput && !robotsMode {
		fmt.Fprintf(cmd.OutOrStdout(), "Burpvalve gate run %s\n", result.Status)
		fmt.Fprintf(cmd.OutOrStdout(), "Run: %s\n", result.RunID)
		fmt.Fprintf(cmd.OutOrStdout(), "Canonical handoff: %s\n", result.CanonicalHandoffPath)
		fmt.Fprintf(cmd.OutOrStdout(), "Journal: %s\n", result.JournalPath)
		for _, step := range result.NextSteps {
			fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", step)
		}
	}
	return nil
}

func planGateRun(cmd *cobra.Command, opts gateRunOptions) (gateRunResult, error) {
	handoff, handoffPath, err := loadGateRunHandoff(opts)
	if err != nil {
		result := baseGateRunResult(opts, gateRunHandoff{}, "")
		result.Status = "blocked"
		result.Phase = "handoff"
		result.NextSteps = []string{"Provide --handoff <file> or --robots JSON with handoff_path or inline handoff."}
		return result, err
	}
	result := baseGateRunResult(opts, handoff, handoffPath)
	if validationErr := validateGateRunHandoff(handoff, opts); validationErr != nil {
		result.Status = "blocked"
		result.Phase = "handoff_validation"
		result.NextSteps = []string{validationErr.Error()}
		return result, validationErr
	}
	if opts.resume {
		if err := reconcileGateRunJournal(&result); err != nil {
			result.Status = "blocked"
			result.Phase = "resume"
			result.NextSteps = []string{err.Error()}
			return result, err
		}
	}
	result.Status = "planned"
	result.Phase = "validated"
	result.NextSteps = []string{
		"Run again with --yes or robots confirm=true to copy the canonical handoff, write the journal, stage exact paths, collect verifier cells, stage the attestation, and commit locally.",
		"Publication target fields are journaled only; gate run v1 records the push command but does not push.",
	}
	if !opts.dryRun {
		if !opts.yes {
			result.Status = "blocked"
			result.Phase = "confirmation_required"
			result.NextSteps = []string{"Run with --yes or robots confirm=true to execute the mutating gate-run ceremony."}
			return result, fail(2, "gate run requires --yes or robots confirm=true before mutation")
		}
		persisted, persistErr := persistGateRunHandoffAndJournal(handoff, handoffPath, result)
		result = persisted
		if persistErr != nil {
			result.Status = "blocked"
			result.Phase = "journal"
			result.NextSteps = []string{persistErr.Error()}
			return result, persistErr
		}
		result.PartialSuccess = true
		preflighted, preflightErr := runGateRunGitPreflight(cmd, handoff, result)
		result = preflighted
		if preflightErr != nil {
			return result, preflightErr
		}
		conditioned, conditionErr := runGateRunExecutableConditionPhase(handoff, result, serialNoopGatePhaseRunner{})
		result = conditioned
		if conditionErr != nil {
			return result, conditionErr
		}
		verified, verifierErr := runGateRunVerifierPhase(cmd, handoff, result)
		result = verified
		if verifierErr != nil {
			return result, verifierErr
		}
		committed, commitErr := runGateRunCommitPhase(handoff, result)
		result = committed
		if commitErr != nil {
			return result, commitErr
		}
	}
	return result, nil
}

func loadGateRunHandoff(opts gateRunOptions) (gateRunHandoff, string, error) {
	if opts.handoff != nil {
		return *opts.handoff, "", nil
	}
	if strings.TrimSpace(opts.handoffPath) == "" {
		return gateRunHandoff{}, "", fail(2, "gate run requires --handoff or robots handoff input")
	}
	body, err := os.ReadFile(opts.handoffPath)
	if err != nil {
		return gateRunHandoff{}, opts.handoffPath, fail(1, "read gate run handoff %s: %v", opts.handoffPath, err)
	}
	var handoff gateRunHandoff
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&handoff); err != nil {
		return gateRunHandoff{}, opts.handoffPath, fail(2, "decode gate run handoff %s: %v", opts.handoffPath, err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return gateRunHandoff{}, opts.handoffPath, fail(2, "decode gate run handoff %s: expected a single JSON object", opts.handoffPath)
		}
		return gateRunHandoff{}, opts.handoffPath, fail(2, "decode gate run handoff %s: %v", opts.handoffPath, err)
	}
	return handoff, opts.handoffPath, nil
}

func persistGateRunHandoffAndJournal(handoff gateRunHandoff, sourcePath string, result gateRunResult) (gateRunResult, error) {
	canonicalBody, err := canonicalGateRunHandoffJSON(handoff)
	if err != nil {
		return result, fail(2, "canonicalize gate run handoff: %v", err)
	}
	if err := writeGateRunFile(result.CanonicalHandoffPath, canonicalBody); err != nil {
		return result, err
	}
	previousHash := ""
	if result.Resumed {
		previousHash = result.PreviousJournalHash
	}
	currentHead := currentGitHead()
	now := time.Now().UTC().Format(time.RFC3339)
	journal := gateRunJournal{
		SchemaVersion:        1,
		Command:              "gate run",
		RunID:                result.RunID,
		Status:               "running",
		Phase:                "journal",
		UpdatedAt:            now,
		SourceHandoffPath:    sourcePath,
		CanonicalHandoffPath: result.CanonicalHandoffPath,
		JournalPath:          result.JournalPath,
		HandoffHash:          gateRunHash(canonicalBody),
		PreviousJournalHash:  previousHash,
		CurrentHead:          currentHead,
		ExpectedHead:         result.ExpectedHead,
		ExpectedStagedHash:   result.ExpectedStagedHash,
		StagePaths:           append([]string(nil), result.StagePaths...),
		Steps: []gateRunJournalStep{
			{ID: "canonical_handoff", Status: "completed", Message: "wrote canonical handoff copy", CreatedAt: now},
			{ID: "journal", Status: "completed", Message: "wrote initial gate-run journal", CreatedAt: now},
		},
	}
	journalBody, err := encodeGateRunJournal(journal)
	if err != nil {
		return result, err
	}
	if err := writeGateRunFile(result.JournalPath, journalBody); err != nil {
		return result, err
	}
	result.HandoffHash = journal.HandoffHash
	result.JournalHash = gateRunHash(journalBody)
	result.CurrentHead = currentHead
	result.PreviousJournalHash = previousHash
	result.Warnings = append(result.Warnings, "canonical handoff and journal were written before mutating gate-run execution")
	return result, nil
}

func runGateRunGitPreflight(cmd *cobra.Command, handoff gateRunHandoff, result gateRunResult) (gateRunResult, error) {
	result.CurrentHead = currentGitHead()
	if result.CurrentHead == "" {
		result.Status = "blocked"
		result.Phase = "head_mismatch"
		result.NextSteps = []string{"Run gate run inside a Git repository with a readable HEAD."}
		_ = appendGateRunJournalStep(&result, "git_head", "blocked", "could not read current git HEAD")
		return result, fail(1, "read current git HEAD")
	}
	if result.ExpectedHead != "" && result.CurrentHead != result.ExpectedHead {
		result.Status = "blocked"
		result.Phase = "head_mismatch"
		result.NextSteps = []string{fmt.Sprintf("Current HEAD %s does not match handoff expected_head %s; regenerate the handoff or rebase before retrying.", result.CurrentHead, result.ExpectedHead)}
		_ = appendGateRunJournalStep(&result, "git_head", "blocked", "current HEAD did not match handoff expected_head")
		return result, fail(2, "gate run head_mismatch: got %s want %s", result.CurrentHead, result.ExpectedHead)
	}
	if attestations := stagedAttestationPathNames("."); len(attestations) > 0 {
		result.Status = "blocked"
		result.Phase = "attestation_bounce"
		result.AttestationPaths = attestations
		result.NextSteps = []string{"A backpressure attestation is already staged. Commit or unstage it before gate run restages the delivery payload."}
		_ = appendGateRunJournalStep(&result, "attestation_bounce", "blocked", "staged attestation would be mixed into the payload")
		return result, fail(2, "gate run attestation_bounce: %s", strings.Join(attestations, ", "))
	}
	initialStaged := stagedPathNames(".")
	allowed := pathSet(result.StagePaths)
	if unrelated := pathsOutsideSet(initialStaged, allowed); len(unrelated) > 0 {
		result.Status = "blocked"
		result.Phase = "dirty_index"
		result.DirtyIndexPaths = unrelated
		result.StagedPaths = initialStaged
		result.NextSteps = []string{"The index already contains paths outside git.stage_paths: " + strings.Join(unrelated, ", ") + ". Unstage or commit them before running gate run."}
		_ = appendGateRunJournalStep(&result, "dirty_index", "blocked", "unrelated staged paths were present before exact staging")
		return result, fail(2, "gate run dirty_index: %s", strings.Join(unrelated, ", "))
	}
	if peerDirt := gateRunPeerDirtPaths(handoff); len(peerDirt) > 0 {
		result.Status = "blocked"
		result.Phase = "peer_dirt"
		result.PeerDirtPaths = peerDirt
		result.NextSteps = []string{"Unstaged or untracked changes overlap protected paths: " + strings.Join(peerDirt, ", ") + ". Coordinate or clean those paths before gate run mutates the index."}
		_ = appendGateRunJournalStep(&result, "peer_dirt", "blocked", "protected paths had unstaged or untracked dirt")
		return result, fail(2, "gate run peer_dirt: %s", strings.Join(peerDirt, ", "))
	}
	if err := gitAddExact(result.StagePaths); err != nil {
		result.Status = "blocked"
		result.Phase = "stage_mismatch"
		result.NextSteps = []string{err.Error()}
		_ = appendGateRunJournalStep(&result, "exact_staging", "blocked", "git add failed for exact stage_paths")
		return result, err
	}
	result.StagedPaths = stagedPathNames(".")
	if !samePathSet(result.StagedPaths, result.StagePaths) {
		result.Status = "blocked"
		result.Phase = "stage_mismatch"
		result.NextSteps = []string{fmt.Sprintf("Staged paths %s do not exactly match handoff stage_paths %s; restore the index before retrying.", strings.Join(result.StagedPaths, ", "), strings.Join(result.StagePaths, ", "))}
		_ = appendGateRunJournalStep(&result, "exact_staging", "blocked", "staged paths did not exactly match handoff stage_paths")
		return result, fail(2, "gate run stage_mismatch")
	}
	if err := runGateRunBeadsClose(cmd, handoff, &result); err != nil {
		return result, err
	}
	result.StagedPaths = stagedPathNames(".")
	payloadHash, err := currentGateRunPayloadHash()
	if err != nil {
		result.Status = "blocked"
		result.Phase = "hash_mismatch"
		result.NextSteps = []string{err.Error()}
		_ = appendGateRunJournalStep(&result, "payload_hash", "blocked", "could not compute staged payload hash")
		return result, err
	}
	result.StagedPayloadHash = payloadHash
	if result.ExpectedStagedHash != "" && result.StagedPayloadHash != result.ExpectedStagedHash {
		result.Status = "blocked"
		result.Phase = "hash_mismatch"
		result.NextSteps = []string{fmt.Sprintf("Computed staged payload hash %s does not match handoff expected_staged_hash %s; regenerate verifier evidence for the exact staged payload.", result.StagedPayloadHash, result.ExpectedStagedHash)}
		_ = appendGateRunJournalStep(&result, "payload_hash", "blocked", "staged payload hash did not match handoff expected_staged_hash")
		return result, fail(2, "gate run hash_mismatch: got %s want %s", result.StagedPayloadHash, result.ExpectedStagedHash)
	}
	if boundHash, ok := gateRunResponseBindingHash(result.ResponsesPath); ok && boundHash != "" && boundHash != result.StagedPayloadHash {
		result.Status = "blocked"
		result.Phase = "hash_mismatch"
		result.NextSteps = []string{fmt.Sprintf("Response file %s is bound to %s, but the staged payload hash is %s; rerun verifier begin for the current payload.", result.ResponsesPath, boundHash, result.StagedPayloadHash)}
		_ = appendGateRunJournalStep(&result, "response_binding", "blocked", "responses binding hash did not match staged payload")
		return result, fail(2, "gate run response binding mismatch: got %s want %s", boundHash, result.StagedPayloadHash)
	}
	if err := appendGateRunJournalStep(&result, "exact_staging", "completed", "staged exactly handoff git.stage_paths and verified payload hash"); err != nil {
		result.Status = "blocked"
		result.Phase = "journal"
		result.NextSteps = []string{err.Error()}
		return result, err
	}
	result.Warnings = append(result.Warnings, "gate run staged only handoff git.stage_paths before verifier dispatch and commit-gate execution")
	return result, nil
}

func runGateRunExecutableConditionPhase(handoff gateRunHandoff, result gateRunResult, runner gateRunPhaseRunner) (gateRunResult, error) {
	if runner == nil {
		runner = serialNoopGatePhaseRunner{}
	}
	ctx, cancel := backpressure.WithTimeout(context.Background())
	defer cancel()
	phase, err := runner.RunExecutableConditions(ctx, handoff, result)
	result.ExecutableConditions = &phase
	if err != nil {
		result.Status = "blocked"
		result.Phase = "test_failure"
		result.NextSteps = []string{"Executable gate-run condition phase failed before verifier dispatch: " + err.Error()}
		_ = appendGateRunJournalStep(&result, "executable_conditions", "blocked", firstNonEmpty(phase.Message, err.Error()))
		return result, err
	}
	stepStatus := "completed"
	if phase.Status == "skipped" || !phase.Enabled {
		stepStatus = "skipped"
	}
	if err := appendGateRunJournalStep(&result, "executable_conditions", stepStatus, phase.Message); err != nil {
		result.Status = "blocked"
		result.Phase = "journal"
		result.NextSteps = []string{err.Error()}
		return result, err
	}
	return result, nil
}

func runGateRunCommitPhase(handoff gateRunHandoff, result gateRunResult) (gateRunResult, error) {
	gateResult, err := runGateRunCommitGate(handoff, result)
	result.Warnings = append(result.Warnings, gateResult.Warnings...)
	if err != nil && gateResult.Status != backpressure.StatusAttestationWritten {
		result.Status = "blocked"
		result.Phase = "commit_gate"
		result.NextSteps = gateRunCommitGateNextSteps(gateResult, "Commit gate blocked before attestation staging; fix the blocker and rerun gate run --resume.")
		_ = appendGateRunJournalStep(&result, "commit_gate", "blocked", firstNonEmpty(gateResult.Message, err.Error()))
		return result, err
	}
	if gateResult.Status == backpressure.StatusAttestationWritten {
		result.Phase = "commit_gate"
		result.AttestationPaths = appendUniqueGateRunPath(result.AttestationPaths, gateResult.ArtifactPath)
		if err := appendGateRunJournalStep(&result, "commit_gate", "waiting", "commit gate wrote passing attestation "+gateResult.ArtifactPath); err != nil {
			result.Status = "blocked"
			result.Phase = "journal"
			result.NextSteps = []string{err.Error()}
			return result, err
		}
		if stageErr := stageGateRunAttestation(gateResult.ArtifactPath); stageErr != nil {
			result.Status = "blocked"
			result.Phase = "stage_attestation"
			result.NextSteps = []string{stageErr.Error()}
			_ = appendGateRunJournalStep(&result, "stage_attestation", "blocked", stageErr.Error())
			return result, stageErr
		}
		result.StagedPaths = stagedPathNames(".")
		result.AttestationPaths = appendUniqueGateRunPath(result.AttestationPaths, gateResult.ArtifactPath)
		if err := appendGateRunJournalStep(&result, "stage_attestation", "completed", "staged exactly generated attestation "+gateResult.ArtifactPath); err != nil {
			result.Status = "blocked"
			result.Phase = "journal"
			result.NextSteps = []string{err.Error()}
			return result, err
		}
		gateResult, err = runGateRunCommitGate(handoff, result)
		result.Warnings = append(result.Warnings, gateResult.Warnings...)
		if err != nil {
			result.Status = "blocked"
			result.Phase = "commit_gate_revalidate"
			result.NextSteps = gateRunCommitGateNextSteps(gateResult, "Gate revalidation failed after staging the attestation; regenerate verifier evidence for the current payload and rerun gate run --resume.")
			_ = appendGateRunJournalStep(&result, "commit_gate_revalidate", "blocked", firstNonEmpty(gateResult.Message, err.Error()))
			return result, err
		}
		if gateResult.Status != backpressure.StatusPassed {
			err := fail(2, "gate run commit gate revalidation returned %s", gateResult.Status)
			result.Status = "blocked"
			result.Phase = "commit_gate_revalidate"
			result.NextSteps = gateRunCommitGateNextSteps(gateResult, "Gate revalidation did not pass; rerun gate run --resume after fixing the blocker.")
			_ = appendGateRunJournalStep(&result, "commit_gate_revalidate", "blocked", firstNonEmpty(gateResult.Message, gateResult.Status))
			return result, err
		}
		if err := appendGateRunJournalStep(&result, "commit_gate_revalidate", "completed", "commit gate revalidated the staged payload plus generated attestation"); err != nil {
			result.Status = "blocked"
			result.Phase = "journal"
			result.NextSteps = []string{err.Error()}
			return result, err
		}
	} else if gateResult.Status == backpressure.StatusPassed {
		if err := appendGateRunJournalStep(&result, "commit_gate", "completed", "commit gate passed with an already staged valid attestation"); err != nil {
			result.Status = "blocked"
			result.Phase = "journal"
			result.NextSteps = []string{err.Error()}
			return result, err
		}
	} else {
		err := fail(2, "gate run commit gate returned %s", gateResult.Status)
		result.Status = "blocked"
		result.Phase = "commit_gate"
		result.NextSteps = gateRunCommitGateNextSteps(gateResult, "Commit gate did not pass; fix the blocker and rerun gate run --resume.")
		_ = appendGateRunJournalStep(&result, "commit_gate", "blocked", firstNonEmpty(gateResult.Message, gateResult.Status))
		return result, err
	}

	if out, err := gitCommitGateRun(result.CommitMessage); err != nil {
		result.Status = "blocked"
		result.Phase = "local_commit"
		result.NextSteps = []string{"git commit failed after gate pass; fix the Git or hook blocker and rerun gate run --resume."}
		if out != "" {
			result.Warnings = append(result.Warnings, out)
		}
		_ = appendGateRunJournalStep(&result, "git_commit", "blocked", firstNonEmpty(out, err.Error()))
		return result, err
	}
	result.Status = "committed"
	result.Phase = "local_commit"
	result.PartialSuccess = true
	result.CurrentHead = currentGitHead()
	result.StagedPaths = stagedPathNames(".")
	if err := appendGateRunJournalStep(&result, "git_commit", "completed", "created local git commit "+result.CurrentHead); err != nil {
		result.Status = "blocked"
		result.Phase = "journal"
		result.NextSteps = []string{err.Error()}
		return result, err
	}
	return runGateRunPostCommit(handoff, result)
}

func runGateRunPostCommit(handoff gateRunHandoff, result gateRunResult) (gateRunResult, error) {
	result.Status = "committed"
	result.Phase = "post_commit"
	result.PartialSuccess = true
	result.NextSteps = []string{"Local git commit completed. Gate run v1 does not execute pushes."}
	if len(result.PlannedPushCommand) > 0 {
		result.PushJournaled = true
		result.NextSteps = append(result.NextSteps, "Orchestrator push command: "+strings.Join(result.PlannedPushCommand, " "))
		if err := appendGateRunJournalStep(&result, "push_journal", "completed", "journaled orchestrator push command "+strings.Join(result.PlannedPushCommand, " ")); err != nil {
			result.Status = "blocked"
			result.Phase = "journal"
			result.NextSteps = []string{err.Error()}
			return result, err
		}
	} else if err := appendGateRunJournalStep(&result, "push_journal", "skipped", "handoff did not request publication journaling"); err != nil {
		result.Status = "blocked"
		result.Phase = "journal"
		result.NextSteps = []string{err.Error()}
		return result, err
	}

	result.ReservationRelease = gateRunReleaseOutputForHandoff(handoff)
	if result.ReservationRelease.Instruction != "" {
		result.NextSteps = append(result.NextSteps, result.ReservationRelease.Instruction)
	}
	releaseStepStatus := "skipped"
	if result.ReservationRelease.Status != "not_requested" {
		releaseStepStatus = "completed"
	}
	if err := appendGateRunJournalStep(&result, "release_reservations", releaseStepStatus, firstNonEmpty(result.ReservationRelease.Instruction, "handoff did not request reservation release")); err != nil {
		result.Status = "blocked"
		result.Phase = "journal"
		result.NextSteps = []string{err.Error()}
		return result, err
	}

	result.WakeRef = strings.TrimSpace(handoff.Release.WakeRef)
	if result.WakeRef != "" {
		result.WakeInstructions = []string{"Wake or hand off the next owner using release.wake_ref: " + result.WakeRef}
		result.NextSteps = append(result.NextSteps, result.WakeInstructions...)
		if err := appendGateRunJournalStep(&result, "wake_handoff", "completed", result.WakeInstructions[0]); err != nil {
			result.Status = "blocked"
			result.Phase = "journal"
			result.NextSteps = []string{err.Error()}
			return result, err
		}
	} else if err := appendGateRunJournalStep(&result, "wake_handoff", "skipped", "handoff did not provide release.wake_ref"); err != nil {
		result.Status = "blocked"
		result.Phase = "journal"
		result.NextSteps = []string{err.Error()}
		return result, err
	}
	return result, nil
}

func gateRunReleaseOutputForHandoff(handoff gateRunHandoff) gateRunReleaseOutput {
	release := handoff.Release
	out := gateRunReleaseOutput{
		AgentMailIdentity:   strings.TrimSpace(release.AgentMailIdentity),
		ReleaseReservations: release.ReleaseReservations,
		AgentMailMCP:        strings.TrimSpace(release.AgentMailMCP),
	}
	if !release.ReleaseReservations {
		out.Status = "not_requested"
		return out
	}
	if out.AgentMailIdentity == "" {
		out.Status = "missing_identity"
		out.Instruction = "Reservation release was requested, but release.agent_mail_identity is empty; orchestrator must identify the reservation owner before release."
		return out
	}
	projectKey, err := filepath.Abs(defaultCLIRoot("."))
	if err != nil {
		projectKey = defaultCLIRoot(".")
	}
	out.ProjectKey = projectKey
	out.MCPTool = "release_file_reservations"
	switch strings.ToLower(out.AgentMailMCP) {
	case "available":
		out.Status = "mcp_instruction"
		out.Instruction = "Use Agent Mail MCP release_file_reservations with project_key " + projectKey + " and agent_name " + out.AgentMailIdentity + "."
	case "unavailable":
		out.Status = "manual_instruction"
		out.Instruction = "Agent Mail MCP is unavailable; release reservations for " + out.AgentMailIdentity + " manually when the commit is accepted."
	default:
		out.Status = "mcp_status_unknown"
		out.Instruction = "Agent Mail MCP availability is unknown; verify MCP availability, then release reservations for " + out.AgentMailIdentity + "."
	}
	return out
}

func runGateRunCommitGate(handoff gateRunHandoff, result gateRunResult) (backpressure.PreCommitResult, error) {
	ctx, cancel := backpressure.WithTimeout(context.Background())
	defer cancel()
	return backpressure.RunPreCommit(ctx, backpressure.PreCommitOptions{
		Root:            ".",
		ExplicitFeature: handoff.Verification.Feature,
		BeadIDs:         append([]string(nil), handoff.WorkUnit.BeadIDs...),
		BeadRationale:   handoff.WorkUnit.Rationale,
		Lane:            gateRunLaneOptions(handoff),
		ResponsesPath:   result.ResponsesPath,
		Agent:           result.Agent,
		Model:           result.Model,
		ColorMode:       colorMode,
	})
}

func gateRunCommitGateNextSteps(gateResult backpressure.PreCommitResult, fallback string) []string {
	if len(gateResult.NextSteps) > 0 {
		return append([]string(nil), gateResult.NextSteps...)
	}
	return []string{fallback}
}

func stageGateRunAttestation(path string) error {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if !validGateRunAttestationPath(clean) {
		return fail(2, "commit gate returned unsafe attestation path %q", path)
	}
	return gitAddExact([]string{clean})
}

func validGateRunAttestationPath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	return clean == strings.TrimSpace(path) &&
		strings.HasPrefix(clean, "backpressure/attestations/") &&
		strings.HasSuffix(clean, ".json") &&
		validGateRunStagePath(clean)
}

func gitCommitGateRun(message string) (string, error) {
	cmd := exec.Command("git", "commit", "-m", message)
	out, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		return trimmed, fail(2, "git commit gate run: %v: %s", err, trimmed)
	}
	return trimmed, nil
}

func appendUniqueGateRunPath(paths []string, path string) []string {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if clean == "." || clean == "" {
		return paths
	}
	for _, existing := range paths {
		if existing == clean {
			return paths
		}
	}
	return append(paths, clean)
}

func cloneGateRunExecutableConditions(phase *gateRunExecutableConditionPhase) *gateRunExecutableConditionPhase {
	if phase == nil {
		return nil
	}
	cloned := *phase
	return &cloned
}

func runGateRunVerifierPhase(cmd *cobra.Command, handoff gateRunHandoff, result gateRunResult) (gateRunResult, error) {
	ctx, cancel := backpressure.WithTimeout(context.Background())
	defer cancel()
	laneOpts := gateRunLaneOptions(handoff)
	responsesPath := strings.TrimSpace(result.ResponsesPath)
	if responsesPath == "" {
		responsesPath = backpressure.ResponsesPath(result.StagedPayloadHash)
		result.ResponsesPath = responsesPath
	}
	if _, err := os.Stat(responsesPath); err != nil {
		if !os.IsNotExist(err) || !handoff.Verification.BeginIfMissing {
			result.Status = "blocked"
			result.Phase = "verifier_responses"
			result.NextSteps = []string{"Run burpvalve verifier begin for the current staged payload or set verification.begin_if_missing=true in the handoff."}
			_ = appendGateRunJournalStep(&result, "verifier_responses", "blocked", "responses file missing")
			return result, fail(2, "gate run verifier responses missing: %s", responsesPath)
		}
		canonicalResponsesPath := backpressure.ResponsesPath(result.StagedPayloadHash)
		if canonicalResponsesPath != responsesPath {
			if _, canonicalErr := os.Stat(canonicalResponsesPath); canonicalErr == nil {
				responsesPath = canonicalResponsesPath
				result.ResponsesPath = canonicalResponsesPath
			}
		}
	}
	if _, err := os.Stat(responsesPath); err != nil {
		if !os.IsNotExist(err) || !handoff.Verification.BeginIfMissing {
			result.Status = "blocked"
			result.Phase = "verifier_responses"
			result.NextSteps = []string{"Run burpvalve verifier begin for the current staged payload or set verification.begin_if_missing=true in the handoff."}
			_ = appendGateRunJournalStep(&result, "verifier_responses", "blocked", "responses file missing")
			return result, fail(2, "gate run verifier responses missing: %s", responsesPath)
		}
		begin, beginErr := backpressure.RunVerifierBegin(ctx, backpressure.BeginResponsesOptions{
			Root:             ".",
			ExplicitFeature:  handoff.Verification.Feature,
			OneFeature:       handoff.WorkUnit.Kind == "single",
			AtomicityMessage: gateRunAtomicityMessage(handoff),
			Lane:             laneOpts,
		})
		result.ResponsesPath = begin.ResponsesPath
		result.Warnings = append(result.Warnings, begin.Warnings...)
		if beginErr != nil {
			result.Status = "blocked"
			result.Phase = "verifier_begin"
			result.NextSteps = begin.NextSteps
			_ = appendGateRunJournalStep(&result, "verifier_begin", "blocked", begin.Message)
			return result, beginErr
		}
		responsesPath = begin.ResponsesPath
		if err := appendGateRunJournalStep(&result, "verifier_begin", "completed", "wrote bound verifier responses file "+responsesPath); err != nil {
			result.Status = "blocked"
			result.Phase = "journal"
			result.NextSteps = []string{err.Error()}
			return result, err
		}
	}
	prompts, promptErr := backpressure.BuildVerifierPrompts(ctx, backpressure.VerifierPromptOptions{
		Root:    ".",
		Feature: handoff.Verification.Feature,
		Profile: handoff.Verification.PromptProfile,
		Lane:    laneOpts,
	})
	if promptErr != nil {
		result.Status = "blocked"
		result.Phase = "verifier_prompts"
		result.NextSteps = []string{"Fix verifier prompt generation for the current staged payload, then rerun gate run --resume: " + promptErr.Error()}
		_ = appendGateRunJournalStep(&result, "verifier_prompts", "blocked", promptErr.Error())
		return result, promptErr
	}
	promptsPath := filepath.Join("log", "backpressure", "gate-runs", result.RunID+"-verifier-prompts.json")
	promptBody, err := json.MarshalIndent(prompts, "", "  ")
	if err != nil {
		result.Status = "blocked"
		result.Phase = "verifier_prompts"
		result.NextSteps = []string{"Fix verifier prompt serialization, then rerun gate run --resume."}
		return result, fail(2, "encode gate run verifier prompts: %v", err)
	}
	if err := writeGateRunFile(promptsPath, append(promptBody, '\n')); err != nil {
		result.Status = "blocked"
		result.Phase = "verifier_prompts"
		result.NextSteps = []string{err.Error()}
		return result, err
	}
	result.VerifierPromptsPath = promptsPath
	if err := appendGateRunJournalStep(&result, "verifier_prompts", "completed", "wrote verifier prompt packets to "+promptsPath); err != nil {
		result.Status = "blocked"
		result.Phase = "journal"
		result.NextSteps = []string{err.Error()}
		return result, err
	}
	if err := validateGateRunResponsesFile(responsesPath, prompts, handoff, &result); err != nil {
		_ = appendGateRunJournalStep(&result, "verifier_responses", "blocked", err.Error())
		return result, err
	}
	if err := appendGateRunJournalStep(&result, "verifier_responses", "completed", "all required verifier responses are pass or not_applicable for the staged payload"); err != nil {
		result.Status = "blocked"
		result.Phase = "journal"
		result.NextSteps = []string{err.Error()}
		return result, err
	}
	return result, nil
}

func runGateRunBeadsClose(cmd *cobra.Command, handoff gateRunHandoff, result *gateRunResult) error {
	if !handoff.Beads.Close {
		return nil
	}
	beadIDs, rationale, parseWarnings, normErr := normalizeCloseBeads(handoff.WorkUnit.BeadIDs, nil, handoff.WorkUnit.Rationale, handoff.Beads.AdminOnly)
	result.Warnings = append(result.Warnings, parseWarnings...)
	if normErr != nil {
		result.Status = "blocked"
		result.Phase = "close_or_sync_failed"
		result.NextSteps = []string{normErr.Error()}
		_ = appendGateRunJournalStep(result, "beads_normalize", "blocked", normErr.Error())
		return normErr
	}
	root, err := filepath.Abs(defaultCLIRoot("."))
	if err != nil {
		root = defaultCLIRoot(".")
	}
	preflight, preflightErr := buildBeadsPreflightReport(beadsPreflightOptions{
		root:                   root,
		adminOnly:              handoff.Beads.AdminOnly,
		beadRationale:          rationale,
		requireDeliveryPayload: !handoff.Beads.AdminOnly,
	}, beadIDs)
	result.Warnings = append(result.Warnings, preflight.Warnings...)
	if preflightErr != nil {
		result.Status = "blocked"
		result.Phase = "close_or_sync_failed"
		result.NextSteps = []string{"Resolve Beads preflight blockers before gate run closes the handoff bead: " + preflightErr.Error()}
		_ = appendGateRunJournalStep(result, "beads_preflight", "blocked", preflightErr.Error())
		return preflightErr
	}
	brPath, err := exec.LookPath("br")
	if err != nil {
		result.Status = "blocked"
		result.Phase = "close_or_sync_failed"
		result.NextSteps = []string{"Install br or run gate run where br is on PATH, then retry."}
		_ = appendGateRunJournalStep(result, "beads_br", "blocked", "br executable not found")
		return err
	}
	closeJournalPath := closureJournalPath(beadIDs[0])
	closeJournal := beadsCloseJournal{
		SchemaVersion: 1,
		Command:       "gate run beads close",
		BeadIDs:       beadIDs,
		Steps:         []beadsCloseJournalStep{beadsCloseMessageStep("preflight", "done", "preflight completed")},
	}
	_ = writeBeadsCloseJournal(root, closeJournalPath, closeJournal)
	statusByID := map[string]beadsPreflightBead{}
	for _, bead := range preflight.Beads {
		statusByID[bead.ID] = bead
	}
	for _, id := range beadIDs {
		if statusByID[id].Status == "closed" {
			closeJournal.Steps = append(closeJournal.Steps, beadsCloseMessageStep("br-close:"+id, "skipped", "bead already closed"))
			_ = writeBeadsCloseJournal(root, closeJournalPath, closeJournal)
			continue
		}
		step, err := runBeadsCloseCommand(cmd, root, closeJournalPath, "br-close-"+sanitizeStepID(id), brPath, "close", id, "--reason", handoff.Beads.Reason)
		closeJournal.Steps = append(closeJournal.Steps, step)
		_ = writeBeadsCloseJournal(root, closeJournalPath, closeJournal)
		result.PartialSuccess = true
		if err != nil {
			result.Status = "blocked"
			result.Phase = "close_or_sync_failed"
			result.NextSteps = []string{"br close failed for " + id + "; inspect the Beads close journal and retry after fixing the blocker."}
			_ = appendGateRunJournalStep(result, "beads_close", "blocked", step.Message)
			return err
		}
	}
	if handoff.Beads.Sync {
		step, err := runBeadsCloseCommand(cmd, root, closeJournalPath, "br-sync", brPath, "sync", "--flush-only")
		closeJournal.Steps = append(closeJournal.Steps, step)
		_ = writeBeadsCloseJournal(root, closeJournalPath, closeJournal)
		result.PartialSuccess = true
		if err != nil {
			result.Status = "blocked"
			result.Phase = "close_or_sync_failed"
			result.NextSteps = []string{"br sync --flush-only failed; inspect the Beads close journal and retry after fixing the blocker."}
			_ = appendGateRunJournalStep(result, "beads_sync", "blocked", step.Message)
			return err
		}
	}
	step, err := runBeadsCloseCommand(cmd, root, closeJournalPath, "stage-beads", "git", "add", ".beads/issues.jsonl")
	closeJournal.Steps = append(closeJournal.Steps, step)
	_ = writeBeadsCloseJournal(root, closeJournalPath, closeJournal)
	result.PartialSuccess = true
	if err != nil {
		result.Status = "blocked"
		result.Phase = "close_or_sync_failed"
		result.NextSteps = []string{"Staging .beads/issues.jsonl failed; retry after fixing the Git/index blocker."}
		_ = appendGateRunJournalStep(result, "beads_stage", "blocked", step.Message)
		return err
	}
	result.StagedPaths = stagedPathNames(root)
	if err := appendGateRunJournalStep(result, "beads_close", "completed", "closed/synced Beads metadata and staged .beads/issues.jsonl through existing Beads close machinery"); err != nil {
		result.Status = "blocked"
		result.Phase = "journal"
		result.NextSteps = []string{err.Error()}
		return err
	}
	result.Warnings = append(result.Warnings, "gate run included .beads/issues.jsonl in the staged payload because handoff.beads.close=true")
	return nil
}

func reconcileGateRunJournal(result *gateRunResult) error {
	body, err := os.ReadFile(result.JournalPath)
	if err != nil {
		return fail(1, "resume requires existing gate run journal %s: %v", result.JournalPath, err)
	}
	var journal gateRunJournal
	if err := json.Unmarshal(body, &journal); err != nil {
		return fail(2, "decode gate run journal %s: %v", result.JournalPath, err)
	}
	result.Resumed = true
	result.PreviousJournalHash = gateRunHash(body)
	result.HandoffHash = journal.HandoffHash
	result.CurrentHead = currentGitHead()
	result.Warnings = append(result.Warnings, "resume reconciled existing journal with current repository HEAD")
	return nil
}

func canonicalGateRunHandoffJSON(handoff gateRunHandoff) ([]byte, error) {
	body, err := json.MarshalIndent(handoff, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

func encodeGateRunJournal(journal gateRunJournal) ([]byte, error) {
	body, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return nil, fail(2, "encode gate run journal: %v", err)
	}
	return append(body, '\n'), nil
}

func appendGateRunJournalStep(result *gateRunResult, id, status, message string) error {
	body, err := os.ReadFile(result.JournalPath)
	if err != nil {
		return fail(1, "read gate run journal %s: %v", result.JournalPath, err)
	}
	var journal gateRunJournal
	if err := json.Unmarshal(body, &journal); err != nil {
		return fail(2, "decode gate run journal %s: %v", result.JournalPath, err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	journal.Status = result.Status
	journal.Phase = result.Phase
	journal.UpdatedAt = now
	journal.CurrentHead = result.CurrentHead
	journal.StagedPayloadHash = result.StagedPayloadHash
	journal.StagedPaths = append([]string(nil), result.StagedPaths...)
	journal.PeerDirtPaths = append([]string(nil), result.PeerDirtPaths...)
	journal.AttestationPaths = append([]string(nil), result.AttestationPaths...)
	journal.VerifierPromptsPath = result.VerifierPromptsPath
	journal.PendingConditions = append([]string(nil), result.PendingConditions...)
	journal.BlockingConditions = append([]string(nil), result.BlockingConditions...)
	journal.ExecutableConditions = cloneGateRunExecutableConditions(result.ExecutableConditions)
	journal.PlannedPushCommand = append([]string(nil), result.PlannedPushCommand...)
	journal.PushJournaled = result.PushJournaled
	journal.ReservationRelease = result.ReservationRelease
	journal.WakeRef = result.WakeRef
	journal.WakeInstructions = append([]string(nil), result.WakeInstructions...)
	journal.Steps = append(journal.Steps, gateRunJournalStep{
		ID:        id,
		Status:    status,
		Message:   message,
		CreatedAt: now,
	})
	journalBody, err := encodeGateRunJournal(journal)
	if err != nil {
		return err
	}
	if err := writeGateRunFile(result.JournalPath, journalBody); err != nil {
		return err
	}
	result.JournalHash = gateRunHash(journalBody)
	return nil
}

func writeGateRunFile(path string, body []byte) error {
	if strings.TrimSpace(path) == "" {
		return fail(2, "gate run output path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fail(1, "create gate run directory %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return fail(1, "write gate run file %s: %v", path, err)
	}
	return nil
}

func gateRunHash(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func currentGateRunPayloadHash() (string, error) {
	ctx, cancel := backpressure.WithTimeout(context.Background())
	defer cancel()
	payload, err := backpressure.HashStagedPayload(ctx, ".", backpressure.GitStagedReader{})
	if err != nil {
		return "", fail(2, "hash staged payload: %v", err)
	}
	return payload.Hash, nil
}

func currentGitHead() string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func validateGateRunHandoff(h gateRunHandoff, opts gateRunOptions) error {
	if h.SchemaVersion != 1 {
		return fail(2, "gate run handoff schema_version must be 1")
	}
	if cleanedGateRunID(h.RunID) == "" {
		return fail(2, "gate run handoff run_id is required")
	}
	switch h.WorkUnit.Kind {
	case "single", "lane":
	default:
		return fail(2, "gate run work_unit.kind must be single or lane")
	}
	if strings.TrimSpace(h.WorkUnit.Feature) == "" {
		return fail(2, "gate run work_unit.feature is required")
	}
	if len(h.WorkUnit.BeadIDs) == 0 {
		return fail(2, "gate run work_unit.bead_ids must name at least one bead")
	}
	if h.WorkUnit.Kind == "lane" && (strings.TrimSpace(h.WorkUnit.LaneID) == "" || strings.TrimSpace(h.WorkUnit.Rationale) == "") {
		return fail(2, "gate run lane work units require lane_id and rationale")
	}
	if strings.TrimSpace(h.Authorization.Kind) == "" || strings.TrimSpace(h.Authorization.Authority) == "" || strings.TrimSpace(h.Authorization.AuditRef) == "" {
		return fail(2, "gate run authorization kind, authority, and audit_ref are required")
	}
	if strings.TrimSpace(h.Git.ExpectedHead) == "" && !opts.dryRun {
		return fail(2, "gate run git.expected_head is required before mutation")
	}
	if strings.TrimSpace(h.Git.ExpectedStagedHash) != "" && !strings.HasPrefix(strings.TrimSpace(h.Git.ExpectedStagedHash), "sha256:") {
		return fail(2, "gate run git.expected_staged_hash must use sha256:<hex> format")
	}
	if len(h.Git.StagePaths) == 0 {
		return fail(2, "gate run git.stage_paths must name exact paths")
	}
	for _, stagePath := range h.Git.StagePaths {
		if !validGateRunStagePath(stagePath) {
			return fail(2, "gate run git.stage_paths contains invalid path %q", stagePath)
		}
	}
	for _, protectedPath := range h.Git.ProtectedPaths {
		if !validGateRunStagePath(protectedPath) {
			return fail(2, "gate run git.protected_paths contains invalid path %q", protectedPath)
		}
	}
	if strings.TrimSpace(commitMessageForGateRun(h, opts)) == "" {
		return fail(2, "gate run git.commit_message is required")
	}
	if opts.message != "" && !h.Git.AllowMessageOverride {
		return fail(2, "gate run --message override is not permitted by this handoff")
	}
	if strings.TrimSpace(h.Verification.Feature) == "" || strings.TrimSpace(h.Verification.ResponsesPath) == "" {
		return fail(2, "gate run verification.feature and responses_path are required")
	}
	if h.Beads.Close && strings.TrimSpace(h.Beads.Reason) == "" {
		return fail(2, "gate run beads.reason is required when beads.close is true")
	}
	return nil
}

func baseGateRunResult(opts gateRunOptions, handoff gateRunHandoff, handoffPath string) gateRunResult {
	runID := cleanedGateRunID(handoff.RunID)
	canonical := ""
	journal := ""
	if runID != "" {
		canonical = filepath.Join("log", "backpressure", "gate-runs", runID+"-handoff.json")
		journal = filepath.Join("log", "backpressure", "gate-runs", runID+"-journal.json")
	}
	remote := firstNonEmpty(opts.remote, handoff.Git.Remote)
	branch := firstNonEmpty(opts.branch, handoff.Git.Branch)
	push := []string(nil)
	if opts.journalPush && handoff.Git.PublishAfterCommit && remote != "" && branch != "" {
		push = []string{"git", "push", remote, branch}
	}
	return gateRunResult{
		SchemaVersion:        1,
		Command:              "gate run",
		Status:               "blocked",
		Phase:                "handoff",
		PartialSuccess:       false,
		Mutating:             !opts.dryRun && opts.yes,
		RunID:                runID,
		WorkUnit:             handoff.WorkUnit,
		HandoffPath:          handoffPath,
		CanonicalHandoffPath: canonical,
		JournalPath:          journal,
		ExpectedHead:         handoff.Git.ExpectedHead,
		ExpectedStagedHash:   strings.TrimSpace(handoff.Git.ExpectedStagedHash),
		StagePaths:           append([]string(nil), handoff.Git.StagePaths...),
		CommitMessage:        commitMessageForGateRun(handoff, opts),
		ResponsesPath:        handoff.Verification.ResponsesPath,
		PlannedPushCommand:   push,
		Agent:                opts.agent,
		Model:                opts.model,
		Warnings: []string{
			"gate run mutates only after --yes or robots confirm=true",
			"publication target fields journal a push command only; gate run v1 does not push",
		},
	}
}

func commitMessageForGateRun(h gateRunHandoff, opts gateRunOptions) string {
	if opts.message != "" {
		return opts.message
	}
	return h.Git.CommitMessage
}

func gitAddExact(paths []string) error {
	args := append([]string{"add", "--"}, paths...)
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fail(2, "stage exact gate-run paths: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func gateRunResponseBindingHash(path string) (string, bool) {
	if strings.TrimSpace(path) == "" {
		return "", false
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var doc struct {
		Binding struct {
			StagedPayloadHash string `json:"staged_payload_hash"`
		} `json:"binding"`
		StagedPayloadHash string `json:"staged_payload_hash"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", false
	}
	if strings.TrimSpace(doc.Binding.StagedPayloadHash) != "" {
		return strings.TrimSpace(doc.Binding.StagedPayloadHash), true
	}
	if strings.TrimSpace(doc.StagedPayloadHash) != "" {
		return strings.TrimSpace(doc.StagedPayloadHash), true
	}
	return "", false
}

func validateGateRunResponsesFile(path string, prompts backpressure.VerifierPromptSet, handoff gateRunHandoff, result *gateRunResult) error {
	responses, err := backpressure.LoadResponses(path)
	if err != nil {
		result.Status = "blocked"
		result.Phase = "verifier_responses"
		result.NextSteps = []string{"Responses file is unreadable or malformed; regenerate it with verifier begin and rerun gate run --resume."}
		return fail(2, "gate run verifier responses unreadable: %v", err)
	}
	if responses.Binding.StagedPayloadHash != result.StagedPayloadHash {
		result.Status = "blocked"
		result.Phase = "verifier_responses"
		result.NextSteps = []string{fmt.Sprintf("Response file %s is bound to %s, but the staged payload hash is %s; rerun verifier begin for the current payload.", path, responses.Binding.StagedPayloadHash, result.StagedPayloadHash)}
		return fail(2, "gate run stale verifier responses: got %s want %s", responses.Binding.StagedPayloadHash, result.StagedPayloadHash)
	}
	if responses.Binding.ManifestHash != prompts.ManifestHash {
		result.Status = "blocked"
		result.Phase = "verifier_responses"
		result.NextSteps = []string{fmt.Sprintf("Response file %s has manifest hash %s, but the current manifest hash is %s; rerun verifier begin.", path, responses.Binding.ManifestHash, prompts.ManifestHash)}
		return fail(2, "gate run stale verifier manifest binding")
	}
	if err := validateGateRunResponseLaneBinding(responses, handoff); err != nil {
		result.Status = "blocked"
		result.Phase = "verifier_responses"
		result.NextSteps = []string{err.Error()}
		return err
	}
	bindingHashes := map[string]string{}
	for _, binding := range responses.Binding.Conditions {
		bindingHashes[binding.ConditionID] = binding.ConditionFileHash
	}
	responsesByCondition := map[string]backpressure.ResponseCondition{}
	for _, response := range responses.Conditions {
		if _, exists := responsesByCondition[response.ConditionID]; exists {
			result.BlockingConditions = append(result.BlockingConditions, response.ConditionID)
			continue
		}
		responsesByCondition[response.ConditionID] = response
	}
	allowed := gateRunRequiredVerdicts(handoff.Verification.RequiredVerdicts)
	for _, packet := range prompts.Packets {
		if got := bindingHashes[packet.ConditionID]; got != packet.ConditionFileHash {
			result.BlockingConditions = append(result.BlockingConditions, packet.ConditionID)
			continue
		}
		response, ok := responsesByCondition[packet.ConditionID]
		if !ok {
			result.PendingConditions = append(result.PendingConditions, packet.ConditionID)
			continue
		}
		if err := validateGateRunResponseCondition(packet, response, allowed); err != nil {
			if response.Verdict == attestations.VerdictUnknown || response.Verdict == "" {
				result.PendingConditions = append(result.PendingConditions, packet.ConditionID)
			} else {
				result.BlockingConditions = append(result.BlockingConditions, packet.ConditionID)
			}
		}
	}
	sort.Strings(result.PendingConditions)
	sort.Strings(result.BlockingConditions)
	if len(result.PendingConditions) > 0 || len(result.BlockingConditions) > 0 {
		result.Status = "blocked"
		result.Phase = "verifier_responses"
		var parts []string
		if len(result.PendingConditions) > 0 {
			parts = append(parts, "pending: "+strings.Join(result.PendingConditions, ", "))
		}
		if len(result.BlockingConditions) > 0 {
			parts = append(parts, "blocking: "+strings.Join(result.BlockingConditions, ", "))
		}
		result.NextSteps = []string{
			"Send or complete verifier packets from " + result.VerifierPromptsPath + ", submit cells into " + path + ", then rerun gate run --resume.",
			"Verifier response status: " + strings.Join(parts, "; "),
		}
		return fail(2, "gate run verifier responses incomplete: %s", strings.Join(parts, "; "))
	}
	return nil
}

func validateGateRunResponseCondition(packet backpressure.VerifierPromptPacket, response backpressure.ResponseCondition, allowed map[attestations.Verdict]bool) error {
	if response.ConditionID != packet.ConditionID {
		return fmt.Errorf("condition %q response id mismatch", packet.ConditionID)
	}
	if response.ConditionFile != "" && response.ConditionFile != packet.ConditionFile {
		return fmt.Errorf("condition %q response file mismatch", packet.ConditionID)
	}
	kind := attestations.EffectiveVerifierKind(response.Verifier, response.SubagentConfirmed)
	if !attestations.ValidVerifierKind(kind) || !attestations.VerifierPolicyAllows(packet.VerifierPolicy, kind) {
		return fmt.Errorf("condition %q verifier policy %q does not allow verifier kind %q", packet.ConditionID, packet.VerifierPolicy, kind)
	}
	if strings.TrimSpace(response.Verifier.Model) == "" && strings.TrimSpace(response.SubagentModel) == "" {
		return fmt.Errorf("condition %q missing verifier model", packet.ConditionID)
	}
	if strings.TrimSpace(response.Verifier.Runtime) == "" {
		return fmt.Errorf("condition %q missing verifier runtime", packet.ConditionID)
	}
	if !response.Verifier.SeparateContext {
		return fmt.Errorf("condition %q requires verifier separate_context=true", packet.ConditionID)
	}
	if !allowed[response.Verdict] {
		return fmt.Errorf("condition %q verdict %q is not allowed", packet.ConditionID, response.Verdict)
	}
	if len(nonEmptyGateRunStrings(response.Evidence)) == 0 {
		return fmt.Errorf("condition %q missing evidence", packet.ConditionID)
	}
	if response.Verdict == attestations.VerdictNotApplicable && strings.TrimSpace(response.Message) == "" {
		return fmt.Errorf("condition %q not_applicable requires message", packet.ConditionID)
	}
	for _, supplemental := range response.Supplemental {
		if supplemental.Verdict != "" && supplemental.Verdict != response.Verdict && response.Adjudication == nil {
			return fmt.Errorf("condition %q has supplemental disagreement without adjudication", packet.ConditionID)
		}
		if supplemental.StagedPayloadHash != "" && supplemental.StagedPayloadHash != packet.StagedPayloadHash {
			return fmt.Errorf("condition %q supplemental staged hash is stale", packet.ConditionID)
		}
		if supplemental.ManifestHash != "" && supplemental.ManifestHash != packet.ManifestHash {
			return fmt.Errorf("condition %q supplemental manifest hash is stale", packet.ConditionID)
		}
		if supplemental.ConditionFileHash != "" && supplemental.ConditionFileHash != packet.ConditionFileHash {
			return fmt.Errorf("condition %q supplemental condition hash is stale", packet.ConditionID)
		}
	}
	return nil
}

func validateGateRunResponseLaneBinding(responses *backpressure.Responses, handoff gateRunHandoff) error {
	if handoff.WorkUnit.Kind != "lane" {
		if responses.Binding.LaneBinding != nil {
			return fail(2, "single-work-unit gate run responses must not include a lane binding")
		}
		return nil
	}
	if responses.Binding.LaneBinding == nil || responses.Atomicity.Lane == nil {
		return fail(2, "lane gate run responses require binding.lane_binding and atomicity.lane")
	}
	expected := gateRunLaneOptions(handoff)
	bound := responses.Binding.LaneBinding
	if strings.TrimSpace(bound.LaneID) != strings.TrimSpace(expected.LaneID) ||
		!samePathSet(bound.BeadIDs, expected.BeadIDs) ||
		strings.TrimSpace(bound.Rationale) != strings.TrimSpace(expected.Rationale) ||
		strings.TrimSpace(bound.AuthorizationRef) != strings.TrimSpace(expected.AuthorizationRef) ||
		strings.TrimSpace(bound.AuthorizedBy) != strings.TrimSpace(expected.AuthorizedBy) ||
		strings.TrimSpace(bound.AuthorizationKind) != strings.TrimSpace(expected.AuthorizationKind) {
		return fail(2, "lane gate run responses do not match the authorized handoff lane binding")
	}
	return nil
}

func gateRunRequiredVerdicts(values []string) map[attestations.Verdict]bool {
	if len(values) == 0 {
		return map[attestations.Verdict]bool{
			attestations.VerdictPass:          true,
			attestations.VerdictNotApplicable: true,
		}
	}
	allowed := map[attestations.Verdict]bool{}
	for _, value := range values {
		switch attestations.Verdict(strings.TrimSpace(value)) {
		case attestations.VerdictPass:
			allowed[attestations.VerdictPass] = true
		case attestations.VerdictNotApplicable:
			allowed[attestations.VerdictNotApplicable] = true
		}
	}
	if len(allowed) == 0 {
		allowed[attestations.VerdictPass] = true
		allowed[attestations.VerdictNotApplicable] = true
	}
	return allowed
}

func gateRunLaneOptions(handoff gateRunHandoff) backpressure.LaneOptions {
	if handoff.WorkUnit.Kind != "lane" {
		return backpressure.LaneOptions{}
	}
	return backpressure.LaneOptions{
		Enabled:           true,
		LaneID:            handoff.WorkUnit.LaneID,
		BeadIDs:           append([]string(nil), handoff.WorkUnit.BeadIDs...),
		Rationale:         handoff.WorkUnit.Rationale,
		AuthorizationRef:  handoff.Authorization.AuditRef,
		AuthorizedBy:      handoff.Authorization.Authority,
		AuthorizationKind: handoff.Authorization.Kind,
	}
}

func gateRunAtomicityMessage(handoff gateRunHandoff) string {
	if handoff.WorkUnit.Kind == "lane" {
		return "Orchestrator-authorized lane " + handoff.WorkUnit.LaneID + ": " + handoff.WorkUnit.Rationale
	}
	if strings.TrimSpace(handoff.WorkUnit.Rationale) != "" {
		return handoff.WorkUnit.Rationale
	}
	return "Gate-run handoff declares " + handoff.WorkUnit.Feature + " as one feature or fix."
}

func nonEmptyGateRunStrings(values []string) []string {
	var out []string
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}

func gateRunPeerDirtPaths(handoff gateRunHandoff) []string {
	return unstagedOverlappingPaths(handoff.Git.ProtectedPaths, !handoff.Git.AllowUntracked)
}

func unstagedOverlappingPaths(protected []string, includeUntracked bool) []string {
	if len(protected) == 0 {
		return nil
	}
	cmd := exec.Command("git", "status", "--porcelain=v1", "--untracked-files=all")
	body, err := cmd.Output()
	if err != nil {
		return nil
	}
	protectedSet := pathSet(protected)
	dirtySet := map[string]bool{}
	for _, line := range strings.Split(string(body), "\n") {
		if len(line) < 4 {
			continue
		}
		path := filepath.ToSlash(strings.TrimSpace(line[3:]))
		if strings.Contains(path, " -> ") {
			parts := strings.Split(path, " -> ")
			path = strings.TrimSpace(parts[len(parts)-1])
		}
		if path == "" || !protectedSet[path] {
			continue
		}
		if line[:2] == "??" {
			if includeUntracked {
				dirtySet[path] = true
			}
			continue
		}
		if line[1] != ' ' {
			dirtySet[path] = true
		}
	}
	var paths []string
	for path := range dirtySet {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func pathSet(paths []string) map[string]bool {
	set := map[string]bool{}
	for _, path := range paths {
		clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
		if clean != "" && clean != "." {
			set[clean] = true
		}
	}
	return set
}

func pathsOutsideSet(paths []string, allowed map[string]bool) []string {
	var outside []string
	for _, path := range paths {
		clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
		if clean != "" && !allowed[clean] {
			outside = append(outside, clean)
		}
	}
	sort.Strings(outside)
	return outside
}

func samePathSet(a, b []string) bool {
	aSet := pathSet(a)
	bSet := pathSet(b)
	if len(aSet) != len(bSet) {
		return false
	}
	for path := range aSet {
		if !bSet[path] {
			return false
		}
	}
	return true
}

var gateRunIDUnsafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func cleanedGateRunID(runID string) string {
	cleaned := filepath.Base(filepath.Clean(strings.TrimSpace(runID)))
	cleaned = gateRunIDUnsafe.ReplaceAllString(cleaned, "-")
	cleaned = strings.Trim(cleaned, ".-_")
	if cleaned == "." || cleaned == string(filepath.Separator) {
		return ""
	}
	return cleaned
}

func validGateRunStagePath(value string) bool {
	if strings.TrimSpace(value) == "" || filepath.IsAbs(value) {
		return false
	}
	cleaned := filepath.Clean(value)
	if cleaned == "." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || cleaned == ".." {
		return false
	}
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
