#!/usr/bin/env bash
set -euo pipefail

if [[ -t 1 ]]; then
  c_agent=$'\033[38;5;42m'
  c_bv=$'\033[38;5;81m'
  c_verifier=$'\033[38;5;214m'
  c_git=$'\033[38;5;177m'
  c_dim=$'\033[2m'
  c_reset=$'\033[0m'
else
  c_agent=""
  c_bv=""
  c_verifier=""
  c_git=""
  c_dim=""
  c_reset=""
fi

say() {
  local color="$1"
  local label="$2"
  local text="$3"
  printf "%s%s>%s %s\n" "$color" "$label" "$c_reset" "$text"
}

agent_prompt() {
  printf "%sagent$%s " "$c_agent" "$c_reset"
  IFS= read -r _reply
}

answer_prompt() {
  printf "%sagent>%s " "$c_agent" "$c_reset"
  IFS= read -r _reply
}

say "$c_dim" "repo" "staged: main.go, config.go, README.md"
agent_prompt

say "$c_git" "git hook" "burpvalve commit"
say "$c_bv" "burpvalve" "Feature for this commit"
say "$c_bv" "burpvalve" "Staged paths cover code, config, and docs, so I will not guess."
say "$c_bv" "burpvalve" "What feature, bug fix, or bead id describes this staged commit?"
answer_prompt

say "$c_bv" "burpvalve" "Verifier cell 1/2"
say "$c_bv" "burpvalve" "Condition: lint-rules"
say "$c_bv" "burpvalve" "Did a dedicated read-only verifier check this exact condition? [y/N]"
answer_prompt

say "$c_bv" "burpvalve" "blocked: lint-rules has no dedicated subagent confirmation"
say "$c_bv" "burpvalve" "blocked report: log/backpressure/failed/blocked.json"
say "$c_bv" "burpvalve" "next: run the verifier, then rerun burpvalve commit"
printf "\n"

agent_prompt
say "$c_verifier" "verifier" "read-only lint verifier started for feature setup-defaults"
say "$c_verifier" "verifier" "go test ./... passed"
say "$c_verifier" "verifier" "gofmt check clean"
say "$c_verifier" "verifier" "verdict: pass"
printf "\n"

agent_prompt
say "$c_git" "git hook" "burpvalve commit"
say "$c_bv" "burpvalve" "Verifier cell 1/2"
say "$c_bv" "burpvalve" "Condition: lint-rules"
say "$c_bv" "burpvalve" "Dedicated verifier confirmation?"
answer_prompt
say "$c_bv" "burpvalve" "Verdict [pass|not_applicable|fail|unknown]"
answer_prompt
say "$c_bv" "burpvalve" "Evidence summary"
answer_prompt

say "$c_bv" "burpvalve" "Verifier cell 2/2"
say "$c_bv" "burpvalve" "Condition: scope-control"
say "$c_bv" "burpvalve" "Dedicated verifier confirmation?"
answer_prompt
say "$c_bv" "burpvalve" "Verdict [pass|not_applicable|fail|unknown]"
answer_prompt
say "$c_bv" "burpvalve" "Evidence summary"
answer_prompt

say "$c_bv" "burpvalve" "wrote passing attestation"
say "$c_bv" "burpvalve" "stage it with: git add attestation.json"
printf "\n"

agent_prompt
agent_prompt
say "$c_git" "git hook" "burpvalve commit"
say "$c_bv" "burpvalve" "staged backpressure attestation is valid"
say "$c_git" "git" "[main abc1234] add setup defaults"
say "$c_git" "git" "12 files changed, 312 insertions(+), 28 deletions(-)"
