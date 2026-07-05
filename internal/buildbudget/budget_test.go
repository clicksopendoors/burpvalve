package buildbudget

import "testing"

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name string
		size int64
		want Level
	}{
		{name: "below warning", size: WarningBytes, want: LevelOK},
		{name: "above warning", size: WarningBytes + 1, want: LevelWarning},
		{name: "above failure", size: FailureBytes + 1, want: LevelFailure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Evaluate(tt.size, WarningBytes, FailureBytes)
			if got != tt.want {
				t.Fatalf("Evaluate(%d) = %s, want %s", tt.size, got, tt.want)
			}
		})
	}
}
