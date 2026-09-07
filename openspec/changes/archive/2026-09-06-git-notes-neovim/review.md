# Adversarial review

## Rubric

The review used the proposal behavior, non-goals, design decisions, and rollout gate. It tested exact ref isolation, canonical mergeable storage, atomic writes, remote boundaries, editor line identity, shell-free execution, packaging, and proof.

`specutil check git-notes-neovim` passed before every reviewed revision. Three fresh read-only critics used separate lenses in each round. A fourth fresh read-only mediator accepted, rejected, reframed, or deferred each objection.

## Result

Terminal state: `CAPPED` after eleven owner-authorized rounds.

The surviving-objection trend was `6 → 6 → 5 → 2 → 7 → 6 → 3 → 7 → 6 → 6 → 3`. Every accepted Round 11 objection has a revision and focused regression test. Per owner direction, the review stops without another mediated round, so this result is not `CLEAN`.

This review is model evidence. It does not replace owner approval. The standing Specutil owner decision remains separate.

## Rounds

### Round 1

Six objections survived mediation:

- Protect external Git notes updates with compare-and-swap publication.
- Reject unsaved Neovim buffers.
- Disable lazy fetches.
- Preserve the original buffer, repository, and range across asynchronous input.
- Include Lua sources in demo freshness.
- Install the Git notes provider in its committed-note example.

The revision added CAS publication, dirty-buffer checks, `GIT_NO_LAZY_FETCH`, captured editor targets, complete demo fingerprints, and corrected installation steps.

### Round 2

Five objections were accepted and one was reframed:

- Scrub inherited Git repository and object-database overrides.
- Reject records attached under the wrong head.
- Lock provider reads with writes.
- Reject disk changes made during popup input.
- Treat demo freshness as a release-sequencing gate.
- Keep local notes as the default writer and install Git notes separately.

The revision implemented each accepted or reframed item.

### Round 3

Four objections were accepted, one was reframed, and one duplicate was rejected:

- Avoid applying the per-document limit to the complete notes tree.
- Bound Neovim file hashing.
- Read the exact captured notes commit through an ABA ref movement.
- Narrow absolute race-free wording to observable stable capture.
- Disable replacement refs.

The revision added targeted fanout lookup, a 16 MiB editor limit, captured-tree reads, replacement-ref isolation, and precise stability language.

### Round 4

Two unique objections survived:

- Reject symbolic `refs/notes/changes` and publish through non-dereferencing CAS.
- Bind the saved editor file to the selected working, index, or commit side.

The revision added symbolic-ref rejection, `update-ref --no-deref`, the `--expected-file-sha256` CLI contract, selected-side reads, and positive and negative side tests.

### Round 5

Six objections were accepted and release freshness was reframed as the planned gate:

- Require exact ref lookup instead of prefix lookup.
- Scope stable keys and IDs to the complete comparison.
- Reject noncanonical request object IDs.
- Prove buffer bytes equal disk bytes.
- Disable text conversion and reject unsupported working-tree conversions and file types.
- Exercise commit and staged digest checks through the real CLI boundary.

The revision added exact ref filtering, comparison-scoped identity, canonical request validation, buffer serialization, byte-domain restrictions, and subprocess coverage.

### Round 6

Six objections survived mediation:

- Validate stored keyed and random IDs semantically.
- Validate stored base and head values as canonical commits.
- Replace descendant enumeration with bounded exact ref lookup.
- Reject dangling symbolic notes refs.
- Repeat endpoint IDs, patch content, and selected digest as one stability tuple.
- Distinguish empty files from one-newline files and cover supported newline forms.

The revision added derived-ID validation, batched stored-commit validation, a symbolic probe plus exact ref resolution, tuple capture, and empty, newline, no-EOL, and CRLF editor tests.

### Round 7

Two objections were accepted and one was reframed:

- Capture a direct ref object before a concurrent symbolic-ref transition can redirect it.
- Make `note add` ambiguity errors recommend its supported `--provider` flag.
- Disambiguate index paths that begin with Git stage syntax such as `0:`.

The revision added a bounded ref snapshot, command-specific provider-selection errors, exact `:./` index paths, and interleaving and subprocess regressions.

### Round 8

Six objections were accepted, one was reframed, and one was rejected:

- Validate the complete normalized note returned to Neovim.
- Run the full Linux and Darwin flake gate on the tagged commit before publication.
- Pin comparison endpoints before reading patch and selected-side bytes.
- Disable lazy object fetches throughout note comparison capture.
- Retry a dangling-to-direct ref inspection instead of reporting false absence.
- Reject a direct non-commit notes ref consistently.
- Disable repository hooks for provider-owned Git plumbing.
- Keep captured-OID read linearization when a direct ref becomes symbolic after capture.

The revision added normalized response validation, tag-gated flake checks, resolved comparison specs, no-lazy-fetch propagation, bounded ref-state retries, object-type validation, hook isolation, and focused regressions.

### Round 9

Five objections were accepted, one was reframed, and one duplicate was rejected:

- Disable configured file-system monitors during provider and note-capture Git commands.
- Make committed-note identity independent of local diff presentation configuration.
- Enforce exact create-response placement and RFC 3339 timestamps in Neovim.
- Record explicit Git notes synchronization between two clones.
- Treat a signaled subprocess as a failure even when its exit code is zero.
- Reject binary buffers and invalid UTF-8 bytes.

The revision added file-system monitor isolation, a canonical raw comparison identity, separate display placement, complete response validation, signal handling, UTF-8 enforcement, cross-clone tests, and a two-clone synchronization reel.

### Round 10

Five objections were accepted and one was reframed:

- Derive note placement from the displayed path filter without narrowing the stored comparison identity.
- Disable configured file-system monitors for attribute checks and selected-side reads.
- Bind a Neovim create response to the complete request captured before the popup opened.
- Make the canonical comparison identity independent of user diff configuration and option ordering.
- Disable replacement refs for every comparison and selected-side Git subprocess.
- Verify the release gate on native aarch64 Linux as well as x86 Linux and Apple systems.

The revision separated comparison and placement inputs, centralized hardened Git execution, validated responses against captured requests, added configuration and replacement-ref regressions, and extended the tagged release matrix to a native ARM Linux runner.

### Round 11

Three objections were accepted and one was rejected:

- Normalize an empty configured Neovim side before starting the CLI.
- Preserve committed context-line notes when local diff context hides their stored coordinate.
- Recompute and verify the canonical committed fingerprint at the Git notes provider boundary.
- Reject broader editor-side validation of base, head, fingerprint, provider, and file digest because Changes owns that stable-capture boundary and the editor cannot derive every field.

The revision normalizes the effective editor side, degrades hidden exact committed placements to file-level placement, verifies provider fingerprints from canonical endpoints, and adds focused regressions for each accepted objection.

## Final deterministic evidence

- `go test -race ./...` and `go vet ./...` pass.
- The headless Neovim contract passes, including empty, newline, no-EOL, and CRLF files.
- Generated-output, Specutil, strict OpenSpec, example-reel, and root-media checks pass.
- `nix flake check -L` passes all checks for the current aarch64-darwin system.
- The Git notes, Neovim, and root media frames passed visual inspection.
