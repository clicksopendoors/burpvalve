package backpressure

import (
	"fmt"
	"strings"

	"burpvalve/internal/lintconfig"
)

const (
	LintPresetAuto  = "auto"
	LintPresetGo    = "go"
	LintPresetNode  = "node"
	LintPresetAstro = "astro"
)

func NormalizeLintPreset(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "auto", "all":
		return LintPresetAuto
	case LintPresetGo, LintPresetNode, LintPresetAstro:
		return strings.ToLower(strings.TrimSpace(name))
	default:
		return strings.ToLower(strings.TrimSpace(name))
	}
}

func BuiltInLintPresetCommands(detection LintDetection, preset string) ([]lintconfig.Command, error) {
	preset = NormalizeLintPreset(preset)
	switch preset {
	case LintPresetAuto, LintPresetGo, LintPresetNode, LintPresetAstro:
	default:
		return nil, fmt.Errorf("unknown lint preset %q; expected auto, go, node, or astro", preset)
	}
	var commands []lintconfig.Command
	if preset == LintPresetAuto || preset == LintPresetGo {
		commands = append(commands, builtInGoPresetCommands(detection)...)
	}
	if preset == LintPresetAuto || preset == LintPresetNode || preset == LintPresetAstro {
		commands = append(commands, builtInNodePresetCommands(detection)...)
	}
	return commands, nil
}

func builtInGoPresetCommands(detection LintDetection) []lintconfig.Command {
	if !detection.GoAvailable {
		return nil
	}
	var commands []lintconfig.Command
	for _, root := range detection.GoRoots {
		commands = append(commands,
			lintPresetCommand(root.Path, "go-test", "go test ./...", true, 120),
			lintPresetCommand(root.Path, "go-vet", "go vet ./...", true, 120),
			lintPresetCommand(root.Path, "go-fmt-check", `test -z "$(gofmt -l .)"`, true, 60),
		)
	}
	return commands
}

func builtInNodePresetCommands(detection LintDetection) []lintconfig.Command {
	var commands []lintconfig.Command
	for _, root := range detection.NodeRoots {
		for _, script := range root.Scripts {
			commands = append(commands, lintPresetCommand(root.Path, root.PackageManager+"-"+lintCommandIDPart(script), nodeRunCommand(root.PackageManager, script), true, 120))
		}
	}
	return commands
}

func lintPresetCommand(root, idPrefix, command string, required bool, timeoutSeconds int) lintconfig.Command {
	id := idPrefix
	if root != "." {
		id += "-" + lintCommandIDPart(root)
	}
	return lintconfig.Command{
		ID:             id,
		Command:        command,
		Required:       required,
		Paths:          []string{root},
		TimeoutSeconds: timeoutSeconds,
		RunDirectory:   root,
	}
}
