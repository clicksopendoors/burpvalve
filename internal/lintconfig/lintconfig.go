package lintconfig

// Command describes one project-declared lint, format, or static-analysis check.
type Command struct {
	ID             string   `json:"id" yaml:"id"`
	Command        string   `json:"command" yaml:"command"`
	Required       bool     `json:"required" yaml:"required"`
	Paths          []string `json:"paths" yaml:"paths"`
	TimeoutSeconds int      `json:"timeout_seconds" yaml:"timeout_seconds"`
	RunDirectory   string   `json:"run_directory,omitempty" yaml:"run_directory,omitempty"`
	Serial         bool     `json:"serial,omitempty" yaml:"serial,omitempty"`
}

type Coverage struct {
	DeclinedRoots []string `json:"declined_roots,omitempty" yaml:"declined_roots,omitempty"`
	DeclinedAt    string   `json:"declined_at,omitempty" yaml:"declined_at,omitempty"`
}
