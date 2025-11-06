#!/usr/bin/env bash
set -euo pipefail

# ====== config ======
OLD="OffchainLabs/prysm/v6"
NEW="OffchainLabs/prysm/v7"

# files by extension (recursive)
EXTENSIONS=("bazel" "go" "proto")

# explicit files (relative to project root)
EXPLICIT_FILES=(
  "hack/update-mockgen.sh"
  "hack/update-go-pbs.sh"
  "go.mod"
  ".deepsource.toml"
)

# commands to run at the end, in order
COMMANDS=(
  'bazel run //:gazelle -- update-repos -from_file=go.mod -to_macro=deps.bzl%prysm_deps -prune=true'
  'go mod tidy && go get ./...'
  './hack/update-go-pbs.sh'
  'bazel clean --expunge --async'
  './hack/update-go-ssz.sh'
  'go build ./... && bazel build //cmd/beacon-chain'
)
# ====================

# Detect BSD vs GNU sed (macOS vs Linux)
if sed --version >/dev/null 2>&1; then
  SED_INPLACE=("sed" "-i")
else
  # macOS / BSD sed needs an empty string after -i
  SED_INPLACE=("sed" "-i" "")
fi

escape_sed() {
  # escape / in pattern and replacement so sed doesn't choke
  printf '%s' "$1" | sed 's/[\/&]/\\&/g'
}

OLD_ESCAPED=$(escape_sed "$OLD")
NEW_ESCAPED=$(escape_sed "$NEW")

############################################
# 1) walk directory by extensions
############################################
for ext in "${EXTENSIONS[@]}"; do
  while IFS= read -r -d '' file; do
    "${SED_INPLACE[@]}" "s/${OLD_ESCAPED}/${NEW_ESCAPED}/g" "$file"
    echo "updated (by ext): $file"
  done < <(find . -type f -name "*.${ext}" -print0)
done

############################################
# 2) specific files
############################################
for f in "${EXPLICIT_FILES[@]}"; do
  if [[ -f "$f" ]]; then
    "${SED_INPLACE[@]}" "s/${OLD_ESCAPED}/${NEW_ESCAPED}/g" "$f"
    echo "updated (explicit): $f"
  else
    echo "warn: explicit file not found: $f" >&2
  fi
done

############################################
# 3) run commands in order
############################################
for cmd in "${COMMANDS[@]}"; do
  echo "==> running: $cmd"
  # use bash -c so we can have && in commands
  bash -c "$cmd"
  echo "==> done: $cmd"
done

echo
echo "✅ version upgrade was successful."