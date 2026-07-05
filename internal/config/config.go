package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const ProjectConfigName = ".burpvalve.json"
const SchemaVersion = 1

const (
	ClaudeRouteAgentSymlink        = "agent-symlink"
	ClaudeRouteOrchestratorSkill   = "orchestrator-skill"
	ClaudeRouteNone                = "none"
	ClaudeRepairRoutePreserve      = "preserve"
	InitOrchestratorOff            = "off"
	InitOrchestratorOrchestratorMD = "orchestrator-md"
)

type File struct {
	SchemaVersion int      `json:"schema_version,omitempty"`
	Defaults      Defaults `json:"defaults,omitempty"`
}

type Defaults struct {
	SkillsDir  string             `json:"skills_dir,omitempty"`
	BinDir     string             `json:"bin_dir,omitempty"`
	Shell      string             `json:"shell,omitempty"`
	Color      string             `json:"color,omitempty"`
	Confirm    *bool              `json:"confirm,omitempty"`
	Completion CompletionDefaults `json:"completion,omitempty"`
	Init       InitDefaults       `json:"init,omitempty"`
	Repair     RepairDefaults     `json:"repair,omitempty"`
	Verifier   VerifierDefaults   `json:"verifier,omitempty"`
}

type CompletionDefaults struct {
	Path     string `json:"path,omitempty"`
	RCFile   string `json:"rc_file,omitempty"`
	UpdateRC *bool  `json:"update_rc,omitempty"`
}

type ScaffoldDefaults struct {
	Agents       *bool `json:"agents,omitempty"`
	Claude       *bool `json:"claude,omitempty"`
	Docs         *bool `json:"docs,omitempty"`
	Plans        *bool `json:"plans,omitempty"`
	Log          *bool `json:"log,omitempty"`
	Backpressure *bool `json:"backpressure,omitempty"`
	Attestations *bool `json:"attestations,omitempty"`
	PreCommit    *bool `json:"precommit,omitempty"`
	HooksPath    *bool `json:"hooks_path,omitempty"`
	RepoBin      *bool `json:"repo_bin,omitempty"`
	ToolDocs     *bool `json:"tool_docs,omitempty"`
	Beads        *bool `json:"beads,omitempty"`
	NTM          *bool `json:"ntm,omitempty"`
}

type InitDefaults struct {
	ScaffoldDefaults
	Orchestrator string `json:"orchestrator,omitempty"`
	ClaudeRoute  string `json:"claude_route,omitempty"`
	Dogfood      *bool  `json:"dogfood,omitempty"`
}

type RepairDefaults struct {
	ScaffoldDefaults
	Orchestrator *bool  `json:"orchestrator,omitempty"`
	ClaudeRoute  string `json:"claude_route,omitempty"`
}

type VerifierDefaults struct {
	Authorized           *bool             `json:"authorized,omitempty"`
	AuthorizedAt         string            `json:"authorized_at,omitempty"`
	AuthorizationScope   string            `json:"authorization_scope,omitempty"`
	SpawnMethod          string            `json:"spawn_method,omitempty"`
	DefaultModel         string            `json:"default_model,omitempty"`
	ConditionModels      map[string]string `json:"condition_models,omitempty"`
	ReadOnlyTools        *bool             `json:"read_only_tools,omitempty"`
	MaxParallelVerifiers *int              `json:"max_parallel_verifiers,omitempty"`
	TranscriptDir        string            `json:"transcript_dir,omitempty"`
	Transcripts          string            `json:"transcripts,omitempty"`
}

type Effective struct {
	GlobalPath   string
	ProjectPath  string
	GlobalFound  bool
	ProjectFound bool
	File         File
	Sources      map[string]string
}

func Load(target string) (Effective, error) {
	globalPath, err := GlobalPath()
	if err != nil {
		return Effective{}, err
	}
	projectPath, err := ProjectPath(target)
	if err != nil {
		return Effective{}, err
	}
	effective := Effective{GlobalPath: globalPath, ProjectPath: projectPath, Sources: map[string]string{}}
	if globalPath != "" {
		global, ok, err := readFile(globalPath)
		if err != nil {
			return effective, err
		}
		if ok {
			effective.GlobalFound = true
			effective.File = mergeFileWithSource(effective.File, effective.Sources, global, "global")
		}
	}
	project, ok, err := readFile(projectPath)
	if err != nil {
		return effective, err
	}
	if ok {
		effective.ProjectFound = true
		effective.File = mergeFileWithSource(effective.File, effective.Sources, project, "project")
	}
	effective.File.SchemaVersion = SchemaVersion
	return effective, nil
}

func GlobalPath() (string, error) {
	if explicit := os.Getenv("BURPVALVE_CONFIG"); explicit != "" {
		return expandUserPath(explicit), nil
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		var err error
		base, err = os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("find user config directory: %w", err)
		}
	}
	return filepath.Join(base, "burpvalve", "config.json"), nil
}

func ProjectPath(target string) (string, error) {
	if target == "" {
		target = "."
	}
	root, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ProjectConfigName), nil
}

func readFile(path string) (File, bool, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return File{}, false, nil
		}
		return File{}, false, fmt.Errorf("read config %s: %w", path, err)
	}
	var file File
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&file); err != nil {
		return File{}, false, fmt.Errorf("parse config %s: %w", path, err)
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return File{}, false, fmt.Errorf("parse config %s: expected a single JSON object", path)
	} else if !errors.Is(err, io.EOF) {
		return File{}, false, fmt.Errorf("parse config %s: %w", path, err)
	}
	file = Normalize(file)
	if err := Validate(file); err != nil {
		return File{}, false, fmt.Errorf("validate config %s: %w", path, err)
	}
	return file, true, nil
}

func mergeFile(base, override File) File {
	base.Defaults = mergeDefaults(base.Defaults, override.Defaults)
	base.SchemaVersion = SchemaVersion
	return base
}

func Merge(base, override File) File {
	return mergeFile(Normalize(base), Normalize(override))
}

func mergeDefaults(base, override Defaults) Defaults {
	if override.SkillsDir != "" {
		base.SkillsDir = override.SkillsDir
	}
	if override.BinDir != "" {
		base.BinDir = override.BinDir
	}
	if override.Shell != "" {
		base.Shell = override.Shell
	}
	if override.Color != "" {
		base.Color = override.Color
	}
	if override.Confirm != nil {
		value := *override.Confirm
		base.Confirm = &value
	}
	base.Completion = mergeCompletionDefaults(base.Completion, override.Completion)
	base.Init = mergeInitDefaults(base.Init, override.Init)
	base.Repair = mergeRepairDefaults(base.Repair, override.Repair)
	base.Verifier = mergeVerifierDefaults(base.Verifier, override.Verifier)
	return base
}

func mergeCompletionDefaults(base, override CompletionDefaults) CompletionDefaults {
	if override.Path != "" {
		base.Path = override.Path
	}
	if override.RCFile != "" {
		base.RCFile = override.RCFile
	}
	if override.UpdateRC != nil {
		base.UpdateRC = override.UpdateRC
	}
	return base
}

func mergeScaffoldDefaults(base, override ScaffoldDefaults) ScaffoldDefaults {
	overrideBool(&base.Agents, override.Agents)
	overrideBool(&base.Claude, override.Claude)
	overrideBool(&base.Docs, override.Docs)
	overrideBool(&base.Plans, override.Plans)
	overrideBool(&base.Log, override.Log)
	overrideBool(&base.Backpressure, override.Backpressure)
	overrideBool(&base.Attestations, override.Attestations)
	overrideBool(&base.PreCommit, override.PreCommit)
	overrideBool(&base.HooksPath, override.HooksPath)
	overrideBool(&base.RepoBin, override.RepoBin)
	overrideBool(&base.ToolDocs, override.ToolDocs)
	overrideBool(&base.Beads, override.Beads)
	overrideBool(&base.NTM, override.NTM)
	return base
}

func mergeInitDefaults(base, override InitDefaults) InitDefaults {
	base.ScaffoldDefaults = mergeScaffoldDefaults(base.ScaffoldDefaults, override.ScaffoldDefaults)
	if override.Orchestrator != "" {
		base.Orchestrator = override.Orchestrator
	}
	if override.ClaudeRoute != "" {
		base.ClaudeRoute = override.ClaudeRoute
	}
	overrideBool(&base.Dogfood, override.Dogfood)
	return base
}

func mergeRepairDefaults(base, override RepairDefaults) RepairDefaults {
	base.ScaffoldDefaults = mergeScaffoldDefaults(base.ScaffoldDefaults, override.ScaffoldDefaults)
	overrideBool(&base.Orchestrator, override.Orchestrator)
	if override.ClaudeRoute != "" {
		base.ClaudeRoute = override.ClaudeRoute
	}
	return base
}

func mergeVerifierDefaults(base, override VerifierDefaults) VerifierDefaults {
	overrideBool(&base.Authorized, override.Authorized)
	if override.AuthorizedAt != "" {
		base.AuthorizedAt = override.AuthorizedAt
	}
	if override.AuthorizationScope != "" {
		base.AuthorizationScope = override.AuthorizationScope
	}
	if override.SpawnMethod != "" {
		base.SpawnMethod = override.SpawnMethod
	}
	if override.DefaultModel != "" {
		base.DefaultModel = override.DefaultModel
	}
	if len(override.ConditionModels) > 0 {
		if base.ConditionModels == nil {
			base.ConditionModels = map[string]string{}
		}
		for key, value := range override.ConditionModels {
			base.ConditionModels[key] = value
		}
	}
	overrideBool(&base.ReadOnlyTools, override.ReadOnlyTools)
	if override.MaxParallelVerifiers != nil {
		value := *override.MaxParallelVerifiers
		base.MaxParallelVerifiers = &value
	}
	if override.TranscriptDir != "" {
		base.TranscriptDir = override.TranscriptDir
	}
	if override.Transcripts != "" {
		base.Transcripts = override.Transcripts
	}
	return base
}

func overrideBool(dst **bool, src *bool) {
	if src == nil {
		return
	}
	value := *src
	*dst = &value
}

func BoolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func Normalize(file File) File {
	if file.SchemaVersion == 0 {
		file.SchemaVersion = SchemaVersion
	}
	file.Defaults.Init = normalizeInitDefaults(file.Defaults.Init)
	file.Defaults.Repair = normalizeRepairDefaults(file.Defaults.Repair)
	file.Defaults.Verifier = normalizeVerifierDefaults(file.Defaults.Verifier)
	return file
}

func normalizeInitDefaults(defaults InitDefaults) InitDefaults {
	if defaults.ClaudeRoute == "" && defaults.Claude != nil {
		if *defaults.Claude {
			defaults.ClaudeRoute = ClaudeRouteAgentSymlink
		} else {
			defaults.ClaudeRoute = ClaudeRouteNone
		}
	}
	return defaults
}

func normalizeRepairDefaults(defaults RepairDefaults) RepairDefaults {
	if defaults.ClaudeRoute == "" && defaults.Claude != nil {
		if *defaults.Claude {
			defaults.ClaudeRoute = ClaudeRepairRoutePreserve
		} else {
			defaults.ClaudeRoute = ClaudeRouteNone
		}
	}
	return defaults
}

func normalizeVerifierDefaults(defaults VerifierDefaults) VerifierDefaults {
	if defaults.TranscriptDir != "" {
		defaults.TranscriptDir = filepath.Clean(defaults.TranscriptDir)
	}
	return defaults
}

func Validate(file File) error {
	file = Normalize(file)
	if file.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schema_version %d", file.SchemaVersion)
	}
	if file.Defaults.Shell != "" && !oneOf(file.Defaults.Shell, "bash", "zsh", "fish", "powershell") {
		return fmt.Errorf("defaults.shell must be bash, zsh, fish, or powershell")
	}
	if file.Defaults.Color != "" && !oneOf(file.Defaults.Color, "auto", "always", "never") {
		return fmt.Errorf("defaults.color must be auto, always, or never")
	}
	if file.Defaults.Init.Orchestrator == "claude-md" {
		return fmt.Errorf("defaults.init.orchestrator=claude-md is no longer a valid Claude route; use defaults.init.claude_route=orchestrator-skill")
	}
	if file.Defaults.Init.Orchestrator != "" && !oneOf(file.Defaults.Init.Orchestrator, InitOrchestratorOff, InitOrchestratorOrchestratorMD) {
		return fmt.Errorf("defaults.init.orchestrator must be off or orchestrator-md")
	}
	if file.Defaults.Init.ClaudeRoute != "" && !oneOf(file.Defaults.Init.ClaudeRoute, ClaudeRouteAgentSymlink, ClaudeRouteOrchestratorSkill, ClaudeRouteNone) {
		return fmt.Errorf("defaults.init.claude_route must be agent-symlink, orchestrator-skill, or none")
	}
	if file.Defaults.Repair.ClaudeRoute != "" && !oneOf(file.Defaults.Repair.ClaudeRoute, ClaudeRepairRoutePreserve, ClaudeRouteAgentSymlink, ClaudeRouteOrchestratorSkill, ClaudeRouteNone) {
		return fmt.Errorf("defaults.repair.claude_route must be preserve, agent-symlink, orchestrator-skill, or none")
	}
	if err := validateClaudeRouteCompatibility("defaults.init", file.Defaults.Init.Claude, file.Defaults.Init.ClaudeRoute, []string{ClaudeRouteAgentSymlink, ClaudeRouteOrchestratorSkill}, ClaudeRouteNone); err != nil {
		return err
	}
	if err := validateClaudeRouteCompatibility("defaults.repair", file.Defaults.Repair.Claude, file.Defaults.Repair.ClaudeRoute, []string{ClaudeRepairRoutePreserve, ClaudeRouteAgentSymlink, ClaudeRouteOrchestratorSkill}, ClaudeRouteNone); err != nil {
		return err
	}
	if err := validateVerifierDefaults(file.Defaults.Verifier); err != nil {
		return err
	}
	for name, value := range map[string]string{
		"defaults.skills_dir":             file.Defaults.SkillsDir,
		"defaults.bin_dir":                file.Defaults.BinDir,
		"defaults.completion.path":        file.Defaults.Completion.Path,
		"defaults.completion.rc_file":     file.Defaults.Completion.RCFile,
		"defaults.verifier.default_model": file.Defaults.Verifier.DefaultModel,
	} {
		if strings.ContainsRune(value, 0) {
			return fmt.Errorf("%s contains a NUL byte", name)
		}
	}
	return nil
}

func validateClaudeRouteCompatibility(prefix string, claude *bool, route string, enabledRoutes []string, disabledRoute string) error {
	if claude == nil || route == "" {
		return nil
	}
	if !*claude && oneOf(route, enabledRoutes...) {
		return fmt.Errorf("%s.claude=false conflicts with %s.claude_route=%s", prefix, prefix, route)
	}
	if *claude && route == disabledRoute {
		return fmt.Errorf("%s.claude=true conflicts with %s.claude_route=%s", prefix, prefix, route)
	}
	return nil
}

func validateVerifierDefaults(defaults VerifierDefaults) error {
	if defaults.SpawnMethod != "" && !oneOf(defaults.SpawnMethod, "native", "ntm", "hermes", "manual") {
		return fmt.Errorf("defaults.verifier.spawn_method must be native, ntm, hermes, or manual")
	}
	if defaults.MaxParallelVerifiers != nil && (*defaults.MaxParallelVerifiers < 1 || *defaults.MaxParallelVerifiers > 32) {
		return fmt.Errorf("defaults.verifier.max_parallel_verifiers must be between 1 and 32")
	}
	if defaults.TranscriptDir != "" {
		if strings.ContainsRune(defaults.TranscriptDir, 0) {
			return fmt.Errorf("defaults.verifier.transcript_dir contains a NUL byte")
		}
		if filepath.IsAbs(defaults.TranscriptDir) {
			return fmt.Errorf("defaults.verifier.transcript_dir must be repo-relative")
		}
		if defaults.TranscriptDir == "." || defaults.TranscriptDir == "" {
			return fmt.Errorf("defaults.verifier.transcript_dir must be a non-empty repo-relative path")
		}
		for _, part := range strings.Split(defaults.TranscriptDir, string(filepath.Separator)) {
			if part == ".." {
				return fmt.Errorf("defaults.verifier.transcript_dir must not contain traversal")
			}
		}
	}
	if defaults.Transcripts != "" && !oneOf(defaults.Transcripts, "summary", "full", "committed") {
		return fmt.Errorf("defaults.verifier.transcripts must be summary, full, or committed")
	}
	for key, value := range defaults.ConditionModels {
		if strings.ContainsRune(key, 0) {
			return fmt.Errorf("defaults.verifier.condition_models key contains a NUL byte")
		}
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("defaults.verifier.condition_models keys must be non-empty")
		}
		if strings.ContainsRune(value, 0) {
			return fmt.Errorf("defaults.verifier.condition_models.%s contains a NUL byte", key)
		}
	}
	return nil
}

func Write(path string, file File) error {
	file = Normalize(file)
	if err := Validate(file); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}

func SortedSources(sources map[string]string) []Source {
	values := make([]Source, 0, len(sources))
	for key, source := range sources {
		values = append(values, Source{Key: key, Source: source})
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Key < values[j].Key })
	return values
}

type Source struct {
	Key    string `json:"key"`
	Source string `json:"source"`
}

func mergeFileWithSource(base File, sources map[string]string, override File, source string) File {
	base = mergeFile(base, override)
	noteDefaultsSources(sources, override.Defaults, source)
	return base
}

func noteDefaultsSources(sources map[string]string, defaults Defaults, source string) {
	noteString(sources, "defaults.skills_dir", defaults.SkillsDir, source)
	noteString(sources, "defaults.bin_dir", defaults.BinDir, source)
	noteString(sources, "defaults.shell", defaults.Shell, source)
	noteString(sources, "defaults.color", defaults.Color, source)
	noteBool(sources, "defaults.confirm", defaults.Confirm, source)
	noteCompletionSources(sources, defaults.Completion, source)
	noteInitSources(sources, defaults.Init, source)
	noteRepairSources(sources, defaults.Repair, source)
	noteVerifierSources(sources, defaults.Verifier, source)
}

func noteCompletionSources(sources map[string]string, defaults CompletionDefaults, source string) {
	noteString(sources, "defaults.completion.path", defaults.Path, source)
	noteString(sources, "defaults.completion.rc_file", defaults.RCFile, source)
	noteBool(sources, "defaults.completion.update_rc", defaults.UpdateRC, source)
}

func noteScaffoldSources(sources map[string]string, prefix string, defaults ScaffoldDefaults, source string) {
	noteBool(sources, prefix+".agents", defaults.Agents, source)
	noteBool(sources, prefix+".claude", defaults.Claude, source)
	noteBool(sources, prefix+".docs", defaults.Docs, source)
	noteBool(sources, prefix+".plans", defaults.Plans, source)
	noteBool(sources, prefix+".log", defaults.Log, source)
	noteBool(sources, prefix+".backpressure", defaults.Backpressure, source)
	noteBool(sources, prefix+".attestations", defaults.Attestations, source)
	noteBool(sources, prefix+".precommit", defaults.PreCommit, source)
	noteBool(sources, prefix+".hooks_path", defaults.HooksPath, source)
	noteBool(sources, prefix+".repo_bin", defaults.RepoBin, source)
	noteBool(sources, prefix+".tool_docs", defaults.ToolDocs, source)
	noteBool(sources, prefix+".beads", defaults.Beads, source)
	noteBool(sources, prefix+".ntm", defaults.NTM, source)
}

func noteInitSources(sources map[string]string, defaults InitDefaults, source string) {
	noteScaffoldSources(sources, "defaults.init", defaults.ScaffoldDefaults, source)
	noteString(sources, "defaults.init.orchestrator", defaults.Orchestrator, source)
	noteString(sources, "defaults.init.claude_route", defaults.ClaudeRoute, source)
	noteBool(sources, "defaults.init.dogfood", defaults.Dogfood, source)
}

func noteRepairSources(sources map[string]string, defaults RepairDefaults, source string) {
	noteScaffoldSources(sources, "defaults.repair", defaults.ScaffoldDefaults, source)
	noteBool(sources, "defaults.repair.orchestrator", defaults.Orchestrator, source)
	noteString(sources, "defaults.repair.claude_route", defaults.ClaudeRoute, source)
}

func noteVerifierSources(sources map[string]string, defaults VerifierDefaults, source string) {
	noteBool(sources, "defaults.verifier.authorized", defaults.Authorized, source)
	noteString(sources, "defaults.verifier.authorized_at", defaults.AuthorizedAt, source)
	noteString(sources, "defaults.verifier.authorization_scope", defaults.AuthorizationScope, source)
	noteString(sources, "defaults.verifier.spawn_method", defaults.SpawnMethod, source)
	noteString(sources, "defaults.verifier.default_model", defaults.DefaultModel, source)
	for key, value := range defaults.ConditionModels {
		noteString(sources, "defaults.verifier.condition_models."+key, value, source)
	}
	noteBool(sources, "defaults.verifier.read_only_tools", defaults.ReadOnlyTools, source)
	if defaults.MaxParallelVerifiers != nil {
		sources["defaults.verifier.max_parallel_verifiers"] = source
	}
	noteString(sources, "defaults.verifier.transcript_dir", defaults.TranscriptDir, source)
	noteString(sources, "defaults.verifier.transcripts", defaults.Transcripts, source)
}

func noteString(sources map[string]string, key string, value string, source string) {
	if value != "" {
		sources[key] = source
	}
}

func noteBool(sources map[string]string, key string, value *bool, source string) {
	if value != nil {
		sources[key] = source
	}
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func expandUserPath(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if len(path) > 2 && path[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
