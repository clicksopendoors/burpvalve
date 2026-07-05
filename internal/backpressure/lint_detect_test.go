package backpressure

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLintDetectSingleGoRootProposesGoCommands(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/project\n")

	detection, err := DetectLintSetup(LintDetectionOptions{
		Root:     root,
		LookPath: foundLintTool,
	})
	if err != nil {
		t.Fatalf("DetectLintSetup returned error: %v", err)
	}
	if !detection.GoAvailable || detection.MultiRoot || detection.NeedsScopedSetup {
		t.Fatalf("single go root detection flags wrong: %#v", detection)
	}
	if got := goRootPaths(detection.GoRoots); !reflect.DeepEqual(got, []string{"."}) {
		t.Fatalf("go roots = %#v", got)
	}
	gotCommands := lintCandidateIDs(detection)
	wantCommands := []string{"go-test", "go-vet", "go-fmt-check"}
	if !reflect.DeepEqual(gotCommands, wantCommands) {
		t.Fatalf("commands = %#v, want %#v", gotCommands, wantCommands)
	}
}

func TestLintDetectSingleNodeRootUsesExactScriptsOnly(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "package.json", `{
    "scripts": {
    "lint": "astro check",
    "test": "vitest",
    "typecheck": "tsc --noEmit",
    "format": "prettier . --check",
    "format:check": "prettier . --check",
    "build": "astro build"
  }
}
`)
	writeFile(t, root, "pnpm-lock.yaml", "lockfileVersion: '9.0'\n")

	detection, err := DetectLintSetup(LintDetectionOptions{Root: root, LookPath: foundLintTool})
	if err != nil {
		t.Fatalf("DetectLintSetup returned error: %v", err)
	}
	if detection.MultiRoot || detection.NeedsScopedSetup || len(detection.GoRoots) != 0 {
		t.Fatalf("single node root detection flags wrong: %#v", detection)
	}
	if len(detection.NodeRoots) != 1 {
		t.Fatalf("node roots = %#v", detection.NodeRoots)
	}
	node := detection.NodeRoots[0]
	if node.Path != "." || node.PackageManager != "pnpm" || node.Lockfile != "pnpm-lock.yaml" {
		t.Fatalf("node root metadata = %#v", node)
	}
	if !reflect.DeepEqual(node.Scripts, []string{"lint", "test", "typecheck", "build", "format:check"}) {
		t.Fatalf("scripts = %#v", node.Scripts)
	}
	commands := lintCandidateCommands(detection)
	if containsString(commands, "pnpm run format") || !containsString(commands, "pnpm run format:check") || !containsString(commands, "pnpm run lint") {
		t.Fatalf("node candidate commands not bounded to exact scripts: %#v", commands)
	}
}

func TestLintDetectGoAndNodePolyglotNeedsScopedSetup(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/project\n")
	writeFile(t, root, "package.json", `{"scripts":{"lint":"eslint ."}}`)

	detection, err := DetectLintSetup(LintDetectionOptions{Root: root, LookPath: foundLintTool})
	if err != nil {
		t.Fatalf("DetectLintSetup returned error: %v", err)
	}
	if !detection.MultiRoot || !detection.NeedsScopedSetup {
		t.Fatalf("polyglot repo should need scoped setup: %#v", detection)
	}
}

func TestLintDetectMultipleNodePackagesNeedScopedSetup(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "apps/web/package.json", `{"scripts":{"check":"astro check"}}`)
	writeFile(t, root, "packages/ui/package.json", `{"scripts":{"test":"vitest"}}`)

	detection, err := DetectLintSetup(LintDetectionOptions{Root: root, LookPath: foundLintTool})
	if err != nil {
		t.Fatalf("DetectLintSetup returned error: %v", err)
	}
	if !detection.MultiRoot || !detection.NeedsScopedSetup {
		t.Fatalf("multi-package node repo should need scoped setup: %#v", detection)
	}
	got := nodeRootPaths(detection.NodeRoots)
	want := []string{"apps/web", "packages/ui"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("node roots = %#v, want %#v", got, want)
	}
}

func TestLintDetectExcludesGeneratedAndVendoredDirs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "node_modules/pkg/package.json", `{"scripts":{"lint":"fake"}}`)
	writeFile(t, root, "vendor/tool/go.mod", "module example.com/vendor\n")
	writeFile(t, root, "dist/package.json", `{"scripts":{"lint":"fake"}}`)
	writeFile(t, root, "docs/demos/generated/site/package.json", `{"scripts":{"lint":"fake"}}`)

	detection, err := DetectLintSetup(LintDetectionOptions{Root: root, LookPath: foundLintTool})
	if err != nil {
		t.Fatalf("DetectLintSetup returned error: %v", err)
	}
	if len(detection.GoRoots) != 0 || len(detection.NodeRoots) != 0 || len(detection.CandidateCommands) != 0 {
		t.Fatalf("excluded roots should not be detected: %#v", detection)
	}
}

func TestLintDetectMissingGoDoesNotProposeGoCommands(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/project\n")

	detection, err := DetectLintSetup(LintDetectionOptions{
		Root:     root,
		LookPath: missingLintTool,
	})
	if err != nil {
		t.Fatalf("DetectLintSetup returned error: %v", err)
	}
	if detection.GoAvailable {
		t.Fatalf("go should be unavailable: %#v", detection)
	}
	if len(detection.GoRoots) != 1 || len(detection.CandidateCommands) != 0 {
		t.Fatalf("missing go should keep facts but not commands: %#v", detection)
	}
}

func TestLintDetectPackageManagerLockfileSelection(t *testing.T) {
	tests := []struct {
		name        string
		lockfile    string
		wantManager string
	}{
		{name: "npm", lockfile: "package-lock.json", wantManager: "npm"},
		{name: "pnpm", lockfile: "pnpm-lock.yaml", wantManager: "pnpm"},
		{name: "yarn", lockfile: "yarn.lock", wantManager: "yarn"},
		{name: "bun", lockfile: "bun.lockb", wantManager: "bun"},
		{name: "default npm", wantManager: "npm"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, "package.json", `{"scripts":{"lint":"astro check"}}`)
			if tt.lockfile != "" {
				writeFile(t, root, tt.lockfile, "\n")
			}

			detection, err := DetectLintSetup(LintDetectionOptions{Root: root, LookPath: foundLintTool})
			if err != nil {
				t.Fatalf("DetectLintSetup returned error: %v", err)
			}
			if got := detection.NodeRoots[0].PackageManager; got != tt.wantManager {
				t.Fatalf("package manager = %q, want %q", got, tt.wantManager)
			}
		})
	}
}

func foundLintTool(name string) (string, error) {
	return filepath.Join(string(os.PathSeparator), "bin", name), nil
}

func missingLintTool(string) (string, error) {
	return "", errors.New("missing")
}

func goRootPaths(roots []LintGoRoot) []string {
	paths := make([]string, len(roots))
	for i, root := range roots {
		paths[i] = root.Path
	}
	return paths
}

func nodeRootPaths(roots []LintNodeRoot) []string {
	paths := make([]string, len(roots))
	for i, root := range roots {
		paths[i] = root.Path
	}
	return paths
}

func lintCandidateIDs(detection LintDetection) []string {
	ids := make([]string, len(detection.CandidateCommands))
	for i, command := range detection.CandidateCommands {
		ids[i] = command.ID
	}
	return ids
}

func lintCandidateCommands(detection LintDetection) []string {
	commands := make([]string, len(detection.CandidateCommands))
	for i, command := range detection.CandidateCommands {
		commands[i] = command.Command
	}
	return commands
}
