# Validate analysis providers

Install the full Changes package. Its reference manifests are discovered from
the package's XDG data directory. Then validate both provider commands against
a synthetic Git repository:

```bash
changes provider list
changes provider validate
```

Use an explicit directory to test local or third-party manifests:

```bash
CHANGES_PROVIDERS_DIRECTORY="$HOME/.config/changes/providers" \
  changes provider validate ast-grep
```

The probe does not inspect or modify the current repository. It fails when a
declared dependency is absent, the protocol is invalid, or the result has no
semantic data for its advertised action.
