#!/usr/bin/env bash
# Runs a `/kurtosis` PR comment and writes $OUT/report.md.
#
# Comment format:
#
#   /kurtosis [finalize=N (0-50)] [package=<ethereum-package ref, default main>]
#
#   ```yaml
#   <optional ethereum-package args, default.yaml if absent; cl_image/vc_image may be {{head}} or {{sha:<commit>}}>
#   ```
#
#   ```bash
#   <optional check script; its exit status is part of the verdict; helpers.sh is sourced>
#   ```
#
# A newer `/kurtosis` comment cancels the running one on the same PR. `/kurtosis cancel` only cancels.
#
# The run passes when the check script exits 0 and every CL node finalizes epoch N (default 3, 0 skips).
#
# Env: COMMENT_FILE, HEAD_SHA, REPO (owner/name), OUT, and optionally PR, RUN_URL, COMMENT_URL.
set -euo pipefail

: "${COMMENT_FILE:?}" "${HEAD_SHA:?}" "${REPO:?}" "${OUT:?}"
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(git -C "$HERE" rev-parse --show-toplevel)
export ENCLAVE=${ENCLAVE:-pr${PR:+-$PR}-$(date +%s)}
FINALIZE=3
CONFIG=comment
PACKAGE_REF=main
# One budget from script start for build, devnet start, check script, and finality wait.
# Keep it under the job's timeout-minutes, so the report and dump still get written.
WAIT_SECONDS=$((80 * 60))
deadline=$(($(date +%s) + WAIT_SECONDS))

mkdir -p "$OUT"
tr -d '\r' < "$COMMENT_FILE" > "$OUT/comment.md"
: > "$OUT/failures"
: > "$OUT/built"
CHECK_RC=

fail() { echo "- $*" >> "$OUT/failures"; }
fatal() { fail "$@"; exit 1; }

# Prints the first fenced block whose info string is one of the given languages.
fence() {
  awk -v langs=" $* " '
    { t = $0; sub(/^[[:space:]]*/, "", t) }
    in_block {
      c = t; sub(/[[:space:]]+$/, "", c)
      if (c ~ /^`+$/ && length(c) >= ticks) { if (want) exit; in_block = 0; next }
      if (want) print
      next
    }
    t ~ /^```/ {
      ticks = match(t, /[^`]/) ? RSTART - 1 : length(t)
      lang = substr(t, ticks + 1); sub(/^[[:space:]]+/, "", lang); sub(/[[:space:]].*/, "", lang)
      in_block = 1; want = index(langs, " " lang " ") > 0
    }
  ' "$OUT/comment.md"
}

api() { curl -sf --max-time 5 "$(kurtosis port print "$ENCLAVE" "$1" http)$2"; }

cl_services() { kurtosis enclave inspect "$ENCLAVE" | awk '$2 ~ /^cl-[0-9]+-/ { print $2 }' | sort -u; }

node_rows() {
  local svc version head fc peers syncing
  for svc in $(cl_services); do
    version=$(api "$svc" /eth/v1/node/version | jq -r '.data.version | split(" ")[0]') || version='?'
    head=$(api "$svc" /eth/v1/beacon/headers/head | jq -r .data.header.message.slot) || head='?'
    fc=$(api "$svc" /eth/v1/beacon/states/head/finality_checkpoints |
      jq -r '[.data.current_justified.epoch, .data.finalized.epoch, .data.finalized.root] | join(" ")') || fc='? ? ?'
    peers=$(api "$svc" /eth/v1/node/peer_count | jq -r .data.connected) || peers='?'
    syncing=$(api "$svc" /eth/v1/node/syncing | jq -r .data.is_syncing) || syncing='?'
    echo "$svc $version $head $fc $peers $syncing"
  done
}

finish() {
  local rc=$? verdict='✅ pass' fence_ticks='``````'
  [ "$rc" = 0 ] && [ ! -s "$OUT/failures" ] || verdict='❌ fail'
  {
    echo "### Kurtosis: $verdict"
    echo
    echo "head \`${HEAD_SHA:0:10}\` · ethereum-package \`${PACKAGE_REF:0:12}\` · finalize=$FINALIZE · config=$CONFIG${RUN_URL:+ · [run]($RUN_URL)}${COMMENT_URL:+ · [input]($COMMENT_URL)}"
    if [ -s "$OUT/failures" ]; then
      printf '\n**Failures**\n\n'
      cat "$OUT/failures"
    fi
    if [ -s "$OUT/nodes" ]; then
      printf '\n| Node | Version | Head slot | Justified | Finalized | Finalized root | Peers | Syncing |\n|---|---|---|---|---|---|---|---|\n'
      awk '{ printf "| %s | %s | %s | %s | %s | `%s` | %s | %s |\n", $1, $2, $3, $4, $5, substr($6, 1, 12), $7, $8 }' "$OUT/nodes"
    fi
    if [ -s "$OUT/images" ]; then
      printf '\n<details><summary>Images</summary>\n\n| Image | Source |\n|---|---|\n'
      cat "$OUT/images"
      printf '\n</details>\n'
    fi
    if [ -n "$CHECK_RC" ]; then
      printf '\n<details><summary>Check script (exit %s)</summary>\n\n%s\n' "$CHECK_RC" "$fence_ticks"
      tail -c 20000 "$OUT/check.log"
      printf '%s\n</details>\n' "$fence_ticks"
    fi
    if [ -s "$OUT/kurtosis-run.log" ] && grep -q 'kurtosis run failed' "$OUT/failures"; then
      printf '\n<details><summary>kurtosis run (tail)</summary>\n\n%s\n' "$fence_ticks"
      tail -c 10000 "$OUT/kurtosis-run.log"
      printf '%s\n</details>\n' "$fence_ticks"
    fi
    printf '\n<details><summary>Input (sha256 %s)</summary>\n\n%s\n' "$(shasum -a 256 "$OUT/comment.md" | cut -c1-12)" "$fence_ticks"
    head -c 20000 "$OUT/comment.md"
    printf '%s\n</details>\n' "$fence_ticks"
  } > "$OUT/report-full.md"
  # GitHub rejects comments over 65536 characters. The full report stays in the artifact.
  if [ "$(wc -c < "$OUT/report-full.md")" -gt 60000 ]; then
    { head -c 60000 "$OUT/report-full.md" | iconv -c -f UTF-8 -t UTF-8
      printf '\n\nTruncated. The full report is `report-full.md` in the run artifacts.\n'; } > "$OUT/report.md"
  else
    cp "$OUT/report-full.md" "$OUT/report.md"
  fi
  [ "$verdict" = '✅ pass' ]
}
trap finish EXIT

# Options on the first line.
for opt in $(head -1 "$OUT/comment.md" | sed 's#^/kurtosis##'); do
  case $opt in
    finalize=*) FINALIZE=${opt#*=} ;;
    package=*) PACKAGE_REF=${opt#*=} ;;
    *) fatal "unknown option \`${opt:0:40}\`" ;;
  esac
done

# Validate options from the first line of the comment.
[[ $FINALIZE =~ ^[0-9]{1,2}$ ]] && [ "$FINALIZE" -le 50 ] || fatal "finalize must be a number from 0 to 50, got \`${FINALIZE:0:40}\`"
[[ $PACKAGE_REF =~ ^[A-Za-z0-9._/-]{1,100}$ && $PACKAGE_REF != *..* ]] || fatal "bad package ref \`${PACKAGE_REF:0:40}\`"

# Run and report the commit, so a rerun with package=<sha> gets the same package.
sha=$(gh api "repos/ethpandaops/ethereum-package/commits/$PACKAGE_REF" --jq .sha < /dev/null) ||
  fatal "cannot resolve package ref \`$PACKAGE_REF\`"
PACKAGE_REF=$sha

# Mandatory.
fence yaml yml > "$OUT/args.in.yaml"
if [ ! -s "$OUT/args.in.yaml" ]; then
  CONFIG=default
  cp "$HERE/default.yaml" "$OUT/args.in.yaml"
fi

# Optional check script.
fence bash sh > "$OUT/check.sh"

# Resolve {{head}} / {{sha:X}} into locally built images.
placeholder='^([[:space:]-]*)(cl_image|vc_image):[[:space:]]*"?\{\{(head|sha:([0-9a-f]{7,40}))\}\}"?[[:space:]]*(#.*)?$'
while IFS= read -r line; do
  if [[ $line =~ $placeholder ]]; then
    indent=${BASH_REMATCH[1]} key=${BASH_REMATCH[2]} ref=${BASH_REMATCH[3]}
    if [ "$ref" = head ]; then
      # PR's head SHA is used.
      sha=$HEAD_SHA
    else
      sha=$(gh api "repos/$REPO/commits/${ref#sha:}" --jq .sha < /dev/null) || fatal "cannot resolve \`$ref\`"
    fi
    bin=beacon-chain
    [ "$key" = vc_image ] && bin=validator
    image=prysm-$bin:${sha:0:12}
    grep -qx "$image $sha $bin" "$OUT/built" || echo "$image $sha $bin" >> "$OUT/built"
    echo "$indent$key: $image"
  elif [[ $line == *'{{'* ]]; then
    fatal "placeholder must be a whole cl_image/vc_image value: \`$line\`"
  else
    echo "$line"
  fi
done < "$OUT/args.in.yaml" > "$OUT/args.yaml"

while read -r image sha bin; do
  src=$OUT/src/$sha
  if [ ! -d "$src" ]; then
    git -C "$ROOT" cat-file -e "$sha^{commit}" 2> /dev/null ||
      git -C "$ROOT" fetch -q --depth=1 origin "$sha" < /dev/null || fatal "cannot fetch \`$sha\`"
    git -C "$ROOT" worktree add -q --detach "$src" "$sha" < /dev/null
  fi
  echo "Building $image"
  docker build -q -f "$HERE/../kurtosis.Dockerfile" --build-arg BIN="$bin" --build-arg TAG="${sha:0:10}" -t "$image" "$src" \
    < /dev/null > /dev/null ||
    fatal "docker build of \`$image\` failed"
  echo "| \`$image\` | built from $sha |" >> "$OUT/images"
done < "$OUT/built"

# The check script must not see the token used above.
unset GH_TOKEN GITHUB_TOKEN

kurtosis run --enclave "$ENCLAVE" "github.com/ethpandaops/ethereum-package@$PACKAGE_REF" --args-file "$OUT/args.yaml" \
  > "$OUT/kurtosis-run.log" 2>&1 || fatal 'kurtosis run failed'

for image in $(docker ps --filter label=com.kurtosistech.container-type=user-service --format '{{.Image}}' | sort -u); do
  case $image in prysm-beacon-chain:* | prysm-validator:*) continue ;; esac
  echo "| \`$image\` | \`$(docker image inspect --format '{{index .RepoDigests 0}}' "$image" 2>/dev/null || echo local)\` |" >> "$OUT/images"
done

if [ -s "$OUT/check.sh" ]; then
  echo "Running check script"
  CHECK_RC=0
  # The check and the finality wait share one deadline. `timeout 0` disables the limit, so the minimum is 1.
  left=$((deadline - $(date +%s)))
  (cd "$OUT" && timeout "$((left > 0 ? left : 1))" bash -c 'source "$0"; source "$1"' "$HERE/helpers.sh" "$OUT/check.sh") \
    > "$OUT/check.log" 2>&1 || CHECK_RC=$?
  [ "$CHECK_RC" = 0 ] || fail "check script exited $CHECK_RC"
fi

if [ "$FINALIZE" -gt 0 ]; then
  echo "Waiting for finalized epoch $FINALIZE"
  until node_rows > "$OUT/nodes" && awk -v n="$FINALIZE" '$5 !~ /^[0-9]+$/ || $5 < n { bad = 1 } END { exit bad || NR == 0 }' "$OUT/nodes"; do
    if [ "$(date +%s)" -ge "$deadline" ]; then
      fail "not every CL node finalized epoch $FINALIZE within $((WAIT_SECONDS / 60)) minutes of the run start"
      break
    fi
    sleep 12
  done
else
  node_rows > "$OUT/nodes"
fi

# Nodes at the same finalized epoch must agree on its root.
awk '{ print $5, $6 }' "$OUT/nodes" | sort -u | awk '{ n[$1]++ } END { for (e in n) if (n[e] > 1) exit 1 }' ||
  fail 'CL nodes disagree on a finalized root'

kurtosis enclave dump "$ENCLAVE" "$OUT/dump" > /dev/null 2>&1 || true
