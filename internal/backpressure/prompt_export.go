package backpressure

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const PromptExportDir = "docs/prompts"

type PromptExportResult struct {
	Path          string `json:"path"`
	PromptName    string `json:"prompt_name"`
	ContentHash   string `json:"content_hash"`
	Written       bool   `json:"written"`
	Divergent     bool   `json:"divergent"`
	LocalModified bool   `json:"local_modified"`
}

type PromptExportStatus struct {
	Path          string
	Exists        bool
	ContentHash   string
	Divergent     bool
	LocalModified bool
}

func PromptExportPath(root string, name string) string {
	return filepath.Join(root, PromptExportDir, name+".md")
}

func PromptBodyHash(body string) string {
	sum := sha256.Sum256([]byte(body))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func PromptExportStatusFor(root string, rendered PromptShowOutput) (PromptExportStatus, error) {
	path := PromptExportPath(root, rendered.Name)
	status := PromptExportStatus{Path: path, ContentHash: PromptBodyHash(rendered.Body)}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return status, nil
		}
		return status, fmt.Errorf("read prompt export %s: %w", path, err)
	}
	status.Exists = true
	local, err := parsePromptExport(data)
	if err != nil {
		status.Divergent = true
		status.LocalModified = true
		return status, nil
	}
	status.LocalModified = local.StoredHash != PromptBodyHash(local.Body)
	status.Divergent = status.LocalModified || local.Name != rendered.Name || local.Body != rendered.Body || local.StoredHash != status.ContentHash
	return status, nil
}

func WritePromptExport(root string, rendered PromptShowOutput, burpvalveVersion string, force bool) (PromptExportResult, error) {
	status, err := PromptExportStatusFor(root, rendered)
	if err != nil {
		return PromptExportResult{}, err
	}
	result := PromptExportResult{
		Path:          status.Path,
		PromptName:    rendered.Name,
		ContentHash:   status.ContentHash,
		Divergent:     status.Divergent,
		LocalModified: status.LocalModified,
	}
	if status.Exists && status.LocalModified && !force {
		return result, fmt.Errorf("refusing to overwrite locally modified prompt export %s; rerun with --force to replace it with the embedded canonical prompt", status.Path)
	}
	data := formatPromptExport(rendered, burpvalveVersion)
	if existing, readErr := os.ReadFile(status.Path); readErr == nil && string(existing) == data {
		return result, nil
	}
	if err := os.MkdirAll(filepath.Dir(status.Path), 0o755); err != nil {
		return result, fmt.Errorf("create prompt export directory: %w", err)
	}
	if err := os.WriteFile(status.Path, []byte(data), 0o644); err != nil {
		return result, fmt.Errorf("write prompt export %s: %w", status.Path, err)
	}
	result.Written = true
	return result, nil
}

type parsedPromptExport struct {
	Name       string
	StoredHash string
	Body       string
}

func formatPromptExport(rendered PromptShowOutput, burpvalveVersion string) string {
	body := strings.TrimRight(rendered.Body, "\n")
	return fmt.Sprintf("---\nburpvalve_version: %q\nprompt_name: %q\ncontent_hash: %q\n---\n\n%s\n", burpvalveVersion, rendered.Name, PromptBodyHash(rendered.Body), body)
}

func parsePromptExport(data []byte) (parsedPromptExport, error) {
	text := string(data)
	if !strings.HasPrefix(text, "---\n") {
		return parsedPromptExport{}, fmt.Errorf("prompt export metadata header missing")
	}
	rest := strings.TrimPrefix(text, "---\n")
	header, body, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		return parsedPromptExport{}, fmt.Errorf("prompt export metadata header unterminated")
	}
	body = strings.TrimPrefix(body, "\n")
	body = strings.TrimRight(body, "\n")
	meta := map[string]string{}
	for _, line := range strings.Split(header, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		meta[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	name := meta["prompt_name"]
	hash := meta["content_hash"]
	if name == "" || hash == "" {
		return parsedPromptExport{}, fmt.Errorf("prompt export metadata missing prompt_name or content_hash")
	}
	return parsedPromptExport{Name: name, StoredHash: hash, Body: body}, nil
}
