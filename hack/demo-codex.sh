#!/usr/bin/env bash
set -euo pipefail

output=""
schema=""
ignore_config=false
ignore_rules=false
strict_config=false
ephemeral=false
read_profile=false
filesystem_read=false
network_denied=false
approval_never=false
project_docs_disabled=false
while [[ $# -gt 0 ]]; do
  case "$1" in
  --output-last-message)
    output=${2:?}
    shift 2
    ;;
  --output-schema)
    schema=${2:?}
    shift 2
    ;;
  --ignore-user-config)
    ignore_config=true
    shift
    ;;
  --ignore-rules)
    ignore_rules=true
    shift
    ;;
  --strict-config)
    strict_config=true
    shift
    ;;
  --ephemeral)
    ephemeral=true
    shift
    ;;
  -c)
    case "$2" in
    'approval_policy="never"') approval_never=true ;;
    'project_doc_max_bytes=0') project_docs_disabled=true ;;
    'default_permissions="changes-review"') read_profile=true ;;
    'permissions.changes-review.filesystem={":minimal"="read",":workspace_roots"={"."="read"}}') filesystem_read=true ;;
    'permissions.changes-review.network.enabled=false') network_denied=true ;;
    esac
    shift 2
    ;;
  --cd | --color)
    shift 2
    ;;
  *)
    shift
    ;;
  esac
done

if [[ -z $output || -z $schema ]]; then
  echo "ERROR: demo Codex invocation omitted structured output paths" >&2
  exit 2
fi
if [[ $ignore_config != true || $ignore_rules != true || $strict_config != true || $ephemeral != true ||
  $approval_never != true || $project_docs_disabled != true || $read_profile != true ||
  $filesystem_read != true || $network_denied != true ]]; then
  echo "ERROR: demo Codex invocation omitted its review boundary" >&2
  exit 2
fi
jq empty "$schema"

printf '%s\n' \
  '{"notes":[{"path":"main.go","side":"RIGHT","startLine":0,"line":8,' \
  '"summary":"Keep the fallback explicit",' \
  '"rationale":"Callers rely on this empty-input behavior."}]}' \
  >"$output"
