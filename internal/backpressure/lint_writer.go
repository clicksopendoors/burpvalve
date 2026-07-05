package backpressure

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"burpvalve/internal/lintconfig"

	"gopkg.in/yaml.v3"
)

const (
	LintRulesPath                      = "backpressure/lint-rules.md"
	LintInitRecommendationsStartMarker = "<!-- burpvalve:lint-init recommendations -->"
	LintInitRecommendationsEndMarker   = "<!-- /burpvalve:lint-init recommendations -->"
)

type LintCommandConflictAction string

const (
	LintCommandConflictSkip   LintCommandConflictAction = "skip"
	LintCommandConflictUpdate LintCommandConflictAction = "update"
	LintCommandConflictRename LintCommandConflictAction = "rename"
)

type LintCommandProposal struct {
	Command    lintconfig.Command
	OnConflict LintCommandConflictAction
	RenameID   string
}

type LintManifestUpdate struct {
	Commands []LintCommandProposal
	Coverage lintconfig.Coverage
}

type LintFileUpdateResult struct {
	Path    string
	Before  string
	After   string
	Changed bool
	Added   []string
	Updated []string
	Renamed []string
	Skipped []string
}

func PlanLintManifestUpdate(root string, update LintManifestUpdate) (LintFileUpdateResult, error) {
	root, err := filepath.Abs(defaultRoot(root))
	if err != nil {
		return LintFileUpdateResult{}, err
	}
	before, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ManifestPath)))
	if err != nil {
		return LintFileUpdateResult{}, err
	}
	after, result, err := applyLintManifestUpdate(before, update)
	if err != nil {
		return LintFileUpdateResult{}, err
	}
	result.Path = ManifestPath
	result.Before = string(before)
	result.After = string(after)
	result.Changed = result.Before != result.After
	return result, nil
}

func WriteLintManifestUpdate(root string, update LintManifestUpdate) (LintFileUpdateResult, error) {
	result, err := PlanLintManifestUpdate(root, update)
	if err != nil || !result.Changed {
		return result, err
	}
	root, err = filepath.Abs(defaultRoot(root))
	if err != nil {
		return LintFileUpdateResult{}, err
	}
	path := filepath.Join(root, filepath.FromSlash(ManifestPath))
	if err := atomicWriteFile(path, []byte(result.After), fileModeOrDefault(path, 0o644)); err != nil {
		return LintFileUpdateResult{}, err
	}
	return result, nil
}

func PlanLintRulesRecommendationsUpdate(root string, recommendations []string) (LintFileUpdateResult, error) {
	root, err := filepath.Abs(defaultRoot(root))
	if err != nil {
		return LintFileUpdateResult{}, err
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(LintRulesPath)))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return LintFileUpdateResult{}, err
	}
	after := replaceLintRecommendationsSection(string(body), recommendations)
	return LintFileUpdateResult{
		Path:    LintRulesPath,
		Before:  string(body),
		After:   after,
		Changed: string(body) != after,
	}, nil
}

func WriteLintRulesRecommendationsUpdate(root string, recommendations []string) (LintFileUpdateResult, error) {
	result, err := PlanLintRulesRecommendationsUpdate(root, recommendations)
	if err != nil || !result.Changed {
		return result, err
	}
	root, err = filepath.Abs(defaultRoot(root))
	if err != nil {
		return LintFileUpdateResult{}, err
	}
	path := filepath.Join(root, filepath.FromSlash(LintRulesPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return LintFileUpdateResult{}, err
	}
	if err := atomicWriteFile(path, []byte(result.After), fileModeOrDefault(path, 0o644)); err != nil {
		return LintFileUpdateResult{}, err
	}
	return result, nil
}

func applyLintManifestUpdate(body []byte, update LintManifestUpdate) ([]byte, LintFileUpdateResult, error) {
	if err := validateManifestShape(body); err != nil {
		return nil, LintFileUpdateResult{}, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(body, &doc); err != nil {
		return nil, LintFileUpdateResult{}, fmt.Errorf("parse %s: %w", ManifestPath, err)
	}
	root := manifestRootMapping(&doc)
	if root.Kind != yaml.MappingNode {
		return nil, LintFileUpdateResult{}, fmt.Errorf("%s: manifest root must be an object", ManifestPath)
	}
	if err := validateLintCommandProposals(update.Commands); err != nil {
		return nil, LintFileUpdateResult{}, err
	}
	result := LintFileUpdateResult{}
	if len(update.Commands) > 0 {
		commandsNode := ensureMappingSequence(root, "lint_commands")
		if err := validateExistingLintCommandIDs(commandsNode); err != nil {
			return nil, LintFileUpdateResult{}, err
		}
		if err := applyLintCommandProposals(commandsNode, update.Commands, &result); err != nil {
			return nil, LintFileUpdateResult{}, err
		}
	}
	changed := len(result.Added) > 0 || len(result.Updated) > 0 || len(result.Renamed) > 0
	if len(update.Coverage.DeclinedRoots) > 0 || strings.TrimSpace(update.Coverage.DeclinedAt) != "" {
		coverageNode := ensureMappingMapping(root, "lint_coverage")
		if err := applyLintCoverageUpdate(coverageNode, update.Coverage); err != nil {
			return nil, LintFileUpdateResult{}, err
		}
		changed = true
	}
	if !changed {
		return body, result, nil
	}
	after, err := encodeYAML(&doc)
	if err != nil {
		return nil, LintFileUpdateResult{}, err
	}
	return after, result, nil
}

func validateLintCommandProposals(proposals []LintCommandProposal) error {
	seen := map[string]int{}
	for i, proposal := range proposals {
		command := proposal.Command
		id := strings.TrimSpace(command.ID)
		if id == "" {
			return fmt.Errorf("lint command proposal %d missing id", i)
		}
		if prior, ok := seen[id]; ok {
			return fmt.Errorf("duplicate lint command proposal id %q at proposals %d and %d", id, prior, i)
		}
		seen[id] = i
		if strings.TrimSpace(command.Command) == "" {
			return fmt.Errorf("lint command proposal %q missing command", id)
		}
		if len(command.Paths) == 0 {
			return fmt.Errorf("lint command proposal %q missing paths", id)
		}
		for p, path := range command.Paths {
			if _, err := normalizeRepoRelativePath(path, ""); err != nil {
				return fmt.Errorf("lint command proposal %q paths[%d] %q is invalid: %w", id, p, path, err)
			}
		}
		if _, err := normalizeRepoRelativePath(command.RunDirectory, "."); err != nil {
			return fmt.Errorf("lint command proposal %q run_directory %q is invalid: %w", id, command.RunDirectory, err)
		}
		if command.TimeoutSeconds <= 0 {
			return fmt.Errorf("lint command proposal %q timeout_seconds must be positive", id)
		}
		switch proposal.OnConflict {
		case "", LintCommandConflictSkip, LintCommandConflictUpdate, LintCommandConflictRename:
		default:
			return fmt.Errorf("lint command proposal %q has unsupported on_conflict %q; expected skip, update, or rename", id, proposal.OnConflict)
		}
	}
	return nil
}

func validateExistingLintCommandIDs(commandsNode *yaml.Node) error {
	seen := map[string]int{}
	for i, item := range commandsNode.Content {
		id := lintCommandNodeID(item)
		if id == "" {
			continue
		}
		if prior, ok := seen[id]; ok {
			return fmt.Errorf("%s: duplicate existing lint command id %q at lint_commands[%d] and lint_commands[%d]", ManifestPath, id, prior, i)
		}
		seen[id] = i
	}
	return nil
}

func applyLintCommandProposals(commandsNode *yaml.Node, proposals []LintCommandProposal, result *LintFileUpdateResult) error {
	existing := lintCommandIndex(commandsNode)
	for _, proposal := range proposals {
		command := normalizeLintCommandProposal(proposal.Command)
		if index, ok := existing[command.ID]; ok {
			action := proposal.OnConflict
			if action == "" {
				action = LintCommandConflictSkip
			}
			switch action {
			case LintCommandConflictSkip:
				result.Skipped = append(result.Skipped, command.ID)
				continue
			case LintCommandConflictUpdate:
				commandsNode.Content[index] = lintCommandNode(command)
				result.Updated = append(result.Updated, command.ID)
				continue
			case LintCommandConflictRename:
				renamed := command
				renamed.ID = strings.TrimSpace(proposal.RenameID)
				if renamed.ID == "" {
					return fmt.Errorf("lint command proposal %q on_conflict=rename requires rename_id", command.ID)
				}
				if _, ok := existing[renamed.ID]; ok {
					return fmt.Errorf("lint command proposal %q rename_id %q already exists", command.ID, renamed.ID)
				}
				commandsNode.Content = append(commandsNode.Content, lintCommandNode(renamed))
				existing[renamed.ID] = len(commandsNode.Content) - 1
				result.Renamed = append(result.Renamed, command.ID+"->"+renamed.ID)
				continue
			}
		}
		commandsNode.Content = append(commandsNode.Content, lintCommandNode(command))
		existing[command.ID] = len(commandsNode.Content) - 1
		result.Added = append(result.Added, command.ID)
	}
	return nil
}

func normalizeLintCommandProposal(command lintconfig.Command) lintconfig.Command {
	command.ID = strings.TrimSpace(command.ID)
	command.Command = strings.TrimSpace(command.Command)
	for i, path := range command.Paths {
		command.Paths[i], _ = normalizeRepoRelativePath(path, "")
	}
	command.RunDirectory, _ = normalizeRepoRelativePath(command.RunDirectory, ".")
	return command
}

func lintCommandIndex(commandsNode *yaml.Node) map[string]int {
	index := map[string]int{}
	for i, item := range commandsNode.Content {
		if id := lintCommandNodeID(item); id != "" {
			index[id] = i
		}
	}
	return index
}

func lintCommandNodeID(node *yaml.Node) string {
	if node.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i < len(node.Content)-1; i += 2 {
		if node.Content[i].Value == "id" {
			return strings.TrimSpace(node.Content[i+1].Value)
		}
	}
	return ""
}

func applyLintCoverageUpdate(node *yaml.Node, coverage lintconfig.Coverage) error {
	node.Kind = yaml.MappingNode
	node.Tag = "!!map"
	node.Content = nil
	if len(coverage.DeclinedRoots) > 0 {
		roots := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for i, root := range coverage.DeclinedRoots {
			clean, err := normalizeRepoRelativePath(root, "")
			if err != nil {
				return fmt.Errorf("lint_coverage.declined_roots[%d] %q is invalid: %w", i, root, err)
			}
			roots.Content = append(roots.Content, stringNode(clean))
		}
		appendMapping(node, "declined_roots", roots)
	}
	if declinedAt := strings.TrimSpace(coverage.DeclinedAt); declinedAt != "" {
		appendMapping(node, "declined_at", stringNode(declinedAt))
	}
	return nil
}

func manifestRootMapping(doc *yaml.Node) *yaml.Node {
	if len(doc.Content) == 0 {
		doc.Kind = yaml.DocumentNode
		doc.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	root := doc.Content[0]
	if root.Kind == 0 {
		root.Kind = yaml.MappingNode
		root.Tag = "!!map"
	}
	return root
}

func ensureMappingSequence(root *yaml.Node, key string) *yaml.Node {
	if node := lintManifestMappingValue(root, key); node != nil {
		return node
	}
	node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	appendMapping(root, key, node)
	return node
}

func ensureMappingMapping(root *yaml.Node, key string) *yaml.Node {
	if node := lintManifestMappingValue(root, key); node != nil {
		return node
	}
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	appendMapping(root, key, node)
	return node
}

func lintCommandNode(command lintconfig.Command) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	appendMapping(node, "id", stringNode(command.ID))
	appendMapping(node, "command", stringNode(command.Command))
	appendMapping(node, "required", boolNode(command.Required))
	paths := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
	for _, path := range command.Paths {
		paths.Content = append(paths.Content, stringNode(path))
	}
	appendMapping(node, "paths", paths)
	appendMapping(node, "timeout_seconds", intNode(command.TimeoutSeconds))
	if command.RunDirectory != "" && command.RunDirectory != "." {
		appendMapping(node, "run_directory", stringNode(command.RunDirectory))
	}
	if command.Serial {
		appendMapping(node, "serial", boolNode(command.Serial))
	}
	return node
}

func appendMapping(node *yaml.Node, key string, value *yaml.Node) {
	node.Content = append(node.Content, stringNode(key), value)
}

func stringNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func boolNode(value bool) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(value)}
}

func intNode(value int) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(value)}
}

func encodeYAML(doc *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(doc); err != nil {
		_ = encoder.Close()
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func replaceLintRecommendationsSection(existing string, recommendations []string) string {
	section := lintRecommendationsSection(recommendations)
	start := strings.Index(existing, LintInitRecommendationsStartMarker)
	if start < 0 {
		if strings.TrimSpace(existing) == "" {
			return section
		}
		separator := "\n\n"
		if strings.HasSuffix(existing, "\n\n") {
			separator = ""
		} else if strings.HasSuffix(existing, "\n") {
			separator = "\n"
		}
		return existing + separator + section
	}
	endSearch := existing[start:]
	end := strings.Index(endSearch, LintInitRecommendationsEndMarker)
	if end < 0 {
		return strings.TrimRight(existing[:start], "\n") + "\n\n" + section
	}
	end += start + len(LintInitRecommendationsEndMarker)
	rest := existing[end:]
	if strings.HasPrefix(rest, "\n") {
		rest = strings.TrimPrefix(rest, "\n")
	}
	return existing[:start] + section + rest
}

func lintRecommendationsSection(recommendations []string) string {
	var b strings.Builder
	b.WriteString(LintInitRecommendationsStartMarker)
	b.WriteString("\n## Lint Init Recommendations\n\n")
	wrote := false
	for _, recommendation := range recommendations {
		trimmed := strings.TrimSpace(recommendation)
		if trimmed == "" {
			continue
		}
		wrote = true
		b.WriteString("- ")
		b.WriteString(trimmed)
		b.WriteString("\n")
	}
	if !wrote {
		b.WriteString("- No structural lint recommendations were generated.\n")
	}
	b.WriteString(LintInitRecommendationsEndMarker)
	b.WriteString("\n")
	return b.String()
}

func atomicWriteFile(path string, body []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tempName)
		}
	}()
	if _, err := temp.Write(body); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func fileModeOrDefault(path string, fallback os.FileMode) os.FileMode {
	info, err := os.Stat(path)
	if err != nil {
		return fallback
	}
	return info.Mode().Perm()
}
