package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"burpvalve/internal/attestations"
	"burpvalve/internal/backpressure"
	"burpvalve/internal/charmui"
	"burpvalve/internal/cliui"
	bvconfig "burpvalve/internal/config"
	"burpvalve/internal/gitindex"
	"burpvalve/internal/scaffold"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var version = "dev"
var colorMode = "auto"
var robotsMode bool

const helpTemplate = `{{with (or .Long .Short)}}{{helpText . | trimTrailingWhitespaces}}

{{end}}{{if .Example}}{{helpSection "Quick Start:"}}
{{.Example | trimTrailingWhitespaces}}

{{end}}{{with (index .Annotations "shell_integration")}}{{helpSection "Shell Setup:"}}
{{. | trimTrailingWhitespaces}}

{{end}}{{helpSection "Usage:"}}
  {{if .HasAvailableSubCommands}}{{helpCommand .CommandPath}} [command]{{else}}{{helpCommand .UseLine}}{{end}}
{{if .HasAvailableSubCommands}}

{{helpSection "Available Commands:"}}
{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}  {{helpCommand (rpad .Name .NamePadding) }} {{.Short}}
{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

{{helpSection "Flags:"}}
{{helpFlagUsages .LocalFlags.FlagUsages | trimTrailingWhitespaces}}
{{end}}{{if .HasAvailableInheritedFlags}}

{{helpSection "Global Flags:"}}
{{helpFlagUsages .InheritedFlags.FlagUsages | trimTrailingWhitespaces}}
{{end}}{{if .HasAvailableSubCommands}}

{{helpText (printf "Use %q for more information about a command." (printf "%s [command] -h" .CommandPath))}}
{{end}}`

type exitError struct {
	code    int
	message string
}

func (e *exitError) Error() string {
	return e.message
}

func fail(code int, format string, args ...any) error {
	return &exitError{code: code, message: fmt.Sprintf(format, args...)}
}

func exitCode(code int) error {
	return &exitError{code: code}
}

type legacyOptions struct {
	mode            string
	target          string
	root            string
	jsonOutput      bool
	noBeads         bool
	noNTM           bool
	noClaude        bool
	noClaudeSymlink bool
	noAgents        bool
	noAgentsMD      bool
	feature         string
	responses       string
	agent           string
	model           string
}

func main() {
	cmd := newRootCommand()
	if err := cmd.Execute(); err != nil {
		var coded *exitError
		if errors.As(err, &coded) {
			if coded.message != "" {
				fmt.Fprintln(os.Stderr, coded.message)
			}
			os.Exit(coded.code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func newRootCommand() *cobra.Command {
	var legacy legacyOptions
	cmd := &cobra.Command{
		Use:           "burpvalve",
		Short:         "Install repo backpressure and gate commits before they land.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: `Burpvalve sets up a repo so each work unit (the atomic change being checked) is checked before it is committed.
Terminal runs use a question-and-answer flow for setup and attestation evidence (the seal written for checked work).
Scripts can use --json and --responses to keep the same workflow deterministic.
Automation can use --robots for structured help and JSON input.
Set NO_TUI=1 to avoid terminal UI prompts.`,
		Example: `  burpvalve setup                                # Check this repo without changing files
  burpvalve init                                 # Answer setup questions, then apply files
  burpvalve init --no-beads --no-ntm             # Skip optional tools you do not use
  burpvalve commit --feature br-123              # Check the files you are about to commit`,
		Annotations: map[string]string{
			"shell_integration": `  export PATH="$HOME/.local/bin:$PATH"          # Make the installed command available
  burpvalve init                                 # Asks setup questions, then adds the commit check
  git commit                                    # Git now runs burpvalve commit first`,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if legacy.mode != "" {
				return runLegacyMode(legacy)
			}
			if robotsMode {
				return encodeJSON(cmd.OutOrStdout(), robotHelpForCommand(cmd), "encode robot help")
			}
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return validateColorMode()
		},
	}
	registerHelpTemplateFuncs(cmd)
	cmd.SetHelpTemplate(helpTemplate)
	cmd.SetVersionTemplate("{{.Version}}\n")
	cmd.SuggestionsMinimumDistance = 1
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.PersistentFlags().StringVar(&colorMode, "color", "auto", "color output: auto, always, or never")
	cmd.PersistentFlags().BoolVar(&robotsMode, "robots", false, "robot-readable JSON help or JSON input mode")
	bindLegacyFlags(cmd, &legacy)
	cmd.AddCommand(
		newSetupCommand(),
		newExplainCommand(),
		newInitCommand(),
		newRepairCommand(),
		newCommitCommand(),
		newHashCommand(),
		newLintCommand(),
		newCICommand(),
		newAccountCommand(),
		newPromptsCommand(),
		newVerifierCommand(),
		newAttestationsCommand(),
		newBeadsCommand(),
		newConfigCommand(),
		newVersionCommand(),
		newCompletionCommand(cmd),
	)
	installRobotHelp(cmd)
	return cmd
}

func registerHelpTemplateFuncs(cmd *cobra.Command) {
	cobra.AddTemplateFunc("helpSection", func(value string) string {
		return cliui.New(shouldColorWriter(cmd.OutOrStdout())).Section(value)
	})
	cobra.AddTemplateFunc("helpCommand", func(value string) string {
		return cliui.New(shouldColorWriter(cmd.OutOrStdout())).Info(value)
	})
	cobra.AddTemplateFunc("helpText", func(value string) string {
		return cliui.New(shouldColorWriter(cmd.OutOrStdout())).Muted(value)
	})
	cobra.AddTemplateFunc("helpFlagUsages", func(value string) string {
		return colorFlagUsages(value, shouldColorWriter(cmd.OutOrStdout()))
	})
}

func colorFlagUsages(value string, color bool) string {
	if !color {
		return value
	}
	ui := cliui.New(true)
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		leading := len(line) - len(strings.TrimLeft(line, " "))
		rest := line[leading:]
		if !strings.HasPrefix(rest, "-") {
			continue
		}
		sep := doubleSpaceIndex(rest)
		if sep < 0 {
			lines[i] = line[:leading] + ui.Info(rest)
			continue
		}
		flagPart := rest[:sep]
		spacing := rest[sep:doubleSpaceEnd(rest, sep)]
		desc := rest[sep+len(spacing):]
		lines[i] = line[:leading] + ui.Info(flagPart) + spacing + desc
	}
	return strings.Join(lines, "\n")
}

type robotHelpDoc struct {
	Schema            string                `json:"schema"`
	Tool              string                `json:"tool"`
	Version           string                `json:"version"`
	Command           string                `json:"command"`
	Use               string                `json:"use"`
	Short             string                `json:"short"`
	Long              string                `json:"long,omitempty"`
	Aliases           []string              `json:"aliases,omitempty"`
	Examples          []string              `json:"examples,omitempty"`
	AvailableCommands []robotCommandSummary `json:"available_commands,omitempty"`
	Flags             []robotFlag           `json:"flags,omitempty"`
	GlobalFlags       []robotFlag           `json:"global_flags,omitempty"`
	RobotInput        any                   `json:"robot_input,omitempty"`
	RobotOutput       any                   `json:"robot_output,omitempty"`
	Notes             []string              `json:"notes,omitempty"`
}

type robotCommandSummary struct {
	Name  string `json:"name"`
	Use   string `json:"use"`
	Short string `json:"short"`
}

type robotFlag struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Type      string `json:"type"`
	Default   string `json:"default,omitempty"`
	Usage     string `json:"usage"`
}

func installRobotHelp(cmd *cobra.Command) {
	for _, child := range cmd.Commands() {
		installRobotHelp(child)
	}
	defaultHelp := cmd.HelpFunc()
	cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		if robotsMode {
			_ = encodeJSON(c.OutOrStdout(), robotHelpForCommand(c), "encode robot help")
			return
		}
		defaultHelp(c, args)
	})
}

func robotHelpForCommand(cmd *cobra.Command) robotHelpDoc {
	doc := robotHelpDoc{
		Schema:      "burpvalve.robot_help.v1",
		Tool:        "burpvalve",
		Version:     version,
		Command:     cmd.CommandPath(),
		Use:         cmd.UseLine(),
		Short:       cmd.Short,
		Long:        strings.TrimSpace(cmd.Long),
		Aliases:     append([]string(nil), cmd.Aliases...),
		Examples:    splitRobotExamples(cmd.Example),
		Flags:       robotFlags(cmd.LocalNonPersistentFlags()),
		GlobalFlags: robotGlobalFlags(cmd),
		RobotInput:  robotInputDoc(cmd.CommandPath()),
		RobotOutput: robotOutputDoc(cmd.CommandPath()),
		Notes:       robotNotes(cmd.CommandPath()),
	}
	for _, child := range cmd.Commands() {
		if !child.IsAvailableCommand() && child.Name() != "help" {
			continue
		}
		doc.AvailableCommands = append(doc.AvailableCommands, robotCommandSummary{
			Name:  child.Name(),
			Use:   child.UseLine(),
			Short: child.Short,
		})
	}
	return doc
}

func robotGlobalFlags(cmd *cobra.Command) []robotFlag {
	flags := pflag.NewFlagSet(cmd.Name()+"-global", pflag.ContinueOnError)
	flags.AddFlagSet(cmd.InheritedFlags())
	if cmd.HasPersistentFlags() {
		flags.AddFlagSet(cmd.PersistentFlags())
	}
	return robotFlags(flags)
}

func robotFlags(flags *pflag.FlagSet) []robotFlag {
	if flags == nil {
		return nil
	}
	values := []robotFlag{}
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden {
			return
		}
		values = append(values, robotFlag{
			Name:      "--" + flag.Name,
			Shorthand: robotFlagShorthand(flag),
			Type:      flag.Value.Type(),
			Default:   flag.DefValue,
			Usage:     flag.Usage,
		})
	})
	return values
}

func robotFlagShorthand(flag *pflag.Flag) string {
	if flag.Shorthand == "" {
		return ""
	}
	return "-" + flag.Shorthand
}

func splitRobotExamples(value string) []string {
	lines := []string{}
	for _, line := range strings.Split(value, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

func robotInputDoc(command string) any {
	switch command {
	case "setup", "burpvalve setup":
		return map[string]any{
			"stdin_json": map[string]any{
				"target": "optional target project root; defaults to .",
			},
		}
	case "explain", "burpvalve explain":
		return map[string]any{
			"argument": "path/ref to a blocked report or attestation, or - to read structured JSON from stdin",
			"flags": map[string]string{
				"root": "repository root used to resolve attestation references",
				"json": "emit machine-readable explanation",
			},
		}
	case "init", "burpvalve init":
		return map[string]any{
			"stdin_json": map[string]any{
				"target":       "optional target project root; defaults to .",
				"targets":      "optional list of scaffold targets such as AGENTS.md, orchestrator, hooks, log, or attestations",
				"dogfood":      "optional boolean; add findings-log instructions when ORCHESTRATOR.md is created",
				"claude_route": "agent-symlink, orchestrator-skill, or none; controls CLAUDE.md/.claude/skills route choice",
				"skip": map[string]string{
					"agents_md":      "skip AGENTS.md creation",
					"attestations":   "skip backpressure/attestations",
					"backpressure":   "skip backpressure rule files",
					"beads":          "skip .beads initialization and verification",
					"bin":            "skip optional repo-local bin/burpvalve fallback",
					"claude_symlink": "skip CLAUDE.md symlink creation",
					"docs":           "skip docs files",
					"git_hooks":      "skip pre-commit hook and hooksPath",
					"hooks_path":     "skip git core.hooksPath configuration",
					"log":            "skip log files",
					"ntm":            "skip ntm quick registration and snapshot verification",
					"plans":          "skip plans files",
					"precommit":      "skip .githooks/pre-commit",
					"tool_docs":      "skip tools/burpvalve documentation",
				},
				"confirm":  "must be true for mutations unless --force is passed",
				"git_init": "when true, run git init before wiring hooks in a non-Git target",
			},
		}
	case "repair", "burpvalve repair":
		return map[string]any{
			"stdin_json": map[string]any{
				"target":          "optional target project root; defaults to .",
				"targets":         "optional list of scaffold targets such as AGENTS.md, orchestrator, hooks, log, or attestations",
				"claude_route":    "preserve, agent-symlink, orchestrator-skill, or none; controls Claude route repair behavior",
				"adopt_claude_md": "when true, import an unmarked regular CLAUDE.md into AGENTS.md before applying the selected repair route",
				"skip": map[string]string{
					"agents_md":      "skip AGENTS.md creation or append repair",
					"attestations":   "skip backpressure/attestations",
					"backpressure":   "skip backpressure rule files",
					"beads":          "skip .beads verification or creation",
					"bin":            "skip optional repo-local bin/burpvalve fallback",
					"claude_symlink": "skip CLAUDE.md symlink repair",
					"docs":           "skip docs files",
					"git_hooks":      "skip pre-commit hook and hooksPath",
					"hooks_path":     "skip git core.hooksPath configuration",
					"log":            "skip log files",
					"plans":          "skip plans files",
					"precommit":      "skip .githooks/pre-commit",
					"tool_docs":      "skip tools/burpvalve documentation",
				},
				"confirm":  "must be true for mutations unless --force is passed",
				"git_init": "when true, run git init before repairing hook wiring in a non-Git target",
			},
		}
	case "commit", "pre-commit", "burpvalve commit", "burpvalve pre-commit":
		return map[string]any{
			"stdin_json": map[string]any{
				"root":               "optional repository root; defaults to .",
				"feature":            "optional explicit atomic feature or bead id",
				"beads":              "optional array of delivery bead ids to record in attestation metadata",
				"bead_rationale":     "required when multiple bead ids intentionally share one staged payload",
				"responses_path":     "optional explicit legacy JSON verifier responses path; omitted commits auto-discover log/backpressure/responses/<staged-payload-hash>.json when present",
				"responses":          "alias for responses_path",
				"responses_template": "when true, print a JSON template for the current staged feature x condition matrix instead of running the gate",
				"agent":              "optional agent name recorded in artifacts",
				"model":              "optional model name recorded in artifacts",
			},
			"responses_file_schema": map[string]any{
				"atomicity": map[string]any{
					"one_feature_or_fix": "boolean; must be true for passing attestations",
					"message":            "why this staged diff is exactly one feature or bug fix",
				},
				"conditions": []map[string]any{{
					"condition_id":    "condition id from backpressure/manifest.yaml",
					"condition_file":  "condition file path; emitted by template for reviewer context",
					"verifier_policy": "policy from manifest: independent_required, main_agent_allowed, ci_allowed, human_allowed, or optional",
					"verifier": map[string]string{
						"kind":             "independent_subagent, main_agent, ci, human, none, or unknown",
						"agent":            "optional verifier agent/person/system name",
						"model":            "optional verifier model name",
						"runtime":          "optional runtime such as codex-cli, github-actions, or terminal",
						"separate_context": "true when an independent verifier ran outside the committing agent context",
						"transcript_ref":   "optional path/hash/reference to verifier transcript, not a full sensitive transcript by default",
						"evidence_ref":     "optional path/hash/reference to evidence artifact",
						"created_at":       "optional RFC3339 timestamp for the verifier evidence",
					},
					"subagent_confirmed": "legacy compatibility boolean; true maps to verifier.kind=independent_subagent only when verifier.kind is omitted",
					"subagent_model":     "legacy compatibility model/runtime name; prefer verifier.model",
					"verdict":            "pass, not_applicable, fail, or unknown",
					"message":            "required for not_applicable, fail, unknown, and subagent_confirmed=false",
					"evidence":           "array of evidence strings",
					"next_action":        "required for fail or unknown",
				}},
			},
		}
	case "hash", "burpvalve hash":
		return map[string]any{
			"flags": map[string]string{
				"root":   "repository root whose current index should be hashed",
				"staged": "required; hash the current staged payload with Burpvalve's canonical HashStagedPayload contract",
				"json":   "emit machine-readable hash reproduction data",
			},
		}
	case "account", "burpvalve account", "burpvalve account payload":
		return map[string]any{
			"stdin_json": map[string]any{
				"records": "optional C1 ownership records array; stdin records override --ownership-file records for the same unit/path/kind claim",
			},
			"flags": map[string]string{
				"root":              "repository root whose staged and optional untracked paths should be inspected",
				"ownership-file":    "optional JSON file using the C1 ownership records schema",
				"include-untracked": "include untracked paths with ignored/generated/covered/unowned classification",
				"include-beads":     "include display-only active Beads metadata; never creates ownership claims",
				"json":              "emit machine-readable ownership accounting output",
			},
		}
	case "prompts", "burpvalve prompts", "burpvalve prompts list":
		return map[string]any{
			"flags": map[string]string{
				"json": "emit machine-readable prompt list",
			},
			"output_schema": map[string]any{
				"prompts": "array of canonical prompt names, versions, descriptions, and variable metadata",
			},
		}
	case "burpvalve prompts show":
		return map[string]any{
			"argument": "stable prompt name; prompt names are a public API",
			"flags": map[string]string{
				"var":   "render variable assignment as key=value; repeat for multiple variables",
				"root":  "repository root used for docs/prompts local exports",
				"write": "export the rendered embedded prompt to docs/prompts/<name>.md with metadata and content hash",
				"force": "overwrite a locally modified prompt export deliberately",
				"json":  "emit exactly {name, version, variables: [{name, required, description}], body} unless --write emits export metadata",
			},
		}
	case "beads", "burpvalve beads", "burpvalve beads preflight":
		return map[string]any{
			"argument": "one or more bead ids to inspect",
			"flags": map[string]string{
				"root":           "repository root to inspect",
				"admin-only":     "classify work as issue-only/admin with no implementation commit evidence expected",
				"bead-rationale": "why multiple bead ids belong to one staged payload",
				"json":           "emit machine-readable dry-run report",
			},
		}
	case "burpvalve beads drift":
		return map[string]any{
			"flags": map[string]string{
				"root":   "repository root to inspect",
				"window": "lookback duration for recently closed beads; defaults to 168h",
				"json":   "emit machine-readable advisory report",
			},
		}
	case "burpvalve beads close":
		return map[string]any{
			"stdin_json": map[string]any{
				"root":           "optional repository root; defaults to .",
				"bead_ids":       "array of bead ids to close",
				"reason":         "required br close reason; reference the bead and feature, not a commit SHA",
				"feature":        "optional explicit feature id for the commit gate",
				"bead_rationale": "required when multiple delivery bead ids intentionally share one staged payload; optional for admin_only batches",
				"responses_path": "optional JSON verifier responses bound to the final staged payload",
				"responses":      "alias for responses_path",
				"admin_only":     "true only for tracker/admin closures with exclusively .beads staged payload; skips verifier/code attestation and may batch bead ids",
				"resume":         "continue from journal after recomputing reality",
				"confirm":        "must be true before git commit runs",
				"commit_message": "required with confirm:true for the final git commit step",
			},
		}
	case "lint", "burpvalve lint":
		return map[string]any{
			"stdin_json": map[string]any{
				"root": "optional repository root; defaults to .",
				"jobs": "optional positive integer for bounded command concurrency; defaults to 4 and is capped by declared command count",
			},
		}
	case "burpvalve lint init":
		return map[string]any{
			"stdin_json": map[string]any{
				"root":    "optional repository root; defaults to .",
				"detect":  "read-only detection/preview; always prevents mutation even with write or force",
				"write":   "request manifest and lint-rules writes after explicit confirmation",
				"preset":  "built-in preset selector: auto, go, node, or astro",
				"force":   "skip interactive confirmation only when write is true; force without write is read-only",
				"jobs":    "optional positive integer for lint runner concurrency metadata",
				"confirm": "must be true before robot mode mutates files",
			},
		}
	case "ci", "burpvalve ci":
		return map[string]any{
			"stdin_json": map[string]any{
				"root":    "optional repository root; defaults to .",
				"commit":  "optional commit SHA/ref to audit; validates evidence stored in that commit",
				"feature": "optional explicit atomic feature or bead id; with commit it is an assertion, not a selector",
			},
		}
	case "verifier", "burpvalve verifier", "burpvalve verifier prompts":
		return map[string]any{
			"flags": map[string]string{
				"root":      "repository root whose staged payload and backpressure manifest should be inspected",
				"feature":   "explicit atomic feature or bead id for staged changes; required when staged changes are ambiguous",
				"condition": "optional enabled condition id to emit a single verifier packet",
				"profile":   "handoff profile: native, ntm, hermes, or manual; all values are lower-case",
				"json":      "emit machine-readable prompt packets",
			},
		}
	case "burpvalve verifier begin":
		return map[string]any{
			"flags": map[string]string{
				"root":              "repository root whose staged payload and backpressure manifest should be inspected",
				"feature":           "explicit atomic feature or bead id for staged changes; required when staged changes are ambiguous",
				"one-feature":       "required confirmation that the staged payload is one atomic feature or bug fix",
				"atomicity-message": "required explanation for why the staged payload is atomic",
				"json":              "emit machine-readable result contract",
			},
		}
	case "burpvalve verifier submit":
		return map[string]any{
			"stdin_json": "one verifier response object for the named condition; must include condition_id, binding hashes, verifier provenance, verdict, evidence, and optional supplemental_verifiers/adjudication",
			"flags": map[string]string{
				"root":                "repository root whose staged payload and response file should be inspected",
				"feature":             "explicit atomic feature or bead id for staged changes; required when staged changes are ambiguous",
				"condition":           "enabled condition id to update in the bound response file",
				"responses":           "optional path to the begin-created response file; defaults to log/backpressure/responses/<staged-payload-hash>.json",
				"staged-payload-hash": "staged payload hash from the verifier packet submit command",
				"manifest-hash":       "manifest hash from the verifier packet submit command",
				"condition-file-hash": "condition file hash from the verifier packet submit command",
				"transcript":          "optional transcript path, or - to read transcript bytes after the stdin JSON object",
				"json":                "emit machine-readable result contract",
			},
		}
	case "attestations", "burpvalve attestations", "burpvalve attestations list", "burpvalve attestations latest":
		return map[string]any{
			"flags": map[string]string{
				"root":    "repository root to inspect",
				"status":  "all, pass, blocked, or malformed",
				"limit":   "maximum record count for list",
				"feature": "filter by feature id",
				"bead":    "filter by bead id when present",
			},
		}
	case "burpvalve attestations show":
		return map[string]any{
			"argument": "full path, filename, payload hash, or unambiguous prefix",
			"flags": map[string]string{
				"root": "repository root to inspect",
			},
		}
	case "config", "burpvalve config", "show", "burpvalve config show":
		return map[string]any{
			"stdin_json": map[string]any{
				"target": "optional target project root for project config lookup; defaults to .",
			},
			"output_schema": map[string]any{
				"global_path":   "global config file path",
				"global_found":  "whether the global config file exists",
				"project_path":  "project config file path",
				"project_found": "whether the project config file exists",
				"defaults":      "effective merged defaults",
				"sources":       "array of key/source pairs showing global or project origin",
				"orchestrator":  "effective ORCHESTRATOR.md defaults: defaults.init.orchestrator and defaults.repair.orchestrator when configured",
				"claude_route":  "effective Claude route defaults: defaults.init.claude_route and defaults.repair.claude_route when configured",
				"verifier":      "effective defaults.verifier block when configured, including authorization metadata and transcript preferences",
			},
		}
	case "burpvalve config init":
		return map[string]any{
			"stdin_json": map[string]any{
				"scope":   "global or project",
				"target":  "optional target project root when scope is project",
				"confirm": "must be true for mutations unless --force is passed",
				"config": map[string]any{
					"schema_version": "versioned Burpvalve config schema",
					"defaults": map[string]any{
						"init": map[string]string{
							"dogfood":      "boolean; add findings-log instructions to ORCHESTRATOR.md when dogfood mode is enabled",
							"orchestrator": "off or orchestrator-md; controls ORCHESTRATOR.md only, not the Claude route",
							"claude_route": "agent-symlink, orchestrator-skill, or none; controls CLAUDE.md/.claude/skills route choice",
						},
						"repair": map[string]string{
							"orchestrator": "boolean; repair touches ORCHESTRATOR.md only when true or when explicitly targeted",
							"claude_route": "preserve, agent-symlink, orchestrator-skill, or none; controls Claude route repair defaults",
						},
						"verifier": map[string]string{
							"authorized":             "standing repo-owner authorization for read-only verifier subagent spawning; true or false can be written to grant or revoke",
							"authorized_at":          "RFC3339 timestamp recording when the authorization decision was made",
							"authorization_scope":    "scope text such as repo:/path/to/repo; this is policy metadata, never verifier evidence",
							"spawn_method":           "native, ntm, hermes, or manual",
							"default_model":          "optional default verifier model name",
							"condition_models":       "optional map of condition id to verifier model name",
							"read_only_tools":        "optional boolean preference for read-only verifier tools",
							"max_parallel_verifiers": "optional verifier fanout limit from 1 to 32",
							"transcript_dir":         "optional repo-relative verifier transcript directory",
							"transcripts":            "summary, full, or committed",
						},
					},
				},
			},
		}
	case "completion", "burpvalve completion":
		return map[string]any{
			"stdin_json": map[string]any{
				"target": "optional target project root for project config lookup; defaults to .",
				"shell":  "optional shell name: bash, zsh, fish, or powershell; overrides config defaults",
			},
		}
	case "completion verify", "burpvalve completion verify":
		return map[string]any{
			"stdin_json": map[string]any{
				"target": "optional target project root for project config lookup; defaults to .",
				"shell":  "optional shell name: bash, zsh, fish, or powershell; overrides config defaults",
			},
		}
	default:
		return nil
	}
}

func robotOutputDoc(command string) any {
	switch command {
	case "setup", "burpvalve setup":
		return map[string]string{
			"schema_version":      "setup report schema version",
			"command":             "setup",
			"status":              "ready or blocked",
			"readiness_severity":  "ready, warning, or blocked",
			"target_root":         "absolute inspected repository path",
			"command_path":        "burpvalve command found on PATH when available",
			"repo_bin_path":       "repo-local bin/burpvalve fallback path when present or configured",
			"hook_command_source": "path, repo-local, repo-local-conflict, or missing",
			"repo_local_binary":   "repo-local binary provenance facts: hook_command_source, repo_local_path, repo_local_exists, repo_local_ignored, path_command, freshness_status, comparison_basis, and warning_code",
			"config":              "global/project config paths, found flags, and value sources",
			"checks":              "individual required and optional readiness checks",
			"next_steps":          "exact recovery commands when Burpvalve knows them",
			"mutating":            "always false for setup",
		}
	case "explain", "burpvalve explain":
		return map[string]string{
			"schema_version":  "explanation schema version",
			"command":         "explain",
			"input_type":      "setup, init, repair, lint, commit, attestation, blocked_report, or unknown",
			"status":          "explained or error",
			"summary":         "short statement of what happened",
			"why_it_matters":  "why the state affects readiness or commit safety",
			"fatal":           "whether the input describes a blocking condition",
			"next_steps":      "commands or decisions the caller should take next",
			"blockers":        "condition/check level blockers and evidence gaps",
			"source":          "path/ref/stdin source metadata",
			"original_status": "status from the original structured input when known",
		}
	case "init", "repair", "burpvalve init", "burpvalve repair":
		return map[string]string{
			"schema_version":      "init or repair result schema version",
			"command":             "init or repair",
			"status":              "applied, partial_success, or blocked",
			"target_root":         "absolute target repository path",
			"claude_route":        "resolved Claude route for this invocation: agent-symlink, orchestrator-skill, none, or preserve for repair previews",
			"claude_route_source": "input, prompt, project, global, or default source for the resolved route",
			"created":             "created scaffold paths",
			"repaired":            "repaired scaffold paths when content was appended or restored",
			"preserved":           "existing or skipped paths preserved by safe scaffold handling",
			"skipped":             "explicitly skipped scaffold pieces",
			"conflicts":           "conflicts that require an explicit human/agent decision before mutation can proceed",
			"next_steps":          "structured recovery actions when blocked or partially successful",
			"config":              "global/project config paths, found flags, and value sources",
			"partial_success":     "true when some mutation succeeded before a conflict stopped the command",
			"repo_local_binary":   "repair/setup hook provenance facts when relevant",
			"checks":              "repair preview readiness checks when repair is run non-mutating",
			"planned_changes":     "repair preview planned changes when repair is run non-mutating",
		}
	case "commit", "pre-commit", "burpvalve commit", "burpvalve pre-commit":
		return map[string]string{
			"schema_version":      "commit result schema version",
			"command":             "commit",
			"status":              "passed, blocked, or attestation_written",
			"message":             "human-readable gate result or blocker",
			"fatal":               "whether the blocker prevents proceeding",
			"warnings":            "legacy response-file notices and nonfatal compatibility warnings",
			"next_steps":          "exact recovery or staging steps",
			"artifact_path":       "passing attestation path for the current staged payload",
			"blocked_report_path": "blocked report path when the gate writes one",
			"responses_path":      "auto-discovered or explicit response file path used or expected",
			"plan":                "staged payload, manifest, feature, and condition matrix used for this gate",
		}
	case "ci", "burpvalve ci":
		return map[string]string{
			"status":               "passed or blocked",
			"message":              "validation result",
			"artifact_path":        "attestation path validated",
			"artifact_paths":       "artifact paths read from the staged payload, latest commit, or target commit",
			"audit_commit":         "commit SHA/ref audited when --commit is used",
			"attestation":          "artifact path, tool version, payload hash, manifest hash, and feature id",
			"condition_provenance": "per-condition verdict, verifier policy, verifier provenance, and condition file hash",
			"plan":                 "payload, manifest, feature, and condition matrix used for validation",
		}
	case "hash", "burpvalve hash":
		return map[string]string{
			"schema_version":               "hash result schema version",
			"command":                      "hash",
			"status":                       "completed or blocked",
			"message":                      "human-readable result or blocker",
			"fatal":                        "whether the blocker prevents hash reproduction",
			"staged_payload_hash":          "canonical Burpvalve staged payload hash for the current index",
			"staged_payload":               "hash-included staged path records; together with hash_excluded_staged_payload, every staged path appears exactly once",
			"included_paths":               "hash-included path list used in the payload hash",
			"hash_excluded_staged_payload": "generated staged path records reviewed but excluded from the payload hash; together with staged_payload, every staged path appears exactly once",
			"excluded_paths":               "generated staged paths excluded from HashStagedPayload",
			"generated_path_prefixes":      "directories that can contain generated evidence excluded from the payload hash",
			"warning":                      "canonical warning that naive git diff hashing is not equivalent",
		}
	case "account", "burpvalve account", "burpvalve account payload":
		return map[string]string{
			"schema_version":          "ownership accounting result schema version",
			"command":                 "account payload",
			"status":                  "completed or blocked",
			"mutating":                "always false; account payload is read-only",
			"staged":                  "staged paths classified as owned, shared_declared, conflict, unowned, generated_exception, or covered_exception",
			"untracked":               "optional untracked paths classified as ignored_untracked, generated_exception, covered_exception, or unowned",
			"beads":                   "optional display-only active Beads metadata when --include-beads is set; explicit ownership records stay authoritative",
			"summary":                 "counts for staged, untracked, owned, conflicts, unowned, generated exceptions, ignored paths, and covered exceptions",
			"generated_path_prefixes": "directories treated as generated Burpvalve evidence paths",
		}
	case "completion", "burpvalve completion":
		return map[string]string{
			"schema_version":  "completion robot output schema version",
			"command":         "completion",
			"target":          "target used for project config lookup",
			"shell":           "effective shell",
			"shell_source":    "input, argument, project, global, or detected",
			"config":          "global/project config paths, found flags, and value sources",
			"completion_path": "effective completion script path",
			"rc_file":         "startup/profile file when applicable",
			"install_command": "explicit install command for the effective shell/path",
			"script_command":  "explicit raw script command for completion managers",
			"next_steps":      "agent-safe guidance for what to do next",
		}
	case "completion verify", "burpvalve completion verify":
		return map[string]string{
			"schema_version":        "completion verification schema version",
			"command":               "completion verify",
			"status":                "ready or action_needed",
			"mutating":              "always false",
			"shell":                 "effective shell",
			"shell_source":          "flag, input, project, global, or detected",
			"detection_evidence":    "facts used for shell detection when detected",
			"config":                "global/project config paths, found flags, and value sources",
			"command_path":          "burpvalve command path when found",
			"command_origin":        "path, path-and-repo-local, repo-local, or missing",
			"command_on_path":       "whether burpvalve resolves from PATH",
			"completion_path":       "completion script path checked",
			"completion_exists":     "whether the completion script path exists",
			"completion_looks_ok":   "whether the script contents look compatible with the shell",
			"rc_path":               "startup/profile file checked when applicable",
			"rc_update_present":     "whether Burpvalve rc wiring exists when required",
			"bin_dir":               "configured/default command shim directory",
			"bin_dir_exists":        "whether the shim directory exists",
			"path_contains_bin_dir": "whether current PATH includes the shim directory",
			"next_steps":            "exact recovery commands when something is missing",
		}
	case "version", "burpvalve version":
		return map[string]string{"version": "Burpvalve build version"}
	case "config", "burpvalve config", "show", "burpvalve config show":
		return map[string]string{
			"schema_version": "config report schema version",
			"global_path":    "global config path",
			"global_found":   "whether global config exists",
			"project_path":   "project config path for target",
			"project_found":  "whether project config exists",
			"defaults":       "merged effective defaults",
			"sources":        "key/source pairs showing global or project origin",
			"orchestrator":   "effective defaults.init.orchestrator and defaults.repair.orchestrator source rows when configured",
			"verifier":       "effective defaults.verifier settings and source rows when configured",
		}
	case "burpvalve config init":
		return map[string]string{
			"schema_version": "config report schema version",
			"command":        "config init",
			"status":         "written",
			"scope":          "global or project",
			"path":           "written config path",
			"config":         "normalized config that was written",
		}
	case "attestations", "burpvalve attestations", "burpvalve attestations list", "burpvalve attestations latest", "burpvalve attestations show":
		return map[string]string{
			"schema_version":     "query response schema version for list responses or record schema version for individual records",
			"records":            "list of normalized attestation records for list",
			"artifact_type":      "passing_attestation or blocked_report",
			"status":             "pass, blocked, or malformed",
			"path":               "project-relative artifact path",
			"feature_ids":        "feature ids recorded in the artifact",
			"bead_ids":           "bead ids recorded in the artifact when present",
			"payload_hash":       "staged payload hash recorded by the artifact",
			"condition_verdicts": "condition id, verdict, verifier policy, and verifier provenance summary",
			"parse_warnings":     "malformed artifact warnings included by list; show returns a JSON error instead",
		}
	case "beads", "burpvalve beads", "burpvalve beads preflight":
		return map[string]string{
			"schema_version":          "beads preflight report schema version",
			"command":                 "beads preflight",
			"status":                  "ready, action_needed, or blocked",
			"mutating":                "always false",
			"bead_ids":                "delivery bead ids inspected",
			"admin_only":              "whether commit evidence is not expected",
			"classification":          "staged-payload classification: preview, admin, or delivery",
			"coupled_work_rationale":  "why multiple bead ids share one staged payload",
			"staged_payload_paths":    "currently staged payload paths, if any",
			"non_beads_payload_paths": "staged paths outside .beads, if any",
			"beads":                   "br show summaries for each bead id",
			"warnings":                "state or ordering issues to resolve before final gate",
			"next_steps":              "exact dry-run delivery sequence",
		}
	case "burpvalve beads drift":
		return map[string]string{
			"schema_version": "beads drift report schema version",
			"command":        "beads drift",
			"status":         "clean, possible, or warning",
			"mutating":       "always false",
			"fatal":          "always false; findings are advisory",
			"window":         "lookback duration for recently closed beads",
			"since":          "RFC3339 cutoff used for br closed_at filtering",
			"dirty_tree":     "whether git status reports staged, unstaged, or untracked paths",
			"dirty_paths":    "current dirty paths from git status",
			"checked_beads":  "recently closed beads and their attestation or commit-message matches",
			"findings":       "possible or informational unmatched closure findings",
			"warnings":       "malformed/missing closed_at, br, git, or attestation query warnings",
			"next_steps":     "structured advisory actions; never br close, git add, or git commit",
		}
	case "burpvalve beads close":
		return map[string]string{
			"schema_version":  "beads close result schema version",
			"command":         "beads close",
			"status":          "completed, blocked, attestation_written_unstaged, awaiting_commit_confirmation, or failed",
			"fatal":           "whether the current stop blocks safe resume",
			"partial_success": "true when a mutating step already succeeded before this stop",
			"journal_path":    "log/backpressure/closures/<bead-id>.json",
			"steps":           "ordered state-machine steps with command, stdout/stderr refs, status, and timestamps",
			"next_steps":      "structured recovery objects with id, message, exact command, and fatal",
			"admin_only":      "admin closures stage/commit tracker changes only and never request verifier/code attestation evidence",
			"bead_rationale":  "multi-bead delivery rationale recorded into the final attestation",
		}
	case "burpvalve lint init":
		return map[string]string{
			"schema_version":        "lint init result schema version",
			"command":               "lint init",
			"status":                "detected, confirmation_required, or written",
			"mutating":              "true only after an explicitly confirmed write",
			"detection":             "Go and Node/Astro detection facts from the repository",
			"coverage":              "full, partial, or none plus scoped root details",
			"proposed_commands":     "candidate lint_commands entries before mutation",
			"manifest_update":       "preview of backpressure/manifest.yaml changes including before and after text",
			"lint_rules_update":     "preview of marked lint-rules recommendations changes",
			"confirmation_required": "true when write was requested without force or robot confirm",
			"warnings":              "read-only force/detect notes and missing command warnings",
			"next_steps":            "structured recovery objects for blocked confirmation paths",
		}
	case "prompts", "burpvalve prompts", "burpvalve prompts list":
		return map[string]string{
			"prompts": "canonical prompt bank entries with stable name, version, description, and variable metadata",
		}
	case "burpvalve prompts show":
		return map[string]string{
			"name":             "stable public prompt name",
			"version":          "embedded prompt version",
			"variables":        "array of variable metadata objects with name, required, and description",
			"body":             "rendered prompt body",
			"write_output":     "with --write, JSON emits export path, prompt_name, content_hash, written, divergent, and local_modified",
			"local_divergence": "without --write, stderr warns when docs/prompts/<name>.md differs from the embedded canonical prompt",
		}
	case "verifier", "burpvalve verifier", "burpvalve verifier prompts":
		return map[string]string{
			"schema_version":               "verifier prompt response schema version",
			"command":                      "verifier prompts",
			"profile":                      "native, ntm, hermes, or manual",
			"feature":                      "detected or explicit feature metadata",
			"manifest_hash":                "hash of backpressure/manifest.yaml used for packet binding",
			"staged_payload_hash":          "canonical Burpvalve staged payload hash used for packet binding",
			"staged_payload":               "hash-included staged path records including path, status, git_status, and old_path for renames",
			"hash_excluded_staged_payload": "generated staged paths listed for review but excluded from the payload hash",
			"generated_path_prefixes":      "generated prefixes excluded from HashStagedPayload",
			"staged_payload_details":       "bounded staged content excerpts and generated/hash-included labels",
			"packets":                      "one bound packet per feature x condition cell after filters",
			"condition_file_hash":          "hash of the condition file bound to the packet",
			"condition_content":            "inline condition file contents for the verifier",
			"hash_reproduction":            "canonical HashStagedPayload reproduction contract",
			"verifier_policy":              "policy from backpressure/manifest.yaml for the condition",
			"authorization":                "recorded verifier authorization metadata and D7 warning; never per-cell evidence",
			"read_only_expectation":        "instruction that verifier agents must not mutate the repo",
			"success_criteria":             "verdict semantics including pass vs not_applicable and condition-specific rules",
			"response_schema":              "condition response object compatible with burpvalve commit --responses",
			"response_schema_json":         "human-readable JSON schema including supplemental_verifiers and adjudication",
			"submit_command":               "prefilled burpvalve verifier submit command with binding arguments",
			"prompt":                       "copyable human prompt for the verifier cell",
		}
	case "burpvalve verifier begin":
		return map[string]string{
			"schema_version":       "verifier begin result schema version",
			"command":              "verifier begin",
			"status":               "responses_written or blocked",
			"message":              "human-readable result or blocker",
			"fatal":                "whether the blocker prevents proceeding",
			"next_steps":           "exact recovery steps when blocked",
			"responses_path":       "log/backpressure/responses/<staged-payload-hash>.json when payload is available",
			"staged_payload_hash":  "canonical staged payload hash bound into the response file",
			"manifest_hash":        "manifest hash bound into the response file",
			"plan":                 "same staged payload and condition matrix used by commit/verifier prompts",
			"response_file_schema": "written JSON contains atomicity, binding, and conditions initialized to unknown",
		}
	case "burpvalve verifier submit":
		return map[string]string{
			"schema_version":      "verifier submit result schema version",
			"command":             "verifier submit",
			"status":              "responses_updated or blocked",
			"message":             "human-readable result or blocker",
			"fatal":               "whether the blocker prevents proceeding",
			"next_steps":          "exact recovery steps when blocked",
			"warnings":            "replacement or duplicate supplemental-verifier warnings",
			"responses_path":      "bound response file that was updated",
			"condition_id":        "condition cell updated",
			"staged_payload_hash": "current staged payload hash required by the response binding",
			"manifest_hash":       "manifest hash required by the response binding",
			"transcript_ref":      "repo-relative transcript record when --transcript is supplied",
			"plan":                "same staged payload and condition matrix used by commit/verifier prompts",
		}
	case "burpvalve verifier doctor":
		return map[string]string{
			"schema_version": "verifier doctor result schema version",
			"command":        "verifier doctor",
			"status":         "completed or blocked",
			"message":        "human-readable report summary",
			"report_only":    "always true; this command never writes, repairs, stages, or commits",
			"checks":         "known runtime config path checks for Claude Code, Codex, and NTM",
			"paths":          "candidate config paths with exists, supported, and message fields",
			"supported":      "false when no recognized readable config format was found for the runtime",
			"limits":         "recognized subagent_limit and depth_limit values or unknown statuses",
			"warnings":       "malformed, unsupported, unreadable, or unknown-format findings",
			"next_steps":     "non-mutating follow-up guidance; never exact unsupported config edits",
		}
	default:
		return "JSON on stdout; diagnostics and human summaries may appear on stderr"
	}
}

func robotNotes(command string) []string {
	switch command {
	case "setup", "burpvalve setup":
		return []string{
			"setup is inspection-only and never initializes Git, repairs files, installs completions, or changes config",
			"setup reports ORCHESTRATOR.md only when defaults.init.orchestrator=orchestrator-md makes it required",
			"use next_steps recovery commands when status is blocked or readiness_severity is warning",
			"parse command_path, repo_bin_path, and hook_command_source instead of inferring hook readiness from prose",
		}
	case "explain", "burpvalve explain":
		return []string{
			"explain is read-only and never repairs, stages, commits, or changes config",
			"prefer piping structured --json output into explain instead of scraping human text",
			"when Burpvalve cannot know the fix, explain says what decision the user must make",
		}
	case "init", "repair", "burpvalve init", "burpvalve repair":
		return []string{
			"--robots never opens Bubble Tea prompts",
			"mutating commands require --force or stdin JSON with confirm=true",
			"skip fields match the --no-* flags and can be combined with CLI flags",
			"ORCHESTRATOR.md is not part of the standard scaffold; request target orchestrator or configure defaults.init.orchestrator=orchestrator-md",
			"dogfood=true adds findings-log instructions to ORCHESTRATOR.md when that target is created",
		}
	case "burpvalve config init":
		return []string{
			"--robots never opens Bubble Tea prompts",
			"mutating commands require --force or stdin JSON with confirm=true",
			"config JSON is validated before writing and unknown fields are rejected",
			"defaults.init.orchestrator accepts off or orchestrator-md and only controls ORCHESTRATOR.md",
			"defaults.init.claude_route accepts agent-symlink, orchestrator-skill, or none and controls the Claude route separately from ORCHESTRATOR.md",
			"defaults.init.dogfood is a boolean and only adds findings-log instructions to the orchestrator contract",
			"defaults.repair.orchestrator is a boolean and only controls ORCHESTRATOR.md repair defaults",
			"defaults.repair.claude_route accepts preserve, agent-symlink, orchestrator-skill, or none and controls Claude route repair defaults",
			"defaults.verifier.authorization_scope is policy metadata and is never verifier evidence",
		}
	case "completion", "burpvalve completion":
		return []string{
			"robot mode emits structured JSON, not a raw completion script",
			"precedence is stdin shell or command argument, then project config, then global config, then detected shell",
			"use script_command when a completion manager needs the raw shell script",
		}
	case "completion verify", "burpvalve completion verify":
		return []string{
			"read-only verification; never writes completion files, rc files, shims, or config",
			"precedence is stdin shell or --shell, then project config, then global config, then detected shell",
			"use next_steps to decide whether to install, source/restart the shell, ask the user, or stop",
		}
	case "attestations", "burpvalve attestations", "burpvalve attestations list", "burpvalve attestations latest", "burpvalve attestations show":
		return []string{
			"agents should use --json and parse the stable query schema instead of scraping human tables",
			"attestation query commands are read-only and never repair, stage, or delete artifacts",
		}
	case "beads", "burpvalve beads", "burpvalve beads preflight":
		return []string{
			"beads preflight is read-only and never closes beads, runs br sync, stages files, commits files, or writes attestations",
			"use --bead on burpvalve commit to record delivery metadata after the exact payload is staged",
			"multiple bead ids require a rationale so coupled work stays explicit",
		}
	case "hash", "burpvalve hash":
		return []string{
			"hash --staged is read-only and never writes files, stages files, commits files, or runs hooks",
			"use this helper to reproduce staged_payload_hash from verifier packets with the same HashStagedPayload path used by commit and verifier prompts",
			"staged_payload is hash-included only; every staged path appears exactly once across staged_payload and hash_excluded_staged_payload",
			"generated evidence JSON is listed in hash_excluded_staged_payload but excluded from the staged payload hash",
			"do not substitute git diff --cached --binary | sha256sum for this command",
		}
	case "burpvalve beads drift":
		return []string{
			"beads drift is read-only and advisory; it never closes beads, stages files, writes files, commits files, or blocks br",
			"possible drift means a recently closed bead has no matching attestation bead id or commit message while the tree is dirty",
			"clean-tree unmatched closures are informational and do not claim the current tree contains that bead's work",
			"malformed or missing closed_at values are reported as warnings instead of crashing the command",
		}
	case "burpvalve beads close":
		return []string{
			"beads close is mutating and follows the numbered safe order from docs/beads-delivery-workflow.md",
			"delivery multi-bead closures require --bead-rationale and close all beads before one gate so .beads/issues.jsonl is attested",
			"admin-only closures require exclusively .beads staged paths, may batch bead ids, and skip verifier/code attestation evidence",
			"it stages only .beads/issues.jsonl and the attestation path named by Burpvalve; it never stages arbitrary dirty files",
			"the attestation-written bounce is a normal nonfatal state; resume stages the named attestation and reruns the gate",
			"git commit runs only with --yes or robots confirm=true plus a commit message",
		}
	case "burpvalve lint init":
		return []string{
			"lint init --detect is always read-only, even when combined with --write or --force",
			"lint init --write mutates only after terminal confirmation, --force, or robots confirm=true",
			"--force without --write is read-only and only records a warning in JSON output",
			"robot mode never opens Bubble Tea prompts and never mutates unless stdin JSON includes confirm=true",
			"--jobs records the requested concurrency limit for command surfaces",
		}
	case "prompts", "burpvalve prompts", "burpvalve prompts list", "burpvalve prompts show":
		return []string{
			"burpvalve prompts serves canonical orchestrator templates embedded in the binary",
			"prompt names are stable public API; renames or removals are breaking changes",
			"burpvalve verifier prompts is different: it generates staged-payload verifier packets with binding hashes",
			"rendering substitutes only declared variables and does not run shell commands",
		}
	case "verifier", "burpvalve verifier", "burpvalve verifier prompts":
		return []string{
			"burpvalve verifier prompts is read-only and does not spawn subagents",
			"recorded authorization permits spawning only; it is never per-cell verifier evidence",
			"spawn read-only verifier subagents yourself when your runtime permits repo-level authorization",
			"packets include binding hashes, hash reproduction text, condition content, and full staged path accounting",
			"Do not fabricate subagent confirmation; route real verifier output into the response_schema for burpvalve commit --responses",
		}
	case "burpvalve verifier begin":
		return []string{
			"verifier begin writes a hash-keyed response file for the current staged payload",
			"--one-feature and --atomicity-message are required; submit commands must not rewrite atomicity",
			"the response file is bound to staged_payload_hash, manifest_hash, and per-condition file hashes",
			"run verifier prompts after begin, then record real verifier evidence into the response file before commit",
		}
	case "burpvalve verifier submit":
		return []string{
			"verifier submit updates exactly one condition in the bound response file",
			"the submitted JSON and command flags must match the current staged payload, manifest, and condition file hashes",
			"evidence is required for every verdict, including pass; authorization text alone is rejected",
			"supplemental_verifiers and adjudication merge into the same condition without rewriting begin atomicity",
			"duplicate supplemental verifier submissions replace deterministically and emit a warning",
		}
	case "burpvalve verifier doctor":
		return []string{
			"verifier doctor is report-only and never writes runtime config, repo files, stages, or commits",
			"unknown or malformed config formats are reported with supported=false",
			"the report may name known config paths and recognized limit values, but it must not invent exact edits for unsupported formats",
			"this report is runtime guidance only and is never per-cell verifier evidence",
		}
	default:
		return nil
	}
}

func doubleSpaceIndex(value string) int {
	for i := 0; i < len(value)-1; i++ {
		if value[i] == ' ' && value[i+1] == ' ' {
			return i
		}
	}
	return -1
}

func doubleSpaceEnd(value string, start int) int {
	end := start
	for end < len(value) && value[end] == ' ' {
		end++
	}
	return end
}

func newCompletionCommand(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Print shell completion setup for your shell.",
		Long: `Set up shell completions for bash, zsh, fish, or PowerShell.
With no shell argument, Burpvalve opens an interactive installer when running
in a terminal. Outside a terminal it prints the install commands instead.
Pass a shell name when you want the raw completion script for redirects.`,
		Example: `  burpvalve completion
  burpvalve completion install --shell zsh --update-rc --force
  burpvalve completion zsh > ~/.zsh/completions/_burpvalve
  burpvalve completion bash > ~/.local/share/bash-completion/completions/burpvalve`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return fail(2, "completion accepts at most one shell name; try: burpvalve completion")
			}
			if len(args) == 1 {
				if _, ok := normalizeCompletionShell(args[0]); !ok {
					return fail(2, "unknown shell %q; expected bash, zsh, fish, or powershell", args[0])
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if robotsMode {
				return runCompletionRobots(cmd, root, args)
			}
			if len(args) == 1 {
				shell := ""
				var ok bool
				shell, ok = normalizeCompletionShell(args[0])
				if !ok {
					return fail(2, "unknown shell %q; expected bash, zsh, fish, or powershell", args[0])
				}
				return printCompletion(root, cmd.OutOrStdout(), shell)
			}
			effectiveConfig, err := bvconfig.Load(".")
			if err != nil {
				return fail(2, "%v", err)
			}
			selection, err := configuredCompletionShellSelection(effectiveConfig)
			if err != nil {
				return err
			}
			if isInteractiveTerminal(os.Stdin, os.Stdout) {
				return runCompletionWizard(root, selection, effectiveConfig.File.Defaults)
			}
			return printCompletionGuide(cmd.OutOrStdout(), selection, effectiveConfig.File.Defaults, shouldColorWriter(cmd.OutOrStdout()))
		},
	}
	cmd.AddCommand(newCompletionInstallCommand(root), newCompletionVerifyCommand())
	return cmd
}

type completionShellSelection struct {
	Shell  string
	Source string
}

func configuredCompletionShellSelection(effective bvconfig.Effective) (completionShellSelection, error) {
	if effective.File.Defaults.Shell != "" {
		shell, ok := normalizeCompletionShell(effective.File.Defaults.Shell)
		if !ok {
			return completionShellSelection{}, fail(2, "config default shell %q is invalid; expected bash, zsh, fish, or powershell", effective.File.Defaults.Shell)
		}
		source := effective.Sources["defaults.shell"]
		if source == "" {
			source = "config"
		}
		return completionShellSelection{Shell: shell, Source: source}, nil
	}
	shell, err := detectCompletionShell()
	if err != nil {
		return completionShellSelection{}, err
	}
	return completionShellSelection{Shell: shell, Source: "detected"}, nil
}

func configuredCompletionShell(defaults bvconfig.Defaults) (string, error) {
	if defaults.Shell != "" {
		shell, ok := normalizeCompletionShell(defaults.Shell)
		if !ok {
			return "", fail(2, "config default shell %q is invalid; expected bash, zsh, fish, or powershell", defaults.Shell)
		}
		return shell, nil
	}
	return detectCompletionShell()
}

func printCompletion(root *cobra.Command, out io.Writer, shell string) error {
	switch shell {
	case "bash":
		return root.GenBashCompletion(out)
	case "zsh":
		return root.GenZshCompletion(out)
	case "fish":
		return root.GenFishCompletion(out, true)
	case "powershell":
		return root.GenPowerShellCompletion(out)
	default:
		return fail(2, "unknown shell %q; expected bash, zsh, fish, or powershell", shell)
	}
}

type completionInstallOptions struct {
	shell    string
	path     string
	rcFile   string
	binDir   string
	updateRC bool
	noPath   bool
	force    bool
}

type completionVerifyOptions struct {
	target   string
	shell    string
	path     string
	rcFile   string
	binDir   string
	updateRC bool
	noPath   bool
	json     bool
}

func newCompletionInstallCommand(root *cobra.Command) *cobra.Command {
	var opts completionInstallOptions
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install shell completions without the setup wizard.",
		Long: `Install shell completions for bash, zsh, fish, or PowerShell.
This writes the completion script to a file. Use --update-rc when your shell
also needs a shell startup file change. Without --force, install asks for final
confirmation before changing files.`,
		Example: `  burpvalve completion install --shell zsh --update-rc
  burpvalve completion install --shell fish --force
  burpvalve completion install --shell zsh --path ~/.zsh/completions/_burpvalve --update-rc --force`,
		RunE: func(cmd *cobra.Command, args []string) error {
			effectiveConfig, err := bvconfig.Load(".")
			if err != nil {
				return fail(2, "%v", err)
			}
			shell := ""
			if opts.shell != "" {
				normalized, ok := normalizeCompletionShell(opts.shell)
				if !ok {
					return fail(2, "unknown shell %q; expected bash, zsh, fish, or powershell", opts.shell)
				}
				shell = normalized
			} else {
				shell, err = configuredCompletionShell(effectiveConfig.File.Defaults)
				if err != nil {
					return err
				}
			}
			if opts.path == "" {
				opts.path = effectiveConfig.File.Defaults.Completion.Path
			}
			if opts.rcFile == "" {
				opts.rcFile = effectiveConfig.File.Defaults.Completion.RCFile
			}
			if opts.binDir == "" {
				opts.binDir = effectiveConfig.File.Defaults.BinDir
			}
			if !opts.updateRC && effectiveConfig.File.Defaults.Completion.UpdateRC != nil {
				opts.updateRC = *effectiveConfig.File.Defaults.Completion.UpdateRC
			}
			plan, err := completionInstallPlanFor(shell, opts.path, opts.rcFile, opts.updateRC, !opts.noPath, opts.binDir)
			if err != nil {
				return err
			}
			printCompletionInstallPlan(cmd.OutOrStdout(), plan, shouldColorWriter(cmd.OutOrStdout()))
			if err := confirmCompletionInstall(plan, opts.force); err != nil {
				return err
			}
			result, err := applyCompletionInstall(root, plan)
			result.Config = setupConfigSummary(effectiveConfig)
			printCompletionInstallResult(cmd.OutOrStdout(), result, shouldColorWriter(cmd.OutOrStdout()))
			return err
		},
	}
	cmd.Flags().StringVar(&opts.shell, "shell", "", "shell to install for: bash, zsh, fish, or powershell")
	cmd.Flags().StringVar(&opts.path, "path", "", "completion file path; defaults to the standard path for the shell")
	cmd.Flags().StringVar(&opts.rcFile, "rc-file", "", "shell startup/profile file to update when --update-rc is used")
	cmd.Flags().StringVar(&opts.binDir, "bin-dir", "", "directory for the burpvalve command shim; defaults to ~/.local/bin")
	cmd.Flags().BoolVar(&opts.updateRC, "update-rc", false, "also update the shell startup/profile file")
	cmd.Flags().BoolVar(&opts.noPath, "no-path", false, "install completions only; do not create a command shim or PATH entry")
	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "skip confirmation and install directly")
	return cmd
}

func newCompletionVerifyCommand() *cobra.Command {
	var opts completionVerifyOptions
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Check shell completion and PATH wiring without changing files.",
		Long: `Check whether Burpvalve shell completions and command PATH wiring are
installed for bash, zsh, fish, or PowerShell. This command is read-only. It
prints what exists, what is missing, and the exact next command to run.`,
		Example: `  burpvalve completion verify
  burpvalve completion verify --shell zsh --json
  burpvalve completion verify --shell fish --path ~/.config/fish/completions/burpvalve.fish --no-path`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if robotsMode {
				var input robotCompletionInput
				if err := decodeRobotInput(cmd.InOrStdin(), &input); err != nil {
					return err
				}
				if input.Target != "" {
					opts.target = input.Target
				}
				if input.Shell != "" {
					opts.shell = input.Shell
				}
				opts.json = true
			}
			report, err := buildCompletionVerifyReport(opts)
			if err != nil {
				return err
			}
			if opts.json || robotsMode {
				return encodeJSON(cmd.OutOrStdout(), report, "encode completion verify report")
			}
			fmt.Fprint(cmd.OutOrStdout(), report.TextWithOptions(scaffold.TextOptions{Color: shouldColorWriter(cmd.OutOrStdout())}))
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.target, "target", ".", "target project root for project config lookup")
	cmd.Flags().StringVar(&opts.shell, "shell", "", "shell to verify: bash, zsh, fish, or powershell")
	cmd.Flags().StringVar(&opts.path, "path", "", "completion file path; defaults to the standard path for the shell")
	cmd.Flags().StringVar(&opts.rcFile, "rc-file", "", "shell startup/profile file to check when --update-rc is used")
	cmd.Flags().StringVar(&opts.binDir, "bin-dir", "", "directory expected to contain the burpvalve command shim")
	cmd.Flags().BoolVar(&opts.updateRC, "update-rc", false, "require shell startup/profile wiring to be present")
	cmd.Flags().BoolVar(&opts.noPath, "no-path", false, "do not require burpvalve to be on PATH or a command shim directory to exist")
	cmd.Flags().BoolVar(&opts.json, "json", false, "print machine-readable JSON")
	return cmd
}

type completionInstallPlan struct {
	Shell             string
	Path              string
	RCFile            string
	UpdateRC          bool
	EnsureCommandPath bool
	BinDir            string
	ShimPath          string
	PathRCFile        string
}

type completionInstallResult struct {
	Plan                 completionInstallPlan
	Config               *scaffold.ConfigSummary
	WroteScript          bool
	UpdatedRC            bool
	RCAlreadySet         bool
	CommandAlreadyOnPath bool
	WroteShim            bool
	UpdatedPathRC        bool
	PathRCAlreadySet     bool
}

type completionVerifyReport struct {
	SchemaVersion      int                     `json:"schema_version"`
	Command            string                  `json:"command"`
	Status             string                  `json:"status"`
	Target             string                  `json:"target"`
	Mutating           bool                    `json:"mutating"`
	Config             *scaffold.ConfigSummary `json:"config,omitempty"`
	Shell              string                  `json:"shell"`
	ShellSource        string                  `json:"shell_source"`
	DetectionEvidence  []string                `json:"detection_evidence,omitempty"`
	CompletionPath     string                  `json:"completion_path"`
	CompletionExists   bool                    `json:"completion_exists"`
	CompletionLooksOK  bool                    `json:"completion_looks_ok"`
	RCPath             string                  `json:"rc_path"`
	RCRequired         bool                    `json:"rc_required"`
	RCUpdatePresent    bool                    `json:"rc_update_present"`
	BinDir             string                  `json:"bin_dir,omitempty"`
	BinDirExists       bool                    `json:"bin_dir_exists"`
	PathContainsBinDir bool                    `json:"path_contains_bin_dir"`
	CommandPath        string                  `json:"command_path,omitempty"`
	CommandOrigin      string                  `json:"command_origin"`
	CommandOnPath      bool                    `json:"command_on_path"`
	Version            string                  `json:"version,omitempty"`
	RepoLocalPath      string                  `json:"repo_local_path,omitempty"`
	RepoLocalExists    bool                    `json:"repo_local_exists"`
	ReloadNeeded       bool                    `json:"reload_needed"`
	ReloadCommand      string                  `json:"reload_command"`
	Verified           bool                    `json:"verified"`
	NextSteps          []string                `json:"next_steps"`
}

func (r completionVerifyReport) TextWithOptions(opts scaffold.TextOptions) string {
	var b strings.Builder
	ui := cliui.New(opts.Color)
	fmt.Fprintf(&b, "%s %s\n", ui.Title("Completion verification for"), ui.Info(r.Shell))
	rows := []completionReportRow{
		{Left: "status", Right: r.Status},
		{Left: "target", Right: ui.Path(r.Target)},
		{Left: "shell", Right: r.Shell + " (" + completionShellSourceLabel(r.ShellSource) + ")"},
	}
	if len(r.DetectionEvidence) > 0 {
		rows = append(rows, completionReportRow{Left: "detection", Right: strings.Join(r.DetectionEvidence, "; ")})
	}
	rows = append(rows,
		completionReportRow{Left: "completion", Right: completionVerifyBoolPath(r.CompletionExists, r.CompletionPath, ui)},
		completionReportRow{Left: "script", Right: completionVerifyBool(r.CompletionLooksOK, "looks compatible", "missing or unexpected content", ui)},
	)
	if r.RCRequired {
		rows = append(rows, completionReportRow{Left: "shell startup", Right: completionVerifyBoolPath(r.RCUpdatePresent, r.RCPath, ui)})
	} else {
		rows = append(rows, completionReportRow{Left: "shell startup", Right: "not required"})
	}
	if r.BinDir != "" {
		rows = append(rows, completionReportRow{Left: "bin dir", Right: completionVerifyBoolPath(r.BinDirExists, r.BinDir, ui)})
	}
	if !r.RCRequired && r.RCPath != "" {
		rows = append(rows, completionReportRow{Left: "rc path", Right: ui.Path(r.RCPath)})
	}
	rows = append(rows,
		completionReportRow{Left: "command", Right: completionCommandSummary(r, ui)},
		completionReportRow{Left: "path contains bin", Right: completionVerifyBool(r.PathContainsBinDir, "yes", "no", ui)},
		completionReportRow{Left: "reload needed", Right: completionVerifyBool(!r.ReloadNeeded, "no", "yes", ui)},
	)
	if r.ReloadCommand != "" {
		rows = append(rows, completionReportRow{Left: "reload command", Right: r.ReloadCommand})
	}
	writeCompletionRows(&b, "checks", "item", "result", rows, opts.Color)
	if r.Config != nil {
		writeCompletionConfigSummary(&b, *r.Config, opts.Color)
	}
	if len(r.NextSteps) > 0 {
		stepRows := make([]completionReportRow, 0, len(r.NextSteps))
		for i, step := range r.NextSteps {
			stepRows = append(stepRows, completionReportRow{Left: fmt.Sprintf("%d", i+1), Right: step})
		}
		writeCompletionRows(&b, "next steps", "#", "action", stepRows, opts.Color)
	}
	return b.String()
}

type completionReportRow struct {
	Left  string
	Right string
}

func writeCompletionRows(b *strings.Builder, title string, leftHeader string, rightHeader string, rows []completionReportRow, color bool) {
	if len(rows) == 0 {
		return
	}
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	ui := cliui.New(color)
	fmt.Fprintln(b, ui.Section(title))
	width := len(leftHeader)
	for _, row := range rows {
		if len(row.Left) > width {
			width = len(row.Left)
		}
	}
	fmt.Fprintf(b, "  %-*s  %s\n", width, leftHeader, rightHeader)
	fmt.Fprintf(b, "  %-*s  %s\n", width, strings.Repeat("-", len(leftHeader)), strings.Repeat("-", len(rightHeader)))
	for _, row := range rows {
		fmt.Fprintf(b, "  %-*s  %s\n", width, row.Left, row.Right)
	}
}

func writeCompletionConfigSummary(b *strings.Builder, config scaffold.ConfigSummary, color bool) {
	rows := []completionReportRow{
		{Left: "global", Right: displayPath(config.GlobalPath) + " " + foundParenWord(config.GlobalFound)},
		{Left: "project", Right: displayPath(config.ProjectPath) + " " + foundParenWord(config.ProjectFound)},
	}
	for _, setting := range config.Settings {
		if setting.Value == "" {
			continue
		}
		rows = append(rows, completionReportRow{Left: setting.Source, Right: setting.Key + " = " + setting.Value})
	}
	writeCompletionRows(b, "config defaults", "source", "value", rows, color)
}

func foundParenWord(found bool) string {
	if found {
		return "(found)"
	}
	return "(missing)"
}

func completionVerifyBoolPath(ok bool, path string, ui cliui.Styles) string {
	label := "missing"
	if ok {
		label = "present"
	}
	if strings.TrimSpace(path) == "" {
		return label
	}
	return label + " " + ui.Path(displayPath(path))
}

func completionVerifyBool(ok bool, good string, bad string, ui cliui.Styles) string {
	if ok {
		return ui.Success(good)
	}
	return ui.Error(bad)
}

func completionCommandSummary(r completionVerifyReport, ui cliui.Styles) string {
	switch r.CommandOrigin {
	case "path":
		return "on PATH " + ui.Path(displayPath(r.CommandPath))
	case "repo-local":
		return "repo-local only " + ui.Path(displayPath(r.RepoLocalPath))
	case "path-and-repo-local":
		return "on PATH " + ui.Path(displayPath(r.CommandPath)) + ui.Muted(" (repo-local fallback also exists)")
	default:
		return "missing"
	}
}

func runCompletionWizard(root *cobra.Command, selection completionShellSelection, defaults bvconfig.Defaults) error {
	shellChoices := []charmui.Choice{
		{ID: "zsh", Label: "zsh", Description: "common on macOS and many developer terminals"},
		{ID: "bash", Label: "bash", Description: "common on Linux and server shells"},
		{ID: "fish", Label: "fish", Description: "writes a fish completion file"},
		{ID: "powershell", Label: "powershell", Description: "writes a script that can be loaded from your profile"},
	}
	defaultShell, description := completionWizardShellPrompt(selection)
	choice, err := charmui.AskSelect(os.Stdin, os.Stdout, charmui.SelectPrompt{
		Title:       "Shell completions",
		Description: description,
		Prompt:      "Which shell should use Burpvalve completions?",
		Choices:     shellChoices,
		DefaultID:   defaultShell,
		Color:       shouldColor(os.Stdout),
	})
	if err != nil {
		return completionPromptError("completion setup", err)
	}
	shell := choice.ID
	defaultPath, err := defaultCompletionPath(shell)
	if err != nil {
		return err
	}
	if defaults.Completion.Path != "" {
		defaultPath = expandUserPath(defaults.Completion.Path)
	}
	useDefault, err := charmui.AskConfirm(os.Stdin, os.Stdout, charmui.ConfirmPrompt{
		Title:       "Install location",
		Description: "Default path:\n" + displayPath(defaultPath),
		Prompt:      "Use the standard completion file path?",
		Default:     true,
		Color:       shouldColor(os.Stdout),
	})
	if err != nil {
		return completionPromptError("completion path", err)
	}
	path := defaultPath
	if !useDefault {
		path, err = charmui.AskText(os.Stdin, os.Stdout, charmui.TextPrompt{
			Title:       "Install location",
			Description: "Enter the file Burpvalve should write.",
			Prompt:      "Completion file path",
			Default:     defaultPath,
			Required:    true,
			Color:       shouldColor(os.Stdout),
		})
		if err != nil {
			return completionPromptError("completion path", err)
		}
	}
	path = expandUserPath(path)

	rcFile, canUpdateRC := defaultCompletionRCFile(shell)
	if defaults.Completion.RCFile != "" {
		rcFile = expandUserPath(defaults.Completion.RCFile)
		canUpdateRC = true
	}
	updateRC := false
	if canUpdateRC {
		defaultUpdateRC := true
		if defaults.Completion.UpdateRC != nil {
			defaultUpdateRC = *defaults.Completion.UpdateRC
		}
		updateRC, err = charmui.AskConfirm(os.Stdin, os.Stdout, charmui.ConfirmPrompt{
			Title:       "Shell config",
			Description: "Startup file:\n" + displayPath(rcFile),
			Prompt:      "Update the shell startup file so completions load automatically?",
			Default:     defaultUpdateRC,
			Color:       shouldColor(os.Stdout),
		})
		if err != nil {
			return completionPromptError("completion shell config", err)
		}
	}
	ensureCommandPath := false
	binDir := defaultCommandBinDir(defaults)
	if !burpvalveCommandOnPath() {
		ensureCommandPath, err = charmui.AskConfirm(os.Stdin, os.Stdout, charmui.ConfirmPrompt{
			Title:       "Command PATH",
			Description: "burpvalve is not on PATH.\nDefault command shim:\n" + displayPath(filepath.Join(binDir, "burpvalve")),
			Prompt:      "Install a command shim and add its directory to PATH?",
			Default:     true,
			Color:       shouldColor(os.Stdout),
		})
		if err != nil {
			return completionPromptError("completion command path", err)
		}
	}
	plan, err := completionInstallPlanFor(shell, path, rcFile, updateRC, ensureCommandPath, binDir)
	if err != nil {
		return err
	}
	if err := confirmCompletionInstall(plan, false); err != nil {
		return err
	}
	result, err := applyCompletionInstall(root, plan)
	printCompletionInstallResult(os.Stdout, result, shouldColor(os.Stdout))
	return err
}

func completionWizardShellPrompt(selection completionShellSelection) (defaultShell string, description string) {
	const chooseShell = "Choose the shell Burpvalve should set up."
	if selection.Shell == "" {
		return "zsh", "Burpvalve could not detect your shell. zsh is selected as a default.\n" + chooseShell
	}
	if selection.Source == "detected" || selection.Source == "" {
		return selection.Shell, "Detected shell: " + selection.Shell + "\n" + chooseShell
	}
	return selection.Shell, "Configured shell: " + selection.Shell + " (" + completionShellSourceLabel(selection.Source) + ")\n" + chooseShell
}

func completionPromptError(context string, err error) error {
	if errors.Is(err, charmui.ErrCancelled) {
		return fail(2, "%s cancelled; no files changed", context)
	}
	return fail(2, "%s failed: %v", context, err)
}

func confirmCompletionInstall(plan completionInstallPlan, force bool) error {
	if force {
		return nil
	}
	if !isInteractiveTerminal(os.Stdin, os.Stdout) {
		return fail(2, "completion install requires confirmation before changing files; run in a terminal or rerun with: burpvalve completion install --shell %s --force", plan.Shell)
	}
	description := "Will write:\n  " + displayPath(plan.Path)
	if plan.UpdateRC {
		description += "\n  " + displayPath(plan.RCFile)
	}
	if plan.EnsureCommandPath {
		description += "\n  " + displayPath(plan.ShimPath)
		if plan.PathRCFile != "" {
			description += "\n  " + displayPath(plan.PathRCFile)
		}
	}
	confirmed, err := configAskConfirm(os.Stdin, os.Stdout, charmui.ConfirmPrompt{
		Title:       "Apply completion setup",
		Description: description + "\nDefault is No.",
		Prompt:      "Apply these changes?",
		Default:     false,
		Color:       shouldColor(os.Stdout),
	})
	if err != nil {
		return completionPromptError("completion install confirmation", err)
	}
	if !confirmed {
		return fail(2, "completion install cancelled; no files changed")
	}
	return nil
}

func printCompletionInstallPlan(out io.Writer, plan completionInstallPlan, color bool) {
	var rows []completionReportRow
	rows = append(rows, completionReportRow{Left: "shell", Right: plan.Shell})
	rows = append(rows, completionReportRow{Left: "completion file", Right: displayPath(plan.Path)})
	if plan.UpdateRC {
		rows = append(rows, completionReportRow{Left: "shell startup", Right: displayPath(plan.RCFile)})
	}
	if plan.EnsureCommandPath {
		rows = append(rows, completionReportRow{Left: "command shim", Right: displayPath(plan.ShimPath)})
		if plan.PathRCFile != "" {
			rows = append(rows, completionReportRow{Left: "PATH startup", Right: displayPath(plan.PathRCFile)})
		}
	}
	var b strings.Builder
	ui := cliui.New(color)
	fmt.Fprintf(&b, "%s\n", ui.Title("Completion install plan"))
	writeCompletionRows(&b, "planned writes", "item", "path", rows, color)
	fmt.Fprintln(&b)
	fmt.Fprint(out, b.String())
}

func completionInstallPlanFor(shell string, path string, rcFile string, updateRC bool, ensureCommandPath bool, binDir string) (completionInstallPlan, error) {
	if strings.TrimSpace(shell) == "" {
		return completionInstallPlan{}, fail(2, "completion install needs --shell bash, zsh, fish, or powershell")
	}
	if strings.TrimSpace(path) == "" {
		defaultPath, err := defaultCompletionPath(shell)
		if err != nil {
			return completionInstallPlan{}, err
		}
		path = defaultPath
	}
	if updateRC && strings.TrimSpace(rcFile) == "" {
		defaultRC, ok := defaultCompletionRCFile(shell)
		if !ok {
			return completionInstallPlan{}, fail(2, "%s completions do not need an rc/profile update; rerun without --update-rc", shell)
		}
		rcFile = defaultRC
	}
	if strings.TrimSpace(binDir) == "" {
		binDir = defaultCommandBinDir(bvconfig.Defaults{})
	}
	pathRCFile := ""
	if ensureCommandPath {
		pathRCFile, _ = defaultPathRCFile(shell)
	}
	return completionInstallPlan{
		Shell:             shell,
		Path:              expandUserPath(path),
		RCFile:            expandUserPath(rcFile),
		UpdateRC:          updateRC,
		EnsureCommandPath: ensureCommandPath,
		BinDir:            expandUserPath(binDir),
		ShimPath:          filepath.Join(expandUserPath(binDir), "burpvalve"),
		PathRCFile:        expandUserPath(pathRCFile),
	}, nil
}

func buildCompletionVerifyReport(opts completionVerifyOptions) (completionVerifyReport, error) {
	target := opts.target
	if target == "" {
		target = "."
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return completionVerifyReport{}, err
	}
	effectiveConfig, err := bvconfig.Load(absTarget)
	if err != nil {
		return completionVerifyReport{}, fail(2, "%v", err)
	}
	selection, evidence, err := completionVerifyShellSelection(opts, effectiveConfig)
	if err != nil {
		return completionVerifyReport{}, err
	}
	path := opts.path
	if path == "" {
		path = effectiveConfig.File.Defaults.Completion.Path
	}
	rcFile := opts.rcFile
	if rcFile == "" {
		rcFile = effectiveConfig.File.Defaults.Completion.RCFile
	}
	updateRC := opts.updateRC
	if !updateRC && effectiveConfig.File.Defaults.Completion.UpdateRC != nil {
		updateRC = *effectiveConfig.File.Defaults.Completion.UpdateRC
	}
	binDir := opts.binDir
	if binDir == "" {
		binDir = effectiveConfig.File.Defaults.BinDir
	}
	plan, err := completionInstallPlanFor(selection.Shell, path, rcFile, updateRC, !opts.noPath, binDir)
	if err != nil {
		return completionVerifyReport{}, err
	}
	report := completionVerifyReport{
		SchemaVersion:      1,
		Command:            "completion verify",
		Target:             absTarget,
		Mutating:           false,
		Config:             setupConfigSummary(effectiveConfig),
		Shell:              selection.Shell,
		ShellSource:        selection.Source,
		DetectionEvidence:  evidence,
		CompletionPath:     plan.Path,
		CompletionExists:   fileExistsAbs(plan.Path),
		CompletionLooksOK:  completionScriptLooksOK(plan.Shell, plan.Path),
		RCPath:             plan.RCFile,
		RCRequired:         plan.UpdateRC,
		BinDir:             plan.BinDir,
		BinDirExists:       dirExistsAbs(plan.BinDir),
		PathContainsBinDir: pathContainsDir(plan.BinDir),
		RepoLocalPath:      filepath.Join(absTarget, "bin", "burpvalve"),
	}
	report.RCUpdatePresent = !report.RCRequired || rcContainsMarker(plan.RCFile, completionRCMarker(plan.Shell))
	report.CommandPath, report.CommandOrigin, report.CommandOnPath = detectBurpvalveCommandOrigin(absTarget)
	report.Version = completionCommandVersion(report.CommandPath)
	report.RepoLocalExists = executableFileExists(report.RepoLocalPath)
	if report.CommandOnPath && report.RepoLocalExists {
		report.CommandOrigin = "path-and-repo-local"
	}
	report.ReloadNeeded, report.ReloadCommand = completionReloadGuidance(plan)
	report.NextSteps = completionVerifyNextSteps(report, plan, opts.noPath)
	report.Verified = len(report.NextSteps) == 0
	report.Status = "ready"
	if !report.Verified {
		report.Status = "action_needed"
	}
	return report, nil
}

func completionVerifyShellSelection(opts completionVerifyOptions, effective bvconfig.Effective) (completionShellSelection, []string, error) {
	if opts.shell != "" {
		shell, ok := normalizeCompletionShell(opts.shell)
		if !ok {
			return completionShellSelection{}, nil, fail(2, "unknown shell %q; expected bash, zsh, fish, or powershell", opts.shell)
		}
		return completionShellSelection{Shell: shell, Source: "flag"}, nil, nil
	}
	if effective.File.Defaults.Shell != "" {
		selection, err := configuredCompletionShellSelection(effective)
		return selection, nil, err
	}
	shell, evidence, err := detectCompletionShellWithEvidence()
	if err != nil {
		return completionShellSelection{}, evidence, err
	}
	return completionShellSelection{Shell: shell, Source: "detected"}, evidence, nil
}

func fileExistsAbs(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExistsAbs(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func executableFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

func completionScriptLooksOK(shell string, path string) bool {
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := string(body)
	switch shell {
	case "zsh":
		return strings.Contains(text, "#compdef burpvalve")
	case "fish":
		return strings.Contains(text, "complete -c burpvalve")
	case "bash":
		return strings.Contains(text, "__start_burpvalve") || strings.Contains(text, "complete -o default")
	case "powershell":
		return strings.Contains(text, "Register-ArgumentCompleter") || strings.Contains(text, "burpvalve")
	default:
		return false
	}
}

func rcContainsMarker(path string, marker string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	body, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(body), marker)
}

func pathContainsDir(dir string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	want, err := filepath.Abs(expandUserPath(dir))
	if err != nil {
		want = expandUserPath(dir)
	}
	for _, item := range filepath.SplitList(os.Getenv("PATH")) {
		got, err := filepath.Abs(item)
		if err != nil {
			got = item
		}
		if got == want {
			return true
		}
	}
	return false
}

func detectBurpvalveCommandOrigin(target string) (commandPath string, origin string, onPath bool) {
	if path, err := exec.LookPath("burpvalve"); err == nil {
		return path, "path", true
	}
	repoLocal := filepath.Join(target, "bin", "burpvalve")
	if executableFileExists(repoLocal) {
		return repoLocal, "repo-local", false
	}
	return "", "missing", false
}

func completionCommandVersion(commandPath string) string {
	if strings.TrimSpace(commandPath) == "" {
		return ""
	}
	out, err := exec.Command(commandPath, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func completionReloadGuidance(plan completionInstallPlan) (bool, string) {
	if plan.UpdateRC && plan.RCFile != "" {
		return true, sourceCommandForShell(plan.Shell, plan.RCFile)
	}
	if plan.EnsureCommandPath && plan.PathRCFile != "" {
		return true, sourceCommandForShell(plan.Shell, plan.PathRCFile)
	}
	return false, ""
}

func sourceCommandForShell(shell string, path string) string {
	switch shell {
	case "fish":
		return "source " + shellQuote(path)
	case "powershell":
		return ". " + powershellQuote(path)
	default:
		return ". " + shellQuote(path)
	}
}

func completionVerifyNextSteps(report completionVerifyReport, plan completionInstallPlan, noPath bool) []string {
	steps := []string{}
	installCommand := fmt.Sprintf("burpvalve completion install --shell %s --path %s --force", plan.Shell, shellQuote(plan.Path))
	if plan.UpdateRC {
		installCommand = fmt.Sprintf("burpvalve completion install --shell %s --path %s --rc-file %s --update-rc --force", plan.Shell, shellQuote(plan.Path), shellQuote(plan.RCFile))
	}
	if !report.CompletionExists || !report.CompletionLooksOK {
		steps = append(steps, "Install the completion script: "+installCommand)
	}
	if report.RCRequired && !report.RCUpdatePresent {
		steps = append(steps, "Update shell startup wiring: "+installCommand)
	}
	if !noPath {
		if !report.CommandOnPath {
			if report.RepoLocalExists {
				steps = append(steps, "Install the global command shim so hooks do not depend on repo-local fallback: burpvalve completion install --shell "+plan.Shell+" --force")
			} else {
				steps = append(steps, "Install Burpvalve on PATH or create the command shim: burpvalve completion install --shell "+plan.Shell+" --force")
			}
		}
		if !report.BinDirExists {
			steps = append(steps, "Create or choose a command shim directory: mkdir -p "+shellQuote(plan.BinDir))
		}
		if report.BinDirExists && !report.PathContainsBinDir {
			steps = append(steps, "Open a new terminal or add the shim directory to PATH: export PATH="+shellQuote(plan.BinDir)+":\"$PATH\"")
		}
	}
	return steps
}

func applyCompletionInstall(root *cobra.Command, plan completionInstallPlan) (completionInstallResult, error) {
	var script strings.Builder
	if err := printCompletion(root, &script, plan.Shell); err != nil {
		return completionInstallResult{Plan: plan}, err
	}
	if err := os.MkdirAll(filepath.Dir(plan.Path), 0o755); err != nil {
		return completionInstallResult{Plan: plan}, fail(1, "create completion directory %s: %v", filepath.Dir(plan.Path), err)
	}
	if err := os.WriteFile(plan.Path, []byte(script.String()), 0o644); err != nil {
		return completionInstallResult{Plan: plan}, fail(1, "write completion file %s: %v", plan.Path, err)
	}
	result := completionInstallResult{Plan: plan, WroteScript: true}
	if plan.UpdateRC {
		updated, alreadySet, err := ensureCompletionRC(plan)
		if err != nil {
			return result, err
		}
		result.UpdatedRC = updated
		result.RCAlreadySet = alreadySet
	}
	if err := ensureCompletionCommandPath(plan, &result); err != nil {
		return result, err
	}
	return result, nil
}

func ensureCompletionCommandPath(plan completionInstallPlan, result *completionInstallResult) error {
	if path, err := exec.LookPath("burpvalve"); err == nil {
		result.CommandAlreadyOnPath = true
		result.Plan.ShimPath = path
		return nil
	}
	if !plan.EnsureCommandPath {
		return nil
	}
	source, err := os.Executable()
	if err != nil {
		return fail(1, "locate current burpvalve executable for PATH shim: %v", err)
	}
	source, err = filepath.Abs(source)
	if err != nil {
		return fail(1, "resolve current burpvalve executable: %v", err)
	}
	if err := os.MkdirAll(plan.BinDir, 0o755); err != nil {
		return fail(1, "create command shim directory %s: %v", plan.BinDir, err)
	}
	if err := installCommandShim(source, plan.ShimPath); err != nil {
		return err
	}
	result.WroteShim = true
	if plan.PathRCFile != "" {
		updated, alreadySet, err := ensurePathRC(plan)
		if err != nil {
			return err
		}
		result.UpdatedPathRC = updated
		result.PathRCAlreadySet = alreadySet
	}
	return nil
}

func installCommandShim(source string, dest string) error {
	if info, err := os.Lstat(dest); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			target, readErr := os.Readlink(dest)
			if readErr != nil {
				return fail(1, "read existing command shim %s: %v", dest, readErr)
			}
			if target == source {
				return nil
			}
		}
		return fail(1, "command shim path %s already exists; move it or rerun with --bin-dir", dest)
	} else if !os.IsNotExist(err) {
		return fail(1, "check command shim path %s: %v", dest, err)
	}
	if err := os.Symlink(source, dest); err != nil {
		return fail(1, "create command shim %s pointing to %s: %v", dest, source, err)
	}
	return nil
}

func ensureCompletionRC(plan completionInstallPlan) (bool, bool, error) {
	block := completionRCBlock(plan)
	body, err := os.ReadFile(plan.RCFile)
	if err != nil && !os.IsNotExist(err) {
		return false, false, fail(1, "read shell startup file %s: %v", plan.RCFile, err)
	}
	if strings.Contains(string(body), completionRCMarker(plan.Shell)) {
		return false, true, nil
	}
	if err := os.MkdirAll(filepath.Dir(plan.RCFile), 0o755); err != nil {
		return false, false, fail(1, "create shell startup directory %s: %v", filepath.Dir(plan.RCFile), err)
	}
	prefix := ""
	if len(body) > 0 && !strings.HasSuffix(string(body), "\n") {
		prefix = "\n"
	}
	next := string(body) + prefix + block
	if err := os.WriteFile(plan.RCFile, []byte(next), 0o644); err != nil {
		return false, false, fail(1, "update shell startup file %s: %v", plan.RCFile, err)
	}
	return true, false, nil
}

func completionRCMarker(shell string) string {
	return "# >>> burpvalve completion " + shell + " >>>"
}

func completionRCBlock(plan completionInstallPlan) string {
	start := completionRCMarker(plan.Shell)
	end := "# <<< burpvalve completion " + plan.Shell + " <<<"
	switch plan.Shell {
	case "zsh":
		dir := filepath.Dir(plan.Path)
		return fmt.Sprintf("%s\nfpath=(%s $fpath)\nautoload -Uz compinit\ncompinit\n%s\n", start, shellQuote(dir), end)
	case "bash":
		return fmt.Sprintf("%s\nif [ -f %s ]; then\n  . %s\nfi\n%s\n", start, shellQuote(plan.Path), shellQuote(plan.Path), end)
	case "powershell":
		return fmt.Sprintf("%s\n. %s\n%s\n", start, powershellQuote(plan.Path), end)
	default:
		return ""
	}
}

func ensurePathRC(plan completionInstallPlan) (bool, bool, error) {
	block := pathRCBlock(plan)
	body, err := os.ReadFile(plan.PathRCFile)
	if err != nil && !os.IsNotExist(err) {
		return false, false, fail(1, "read shell startup file %s: %v", plan.PathRCFile, err)
	}
	if strings.Contains(string(body), pathRCMarker(plan.Shell)) {
		return false, true, nil
	}
	if err := os.MkdirAll(filepath.Dir(plan.PathRCFile), 0o755); err != nil {
		return false, false, fail(1, "create shell startup directory %s: %v", filepath.Dir(plan.PathRCFile), err)
	}
	prefix := ""
	if len(body) > 0 && !strings.HasSuffix(string(body), "\n") {
		prefix = "\n"
	}
	if err := os.WriteFile(plan.PathRCFile, []byte(string(body)+prefix+block), 0o644); err != nil {
		return false, false, fail(1, "update shell startup file %s: %v", plan.PathRCFile, err)
	}
	return true, false, nil
}

func pathRCMarker(shell string) string {
	return "# >>> burpvalve PATH " + shell + " >>>"
}

func pathRCBlock(plan completionInstallPlan) string {
	start := pathRCMarker(plan.Shell)
	end := "# <<< burpvalve PATH " + plan.Shell + " <<<"
	switch plan.Shell {
	case "fish":
		return fmt.Sprintf("%s\nfish_add_path %s\n%s\n", start, shellQuote(plan.BinDir), end)
	case "powershell":
		return fmt.Sprintf("%s\n$env:PATH = %s + [IO.Path]::PathSeparator + $env:PATH\n%s\n", start, powershellQuote(plan.BinDir), end)
	default:
		return fmt.Sprintf("%s\nexport PATH=%s:\"$PATH\"\n%s\n", start, shellQuote(plan.BinDir), end)
	}
}

func printCompletionInstallResult(out io.Writer, result completionInstallResult, color bool) {
	ui := cliui.New(color)
	fmt.Fprintln(out, ui.Section("Completion setup"))
	fmt.Fprintf(out, "%s %s\n", ui.Label("Shell:"), ui.Info(result.Plan.Shell))
	if result.WroteScript {
		fmt.Fprintf(out, "%s %s\n", ui.Label("Wrote:"), ui.Path(displayPath(result.Plan.Path)))
	}
	if result.UpdatedRC {
		fmt.Fprintf(out, "%s %s\n", ui.Label("Updated:"), ui.Path(displayPath(result.Plan.RCFile)))
	} else if result.RCAlreadySet {
		fmt.Fprintf(out, "%s %s %s\n", ui.Label("Already set:"), ui.Path(displayPath(result.Plan.RCFile)), ui.Muted("(left unchanged)"))
	}
	if result.CommandAlreadyOnPath {
		fmt.Fprintf(out, "%s %s %s\n", ui.Label("Command:"), ui.Path(displayPath(result.Plan.ShimPath)), ui.Muted("(already on PATH)"))
	}
	if result.WroteShim {
		fmt.Fprintf(out, "%s %s\n", ui.Label("Command:"), ui.Path(displayPath(result.Plan.ShimPath)))
	}
	if result.UpdatedPathRC {
		fmt.Fprintf(out, "%s %s\n", ui.Label("PATH:"), ui.Path(displayPath(result.Plan.PathRCFile)))
	} else if result.PathRCAlreadySet {
		fmt.Fprintf(out, "%s %s %s\n", ui.Label("PATH:"), ui.Path(displayPath(result.Plan.PathRCFile)), ui.Muted("(already set)"))
	}
	printInlineConfigSummary(out, ui, result.Config)
	if reloadNeeded, reloadCommand := completionReloadGuidance(result.Plan); reloadNeeded && reloadCommand != "" {
		fmt.Fprintf(out, "%s %s\n", ui.Label("Reload:"), ui.Info(reloadCommand))
		fmt.Fprintln(out, ui.Muted("Or restart your shell/open a new terminal for the changes to load."))
	} else {
		fmt.Fprintln(out, ui.Muted("Restart your shell or open a new terminal if completions are not visible yet."))
	}
}

func printInlineConfigSummary(out io.Writer, ui cliui.Styles, config *scaffold.ConfigSummary) {
	if config == nil || len(config.Settings) == 0 {
		return
	}
	fmt.Fprintln(out, ui.Section("Config defaults"))
	for _, setting := range config.Settings {
		if setting.Value == "" {
			continue
		}
		fmt.Fprintf(out, "%s %s %s\n", ui.Label(setting.Source+":"), setting.Key, ui.Muted("= "+setting.Value))
	}
}

func printCompletionGuide(out io.Writer, selection completionShellSelection, defaults bvconfig.Defaults, color bool) error {
	ui := cliui.New(color)
	shell := selection.Shell
	path, err := defaultCompletionPath(shell)
	if err != nil {
		return err
	}
	if defaults.Completion.Path != "" {
		path = expandUserPath(defaults.Completion.Path)
	}
	rcFile, hasRC := defaultCompletionRCFile(shell)
	if defaults.Completion.RCFile != "" {
		rcFile = expandUserPath(defaults.Completion.RCFile)
		hasRC = true
	}
	fmt.Fprintln(out, ui.Section("Shell completions"))
	if selection.Source == "detected" || selection.Source == "" {
		fmt.Fprintf(out, "%s %s\n\n", ui.Label("Detected shell:"), ui.Info(shell))
	} else {
		fmt.Fprintf(out, "%s %s %s\n\n", ui.Label("Configured shell:"), ui.Info(shell), ui.Muted("("+completionShellSourceLabel(selection.Source)+")"))
	}
	fmt.Fprintln(out, "Run this in a terminal for the guided installer:")
	fmt.Fprintf(out, "  %s\n\n", ui.Info("burpvalve completion"))
	fmt.Fprintln(out, "Or install directly:")
	install := fmt.Sprintf("burpvalve completion install --shell %s --path %s --force", shell, shellQuote(path))
	if defaults.Completion.UpdateRC != nil && !*defaults.Completion.UpdateRC {
		hasRC = false
	}
	if hasRC {
		install = fmt.Sprintf("burpvalve completion install --shell %s --path %s --rc-file %s --update-rc --force", shell, shellQuote(path), shellQuote(rcFile))
	}
	fmt.Fprintf(out, "  %s\n\n", ui.Info(install))
	fmt.Fprintln(out, "To print only the raw completion script:")
	fmt.Fprintf(out, "  %s\n", ui.Info(fmt.Sprintf("burpvalve completion %s > %s", shell, shellQuote(path))))
	return nil
}

func completionShellSourceLabel(source string) string {
	switch source {
	case "project":
		return "project config"
	case "global":
		return "global config"
	case "input":
		return "robot input"
	case "argument":
		return "command argument"
	case "detected":
		return "detected shell"
	default:
		return source
	}
}

func defaultCompletionPath(shell string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fail(1, "find home directory for completion install: %v", err)
	}
	switch shell {
	case "zsh":
		return filepath.Join(home, ".zsh", "completions", "_burpvalve"), nil
	case "bash":
		return filepath.Join(home, ".local", "share", "bash-completion", "completions", "burpvalve"), nil
	case "fish":
		return filepath.Join(home, ".config", "fish", "completions", "burpvalve.fish"), nil
	case "powershell":
		return filepath.Join(home, ".config", "powershell", "burpvalve-completion.ps1"), nil
	default:
		return "", fail(2, "unknown shell %q; expected bash, zsh, fish, or powershell", shell)
	}
}

func defaultCompletionRCFile(shell string) (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	switch shell {
	case "zsh":
		return filepath.Join(home, ".zshrc"), true
	case "bash":
		return filepath.Join(home, ".bashrc"), true
	case "powershell":
		return filepath.Join(home, ".config", "powershell", "Microsoft.PowerShell_profile.ps1"), true
	default:
		return "", false
	}
}

func defaultPathRCFile(shell string) (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	switch shell {
	case "zsh":
		return filepath.Join(home, ".zshrc"), true
	case "bash":
		return filepath.Join(home, ".bashrc"), true
	case "fish":
		return filepath.Join(home, ".config", "fish", "config.fish"), true
	case "powershell":
		return filepath.Join(home, ".config", "powershell", "Microsoft.PowerShell_profile.ps1"), true
	default:
		return "", false
	}
}

func defaultCommandBinDir(defaults bvconfig.Defaults) string {
	if defaults.BinDir != "" {
		return expandUserPath(defaults.BinDir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".local", "bin")
}

func burpvalveCommandOnPath() bool {
	_, err := exec.LookPath("burpvalve")
	return err == nil
}

func expandUserPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func displayPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(os.PathSeparator)) {
		return "~/" + strings.TrimPrefix(path, home+string(os.PathSeparator))
	}
	return path
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func powershellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func detectCompletionShell() (string, error) {
	shell, _, err := detectCompletionShellWithEvidence()
	return shell, err
}

var (
	completionParentPID           = os.Getppid
	completionParentShellDetector = detectParentCompletionShell
)

func detectCompletionShellWithEvidence() (string, []string, error) {
	var evidence []string
	if value := os.Getenv("SHELL"); strings.TrimSpace(value) != "" {
		if shell, ok := normalizeCompletionShell(value); ok {
			return shell, []string{"SHELL=" + value}, nil
		}
		evidence = append(evidence, "SHELL="+value+" (unsupported)")
	}
	if shell, detail, ok := completionParentShellDetector(completionParentPID()); ok {
		return shell, append(evidence, detail), nil
	} else if detail != "" {
		evidence = append(evidence, detail)
	}
	if runtime.GOOS == "windows" {
		if value := os.Getenv("ComSpec"); strings.TrimSpace(value) != "" {
			if shell, ok := normalizeCompletionShell(value); ok {
				return shell, append(evidence, "ComSpec="+value), nil
			}
			evidence = append(evidence, "ComSpec="+value+" (unsupported)")
		}
	}
	return "", evidence, fail(2, "could not detect shell; run one of: burpvalve completion bash, burpvalve completion zsh, burpvalve completion fish, burpvalve completion powershell")
}

func detectParentCompletionShell(ppid int) (string, string, bool) {
	if ppid <= 0 || runtime.GOOS == "windows" {
		return "", "", false
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(ppid), "-o", "comm=").Output()
	if err != nil {
		return "", "parent process lookup failed", false
	}
	name := strings.TrimSpace(string(out))
	shell, ok := normalizeCompletionShell(name)
	if !ok {
		return "", "parent process=" + name + " (unsupported)", false
	}
	return shell, "parent process=" + name, true
}

func normalizeCompletionShell(value string) (string, bool) {
	name := strings.ToLower(strings.TrimSpace(value))
	if name == "" {
		return "", false
	}
	name = strings.TrimPrefix(filepath.Base(name), "-")
	name = strings.TrimSuffix(name, ".exe")
	switch name {
	case "bash", "zsh", "fish":
		return name, true
	case "pwsh", "powershell", "powershell-preview":
		return "powershell", true
	default:
		return "", false
	}
}

func bindLegacyFlags(cmd *cobra.Command, legacy *legacyOptions) {
	flags := cmd.PersistentFlags()
	flags.StringVar(&legacy.mode, "mode", "", "legacy compatibility mode: check, init, repair, pre-commit, lint, or ci")
	flags.StringVar(&legacy.target, "target", ".", "legacy target project root")
	flags.StringVar(&legacy.root, "root", ".", "legacy repository root")
	flags.BoolVar(&legacy.jsonOutput, "json", false, "legacy machine-readable setup output")
	flags.BoolVar(&legacy.noBeads, "no-beads", false, "legacy init only: skip .beads")
	flags.BoolVar(&legacy.noNTM, "no-ntm", false, "legacy init only: skip ntm quick")
	flags.BoolVar(&legacy.noClaude, "no-claude", false, "legacy init only: skip CLAUDE.md")
	flags.BoolVar(&legacy.noClaudeSymlink, "no-claude-symlink", false, "legacy init only: alias for --no-claude")
	flags.BoolVar(&legacy.noAgents, "no-agents", false, "legacy init only: skip AGENTS.md")
	flags.BoolVar(&legacy.noAgentsMD, "no-agents-md", false, "legacy init only: alias for --no-agents")
	flags.StringVar(&legacy.feature, "feature", "", "legacy explicit atomic feature or bead id")
	flags.StringVar(&legacy.responses, "responses", "", "legacy JSON matrix responses")
	flags.StringVar(&legacy.agent, "agent", "codex", "legacy agent name")
	flags.StringVar(&legacy.model, "model", "unspecified", "legacy model name")
	for _, name := range []string{"mode", "target", "root", "json", "no-beads", "no-ntm", "no-claude", "no-claude-symlink", "no-agents", "no-agents-md", "feature", "responses", "agent", "model"} {
		_ = flags.MarkHidden(name)
	}
}

func newSetupCommand() *cobra.Command {
	opts := setupOptions{target: "."}
	cmd := &cobra.Command{
		Use:     "setup",
		Aliases: []string{"check"},
		Short:   "Check what Burpvalve would add without changing files.",
		Long: `Check the target repository and list what Burpvalve can use, what is
missing, and what init or repair would change. This command does not edit files.`,
		Example: `  burpvalve setup
  burpvalve setup --json
  burpvalve setup --target /path/to/repo --json
  burpvalve setup --no-beads --no-ntm`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if robotsMode {
				opts.jsonOutput = true
				return runSetupRobots(cmd, opts)
			}
			return runSetup(scaffold.ModeCheck, opts)
		},
	}
	cmd.Flags().StringVar(&opts.target, "target", ".", "target project root")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "print machine-readable JSON")
	cmd.Flags().BoolVar(&opts.noBeads, "no-beads", false, "skip Beads readiness checks")
	cmd.Flags().BoolVar(&opts.noNTM, "no-ntm", false, "skip NTM readiness checks")
	return cmd
}

type explainOptions struct {
	root       string
	jsonOutput bool
}

type explanation struct {
	SchemaVersion  int                     `json:"schema_version"`
	Command        string                  `json:"command"`
	InputType      string                  `json:"input_type"`
	Status         string                  `json:"status"`
	OriginalStatus string                  `json:"original_status,omitempty"`
	Summary        string                  `json:"summary"`
	WhyItMatters   string                  `json:"why_it_matters"`
	Fatal          bool                    `json:"fatal"`
	Source         map[string]string       `json:"source,omitempty"`
	Config         *scaffold.ConfigSummary `json:"config,omitempty"`
	FeatureIDs     []string                `json:"feature_ids,omitempty"`
	BeadIDs        []string                `json:"bead_ids,omitempty"`
	NextSteps      []string                `json:"next_steps"`
	Blockers       []explainBlocker        `json:"blockers,omitempty"`
}

type explainBlocker struct {
	ID              string                      `json:"id"`
	Status          string                      `json:"status,omitempty"`
	Message         string                      `json:"message,omitempty"`
	Command         string                      `json:"command,omitempty"`
	Fatal           bool                        `json:"fatal,omitempty"`
	ConditionFile   string                      `json:"condition_file,omitempty"`
	VerifierPolicy  attestations.VerifierPolicy `json:"verifier_policy,omitempty"`
	VerifierKind    attestations.VerifierKind   `json:"verifier_kind,omitempty"`
	Verdict         attestations.Verdict        `json:"verdict,omitempty"`
	NextAction      string                      `json:"next_action,omitempty"`
	EvidenceMissing bool                        `json:"evidence_missing,omitempty"`
	Supplemental    bool                        `json:"supplemental,omitempty"`
	AdjudicationRef string                      `json:"adjudication_ref,omitempty"`
}

type scaffoldMutationExplanationInput struct {
	Command        string                   `json:"command"`
	Status         string                   `json:"status"`
	Fatal          bool                     `json:"fatal"`
	PartialSuccess bool                     `json:"partial_success"`
	Mutating       bool                     `json:"mutating"`
	NextSteps      []scaffold.RecoveryStep  `json:"next_steps"`
	Conflicts      []scaffold.ApplyConflict `json:"conflicts"`
	Created        []string                 `json:"created"`
	Repaired       []string                 `json:"repaired"`
	Preserved      []string                 `json:"preserved"`
	Skipped        []string                 `json:"skipped"`
}

func newExplainCommand() *cobra.Command {
	opts := explainOptions{root: "."}
	cmd := &cobra.Command{
		Use:   "explain [path-or--]",
		Short: "Explain setup, lint, commit, and attestation blockers.",
		Long: `Explain structured Burpvalve output or evidence artifacts in plain language.
This command is read-only: it never repairs, stages, commits, or changes config.`,
		Args: cobra.MaximumNArgs(1),
		Example: `  burpvalve setup --json | burpvalve explain -
  burpvalve lint --json | burpvalve explain --json -
  burpvalve explain log/backpressure/failed/<file>.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := "-"
			if len(args) == 1 {
				ref = args[0]
			}
			return runExplain(cmd, opts, ref)
		},
	}
	cmd.Flags().StringVar(&opts.root, "root", ".", "repository root for artifact references")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "print machine-readable JSON")
	return cmd
}

func runExplain(cmd *cobra.Command, opts explainOptions, ref string) error {
	exp, err := buildExplanation(cmd, opts, ref)
	if err != nil {
		exp = explanation{
			SchemaVersion: 1,
			Command:       "explain",
			InputType:     "unknown",
			Status:        "error",
			Summary:       err.Error(),
			WhyItMatters:  "Burpvalve needs structured JSON or a known evidence artifact before it can give reliable recovery steps.",
			Fatal:         true,
			NextSteps:     []string{"Pass a blocked report or attestation path, or pipe JSON from burpvalve setup --json, burpvalve init --json, burpvalve repair --json, burpvalve lint --json, or burpvalve commit."},
		}
		if opts.jsonOutput || robotsMode {
			_ = encodeJSON(cmd.OutOrStdout(), exp, "encode explain error")
			return exitCode(2)
		}
		printExplanation(cmd.OutOrStdout(), exp)
		return exitCode(2)
	}
	if opts.jsonOutput || robotsMode {
		return encodeJSON(cmd.OutOrStdout(), exp, "encode explanation")
	}
	printExplanation(cmd.OutOrStdout(), exp)
	return nil
}

func buildExplanation(cmd *cobra.Command, opts explainOptions, ref string) (explanation, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || ref == "-" {
		body, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return explanation{}, err
		}
		if strings.TrimSpace(string(body)) == "" {
			return explanation{}, fmt.Errorf("explain requires structured JSON on stdin or a path/ref argument")
		}
		return explainJSON(body, map[string]string{"kind": "stdin"})
	}
	record, artifact, err := attestations.ShowArtifact(opts.root, ref)
	if err != nil {
		return explanation{}, err
	}
	return explainArtifact(record, artifact, map[string]string{"kind": "artifact", "ref": ref, "path": record.Path}), nil
}

func explainJSON(body []byte, source map[string]string) (explanation, error) {
	var header struct {
		Command      string                    `json:"command"`
		Tool         string                    `json:"tool"`
		ArtifactKind attestations.ArtifactKind `json:"artifact_kind"`
	}
	if err := json.Unmarshal(body, &header); err != nil {
		return explanation{}, fmt.Errorf("parse explain input: %w", err)
	}
	switch header.Command {
	case "setup":
		var report scaffold.Report
		if err := json.Unmarshal(body, &report); err != nil {
			return explanation{}, fmt.Errorf("parse setup report: %w", err)
		}
		return explainSetup(report, source), nil
	case "init", "repair":
		var result scaffoldMutationExplanationInput
		if err := json.Unmarshal(body, &result); err != nil {
			return explanation{}, fmt.Errorf("parse %s result: %w", header.Command, err)
		}
		return explainScaffoldMutation(result, source), nil
	case "lint":
		var result backpressure.LintResult
		if err := json.Unmarshal(body, &result); err != nil {
			return explanation{}, fmt.Errorf("parse lint result: %w", err)
		}
		return explainLint(result, source), nil
	case "commit":
		var result backpressure.PreCommitResult
		if err := json.Unmarshal(body, &result); err != nil {
			return explanation{}, fmt.Errorf("parse commit result: %w", err)
		}
		return explainCommit(result, source), nil
	case "completion verify":
		var result completionVerifyReport
		if err := json.Unmarshal(body, &result); err != nil {
			return explanation{}, fmt.Errorf("parse completion verify result: %w", err)
		}
		return explainCompletionVerify(result, source), nil
	}
	if header.Tool == attestations.ToolName || header.ArtifactKind != "" {
		var artifact attestations.Artifact
		if err := json.Unmarshal(body, &artifact); err != nil {
			return explanation{}, fmt.Errorf("parse attestation artifact: %w", err)
		}
		if err := validateExplainArtifact(artifact); err != nil {
			return explanation{}, err
		}
		record := recordFromArtifact(artifact)
		return explainArtifact(record, artifact, source), nil
	}
	return explanation{}, fmt.Errorf("unknown explain input; expected Burpvalve setup, init, repair, lint, commit, blocked report, or attestation JSON")
}

func explainSetup(report scaffold.Report, source map[string]string) explanation {
	exp := baseExplanation("setup", report.Status, source)
	exp.Fatal = report.Fatal
	exp.Config = report.Config
	exp.Summary = "Setup inspected the repository and reported " + report.Status + " readiness."
	if report.ReadinessSeverity == "warning" {
		exp.Summary = "Setup found warnings, but no required blocker."
	}
	if report.Fatal {
		exp.Summary = "Setup found required blockers before this repo is ready."
	}
	exp.WhyItMatters = "Setup is the read-only readiness surface; these checks tell you what init or repair would need before hooks and backpressure are reliable."
	for _, step := range report.NextSteps {
		next := strings.TrimSpace(step.Command)
		if next == "" {
			next = step.Message
		}
		exp.NextSteps = append(exp.NextSteps, next)
		exp.Blockers = append(exp.Blockers, explainBlocker{ID: step.ID, Message: step.Message, Command: step.Command, Fatal: step.Fatal})
	}
	if len(exp.NextSteps) == 0 {
		exp.NextSteps = []string{"No setup recovery command is required."}
	}
	return exp
}

func explainLint(result backpressure.LintResult, source map[string]string) explanation {
	exp := baseExplanation("lint", result.Status, source)
	exp.Fatal = result.Fatal
	exp.NextSteps = append([]string(nil), result.NextSteps...)
	if len(exp.NextSteps) == 0 {
		exp.NextSteps = []string{"No lint recovery step is required."}
	}
	if !result.Enforced || result.CommandCount == 0 {
		exp.Summary = "Lint is not enforcing executable commands yet."
		exp.WhyItMatters = "A lint no-op is not proof that the code was checked; it only means no deterministic lint command is configured."
		exp.Blockers = append(exp.Blockers, explainBlocker{ID: "lint_commands", Status: "missing", Message: result.Message, Command: firstString(exp.NextSteps)})
		return exp
	}
	if result.Fatal {
		exp.Summary = "A required lint command failed."
		exp.WhyItMatters = "Required lint commands are deterministic backpressure; failing output blocks the commit until fixed."
	} else {
		exp.Summary = "Configured lint commands passed."
		exp.WhyItMatters = "Executable lint command output can be used as stronger evidence than a policy wishlist."
	}
	for _, command := range result.Commands {
		if command.Status != backpressure.LintStatusPassed && command.Required {
			exp.Blockers = append(exp.Blockers, explainBlocker{ID: command.ID, Status: command.Status, Message: command.Error, Command: command.Command, Fatal: true})
		}
	}
	return exp
}

func explainScaffoldMutation(result scaffoldMutationExplanationInput, source map[string]string) explanation {
	command := strings.TrimSpace(result.Command)
	if command == "" {
		command = "scaffold"
	}
	exp := baseExplanation(command, result.Status, source)
	exp.Fatal = result.Fatal
	if result.PartialSuccess {
		exp.Summary = command + " applied some changes, but stopped on conflicts."
	} else if result.Fatal {
		exp.Summary = command + " stopped before the repo reached the requested scaffold state."
	} else {
		exp.Summary = command + " completed without a blocking recovery step."
	}
	exp.WhyItMatters = "Init and repair are mutating scaffold commands; explain turns their structured result into the recovery steps needed before rerunning them."
	for _, step := range result.NextSteps {
		next := strings.TrimSpace(step.Command)
		if next == "" {
			next = step.Message
		}
		exp.NextSteps = append(exp.NextSteps, next)
		exp.Blockers = append(exp.Blockers, explainBlocker{ID: step.ID, Message: step.Message, Command: step.Command, Fatal: step.Fatal})
	}
	for _, conflict := range result.Conflicts {
		exp.Blockers = append(exp.Blockers, explainBlocker{ID: conflict.Path, Status: "conflict", Message: conflict.Message, Fatal: true})
	}
	if len(exp.NextSteps) == 0 {
		if result.Fatal {
			exp.NextSteps = []string{"Inspect the blockers above, then rerun burpvalve " + command + " after resolving them."}
		} else {
			exp.NextSteps = []string{"No " + command + " recovery step is required."}
		}
	}
	return exp
}

func explainCommit(result backpressure.PreCommitResult, source map[string]string) explanation {
	exp := baseExplanation("commit", result.Status, source)
	exp.Fatal = result.Fatal
	exp.Summary = result.Message
	exp.WhyItMatters = "The commit gate binds evidence to the exact staged payload before Git records the commit."
	exp.NextSteps = append([]string(nil), result.NextSteps...)
	if result.BlockedReportPath != "" {
		exp.Blockers = append(exp.Blockers, explainBlocker{ID: "blocked_report", Status: result.Status, Message: result.Message, Command: "burpvalve explain " + result.BlockedReportPath, Fatal: result.Fatal})
	}
	if len(exp.NextSteps) == 0 {
		exp.NextSteps = []string{"Inspect the commit result and rerun burpvalve commit after resolving blockers."}
	}
	return exp
}

func explainCompletionVerify(result completionVerifyReport, source map[string]string) explanation {
	exp := baseExplanation("completion_verify", result.Status, source)
	exp.Fatal = result.Status == "blocked"
	if result.Verified {
		exp.Summary = "Shell completion setup is verified."
		exp.WhyItMatters = "Completion and PATH wiring are convenience surfaces; verified setup means the command and completion script are reachable where expected."
	} else {
		exp.Summary = "Shell completion setup needs action."
		exp.WhyItMatters = "Completion setup affects how humans and agents discover Burpvalve commands, but explain is read-only and will not install anything."
	}
	exp.NextSteps = append([]string(nil), result.NextSteps...)
	if len(exp.NextSteps) == 0 {
		exp.NextSteps = []string{"No completion recovery step is required."}
	}
	if !result.CompletionExists || !result.CompletionLooksOK {
		exp.Blockers = append(exp.Blockers, explainBlocker{
			ID:      "completion_script",
			Status:  "missing",
			Message: "Completion script is missing or does not look compatible.",
			Command: firstString(exp.NextSteps),
			Fatal:   false,
		})
	}
	if !result.Verified && !result.CommandOnPath && result.CommandOrigin == "missing" {
		exp.Blockers = append(exp.Blockers, explainBlocker{
			ID:      "command_path",
			Status:  result.CommandOrigin,
			Message: "burpvalve was not found on PATH for this shell setup.",
			Command: firstString(exp.NextSteps),
			Fatal:   false,
		})
	}
	if result.RCRequired && !result.RCUpdatePresent {
		exp.Blockers = append(exp.Blockers, explainBlocker{
			ID:      "shell_startup",
			Status:  "missing",
			Message: "Shell startup file does not load the completion directory yet.",
			Command: firstString(exp.NextSteps),
			Fatal:   false,
		})
	}
	return exp
}

func validateExplainArtifact(artifact attestations.Artifact) error {
	if err := artifact.ValidateShape(); err != nil {
		return fmt.Errorf("parse attestation artifact: %w", err)
	}
	switch artifact.ArtifactKind {
	case attestations.ArtifactPassing, attestations.ArtifactBlocked:
		return nil
	default:
		return fmt.Errorf("parse attestation artifact: unknown artifact_kind %q", artifact.ArtifactKind)
	}
}

func explainArtifact(record attestations.Record, artifact attestations.Artifact, source map[string]string) explanation {
	inputType := record.ArtifactType
	if inputType == "" {
		inputType = string(artifact.ArtifactKind)
	}
	exp := baseExplanation(inputType, record.Status, source)
	exp.Fatal = record.Status == "blocked" || artifact.ArtifactKind == attestations.ArtifactBlocked
	exp.FeatureIDs = append([]string(nil), record.FeatureIDs...)
	exp.BeadIDs = append([]string(nil), record.BeadIDs...)
	exp.Summary = "Burpvalve evidence artifact is " + record.Status + "."
	if exp.Fatal {
		exp.Summary = "Blocked report recorded unmet verifier evidence."
	}
	exp.WhyItMatters = "Attestation artifacts preserve traceability between the staged payload, manifest conditions, verifier policy, and the evidence used at commit time."
	exp.NextSteps = append([]string(nil), artifact.NextSteps...)
	supplementalDisagreement := false
	for _, condition := range artifact.Conditions {
		blocker := explainBlocker{
			ID:              condition.ConditionID,
			Status:          string(condition.Verdict),
			Message:         condition.Message,
			ConditionFile:   condition.ConditionFile,
			VerifierPolicy:  condition.VerifierPolicy,
			VerifierKind:    condition.EffectiveVerifierKind(),
			Verdict:         condition.Verdict,
			NextAction:      condition.NextAction,
			EvidenceMissing: len(condition.Evidence) == 0,
		}
		if condition.Verdict != attestations.VerdictPass || !condition.VerifierPolicyAccepted() || len(condition.Evidence) == 0 {
			blocker.Fatal = exp.Fatal
			exp.Blockers = append(exp.Blockers, blocker)
		}
		for _, supplemental := range condition.Supplemental {
			if supplemental.Verdict == attestations.VerdictFail ||
				supplemental.Verdict == attestations.VerdictUnknown ||
				(condition.Verdict != "" && supplemental.Verdict != "" && supplemental.Verdict != condition.Verdict) {
				supplementalDisagreement = true
				message := supplemental.Message
				if message == "" {
					message = "Supplemental verifier disagrees with primary verifier evidence."
				}
				exp.Blockers = append(exp.Blockers, explainBlocker{
					ID:              condition.ConditionID + "/supplemental/" + supplementalVerifierLabel(supplemental),
					Status:          "supplemental_" + string(supplemental.Verdict),
					Message:         message,
					ConditionFile:   condition.ConditionFile,
					VerifierPolicy:  condition.VerifierPolicy,
					VerifierKind:    attestations.EffectiveVerifierKind(supplemental.Verifier, supplemental.SubagentConfirmed),
					Verdict:         supplemental.Verdict,
					NextAction:      firstNonEmptyString(supplemental.NextAction, "Hold and escalate the supplemental verifier disagreement before relying on this artifact."),
					EvidenceMissing: len(supplemental.Evidence) == 0,
					Supplemental:    true,
				})
			}
		}
		if condition.Adjudication != nil {
			for i := range exp.Blockers {
				if exp.Blockers[i].ConditionFile == condition.ConditionFile && exp.Blockers[i].AdjudicationRef == "" {
					exp.Blockers[i].AdjudicationRef = condition.Adjudication.AuditRef
				}
			}
		}
	}
	if len(exp.NextSteps) == 0 {
		if supplementalDisagreement {
			exp.NextSteps = []string{"Hold and escalate supplemental verifier disagreement before relying on this artifact; adjudication final_verdict is audit metadata only."}
		} else if exp.Fatal {
			exp.NextSteps = []string{"Collect acceptable verifier evidence for each blocked condition, then rerun burpvalve commit."}
		} else {
			exp.NextSteps = []string{"No artifact recovery step is required."}
		}
	}
	return exp
}

func supplementalVerifierLabel(s attestations.SupplementalVerifier) string {
	parts := []string{
		string(attestations.EffectiveVerifierKind(s.Verifier, s.SubagentConfirmed)),
		strings.TrimSpace(s.Verifier.Agent),
		strings.TrimSpace(s.Verifier.Model),
		strings.TrimSpace(s.Verifier.Runtime),
	}
	for i, part := range parts {
		if part == "" {
			parts[i] = "unknown"
		}
	}
	return strings.Join(parts, "/")
}

func recordFromArtifact(artifact attestations.Artifact) attestations.Record {
	status := "malformed"
	artifactType := "malformed"
	switch artifact.ArtifactKind {
	case attestations.ArtifactPassing:
		status = "pass"
		artifactType = "passing_attestation"
	case attestations.ArtifactBlocked:
		status = "blocked"
		artifactType = "blocked_report"
	}
	record := attestations.Record{
		SchemaVersion: artifact.SchemaVersion,
		ArtifactType:  artifactType,
		Status:        status,
		PayloadHash:   artifact.StagedPayloadHash,
		ManifestHash:  artifact.ManifestHash,
		GeneratedBy:   artifact.GeneratedBy,
	}
	if !artifact.CreatedAt.IsZero() {
		created := artifact.CreatedAt
		record.CreatedAt = &created
	}
	if artifact.Feature.ID != "" {
		record.FeatureIDs = []string{artifact.Feature.ID}
	}
	record.BeadIDs = appendUniqueStrings(record.BeadIDs, artifact.BeadIDs...)
	record.BeadIDs = appendUniqueStrings(record.BeadIDs, artifact.Feature.BeadIDs...)
	if artifact.Feature.SourceBead != "" && len(record.BeadIDs) == 0 && looksLikeLegacyBeadID(artifact.Feature.SourceBead) {
		record.BeadIDs = appendUniqueStrings(record.BeadIDs, artifact.Feature.SourceBead)
	}
	return record
}

func appendUniqueStrings(values []string, next ...string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range next {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		values = append(values, value)
		seen[value] = true
	}
	return values
}

func looksLikeLegacyBeadID(id string) bool {
	id = strings.TrimSpace(id)
	return strings.HasPrefix(id, "br-") || strings.HasPrefix(id, "bd-")
}

func baseExplanation(inputType string, originalStatus string, source map[string]string) explanation {
	return explanation{
		SchemaVersion:  1,
		Command:        "explain",
		InputType:      inputType,
		Status:         "explained",
		OriginalStatus: originalStatus,
		Source:         source,
		NextSteps:      []string{},
	}
}

func printExplanation(out io.Writer, exp explanation) {
	ui := cliui.New(shouldColorWriter(out))
	fmt.Fprintln(out, ui.Section("Burpvalve explain"))
	fmt.Fprintf(out, "%s %s\n", ui.Label("Input:"), exp.InputType)
	fmt.Fprintf(out, "%s %s\n", ui.Label("Status:"), exp.OriginalStatus)
	fmt.Fprintf(out, "%s %s\n", ui.Label("Fatal:"), ui.Bool(exp.Fatal))
	if len(exp.FeatureIDs) > 0 {
		fmt.Fprintf(out, "%s %s\n", ui.Label("Feature:"), strings.Join(exp.FeatureIDs, ", "))
	}
	if len(exp.BeadIDs) > 0 {
		fmt.Fprintf(out, "%s %s\n", ui.Label("Beads:"), strings.Join(exp.BeadIDs, ", "))
	}
	fmt.Fprintf(out, "%s %s\n", ui.Label("Summary:"), exp.Summary)
	fmt.Fprintf(out, "%s %s\n", ui.Label("Why:"), exp.WhyItMatters)
	if exp.Config != nil {
		fmt.Fprintf(out, "%s global=%s project=%s\n", ui.Label("Config:"), foundWord(exp.Config.GlobalFound), foundWord(exp.Config.ProjectFound))
	}
	if len(exp.Blockers) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, ui.Section("Blockers"))
		for _, blocker := range exp.Blockers {
			fmt.Fprintf(out, "- %s: %s\n", blocker.ID, firstNonEmptyString(blocker.Message, blocker.Status))
			if blocker.VerifierPolicy != "" {
				fmt.Fprintf(out, "  policy: %s verifier: %s verdict: %s\n", blocker.VerifierPolicy, blocker.VerifierKind, blocker.Verdict)
			}
			if blocker.NextAction != "" {
				fmt.Fprintf(out, "  next: %s\n", blocker.NextAction)
			}
		}
	}
	if len(exp.NextSteps) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, ui.Section("Next steps"))
		for _, step := range exp.NextSteps {
			fmt.Fprintf(out, "- %s\n", step)
		}
	}
}

func foundWord(found bool) string {
	if found {
		return "found"
	}
	return "missing"
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func newInitCommand() *cobra.Command {
	var opts initOptions
	cmd := &cobra.Command{
		Use:   "init [target...]",
		Short: "Add Burpvalve files and the local commit check.",
		Long: `Add the standard Burpvalve files to a repo. In a terminal, init opens a
Bubble Tea question-and-answer setup flow. It asks where to install and which
pieces to include, then applies the scaffold. Skip flags hide those pieces from
the questions. Before changing files, init asks for final confirmation and
defaults to No. Use --force to skip questions and apply targets/flags directly.
In scripts, combine --force with --json, or use --robots with JSON input.
If AGENTS.md already exists, init preserves existing text and appends the missing
Burpvalve operating sections, then reports that work under "repaired".

Common targets: AGENTS.md, CLAUDE.md, orchestrator, docs, plans, log,
backpressure, attestations, beads, ntm, hooks, precommit, hooks-path.
ORCHESTRATOR.md is not part of the standard scaffold; request the orchestrator
target, set defaults.init.orchestrator=orchestrator-md, or select the init TUI
checkbox. Use --dogfood or defaults.init.dogfood=true to add findings-log
instructions to the orchestrator contract. Use --repo-bin or the bin target
only when this repo needs a local Burpvalve fallback.`,
		Example: `  burpvalve init
  burpvalve init --force --json
  printf '{"target":".","confirm":true}' | burpvalve init --robots
  burpvalve init AGENTS.md CLAUDE.md
  burpvalve init hooks
  burpvalve init --force --json --git-init hooks
  burpvalve init --force --no-beads --no-ntm
  burpvalve init --no-beads --no-ntm
  burpvalve init --force --json orchestrator --dogfood
  burpvalve init --no-agents --no-claude
  burpvalve init --no-log --no-attestations`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.targets = args
			if robotsMode {
				return runInitRobots(cmd, opts)
			}
			return runInit(opts)
		},
	}
	cmd.Flags().StringVar(&opts.target, "target", ".", "target project root")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "print machine-readable JSON")
	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "skip prompts and apply targets/flags directly")
	cmd.Flags().BoolVar(&opts.noBeads, "no-beads", false, "skip .beads initialization and verification")
	cmd.Flags().BoolVar(&opts.noNTM, "no-ntm", false, "skip ntm quick registration and snapshot verification")
	cmd.Flags().BoolVar(&opts.noClaude, "no-claude", false, "skip CLAUDE.md symlink creation")
	cmd.Flags().BoolVar(&opts.noClaudeSymlink, "no-claude-symlink", false, "alias for --no-claude")
	cmd.Flags().StringVar(&opts.claudeRoute, "claude-route", "", "Claude route: agent-symlink, orchestrator-skill, or none")
	cmd.Flags().BoolVar(&opts.noAgents, "no-agents", false, "skip AGENTS.md creation")
	cmd.Flags().BoolVar(&opts.noAgentsMD, "no-agents-md", false, "alias for --no-agents")
	cmd.Flags().BoolVar(&opts.noDocs, "no-docs", false, "skip docs files")
	cmd.Flags().BoolVar(&opts.noPlans, "no-plans", false, "skip plans files")
	cmd.Flags().BoolVar(&opts.noLog, "no-log", false, "skip log files")
	cmd.Flags().BoolVar(&opts.noBackpressure, "no-backpressure", false, "skip backpressure rule files")
	cmd.Flags().BoolVar(&opts.noAttestations, "no-attestations", false, "skip backpressure/attestations")
	cmd.Flags().BoolVar(&opts.noHooks, "no-hooks", false, "skip pre-commit hook and hooksPath")
	cmd.Flags().BoolVar(&opts.noGitHooks, "no-git-hooks", false, "alias for --no-hooks")
	cmd.Flags().BoolVar(&opts.noPreCommit, "no-precommit", false, "skip .githooks/pre-commit")
	cmd.Flags().BoolVar(&opts.noHooksPath, "no-hooks-path", false, "skip git core.hooksPath configuration")
	cmd.Flags().BoolVar(&opts.gitInit, "git-init", false, "run git init before wiring hooks when the target is not already a Git repo")
	cmd.Flags().BoolVar(&opts.repoBin, "repo-bin", false, "install optional repo-local bin/burpvalve fallback")
	cmd.Flags().BoolVar(&opts.noBin, "no-bin", false, "do not install optional repo-local bin/burpvalve fallback")
	cmd.Flags().BoolVar(&opts.noToolDocs, "no-tool-docs", false, "skip tools/burpvalve documentation")
	cmd.Flags().BoolVar(&opts.dogfood, "dogfood", false, "add dogfooding findings-log instructions to ORCHESTRATOR.md")
	cmd.Flags().BoolVar(&opts.noDogfood, "no-dogfood", false, "disable dogfooding findings-log instructions even when config enables them")
	return cmd
}

func newRepairCommand() *cobra.Command {
	var opts repairOptions
	cmd := &cobra.Command{
		Use:   "repair [target...]",
		Short: "Put missing Burpvalve files back safely.",
		Long: `Recreate missing Burpvalve-generated files. Repair keeps existing project
notes and reports a conflict instead of replacing a file when the safe choice is
not obvious.

In a terminal, repair opens a Bubble Tea question-and-answer repair flow. Skip
flags hide those pieces from the questions. Before changing files, repair asks
for final confirmation and defaults to No. Use --force to skip questions and
apply targets/flags directly. Repairing CLAUDE.md also repairs AGENTS.md by default,
imports any existing CLAUDE.md content into AGENTS.md, then replaces CLAUDE.md
with a symlink.

Common targets: AGENTS.md, CLAUDE.md, orchestrator, docs, plans, log,
backpressure, attestations, beads, hooks, precommit, hooks-path.
ORCHESTRATOR.md repair is opt-in: request the orchestrator target or set
defaults.repair.orchestrator=true. Use --repo-bin or the bin target only when
this repo needs a local Burpvalve fallback.`,
		Example: `  burpvalve repair
  burpvalve repair AGENTS.md
  printf '{"targets":["AGENTS.md"],"confirm":true}' | burpvalve repair --robots
  burpvalve repair --force CLAUDE.md --no-agents
  burpvalve repair log attestations
  burpvalve repair hooks
  burpvalve repair --force --json --git-init hooks-path
  burpvalve repair --force --json
  burpvalve repair --target /path/to/repo`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.targets = args
			if robotsMode {
				return runRepairRobots(cmd, opts)
			}
			return runRepair(opts)
		},
	}
	cmd.Flags().StringVar(&opts.target, "target", ".", "target project root")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "print machine-readable JSON")
	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "skip prompts and apply targets/flags directly")
	cmd.Flags().BoolVar(&opts.noBeads, "no-beads", false, "skip .beads verification or creation")
	cmd.Flags().BoolVar(&opts.noClaude, "no-claude", false, "skip CLAUDE.md symlink repair")
	cmd.Flags().BoolVar(&opts.noClaudeSymlink, "no-claude-symlink", false, "alias for --no-claude")
	cmd.Flags().StringVar(&opts.claudeRoute, "claude-route", "", "Claude repair route: preserve, agent-symlink, orchestrator-skill, or none")
	cmd.Flags().BoolVar(&opts.adoptClaudeMD, "adopt-claude-md", false, "import an unmarked regular CLAUDE.md into AGENTS.md before applying the selected repair route")
	cmd.Flags().BoolVar(&opts.noAgents, "no-agents", false, "skip AGENTS.md creation or append repair")
	cmd.Flags().BoolVar(&opts.noAgentsMD, "no-agents-md", false, "alias for --no-agents")
	cmd.Flags().BoolVar(&opts.noDocs, "no-docs", false, "skip docs files")
	cmd.Flags().BoolVar(&opts.noPlans, "no-plans", false, "skip plans files")
	cmd.Flags().BoolVar(&opts.noLog, "no-log", false, "skip log files")
	cmd.Flags().BoolVar(&opts.noBackpressure, "no-backpressure", false, "skip backpressure rule files")
	cmd.Flags().BoolVar(&opts.noAttestations, "no-attestations", false, "skip backpressure/attestations")
	cmd.Flags().BoolVar(&opts.noHooks, "no-hooks", false, "skip pre-commit hook and hooksPath")
	cmd.Flags().BoolVar(&opts.noGitHooks, "no-git-hooks", false, "alias for --no-hooks")
	cmd.Flags().BoolVar(&opts.noPreCommit, "no-precommit", false, "skip .githooks/pre-commit")
	cmd.Flags().BoolVar(&opts.noHooksPath, "no-hooks-path", false, "skip git core.hooksPath configuration")
	cmd.Flags().BoolVar(&opts.gitInit, "git-init", false, "run git init before repairing hook wiring when the target is not already a Git repo")
	cmd.Flags().BoolVar(&opts.repoBin, "repo-bin", false, "install optional repo-local bin/burpvalve fallback")
	cmd.Flags().BoolVar(&opts.noBin, "no-bin", false, "do not install optional repo-local bin/burpvalve fallback")
	cmd.Flags().BoolVar(&opts.noToolDocs, "no-tool-docs", false, "skip tools/burpvalve documentation")
	return cmd
}

func newCommitCommand() *cobra.Command {
	var opts commitOptions
	cmd := &cobra.Command{
		Use:     "commit",
		Aliases: []string{"pre-commit"},
		Short:   "Check the files you are about to commit.",
		Long: `Check the work unit (the atomic staged change) against the enabled
backpressure rules before Git commits it.

Burpvalve writes a seal, also called an attestation (the evidence artifact),
when the change is ready. If the staged diff needs an explicit feature id,
commit asks for it. If evidence is missing and a terminal is available, commit
asks Bubble Tea questions for each verifier cell. When the valve (the fail-closed
commit gate) refuses the work unit, blocked reports may say it was burped back,
meaning refused by the valve.
If --responses is omitted, noninteractive runs look for the hash-bound file
created by verifier begin and verifier submit. Use --responses FILE for an
explicit legacy response file override, or set NO_TUI=1 to keep terminal prompts
plain. Use --responses-template to print a JSON file skeleton for the current
staged feature and enabled conditions. The git hook calls this command before a
commit finishes.`,
		Example: `  burpvalve commit
  burpvalve commit --feature br-123
  burpvalve commit --bead br-123 --feature docs-example
  burpvalve commit --responses-template --feature br-123 > responses.json
  burpvalve commit --responses responses.json --agent codex --model gpt-5`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if robotsMode {
				return runCommitRobots(cmd, opts)
			}
			return runPreCommit(opts)
		},
	}
	cmd.Flags().StringVar(&opts.root, "root", ".", "repository root")
	cmd.Flags().StringVar(&opts.feature, "feature", "", "explicit atomic feature or bead id for staged changes")
	cmd.Flags().StringArrayVar(&opts.beads, "bead", nil, "delivery bead id to record in attestation metadata; repeat for coupled work")
	cmd.Flags().StringVar(&opts.beadRationale, "bead-rationale", "", "why multiple --bead ids belong to one staged payload")
	cmd.Flags().StringVar(&opts.responses, "responses", "", "JSON matrix responses for non-interactive pre-commit artifact processing")
	cmd.Flags().BoolVar(&opts.responsesTemplate, "responses-template", false, "print a JSON response template for the current staged condition matrix")
	cmd.Flags().StringVar(&opts.agent, "agent", "codex", "agent name recorded in generated artifacts")
	cmd.Flags().StringVar(&opts.model, "model", "unspecified", "model name recorded in generated artifacts")
	return cmd
}

func newLintCommand() *cobra.Command {
	var root string
	var jsonOutput bool
	var jobs int
	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Run lint commands listed by this repo.",
		Long: `Run exact lint, format, or static-analysis commands listed in
backpressure/manifest.yaml. Notes in backpressure/lint-rules.md stay as reminders
until they are wired to real commands.`,
		Example: `  burpvalve lint
  burpvalve lint --root /path/to/repo
  burpvalve lint init --detect --json
  burpvalve lint init --write --force --preset go --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if robotsMode {
				return runLintRobots(cmd, root, jobs)
			}
			return runLint(root, jsonOutput, jobs)
		},
	}
	cmd.Flags().StringVar(&root, "root", ".", "repository root")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "print machine-readable JSON without human stderr summary")
	cmd.Flags().IntVar(&jobs, "jobs", 4, "maximum lint command jobs to run")
	cmd.AddCommand(newLintInitCommand())
	return cmd
}

func newLintInitCommand() *cobra.Command {
	var opts lintInitOptions
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Detect and optionally write lint setup.",
		Long: `Detect Go and Node/Astro lint setup and preview exact manifest changes.
The command is read-only by default. Use --write with confirmation, --force, or
robot stdin JSON confirm=true before it writes backpressure/manifest.yaml or the
marked recommendations section in backpressure/lint-rules.md.`,
		Example: `  burpvalve lint init --detect --json
  burpvalve lint init --write --force --preset go --json
  printf '{"root":".","write":true,"preset":"go","confirm":true}' | burpvalve lint init --robots`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if robotsMode {
				return runLintInitRobots(cmd, opts)
			}
			return runLintInit(opts)
		},
	}
	cmd.Flags().StringVar(&opts.root, "root", ".", "repository root")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "print machine-readable JSON without human stderr summary")
	cmd.Flags().BoolVar(&opts.detect, "detect", false, "detect and preview lint setup without writing files")
	cmd.Flags().BoolVar(&opts.write, "write", false, "write proposed manifest and lint-rules recommendations after confirmation")
	cmd.Flags().StringVar(&opts.preset, "preset", "auto", "built-in preset to apply: auto, go, node, or astro")
	cmd.Flags().BoolVar(&opts.force, "force", false, "skip confirmation only when --write is also set")
	cmd.Flags().IntVar(&opts.jobs, "jobs", 4, "maximum lint command jobs metadata to record")
	return cmd
}

func newCICommand() *cobra.Command {
	var opts ciOptions
	cmd := &cobra.Command{
		Use:   "ci",
		Short: "Check Burpvalve evidence in scripts or CI.",
		Long: `Check that the staged change or latest commit has matching Burpvalve evidence.
Use this in automation after commits, or locally when you want the same check
without making a commit.`,
		Example: `  burpvalve ci
  burpvalve ci --feature br-123
  burpvalve ci --commit HEAD
  burpvalve ci --root /path/to/repo`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if robotsMode {
				return runCIRobots(cmd, opts)
			}
			return runCI(opts)
		},
	}
	cmd.Flags().StringVar(&opts.root, "root", ".", "repository root")
	cmd.Flags().StringVar(&opts.feature, "feature", "", "explicit atomic feature or bead id for staged changes")
	cmd.Flags().StringVar(&opts.commit, "commit", "", "validate evidence stored in this commit; --feature is an assertion when set")
	return cmd
}

type hashOptions struct {
	root       string
	staged     bool
	jsonOutput bool
}

type stagedHashResult struct {
	SchemaVersion             int                              `json:"schema_version"`
	Command                   string                           `json:"command"`
	Status                    string                           `json:"status"`
	Message                   string                           `json:"message"`
	Fatal                     bool                             `json:"fatal"`
	StagedPayloadHash         string                           `json:"staged_payload_hash"`
	IncludedPaths             []string                         `json:"included_paths"`
	StagedPayload             []backpressure.StagedPayloadFile `json:"staged_payload"`
	HashExcludedStagedPayload []backpressure.StagedPayloadFile `json:"hash_excluded_staged_payload"`
	ExcludedPaths             []string                         `json:"excluded_paths"`
	GeneratedPathPrefixes     []string                         `json:"generated_path_prefixes"`
	Warning                   string                           `json:"warning"`
}

func newHashCommand() *cobra.Command {
	var opts hashOptions
	cmd := &cobra.Command{
		Use:   "hash --staged",
		Short: "Reproduce Burpvalve payload hashes.",
		Long: `Reproduce Burpvalve's canonical staged payload hash for verifier work.
This command is read-only. It uses the same HashStagedPayload path as
burpvalve commit and burpvalve verifier prompts, including generated evidence
exclusions. Do not substitute naive git diff hashing for this helper.`,
		Example: `  burpvalve hash --staged
  burpvalve hash --staged --json
  burpvalve hash --staged --root /path/to/repo --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHash(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.root, "root", ".", "repository root")
	cmd.Flags().BoolVar(&opts.staged, "staged", false, "hash the current staged payload")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "print machine-readable JSON")
	return cmd
}

func runHash(cmd *cobra.Command, opts hashOptions) error {
	if !opts.staged {
		return fail(2, "hash currently supports only --staged; pass --staged to reproduce the current index payload hash")
	}
	ctx, cancel := backpressure.WithTimeout(context.Background())
	defer cancel()
	reader := backpressure.GitStagedReader{}
	payload, err := backpressure.HashStagedPayload(ctx, opts.root, reader)
	if err != nil {
		return fail(2, "hash staged payload: %v", err)
	}
	allEntries, err := reader.StagedEntries(ctx, opts.root)
	if err != nil {
		return fail(2, "inspect staged payload: %v", err)
	}
	excluded := stagedHashExcludedFiles(payload.ExcludedPaths, allEntries)
	result := stagedHashResult{
		SchemaVersion:             1,
		Command:                   "hash",
		Status:                    "completed",
		Message:                   "Computed canonical Burpvalve staged payload hash.",
		Fatal:                     false,
		StagedPayloadHash:         payload.Hash,
		IncludedPaths:             append([]string(nil), payload.IncludedPaths...),
		StagedPayload:             append([]backpressure.StagedPayloadFile(nil), payload.IncludedFiles...),
		HashExcludedStagedPayload: excluded,
		ExcludedPaths:             append([]string(nil), payload.ExcludedPaths...),
		GeneratedPathPrefixes:     gitindex.GeneratedPathPrefixes(),
		Warning:                   stagedHashWarning(),
	}
	if opts.jsonOutput || robotsMode {
		return encodeJSON(cmd.OutOrStdout(), result, "encode staged hash")
	}
	printStagedHash(cmd.OutOrStdout(), result)
	return nil
}

func stagedHashExcludedFiles(paths []string, entries []backpressure.StagedPayloadFile) []backpressure.StagedPayloadFile {
	byPath := make(map[string]backpressure.StagedPayloadFile, len(entries))
	for _, entry := range entries {
		entry.Path = filepath.ToSlash(entry.Path)
		entry.OldPath = filepath.ToSlash(entry.OldPath)
		if entry.Status == "" {
			entry.Status = "modified"
		}
		byPath[entry.Path] = entry
	}
	excluded := make([]backpressure.StagedPayloadFile, 0, len(paths))
	for _, path := range paths {
		if entry, ok := byPath[filepath.ToSlash(path)]; ok {
			excluded = append(excluded, entry)
		}
	}
	return excluded
}

func stagedHashWarning() string {
	return "Naive git diff hashing is not equivalent; Burpvalve hashes sorted staged entries with normalized metadata, staged content for non-deleted files, and generated evidence JSON exclusions."
}

func printStagedHash(out io.Writer, result stagedHashResult) {
	fmt.Fprintln(out, "Burpvalve staged payload hash")
	fmt.Fprintf(out, "Hash: %s\n", result.StagedPayloadHash)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Hash-included staged paths:")
	printStagedHashFiles(out, result.StagedPayload)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Hash-excluded generated evidence paths:")
	printStagedHashFiles(out, result.HashExcludedStagedPayload)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Generated evidence prefixes:")
	if len(result.GeneratedPathPrefixes) == 0 {
		fmt.Fprintln(out, "  (none)")
	} else {
		for _, prefix := range result.GeneratedPathPrefixes {
			fmt.Fprintf(out, "  - %s\n", prefix)
		}
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Warning: %s\n", result.Warning)
}

func printStagedHashFiles(out io.Writer, files []backpressure.StagedPayloadFile) {
	if len(files) == 0 {
		fmt.Fprintln(out, "  (none)")
		return
	}
	for _, file := range files {
		status := file.GitStatus
		if status == "" {
			status = file.Status
		}
		if file.OldPath != "" {
			fmt.Fprintf(out, "  - %s %s (from %s)\n", status, file.Path, file.OldPath)
			continue
		}
		fmt.Fprintf(out, "  - %s %s\n", status, file.Path)
	}
}

type accountPayloadOptions struct {
	root             string
	ownershipFile    string
	includeUntracked bool
	includeBeads     bool
	jsonOutput       bool
}

func newAccountCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "account",
		Short: "Inspect ownership accounting for staged payloads.",
		Long: `Inspect explicit ownership records against the current staged payload.
This command is read-only. It does not infer Beads ownership, reserve files,
stage files, or change the worktree.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newAccountPayloadCommand())
	return cmd
}

func newAccountPayloadCommand() *cobra.Command {
	var opts accountPayloadOptions
	cmd := &cobra.Command{
		Use:   "payload",
		Short: "Classify staged and untracked paths by explicit ownership records.",
		Long: `Classify the current staged payload against explicit ownership records.
Records can come from --ownership-file, stdin JSON, or both. When both are
present, stdin wins for the same unit/path/kind claim. Use --include-untracked
to also report ignored/generated/covered/unowned untracked paths.
Use --include-beads only for display-only active Beads context. Beads metadata
does not create ownership claims; explicit records remain authoritative.`,
		Example: `  burpvalve account payload --ownership-file ownership.json
  burpvalve account payload --ownership-file ownership.json --include-untracked --json
  burpvalve account payload --ownership-file ownership.json --include-beads --json
  cat ownership.json | burpvalve account payload --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAccountPayload(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.root, "root", ".", "repository root")
	cmd.Flags().StringVar(&opts.ownershipFile, "ownership-file", "", "JSON ownership records file")
	cmd.Flags().BoolVar(&opts.includeUntracked, "include-untracked", false, "include untracked paths in the accounting report")
	cmd.Flags().BoolVar(&opts.includeBeads, "include-beads", false, "include display-only active Beads metadata without creating ownership claims")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "print machine-readable JSON")
	return cmd
}

func runAccountPayload(cmd *cobra.Command, opts accountPayloadOptions) error {
	input, err := readAccountPayloadOwnershipInput(cmd, opts.ownershipFile)
	if err != nil {
		return err
	}
	ctx, cancel := backpressure.WithTimeout(context.Background())
	defer cancel()
	result, err := backpressure.RunOwnershipAccounting(ctx, backpressure.OwnershipAccountingOptions{
		Root:             opts.root,
		Ownership:        input,
		IncludeUntracked: opts.includeUntracked,
		IncludeBeads:     opts.includeBeads,
	})
	if err != nil {
		return fail(2, "account payload: %v", err)
	}
	if opts.jsonOutput || robotsMode {
		return encodeJSON(cmd.OutOrStdout(), result, "encode ownership accounting")
	}
	printOwnershipAccounting(cmd.OutOrStdout(), result)
	return nil
}

func readAccountPayloadOwnershipInput(cmd *cobra.Command, path string) (backpressure.OwnershipInput, error) {
	var fileInput backpressure.OwnershipInput
	if strings.TrimSpace(path) != "" {
		file, err := os.Open(path)
		if err != nil {
			return backpressure.OwnershipInput{}, fail(2, "read ownership file: %v", err)
		}
		defer file.Close()
		parsed, err := backpressure.ParseOwnershipInput(file)
		if err != nil {
			return backpressure.OwnershipInput{}, fail(2, "parse ownership file: %v", err)
		}
		fileInput = parsed
	}
	stdinInput, err := readOptionalOwnershipStdin(cmd.InOrStdin())
	if err != nil {
		return backpressure.OwnershipInput{}, err
	}
	merged := backpressure.MergeOwnershipInputs(fileInput, stdinInput)
	if err := backpressure.ValidateOwnershipInput(&merged); err != nil {
		return backpressure.OwnershipInput{}, fail(2, "validate ownership records: %v", err)
	}
	return merged, nil
}

func readOptionalOwnershipStdin(in io.Reader) (backpressure.OwnershipInput, error) {
	if file, ok := in.(*os.File); ok {
		info, err := file.Stat()
		if err == nil && info.Mode()&os.ModeCharDevice != 0 {
			return backpressure.OwnershipInput{}, nil
		}
	}
	body, err := io.ReadAll(in)
	if err != nil {
		return backpressure.OwnershipInput{}, fail(2, "read ownership stdin: %v", err)
	}
	if strings.TrimSpace(string(body)) == "" {
		return backpressure.OwnershipInput{}, nil
	}
	input, err := backpressure.ParseOwnershipInput(bytes.NewReader(body))
	if err != nil {
		return backpressure.OwnershipInput{}, fail(2, "parse ownership stdin: %v", err)
	}
	return input, nil
}

func printOwnershipAccounting(out io.Writer, result backpressure.OwnershipAccountingResult) {
	fmt.Fprintln(out, "Burpvalve payload ownership accounting")
	fmt.Fprintf(out, "Status: %s\n", result.Status)
	fmt.Fprintf(out, "Read-only: %t\n", !result.Mutating)
	if result.Beads != nil {
		fmt.Fprintf(out, "Beads context: available=%t display_only=%t active=%d\n", result.Beads.Available, result.Beads.DisplayOnly, len(result.Beads.Active))
		for _, warning := range result.Beads.Warnings {
			fmt.Fprintf(out, "  warning: %s\n", warning)
		}
	}
	fmt.Fprintf(out, "Staged paths: %d\n", result.Summary.StagedTotal)
	printOwnershipPathResults(out, "Staged", result.Staged)
	if len(result.Untracked) > 0 {
		fmt.Fprintf(out, "\nUntracked paths: %d\n", result.Summary.UntrackedTotal)
		printOwnershipPathResults(out, "Untracked", result.Untracked)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Summary: owned=%d shared_declared=%d conflicts=%d unowned=%d generated_exception=%d ignored_untracked=%d covered_exception=%d\n",
		result.Summary.Owned,
		result.Summary.SharedDeclared,
		result.Summary.Conflicts,
		result.Summary.Unowned,
		result.Summary.Generated,
		result.Summary.Ignored,
		result.Summary.Covered,
	)
}

func printOwnershipPathResults(out io.Writer, label string, results []backpressure.OwnershipPathResult) {
	if len(results) == 0 {
		fmt.Fprintf(out, "%s: (none)\n", label)
		return
	}
	fmt.Fprintf(out, "%s:\n", label)
	for _, result := range results {
		owners := "(none)"
		if len(result.OwnerUnitIDs) > 0 {
			owners = strings.Join(result.OwnerUnitIDs, ",")
		}
		fmt.Fprintf(out, "  - %s %s owners=%s", result.Status, result.Path, owners)
		if result.GitStatus != "" {
			fmt.Fprintf(out, " git=%s", result.GitStatus)
		}
		if result.Ignored {
			fmt.Fprint(out, " ignored=true")
		}
		if result.Generated {
			fmt.Fprint(out, " generated=true")
		}
		if len(result.BeadsContext) > 0 {
			ids := make([]string, 0, len(result.BeadsContext))
			for _, bead := range result.BeadsContext {
				ids = append(ids, bead.ID)
			}
			fmt.Fprintf(out, " beads_context=%s", strings.Join(ids, ","))
		}
		if result.Rationale != "" {
			fmt.Fprintf(out, " rationale=%q", result.Rationale)
		}
		fmt.Fprintln(out)
	}
}

func newPromptsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prompts",
		Short: "List and render canonical orchestrator prompt templates.",
		Long: `List and render canonical Burpvalve prompt templates embedded in the binary.
Prompt names are stable public API. This command serves reusable orchestrator
templates; use burpvalve verifier prompts for staged-payload verifier packets
with binding hashes.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newPromptsListCommand(), newPromptsShowCommand())
	return cmd
}

func newPromptsListCommand() *cobra.Command {
	var opts promptListOptions
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List canonical prompt names.",
		Long: `List stable canonical prompt names, versions, descriptions, and declared variables.
For staged-payload verifier packets, use burpvalve verifier prompts instead.`,
		Example: `  burpvalve prompts list
  burpvalve prompts list --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromptsList(cmd, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "print machine-readable prompt list")
	return cmd
}

func newPromptsShowCommand() *cobra.Command {
	var opts promptShowOptions
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Render one canonical prompt template.",
		Long: `Render one canonical prompt template by stable name.
Use --var key=value for declared variables. Missing required variables are usage
errors that list every missing name. For verifier packets bound to the current
staged payload, use burpvalve verifier prompts instead.

Use --write to export a local copy under docs/prompts/. Exported prompt files
are not authoritative; the embedded prompt bank remains the source of truth.`,
		Example: `  burpvalve prompts show marching-orders --var agent=LilacGlacier --var bead=br-123
  burpvalve prompts show marching-orders --var agent=LilacGlacier --var bead=br-123 --json
  burpvalve prompts show verifier-bootstrap --var agent=BrightOwl --var project_key=/repo --var orchestrator=RusticDog --write`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fail(2, "prompts show requires exactly one prompt name; valid prompts: %s", strings.Join(backpressure.PromptNames(), ", "))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPromptsShow(cmd, opts, args[0])
		},
	}
	cmd.Flags().StringArrayVar(&opts.vars, "var", nil, "render variable assignment as key=value; repeat for multiple variables")
	cmd.Flags().StringVar(&opts.root, "root", ".", "repository root for docs/prompts exports")
	cmd.Flags().BoolVar(&opts.write, "write", false, "write the rendered prompt to docs/prompts/<name>.md")
	cmd.Flags().BoolVar(&opts.force, "force", false, "overwrite a locally modified prompt export")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "print machine-readable rendered prompt")
	return cmd
}

func runPromptsList(cmd *cobra.Command, opts promptListOptions) error {
	items := backpressure.ListPromptBank()
	if opts.jsonOutput {
		return encodeJSON(cmd.OutOrStdout(), map[string]any{"prompts": items}, "encode prompt list")
	}
	printPromptsList(cmd.OutOrStdout(), items)
	return nil
}

func runPromptsShow(cmd *cobra.Command, opts promptShowOptions, name string) error {
	values, err := parsePromptVars(opts.vars)
	if err != nil {
		return fail(2, "%v", err)
	}
	rendered, err := backpressure.ShowPrompt(name, values)
	if err != nil {
		return fail(2, "%v", err)
	}
	root, err := filepath.Abs(defaultCLIRoot(opts.root))
	if err != nil {
		root = defaultCLIRoot(opts.root)
	}
	if opts.write {
		result, err := backpressure.WritePromptExport(root, rendered, version, opts.force)
		if err != nil {
			return fail(2, "%v", err)
		}
		if opts.jsonOutput {
			return encodeJSON(cmd.OutOrStdout(), result, "encode prompt export")
		}
		printPromptExport(cmd.OutOrStdout(), result)
		return nil
	}
	status, err := backpressure.PromptExportStatusFor(root, rendered)
	if err != nil {
		return fail(2, "%v", err)
	}
	if status.Exists && status.Divergent {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: local prompt export %s differs from the embedded canonical prompt; rendering embedded canonical content\n", status.Path)
	}
	if opts.jsonOutput {
		return encodeJSON(cmd.OutOrStdout(), rendered, "encode rendered prompt")
	}
	printPrompt(cmd.OutOrStdout(), rendered)
	return nil
}

func parsePromptVars(assignments []string) (map[string]string, error) {
	values := map[string]string{}
	for _, assignment := range assignments {
		key, value, ok := strings.Cut(assignment, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("--var must be key=value, got %q", assignment)
		}
		values[strings.TrimSpace(key)] = value
	}
	return values, nil
}

type verifierPromptOptions struct {
	root       string
	feature    string
	condition  string
	profile    string
	jsonOutput bool
}

type promptShowOptions struct {
	jsonOutput bool
	vars       []string
	root       string
	write      bool
	force      bool
}

type promptListOptions struct {
	jsonOutput bool
}

type verifierBeginOptions struct {
	root             string
	feature          string
	oneFeature       bool
	atomicityMessage string
	jsonOutput       bool
}

type verifierSubmitOptions struct {
	root              string
	feature           string
	condition         string
	responses         string
	stagedPayloadHash string
	manifestHash      string
	conditionFileHash string
	transcript        string
	jsonOutput        bool
}

type verifierDoctorOptions struct {
	root       string
	jsonOutput bool
}

func newVerifierCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verifier",
		Short: "Generate verifier handoff packets.",
		Long:  "Generate read-only verifier handoff packets and inspect verifier-runtime limits without mutating runtime configuration.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newVerifierBeginCommand())
	cmd.AddCommand(newVerifierDoctorCommand())
	cmd.AddCommand(newVerifierPromptsCommand())
	cmd.AddCommand(newVerifierSubmitCommand())
	return cmd
}

func newVerifierBeginCommand() *cobra.Command {
	var opts verifierBeginOptions
	cmd := &cobra.Command{
		Use:   "begin",
		Short: "Create a bound verifier response file.",
		Long: `Create the hash-keyed verifier response file for the current staged payload.
The file is bound to the staged payload hash, manifest hash, and condition file
hashes. Atomicity is supplied once here and preserved for later submit steps.`,
		Example: `  burpvalve verifier begin --feature br-123 --one-feature --atomicity-message "Staged changes map only to br-123" --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerifierBegin(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.root, "root", ".", "repository root")
	cmd.Flags().StringVar(&opts.feature, "feature", "", "explicit atomic feature or bead id for staged changes")
	cmd.Flags().BoolVar(&opts.oneFeature, "one-feature", false, "confirm the staged payload is exactly one feature or bug fix")
	cmd.Flags().StringVar(&opts.atomicityMessage, "atomicity-message", "", "why the staged payload is exactly one feature or bug fix")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "print machine-readable result")
	return cmd
}

func newVerifierSubmitCommand() *cobra.Command {
	var opts verifierSubmitOptions
	cmd := &cobra.Command{
		Use:   "submit",
		Short: "Merge one verifier response into a bound response file.",
		Long: `Merge one real verifier response into the hash-keyed response file.
The submitted JSON and command flags must match the current staged payload,
manifest hash, and condition file hash. Submit preserves begin atomicity and
updates only the named condition under a response-file lock.`,
		Example: `  burpvalve verifier submit --feature br-123 --condition dry --staged-payload-hash sha256:... --manifest-hash sha256:... --condition-file-hash sha256:... --json < verdict.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerifierSubmit(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.root, "root", ".", "repository root")
	cmd.Flags().StringVar(&opts.feature, "feature", "", "explicit atomic feature or bead id for staged changes")
	cmd.Flags().StringVar(&opts.condition, "condition", "", "enabled condition id to update")
	cmd.Flags().StringVar(&opts.responses, "responses", "", "bound verifier responses file to update")
	cmd.Flags().StringVar(&opts.stagedPayloadHash, "staged-payload-hash", "", "staged payload hash from the verifier packet")
	cmd.Flags().StringVar(&opts.manifestHash, "manifest-hash", "", "manifest hash from the verifier packet")
	cmd.Flags().StringVar(&opts.conditionFileHash, "condition-file-hash", "", "condition file hash from the verifier packet")
	cmd.Flags().StringVar(&opts.transcript, "transcript", "", "optional transcript path, or - to read transcript bytes after stdin JSON")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "print machine-readable result")
	return cmd
}

func newVerifierDoctorCommand() *cobra.Command {
	var opts verifierDoctorOptions
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Report verifier runtime limits without writing config.",
		Long: `Inspect known verifier runtime config files and report subagent limits
without writing, repairing, staging, or suggesting exact edits for unsupported
formats. Unknown or malformed config content is reported with supported=false.`,
		Example: `  burpvalve verifier doctor --json
  burpvalve verifier doctor --root /path/to/repo --color never`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerifierDoctor(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.root, "root", ".", "repository root")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "print machine-readable result")
	return cmd
}

func newVerifierPromptsCommand() *cobra.Command {
	var opts verifierPromptOptions
	cmd := &cobra.Command{
		Use:   "prompts",
		Short: "Generate one verifier prompt per feature x condition cell.",
		Long: `Generate copyable verifier packets for the current staged payload.
Burpvalve does not spawn subagents here; it explains what to delegate and what
evidence the verifier must return.`,
		Example: `  burpvalve verifier prompts --feature br-123
  burpvalve verifier prompts --feature br-123 --condition dry --json
  burpvalve verifier prompts --profile ntm --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerifierPrompts(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.root, "root", ".", "repository root")
	cmd.Flags().StringVar(&opts.feature, "feature", "", "explicit atomic feature or bead id for staged changes")
	cmd.Flags().StringVar(&opts.condition, "condition", "", "limit output to one enabled condition id")
	cmd.Flags().StringVar(&opts.profile, "profile", "native", "handoff profile: native, ntm, hermes, or manual")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "print machine-readable JSON")
	return cmd
}

func runVerifierBegin(cmd *cobra.Command, opts verifierBeginOptions) error {
	ctx, cancel := backpressure.WithTimeout(context.Background())
	defer cancel()
	result, err := backpressure.RunVerifierBegin(ctx, backpressure.BeginResponsesOptions{
		Root:             opts.root,
		ExplicitFeature:  opts.feature,
		OneFeature:       opts.oneFeature,
		AtomicityMessage: opts.atomicityMessage,
	})
	if opts.jsonOutput {
		if encodeErr := encodeJSON(cmd.OutOrStdout(), result, "encode verifier begin"); encodeErr != nil {
			return encodeErr
		}
	} else {
		printVerifierBeginResult(cmd.OutOrStdout(), result)
	}
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), result.Message)
		return exitCode(2)
	}
	return nil
}

func runVerifierSubmit(cmd *cobra.Command, opts verifierSubmitOptions) error {
	ctx, cancel := backpressure.WithTimeout(context.Background())
	defer cancel()
	input, transcriptReader, err := readVerifierSubmitInput(cmd, opts)
	if err != nil {
		return fail(2, "%v", err)
	}
	result, err := backpressure.RunVerifierSubmit(ctx, backpressure.SubmitVerifierOptions{
		Root:              opts.root,
		ExplicitFeature:   opts.feature,
		ConditionID:       opts.condition,
		ResponsesPath:     opts.responses,
		StagedPayloadHash: opts.stagedPayloadHash,
		ManifestHash:      opts.manifestHash,
		ConditionFileHash: opts.conditionFileHash,
		Response:          input,
		TranscriptPath:    opts.transcript,
		TranscriptReader:  transcriptReader,
	})
	if opts.jsonOutput {
		if encodeErr := encodeJSON(cmd.OutOrStdout(), result, "encode verifier submit"); encodeErr != nil {
			return encodeErr
		}
	} else {
		printVerifierSubmitResult(cmd.OutOrStdout(), result)
	}
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), result.Message)
		return exitCode(2)
	}
	return nil
}

func readVerifierSubmitInput(cmd *cobra.Command, opts verifierSubmitOptions) (*backpressure.SubmitVerifierInput, io.Reader, error) {
	reader := cmd.InOrStdin()
	dec := json.NewDecoder(reader)
	dec.DisallowUnknownFields()
	var input backpressure.SubmitVerifierInput
	if err := dec.Decode(&input); err != nil {
		return nil, nil, fmt.Errorf("parse verifier response JSON from stdin: %w", err)
	}
	if opts.transcript == "-" {
		return &input, io.MultiReader(dec.Buffered(), reader), nil
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return nil, nil, fmt.Errorf("verifier submit stdin must contain exactly one JSON object")
	} else if err != io.EOF {
		return nil, nil, fmt.Errorf("parse verifier response JSON from stdin: %w", err)
	}
	return &input, nil, nil
}

func runVerifierDoctor(cmd *cobra.Command, opts verifierDoctorOptions) error {
	ctx, cancel := backpressure.WithTimeout(context.Background())
	defer cancel()
	result, err := backpressure.RunVerifierDoctor(ctx, backpressure.VerifierDoctorOptions{
		Root: opts.root,
	})
	if opts.jsonOutput {
		if encodeErr := encodeJSON(cmd.OutOrStdout(), result, "encode verifier doctor"); encodeErr != nil {
			return encodeErr
		}
	} else {
		printVerifierDoctorResult(cmd.OutOrStdout(), result)
	}
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), result.Message)
		return exitCode(2)
	}
	return nil
}

func runVerifierPrompts(cmd *cobra.Command, opts verifierPromptOptions) error {
	ctx, cancel := backpressure.WithTimeout(context.Background())
	defer cancel()
	set, err := backpressure.BuildVerifierPrompts(ctx, backpressure.VerifierPromptOptions{
		Root:      opts.root,
		Feature:   opts.feature,
		Condition: opts.condition,
		Profile:   opts.profile,
	})
	if err != nil {
		return fail(2, "%v", err)
	}
	if opts.jsonOutput {
		return encodeJSON(cmd.OutOrStdout(), set, "encode verifier prompts")
	}
	printVerifierPrompts(cmd.OutOrStdout(), set)
	return nil
}

func printVerifierBeginResult(out io.Writer, result backpressure.BeginResponsesResult) {
	ui := cliui.New(shouldColorWriter(out))
	fmt.Fprintln(out, ui.Section("Burpvalve verifier begin"))
	fmt.Fprintf(out, "%s %s\n", ui.Label("Status:"), result.Status)
	if result.ResponsesPath != "" {
		fmt.Fprintf(out, "%s %s\n", ui.Label("Responses file:"), result.ResponsesPath)
	}
	if result.StagedPayloadHash != "" {
		fmt.Fprintf(out, "%s %s\n", ui.Label("Staged payload hash:"), result.StagedPayloadHash)
	}
	if result.ManifestHash != "" {
		fmt.Fprintf(out, "%s %s\n", ui.Label("Manifest hash:"), result.ManifestHash)
	}
	if result.Message != "" {
		fmt.Fprintln(out, result.Message)
	}
	for _, step := range result.NextSteps {
		fmt.Fprintf(out, "- %s\n", step)
	}
}

func printVerifierSubmitResult(out io.Writer, result backpressure.SubmitVerifierResult) {
	ui := cliui.New(shouldColorWriter(out))
	fmt.Fprintln(out, ui.Section("Burpvalve verifier submit"))
	fmt.Fprintf(out, "%s %s\n", ui.Label("Status:"), result.Status)
	if result.ResponsesPath != "" {
		fmt.Fprintf(out, "%s %s\n", ui.Label("Responses file:"), result.ResponsesPath)
	}
	if result.ConditionID != "" {
		fmt.Fprintf(out, "%s %s\n", ui.Label("Condition:"), result.ConditionID)
	}
	if result.TranscriptRef != "" {
		fmt.Fprintf(out, "%s %s\n", ui.Label("Transcript:"), result.TranscriptRef)
	}
	if result.Message != "" {
		fmt.Fprintln(out, result.Message)
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(out, "%s %s\n", ui.Warn("Warning:"), warning)
	}
	for _, step := range result.NextSteps {
		fmt.Fprintf(out, "- %s\n", step)
	}
}

func printVerifierDoctorResult(out io.Writer, result backpressure.VerifierDoctorResult) {
	ui := cliui.New(shouldColorWriter(out))
	fmt.Fprintln(out, ui.Section("Burpvalve verifier doctor"))
	fmt.Fprintf(out, "%s %s\n", ui.Label("Status:"), result.Status)
	fmt.Fprintf(out, "%s %t\n", ui.Label("Report only:"), result.ReportOnly)
	if result.Message != "" {
		fmt.Fprintln(out, result.Message)
	}
	for _, check := range result.Checks {
		fmt.Fprintln(out)
		fmt.Fprintf(out, "%s %s\n", ui.Section("Runtime:"), check.Runtime)
		fmt.Fprintf(out, "%s %t\n", ui.Label("Supported config found:"), check.Supported)
		for _, path := range check.Paths {
			fmt.Fprintf(out, "- %s exists=%t supported=%t", path.Path, path.Exists, path.Supported)
			if path.Message != "" {
				fmt.Fprintf(out, " (%s)", path.Message)
			}
			fmt.Fprintln(out)
		}
		for _, key := range []string{"subagent_limit", "depth_limit"} {
			value := check.Limits[key]
			if value.Status == "known" {
				fmt.Fprintf(out, "%s %v (%s)\n", ui.Label(key+":"), value.Value, value.Source)
			} else {
				fmt.Fprintf(out, "%s unknown\n", ui.Label(key+":"))
			}
		}
		for _, warning := range check.Warnings {
			fmt.Fprintf(out, "%s %s\n", ui.Warn("Warning:"), warning)
		}
	}
	for _, step := range result.NextSteps {
		fmt.Fprintf(out, "- %s\n", step)
	}
}

func printVerifierPrompts(out io.Writer, set backpressure.VerifierPromptSet) {
	ui := cliui.New(shouldColorWriter(out))
	fmt.Fprintln(out, ui.Section("Burpvalve verifier prompts"))
	fmt.Fprintf(out, "%s %s\n", ui.Label("Profile:"), set.Profile)
	fmt.Fprintf(out, "%s %s\n", ui.Label("Feature:"), set.Feature.ID)
	for _, note := range set.Notes {
		fmt.Fprintf(out, "- %s\n", note)
	}
	for _, packet := range set.Packets {
		fmt.Fprintln(out)
		fmt.Fprintf(out, "%s %s\n", ui.Section("Packet:"), packet.ID)
		fmt.Fprintf(out, "%s %s\n", ui.Label("Condition:"), packet.ConditionID+" ("+packet.ConditionFile+")")
		fmt.Fprintf(out, "%s %s\n", ui.Label("Policy:"), packet.VerifierPolicy)
		fmt.Fprintln(out, ui.Muted(packet.Authorization.Message))
		fmt.Fprintln(out, packet.Prompt)
	}
}

func printPromptsList(out io.Writer, items []backpressure.PromptListItem) {
	ui := cliui.New(shouldColorWriter(out))
	fmt.Fprintln(out, ui.Section("Burpvalve prompts"))
	fmt.Fprintln(out, "Canonical orchestrator templates embedded in this Burpvalve binary.")
	fmt.Fprintln(out, "Use burpvalve verifier prompts for staged-payload verifier packets.")
	for _, item := range items {
		fmt.Fprintf(out, "- %s %s\n", ui.Info(item.Name), ui.Muted("v"+item.Version))
		fmt.Fprintf(out, "  %s\n", item.Description)
		if len(item.Variables) > 0 {
			names := make([]string, 0, len(item.Variables))
			for _, variable := range item.Variables {
				label := variable.Name
				if variable.Required {
					label += " required"
				}
				names = append(names, label)
			}
			fmt.Fprintf(out, "  variables: %s\n", strings.Join(names, ", "))
		}
	}
}

func printPrompt(out io.Writer, rendered backpressure.PromptShowOutput) {
	ui := cliui.New(shouldColorWriter(out))
	fmt.Fprintln(out, ui.Section("Burpvalve prompt"))
	fmt.Fprintf(out, "%s %s\n", ui.Label("Name:"), rendered.Name)
	fmt.Fprintf(out, "%s %s\n", ui.Label("Version:"), rendered.Version)
	if len(rendered.Variables) > 0 {
		fmt.Fprintln(out, ui.Label("Variables:"))
		for _, variable := range rendered.Variables {
			required := "optional"
			if variable.Required {
				required = "required"
			}
			fmt.Fprintf(out, "- %s (%s): %s\n", variable.Name, required, variable.Description)
		}
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, rendered.Body)
}

func printPromptExport(out io.Writer, result backpressure.PromptExportResult) {
	ui := cliui.New(shouldColorWriter(out))
	fmt.Fprintln(out, ui.Section("Burpvalve prompt export"))
	fmt.Fprintf(out, "%s %s\n", ui.Label("Prompt:"), result.PromptName)
	fmt.Fprintf(out, "%s %s\n", ui.Label("Path:"), result.Path)
	fmt.Fprintf(out, "%s %s\n", ui.Label("Content hash:"), result.ContentHash)
	status := "unchanged"
	if result.Written {
		status = "written"
	}
	fmt.Fprintf(out, "%s %s\n", ui.Label("Status:"), status)
	fmt.Fprintln(out, "Exported prompt files are local copies; embedded prompts remain authoritative.")
}

type attestationQueryOptions struct {
	root       string
	status     string
	limit      int
	feature    string
	bead       string
	jsonOutput bool
}

func newAttestationsCommand() *cobra.Command {
	opts := attestationQueryOptions{root: ".", status: "all"}
	cmd := &cobra.Command{
		Use:   "attestations",
		Short: "List and inspect Burpvalve attestation evidence.",
		Long: `Read seals, also called attestations (the evidence artifacts), and
blocked reports without changing files.
Use JSON output for agents and scripts. In an interactive terminal, running this
command without a subcommand opens a read-only browser.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if isInteractiveTerminal(os.Stdin, os.Stdout) {
				return runAttestationsBrowse(cmd, attestationBrowseOptions{
					root:    opts.root,
					status:  opts.status,
					limit:   opts.limit,
					feature: opts.feature,
					bead:    opts.bead,
				})
			}
			return cmd.Help()
		},
	}
	bindAttestationFilterFlags(cmd, &opts)
	cmd.AddCommand(
		newAttestationsBrowseCommand(&opts),
		newAttestationsListCommand(&opts),
		newAttestationsShowCommand(&opts),
		newAttestationsLatestCommand(&opts),
	)
	return cmd
}

func bindAttestationFilterFlags(cmd *cobra.Command, opts *attestationQueryOptions) {
	cmd.Flags().StringVar(&opts.root, "root", ".", "repository root")
	cmd.Flags().StringVar(&opts.status, "status", "all", "filter status: all, pass, blocked, or malformed")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "maximum records to return; 0 means no limit")
	cmd.Flags().StringVar(&opts.feature, "feature", "", "filter by feature id")
	cmd.Flags().StringVar(&opts.bead, "bead", "", "filter by bead id when present")
}

func bindAttestationQueryFlags(cmd *cobra.Command, opts *attestationQueryOptions) {
	bindAttestationFilterFlags(cmd, opts)
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "print machine-readable JSON")
}

func newAttestationsListCommand(parent *attestationQueryOptions) *cobra.Command {
	opts := *parent
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List passing attestations and blocked reports.",
		Long:  "List Burpvalve evidence artifacts from backpressure/attestations and log/backpressure/failed.",
		Example: `  burpvalve attestations list
  burpvalve attestations list --status blocked
  burpvalve attestations list --json --feature br-123`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAttestationsList(cmd, opts)
		},
	}
	bindAttestationQueryFlags(cmd, &opts)
	return cmd
}

func newAttestationsShowCommand(parent *attestationQueryOptions) *cobra.Command {
	opts := *parent
	cmd := &cobra.Command{
		Use:   "show <hash-or-path>",
		Short: "Show one attestation or blocked report.",
		Long:  "Show one Burpvalve evidence artifact by full path, filename, payload hash, or unambiguous prefix.",
		Args:  cobra.ExactArgs(1),
		Example: `  burpvalve attestations show backpressure/attestations/<hash>.json
  burpvalve attestations show <hash-prefix> --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAttestationsShow(cmd, opts, args[0])
		},
	}
	cmd.Flags().StringVar(&opts.root, "root", ".", "repository root")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "print machine-readable JSON")
	return cmd
}

func newAttestationsLatestCommand(parent *attestationQueryOptions) *cobra.Command {
	opts := *parent
	cmd := &cobra.Command{
		Use:   "latest",
		Short: "Show the newest attestation or blocked report.",
		Long:  "Show the newest Burpvalve evidence artifact after optional status, feature, or bead filtering.",
		Example: `  burpvalve attestations latest
  burpvalve attestations latest --status pass --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAttestationsLatest(cmd, opts)
		},
	}
	bindAttestationQueryFlags(cmd, &opts)
	return cmd
}

func runAttestationsList(cmd *cobra.Command, opts attestationQueryOptions) error {
	if err := validateAttestationStatus(opts.status); err != nil {
		return err
	}
	records, err := attestations.List(opts.root, attestations.QueryOptions{
		Status:  opts.status,
		Limit:   opts.limit,
		Feature: opts.feature,
		Bead:    opts.bead,
	})
	if err != nil {
		return fail(2, "list attestations: %v", err)
	}
	if opts.jsonOutput {
		return encodeJSON(cmd.OutOrStdout(), map[string]any{
			"schema_version": 1,
			"records":        records,
		}, "encode attestations list")
	}
	printAttestationList(cmd.OutOrStdout(), records)
	return nil
}

func runAttestationsShow(cmd *cobra.Command, opts attestationQueryOptions, ref string) error {
	record, err := attestations.Show(opts.root, ref)
	if err != nil {
		return attestationQueryError(cmd, opts.jsonOutput, err)
	}
	if opts.jsonOutput {
		return encodeJSON(cmd.OutOrStdout(), record, "encode attestation")
	}
	printAttestationDetail(cmd.OutOrStdout(), record)
	return nil
}

func runAttestationsLatest(cmd *cobra.Command, opts attestationQueryOptions) error {
	if err := validateAttestationStatus(opts.status); err != nil {
		return err
	}
	record, err := attestations.Latest(opts.root, attestations.QueryOptions{
		Status:  opts.status,
		Feature: opts.feature,
		Bead:    opts.bead,
	})
	if err != nil {
		return attestationQueryError(cmd, opts.jsonOutput, err)
	}
	if opts.jsonOutput {
		return encodeJSON(cmd.OutOrStdout(), record, "encode latest attestation")
	}
	printAttestationDetail(cmd.OutOrStdout(), record)
	return nil
}

func validateAttestationStatus(status string) error {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "all", "pass", "blocked", "malformed":
		return nil
	default:
		return fail(2, "unknown attestation status %q; expected all, pass, blocked, or malformed", status)
	}
}

func attestationQueryError(cmd *cobra.Command, jsonOutput bool, err error) error {
	var showErr *attestations.ShowError
	if errors.As(err, &showErr) && jsonOutput {
		_ = encodeJSON(cmd.OutOrStdout(), showErr, "encode attestation error")
		return exitCode(2)
	}
	if errors.As(err, &showErr) {
		return fail(2, "%s", showErr.Message)
	}
	return fail(2, "%v", err)
}

func printAttestationList(out io.Writer, records []attestations.Record) {
	ui := cliui.New(shouldColorWriter(out))
	fmt.Fprintln(out, ui.Section("Burpvalve attestations"))
	if len(records) == 0 {
		fmt.Fprintln(out, ui.Muted("No attestation artifacts found."))
		return
	}
	fmt.Fprintf(out, "%-10s %-12s %-18s %-18s %-20s %s\n", "STATUS", "HASH", "FEATURE", "BEADS", "CREATED", "PATH")
	for _, record := range records {
		fmt.Fprintf(out, "%-10s %-12s %-18s %-18s %-20s %s\n",
			ui.Status(record.Status),
			attestationShortValue(firstNonEmptyString(record.PayloadHash, record.ID), 12),
			attestationShortValue(strings.Join(record.FeatureIDs, ","), 18),
			attestationShortValue(strings.Join(record.BeadIDs, ","), 18),
			attestationShortTime(record.CreatedAt),
			ui.Path(record.Path),
		)
	}
}

func printAttestationDetail(out io.Writer, record attestations.Record) {
	ui := cliui.New(shouldColorWriter(out))
	fmt.Fprintln(out, ui.Section("Burpvalve attestation"))
	fmt.Fprintf(out, "%s %s\n", ui.Label("Status:"), ui.Status(record.Status))
	fmt.Fprintf(out, "%s %s\n", ui.Label("Type:"), record.ArtifactType)
	fmt.Fprintf(out, "%s %s\n", ui.Label("Path:"), ui.Path(record.Path))
	if record.PayloadHash != "" {
		fmt.Fprintf(out, "%s %s\n", ui.Label("Payload:"), attestationShortValue(record.PayloadHash, 24))
	}
	if len(record.FeatureIDs) > 0 {
		fmt.Fprintf(out, "%s %s\n", ui.Label("Feature:"), strings.Join(record.FeatureIDs, ", "))
	}
	if len(record.BeadIDs) > 0 {
		fmt.Fprintf(out, "%s %s\n", ui.Label("Beads:"), strings.Join(record.BeadIDs, ", "))
	}
	if record.CreatedAt != nil {
		fmt.Fprintf(out, "%s %s\n", ui.Label("Created:"), attestationShortTime(record.CreatedAt))
	}
	if len(record.ConditionVerdicts) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, ui.Section("Conditions"))
		for _, condition := range record.ConditionVerdicts {
			fmt.Fprintf(out, "%s %s %s\n", ui.Label(condition.ConditionID+":"), ui.Status(string(condition.Verdict)), ui.Muted(string(condition.VerifierKind)))
		}
	}
	if len(record.ParseWarnings) > 0 || len(record.Warnings) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, ui.Section("Warnings"))
		for _, warning := range append(record.ParseWarnings, record.Warnings...) {
			fmt.Fprintf(out, "- %s\n", warning)
		}
	}
}

type beadsPreflightOptions struct {
	root                   string
	jsonOutput             bool
	adminOnly              bool
	beadRationale          string
	requireDeliveryPayload bool
}

type beadsPreflightReport struct {
	SchemaVersion        int                  `json:"schema_version"`
	Command              string               `json:"command"`
	Status               string               `json:"status"`
	Mutating             bool                 `json:"mutating"`
	Root                 string               `json:"root"`
	BeadIDs              []string             `json:"bead_ids"`
	AdminOnly            bool                 `json:"admin_only"`
	Classification       string               `json:"classification"`
	CoupledWorkRationale string               `json:"coupled_work_rationale,omitempty"`
	BRAvailable          bool                 `json:"br_available"`
	StagedPayloadPaths   []string             `json:"staged_payload_paths,omitempty"`
	NonBeadsPayloadPaths []string             `json:"non_beads_payload_paths,omitempty"`
	Beads                []beadsPreflightBead `json:"beads,omitempty"`
	Warnings             []string             `json:"warnings,omitempty"`
	NextSteps            []string             `json:"next_steps"`
}

type beadsPreflightBead struct {
	ID          string   `json:"id"`
	Title       string   `json:"title,omitempty"`
	Status      string   `json:"status,omitempty"`
	IssueType   string   `json:"issue_type,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	ClosedAt    string   `json:"closed_at,omitempty"`
	CloseReason string   `json:"close_reason,omitempty"`
}

type beadsDriftOptions struct {
	root       string
	jsonOutput bool
	window     time.Duration
}

type beadsDriftReport struct {
	SchemaVersion int                     `json:"schema_version"`
	Command       string                  `json:"command"`
	Status        string                  `json:"status"`
	Mutating      bool                    `json:"mutating"`
	Fatal         bool                    `json:"fatal"`
	Root          string                  `json:"root"`
	Window        string                  `json:"window"`
	Since         string                  `json:"since"`
	DirtyTree     bool                    `json:"dirty_tree"`
	DirtyPaths    []string                `json:"dirty_paths,omitempty"`
	CheckedBeads  []beadsDriftFinding     `json:"checked_beads,omitempty"`
	Findings      []beadsDriftFinding     `json:"findings,omitempty"`
	Warnings      []string                `json:"warnings,omitempty"`
	NextSteps     []scaffold.RecoveryStep `json:"next_steps,omitempty"`
}

type beadsDriftFinding struct {
	BeadID           string   `json:"bead_id"`
	Title            string   `json:"title,omitempty"`
	ClosedAt         string   `json:"closed_at,omitempty"`
	Status           string   `json:"status"`
	Severity         string   `json:"severity"`
	Message          string   `json:"message"`
	AttestationPaths []string `json:"attestation_paths,omitempty"`
	CommitMatches    []string `json:"commit_matches,omitempty"`
}

type beadsCloseOptions struct {
	root          string
	beadIDs       []string
	jsonOutput    bool
	adminOnly     bool
	reason        string
	beadRationale string
	responses     string
	feature       string
	resume        bool
	yes           bool
	commitMessage string
	agent         string
	model         string
}

type beadsCloseResult struct {
	SchemaVersion int                     `json:"schema_version"`
	Command       string                  `json:"command"`
	Status        string                  `json:"status"`
	Fatal         bool                    `json:"fatal"`
	Partial       bool                    `json:"partial_success"`
	Root          string                  `json:"root"`
	BeadIDs       []string                `json:"bead_ids"`
	JournalPath   string                  `json:"journal_path"`
	Preflight     beadsPreflightReport    `json:"preflight,omitempty"`
	Steps         []beadsCloseJournalStep `json:"steps"`
	NextSteps     []scaffold.RecoveryStep `json:"next_steps,omitempty"`
	Warnings      []string                `json:"warnings,omitempty"`
}

type beadsCloseJournal struct {
	SchemaVersion int                     `json:"schema_version"`
	Command       string                  `json:"command"`
	BeadIDs       []string                `json:"bead_ids"`
	UpdatedAt     string                  `json:"updated_at"`
	Steps         []beadsCloseJournalStep `json:"steps"`
}

type beadsCloseJournalStep struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	Command      string `json:"command,omitempty"`
	StdoutRef    string `json:"stdout_ref,omitempty"`
	StderrRef    string `json:"stderr_ref,omitempty"`
	ExitStatus   int    `json:"exit_status,omitempty"`
	StartedAt    string `json:"started_at,omitempty"`
	CompletedAt  string `json:"completed_at,omitempty"`
	Message      string `json:"message,omitempty"`
	ArtifactPath string `json:"artifact_path,omitempty"`
}

type robotBeadsCloseInput struct {
	Root          string   `json:"root"`
	BeadIDs       []string `json:"bead_ids"`
	Reason        string   `json:"reason"`
	Feature       string   `json:"feature"`
	BeadRationale string   `json:"bead_rationale"`
	ResponsesPath string   `json:"responses_path"`
	Responses     string   `json:"responses"`
	AdminOnly     bool     `json:"admin_only"`
	Resume        bool     `json:"resume"`
	Confirm       bool     `json:"confirm"`
	CommitMessage string   `json:"commit_message"`
	Agent         string   `json:"agent"`
	Model         string   `json:"model"`
}

func newBeadsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "beads",
		Short: "Plan or close Beads delivery evidence safely.",
		Long: `Read Beads state and staged files to show or execute the safe delivery
sequence. The preflight subcommand is read-only. The close subcommand is a
journaled state machine that closes delivery beads only through the numbered
Burpvalve gate order.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newBeadsPreflightCommand())
	cmd.AddCommand(newBeadsDriftCommand())
	cmd.AddCommand(newBeadsCloseCommand())
	return cmd
}

func newBeadsPreflightCommand() *cobra.Command {
	var opts beadsPreflightOptions
	cmd := &cobra.Command{
		Use:   "preflight <bead-id...>",
		Short: "Show the safe order for closing delivery beads.",
		Long: `Show the delivery sequence for Beads-backed work without changing files.
Use this before closing implementation, docs, config, or test beads so the final
Burpvalve commit gate can bind evidence to the exact staged payload.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if robotsMode {
				return nil
			}
			return cobra.MinimumNArgs(1)(cmd, args)
		},
		Example: `  burpvalve beads preflight br-123
  burpvalve beads preflight br-123 br-456 --bead-rationale "same staged payload"
  burpvalve beads preflight admin-123 --admin-only --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := buildBeadsPreflightReport(opts, args)
			if opts.jsonOutput {
				if encodeErr := encodeJSON(cmd.OutOrStdout(), report, "encode beads preflight"); encodeErr != nil {
					return encodeErr
				}
				if err != nil {
					return exitCode(2)
				}
				return nil
			}
			printBeadsPreflight(cmd.OutOrStdout(), report)
			if err != nil {
				return exitCode(2)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.root, "root", ".", "repository root")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "print machine-readable JSON")
	cmd.Flags().BoolVar(&opts.adminOnly, "admin-only", false, "classify this as issue-only/admin work that does not need commit evidence")
	cmd.Flags().StringVar(&opts.beadRationale, "bead-rationale", "", "why multiple bead ids belong to one staged payload")
	return cmd
}

func newBeadsDriftCommand() *cobra.Command {
	var opts beadsDriftOptions
	opts.window = 7 * 24 * time.Hour
	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Advisory check for recently closed beads without attestation evidence.",
		Long: `Read Beads closures, Burpvalve attestations, git history, and current
tree dirtiness to flag possible closure drift. This command is advisory and
read-only: it does not close beads, stage files, write files, or block br.`,
		Example: `  burpvalve beads drift --json
  burpvalve beads drift --window 72h`,
		RunE: func(cmd *cobra.Command, args []string) error {
			report := buildBeadsDriftReport(opts, time.Now().UTC())
			if opts.jsonOutput || robotsMode {
				return encodeJSON(cmd.OutOrStdout(), report, "encode beads drift")
			}
			printBeadsDrift(cmd.OutOrStdout(), report)
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.root, "root", ".", "repository root")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "print machine-readable JSON")
	cmd.Flags().DurationVar(&opts.window, "window", opts.window, "lookback window for recently closed beads")
	return cmd
}

func newBeadsCloseCommand() *cobra.Command {
	var opts beadsCloseOptions
	cmd := &cobra.Command{
		Use:   "close <bead-id...>",
		Short: "Close delivery beads through the Burpvalve gate.",
		Long: `Close Beads work with the numbered safe order.
Delivery closures expect the payload to already be staged. They close every bead
before one commit gate so .beads/issues.jsonl is included in the attested
payload. Multiple delivery bead ids require --bead-rationale.

Admin-only closures require tracker-only staged paths, skip verifier evidence
and code attestations, and may batch multiple admin bead ids.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if robotsMode {
				return nil
			}
			return cobra.MinimumNArgs(1)(cmd, args)
		},
		Example: `  burpvalve beads close br-123 --reason "Complete br-123 delivery" --responses responses.json --json
  burpvalve beads close br-123 br-456 --bead-rationale "same staged payload" --reason "Complete coupled delivery" --responses responses.json --json
  burpvalve beads close admin-1 admin-2 --admin-only --reason "Tracker cleanup" --yes --message "Close admin beads" --json
  burpvalve beads close br-123 --reason "Complete br-123 delivery" --responses responses.json --resume --json
  printf '{"bead_ids":["br-123"],"reason":"Complete br-123 delivery","responses_path":"responses.json","confirm":true,"commit_message":"Close br-123"}' | burpvalve beads close --robots`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.beadIDs = args
			if robotsMode {
				return runBeadsCloseRobots(cmd, opts)
			}
			return runBeadsClose(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.root, "root", ".", "repository root")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "print machine-readable JSON")
	cmd.Flags().BoolVar(&opts.adminOnly, "admin-only", false, "classify this as issue-only/admin work that does not need code attestation")
	cmd.Flags().StringVar(&opts.reason, "reason", "", "required br close reason; reference the bead and feature, not a commit SHA")
	cmd.Flags().StringVar(&opts.beadRationale, "bead-rationale", "", "why multiple bead ids belong to one staged payload")
	cmd.Flags().StringVar(&opts.responses, "responses", "", "JSON matrix responses for the final staged payload")
	cmd.Flags().StringVar(&opts.feature, "feature", "", "explicit atomic feature id for the commit gate")
	cmd.Flags().BoolVar(&opts.resume, "resume", false, "resume from the journal after recomputing reality")
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "confirm the final git commit")
	cmd.Flags().StringVar(&opts.commitMessage, "message", "", "git commit message used only with --yes or robots confirm:true")
	cmd.Flags().StringVar(&opts.agent, "agent", "codex", "agent name recorded in generated artifacts")
	cmd.Flags().StringVar(&opts.model, "model", "unspecified", "model name recorded in generated artifacts")
	return cmd
}

func buildBeadsPreflightReport(opts beadsPreflightOptions, ids []string) (beadsPreflightReport, error) {
	beadIDs, rationale, normErr := normalizeCloseBeads(ids, opts.beadRationale, opts.adminOnly)
	root, err := filepath.Abs(defaultCLIRoot(opts.root))
	if err != nil {
		root = defaultCLIRoot(opts.root)
	}
	report := beadsPreflightReport{
		SchemaVersion:        1,
		Command:              "beads preflight",
		Status:               "ready",
		Mutating:             false,
		Root:                 root,
		BeadIDs:              beadIDs,
		AdminOnly:            opts.adminOnly,
		CoupledWorkRationale: rationale,
	}
	if normErr != nil {
		report.Status = "blocked"
		report.Warnings = append(report.Warnings, normErr.Error())
		if opts.adminOnly {
			report.NextSteps = []string{"Pass one or more admin bead ids with --admin-only, or remove duplicate/invalid bead ids."}
		} else {
			report.NextSteps = []string{"Pass one --bead id, or add --bead-rationale when multiple bead ids share one staged payload."}
		}
		return report, normErr
	}
	brPath, brErr := exec.LookPath("br")
	if brErr != nil {
		report.Status = "blocked"
		report.BRAvailable = false
		report.Warnings = append(report.Warnings, "br executable not found on PATH")
		report.NextSteps = []string{"Install br or run this command in a shell where br is on PATH, then rerun burpvalve beads preflight."}
		return report, brErr
	}
	report.BRAvailable = true
	report.StagedPayloadPaths = stagedPathNames(root)
	report.NonBeadsPayloadPaths = nonBeadsPayloadPaths(report.StagedPayloadPaths)
	report.Classification = classifyBeadsPayload(report.StagedPayloadPaths, opts.adminOnly)
	if dirty := unrelatedDirtyPathNames(root, report.StagedPayloadPaths); len(dirty) > 0 {
		report.Warnings = append(report.Warnings, fmt.Sprintf("unrelated dirty files present and will not be staged by beads close: %s", strings.Join(dirty, ", ")))
	}
	if opts.adminOnly && len(report.NonBeadsPayloadPaths) > 0 {
		report.Status = "blocked"
		report.Warnings = append(report.Warnings, fmt.Sprintf("--admin-only cannot be used with non-.beads staged paths: %s", strings.Join(report.NonBeadsPayloadPaths, ", ")))
	}
	for _, id := range beadIDs {
		bead, err := inspectBead(root, brPath, id)
		if err != nil {
			report.Status = "blocked"
			report.Warnings = append(report.Warnings, err.Error())
			continue
		}
		report.Beads = append(report.Beads, bead)
		if !opts.adminOnly && bead.Status != "" && bead.Status != "closed" {
			report.Warnings = append(report.Warnings, fmt.Sprintf("bead %s is %s; close it only after the staged work and verifier evidence are ready", bead.ID, bead.Status))
		}
		if warning := beadMetadataClassificationWarning(bead, report.Classification); warning != "" {
			report.Warnings = append(report.Warnings, warning)
		}
	}
	if !opts.adminOnly && len(report.StagedPayloadPaths) == 0 {
		report.Warnings = append(report.Warnings, "no staged implementation/docs/config/test payload detected")
	}
	if opts.requireDeliveryPayload && report.Classification != "admin" && len(report.StagedPayloadPaths) == 0 {
		report.Status = "blocked"
		report.Classification = "delivery"
		report.Warnings = append(report.Warnings, "delivery close requires a staged payload outside .beads before closing the bead")
	}
	report.NextSteps = beadsPreflightNextSteps(report)
	if report.Status == "ready" && len(report.Warnings) > 0 {
		report.Status = "action_needed"
	}
	if hasFatalPreflightWarning(report) {
		report.Status = "blocked"
		return report, errors.New(strings.Join(report.Warnings, "; "))
	}
	return report, nil
}

func inspectBead(root, brPath, id string) (beadsPreflightBead, error) {
	cmd := exec.Command(brPath, "show", id, "--json")
	cmd.Dir = root
	body, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return beadsPreflightBead{ID: id}, fmt.Errorf("br show %s failed: %s", id, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return beadsPreflightBead{ID: id}, fmt.Errorf("br show %s failed: %v", id, err)
	}
	var rows []struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Status      string   `json:"status"`
		IssueType   string   `json:"issue_type"`
		Labels      []string `json:"labels"`
		ClosedAt    string   `json:"closed_at"`
		CloseReason string   `json:"close_reason"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return beadsPreflightBead{ID: id}, fmt.Errorf("parse br show %s --json: %v", id, err)
	}
	if len(rows) == 0 {
		return beadsPreflightBead{ID: id}, fmt.Errorf("bead %s was not found", id)
	}
	return beadsPreflightBead{
		ID:          rows[0].ID,
		Title:       rows[0].Title,
		Status:      rows[0].Status,
		IssueType:   rows[0].IssueType,
		Labels:      rows[0].Labels,
		ClosedAt:    rows[0].ClosedAt,
		CloseReason: rows[0].CloseReason,
	}, nil
}

func stagedPathNames(root string) []string {
	cmd := exec.Command("git", "diff", "--cached", "--name-only")
	cmd.Dir = root
	body, err := cmd.Output()
	if err != nil {
		return nil
	}
	var paths []string
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			paths = append(paths, filepath.ToSlash(line))
		}
	}
	return paths
}

func stagedAttestationPathNames(root string) []string {
	cmd := exec.Command("git", "diff", "--cached", "--name-only", "--", "backpressure/attestations")
	cmd.Dir = root
	body, err := cmd.Output()
	if err != nil {
		return nil
	}
	var paths []string
	for _, line := range strings.Split(string(body), "\n") {
		line = filepath.ToSlash(strings.TrimSpace(line))
		if line != "" && strings.HasPrefix(line, "backpressure/attestations/") && strings.HasSuffix(line, ".json") {
			paths = append(paths, line)
		}
	}
	sort.Strings(paths)
	return paths
}

func unrelatedDirtyPathNames(root string, staged []string) []string {
	cmd := exec.Command("git", "status", "--porcelain=v1", "--untracked-files=all")
	cmd.Dir = root
	body, err := cmd.Output()
	if err != nil {
		return nil
	}
	stagedSet := map[string]bool{}
	for _, path := range staged {
		stagedSet[filepath.ToSlash(strings.TrimSpace(path))] = true
	}
	dirtySet := map[string]bool{}
	for _, line := range strings.Split(string(body), "\n") {
		if len(line) < 4 {
			continue
		}
		indexStatus := line[0]
		worktreeStatus := line[1]
		path := filepath.ToSlash(strings.TrimSpace(line[3:]))
		if strings.Contains(path, " -> ") {
			parts := strings.Split(path, " -> ")
			path = strings.TrimSpace(parts[len(parts)-1])
		}
		if path == "" || stagedSet[path] {
			continue
		}
		if line[:2] == "??" || worktreeStatus != ' ' {
			dirtySet[path] = true
			continue
		}
		if indexStatus == '?' {
			dirtySet[path] = true
		}
	}
	var paths []string
	for path := range dirtySet {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func classifyBeadsPayload(paths []string, adminOnly bool) string {
	if adminOnly && len(nonBeadsPayloadPaths(paths)) == 0 {
		return "admin"
	}
	if len(paths) == 0 {
		return "preview"
	}
	if len(nonBeadsPayloadPaths(paths)) == 0 {
		return "admin"
	}
	return "delivery"
}

func nonBeadsPayloadPaths(paths []string) []string {
	var nonBeads []string
	for _, path := range paths {
		if !isBeadsPath(path) {
			nonBeads = append(nonBeads, path)
		}
	}
	return nonBeads
}

func isBeadsPath(path string) bool {
	path = filepath.ToSlash(strings.TrimSpace(path))
	return path == ".beads" || strings.HasPrefix(path, ".beads/")
}

func beadMetadataClassificationWarning(bead beadsPreflightBead, classification string) string {
	if classification == "" || classification == "preview" {
		return ""
	}
	metadata := beadMetadataClassification(bead)
	if metadata == "" || metadata == classification {
		return ""
	}
	return fmt.Sprintf("bead %s metadata suggests %s work, but staged payload classifies this as %s; staged paths decide", bead.ID, metadata, classification)
}

func beadMetadataClassification(bead beadsPreflightBead) string {
	issueType := strings.ToLower(strings.TrimSpace(bead.IssueType))
	switch issueType {
	case "bug", "feature", "task", "docs":
		return "delivery"
	case "chore", "question":
		return "admin"
	}
	for _, label := range bead.Labels {
		switch strings.ToLower(strings.TrimSpace(label)) {
		case "admin", "triage", "planning", "tracker", "issue-maintenance":
			return "admin"
		case "delivery", "implementation", "docs", "config", "tests":
			return "delivery"
		}
	}
	return ""
}

func hasFatalPreflightWarning(report beadsPreflightReport) bool {
	if report.Status == "blocked" {
		return true
	}
	for _, warning := range report.Warnings {
		if strings.HasPrefix(warning, "br show ") || strings.HasPrefix(warning, "parse br show ") || strings.Contains(warning, "was not found") {
			return true
		}
	}
	return false
}

func beadsPreflightNextSteps(report beadsPreflightReport) []string {
	if report.AdminOnly || report.Classification == "admin" {
		return []string{
			"Record the admin-only rationale in the bead or durable docs.",
			"Run br sync --flush-only before committing Beads metadata if it changed.",
			"Do not fabricate implementation commit evidence for issue-only work.",
		}
	}
	return []string{
		"Stage the intended implementation/docs/config/test payload.",
		"Run burpvalve verifier prompts for the staged feature/condition cells and gather real verifier evidence.",
		"Close the delivery bead only after the work is ready and blockers are explicit.",
		"Run br sync --flush-only.",
		"Stage the payload plus .beads/issues.jsonl and intentional Beads metadata files.",
		"Run burpvalve commit with --bead metadata against that exact staged payload.",
		"If Burpvalve writes an attestation and you stage it, rerun the final gate so the attested payload and committed payload match.",
	}
}

func printBeadsPreflight(out io.Writer, report beadsPreflightReport) {
	ui := cliui.New(shouldColorWriter(out))
	fmt.Fprintln(out, ui.Section("Burpvalve beads preflight"))
	fmt.Fprintf(out, "%s %s\n", ui.Label("Status:"), ui.Status(report.Status))
	fmt.Fprintf(out, "%s %v\n", ui.Label("Mutating:"), report.Mutating)
	fmt.Fprintf(out, "%s %s\n", ui.Label("Root:"), ui.Path(report.Root))
	fmt.Fprintf(out, "%s %s\n", ui.Label("Beads:"), strings.Join(report.BeadIDs, ", "))
	if report.CoupledWorkRationale != "" {
		fmt.Fprintf(out, "%s %s\n", ui.Label("Rationale:"), report.CoupledWorkRationale)
	}
	if len(report.StagedPayloadPaths) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, ui.Section("Staged payload"))
		for _, path := range report.StagedPayloadPaths {
			fmt.Fprintf(out, "- %s\n", ui.Path(path))
		}
	}
	if len(report.Warnings) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, ui.Section("Warnings"))
		for _, warning := range report.Warnings {
			fmt.Fprintf(out, "- %s\n", warning)
		}
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, ui.Section("Next steps"))
	for _, step := range report.NextSteps {
		fmt.Fprintf(out, "- %s\n", step)
	}
}

type closedBeadRow struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	ClosedAt    string `json:"closed_at"`
	CloseReason string `json:"close_reason"`
}

func buildBeadsDriftReport(opts beadsDriftOptions, now time.Time) beadsDriftReport {
	root, err := filepath.Abs(defaultCLIRoot(opts.root))
	if err != nil {
		root = defaultCLIRoot(opts.root)
	}
	if opts.window <= 0 {
		opts.window = 7 * 24 * time.Hour
	}
	since := now.Add(-opts.window)
	report := beadsDriftReport{
		SchemaVersion: 1,
		Command:       "beads drift",
		Status:        "clean",
		Mutating:      false,
		Fatal:         false,
		Root:          root,
		Window:        opts.window.String(),
		Since:         since.Format(time.RFC3339),
	}
	report.DirtyPaths = dirtyPathNames(root)
	report.DirtyTree = len(report.DirtyPaths) > 0

	brPath, err := exec.LookPath("br")
	if err != nil {
		report.Status = "warning"
		report.Warnings = append(report.Warnings, "br executable not found on PATH")
		report.NextSteps = beadsDriftNextSteps(report)
		return report
	}
	closed, warnings := recentlyClosedBeads(root, brPath, since)
	report.Warnings = append(report.Warnings, warnings...)
	attestationMatches, attestationWarnings := attestationBeadMatches(root)
	report.Warnings = append(report.Warnings, attestationWarnings...)
	commitMatches, commitWarnings := gitBeadCommitMatches(root, since, closed)
	report.Warnings = append(report.Warnings, commitWarnings...)

	for _, bead := range closed {
		attestationPaths := append([]string(nil), attestationMatches[bead.ID]...)
		commits := append([]string(nil), commitMatches[bead.ID]...)
		sort.Strings(attestationPaths)
		sort.Strings(commits)
		finding := beadsDriftFinding{
			BeadID:           bead.ID,
			Title:            bead.Title,
			ClosedAt:         bead.ClosedAt,
			AttestationPaths: attestationPaths,
			CommitMatches:    commits,
		}
		switch {
		case len(attestationPaths) > 0:
			finding.Status = "attested"
			finding.Severity = "ok"
			finding.Message = "recent closure has matching Burpvalve attestation bead metadata"
		case len(commits) > 0:
			finding.Status = "commit_message"
			finding.Severity = "ok"
			finding.Message = "recent closure is named in git commit history"
		case report.DirtyTree:
			finding.Status = "possible"
			finding.Severity = "warning"
			finding.Message = "recent closure has no matching attestation bead id or commit message while the tree is dirty"
			report.Findings = append(report.Findings, finding)
		default:
			finding.Status = "unmatched_clean_tree"
			finding.Severity = "info"
			finding.Message = "recent closure is unmatched, but the tree is clean; this is advisory and does not claim current changes belong to the bead"
			report.Findings = append(report.Findings, finding)
		}
		report.CheckedBeads = append(report.CheckedBeads, finding)
	}
	if hasDriftStatus(report.Findings, "possible") {
		report.Status = "possible"
	} else if len(report.Warnings) > 0 {
		report.Status = "warning"
	}
	report.NextSteps = beadsDriftNextSteps(report)
	return report
}

func recentlyClosedBeads(root, brPath string, since time.Time) ([]closedBeadRow, []string) {
	cmd := exec.Command(brPath, "list", "--status", "closed", "--json")
	cmd.Dir = root
	body, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, []string{"br list --status closed --json failed: " + strings.TrimSpace(string(exitErr.Stderr))}
		}
		return nil, []string{"br list --status closed --json failed: " + err.Error()}
	}
	var rows []closedBeadRow
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, []string{"parse br list --status closed --json: " + err.Error()}
	}
	recent := make([]closedBeadRow, 0, len(rows))
	warnings := []string{}
	for _, row := range rows {
		row.ID = strings.TrimSpace(row.ID)
		if row.ID == "" {
			warnings = append(warnings, "closed bead row missing id")
			continue
		}
		closedAt := strings.TrimSpace(row.ClosedAt)
		if closedAt == "" {
			warnings = append(warnings, fmt.Sprintf("closed bead %s missing closed_at; skipped from drift window", row.ID))
			continue
		}
		parsed, err := time.Parse(time.RFC3339Nano, closedAt)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("closed bead %s has malformed closed_at %q; skipped from drift window", row.ID, closedAt))
			continue
		}
		if !parsed.Before(since) {
			recent = append(recent, row)
		}
	}
	sort.Slice(recent, func(i, j int) bool {
		return recent[i].ID < recent[j].ID
	})
	return recent, warnings
}

func attestationBeadMatches(root string) (map[string][]string, []string) {
	records, err := attestations.List(root, attestations.QueryOptions{Status: "pass"})
	if err != nil {
		return map[string][]string{}, []string{"list Burpvalve attestations failed: " + err.Error()}
	}
	matches := map[string][]string{}
	warnings := []string{}
	for _, record := range records {
		if len(record.ParseWarnings) > 0 {
			warnings = append(warnings, fmt.Sprintf("attestation %s malformed: %s", record.Path, strings.Join(record.ParseWarnings, "; ")))
			continue
		}
		for _, beadID := range record.BeadIDs {
			beadID = strings.TrimSpace(beadID)
			if beadID != "" {
				matches[beadID] = append(matches[beadID], record.Path)
			}
		}
	}
	return matches, warnings
}

func gitBeadCommitMatches(root string, since time.Time, beads []closedBeadRow) (map[string][]string, []string) {
	matches := map[string][]string{}
	if len(beads) == 0 {
		return matches, nil
	}
	cmd := exec.Command("git", "log", "--since", since.Format(time.RFC3339), "--format=%H%x00%s%x00%b%x1e")
	cmd.Dir = root
	body, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return matches, []string{"git log for drift check failed: " + strings.TrimSpace(string(exitErr.Stderr))}
		}
		return matches, []string{"git log for drift check failed: " + err.Error()}
	}
	for _, record := range strings.Split(string(body), "\x1e") {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}
		parts := strings.SplitN(record, "\x00", 3)
		if len(parts) < 2 {
			continue
		}
		hash := parts[0]
		message := strings.Join(parts[1:], "\n")
		for _, bead := range beads {
			if bead.ID != "" && strings.Contains(message, bead.ID) {
				matches[bead.ID] = append(matches[bead.ID], shortHash(hash))
			}
		}
	}
	return matches, nil
}

func dirtyPathNames(root string) []string {
	cmd := exec.Command("git", "status", "--porcelain=v1", "--untracked-files=all")
	cmd.Dir = root
	body, err := cmd.Output()
	if err != nil {
		return nil
	}
	paths := map[string]bool{}
	for _, line := range strings.Split(string(body), "\n") {
		if len(line) < 4 {
			continue
		}
		path := filepath.ToSlash(strings.TrimSpace(line[3:]))
		if strings.Contains(path, " -> ") {
			parts := strings.Split(path, " -> ")
			path = strings.TrimSpace(parts[len(parts)-1])
		}
		if path != "" {
			paths[path] = true
		}
	}
	out := make([]string, 0, len(paths))
	for path := range paths {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func hasDriftStatus(findings []beadsDriftFinding, status string) bool {
	for _, finding := range findings {
		if finding.Status == status {
			return true
		}
	}
	return false
}

func beadsDriftNextSteps(report beadsDriftReport) []scaffold.RecoveryStep {
	if report.Status == "possible" {
		return []scaffold.RecoveryStep{{
			ID:      "inspect-possible-drift",
			Message: "Inspect the dirty paths and the named bead before deciding whether the closure needs a corrective commit or tracker note.",
			Command: "git status --short && br show <bead-id>",
			Fatal:   false,
		}, {
			ID:      "query-attestations",
			Message: "Confirm whether a relevant attestation exists under a different bead or feature id.",
			Command: "burpvalve attestations list --json",
			Fatal:   false,
		}}
	}
	if report.Status == "warning" {
		return []scaffold.RecoveryStep{{
			ID:      "repair-drift-inputs",
			Message: "Resolve the warning source, then rerun the read-only drift check.",
			Command: "burpvalve beads drift --json",
			Fatal:   false,
		}}
	}
	return []scaffold.RecoveryStep{{
		ID:      "continue",
		Message: "No possible dirty-tree closure drift was detected in the selected window.",
		Command: "burpvalve beads drift --json",
		Fatal:   false,
	}}
}

func shortHash(hash string) string {
	hash = strings.TrimSpace(hash)
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

func printBeadsDrift(out io.Writer, report beadsDriftReport) {
	ui := cliui.New(shouldColorWriter(out))
	fmt.Fprintln(out, ui.Section("Burpvalve beads drift"))
	fmt.Fprintf(out, "%s %s\n", ui.Label("Status:"), ui.Status(report.Status))
	fmt.Fprintf(out, "%s %v\n", ui.Label("Mutating:"), report.Mutating)
	fmt.Fprintf(out, "%s %s\n", ui.Label("Root:"), ui.Path(report.Root))
	fmt.Fprintf(out, "%s %s since %s\n", ui.Label("Window:"), report.Window, report.Since)
	fmt.Fprintf(out, "%s %v\n", ui.Label("Dirty tree:"), report.DirtyTree)
	if len(report.Findings) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, ui.Section("Findings"))
		for _, finding := range report.Findings {
			fmt.Fprintf(out, "- %s: %s (%s)\n", finding.BeadID, finding.Status, finding.Message)
		}
	}
	if len(report.Warnings) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, ui.Section("Warnings"))
		for _, warning := range report.Warnings {
			fmt.Fprintf(out, "- %s\n", warning)
		}
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, ui.Section("Next steps"))
	for _, step := range report.NextSteps {
		fmt.Fprintf(out, "- %s\n", step.Message)
	}
}

func runBeadsCloseRobots(cmd *cobra.Command, opts beadsCloseOptions) error {
	var input robotBeadsCloseInput
	if err := decodeRobotInput(cmd.InOrStdin(), &input); err != nil {
		return err
	}
	if input.Root != "" {
		opts.root = input.Root
	}
	if len(input.BeadIDs) > 0 {
		opts.beadIDs = append([]string(nil), input.BeadIDs...)
	}
	if input.Reason != "" {
		opts.reason = input.Reason
	}
	if input.Feature != "" {
		opts.feature = input.Feature
	}
	if input.BeadRationale != "" {
		opts.beadRationale = input.BeadRationale
	}
	if input.ResponsesPath != "" {
		opts.responses = input.ResponsesPath
	} else if input.Responses != "" {
		opts.responses = input.Responses
	}
	opts.adminOnly = opts.adminOnly || input.AdminOnly
	opts.resume = opts.resume || input.Resume
	opts.yes = opts.yes || input.Confirm
	if input.CommitMessage != "" {
		opts.commitMessage = input.CommitMessage
	}
	if input.Agent != "" {
		opts.agent = input.Agent
	}
	if input.Model != "" {
		opts.model = input.Model
	}
	opts.jsonOutput = true
	return runBeadsClose(cmd, opts)
}

func runBeadsClose(cmd *cobra.Command, opts beadsCloseOptions) error {
	result, err := executeBeadsClose(cmd, opts)
	if opts.jsonOutput || robotsMode {
		if encodeErr := encodeJSON(cmd.OutOrStdout(), result, "encode beads close"); encodeErr != nil {
			return encodeErr
		}
	} else {
		printBeadsClose(cmd.OutOrStdout(), result)
	}
	if err != nil {
		return exitCode(2)
	}
	return nil
}

func executeBeadsClose(cmd *cobra.Command, opts beadsCloseOptions) (beadsCloseResult, error) {
	beadIDs, rationale, normErr := normalizeCloseBeads(opts.beadIDs, opts.beadRationale, opts.adminOnly)
	root, err := filepath.Abs(defaultCLIRoot(opts.root))
	if err != nil {
		root = defaultCLIRoot(opts.root)
	}
	if len(beadIDs) > 0 && strings.TrimSpace(opts.feature) == "" {
		opts.feature = beadIDs[0]
	}
	journalPath := ""
	if len(beadIDs) > 0 {
		journalPath = closureJournalPath(beadIDs[0])
	}
	result := beadsCloseResult{
		SchemaVersion: 1,
		Command:       "beads close",
		Status:        "blocked",
		Fatal:         true,
		Root:          root,
		BeadIDs:       beadIDs,
		JournalPath:   journalPath,
	}
	journal := beadsCloseJournal{SchemaVersion: 1, Command: "beads close", BeadIDs: beadIDs}
	if normErr != nil {
		result.Warnings = append(result.Warnings, normErr.Error())
		result.NextSteps = beadsCloseResumeSteps(beadIDs, opts, normErr.Error(), true)
		return result, normErr
	}
	opts.beadIDs = beadIDs
	opts.beadRationale = rationale
	if strings.TrimSpace(opts.reason) == "" {
		err := errors.New("--reason is required for beads close")
		result.Warnings = append(result.Warnings, err.Error())
		result.NextSteps = beadsCloseResumeSteps(beadIDs, opts, "Pass --reason with a bead/feature reason, not a commit SHA.", true)
		return result, err
	}
	if opts.yes && strings.TrimSpace(opts.commitMessage) == "" {
		err := errors.New("--message is required with --yes")
		result.Warnings = append(result.Warnings, err.Error())
		result.NextSteps = beadsCloseResumeSteps(beadIDs, opts, "Pass --message for the final git commit, or omit --yes to stop before commit.", true)
		return result, err
	}

	preflight, preflightErr := buildBeadsPreflightReport(beadsPreflightOptions{
		root:                   root,
		adminOnly:              opts.adminOnly,
		beadRationale:          opts.beadRationale,
		requireDeliveryPayload: !opts.adminOnly,
	}, beadIDs)
	result.Preflight = preflight
	if preflightErr != nil {
		journal.Steps = append(journal.Steps, beadsCloseMessageStep("preflight", "failed", preflightErr.Error()))
		result.Steps = journal.Steps
		result.Warnings = append(result.Warnings, preflight.Warnings...)
		result.NextSteps = beadsCloseResumeSteps(beadIDs, opts, "Resolve preflight blockers before closing the bead.", true)
		_ = writeBeadsCloseJournal(root, journalPath, journal)
		return result, preflightErr
	}
	journal.Steps = append(journal.Steps, beadsCloseMessageStep("preflight", "done", "preflight completed"))
	_ = writeBeadsCloseJournal(root, journalPath, journal)

	brPath, err := exec.LookPath("br")
	if err != nil {
		journal.Steps = append(journal.Steps, beadsCloseMessageStep("br-close", "failed", "br executable not found on PATH"))
		result.Steps = journal.Steps
		result.Warnings = append(result.Warnings, "br executable not found on PATH")
		result.NextSteps = beadsCloseResumeSteps(beadIDs, opts, "Install br, then resume beads close.", true)
		_ = writeBeadsCloseJournal(root, journalPath, journal)
		return result, err
	}

	beadStatus := map[string]beadsPreflightBead{}
	for _, bead := range preflight.Beads {
		beadStatus[bead.ID] = bead
	}
	for _, id := range beadIDs {
		if beadStatus[id].Status == "closed" {
			journal.Steps = append(journal.Steps, beadsCloseMessageStep("br-close:"+id, "skipped", "bead already closed"))
			continue
		}
		step, err := runBeadsCloseCommand(cmd, root, journalPath, "br-close-"+sanitizeStepID(id), brPath, "close", id, "--reason", opts.reason)
		journal.Steps = append(journal.Steps, step)
		result.Partial = true
		_ = writeBeadsCloseJournal(root, journalPath, journal)
		if err != nil {
			result.Status = "failed"
			result.Steps = journal.Steps
			result.Warnings = append(result.Warnings, step.Message)
			result.NextSteps = beadsCloseResumeSteps(beadIDs, opts, "br close failed; inspect captured stderr and resume after fixing the blocker.", true)
			return result, err
		}
	}

	step, err := runBeadsCloseCommand(cmd, root, journalPath, "br-sync", brPath, "sync", "--flush-only")
	journal.Steps = append(journal.Steps, step)
	result.Partial = true
	_ = writeBeadsCloseJournal(root, journalPath, journal)
	if err != nil {
		result.Status = "failed"
		result.Steps = journal.Steps
		result.Warnings = append(result.Warnings, step.Message)
		result.NextSteps = beadsCloseResumeSteps(beadIDs, opts, "br sync failed after close; resume will recompute bead state first.", true)
		return result, err
	}

	step, err = runBeadsCloseCommand(cmd, root, journalPath, "stage-beads", "git", "add", ".beads/issues.jsonl")
	journal.Steps = append(journal.Steps, step)
	_ = writeBeadsCloseJournal(root, journalPath, journal)
	if err != nil {
		result.Status = "failed"
		result.Steps = journal.Steps
		result.Warnings = append(result.Warnings, step.Message)
		result.NextSteps = beadsCloseResumeSteps(beadIDs, opts, "Staging .beads/issues.jsonl failed; resume after fixing the Git/index blocker.", true)
		return result, err
	}

	if opts.adminOnly {
		if !opts.yes {
			result.Status = "awaiting_commit_confirmation"
			result.Fatal = false
			result.Steps = journal.Steps
			result.NextSteps = []scaffold.RecoveryStep{{
				ID:      "git-commit",
				Message: "Admin-only tracker changes are staged. Run the tracker-only commit only after explicit confirmation.",
				Command: "git commit -m " + shellQuote(defaultCLIString(opts.commitMessage, "Close "+strings.Join(beadIDs, ", "))),
				Fatal:   false,
			}, {
				ID:      "resume",
				Message: "Resume the closure state machine after confirmation.",
				Command: beadsCloseResumeCommand(beadIDs, opts),
				Fatal:   false,
			}}
			_ = writeBeadsCloseJournal(root, journalPath, journal)
			return result, nil
		}

		step, err = runBeadsCloseCommand(cmd, root, journalPath, "git-commit", "git", "commit", "-m", opts.commitMessage)
		journal.Steps = append(journal.Steps, step)
		result.Steps = journal.Steps
		_ = writeBeadsCloseJournal(root, journalPath, journal)
		if err != nil {
			result.Status = "failed"
			result.Fatal = true
			result.Warnings = append(result.Warnings, step.Message)
			result.NextSteps = beadsCloseResumeSteps(beadIDs, opts, "tracker-only git commit failed; fix the commit blocker and resume.", true)
			return result, err
		}
		result.Status = "completed"
		result.Fatal = false
		_ = writeBeadsCloseJournal(root, journalPath, journal)
		return result, nil
	}

	if staleAttestations := stagedAttestationPathNames(root); len(staleAttestations) > 0 {
		message := fmt.Sprintf("staged backpressure attestation must be regenerated for the current staged payload: %s", strings.Join(staleAttestations, ", "))
		journal.Steps = append(journal.Steps, beadsCloseMessageStep("commit-gate", "failed", message))
		result.Status = "failed"
		result.Fatal = true
		result.Partial = true
		result.Steps = journal.Steps
		result.Warnings = append(result.Warnings, message)
		result.NextSteps = beadsCloseResumeSteps(beadIDs, opts, "Unstage stale attestation evidence, regenerate verifier evidence for the current staged payload, then resume.", true)
		_ = writeBeadsCloseJournal(root, journalPath, journal)
		return result, errors.New(message)
	}

	gateResult, err := runBeadsCloseGate(root, opts)
	gateStep := beadsCloseGateStep("commit-gate", gateResult, err)
	journal.Steps = append(journal.Steps, gateStep)
	_ = writeBeadsCloseJournal(root, journalPath, journal)
	if err != nil && gateResult.Status != backpressure.StatusAttestationWritten {
		result.Status = "failed"
		result.Steps = journal.Steps
		result.Warnings = append(result.Warnings, gateResult.Message)
		result.NextSteps = beadsCloseResumeSteps(beadIDs, opts, "Commit gate blocked. Verifier evidence must be for the final staged payload after .beads/issues.jsonl is staged.", true)
		return result, err
	}
	if gateResult.Status == backpressure.StatusAttestationWritten {
		gateStep.Status = "waiting"
		gateStep.Message = "attestation written and not yet staged; this is the expected nonfatal bounce"
		journal.Steps[len(journal.Steps)-1] = gateStep
		result.Status = "attestation_written_unstaged"
		result.Fatal = false
		result.Steps = journal.Steps
		result.NextSteps = []scaffold.RecoveryStep{{
			ID:      "stage-attestation",
			Message: "Stage exactly the attestation path named by Burpvalve, then resume so the gate revalidates the final staged payload.",
			Command: "git add " + gateResult.ArtifactPath + " && " + beadsCloseResumeCommand(beadIDs, opts),
			Fatal:   false,
		}}
		_ = writeBeadsCloseJournal(root, journalPath, journal)

		step, err = runBeadsCloseCommand(cmd, root, journalPath, "stage-attestation", "git", "add", gateResult.ArtifactPath)
		journal.Steps = append(journal.Steps, step)
		_ = writeBeadsCloseJournal(root, journalPath, journal)
		if err != nil {
			result.Status = "failed"
			result.Fatal = true
			result.Steps = journal.Steps
			result.Warnings = append(result.Warnings, step.Message)
			result.NextSteps = beadsCloseResumeSteps(beadIDs, opts, "Staging the named attestation failed; resume after fixing the Git/index blocker.", true)
			return result, err
		}
		gateResult, err = runBeadsCloseGate(root, opts)
		gateStep = beadsCloseGateStep("commit-gate-revalidate", gateResult, err)
		journal.Steps = append(journal.Steps, gateStep)
		_ = writeBeadsCloseJournal(root, journalPath, journal)
		if err != nil {
			result.Status = "failed"
			result.Fatal = true
			result.Steps = journal.Steps
			result.Warnings = append(result.Warnings, gateResult.Message)
			result.NextSteps = beadsCloseResumeSteps(beadIDs, opts, "Gate revalidation failed after staging the attestation; regenerate evidence for the current staged payload.", true)
			return result, err
		}
	}

	if !opts.yes {
		result.Status = "awaiting_commit_confirmation"
		result.Fatal = false
		result.Steps = journal.Steps
		result.NextSteps = []scaffold.RecoveryStep{{
			ID:      "git-commit",
			Message: "Gate revalidated. Run the final commit only after explicit confirmation.",
			Command: "git commit -m " + shellQuote(defaultCLIString(opts.commitMessage, "Close "+strings.Join(beadIDs, ", "))),
			Fatal:   false,
		}, {
			ID:      "resume",
			Message: "Resume the closure state machine after confirmation.",
			Command: beadsCloseResumeCommand(beadIDs, opts),
			Fatal:   false,
		}}
		_ = writeBeadsCloseJournal(root, journalPath, journal)
		return result, nil
	}

	step, err = runBeadsCloseCommand(cmd, root, journalPath, "git-commit", "git", "commit", "-m", opts.commitMessage)
	journal.Steps = append(journal.Steps, step)
	result.Steps = journal.Steps
	_ = writeBeadsCloseJournal(root, journalPath, journal)
	if err != nil {
		result.Status = "failed"
		result.Fatal = true
		result.Warnings = append(result.Warnings, step.Message)
		result.NextSteps = beadsCloseResumeSteps(beadIDs, opts, "git commit failed after gate pass; fix the commit blocker and resume.", true)
		return result, err
	}
	result.Status = "completed"
	result.Fatal = false
	return result, nil
}

func runBeadsCloseGate(root string, opts beadsCloseOptions) (backpressure.PreCommitResult, error) {
	ctx, cancel := backpressure.WithTimeout(context.Background())
	defer cancel()
	return backpressure.RunPreCommit(ctx, backpressure.PreCommitOptions{
		Root:            root,
		ExplicitFeature: opts.feature,
		BeadIDs:         opts.beadIDs,
		BeadRationale:   opts.beadRationale,
		ResponsesPath:   opts.responses,
		Agent:           opts.agent,
		Model:           opts.model,
		ColorMode:       colorMode,
	})
}

func runBeadsCloseCommand(cmd *cobra.Command, root, journalPath, stepID, name string, args ...string) (beadsCloseJournalStep, error) {
	start := time.Now().UTC()
	command := shellCommandLine(append([]string{name}, args...))
	fmt.Fprintf(cmd.ErrOrStderr(), "running: %s\n", command)
	execCmd := exec.Command(name, args...)
	execCmd.Dir = root
	var stdout, stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr
	err := execCmd.Run()
	completed := time.Now().UTC()
	step := beadsCloseJournalStep{ID: stepID, Status: "done", Command: command, StartedAt: start.Format(time.RFC3339Nano), CompletedAt: completed.Format(time.RFC3339Nano)}
	if stdout.Len() > 0 {
		step.StdoutRef = writeBeadsCloseOutputRef(root, journalPath, stepID, "stdout", stdout.Bytes())
	}
	if stderr.Len() > 0 {
		step.StderrRef = writeBeadsCloseOutputRef(root, journalPath, stepID, "stderr", stderr.Bytes())
	}
	if err != nil {
		step.Status = "failed"
		step.ExitStatus = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			step.ExitStatus = exitErr.ExitCode()
		}
		step.Message = strings.TrimSpace(stderr.String())
		if step.Message == "" {
			step.Message = err.Error()
		}
	}
	return step, err
}

func beadsCloseGateStep(id string, result backpressure.PreCommitResult, err error) beadsCloseJournalStep {
	step := beadsCloseMessageStep(id, "done", result.Message)
	step.ArtifactPath = result.ArtifactPath
	if err != nil {
		step.Status = "failed"
		step.Message = defaultCLIString(result.Message, err.Error())
	}
	if result.Status == backpressure.StatusPassed {
		step.Status = "done"
		step.Message = "commit gate passed"
	}
	return step
}

func beadsCloseMessageStep(id, status, message string) beadsCloseJournalStep {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return beadsCloseJournalStep{ID: id, Status: status, Message: message, StartedAt: now, CompletedAt: now}
}

func writeBeadsCloseJournal(root, path string, journal beadsCloseJournal) error {
	if path == "" {
		return nil
	}
	journal.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	body, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return err
	}
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, append(body, '\n'), 0o644)
}

func writeBeadsCloseOutputRef(root, journalPath, stepID, stream string, body []byte) string {
	if journalPath == "" {
		return ""
	}
	dir := filepath.Dir(filepath.ToSlash(journalPath))
	ref := filepath.ToSlash(filepath.Join(dir, sanitizeStepID(stepID)+"."+stream+".txt"))
	full := filepath.Join(root, filepath.FromSlash(ref))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return ""
	}
	if err := os.WriteFile(full, body, 0o644); err != nil {
		return ""
	}
	return ref
}

func closureJournalPath(beadID string) string {
	return "log/backpressure/closures/" + sanitizeStepID(beadID) + ".json"
}

func sanitizeStepID(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

func beadsCloseResumeSteps(beadIDs []string, opts beadsCloseOptions, message string, fatal bool) []scaffold.RecoveryStep {
	return []scaffold.RecoveryStep{{ID: "resume", Message: message, Command: beadsCloseResumeCommand(beadIDs, opts), Fatal: fatal}}
}

func beadsCloseResumeCommand(beadIDs []string, opts beadsCloseOptions) string {
	parts := []string{"burpvalve", "beads", "close"}
	parts = append(parts, beadIDs...)
	parts = append(parts, "--resume", "--reason", opts.reason)
	if opts.root != "" && opts.root != "." {
		parts = append(parts, "--root", opts.root)
	}
	if opts.responses != "" {
		parts = append(parts, "--responses", opts.responses)
	}
	if opts.feature != "" {
		parts = append(parts, "--feature", opts.feature)
	}
	if opts.beadRationale != "" {
		parts = append(parts, "--bead-rationale", opts.beadRationale)
	}
	if opts.adminOnly {
		parts = append(parts, "--admin-only")
	}
	return shellCommandLine(parts)
}

func shellCommandLine(parts []string) string {
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		quoted = append(quoted, shellArgQuote(part))
	}
	return strings.Join(quoted, " ")
}

func shellArgQuote(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.ContainsAny(value, " \t\r\n'\"\\$`!()[]{};&|<>*?") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func defaultCLIString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func printBeadsClose(out io.Writer, result beadsCloseResult) {
	ui := cliui.New(shouldColorWriter(out))
	fmt.Fprintln(out, ui.Section("Burpvalve beads close"))
	fmt.Fprintf(out, "%s %s\n", ui.Label("Status:"), ui.Status(result.Status))
	fmt.Fprintf(out, "%s %s\n", ui.Label("Journal:"), ui.Path(result.JournalPath))
	for _, step := range result.Steps {
		fmt.Fprintf(out, "- %s: %s\n", step.ID, step.Status)
	}
	if len(result.NextSteps) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, ui.Section("Next steps"))
		for _, step := range result.NextSteps {
			fmt.Fprintf(out, "- %s\n", step.Command)
		}
	}
}

func shortValue(value string, max int) string {
	if value == "" || len(value) <= max {
		return value
	}
	if max <= 1 {
		return value[:max]
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func shortTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format("2006-01-02T15:04Z")
}

func newConfigCommand() *cobra.Command {
	var target string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "config [show]",
		Short: "Show Burpvalve config paths and merged defaults.",
		Long: `Show the global Burpvalve config path, the project override path, and the
effective defaults after merging them. The global file is loaded first. The
project file .burpvalve.json overrides it for one repo. The output also shows
which effective values came from the global file or the project file.`,
		Example: `  burpvalve config
  burpvalve config show
  burpvalve config --target /path/to/repo --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigShow(cmd, target, jsonOutput || robotsMode)
		},
	}
	cmd.Flags().StringVar(&target, "target", ".", "target project root")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "print machine-readable JSON")
	show := &cobra.Command{
		Use:     "show",
		Aliases: []string{"inspect"},
		Short:   "Show config paths, effective defaults, and value sources.",
		Long:    "Show global and project config paths, merged defaults, and whether each configured value came from the global or project file.",
		Example: `  burpvalve config show
  burpvalve config show --target /path/to/repo --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigShow(cmd, target, jsonOutput || robotsMode)
		},
	}
	show.Flags().StringVar(&target, "target", ".", "target project root")
	show.Flags().BoolVar(&jsonOutput, "json", false, "print machine-readable JSON")
	cmd.AddCommand(show)
	cmd.AddCommand(newConfigInitCommand())
	return cmd
}

func newConfigInitCommand() *cobra.Command {
	var opts configInitOptions
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create or update global or project Burpvalve config.",
		Long: `Create or update a versioned Burpvalve config file.

Choose global config when you want to remember your personal defaults for
skills, command shims, shell completion, colors, and future setup selections.
Choose project config when a repo should carry its own defaults in
.burpvalve.json. Project config overrides global config for that repo.

Config init only writes the selected config file. It does not run setup, init,
repair, completion install, or hook wiring.

Terminal runs without --file open a question-and-answer flow, show the final JSON,
and ask for final confirmation defaulting to No. Scripts can pass --file with
--force, or use --robots with stdin JSON and confirm=true. Partial file or robot
updates are merged with existing config so omitted fields are preserved.`,
		Example: `  burpvalve config init --project --file .burpvalve.seed.json --force --json
  burpvalve config init --project
  burpvalve config init --global --file config.json --force
  printf '{"scope":"project","confirm":true,"config":{"schema_version":1,"defaults":{"verifier":{"authorized":true,"authorized_at":"2026-07-02T12:00:00Z","authorization_scope":"repo:/path/to/repo","spawn_method":"ntm","transcripts":"summary"}}}}' | burpvalve config init --robots`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigInit(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.target, "target", ".", "target project root")
	cmd.Flags().BoolVar(&opts.global, "global", false, "write the global config file")
	cmd.Flags().BoolVar(&opts.project, "project", false, "write the project .burpvalve.json file")
	cmd.Flags().StringVar(&opts.file, "file", "", "JSON config file to write; use - for stdin")
	cmd.Flags().BoolVar(&opts.force, "force", false, "write without interactive confirmation")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "print machine-readable JSON")
	return cmd
}

func runConfigShow(cmd *cobra.Command, target string, jsonOutput bool) error {
	effective, err := bvconfig.Load(target)
	if err != nil {
		return fail(2, "%v", err)
	}
	view := configView(effective)
	if jsonOutput {
		return encodeJSON(cmd.OutOrStdout(), view, "encode config")
	}
	printConfigView(cmd.OutOrStdout(), view)
	return nil
}

func configView(effective bvconfig.Effective) map[string]any {
	return map[string]any{
		"schema_version": bvconfig.SchemaVersion,
		"global_path":    effective.GlobalPath,
		"global_found":   effective.GlobalFound,
		"project_path":   effective.ProjectPath,
		"project_found":  effective.ProjectFound,
		"defaults":       effective.File.Defaults,
		"sources":        bvconfig.SortedSources(effective.Sources),
		"note":           "Project .burpvalve.json overrides the global config for this repo.",
	}
}

func printConfigView(out io.Writer, view map[string]any) {
	ui := cliui.New(shouldColorWriter(out))
	fmt.Fprintln(out, ui.Section("Burpvalve config"))
	fmt.Fprintf(out, "%s %s %s\n", ui.Label("Global:"), ui.Path(displayPath(fmt.Sprint(view["global_path"]))), configFoundLabel(ui, view["global_found"]))
	fmt.Fprintf(out, "%s %s %s\n", ui.Label("Project:"), ui.Path(displayPath(fmt.Sprint(view["project_path"]))), configFoundLabel(ui, view["project_found"]))
	fmt.Fprintln(out, ui.Muted("Project .burpvalve.json overrides the global config for this repo."))
	body, err := json.MarshalIndent(view["defaults"], "", "  ")
	if err == nil {
		fmt.Fprintln(out)
		fmt.Fprintln(out, ui.Section("Effective defaults"))
		fmt.Fprintln(out, string(body))
	}
	if sources, ok := view["sources"].([]bvconfig.Source); ok && len(sources) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, ui.Section("Sources"))
		for _, source := range sources {
			fmt.Fprintf(out, "%s %s\n", ui.Label(source.Key+":"), source.Source)
		}
	}
}

func configFoundLabel(ui cliui.Styles, value any) string {
	if found, ok := value.(bool); ok && found {
		return ui.Muted("(found)")
	}
	return ui.Muted("(missing)")
}

func runConfigInit(cmd *cobra.Command, opts configInitOptions) error {
	var input robotConfigInitInput
	if robotsMode {
		if err := decodeStdinJSON(cmd.InOrStdin(), &input); err != nil {
			return fail(2, "parse config init robot input: %v", err)
		}
		if input.Target != "" {
			opts.target = input.Target
		}
		switch input.Scope {
		case "global":
			opts.global = true
		case "project":
			opts.project = true
		case "":
			return fail(2, "config init robot input requires scope=global or scope=project")
		default:
			return fail(2, "invalid config init scope %q; expected global or project", input.Scope)
		}
		opts.force = opts.force || input.Confirm
		return writeConfigInit(cmd, opts, input.Config)
	}
	if shouldRunConfigInitWizard(opts) {
		return runConfigInitWizard(cmd, opts)
	}
	configFile, err := loadConfigInitFile(cmd, opts.file)
	if err != nil {
		return fail(2, "%v", err)
	}
	return writeConfigInit(cmd, opts, configFile)
}

func shouldRunConfigInitWizard(opts configInitOptions) bool {
	if opts.force || opts.jsonOutput || strings.TrimSpace(opts.file) != "" {
		return false
	}
	return isInteractiveTerminal(os.Stdin, os.Stdout)
}

func runConfigInitWizard(cmd *cobra.Command, opts configInitOptions) error {
	effective, err := bvconfig.Load(opts.target)
	if err != nil {
		return fail(2, "%v", err)
	}
	scope, err := chooseConfigScope(opts, effective)
	if err != nil {
		return err
	}
	opts.global = scope == "global"
	opts.project = scope == "project"
	path, _, err := configInitPath(opts)
	if err != nil {
		return fail(2, "%v", err)
	}
	file, err := readConfigFileIfExists(path)
	if err != nil {
		return fail(2, "%v", err)
	}
	if file.SchemaVersion == 0 {
		file.SchemaVersion = bvconfig.SchemaVersion
	}
	color := shouldColor(os.Stdout)
	file, err = askConfigSections(os.Stdin, os.Stdout, file, effective, color)
	if err != nil {
		if errors.Is(err, charmui.ErrCancelled) {
			return fail(2, "config init cancelled")
		}
		return fail(2, "config init prompt failed: %v", err)
	}
	description, err := configInitConfirmDescription(scope, path, file)
	if err != nil {
		return fail(2, "%v", err)
	}
	confirmed, err := configAskConfirm(os.Stdin, os.Stdout, charmui.ConfirmPrompt{
		Title:       "Confirm Burpvalve config init",
		Description: description,
		Prompt:      "Write this config file?",
		Default:     false,
		Color:       color,
	})
	if err != nil {
		if errors.Is(err, charmui.ErrCancelled) {
			return fail(2, "config init cancelled")
		}
		return fail(2, "config init confirmation failed: %v", err)
	}
	if !confirmed {
		return fail(2, "config init cancelled")
	}
	opts.force = true
	return writeConfigInit(cmd, opts, file)
}

func chooseConfigScope(opts configInitOptions, effective bvconfig.Effective) (string, error) {
	switch {
	case opts.global && opts.project:
		return "", fail(2, "choose exactly one of --global or --project")
	case opts.global:
		return "global", nil
	case opts.project:
		return "project", nil
	}
	choice, err := configAskSelect(os.Stdin, os.Stdout, charmui.SelectPrompt{
		Title:       "Burpvalve config",
		Description: fmt.Sprintf("Global remembers your defaults at %s. Project writes %s and overrides global values for this repo.", displayPath(effective.GlobalPath), displayPath(effective.ProjectPath)),
		Prompt:      "Where should Burpvalve write config?",
		DefaultID:   "project",
		Color:       shouldColor(os.Stdout),
		Choices: []charmui.Choice{
			{ID: "project", Label: "project", Description: "store defaults with this repo"},
			{ID: "global", Label: "global", Description: "remember defaults for your user account"},
		},
	})
	if err != nil {
		if errors.Is(err, charmui.ErrCancelled) {
			return "", fail(2, "config init cancelled")
		}
		return "", fail(2, "config init scope prompt failed: %v", err)
	}
	return choice.ID, nil
}

func readConfigFileIfExists(path string) (bvconfig.File, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return bvconfig.File{SchemaVersion: bvconfig.SchemaVersion}, nil
		}
		return bvconfig.File{}, fmt.Errorf("read existing config %s: %w", path, err)
	}
	var file bvconfig.File
	if err := decodeStrictJSON(body, &file); err != nil {
		return bvconfig.File{}, fmt.Errorf("parse existing config %s: %w", path, err)
	}
	file = bvconfig.Normalize(file)
	if err := bvconfig.Validate(file); err != nil {
		return bvconfig.File{}, fmt.Errorf("validate existing config %s: %w", path, err)
	}
	return file, nil
}

func askConfigSections(in io.Reader, out io.Writer, file bvconfig.File, effective bvconfig.Effective, color bool) (bvconfig.File, error) {
	var err error
	file, err = askInstallConfigSection(in, out, file, effective, color)
	if err != nil {
		return file, err
	}
	file, err = askCompletionConfigSection(in, out, file, effective, color)
	if err != nil {
		return file, err
	}
	file, err = askOutputConfigSection(in, out, file, effective, color)
	if err != nil {
		return file, err
	}
	file, err = askVerifierConfigSection(in, out, file, effective, color)
	if err != nil {
		return file, err
	}
	if err := askScaffoldConfigSection(in, out, "Default repo scaffold pieces", "Which pieces should init include by default?", &file.Defaults.Init.ScaffoldDefaults, color, true); err != nil {
		return file, err
	}
	copyRepair, err := configAskConfirm(in, out, charmui.ConfirmPrompt{
		Title:       "Repair defaults",
		Description: "Repair can use the same defaults as init, or preserve its current separate settings.",
		Prompt:      "Use the same scaffold defaults for repair?",
		Default:     false,
		Color:       color,
	})
	if err != nil {
		return file, err
	}
	if copyRepair {
		file.Defaults.Repair.ScaffoldDefaults = file.Defaults.Init.ScaffoldDefaults
		return file, nil
	}
	if err := askScaffoldConfigSection(in, out, "Repair defaults", "Which pieces should repair include by default?", &file.Defaults.Repair.ScaffoldDefaults, color, false); err != nil {
		return file, err
	}
	return file, nil
}

func askInstallConfigSection(in io.Reader, out io.Writer, file bvconfig.File, effective bvconfig.Effective, color bool) (bvconfig.File, error) {
	ok, err := configAskConfirm(in, out, charmui.ConfirmPrompt{
		Title:       "Install locations",
		Description: "These defaults remember where Burpvalve installs skills and the global command shim. Existing values are prefilled.",
		Prompt:      "Configure install locations?",
		Default:     false,
		Color:       color,
	})
	if err != nil || !ok {
		return file, err
	}
	if file.Defaults.SkillsDir, err = askOptionalText(in, out, "Install locations", "Skills directory", firstNonEmptyString(file.Defaults.SkillsDir, effective.File.Defaults.SkillsDir, "~/skills"), color); err != nil {
		return file, err
	}
	if file.Defaults.BinDir, err = askOptionalText(in, out, "Install locations", "Command bin directory", firstNonEmptyString(file.Defaults.BinDir, effective.File.Defaults.BinDir, "~/.local/bin"), color); err != nil {
		return file, err
	}
	return file, nil
}

func askCompletionConfigSection(in io.Reader, out io.Writer, file bvconfig.File, effective bvconfig.Effective, color bool) (bvconfig.File, error) {
	ok, err := configAskConfirm(in, out, charmui.ConfirmPrompt{
		Title:       "Shell and completion",
		Description: "These defaults help completion install choose a shell, completion path, and startup file.",
		Prompt:      "Configure shell completion defaults?",
		Default:     false,
		Color:       color,
	})
	if err != nil || !ok {
		return file, err
	}
	shell, err := askChoice(in, out, "Shell and completion", "Preferred shell", firstNonEmptyString(file.Defaults.Shell, effective.File.Defaults.Shell, "zsh"), color, []charmui.Choice{
		{ID: "bash", Label: "bash", Description: "Bash completion"},
		{ID: "zsh", Label: "zsh", Description: "Zsh completion"},
		{ID: "fish", Label: "fish", Description: "Fish completion"},
		{ID: "powershell", Label: "powershell", Description: "PowerShell completion"},
	})
	if err != nil {
		return file, err
	}
	file.Defaults.Shell = shell
	if file.Defaults.Completion.Path, err = askOptionalText(in, out, "Shell and completion", "Completion script path", firstNonEmptyString(file.Defaults.Completion.Path, effective.File.Defaults.Completion.Path), color); err != nil {
		return file, err
	}
	if file.Defaults.Completion.RCFile, err = askOptionalText(in, out, "Shell and completion", "Shell startup file", firstNonEmptyString(file.Defaults.Completion.RCFile, effective.File.Defaults.Completion.RCFile), color); err != nil {
		return file, err
	}
	update, err := configAskConfirm(in, out, charmui.ConfirmPrompt{
		Title:       "Shell and completion",
		Description: "When enabled, completion install can update the shell startup file after confirmation.",
		Prompt:      "Remember update-rc as the default?",
		Default:     bvconfig.BoolValue(firstBool(file.Defaults.Completion.UpdateRC, effective.File.Defaults.Completion.UpdateRC), false),
		Color:       color,
	})
	if err != nil {
		return file, err
	}
	file.Defaults.Completion.UpdateRC = boolPtr(update)
	return file, nil
}

func askOutputConfigSection(in io.Reader, out io.Writer, file bvconfig.File, effective bvconfig.Effective, color bool) (bvconfig.File, error) {
	ok, err := configAskConfirm(in, out, charmui.ConfirmPrompt{
		Title:       "Output and confirmation",
		Description: "These defaults control color and future confirmation defaults. Mutating commands still ask before writing unless forced or confirmed by robot input.",
		Prompt:      "Configure output defaults?",
		Default:     false,
		Color:       color,
	})
	if err != nil || !ok {
		return file, err
	}
	file.Defaults.Color, err = askChoice(in, out, "Output and confirmation", "Color mode", firstNonEmptyString(file.Defaults.Color, effective.File.Defaults.Color, "auto"), color, []charmui.Choice{
		{ID: "auto", Label: "auto", Description: "detect terminal support"},
		{ID: "always", Label: "always", Description: "force color"},
		{ID: "never", Label: "never", Description: "disable color"},
	})
	if err != nil {
		return file, err
	}
	confirm, err := configAskConfirm(in, out, charmui.ConfirmPrompt{
		Title:       "Output and confirmation",
		Description: "This stores the preferred confirmation default; commands still show their final confirmation step.",
		Prompt:      "Default future confirmations to Yes?",
		Default:     bvconfig.BoolValue(firstBool(file.Defaults.Confirm, effective.File.Defaults.Confirm), false),
		Color:       color,
	})
	if err != nil {
		return file, err
	}
	file.Defaults.Confirm = boolPtr(confirm)
	return file, nil
}

func askVerifierConfigSection(in io.Reader, out io.Writer, file bvconfig.File, effective bvconfig.Effective, color bool) (bvconfig.File, error) {
	ok, err := configAskConfirm(in, out, charmui.ConfirmPrompt{
		Title:       "Verifier orchestration",
		Description: "These defaults record standing authorization and preferences for read-only verifier subagents. Authorization is policy metadata, not verifier evidence.",
		Prompt:      "Configure verifier defaults?",
		Default:     file.Defaults.Verifier.Authorized != nil || effective.File.Defaults.Verifier.Authorized != nil,
		Color:       color,
	})
	if err != nil || !ok {
		return file, err
	}
	authorized, err := configAskConfirm(in, out, charmui.ConfirmPrompt{
		Title:       "Verifier orchestration",
		Description: "Answer Yes only if agents in this repo are authorized to spawn read-only verifier subagents for backpressure checks.",
		Prompt:      "Are agents in this repo authorized to spawn read-only verifier subagents for backpressure checks?",
		Default:     bvconfig.BoolValue(firstBool(file.Defaults.Verifier.Authorized, effective.File.Defaults.Verifier.Authorized), false),
		Color:       color,
	})
	if err != nil {
		return file, err
	}
	file.Defaults.Verifier.Authorized = boolPtr(authorized)
	if file.Defaults.Verifier.AuthorizedAt, err = askOptionalText(in, out, "Verifier orchestration", "Authorization timestamp", firstNonEmptyString(file.Defaults.Verifier.AuthorizedAt, effective.File.Defaults.Verifier.AuthorizedAt, time.Now().UTC().Format(time.RFC3339)), color); err != nil {
		return file, err
	}
	if file.Defaults.Verifier.AuthorizationScope, err = askOptionalText(in, out, "Verifier orchestration", "Authorization scope", firstNonEmptyString(file.Defaults.Verifier.AuthorizationScope, effective.File.Defaults.Verifier.AuthorizationScope, "repo:"+displayPath(".")), color); err != nil {
		return file, err
	}
	spawnMethod, err := askChoice(in, out, "Verifier orchestration", "Verifier spawn method", firstNonEmptyString(file.Defaults.Verifier.SpawnMethod, effective.File.Defaults.Verifier.SpawnMethod, "manual"), color, []charmui.Choice{
		{ID: "native", Label: "native", Description: "runtime-native read-only verifier subagents"},
		{ID: "ntm", Label: "ntm", Description: "NTM panes for verifier fanout"},
		{ID: "hermes", Label: "hermes", Description: "Hermes-routed verifier agents"},
		{ID: "manual", Label: "manual", Description: "human-managed verifier handoff"},
	})
	if err != nil {
		return file, err
	}
	file.Defaults.Verifier.SpawnMethod = spawnMethod
	transcripts, err := askChoice(in, out, "Verifier orchestration", "Verifier transcript storage", firstNonEmptyString(file.Defaults.Verifier.Transcripts, effective.File.Defaults.Verifier.Transcripts, "summary"), color, []charmui.Choice{
		{ID: "summary", Label: "summary", Description: "store transcript references and summaries"},
		{ID: "full", Label: "full", Description: "store full local transcripts when allowed"},
		{ID: "committed", Label: "committed", Description: "commit transcript records intentionally"},
	})
	if err != nil {
		return file, err
	}
	file.Defaults.Verifier.Transcripts = transcripts
	return file, nil
}

func askScaffoldConfigSection(in io.Reader, out io.Writer, title string, prompt string, defaults *bvconfig.ScaffoldDefaults, color bool, includeNTM bool) error {
	ok, err := configAskConfirm(in, out, charmui.ConfirmPrompt{
		Title:       title,
		Description: "Skipped sections preserve existing values. Answering here stores defaults; it does not initialize or repair the repo.",
		Prompt:      "Configure this section?",
		Default:     false,
		Color:       color,
	})
	if err != nil || !ok {
		return err
	}
	items := []struct {
		label       string
		description string
		value       **bool
		fallback    bool
	}{
		{"AGENTS.md", "agent operating contract", &defaults.Agents, true},
		{"CLAUDE.md", "compatibility symlink", &defaults.Claude, true},
		{"docs/", "durable project knowledge", &defaults.Docs, true},
		{"plans/", "implementation plans", &defaults.Plans, true},
		{"log/", "work logs and failed reports", &defaults.Log, true},
		{"backpressure/", "conditions and manifest", &defaults.Backpressure, true},
		{"attestations", "passing evidence directory", &defaults.Attestations, true},
		{"pre-commit hook", "commit and lint gate", &defaults.PreCommit, true},
		{"git hooksPath", "point Git at .githooks/", &defaults.HooksPath, true},
		{"repo-local bin/burpvalve", "optional hook fallback", &defaults.RepoBin, false},
		{"tools/burpvalve docs", "local replacement notes", &defaults.ToolDocs, true},
		{".beads", "optional local task graph", &defaults.Beads, true},
	}
	if includeNTM {
		items = append(items, struct {
			label       string
			description string
			value       **bool
			fallback    bool
		}{"NTM bridge", "optional coordination snapshot", &defaults.NTM, true})
	}
	for _, item := range items {
		value, err := configAskConfirm(in, out, charmui.ConfirmPrompt{
			Title:       title,
			Description: item.description,
			Prompt:      prompt + " " + item.label + "?",
			Default:     bvconfig.BoolValue(*item.value, item.fallback),
			Color:       color,
		})
		if err != nil {
			return err
		}
		*item.value = boolPtr(value)
	}
	return nil
}

func askOptionalText(in io.Reader, out io.Writer, title string, prompt string, fallback string, color bool) (string, error) {
	return configAskText(in, out, charmui.TextPrompt{
		Title:       title,
		Description: "Press enter to keep the shown value, or clear it before accepting to leave this value unset or unchanged.",
		Prompt:      prompt,
		Default:     fallback,
		Color:       color,
	})
}

func askChoice(in io.Reader, out io.Writer, title string, prompt string, defaultID string, color bool, choices []charmui.Choice) (string, error) {
	choice, err := configAskSelect(in, out, charmui.SelectPrompt{
		Title:     title,
		Prompt:    prompt,
		DefaultID: defaultID,
		Choices:   choices,
		Color:     color,
	})
	if err != nil {
		return "", err
	}
	return choice.ID, nil
}

func firstBool(values ...*bool) *bool {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func boolPtr(value bool) *bool {
	return &value
}

func configPreviewJSON(file bvconfig.File) (string, error) {
	body, err := json.MarshalIndent(bvconfig.Normalize(file), "", "  ")
	if err != nil {
		return "", fmt.Errorf("render config preview: %w", err)
	}
	return string(body), nil
}

func configInitConfirmDescription(scope string, path string, file bvconfig.File) (string, error) {
	preview, err := configPreviewJSON(file)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Scope: %s\nPath: %s\n\n%s", scope, displayPath(path), preview), nil
}

func writeConfigInit(cmd *cobra.Command, opts configInitOptions, file bvconfig.File) error {
	path, scope, err := configInitPath(opts)
	if err != nil {
		return fail(2, "%v", err)
	}
	if !opts.force {
		return fail(2, "config init is mutating; pass --force or use --robots with confirm=true")
	}
	existing, err := readConfigFileIfExists(path)
	if err != nil {
		return fail(2, "%v", err)
	}
	file = bvconfig.Merge(existing, file)
	if err := bvconfig.Write(path, file); err != nil {
		return fail(2, "write %s config: %v", scope, err)
	}
	result := map[string]any{
		"schema_version": bvconfig.SchemaVersion,
		"command":        "config init",
		"status":         "written",
		"scope":          scope,
		"path":           path,
		"config":         bvconfig.Normalize(file),
	}
	if opts.jsonOutput || robotsMode {
		return encodeJSON(cmd.OutOrStdout(), result, "encode config init result")
	}
	ui := cliui.New(shouldColorWriter(cmd.OutOrStdout()))
	fmt.Fprintln(cmd.OutOrStdout(), ui.Section("Config written"))
	fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", ui.Label("Scope:"), scope)
	fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", ui.Label("Path:"), ui.Path(displayPath(path)))
	return nil
}

func configInitPath(opts configInitOptions) (string, string, error) {
	if opts.global == opts.project {
		return "", "", fmt.Errorf("choose exactly one of --global or --project")
	}
	if opts.global {
		path, err := bvconfig.GlobalPath()
		return path, "global", err
	}
	path, err := bvconfig.ProjectPath(opts.target)
	return path, "project", err
}

func loadConfigInitFile(cmd *cobra.Command, path string) (bvconfig.File, error) {
	if strings.TrimSpace(path) == "" {
		return bvconfig.File{}, fmt.Errorf("config init requires --file JSON in non-robot mode")
	}
	var body []byte
	var err error
	if path == "-" {
		body, err = io.ReadAll(cmd.InOrStdin())
	} else {
		body, err = os.ReadFile(path)
	}
	if err != nil {
		return bvconfig.File{}, fmt.Errorf("read config init file: %w", err)
	}
	var file bvconfig.File
	if err := decodeStrictJSON(body, &file); err != nil {
		return bvconfig.File{}, fmt.Errorf("parse config init file: %w", err)
	}
	return file, nil
}

func decodeStrictJSON(body []byte, dst any) error {
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return fmt.Errorf("expected a single JSON object")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func decodeStdinJSON(in io.Reader, dst any) error {
	body, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	return decodeStrictJSON(body, dst)
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the Burpvalve version.",
		Long:  "Print the burpvalve build version.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if robotsMode {
				return encodeJSON(cmd.OutOrStdout(), map[string]string{"version": version}, "encode version")
			}
			fmt.Fprintln(cmd.OutOrStdout(), version)
			return nil
		},
	}
}

type setupOptions struct {
	target     string
	jsonOutput bool
	noBeads    bool
	noNTM      bool
}

type initOptions struct {
	target          string
	targets         []string
	jsonOutput      bool
	force           bool
	noBeads         bool
	noNTM           bool
	noClaude        bool
	noClaudeSymlink bool
	noAgents        bool
	noAgentsMD      bool
	noDocs          bool
	noPlans         bool
	noLog           bool
	noBackpressure  bool
	noAttestations  bool
	noHooks         bool
	noGitHooks      bool
	noPreCommit     bool
	noHooksPath     bool
	gitInit         bool
	noBin           bool
	repoBin         bool
	noToolDocs      bool
	orchestrator    bool
	claudeRoute     string
	dogfood         bool
	noDogfood       bool
}

type repairOptions struct {
	target          string
	targets         []string
	jsonOutput      bool
	force           bool
	noBeads         bool
	noClaude        bool
	noClaudeSymlink bool
	noAgents        bool
	noAgentsMD      bool
	noDocs          bool
	noPlans         bool
	noLog           bool
	noBackpressure  bool
	noAttestations  bool
	noHooks         bool
	noGitHooks      bool
	noPreCommit     bool
	noHooksPath     bool
	gitInit         bool
	noBin           bool
	repoBin         bool
	noToolDocs      bool
	orchestrator    bool
	claudeRoute     string
	adoptClaudeMD   bool
}

type commitOptions struct {
	root              string
	feature           string
	beads             []string
	beadRationale     string
	responses         string
	responsesTemplate bool
	agent             string
	model             string
}

type configInitOptions struct {
	target     string
	global     bool
	project    bool
	file       string
	force      bool
	jsonOutput bool
}

type robotConfigInitInput struct {
	Scope   string        `json:"scope"`
	Target  string        `json:"target"`
	Confirm bool          `json:"confirm"`
	Config  bvconfig.File `json:"config"`
}

var (
	configAskConfirm = charmui.AskConfirm
	configAskSelect  = charmui.AskSelect
	configAskText    = charmui.AskText
)

type lintOptions struct {
	root string
}

type ciOptions struct {
	root    string
	feature string
	commit  string
}

type robotTargetInput struct {
	Target string `json:"target"`
}

type robotCompletionInput struct {
	Target string `json:"target"`
	Shell  string `json:"shell"`
}

type robotCompletionOutput struct {
	SchemaVersion  int                     `json:"schema_version"`
	Command        string                  `json:"command"`
	Target         string                  `json:"target"`
	Shell          string                  `json:"shell"`
	ShellSource    string                  `json:"shell_source"`
	Config         *scaffold.ConfigSummary `json:"config,omitempty"`
	CompletionPath string                  `json:"completion_path"`
	RCFile         string                  `json:"rc_file,omitempty"`
	InstallCommand string                  `json:"install_command"`
	ScriptCommand  string                  `json:"script_command"`
	NextSteps      []string                `json:"next_steps"`
}

type robotScaffoldInput struct {
	Target        string         `json:"target"`
	Targets       []string       `json:"targets"`
	Skip          robotSkipInput `json:"skip"`
	Dogfood       *bool          `json:"dogfood,omitempty"`
	ClaudeRoute   string         `json:"claude_route,omitempty"`
	AdoptClaudeMD bool           `json:"adopt_claude_md,omitempty"`
	Confirm       bool           `json:"confirm"`
	GitInit       bool           `json:"git_init"`
}

type robotSkipInput struct {
	Agents        bool `json:"agents"`
	AgentsMD      bool `json:"agents_md"`
	Attestations  bool `json:"attestations"`
	Backpressure  bool `json:"backpressure"`
	Beads         bool `json:"beads"`
	Bin           bool `json:"bin"`
	Claude        bool `json:"claude"`
	ClaudeSymlink bool `json:"claude_symlink"`
	Docs          bool `json:"docs"`
	GitHooks      bool `json:"git_hooks"`
	Hooks         bool `json:"hooks"`
	HooksPath     bool `json:"hooks_path"`
	Log           bool `json:"log"`
	NTM           bool `json:"ntm"`
	Plans         bool `json:"plans"`
	PreCommit     bool `json:"precommit"`
	ToolDocs      bool `json:"tool_docs"`
}

type robotCommitInput struct {
	Root              string   `json:"root"`
	Feature           string   `json:"feature"`
	Beads             []string `json:"beads"`
	BeadRationale     string   `json:"bead_rationale"`
	Responses         string   `json:"responses"`
	ResponsesPath     string   `json:"responses_path"`
	ResponsesTemplate bool     `json:"responses_template"`
	Agent             string   `json:"agent"`
	Model             string   `json:"model"`
}

type robotLintInput struct {
	Root string `json:"root"`
	Jobs int    `json:"jobs"`
}

type robotLintInitInput struct {
	Root    string `json:"root"`
	Detect  bool   `json:"detect"`
	Write   bool   `json:"write"`
	Force   bool   `json:"force"`
	Preset  string `json:"preset"`
	Jobs    int    `json:"jobs"`
	Confirm bool   `json:"confirm"`
}

type robotCIInput struct {
	Root    string `json:"root"`
	Feature string `json:"feature"`
	Commit  string `json:"commit"`
}

func decodeRobotInput(in io.Reader, dst any) error {
	if file, ok := in.(*os.File); ok {
		info, err := file.Stat()
		if err == nil && info.Mode()&os.ModeCharDevice != 0 {
			return nil
		}
	}
	body, err := io.ReadAll(in)
	if err != nil {
		return fail(2, "read --robots JSON input: %v", err)
	}
	if strings.TrimSpace(string(body)) == "" {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fail(2, "invalid --robots JSON input: %v", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fail(2, "invalid --robots JSON input: expected a single JSON object")
		}
		return fail(2, "invalid --robots JSON input: %v", err)
	}
	return nil
}

func runSetupRobots(cmd *cobra.Command, opts setupOptions) error {
	var input robotTargetInput
	if err := decodeRobotInput(cmd.InOrStdin(), &input); err != nil {
		return err
	}
	if input.Target != "" {
		opts.target = input.Target
	}
	opts.jsonOutput = true
	return runSetup(scaffold.ModeCheck, opts)
}

func runInitRobots(cmd *cobra.Command, opts initOptions) error {
	var input robotScaffoldInput
	if err := decodeRobotInput(cmd.InOrStdin(), &input); err != nil {
		return err
	}
	applyRobotScaffoldInputToInit(&opts, input)
	opts.jsonOutput = true
	if input.Confirm {
		opts.force = true
	}
	return runInit(opts)
}

func runRepairRobots(cmd *cobra.Command, opts repairOptions) error {
	var input robotScaffoldInput
	if err := decodeRobotInput(cmd.InOrStdin(), &input); err != nil {
		return err
	}
	applyRobotScaffoldInputToRepair(&opts, input)
	opts.jsonOutput = true
	if input.Confirm {
		opts.force = true
	}
	return runRepair(opts)
}

func applyRobotScaffoldInputToInit(opts *initOptions, input robotScaffoldInput) {
	if input.Target != "" {
		opts.target = input.Target
	}
	if len(input.Targets) > 0 {
		opts.targets = append([]string(nil), input.Targets...)
	}
	if input.Dogfood != nil {
		opts.dogfood = *input.Dogfood
		opts.noDogfood = !*input.Dogfood
	}
	if input.ClaudeRoute != "" {
		opts.claudeRoute = input.ClaudeRoute
	}
	skip := input.Skip
	opts.noAgents = opts.noAgents || skip.Agents || skip.AgentsMD
	opts.noAgentsMD = opts.noAgentsMD || skip.AgentsMD
	opts.noAttestations = opts.noAttestations || skip.Attestations
	opts.noBackpressure = opts.noBackpressure || skip.Backpressure
	opts.noBeads = opts.noBeads || skip.Beads
	opts.noBin = opts.noBin || skip.Bin
	opts.noClaude = opts.noClaude || skip.Claude || skip.ClaudeSymlink
	opts.noClaudeSymlink = opts.noClaudeSymlink || skip.ClaudeSymlink
	opts.noDocs = opts.noDocs || skip.Docs
	opts.noGitHooks = opts.noGitHooks || skip.GitHooks
	opts.noHooks = opts.noHooks || skip.Hooks
	opts.noHooksPath = opts.noHooksPath || skip.HooksPath
	opts.gitInit = opts.gitInit || input.GitInit
	opts.noLog = opts.noLog || skip.Log
	opts.noNTM = opts.noNTM || skip.NTM
	opts.noPlans = opts.noPlans || skip.Plans
	opts.noPreCommit = opts.noPreCommit || skip.PreCommit
	opts.noToolDocs = opts.noToolDocs || skip.ToolDocs
}

func applyRobotScaffoldInputToRepair(opts *repairOptions, input robotScaffoldInput) {
	if input.Target != "" {
		opts.target = input.Target
	}
	if len(input.Targets) > 0 {
		opts.targets = append([]string(nil), input.Targets...)
	}
	if input.ClaudeRoute != "" {
		opts.claudeRoute = input.ClaudeRoute
	}
	opts.adoptClaudeMD = opts.adoptClaudeMD || input.AdoptClaudeMD
	skip := input.Skip
	opts.noAgents = opts.noAgents || skip.Agents || skip.AgentsMD
	opts.noAgentsMD = opts.noAgentsMD || skip.AgentsMD
	opts.noAttestations = opts.noAttestations || skip.Attestations
	opts.noBackpressure = opts.noBackpressure || skip.Backpressure
	opts.noBeads = opts.noBeads || skip.Beads
	opts.noBin = opts.noBin || skip.Bin
	opts.noClaude = opts.noClaude || skip.Claude || skip.ClaudeSymlink
	opts.noClaudeSymlink = opts.noClaudeSymlink || skip.ClaudeSymlink
	opts.noDocs = opts.noDocs || skip.Docs
	opts.noGitHooks = opts.noGitHooks || skip.GitHooks
	opts.noHooks = opts.noHooks || skip.Hooks
	opts.noHooksPath = opts.noHooksPath || skip.HooksPath
	opts.gitInit = opts.gitInit || input.GitInit
	opts.noLog = opts.noLog || skip.Log
	opts.noPlans = opts.noPlans || skip.Plans
	opts.noPreCommit = opts.noPreCommit || skip.PreCommit
	opts.noToolDocs = opts.noToolDocs || skip.ToolDocs
}

func validateInitClaudeRoute(route string) (string, error) {
	route = strings.TrimSpace(route)
	switch route {
	case "", scaffold.ClaudeRouteAgentSymlink, scaffold.ClaudeRouteOrchestratorSkill, scaffold.ClaudeRouteNone:
		return route, nil
	default:
		return "", fmt.Errorf("invalid --claude-route %q; expected agent-symlink, orchestrator-skill, or none", route)
	}
}

func validateRepairClaudeRoute(route string) (string, error) {
	route = strings.TrimSpace(route)
	switch route {
	case "", scaffold.ClaudeRepairRoutePreserve, scaffold.ClaudeRouteAgentSymlink, scaffold.ClaudeRouteOrchestratorSkill, scaffold.ClaudeRouteNone:
		return route, nil
	default:
		return "", fmt.Errorf("invalid --claude-route %q; expected preserve, agent-symlink, orchestrator-skill, or none", route)
	}
}

func resolveSkippedClaudeRoute(route string, explicit bool, skipClaude bool) (string, error) {
	if !skipClaude {
		return route, nil
	}
	switch route {
	case "", scaffold.ClaudeRouteNone:
		return scaffold.ClaudeRouteNone, nil
	case scaffold.ClaudeRepairRoutePreserve:
		if !explicit {
			return scaffold.ClaudeRouteNone, nil
		}
	}
	if explicit {
		return "", fmt.Errorf("--no-claude conflicts with --claude-route=%s; omit one of them", route)
	}
	return scaffold.ClaudeRouteNone, nil
}

func runCommitRobots(cmd *cobra.Command, opts commitOptions) error {
	var input robotCommitInput
	if err := decodeRobotInput(cmd.InOrStdin(), &input); err != nil {
		return err
	}
	if input.Root != "" {
		opts.root = input.Root
	}
	if input.Feature != "" {
		opts.feature = input.Feature
	}
	if len(input.Beads) > 0 {
		opts.beads = append([]string(nil), input.Beads...)
	}
	if input.BeadRationale != "" {
		opts.beadRationale = input.BeadRationale
	}
	if input.ResponsesPath != "" {
		opts.responses = input.ResponsesPath
	} else if input.Responses != "" {
		opts.responses = input.Responses
	}
	if input.ResponsesTemplate {
		opts.responsesTemplate = true
	}
	if input.Agent != "" {
		opts.agent = input.Agent
	}
	if input.Model != "" {
		opts.model = input.Model
	}
	return runPreCommit(opts)
}

func runLintRobots(cmd *cobra.Command, root string, jobs int) error {
	var input robotLintInput
	if err := decodeRobotInput(cmd.InOrStdin(), &input); err != nil {
		return err
	}
	if input.Root != "" {
		root = input.Root
	}
	if input.Jobs != 0 {
		jobs = input.Jobs
	}
	return runLint(root, true, jobs)
}

func runLintInitRobots(cmd *cobra.Command, opts lintInitOptions) error {
	var input robotLintInitInput
	if err := decodeRobotInput(cmd.InOrStdin(), &input); err != nil {
		return err
	}
	if input.Root != "" {
		opts.root = input.Root
	}
	opts.detect = opts.detect || input.Detect
	opts.write = opts.write || input.Write
	opts.force = opts.force || input.Force
	if input.Preset != "" {
		opts.preset = input.Preset
	}
	if input.Jobs != 0 {
		opts.jobs = input.Jobs
	}
	opts.robotConfirm = input.Confirm
	opts.jsonOutput = true
	return runLintInit(opts)
}

func runCIRobots(cmd *cobra.Command, opts ciOptions) error {
	var input robotCIInput
	if err := decodeRobotInput(cmd.InOrStdin(), &input); err != nil {
		return err
	}
	if input.Root != "" {
		opts.root = input.Root
	}
	if input.Feature != "" {
		opts.feature = input.Feature
	}
	if input.Commit != "" {
		opts.commit = input.Commit
	}
	return runCI(opts)
}

func runCompletionRobots(cmd *cobra.Command, _ *cobra.Command, args []string) error {
	var input robotCompletionInput
	if err := decodeRobotInput(cmd.InOrStdin(), &input); err != nil {
		return err
	}
	target := input.Target
	if target == "" {
		target = "."
	}
	effectiveConfig, err := bvconfig.Load(target)
	if err != nil {
		return fail(2, "%v", err)
	}
	selection := completionShellSelection{}
	if input.Shell != "" {
		normalized, ok := normalizeCompletionShell(input.Shell)
		if !ok {
			return fail(2, "unknown shell %q; expected bash, zsh, fish, or powershell", input.Shell)
		}
		selection = completionShellSelection{Shell: normalized, Source: "input"}
	}
	if len(args) == 1 {
		normalized, ok := normalizeCompletionShell(args[0])
		if !ok {
			return fail(2, "unknown shell %q; expected bash, zsh, fish, or powershell", args[0])
		}
		if selection.Shell != "" && selection.Shell != normalized {
			return fail(2, "completion shell mismatch: arg is %q but --robots JSON shell is %q", normalized, selection.Shell)
		}
		selection = completionShellSelection{Shell: normalized, Source: "argument"}
	}
	if selection.Shell == "" {
		selection, err = configuredCompletionShellSelection(effectiveConfig)
		if err != nil {
			return err
		}
	}
	output, err := completionRobotOutput(target, selection, effectiveConfig)
	if err != nil {
		return err
	}
	return encodeJSON(cmd.OutOrStdout(), output, "encode completion robot output")
}

func completionRobotOutput(target string, selection completionShellSelection, effective bvconfig.Effective) (robotCompletionOutput, error) {
	path, err := defaultCompletionPath(selection.Shell)
	if err != nil {
		return robotCompletionOutput{}, err
	}
	if effective.File.Defaults.Completion.Path != "" {
		path = expandUserPath(effective.File.Defaults.Completion.Path)
	}
	rcFile, hasRC := defaultCompletionRCFile(selection.Shell)
	if effective.File.Defaults.Completion.RCFile != "" {
		rcFile = expandUserPath(effective.File.Defaults.Completion.RCFile)
		hasRC = true
	}
	if effective.File.Defaults.Completion.UpdateRC != nil && !*effective.File.Defaults.Completion.UpdateRC {
		hasRC = false
	}
	install := fmt.Sprintf("burpvalve completion install --shell %s --path %s --force", selection.Shell, shellQuote(path))
	if hasRC {
		install = fmt.Sprintf("burpvalve completion install --shell %s --path %s --rc-file %s --update-rc --force", selection.Shell, shellQuote(path), shellQuote(rcFile))
	}
	nextSteps := []string{
		"Run install_command in an interactive shell or rerun completion install with --force when the user has approved the write.",
		"Use script_command only when a shell completion manager needs the raw completion script.",
	}
	return robotCompletionOutput{
		SchemaVersion:  1,
		Command:        "completion",
		Target:         target,
		Shell:          selection.Shell,
		ShellSource:    selection.Source,
		Config:         setupConfigSummary(effective),
		CompletionPath: path,
		RCFile:         rcFile,
		InstallCommand: install,
		ScriptCommand:  fmt.Sprintf("burpvalve completion %s > %s", selection.Shell, shellQuote(path)),
		NextSteps:      nextSteps,
	}, nil
}

func runLegacyMode(opts legacyOptions) error {
	switch opts.mode {
	case string(scaffold.ModeCheck), string(scaffold.ModeInit), string(scaffold.ModeRepair):
		setup := setupOptions{target: opts.target, jsonOutput: opts.jsonOutput}
		if opts.mode == string(scaffold.ModeInit) {
			return runInit(initOptions{
				target:          opts.target,
				jsonOutput:      opts.jsonOutput,
				force:           true,
				noBeads:         opts.noBeads,
				noNTM:           opts.noNTM,
				noClaude:        opts.noClaude,
				noClaudeSymlink: opts.noClaudeSymlink,
				noAgents:        opts.noAgents,
				noAgentsMD:      opts.noAgentsMD,
			})
		}
		if opts.mode == string(scaffold.ModeRepair) {
			return runRepair(repairOptions{target: opts.target, jsonOutput: opts.jsonOutput, force: true})
		}
		return runSetup(scaffold.ModeCheck, setup)
	case "pre-commit":
		return runPreCommit(commitOptions{root: opts.root, feature: opts.feature, responses: opts.responses, agent: opts.agent, model: opts.model})
	case "lint":
		return runLint(opts.root, opts.jsonOutput, 1)
	case "ci":
		return runCI(ciOptions{root: opts.root, feature: opts.feature})
	default:
		return fail(2, "unknown mode %q; expected check, init, repair, pre-commit, lint, or ci", opts.mode)
	}
}

func runSetup(mode scaffold.Mode, opts setupOptions) error {
	if mode != scaffold.ModeCheck {
		return fail(2, "unknown setup mode %q; expected check", mode)
	}
	effectiveConfig, err := bvconfig.Load(opts.target)
	if err != nil {
		return fail(2, "%v", err)
	}
	inspectOpts := inspectOptionsFromConfig(effectiveConfig.File.Defaults.Init)
	if opts.noBeads {
		inspectOpts.SkipBeads = true
	}
	if opts.noNTM {
		inspectOpts.SkipNTM = true
	}
	report, err := scaffold.Inspect(opts.target, inspectOpts)
	if err != nil {
		return fail(1, "inspect %s: %v", opts.target, err)
	}
	if report.ClaudeRoute != nil {
		report.ClaudeRoute.Source = claudeRouteSource(false, false, effectiveConfig, "defaults.init.claude_route")
	}
	report.Config = setupConfigSummary(effectiveConfig)
	if opts.jsonOutput {
		return encodeJSON(os.Stdout, report, "encode report")
	}
	fmt.Print(report.TextWithOptions(scaffold.TextOptions{Color: shouldColor(os.Stdout)}))
	return nil
}

func setupConfigSummary(effective bvconfig.Effective) *scaffold.ConfigSummary {
	sources := bvconfig.SortedSources(effective.Sources)
	summary := &scaffold.ConfigSummary{
		GlobalPath:   effective.GlobalPath,
		GlobalFound:  effective.GlobalFound,
		ProjectPath:  effective.ProjectPath,
		ProjectFound: effective.ProjectFound,
		Sources:      make([]scaffold.ConfigSource, 0, len(sources)),
		Settings:     make([]scaffold.ConfigSetting, 0, len(sources)),
	}
	for _, source := range sources {
		summary.Sources = append(summary.Sources, scaffold.ConfigSource{Key: source.Key, Source: source.Source})
		summary.Settings = append(summary.Settings, scaffold.ConfigSetting{Key: source.Key, Source: source.Source, Value: configSettingValue(effective.File.Defaults, source.Key)})
	}
	return summary
}

func configSettingValue(defaults bvconfig.Defaults, key string) string {
	switch key {
	case "defaults.skills_dir":
		return defaults.SkillsDir
	case "defaults.bin_dir":
		return defaults.BinDir
	case "defaults.shell":
		return defaults.Shell
	case "defaults.color":
		return defaults.Color
	case "defaults.confirm":
		return configBoolValue(defaults.Confirm)
	case "defaults.completion.path":
		return defaults.Completion.Path
	case "defaults.completion.rc_file":
		return defaults.Completion.RCFile
	case "defaults.completion.update_rc":
		return configBoolValue(defaults.Completion.UpdateRC)
	case "defaults.init.agents":
		return configBoolValue(defaults.Init.Agents)
	case "defaults.init.claude":
		return configBoolValue(defaults.Init.Claude)
	case "defaults.init.claude_route":
		return defaults.Init.ClaudeRoute
	case "defaults.init.docs":
		return configBoolValue(defaults.Init.Docs)
	case "defaults.init.plans":
		return configBoolValue(defaults.Init.Plans)
	case "defaults.init.log":
		return configBoolValue(defaults.Init.Log)
	case "defaults.init.backpressure":
		return configBoolValue(defaults.Init.Backpressure)
	case "defaults.init.attestations":
		return configBoolValue(defaults.Init.Attestations)
	case "defaults.init.precommit":
		return configBoolValue(defaults.Init.PreCommit)
	case "defaults.init.hooks_path":
		return configBoolValue(defaults.Init.HooksPath)
	case "defaults.init.repo_bin":
		return configBoolValue(defaults.Init.RepoBin)
	case "defaults.init.tool_docs":
		return configBoolValue(defaults.Init.ToolDocs)
	case "defaults.init.beads":
		return configBoolValue(defaults.Init.Beads)
	case "defaults.init.ntm":
		return configBoolValue(defaults.Init.NTM)
	case "defaults.init.orchestrator":
		if defaults.Init.Orchestrator == "" {
			return "off"
		}
		return defaults.Init.Orchestrator
	case "defaults.init.dogfood":
		return configBoolValue(defaults.Init.Dogfood)
	case "defaults.repair.agents":
		return configBoolValue(defaults.Repair.Agents)
	case "defaults.repair.claude":
		return configBoolValue(defaults.Repair.Claude)
	case "defaults.repair.claude_route":
		return defaults.Repair.ClaudeRoute
	case "defaults.repair.docs":
		return configBoolValue(defaults.Repair.Docs)
	case "defaults.repair.plans":
		return configBoolValue(defaults.Repair.Plans)
	case "defaults.repair.log":
		return configBoolValue(defaults.Repair.Log)
	case "defaults.repair.backpressure":
		return configBoolValue(defaults.Repair.Backpressure)
	case "defaults.repair.attestations":
		return configBoolValue(defaults.Repair.Attestations)
	case "defaults.repair.precommit":
		return configBoolValue(defaults.Repair.PreCommit)
	case "defaults.repair.hooks_path":
		return configBoolValue(defaults.Repair.HooksPath)
	case "defaults.repair.repo_bin":
		return configBoolValue(defaults.Repair.RepoBin)
	case "defaults.repair.tool_docs":
		return configBoolValue(defaults.Repair.ToolDocs)
	case "defaults.repair.beads":
		return configBoolValue(defaults.Repair.Beads)
	case "defaults.repair.ntm":
		return configBoolValue(defaults.Repair.NTM)
	case "defaults.repair.orchestrator":
		return configBoolValue(defaults.Repair.Orchestrator)
	case "defaults.verifier.authorized":
		return configBoolValue(defaults.Verifier.Authorized)
	case "defaults.verifier.authorized_at":
		return defaults.Verifier.AuthorizedAt
	case "defaults.verifier.authorization_scope":
		return defaults.Verifier.AuthorizationScope
	case "defaults.verifier.spawn_method":
		return defaults.Verifier.SpawnMethod
	case "defaults.verifier.default_model":
		return defaults.Verifier.DefaultModel
	case "defaults.verifier.read_only_tools":
		return configBoolValue(defaults.Verifier.ReadOnlyTools)
	case "defaults.verifier.max_parallel_verifiers":
		return configIntValue(defaults.Verifier.MaxParallelVerifiers)
	case "defaults.verifier.transcript_dir":
		return defaults.Verifier.TranscriptDir
	case "defaults.verifier.transcripts":
		return defaults.Verifier.Transcripts
	default:
		if strings.HasPrefix(key, "defaults.verifier.condition_models.") {
			return defaults.Verifier.ConditionModels[strings.TrimPrefix(key, "defaults.verifier.condition_models.")]
		}
		return ""
	}
}

func configBoolValue(value *bool) string {
	if value == nil {
		return ""
	}
	return strconv.FormatBool(*value)
}

func configIntValue(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

func inspectOptionsFromConfig(defaults bvconfig.InitDefaults) scaffold.InspectOptions {
	return scaffold.InspectOptions{
		SkipAgents:          boolDefaultOff(defaults.Agents),
		SkipClaude:          boolDefaultOff(defaults.Claude),
		SkipDocs:            boolDefaultOff(defaults.Docs),
		SkipPlans:           boolDefaultOff(defaults.Plans),
		SkipLog:             boolDefaultOff(defaults.Log),
		SkipBackpressure:    boolDefaultOff(defaults.Backpressure),
		SkipAttestations:    boolDefaultOff(defaults.Attestations),
		SkipBeads:           boolDefaultOff(defaults.Beads),
		SkipPreCommit:       boolDefaultOff(defaults.PreCommit),
		SkipHooksPath:       boolDefaultOff(defaults.HooksPath),
		RequireOrchestrator: defaults.Orchestrator == "orchestrator-md",
		ClaudeRoute:         defaults.ClaudeRoute,
		RequireRepoBin:      bvconfig.BoolValue(defaults.RepoBin, false),
		SkipNTM:             boolDefaultOff(defaults.NTM),
	}
}

func confirmScaffoldMutation(command string, target string, targets []scaffold.ScaffoldTarget, claudeRoute string, force bool, jsonOutput bool) error {
	if force {
		return nil
	}
	if jsonOutput {
		return fail(2, "%s --json will not change files without confirmation; rerun with: burpvalve %s --force --json", command, command)
	}
	if !isInteractiveTerminal(os.Stdin, os.Stdout) {
		return fail(2, "%s requires confirmation before changing files; run in a terminal or rerun with: burpvalve %s --force", command, command)
	}
	confirmed, err := charmui.AskConfirm(os.Stdin, os.Stdout, charmui.ConfirmPrompt{
		Title:       "Confirm Burpvalve " + command,
		Description: confirmScaffoldDescription(target, targets, claudeRoute),
		Prompt:      "Apply these changes?",
		Default:     false,
		Color:       shouldColor(os.Stdout),
	})
	if err != nil {
		if errors.Is(err, charmui.ErrCancelled) {
			return fail(2, "%s cancelled; no files changed", command)
		}
		return fail(2, "%s confirmation failed: %v", command, err)
	}
	if !confirmed {
		return fail(2, "%s cancelled; no files changed", command)
	}
	return nil
}

func confirmScaffoldDescription(target string, targets []scaffold.ScaffoldTarget, claudeRoute string) string {
	parts := []string{
		"Target: " + target,
		"Pieces: " + describeScaffoldTargets(targets),
	}
	if strings.TrimSpace(claudeRoute) != "" {
		parts = append(parts, "Claude route: "+describeClaudeRouteForConfirm(claudeRoute))
	}
	parts = append(parts, "Default is No.")
	return strings.Join(parts, "\n")
}

func describeClaudeRouteForConfirm(route string) string {
	switch strings.TrimSpace(route) {
	case scaffold.ClaudeRouteAgentSymlink:
		return "ordinary agent via CLAUDE.md symlink"
	case scaffold.ClaudeRouteOrchestratorSkill:
		return "orchestrator skill via generated CLAUDE.md and .claude/skills/burpvalve-orchestrator/"
	case scaffold.ClaudeRouteNone:
		return "none"
	case scaffold.ClaudeRepairRoutePreserve:
		return "preserve existing route"
	default:
		return route
	}
}

func claudeRouteSource(explicit bool, prompt bool, effective bvconfig.Effective, key string) string {
	if explicit {
		return "input"
	}
	if prompt {
		return "prompt"
	}
	if source := strings.TrimSpace(effective.Sources[key]); source != "" {
		return source
	}
	return "default"
}

func describeScaffoldTargets(targets []scaffold.ScaffoldTarget) string {
	if len(targets) == 0 {
		return "standard Burpvalve scaffold"
	}
	names := make([]string, 0, len(targets))
	for _, target := range targets {
		names = append(names, string(target))
	}
	return strings.Join(names, ", ")
}

func maybeAskGitInit(command, target string, force bool, gitInit bool, noHooks bool, noPreCommit bool, noHooksPath bool, set func(bool)) error {
	if !needsGitInitPrompt(target, force, gitInit, noHooks, noPreCommit, noHooksPath) {
		return nil
	}
	confirmed, err := charmui.AskConfirm(os.Stdin, os.Stdout, charmui.ConfirmPrompt{
		Title:       "Initialize Git for Burpvalve hooks",
		Description: "This target is not a Git repo, but hook wiring needs .git before Burpvalve can configure core.hooksPath.",
		Prompt:      "Run git init before applying hook wiring?",
		Default:     false,
		Color:       shouldColor(os.Stdout),
	})
	if err != nil {
		if errors.Is(err, charmui.ErrCancelled) {
			return fail(2, "%s cancelled; no files changed", command)
		}
		return fail(2, "%s git init prompt failed: %v", command, err)
	}
	set(confirmed)
	return nil
}

func needsGitInitPrompt(target string, force bool, gitInit bool, noHooks bool, noPreCommit bool, noHooksPath bool) bool {
	if force || gitInit || noHooks || (noPreCommit && noHooksPath) {
		return false
	}
	if _, err := os.Stat(filepath.Join(target, ".git")); err == nil {
		return false
	}
	return true
}

func configuredInitDefaults(target string, defaults bvconfig.InitDefaults) charmui.InitWizardResult {
	result := charmui.DefaultInitWizardResult(target)
	applyScaffoldDefaultsToWizardResult(&result, defaults.ScaffoldDefaults)
	result.ClaudeRoute = initWizardClaudeRoute(defaults.ClaudeRoute, result.Claude)
	if result.ClaudeRoute == scaffold.ClaudeRouteNone {
		result.Claude = true
	}
	result.Orchestrator = defaults.Orchestrator == "orchestrator-md"
	return result
}

func configuredRepairDefaults(target string, defaults bvconfig.RepairDefaults) charmui.InitWizardResult {
	result := charmui.DefaultInitWizardResult(target)
	applyScaffoldDefaultsToWizardResult(&result, defaults.ScaffoldDefaults)
	result.ClaudeRoute = repairWizardClaudeRoute(defaults.ClaudeRoute, result.Claude)
	if result.ClaudeRoute == scaffold.ClaudeRouteNone {
		result.Claude = true
	}
	result.NTM = false
	return result
}

func initWizardClaudeRoute(route string, claude bool) string {
	route = strings.TrimSpace(route)
	if !claude {
		return scaffold.ClaudeRouteNone
	}
	switch route {
	case scaffold.ClaudeRouteAgentSymlink, scaffold.ClaudeRouteOrchestratorSkill, scaffold.ClaudeRouteNone:
		return route
	default:
		return scaffold.ClaudeRouteAgentSymlink
	}
}

func repairWizardClaudeRoute(route string, claude bool) string {
	route = strings.TrimSpace(route)
	if !claude {
		return scaffold.ClaudeRouteNone
	}
	switch route {
	case scaffold.ClaudeRouteAgentSymlink, scaffold.ClaudeRouteOrchestratorSkill, scaffold.ClaudeRouteNone:
		return route
	default:
		return scaffold.ClaudeRouteAgentSymlink
	}
}

func applyScaffoldDefaultsToWizardResult(result *charmui.InitWizardResult, defaults bvconfig.ScaffoldDefaults) {
	result.Agents = bvconfig.BoolValue(defaults.Agents, result.Agents)
	result.Claude = bvconfig.BoolValue(defaults.Claude, result.Claude)
	result.Docs = bvconfig.BoolValue(defaults.Docs, result.Docs)
	result.Plans = bvconfig.BoolValue(defaults.Plans, result.Plans)
	result.Log = bvconfig.BoolValue(defaults.Log, result.Log)
	result.Backpressure = bvconfig.BoolValue(defaults.Backpressure, result.Backpressure)
	result.Attestations = bvconfig.BoolValue(defaults.Attestations, result.Attestations)
	result.Hooks = bvconfig.BoolValue(defaults.PreCommit, result.Hooks)
	result.HooksPath = bvconfig.BoolValue(defaults.HooksPath, result.HooksPath)
	result.Bin = bvconfig.BoolValue(defaults.RepoBin, result.Bin)
	result.ToolDocs = bvconfig.BoolValue(defaults.ToolDocs, result.ToolDocs)
	result.Beads = bvconfig.BoolValue(defaults.Beads, result.Beads)
	result.NTM = bvconfig.BoolValue(defaults.NTM, result.NTM)
}

func applyScaffoldDefaultsToInitOptions(opts *initOptions, defaults bvconfig.InitDefaults) {
	opts.noAgents = opts.noAgents || boolDefaultOff(defaults.Agents)
	opts.noClaude = opts.noClaude || boolDefaultOff(defaults.Claude)
	opts.noDocs = opts.noDocs || boolDefaultOff(defaults.Docs)
	opts.noPlans = opts.noPlans || boolDefaultOff(defaults.Plans)
	opts.noLog = opts.noLog || boolDefaultOff(defaults.Log)
	opts.noBackpressure = opts.noBackpressure || boolDefaultOff(defaults.Backpressure)
	opts.noAttestations = opts.noAttestations || boolDefaultOff(defaults.Attestations)
	opts.noPreCommit = opts.noPreCommit || boolDefaultOff(defaults.PreCommit)
	opts.noHooksPath = opts.noHooksPath || boolDefaultOff(defaults.HooksPath)
	opts.noToolDocs = opts.noToolDocs || boolDefaultOff(defaults.ToolDocs)
	opts.noBeads = opts.noBeads || boolDefaultOff(defaults.Beads)
	opts.noNTM = opts.noNTM || boolDefaultOff(defaults.NTM)
	if opts.claudeRoute == "" {
		opts.claudeRoute = defaults.ClaudeRoute
	}
	if defaults.RepoBin != nil && !opts.repoBin {
		opts.repoBin = *defaults.RepoBin
	}
	if opts.noBin {
		opts.repoBin = false
	}
}

func applyScaffoldDefaultsToRepairOptions(opts *repairOptions, defaults bvconfig.RepairDefaults) {
	opts.noAgents = opts.noAgents || boolDefaultOff(defaults.Agents)
	opts.noClaude = opts.noClaude || boolDefaultOff(defaults.Claude)
	opts.noDocs = opts.noDocs || boolDefaultOff(defaults.Docs)
	opts.noPlans = opts.noPlans || boolDefaultOff(defaults.Plans)
	opts.noLog = opts.noLog || boolDefaultOff(defaults.Log)
	opts.noBackpressure = opts.noBackpressure || boolDefaultOff(defaults.Backpressure)
	opts.noAttestations = opts.noAttestations || boolDefaultOff(defaults.Attestations)
	opts.noPreCommit = opts.noPreCommit || boolDefaultOff(defaults.PreCommit)
	opts.noHooksPath = opts.noHooksPath || boolDefaultOff(defaults.HooksPath)
	opts.noToolDocs = opts.noToolDocs || boolDefaultOff(defaults.ToolDocs)
	opts.noBeads = opts.noBeads || boolDefaultOff(defaults.Beads)
	if opts.claudeRoute == "" {
		opts.claudeRoute = defaults.ClaudeRoute
	}
	if defaults.RepoBin != nil && !opts.repoBin {
		opts.repoBin = *defaults.RepoBin
	}
	if opts.noBin {
		opts.repoBin = false
	}
}

func boolDefaultOff(value *bool) bool {
	return value != nil && !*value
}

func scaffoldTargetNamesWithOptions(names []string, repoBin bool, noBin bool, orchestrator bool) []string {
	if !repoBin || noBin {
		if !orchestrator {
			return names
		}
		next := append([]string(nil), names...)
		if len(next) == 0 {
			next = append(next, "standard")
		}
		return append(next, "orchestrator")
	}
	next := append([]string(nil), names...)
	if len(next) == 0 {
		next = append(next, "standard")
	}
	next = append(next, "bin")
	if orchestrator {
		next = append(next, "orchestrator")
	}
	return next
}

func runInit(opts initOptions) error {
	effectiveConfig, err := bvconfig.Load(opts.target)
	if err != nil {
		return fail(2, "%v", err)
	}
	if opts.dogfood && opts.noDogfood {
		return fail(2, "--dogfood and --no-dogfood cannot be combined")
	}
	defaults := configuredInitDefaults(opts.target, effectiveConfig.File.Defaults.Init)
	if opts.repoBin {
		defaults.Bin = true
	}
	dogfood := bvconfig.BoolValue(effectiveConfig.File.Defaults.Init.Dogfood, false)
	if opts.dogfood {
		dogfood = true
	}
	if opts.noDogfood {
		dogfood = false
	}
	ranWizard := shouldRunInitWizard(opts)
	explicitClaudeRoute := strings.TrimSpace(opts.claudeRoute) != ""
	if ranWizard {
		result, err := charmui.RunInitWizard(os.Stdin, os.Stdout, charmui.InitWizardOptions{Target: opts.target, Color: shouldColor(os.Stdout), Skip: initWizardSkips(opts), Defaults: &defaults})
		if err != nil {
			if errors.Is(err, charmui.ErrCancelled) {
				return fail(2, "init cancelled")
			}
			return fail(2, "init prompt failed: %v", err)
		}
		applyInitWizardResult(&opts, result)
	} else {
		if len(opts.targets) == 0 {
			applyScaffoldDefaultsToInitOptions(&opts, effectiveConfig.File.Defaults.Init)
			opts.orchestrator = effectiveConfig.File.Defaults.Init.Orchestrator == "orchestrator-md"
		}
	}
	if ranWizard {
		if err := maybeAskGitInit("init", opts.target, opts.force, opts.gitInit, opts.noHooks || opts.noGitHooks, opts.noPreCommit, opts.noHooksPath, func(v bool) { opts.gitInit = v }); err != nil {
			return err
		}
	}
	targetNames := scaffoldTargetNamesWithOptions(opts.targets, opts.repoBin, opts.noBin, opts.orchestrator)
	targets, err := scaffold.NormalizeScaffoldTargets(targetNames)
	if err != nil {
		return fail(2, "%v", err)
	}
	claudeRoute, err := validateInitClaudeRoute(opts.claudeRoute)
	if err != nil {
		return fail(2, "%v", err)
	}
	claudeRoute, err = resolveSkippedClaudeRoute(claudeRoute, explicitClaudeRoute, opts.noClaude || opts.noClaudeSymlink)
	if err != nil {
		return fail(2, "%v", err)
	}
	if err := confirmScaffoldMutation("init", opts.target, targets, claudeRoute, opts.force, opts.jsonOutput); err != nil {
		return err
	}
	if ranWizard {
		if err := maybeAskInitVerifierAuthorization(opts.target, effectiveConfig); err != nil {
			return err
		}
		effectiveConfig, err = bvconfig.Load(opts.target)
		if err != nil {
			return fail(2, "%v", err)
		}
	}
	result, err := scaffold.ApplyInitWithOptions(opts.target, scaffold.ApplyOptions{
		Targets:            targets,
		ClaudeRoute:        claudeRoute,
		SkipBeads:          opts.noBeads,
		SkipNTM:            opts.noNTM,
		SkipClaude:         opts.noClaude || opts.noClaudeSymlink,
		SkipAgents:         opts.noAgents || opts.noAgentsMD,
		SkipDocs:           opts.noDocs,
		SkipPlans:          opts.noPlans,
		SkipLog:            opts.noLog,
		SkipBackpressure:   opts.noBackpressure,
		SkipAttestations:   opts.noAttestations,
		SkipPreCommit:      opts.noPreCommit || opts.noHooks || opts.noGitHooks,
		SkipHooksPath:      opts.noHooksPath || opts.noHooks || opts.noGitHooks,
		SkipTool:           opts.noBin,
		SkipToolDocs:       opts.noToolDocs,
		VerifierConfigured: verifierDefaultsConfigured(effectiveConfig.File.Defaults.Verifier),
		Dogfood:            dogfood,
		GitInit:            opts.gitInit,
	})
	result.ClaudeRouteSource = claudeRouteSource(explicitClaudeRoute, ranWizard, effectiveConfig, "defaults.init.claude_route")
	result.Config = setupConfigSummary(effectiveConfig)
	if opts.jsonOutput {
		if encodeErr := encodeJSON(os.Stdout, result, "encode result"); encodeErr != nil {
			return encodeErr
		}
	} else {
		fmt.Print(result.TextWithOptions(scaffold.TextOptions{Color: shouldColor(os.Stdout)}))
	}
	if err != nil {
		return fail(1, "init %s: %v", opts.target, err)
	}
	return nil
}

func verifierDefaultsConfigured(defaults bvconfig.VerifierDefaults) bool {
	return defaults.Authorized != nil ||
		defaults.AuthorizedAt != "" ||
		defaults.AuthorizationScope != "" ||
		defaults.SpawnMethod != "" ||
		defaults.DefaultModel != "" ||
		len(defaults.ConditionModels) > 0 ||
		defaults.ReadOnlyTools != nil ||
		defaults.MaxParallelVerifiers != nil ||
		defaults.TranscriptDir != "" ||
		defaults.Transcripts != ""
}

func maybeAskInitVerifierAuthorization(target string, effective bvconfig.Effective) error {
	if _, ok := effective.Sources["defaults.verifier.authorized"]; ok {
		return nil
	}
	authorized, err := charmui.AskConfirm(os.Stdin, os.Stdout, charmui.ConfirmPrompt{
		Title:       "Verifier authorization",
		Description: "This records standing policy for future backpressure checks. It is never verifier evidence for any condition cell.",
		Prompt:      "Are agents in this repo authorized to spawn read-only verifier subagents for backpressure checks?",
		Default:     false,
		Color:       shouldColor(os.Stdout),
	})
	if err != nil {
		if errors.Is(err, charmui.ErrCancelled) {
			return fail(2, "init cancelled; no files changed")
		}
		return fail(2, "init verifier authorization prompt failed: %v", err)
	}
	return recordProjectVerifierAuthorization(target, authorized)
}

func recordProjectVerifierAuthorization(target string, authorized bool) error {
	path, err := bvconfig.ProjectPath(target)
	if err != nil {
		return fail(2, "%v", err)
	}
	existing, err := readConfigFileIfExists(path)
	if err != nil {
		return fail(2, "%v", err)
	}
	scope, err := filepath.Abs(target)
	if err != nil {
		scope = target
	}
	update := bvconfig.File{
		SchemaVersion: bvconfig.SchemaVersion,
		Defaults: bvconfig.Defaults{
			Verifier: bvconfig.VerifierDefaults{
				Authorized:         boolPtr(authorized),
				AuthorizedAt:       time.Now().UTC().Format(time.RFC3339),
				AuthorizationScope: "repo:" + scope,
				SpawnMethod:        "manual",
				Transcripts:        "summary",
			},
		},
	}
	if err := bvconfig.Write(path, bvconfig.Merge(existing, update)); err != nil {
		return fail(2, "write project verifier authorization: %v", err)
	}
	return nil
}

func runRepair(opts repairOptions) error {
	effectiveConfig, err := bvconfig.Load(opts.target)
	if err != nil {
		return fail(2, "%v", err)
	}
	defaults := configuredRepairDefaults(opts.target, effectiveConfig.File.Defaults.Repair)
	if opts.repoBin {
		defaults.Bin = true
	}
	ranWizard := shouldRunRepairWizard(opts)
	explicitClaudeRoute := strings.TrimSpace(opts.claudeRoute) != ""
	if ranWizard {
		result, err := charmui.RunRepairWizard(os.Stdin, os.Stdout, charmui.RepairWizardOptions{Target: opts.target, Color: shouldColor(os.Stdout), Skip: repairWizardSkips(opts), Defaults: &defaults})
		if err != nil {
			if errors.Is(err, charmui.ErrCancelled) {
				return fail(2, "repair cancelled")
			}
			return fail(2, "repair prompt failed: %v", err)
		}
		applyRepairWizardResult(&opts, result)
	} else {
		if len(opts.targets) == 0 {
			applyScaffoldDefaultsToRepairOptions(&opts, effectiveConfig.File.Defaults.Repair)
			opts.orchestrator = bvconfig.BoolValue(effectiveConfig.File.Defaults.Repair.Orchestrator, false)
		}
	}
	if ranWizard {
		if err := maybeAskGitInit("repair", opts.target, opts.force, opts.gitInit, opts.noHooks || opts.noGitHooks, opts.noPreCommit, opts.noHooksPath, func(v bool) { opts.gitInit = v }); err != nil {
			return err
		}
	}
	targetNames := scaffoldTargetNamesWithOptions(opts.targets, opts.repoBin, opts.noBin, opts.orchestrator)
	targets, err := scaffold.NormalizeScaffoldTargets(targetNames)
	if err != nil {
		return fail(2, "%v", err)
	}
	claudeRoute, err := validateRepairClaudeRoute(opts.claudeRoute)
	if err != nil {
		return fail(2, "%v", err)
	}
	claudeRoute, err = resolveSkippedClaudeRoute(claudeRoute, explicitClaudeRoute, opts.noClaude || opts.noClaudeSymlink)
	if err != nil {
		return fail(2, "%v", err)
	}
	if err := confirmScaffoldMutation("repair", opts.target, targets, claudeRoute, opts.force, opts.jsonOutput); err != nil {
		return err
	}
	result, err := scaffold.ApplyRepairWithOptions(opts.target, scaffold.ApplyOptions{
		Targets:            targets,
		ClaudeRoute:        claudeRoute,
		AdoptClaude:        opts.adoptClaudeMD,
		SkipBeads:          opts.noBeads,
		SkipClaude:         opts.noClaude || opts.noClaudeSymlink,
		SkipAgents:         opts.noAgents || opts.noAgentsMD,
		SkipDocs:           opts.noDocs,
		SkipPlans:          opts.noPlans,
		SkipLog:            opts.noLog,
		SkipBackpressure:   opts.noBackpressure,
		SkipAttestations:   opts.noAttestations,
		SkipPreCommit:      opts.noPreCommit || opts.noHooks || opts.noGitHooks,
		SkipHooksPath:      opts.noHooksPath || opts.noHooks || opts.noGitHooks,
		SkipTool:           opts.noBin,
		SkipToolDocs:       opts.noToolDocs,
		VerifierConfigured: verifierDefaultsConfigured(effectiveConfig.File.Defaults.Verifier),
		GitInit:            opts.gitInit,
	})
	result.ClaudeRouteSource = claudeRouteSource(explicitClaudeRoute, ranWizard, effectiveConfig, "defaults.repair.claude_route")
	result.Config = setupConfigSummary(effectiveConfig)
	if opts.jsonOutput {
		if encodeErr := encodeJSON(os.Stdout, result, "encode repair result"); encodeErr != nil {
			return encodeErr
		}
	} else {
		fmt.Print(result.TextWithOptions(scaffold.TextOptions{Color: shouldColor(os.Stdout)}))
	}
	if err != nil {
		return fail(1, "repair %s: %v", opts.target, err)
	}
	return nil
}

func normalizeCommitBeads(ids []string, rationale string) ([]string, string, error) {
	return normalizeBeadIDs(ids, rationale, false)
}

func normalizeCloseBeads(ids []string, rationale string, adminOnly bool) ([]string, string, error) {
	return normalizeBeadIDs(ids, rationale, adminOnly)
}

func normalizeBeadIDs(ids []string, rationale string, allowMultipleWithoutRationale bool) ([]string, string, error) {
	seen := map[string]bool{}
	var clean []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, "", fmt.Errorf("--bead values must not be empty")
		}
		if strings.ContainsAny(id, " \t\r\n") {
			return nil, "", fmt.Errorf("--bead %q must not contain whitespace", id)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		clean = append(clean, id)
	}
	rationale = strings.TrimSpace(rationale)
	if len(clean) > 1 && rationale == "" && !allowMultipleWithoutRationale {
		return nil, "", fmt.Errorf("--bead-rationale is required when multiple --bead values describe one staged payload")
	}
	return clean, rationale, nil
}

func defaultCLIRoot(root string) string {
	if strings.TrimSpace(root) == "" {
		return "."
	}
	return root
}

func runPreCommit(opts commitOptions) error {
	beads, beadRationale, err := normalizeCommitBeads(opts.beads, opts.beadRationale)
	if err != nil {
		return fail(2, "%v", err)
	}
	opts.beads = beads
	opts.beadRationale = beadRationale
	ctx, cancel := backpressure.WithTimeout(context.Background())
	defer cancel()
	if opts.responsesTemplate {
		plan, err := backpressure.BuildPlan(ctx, backpressure.Options{
			Root:            opts.root,
			Mode:            "pre-commit",
			ExplicitFeature: opts.feature,
		})
		if err != nil {
			return fail(2, "build responses template: %v", err)
		}
		return encodeJSON(os.Stdout, backpressure.BuildResponsesTemplate(plan), "encode responses template")
	}
	result, err := backpressure.RunPreCommit(ctx, backpressure.PreCommitOptions{
		Root:            opts.root,
		ExplicitFeature: opts.feature,
		BeadIDs:         opts.beads,
		BeadRationale:   opts.beadRationale,
		ResponsesPath:   opts.responses,
		Agent:           opts.agent,
		Model:           opts.model,
		ColorMode:       colorMode,
	})
	if encodeErr := encodeJSON(os.Stdout, result, "encode commit result"); encodeErr != nil {
		return encodeErr
	}
	for _, warning := range result.Warnings {
		fmt.Fprintln(os.Stderr, cliStyles(os.Stderr).Warn("Warning: "+warning))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, cliStyles(os.Stderr).Error(result.Message))
		return exitCode(2)
	}
	return nil
}

func runCI(opts ciOptions) error {
	ctx, cancel := backpressure.WithTimeout(context.Background())
	defer cancel()
	result, err := backpressure.RunCI(ctx, backpressure.CIOptions{
		Root:            opts.root,
		ExplicitFeature: opts.feature,
		Commit:          opts.commit,
	})
	if encodeErr := encodeJSON(os.Stdout, result, "encode ci result"); encodeErr != nil {
		return encodeErr
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, cliStyles(os.Stderr).Error(result.Message))
		return exitCode(2)
	}
	return nil
}

func runLint(root string, jsonOutput bool, jobs int) error {
	if jobs <= 0 {
		return fail(2, "lint --jobs must be positive")
	}
	result, err := backpressure.RunLint(context.Background(), backpressure.LintOptions{
		Root: root,
		Jobs: jobs,
	})
	if encodeErr := encodeJSON(os.Stdout, result, "encode lint result"); encodeErr != nil {
		return encodeErr
	}
	if !jsonOutput {
		fmt.Fprint(os.Stderr, backpressure.PrintLintSummaryWithOptions(result, backpressure.TextOptions{Color: shouldColor(os.Stderr)}))
	}
	if err != nil {
		return exitCode(2)
	}
	return nil
}

func encodeJSON(out io.Writer, value any, context string) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return fail(1, "%s: %v", context, err)
	}
	return nil
}

func shouldColor(out *os.File) bool {
	return shouldColorWriter(out)
}

func cliStyles(out *os.File) cliui.Styles {
	color := shouldColor(out)
	darkBackground := true
	if color {
		darkBackground = lipgloss.HasDarkBackground(os.Stdin, out)
	}
	return cliui.NewWithDarkBackground(color, darkBackground)
}

func shouldColorWriter(out io.Writer) bool {
	switch colorMode {
	case "always":
		return true
	case "never":
		return false
	}
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func validateColorMode() error {
	switch colorMode {
	case "auto", "always", "never":
		return nil
	default:
		return fail(2, "invalid --color %q; expected auto, always, or never", colorMode)
	}
}

func shouldRunInitWizard(opts initOptions) bool {
	if opts.jsonOutput || opts.force {
		return false
	}
	return isInteractiveTerminal(os.Stdin, os.Stdout)
}

func shouldRunRepairWizard(opts repairOptions) bool {
	if opts.jsonOutput || opts.force {
		return false
	}
	return isInteractiveTerminal(os.Stdin, os.Stdout)
}

func initWizardSkips(opts initOptions) charmui.InitWizardResult {
	noHooks := opts.noHooks || opts.noGitHooks
	return charmui.InitWizardResult{
		Agents:       opts.noAgents || opts.noAgentsMD,
		Claude:       opts.noClaude || opts.noClaudeSymlink,
		Docs:         opts.noDocs,
		Plans:        opts.noPlans,
		Log:          opts.noLog,
		Backpressure: opts.noBackpressure,
		Attestations: opts.noAttestations,
		Hooks:        opts.noPreCommit || noHooks,
		HooksPath:    opts.noHooksPath || noHooks,
		Bin:          opts.noBin,
		ToolDocs:     opts.noToolDocs,
		Beads:        opts.noBeads,
		NTM:          opts.noNTM,
	}
}

func repairWizardSkips(opts repairOptions) charmui.InitWizardResult {
	noHooks := opts.noHooks || opts.noGitHooks
	return charmui.InitWizardResult{
		Agents:       opts.noAgents || opts.noAgentsMD,
		Claude:       opts.noClaude || opts.noClaudeSymlink,
		Docs:         opts.noDocs,
		Plans:        opts.noPlans,
		Log:          opts.noLog,
		Backpressure: opts.noBackpressure,
		Attestations: opts.noAttestations,
		Hooks:        opts.noPreCommit || noHooks,
		HooksPath:    opts.noHooksPath || noHooks,
		Bin:          opts.noBin,
		ToolDocs:     opts.noToolDocs,
		Beads:        opts.noBeads,
	}
}

func applyInitWizardResult(opts *initOptions, result charmui.InitWizardResult) {
	opts.target = result.Target
	opts.noAgents = !result.Agents
	opts.noClaude = !result.Claude
	opts.claudeRoute = result.ClaudeRoute
	opts.noDocs = !result.Docs
	opts.noPlans = !result.Plans
	opts.noLog = !result.Log
	opts.noBackpressure = !result.Backpressure
	opts.noAttestations = !result.Attestations
	opts.noPreCommit = !result.Hooks
	opts.noHooksPath = !result.HooksPath
	opts.noBin = !result.Bin
	opts.noToolDocs = !result.ToolDocs
	opts.noBeads = !result.Beads
	opts.noNTM = !result.NTM
	opts.orchestrator = result.Orchestrator && (result.Beads || result.NTM)
}

func applyRepairWizardResult(opts *repairOptions, result charmui.InitWizardResult) {
	opts.target = result.Target
	opts.noAgents = !result.Agents
	opts.noClaude = !result.Claude
	opts.claudeRoute = result.ClaudeRoute
	opts.noDocs = !result.Docs
	opts.noPlans = !result.Plans
	opts.noLog = !result.Log
	opts.noBackpressure = !result.Backpressure
	opts.noAttestations = !result.Attestations
	opts.noPreCommit = !result.Hooks
	opts.noHooksPath = !result.HooksPath
	opts.noBin = !result.Bin
	opts.noToolDocs = !result.ToolDocs
	opts.noBeads = !result.Beads
}

var isInteractiveTerminal = interactiveTerminal

func interactiveTerminal(in, out *os.File) bool {
	if os.Getenv("NO_TUI") != "" || os.Getenv("CI") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	inInfo, err := in.Stat()
	if err != nil || inInfo.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	outInfo, err := out.Stat()
	if err != nil || outInfo.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	return true
}
