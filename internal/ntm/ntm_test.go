package ntm

import (
	"context"
	"os"
	"reflect"
	"testing"
)

type fakeRunner struct {
	results map[string]CommandResult
	errors  map[string]error
	calls   []string
}

func (f *fakeRunner) Run(_ context.Context, _ string, name string, args ...string) (CommandResult, error) {
	key := name
	for _, arg := range args {
		key += " " + arg
	}
	f.calls = append(f.calls, key)
	return f.results[key], f.errors[key]
}

type fakeLooker map[string]bool

func (f fakeLooker) LookPath(file string) (string, error) {
	if f[file] {
		return "/fake/bin/" + file, nil
	}
	return "", os.ErrNotExist
}

func TestCheckUnavailableNTM(t *testing.T) {
	report := Check("/example/burpvalve", &fakeRunner{}, fakeLooker{})
	if report.Status != StatusUnavailable {
		t.Fatalf("status = %s", report.Status)
	}
	if report.BaseSessionName != "burpvalve" {
		t.Fatalf("base = %s", report.BaseSessionName)
	}
	if !reflect.DeepEqual(report.IntendedCommand, []string{"ntm", "quick", "burpvalve"}) {
		t.Fatalf("intended command = %#v", report.IntendedCommand)
	}
}

func TestCheckCapabilitiesAndQuickIntent(t *testing.T) {
	runner := &fakeRunner{results: map[string]CommandResult{
		"ntm --robot-capabilities": {Stdout: "capabilities ok\nmore"},
	}, errors: map[string]error{}}
	report := Check("/example/burpvalve", runner, fakeLooker{"ntm": true})
	if report.Status != StatusReady {
		t.Fatalf("status = %s blocker=%s", report.Status, report.Blocker)
	}
	if report.CapabilitiesOutput != "capabilities ok" {
		t.Fatalf("capabilities = %q", report.CapabilitiesOutput)
	}
	if !reflect.DeepEqual(runner.calls, []string{"ntm --robot-capabilities"}) {
		t.Fatalf("calls = %#v", runner.calls)
	}
}

func TestQuickRequiresSnapshotEvidence(t *testing.T) {
	runner := &fakeRunner{results: map[string]CommandResult{
		"ntm --robot-capabilities": {Stdout: "capabilities ok"},
		"ntm quick project":        {Stdout: "created"},
		"ntm --robot-snapshot":     {Stdout: "snapshot ok\nattention tail"},
	}, errors: map[string]error{}}
	report := Quick("/example/project", runner, fakeLooker{"ntm": true})
	if report.Status != StatusRegistered {
		t.Fatalf("status = %s blocker=%s", report.Status, report.Blocker)
	}
	want := []string{"ntm --robot-capabilities", "ntm quick project", "ntm --robot-snapshot"}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
	if report.SnapshotOutput != "snapshot ok\nattention tail" {
		t.Fatalf("snapshot = %q", report.SnapshotOutput)
	}
}

func TestQuickBlocksOnFailedSnapshot(t *testing.T) {
	runner := &fakeRunner{results: map[string]CommandResult{
		"ntm --robot-capabilities": {Stdout: "capabilities ok"},
		"ntm quick project":        {Stdout: "created"},
		"ntm --robot-snapshot":     {Stderr: "no session"},
	}, errors: map[string]error{"ntm --robot-snapshot": os.ErrInvalid}}
	report := Quick("/example/project", runner, fakeLooker{"ntm": true})
	if report.Status != StatusBlocked {
		t.Fatalf("status = %s", report.Status)
	}
	if report.Blocker == "" {
		t.Fatal("expected blocker")
	}
}

func TestQuickBlocksOnEmptySnapshotEvidence(t *testing.T) {
	runner := &fakeRunner{results: map[string]CommandResult{
		"ntm --robot-capabilities": {Stdout: "capabilities ok"},
		"ntm quick project":        {Stdout: "created"},
		"ntm --robot-snapshot":     {Stdout: "   \n"},
	}, errors: map[string]error{}}
	report := Quick("/example/project", runner, fakeLooker{"ntm": true})
	if report.Status != StatusBlocked {
		t.Fatalf("status = %s", report.Status)
	}
	if report.Blocker != "ntm --robot-snapshot returned no evidence after quick" {
		t.Fatalf("blocker = %q", report.Blocker)
	}
}
