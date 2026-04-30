### Added

- Wire the consensus-specs fork-choice compliance-runner suites into the `testing/spectest/minimal` test target. The shared runner exposes `forkchoice.RunComplianceSuite` for per-suite execution; the `minimal` target ships one top-level `Test*` per suite (Fulu and Gloas) so bazel test sharding can fan suites out across processes (`shard_count = 12`). Test data resolution falls back to the `COMPLIANCE_FC_DIR` environment variable when set (an absolute path to an unpacked compliance-tests tarball rooted at `tests/`) and to bazel runfiles otherwise. Setting `PRYSM_SPECTEST_SKIP_BLS=1` short-circuits `SignatureBatch.Verify`/`VerifyVerbosely` for the `bls_setting: 2` cases that ship in the compliance tarball.
- `hack/compliance-fc-report.sh` orchestrates the run end-to-end: it resolves the latest successful run of the consensus-specs "Compliance Tests" workflow on master via the GitHub API, downloads its `small.tar.gz` artifact (cached per run-id under `/var/tmp/compliance_fc_cache`), invokes `bazel test` with sharding, and prints an aggregated per-suite total/pass/fail/skip table by walking the per-shard test logs (`-test.v` is enabled so `=== RUN` lines drive the totals).

### Fixed

- `hack/workspace_status.sh` now uses POSIX-portable `date -u` flags instead of GNU `--rfc-3339` / `--utc`. macOS users no longer see `date: illegal option -- -` on every bazel build; the emitted RFC-3339 timestamp string is unchanged.
