package backpressure

import (
	"strings"
	"testing"
)

func TestStyleLinePromptUsesColorWhenEnabled(t *testing.T) {
	got := styleLinePrompt("Atomicity: does this pass? [y/N]: ", true)
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("styled prompt missing ANSI: %q", got)
	}
	if !strings.Contains(got, "Atomicity:") || !strings.Contains(got, "does this pass?") {
		t.Fatalf("styled prompt lost text: %q", got)
	}
}

func TestStyleLinePromptStaysPlainWhenColorDisabled(t *testing.T) {
	const prompt = "Atomicity: does this pass? [y/N]: "
	if got := styleLinePrompt(prompt, false); got != prompt {
		t.Fatalf("plain prompt changed:\ngot  %q\nwant %q", got, prompt)
	}
}
