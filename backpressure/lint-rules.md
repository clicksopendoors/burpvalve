# Lint Rules

Purpose: project-specific formatting, linting, static analysis, and style gates. Add exact commands and failure-handling rules here.

Suggested policy candidates to enable when the project has tooling for them:

- cap nested `if`/loop depth;
- prefer early returns over deep nesting;
- forbid unexplained magic numbers;
- require descriptive function, method, variable, and package names;
- set minimum identifier length, with explicit exceptions for idiomatic short names such as loop indexes or coordinates;
- cap function length and cyclomatic/cognitive complexity;
- require errors to be handled explicitly;
- reject dead code, unused exports, and broad catch-all exception handling;
- require exact lint/format/test commands before a rule becomes enforced.

Setup should create this as a policy wishlist, not as fake enforcement. A rule is enforced only after this file names the command or analyzer that checks it.

Important distinction:

- `lint-rules` as a backpressure condition asks a subagent to verify that applicable lint/style rules were considered for the feature.
- `burpvalve lint` runs executable lint/format/static-analysis commands declared by the project.

The first is an attestation. The second is a command runner. A commit should satisfy both when executable lint commands exist.
