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
copied=()
for ((i = 0; i < arraylength; i++)); do
    destination=${file_list[i]#*$searchstring}
    copied=("${copied[@]}" "$destination")
    color "34" "$destination"
    # Bazel's protoc output carries no //go:build constraint, while `make gen`
    # (build/gen/proto.go) stamps one on the preset-varying pb.go pairs. If the
    # committed file leads with a constraint and the Bazel copy does not,
    # re-prepend it so both codegen paths produce identical files.
    constraint=""
    if [ -f "$destination" ]; then
        committed_first=$(head -n 1 "$destination")
        if [[ "$committed_first" == "//go:build "* && "$(head -n 1 "${file_list[i]}")" != "//go:build "* ]]; then
            constraint="$committed_first"
        fi
    fi
    if [ -n "$constraint" ]; then
        chmod 755 "$destination"
        { printf '%s\n\n' "$constraint"; cat -- "${file_list[i]}"; } > "$destination"
    else
        cp -R -L "${file_list[i]}" "$destination"
    fi
    chmod 755 "$destination"
done

# Run goimports on the newly copied protos to format imports properly
# (https://github.com/gogo/protobuf/issues/554). Scope it to the files this
# script wrote: reformatting files it didn't touch (e.g. the `make gen`
# authored *.minimal.ssz.go twins) would show up as spurious drift.
goimports -w "${copied[@]}"
gofmt -s -w "${copied[@]}"

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
# After regeneration, the only difference from the committed tree we tolerate
# is the known-benign protoc version header change between this Bazel output
# and `make gen`, in some pb.go files:
#       -// 	protoc        (unknown)
#       +// 	protoc        v3.21.7
# Any other added/removed line under proto/ is real drift and fails the script.
# In particular the //go:build (!)minimal constraints must match: both codegen
# paths stamp them via methodical's --go-build-constraint flag.
color "34" "Checking generated-code drift..."

# Keep only the diff's added/removed content lines (drop file headers, hunk
# headers and context), then strip out the known-benign change. Whatever
# remains is real drift.
drift=$(git diff -U0 -- proto |
    grep -E '^[-+]' |
    grep -vE '^(---|\+\+\+) ' |                              # file headers
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
