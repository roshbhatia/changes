#!/usr/bin/env bash
set -euo pipefail

codex_command=${1:?usage: check-codex-permissions.sh CODEX}
result_file=$(mktemp)
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"; rm -f "$result_file"' EXIT

printf '%s\n' CHANGES_PROJECT_INSTRUCTION_SENTINEL >"$fixture/AGENTS.md"
"$codex_command" -C "$fixture" debug prompt-input probe |
  grep -Fq CHANGES_PROJECT_INSTRUCTION_SENTINEL
if "$codex_command" -C "$fixture" -c project_doc_max_bytes=0 debug prompt-input probe |
  grep -Fq CHANGES_PROJECT_INSTRUCTION_SENTINEL; then
  echo "project_doc_max_bytes=0 still loaded AGENTS.md" >&2
  exit 1
fi

if printf '%s\n' 'config validation only' | "$codex_command" exec \
  --ignore-user-config \
  --ignore-rules \
  --strict-config \
  --cd . \
  -c 'approval_policy="never"' \
  -c 'project_doc_max_bytes=0' \
  -c 'default_permissions="changes-review"' \
  -c 'permissions.changes-review.description="Read only diff review"' \
  -c 'permissions.changes-review.filesystem={":minimal"="read",":workspace_roots"={"."="read"}}' \
  -c 'permissions.changes-review.network.enabled=false' \
  -c 'model_provider="changes-config-validation"' \
  --ephemeral \
  --color never \
  - >"$result_file" 2>&1; then
  echo "Codex accepted a missing validation provider" >&2
  exit 1
fi

grep -Fq "Model provider \`changes-config-validation\` not found" "$result_file"
