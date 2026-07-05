package backpressure

import (
	"reflect"
	"testing"

	"burpvalve/internal/lintconfig"
)

func TestLintGoPresetCarriesFullCommandDefinitions(t *testing.T) {
	commands, err := BuiltInLintPresetCommands(LintDetection{
		GoAvailable: true,
		GoRoots:     []LintGoRoot{{Path: "services/api"}},
	}, "go")
	if err != nil {
		t.Fatalf("BuiltInLintPresetCommands returned error: %v", err)
	}
	gotIDs := commandIDs(commands)
	wantIDs := []string{"go-test-services-api", "go-vet-services-api", "go-fmt-check-services-api"}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("ids = %#v, want %#v", gotIDs, wantIDs)
	}
	gotCommands := commandStrings(commands)
	wantCommands := []string{"go test ./...", "go vet ./...", `test -z "$(gofmt -l .)"`}
	if !reflect.DeepEqual(gotCommands, wantCommands) {
		t.Fatalf("commands = %#v, want %#v", gotCommands, wantCommands)
	}
	for _, command := range commands {
		if !command.Required || command.RunDirectory != "services/api" || !reflect.DeepEqual(command.Paths, []string{"services/api"}) || command.TimeoutSeconds <= 0 {
			t.Fatalf("go preset command missing full definition: %#v", command)
		}
	}
}

func TestLintGoPresetDoesNotProposeWhenGoUnavailable(t *testing.T) {
	commands, err := BuiltInLintPresetCommands(LintDetection{
		GoAvailable: false,
		GoRoots:     []LintGoRoot{{Path: "."}},
	}, "go")
	if err != nil {
		t.Fatalf("BuiltInLintPresetCommands returned error: %v", err)
	}
	if len(commands) != 0 {
		t.Fatalf("go unavailable should not propose commands: %#v", commands)
	}
}

func TestLintNodeAndAstroPresetsUseDetectedScriptsAndPackageManager(t *testing.T) {
	detection := LintDetection{
		NodeRoots: []LintNodeRoot{{
			Path:           "apps/web",
			PackageManager: "pnpm",
			Scripts:        []string{"lint", "typecheck", "format:check"},
		}},
	}
	for _, preset := range []string{"node", "astro"} {
		t.Run(preset, func(t *testing.T) {
			commands, err := BuiltInLintPresetCommands(detection, preset)
			if err != nil {
				t.Fatalf("BuiltInLintPresetCommands returned error: %v", err)
			}
			gotIDs := commandIDs(commands)
			wantIDs := []string{"pnpm-lint-apps-web", "pnpm-typecheck-apps-web", "pnpm-format-check-apps-web"}
			if !reflect.DeepEqual(gotIDs, wantIDs) {
				t.Fatalf("ids = %#v, want %#v", gotIDs, wantIDs)
			}
			gotCommands := commandStrings(commands)
			wantCommands := []string{"pnpm run lint", "pnpm run typecheck", "pnpm run format:check"}
			if !reflect.DeepEqual(gotCommands, wantCommands) {
				t.Fatalf("commands = %#v, want %#v", gotCommands, wantCommands)
			}
			for _, command := range commands {
				if command.RunDirectory != "apps/web" || !reflect.DeepEqual(command.Paths, []string{"apps/web"}) || !command.Required || command.TimeoutSeconds != 120 {
					t.Fatalf("node preset command missing full definition: %#v", command)
				}
			}
		})
	}
}

func TestLintAutoPresetCombinesGoAndDetectedNodeScripts(t *testing.T) {
	commands, err := BuiltInLintPresetCommands(LintDetection{
		GoAvailable: true,
		GoRoots:     []LintGoRoot{{Path: "."}},
		NodeRoots: []LintNodeRoot{{
			Path:           "apps/web",
			PackageManager: "npm",
			Scripts:        []string{"test"},
		}},
	}, "auto")
	if err != nil {
		t.Fatalf("BuiltInLintPresetCommands returned error: %v", err)
	}
	gotIDs := commandIDs(commands)
	wantIDs := []string{"go-test", "go-vet", "go-fmt-check", "npm-test-apps-web"}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("ids = %#v, want %#v", gotIDs, wantIDs)
	}
}

func TestLintPresetRejectsUnknownNames(t *testing.T) {
	_, err := BuiltInLintPresetCommands(LintDetection{}, "python")
	if err == nil {
		t.Fatalf("unknown preset should fail")
	}
}

func commandIDs(commands []lintconfig.Command) []string {
	ids := make([]string, len(commands))
	for i, command := range commands {
		ids[i] = command.ID
	}
	return ids
}

func commandStrings(commands []lintconfig.Command) []string {
	values := make([]string, len(commands))
	for i, command := range commands {
		values[i] = command.Command
	}
	return values
}
