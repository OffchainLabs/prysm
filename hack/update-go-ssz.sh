#!/bin/bash
. "$(dirname "$0")"/common.sh

# Script to copy ssz.go files from bazel build folder to appropriate location.
# Bazel builds to bazel-bin/... folder, script copies them back to original folder where target is.

# Optional first positional arg `progressive` regenerates with SSZ progressive
# merkleization ON, overriding the .bazelrc default that gates it off
# (--//tools:disable_progressive_merkleization). Without it, generation matches
# the current non-progressive spectest fixtures. Any remaining args pass through
# to `bazel build`.
progressive_flag=""
if [[ "${1:-}" == "progressive" ]]; then
    progressive_flag="--//tools:disable_progressive_merkleization=false"
    shift
    color "32" "regenerating with progressive merkleization ON"
fi

bazel query 'kind(ssz_methodical, //proto/...)' | xargs bazel build $progressive_flag "$@"

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
