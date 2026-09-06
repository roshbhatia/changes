# Write notes from a harness session

Install the writable local provider beside the core:

```bash
nix profile install github:roshbhatia/changes#changes \
  github:roshbhatia/changes#provider-local-notes
```

Pass the harness session ID and use an agent origin. The line must be visible
on the selected diff side:

```bash
changes note add \
  --file internal/worker.go \
  --line 84 \
  --origin agent \
  --author harness \
  --session "$HARNESS_SESSION_ID" \
  --message "Preserve the retry boundary
The upstream call is not idempotent after this point."
```

Use `--staged` when the harness owns an index diff. Use `--commit <rev>` after
it creates a commit. Changes stores the resolved comparison IDs, patch
fingerprint, line context, and session ID with the note.

The local provider uses the existing `agentDiffNotes` XDG state record. Notes
created here remain available to the sysinit Neovim annotation view.
