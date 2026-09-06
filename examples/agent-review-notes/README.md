# Generate notes with a review provider

[![Codex review provider demo](demo.gif)](demo.tape)

Install a generator and a writable store. Then run the generator explicitly:

```bash
nix profile install github:roshbhatia/changes#changes \
  github:roshbhatia/changes#provider-codex-review \
  github:roshbhatia/changes#provider-local-notes
changes note generate \
  --provider codex-review \
  --store local-notes
```

The generator receives the selected patch and returns structured advisory
notes through `changes.notes.generate`. Changes checks each path and diff line
before it sends every note to one atomic `changes.notes.create` request. The
generator must return the same note ID for the same semantic finding on an
equivalent request. That ID becomes an idempotency key, so a retry returns the
existing stored note instead of creating a duplicate.

The Codex provider uses non-interactive, ephemeral execution with a JSON output
schema. Its isolated permission profile grants only minimal runtime reads and
repository reads. It denies model-command network access. The provider ignores
user configuration and execution rules, and it disables repository instruction
discovery. It sends the patch as escaped JSON data. Normal rendering and watch
mode never run a generator. Another agent can implement the same provider
action without changing the store or diff viewer.

Replay the [VHS tape](demo.tape) from the repository root:

```bash
nix develop -c ./hack/example-demos.sh agent-review-notes
```
