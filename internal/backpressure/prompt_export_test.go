package backpressure

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptExportContentHashDetectsLocalModification(t *testing.T) {
	root := t.TempDir()
	rendered, err := ShowPrompt("marching-orders", map[string]string{
		"agent": "LilacGlacier",
		"bead":  "burpvalve-oxp-prompt-export-divergence-9le",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := WritePromptExport(root, rendered, "test-version", false)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Written || first.ContentHash != PromptBodyHash(rendered.Body) {
		t.Fatalf("unexpected first export: %#v", first)
	}
	path := PromptExportPath(root, rendered.Name)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `burpvalve_version: "test-version"`) || !strings.Contains(string(body), `prompt_name: "marching-orders"`) || !strings.Contains(string(body), `content_hash: "sha256:`) {
		t.Fatalf("export missing metadata header:\n%s", body)
	}
	if err := os.WriteFile(path, []byte(string(body)+"local note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := PromptExportStatusFor(root, rendered)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Exists || !status.Divergent || !status.LocalModified {
		t.Fatalf("local edit should be divergent and modified: %#v", status)
	}
	if _, err := WritePromptExport(root, rendered, "test-version", false); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("modified export should require --force, got %v", err)
	}
	forced, err := WritePromptExport(root, rendered, "test-version", true)
	if err != nil {
		t.Fatal(err)
	}
	if !forced.Written || !forced.LocalModified {
		t.Fatalf("force should overwrite known local modification: %#v", forced)
	}
	after, err := os.ReadFile(filepath.Join(root, PromptExportDir, "marching-orders.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(after), "local note") {
		t.Fatalf("force overwrite preserved local note:\n%s", after)
	}
}

func TestPromptExportIdempotentWhenUnmodified(t *testing.T) {
	root := t.TempDir()
	rendered, err := ShowPrompt("verifier-brief", map[string]string{"feature": "br-123"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WritePromptExport(root, rendered, "test-version", false); err != nil {
		t.Fatal(err)
	}
	second, err := WritePromptExport(root, rendered, "test-version", false)
	if err != nil {
		t.Fatal(err)
	}
	if second.Written || second.LocalModified || second.Divergent {
		t.Fatalf("unmodified re-export should be unchanged: %#v", second)
	}
}
