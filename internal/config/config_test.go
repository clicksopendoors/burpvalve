package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMergesGlobalAndProjectConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("BURPVALVE_CONFIG", "")
	globalPath, err := GlobalPath()
	if err != nil {
		t.Fatal(err)
	}
	writeConfig(t, globalPath, `{
  "defaults": {
    "shell": "zsh",
    "bin_dir": "~/bin",
    "completion": {"update_rc": true},
    "init": {"beads": false, "repo_bin": false}
  }
}`)
	project := t.TempDir()
	writeConfig(t, filepath.Join(project, ProjectConfigName), `{
  "defaults": {
    "shell": "fish",
    "init": {"repo_bin": true}
  }
}`)

	effective, err := Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if effective.File.Defaults.Shell != "fish" {
		t.Fatalf("shell = %q, want project override fish", effective.File.Defaults.Shell)
	}
	if effective.File.Defaults.BinDir != "~/bin" {
		t.Fatalf("bin_dir = %q, want global value", effective.File.Defaults.BinDir)
	}
	if !BoolValue(effective.File.Defaults.Completion.UpdateRC, false) {
		t.Fatal("completion.update_rc should come from global config")
	}
	if BoolValue(effective.File.Defaults.Init.Beads, true) {
		t.Fatal("init.beads should come from global config")
	}
	if !BoolValue(effective.File.Defaults.Init.RepoBin, false) {
		t.Fatal("init.repo_bin should be overridden by project config")
	}
	if effective.File.SchemaVersion != SchemaVersion {
		t.Fatalf("schema_version = %d, want %d", effective.File.SchemaVersion, SchemaVersion)
	}
	if effective.Sources["defaults.shell"] != "project" {
		t.Fatalf("shell source = %q, want project", effective.Sources["defaults.shell"])
	}
	if effective.Sources["defaults.bin_dir"] != "global" {
		t.Fatalf("bin_dir source = %q, want global", effective.Sources["defaults.bin_dir"])
	}
	if !effective.GlobalFound || !effective.ProjectFound {
		t.Fatalf("found flags not set: %#v", effective)
	}
}

func TestLoadVerifierDefaultsMergeProjectOverrideAndSourceTracking(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("BURPVALVE_CONFIG", "")
	globalPath, err := GlobalPath()
	if err != nil {
		t.Fatal(err)
	}
	writeConfig(t, globalPath, `{
  "defaults": {
    "verifier": {
      "authorized": true,
      "authorized_at": "2026-07-02",
      "authorization_scope": "read-only verifier subagents for backpressure checks",
      "spawn_method": "native",
      "default_model": "sonnet",
      "condition_models": {
        "anti-reward-hacking": "high-reasoning",
        "security-boundaries": "sonnet"
      },
      "read_only_tools": true,
      "max_parallel_verifiers": 4,
      "transcript_dir": "log/backpressure/verifiers",
      "transcripts": "summary"
    }
  }
}`)
	project := t.TempDir()
	writeConfig(t, filepath.Join(project, ProjectConfigName), `{
  "defaults": {
    "verifier": {
      "authorized": false,
      "spawn_method": "ntm",
      "condition_models": {
        "security-boundaries": "high-reasoning"
      },
      "transcript_dir": "log/backpressure/../backpressure/verifier-transcripts",
      "transcripts": "full"
    }
  }
}`)

	effective, err := Load(project)
	if err != nil {
		t.Fatal(err)
	}
	verifier := effective.File.Defaults.Verifier
	if verifier.Authorized == nil || *verifier.Authorized {
		t.Fatalf("project authorized=false should revoke global true: %#v", verifier.Authorized)
	}
	if verifier.AuthorizedAt != "2026-07-02" {
		t.Fatalf("authorized_at = %q, want global value", verifier.AuthorizedAt)
	}
	if verifier.AuthorizationScope != "read-only verifier subagents for backpressure checks" {
		t.Fatalf("authorization_scope = %q", verifier.AuthorizationScope)
	}
	if verifier.SpawnMethod != "ntm" || verifier.DefaultModel != "sonnet" {
		t.Fatalf("verifier merge wrong: %#v", verifier)
	}
	if verifier.ConditionModels["anti-reward-hacking"] != "high-reasoning" {
		t.Fatalf("global condition model missing: %#v", verifier.ConditionModels)
	}
	if verifier.ConditionModels["security-boundaries"] != "high-reasoning" {
		t.Fatalf("project condition model did not override: %#v", verifier.ConditionModels)
	}
	if !BoolValue(verifier.ReadOnlyTools, false) {
		t.Fatal("read_only_tools should come from global config")
	}
	if verifier.MaxParallelVerifiers == nil || *verifier.MaxParallelVerifiers != 4 {
		t.Fatalf("max_parallel_verifiers = %#v, want 4", verifier.MaxParallelVerifiers)
	}
	if verifier.TranscriptDir != "log/backpressure/verifier-transcripts" {
		t.Fatalf("transcript_dir = %q, want cleaned repo-relative path", verifier.TranscriptDir)
	}
	if verifier.Transcripts != "full" {
		t.Fatalf("transcripts = %q, want project override", verifier.Transcripts)
	}
	for key, source := range map[string]string{
		"defaults.verifier.authorized":                           "project",
		"defaults.verifier.authorized_at":                        "global",
		"defaults.verifier.authorization_scope":                  "global",
		"defaults.verifier.spawn_method":                         "project",
		"defaults.verifier.default_model":                        "global",
		"defaults.verifier.condition_models.anti-reward-hacking": "global",
		"defaults.verifier.condition_models.security-boundaries": "project",
		"defaults.verifier.read_only_tools":                      "global",
		"defaults.verifier.max_parallel_verifiers":               "global",
		"defaults.verifier.transcript_dir":                       "project",
		"defaults.verifier.transcripts":                          "project",
	} {
		if effective.Sources[key] != source {
			t.Fatalf("source %s = %q, want %q; all sources=%#v", key, effective.Sources[key], source, effective.Sources)
		}
	}
}

func TestLoadOrchestratorDefaultsMergeProjectOverrideAndSourceTracking(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("BURPVALVE_CONFIG", "")
	globalPath, err := GlobalPath()
	if err != nil {
		t.Fatal(err)
	}
	writeConfig(t, globalPath, `{
  "defaults": {
    "init": {"orchestrator": "off", "dogfood": false},
    "repair": {"orchestrator": false}
  }
}`)
	project := t.TempDir()
	writeConfig(t, filepath.Join(project, ProjectConfigName), `{
  "defaults": {
    "init": {"orchestrator": "orchestrator-md", "dogfood": true},
    "repair": {"orchestrator": true}
  }
}`)

	effective, err := Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if effective.File.Defaults.Init.Orchestrator != "orchestrator-md" {
		t.Fatalf("init orchestrator = %q", effective.File.Defaults.Init.Orchestrator)
	}
	if effective.File.Defaults.Init.Dogfood == nil || !*effective.File.Defaults.Init.Dogfood {
		t.Fatalf("init dogfood = %#v, want true", effective.File.Defaults.Init.Dogfood)
	}
	if effective.File.Defaults.Repair.Orchestrator == nil || !*effective.File.Defaults.Repair.Orchestrator {
		t.Fatalf("repair orchestrator = %#v, want true", effective.File.Defaults.Repair.Orchestrator)
	}
	if effective.Sources["defaults.init.orchestrator"] != "project" {
		t.Fatalf("init orchestrator source = %q; all sources=%#v", effective.Sources["defaults.init.orchestrator"], effective.Sources)
	}
	if effective.Sources["defaults.init.dogfood"] != "project" {
		t.Fatalf("init dogfood source = %q; all sources=%#v", effective.Sources["defaults.init.dogfood"], effective.Sources)
	}
	if effective.Sources["defaults.repair.orchestrator"] != "project" {
		t.Fatalf("repair orchestrator source = %q; all sources=%#v", effective.Sources["defaults.repair.orchestrator"], effective.Sources)
	}
}

func TestLoadClaudeRouteDefaultsMergeProjectOverrideAndSourceTracking(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("BURPVALVE_CONFIG", "")
	globalPath, err := GlobalPath()
	if err != nil {
		t.Fatal(err)
	}
	writeConfig(t, globalPath, `{
  "defaults": {
    "init": {"claude_route": "agent-symlink"},
    "repair": {"claude_route": "preserve"}
  }
}`)
	project := t.TempDir()
	writeConfig(t, filepath.Join(project, ProjectConfigName), `{
  "defaults": {
    "init": {"claude_route": "orchestrator-skill"},
    "repair": {"claude_route": "agent-symlink"}
  }
}`)

	effective, err := Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if effective.File.Defaults.Init.ClaudeRoute != ClaudeRouteOrchestratorSkill {
		t.Fatalf("init claude_route = %q", effective.File.Defaults.Init.ClaudeRoute)
	}
	if effective.File.Defaults.Repair.ClaudeRoute != ClaudeRouteAgentSymlink {
		t.Fatalf("repair claude_route = %q", effective.File.Defaults.Repair.ClaudeRoute)
	}
	if effective.Sources["defaults.init.claude_route"] != "project" {
		t.Fatalf("init claude_route source = %q; all sources=%#v", effective.Sources["defaults.init.claude_route"], effective.Sources)
	}
	if effective.Sources["defaults.repair.claude_route"] != "project" {
		t.Fatalf("repair claude_route source = %q; all sources=%#v", effective.Sources["defaults.repair.claude_route"], effective.Sources)
	}
}

func TestLoadClaudeRouteLegacyBooleanDefaults(t *testing.T) {
	for _, tt := range []struct {
		name       string
		body       string
		wantInit   string
		wantRepair string
	}{
		{
			name:       "legacy false disables claude route",
			body:       `{"defaults":{"init":{"claude":false},"repair":{"claude":false}}}`,
			wantInit:   ClaudeRouteNone,
			wantRepair: ClaudeRouteNone,
		},
		{
			name:       "legacy true preserves agent route",
			body:       `{"defaults":{"init":{"claude":true},"repair":{"claude":true}}}`,
			wantInit:   ClaudeRouteAgentSymlink,
			wantRepair: ClaudeRepairRoutePreserve,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			project := t.TempDir()
			writeConfig(t, filepath.Join(project, ProjectConfigName), tt.body)
			effective, err := Load(project)
			if err != nil {
				t.Fatal(err)
			}
			if effective.File.Defaults.Init.ClaudeRoute != tt.wantInit {
				t.Fatalf("init claude_route = %q, want %q", effective.File.Defaults.Init.ClaudeRoute, tt.wantInit)
			}
			if effective.File.Defaults.Repair.ClaudeRoute != tt.wantRepair {
				t.Fatalf("repair claude_route = %q, want %q", effective.File.Defaults.Repair.ClaudeRoute, tt.wantRepair)
			}
		})
	}
}

func TestLoadAbsentConfigUsesEmptyDefaults(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("BURPVALVE_CONFIG", "")

	effective, err := Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if effective.GlobalFound || effective.ProjectFound {
		t.Fatalf("absent config should not mark files found: %#v", effective)
	}
	if effective.File.SchemaVersion != SchemaVersion {
		t.Fatalf("schema_version = %d", effective.File.SchemaVersion)
	}
	if len(effective.Sources) != 0 {
		t.Fatalf("sources = %#v, want empty", effective.Sources)
	}
}

func TestLoadGlobalOnlyConfig(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("BURPVALVE_CONFIG", "")
	writeConfig(t, filepath.Join(home, ".config", "burpvalve", "config.json"), `{"defaults":{"shell":"zsh"}}`)

	effective, err := Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if !effective.GlobalFound || effective.ProjectFound {
		t.Fatalf("found flags = global %v project %v", effective.GlobalFound, effective.ProjectFound)
	}
	if effective.File.Defaults.Shell != "zsh" || effective.Sources["defaults.shell"] != "global" {
		t.Fatalf("global-only merge wrong: %#v sources=%#v", effective.File.Defaults, effective.Sources)
	}
}

func TestLoadProjectOnlyConfig(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("BURPVALVE_CONFIG", "")
	writeConfig(t, filepath.Join(project, ProjectConfigName), `{"defaults":{"shell":"fish"}}`)

	effective, err := Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if effective.GlobalFound || !effective.ProjectFound {
		t.Fatalf("found flags = global %v project %v", effective.GlobalFound, effective.ProjectFound)
	}
	if effective.File.Defaults.Shell != "fish" || effective.Sources["defaults.shell"] != "project" {
		t.Fatalf("project-only merge wrong: %#v sources=%#v", effective.File.Defaults, effective.Sources)
	}
}

func TestLoadRejectsUnknownConfigKeys(t *testing.T) {
	project := t.TempDir()
	writeConfig(t, filepath.Join(project, ProjectConfigName), `{"defaults":{"init":{"repo_binary":true}}}`)

	_, err := Load(project)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Load should reject unknown keys, got %v", err)
	}
}

func TestLoadRejectsMalformedJSON(t *testing.T) {
	project := t.TempDir()
	writeConfig(t, filepath.Join(project, ProjectConfigName), `{"defaults":`)

	_, err := Load(project)
	if err == nil || !strings.Contains(err.Error(), "parse config") {
		t.Fatalf("Load malformed JSON error = %v", err)
	}
}

func TestLoadAcceptsAndValidatesVersionedConfig(t *testing.T) {
	project := t.TempDir()
	writeConfig(t, filepath.Join(project, ProjectConfigName), `{
  "schema_version": 1,
  "defaults": {
    "shell": "zsh",
    "color": "always",
    "confirm": true,
    "completion": {"path": "~/completions/_burpvalve"}
  }
}`)

	effective, err := Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if effective.File.Defaults.Color != "always" {
		t.Fatalf("color = %q", effective.File.Defaults.Color)
	}
	if !BoolValue(effective.File.Defaults.Confirm, false) {
		t.Fatal("confirm should be true")
	}
	if effective.Sources["defaults.confirm"] != "project" {
		t.Fatalf("confirm source = %q", effective.Sources["defaults.confirm"])
	}
}

func TestLoadRejectsBadSchemaShellColorAndPath(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "schema", body: `{"schema_version":99}`, want: "unsupported schema_version"},
		{name: "shell", body: `{"defaults":{"shell":"tcsh"}}`, want: "defaults.shell"},
		{name: "color", body: `{"defaults":{"color":"loud"}}`, want: "defaults.color"},
		{name: "nul", body: "{\"defaults\":{\"bin_dir\":\"bad\\u0000path\"}}", want: "NUL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := t.TempDir()
			writeConfig(t, filepath.Join(project, ProjectConfigName), tt.body)
			_, err := Load(project)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadRejectsBadOrchestratorDefaults(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "bad init enum", body: `{"defaults":{"init":{"orchestrator":"agents-md"}}}`, want: "defaults.init.orchestrator must be off or orchestrator-md"},
		{name: "migrated claude mode", body: `{"defaults":{"init":{"orchestrator":"claude-md"}}}`, want: "defaults.init.claude_route=orchestrator-skill"},
		{name: "repair wrong type", body: `{"defaults":{"repair":{"orchestrator":"orchestrator-md"}}}`, want: "cannot unmarshal string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := t.TempDir()
			writeConfig(t, filepath.Join(project, ProjectConfigName), tt.body)
			_, err := Load(project)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadRejectsBadClaudeRouteDefaults(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "bad init route enum", body: `{"defaults":{"init":{"claude_route":"claude-md"}}}`, want: "defaults.init.claude_route must be agent-symlink, orchestrator-skill, or none"},
		{name: "bad repair route enum", body: `{"defaults":{"repair":{"claude_route":"replace"}}}`, want: "defaults.repair.claude_route must be preserve, agent-symlink, orchestrator-skill, or none"},
		{name: "init false conflicts with agent route", body: `{"defaults":{"init":{"claude":false,"claude_route":"agent-symlink"}}}`, want: "defaults.init.claude=false conflicts with defaults.init.claude_route=agent-symlink"},
		{name: "init false conflicts with orchestrator route", body: `{"defaults":{"init":{"claude":false,"claude_route":"orchestrator-skill"}}}`, want: "defaults.init.claude=false conflicts with defaults.init.claude_route=orchestrator-skill"},
		{name: "init true conflicts with none", body: `{"defaults":{"init":{"claude":true,"claude_route":"none"}}}`, want: "defaults.init.claude=true conflicts with defaults.init.claude_route=none"},
		{name: "repair false conflicts with preserve", body: `{"defaults":{"repair":{"claude":false,"claude_route":"preserve"}}}`, want: "defaults.repair.claude=false conflicts with defaults.repair.claude_route=preserve"},
		{name: "repair false conflicts with orchestrator route", body: `{"defaults":{"repair":{"claude":false,"claude_route":"orchestrator-skill"}}}`, want: "defaults.repair.claude=false conflicts with defaults.repair.claude_route=orchestrator-skill"},
		{name: "repair true conflicts with none", body: `{"defaults":{"repair":{"claude":true,"claude_route":"none"}}}`, want: "defaults.repair.claude=true conflicts with defaults.repair.claude_route=none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := t.TempDir()
			writeConfig(t, filepath.Join(project, ProjectConfigName), tt.body)
			_, err := Load(project)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadRejectsBadVerifierDefaults(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "spawn method", body: `{"defaults":{"verifier":{"spawn_method":"screen"}}}`, want: "defaults.verifier.spawn_method"},
		{name: "max parallel low", body: `{"defaults":{"verifier":{"max_parallel_verifiers":0}}}`, want: "defaults.verifier.max_parallel_verifiers"},
		{name: "max parallel high", body: `{"defaults":{"verifier":{"max_parallel_verifiers":33}}}`, want: "defaults.verifier.max_parallel_verifiers"},
		{name: "absolute transcript dir", body: `{"defaults":{"verifier":{"transcript_dir":"/example/verifiers"}}}`, want: "defaults.verifier.transcript_dir"},
		{name: "traversal transcript dir", body: `{"defaults":{"verifier":{"transcript_dir":"../verifiers"}}}`, want: "defaults.verifier.transcript_dir"},
		{name: "empty transcript dir after clean", body: `{"defaults":{"verifier":{"transcript_dir":"."}}}`, want: "defaults.verifier.transcript_dir"},
		{name: "bad transcripts", body: `{"defaults":{"verifier":{"transcripts":"verbose"}}}`, want: "defaults.verifier.transcripts"},
		{name: "empty condition model key", body: `{"defaults":{"verifier":{"condition_models":{" ":"sonnet"}}}}`, want: "defaults.verifier.condition_models"},
		{name: "condition model nul", body: "{\"defaults\":{\"verifier\":{\"condition_models\":{\"dry\":\"bad\\u0000model\"}}}}", want: "defaults.verifier.condition_models.dry"},
		{name: "default model nul", body: "{\"defaults\":{\"verifier\":{\"default_model\":\"bad\\u0000model\"}}}", want: "defaults.verifier.default_model"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := t.TempDir()
			writeConfig(t, filepath.Join(project, ProjectConfigName), tt.body)
			_, err := Load(project)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Load error = %v, want success", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestWriteNormalizesAndFormatsConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := Write(path, File{Defaults: Defaults{Shell: "zsh"}}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, needle := range []string{`"schema_version": 1`, `"shell": "zsh"`} {
		if !strings.Contains(text, needle) {
			t.Fatalf("written config missing %q:\n%s", needle, text)
		}
	}
	if !strings.HasSuffix(text, "\n") {
		t.Fatalf("written config missing trailing newline: %q", text)
	}
}

func TestWriteVerifierDefaultsRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := Write(path, File{Defaults: Defaults{Verifier: VerifierDefaults{
		Authorized:           boolPtr(true),
		AuthorizedAt:         "2026-07-02",
		AuthorizationScope:   "read-only verifier subagents for backpressure checks",
		SpawnMethod:          "manual",
		DefaultModel:         "sonnet",
		ConditionModels:      map[string]string{"security-boundaries": "high-reasoning"},
		ReadOnlyTools:        boolPtr(true),
		MaxParallelVerifiers: intPtr(2),
		TranscriptDir:        "log/backpressure/verifiers",
		Transcripts:          "committed",
	}}}); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := readFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("written config should exist")
	}
	verifier := loaded.Defaults.Verifier
	if verifier.Authorized == nil || !*verifier.Authorized || verifier.ReadOnlyTools == nil || !*verifier.ReadOnlyTools {
		t.Fatalf("bool verifier defaults did not round-trip: %#v", verifier)
	}
	if verifier.SpawnMethod != "manual" || verifier.Transcripts != "committed" || verifier.ConditionModels["security-boundaries"] != "high-reasoning" {
		t.Fatalf("verifier defaults did not round-trip: %#v", verifier)
	}
	if verifier.MaxParallelVerifiers == nil || *verifier.MaxParallelVerifiers != 2 {
		t.Fatalf("max_parallel_verifiers did not round-trip: %#v", verifier.MaxParallelVerifiers)
	}
}

func TestLoadRejectsUnknownVerifierConfigKeys(t *testing.T) {
	project := t.TempDir()
	writeConfig(t, filepath.Join(project, ProjectConfigName), `{"defaults":{"verifier":{"model":"sonnet"}}}`)

	_, err := Load(project)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Load should reject unknown verifier keys, got %v", err)
	}
}

func writeConfig(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func intPtr(value int) *int {
	return &value
}
