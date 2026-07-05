package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"burpvalve/internal/charmui"
	bvconfig "burpvalve/internal/config"
)

func TestConfigShowJSONIncludesSources(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("BURPVALVE_CONFIG", "")
	writeCmdTestFile(t, filepath.Join(home, ".config", "burpvalve", "config.json"), `{
  "schema_version": 1,
  "defaults": {"shell": "zsh", "bin_dir": "~/bin", "init": {"orchestrator": "off", "claude_route": "agent-symlink", "dogfood": false}, "repair": {"orchestrator": false, "claude_route": "preserve"}, "verifier": {"authorized": true, "authorized_at": "2026-07-02T12:00:00Z", "authorization_scope": "global", "spawn_method": "ntm", "transcripts": "summary"}}
}`)
	writeCmdTestFile(t, filepath.Join(project, ".burpvalve.json"), `{
  "schema_version": 1,
  "defaults": {"shell": "fish", "color": "always", "init": {"orchestrator": "orchestrator-md", "claude_route": "orchestrator-skill", "dogfood": true}, "repair": {"orchestrator": true, "claude_route": "agent-symlink"}, "verifier": {"authorized": false, "authorization_scope": "repo"}}
}`)

	stdout, stderr, err := executeBurpvalveCommand("config", "--target", project, "--json")
	if err != nil {
		t.Fatalf("config --json failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("config --json wrote stderr: %s", stderr)
	}
	var got struct {
		SchemaVersion int  `json:"schema_version"`
		GlobalFound   bool `json:"global_found"`
		ProjectFound  bool `json:"project_found"`
		Defaults      struct {
			Shell  string `json:"shell"`
			BinDir string `json:"bin_dir"`
			Color  string `json:"color"`
			Init   struct {
				Orchestrator string `json:"orchestrator"`
				ClaudeRoute  string `json:"claude_route"`
				Dogfood      *bool  `json:"dogfood"`
			} `json:"init"`
			Repair struct {
				Orchestrator *bool  `json:"orchestrator"`
				ClaudeRoute  string `json:"claude_route"`
			} `json:"repair"`
			Verifier struct {
				Authorized         *bool  `json:"authorized"`
				AuthorizedAt       string `json:"authorized_at"`
				AuthorizationScope string `json:"authorization_scope"`
				SpawnMethod        string `json:"spawn_method"`
				Transcripts        string `json:"transcripts"`
			} `json:"verifier"`
		} `json:"defaults"`
		Sources []struct {
			Key    string `json:"key"`
			Source string `json:"source"`
		} `json:"sources"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode config output: %v\n%s", err, stdout)
	}
	if got.SchemaVersion != 1 || !got.GlobalFound || !got.ProjectFound {
		t.Fatalf("unexpected config identity: %#v", got)
	}
	if got.Defaults.Shell != "fish" || got.Defaults.BinDir != "~/bin" || got.Defaults.Color != "always" {
		t.Fatalf("merged defaults wrong: %#v", got.Defaults)
	}
	if got.Defaults.Init.Orchestrator != "orchestrator-md" || got.Defaults.Init.Dogfood == nil || !*got.Defaults.Init.Dogfood || got.Defaults.Repair.Orchestrator == nil || !*got.Defaults.Repair.Orchestrator {
		t.Fatalf("merged orchestrator defaults wrong: %#v", got.Defaults)
	}
	if got.Defaults.Init.ClaudeRoute != "orchestrator-skill" || got.Defaults.Repair.ClaudeRoute != "agent-symlink" {
		t.Fatalf("merged claude route defaults wrong: %#v", got.Defaults)
	}
	if got.Defaults.Verifier.Authorized == nil || *got.Defaults.Verifier.Authorized || got.Defaults.Verifier.AuthorizationScope != "repo" || got.Defaults.Verifier.SpawnMethod != "ntm" || got.Defaults.Verifier.Transcripts != "summary" {
		t.Fatalf("merged verifier defaults wrong: %#v", got.Defaults.Verifier)
	}
	if sourceFor(got.Sources, "defaults.shell") != "project" || sourceFor(got.Sources, "defaults.bin_dir") != "global" {
		t.Fatalf("sources wrong: %#v", got.Sources)
	}
	if sourceFor(got.Sources, "defaults.verifier.authorized") != "project" || sourceFor(got.Sources, "defaults.verifier.spawn_method") != "global" || sourceFor(got.Sources, "defaults.verifier.transcripts") != "global" {
		t.Fatalf("verifier sources wrong: %#v", got.Sources)
	}
	if sourceFor(got.Sources, "defaults.init.orchestrator") != "project" || sourceFor(got.Sources, "defaults.init.dogfood") != "project" || sourceFor(got.Sources, "defaults.repair.orchestrator") != "project" {
		t.Fatalf("orchestrator sources wrong: %#v", got.Sources)
	}
	if sourceFor(got.Sources, "defaults.init.claude_route") != "project" || sourceFor(got.Sources, "defaults.repair.claude_route") != "project" {
		t.Fatalf("claude route sources wrong: %#v", got.Sources)
	}
}

func TestConfigShowHumanOutputIncludesValuesAndSources(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("BURPVALVE_CONFIG", "")
	writeCmdTestFile(t, filepath.Join(project, ".burpvalve.json"), `{
  "schema_version": 1,
  "defaults": {"shell": "zsh", "color": "never"}
}`)

	stdout, stderr, err := executeBurpvalveCommand("config", "show", "--target", project, "--color", "never")
	if err != nil {
		t.Fatalf("config show failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("config show wrote stderr: %s", stderr)
	}
	for _, needle := range []string{"Burpvalve config", "Global:", "(missing)", "Project:", "(found)", "Effective defaults", `"shell": "zsh"`, "Sources", "defaults.shell", "project"} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("human config output missing %q:\n%s", needle, stdout)
		}
	}
}

func TestConfigInitRequiresConfirmation(t *testing.T) {
	project := t.TempDir()
	seed := filepath.Join(t.TempDir(), "seed.json")
	writeCmdTestFile(t, seed, `{"schema_version":1,"defaults":{"shell":"zsh"}}`)

	stdout, stderr, err := executeBurpvalveCommand("config", "init", "--project", "--target", project, "--file", seed, "--json")
	if err == nil || !strings.Contains(err.Error(), "pass --force") {
		t.Fatalf("config init error = %v, want force requirement\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if _, statErr := os.Stat(filepath.Join(project, ".burpvalve.json")); !os.IsNotExist(statErr) {
		t.Fatalf("config init wrote without force, stat=%v", statErr)
	}
}

func TestConfigInitWithoutFileUsesWizardOnlyInTerminal(t *testing.T) {
	project := t.TempDir()
	stdout, stderr, err := executeBurpvalveCommand("config", "init", "--project", "--target", project, "--json")
	if err == nil || !strings.Contains(err.Error(), "config init requires --file") {
		t.Fatalf("noninteractive config init without --file should fail before writing\nerr=%v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if _, statErr := os.Stat(filepath.Join(project, ".burpvalve.json")); !os.IsNotExist(statErr) {
		t.Fatalf("config init wrote without file or terminal wizard, stat=%v", statErr)
	}
}

func TestConfigInitHelpExplainsGuidedFlow(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("config", "init", "-h")
	if err != nil {
		t.Fatalf("config init help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	for _, needle := range []string{
		"question-and-answer flow",
		"show the final JSON",
		"confirmation defaulting to No",
		"Choose global config",
		"Choose project config",
		".burpvalve.json",
		"Project config overrides global config",
		"does not run setup, init,",
		"omitted fields are preserved",
		"--file",
		"--force",
		"--robots",
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("config init help missing %q:\n%s", needle, stdout)
		}
	}
	if strings.Contains(stdout, "guided question-and-answer config flow is separate") {
		t.Fatalf("config init help still describes old noninteractive-only design:\n%s", stdout)
	}
}

func TestConfigInitSkippedSectionsPreserveExistingValues(t *testing.T) {
	withConfigPromptStubs(t,
		func(io.Reader, io.Writer, charmui.ConfirmPrompt) (bool, error) {
			return false, nil
		},
		nil,
		nil,
	)
	falseValue := false
	trueValue := true
	file := bvconfig.File{
		SchemaVersion: 1,
		Defaults: bvconfig.Defaults{
			SkillsDir: "~/existing-skills",
			BinDir:    "~/.local/bin",
			Shell:     "fish",
			Color:     "never",
			Confirm:   &trueValue,
			Completion: bvconfig.CompletionDefaults{
				Path:     "~/.config/fish/completions/burpvalve.fish",
				UpdateRC: &falseValue,
			},
			Init: bvconfig.InitDefaults{
				ScaffoldDefaults: bvconfig.ScaffoldDefaults{
					Beads:   &falseValue,
					RepoBin: &trueValue,
				},
				Orchestrator: "orchestrator-md",
				ClaudeRoute:  "orchestrator-skill",
				Dogfood:      &trueValue,
			},
			Repair: bvconfig.RepairDefaults{
				ScaffoldDefaults: bvconfig.ScaffoldDefaults{
					Docs:    &falseValue,
					RepoBin: &trueValue,
				},
				Orchestrator: &trueValue,
				ClaudeRoute:  "preserve",
			},
			Verifier: bvconfig.VerifierDefaults{
				Authorized:         &trueValue,
				AuthorizedAt:       "2026-07-02T12:00:00Z",
				AuthorizationScope: "repo:/existing",
				SpawnMethod:        "manual",
				Transcripts:        "summary",
			},
		},
	}
	before := mustJSONString(t, file)
	got, err := askConfigSections(nil, io.Discard, file, bvconfig.Effective{}, false)
	if err != nil {
		t.Fatalf("skipping config sections failed: %v", err)
	}
	after := mustJSONString(t, got)
	if after != before {
		t.Fatalf("skipped sections changed config:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestConfigInitWizardCanRecordVerifierDefaults(t *testing.T) {
	withConfigPromptStubs(t,
		func(_ io.Reader, _ io.Writer, prompt charmui.ConfirmPrompt) (bool, error) {
			switch prompt.Prompt {
			case "Configure verifier defaults?":
				return true, nil
			case "Are agents in this repo authorized to spawn read-only verifier subagents for backpressure checks?":
				return true, nil
			default:
				return false, nil
			}
		},
		func(_ io.Reader, _ io.Writer, prompt charmui.SelectPrompt) (charmui.Choice, error) {
			switch prompt.Prompt {
			case "Verifier spawn method":
				return charmui.Choice{ID: "ntm"}, nil
			case "Verifier transcript storage":
				return charmui.Choice{ID: "summary"}, nil
			default:
				return charmui.Choice{ID: prompt.DefaultID}, nil
			}
		},
		func(_ io.Reader, _ io.Writer, prompt charmui.TextPrompt) (string, error) {
			switch prompt.Prompt {
			case "Authorization timestamp":
				return "2026-07-02T12:00:00Z", nil
			case "Authorization scope":
				return "repo:/example/project", nil
			default:
				return prompt.Default, nil
			}
		},
	)
	got, err := askConfigSections(nil, io.Discard, bvconfig.File{SchemaVersion: 1}, bvconfig.Effective{}, false)
	if err != nil {
		t.Fatalf("verifier config prompts failed: %v", err)
	}
	if got.Defaults.Verifier.Authorized == nil || !*got.Defaults.Verifier.Authorized {
		t.Fatalf("verifier authorization not recorded: %#v", got.Defaults.Verifier)
	}
	if got.Defaults.Verifier.AuthorizedAt != "2026-07-02T12:00:00Z" || got.Defaults.Verifier.AuthorizationScope != "repo:/example/project" || got.Defaults.Verifier.SpawnMethod != "ntm" || got.Defaults.Verifier.Transcripts != "summary" {
		t.Fatalf("verifier prompt defaults wrong: %#v", got.Defaults.Verifier)
	}
}

func TestRecordProjectVerifierAuthorizationWritesProjectConfig(t *testing.T) {
	project := t.TempDir()
	writeCmdTestFile(t, filepath.Join(project, ".burpvalve.json"), `{
  "schema_version": 1,
  "defaults": {"shell": "zsh"}
}`)
	if err := recordProjectVerifierAuthorization(project, false); err != nil {
		t.Fatalf("record verifier authorization failed: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(project, ".burpvalve.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{`"shell": "zsh"`, `"verifier": {`, `"authorized": false`, `"authorization_scope": "repo:` + project, `"spawn_method": "manual"`, `"transcripts": "summary"`} {
		if !strings.Contains(string(body), needle) {
			t.Fatalf("recorded verifier config missing %q:\n%s", needle, body)
		}
	}
}

func TestConfigInitFinalCancelWritesNothing(t *testing.T) {
	project := t.TempDir()
	var sawFinal bool
	withConfigPromptStubs(t,
		func(_ io.Reader, _ io.Writer, prompt charmui.ConfirmPrompt) (bool, error) {
			if prompt.Title == "Confirm Burpvalve config init" {
				sawFinal = true
				if prompt.Default {
					t.Fatalf("final confirmation default = true, want false")
				}
				return false, nil
			}
			return false, nil
		},
		nil,
		nil,
	)
	err := runConfigInitWizard(newRootCommand(), configInitOptions{target: project, project: true})
	if err == nil || !strings.Contains(err.Error(), "config init cancelled") {
		t.Fatalf("final confirmation cancellation should fail safely, got %v", err)
	}
	if !sawFinal {
		t.Fatal("final confirmation prompt was not shown")
	}
	if _, statErr := os.Stat(filepath.Join(project, ".burpvalve.json")); !os.IsNotExist(statErr) {
		t.Fatalf("config init wrote after final cancellation, stat=%v", statErr)
	}
}

func TestConfigInitExistingFileAndPreviewPreserveValues(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, ".burpvalve.json")
	writeCmdTestFile(t, path, `{
  "schema_version": 1,
  "defaults": {
    "shell": "zsh",
    "bin_dir": "~/.local/bin",
    "init": {"beads": false}
  }
}`)
	file, err := readConfigFileIfExists(path)
	if err != nil {
		t.Fatal(err)
	}
	if file.SchemaVersion != 1 || file.Defaults.Shell != "zsh" || file.Defaults.Init.Beads == nil || *file.Defaults.Init.Beads {
		t.Fatalf("existing config was not preserved: %#v", file)
	}
	preview, err := configPreviewJSON(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{`"schema_version": 1`, `"shell": "zsh"`, `"beads": false`} {
		if !strings.Contains(preview, needle) {
			t.Fatalf("preview missing %q:\n%s", needle, preview)
		}
	}
	missing, err := readConfigFileIfExists(filepath.Join(project, "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if missing.SchemaVersion != 1 {
		t.Fatalf("missing config should produce empty schema v1 file: %#v", missing)
	}
}

func TestConfigInitRejectsInvalidExistingFileBeforePreview(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, ".burpvalve.json")
	writeCmdTestFile(t, path, `{
  "schema_version": 1,
  "defaults": {"color": "purple"}
}`)
	if _, err := readConfigFileIfExists(path); err == nil || !strings.Contains(err.Error(), "defaults.color must be auto, always, or never") {
		t.Fatalf("expected validation error for existing config, got %v", err)
	}
}

func TestConfigInitScopeAndWizardGating(t *testing.T) {
	effective := bvconfig.Effective{
		GlobalPath:  "/example/user/.config/burpvalve/config.json",
		ProjectPath: "/repo/.burpvalve.json",
	}
	for _, tt := range []struct {
		name string
		opts configInitOptions
		want string
	}{
		{name: "global", opts: configInitOptions{global: true}, want: "global"},
		{name: "project", opts: configInitOptions{project: true}, want: "project"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := chooseConfigScope(tt.opts, effective)
			if err != nil {
				t.Fatalf("chooseConfigScope failed: %v", err)
			}
			if got != tt.want {
				t.Fatalf("scope = %q, want %q", got, tt.want)
			}
		})
	}
	if _, err := chooseConfigScope(configInitOptions{global: true, project: true}, effective); err == nil || !strings.Contains(err.Error(), "choose exactly one") {
		t.Fatalf("both scopes should fail, got %v", err)
	}
	for _, opts := range []configInitOptions{
		{force: true},
		{jsonOutput: true},
		{file: "seed.json"},
	} {
		if shouldRunConfigInitWizard(opts) {
			t.Fatalf("wizard should be disabled for noninteractive opts: %#v", opts)
		}
	}
}

func TestConfigInitConfirmDescriptionShowsScopePathAndJSON(t *testing.T) {
	description, err := configInitConfirmDescription("project", "/repo/.burpvalve.json", bvconfig.File{
		SchemaVersion: 1,
		Defaults: bvconfig.Defaults{
			Shell: "zsh",
			Init:  bvconfig.InitDefaults{ScaffoldDefaults: bvconfig.ScaffoldDefaults{Beads: boolPtr(false)}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"Scope: project", "Path: /repo/.burpvalve.json", `"schema_version": 1`, `"shell": "zsh"`, `"beads": false`} {
		if !strings.Contains(description, needle) {
			t.Fatalf("config init confirmation description missing %q:\n%s", needle, description)
		}
	}
}

func TestConfigInitWritesGlobalConfigFromFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("BURPVALVE_CONFIG", "")
	seed := filepath.Join(t.TempDir(), "seed.json")
	writeCmdTestFile(t, seed, `{"schema_version":1,"defaults":{"shell":"bash"}}`)

	stdout, stderr, err := executeBurpvalveCommand("config", "init", "--global", "--file", seed, "--force", "--json")
	if err != nil {
		t.Fatalf("global config init failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("global config init wrote stderr: %s", stderr)
	}
	if !strings.Contains(stdout, `"scope": "global"`) {
		t.Fatalf("global config init output missing scope:\n%s", stdout)
	}
	body, err := os.ReadFile(filepath.Join(home, ".config", "burpvalve", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"shell": "bash"`) {
		t.Fatalf("global config not written from file:\n%s", body)
	}
}

func TestConfigInitMergesExistingProjectConfigFromFile(t *testing.T) {
	project := t.TempDir()
	writeCmdTestFile(t, filepath.Join(project, ".burpvalve.json"), `{
  "schema_version": 1,
  "defaults": {
    "shell": "fish",
    "init": {"beads": false}
  }
}`)
	seed := filepath.Join(t.TempDir(), "seed.json")
	writeCmdTestFile(t, seed, `{"schema_version":1,"defaults":{"color":"always"}}`)

	stdout, stderr, err := executeBurpvalveCommand("config", "init", "--project", "--target", project, "--file", seed, "--force", "--json")
	if err != nil {
		t.Fatalf("config init update failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("config init update wrote stderr: %s", stderr)
	}
	body, err := os.ReadFile(filepath.Join(project, ".burpvalve.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{`"shell": "fish"`, `"color": "always"`, `"beads": false`} {
		if !strings.Contains(string(body), needle) {
			t.Fatalf("merged config missing %q:\n%s\nstdout=%s", needle, body, stdout)
		}
	}
}

func TestConfigInitForceRejectsInvalidConfigFile(t *testing.T) {
	project := t.TempDir()
	seed := filepath.Join(t.TempDir(), "seed.json")
	writeCmdTestFile(t, seed, `{"schema_version":1,"defaults":{"shell":"tcsh"}}`)

	stdout, stderr, err := executeBurpvalveCommand("config", "init", "--project", "--target", project, "--file", seed, "--force", "--json")
	if err == nil || !strings.Contains(err.Error(), "defaults.shell must be bash, zsh, fish, or powershell") {
		t.Fatalf("invalid config should fail validation\nerr=%v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if _, statErr := os.Stat(filepath.Join(project, ".burpvalve.json")); !os.IsNotExist(statErr) {
		t.Fatalf("invalid config init wrote file, stat=%v", statErr)
	}
}

func TestRobotsConfigInitRequiresConfirmTrue(t *testing.T) {
	project := t.TempDir()
	input := `{"scope":"project","target":"` + strings.ReplaceAll(project, `\`, `\\`) + `","confirm":false,"config":{"schema_version":1,"defaults":{"shell":"zsh"}}}`

	stdout, stderr, err := executeBurpvalveCommandWithInput(input, "--robots", "config", "init")
	if err == nil || !strings.Contains(err.Error(), "confirm=true") {
		t.Fatalf("robots config init without confirm should fail\nerr=%v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if _, statErr := os.Stat(filepath.Join(project, ".burpvalve.json")); !os.IsNotExist(statErr) {
		t.Fatalf("robots config init wrote without confirm, stat=%v", statErr)
	}
}

func TestConfigInitHumanSummaryAfterWrite(t *testing.T) {
	project := t.TempDir()
	seed := filepath.Join(t.TempDir(), "seed.json")
	writeCmdTestFile(t, seed, `{"schema_version":1,"defaults":{"shell":"zsh"}}`)

	stdout, stderr, err := executeBurpvalveCommand("config", "init", "--project", "--target", project, "--file", seed, "--force", "--color", "never")
	if err != nil {
		t.Fatalf("config init human output failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("config init human output wrote stderr: %s", stderr)
	}
	for _, needle := range []string{"Config written", "Scope: project", "Path: " + filepath.Join(project, ".burpvalve.json")} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("human summary missing %q:\n%s", needle, stdout)
		}
	}
}

func TestConfigInitWritesProjectConfigFromFile(t *testing.T) {
	project := t.TempDir()
	seed := filepath.Join(t.TempDir(), "seed.json")
	writeCmdTestFile(t, seed, `{"defaults":{"shell":"zsh","color":"never"}}`)

	stdout, stderr, err := executeBurpvalveCommand("config", "init", "--project", "--target", project, "--file", seed, "--force", "--json")
	if err != nil {
		t.Fatalf("config init failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("config init wrote stderr: %s", stderr)
	}
	var got struct {
		Status string `json:"status"`
		Scope  string `json:"scope"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode config init output: %v\n%s", err, stdout)
	}
	if got.Status != "written" || got.Scope != "project" || got.Path != filepath.Join(project, ".burpvalve.json") {
		t.Fatalf("unexpected init result: %#v", got)
	}
	body, err := os.ReadFile(filepath.Join(project, ".burpvalve.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{`"schema_version": 1`, `"shell": "zsh"`, `"color": "never"`} {
		if !strings.Contains(string(body), needle) {
			t.Fatalf("written project config missing %q:\n%s", needle, body)
		}
	}
}

func TestRobotsConfigInitWritesGlobalConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("BURPVALVE_CONFIG", "")
	input := `{"scope":"global","confirm":true,"config":{"schema_version":1,"defaults":{"shell":"fish"}}}`

	stdout, stderr, err := executeBurpvalveCommandWithInput(input, "--robots", "config", "init")
	if err != nil {
		t.Fatalf("robots config init failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("robots config init wrote stderr: %s", stderr)
	}
	if !strings.Contains(stdout, `"scope": "global"`) {
		t.Fatalf("robots output missing global scope:\n%s", stdout)
	}
	body, err := os.ReadFile(filepath.Join(home, ".config", "burpvalve", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"shell": "fish"`) {
		t.Fatalf("global config not written:\n%s", body)
	}
}

func TestRobotsConfigInitCanRevokeVerifierAuthorization(t *testing.T) {
	project := t.TempDir()
	writeCmdTestFile(t, filepath.Join(project, ".burpvalve.json"), `{
  "schema_version": 1,
  "defaults": {
    "verifier": {
      "authorized": true,
      "authorized_at": "2026-07-02T12:00:00Z",
      "authorization_scope": "repo:/old",
      "spawn_method": "ntm",
      "transcripts": "full"
    }
  }
}`)
	input := `{"scope":"project","target":"` + strings.ReplaceAll(project, `\`, `\\`) + `","confirm":true,"config":{"schema_version":1,"defaults":{"verifier":{"authorized":false,"authorized_at":"2026-07-02T13:00:00Z","authorization_scope":"repo:/new","spawn_method":"manual","transcripts":"summary"}}}}`

	stdout, stderr, err := executeBurpvalveCommandWithInput(input, "--robots", "config", "init")
	if err != nil {
		t.Fatalf("robots verifier revocation failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("robots verifier revocation wrote stderr: %s", stderr)
	}
	body, err := os.ReadFile(filepath.Join(project, ".burpvalve.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{`"authorized": false`, `"authorized_at": "2026-07-02T13:00:00Z"`, `"authorization_scope": "repo:/new"`, `"spawn_method": "manual"`, `"transcripts": "summary"`} {
		if !strings.Contains(string(body), needle) {
			t.Fatalf("verifier revocation missing %q:\n%s\nstdout=%s", needle, body, stdout)
		}
	}
}

func TestRobotsConfigHelpDocumentsSchemas(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("--robots", "config", "init", "-h")
	if err != nil {
		t.Fatalf("robots config init help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("robots config init help wrote stderr: %s", stderr)
	}
	var doc struct {
		RobotInput map[string]any `json:"robot_input"`
		Notes      []string       `json:"notes"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("decode robots config init help: %v\n%s", err, stdout)
	}
	stdin, ok := doc.RobotInput["stdin_json"].(map[string]any)
	if !ok {
		t.Fatalf("missing stdin_json robot schema: %#v", doc.RobotInput)
	}
	if _, ok := stdin["config"]; !ok {
		t.Fatalf("robot schema missing config field: %#v", stdin)
	}
	for _, needle := range []string{
		`"init"`,
		`"repair"`,
		`"orchestrator"`,
		`"claude_route"`,
		`"dogfood"`,
		"controls ORCHESTRATOR.md only, not the Claude route",
		"controls CLAUDE.md/.claude/skills route choice",
		"repair touches ORCHESTRATOR.md only when true or when explicitly targeted",
		"controls Claude route repair defaults",
	} {
		if !strings.Contains(stdout, needle) {
			t.Fatalf("config init robot help missing %q:\n%s", needle, stdout)
		}
	}
	if !containsString(doc.Notes, "config JSON is validated before writing and unknown fields are rejected") {
		t.Fatalf("config init robot help has wrong notes: %#v", doc.Notes)
	}
	if !containsString(doc.Notes, "defaults.init.orchestrator accepts off or orchestrator-md and only controls ORCHESTRATOR.md") {
		t.Fatalf("config init robot help missing orchestrator enum note: %#v", doc.Notes)
	}
	if !containsString(doc.Notes, "defaults.init.claude_route accepts agent-symlink, orchestrator-skill, or none and controls the Claude route separately from ORCHESTRATOR.md") {
		t.Fatalf("config init robot help missing init claude route note: %#v", doc.Notes)
	}
	if !containsString(doc.Notes, "defaults.init.dogfood is a boolean and only adds findings-log instructions to the orchestrator contract") {
		t.Fatalf("config init robot help missing dogfood note: %#v", doc.Notes)
	}
	if !containsString(doc.Notes, "defaults.repair.orchestrator is a boolean and only controls ORCHESTRATOR.md repair defaults") {
		t.Fatalf("config init robot help missing repair orchestrator note: %#v", doc.Notes)
	}
	if !containsString(doc.Notes, "defaults.repair.claude_route accepts preserve, agent-symlink, orchestrator-skill, or none and controls Claude route repair defaults") {
		t.Fatalf("config init robot help missing repair claude route note: %#v", doc.Notes)
	}
	if !containsString(doc.Notes, "defaults.verifier.authorization_scope is policy metadata and is never verifier evidence") {
		t.Fatalf("config init robot help missing verifier evidence note: %#v", doc.Notes)
	}
	if containsString(doc.Notes, "skip fields match the --no-* flags and can be combined with CLI flags") {
		t.Fatalf("config init robot help leaked scaffold init notes: %#v", doc.Notes)
	}
}

func sourceFor(sources []struct {
	Key    string `json:"key"`
	Source string `json:"source"`
}, key string) string {
	for _, source := range sources {
		if source.Key == key {
			return source.Source
		}
	}
	return ""
}

func writeCmdTestFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func withConfigPromptStubs(
	t *testing.T,
	confirm func(io.Reader, io.Writer, charmui.ConfirmPrompt) (bool, error),
	selectFn func(io.Reader, io.Writer, charmui.SelectPrompt) (charmui.Choice, error),
	textFn func(io.Reader, io.Writer, charmui.TextPrompt) (string, error),
) {
	t.Helper()
	previousConfirm := configAskConfirm
	previousSelect := configAskSelect
	previousText := configAskText
	if confirm != nil {
		configAskConfirm = confirm
	}
	if selectFn != nil {
		configAskSelect = selectFn
	}
	if textFn != nil {
		configAskText = textFn
	}
	t.Cleanup(func() {
		configAskConfirm = previousConfirm
		configAskSelect = previousSelect
		configAskText = previousText
	})
}
