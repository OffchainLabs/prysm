#!/bin/bash

# Regenerates the committed *.pb.go files with protoc + protoc-gen-go-cast,
# replacing the previous Bazel-wrapping version. Reproduces Bazel's output
# byte-for-byte by replicating what rules_go fed protoc (see comments below).
#
# minimal vs mainnet (Phase 3): five protos use bitvector `.type` tokens whose
# generated Go field TYPE differs between configs (e.g. CommitteeBits is
# Bitvector64 mainnet / Bitvector4 minimal), so they will not compile under
# `-tags=minimal` without a variant. For those we commit a `<name>.minimal.pb.go`
# (`//go:build minimal`) and tag the mainnet twin `//go:build !minimal`. Every
# other proto is config-invariant at the Go-type level (only ssz-size *tag*
# strings differ, which fastssz ignores), so it stays a single untagged file.
#
# Usage:
#   update-go-pbs.sh                      regenerate the committed *.pb.go (+ minimal twins)
#   update-go-pbs.sh --emit-minimal-pbgo DIR
#                                         emit minimal *.pb.go (untagged) for every
#                                         package into DIR/proto/... — used by
#                                         update-go-ssz.sh as sszgen input.

set -euo pipefail
cd "$(dirname "$0")/.."

CAST_PIN="v0.0.0-20230228205207-28762a7b9294"
PROTOBUF_GO_VER="v1.36.3"     # the workspace protobuf-go that produced the committed files
PROTOC_STAMP="v3.21.7"        # committed "// protoc vX" stamp (non-semantic; normalized)

# Generated *.pb.go whose Go field TYPE differs mainnet vs minimal (the protos
# with a `.type` token). Only these get a committed minimal variant. Space-padded
# for whole-token matching.
TYPE_DIFFERING=" proto/prysm/v1alpha1/attestation.pb.go proto/prysm/v1alpha1/sync_committee.pb.go proto/prysm/v1alpha1/beacon_core_types.pb.go proto/prysm/v1alpha1/gloas.pb.go proto/eth/v1/beacon_block.pb.go "

EMIT_MINIMAL_PBGO=""
if [ "${1:-}" = "--emit-minimal-pbgo" ]; then EMIT_MINIMAL_PBGO="${2:?dir required}"; fi

command -v protoc >/dev/null || { echo "protoc not found on PATH" >&2; exit 1; }
WKT_INC="$(cd "$(dirname "$(command -v protoc)")/../include" && pwd)"
GOOGLEAPIS_INC="third_party/googleapis"

TMP_ROOT="$(mktemp -d)"
BIN_DIR="$(mktemp -d)"
PLUGIN_MOD="$(mktemp -d)"
cleanup() { rm -rf "$TMP_ROOT" "$BIN_DIR" "$PLUGIN_MOD"; }
trap cleanup EXIT

# --- build the plugins against protobuf-go v1.36.3, isolated from this module --
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

# --- SSZ substitution dicts (the mainnet/minimal maps from ssz_proto_library.bzl)
# Bazel's ssz_proto_files rule does a plain (bare, unanchored) string replacement
# of each key, so tokens may be embedded in larger strings, e.g.
#   (ssz_size) = "?,bytes_per_cell.size"  ->  "?,2048"
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
minimal_dict() {
  cat <<'EOF'
block_roots.size=64,32
state_roots.size=64,32
eth1_data_votes.size=32
randao_mixes.size=64,32
previous_epoch_attestations.max=1024
current_epoch_attestations.max=1024
slashings.size=64
sync_committee_bits.size=32
sync_committee_bytes.size=4
sync_committee_bits.type=github.com/OffchainLabs/go-bitfield.Bitvector32
sync_committee_aggregate_bytes.size=1
sync_committee_aggregate_bits.type=github.com/OffchainLabs/go-bitfield.Bitvector8
withdrawal.size=4
blob.size=131072
logs_bloom.size=256
extra_data.size=32
max_blobs_per_block.size=6
max_blob_commitments.size=4096
max_cell_proofs_length.size=33554432
kzg_commitment_inclusion_proof_depth.size=17
max_withdrawal_requests_per_payload.size=16
max_deposit_requests_per_payload.size=8192
max_attesting_indices.size=8192
max_committees_per_slot.size=4
committee_bits.size=1
committee_bits.type=github.com/OffchainLabs/go-bitfield.Bitvector4
pending_deposits_limit=134217728
pending_partial_withdrawals_limit=64
pending_consolidations_limit=64
max_consolidation_requests_per_payload.size=2
field_elements_per_cell.size=64
field_elements_per_ext_blob.size=8192
bytes_per_cell.size=2048
cells_per_blob.size=128
kzg_commitments_inclusion_proof_depth.size=4
proposer_lookahead_size=16
ptc_window.size=24
ptc_committee_indices.size=16
ptc.size=2
ptc.type=github.com/OffchainLabs/go-bitfield.Bitvector16
payload_attestation.size=4
execution_payload_availability.size=8
builder_pending_payments.size=16
builder_registry_limit=1099511627776
EOF
}

# build_sedfile <mainnet|minimal> -> path to a sed program (length-sorted longest
# first so a shorter key never corrupts a longer one containing it).
build_sedfile() {
  local out; out="$(mktemp -p "$TMP_ROOT")"
  "${1}_dict" | awk -F= '{print length($1)"\t"$0}' | sort -rn | cut -f2- | while IFS= read -r line; do
    printf 's|%s|%s|g\n' "$(printf '%s' "${line%%=*}" | sed 's/\./\\./g')" "${line#*=}"
  done > "$out"
  echo "$out"
}

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

# eth/v1/data_columns.proto references a v1alpha1 type it does not import, so it
# never compiled to Go; it stays staged only so other imports resolve.
EXCLUDE_PROTOS=" proto/eth/v1/data_columns.proto "

pkg_protos() {
  local pkg=$1 p
  for p in "$pkg"/*.proto; do
    [ -e "$p" ] || continue
    case "$EXCLUDE_PROTOS" in *" $p "*) continue ;; esac
    echo "$p"
  done
}

# generate <network> <out_dir>: substitute the network's SSZ dict into a staged
# proto tree, compile a comment-free descriptor set, run the Go plugins per
# package into out_dir, then strip the leading license + normalize the protoc
# stamp in place. (Files land at out_dir/proto/<pkg>/<name>.pb.go.)
generate() {
  local net=$1 outdir=$2
  local SEDF STAGE DESC MMAP
  SEDF="$(build_sedfile "$net")"
  STAGE="$(mktemp -d -p "$TMP_ROOT")"
  while IFS= read -r f; do
    mkdir -p "$STAGE/$(dirname "$f")"
    sed -f "$SEDF" < "$f" > "$STAGE/$f"
  done < <(find proto -name '*.proto')

  # Import (M) mappings: map every proto file to its dir-derived Go import path so
  # same-package refs stay unqualified (also corrects light_client.proto's typo'd
  # go_package). googleapis maps to its genproto package.
  MMAP=""
  while IFS= read -r f; do
    local rel="${f#"$STAGE"/}"
    MMAP="$MMAP,M$rel=github.com/OffchainLabs/prysm/v7/$(dirname "$rel")"
  done < <(find "$STAGE/proto" -name '*.proto')
  MMAP="$MMAP,Mgoogle/api/annotations.proto=google.golang.org/genproto/googleapis/api/annotations"
  MMAP="$MMAP,Mgoogle/api/http.proto=google.golang.org/genproto/googleapis/api/annotations"

  DESC="$(mktemp -p "$TMP_ROOT")"
  local desc_inputs=() entry p
  for entry in "${PKGS[@]}"; do
    set -- $entry
    while IFS= read -r p; do desc_inputs+=("$STAGE/$p"); done < <(pkg_protos "$1")
  done
  protoc --proto_path="$STAGE" --proto_path="$GOOGLEAPIS_INC" --proto_path="$WKT_INC" \
    --include_imports --descriptor_set_out="$DESC" "${desc_inputs[@]}"

  mkdir -p "$outdir"
  for entry in "${PKGS[@]}"; do
    set -- $entry
    local pkg=$1 mode=$2 opt="paths=source_relative$MMAP" optflag out_flag
    local plugin_flag=() protos=()
    while IFS= read -r p; do protos+=("$p"); done < <(pkg_protos "$pkg")
    echo "  [$net] $pkg ($mode): ${#protos[@]} files"
    case "$mode" in
      cast)      plugin_flag=(--plugin=protoc-gen-go-cast="$BIN_DIR/protoc-gen-go-cast"); out_flag=--go-cast_out="$outdir"; optflag=--go-cast_opt ;;
      cast_grpc) plugin_flag=(--plugin=protoc-gen-go-cast="$BIN_DIR/protoc-gen-go-cast"); out_flag=--go-cast_out="$outdir"; optflag=--go-cast_opt; opt="$opt,plugins=grpc" ;;
      stock)     plugin_flag=(--plugin=protoc-gen-go="$BIN_DIR/protoc-gen-go");           out_flag=--go_out="$outdir";      optflag=--go_opt ;;
    esac
    protoc --descriptor_set_in="$DESC" "${plugin_flag[@]}" "$out_flag" "$optflag=$opt" "${protos[@]}"
  done

  # Strip the leading license block (matching Bazel's reset plugin) + normalize
  # the non-semantic "// protoc vX" stamp, in place.
  local gen tmp
  while IFS= read -r gen; do
    tmp="$(mktemp -p "$TMP_ROOT")"
    awk 'f||/^\/\/ Code generated by protoc-gen-go/{f=1} f' "$gen" \
      | sed -E "s|^(//[[:space:]]*protoc) +v[0-9].*|\1        $PROTOC_STAMP|" > "$tmp"
    mv "$tmp" "$gen"
  done < <(find "$outdir" -name '*.pb.go')
}

# prepend a build constraint to a file on the way to a destination.
write_tagged() { # write_tagged <tag-expr> <src> <dest>
  { printf '//go:build %s\n\n' "$1"; cat "$2"; } > "$3"
  chmod 755 "$3"
}

# --- mode: emit minimal *.pb.go for sszgen input, then exit -------------------
if [ -n "$EMIT_MINIMAL_PBGO" ]; then
  echo "emitting minimal *.pb.go into $EMIT_MINIMAL_PBGO"
  generate minimal "$EMIT_MINIMAL_PBGO"
  exit 0
fi

# --- normal mode: write committed mainnet files + minimal twins ---------------
OUT_MAIN="$(mktemp -d -p "$TMP_ROOT")"
OUT_MIN="$(mktemp -d -p "$TMP_ROOT")"
echo "generating mainnet *.pb.go"; generate mainnet "$OUT_MAIN"
echo "generating minimal *.pb.go (twins for type-differing protos)"; generate minimal "$OUT_MIN"

while IFS= read -r gen; do
  dest="${gen#"$OUT_MAIN"/}"
  case "$TYPE_DIFFERING" in
    *" $dest "*)
      write_tagged '!minimal' "$gen" "$dest"
      write_tagged 'minimal' "$OUT_MIN/$dest" "${dest%.pb.go}.minimal.pb.go"
      ;;
    *)
      cp "$gen" "$dest"; chmod 755 "$dest"
      ;;
  esac
done < <(find "$OUT_MAIN" -name '*.pb.go')

go run golang.org/x/tools/cmd/goimports -w proto
gofmt -s -w proto
