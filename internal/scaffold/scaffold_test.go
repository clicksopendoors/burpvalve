package scaffold

import "testing"

func TestValidMode(t *testing.T) {
	tests := map[Mode]bool{
		ModeCheck:     true,
		ModeInit:      true,
		ModeRepair:    true,
		Mode("audit"): false,
		Mode(""):      false,
	}

	for mode, want := range tests {
		if got := ValidMode(mode); got != want {
			t.Fatalf("ValidMode(%q) = %v, want %v", mode, got, want)
		}
	}
}
