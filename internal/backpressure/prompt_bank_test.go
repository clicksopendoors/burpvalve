package backpressure

import (
	"strings"
	"testing"
)

var requiredPromptNames = []string{
	"bead-conversion",
	"bead-conversion-assignment",
	"commit-choreography",
	"cross-review-polish",
	"gate-operator-brief",
	"marching-orders",
	"orchestrator-tick",
	"packet-not-received-status",
	"plan-review-packet",
	"verifier-bootstrap",
	"verifier-brief",
	"verifier-packet-relay",
	"verifier-standby-brief",
}

func TestPromptBankRenderRequiresAllRequiredVariables(t *testing.T) {
	_, err := ShowPrompt("marching-orders", map[string]string{"agent": "LilacGlacier"})
	if err == nil {
		t.Fatal("missing required variables should fail")
	}
	if !strings.Contains(err.Error(), "bead") || !strings.Contains(err.Error(), "marching-orders") {
		t.Fatalf("missing-variable error should name every missing variable and prompt: %v", err)
	}
}

func TestPromptBankRenderHostileValuesLiterally(t *testing.T) {
	hostile := "br 123 'quoted' `tick` $status && rm -rf /"
	rendered, err := ShowPrompt("marching-orders", map[string]string{
		"agent": "LilacGlacier",
		"bead":  hostile,
		"track": "OXP prompts",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.Body, hostile) {
		t.Fatalf("hostile value was not rendered literally:\n%s", rendered.Body)
	}
	if strings.Contains(rendered.Body, "{{bead}}") || strings.Contains(rendered.Body, "{{agent}}") {
		t.Fatalf("declared variables were not replaced:\n%s", rendered.Body)
	}
}

func TestPromptBankUnknownPromptListsValidNames(t *testing.T) {
	_, err := ShowPrompt("missing", nil)
	if err == nil {
		t.Fatal("unknown prompt should fail")
	}
	if !strings.Contains(err.Error(), "valid prompts:") || !strings.Contains(err.Error(), "marching-orders") || !strings.Contains(err.Error(), "verifier-bootstrap") {
		t.Fatalf("unknown prompt error should list valid names: %v", err)
	}
}

func TestPromptBankListIsStable(t *testing.T) {
	items := ListPromptBank()
	if len(items) != len(requiredPromptNames) || items[0].Name != "bead-conversion" || items[len(items)-1].Name != "verifier-standby-brief" {
		t.Fatalf("unexpected prompt bank list: %#v", items)
	}
	seen := map[string]bool{}
	for _, item := range items {
		if item.Version == "" || item.Description == "" {
			t.Fatalf("prompt list item missing stable metadata: %#v", item)
		}
		if seen[item.Name] {
			t.Fatalf("duplicate prompt name %q", item.Name)
		}
		seen[item.Name] = true
	}
	for _, name := range requiredPromptNames {
		if !seen[name] {
			t.Fatalf("required prompt %q missing from list %#v", name, items)
		}
	}
}

func TestPromptBankUsesLexiconWithoutRenamingContracts(t *testing.T) {
	commit, err := ShowPrompt("commit-choreography", map[string]string{
		"bead":    "burpvalve-lexicon-prompt-bank-skill-wording-r3rz",
		"feature": "burpvalve-lexicon-prompt-bank-skill-wording-r3rz",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"work unit",
		"seal/attestation",
		"Beads tracker state",
		"verifier cell",
		"Verify-early variant",
		"At park time, compute the staged-payload hash",
		"Hold the gate window only for mechanics",
	} {
		if !strings.Contains(commit.Body, needle) {
			t.Fatalf("commit-choreography missing lexicon term %q:\n%s", needle, commit.Body)
		}
	}

	gateOperator, err := ShowPrompt("gate-operator-brief", map[string]string{
		"operator": "Spark",
		"feature":  "burpvalve-cos-throughput-doctrine-jyua",
		"queue":    "EmeraldBrook, then LilacGlacier, then WhiteGorge",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"Run gate mechanics only",
		"Do not claim implementation work",
		"Do not use CLAUDE.md as the full project contract",
		"Stage only the named payload",
		"Escalate; do not judge",
		"hashes differ",
		"current queue order",
	} {
		if !strings.Contains(gateOperator.Body, needle) {
			t.Fatalf("gate-operator-brief missing %q:\n%s", needle, gateOperator.Body)
		}
	}

	marching, err := ShowPrompt("marching-orders", map[string]string{
		"agent": "ScarletOwl",
		"bead":  "burpvalve-lexicon-prompt-bank-skill-wording-r3rz",
		"track": "lexicon",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"Work unit:",
		"If this is Beads-backed, claim the bead",
		"Burpvalve gate, the valve",
	} {
		if !strings.Contains(marching.Body, needle) {
			t.Fatalf("marching-orders missing lexicon term %q:\n%s", needle, marching.Body)
		}
	}

	beadConversion, err := ShowPrompt("bead-conversion", map[string]string{"project": "burpvalve", "plan": "plan text"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(beadConversion.Body, "A bead is a Beads/br tracker issue") || !strings.Contains(beadConversion.Body, "work unit") {
		t.Fatalf("bead-conversion should define Beads-specific bead vocabulary:\n%s", beadConversion.Body)
	}

	for _, prompt := range ListPromptBank() {
		if prompt.Name == "work-unit-conversion" || prompt.Name == "seal-choreography" {
			t.Fatalf("lexicon pass must not rename stable prompt ids: %#v", prompt)
		}
		for _, variable := range prompt.Variables {
			if variable.Name == "work_unit" {
				t.Fatalf("lexicon pass must not rename stable prompt variables: %#v", prompt)
			}
		}
	}
}

func TestVerifierBootstrapWouldOnboardFreshVerifier(t *testing.T) {
	rendered, err := ShowPrompt("verifier-bootstrap", map[string]string{
		"agent":        "WhiteGorge",
		"project_key":  "/path/to/burpvalve",
		"orchestrator": "RusticDog",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"Read AGENTS.md and backpressure/README.md",
		"ensure_project plus register_agent",
		"macro_start_session",
		"assigned work unit",
		"Request or confirm contact",
		"Poll your inbox",
		"priority work",
		"burpvalve verifier prompts --feature <feature-id> --json",
		"pass, not_applicable, fail, or unknown",
		"Agent Mail identity",
		"Do not fabricate confirmations",
	} {
		if !strings.Contains(rendered.Body, needle) {
			t.Fatalf("verifier-bootstrap missing %q:\n%s", needle, rendered.Body)
		}
	}
}

func TestPromptBankDispatchPromptsRequireAgentMailRegistration(t *testing.T) {
	cases := []struct {
		name    string
		values  map[string]string
		needles []string
	}{
		{
			name: "marching-orders",
			values: map[string]string{
				"agent": "ScarletOwl",
				"bead":  "burpvalve-j97h",
				"track": "templates",
			},
			needles: []string{
				"Register with Agent Mail before starting work",
				"repo's absolute path as project key",
				"Agent Mail identity in completion messages",
			},
		},
		{
			name: "orchestrator-tick",
			values: map[string]string{
				"project": "burpvalve",
				"window":  "verifier dispatch",
			},
			needles: []string{
				"Every dispatch brief or wake must require Agent Mail registration",
				"repo's absolute path as project key",
				"not considered dispatched",
			},
		},
		{
			name: "packet-not-received-status",
			values: map[string]string{
				"recipient": "GrayCat",
				"feature":   "burpvalve-j97h",
				"packet":    "message 42",
			},
			needles: []string{
				"register first using the repo's absolute path as project key",
				"include your Agent Mail identity in the reply",
			},
		},
		{
			name: "verifier-packet-relay",
			values: map[string]string{
				"verifier": "CloudyEagle",
				"packet":   "message 43",
				"pane":     "5",
			},
			needles: []string{
				"mail and wake must instruct the verifier to register with Agent Mail",
				"repo's absolute path as project key",
				"not considered dispatched",
				"verifier's Agent Mail identity",
			},
		},
		{
			name: "verifier-standby-brief",
			values: map[string]string{
				"agent":       "GrayCat",
				"project_key": "/path/to/burpvalve",
				"lane":        "judgment",
			},
			needles: []string{
				"ensure_project plus register_agent",
				"macro_start_session",
				"before work starts",
				"your Agent Mail identity",
			},
		},
	}

	for _, tc := range cases {
		rendered, err := ShowPrompt(tc.name, tc.values)
		if err != nil {
			t.Fatalf("render %q: %v", tc.name, err)
		}
		for _, needle := range tc.needles {
			if !strings.Contains(rendered.Body, needle) {
				t.Fatalf("%s missing %q:\n%s", tc.name, needle, rendered.Body)
			}
		}
	}
}

func TestPromptBankPasteSafetyAndHostileRendering(t *testing.T) {
	hostile := "spaces 'quotes' `ticks` $status && rm -rf /"
	for _, name := range requiredPromptNames {
		prompt, ok := FindPrompt(name)
		if !ok {
			t.Fatalf("prompt %q missing", name)
		}
		if strings.Contains(prompt.Body, " -> ") || strings.Contains(prompt.Body, "=>") {
			t.Fatalf("prompt %q contains shell-hazardous arrow text:\n%s", name, prompt.Body)
		}
		if strings.Contains(prompt.Body, "$status") {
			t.Fatalf("prompt %q contains reserved hostile test token in prompt body:\n%s", name, prompt.Body)
		}
		values := map[string]string{}
		for _, variable := range prompt.Variables {
			values[variable.Name] = hostile
		}
		rendered, err := ShowPrompt(name, values)
		if err != nil {
			t.Fatalf("render %q with hostile values: %v", name, err)
		}
		if len(prompt.Variables) > 0 && !strings.Contains(rendered.Body, hostile) {
			t.Fatalf("prompt %q did not render hostile values literally:\n%s", name, rendered.Body)
		}
	}
}
