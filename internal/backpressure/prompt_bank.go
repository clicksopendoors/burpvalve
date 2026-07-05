package backpressure

import (
	"fmt"
	"sort"
	"strings"
)

const PromptBankVersion = "1"

type PromptVariable struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

type PromptDefinition struct {
	Name        string           `json:"name"`
	Version     string           `json:"version"`
	Description string           `json:"description"`
	Variables   []PromptVariable `json:"variables"`
	Body        string           `json:"body"`
}

type PromptListItem struct {
	Name        string           `json:"name"`
	Version     string           `json:"version"`
	Description string           `json:"description"`
	Variables   []PromptVariable `json:"variables,omitempty"`
}

type PromptShowOutput struct {
	Name      string           `json:"name"`
	Version   string           `json:"version"`
	Variables []PromptVariable `json:"variables"`
	Body      string           `json:"body"`
}

var promptBank = []PromptDefinition{
	{
		Name:        "bead-conversion",
		Version:     PromptBankVersion,
		Description: "Convert an approved markdown plan into scoped Beads tracker issues with dependencies and acceptance criteria.",
		Variables: []PromptVariable{
			{Name: "plan", Required: true, Description: "Plan file, section, or pasted plan summary."},
			{Name: "project", Required: false, Description: "Project or repo name for scoping."},
		},
		Body: strings.TrimSpace(`Convert this approved plan into Beads for {{project}}.

Plan source:
{{plan}}

Create one bead per independently reviewable work unit. A bead is a Beads/br tracker issue, not the generic Burpvalve name for all work. Preserve dependencies, acceptance criteria, tests, and non-goals. Use concrete titles, priority, labels, and dependency links. Keep deferred or speculative work as backlog beads instead of mixing it into ready implementation beads. Report the proposed graph before changing tracker state if ownership or ordering is unclear.`),
	},
	{
		Name:        "bead-conversion-assignment",
		Version:     PromptBankVersion,
		Description: "Assign an agent to convert approved planning material into Beads tracker issues without starting implementation.",
		Variables: []PromptVariable{
			{Name: "agent", Required: true, Description: "Assigned agent name."},
			{Name: "plan", Required: true, Description: "Plan file or plan section to convert."},
			{Name: "owner", Required: false, Description: "Coordinator or owner to contact for approval."},
		},
		Body: strings.TrimSpace(`Role: {{agent}}
Assignment: convert approved planning material into Beads.
Plan source: {{plan}}
Owner: {{owner}}

Read AGENTS.md and backpressure/README.md. Do tracker work only; do not implement code for the converted beads. Produce scoped beads, one per independently reviewable work unit, with dependencies, acceptance criteria, tests, and labels. Sync tracker state when finished and report the created bead ids to the owner.`),
	},
	{
		Name:        "commit-choreography",
		Version:     PromptBankVersion,
		Description: "Paste-safe numbered checklist for the Burpvalve commit flow.",
		Variables: []PromptVariable{
			{Name: "bead", Required: true, Description: "Beads issue id, or another work-unit id when the repo is not Beads-backed."},
			{Name: "feature", Required: false, Description: "Explicit Burpvalve feature id when different from the work-unit id."},
		},
		Body: strings.TrimSpace(`Commit choreography for work unit {{bead}}

1. Finish only the atomic payload for this work unit.
2. Run focused checks and record the evidence.
3. Close or update tracker state only when the work-unit contract is satisfied.
4. Run br sync --flush-only when Beads tracker state changed.
5. Stage only the payload, generated seal/attestation, and required tracker export.
6. Run Burpvalve for {{feature}} and resolve every verifier cell with real evidence.
7. Commit after the gate passes.
8. Push exactly once if the project protocol says the commit must be published immediately.

Verify-early variant for shared queues:

1. At park time, compute the staged-payload hash for only this work unit.
2. Mail hash-bound verifier packets and wake verifier panes before taking the gate window.
3. Keep the window free while verifiers work; if the slice changes, send a superseding packet.
4. When your turn arrives, confirm the index is empty, stage only this payload, and confirm the hash still matches the verifier-bound hash.
5. Hold the gate window only for mechanics: run the gate, stage the generated seal/attestation if written, rerun the commit, push once when required, close the work unit, release, and wake the next queued owner.

Paste-safety rule: run one command at a time. Do not paste chained workflows, redirection arrows, or shell-looking prose as commands.`),
	},
	{
		Name:        "gate-operator-brief",
		Version:     PromptBankVersion,
		Description: "Minimal-context brief for an operator running gate-window mechanics without claiming implementation work.",
		Variables: []PromptVariable{
			{Name: "operator", Required: true, Description: "Gate operator agent name."},
			{Name: "feature", Required: true, Description: "Feature id or Beads issue id being gated."},
			{Name: "queue", Required: false, Description: "Current gate queue order and next wake target."},
		},
		Body: strings.TrimSpace(`Gate operator: {{operator}}
Feature: {{feature}}
Queue: {{queue}}

Role scope:
1. Run gate mechanics only. Do not claim implementation work, change product scope, or decide whether a feature is desirable.
2. Do not use CLAUDE.md as the full project contract. Use the operator brief, AGENTS.md, the assigned packet, and explicit owner instructions.
3. Confirm the index is empty before staging. Stage only the named payload or the generated seal/attestation the gate asked for.
4. Run the exact Burpvalve and git commands supplied by the owner or packet. Commit and push only when instructed by the project protocol.
5. Escalate; do not judge. If hashes differ, tests fail, verifier evidence disagrees, scope is unclear, reservations conflict, or unexpected files are staged, stop and ask for a ruling.
6. On release, include the current queue order in the wake so the next owner can self-promote after a step-zero index check.`),
	},
	{
		Name:        "cross-review-polish",
		Version:     PromptBankVersion,
		Description: "Dispatch a focused adversarial review to improve an implementation before gate.",
		Variables: []PromptVariable{
			{Name: "artifact", Required: true, Description: "Commit, diff, file set, or feature under review."},
			{Name: "risk", Required: false, Description: "Known risk area to stress."},
		},
		Body: strings.TrimSpace(`Review target:
{{artifact}}

Primary risk:
{{risk}}

Take a code-review stance. Lead with concrete bugs, regressions, missing tests, or protocol violations. Cite files, commands, and observed behavior. Do not rewrite the implementation unless asked; produce findings, severity, and a minimal fix direction. If no issue is found, say that and name residual test gaps.`),
	},
	{
		Name:        "marching-orders",
		Version:     PromptBankVersion,
		Description: "Start-of-session brief for a coder taking a work unit in a coordinated Burpvalve repo.",
		Variables: []PromptVariable{
			{Name: "agent", Required: true, Description: "Assigned agent name."},
			{Name: "bead", Required: true, Description: "Beads issue id, or another work-unit id when the repo is not Beads-backed."},
			{Name: "track", Required: false, Description: "Optional chain or track name."},
		},
		Body: strings.TrimSpace(`Role: {{agent}}
Work unit: {{bead}}
Track: {{track}}

Read AGENTS.md and backpressure/README.md before editing. Register with Agent Mail before starting work using the repo's absolute path as project key, and include your Agent Mail identity in completion messages. If this is Beads-backed, claim the bead before editing; otherwise follow the repo's work-unit ownership protocol. Reserve the files you will touch, keep the staged payload atomic, collect real verifier evidence, and run the Burpvalve gate, the valve, before committing.`),
	},
	{
		Name:        "orchestrator-tick",
		Version:     PromptBankVersion,
		Description: "Coordinator loop for observing work, assigning ready work units, unblocking queues, and logging decisions.",
		Variables: []PromptVariable{
			{Name: "project", Required: true, Description: "Project key or repo name."},
			{Name: "window", Required: false, Description: "Current gate or coordination window to inspect."},
		},
		Body: strings.TrimSpace(`Orchestrator tick for {{project}}
Window: {{window}}

Observe tracker state, Agent Mail, reservations, git status, and active panes. Assign only ready work units; in Beads-backed repos, assign ready beads. Every dispatch brief or wake must require Agent Mail registration with the repo's absolute path as project key before work starts; a pane that has not registered is not considered dispatched. Unblock queues by asking holders for status, release, or blockers. Verify packets were mailed and terminal-woken before a gate window is held. Record durable decisions in docs or tracker state, not only in chat. Escalate shared-cell verifier disagreements and stale held windows promptly.`),
	},
	{
		Name:        "packet-not-received-status",
		Version:     PromptBankVersion,
		Description: "Status request for missing verifier packets or delayed verdicts while a queue is waiting.",
		Variables: []PromptVariable{
			{Name: "recipient", Required: true, Description: "Agent expected to receive or answer packets."},
			{Name: "feature", Required: true, Description: "Feature id, Beads issue id, or staged payload under verification."},
			{Name: "packet", Required: false, Description: "Agent Mail message id, packet hash, or packet summary."},
		},
		Body: strings.TrimSpace(`WAKE {{recipient}}: verifier packet status needed for {{feature}}.

Expected packet or thread:
{{packet}}

If you have not registered with Agent Mail for this repo, register first using the repo's absolute path as project key and include your Agent Mail identity in the reply. Please reply in-thread with one of these states: packet received and reviewing, verdict posted with message id, packet not visible, or blocked with reason. If a gate window is held, treat this as higher priority than new implementation work until the verdict or blocker is posted.`),
	},
	{
		Name:        "plan-review-packet",
		Version:     PromptBankVersion,
		Description: "Dispatch a plan review with scope, criteria, and evidence expectations.",
		Variables: []PromptVariable{
			{Name: "plan", Required: true, Description: "Plan file, section, or proposal under review."},
			{Name: "criteria", Required: false, Description: "Specific review criteria or risk areas."},
		},
		Body: strings.TrimSpace(`Plan review target:
{{plan}}

Review criteria:
{{criteria}}

Check whether the plan is implementable, scoped, testable, and compatible with current project protocol. Identify missing dependencies, risky coupling, unclear acceptance criteria, and places where Beads tracker issues should be split into smaller work units. Return findings first, then a short recommendation: accept, revise, defer, or reject.`),
	},
	{
		Name:        "verifier-bootstrap",
		Version:     PromptBankVersion,
		Description: "Arrival prompt for a fresh verifier joining a coordinated Burpvalve project.",
		Variables: []PromptVariable{
			{Name: "agent", Required: true, Description: "Verifier agent name."},
			{Name: "project_key", Required: true, Description: "Agent Mail project key or absolute repo path."},
			{Name: "orchestrator", Required: true, Description: "Coordinator or implementer to contact for approval."},
		},
		Body: strings.TrimSpace(`Role: {{agent}}
Project key: {{project_key}}
Coordinator: {{orchestrator}}

Bootstrap steps:
1. Read AGENTS.md and backpressure/README.md before accepting work.
2. Register with Agent Mail before starting work using ensure_project plus register_agent or macro_start_session for the project key, with program, model, and assigned work unit in the task description.
3. Request or confirm contact with the coordinator.
4. Poll your inbox for verifier packets before taking unrelated work.
5. Packet-priority rule: treat verifier packets, reroutes, and held-window blocker messages as priority work.
6. Use the packet's submit command and schema as the verdict contract. If missing, ask for the packet to be resent.
7. Return one verdict per assigned cell: pass, not_applicable, fail, or unknown.
8. Include concrete read-only evidence. Do not fabricate confirmations, tests, pane wakes, or file inspection.
9. Report your Agent Mail identity in completion and verdict messages so it becomes the audit reference.
10. For shared judgment cells, report disagreements instead of smoothing them over.

Verdict schema pointer: run burpvalve verifier prompts --feature <feature-id> --json or inspect the packet's verifier submit command for the exact condition hashes and response fields.`),
	},
	{
		Name:        "verifier-brief",
		Version:     PromptBankVersion,
		Description: "Rules of engagement for a verifier reviewing feature-condition cells.",
		Variables: []PromptVariable{
			{Name: "feature", Required: true, Description: "Stable feature id or Beads issue id being verified."},
			{Name: "cells", Required: false, Description: "Assigned condition cells or review focus."},
		},
		Body: strings.TrimSpace(`Verifier brief for {{feature}}
Assigned cells:
{{cells}}

Work read-only unless explicitly asked for a fix. Inspect the staged payload, condition text, and packet bindings. Verdicts are pass, not_applicable, fail, or unknown. Use pass only when a governed surface was inspected and satisfies the condition. Use not_applicable only when the staged payload has no governed surface for that condition. Include concrete evidence for every verdict. Never claim tests, file reads, subagent checks, pane wakes, or approvals you did not observe.`),
	},
	{
		Name:        "verifier-packet-relay",
		Version:     PromptBankVersion,
		Description: "Relay instructions for forwarding verifier packets to peer panes without losing binding evidence.",
		Variables: []PromptVariable{
			{Name: "verifier", Required: true, Description: "Verifier recipient name."},
			{Name: "packet", Required: true, Description: "Agent Mail message id, packet path, or packet summary."},
			{Name: "pane", Required: false, Description: "Terminal pane or wake target."},
		},
		Body: strings.TrimSpace(`Relay packet to {{verifier}}
Packet: {{packet}}
Pane: {{pane}}

Mail the packet or packet summary first, then wake the verifier terminal. The mail and wake must instruct the verifier to register with Agent Mail before work starts using the repo's absolute path as project key; a pane that has not registered is not considered dispatched. Include feature id, staged payload hash, manifest hash, assigned cells, submit commands, and evidence expectations. After waking, record the wake target and the verifier's Agent Mail identity in-thread. Do not edit packet hashes, omit binding fields, or treat a terminal wake as a verdict.`),
	},
	{
		Name:        "verifier-standby-brief",
		Version:     PromptBankVersion,
		Description: "Prepare verifier-pool agents to wait for packets without taking conflicting work.",
		Variables: []PromptVariable{
			{Name: "agent", Required: true, Description: "Standby verifier name."},
			{Name: "project_key", Required: true, Description: "Agent Mail project key or repo path."},
			{Name: "lane", Required: false, Description: "Judgment, mechanical, or supplemental lane."},
		},
		Body: strings.TrimSpace(`Standby verifier: {{agent}}
Project key: {{project_key}}
Lane: {{lane}}

Read AGENTS.md and backpressure/README.md, register with Agent Mail using ensure_project plus register_agent or macro_start_session for the project key before work starts, and keep your inbox visible. Do not claim implementation beads while on standby. When a packet arrives, acknowledge receipt, verify only your assigned cells, and post verdicts with concrete evidence and your Agent Mail identity. If no packet is visible after a wake, reply with packet-not-visible status instead of waiting silently.`),
	},
}

func ListPromptBank() []PromptListItem {
	items := make([]PromptListItem, 0, len(promptBank))
	for _, prompt := range promptBank {
		items = append(items, PromptListItem{
			Name:        prompt.Name,
			Version:     prompt.Version,
			Description: prompt.Description,
			Variables:   append([]PromptVariable(nil), prompt.Variables...),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items
}

func ShowPrompt(name string, values map[string]string) (PromptShowOutput, error) {
	prompt, ok := FindPrompt(name)
	if !ok {
		return PromptShowOutput{}, fmt.Errorf("unknown prompt %q; valid prompts: %s", name, strings.Join(PromptNames(), ", "))
	}
	body, err := RenderPrompt(prompt, values)
	if err != nil {
		return PromptShowOutput{}, err
	}
	return PromptShowOutput{
		Name:      prompt.Name,
		Version:   prompt.Version,
		Variables: append([]PromptVariable(nil), prompt.Variables...),
		Body:      body,
	}, nil
}

func FindPrompt(name string) (PromptDefinition, bool) {
	for _, prompt := range promptBank {
		if prompt.Name == name {
			return prompt, true
		}
	}
	return PromptDefinition{}, false
}

func PromptNames() []string {
	names := make([]string, 0, len(promptBank))
	for _, prompt := range promptBank {
		names = append(names, prompt.Name)
	}
	sort.Strings(names)
	return names
}

func RenderPrompt(prompt PromptDefinition, values map[string]string) (string, error) {
	known := map[string]PromptVariable{}
	for _, variable := range prompt.Variables {
		known[variable.Name] = variable
	}
	var unknown []string
	for key := range values {
		if _, ok := known[key]; !ok {
			unknown = append(unknown, key)
		}
	}
	sort.Strings(unknown)
	if len(unknown) > 0 {
		return "", fmt.Errorf("unknown variables for prompt %q: %s", prompt.Name, strings.Join(unknown, ", "))
	}
	var missing []string
	for _, variable := range prompt.Variables {
		if !variable.Required {
			continue
		}
		if _, ok := values[variable.Name]; !ok || values[variable.Name] == "" {
			missing = append(missing, variable.Name)
		}
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("missing required variables for prompt %q: %s", prompt.Name, strings.Join(missing, ", "))
	}
	rendered := prompt.Body
	for _, variable := range prompt.Variables {
		rendered = strings.ReplaceAll(rendered, "{{"+variable.Name+"}}", values[variable.Name])
	}
	return rendered, nil
}
