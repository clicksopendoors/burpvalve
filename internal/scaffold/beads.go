package scaffold

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func ensureBeads(root string, runner Runner, looker Looker, result *ApplyResult) error {
	if _, err := looker.LookPath("br"); err != nil {
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: ".beads", Message: "br executable unavailable; cannot initialize or verify beads"})
		return nil
	}
	if _, err := os.Stat(filepath.Join(root, ".beads")); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := runBR(root, runner, result, "br", "init"); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(root, ".beads"), 0o755); err != nil {
			return err
		}
		result.Created = append(result.Created, ".beads")
		if err := runBR(root, runner, result, "br", "sync", "--import-only"); err != nil {
			return err
		}
	} else {
		result.Preserved = append(result.Preserved, ".beads")
	}

	if err := runBRDoctor(root, runner, result); err != nil {
		result.Conflicts = append(result.Conflicts, ApplyConflict{Path: ".beads", Message: "br doctor failed: " + err.Error()})
		return nil
	}

	for _, args := range [][]string{
		{"config", "list"},
		{"dep", "cycles"},
		{"sync", "--flush-only"},
	} {
		if err := runBR(root, runner, result, append([]string{"br"}, args...)...); err != nil {
			result.Conflicts = append(result.Conflicts, ApplyConflict{Path: ".beads", Message: strings.Join(append([]string{"br"}, args...), " ") + " failed: " + err.Error()})
			return nil
		}
	}
	return nil
}

func runBRDoctor(root string, runner Runner, result *ApplyResult) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result.Commands = append(result.Commands, "br doctor --json")
	output, err := runner.Run(ctx, root, "br", "doctor", "--json")
	combined := output.Stdout + "\n" + output.Stderr
	if strings.Contains(combined, `"workspace_health":"degraded"`) || strings.Contains(combined, `"workspace_health": "degraded"`) {
		return fmt.Errorf("workspace health degraded")
	}
	if err != nil && !strings.Contains(combined, `"workspace_health":"healthy"`) && !strings.Contains(combined, `"workspace_health": "healthy"`) {
		detail := strings.TrimSpace(output.Stderr)
		if detail == "" {
			detail = strings.TrimSpace(output.Stdout)
		}
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("%s", detail)
	}
	return nil
}

func runBR(root string, runner Runner, result *ApplyResult, command ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	name := command[0]
	args := command[1:]
	result.Commands = append(result.Commands, strings.Join(command, " "))
	output, err := runner.Run(ctx, root, name, args...)
	if err != nil {
		detail := strings.TrimSpace(output.Stderr)
		if detail == "" {
			detail = strings.TrimSpace(output.Stdout)
		}
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("%s", detail)
	}
	return nil
}
