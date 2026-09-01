#!/bin/bash

set -eo pipefail

# Constants
PROJECT_ROOT=$(pwd)
REPO_ROOT="${PROJECT_ROOT%/hack}"
PRYSM_DIR="$REPO_ROOT/testing/spectest"
EXCLUSION_LIST="$PRYSM_DIR/exclusions.txt"
BAZEL_DIR="/tmp/spectest_report"
SPEC_DIR="/tmp/consensus-spec"

# Return success if $1 matches any pattern in the exclusion list. 
# Patterns support shell globs.
is_excluded() {
    local line="$1" pat
    while IFS= read -r pat; do
        case "$pat" in ''|\#*) continue ;; esac   # skip blank lines and comments
        case "$line" in $pat) return 0 ;; esac
    done < "$EXCLUSION_LIST"
    return 1
}

clean_up() {
    rm -f "$PRYSM_DIR/tests.txt" "$PRYSM_DIR/spec.txt" "$PRYSM_DIR/report.raw"
}

# Create directory if it doesn't already exist
mkdir -p "$BAZEL_DIR"

# Add any passed flags to BAZEL_FLAGS
BAZEL_FLAGS=""
for flag in "$@"
do
    BAZEL_FLAGS="$BAZEL_FLAGS $flag"
done

# Run spectests
bazel test //testing/spectest/... --test_env=SPEC_TEST_REPORT_OUTPUT_DIR="$BAZEL_DIR" $BAZEL_FLAGS

# Compare against the SAME spec test data the bazel tests actually run against:
# the consensus-specs release tarballs pinned in WORKSPACE.
consensus_spec_version=$(grep -E '^consensus_spec_version =' "$REPO_ROOT/WORKSPACE" | sed -E 's/.*"([^"]+)".*/\1/')
if [ -z "$consensus_spec_version" ]; then
    echo "Could not parse consensus_spec_version from $REPO_ROOT/WORKSPACE"
    exit 1
fi
echo "Using consensus spec tests $consensus_spec_version"

# Download + extract the release tarballs (cached by version under $SPEC_DIR).
if [ "$(cat "$SPEC_DIR/.version" 2>/dev/null)" != "$consensus_spec_version" ]; then
    rm -rf "$SPEC_DIR"
    mkdir -p "$SPEC_DIR"
    for flavor in general minimal mainnet; do
        url="https://github.com/ethereum/consensus-specs/releases/download/${consensus_spec_version}/${flavor}.tar.gz"
        echo "Downloading $url"
        curl -fsSL "$url" | tar xz -C "$SPEC_DIR"
    done
    echo "$consensus_spec_version" > "$SPEC_DIR/.version"
else
    echo "Spec tests $consensus_spec_version already downloaded, skipping."
fi

# spec.txt: every spec test handler dir at runner granularity, i.e.
# tests/<config>/<fork>/<runner>/<handler>.
(cd "$SPEC_DIR" && find tests -maxdepth 4 -mindepth 4 -type d | sort -u > "$PRYSM_DIR/spec.txt")

# tests.txt: what Prysm actually exercised.
find "$BAZEL_DIR" -type f -name '*_tests.txt' -exec cut -d/ -f1-5 {} + | sort -u > "$PRYSM_DIR/tests.txt"

# Classify each spec handler as found / missing / excluded.
while IFS= read -r line; do
    if is_excluded "$line"; then
        # Excluding something we actually test is a mistake -- flag it loudly.
        if grep -Fxq "$line" "$PRYSM_DIR/tests.txt"; then
            echo "Error: excluded item is actually tested: $line" >&2
            clean_up
            exit 1
        fi
        echo "excluded $line"
        continue
    fi
    if grep -Fxq "$line" "$PRYSM_DIR/tests.txt"; then
        echo "found $line"
    else
        echo "missing $line"
    fi
done < "$PRYSM_DIR/spec.txt" > "$PRYSM_DIR/report.raw"

found=$(grep -c '^found '    "$PRYSM_DIR/report.raw" || true)
missing=$(grep -c '^missing '  "$PRYSM_DIR/report.raw" || true)
excluded=$(grep -c '^excluded ' "$PRYSM_DIR/report.raw" || true)

# report.txt: summary + missing-by-runner breakdown + the full missing list.
# (The found list is just a count -- it is large and rarely useful in the file.)
{
    echo "Prysm Spectest Report ($consensus_spec_version)"
    echo
    echo "Summary: $found found, $missing missing, $excluded excluded"
    echo
    echo "Missing by runner:"
    grep '^missing ' "$PRYSM_DIR/report.raw" | awk '{print $2}' | cut -d/ -f4 | sort | uniq -c | sort -rn
    echo
    echo "Tests Missing"
    grep '^missing ' "$PRYSM_DIR/report.raw" | sort
} > "$PRYSM_DIR/report.txt"

# Terminal summary, so the result is visible without opening the file.
echo
echo "================ Spectest coverage ($consensus_spec_version) ================"
printf '  found: %s   missing: %s   excluded: %s\n' "$found" "$missing" "$excluded"
if [ "$missing" -gt 0 ]; then
    echo "  missing by runner:"
    grep '^missing ' "$PRYSM_DIR/report.raw" | awk '{print $2}' | cut -d/ -f4 | sort | uniq -c | sort -rn | sed 's/^/    /'
    echo "  full list: $PRYSM_DIR/report.txt"
fi

clean_up

# Fail if any non-excluded handler is uncovered.
if [ "$missing" -gt 0 ]; then
    exit 1
fi
