# ast-grep

Inspect the changed token parser symbol.

## Install

```sh
brew install roshbhatia/tap/changes-provider-ast-grep
nix profile add 'github:roshbhatia/changes#provider-ast-grep'
```

Install the core utility separately, or select its all-provider bundle. Runtime tools still need their own credentials.

## Demo

![Inspect the changed token parser symbol](demo.gif)

[Tape source](demo.tape) · [Task script](demo.sh)

Run `nix develop -c bash extras/ast-grep/demo.sh` to run the task without recording.
Run `nix develop -c python3 hack/extra-demos.py ast-grep` to record it.
