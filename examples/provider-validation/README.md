# Validate analysis providers

[![Provider validation demo](demo.gif)](demo.tape)

Install an optional provider package. Its manifest is discovered from the
package's XDG data directory. Then validate the provider against a synthetic
repository:

```bash
nix profile install github:roshbhatia/changes#changes \
  github:roshbhatia/changes#provider-ast-grep
changes provider list
changes provider validate
```

Use an explicit directory to test local or third-party manifests:

```bash
CHANGES_PROVIDERS_DIRECTORY="$HOME/.config/changes/providers" \
  changes provider validate provider-name
```

The probe does not inspect or modify the current repository. It fails when a
declared dependency is absent, the protocol is invalid, or the result has no
semantic data for its advertised action. The probe covers logical groups,
symbols, calls, note readers, note writers, and note generators.

Replay the [VHS tape](demo.tape) from the repository root:

```bash
nix develop -c ./hack/example-demos.sh provider-validation
```
