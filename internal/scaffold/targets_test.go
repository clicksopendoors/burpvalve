package scaffold

import (
	"reflect"
	"testing"
)

func TestNormalizeScaffoldTargetsIncludesExplicitOrchestratorAliases(t *testing.T) {
	for _, name := range []string{"orchestrator", "ORCHESTRATOR.md", "./orchestrator.md", "orchestratormd"} {
		got, err := NormalizeScaffoldTargets([]string{name})
		if err != nil {
			t.Fatalf("NormalizeScaffoldTargets(%q) returned error: %v", name, err)
		}
		if !reflect.DeepEqual(got, []ScaffoldTarget{TargetOrchestrator}) {
			t.Fatalf("NormalizeScaffoldTargets(%q) = %#v, want orchestrator", name, got)
		}
	}
}

func TestNormalizeScaffoldTargetsKeepsOrchestratorOutOfStandardTargets(t *testing.T) {
	for _, name := range []string{".", "standard", "default"} {
		got, err := NormalizeScaffoldTargets([]string{name})
		if err != nil {
			t.Fatalf("NormalizeScaffoldTargets(%q) returned error: %v", name, err)
		}
		for _, target := range got {
			if target == TargetOrchestrator {
				t.Fatalf("NormalizeScaffoldTargets(%q) included orchestrator in standard set: %#v", name, got)
			}
		}
	}
}

func TestClaudeOrchestratorRouteAliasesTargetClaude(t *testing.T) {
	if ClaudeRouteAgentSymlink != "agent-symlink" ||
		ClaudeRouteOrchestratorSkill != "orchestrator-skill" ||
		ClaudeRouteNone != "none" ||
		ClaudeRepairRoutePreserve != "preserve" ||
		ClaudeOrchestratorSkillDir != ".claude/skills/burpvalve-orchestrator" {
		t.Fatalf("unexpected Claude route constants")
	}
	for _, name := range []string{"claude-orchestrator", "claude-skill", "orchestrator-skill", ".claude/skills/burpvalve-orchestrator"} {
		got, err := NormalizeScaffoldTargets([]string{name})
		if err != nil {
			t.Fatalf("NormalizeScaffoldTargets(%q) returned error: %v", name, err)
		}
		if !reflect.DeepEqual(got, []ScaffoldTarget{TargetClaude}) {
			t.Fatalf("NormalizeScaffoldTargets(%q) = %#v, want claude target", name, got)
		}
	}
}
