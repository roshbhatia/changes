# Validate analysis providers

Install the full Changes package, then validate both provider commands against
a synthetic working tree:

```bash
CHANGES_CONFIG="$PWD/examples/provider-validation/config.yaml" changes provider list
CHANGES_CONFIG="$PWD/examples/provider-validation/config.yaml" changes provider validate
```

The probe does not inspect or modify the current repository.
