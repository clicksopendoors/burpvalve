package scaffold

import (
	"fmt"
	"sort"
	"strings"
)

type ScaffoldTarget string

const (
	TargetAgents       ScaffoldTarget = "agents"
	TargetClaude       ScaffoldTarget = "claude"
	TargetOrchestrator ScaffoldTarget = "orchestrator"
	TargetDocs         ScaffoldTarget = "docs"
	TargetPlans        ScaffoldTarget = "plans"
	TargetLog          ScaffoldTarget = "log"
	TargetBackpressure ScaffoldTarget = "backpressure"
	TargetAttestations ScaffoldTarget = "attestations"
	TargetBeads        ScaffoldTarget = "beads"
	TargetNTM          ScaffoldTarget = "ntm"
	TargetPreCommit    ScaffoldTarget = "precommit"
	TargetHooksPath    ScaffoldTarget = "hooks-path"
	TargetTool         ScaffoldTarget = "tool"
	TargetToolDocs     ScaffoldTarget = "tool-docs"
)

const (
	ClaudeRouteAgentSymlink      = "agent-symlink"
	ClaudeRouteOrchestratorSkill = "orchestrator-skill"
	ClaudeRouteNone              = "none"
	ClaudeRepairRoutePreserve    = "preserve"
	ClaudeOrchestratorSkillDir   = ".claude/skills/burpvalve-orchestrator"
)

var standardScaffoldTargets = []ScaffoldTarget{
	TargetAgents,
	TargetClaude,
	TargetDocs,
	TargetPlans,
	TargetLog,
	TargetBackpressure,
	TargetAttestations,
	TargetBeads,
	TargetNTM,
	TargetPreCommit,
	TargetHooksPath,
	TargetToolDocs,
}

var allScaffoldTargets = append(append([]ScaffoldTarget{}, standardScaffoldTargets...), TargetOrchestrator, TargetTool)

var scaffoldTargetAliases = map[string][]ScaffoldTarget{
	"agents":                                {TargetAgents},
	"agent":                                 {TargetAgents},
	"agents.md":                             {TargetAgents},
	"agentsmd":                              {TargetAgents},
	"claude":                                {TargetClaude},
	"claude.md":                             {TargetClaude},
	"claudemd":                              {TargetClaude},
	"claude-orchestrator":                   {TargetClaude},
	"claude-skill":                          {TargetClaude},
	"orchestrator":                          {TargetOrchestrator},
	"orchestrator.md":                       {TargetOrchestrator},
	"orchestratormd":                        {TargetOrchestrator},
	"orchestrator-skill":                    {TargetClaude},
	".claude/skills/burpvalve-orchestrator": {TargetClaude},
	"docs":                                  {TargetDocs},
	"doc":                                   {TargetDocs},
	"plans":                                 {TargetPlans},
	"plan":                                  {TargetPlans},
	"log":                                   {TargetLog},
	"logs":                                  {TargetLog},
	"backpressure":                          {TargetBackpressure},
	"rules":                                 {TargetBackpressure},
	"attestations":                          {TargetAttestations},
	"attestation":                           {TargetAttestations},
	"attestioatns":                          {TargetAttestations},
	"backpressure/attestations":             {TargetAttestations},
	"beads":                                 {TargetBeads},
	"br":                                    {TargetBeads},
	".beads":                                {TargetBeads},
	"ntm":                                   {TargetNTM},
	"precommit":                             {TargetPreCommit},
	"pre-commit":                            {TargetPreCommit},
	".githooks/pre-commit":                  {TargetPreCommit},
	"hooks-path":                            {TargetHooksPath},
	"hookspath":                             {TargetHooksPath},
	"core.hookspath":                        {TargetHooksPath},
	"git-core-hookspath":                    {TargetHooksPath},
	"paths":                                 {TargetHooksPath},
	"path":                                  {TargetHooksPath},
	"tool":                                  {TargetTool},
	"bin":                                   {TargetTool},
	"binary":                                {TargetTool},
	"burpvalve":                             {TargetTool},
	"bin/burpvalve":                         {TargetTool},
	"tool-docs":                             {TargetToolDocs},
	"tools":                                 {TargetToolDocs},
	"tools/burpvalve":                       {TargetToolDocs},
	"tools/burpvalve/readme.md":             {TargetToolDocs},
	"hooks":                                 {TargetPreCommit, TargetHooksPath},
	"git-hooks":                             {TargetPreCommit, TargetHooksPath},
	"githooks":                              {TargetPreCommit, TargetHooksPath},
	"git":                                   {TargetPreCommit, TargetHooksPath},
	"all":                                   allScaffoldTargets,
	"*":                                     allScaffoldTargets,
	".":                                     standardScaffoldTargets,
	"standard":                              standardScaffoldTargets,
	"default":                               standardScaffoldTargets,
}

func NormalizeScaffoldTargets(names []string) ([]ScaffoldTarget, error) {
	if len(names) == 0 {
		return nil, nil
	}
	seen := map[ScaffoldTarget]bool{}
	var targets []ScaffoldTarget
	for _, name := range names {
		key := normalizeTargetName(name)
		matches, ok := scaffoldTargetAliases[key]
		if !ok {
			return nil, fmt.Errorf("unknown target %q; expected one of: %s", name, strings.Join(ScaffoldTargetNames(), ", "))
		}
		for _, target := range matches {
			if !seen[target] {
				seen[target] = true
				targets = append(targets, target)
			}
		}
	}
	return targets, nil
}

func ScaffoldTargetNames() []string {
	names := make([]string, 0, len(scaffoldTargetAliases))
	for name := range scaffoldTargetAliases {
		if name == "*" || name == "." || name == "standard" || name == "default" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func normalizeTargetName(name string) string {
	key := strings.TrimSpace(strings.ToLower(name))
	key = strings.TrimPrefix(key, "./")
	key = strings.Trim(key, "/")
	return key
}

type scaffoldTargetSet map[ScaffoldTarget]bool

func effectiveScaffoldTargets(targets []ScaffoldTarget, skips map[ScaffoldTarget]bool) scaffoldTargetSet {
	selected := map[ScaffoldTarget]bool{}
	if len(targets) == 0 {
		for _, target := range standardScaffoldTargets {
			selected[target] = true
		}
	} else {
		for _, target := range targets {
			selected[target] = true
		}
		if selected[TargetClaude] && !skips[TargetAgents] {
			selected[TargetAgents] = true
		}
	}
	return selected
}

func activeScaffoldTargets(targets []ScaffoldTarget, skips map[ScaffoldTarget]bool) scaffoldTargetSet {
	selected := effectiveScaffoldTargets(targets, skips)
	for target, skip := range skips {
		if skip {
			delete(selected, target)
		}
	}
	return selected
}

func (s scaffoldTargetSet) has(target ScaffoldTarget) bool {
	return s[target]
}

func scaffoldTargetForTemplate(target string) (ScaffoldTarget, bool) {
	switch {
	case target == "AGENTS.md":
		return TargetAgents, true
	case target == "CLAUDE.md":
		return TargetClaude, true
	case target == "ORCHESTRATOR.md":
		return TargetOrchestrator, true
	case strings.HasPrefix(target, ".claude/skills/burpvalve-orchestrator/"):
		return TargetClaude, true
	case strings.HasPrefix(target, "docs/"):
		return TargetDocs, true
	case strings.HasPrefix(target, "plans/"):
		return TargetPlans, true
	case strings.HasPrefix(target, "log/"):
		return TargetLog, true
	case strings.HasPrefix(target, "backpressure/attestations/"):
		return TargetAttestations, true
	case strings.HasPrefix(target, "backpressure/"):
		return TargetBackpressure, true
	case strings.HasPrefix(target, "tools/burpvalve/"):
		return TargetToolDocs, true
	default:
		return "", false
	}
}
