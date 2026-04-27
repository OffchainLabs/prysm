#!/bin/bash
# hack/compliance-fc-report.sh
#
# Run the fork-choice compliance test suites under bazel sharding and print an
# aggregated pass/fail summary by walking the per-shard test logs. See --help.

set -uo pipefail

# ---- defaults (overridable via flags or env) ---------------------------------

PRESET="${COMPLIANCE_FC_PRESET:-minimal}"
DIR="${COMPLIANCE_FC_DIR:-}"
TARBALL="${COMPLIANCE_FC_TARBALL:-}"
URL="${COMPLIANCE_FC_URL:-}"
RUN_ID="${COMPLIANCE_FC_RUN_ID:-}"
CACHE_ROOT="${COMPLIANCE_FC_CACHE_DIR:-/var/tmp/compliance_fc_cache}"

usage() {
  cat <<'EOF'
Run the fork-choice compliance suites under bazel and print a pass/fail report.

USAGE
  hack/compliance-fc-report.sh [options] [-- bazel_args...]

DATA SOURCE (first non-empty wins)
  --dir PATH        Use a pre-extracted tree at PATH (must contain tests/).
  --tarball PATH    Use a local .tar.gz; extracted to cache on first use.
  --url URL         Download tarball via curl; cached + extracted.
  --run-id ID       Pin to a consensus-specs run id; download via gh.
  (default)         Resolve the latest successful run of the consensus-specs
                    "Compliance Tests" workflow on master via gh.

OTHER OPTIONS
  --preset NAME     Preset: minimal or mainnet (default: minimal).
                    Only minimal currently ships compliance data.
  --cache-dir PATH  Cache root (default: /var/tmp/compliance_fc_cache).
  -h, --help        Print this help and exit.

ENVIRONMENT (each flag has a matching env var; the flag wins if both are set)
  COMPLIANCE_FC_PRESET, COMPLIANCE_FC_DIR, COMPLIANCE_FC_TARBALL,
  COMPLIANCE_FC_URL, COMPLIANCE_FC_RUN_ID, COMPLIANCE_FC_CACHE_DIR
  GITHUB_TOKEN      Required only for --run-id and the default auto-fetch path
                    (GitHub Actions artifacts return 403 to anonymous requests
                    even for public repos, so a token is unavoidable there).
                    Use --tarball or --url to avoid the token entirely.

Anything after '--' is forwarded verbatim to 'bazel test'.

EXAMPLES
  # Auto-fetch latest (needs token)
  GITHUB_TOKEN=... hack/compliance-fc-report.sh

  # Use a manually-downloaded tarball — no token, no gh
  hack/compliance-fc-report.sh --tarball ~/Downloads/small.tar.gz

  # Pull the artifact through a public mirror via curl — no token
  hack/compliance-fc-report.sh --url https://example.org/small.tar.gz

  # Re-use an already-extracted tree
  hack/compliance-fc-report.sh --dir /var/tmp/compliance_fc_root

  # Forward bazel args (run only the BlockTree suite)
  hack/compliance-fc-report.sh -- --test_filter='..._BlockTree$'
EOF
}

# ---- argument parsing --------------------------------------------------------

while (( $# > 0 )); do
  case "$1" in
    --preset)    PRESET="$2"; shift 2 ;;
    --dir)       DIR="$2"; shift 2 ;;
    --tarball)   TARBALL="$2"; shift 2 ;;
    --url)       URL="$2"; shift 2 ;;
    --run-id)    RUN_ID="$2"; shift 2 ;;
    --cache-dir) CACHE_ROOT="$2"; shift 2 ;;
    -h|--help)   usage; exit 0 ;;
    --)          shift; break ;;
    -*)          echo "error: unknown option: $1" >&2; usage >&2; exit 1 ;;
    *)           echo "error: unexpected positional arg: $1 (use -- to forward to bazel)" >&2; exit 1 ;;
  esac
done
BAZEL_EXTRA_ARGS=("$@")

case "$PRESET" in
  minimal|mainnet) ;;
  *) echo "error: --preset must be minimal or mainnet (got: $PRESET)" >&2; exit 1 ;;
esac

# ---- data source resolution --------------------------------------------------

extract_into() {
  # extract_into <tarball> <cache-key>; sets DIR to the cache target.
  local tarball="$1" key="$2" target="$CACHE_ROOT/$key"
  if [[ -d "$target/tests" ]]; then
    echo "Reusing cached compliance data at $target"
  else
    mkdir -p "$target"
    echo "Extracting $tarball -> $target..."
    tar -xzf "$tarball" -C "$target"
  fi
  DIR="$target"
}

require_token() {
  : "${GITHUB_TOKEN:?required for gh artifact download (use --tarball or --url to avoid)}"
  command -v gh >/dev/null || {
    echo "error: gh CLI required for auto-fetch (brew install gh, or use --tarball/--url/--dir)" >&2
    exit 1
  }
}

if [[ -n "$DIR" ]]; then
  : # use as-is
elif [[ -n "$TARBALL" ]]; then
  [[ -f "$TARBALL" ]] || { echo "error: tarball not found: $TARBALL" >&2; exit 1; }
  key="tarball-$(shasum -a 256 "$TARBALL" | awk '{print $1}')"
  extract_into "$TARBALL" "$key"
elif [[ -n "$URL" ]]; then
  command -v curl >/dev/null || { echo "error: curl required for --url" >&2; exit 1; }
  key="url-$(printf '%s' "$URL" | shasum -a 256 | awk '{print $1}')"
  target="$CACHE_ROOT/$key"
  if [[ -d "$target/tests" ]]; then
    echo "Reusing cached compliance data at $target"
    DIR="$target"
  else
    tmpfile="$(mktemp)"
    trap 'rm -f "$tmpfile"' EXIT
    echo "Downloading $URL..."
    curl -fL --output "$tmpfile" "$URL"
    extract_into "$tmpfile" "$key"
    rm -f "$tmpfile"
    trap - EXIT
  fi
elif [[ -n "$RUN_ID" || -n "${GITHUB_TOKEN:-}" ]]; then
  require_token
  if [[ -z "$RUN_ID" ]]; then
    # 261432977 = "Compliance Tests" workflow on ethereum/consensus-specs
    echo "Resolving latest successful Compliance Tests run on master..."
    RUN_ID=$(gh api \
      'repos/ethereum/consensus-specs/actions/workflows/261432977/runs?branch=master&status=success&per_page=1' \
      --jq '.workflow_runs[0].id // empty')
    [[ -n "$RUN_ID" ]] || { echo "error: no successful runs found" >&2; exit 1; }
    echo "Latest run: $RUN_ID"
  fi
  target="$CACHE_ROOT/run-$RUN_ID"
  if [[ -d "$target/tests" ]]; then
    echo "Reusing cached compliance data at $target"
    DIR="$target"
  else
    tmpdir="$(mktemp -d)"
    trap 'rm -rf "$tmpdir"' EXIT
    echo "Downloading artifact from run $RUN_ID..."
    gh run download "$RUN_ID" --repo ethereum/consensus-specs --name small.tar.gz --dir "$tmpdir"
    [[ -f "$tmpdir/small.tar.gz" ]] || { echo "error: small.tar.gz not present in artifact" >&2; exit 1; }
    extract_into "$tmpdir/small.tar.gz" "run-$RUN_ID"
    rm -rf "$tmpdir"
    trap - EXIT
  fi
else
  echo "error: no data source given." >&2
  echo "       Pass --dir / --tarball / --url / --run-id, or set GITHUB_TOKEN" >&2
  echo "       to auto-fetch the latest run. See --help for details." >&2
  exit 1
fi

if [[ ! -d "$DIR/tests/$PRESET" ]]; then
  echo "error: no $PRESET-preset compliance data at $DIR/tests/$PRESET" >&2
  echo "       (current artifact ships only the minimal preset)" >&2
  exit 1
fi

# ---- run ---------------------------------------------------------------------

# Preset-dependent bazel knobs.
case "$PRESET" in
  minimal)
    TARGET="//testing/spectest/minimal:go_default_test"
    BAZEL_CONFIG=(--config=minimal)
    TEST_PREFIX="TestMinimal"
    ;;
  mainnet)
    TARGET="//testing/spectest/mainnet:go_default_test"
    BAZEL_CONFIG=()
    TEST_PREFIX="TestMainnet"
    ;;
esac

bazel test "$TARGET" \
  ${BAZEL_CONFIG[@]+"${BAZEL_CONFIG[@]}"} \
  --test_filter="${TEST_PREFIX}_(Fulu|Gloas)_ForkchoiceCompliance_" \
  --test_env=COMPLIANCE_FC_DIR="$DIR" \
  --test_env=PRYSM_SPECTEST_SKIP_BLS=1 \
  --test_arg=-test.v \
  --test_output=summary \
  --test_summary=terse \
  --test_timeout=900 \
  --local_test_jobs=12 \
  ${BAZEL_EXTRA_ARGS[@]+"${BAZEL_EXTRA_ARGS[@]}"} || true

# ---- aggregate report --------------------------------------------------------

# Use the workspace `bazel-testlogs` symlink — bazel maintains it pointing at the
# transitioned-config testlogs after a run, which `bazel info bazel-testlogs`
# does not (it returns the un-transitioned path).
WORKSPACE_DIR="$(bazel info workspace 2>/dev/null)"
case "$PRESET" in
  minimal) LOGS_DIR="$WORKSPACE_DIR/bazel-testlogs/testing/spectest/minimal/go_default_test" ;;
  mainnet) LOGS_DIR="$WORKSPACE_DIR/bazel-testlogs/testing/spectest/mainnet/go_default_test" ;;
esac
if [[ ! -d "$LOGS_DIR" ]]; then
  echo "error: no test logs at $LOGS_DIR" >&2
  exit 1
fi

# With `-test.v`, the Go test runner emits `=== RUN`, `--- PASS`, `--- FAIL`,
# `--- SKIP` for every subtest. We tally per top-level test (the suite-scoped
# wrapper, e.g. TestMinimal_Fulu_ForkchoiceCompliance_BlockTree).
report=$(
  find "$LOGS_DIR" -name test.log -exec cat {} + 2>/dev/null \
    | awk -v prefix="$TEST_PREFIX" '
        function topLevel(s,    p) {
          p = index(s, "/")
          return (p == 0) ? s : substr(s, 1, p-1)
        }
        $0 ~ "^=== RUN[[:space:]]+" prefix "_[A-Za-z_]+_ForkchoiceCompliance_" {
          if (index($3, "/") > 0) total[topLevel($3)]++
          next
        }
        $0 ~ "^[[:space:]]*--- FAIL: " prefix "_[A-Za-z_]+_ForkchoiceCompliance_" {
          if (index($3, "/") > 0) fail[topLevel($3)]++
          next
        }
        $0 ~ "^[[:space:]]*--- SKIP: " prefix "_[A-Za-z_]+_ForkchoiceCompliance_" {
          if (index($3, "/") > 0) skip[topLevel($3)]++
          next
        }
        END {
          for (k in total) printf "%s\t%d\t%d\t%d\n", k, total[k], fail[k]+0, skip[k]+0
        }
      ' \
    | sort
)

if [[ -z "$report" ]]; then
  echo "error: no top-level test runs detected in $LOGS_DIR — was -test.v honoured?" >&2
  exit 1
fi

echo
echo "=== Fork-choice compliance report ($PRESET) ==="
printf '%-60s %8s %8s %8s %8s\n' "Top-level test" "Total" "Pass" "Fail" "Skip"
printf '%-60s %8s %8s %8s %8s\n' "--------------" "-----" "----" "----" "----"
awk -F'\t' '
  { p = $2 - $3 - $4
    printf "%-60s %8d %8d %8d %8d\n", $1, $2, p, $3, $4
    tt += $2; tp += p; tf += $3; ts += $4 }
  END {
    printf "%-60s %8s %8s %8s %8s\n", "--------------", "-----", "----", "----", "----"
    printf "%-60s %8d %8d %8d %8d\n", "TOTAL", tt, tp, tf, ts
  }
' <<<"$report"

echo
echo "Per-shard logs (look here for failure details):"
echo "  $LOGS_DIR/shard_<N>_of_<M>/test.log"

# Exit non-zero if any case failed.
total_fail=$(awk -F'\t' '{s+=$3} END{print s+0}' <<<"$report")
(( total_fail == 0 ))
