#!/bin/bash
. "$(dirname "$0")"/common.sh

set -eo pipefail

# Script to copy generated pb.go and ssz.go files from the bazel build folder
# to the appropriate location.
# Bazel builds to bazel-bin/... folder, script copies them back to the original
# folder where the .proto / target is.

# --- pb.go files ---------------------------------------------------------

bazel query 'attr(testonly, 0, //proto/...)' | xargs bazel build $@

file_list=()
while IFS= read -d $'\0' -r file; do
    file_list=("${file_list[@]}" "$file")
done < <($findutil -L "$(bazel info bazel-bin)"/proto -type f -regextype sed -regex ".*pb\.go$" -print0)

arraylength=${#file_list[@]}
searchstring="OffchainLabs/prysm/v7/"

# Copy pb.go files from bazel-bin to original folder where .proto is.
for ((i = 0; i < arraylength; i++)); do
    color "34" "$destination"
    destination=${file_list[i]#*$searchstring}
    cp -R -L "${file_list[i]}" "$destination"
    chmod 755 "$destination"
done

# Run goimports on newly generated protos
# formats imports properly.
# https://github.com/gogo/protobuf/issues/554
goimports -w proto
gofmt -s -w proto

# --- ssz.go files --------------------------------------------------------

bazel query 'kind(ssz_gen_marshal, //proto/...)' | xargs bazel build $@

# Get locations of proto ssz.go files.
file_list=()
while IFS= read -d $'\0' -r file; do
    file_list=("${file_list[@]}" "$file")
done < <($findutil "$(bazel info bazel-bin)"/proto -type f -name "*.ssz.go" -print0)

arraylength=${#file_list[@]}
searchstring="/bin/"

# Copy ssz.go files from bazel-bin to original folder where the target is located.
for ((i = 0; i < arraylength; i++)); do
    destination=${file_list[i]#*$searchstring}
    color "34" "$destination"
    chmod 644 "$destination"

    # Copy to destination while removing the `// Hash: ...` line from the file header.
    sed '/\/\/ Hash: /d' "${file_list[i]}" > "$destination"
done

# --- drift check --------------------------------------------------------
# After regeneration, the only differences from the committed tree we tolerate
# are the two known-benign ones between this Bazel output and `make gen`:
#   - the protoc version header, in some pb.go file:
#       -// 	protoc        (unknown)
#       +// 	protoc        v3.21.7
#   - the leading build constraint, in some pb.go and .ssz.go files:
#       -//go:build !minimal
#       -
# Any other added/removed line under proto/ is real drift and fails the script.
color "34" "Checking generated-code drift..."

# Keep only the diff's added/removed content lines (drop file headers, hunk
# headers and context), then strip out the two known-benign changes. Whatever
# remains is real drift.
drift=$(git diff -U0 -- proto |
    grep -E '^[-+]' |
    grep -vE '^(---|\+\+\+) ' |                              # file headers
    grep -vE '^-[[:space:]]*$' |                             # removed blank line
    grep -vE '^-//go:build !minimal$' |                      # removed build constraint
    grep -vE '^-//[[:space:]]+protoc[[:space:]]+\(unknown\)$' | # old protoc version
    grep -vE '^\+//[[:space:]]+protoc[[:space:]]+v3\.21\.7$' || true) # new protoc version
# `|| true`: a clean tree makes the final grep exit non-zero (no lines left),
# which must not trip `set -e`/`pipefail` — empty `$drift` is the success case.

if [ -n "$drift" ]; then
    color "31" "Unexpected generated-code drift (run 'git diff -- proto' to inspect):"
    echo "$drift"
    exit 1
fi

color "32" "No unexpected generated-code drift."
