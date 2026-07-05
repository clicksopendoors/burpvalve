package ntm

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type Runner interface {
	Run(ctx context.Context, dir string, name string, args ...string) (CommandResult, error)
}

type Looker interface {
	LookPath(file string) (string, error)
}

type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type Status string

const (
	StatusUnavailable Status = "unavailable"
	StatusReady       Status = "ready"
	StatusRegistered  Status = "registered"
	StatusBlocked     Status = "blocked"
	StatusSkipped     Status = "skipped"
)

type Report struct {
	Status             Status   `json:"status"`
	BaseSessionName    string   `json:"base_session_name"`
	IntendedCommand    []string `json:"intended_command,omitempty"`
	CapabilitiesOutput string   `json:"capabilities_output,omitempty"`
	SnapshotOutput     string   `json:"snapshot_output,omitempty"`
	Blocker            string   `json:"blocker,omitempty"`
}

// BaseSessionName returns the repo basename used as the default NTM session.
func BaseSessionName(projectRoot string) string {
	return filepath.Base(filepath.Clean(projectRoot))
}

func Check(projectRoot string, runner Runner, looker Looker) Report {
	base := BaseSessionName(projectRoot)
	report := Report{
		BaseSessionName: base,
		IntendedCommand: []string{"ntm", "quick", base},
	}
	if _, err := looker.LookPath("ntm"); err != nil {
		report.Status = StatusUnavailable
		report.Blocker = "ntm executable unavailable"
		return report
	}
	output, err := run(projectRoot, runner, "ntm", "--robot-capabilities")
	if err != nil {
		report.Status = StatusBlocked
		report.Blocker = "ntm --robot-capabilities failed: " + err.Error()
		return report
	}
	report.Status = StatusReady
	report.CapabilitiesOutput = firstLine(output.Stdout)
	return report
}

func Quick(projectRoot string, runner Runner, looker Looker) Report {
	report := Check(projectRoot, runner, looker)
	if report.Status != StatusReady {
		return report
	}
	if _, err := run(projectRoot, runner, "ntm", "quick", report.BaseSessionName); err != nil {
		report.Status = StatusBlocked
		report.Blocker = "ntm quick failed: " + err.Error()
		return report
	}
	snapshot, err := run(projectRoot, runner, "ntm", "--robot-snapshot")
	if err != nil {
		report.Status = StatusBlocked
		report.Blocker = "ntm --robot-snapshot failed after quick: " + err.Error()
		return report
	}
	if strings.TrimSpace(snapshot.Stdout) == "" {
		report.Status = StatusBlocked
		report.Blocker = "ntm --robot-snapshot returned no evidence after quick"
		return report
	}
	report.Status = StatusRegistered
	report.SnapshotOutput = strings.TrimSpace(snapshot.Stdout)
	return report
}

func run(projectRoot string, runner Runner, name string, args ...string) (CommandResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := runner.Run(ctx, projectRoot, name, args...)
	if err == nil {
		return output, nil
	}
	detail := strings.TrimSpace(output.Stderr)
	if detail == "" {
		detail = strings.TrimSpace(output.Stdout)
	}
	if detail == "" {
		detail = err.Error()
	}
	return output, errors.New(detail)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

func (r Report) Summary() string {
	if r.Blocker != "" {
		return fmt.Sprintf("%s: %s", r.Status, r.Blocker)
	}
	return string(r.Status)
}
