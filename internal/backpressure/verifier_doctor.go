package backpressure

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const verifierDoctorSchemaVersion = 1

type VerifierDoctorOptions struct {
	Root    string
	HomeDir string
}

type VerifierDoctorResult struct {
	SchemaVersion int                          `json:"schema_version"`
	Command       string                       `json:"command"`
	Status        string                       `json:"status"`
	Message       string                       `json:"message"`
	ReportOnly    bool                         `json:"report_only"`
	Checks        []VerifierDoctorRuntimeCheck `json:"checks"`
	NextSteps     []string                     `json:"next_steps"`
}

type VerifierDoctorRuntimeCheck struct {
	Runtime   string                         `json:"runtime"`
	Paths     []VerifierDoctorPathCheck      `json:"paths"`
	Supported bool                           `json:"supported"`
	Limits    map[string]VerifierDoctorValue `json:"limits,omitempty"`
	Warnings  []string                       `json:"warnings,omitempty"`
}

type VerifierDoctorPathCheck struct {
	Path      string `json:"path"`
	Exists    bool   `json:"exists"`
	Supported bool   `json:"supported"`
	Message   string `json:"message,omitempty"`
}

type VerifierDoctorValue struct {
	Value  any    `json:"value,omitempty"`
	Source string `json:"source,omitempty"`
	Status string `json:"status"`
}

func RunVerifierDoctor(ctx context.Context, opts VerifierDoctorOptions) (VerifierDoctorResult, error) {
	root := opts.Root
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	home := opts.HomeDir
	if strings.TrimSpace(home) == "" {
		home, _ = os.UserHomeDir()
	}
	candidates := verifierDoctorCandidates(absRoot, home)
	result := VerifierDoctorResult{
		SchemaVersion: verifierDoctorSchemaVersion,
		Command:       "verifier doctor",
		Status:        "completed",
		Message:       "verifier doctor completed without writing files",
		ReportOnly:    true,
		NextSteps: []string{
			"Use this report to decide how many read-only verifier agents to run in your current runtime.",
			"For unsupported or unknown config formats, inspect the runtime documentation or ask the repo owner before changing local settings.",
			"Do not treat standing authorization or this doctor report as per-cell verifier evidence.",
		},
	}
	for _, runtime := range []string{"claude-code", "codex", "ntm"} {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		result.Checks = append(result.Checks, runVerifierDoctorRuntime(runtime, candidates[runtime]))
	}
	return result, nil
}

func verifierDoctorCandidates(root, home string) map[string][]string {
	paths := map[string][]string{
		"claude-code": {
			filepath.Join(root, ".claude", "settings.json"),
			filepath.Join(root, ".claude", "settings.local.json"),
		},
		"codex": {
			filepath.Join(root, ".codex", "config.toml"),
			filepath.Join(root, ".codex", "config.json"),
		},
		"ntm": {
			filepath.Join(root, ".ntm", "config.toml"),
			filepath.Join(root, ".ntm", "config.json"),
		},
	}
	if home != "" {
		paths["claude-code"] = append(paths["claude-code"],
			filepath.Join(home, ".claude", "settings.json"),
			filepath.Join(home, ".claude", "settings.local.json"),
		)
		paths["codex"] = append(paths["codex"],
			filepath.Join(home, ".codex", "config.toml"),
			filepath.Join(home, ".codex", "config.json"),
		)
		paths["ntm"] = append(paths["ntm"],
			filepath.Join(home, ".config", "ntm", "config.toml"),
			filepath.Join(home, ".config", "ntm", "config.json"),
		)
	}
	return paths
}

func runVerifierDoctorRuntime(runtime string, paths []string) VerifierDoctorRuntimeCheck {
	check := VerifierDoctorRuntimeCheck{
		Runtime: runtime,
		Limits: map[string]VerifierDoctorValue{
			"subagent_limit": {Status: "unknown"},
			"depth_limit":    {Status: "unknown"},
		},
	}
	for _, path := range paths {
		pathCheck := VerifierDoctorPathCheck{Path: path}
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				pathCheck.Message = "not found"
			} else {
				pathCheck.Message = "stat failed: " + err.Error()
				check.Warnings = append(check.Warnings, pathCheck.Message)
			}
			check.Paths = append(check.Paths, pathCheck)
			continue
		}
		pathCheck.Exists = true
		if info.IsDir() {
			pathCheck.Message = "path is a directory, not a config file"
			check.Warnings = append(check.Warnings, pathCheck.Path+": "+pathCheck.Message)
			check.Paths = append(check.Paths, pathCheck)
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			pathCheck.Message = "read failed: " + err.Error()
			check.Warnings = append(check.Warnings, pathCheck.Message)
			check.Paths = append(check.Paths, pathCheck)
			continue
		}
		values, supported, message := parseVerifierDoctorConfig(path, body)
		pathCheck.Supported = supported
		pathCheck.Message = message
		if supported {
			check.Supported = true
			mergeVerifierDoctorLimits(check.Limits, values)
		} else {
			check.Warnings = append(check.Warnings, path+": "+message)
		}
		check.Paths = append(check.Paths, pathCheck)
	}
	if !check.Supported {
		check.Warnings = append(check.Warnings, "no supported "+runtime+" config file with readable verifier limit keys was found")
	}
	return check
}

func parseVerifierDoctorConfig(path string, body []byte) (map[string]VerifierDoctorValue, bool, string) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		return parseVerifierDoctorJSON(path, body)
	case ".toml":
		return parseVerifierDoctorTOML(path, body)
	default:
		return nil, false, "unsupported config file extension"
	}
}

func parseVerifierDoctorJSON(path string, body []byte) (map[string]VerifierDoctorValue, bool, string) {
	var data any
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, false, "unsupported JSON format: " + err.Error()
	}
	values := map[string]VerifierDoctorValue{}
	collectVerifierDoctorJSONValues(path, data, nil, values)
	if len(values) == 0 {
		return nil, true, "supported JSON format; no known verifier limit keys found"
	}
	return values, true, "supported JSON format"
}

func collectVerifierDoctorJSONValues(path string, value any, stack []string, values map[string]VerifierDoctorValue) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			collectVerifierDoctorJSONValues(path, child, append(stack, key), values)
		}
	case []any:
		for index, child := range typed {
			collectVerifierDoctorJSONValues(path, child, append(stack, strconv.Itoa(index)), values)
		}
	default:
		key := strings.Join(stack, ".")
		if normalized, ok := verifierDoctorKnownKey(key); ok {
			values[normalized] = VerifierDoctorValue{Value: typed, Source: path + "#" + key, Status: "known"}
		}
	}
}

var verifierDoctorTOMLLine = regexp.MustCompile(`^([A-Za-z0-9_.-]+)\s*=\s*(.+)$`)

func parseVerifierDoctorTOML(path string, body []byte) (map[string]VerifierDoctorValue, bool, string) {
	values := map[string]VerifierDoctorValue{}
	section := ""
	for lineNo, raw := range strings.Split(string(body), "\n") {
		line := strings.TrimSpace(stripVerifierDoctorComment(raw))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(strings.Trim(line, "[]"))
			continue
		}
		matches := verifierDoctorTOMLLine.FindStringSubmatch(line)
		if len(matches) != 3 {
			return nil, false, fmt.Sprintf("unsupported TOML content near line %d", lineNo+1)
		}
		key := matches[1]
		if section != "" && !strings.Contains(key, ".") {
			key = section + "." + key
		}
		normalized, ok := verifierDoctorKnownKey(key)
		if !ok {
			continue
		}
		values[normalized] = VerifierDoctorValue{
			Value:  parseVerifierDoctorScalar(matches[2]),
			Source: fmt.Sprintf("%s#%s", path, key),
			Status: "known",
		}
	}
	if len(values) == 0 {
		return nil, true, "supported TOML format; no known verifier limit keys found"
	}
	return values, true, "supported TOML format"
}

func stripVerifierDoctorComment(line string) string {
	inQuote := false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '"':
			inQuote = !inQuote
		case '#':
			if !inQuote {
				return line[:i]
			}
		}
	}
	return line
}

func parseVerifierDoctorScalar(value string) any {
	value = strings.TrimSpace(strings.TrimSuffix(value, ","))
	if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		return strings.Trim(value, `"`)
	}
	if parsed, err := strconv.Atoi(value); err == nil {
		return parsed
	}
	if parsed, err := strconv.ParseBool(value); err == nil {
		return parsed
	}
	return value
}

func verifierDoctorKnownKey(key string) (string, bool) {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), ".", "_"))
	switch {
	case strings.HasSuffix(normalized, "subagent_limit"),
		strings.HasSuffix(normalized, "subagents_limit"),
		strings.HasSuffix(normalized, "max_subagents"),
		strings.HasSuffix(normalized, "max_parallel_subagents"),
		strings.HasSuffix(normalized, "max_parallel_verifiers"):
		return "subagent_limit", true
	case strings.HasSuffix(normalized, "depth_limit"),
		strings.HasSuffix(normalized, "subagent_depth"),
		strings.HasSuffix(normalized, "max_depth"),
		strings.HasSuffix(normalized, "max_subagent_depth"):
		return "depth_limit", true
	default:
		return "", false
	}
}

func mergeVerifierDoctorLimits(target map[string]VerifierDoctorValue, values map[string]VerifierDoctorValue) {
	for key, value := range values {
		if value.Status == "" {
			value.Status = "known"
		}
		target[key] = value
	}
}
