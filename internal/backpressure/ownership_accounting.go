package backpressure

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"burpvalve/internal/gitindex"
)

type OwnershipAccountingOptions struct {
	Root             string
	Ownership        OwnershipInput
	IncludeUntracked bool
	IncludeBeads     bool
	Staged           StagedEntryReader
	Untracked        UntrackedReader
	Beads            OwnershipBeadsReader
}

type UntrackedReader interface {
	UntrackedFiles(ctx context.Context, root string) ([]OwnershipUntrackedFile, error)
}

type GitUntrackedReader struct{}

type OwnershipBeadsReader interface {
	ActiveBeads(ctx context.Context, root string) (OwnershipBeadsContext, error)
}

type FileOwnershipBeadsReader struct{}

type OwnershipUntrackedFile struct {
	Path    string `json:"path"`
	Ignored bool   `json:"ignored"`
}

type OwnershipBeadsContext struct {
	Available   bool                   `json:"available"`
	DisplayOnly bool                   `json:"display_only"`
	SourcePath  string                 `json:"source_path,omitempty"`
	Active      []OwnershipBeadContext `json:"active,omitempty"`
	Warnings    []string               `json:"warnings,omitempty"`
}

type OwnershipBeadContext struct {
	ID       string   `json:"id"`
	Title    string   `json:"title,omitempty"`
	Status   string   `json:"status,omitempty"`
	Priority int      `json:"priority,omitempty"`
	Type     string   `json:"type,omitempty"`
	Labels   []string `json:"labels,omitempty"`
}

type OwnershipAccountingResult struct {
	SchemaVersion     int                    `json:"schema_version"`
	Command           string                 `json:"command"`
	Status            string                 `json:"status"`
	Mutating          bool                   `json:"mutating"`
	Staged            []OwnershipPathResult  `json:"staged"`
	Untracked         []OwnershipPathResult  `json:"untracked,omitempty"`
	Beads             *OwnershipBeadsContext `json:"beads,omitempty"`
	Summary           OwnershipSummary       `json:"summary"`
	GeneratedPrefixes []string               `json:"generated_path_prefixes"`
	Warnings          []string               `json:"warnings,omitempty"`
}

type OwnershipSummary struct {
	StagedTotal    int `json:"staged_total"`
	UntrackedTotal int `json:"untracked_total"`
	Owned          int `json:"owned"`
	SharedDeclared int `json:"shared_declared"`
	Conflicts      int `json:"conflicts"`
	Unowned        int `json:"unowned"`
	Generated      int `json:"generated_exception"`
	Ignored        int `json:"ignored_untracked"`
	Covered        int `json:"covered_exception"`
}

func RunOwnershipAccounting(ctx context.Context, opts OwnershipAccountingOptions) (OwnershipAccountingResult, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	records := append([]OwnershipRecord(nil), opts.Ownership.Records...)
	input := OwnershipInput{Records: records}
	if err := ValidateOwnershipInput(&input); err != nil {
		return OwnershipAccountingResult{}, err
	}
	byPath := ownershipRecordsByPath(input.Records)
	stagedReader := opts.Staged
	if stagedReader == nil {
		stagedReader = GitStagedReader{}
	}
	staged, err := stagedReader.StagedEntries(ctx, root)
	if err != nil {
		return OwnershipAccountingResult{}, fmt.Errorf("inspect staged paths: %w", err)
	}
	sortStagedEntries(staged)

	result := OwnershipAccountingResult{
		SchemaVersion:     1,
		Command:           "account payload",
		Status:            "completed",
		Mutating:          false,
		GeneratedPrefixes: gitindex.GeneratedPathPrefixes(),
	}
	var activeBeads map[string]OwnershipBeadContext
	if opts.IncludeBeads {
		beadsReader := opts.Beads
		if beadsReader == nil {
			beadsReader = FileOwnershipBeadsReader{}
		}
		context, err := beadsReader.ActiveBeads(ctx, root)
		if err != nil {
			return OwnershipAccountingResult{}, fmt.Errorf("inspect beads: %w", err)
		}
		context.DisplayOnly = true
		result.Beads = &context
		activeBeads = ownershipBeadsByID(context.Active)
	}
	for _, entry := range staged {
		path := filepath.ToSlash(entry.Path)
		result.Staged = append(result.Staged, classifyOwnershipPath(path, "staged", entry.Status, false, byPath[path], activeBeads))
	}
	if opts.IncludeUntracked {
		untrackedReader := opts.Untracked
		if untrackedReader == nil {
			untrackedReader = GitUntrackedReader{}
		}
		untracked, err := untrackedReader.UntrackedFiles(ctx, root)
		if err != nil {
			return OwnershipAccountingResult{}, fmt.Errorf("inspect untracked paths: %w", err)
		}
		sort.Slice(untracked, func(i, j int) bool {
			return untracked[i].Path < untracked[j].Path
		})
		for _, file := range untracked {
			path := filepath.ToSlash(file.Path)
			result.Untracked = append(result.Untracked, classifyOwnershipPath(path, "untracked", "untracked", file.Ignored, byPath[path], activeBeads))
		}
	}
	result.Summary = summarizeOwnership(result.Staged, result.Untracked)
	return result, nil
}

func (FileOwnershipBeadsReader) ActiveBeads(ctx context.Context, root string) (OwnershipBeadsContext, error) {
	path := filepath.Join(root, ".beads", "issues.jsonl")
	context := OwnershipBeadsContext{
		Available:   false,
		DisplayOnly: true,
		SourcePath:  ".beads/issues.jsonl",
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			context.Warnings = []string{"Beads metadata unavailable: .beads/issues.jsonl not found"}
			return context, nil
		}
		return OwnershipBeadsContext{}, err
	}
	context.Available = true
	for lineNumber, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var beadLine struct {
			ID        string   `json:"id"`
			Title     string   `json:"title"`
			Status    string   `json:"status"`
			Priority  int      `json:"priority"`
			Type      string   `json:"type"`
			IssueType string   `json:"issue_type"`
			Labels    []string `json:"labels"`
		}
		if err := json.Unmarshal([]byte(line), &beadLine); err != nil {
			context.Warnings = append(context.Warnings, fmt.Sprintf("skipped malformed Beads line %d", lineNumber+1))
			continue
		}
		bead := OwnershipBeadContext{
			ID:       beadLine.ID,
			Title:    beadLine.Title,
			Status:   beadLine.Status,
			Priority: beadLine.Priority,
			Type:     firstNonEmpty(beadLine.Type, beadLine.IssueType),
			Labels:   beadLine.Labels,
		}
		if ownershipBeadStatusActive(bead.Status) {
			bead.Labels = append([]string(nil), bead.Labels...)
			sort.Strings(bead.Labels)
			context.Active = append(context.Active, bead)
		}
	}
	sort.Slice(context.Active, func(i, j int) bool {
		return context.Active[i].ID < context.Active[j].ID
	})
	return context, nil
}

func (GitUntrackedReader) UntrackedFiles(ctx context.Context, root string) ([]OwnershipUntrackedFile, error) {
	unignored, err := gitUntrackedList(ctx, root, false)
	if err != nil {
		return nil, err
	}
	ignored, err := gitUntrackedList(ctx, root, true)
	if err != nil {
		return nil, err
	}
	byPath := map[string]OwnershipUntrackedFile{}
	for _, path := range unignored {
		byPath[path] = OwnershipUntrackedFile{Path: path}
	}
	for _, path := range ignored {
		byPath[path] = OwnershipUntrackedFile{Path: path, Ignored: true}
	}
	files := make([]OwnershipUntrackedFile, 0, len(byPath))
	for _, file := range byPath {
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, nil
}

func gitUntrackedList(ctx context.Context, root string, ignored bool) ([]string, error) {
	args := []string{"ls-files", "--others", "-z"}
	if ignored {
		args = append(args, "--ignored")
	}
	args = append(args, "--exclude-standard")
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseNULPaths(out), nil
}

func parseNULPaths(out []byte) []string {
	parts := strings.Split(string(out), "\x00")
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		paths = append(paths, filepath.ToSlash(part))
	}
	sort.Strings(paths)
	return paths
}

func classifyOwnershipPath(path, pathState, gitStatus string, ignored bool, records []OwnershipRecord, beads map[string]OwnershipBeadContext) OwnershipPathResult {
	result := OwnershipPathResult{
		Path:         filepath.ToSlash(path),
		Status:       OwnershipStatusUnowned,
		PathState:    pathState,
		GitStatus:    gitStatus,
		Ignored:      ignored,
		Generated:    gitindex.IsGeneratedEvidencePath(path),
		Owners:       append([]OwnershipRecord(nil), records...),
		OwnerUnitIDs: ownerUnitIDs(records),
		BeadsContext: ownershipBeadContextForRecords(records, beads),
	}
	if result.Generated {
		result.Status = OwnershipStatusGeneratedException
		result.Rationale = "generated Burpvalve evidence path"
		return result
	}
	if pathState == "untracked" && ignored {
		result.Status = OwnershipStatusIgnoredUntracked
		result.Rationale = "untracked path is ignored by git"
		return result
	}
	if len(records) == 0 {
		result.Status = OwnershipStatusUnowned
		return result
	}
	if hasOwnershipKind(records, OwnershipKindException) || hasOwnershipKind(records, OwnershipKindGenerated) {
		result.Status = OwnershipStatusCoveredException
		result.Rationale = firstRationale(records)
		result.Source = firstSource(records)
		return result
	}
	if ownershipSharedDeclared(records) {
		result.Status = OwnershipStatusSharedDeclared
		result.Rationale = firstRationale(records)
		result.Source = firstSource(records)
		return result
	}
	if len(uniqueOwnerUnits(records)) == 1 {
		result.Status = OwnershipStatusOwned
		result.Rationale = firstRationale(records)
		result.Source = firstSource(records)
		return result
	}
	result.Status = OwnershipStatusConflict
	result.Rationale = "multiple active units claim the same path incompatibly"
	return result
}

func ownershipBeadStatusActive(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "open", "in_progress":
		return true
	default:
		return false
	}
}

func ownershipBeadsByID(beads []OwnershipBeadContext) map[string]OwnershipBeadContext {
	if len(beads) == 0 {
		return nil
	}
	byID := make(map[string]OwnershipBeadContext, len(beads))
	for _, bead := range beads {
		if id := strings.TrimSpace(bead.ID); id != "" {
			byID[id] = bead
		}
	}
	return byID
}

func ownershipBeadContextForRecords(records []OwnershipRecord, beads map[string]OwnershipBeadContext) []OwnershipBeadContext {
	if len(records) == 0 || len(beads) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	var matched []OwnershipBeadContext
	for _, record := range records {
		for _, id := range []string{record.BeadID, record.UnitID} {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			bead, ok := beads[id]
			if !ok {
				continue
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			matched = append(matched, bead)
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].ID < matched[j].ID
	})
	return matched
}

func ownershipRecordsByPath(records []OwnershipRecord) map[string][]OwnershipRecord {
	byPath := map[string][]OwnershipRecord{}
	for _, record := range records {
		path := filepath.ToSlash(record.Path)
		byPath[path] = append(byPath[path], record)
	}
	return byPath
}

func ownershipSharedDeclared(records []OwnershipRecord) bool {
	if len(uniqueOwnerUnits(records)) < 2 {
		return false
	}
	for _, record := range records {
		switch record.OwnershipKind {
		case OwnershipKindFunction, OwnershipKindTest, OwnershipKindHunk:
		default:
			return false
		}
		if strings.TrimSpace(record.Rationale) == "" {
			return false
		}
	}
	return true
}

func uniqueOwnerUnits(records []OwnershipRecord) map[string]struct{} {
	units := map[string]struct{}{}
	for _, record := range records {
		unit := strings.TrimSpace(record.UnitID)
		if unit != "" {
			units[unit] = struct{}{}
		}
	}
	return units
}

func ownerUnitIDs(records []OwnershipRecord) []string {
	seen := uniqueOwnerUnits(records)
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func hasOwnershipKind(records []OwnershipRecord, kind OwnershipKind) bool {
	for _, record := range records {
		if record.OwnershipKind == kind {
			return true
		}
	}
	return false
}

func firstRationale(records []OwnershipRecord) string {
	for _, record := range records {
		if text := strings.TrimSpace(record.Rationale); text != "" {
			return text
		}
	}
	return ""
}

func firstSource(records []OwnershipRecord) string {
	for _, record := range records {
		if text := strings.TrimSpace(record.Source); text != "" {
			return text
		}
	}
	return ""
}

func summarizeOwnership(groups ...[]OwnershipPathResult) OwnershipSummary {
	var summary OwnershipSummary
	for i, group := range groups {
		if i == 0 {
			summary.StagedTotal = len(group)
		} else {
			summary.UntrackedTotal += len(group)
		}
		for _, item := range group {
			switch item.Status {
			case OwnershipStatusOwned:
				summary.Owned++
			case OwnershipStatusSharedDeclared:
				summary.SharedDeclared++
			case OwnershipStatusConflict:
				summary.Conflicts++
			case OwnershipStatusUnowned:
				summary.Unowned++
			case OwnershipStatusGeneratedException:
				summary.Generated++
			case OwnershipStatusIgnoredUntracked:
				summary.Ignored++
			case OwnershipStatusCoveredException:
				summary.Covered++
			}
		}
	}
	return summary
}
