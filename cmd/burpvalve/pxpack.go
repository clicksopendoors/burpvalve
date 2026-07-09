package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type pxpackOptions struct {
	orchestrator     bool
	outDir           string
	packetID         string
	sources          []string
	excludes         []string
	factsheetSources []string
	imageSources     []string
	liveSources      []string
	maxPages         int
	dryRun           bool
	replace          bool
	checkDir         string
	jsonOutput       bool
}

type robotPxpackInput struct {
	Mode             string   `json:"mode"`
	PacketID         string   `json:"packet_id"`
	OutDir           string   `json:"out_dir"`
	Sources          []string `json:"sources"`
	FactsheetSources []string `json:"factsheet_sources"`
	ImageSources     []string `json:"image_sources"`
	LiveSources      []string `json:"live_sources"`
	Excludes         []string `json:"excludes"`
	MaxPages         int      `json:"max_pages"`
	Replace          bool     `json:"replace"`
	DryRun           bool     `json:"dry_run"`
	Check            string   `json:"check"`
}

type pxpackValidateOptions struct {
	fixturePath string
	jsonOutput  bool
}

type robotPxpackValidateInput struct {
	FixturePath string                   `json:"fixture_path"`
	Fixture     *pxpackValidationFixture `json:"fixture,omitempty"`
}

type pxpackValidationFixture struct {
	SchemaVersion            int                 `json:"schema_version"`
	Experiment               string              `json:"experiment"`
	PacketArm                pxpackValidationArm `json:"packet_arm"`
	PlainTextArm             pxpackValidationArm `json:"plain_text_arm"`
	ExpectedExactStrings     []string            `json:"expected_exact_strings"`
	ForbiddenFacts           []string            `json:"forbidden_facts"`
	RequiredSourceRereads    []string            `json:"required_source_rereads"`
	RequiredDecisionNeedles  []string            `json:"required_decision_needles"`
	PrototypeDroppedCommands []string            `json:"prototype_dropped_commands"`
}

type pxpackValidationArm struct {
	Name               string            `json:"name"`
	CostUnits          int               `json:"cost_units"`
	LatencyMs          int               `json:"latency_ms"`
	OperatorFocusScore int               `json:"operator_focus_score"`
	Answers            map[string]string `json:"answers"`
	SourceRereads      []string          `json:"source_rereads"`
	FactsheetText      string            `json:"factsheet_text,omitempty"`
	FactsheetPath      string            `json:"factsheet_path,omitempty"`
}

type pxpackValidationResult struct {
	SchemaVersion  int                   `json:"schema_version"`
	Command        string                `json:"command"`
	Status         string                `json:"status"`
	Recommended    bool                  `json:"recommended"`
	Experiment     string                `json:"experiment"`
	PacketScore    pxpackValidationScore `json:"packet_score"`
	PlainTextScore pxpackValidationScore `json:"plain_text_score"`
	Benefits       []string              `json:"benefits"`
	Failures       []string              `json:"failures"`
	NextSteps      []string              `json:"next_steps"`
	Mutating       bool                  `json:"mutating"`
}

type pxpackValidationScore struct {
	Arm                   string   `json:"arm"`
	CostUnits             int      `json:"cost_units"`
	LatencyMs             int      `json:"latency_ms"`
	OperatorFocusScore    int      `json:"operator_focus_score"`
	MissedExactStrings    []string `json:"missed_exact_strings"`
	InventedFacts         []string `json:"invented_facts"`
	MissingSourceRereads  []string `json:"missing_source_rereads"`
	DecisionFailures      []string `json:"decision_failures"`
	MissingFactsheetProof []string `json:"missing_factsheet_proof,omitempty"`
}

type pxpackResult struct {
	SchemaVersion        int               `json:"schema_version"`
	Command              string            `json:"command"`
	Status               string            `json:"status"`
	Mode                 string            `json:"mode"`
	PacketID             string            `json:"packet_id"`
	PacketDir            string            `json:"packet_dir"`
	ManifestPath         string            `json:"manifest_path"`
	FactsheetPath        string            `json:"factsheet_path"`
	SourceMapPath        string            `json:"source_map_path"`
	PageCount            int               `json:"page_count"`
	SourceHashes         []pxpackSourceRef `json:"source_hashes"`
	Stale                bool              `json:"stale"`
	PxpipeRole           string            `json:"pxpipe_role"`
	FactsheetMode        string            `json:"factsheet_mode"`
	ManifestHashMode     string            `json:"manifest_hash_mode"`
	SourceInventory      pxpackInventory   `json:"source_inventory"`
	SensitiveFindings    []pxpackFinding   `json:"sensitive_findings,omitempty"`
	PlannedPxpipeCommand []string          `json:"planned_pxpipe_command"`
	RendererTelemetry    string            `json:"renderer_telemetry_path,omitempty"`
	RendererStdout       string            `json:"renderer_stdout_path,omitempty"`
	RendererStderr       string            `json:"renderer_stderr_path,omitempty"`
	RendererManifest     string            `json:"renderer_manifest_path,omitempty"`
	Warnings             []string          `json:"warnings"`
	NextSteps            []string          `json:"next_steps"`
	Mutating             bool              `json:"mutating"`
}

type pxpackSourceRef struct {
	Path string `json:"path"`
	Lane string `json:"lane"`
	Hash string `json:"hash,omitempty"`
}

type pxpackInventory struct {
	Factsheet []string `json:"factsheet"`
	Image     []string `json:"image"`
	Live      []string `json:"live"`
	Extra     []string `json:"extra,omitempty"`
	Excludes  []string `json:"excludes,omitempty"`
}

type pxpackFinding struct {
	Path    string `json:"path"`
	Lane    string `json:"lane"`
	Reason  string `json:"reason"`
	Pattern string `json:"pattern"`
}

type pxpackManifest struct {
	SchemaVersion           int               `json:"schema_version"`
	PacketID                string            `json:"packet_id"`
	Mode                    string            `json:"mode"`
	GeneratedBy             string            `json:"generated_by"`
	PxpipeRole              string            `json:"pxpipe_role"`
	FactsheetMode           string            `json:"factsheet_mode"`
	ManifestHashMode        string            `json:"manifest_hash_mode"`
	RendererManifestTrusted bool              `json:"renderer_manifest_trusted"`
	NoEvidenceStatement     string            `json:"no_evidence_statement"`
	SourceInventory         pxpackInventory   `json:"source_inventory"`
	SourceHashes            []pxpackSourceRef `json:"source_hashes"`
	OutputHashes            []pxpackOutputRef `json:"output_hashes"`
	PlannedPxpipeCommand    []string          `json:"planned_pxpipe_command"`
	PageCount               int               `json:"page_count"`
	MaxPages                int               `json:"max_pages"`
	Exclusions              []string          `json:"exclusions,omitempty"`
	Warnings                []string          `json:"warnings,omitempty"`
	RendererTelemetry       string            `json:"renderer_telemetry_path,omitempty"`
	RendererStdout          string            `json:"renderer_stdout_path,omitempty"`
	RendererStderr          string            `json:"renderer_stderr_path,omitempty"`
	RendererManifest        string            `json:"renderer_manifest_path,omitempty"`
}

type pxpackOutputRef struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

func newPxpackCommand() *cobra.Command {
	opts := pxpackOptions{}
	cmd := &cobra.Command{
		Use:   "pxpack",
		Short: "Plan PXPIPE-backed context packets without treating images as evidence.",
		Long: `Plan Burpvalve context packets that use PXPIPE only for dense image-lane rendering.

The orchestrator mode keeps exact operational facts in Burpvalve-generated
factsheet.txt and source-map.md files. PXPIPE's auto-extracted factsheet and
manifest are renderer telemetry only; Burpvalve owns exact command strings and
source-content hashes.`,
		Example: `  burpvalve pxpack --orchestrator --out backpressure/pxpipe-packets/orchestrator-bootstrap --dry-run --json
  burpvalve pxpack --orchestrator --check backpressure/pxpipe-packets/orchestrator-bootstrap --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fail(2, "pxpack does not accept positional arguments; use --out or --check")
			}
			if robotsMode {
				return runPxpackRobots(cmd, opts)
			}
			return runPxpack(cmd, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.orchestrator, "orchestrator", false, "select the orchestrator bootstrap packet mode")
	cmd.Flags().StringVar(&opts.outDir, "out", "", "packet output directory")
	cmd.Flags().StringVar(&opts.packetID, "packet-id", "", "stable packet id; defaults from --out basename")
	cmd.Flags().StringArrayVar(&opts.sources, "source", nil, "extra dense source classified into the image lane unless protected live-only; repeatable")
	cmd.Flags().StringArrayVar(&opts.excludes, "exclude", nil, "source exclusion glob; repeatable")
	cmd.Flags().StringArrayVar(&opts.factsheetSources, "factsheet-source", nil, "exact-text source for Burpvalve-generated factsheet.txt; repeatable")
	cmd.Flags().StringArrayVar(&opts.imageSources, "image-source", nil, "dense source rendered by PXPIPE into image pages; repeatable")
	cmd.Flags().StringArrayVar(&opts.liveSources, "live-source", nil, "source recorded as live-only and not rendered; repeatable")
	cmd.Flags().IntVar(&opts.maxPages, "max-pages", 0, "maximum image pages allowed; 0 means no explicit cap")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "print the packet plan without writing files")
	cmd.Flags().BoolVar(&opts.replace, "replace", false, "allow replacing an existing generated packet directory in mutating modes")
	cmd.Flags().StringVar(&opts.checkDir, "check", "", "read-only check of an existing packet directory")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "emit machine-readable result JSON")
	cmd.AddCommand(newPxpackValidateCommand())
	return cmd
}

func newPxpackValidateCommand() *cobra.Command {
	opts := pxpackValidateOptions{}
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Score a PXPACK packet-vs-plain-text validation fixture.",
		Long: `Score the PXPACK A/B validation gate before treating orchestrator packets as recommended.

The packet arm must be at least as safe as the plain-text arm for missed exact
strings, invented facts, source re-read discipline, and decision quality. It
must also preserve prototype-dropped command strings in the Burpvalve-generated
factsheet and show a cost, latency, or operator-focus benefit.`,
		Example: `  burpvalve pxpack validate --fixture cmd/burpvalve/testdata/pxpack-validation-safe.json --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fail(2, "pxpack validate does not accept positional arguments; use --fixture")
			}
			if robotsMode {
				return runPxpackValidateRobots(cmd, opts)
			}
			return runPxpackValidate(cmd, opts, nil, "")
		},
	}
	cmd.Flags().StringVar(&opts.fixturePath, "fixture", "", "validation fixture JSON path")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "emit machine-readable result JSON")
	return cmd
}

func runPxpackRobots(cmd *cobra.Command, opts pxpackOptions) error {
	var input robotPxpackInput
	if err := decodeRobotInput(cmd.InOrStdin(), &input); err != nil {
		return err
	}
	if input.Mode != "" {
		switch input.Mode {
		case "orchestrator":
			opts.orchestrator = true
		default:
			return fail(2, "invalid pxpack mode %q; expected orchestrator", input.Mode)
		}
	}
	if input.OutDir != "" {
		opts.outDir = input.OutDir
	}
	if input.PacketID != "" {
		opts.packetID = input.PacketID
	}
	opts.sources = append(opts.sources, input.Sources...)
	opts.factsheetSources = append(opts.factsheetSources, input.FactsheetSources...)
	opts.imageSources = append(opts.imageSources, input.ImageSources...)
	opts.liveSources = append(opts.liveSources, input.LiveSources...)
	opts.excludes = append(opts.excludes, input.Excludes...)
	if input.MaxPages != 0 {
		opts.maxPages = input.MaxPages
	}
	opts.replace = opts.replace || input.Replace
	opts.dryRun = opts.dryRun || input.DryRun
	if input.Check != "" {
		opts.checkDir = input.Check
	}
	opts.jsonOutput = true
	return runPxpack(cmd, opts)
}

func runPxpackValidateRobots(cmd *cobra.Command, opts pxpackValidateOptions) error {
	var input robotPxpackValidateInput
	if err := decodeRobotInput(cmd.InOrStdin(), &input); err != nil {
		return err
	}
	if input.FixturePath != "" {
		opts.fixturePath = input.FixturePath
	}
	return runPxpackValidate(cmd, opts, input.Fixture, "")
}

func runPxpackValidate(cmd *cobra.Command, opts pxpackValidateOptions, inline *pxpackValidationFixture, inlineBase string) error {
	result, err := validatePxpackExperiment(opts, inline, inlineBase)
	if opts.jsonOutput || robotsMode {
		if encodeErr := encodeJSON(cmd.OutOrStdout(), result, "encode pxpack validation result"); encodeErr != nil {
			return encodeErr
		}
	}
	if err != nil {
		return err
	}
	if !opts.jsonOutput && !robotsMode {
		fmt.Fprintf(cmd.OutOrStdout(), "Burpvalve pxpack validation %s\n", result.Status)
		fmt.Fprintf(cmd.OutOrStdout(), "Recommended: %t\n", result.Recommended)
		for _, benefit := range result.Benefits {
			fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", benefit)
		}
	}
	return nil
}

func runPxpack(cmd *cobra.Command, opts pxpackOptions) error {
	result, err := planPxpack(opts)
	if opts.jsonOutput || robotsMode {
		if encodeErr := encodeJSON(cmd.OutOrStdout(), result, "encode pxpack result"); encodeErr != nil {
			return encodeErr
		}
	}
	if err != nil {
		return err
	}
	if !opts.jsonOutput && !robotsMode {
		fmt.Fprintf(cmd.OutOrStdout(), "Burpvalve pxpack %s packet plan\n", result.Mode)
		fmt.Fprintf(cmd.OutOrStdout(), "Packet: %s\n", result.PacketDir)
		fmt.Fprintf(cmd.OutOrStdout(), "Factsheet: %s (%s)\n", result.FactsheetPath, result.FactsheetMode)
		fmt.Fprintf(cmd.OutOrStdout(), "Manifest: %s (%s)\n", result.ManifestPath, result.ManifestHashMode)
		fmt.Fprintf(cmd.OutOrStdout(), "PXPIPE role: %s\n", result.PxpipeRole)
		for _, step := range result.NextSteps {
			fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", step)
		}
	}
	return nil
}

func validatePxpackExperiment(opts pxpackValidateOptions, inline *pxpackValidationFixture, inlineBase string) (pxpackValidationResult, error) {
	result := pxpackValidationResult{
		SchemaVersion: 1,
		Command:       "pxpack validate",
		Status:        "blocked",
		Recommended:   false,
		NextSteps:     []string{"Provide a validation fixture before recommending pxpack in shipped templates."},
		Mutating:      false,
	}
	fixture, baseDir, err := loadPxpackValidationFixture(opts.fixturePath, inline, inlineBase)
	if err != nil {
		result.Failures = []string{err.Error()}
		return result, fail(2, "load pxpack validation fixture: %v", err)
	}
	if fixture.SchemaVersion != 1 {
		result.Failures = []string{fmt.Sprintf("unsupported fixture schema_version %d", fixture.SchemaVersion)}
		return result, fail(2, "unsupported pxpack validation fixture schema_version %d", fixture.SchemaVersion)
	}
	result.Experiment = fixture.Experiment
	packetScore, err := scorePxpackValidationArm(fixture.PacketArm, fixture, baseDir, true)
	if err != nil {
		result.Failures = []string{err.Error()}
		return result, fail(2, "score packet arm: %v", err)
	}
	plainScore, err := scorePxpackValidationArm(fixture.PlainTextArm, fixture, baseDir, false)
	if err != nil {
		result.Failures = []string{err.Error()}
		return result, fail(2, "score plain-text arm: %v", err)
	}
	result.PacketScore = packetScore
	result.PlainTextScore = plainScore
	result.Benefits = pxpackValidationBenefits(packetScore, plainScore)
	result.Failures = pxpackValidationFailures(packetScore, plainScore, result.Benefits)
	if len(result.Failures) > 0 {
		result.Status = "blocked"
		result.NextSteps = []string{"Keep pxpack experimental; fix the packet fixture or reviewer workflow and rerun validation."}
		return result, fail(1, "pxpack validation blocked recommendation: %s", strings.Join(result.Failures, "; "))
	}
	result.Status = "passed"
	result.Recommended = true
	result.NextSteps = []string{"The packet arm is at least as safe as plain text and has a measured benefit for this fixture."}
	return result, nil
}

func loadPxpackValidationFixture(path string, inline *pxpackValidationFixture, inlineBase string) (pxpackValidationFixture, string, error) {
	if inline != nil {
		return *inline, inlineBase, nil
	}
	if strings.TrimSpace(path) == "" {
		return pxpackValidationFixture{}, "", fmt.Errorf("missing --fixture")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return pxpackValidationFixture{}, "", err
	}
	var fixture pxpackValidationFixture
	if err := json.Unmarshal(body, &fixture); err != nil {
		return pxpackValidationFixture{}, "", err
	}
	return fixture, filepath.Dir(path), nil
}

func scorePxpackValidationArm(arm pxpackValidationArm, fixture pxpackValidationFixture, baseDir string, packet bool) (pxpackValidationScore, error) {
	score := pxpackValidationScore{
		Arm:                defaultPxpackArmName(arm, packet),
		CostUnits:          arm.CostUnits,
		LatencyMs:          arm.LatencyMs,
		OperatorFocusScore: arm.OperatorFocusScore,
	}
	answers := pxpackValidationAnswerCorpus(arm)
	score.MissedExactStrings = missingPxpackValidationNeedles(fixture.ExpectedExactStrings, answers)
	score.InventedFacts = presentPxpackValidationNeedles(fixture.ForbiddenFacts, answers)
	score.MissingSourceRereads = missingPxpackValidationNeedles(fixture.RequiredSourceRereads, strings.Join(arm.SourceRereads, "\n"))
	score.DecisionFailures = missingPxpackValidationNeedles(fixture.RequiredDecisionNeedles, answers)
	if packet {
		factsheet, err := pxpackValidationFactsheetText(arm, baseDir)
		if err != nil {
			return score, err
		}
		score.MissingFactsheetProof = missingPxpackValidationNeedles(fixture.PrototypeDroppedCommands, factsheet)
	}
	return score, nil
}

func defaultPxpackArmName(arm pxpackValidationArm, packet bool) string {
	if strings.TrimSpace(arm.Name) != "" {
		return arm.Name
	}
	if packet {
		return "packet"
	}
	return "plain_text"
}

func pxpackValidationAnswerCorpus(arm pxpackValidationArm) string {
	if len(arm.Answers) == 0 {
		return ""
	}
	keys := make([]string, 0, len(arm.Answers))
	for key := range arm.Answers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var parts []string
	for _, key := range keys {
		parts = append(parts, key, arm.Answers[key])
	}
	return strings.Join(parts, "\n")
}

func pxpackValidationFactsheetText(arm pxpackValidationArm, baseDir string) (string, error) {
	if strings.TrimSpace(arm.FactsheetText) != "" {
		return arm.FactsheetText, nil
	}
	if strings.TrimSpace(arm.FactsheetPath) == "" {
		return "", nil
	}
	path := arm.FactsheetPath
	if !filepath.IsAbs(path) && baseDir != "" {
		path = filepath.Join(baseDir, path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func pxpackValidationBenefits(packet, plain pxpackValidationScore) []string {
	var benefits []string
	if packet.CostUnits > 0 && plain.CostUnits > 0 && packet.CostUnits < plain.CostUnits {
		benefits = append(benefits, "packet arm used fewer cost units than plain text")
	}
	if packet.LatencyMs > 0 && plain.LatencyMs > 0 && packet.LatencyMs < plain.LatencyMs {
		benefits = append(benefits, "packet arm had lower latency than plain text")
	}
	if packet.OperatorFocusScore > plain.OperatorFocusScore {
		benefits = append(benefits, "packet arm had higher operator-focus score than plain text")
	}
	return benefits
}

func pxpackValidationFailures(packet, plain pxpackValidationScore, benefits []string) []string {
	var failures []string
	if len(packet.MissedExactStrings) > len(plain.MissedExactStrings) {
		failures = append(failures, "packet arm missed more exact strings than plain text")
	}
	if len(packet.InventedFacts) > len(plain.InventedFacts) {
		failures = append(failures, "packet arm invented more forbidden facts than plain text")
	}
	if len(packet.MissingSourceRereads) > len(plain.MissingSourceRereads) {
		failures = append(failures, "packet arm missed more source re-read requirements than plain text")
	}
	if len(packet.DecisionFailures) > len(plain.DecisionFailures) {
		failures = append(failures, "packet arm had more decision-quality failures than plain text")
	}
	if len(packet.MissingFactsheetProof) > 0 {
		failures = append(failures, "packet factsheet missed prototype-dropped command strings")
	}
	if len(benefits) == 0 {
		failures = append(failures, "packet arm showed no cost, latency, or operator-focus benefit")
	}
	return failures
}

func missingPxpackValidationNeedles(needles []string, haystack string) []string {
	var missing []string
	for _, needle := range needles {
		needle = strings.TrimSpace(needle)
		if needle == "" {
			continue
		}
		if !strings.Contains(haystack, needle) {
			missing = append(missing, needle)
		}
	}
	return missing
}

func presentPxpackValidationNeedles(needles []string, haystack string) []string {
	var present []string
	for _, needle := range needles {
		needle = strings.TrimSpace(needle)
		if needle == "" {
			continue
		}
		if strings.Contains(haystack, needle) {
			present = append(present, needle)
		}
	}
	return present
}

func planPxpack(opts pxpackOptions) (pxpackResult, error) {
	if !opts.orchestrator {
		result := basePxpackResult(opts)
		result.Status = "blocked"
		result.NextSteps = []string{"Pass --orchestrator; no other pxpack mode exists in this increment."}
		return result, fail(2, "pxpack requires --orchestrator")
	}
	if opts.maxPages < 0 {
		result := basePxpackResult(opts)
		result.Status = "blocked"
		result.NextSteps = []string{"Use --max-pages with 0 or a positive integer."}
		return result, fail(2, "--max-pages must be >= 0")
	}
	if opts.checkDir != "" {
		return checkPxpack(opts)
	}
	if strings.TrimSpace(opts.outDir) == "" {
		result := basePxpackResult(opts)
		result.Status = "blocked"
		result.NextSteps = []string{"Pass --out backpressure/pxpipe-packets/<packet-id>."}
		return result, fail(2, "pxpack requires --out unless --check is used")
	}
	result := basePxpackResult(opts)
	if err := validatePxpackSourceInventory(result.SourceInventory); err != nil {
		result.Status = "blocked"
		result.NextSteps = []string{"Use existing source paths, or move authoritative instructions to --live-source instead of image or factsheet lanes."}
		return result, fail(2, "%s", err.Error())
	}
	findings := scanPxpackSensitiveInputs(result.SourceInventory)
	if len(findings) > 0 {
		result.Status = "blocked"
		result.SensitiveFindings = findings
		result.NextSteps = []string{"Remove sensitive inputs or add a scrubbed source file before generating a pxpack packet."}
		return result, fail(2, "pxpack blocked %d sensitive source finding(s)", len(findings))
	}
	sourceHashes, err := buildPxpackSourceHashes(result.SourceInventory)
	if err != nil {
		result.Status = "blocked"
		result.NextSteps = []string{"Use readable source files before generating a pxpack packet."}
		return result, fail(2, "%s", err.Error())
	}
	result.SourceHashes = sourceHashes
	if !opts.dryRun {
		return runPxpackExport(opts, result)
	}
	result.Status = "planned"
	result.NextSteps = []string{
		"Use this plan to verify source lanes before rendering images.",
		"Do not rely on PXPIPE auto factsheet or manifest for exact commands or source hashes.",
	}
	return result, nil
}

func checkPxpack(opts pxpackOptions) (pxpackResult, error) {
	opts.outDir = opts.checkDir
	result := basePxpackResult(opts)
	result.Mutating = false
	manifestPath := filepath.Join(result.PacketDir, "manifest.json")
	manifest, err := readPxpackManifest(manifestPath)
	if err != nil {
		result.Status = "blocked"
		result.NextSteps = []string{"Generate a Burpvalve-owned manifest.json before --check can validate source hashes."}
		return result, fail(1, "pxpack check requires %s: %v", manifestPath, err)
	}
	result.PacketID = manifest.PacketID
	result.Mode = manifest.Mode
	result.PageCount = manifest.PageCount
	result.SourceInventory = manifest.SourceInventory
	result.SourceHashes = manifest.SourceHashes
	result.PxpipeRole = manifest.PxpipeRole
	result.FactsheetMode = manifest.FactsheetMode
	result.ManifestHashMode = manifest.ManifestHashMode
	result.PlannedPxpipeCommand = manifest.PlannedPxpipeCommand
	result.RendererTelemetry = joinPacketPath(result.PacketDir, filepath.FromSlash(manifest.RendererTelemetry))
	result.RendererStdout = joinPacketPath(result.PacketDir, filepath.FromSlash(manifest.RendererStdout))
	result.RendererStderr = joinPacketPath(result.PacketDir, filepath.FromSlash(manifest.RendererStderr))
	result.RendererManifest = joinPacketPath(result.PacketDir, filepath.FromSlash(manifest.RendererManifest))
	findings := scanPxpackSensitiveInputs(result.SourceInventory)
	if len(findings) > 0 {
		result.Status = "blocked"
		result.SensitiveFindings = findings
		result.NextSteps = []string{"Remove sensitive inputs or add a scrubbed source file before checking a packet."}
		return result, fail(2, "pxpack blocked %d sensitive source finding(s)", len(findings))
	}
	staleReasons := checkPxpackManifestFreshness(result.PacketDir, manifest)
	if len(staleReasons) > 0 {
		result.Status = "blocked"
		result.Stale = true
		result.Warnings = append(result.Warnings, staleReasons...)
		result.NextSteps = []string{"Regenerate the pxpack packet from current sources before using it for orchestration context."}
		return result, fail(1, "pxpack packet is stale: %s", strings.Join(staleReasons, "; "))
	}
	result.Status = "ok"
	result.Stale = false
	result.NextSteps = []string{"Packet manifest matches current sources and generated outputs."}
	return result, nil
}

func basePxpackResult(opts pxpackOptions) pxpackResult {
	packetDir := filepath.Clean(opts.outDir)
	if packetDir == "." && strings.TrimSpace(opts.outDir) == "" {
		packetDir = ""
	}
	packetID := opts.packetID
	if packetID == "" && packetDir != "" {
		packetID = filepath.Base(packetDir)
	}
	inventory, inventoryWarnings := buildPxpackInventory(opts)
	pxArgs := pxpackRendererCommand(inventory, packetDir)
	return pxpackResult{
		SchemaVersion:        1,
		Command:              "pxpack",
		Status:               "planned",
		Mode:                 "orchestrator",
		PacketID:             packetID,
		PacketDir:            packetDir,
		ManifestPath:         joinPacketPath(packetDir, "manifest.json"),
		FactsheetPath:        joinPacketPath(packetDir, "factsheet.txt"),
		SourceMapPath:        joinPacketPath(packetDir, "source-map.md"),
		PageCount:            0,
		SourceHashes:         []pxpackSourceRef{},
		Stale:                false,
		PxpipeRole:           "image_lane_renderer_only",
		FactsheetMode:        "burpvalve_generated",
		ManifestHashMode:     "burpvalve_source_content_hashes",
		SourceInventory:      inventory,
		PlannedPxpipeCommand: pxArgs,
		RendererTelemetry:    joinPacketPath(packetDir, "renderer/telemetry.json"),
		RendererStdout:       joinPacketPath(packetDir, "renderer/stdout.txt"),
		RendererStderr:       joinPacketPath(packetDir, "renderer/stderr.txt"),
		RendererManifest:     joinPacketPath(packetDir, "renderer/pxpipe-manifest.json"),
		Warnings: append(inventoryWarnings,
			"prototype proved PXPIPE auto factsheet drops commands and identifiers; Burpvalve must generate factsheet.txt",
			"prototype proved PXPIPE manifest lacks source-content hashes; Burpvalve must generate manifest.json",
		),
		Mutating: false,
	}
}

func pxpackRendererCommand(inventory pxpackInventory, outDir string) []string {
	pxArgs := []string{"npx", "-y", "pxpipe-proxy", "export"}
	if override := strings.TrimSpace(os.Getenv("BURPVALVE_PXPACK_RENDERER")); override != "" {
		pxArgs = []string{override, "export"}
	}
	pxArgs = append(pxArgs, inventory.Image...)
	for _, exclude := range inventory.Excludes {
		pxArgs = append(pxArgs, "--exclude", exclude)
	}
	if outDir != "" {
		pxArgs = append(pxArgs, "--out", outDir)
	}
	return pxArgs
}

func runPxpackExport(opts pxpackOptions, result pxpackResult) (pxpackResult, error) {
	if result.PacketDir == "" {
		result.Status = "blocked"
		result.NextSteps = []string{"Pass --out backpressure/pxpipe-packets/<packet-id>."}
		return result, fail(2, "pxpack requires --out")
	}
	if _, err := os.Stat(result.PacketDir); err == nil && !opts.replace {
		result.Status = "blocked"
		result.NextSteps = []string{"Pass --replace to overwrite an existing generated packet directory."}
		return result, fail(2, "pxpack output directory already exists: %s", result.PacketDir)
	} else if err != nil && !os.IsNotExist(err) {
		result.Status = "blocked"
		result.NextSteps = []string{"Choose a writable packet output directory."}
		return result, fail(2, "inspect pxpack output directory: %v", err)
	}
	parent := filepath.Dir(result.PacketDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		result.Status = "failed"
		result.NextSteps = []string{"Choose a writable packet output directory."}
		return result, fail(1, "create pxpack output parent: %v", err)
	}
	tmpDir, err := os.MkdirTemp(parent, "."+filepath.Base(result.PacketDir)+".tmp-")
	if err != nil {
		result.Status = "failed"
		result.NextSteps = []string{"Choose a writable packet output directory."}
		return result, fail(1, "create temporary pxpack directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	renderDir := filepath.Join(tmpDir, "renderer-output")
	if err := os.MkdirAll(renderDir, 0o755); err != nil {
		result.Status = "failed"
		result.NextSteps = []string{"Check filesystem permissions for the packet output directory."}
		return result, fail(1, "create renderer output directory: %v", err)
	}
	renderCommand := pxpackRendererCommand(result.SourceInventory, renderDir)
	stdout, stderr, err := runPxpackRenderer(renderCommand)
	if err != nil {
		result.Status = "failed"
		result.NextSteps = []string{"Install PXPIPE or set BURPVALVE_PXPACK_RENDERER to a compatible executable, then rerun pxpack."}
		result.Warnings = append(result.Warnings, "PXPIPE renderer failed before a packet directory was published")
		return result, fail(1, "run PXPIPE renderer: %v", err)
	}

	stagedPacket := filepath.Join(tmpDir, "packet")
	rendererDir := filepath.Join(stagedPacket, "renderer")
	if err := os.MkdirAll(rendererDir, 0o755); err != nil {
		result.Status = "failed"
		result.NextSteps = []string{"Check filesystem permissions for the packet output directory."}
		return result, fail(1, "create renderer telemetry directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rendererDir, "stdout.txt"), stdout, 0o644); err != nil {
		result.Status = "failed"
		return result, fail(1, "write renderer stdout: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rendererDir, "stderr.txt"), stderr, 0o644); err != nil {
		result.Status = "failed"
		return result, fail(1, "write renderer stderr: %v", err)
	}
	pageCount, err := copyPxpackRendererPages(renderDir, stagedPacket)
	if err != nil {
		result.Status = "failed"
		result.NextSteps = []string{"Inspect the renderer output and rerun pxpack after fixing the PXPIPE export."}
		return result, fail(1, "copy renderer pages: %v", err)
	}
	if pageCount == 0 {
		result.Status = "failed"
		result.NextSteps = []string{"PXPIPE produced no page-*.png files; fix the renderer inputs before generating a packet."}
		return result, fail(1, "PXPIPE renderer produced no page-*.png files")
	}
	manifestPath := filepath.Join(renderDir, "manifest.json")
	if _, err := os.Stat(manifestPath); err == nil {
		if err := copyFile(manifestPath, filepath.Join(rendererDir, "pxpipe-manifest.json")); err != nil {
			result.Status = "failed"
			return result, fail(1, "copy renderer manifest telemetry: %v", err)
		}
	}
	telemetry := map[string]any{
		"schema_version":   1,
		"renderer_role":    "image_lane_renderer_only",
		"command":          renderCommand,
		"stdout_path":      "renderer/stdout.txt",
		"stderr_path":      "renderer/stderr.txt",
		"page_count":       pageCount,
		"manifest_trusted": false,
	}
	telemetryFile, err := os.Create(filepath.Join(rendererDir, "telemetry.json"))
	if err != nil {
		result.Status = "failed"
		return result, fail(1, "create renderer telemetry: %v", err)
	}
	if err := encodeJSON(telemetryFile, telemetry, "encode pxpack renderer telemetry"); err != nil {
		_ = telemetryFile.Close()
		result.Status = "failed"
		return result, err
	}
	if err := telemetryFile.Close(); err != nil {
		result.Status = "failed"
		return result, fail(1, "close renderer telemetry: %v", err)
	}
	if err := writePxpackFactsheetAndSourceMap(stagedPacket, result); err != nil {
		result.Status = "failed"
		result.NextSteps = []string{"Fix factsheet/source-map source readability and rerun pxpack."}
		return result, fail(1, "write Burpvalve factsheet/source-map: %v", err)
	}
	result.PageCount = pageCount
	if err := writePxpackManifest(stagedPacket, opts, result); err != nil {
		result.Status = "failed"
		result.NextSteps = []string{"Fix manifest output hashing and rerun pxpack."}
		return result, fail(1, "write Burpvalve manifest: %v", err)
	}
	if opts.replace {
		if err := os.RemoveAll(result.PacketDir); err != nil {
			result.Status = "failed"
			return result, fail(1, "replace existing pxpack output directory: %v", err)
		}
	}
	if err := os.Rename(stagedPacket, result.PacketDir); err != nil {
		result.Status = "failed"
		result.NextSteps = []string{"Check filesystem permissions for the packet output directory."}
		return result, fail(1, "publish pxpack output directory: %v", err)
	}
	result.Status = "ok"
	result.Mutating = true
	result.NextSteps = []string{
		"Use renderer image pages only for broad context; do not treat them as verifier evidence.",
		"Use manifest.json for source/output freshness checks and source-map.md to re-read authoritative files.",
	}
	return result, nil
}

func readPxpackManifest(path string) (pxpackManifest, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return pxpackManifest{}, err
	}
	var manifest pxpackManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return pxpackManifest{}, err
	}
	if manifest.SchemaVersion != 1 {
		return pxpackManifest{}, fmt.Errorf("unsupported manifest schema_version %d", manifest.SchemaVersion)
	}
	return manifest, nil
}

func writePxpackManifest(packetDir string, opts pxpackOptions, result pxpackResult) error {
	outputHashes, err := buildPxpackOutputHashes(packetDir)
	if err != nil {
		return err
	}
	manifest := pxpackManifest{
		SchemaVersion:           1,
		PacketID:                result.PacketID,
		Mode:                    result.Mode,
		GeneratedBy:             "burpvalve pxpack",
		PxpipeRole:              result.PxpipeRole,
		FactsheetMode:           result.FactsheetMode,
		ManifestHashMode:        result.ManifestHashMode,
		RendererManifestTrusted: false,
		NoEvidenceStatement:     "PXPIPE image pages and renderer telemetry are context indexes only; they are not verifier evidence.",
		SourceInventory:         result.SourceInventory,
		SourceHashes:            result.SourceHashes,
		OutputHashes:            outputHashes,
		PlannedPxpipeCommand:    result.PlannedPxpipeCommand,
		PageCount:               result.PageCount,
		MaxPages:                opts.maxPages,
		Exclusions:              result.SourceInventory.Excludes,
		Warnings:                result.Warnings,
		RendererTelemetry:       "renderer/telemetry.json",
		RendererStdout:          "renderer/stdout.txt",
		RendererStderr:          "renderer/stderr.txt",
		RendererManifest:        "renderer/pxpipe-manifest.json",
	}
	file, err := os.Create(filepath.Join(packetDir, "manifest.json"))
	if err != nil {
		return err
	}
	if err := encodeJSON(file, manifest, "encode pxpack manifest"); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func buildPxpackOutputHashes(packetDir string) ([]pxpackOutputRef, error) {
	var refs []pxpackOutputRef
	if err := filepath.WalkDir(packetDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(packetDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "manifest.json" {
			return nil
		}
		hash, err := hashPxpackFile(path)
		if err != nil {
			return err
		}
		refs = append(refs, pxpackOutputRef{Path: rel, Hash: hash})
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i].Path < refs[j].Path
	})
	return refs, nil
}

func checkPxpackManifestFreshness(packetDir string, manifest pxpackManifest) []string {
	var stale []string
	for _, expected := range manifest.SourceHashes {
		actual, err := hashPxpackSource(expected.Path)
		if err != nil {
			stale = append(stale, fmt.Sprintf("source %s lane=%s is missing or unreadable", expected.Path, expected.Lane))
			continue
		}
		if actual != expected.Hash {
			stale = append(stale, fmt.Sprintf("source %s lane=%s hash changed", expected.Path, expected.Lane))
		}
	}
	actualOutputs, err := buildPxpackOutputHashes(packetDir)
	if err != nil {
		stale = append(stale, fmt.Sprintf("packet outputs are unreadable: %v", err))
		return stale
	}
	actualByPath := make(map[string]string, len(actualOutputs))
	for _, actual := range actualOutputs {
		actualByPath[actual.Path] = actual.Hash
	}
	expectedByPath := make(map[string]string, len(manifest.OutputHashes))
	for _, expected := range manifest.OutputHashes {
		if expected.Path == "" {
			continue
		}
		expectedByPath[expected.Path] = expected.Hash
		actual, ok := actualByPath[expected.Path]
		if !ok {
			stale = append(stale, fmt.Sprintf("output %s is missing or unreadable", expected.Path))
			continue
		}
		if actual != expected.Hash {
			stale = append(stale, fmt.Sprintf("output %s hash changed", expected.Path))
		}
	}
	for _, actual := range actualOutputs {
		if _, ok := expectedByPath[actual.Path]; !ok {
			stale = append(stale, fmt.Sprintf("output %s is not listed in manifest", actual.Path))
		}
	}
	return stale
}

func writePxpackFactsheetAndSourceMap(packetDir string, result pxpackResult) error {
	factsheet, err := renderPxpackFactsheet(result)
	if err != nil {
		return err
	}
	sourceMap := renderPxpackSourceMap(result)
	if err := os.WriteFile(filepath.Join(packetDir, "factsheet.txt"), []byte(factsheet), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(packetDir, "source-map.md"), []byte(sourceMap), 0o644)
}

func renderPxpackFactsheet(result pxpackResult) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "# Burpvalve PXPACK Factsheet\n\n")
	fmt.Fprintf(&b, "Packet ID: %s\n", result.PacketID)
	fmt.Fprintf(&b, "Mode: %s\n", result.Mode)
	fmt.Fprintf(&b, "PXPIPE role: %s\n", result.PxpipeRole)
	fmt.Fprintf(&b, "Factsheet mode: %s\n", result.FactsheetMode)
	fmt.Fprintf(&b, "Manifest hash mode: %s\n\n", result.ManifestHashMode)
	fmt.Fprintf(&b, "## Trust Boundary\n\n")
	fmt.Fprintf(&b, "- Burpvalve generated this factsheet from factsheet-lane sources.\n")
	fmt.Fprintf(&b, "- PXPIPE is only the image-lane renderer.\n")
	fmt.Fprintf(&b, "- PXPIPE factsheet.txt is discarded and PXPIPE manifest data is renderer telemetry only.\n")
	fmt.Fprintf(&b, "- Re-read the source path before executing commands, quoting evidence, or approving mutations.\n\n")
	fmt.Fprintf(&b, "## Source Hashes\n\n")
	for _, ref := range result.SourceHashes {
		hash := ref.Hash
		if hash == "" {
			hash = "unhashed-directory"
		}
		fmt.Fprintf(&b, "- %s | lane=%s | hash=%s\n", ref.Path, ref.Lane, hash)
	}
	fmt.Fprintf(&b, "\n## Exact Text Sources\n\n")
	for _, source := range result.SourceInventory.Factsheet {
		body, err := readPxpackTextSource(source)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "### %s\n\n", source)
		fmt.Fprintf(&b, "```text\n%s\n```\n\n", strings.TrimRight(body, "\n"))
	}
	return b.String(), nil
}

func renderPxpackSourceMap(result pxpackResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Burpvalve PXPACK Source Map\n\n")
	fmt.Fprintf(&b, "Packet ID: %s\n", result.PacketID)
	fmt.Fprintf(&b, "Packet directory: %s\n\n", result.PacketDir)
	fmt.Fprintf(&b, "## Re-read Rule\n\n")
	fmt.Fprintf(&b, "Factsheet and image pages are indexes. Re-read the source path before executing commands, approving mutations, quoting evidence, or deciding whether a packet is stale.\n\n")
	fmt.Fprintf(&b, "## Lane Inventory\n\n")
	for _, source := range pxpackLaneSources(result.SourceInventory) {
		hash := pxpackHashForSource(result.SourceHashes, source.Path, source.Lane)
		if hash == "" {
			hash = "unhashed-directory"
		}
		role := "exact text index"
		if source.Lane == "image" {
			role = "dense context rendered by PXPIPE"
		} else if source.Lane == "live" {
			role = "live-only authority; not rendered"
		}
		fmt.Fprintf(&b, "- `%s` lane=`%s` hash=`%s` role=%s\n", source.Path, source.Lane, hash, role)
	}
	fmt.Fprintf(&b, "\n## Renderer Telemetry\n\n")
	fmt.Fprintf(&b, "- stdout: %s\n", result.RendererStdout)
	fmt.Fprintf(&b, "- stderr: %s\n", result.RendererStderr)
	fmt.Fprintf(&b, "- telemetry: %s\n", result.RendererTelemetry)
	fmt.Fprintf(&b, "- renderer manifest: %s (not trusted for staleness)\n", result.RendererManifest)
	return b.String()
}

func readPxpackTextSource(source string) (string, error) {
	resolved := resolvePxpackSourcePath(source)
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		entries, err := os.ReadDir(resolved)
		if err != nil {
			return "", err
		}
		var names []string
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		sort.Strings(names)
		return "Directory source. Entries:\n" + strings.Join(names, "\n"), nil
	}
	body, err := os.ReadFile(resolved)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func buildPxpackSourceHashes(inventory pxpackInventory) ([]pxpackSourceRef, error) {
	var refs []pxpackSourceRef
	for _, source := range pxpackLaneSources(inventory) {
		hash, err := hashPxpackSource(source.Path)
		if err != nil {
			return nil, err
		}
		refs = append(refs, pxpackSourceRef{Path: source.Path, Lane: source.Lane, Hash: hash})
	}
	return refs, nil
}

func hashPxpackSource(source string) (string, error) {
	resolved := resolvePxpackSourcePath(source)
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("hash pxpack source %q: %w", source, err)
	}
	if !info.IsDir() {
		body, err := os.ReadFile(resolved)
		if err != nil {
			return "", fmt.Errorf("hash pxpack source %q: %w", source, err)
		}
		return hashPxpackBytes(body), nil
	}
	var entries []string
	if err := filepath.WalkDir(resolved, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(resolved, path)
		if err != nil {
			return err
		}
		entries = append(entries, filepath.ToSlash(rel)+" "+hashPxpackBytes(body))
		return nil
	}); err != nil {
		return "", fmt.Errorf("hash pxpack source directory %q: %w", source, err)
	}
	sort.Strings(entries)
	return hashPxpackBytes([]byte(strings.Join(entries, "\n"))), nil
}

func hashPxpackFile(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return hashPxpackBytes(body), nil
}

func hashPxpackBytes(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func pxpackHashForSource(refs []pxpackSourceRef, path, lane string) string {
	for _, ref := range refs {
		if ref.Path == path && ref.Lane == lane {
			return ref.Hash
		}
	}
	return ""
}

func runPxpackRenderer(args []string) ([]byte, []byte, error) {
	if len(args) == 0 {
		return nil, nil, fmt.Errorf("empty renderer command")
	}
	command := exec.Command(args[0], args[1:]...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func copyPxpackRendererPages(srcDir, dstDir string) (int, error) {
	count := 0
	err := filepath.WalkDir(srcDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		name := filepath.Base(path)
		if strings.HasPrefix(name, "page-") && strings.EqualFold(filepath.Ext(name), ".png") {
			if err := copyFile(path, filepath.Join(dstDir, name)); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	return count, err
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func buildPxpackInventory(opts pxpackOptions) (pxpackInventory, []string) {
	var warnings []string
	inventory := pxpackInventory{
		Excludes: normalizePxpackSourceList(opts.excludes),
	}
	for _, source := range defaultOr(opts.factsheetSources, defaultPxpackFactsheetSources()) {
		addPxpackLaneSource(&inventory, &warnings, "factsheet", source, "factsheet")
	}
	for _, source := range defaultOr(opts.imageSources, defaultPxpackImageSources()) {
		addPxpackLaneSource(&inventory, &warnings, "image", source, "image")
	}
	for _, source := range defaultOr(opts.liveSources, defaultPxpackLiveSources()) {
		addPxpackLaneSource(&inventory, &warnings, "live", source, "live")
	}
	for _, source := range opts.sources {
		lane := "image"
		if isPxpackLiveOnlySource(source) {
			lane = "live"
		}
		addPxpackLaneSource(&inventory, &warnings, lane, source, "source")
	}
	return inventory, warnings
}

func addPxpackLaneSource(inventory *pxpackInventory, warnings *[]string, lane, source, origin string) {
	source = normalizePxpackSource(source)
	if source == "" {
		return
	}
	if lane != "live" && isPxpackLiveOnlySource(source) {
		lane = "live"
		*warnings = append(*warnings, fmt.Sprintf("%s source %q is live-only and will not be sent to PXPIPE", origin, source))
	}
	switch lane {
	case "factsheet":
		inventory.Factsheet = appendUniqueStrings(inventory.Factsheet, source)
	case "image":
		inventory.Image = appendUniqueStrings(inventory.Image, source)
	case "live":
		inventory.Live = appendUniqueStrings(inventory.Live, source)
	}
}

func normalizePxpackSourceList(values []string) []string {
	var normalized []string
	for _, value := range values {
		normalized = appendUniqueStrings(normalized, normalizePxpackSource(value))
	}
	return normalized
}

func normalizePxpackSource(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return filepath.Clean(value)
}

func isPxpackLiveOnlySource(path string) bool {
	path = normalizePxpackSource(path)
	if path == "." || path == "" {
		return false
	}
	if filepath.IsAbs(path) {
		if rel, err := filepath.Rel(".", path); err == nil && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
			path = filepath.Clean(rel)
		}
	}
	path = filepath.ToSlash(path)
	return path == "AGENTS.md" ||
		strings.HasPrefix(path, "backpressure/") ||
		strings.HasPrefix(path, "log/backpressure/responses/") ||
		strings.HasPrefix(path, "log/backpressure/failed/")
}

func validatePxpackSourceInventory(inventory pxpackInventory) error {
	for _, source := range pxpackLaneSources(inventory) {
		if _, err := os.Stat(resolvePxpackSourcePath(source.Path)); err != nil {
			return fmt.Errorf("pxpack source path %q is not readable: %w", source.Path, err)
		}
	}
	return nil
}

type pxpackLaneSource struct {
	Path string
	Lane string
}

func pxpackLaneSources(inventory pxpackInventory) []pxpackLaneSource {
	var sources []pxpackLaneSource
	for _, source := range inventory.Factsheet {
		sources = append(sources, pxpackLaneSource{Path: source, Lane: "factsheet"})
	}
	for _, source := range inventory.Image {
		sources = append(sources, pxpackLaneSource{Path: source, Lane: "image"})
	}
	for _, source := range inventory.Live {
		sources = append(sources, pxpackLaneSource{Path: source, Lane: "live"})
	}
	return sources
}

func scanPxpackSensitiveInputs(inventory pxpackInventory) []pxpackFinding {
	var findings []pxpackFinding
	for _, source := range pxpackLaneSources(inventory) {
		path := resolvePxpackSourcePath(source.Path)
		findings = append(findings, scanPxpackSensitivePath(source, path)...)
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		findings = append(findings, scanPxpackSensitiveContent(source, string(body))...)
	}
	return findings
}

func scanPxpackSensitivePath(source pxpackLaneSource, resolved string) []pxpackFinding {
	slash := filepath.ToSlash(source.Path)
	base := strings.ToLower(filepath.Base(slash))
	lower := strings.ToLower(slash)
	for _, pattern := range []string{".env", ".env.", "id_rsa", "id_ed25519", "credentials", "secret", "secrets", "private-key"} {
		if base == pattern || strings.Contains(lower, "/"+pattern) || strings.Contains(lower, pattern+"/") {
			return []pxpackFinding{{Path: source.Path, Lane: source.Lane, Reason: "sensitive path is not allowed in pxpack sources", Pattern: pattern}}
		}
	}
	if strings.Contains(strings.ToLower(resolved), "/.ssh/") {
		return []pxpackFinding{{Path: source.Path, Lane: source.Lane, Reason: "SSH material is not allowed in pxpack sources", Pattern: ".ssh"}}
	}
	return nil
}

func scanPxpackSensitiveContent(source pxpackLaneSource, body string) []pxpackFinding {
	checks := []struct {
		reason  string
		pattern string
		re      *regexp.Regexp
	}{
		{"private key material is not allowed in pxpack sources", "private-key-block", regexp.MustCompile(`(?m)-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
		{"bearer tokens are not allowed in pxpack sources", "authorization-bearer", regexp.MustCompile(`(?i)authorization\s*[:=]\s*bearer\s+[A-Za-z0-9._~+/=-]{16,}`)},
		{"token-like assignments are not allowed in pxpack sources", "token-assignment", regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|auth[_-]?token|secret[_-]?key|client[_-]?secret|password)\b\s*[:=]\s*['"]?[A-Za-z0-9._~+/=-]{16,}`)},
		{"GitHub token-looking values are not allowed in pxpack sources", "github-token", regexp.MustCompile(`\b(ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9_]{20,}\b`)},
		{"private IPv4 addresses are not allowed in pxpack sources", "private-ipv4", regexp.MustCompile(`\b(10\.\d{1,3}\.\d{1,3}\.\d{1,3}|192\.168\.\d{1,3}\.\d{1,3}|172\.(1[6-9]|2\d|3[0-1])\.\d{1,3}\.\d{1,3})\b`)},
	}
	var findings []pxpackFinding
	for _, check := range checks {
		if check.re.FindStringIndex(body) != nil {
			findings = append(findings, pxpackFinding{Path: source.Path, Lane: source.Lane, Reason: check.reason, Pattern: check.pattern})
		}
	}
	return findings
}

func resolvePxpackSourcePath(source string) string {
	if filepath.IsAbs(source) {
		return source
	}
	if _, err := os.Stat(source); err == nil {
		return source
	}
	dir, err := os.Getwd()
	if err != nil {
		return source
	}
	for {
		candidate := filepath.Join(dir, source)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return source
		}
		dir = parent
	}
}

func defaultOr(values, fallback []string) []string {
	if len(values) > 0 {
		return append([]string(nil), values...)
	}
	return append([]string(nil), fallback...)
}

func joinPacketPath(dir, name string) string {
	if dir == "" {
		return name
	}
	return filepath.Join(dir, name)
}

func defaultPxpackFactsheetSources() []string {
	return []string{
		"templates/claude/skills/burpvalve-orchestrator/references/orchestrator-toolbox.md.tmpl",
		"docs/ntm-bridge.md",
		"ORCHESTRATOR.md",
	}
}

func defaultPxpackImageSources() []string {
	return []string{
		"ORCHESTRATOR.md",
		"docs/dogfooding-findings-2026-07.md",
	}
}

func defaultPxpackLiveSources() []string {
	return []string{
		"AGENTS.md",
		"backpressure/README.md",
	}
}
