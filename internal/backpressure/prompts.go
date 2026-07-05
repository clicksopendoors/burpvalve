package backpressure

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"burpvalve/internal/attestations"
	"burpvalve/internal/charmui"
	"burpvalve/internal/cliui"
)

type PromptIO struct {
	In    io.Reader
	Out   io.Writer
	TUI   bool
	Color bool
}

func CollectPromptResponses(plan Plan, prompt PromptIO) (*Responses, error) {
	if prompt.TUI {
		return CollectTUIResponses(plan, prompt)
	}
	if prompt.In == nil {
		return nil, fmt.Errorf("interactive prompt input is unavailable")
	}
	if prompt.Out == nil {
		prompt.Out = io.Discard
	}
	scanner := bufio.NewScanner(prompt.In)
	responses := &Responses{
		Atomicity: attestations.Atomicity{
			OneFeatureOrFix: false,
			Message:         "Atomicity was not confirmed.",
		},
	}
	artifactPath := AttestationPath(plan.StagedPayloadHash)
	writePromptBanner(prompt.Out, plan, artifactPath, prompt.Color)
	ui := cliui.New(prompt.Color)

	atomic, err := askLine(prompt.Out, scanner, "Atomicity: does the staged diff contain exactly one atomic feature or bug fix? [y/N]: ", prompt.Color)
	if err != nil {
		return responses, err
	}
	if !yes(atomic) {
		message, err := askRequired(prompt.Out, scanner, "Atomicity blocker message: ", prompt.Color)
		if err != nil {
			return responses, err
		}
		responses.Atomicity.Message = message
		return responses, fmt.Errorf("atomicity not confirmed: %s", message)
	}
	responses.Atomicity.OneFeatureOrFix = true
	responses.Atomicity.Message = "Committing agent confirmed the staged diff contains exactly one atomic feature or bug fix."

	for i, condition := range plan.Matrix.Conditions {
		feature := promptFeature(plan)
		fmt.Fprintf(prompt.Out, "\n%s %s\n", ui.Section("Matrix cell"), ui.Info(fmt.Sprintf("%d/%d", i+1, len(plan.Matrix.Conditions))))
		fmt.Fprintf(prompt.Out, "%s %s %s\n", ui.Header("Feature:"), ui.Info(feature.ID), ui.Muted("("+feature.Name+")"))
		fmt.Fprintf(prompt.Out, "%s %s %s\n", ui.Header("Condition:"), ui.Info(condition.ID), ui.Muted("("+condition.Path+")"))
		confirmed, err := askLine(prompt.Out, scanner, "Dedicated subagent checked this exact condition for this exact feature? [y/N]: ", prompt.Color)
		if err != nil {
			return responses, err
		}
		if !yes(confirmed) {
			message, err := askRequired(prompt.Out, scanner, "Missing-subagent message: ", prompt.Color)
			if err != nil {
				return responses, err
			}
			evidence, err := askRequiredList(prompt.Out, scanner, "Missing evidence notes (comma-separated): ", prompt.Color)
			if err != nil {
				return responses, err
			}
			nextAction, err := askRequired(prompt.Out, scanner, "Next action: ", prompt.Color)
			if err != nil {
				return responses, err
			}
			responses.Conditions = append(responses.Conditions, ResponseCondition{
				ConditionID:       condition.ID,
				VerifierPolicy:    normalizeConditionPolicy(condition),
				Verifier:          attestations.Verifier{Kind: attestations.VerifierNone},
				SubagentConfirmed: false,
				Verdict:           attestations.VerdictUnknown,
				Message:           message,
				Evidence:          evidence,
				NextAction:        nextAction,
			})
			return responses, fmt.Errorf("missing dedicated subagent confirmation for %s/%s", feature.ID, condition.ID)
		}

		model, err := askLine(prompt.Out, scanner, "Subagent model/source alias (optional): ", prompt.Color)
		if err != nil {
			return responses, err
		}
		verdictText, err := askRequired(prompt.Out, scanner, "Verdict [pass|not_applicable|fail|unknown]: ", prompt.Color)
		if err != nil {
			return responses, err
		}
		verdict := attestations.Verdict(strings.TrimSpace(verdictText))
		response := ResponseCondition{
			ConditionID:       condition.ID,
			VerifierPolicy:    normalizeConditionPolicy(condition),
			Verifier:          attestations.Verifier{Kind: attestations.VerifierIndependentSubagent, Model: strings.TrimSpace(model), SeparateContext: true},
			SubagentConfirmed: true,
			SubagentModel:     strings.TrimSpace(model),
			Verdict:           verdict,
		}
		switch verdict {
		case attestations.VerdictPass:
			evidence, err := askRequiredList(prompt.Out, scanner, "Evidence summary (comma-separated): ", prompt.Color)
			if err != nil {
				return responses, err
			}
			response.Evidence = evidence
		case attestations.VerdictNotApplicable:
			message, err := askRequired(prompt.Out, scanner, "Not-applicable reason: ", prompt.Color)
			if err != nil {
				return responses, err
			}
			evidence, err := askLine(prompt.Out, scanner, "Evidence summary (comma-separated, optional): ", prompt.Color)
			if err != nil {
				return responses, err
			}
			response.Message = message
			response.Evidence = splitList(evidence)
		case attestations.VerdictFail, attestations.VerdictUnknown:
			message, err := askRequired(prompt.Out, scanner, "Failure/unknown blocker message: ", prompt.Color)
			if err != nil {
				return responses, err
			}
			evidence, err := askRequiredList(prompt.Out, scanner, "Evidence and files/commands involved (comma-separated): ", prompt.Color)
			if err != nil {
				return responses, err
			}
			nextAction, err := askRequired(prompt.Out, scanner, "Next action: ", prompt.Color)
			if err != nil {
				return responses, err
			}
			response.Message = message
			response.Evidence = evidence
			response.NextAction = nextAction
			responses.Conditions = append(responses.Conditions, response)
			return responses, fmt.Errorf("condition %s returned blocking verdict %q", condition.ID, verdict)
		default:
			response.Verdict = attestations.VerdictUnknown
			response.Message = "Invalid verdict " + string(verdict) + "."
			response.Evidence = []string{"interactive prompt received invalid verdict"}
			response.NextAction = "Rerun burpvalve commit and enter pass, not_applicable, fail, or unknown."
			responses.Conditions = append(responses.Conditions, response)
			return responses, fmt.Errorf("invalid verdict %q for condition %s", verdict, condition.ID)
		}
		responses.Conditions = append(responses.Conditions, response)
	}
	return responses, nil
}

func CollectTUIResponses(plan Plan, prompt PromptIO) (*Responses, error) {
	if prompt.In == nil {
		return nil, fmt.Errorf("interactive prompt input is unavailable")
	}
	if prompt.Out == nil {
		prompt.Out = io.Discard
	}
	responses := &Responses{
		Atomicity: attestations.Atomicity{
			OneFeatureOrFix: false,
			Message:         "Atomicity was not confirmed.",
		},
	}
	feature := promptFeature(plan)
	artifactPath := AttestationPath(plan.StagedPayloadHash)
	atomic, err := charmui.AskConfirm(prompt.In, prompt.Out, charmui.ConfirmPrompt{
		Title: "Backpressure commit gate",
		Description: "Artifact: " + artifactPath + "\n" +
			"Feature: " + feature.ID + " (" + feature.Name + ")\n" +
			"Missing, failing, unknown, unconfirmed, malformed, or stale attestations block the commit.",
		Prompt:  "Does the staged diff contain exactly one atomic feature or bug fix?",
		Default: false,
		Color:   prompt.Color,
	})
	if err != nil {
		return responses, promptError("atomicity", err)
	}
	if !atomic {
		message, err := charmui.AskText(prompt.In, prompt.Out, charmui.TextPrompt{
			Title:       "Atomicity blocker",
			Description: "The commit must be split or explained before it can pass.",
			Prompt:      "Why is this not one atomic feature or bug fix?",
			Placeholder: "Split the staged changes into ...",
			Required:    true,
			Color:       prompt.Color,
		})
		if err != nil {
			return responses, promptError("atomicity blocker", err)
		}
		responses.Atomicity.Message = message
		return responses, fmt.Errorf("atomicity not confirmed: %s", message)
	}
	responses.Atomicity.OneFeatureOrFix = true
	responses.Atomicity.Message = "Committing agent confirmed the staged diff contains exactly one atomic feature or bug fix."

	for i, condition := range plan.Matrix.Conditions {
		title := fmt.Sprintf("Verifier cell %d/%d", i+1, len(plan.Matrix.Conditions))
		description := "Feature: " + feature.ID + "\nCondition: " + condition.ID + " (" + condition.Path + ")"
		confirmed, err := charmui.AskConfirm(prompt.In, prompt.Out, charmui.ConfirmPrompt{
			Title:       title,
			Description: description,
			Prompt:      "Did a dedicated read-only verifier subagent check this exact condition for this exact feature?",
			Default:     false,
			Color:       prompt.Color,
		})
		if err != nil {
			return responses, promptError(condition.ID+" subagent confirmation", err)
		}
		if !confirmed {
			message, err := charmui.AskText(prompt.In, prompt.Out, charmui.TextPrompt{
				Title:       title,
				Description: description,
				Prompt:      "Why is the verifier subagent confirmation missing?",
				Placeholder: "No verifier was spawned for ...",
				Required:    true,
				Color:       prompt.Color,
			})
			if err != nil {
				return responses, promptError(condition.ID+" missing-subagent message", err)
			}
			evidenceText, err := charmui.AskText(prompt.In, prompt.Out, charmui.TextPrompt{
				Title:       title,
				Description: description,
				Prompt:      "Evidence notes, comma-separated",
				Placeholder: "blocked report, command output, file path",
				Required:    true,
				Color:       prompt.Color,
			})
			if err != nil {
				return responses, promptError(condition.ID+" missing evidence", err)
			}
			nextAction, err := charmui.AskText(prompt.In, prompt.Out, charmui.TextPrompt{
				Title:       title,
				Description: description,
				Prompt:      "Next action",
				Placeholder: "Spawn verifier for " + condition.ID + " and rerun burpvalve commit",
				Required:    true,
				Color:       prompt.Color,
			})
			if err != nil {
				return responses, promptError(condition.ID+" next action", err)
			}
			responses.Conditions = append(responses.Conditions, ResponseCondition{
				ConditionID:       condition.ID,
				VerifierPolicy:    normalizeConditionPolicy(condition),
				Verifier:          attestations.Verifier{Kind: attestations.VerifierNone},
				SubagentConfirmed: false,
				Verdict:           attestations.VerdictUnknown,
				Message:           message,
				Evidence:          splitList(evidenceText),
				NextAction:        nextAction,
			})
			return responses, fmt.Errorf("missing dedicated subagent confirmation for %s/%s", feature.ID, condition.ID)
		}

		model, err := charmui.AskText(prompt.In, prompt.Out, charmui.TextPrompt{
			Title:       title,
			Description: description,
			Prompt:      "Subagent model/source alias",
			Placeholder: "codex/gpt-5, claude, reviewer-1",
			Color:       prompt.Color,
		})
		if err != nil {
			return responses, promptError(condition.ID+" subagent model", err)
		}
		verdictChoice, err := charmui.AskSelect(prompt.In, prompt.Out, charmui.SelectPrompt{
			Title:       title,
			Description: description,
			Prompt:      "Verifier verdict",
			DefaultID:   string(attestations.VerdictPass),
			Color:       prompt.Color,
			Choices: []charmui.Choice{
				{ID: string(attestations.VerdictPass), Label: "pass", Description: "condition is satisfied"},
				{ID: string(attestations.VerdictNotApplicable), Label: "not_applicable", Description: "condition does not apply; explain why"},
				{ID: string(attestations.VerdictFail), Label: "fail", Description: "condition found a real blocker"},
				{ID: string(attestations.VerdictUnknown), Label: "unknown", Description: "verifier cannot determine pass/fail"},
			},
		})
		if err != nil {
			return responses, promptError(condition.ID+" verdict", err)
		}
		verdict := attestations.Verdict(verdictChoice.ID)
		response := ResponseCondition{
			ConditionID:       condition.ID,
			VerifierPolicy:    normalizeConditionPolicy(condition),
			Verifier:          attestations.Verifier{Kind: attestations.VerifierIndependentSubagent, Model: strings.TrimSpace(model), SeparateContext: true},
			SubagentConfirmed: true,
			SubagentModel:     strings.TrimSpace(model),
			Verdict:           verdict,
		}
		switch verdict {
		case attestations.VerdictPass:
			evidenceText, err := charmui.AskText(prompt.In, prompt.Out, charmui.TextPrompt{
				Title:       title,
				Description: description,
				Prompt:      "Evidence summary, comma-separated",
				Placeholder: "command output, file path, reviewer note",
				Required:    true,
				Color:       prompt.Color,
			})
			if err != nil {
				return responses, promptError(condition.ID+" evidence", err)
			}
			response.Evidence = splitList(evidenceText)
		case attestations.VerdictNotApplicable:
			message, err := charmui.AskText(prompt.In, prompt.Out, charmui.TextPrompt{
				Title:       title,
				Description: description,
				Prompt:      "Why is this condition not applicable?",
				Placeholder: "No data/schema files changed ...",
				Required:    true,
				Color:       prompt.Color,
			})
			if err != nil {
				return responses, promptError(condition.ID+" not-applicable reason", err)
			}
			evidenceText, err := charmui.AskText(prompt.In, prompt.Out, charmui.TextPrompt{
				Title:       title,
				Description: description,
				Prompt:      "Evidence summary, comma-separated",
				Placeholder: "optional evidence",
				Color:       prompt.Color,
			})
			if err != nil {
				return responses, promptError(condition.ID+" evidence", err)
			}
			response.Message = message
			response.Evidence = splitList(evidenceText)
		case attestations.VerdictFail, attestations.VerdictUnknown:
			message, err := charmui.AskText(prompt.In, prompt.Out, charmui.TextPrompt{
				Title:       title,
				Description: description,
				Prompt:      "Blocker message",
				Placeholder: "Verifier found ...",
				Required:    true,
				Color:       prompt.Color,
			})
			if err != nil {
				return responses, promptError(condition.ID+" blocker message", err)
			}
			evidenceText, err := charmui.AskText(prompt.In, prompt.Out, charmui.TextPrompt{
				Title:       title,
				Description: description,
				Prompt:      "Evidence and files/commands involved, comma-separated",
				Placeholder: "file path, command output, reviewer note",
				Required:    true,
				Color:       prompt.Color,
			})
			if err != nil {
				return responses, promptError(condition.ID+" blocker evidence", err)
			}
			nextAction, err := charmui.AskText(prompt.In, prompt.Out, charmui.TextPrompt{
				Title:       title,
				Description: description,
				Prompt:      "Next action",
				Placeholder: "Fix ... and rerun burpvalve commit",
				Required:    true,
				Color:       prompt.Color,
			})
			if err != nil {
				return responses, promptError(condition.ID+" next action", err)
			}
			response.Message = message
			response.Evidence = splitList(evidenceText)
			response.NextAction = nextAction
			responses.Conditions = append(responses.Conditions, response)
			return responses, fmt.Errorf("condition %s returned blocking verdict %q", condition.ID, verdict)
		}
		responses.Conditions = append(responses.Conditions, response)
	}
	return responses, nil
}

func writePromptBanner(out io.Writer, plan Plan, artifactPath string, color bool) {
	feature := promptFeature(plan)
	ui := cliui.New(color)
	fmt.Fprintln(out, ui.Title("Backpressure commit gate"))
	fmt.Fprintln(out, ui.Muted("This commit is blocked until every enabled backpressure condition has a subagent attestation for the staged atomic feature or bug fix."))
	fmt.Fprintf(out, "%s %s\n", ui.Header("Artifact path:"), ui.Path(artifactPath))
	fmt.Fprintln(out, ui.Warn("Fail-closed:")+" missing, failing, unknown, unconfirmed, malformed, or stale attestations block the commit.")
	fmt.Fprintf(out, "%s %s %s\n", ui.Header("Detected feature:"), ui.Info(feature.ID), ui.Muted("("+feature.Name+")"))
	if len(plan.StagedPayloadPaths) > 0 {
		fmt.Fprintln(out, ui.Section("Staged payload"))
		for _, path := range plan.StagedPayloadPaths {
			fmt.Fprintf(out, "  %s %s\n", ui.Muted("-"), ui.Path(path))
		}
	}
}

func WriteResponseSummary(out io.Writer, plan Plan, responses *Responses) {
	WriteResponseSummaryWithOptions(out, plan, responses, TextOptions{})
}

func WriteResponseSummaryWithOptions(out io.Writer, plan Plan, responses *Responses, opts TextOptions) {
	if out == nil {
		return
	}
	ui := cliui.New(opts.Color)
	fmt.Fprintf(out, "\n%s\n", ui.Section("Summary"))
	fmt.Fprintf(out, "%s %s %s\n", ui.Header("Atomicity:"), ui.Bool(responses != nil && responses.Atomicity.OneFeatureOrFix), ui.Muted("- "+atomicityMessage(responses)))
	responseByCondition := map[string]ResponseCondition{}
	if responses != nil {
		for _, response := range responses.Conditions {
			responseByCondition[response.ConditionID] = response
		}
	}
	feature := promptFeature(plan)
	fmt.Fprintf(out, "  %s  %s  %s  %s\n", padStyled(ui.Header("cell"), 28), padStyled(ui.Header("subagent"), 10), padStyled(ui.Header("verdict"), 9), ui.Header("message"))
	fmt.Fprintf(out, "  %s  %s  %s  %s\n", padStyled(ui.Muted("----"), 28), padStyled(ui.Muted("--------"), 10), padStyled(ui.Muted("-------"), 9), ui.Muted("-------"))
	for _, condition := range plan.Matrix.Conditions {
		response, ok := responseByCondition[condition.ID]
		if !ok {
			response = ResponseCondition{
				ConditionID:       condition.ID,
				VerifierPolicy:    normalizeConditionPolicy(condition),
				Verifier:          attestations.Verifier{Kind: attestations.VerifierNone},
				SubagentConfirmed: false,
				Verdict:           attestations.VerdictUnknown,
				Message:           "Missing response for condition " + condition.ID + ".",
			}
		}
		message := strings.TrimSpace(response.Message)
		if message == "" {
			message = "-"
		}
		fmt.Fprintf(out, "  %s  %s  %s  %s\n", padStyled(ui.Path(feature.ID+" x "+condition.ID), 28), padStyled(ui.Bool(response.SubagentConfirmed), 10), padStyled(ui.Status(string(response.Verdict)), 9), message)
	}
	fmt.Fprintln(out, ui.Muted("Artifact step follows this summary."))
}

func promptFeature(plan Plan) Feature {
	if len(plan.Features) == 0 {
		return Feature{ID: "unknown", Kind: "unknown", Name: "unknown"}
	}
	return plan.Features[0]
}

func atomicityMessage(responses *Responses) string {
	if responses == nil || strings.TrimSpace(responses.Atomicity.Message) == "" {
		return "missing atomicity response"
	}
	return responses.Atomicity.Message
}

func askRequired(out io.Writer, scanner *bufio.Scanner, prompt string, color ...bool) (string, error) {
	value, err := askLine(out, scanner, prompt, color...)
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", strings.TrimSuffix(prompt, ": "))
	}
	return value, nil
}

func askRequiredList(out io.Writer, scanner *bufio.Scanner, prompt string, color ...bool) ([]string, error) {
	value, err := askRequired(out, scanner, prompt, color...)
	if err != nil {
		return nil, err
	}
	items := splitList(value)
	if len(items) == 0 {
		return nil, fmt.Errorf("%s requires at least one item", strings.TrimSuffix(prompt, ": "))
	}
	return items, nil
}

func askLine(out io.Writer, scanner *bufio.Scanner, prompt string, color ...bool) (string, error) {
	fmt.Fprint(out, styleLinePrompt(prompt, linePromptColor(color)))
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("prompt input ended before %q", strings.TrimSpace(prompt))
	}
	return scanner.Text(), nil
}

func linePromptColor(color []bool) bool {
	return len(color) > 0 && color[0]
}

func styleLinePrompt(prompt string, color bool) string {
	if !color {
		return prompt
	}
	ui := cliui.New(true)
	if idx := strings.Index(prompt, ":"); idx >= 0 {
		return ui.Header(prompt[:idx+1]) + prompt[idx+1:]
	}
	return ui.Header(prompt)
}

func yes(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "y", "yes", "true":
		return true
	default:
		return false
	}
}

func splitList(value string) []string {
	var items []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}

func promptError(label string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, charmui.ErrCancelled) {
		return fmt.Errorf("%s prompt cancelled", label)
	}
	return err
}
