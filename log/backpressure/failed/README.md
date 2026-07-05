# Failed Backpressure Attempts

Local blocked-attempt reports live here. These files help an agent recover from a failed pre-commit check, but they are not passing commit attestations and should not be treated as evidence that a commit is ready.

Burpvalve writes blocked reports as generated, indented JSON with a trailing newline. If a broad project formatter rejects generated evidence, exclude `log/backpressure/failed/*.json` from that formatter instead of hand-editing the report.
