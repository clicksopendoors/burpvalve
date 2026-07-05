package main

import (
	"strings"
	"testing"
	"time"

	"burpvalve/internal/attestations"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestAttestationBrowserRendersListDetailAndHelp(t *testing.T) {
	records := attestationBrowserFixtureRecords(t)
	model := newAttestationBrowserModel(records, false)
	model.width = 110
	model.height = 24
	model.showHelp = true

	view := model.render()
	for _, needle := range []string{
		"Burpvalve attestations",
		"pass",
		"blocked",
		"br-pass",
		"br-blocked",
		"passhash",
		"log/backpressure/failed/blocked.json",
		"Conditions",
		"independent_required",
		"verifier test",
		"Equivalent JSON",
	} {
		if !strings.Contains(view, needle) {
			t.Fatalf("browser view missing %q:\n%s", needle, view)
		}
	}
	assertMaxTUILineWidth(t, view, 112)
}

func TestAttestationBrowserMovementFilteringAndDetail(t *testing.T) {
	records := attestationBrowserFixtureRecords(t)
	model := newAttestationBrowserModel(records, false)
	model.width = 100
	model.height = 24

	updated, _ := model.Update(testKey("enter"))
	model = updated.(attestationBrowserModel)
	if !model.detail || !strings.Contains(model.render(), "burpvalve explain 'log/backpressure/failed/blocked.json'") {
		t.Fatalf("enter should show blocked detail with explain command:\n%s", model.render())
	}
	updated, _ = model.Update(testKey("esc"))
	model = updated.(attestationBrowserModel)
	updated, _ = model.Update(testKey("down"))
	model = updated.(attestationBrowserModel)
	if model.cursor != 1 {
		t.Fatalf("down should move cursor, got %d", model.cursor)
	}
	updated, _ = model.Update(testKey("/"))
	model = updated.(attestationBrowserModel)
	updated, _ = model.Update(testText("br-pass"))
	model = updated.(attestationBrowserModel)
	if len(model.filtered) != 1 || model.records[model.filtered[0]].Status != "pass" {
		t.Fatalf("search should filter to passing record: filtered=%#v", model.filtered)
	}
}

func TestAttestationBrowserWarningsEmptyNarrowAndNoColor(t *testing.T) {
	records := attestationBrowserFixtureRecords(t)
	model := newAttestationBrowserModel(records, false)
	model.width = 72
	model.height = 16
	model.cursor = 2
	model.detail = true

	view := model.render()
	for _, needle := range []string{"malformed", "Warnings", "unexpected end of JSON input"} {
		if !strings.Contains(view, needle) {
			t.Fatalf("warning detail missing %q:\n%s", needle, view)
		}
	}
	if strings.Contains(view, "\x1b[") {
		t.Fatalf("no-color TUI render should not include ANSI escapes:\n%s", view)
	}
	assertMaxTUILineWidth(t, view, 76)

	empty := newAttestationBrowserModel(nil, false)
	empty.width = 70
	emptyView := empty.render()
	if !strings.Contains(emptyView, "No attestation artifacts found") {
		t.Fatalf("empty state missing:\n%s", emptyView)
	}
}

func TestAttestationsBrowseNonInteractiveDoesNotStartTUI(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("attestations", "browse")
	if err == nil || !strings.Contains(err.Error(), "requires an interactive terminal") {
		t.Fatalf("noninteractive browse should fail quickly, err=%v stdout=%s stderr=%s", err, stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("noninteractive browse should not render TUI stdout:\n%s", stdout)
	}
}

func TestAttestationsRootNonInteractiveShowsHelp(t *testing.T) {
	stdout, stderr, err := executeBurpvalveCommand("attestations")
	if err != nil {
		t.Fatalf("attestations help failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "browse") || !strings.Contains(stdout, "list") || !strings.Contains(stdout, "show") {
		t.Fatalf("attestations help should name browser and JSON-friendly commands:\n%s", stdout)
	}
	if stderr != "" {
		t.Fatalf("attestations help wrote stderr: %s", stderr)
	}
}

func attestationBrowserFixtureRecords(t *testing.T) []attestations.Record {
	t.Helper()
	root := t.TempDir()
	created := time.Date(2026, 6, 29, 3, 0, 0, 0, time.UTC)
	writeAttestationQueryFixture(t, root, "backpressure/attestations/passhash.json", attestationQueryArtifactFixture(attestations.ArtifactPassing, "passhash", "br-pass", created))
	writeAttestationQueryFixture(t, root, "log/backpressure/failed/blocked.json", attestationQueryArtifactFixture(attestations.ArtifactBlocked, "blockedhash", "br-blocked", created.Add(time.Minute)))
	writeCmdTestFile(t, root+"/backpressure/attestations/bad.json", `{"not valid"`)
	records, err := attestations.List(root, attestations.QueryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("fixture records = %d, want 3: %#v", len(records), records)
	}
	return records
}

func testKey(key string) tea.KeyPressMsg {
	switch key {
	case "down":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})
	case "enter":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	case "/":
		return tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"})
	default:
		return tea.KeyPressMsg(tea.Key{Code: []rune(key)[0], Text: key})
	}
}

func testText(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyExtended, Text: text})
}

func assertMaxTUILineWidth(t *testing.T, view string, maxWidth int) {
	t.Helper()
	for _, line := range strings.Split(view, "\n") {
		if width := lipgloss.Width(line); width > maxWidth {
			t.Fatalf("line width %d exceeds %d:\n%s\nfull view:\n%s", width, maxWidth, line, view)
		}
	}
}
