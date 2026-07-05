package backpressure

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"burpvalve/internal/lintconfig"
)

type LintDetectionOptions struct {
	Root     string
	LookPath func(string) (string, error)
}

type LintDetection struct {
	Root              string               `json:"root"`
	GoAvailable       bool                 `json:"go_available"`
	GoRoots           []LintGoRoot         `json:"go_roots,omitempty"`
	NodeRoots         []LintNodeRoot       `json:"node_roots,omitempty"`
	MultiRoot         bool                 `json:"multi_root"`
	NeedsScopedSetup  bool                 `json:"needs_scoped_setup"`
	CandidateCommands []lintconfig.Command `json:"candidate_commands,omitempty"`
}

type LintGoRoot struct {
	Path string `json:"path"`
}

type LintNodeRoot struct {
	Path           string   `json:"path"`
	PackageManager string   `json:"package_manager"`
	Lockfile       string   `json:"lockfile,omitempty"`
	Scripts        []string `json:"scripts,omitempty"`
}

var nodeLintScriptNames = []string{"lint", "test", "typecheck", "check", "build", "format:check"}

func DetectLintSetup(opts LintDetectionOptions) (LintDetection, error) {
	root, err := filepath.Abs(defaultRoot(opts.Root))
	if err != nil {
		return LintDetection{}, err
	}
	lookPath := opts.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	_, goErr := lookPath("go")
	result := LintDetection{
		Root:        root,
		GoAvailable: goErr == nil,
	}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if shouldSkipLintDetectionDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		switch entry.Name() {
		case "go.mod":
			if shouldSkipLintDetectionDir(filepath.ToSlash(filepath.Dir(rel))) {
				return nil
			}
			result.GoRoots = append(result.GoRoots, LintGoRoot{Path: normalizeDetectedRootPath(filepath.Dir(rel))})
		case "package.json":
			if shouldSkipLintDetectionDir(filepath.ToSlash(filepath.Dir(rel))) {
				return nil
			}
			nodeRoot, err := detectNodeRoot(path, root, filepath.Dir(rel))
			if err != nil {
				return err
			}
			result.NodeRoots = append(result.NodeRoots, nodeRoot)
		}
		return nil
	})
	if err != nil {
		return LintDetection{}, err
	}
	sort.Slice(result.GoRoots, func(i, j int) bool { return result.GoRoots[i].Path < result.GoRoots[j].Path })
	sort.Slice(result.NodeRoots, func(i, j int) bool { return result.NodeRoots[i].Path < result.NodeRoots[j].Path })
	result.MultiRoot = lintDetectionIsMultiRoot(result.GoRoots, result.NodeRoots)
	result.NeedsScopedSetup = result.MultiRoot
	result.CandidateCommands = lintDetectionCandidateCommands(result)
	return result, nil
}

func shouldSkipLintDetectionDir(rel string) bool {
	clean := filepath.ToSlash(filepath.Clean(rel))
	if clean == "." {
		return false
	}
	switch clean {
	case ".git", "node_modules", "vendor", "dist", "docs/demos/generated":
		return true
	default:
		return strings.HasPrefix(clean, "node_modules/") ||
			strings.HasPrefix(clean, "vendor/") ||
			strings.HasPrefix(clean, "dist/") ||
			strings.HasPrefix(clean, "docs/demos/generated/")
	}
}

func detectNodeRoot(packageJSONPath, root, relDir string) (LintNodeRoot, error) {
	scripts, err := readLintPackageScripts(packageJSONPath)
	if err != nil {
		return LintNodeRoot{}, err
	}
	rel := normalizeDetectedRootPath(relDir)
	manager, lockfile := detectNodePackageManager(filepath.Join(root, filepath.FromSlash(rel)))
	return LintNodeRoot{
		Path:           rel,
		PackageManager: manager,
		Lockfile:       lockfile,
		Scripts:        scripts,
	}, nil
}

func readLintPackageScripts(path string) ([]string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pkg struct {
		Scripts map[string]json.RawMessage `json:"scripts"`
	}
	if err := json.Unmarshal(body, &pkg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.ToSlash(path), err)
	}
	var scripts []string
	for _, name := range nodeLintScriptNames {
		if value, ok := pkg.Scripts[name]; ok && string(value) != "null" {
			scripts = append(scripts, name)
		}
	}
	return scripts, nil
}

func detectNodePackageManager(root string) (string, string) {
	for _, candidate := range []struct {
		lockfile string
		manager  string
	}{
		{lockfile: "package-lock.json", manager: "npm"},
		{lockfile: "pnpm-lock.yaml", manager: "pnpm"},
		{lockfile: "yarn.lock", manager: "yarn"},
		{lockfile: "bun.lockb", manager: "bun"},
	} {
		if _, err := os.Stat(filepath.Join(root, candidate.lockfile)); err == nil {
			return candidate.manager, candidate.lockfile
		}
	}
	return "npm", ""
}

func lintDetectionIsMultiRoot(goRoots []LintGoRoot, nodeRoots []LintNodeRoot) bool {
	if len(goRoots) > 0 && len(nodeRoots) > 0 {
		return true
	}
	roots := map[string]bool{}
	for _, root := range goRoots {
		roots[root.Path] = true
	}
	for _, root := range nodeRoots {
		roots[root.Path] = true
	}
	return len(roots) > 1
}

func lintDetectionCandidateCommands(detection LintDetection) []lintconfig.Command {
	commands, _ := BuiltInLintPresetCommands(detection, LintPresetAuto)
	return commands
}

func nodeRunCommand(manager, script string) string {
	switch manager {
	case "pnpm":
		return "pnpm run " + script
	case "yarn":
		return "yarn run " + script
	case "bun":
		return "bun run " + script
	default:
		return "npm run " + script
	}
}

func lintCommandIDPart(value string) string {
	var b strings.Builder
	previousDash := false
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			previousDash = false
			continue
		}
		if !previousDash {
			b.WriteByte('-')
			previousDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func normalizeDetectedRootPath(rel string) string {
	clean := filepath.ToSlash(filepath.Clean(rel))
	if clean == "." || clean == "/" {
		return "."
	}
	return clean
}
