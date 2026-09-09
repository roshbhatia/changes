# codex-review

Replay an offline token parser review.

The demo replays an offline response fixture. It does not contact a model or claim a new agent run.

## Install

```sh
brew install roshbhatia/tap/changes-provider-codex-review
nix profile add 'github:roshbhatia/changes#provider-codex-review'
```

Install the core utility separately, or select its all-provider bundle. Runtime tools still need their own credentials.

Install and authenticate Codex before generating review notes. On macOS, use `brew install --cask codex`. The Nix package includes the CLI.

## Demo

![Replay an offline token parser review](demo.gif)

[Tape source](demo.tape) · [Task script](demo.sh)

Run `nix develop -c bash extras/codex-review/demo.sh` to run the task without recording.
Run `nix develop -c python3 hack/extra-demos.py codex-review` to record it.
