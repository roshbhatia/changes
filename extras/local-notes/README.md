# local-notes

Keep a local note on prefix validation.

## Install

```sh
brew install roshbhatia/tap/changes-provider-local-notes
nix profile add 'github:roshbhatia/changes#provider-local-notes'
```

Install the core utility separately, or select its all-provider bundle. Runtime tools still need their own credentials.

## Demo

![Keep a local note on prefix validation](demo.gif)

[Tape source](demo.tape) · [Task script](demo.sh)

Run `nix develop -c bash extras/local-notes/demo.sh` to run the task without recording.
Run `nix develop -c python3 hack/extra-demos.py local-notes` to record it.
