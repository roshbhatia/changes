#!/usr/bin/env bash
set -euo pipefail

if [[ ${1:-} == pr && ${2:-} == view ]]; then
  printf '{"number":17,"baseRefOid":"%s","headRefOid":"%s","url":"https://github.com/replay-fixtures/checkout-service/pull/17"}\n' \
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
    '"path":"internal/auth/token.go","diffSide":"RIGHT","startDiffSide":null,' \
    '"startLine":null,"line":6,"originalStartLine":null,"originalLine":5,' \
    '"subjectType":"LINE","comments":{"nodes":[{' \
    '"id":"comment-1","body":"Require the Bearer prefix\nReject an embedded prefix before session lookup.",' \
    '"url":"https://github.com/replay-fixtures/checkout-service/pull/17#discussion_r1",' \
    '"createdAt":"2026-09-05T01:00:00Z","updatedAt":"2026-09-05T01:00:00Z",' \
    '"author":{"login":"reviewer"},"replyTo":null,' \
    '"commit":{"oid":"recorded-head"},"originalCommit":{"oid":"recorded-head"}}],' \
    '"pageInfo":{"hasNextPage":false,"endCursor":""}}}],' \
    '"pageInfo":{"hasNextPage":false,"endCursor":""}' \
    '}}}}}' | jq --arg head "${DEMO_HEAD_SHA:?}" '.data.repository.pullRequest.reviewThreads.nodes[].comments.nodes[] |= (.commit.oid = $head | .originalCommit.oid = $head)'
  exit 0
fi

echo "ERROR: unsupported demo gh command: $*" >&2
exit 2
