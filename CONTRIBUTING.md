# Contributing

Issues, bug reports, and focused pull requests are welcome.

Burpvalve is maintained as an opinionated tool. Contributions are reviewed for
fit with the project contract, backpressure model, release process, and public
roadmap. The maintainer may adapt, rewrite, split, defer, or decline submitted
changes after review.

## Good Contributions

- Bug reports with a clear reproduction, expected behavior, and actual output.
- Focused fixes that include tests or a reason tests do not apply.
- Documentation improvements that make installation, gate behavior, or
  verifier evidence easier to understand.
- Small compatibility improvements that preserve existing CLI flags, JSON
  fields, attestation paths, and generated scaffold contracts.

## Before Opening A Pull Request

1. Keep the change scoped to one work unit.
2. Run `go test ./...`.
3. Run `make build` when touching CLI behavior or release packaging.
4. Run `jsm validate skill/burpvalve` when touching the packaged skill.
5. Avoid Go module installer instructions for Burpvalve; the public install
   path is the release package and installer.
6. Do not add AI co-authorship trailers. The author identity belongs to the
   human contributor.

## Project Boundaries

Do not include secrets, local machine paths, non-public repository names, or
machine-specific history in examples, logs, tests, screenshots, or generated
artifacts.

Burpvalve publishes release assets for Linux and macOS on amd64 and arm64.

## Maintainer Policy

Opening an issue or pull request does not guarantee acceptance or a particular
response path. The maintainer may close stale, broad, duplicate, unsafe, or
out-of-scope work to keep the project focused.
