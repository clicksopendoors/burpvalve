package backpressure

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunVerifierDoctorReportsKnownRuntimeValues(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeFile(t, root, ".claude/settings.json", `{"subagents":{"max_subagents":4,"max_depth":2}}`)
	writeFile(t, root, ".codex/config.toml", "[verifier]\nmax_parallel_verifiers = 3\nmax_depth = 1\n")
	writeFile(t, home, ".config/ntm/config.json", `{"runtime":{"subagent_limit":2,"depth_limit":1}}`)

	result, err := RunVerifierDoctor(context.Background(), VerifierDoctorOptions{Root: root, HomeDir: home})
	if err != nil {
		t.Fatalf("doctor failed: %v", err)
	}
	if result.Command != "verifier doctor" || !result.ReportOnly || len(result.NextSteps) == 0 {
		t.Fatalf("result contract missing: %#v", result)
	}
	claude := doctorRuntime(t, result, "claude-code")
	if !claude.Supported || doctorLimit(t, claude, "subagent_limit") != float64(4) || doctorLimit(t, claude, "depth_limit") != float64(2) {
		t.Fatalf("claude limits not reported: %#v", claude)
	}
	codex := doctorRuntime(t, result, "codex")
	if !codex.Supported || doctorLimit(t, codex, "subagent_limit") != 3 || doctorLimit(t, codex, "depth_limit") != 1 {
		t.Fatalf("codex limits not reported: %#v", codex)
	}
	ntm := doctorRuntime(t, result, "ntm")
	if !ntm.Supported || doctorLimit(t, ntm, "subagent_limit") != float64(2) || doctorLimit(t, ntm, "depth_limit") != float64(1) {
		t.Fatalf("ntm limits not reported: %#v", ntm)
	}
}

func TestRunVerifierDoctorMarksMalformedConfigUnsupported(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".claude/settings.json", `{"max_subagents":`)

	result, err := RunVerifierDoctor(context.Background(), VerifierDoctorOptions{Root: root, HomeDir: t.TempDir()})
	if err != nil {
		t.Fatalf("doctor failed: %v", err)
	}
	claude := doctorRuntime(t, result, "claude-code")
	if claude.Supported {
		t.Fatalf("malformed config should not be supported: %#v", claude)
	}
	if len(claude.Paths) == 0 || claude.Paths[0].Supported || claude.Paths[0].Message == "" {
		t.Fatalf("unsupported path details missing: %#v", claude.Paths)
	}
}

func TestRunVerifierDoctorDoesNotWriteCandidateFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".codex", "config.toml")
	writeFile(t, root, ".codex/config.toml", "max_parallel_verifiers = 5\n")
	wantBody, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wantInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	wantInfo, err = os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := RunVerifierDoctor(context.Background(), VerifierDoctorOptions{Root: root, HomeDir: t.TempDir()}); err != nil {
		t.Fatalf("doctor failed: %v", err)
	}
	gotBody, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	gotInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotBody) != string(wantBody) || gotInfo.Mode() != wantInfo.Mode() || !gotInfo.ModTime().Equal(wantInfo.ModTime()) {
		t.Fatalf("doctor mutated candidate file: body=%q/%q mode=%v/%v mtime=%v/%v", gotBody, wantBody, gotInfo.Mode(), wantInfo.Mode(), gotInfo.ModTime(), wantInfo.ModTime())
	}
}

func doctorRuntime(t *testing.T, result VerifierDoctorResult, runtime string) VerifierDoctorRuntimeCheck {
	t.Helper()
	for _, check := range result.Checks {
		if check.Runtime == runtime {
			return check
		}
	}
	t.Fatalf("missing runtime %s in %#v", runtime, result.Checks)
	return VerifierDoctorRuntimeCheck{}
}

func doctorLimit(t *testing.T, check VerifierDoctorRuntimeCheck, name string) any {
	t.Helper()
	value, ok := check.Limits[name]
	if !ok {
		t.Fatalf("missing limit %s in %#v", name, check.Limits)
	}
	return value.Value
}
