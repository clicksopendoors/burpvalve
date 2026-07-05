package backpressure

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"burpvalve/internal/attestations"
	bvconfig "burpvalve/internal/config"
	"burpvalve/internal/gitindex"
)

const (
	VerifierAuthorizationText = "Standing verifier authorization permits spawning read-only verifier subagents for backpressure checks when recorded in defaults.verifier. Authorization is policy metadata only and is never per-cell verification evidence. Do not fabricate subagent confirmation."
	stagedExcerptLimit        = 4096
)

type VerifierPromptOptions struct {
	Root      string
	Feature   string
	Condition string
	Profile   string
	Staged    StagedReader
}

type VerifierPromptSet struct {
	SchemaVersion             int                       `json:"schema_version"`
	Command                   string                    `json:"command"`
	Profile                   string                    `json:"profile"`
	Feature                   Feature                   `json:"feature"`
	ManifestHash              string                    `json:"manifest_hash"`
	StagedPayloadHash         string                    `json:"staged_payload_hash"`
	StagedPayload             []StagedPayloadFile       `json:"staged_payload"`
	HashExcludedStagedPayload []StagedPayloadFile       `json:"hash_excluded_staged_payload,omitempty"`
	GeneratedPathPrefixes     []string                  `json:"generated_path_prefixes"`
	Authorization             VerifierPromptAuth        `json:"authorization"`
	Packets                   []VerifierPromptPacket    `json:"packets"`
	Notes                     []string                  `json:"notes"`
	Warnings                  []string                  `json:"warnings,omitempty"`
	StagedPayloadDetails      []VerifierPromptFileSlice `json:"staged_payload_details,omitempty"`
}

type VerifierPromptPacket struct {
	ID                        string                      `json:"id"`
	Profile                   string                      `json:"profile"`
	FeatureID                 string                      `json:"feature_id"`
	FeatureName               string                      `json:"feature_name"`
	ConditionID               string                      `json:"condition_id"`
	ConditionFile             string                      `json:"condition_file"`
	ConditionFileHash         string                      `json:"condition_file_hash"`
	ConditionContent          string                      `json:"condition_content"`
	VerifierPolicy            attestations.VerifierPolicy `json:"verifier_policy"`
	ManifestHash              string                      `json:"manifest_hash"`
	StagedPayloadHash         string                      `json:"staged_payload_hash"`
	StagedPayload             []StagedPayloadFile         `json:"staged_payload"`
	HashExcludedStagedPayload []StagedPayloadFile         `json:"hash_excluded_staged_payload,omitempty"`
	GeneratedPathPrefixes     []string                    `json:"generated_path_prefixes"`
	StagedPayloadDetails      []VerifierPromptFileSlice   `json:"staged_payload_details"`
	HashReproduction          string                      `json:"hash_reproduction"`
	Authorization             VerifierPromptAuth          `json:"authorization"`
	ReadOnlyExpectation       string                      `json:"read_only_expectation"`
	SuccessCriteria           string                      `json:"success_criteria"`
	ResponseSchema            ResponseCondition           `json:"response_schema"`
	ResponseSchemaJSON        string                      `json:"response_schema_json"`
	SubmitCommand             string                      `json:"submit_command"`
	Prompt                    string                      `json:"prompt"`
	ProfileNotes              []string                    `json:"profile_notes"`
	Warnings                  []string                    `json:"warnings,omitempty"`
}

type VerifierPromptAuth struct {
	Recorded           bool   `json:"recorded"`
	Authorized         bool   `json:"authorized"`
	AuthorizedAt       string `json:"authorized_at,omitempty"`
	AuthorizationScope string `json:"authorization_scope,omitempty"`
	SpawnMethod        string `json:"spawn_method,omitempty"`
	Message            string `json:"message"`
}

type VerifierPromptFileSlice struct {
	Path             string `json:"path"`
	OldPath          string `json:"old_path,omitempty"`
	Status           string `json:"status"`
	GitStatus        string `json:"git_status,omitempty"`
	HashIncluded     bool   `json:"hash_included"`
	Generated        bool   `json:"generated"`
	ContentSize      int    `json:"content_size,omitempty"`
	ContentExcerpt   string `json:"content_excerpt,omitempty"`
	ContentTruncated bool   `json:"content_truncated,omitempty"`
	ReadError        string `json:"read_error,omitempty"`
}

func BuildVerifierPrompts(ctx context.Context, opts VerifierPromptOptions) (VerifierPromptSet, error) {
	profile := normalizeVerifierPromptProfile(opts.Profile)
	if !validVerifierPromptProfile(profile) {
		return VerifierPromptSet{}, fmt.Errorf("unsupported verifier prompt profile %q; expected native, ntm, hermes, or manual", opts.Profile)
	}
	root, err := filepath.Abs(defaultRoot(opts.Root))
	if err != nil {
		return VerifierPromptSet{}, err
	}
	staged := opts.Staged
	if staged == nil {
		staged = GitStagedReader{}
	}
	plan, err := BuildPlan(ctx, Options{
		Root:            root,
		Mode:            "pre-commit",
		ExplicitFeature: opts.Feature,
		Staged:          staged,
	})
	if err != nil {
		return VerifierPromptSet{}, err
	}
	if len(plan.Features) == 0 {
		return VerifierPromptSet{}, fmt.Errorf("missing feature for verifier prompts")
	}
	if len(plan.StagedPayloadFiles) == 0 {
		return VerifierPromptSet{}, fmt.Errorf("no staged payload for verifier prompts")
	}
	allStaged, err := verifierPromptAllStagedEntries(ctx, root, staged)
	if err != nil {
		return VerifierPromptSet{}, err
	}
	excluded := verifierPromptExcludedEntries(allStaged)
	details := verifierPromptFileSlices(ctx, root, staged, allStaged)
	auth, configWarnings := verifierPromptAuthorization(root, plan.Matrix.Conditions)
	feature := plan.Features[0]
	set := VerifierPromptSet{
		SchemaVersion:             1,
		Command:                   "verifier prompts",
		Profile:                   profile,
		Feature:                   feature,
		ManifestHash:              plan.ManifestHash,
		StagedPayloadHash:         plan.StagedPayloadHash,
		StagedPayload:             append([]StagedPayloadFile(nil), plan.StagedPayloadFiles...),
		HashExcludedStagedPayload: excluded,
		GeneratedPathPrefixes:     gitindex.GeneratedPathPrefixes(),
		Authorization:             auth,
		Notes:                     verifierPromptProfileNotes(profile),
		Warnings:                  configWarnings,
		StagedPayloadDetails:      details,
	}
	for _, condition := range plan.Matrix.Conditions {
		if opts.Condition != "" && condition.ID != opts.Condition {
			continue
		}
		conditionContent, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(condition.Path)))
		if err != nil {
			return VerifierPromptSet{}, fmt.Errorf("read condition %s: %w", condition.ID, err)
		}
		set.Packets = append(set.Packets, buildVerifierPromptPacket(profile, feature, condition, plan, auth, string(conditionContent), excluded, details, configWarnings))
	}
	if len(set.Packets) == 0 {
		return VerifierPromptSet{}, fmt.Errorf("condition %q not found in enabled backpressure matrix", opts.Condition)
	}
	return set, nil
}

func buildVerifierPromptPacket(profile string, feature Feature, condition ConditionSpec, plan Plan, auth VerifierPromptAuth, conditionContent string, excluded []StagedPayloadFile, details []VerifierPromptFileSlice, warnings []string) VerifierPromptPacket {
	policy := attestations.NormalizeVerifierPolicy(condition.VerifierPolicy)
	response := ResponseCondition{
		ConditionID:    condition.ID,
		ConditionFile:  condition.Path,
		VerifierPolicy: policy,
		Verifier: attestations.Verifier{
			Kind:            attestations.VerifierUnknown,
			SeparateContext: true,
		},
		SubagentConfirmed: false,
		Verdict:           attestations.VerdictUnknown,
		Message:           "replace with verifier finding",
		Evidence:          []string{"replace with concrete evidence"},
		NextAction:        "replace when verdict is fail or unknown",
	}
	conditionHash := plan.ConditionFileHashes[condition.ID]
	packet := VerifierPromptPacket{
		ID:                        feature.ID + "/" + condition.ID,
		Profile:                   profile,
		FeatureID:                 feature.ID,
		FeatureName:               feature.Name,
		ConditionID:               condition.ID,
		ConditionFile:             condition.Path,
		ConditionFileHash:         conditionHash,
		ConditionContent:          conditionContent,
		VerifierPolicy:            policy,
		ManifestHash:              plan.ManifestHash,
		StagedPayloadHash:         plan.StagedPayloadHash,
		StagedPayload:             append([]StagedPayloadFile(nil), plan.StagedPayloadFiles...),
		HashExcludedStagedPayload: append([]StagedPayloadFile(nil), excluded...),
		GeneratedPathPrefixes:     gitindex.GeneratedPathPrefixes(),
		StagedPayloadDetails:      append([]VerifierPromptFileSlice(nil), details...),
		HashReproduction:          stagedPayloadHashReproductionText(),
		Authorization:             auth,
		ReadOnlyExpectation:       "Inspect the current staged payload, hash-excluded generated paths, and this condition file only. Do not edit files, stage files, commit, or fabricate confirmation.",
		SuccessCriteria:           verifierPromptSuccessCriteria(condition.ID),
		ResponseSchema:            response,
		ResponseSchemaJSON:        verifierPromptResponseSchemaJSON(response),
		SubmitCommand:             verifierSubmitCommand(feature.ID, condition.ID, plan.StagedPayloadHash, plan.ManifestHash, conditionHash),
		ProfileNotes:              verifierPromptProfileNotes(profile),
		Warnings:                  append([]string(nil), warnings...),
	}
	packet.Prompt = renderVerifierPromptPacket(packet)
	return packet
}

func renderVerifierPromptPacket(packet VerifierPromptPacket) string {
	var b strings.Builder
	fmt.Fprintln(&b, packet.Authorization.Message)
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "Feature: %s (%s)\n", packet.FeatureID, packet.FeatureName)
	fmt.Fprintf(&b, "Condition: %s (%s)\n", packet.ConditionID, packet.ConditionFile)
	fmt.Fprintf(&b, "Verifier policy: %s\n", packet.VerifierPolicy)
	fmt.Fprintf(&b, "Staged payload hash: %s\n", packet.StagedPayloadHash)
	fmt.Fprintf(&b, "Manifest hash: %s\n", packet.ManifestHash)
	fmt.Fprintf(&b, "Condition file hash: %s\n", packet.ConditionFileHash)
	fmt.Fprintln(&b, "Read-only expectation: "+packet.ReadOnlyExpectation)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Hash-included staged payload:")
	for _, file := range packet.StagedPayload {
		if file.OldPath != "" {
			fmt.Fprintf(&b, "- %s %s (from %s)\n", file.Status, file.Path, file.OldPath)
		} else {
			fmt.Fprintf(&b, "- %s %s\n", file.Status, file.Path)
		}
	}
	if len(packet.HashExcludedStagedPayload) > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "Hash-excluded generated staged paths:")
		for _, file := range packet.HashExcludedStagedPayload {
			fmt.Fprintf(&b, "- %s %s (generated/hash-excluded)\n", file.Status, file.Path)
		}
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Staged content excerpts:")
	for _, detail := range packet.StagedPayloadDetails {
		label := "hash-included"
		if !detail.HashIncluded {
			label = "generated/hash-excluded"
		}
		fmt.Fprintf(&b, "- %s %s [%s]\n", detail.Status, detail.Path, label)
		if detail.ContentExcerpt != "" {
			fmt.Fprintf(&b, "  excerpt (%d bytes%s):\n%s\n", detail.ContentSize, truncatedSuffix(detail.ContentTruncated), indentBlock(detail.ContentExcerpt, "  "))
		} else if detail.ReadError != "" {
			fmt.Fprintf(&b, "  content: %s\n", detail.ReadError)
		}
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Condition contents:")
	fmt.Fprintln(&b, indentBlock(packet.ConditionContent, "  "))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Hash reproduction:")
	fmt.Fprintln(&b, indentBlock(packet.HashReproduction, "  "))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Success criteria:")
	fmt.Fprintln(&b, indentBlock(packet.SuccessCriteria, "  "))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Response JSON schema:")
	fmt.Fprintln(&b, indentBlock(packet.ResponseSchemaJSON, "  "))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Submit command:")
	fmt.Fprintln(&b, indentBlock(packet.SubmitCommand, "  "))
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Return JSON matching the inline schema for this packet. Do not fabricate subagent confirmation.")
	return strings.TrimSpace(b.String())
}

func verifierPromptAllStagedEntries(ctx context.Context, root string, staged StagedReader) ([]StagedPayloadFile, error) {
	entries, err := stagedPayloadEntries(ctx, root, staged)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		entries[i].Path = filepath.ToSlash(entries[i].Path)
		entries[i].OldPath = filepath.ToSlash(entries[i].OldPath)
		if entries[i].Status == "" {
			entries[i].Status = "modified"
		}
	}
	sortStagedEntries(entries)
	return entries, nil
}

func verifierPromptExcludedEntries(entries []StagedPayloadFile) []StagedPayloadFile {
	var excluded []StagedPayloadFile
	for _, entry := range entries {
		if isGeneratedPath(entry.Path) {
			excluded = append(excluded, entry)
		}
	}
	return excluded
}

func verifierPromptFileSlices(ctx context.Context, root string, staged StagedReader, entries []StagedPayloadFile) []VerifierPromptFileSlice {
	details := make([]VerifierPromptFileSlice, 0, len(entries))
	for _, entry := range entries {
		generated := isGeneratedPath(entry.Path)
		detail := VerifierPromptFileSlice{
			Path:         entry.Path,
			OldPath:      entry.OldPath,
			Status:       entry.Status,
			GitStatus:    entry.GitStatus,
			HashIncluded: !generated,
			Generated:    generated,
		}
		if entry.Status == "deleted" {
			detail.ReadError = "deleted file has no staged content"
			details = append(details, detail)
			continue
		}
		body, err := staged.StagedFileContent(ctx, root, entry.Path)
		if err != nil {
			detail.ReadError = err.Error()
			details = append(details, detail)
			continue
		}
		detail.ContentSize = len(body)
		if len(body) > stagedExcerptLimit {
			detail.ContentExcerpt = string(body[:stagedExcerptLimit])
			detail.ContentTruncated = true
		} else {
			detail.ContentExcerpt = string(body)
		}
		details = append(details, detail)
	}
	return details
}

func verifierPromptAuthorization(root string, conditions []ConditionSpec) (VerifierPromptAuth, []string) {
	effective, err := bvconfig.Load(root)
	if err != nil {
		return VerifierPromptAuth{
			Message: "Verifier authorization is not recorded because Burpvalve config could not be loaded. Obtain repo-owner authorization with `burpvalve config init` before spawning read-only verifier subagents. Authorization is never per-cell evidence.",
		}, []string{"load verifier config: " + err.Error()}
	}
	defaults := effective.File.Defaults.Verifier
	auth := VerifierPromptAuth{
		Recorded:           defaults.Authorized != nil,
		Authorized:         bvconfig.BoolValue(defaults.Authorized, false),
		AuthorizedAt:       defaults.AuthorizedAt,
		AuthorizationScope: defaults.AuthorizationScope,
		SpawnMethod:        defaults.SpawnMethod,
	}
	if auth.Recorded && auth.Authorized {
		auth.Message = fmt.Sprintf("Recorded verifier authorization scope: %s. %s", defaultString(auth.AuthorizationScope, "unspecified"), VerifierAuthorizationText)
	} else {
		auth.Message = "Verifier authorization is not recorded as granted. Obtain repo-owner authorization with `burpvalve config init` or direct owner approval before spawning read-only verifier subagents. Authorization is never per-cell evidence."
	}
	warnings := verifierConditionModelWarnings(defaults.ConditionModels, conditions)
	return auth, warnings
}

func verifierConditionModelWarnings(models map[string]string, conditions []ConditionSpec) []string {
	if len(models) == 0 {
		return nil
	}
	active := map[string]bool{}
	for _, condition := range conditions {
		active[condition.ID] = true
	}
	var keys []string
	for key := range models {
		if !active[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	warnings := make([]string, 0, len(keys))
	for _, key := range keys {
		warnings = append(warnings, "defaults.verifier.condition_models."+key+" is configured but is not present in the active manifest; ignoring it for this prompt run")
	}
	return warnings
}

func stagedPayloadHashReproductionText() string {
	return "Burpvalve hashes only hash-included staged entries after sorting by path, old_path, and normalized status. Generated evidence JSON under " + strings.Join(gitindex.GeneratedPathPrefixes(), ", ") + " is listed in packets but excluded from the payload hash; scaffold/docs files under those directories are hash-included unless they are recognized generated evidence. For each included entry, the hash input is path, normalized status, old path, raw git status, then staged content size and staged content for non-deleted files. Deleted files contribute only their metadata. This is the canonical HashStagedPayload contract; do not substitute `git diff --cached --binary | sha256sum`."
}

func verifierPromptSuccessCriteria(conditionID string) string {
	criteria := "Return one response object for this exact feature x condition cell with verifier provenance, verdict, message, evidence, and next_action when blocked. Verdict semantics are binding: not_applicable means the staged payload contains no surface governed by this condition; pass means the staged payload contains a governed surface, the verifier inspected that surface, and the condition is satisfied, even when the factual finding is that no dangerous thing is present. The distinction is whether there was a governed thing to check, not whether a problem was found."
	if conditionID == "anti-reward-hacking" {
		criteria += " For anti-reward-hacking, check the staged payload's consistency with its own claims, not merely the absence of shortcut-authorization language. In land-04, the staged decision document contained a bare workflow chain while another decision in the same document declared that form defective; the verifier correctly failed anti-reward-hacking because the payload contradicted its own standard."
	}
	criteria += " D7 hard rule: authorization permits spawning; the authorization string alone is never acceptable evidence."
	return criteria
}

func verifierPromptResponseSchemaJSON(response ResponseCondition) string {
	schema := map[string]any{
		"condition_id":       response.ConditionID,
		"condition_file":     response.ConditionFile,
		"verifier_policy":    response.VerifierPolicy,
		"verifier":           response.Verifier,
		"subagent_confirmed": response.SubagentConfirmed,
		"subagent_model":     response.SubagentModel,
		"verdict":            "pass | not_applicable | fail | unknown",
		"message":            response.Message,
		"evidence":           response.Evidence,
		"next_action":        response.NextAction,
		"supplemental_verifiers": []map[string]any{
			{
				"verifier":       response.Verifier,
				"verdict":        "pass | not_applicable | fail | unknown",
				"message":        "supplemental verifier finding",
				"evidence":       []string{"supplemental evidence"},
				"transcript_ref": "optional transcript reference",
				"next_action":    "required for fail or unknown",
			},
		},
		"adjudication": map[string]any{
			"authority":     "agent or person who ruled on a disagreement",
			"summary":       "ruling summary",
			"final_verdict": "optional final verdict",
			"audit_ref":     "Agent Mail message id, thread id, or equivalent audit reference",
		},
	}
	body, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(body)
}

func verifierSubmitCommand(featureID, conditionID, payloadHash, manifestHash, conditionHash string) string {
	return fmt.Sprintf("burpvalve verifier submit --feature %s --condition %s --staged-payload-hash %s --manifest-hash %s --condition-file-hash %s", shellQuote(featureID), shellQuote(conditionID), shellQuote(payloadHash), shellQuote(manifestHash), shellQuote(conditionHash))
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if strings.IndexFunc(value, func(r rune) bool {
		return !(r == '/' || r == ':' || r == '.' || r == '-' || r == '_' || r == '=' || r == '+' || r == '@' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z')
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func truncatedSuffix(truncated bool) string {
	if truncated {
		return ", truncated"
	}
	return ""
}

func indentBlock(value, prefix string) string {
	value = strings.TrimRight(value, "\n")
	if value == "" {
		return prefix + "(empty)"
	}
	lines := strings.Split(value, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

func normalizeVerifierPromptProfile(profile string) string {
	profile = strings.ToLower(strings.TrimSpace(profile))
	if profile == "" {
		return "native"
	}
	return profile
}

func validVerifierPromptProfile(profile string) bool {
	switch profile {
	case "native", "ntm", "hermes", "manual":
		return true
	default:
		return false
	}
}

func verifierPromptProfileNotes(profile string) []string {
	switch profile {
	case "native":
		return []string{"Burpvalve generated this verifier packet; it does not spawn native subagents itself.", "Use the current runtime's read-only subagent feature when available, then close the subagent thread when done."}
	case "ntm":
		return []string{"Burpvalve generated an NTM-safe task brief; it did not launch NTM or a swarm.", "Batch cells per reviewer pane according to docs/ntm-bridge.md and preserve per-cell evidence."}
	case "hermes":
		return []string{"Burpvalve generated a Hermes-ready handoff packet; it did not send a message.", "Send one packet per verifier cell and attach the response evidence to the commit response file."}
	case "manual":
		return []string{"Burpvalve generated a manual reviewer checklist; it did not verify the condition.", "Paste the response schema back only after real review evidence exists."}
	default:
		return nil
	}
}
