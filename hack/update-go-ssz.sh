#!/bin/bash
. "$(dirname "$0")"/common.sh

# Script to regenerate SSZ marshal/unmarshal code.
# Requires: sszgen (github.com/prysmaticlabs/fastssz/sszgen)
#
# Install:
#   go install github.com/prysmaticlabs/fastssz/sszgen@latest

# Find all directories containing ssz-tagged structs under proto/
file_list=()
while IFS= read -d $'\0' -r file; do
    file_list=("${file_list[@]}" "$(dirname "$file")")
done < <($findutil ./proto -type f -name "*_generated.ssz.go" -print0)

# Deduplicate directories
declare -A seen
for dir in "${file_list[@]}"; do
    if [[ -z "${seen[$dir]}" ]]; then
        seen[$dir]=1
        color "34" "Generating SSZ for $dir"
        sszgen --path "$dir"

        # Remove the `// Hash: ...` line from generated files
        for f in "$dir"/*_generated.ssz.go; do
            if [ -f "$f" ]; then
                sed -i'' '/\/\/ Hash: /d' "$f"
            fi
        done
    fi
done
