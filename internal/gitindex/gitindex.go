package gitindex

import "path/filepath"

var generatedPathPrefixes = []string{
	"backpressure/attestations/",
	"log/backpressure/failed/",
}

// GeneratedPathPrefixes returns directories that can contain generated evidence.
func GeneratedPathPrefixes() []string {
	prefixes := make([]string, len(generatedPathPrefixes))
	copy(prefixes, generatedPathPrefixes)
	return prefixes
}

// IsGeneratedEvidencePath reports whether path is a Burpvalve-generated
// evidence artifact that should be excluded from payload identity.
func IsGeneratedEvidencePath(path string) bool {
	path = filepath.ToSlash(path)
	switch {
	case isGeneratedJSONUnder(path, "backpressure/attestations/"):
		return true
	case isGeneratedJSONUnder(path, "log/backpressure/failed/"):
		return true
	default:
		return false
	}
}

func isGeneratedJSONUnder(path, prefix string) bool {
	if len(path) <= len(prefix) || path[:len(prefix)] != prefix {
		return false
	}
	name := path[len(prefix):]
	if name == "" || name == "README.md" {
		return false
	}
	return filepath.Ext(name) == ".json"
}
