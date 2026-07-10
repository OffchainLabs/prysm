---
name: precheck
description: Run pre-commit checks (gofmt, goimports, gazelle, code generators, build) before committing Prysm changes. Required before every commit.
---

# Pre-commit checks

Run these in order from the repo root before every commit. Stop and report if any step fails.

1. **gofmt** — format changed Go files:
   ```bash
   gofmt -l $(git diff --name-only --diff-filter=ACM HEAD | grep '\.go$')
   ```
   If output is non-empty, run `gofmt -w` on those files.

2. **goimports** — organize imports:
   ```bash
   goimports -l $(git diff --name-only --diff-filter=ACM HEAD | grep '\.go$')
   ```
   If output is non-empty, run `goimports -w` on those files.

3. **Gazelle — sync `deps.bzl`** (when `go.mod` changed):
   ```bash
   bazel run //:gazelle -- update-repos -from_file=go.mod -to_macro=deps.bzl%prysm_deps -prune=true
   git diff --exit-code deps.bzl
   ```

4. **Gazelle — sync `BUILD.bazel`**:
   ```bash
   bazel run //:gazelle -- fix --mode=diff
   ```
   If the diff is non-empty, run `bazel run //:gazelle -- fix`.

5. **Regenerate protobuf** (when `.proto` files changed):
   ```bash
   hack/update-go-pbs.sh
   ```

6. **Regenerate SSZ** (when SSZ-tagged structs changed):
   ```bash
   hack/update-go-ssz.sh
   ```

7. **Regenerate mocks** (when proto service interfaces changed):
   ```bash
   hack/update-mockgen.sh
   ```

8. **Smoke run with Bazel**:
   ```bash
   bazel run //cmd/beacon-chain:beacon-chain  -- --help
   bazel run //cmd/validator:validator  -- --help
   ```

Hack scripts are idempotent — if unsure which generated files are affected, run all three.

## Report

- Success: "✅ All pre-checks passed. Ready to commit."
- Failure: name the failed check and the fix command.
