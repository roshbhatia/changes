# calldiff

Inspect the new validation call.

## Install

```sh
brew install roshbhatia/tap/changes-provider-calldiff
nix profile add 'github:roshbhatia/changes#provider-calldiff'
```

Install the core utility separately, or select its all-provider bundle. Runtime tools still need their own credentials.

## Demo

![Inspect the new validation call](demo.gif)

[Tape source](demo.tape) · [Task script](demo.sh)

Run `nix develop -c bash extras/calldiff/demo.sh` to run the task without recording.
Run `nix develop -c python3 hack/extra-demos.py calldiff` to record it.
