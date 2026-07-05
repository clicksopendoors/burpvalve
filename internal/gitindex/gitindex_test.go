package gitindex

import "testing"

func TestIsGeneratedEvidencePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "passing attestation json", path: "backpressure/attestations/hash.json", want: true},
		{name: "nested passing attestation json", path: "backpressure/attestations/nested/hash.json", want: true},
		{name: "blocked report json", path: "log/backpressure/failed/blocked.json", want: true},
		{name: "nested blocked report json", path: "log/backpressure/failed/nested/blocked.json", want: true},
		{name: "attestation readme", path: "backpressure/attestations/README.md", want: false},
		{name: "failed report readme", path: "log/backpressure/failed/README.md", want: false},
		{name: "attestation non-json", path: "backpressure/attestations/notes.md", want: false},
		{name: "failed report non-json", path: "log/backpressure/failed/notes.md", want: false},
		{name: "similar prefix", path: "backpressure/attestations-extra/hash.json", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsGeneratedEvidencePath(tt.path); got != tt.want {
				t.Fatalf("IsGeneratedEvidencePath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
