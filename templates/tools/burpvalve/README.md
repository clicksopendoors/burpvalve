# burpvalve Tool

The standard setup workflow expects the global `burpvalve` command to be on `PATH`.
Use `burpvalve init --repo-bin` or `burpvalve repair bin --force` only when this
repo needs an optional `bin/burpvalve` fallback for hook environments that cannot
see the global command.

This directory is reserved for projects that choose to replace the installed binary with a repo-local Go package. A README-only directory is a placeholder, not a runnable command.
