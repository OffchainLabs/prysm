#!/bin/bash

# Regenerates the committed *.pb.go files with protoc + protoc-gen-go-cast,
# replacing the previous Bazel-wrapping version. Reproduces Bazel's output
# byte-for-byte (verified) by replicating what rules_go fed protoc:
#
#  1. The cast plugin (and stock protoc-gen-go for the non-cast packages) is built
#     against protobuf-go v1.36.3 — the version Bazel's workspace links — which
#     fixes the generated rawDesc format / imports / "protoc-gen-go vX" header.
#  2. Templated .proto files get the mainnet SSZ dict substituted before protoc
#     (the ssz_proto_files rule), e.g. "block_roots.size" -> "8192,32".
#  3. protoc runs with paths=source_relative; googleapis + WKT are on the proto
#     path (googleapis vendored under third_party/).
#  4. Post-processing matches Bazel's "reset" plugin + the old copy-back:
#     strip the leading license comment, normalize the non-semantic
#     "// protoc vX" stamp, then goimports + gofmt.
#
# protoc itself is whatever is on PATH: its version only appears in a comment we
# normalize, and it does not affect the generated descriptor bytes.

set -euo pipefail
cd "$(dirname "$0")/.."

CAST_PIN="v0.0.0-20230228205207-28762a7b9294"
PROTOBUF_GO_VER="v1.36.3"     # the workspace protobuf-go that produced the committed files
PROTOC_STAMP="v3.21.7"        # committed "// protoc vX" stamp (non-semantic; normalized)

command -v protoc >/dev/null || { echo "protoc not found on PATH" >&2; exit 1; }
WKT_INC="$(cd "$(dirname "$(command -v protoc)")/../include" && pwd)"
GOOGLEAPIS_INC="third_party/googleapis"

# --- build the plugins against protobuf-go v1.36.3, isolated from this module --
BIN_DIR="$(mktemp -d)"
PLUGIN_MOD="$(mktemp -d)"
STAGE=""; OUT=""; DESC=""; SEDFILE=""
cleanup() { rm -rf "$BIN_DIR" "$PLUGIN_MOD" "${STAGE:-/nonexistent}" "${OUT:-/nonexistent}" "${DESC:-/nonexistent}" "${SEDFILE:-/nonexistent}"; }
trap cleanup EXIT
cat > "$PLUGIN_MOD/go.mod" <<EOF
module pluginbuild
go 1.23
require github.com/prysmaticlabs/protoc-gen-go-cast $CAST_PIN
require google.golang.org/protobuf $PROTOBUF_GO_VER
EOF
echo "building protoc-gen-go-cast + protoc-gen-go against protobuf-go $PROTOBUF_GO_VER"
( cd "$PLUGIN_MOD"
  GOFLAGS=-mod=mod go build -o "$BIN_DIR/protoc-gen-go-cast" github.com/prysmaticlabs/protoc-gen-go-cast
  GOFLAGS=-mod=mod go build -o "$BIN_DIR/protoc-gen-go" google.golang.org/protobuf/cmd/protoc-gen-go )

# --- stage the proto tree, substituting the mainnet SSZ dict in templated protos
# The dict is the `mainnet` map from proto/ssz_proto_library.bzl. Bazel's
# ssz_proto_files rule does a plain (bare, unanchored) string replacement of each
# key, so tokens may be embedded in larger strings, e.g.
#   (ssz_size) = "?,bytes_per_cell.size"  ->  "?,2048"
# We build the sed program sorted by key length DESCENDING so a shorter key never
# corrupts a longer one that contains it (e.g. blob.size in cells_per_blob.size,
# committee_bits.size in sync_committee_bits.size). Dots in keys are escaped.
mainnet_dict() {
  cat <<'EOF'
block_roots.size=8192,32
state_roots.size=8192,32
eth1_data_votes.size=2048
randao_mixes.size=65536,32
previous_epoch_attestations.max=4096
current_epoch_attestations.max=4096
slashings.size=8192
sync_committee_bits.size=512
sync_committee_bytes.size=64
sync_committee_bits.type=github.com/OffchainLabs/go-bitfield.Bitvector512
sync_committee_aggregate_bytes.size=16
sync_committee_aggregate_bits.type=github.com/OffchainLabs/go-bitfield.Bitvector128
withdrawal.size=16
blob.size=131072
logs_bloom.size=256
extra_data.size=32
max_blobs_per_block.size=6
max_blob_commitments.size=4096
max_cell_proofs_length.size=33554432
kzg_commitment_inclusion_proof_depth.size=17
max_withdrawal_requests_per_payload.size=16
max_deposit_requests_per_payload.size=8192
max_attesting_indices.size=131072
max_committees_per_slot.size=64
committee_bits.size=8
committee_bits.type=github.com/OffchainLabs/go-bitfield.Bitvector64
pending_deposits_limit=134217728
pending_partial_withdrawals_limit=134217728
pending_consolidations_limit=262144
max_consolidation_requests_per_payload.size=2
field_elements_per_cell.size=64
field_elements_per_ext_blob.size=8192
bytes_per_cell.size=2048
cells_per_blob.size=128
kzg_commitments_inclusion_proof_depth.size=4
proposer_lookahead_size=64
ptc_window.size=96
ptc_committee_indices.size=512
ptc.size=64
ptc.type=github.com/OffchainLabs/go-bitfield.Bitvector512
payload_attestation.size=4
execution_payload_availability.size=1024
builder_pending_payments.size=64
builder_registry_limit=1099511627776
EOF
}

SEDFILE="$(mktemp)"
mainnet_dict | awk -F= '{print length($1)"\t"$0}' | sort -rn | cut -f2- | while IFS= read -r line; do
  key=${line%%=*}; val=${line#*=}
  printf 's|%s|%s|g\n' "$(printf '%s' "$key" | sed 's/\./\\./g')" "$val"
done > "$SEDFILE"
mainnet_subst() { sed -f "$SEDFILE"; }

STAGE="$(mktemp -d)"
while IFS= read -r f; do
  mkdir -p "$STAGE/$(dirname "$f")"
  mainnet_subst < "$f" > "$STAGE/$f"
done < <(find proto -name '*.proto')

# --- import (M) mappings -------------------------------------------------------
# Bazel passes -import file=goimportpath for every proto so all files in a Go
# package share one import path (same-package refs stay unqualified). We derive
# the import path from each proto's directory, which also corrects a typo'd
# go_package (light_client.proto declares .../proto/eth/v1alpha1 instead of
# .../proto/prysm/v1alpha1). googleapis maps to its genproto package.
MMAP=""
while IFS= read -r f; do
  rel="${f#"$STAGE"/}"
  MMAP="$MMAP,M$rel=github.com/OffchainLabs/prysm/v7/$(dirname "$rel")"
done < <(find "$STAGE/proto" -name '*.proto')
MMAP="$MMAP,Mgoogle/api/annotations.proto=google.golang.org/genproto/googleapis/api/annotations"
MMAP="$MMAP,Mgoogle/api/http.proto=google.golang.org/genproto/googleapis/api/annotations"

# --- proto packages and their plugin mode -------------------------------------
# proto/testing is testonly and was never covered by the Bazel script (its
# test.proto has no go_package), so it is omitted.
PKGS=(
  "proto/eth/ext cast"
  "proto/dbval stock"
  "proto/engine/v1 cast_grpc"
  "proto/eth/v1 cast_grpc"
  "proto/prysm/v1alpha1 cast_grpc"
  "proto/prysm/v1alpha1/validator-client cast_grpc"
  "proto/ssz_query cast_grpc"
  "proto/ssz_query/testing cast_grpc"
)

# Protos that exist on disk but are NOT part of any go_proto target (no committed
# *.pb.go). eth/v1/data_columns.proto references a v1alpha1 type it does not
# import, so it never compiled to Go; it stays staged only so other imports
# resolve. Listed with surrounding spaces for whole-token matching.
EXCLUDE_PROTOS=" proto/eth/v1/data_columns.proto "

# proto names to compile for a package: every *.proto in the dir except the
# excluded ones. Driving off the .proto sources (not committed *.pb.go) means a
# deleted *.pb.go is regenerated rather than silently skipped.
pkg_protos() {
  local pkg=$1 p
  for p in "$pkg"/*.proto; do
    [ -e "$p" ] || continue
    case "$EXCLUDE_PROTOS" in *" $p "*) continue ;; esac
    echo "$p"
  done
}

# --- build a comment-free descriptor set --------------------------------------
# rules_go's go_proto "reset" plugin clears source_code_info so the generated
# files carry no proto comments (or license header). Reproduce that by compiling
# to a descriptor set WITHOUT --include_source_info, then driving the Go plugins
# from that set via --descriptor_set_in.
DESC="$(mktemp)"
desc_inputs=()
for entry in "${PKGS[@]}"; do
  set -- $entry
  while IFS= read -r p; do desc_inputs+=("$STAGE/$p"); done < <(pkg_protos "$1")
done
protoc --proto_path="$STAGE" --proto_path="$GOOGLEAPIS_INC" --proto_path="$WKT_INC" \
  --include_imports --descriptor_set_out="$DESC" "${desc_inputs[@]}"

# --- per-package generation from the descriptor set ---------------------------
OUT="$(mktemp -d)"
gen_pb() {
  local pkg=$1 mode=$2 opt="paths=source_relative$MMAP" optflag out_flag
  local plugin_flag=() protos=()
  while IFS= read -r p; do protos+=("$p"); done < <(pkg_protos "$pkg")
  echo "generating $pkg ($mode): ${#protos[@]} files"
  case "$mode" in
    cast)      plugin_flag=(--plugin=protoc-gen-go-cast="$BIN_DIR/protoc-gen-go-cast"); out_flag=--go-cast_out="$OUT"; optflag=--go-cast_opt ;;
    cast_grpc) plugin_flag=(--plugin=protoc-gen-go-cast="$BIN_DIR/protoc-gen-go-cast"); out_flag=--go-cast_out="$OUT"; optflag=--go-cast_opt; opt="$opt,plugins=grpc" ;;
    stock)     plugin_flag=(--plugin=protoc-gen-go="$BIN_DIR/protoc-gen-go");           out_flag=--go_out="$OUT";      optflag=--go_opt ;;
  esac
  protoc --descriptor_set_in="$DESC" "${plugin_flag[@]}" "$out_flag" "$optflag=$opt" "${protos[@]}"
}

for entry in "${PKGS[@]}"; do
  set -- $entry
  gen_pb "$1" "$2"
done

# --- copy generated files back, normalizing to the committed form -------------
# Strip the leading license comment block (matching Bazel's reset plugin) and
# normalize the non-semantic "// protoc vX" stamp.
while IFS= read -r gen; do
  dest="${gen#"$OUT"/}"
  awk 'f||/^\/\/ Code generated by protoc-gen-go/{f=1} f' "$gen" \
    | sed -E "s|^(//[[:space:]]*protoc) +v[0-9].*|\1        $PROTOC_STAMP|" \
    > "$dest"
  chmod 755 "$dest"   # match the committed mode (the Bazel-era copy-back chmod'd 755)
done < <(find "$OUT" -name '*.pb.go')

go run golang.org/x/tools/cmd/goimports -w proto
gofmt -s -w proto
