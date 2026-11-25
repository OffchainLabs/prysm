#!/bin/bash
#
# Spectest coverage: every handler in the pinned consensus spec test data versus the handlers
# Prysm's spectests read across the four CI passes ({mainnet,minimal} x {real,fake} BLS).
#
# Usage: hack/spectest-report.sh [--no-run]
#   --no-run  reuse the previous run's results, e.g. after editing exclusions.txt

set -eo pipefail
export LC_ALL=C # sort and comm must agree on collation

ROOT=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)
EXCLUSIONS="$ROOT/testing/spectest/exclusions.txt"
REPORT="$ROOT/testing/spectest/report.txt"
DATA="$ROOT/third_party/testdata"
CACHE="${XDG_CACHE_HOME:-$HOME/.cache}/prysm-spectest-report"
RUN="$CACHE/run"

version=$(sed -nE 's/^consensus_spec_version = "([^"]+)"/\1/p' "$ROOT/WORKSPACE")
[ -n "$version" ] || { echo "Could not parse consensus_spec_version from $ROOT/WORKSPACE" >&2; exit 1; }

# Fork choice compliance vectors are a separate release asset that no test fetches.
comptests="$CACHE/comptests-$version"
if [ ! -d "$comptests/tests" ]; then
    echo "Downloading comptests $version"
    rm -rf "$comptests.tmp"
    mkdir -p "$comptests.tmp"
    curl -fsSL "https://github.com/ethereum/consensus-specs/releases/download/$version/comptests.tar.gz" | tar xz -C "$comptests.tmp"
    mv "$comptests.tmp" "$comptests"
fi

run_pass() {
    local name=$1 tags=$2
    shift 2
    mkdir -p "$RUN/$name"
    # A fresh output dir changes the env, so the go test cache cannot skip the writes.
    (cd "$ROOT" && SPEC_TEST_REPORT_OUTPUT_DIR="$RUN/$name" go test -timeout=0 -tags="$tags" "$@") > "$RUN/$name.log" 2>&1 \
        || echo "warning: $name pass failed, see $RUN/$name.log" >&2
}

if [ "$1" != "--no-run" ]; then
    make -C "$ROOT" --no-print-directory gen
    make -C "$ROOT" --no-print-directory testdata
    rm -rf "$RUN"
    mainnet=$(cd "$ROOT" && go list ./testing/spectest/... | grep -v /testing/spectest/minimal)
    minimal=$(cd "$ROOT" && go list ./testing/spectest/minimal/...)
    echo "Running spectests (several minutes)"
    # shellcheck disable=SC2086
    {
        run_pass mainnet-real develop $mainnet &
        run_pass mainnet-fake develop,fake_crypto $mainnet &
        run_pass minimal-real develop,minimal $minimal &
        run_pass minimal-fake develop,minimal,fake_crypto $minimal &
        wait
    }
fi

# Handlers are tests/<config>/<fork>/<runner>/<handler>, or tests/<runner>/<handler> for cryptography-specs.
{
    (cd "$DATA" && find tests -mindepth 2 -maxdepth 4 -type d) | awk -F/ '$2 ~ /^(mainnet|minimal|general)$/ ? NF == 5 : NF == 3'
    (cd "$comptests" && find tests/mainnet tests/minimal -mindepth 3 -maxdepth 3 -type d 2>/dev/null || true)
} | sort -u > "$CACHE/spec.txt"
find "$RUN/" -type f -name '*_tests.txt' -exec cut -d/ -f1-5 {} + | sort -u > "$CACHE/tested.txt"
comm -12 "$CACHE/spec.txt" "$CACHE/tested.txt" > "$CACHE/found.txt"
comm -23 "$CACHE/spec.txt" "$CACHE/tested.txt" > "$CACHE/untested.txt"

patterns=()
while IFS= read -r pat; do
    case "$pat" in '' | \#*) ;; *) patterns+=("$pat") ;; esac
done < "$EXCLUSIONS"

# Print the lines of $1 matching (or, with -v, not matching) any exclusion pattern.
excluded() {
    local invert=0 line pat hit
    [ "$1" = -v ] && { invert=1; shift; }
    while IFS= read -r line; do
        hit=0
        # shellcheck disable=SC2254
        for pat in "${patterns[@]}"; do case "$line" in $pat) hit=1 && break ;; esac; done
        if [ "$hit" != "$invert" ]; then echo "$line"; fi
    done < "$1"
}

# Print the lines of $2 matching glob $1.
matches() {
    local line
    # shellcheck disable=SC2254
    while IFS= read -r line; do case "$line" in $1) echo "$line" ;; esac; done < "$2"
}

errors=0
while IFS= read -r line; do
    echo "error: excluded handler is tested: $line" >&2
    errors=$((errors + 1))
done < <(excluded "$CACHE/found.txt")
for pat in "${patterns[@]}"; do
    [ -n "$(matches "$pat" "$CACHE/spec.txt")" ] && continue
    echo "error: exclusion matches no spec handler: $pat" >&2
    errors=$((errors + 1))
done

excluded "$CACHE/untested.txt" > "$CACHE/excluded.txt"
excluded -v "$CACHE/untested.txt" > "$CACHE/missing.txt"
found=$(wc -l < "$CACHE/found.txt" | tr -d ' ')
missing=$(wc -l < "$CACHE/missing.txt" | tr -d ' ')
skipped=$(wc -l < "$CACHE/excluded.txt" | tr -d ' ')

by_runner() {
    awk -F/ '{ print (NF == 3 ? $2 : $4) }' "$1" | sort | uniq -c | sort -rn
}

# One "runner/handler  config: forks" line per handler and config.
group() {
    awk -F/ '{
        k = NF == 3 ? $2 "/" $3 : $4 "/" $5
        c = NF == 3 ? "cryptography" : $2
        f[k "\t" c] = f[k "\t" c] (NF == 3 ? "" : " " $3)
    } END { for (kc in f) { split(kc, p, "\t"); printf "  %-56s %s%s\n", p[1], p[2], (f[kc] == "" ? "" : ":" f[kc]) } }' "$1" | sort
}

{
    echo "Prysm spectest coverage ($version)"
    echo
    echo "Summary: $found found, $missing missing, $skipped excluded"
    echo
    echo "Missing by runner:"
    by_runner "$CACHE/missing.txt"
    echo
    echo "Missing:"
    group "$CACHE/missing.txt"
    echo
    echo "Excluded (handlers per pattern):"
    for pat in "${patterns[@]}"; do
        printf '%6d %s\n' "$(matches "$pat" "$CACHE/excluded.txt" | wc -l)" "$pat"
    done
} > "$REPORT"

echo
echo "================ Spectest coverage ($version) ================"
echo "  found: $found   missing: $missing   excluded: $skipped"
if [ "$missing" -gt 0 ]; then
    echo "  missing by runner:"
    by_runner "$CACHE/missing.txt" | sed 's/^/  /'
fi
echo "  full report: $REPORT"

[ "$missing" -eq 0 ] && [ "$errors" -eq 0 ]
