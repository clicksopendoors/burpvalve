package scaffold

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeRunner struct {
	mu      sync.Mutex
	results map[string]CommandResult
	errors  map[string]error
	calls   []string
}

func (f *fakeRunner) Run(_ context.Context, _ string, name string, args ...string) (CommandResult, error) {
	key := name + " " + strings.Join(args, " ")
	f.mu.Lock()
	f.calls = append(f.calls, key)
	f.mu.Unlock()
	if strings.HasPrefix(key, "ntm quick ") {
		return CommandResult{Stdout: "quick ok"}, nil
	}
	if strings.HasPrefix(key, "git check-ignore ") {
		if result, ok := f.results[key]; ok {
			return result, f.errors[key]
		}
		return CommandResult{ExitCode: 1}, errors.New("exit status 1")
	}
	return f.results[key], f.errors[key]
}

func (f *fakeRunner) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

type fakeLooker map[string]bool

func (f fakeLooker) LookPath(file string) (string, error) {
	if f[file] {
		return "/fake/bin/" + file, nil
	}
	return "", os.ErrNotExist
}

type blockingRunner struct {
	fakeRunner
	mu       sync.Mutex
	watched  map[string]bool
	started  map[string]bool
	released chan struct{}
	once     sync.Once
}

func newBlockingRunner(watched []string, results map[string]CommandResult, errors map[string]error) *blockingRunner {
	runner := &blockingRunner{
		fakeRunner: fakeRunner{
			results: results,
			errors:  errors,
		},
		watched:  map[string]bool{},
		started:  map[string]bool{},
		released: make(chan struct{}),
	}
	for _, key := range watched {
		runner.watched[key] = true
	}
	return runner
}

func (b *blockingRunner) Run(ctx context.Context, dir string, name string, args ...string) (CommandResult, error) {
	key := name + " " + strings.Join(args, " ")
	if b.watched[key] {
		b.mu.Lock()
		b.started[key] = true
		if len(b.started) == len(b.watched) {
			b.once.Do(func() { close(b.released) })
		}
		b.mu.Unlock()
		select {
		case <-b.released:
		case <-time.After(2 * time.Second):
			return CommandResult{ExitCode: -1}, errors.New("timed out waiting for concurrent inspect probe")
		}
	}
	return b.fakeRunner.Run(ctx, dir, name, args...)
}

func TestInspectMissingFixtureIsNonMutating(t *testing.T) {
	root := t.TempDir()
	before := snapshotFiles(t, root)
	report, err := Inspect(root, InspectOptions{
		Runner: fakeRunnerPtr(map[string]CommandResult{}, map[string]error{}),
		Looker: fakeLooker{},
		Now:    fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	after := snapshotFiles(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("Inspect mutated fixture\nbefore=%v\nafter=%v", before, after)
	}
	if report.Mutating {
		t.Fatal("check report claims mutation")
	}
	if len(report.PlannedChanges) == 0 {
		t.Fatal("missing fixture should include planned changes")
	}
	assertCheck(t, report, "agents", StatusMissing)
	assertCheck(t, report, "claude", StatusMissing)
	assertCheck(t, report, "docs", StatusMissing)
	assertCheck(t, report, "beads", StatusMissing)
	assertCheck(t, report, "br-tool", StatusUnavailable)
	assertCheck(t, report, "ntm", StatusUnavailable)
	if report.Summary.Ready {
		t.Fatal("empty fixture should not be ready")
	}
	if report.Status != "blocked" || !report.Fatal {
		t.Fatalf("missing fixture should expose blocked/fatal status: %#v", report)
	}
	for _, tt := range []struct {
		id      string
		command string
		fatal   bool
	}{
		{id: "git-repo", command: "git init", fatal: true},
		{id: "git-hooks-path", command: "git init && burpvalve repair --force --json hooks-path", fatal: true},
		{id: "backpressure-attestations", command: "burpvalve repair --force --json attestations", fatal: true},
		{id: "br-tool", command: "install br, then run burpvalve repair --force --json beads", fatal: true},
		{id: "backpressure-tool", command: "install-burpvalve, or run burpvalve repair --force --json bin/burpvalve", fatal: true},
		{id: "ntm", command: "install ntm, or run burpvalve init --force --json --no-ntm", fatal: false},
	} {
		if !hasRecoveryStep(report, tt.id, tt.command, tt.fatal) {
			t.Fatalf("missing recovery step %#v in %#v", tt, report.NextSteps)
		}
	}
	text := report.Text()
	for _, needle := range []string{
		"next steps",
		"git init",
		"git init && burpvalve repair --force --json hooks-path",
		"burpvalve repair --force --json attestations",
		"install-burpvalve, or run burpvalve repair --force --json bin/burpvalve",
		"install ntm, or run burpvalve init --force --json --no-ntm",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("setup text missing recovery command %q:\n%s", needle, text)
		}
	}
	body, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		`"command_path":""`,
		`"repo_bin_path":""`,
		`"hook_command_source":"missing"`,
		`"next_steps":[`,
	} {
		if !strings.Contains(string(body), needle) {
			t.Fatalf("setup JSON missing stable field %q:\n%s", needle, body)
		}
	}
}

func TestInspectPartialFixture(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "AGENTS.md", "# Agent Operating Contract\n")
	writeFile(t, root, "docs/README.md", "# Docs\n")
	writeFile(t, root, "backpressure/README.md", "# Backpressure\n")
	if err := os.Symlink("OTHER.md", filepath.Join(root, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}

	report, err := Inspect(root, InspectOptions{
		Runner: fakeRunnerPtr(map[string]CommandResult{}, map[string]error{}),
		Looker: fakeLooker{},
		Now:    fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertCheck(t, report, "agents", StatusPresent)
	assertCheck(t, report, "docs", StatusPresent)
	assertCheck(t, report, "backpressure", StatusPresent)
	assertCheck(t, report, "claude", StatusConflict)
	if !hasPlannedChange(report, "claude", "manual review before repair") {
		t.Fatalf("CLAUDE.md conflict should be present in planned changes: %#v", report.PlannedChanges)
	}
	assertCheck(t, report, "plans", StatusMissing)
	assertCheck(t, report, "log", StatusMissing)
}

func TestInspectCompleteFixtureWithReadOnlyCommands(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{
		"AGENTS.md",
		"docs/README.md",
		"plans/README.md",
		"log/README.md",
		"backpressure/README.md",
		"backpressure/attestations/README.md",
		".beads/config.yaml",
		"tools/burpvalve/README.md",
		".git/HEAD",
	} {
		writeFile(t, root, rel, "ok\n")
	}
	writeExecutable(t, root, "bin/burpvalve", "#!/usr/bin/env bash\n")
	writeExecutable(t, root, ".githooks/pre-commit", "#!/usr/bin/env bash\n")
	if err := os.Symlink("AGENTS.md", filepath.Join(root, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}
	runner := fakeRunnerPtr(map[string]CommandResult{
		"git status --short":              {Stdout: ""},
		"git config --get core.hooksPath": {Stdout: ".githooks\n"},
		"ntm --robot-capabilities":        {Stdout: "capabilities ok\nmore"},
	}, map[string]error{})
	report, err := Inspect(root, InspectOptions{
		Runner: runner,
		Looker: fakeLooker{"br": true, "git": true, "ntm": true, "burpvalve": true},
		Now:    fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertCheck(t, report, "agents", StatusPresent)
	assertCheck(t, report, "claude", StatusPresent)
	assertCheck(t, report, "git-repo", StatusPresent)
	assertCheck(t, report, "git-dirty", StatusClean)
	assertCheck(t, report, "githook", StatusPresent)
	assertCheck(t, report, "git-hooks-path", StatusOK)
	assertCheck(t, report, "backpressure-tool", StatusOK)
	assertCheck(t, report, "br-tool", StatusOK)
	assertCheck(t, report, "ntm", StatusOK)
	if !report.Summary.Ready {
		t.Fatalf("complete fixture should be ready: %#v", report.Summary)
	}
	if report.Status != "ready" || report.Fatal || len(report.NextSteps) != 0 {
		t.Fatalf("ready fixture should not expose recovery steps: %#v", report)
	}
	if report.ReadinessSeverity != "ready" || report.CommandPath != "/fake/bin/burpvalve" || report.RepoBinPath != "bin/burpvalve" || report.HookCommandSource != "path" {
		t.Fatalf("ready fixture readiness facts wrong: severity=%q command=%q repo_bin=%q hook=%q", report.ReadinessSeverity, report.CommandPath, report.RepoBinPath, report.HookCommandSource)
	}
	if got, want := sortedStrings(runner.Calls()), []string{"git check-ignore -v -- bin/burpvalve", "git config --get core.hooksPath", "git status --short", "ntm --robot-capabilities"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("runner calls = %#v, want set %#v", got, want)
	}
}

func TestInspectRunsIndependentSetupProbesConcurrently(t *testing.T) {
	root := t.TempDir()
	writeReadyInspectFixture(t, root)
	writeExecutable(t, root, "bin/burpvalve", "#!/usr/bin/env bash\n")
	runner := newBlockingRunner([]string{
		"git config --get core.hooksPath",
		"git check-ignore -v -- bin/burpvalve",
	}, map[string]CommandResult{
		"git status --short":              {Stdout: ""},
		"git config --get core.hooksPath": {Stdout: ".githooks\n"},
		"git check-ignore -v -- bin/burpvalve": {
			ExitCode: 1,
		},
		"ntm --robot-capabilities": {Stdout: "capabilities ok\nmore"},
	}, map[string]error{
		"git check-ignore -v -- bin/burpvalve": errors.New("exit status 1"),
	})

	report, err := Inspect(root, InspectOptions{
		Runner: runner,
		Looker: fakeLooker{"br": true, "git": true, "ntm": true, "burpvalve": true},
		Now:    fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct {
		id     string
		status CheckStatus
	}{
		{id: "git-hooks-path", status: StatusOK},
		{id: "backpressure-tool", status: StatusOK},
		{id: "backpressure-tool-fallback", status: StatusPresent},
		{id: "git-repo", status: StatusPresent},
		{id: "git-dirty", status: StatusClean},
		{id: "ntm", status: StatusOK},
	} {
		assertCheck(t, report, want.id, want.status)
	}
	if got, want := checkIDs(report), []string{
		"agents",
		"claude",
		"docs",
		"plans",
		"log",
		"backpressure",
		"backpressure-attestations",
		"beads",
		"br-tool",
		"githook",
		"git-hooks-path",
		"backpressure-tool",
		"backpressure-tool-fallback",
		"git-repo",
		"git-dirty",
		"ntm",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("check order changed under concurrent probes:\ngot  %#v\nwant %#v", got, want)
	}
}

func TestInspectReportsPathToolAndRepoLocalFallbackSeparately(t *testing.T) {
	root := t.TempDir()
	writeReadyInspectFixture(t, root)
	writeExecutable(t, root, "bin/burpvalve", "#!/usr/bin/env bash\n")
	runner := fakeRunnerPtr(map[string]CommandResult{
		"git status --short":              {Stdout: ""},
		"git config --get core.hooksPath": {Stdout: ".githooks\n"},
	}, map[string]error{})

	report, err := Inspect(root, InspectOptions{
		Runner: runner,
		Looker: fakeLooker{"br": true, "git": true, "burpvalve": true},
		Now:    fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	tool := findRequiredCheck(t, report, "backpressure-tool")
	if tool.Status != StatusOK || tool.Path != "/fake/bin/burpvalve" {
		t.Fatalf("global tool check = %#v", tool)
	}
	fallback := findRequiredCheck(t, report, "backpressure-tool-fallback")
	if fallback.Required || fallback.Status != StatusPresent || fallback.Path != "bin/burpvalve" {
		t.Fatalf("repo-local fallback check = %#v", fallback)
	}
	if !report.Summary.Ready {
		t.Fatalf("optional repo-local fallback should not block readiness: %#v", report.Summary)
	}
	if report.CommandPath != "/fake/bin/burpvalve" || report.RepoBinPath != "bin/burpvalve" || report.HookCommandSource != "path" {
		t.Fatalf("PATH and repo-local facts not separated: command=%q repo_bin=%q hook=%q", report.CommandPath, report.RepoBinPath, report.HookCommandSource)
	}
	if report.RepoLocalBinary == nil || report.RepoLocalBinary.FreshnessStatus != "not_applicable" || report.RepoLocalBinary.WarningCode != "" {
		t.Fatalf("PATH-owned hook should report repo-local facts without active warning: %#v", report.RepoLocalBinary)
	}
}

func TestInspectWarnsWhenRepoLocalFallbackIsIgnored(t *testing.T) {
	root := t.TempDir()
	writeReadyInspectFixture(t, root)
	writeExecutable(t, root, "bin/burpvalve", "#!/usr/bin/env bash\n")
	runner := fakeRunnerPtr(map[string]CommandResult{
		"git status --short":              {Stdout: ""},
		"git config --get core.hooksPath": {Stdout: ".githooks\n"},
		"git check-ignore -v -- bin/burpvalve": {
			Stdout: ".gitignore:1:bin/\tbin/burpvalve\n",
		},
	}, map[string]error{})

	report, err := Inspect(root, InspectOptions{
		Runner: runner,
		Looker: fakeLooker{"br": true, "git": true, "burpvalve": true},
		Now:    fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertCheck(t, report, "backpressure-tool", StatusOK)
	fallback := findRequiredCheck(t, report, "backpressure-tool-fallback")
	if fallback.Required || fallback.Status != StatusConflict || !strings.Contains(fallback.Message, "ignored by git") {
		t.Fatalf("repo-local ignored fallback check = %#v", fallback)
	}
	if !report.Summary.Ready {
		t.Fatalf("ignored optional fallback should warn without blocking when PATH tool exists: %#v", report.Summary)
	}
	if report.RepoLocalBinary == nil || !report.RepoLocalBinary.RepoLocalIgnored || report.RepoLocalBinary.FreshnessStatus != "not_applicable" {
		t.Fatalf("ignored optional fallback facts missing: %#v", report.RepoLocalBinary)
	}
}

func TestInspectBlocksWhenIgnoredRepoLocalFallbackIsOnlyTool(t *testing.T) {
	root := t.TempDir()
	writeReadyInspectFixture(t, root)
	writeExecutable(t, root, "bin/burpvalve", "#!/usr/bin/env bash\n")
	runner := fakeRunnerPtr(map[string]CommandResult{
		"git status --short":              {Stdout: ""},
		"git config --get core.hooksPath": {Stdout: ".githooks\n"},
		"git check-ignore -v -- bin/burpvalve": {
			Stdout: ".gitignore:1:bin/\tbin/burpvalve\n",
		},
	}, map[string]error{})

	report, err := Inspect(root, InspectOptions{
		Runner: runner,
		Looker: fakeLooker{"br": true, "git": true},
		Now:    fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	tool := findRequiredCheck(t, report, "backpressure-tool")
	if tool.Status != StatusConflict || !tool.Required || !strings.Contains(tool.Message, "ignored by git") {
		t.Fatalf("ignored only-tool check = %#v", tool)
	}
	if report.Summary.Ready {
		t.Fatalf("ignored required fallback should block readiness: %#v", report.Summary)
	}
	if report.RepoBinPath != "bin/burpvalve" || report.HookCommandSource != "repo-local-conflict" || report.ReadinessSeverity != "blocked" {
		t.Fatalf("ignored only-tool facts wrong: severity=%q repo_bin=%q hook=%q", report.ReadinessSeverity, report.RepoBinPath, report.HookCommandSource)
	}
	if report.RepoLocalBinary == nil || report.RepoLocalBinary.WarningCode != "repo_local_ignored" || !strings.Contains(report.RepoLocalBinary.ComparisonBasis, "ignored") {
		t.Fatalf("ignored active fallback facts missing: %#v", report.RepoLocalBinary)
	}
}

func TestInspectWarnsForActiveRepoLocalFallbackFreshness(t *testing.T) {
	root := t.TempDir()
	writeReadyInspectFixture(t, root)
	writeExecutable(t, root, "bin/burpvalve", "#!/usr/bin/env bash\nif [ \"$1\" = \"--version\" ]; then echo old; fi\n")
	writeFile(t, root, "cmd/burpvalve/main.go", "package main\n")
	old := fixedNow().Add(-2 * time.Hour)
	newer := fixedNow().Add(-1 * time.Hour)
	if err := os.Chtimes(filepath.Join(root, "bin/burpvalve"), old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(root, "cmd/burpvalve/main.go"), newer, newer); err != nil {
		t.Fatal(err)
	}
	runner := fakeRunnerPtr(map[string]CommandResult{
		"git status --short":                       {Stdout: ""},
		"git status --short --untracked-files=all": {Stdout: ""},
		"git config --get core.hooksPath":          {Stdout: ".githooks\n"},
		"git check-ignore -v -- bin/burpvalve":     {ExitCode: 1},
		"git ls-files -z -- cmd internal go.mod go.sum internal/scaffold/templates templates scripts install.sh": {Stdout: "cmd/burpvalve/main.go\x00"},
		"bin/burpvalve --version": {Stdout: "old\n"},
	}, map[string]error{
		"git check-ignore -v -- bin/burpvalve": errors.New("exit status 1"),
	})

	report, err := Inspect(root, InspectOptions{
		Runner: runner,
		Looker: fakeLooker{"br": true, "git": true},
		Now:    fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Summary.Ready || report.Fatal || report.Status != "ready" {
		t.Fatalf("stale-risk repo-local fallback should warn without blocking: %#v", report)
	}
	if report.ReadinessSeverity != "warning" {
		t.Fatalf("repo-local stale risk should produce warning severity: %#v", report)
	}
	if report.RepoLocalBinary == nil || report.RepoLocalBinary.FreshnessStatus != "stale" || report.RepoLocalBinary.WarningCode != "repo_local_stale" {
		t.Fatalf("stale repo-local facts wrong: %#v", report.RepoLocalBinary)
	}
	if !strings.Contains(report.RepoLocalBinary.ComparisonBasis, "repo_local_older_than_source") {
		t.Fatalf("stale basis missing source comparison: %#v", report.RepoLocalBinary)
	}
	if !hasRecoveryStep(report, "repo-local-binary-provenance", "go run ./cmd/burpvalve setup, or install/use PATH burpvalve, or keep bin/burpvalve intentionally", false) {
		t.Fatalf("repo-local warning should include explicit choices: %#v", report.NextSteps)
	}
	text := report.Text()
	for _, needle := range []string{"repo-local binary provenance", "freshness", "stale", "repo_local_stale"} {
		if !strings.Contains(text, needle) {
			t.Fatalf("human setup text missing %q:\n%s", needle, text)
		}
	}
}

func TestInspectRepoLocalFallbackFreshAndUnknownStates(t *testing.T) {
	for _, tt := range []struct {
		name        string
		pathLooker  fakeLooker
		pathCommand string
		want        string
		wantCode    string
	}{
		{name: "fresh", pathLooker: fakeLooker{"br": true, "git": true}, want: "fresh", wantCode: "repo_local_fallback_active"},
		{name: "path command non regular", pathLooker: fakeLooker{"br": true, "git": true, "burpvalve": true}, pathCommand: "/fake/bin/burpvalve", want: "not_applicable", wantCode: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeReadyInspectFixture(t, root)
			writeExecutable(t, root, "bin/burpvalve", "#!/usr/bin/env bash\nif [ \"$1\" = \"--version\" ]; then echo same; fi\n")
			writeFile(t, root, "cmd/burpvalve/main.go", "package main\n")
			newer := fixedNow()
			older := fixedNow().Add(-1 * time.Hour)
			if err := os.Chtimes(filepath.Join(root, "bin/burpvalve"), newer, newer); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(filepath.Join(root, "cmd/burpvalve/main.go"), older, older); err != nil {
				t.Fatal(err)
			}
			runner := fakeRunnerPtr(map[string]CommandResult{
				"git status --short":                       {Stdout: ""},
				"git status --short --untracked-files=all": {Stdout: ""},
				"git config --get core.hooksPath":          {Stdout: ".githooks\n"},
				"git check-ignore -v -- bin/burpvalve":     {ExitCode: 1},
				"git ls-files -z -- cmd internal go.mod go.sum internal/scaffold/templates templates scripts install.sh": {Stdout: "cmd/burpvalve/main.go\x00"},
				"bin/burpvalve --version": {Stdout: "same\n"},
			}, map[string]error{
				"git check-ignore -v -- bin/burpvalve": errors.New("exit status 1"),
			})
			report, err := Inspect(root, InspectOptions{
				Runner: runner,
				Looker: tt.pathLooker,
				Now:    fixedNow,
			})
			if err != nil {
				t.Fatal(err)
			}
			if report.RepoLocalBinary == nil || report.RepoLocalBinary.FreshnessStatus != tt.want || report.RepoLocalBinary.WarningCode != tt.wantCode {
				t.Fatalf("freshness facts = %#v, want status=%s code=%s", report.RepoLocalBinary, tt.want, tt.wantCode)
			}
		})
	}
}

func TestInspectRepoLocalFreshnessUnknownForDirtySourceOutsideComparison(t *testing.T) {
	root := t.TempDir()
	writeReadyInspectFixture(t, root)
	writeExecutable(t, root, "bin/burpvalve", "#!/usr/bin/env bash\n")
	writeFile(t, root, "cmd/burpvalve/main.go", "package main\n")
	newer := fixedNow()
	older := fixedNow().Add(-1 * time.Hour)
	if err := os.Chtimes(filepath.Join(root, "bin/burpvalve"), newer, newer); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(root, "cmd/burpvalve/main.go"), older, older); err != nil {
		t.Fatal(err)
	}
	runner := fakeRunnerPtr(map[string]CommandResult{
		"git status --short":                       {Stdout: "?? cmd/burpvalve/generated.go\n"},
		"git status --short --untracked-files=all": {Stdout: "?? cmd/burpvalve/generated.go\n"},
		"git config --get core.hooksPath":          {Stdout: ".githooks\n"},
		"git check-ignore -v -- bin/burpvalve":     {ExitCode: 1},
		"git ls-files -z -- cmd internal go.mod go.sum internal/scaffold/templates templates scripts install.sh": {Stdout: "cmd/burpvalve/main.go\x00"},
	}, map[string]error{
		"git check-ignore -v -- bin/burpvalve": errors.New("exit status 1"),
	})

	report, err := Inspect(root, InspectOptions{
		Runner: runner,
		Looker: fakeLooker{"br": true, "git": true},
		Now:    fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.RepoLocalBinary == nil || report.RepoLocalBinary.FreshnessStatus != "unknown" || report.RepoLocalBinary.WarningCode != "repo_local_freshness_unknown" {
		t.Fatalf("dirty source should make freshness unknown: %#v", report.RepoLocalBinary)
	}
	if !strings.Contains(report.RepoLocalBinary.ComparisonBasis, "dirty_source_unknown") {
		t.Fatalf("dirty source basis missing: %#v", report.RepoLocalBinary)
	}
}

func TestInspectRepoLocalFreshnessUnknownForSymlinkFallback(t *testing.T) {
	root := t.TempDir()
	writeReadyInspectFixture(t, root)
	writeFile(t, root, "real-burpvalve", "#!/usr/bin/env bash\n")
	if err := os.Chmod(filepath.Join(root, "real-burpvalve"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../real-burpvalve", filepath.Join(root, "bin/burpvalve")); err != nil {
		t.Fatal(err)
	}
	runner := fakeRunnerPtr(map[string]CommandResult{
		"git status --short":                   {Stdout: ""},
		"git config --get core.hooksPath":      {Stdout: ".githooks\n"},
		"git check-ignore -v -- bin/burpvalve": {ExitCode: 1},
	}, map[string]error{
		"git check-ignore -v -- bin/burpvalve": errors.New("exit status 1"),
	})

	report, err := Inspect(root, InspectOptions{
		Runner: runner,
		Looker: fakeLooker{"br": true, "git": true},
		Now:    fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.RepoLocalBinary == nil || report.RepoLocalBinary.FreshnessStatus != "unknown" || report.RepoLocalBinary.WarningCode != "repo_local_freshness_unknown" {
		t.Fatalf("symlink fallback should stay unknown: %#v", report.RepoLocalBinary)
	}
	if !strings.Contains(report.RepoLocalBinary.ComparisonBasis, "repo_local_not_regular") {
		t.Fatalf("symlink basis missing: %#v", report.RepoLocalBinary)
	}
}

func TestInspectIgnoresLocalProjectRegistry(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "Projects", "repo")
	writeFile(t, root, "AGENTS.md", "ok\n")
	writeFile(t, home, "Projects/.registry/projects.yaml", "- id: internal\n")
	runner := fakeRunnerPtr(map[string]CommandResult{}, map[string]error{})

	report, err := Inspect(root, InspectOptions{
		Runner: runner,
		Looker: fakeLooker{},
		Now:    fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hasCheck(report, "project-registry") {
		t.Fatalf("inspect should not include internal project registry checks: %#v", report.Checks)
	}
	if calls := runner.Calls(); len(calls) != 0 {
		t.Fatalf("inspect should not query git remotes for registry metadata: %#v", calls)
	}
	if got := readFile(t, home, "Projects/.registry/projects.yaml"); got != "- id: internal\n" {
		t.Fatalf("inspect mutated local registry: %q", got)
	}
}

func TestInspectReportsHookConflicts(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".git/HEAD", "ref: refs/heads/main\n")
	writeFile(t, root, ".githooks/pre-commit", "#!/usr/bin/env bash\n")
	runner := fakeRunnerPtr(map[string]CommandResult{
		"git status --short":              {Stdout: ""},
		"git config --get core.hooksPath": {Stdout: "hooks\n"},
	}, map[string]error{})
	report, err := Inspect(root, InspectOptions{
		Runner: runner,
		Looker: fakeLooker{"br": true, "git": true},
		Now:    fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertCheck(t, report, "githook", StatusConflict)
	assertCheck(t, report, "git-hooks-path", StatusConflict)
	if !hasPlannedChange(report, "githook", "manual review before repair") {
		t.Fatalf("expected hook conflict planned change: %#v", report.PlannedChanges)
	}
}

func TestInspectReportsOrchestratorOnlyWhenRequired(t *testing.T) {
	root := t.TempDir()
	report, err := Inspect(root, InspectOptions{
		Runner: fakeRunnerPtr(map[string]CommandResult{}, map[string]error{}),
		Looker: fakeLooker{},
		Now:    fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hasCheck(report, "orchestrator") {
		t.Fatalf("default inspect should not require ORCHESTRATOR.md: %#v", report.Checks)
	}

	report, err = Inspect(root, InspectOptions{
		Runner:              fakeRunnerPtr(map[string]CommandResult{}, map[string]error{}),
		Looker:              fakeLooker{},
		Now:                 fixedNow,
		RequireOrchestrator: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertCheck(t, report, "orchestrator", StatusMissing)
	if !hasPlannedChange(report, "orchestrator", "create missing scaffold path") {
		t.Fatalf("required orchestrator should produce a create plan: %#v", report.PlannedChanges)
	}
}

func TestInspectReportsClaudeRouteFacts(t *testing.T) {
	t.Run("default symlink route", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "AGENTS.md", "# Agent Operating Contract\n")
		if err := os.Symlink("AGENTS.md", filepath.Join(root, "CLAUDE.md")); err != nil {
			t.Fatal(err)
		}
		report, err := Inspect(root, InspectOptions{
			Runner: fakeRunnerPtr(map[string]CommandResult{}, map[string]error{}),
			Looker: fakeLooker{},
			Now:    fixedNow,
		})
		if err != nil {
			t.Fatal(err)
		}
		assertCheck(t, report, "claude", StatusPresent)
		if report.ClaudeRoute == nil || report.ClaudeRoute.Expected != ClaudeRouteAgentSymlink || report.ClaudeRoute.Detected != ClaudeRouteAgentSymlink {
			t.Fatalf("default route facts wrong: %#v", report.ClaudeRoute)
		}
	})

	t.Run("orchestrator route missing pieces", func(t *testing.T) {
		root := t.TempDir()
		report, err := Inspect(root, InspectOptions{
			Runner:      fakeRunnerPtr(map[string]CommandResult{}, map[string]error{}),
			Looker:      fakeLooker{},
			Now:         fixedNow,
			ClaudeRoute: ClaudeRouteOrchestratorSkill,
		})
		if err != nil {
			t.Fatal(err)
		}
		assertCheck(t, report, "claude", StatusMissing)
		if report.ClaudeRoute == nil || report.ClaudeRoute.Expected != ClaudeRouteOrchestratorSkill || report.ClaudeRoute.Detected != ClaudeRouteNone {
			t.Fatalf("orchestrator missing route facts wrong: %#v", report.ClaudeRoute)
		}
		if !contains(report.ClaudeRoute.MissingPieces, "CLAUDE.md") ||
			!contains(report.ClaudeRoute.MissingPieces, ".claude/skills/burpvalve-orchestrator/SKILL.md") {
			t.Fatalf("orchestrator missing pieces incomplete: %#v", report.ClaudeRoute.MissingPieces)
		}
	})

	t.Run("orchestrator route present", func(t *testing.T) {
		root := t.TempDir()
		_, err := ApplyInitWithOptions(root, ApplyOptions{
			Targets:     []ScaffoldTarget{TargetClaude},
			ClaudeRoute: ClaudeRouteOrchestratorSkill,
		})
		if err != nil {
			t.Fatal(err)
		}
		report, err := Inspect(root, InspectOptions{
			Runner:      fakeRunnerPtr(map[string]CommandResult{}, map[string]error{}),
			Looker:      fakeLooker{},
			Now:         fixedNow,
			ClaudeRoute: ClaudeRouteOrchestratorSkill,
		})
		if err != nil {
			t.Fatal(err)
		}
		assertCheck(t, report, "claude", StatusPresent)
		if report.ClaudeRoute.Detected != ClaudeRouteOrchestratorSkill || len(report.ClaudeRoute.MissingPieces) != 0 || len(report.ClaudeRoute.Drift) != 0 {
			t.Fatalf("orchestrator route facts wrong: %#v", report.ClaudeRoute)
		}
	})

	t.Run("orchestrator route drift", func(t *testing.T) {
		root := t.TempDir()
		_, err := ApplyInitWithOptions(root, ApplyOptions{
			Targets:     []ScaffoldTarget{TargetClaude},
			ClaudeRoute: ClaudeRouteOrchestratorSkill,
		})
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, root, ".claude/skills/burpvalve-orchestrator/SKILL.md", "# drift\n")
		report, err := Inspect(root, InspectOptions{
			Runner:      fakeRunnerPtr(map[string]CommandResult{}, map[string]error{}),
			Looker:      fakeLooker{},
			Now:         fixedNow,
			ClaudeRoute: ClaudeRouteOrchestratorSkill,
		})
		if err != nil {
			t.Fatal(err)
		}
		assertCheck(t, report, "claude", StatusConflict)
		if !contains(report.ClaudeRoute.Drift, ".claude/skills/burpvalve-orchestrator/SKILL.md") {
			t.Fatalf("drift facts missing SKILL.md: %#v", report.ClaudeRoute)
		}
	})

	t.Run("unmarked regular claude conflicts", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "CLAUDE.md", "# Human Claude Notes\n")
		report, err := Inspect(root, InspectOptions{
			Runner: fakeRunnerPtr(map[string]CommandResult{}, map[string]error{}),
			Looker: fakeLooker{},
			Now:    fixedNow,
		})
		if err != nil {
			t.Fatal(err)
		}
		assertCheck(t, report, "claude", StatusConflict)
		if report.ClaudeRoute == nil || len(report.ClaudeRoute.Conflicts) == 0 {
			t.Fatalf("regular CLAUDE.md conflict facts missing: %#v", report.ClaudeRoute)
		}
	})
}

func TestInspectRejectsPlaceholderBackpressureTool(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "tools/burpvalve/README.md", "placeholder\n")

	report, err := Inspect(root, InspectOptions{
		Runner: fakeRunnerPtr(map[string]CommandResult{}, map[string]error{}),
		Looker: fakeLooker{"br": true},
		Now:    fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertCheck(t, report, "backpressure-tool", StatusUnavailable)
	if hasCheck(report, "backpressure-tool-fallback") {
		t.Fatalf("tools/burpvalve docs should not be reported as repo-local binary fallback: %#v", report.Checks)
	}
	if !hasPlannedChange(report, "backpressure-tool", "resolve blocker") {
		t.Fatalf("expected runnable verifier planned change: %#v", report.PlannedChanges)
	}
}

func TestReportTextContainsCoreFields(t *testing.T) {
	report := Report{
		TargetRoot: "/example/repo",
		Mutating:   false,
		Summary:    Summary{Ready: false, Missing: 1},
		Checks: []Check{{
			ID:      "agents",
			Status:  StatusMissing,
			Path:    "AGENTS.md",
			Message: "repo-local operating contract",
		}},
	}
	text := report.Text()
	for _, needle := range []string{
		"burpvalve setup report",
		"summary",
		"target",
		"/example/repo",
		"edits files",
		"ready",
		"severity",
		"hook command",
		"optional context",
		"status   item / purpose",
		"missing  AGENTS.md - shared instructions that agents and tools read before working",
		"planned changes",
		"-    none",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("report text missing %q:\n%s", needle, text)
		}
	}
}

func TestReportTextIncludesConfigSources(t *testing.T) {
	report := Report{
		TargetRoot:        "/example/repo",
		ReadinessSeverity: "ready",
		HookCommandSource: "path",
		Summary:           Summary{Ready: true},
		Config: &ConfigSummary{
			GlobalPath:   "/example/user/.config/burpvalve/config.json",
			GlobalFound:  true,
			ProjectPath:  "/example/repo/.burpvalve.json",
			ProjectFound: true,
			Sources: []ConfigSource{
				{Key: "defaults.init.beads", Source: "project"},
				{Key: "defaults.shell", Source: "global"},
			},
		},
	}
	text := report.Text()
	for _, needle := range []string{
		"config sources",
		"global",
		"/example/user/.config/burpvalve/config.json (found)",
		"project",
		"/example/repo/.burpvalve.json (found)",
		"defaults.init.beads",
		"defaults.shell",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("config source text missing %q:\n%s", needle, text)
		}
	}
}

func TestReportTextIncludesConfigSettingValuesWithoutDuplicateSourceRows(t *testing.T) {
	report := Report{
		TargetRoot:        "/example/repo",
		ReadinessSeverity: "ready",
		HookCommandSource: "path",
		Summary:           Summary{Ready: true},
		Config: &ConfigSummary{
			GlobalPath:   "/example/user/.config/burpvalve/config.json",
			GlobalFound:  true,
			ProjectPath:  "/example/repo/.burpvalve.json",
			ProjectFound: true,
			Sources: []ConfigSource{
				{Key: "defaults.init.beads", Source: "project"},
			},
			Settings: []ConfigSetting{
				{Key: "defaults.init.beads", Source: "project", Value: "false"},
			},
		},
	}
	text := report.Text()
	if !strings.Contains(text, "defaults.init.beads = false") {
		t.Fatalf("config text missing effective value:\n%s", text)
	}
	if strings.Count(text, "defaults.init.beads") != 1 {
		t.Fatalf("config text should not duplicate source and setting rows:\n%s", text)
	}
}

func TestReportTextExplainsGitShortStatus(t *testing.T) {
	report := Report{
		TargetRoot: "/example/repo",
		Checks: []Check{{
			ID:      "git-dirty",
			Status:  StatusDirty,
			Message: "working tree has changes",
			Detail:  " M README.md\n D old.go\n?? new.go",
		}},
	}
	text := report.Text()
	for _, needle := range []string{
		"git changes",
		"Git shows two status columns before each file: staged | working tree.",
		"M = modified",
		"D = deleted",
		"? = untracked",
		"-M      README.md",
		"-D      old.go",
		"??      new.go",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("git status report missing %q:\n%s", needle, text)
		}
	}
}

func TestInspectPreservesLeadingGitShortStatusColumn(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".git/HEAD", "ref: refs/heads/main\n")
	runner := fakeRunnerPtr(map[string]CommandResult{
		"git config --get core.hooksPath": {Stdout: ".githooks\n"},
		"git status --short":              {Stdout: " M .github/workflows/ci.yml\n D old.go\n"},
	}, map[string]error{})
	report, err := Inspect(root, InspectOptions{
		Runner: runner,
		Looker: fakeLooker{"git": true},
		Now:    fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	text := report.Text()
	for _, needle := range []string{
		"-M      .github/workflows/ci.yml",
		"-D      old.go",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("git status report missing %q:\n%s", needle, text)
		}
	}
}

func TestReportTextWithColorHighlightsStatuses(t *testing.T) {
	report := Report{
		TargetRoot: "/example/repo",
		Checks: []Check{
			{ID: "agents", Status: StatusMissing, Path: "AGENTS.md", Message: "missing"},
			{ID: "git-dirty", Status: StatusDirty, Detail: " D old.go"},
		},
	}
	text := report.TextWithOptions(TextOptions{Color: true})
	for _, needle := range []string{
		"\x1b[",
		"missing",
		"-D",
		"old.go",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("colored report missing %q:\n%s", needle, text)
		}
	}
}

func hasPlannedChange(report Report, id, action string) bool {
	for _, change := range report.PlannedChanges {
		if change.ID == id && change.Action == action {
			return true
		}
	}
	return false
}

func hasRecoveryStep(report Report, id, command string, fatal bool) bool {
	for _, step := range report.NextSteps {
		if step.ID == id && step.Command == command && step.Fatal == fatal {
			return true
		}
	}
	return false
}

func assertCheck(t *testing.T, report Report, id string, status CheckStatus) {
	t.Helper()
	for _, check := range report.Checks {
		if check.ID == id {
			if check.Status != status {
				t.Fatalf("check %s status = %s, want %s (%#v)", id, check.Status, status, check)
			}
			return
		}
	}
	t.Fatalf("missing check %s in %#v", id, report.Checks)
}

func findRequiredCheck(t *testing.T, report Report, id string) Check {
	t.Helper()
	for _, check := range report.Checks {
		if check.ID == id {
			return check
		}
	}
	t.Fatalf("missing check %s in %#v", id, report.Checks)
	return Check{}
}

func hasCheck(report Report, id string) bool {
	for _, check := range report.Checks {
		if check.ID == id {
			return true
		}
	}
	return false
}

func checkIDs(report Report) []string {
	ids := make([]string, 0, len(report.Checks))
	for _, check := range report.Checks {
		ids = append(ids, check.ID)
	}
	return ids
}

func sortedStrings(values []string) []string {
	values = append([]string(nil), values...)
	sort.Strings(values)
	return values
}

func writeReadyInspectFixture(t *testing.T, root string) {
	t.Helper()
	for _, rel := range []string{
		"AGENTS.md",
		"docs/README.md",
		"plans/README.md",
		"log/README.md",
		"backpressure/README.md",
		"backpressure/attestations/README.md",
		".beads/config.yaml",
		".git/HEAD",
	} {
		writeFile(t, root, rel, "ok\n")
	}
	writeExecutable(t, root, ".githooks/pre-commit", "#!/usr/bin/env bash\n")
	if err := os.Symlink("AGENTS.md", filepath.Join(root, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}
}

func snapshotFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		entry := rel + "|" + info.Mode().String()
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entry += "|link:" + target
		case info.Mode().IsRegular():
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			entry += "|file:" + string(body)
		case info.IsDir():
			entry += "|dir"
		default:
			entry += "|other"
		}
		files = append(files, entry)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}

func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeExecutable(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func fakeRunnerPtr(results map[string]CommandResult, errors map[string]error) *fakeRunner {
	return &fakeRunner{results: results, errors: errors}
}

func fixedNow() time.Time {
	return time.Date(2026, 6, 20, 1, 0, 0, 0, time.UTC)
}
