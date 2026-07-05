package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookEnvForwardsBeadsAndRationaleForAllHookCopies(t *testing.T) {
	repo := findRepoRoot(t)
	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "template", path: filepath.Join(repo, "templates/githooks/pre-commit")},
		{name: "embedded-template", path: filepath.Join(repo, "internal/scaffold/templates/githooks/pre-commit")},
		{name: "live-hook", path: filepath.Join(repo, ".githooks/pre-commit")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, err := runHookEnvFixture(t, tc.path, map[string]string{
				"BURPVALVE_FEATURE":        "feature-123",
				"BURPVALVE_BEAD":           " br-one,br-two, br-one ",
				"BURPVALVE_BEAD_RATIONALE": "same staged payload",
			})
			if err != nil {
				t.Fatalf("hook failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
			}
			if !strings.Contains(stderr, "BURPVALVE_BEAD_DUPLICATE: ignoring duplicate bead ID 'br-one'") {
				t.Fatalf("duplicate warning missing from stderr:\n%s", stderr)
			}
			want := strings.Join([]string{
				"hook=pre-commit",
				"source=source",
				"run",
				"./cmd/burpvalve",
				"commit",
				"--feature",
				"feature-123",
				"--bead",
				"br-one",
				"--bead",
				"br-two",
				"--bead-rationale",
				"same staged payload",
				"hook=pre-commit",
				"source=source",
				"run",
				"./cmd/burpvalve",
				"lint",
				"",
			}, "\n")
			if stdout != want {
				t.Fatalf("hook argv = %q, want %q", stdout, want)
			}
		})
	}
}

func TestHookEnvRejectsEmptyBeadTokens(t *testing.T) {
	repo := findRepoRoot(t)
	for _, hookPath := range []string{
		filepath.Join(repo, "templates/githooks/pre-commit"),
		filepath.Join(repo, "internal/scaffold/templates/githooks/pre-commit"),
		filepath.Join(repo, ".githooks/pre-commit"),
	} {
		stdout, stderr, err := runHookEnvFixture(t, hookPath, map[string]string{
			"BURPVALVE_BEAD": "br-one,,br-two",
		})
		if err == nil {
			t.Fatalf("%s should reject empty bead tokens\nstdout=%s\nstderr=%s", hookPath, stdout, stderr)
		}
		if !strings.Contains(stderr, "BURPVALVE_BEAD_EMPTY_TOKEN") {
			t.Fatalf("%s missing named empty-token error:\n%s", hookPath, stderr)
		}
		if strings.Contains(stdout, "commit") {
			t.Fatalf("%s should fail before invoking burpvalve commit:\n%s", hookPath, stdout)
		}
	}
}

func TestHookEnvMultipleBeadsWithoutRationaleFallsThroughToCommitValidation(t *testing.T) {
	repo := findRepoRoot(t)
	stdout, stderr, err := runHookEnvFixture(t, filepath.Join(repo, "templates/githooks/pre-commit"), map[string]string{
		"BURPVALVE_BEAD": "br-one,br-two",
		"FAKE_GO_FAIL":   "--bead-rationale is required when multiple --bead values describe one staged payload",
	})
	if err == nil {
		t.Fatal("hook should return the commit validation failure")
	}
	if !strings.Contains(stderr, "--bead-rationale is required") {
		t.Fatalf("missing commit rationale failure:\n%s", stderr)
	}
	if !strings.Contains(stdout, "--bead\nbr-one\n--bead\nbr-two") {
		t.Fatalf("multiple beads were not forwarded as repeated flags:\n%s", stdout)
	}
}

func runHookEnvFixture(t *testing.T, hookSource string, env map[string]string) (string, string, error) {
	t.Helper()
	root := t.TempDir()
	writeCLIFile(t, root, ".githooks/pre-commit", readTestFile(t, hookSource))
	if err := os.Chmod(filepath.Join(root, ".githooks/pre-commit"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "cmd/burpvalve"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeCLIFile(t, root, "go", `#!/usr/bin/env bash
if [[ "$1" == "run" && "$2" == "./cmd/burpvalve" && "$3" == "commit" ]]; then
  printf 'hook=%s\nsource=%s\n' "${BURPVALVE_HOOK_CONTEXT:-}" "${BURPVALVE_HOOK_COMMAND_SOURCE:-}" >> calls.txt
  printf '%s\n' "$@" >> calls.txt
  if [[ -n "${FAKE_GO_FAIL:-}" ]]; then
    echo "$FAKE_GO_FAIL" >&2
    exit 2
  fi
  exit 0
fi
printf 'hook=%s\nsource=%s\n' "${BURPVALVE_HOOK_CONTEXT:-}" "${BURPVALVE_HOOK_COMMAND_SOURCE:-}" >> calls.txt
printf '%s\n' "$@" >> calls.txt
`)
	if err := os.Chmod(filepath.Join(root, "go"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(".githooks/pre-commit")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"))
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	calls, readErr := os.ReadFile(filepath.Join(root, "calls.txt"))
	if readErr == nil {
		stdout.Write(calls)
	}
	return stdout.String(), stderr.String(), err
}
