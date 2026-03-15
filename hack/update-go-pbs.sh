#!/bin/bash
. "$(dirname "$0")"/common.sh

# Script to regenerate pb.go files from proto definitions.
# Requires: protoc, protoc-gen-go, protoc-gen-go-grpc
#
# Install dependencies:
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Find all .proto files under proto/
file_list=()
while IFS= read -d $'\0' -r file; do
    file_list=("${file_list[@]}" "$file")
done < <($findutil ./proto -type f -name "*.proto" -print0)

for proto_file in "${file_list[@]}"; do
    dir=$(dirname "$proto_file")
    color "34" "Compiling $proto_file"
    protoc \
        --proto_path=. \
        --proto_path=proto \
        --go_out=. --go_opt=paths=source_relative \
        --go-grpc_out=. --go-grpc_opt=paths=source_relative \
        "$proto_file"
done

# Run goimports on newly generated protos
# formats imports properly.
# https://github.com/gogo/protobuf/issues/554
goimports -w proto
gofmt -s -w proto
