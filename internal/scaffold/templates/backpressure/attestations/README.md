# Commit Backpressure Attestations

Tracked passing commit attestations live here. Each successful feature or bug-fix commit must include one JSON artifact binding the atomic work item, staged payload hash, enabled backpressure conditions, subagent confirmations, verdicts, and messages.

Burpvalve writes these files as generated, indented JSON with a trailing newline. If a broad project formatter rejects generated evidence, exclude `backpressure/attestations/*.json` from that formatter instead of hand-editing the attestation.

Do not put blocked-attempt reports here. Blocked attempts belong in `/log/backpressure/failed/`.
