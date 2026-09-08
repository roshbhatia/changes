// Package provider narrows the canonical provider/v1 contract to Changes.
//
// The contract itself is provider.cue in roshbhatia/provider-spec, pinned as
// the provider-spec flake input. This file adds only the Changes rule: a
// provider declares at least one Changes action, which is what provider
// validate probes at runtime. Vet a manifest with both files:
//
//	cue vet -d '#Manifest' "$PROVIDER_SPEC/provider.cue" schema/narrow.cue extras/calldiff/provider.yaml
package provider

import "list"

#ActionNames: [
	"changes.calls",
	"changes.groups",
	"changes.notes",
	"changes.notes.create",
	"changes.notes.generate",
	"changes.symbols",
]

#Manifest: {
	actions: _
	// The spec admits any lowercase action name, so the hidden field collects
	// the Changes ones and requires at least one. The guard keeps the
	// definition itself valid: with no data unified, actions holds only the
	// spec's pattern constraint and iterates as empty. A manifest with no
	// actions at all is the runtime's rule, as the spec states.
	if len(actions) > 0 {
		_changes: list.MinItems(1) & [for name, _ in actions if list.Contains(#ActionNames, name) {name}]
	}
}
