# Claude Code Project Instructions for Prysm

## Building

- **Never edit BUILD.bazel files directly.** Instead, run `bazel run //:gazelle -- fix` to regenerate them.

## Testing

- **Always use bazel for running tests**, never `go test`.
- Example: `bazel test //beacon-chain/rpc/prysm/v1alpha1/validator:go_default_test --test_filter="TestName"`
- For streamed output: `bazel test //path:target --test_output=streamed`

## Notifications

- Run `say "Attention needed" -v Trinoids` whenever input is required from the user or when a task is finished.

## Code Style

Inspired by NASA/JPL's "Power of 10" — code must be quickly and easily reviewable by a human.

- Write minimal, concise code. No unnecessary abstractions or indirection.
- Functions should be short enough to fit on a screen (~60 lines max). If longer, split by responsibility.
- Simple control flow. Minimal nesting, early returns over deep if/else chains.
- Smallest possible scope for all variables and data.
- No comments unless the logic is genuinely non-obvious. Never restate what the code already says.
- Godocs are required on exported functions and types. Keep them minimal.
- No JSDoc unless it's a public API. Skip @param/@returns that just repeat type signatures.
- Don't add error handling, validation, or fallbacks for cases that can't realistically happen.
- Prefer fewer lines. Three similar lines are better than a helper function used once.
- Don't refactor, rename, or "improve" code you weren't asked to change.
- No clever tricks. Code should be obvious, not impressive.

## Stategen

- `SaveState` is keyed by beacon block root on the block path (all forks).
- `SaveState` is keyed by execution block hash on the payload envelope path (post-Gloas).
