---
name: test
description: Run Prysm unit tests with Bazel for affected packages, or a given target.
---

# Unit test runner

## Usage
- `/test` — test affected packages (from git diff)
- `/test //beacon-chain/sync/...` — test a specific target
- `/test //...` — test everything (slow)

## Steps

1. **Determine targets** — if the user gave one, use it; otherwise derive from the diff:
   ```bash
   git diff --name-only HEAD | grep '\.go$' | xargs -I{} dirname {} | sort -u | sed 's|^|//|;s|$|/...|'
   ```

2. **Run**:
   ```bash
   bazel test <targets> \
     --keep_going \
     --test_output=errors \
     --flaky_test_attempts=3 \
     --build_tests_only
   ```

3. **Report**:
   ```
   ✅ Passed: X
   ❌ Failed: Y
     - //package:test — error summary
   ```
   Flaky tests that pass on retry are fine.
