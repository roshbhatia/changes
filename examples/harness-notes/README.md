# Write notes from a harness session

[![Harness notes demo](demo.gif)](demo.tape)

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

The local provider stores notes in the user's XDG state directory. It does not
change Git refs, contact a remote, or push notes.

[Git notes](https://git-scm.com/docs/git-notes) could back a separate store for
committed comparisons. They attach data to one Git object, so the payload must
still record both comparison IDs.
Changes does not bundle that store. A default branch push does not include a
notes ref. Sharing it would require an explicit command such as:

```bash
git push origin refs/notes/changes:refs/notes/changes
```

Any future Git-notes provider should keep fetch and push explicit.

Replay the [VHS tape](demo.tape) from the repository root:

```bash
nix develop -c ./hack/example-demos.sh harness-notes
```
