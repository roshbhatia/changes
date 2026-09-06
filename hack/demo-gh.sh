#!/usr/bin/env bash
set -euo pipefail

if [[ ${1:-} == pr && ${2:-} == view ]]; then
  printf '{"number":17,"baseRefOid":"%s","headRefOid":"%s","url":"https://github.com/example/changes-demo/pull/17"}\n' \
    "${DEMO_BASE_SHA:?}" "${DEMO_HEAD_SHA:?}"
  exit 0
fi

if [[ ${1:-} == api && ${2:-} == --hostname ]]; then
  printf '%s\n' "${DEMO_BASE_SHA:?}"
  exit 0
fi

if [[ ${1:-} == api && ${2:-} == graphql ]]; then
  printf '%s\n' \
    '{"data":{"repository":{"pullRequest":{"reviewThreads":{' \
    '"nodes":[{"id":"thread-1","isOutdated":false,"isResolved":false,' \
    '"path":"main.go","diffSide":"RIGHT","startDiffSide":null,' \
    '"startLine":null,"line":8,"originalStartLine":null,"originalLine":5,' \
    '"subjectType":"LINE","comments":{"nodes":[{' \
    '"id":"comment-1","body":"Keep the fallback explicit\nCallers rely on this empty-input behavior.",' \
    '"url":"https://github.com/example/changes-demo/pull/17#discussion_r1",' \
    '"createdAt":"2026-09-05T01:00:00Z","updatedAt":"2026-09-05T01:00:00Z",' \
    '"author":{"login":"reviewer"},"replyTo":null,' \
    '"commit":{"oid":"demo-head"},"originalCommit":{"oid":"demo-head"}}],' \
    '"pageInfo":{"hasNextPage":false,"endCursor":""}}}],' \
    '"pageInfo":{"hasNextPage":false,"endCursor":""}' \
    '}}}}}'
  exit 0
fi

echo "ERROR: unsupported demo gh command: $*" >&2
exit 2
