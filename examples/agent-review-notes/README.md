# Add notes from a post-hoc agent review

Ask any agent command to emit review text, then send that text to the writable
local provider. `--message-file -` reads standard input:

```bash
codex exec 'Review this diff. Return one summary and rationale for line 41 of internal/cache.go.' \
  | changes note add \
      --file internal/cache.go \
      --line 41 \
      --origin agent \
      --author codex \
      --message-file -
```

The same contract works with `claude -p`, `ask`, or a local script. The first
non-empty output line becomes the summary. Remaining lines become the
rationale.

The agent command decides what to say. Changes verifies that the selected line
exists on the diff and records the comparison before the provider writes it.
