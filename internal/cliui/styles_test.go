package cliui

import (
	"strings"
	"testing"
)

func TestStylesUseLightAndDarkPalettes(t *testing.T) {
	light := NewWithDarkBackground(true, false).Title("Burpvalve")
	dark := NewWithDarkBackground(true, true).Title("Burpvalve")
	if light == dark {
		t.Fatal("light and dark palettes should render different ANSI styles")
	}
	if !strings.Contains(light, "\x1b[") || !strings.Contains(dark, "\x1b[") {
		t.Fatalf("styled output should include ANSI escapes:\nlight=%q\ndark=%q", light, dark)
	}
}

func TestStylesStayPlainWhenColorDisabled(t *testing.T) {
	got := NewWithDarkBackground(false, false).Error("blocked")
	if got != "blocked" {
		t.Fatalf("plain output changed: %q", got)
	}
}
