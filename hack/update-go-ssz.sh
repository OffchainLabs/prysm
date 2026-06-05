#!/bin/bash

# Regenerates the committed *.ssz.go files with fastssz/sszgen, pinned via go.mod
# and run as `go tool sszgen`. Replaces the previous Bazel-wrapping version.
#
# sszgen must run against a directory that contains ONLY the generated *.pb.go:
# the hand-written package files import "reflect" with a different alias than the
# generated code, which makes sszgen's loader fail. So for each target we stage
# the package's *.pb.go into a temporary dir and point --path there, mirroring
# the generated-only source set Bazel fed to the ssz_gen_marshal rule.

set -euo pipefail
cd "$(dirname "$0")/.."

# Remove any leftover staging dirs on exit, so a failed sszgen run never leaves
# generated-only *.pb.go copies polluting the proto packages.
cleanup() {
  find proto -type d \( -name '.sszgen_tmp' -o -name '.sszinc_tmp' \) 2>/dev/null \
    | while IFS= read -r d; do rm -rf "$d"; done
}
trap cleanup EXIT

sszgen() { go tool sszgen "$@"; }
join() { local IFS=,; echo "$*"; }

stage_pbgo() { # stage_pbgo <pkg_dir> <stage_dir> : copy only the generated *.pb.go
  mkdir -p "$2"
  cp "$1"/*.pb.go "$2/"
}
unstage() { find "$1" -type f -delete; rmdir "$1"; }

# gen_ssz <pkg_dir> <out> <lib-includes-csv> <proto-includes-csv> <objs-csv> [<exclude-objs-csv>]
#
# sszgen must see a package as ONLY its generated *.pb.go. That is true both for
# the target package (--path) and for any proto-package --include: pointing at the
# source dir (which also holds hand-written files) makes sszgen mis-resolve
# cross-package references (e.g. BeaconStateGloas -> []*enginev1.Withdrawal) and
# panic with "create not implemented for type reference". So proto includes are
# staged into generated-only dirs too, mirroring Bazel's go_proto_ output dir.
# Library includes (consensus-types/primitives, math) are plain Go packages and
# are passed as their source dirs, exactly as Bazel does.
gen_ssz() {
  local pkg=$1 out=$2 lib_inc=$3 proto_inc=$4 objs=$5 exclude=${6:-}
  local stage="$pkg/.sszgen_tmp"
  local tmp_out staged_incs=() inc="$lib_inc"
  tmp_out=$(mktemp)
  echo "generating $pkg/$out"

  stage_pbgo "$pkg" "$stage"
  if [ -n "$proto_inc" ]; then
    local p
    for p in $(echo "$proto_inc" | tr ',' ' '); do
      local istage="$p/.sszinc_tmp"
      stage_pbgo "$p" "$istage"
      staged_incs+=("$istage")
      inc="${inc:+$inc,}$istage"
    done
  fi

  local args=(--output="$tmp_out" --path="$stage" --objs="$objs")
  [ -n "$inc" ] && args+=(--include="$inc")
  [ -n "$exclude" ] && args+=(--exclude-objs="$exclude")
  sszgen "${args[@]}"

  # Strip the `// Hash: ...` line (as the old bazel copy-back did) on the way in.
  sed '/\/\/ Hash: /d' "$tmp_out" > "$pkg/$out"

  rm -f "$tmp_out"
  unstage "$stage"
  # Guard the array expansion: macOS bash 3.2 treats "${empty[@]}" as unbound
  # under `set -u`.
  if [ "${#staged_incs[@]}" -gt 0 ]; then
    local d
    for d in "${staged_incs[@]}"; do unstage "$d"; done
  fi
}

# --- proto/prysm/v1alpha1 -----------------------------------------------------
# Fork files share one Go package, so each fork's objs are generated while
# excluding all prior forks' objs (cumulative), matching the BUILD layering.
v1=proto/prysm/v1alpha1
lib_v1="consensus-types/primitives,math"
proto_v1="proto/engine/v1"

phase0_objs=(AggregateAttestationAndProof Attestation AttestationData AttesterSlashing
  BeaconBlock BeaconBlockHeader BeaconState Checkpoint Deposit Deposit_Data DepositMessage
  ENRForkID Eth1Data Fork ForkData HistoricalBatch IndexedAttestation PowBlock ProposerSlashing
  SignedAggregateAttestationAndProof SignedBeaconBlock SignedBeaconBlockHeader SignedVoluntaryExit
  SigningData Status Status Validator ValidatorIdentity VoluntaryExit)

altair_objs=(BeaconBlockAltair BeaconBlockBodyAltair BeaconStateAltair ContributionAndProof
  LightClientHeaderAltair LightClientBootstrapAltair LightClientUpdateAltair
  LightClientFinalityUpdateAltair LightClientOptimisticUpdateAltair SignedBeaconBlockAltair
  SignedContributionAndProof SyncAggregate SyncAggregate SyncAggregatorSelectionData SyncCommittee
  SyncCommitteeContribution SyncCommitteeMessage)

bellatrix_objs=(BeaconBlockBellatrix BeaconBlockBodyBellatrix BeaconStateBellatrix
  BlindedBeaconBlockBellatrix BlindedBeaconBlockBodyBellatrix SignedBeaconBlockBellatrix
  SignedBlindedBeaconBlockBellatrix)

capella_objs=(BLSToExecutionChange BeaconBlockBodyCapella BeaconBlockCapella BeaconStateCapella
  BlindedBeaconBlockBodyCapella BlindedBeaconBlockCapella BuilderBidCapella HistoricalSummary
  LightClientHeaderCapella LightClientBootstrapCapella LightClientUpdateCapella
  LightClientFinalityUpdateCapella LightClientOptimisticUpdateCapella SignedBLSToExecutionChange
  SignedBeaconBlockCapella SignedBlindedBeaconBlockCapella Withdrawal SignedBuilderBidCapella)

deneb_objs=(BeaconBlockBodyDeneb BeaconBlockContentsDeneb BeaconBlockDeneb BeaconStateDeneb
  BlindedBeaconBlockBodyDeneb BlindedBeaconBlockDeneb BlobIdentifier BlobSidecar BlobSidecars
  BuilderBidDeneb LightClientHeaderDeneb LightClientBootstrapDeneb LightClientUpdateDeneb
  LightClientFinalityUpdateDeneb LightClientOptimisticUpdateDeneb SignedBeaconBlockContentsDeneb
  SignedBeaconBlockDeneb SignedBlindedBeaconBlockDeneb SignedBuilderBidDeneb)

electra_objs=(AggregateAttestationAndProofElectra AggregateAttestationAndProofSingle
  AttestationElectra AttesterSlashingElectra BeaconBlockElectra BeaconBlockBodyElectra
  BeaconBlockContentsElectra BeaconStateElectra BlindedBeaconBlockBodyElectra BlindedBeaconBlockElectra
  BuilderBidElectra Consolidation IndexedAttestationElectra LightClientHeaderElectra
  LightClientBootstrapElectra LightClientUpdateElectra LightClientFinalityUpdateElectra
  PendingDeposit PendingDeposits PendingConsolidation PendingPartialWithdrawal
  SignedAggregateAttestationAndProofElectra SignedAggregateAttestationAndProofSingle
  SignedBeaconBlockContentsElectra SignedBeaconBlockElectra SignedBlindedBeaconBlockElectra
  SignedConsolidation SingleAttestation SignedBuilderBidElectra)

fulu_objs=(BeaconBlockContentsFulu BeaconStateFulu BlindedBeaconBlockFulu DataColumnIdentifier
  DataColumnsByRootIdentifier DataColumnSidecar StatusV2 SignedBeaconBlockContentsFulu
  SignedBeaconBlockFulu SignedBlindedBeaconBlockFulu)

gloas_objs=(BlindedExecutionPayloadEnvelope BuilderPendingPayment BuilderPendingWithdrawal
  DataColumnSidecarGloas ExecutionPayloadEnvelope PTCs ProposerPreferences SignedProposerPreferences
  PayloadAttestation PayloadAttestationData PayloadAttestationMessage ExecutionPayloadBid
  SignedExecutionPayloadBid SignedBlindedExecutionPayloadEnvelope SignedExecutionPayloadEnvelope
  BeaconBlockGloas BeaconBlockContentsGloas SignedBeaconBlockGloas BeaconStateGloas)

non_core_objs=(BeaconBlocksByRangeRequest BlobSidecarsByRangeRequest DataColumnSidecarsByRangeRequest
  ExecutionPayloadEnvelopesByRangeRequest MetaDataV0 MetaDataV1 MetaDataV2
  SignedValidatorRegistrationV1 ValidatorRegistrationV1 BuilderBid SignedBuilderBid DepositSnapshot)

gen_ssz "$v1" phase0.ssz.go    "$lib_v1" "$proto_v1" "$(join "${phase0_objs[@]}")"
gen_ssz "$v1" altair.ssz.go    "$lib_v1" "$proto_v1" "$(join "${altair_objs[@]}")"    "$(join "${phase0_objs[@]}")"
gen_ssz "$v1" bellatrix.ssz.go "$lib_v1" "$proto_v1" "$(join "${bellatrix_objs[@]}")" "$(join "${phase0_objs[@]}" "${altair_objs[@]}")"
gen_ssz "$v1" capella.ssz.go   "$lib_v1" "$proto_v1" "$(join "${capella_objs[@]}")"   "$(join "${phase0_objs[@]}" "${altair_objs[@]}" "${bellatrix_objs[@]}")"
gen_ssz "$v1" deneb.ssz.go     "$lib_v1" "$proto_v1" "$(join "${deneb_objs[@]}")"     "$(join "${phase0_objs[@]}" "${altair_objs[@]}" "${bellatrix_objs[@]}" "${capella_objs[@]}")"
gen_ssz "$v1" electra.ssz.go   "$lib_v1" "$proto_v1" "$(join "${electra_objs[@]}")"   "$(join "${phase0_objs[@]}" "${altair_objs[@]}" "${bellatrix_objs[@]}" "${capella_objs[@]}" "${deneb_objs[@]}")"
gen_ssz "$v1" fulu.ssz.go      "$lib_v1" "$proto_v1" "$(join "${fulu_objs[@]}")"      "$(join "${phase0_objs[@]}" "${altair_objs[@]}" "${bellatrix_objs[@]}" "${capella_objs[@]}" "${deneb_objs[@]}" "${electra_objs[@]}")"
gen_ssz "$v1" gloas.ssz.go     "$lib_v1" "$proto_v1" "$(join "${gloas_objs[@]}")"     "$(join "${phase0_objs[@]}" "${altair_objs[@]}" "${bellatrix_objs[@]}" "${capella_objs[@]}" "${deneb_objs[@]}" "${electra_objs[@]}" "${fulu_objs[@]}")"
gen_ssz "$v1" non-core.ssz.go  "$lib_v1" "$proto_v1" "$(join "${non_core_objs[@]}")"

# --- other proto packages -----------------------------------------------------
engine_objs=(ExecutionPayload ExecutionPayloadCapella ExecutionPayloadHeader
  ExecutionPayloadHeaderCapella ExecutionPayloadHeaderDeneb ExecutionPayloadDeneb ExecutionPayloadGloas
  ExecutionPayloadDenebAndBlobsBundle ExecutionPayloadDenebAndBlobsBundleV2 BlindedBlobsBundle
  BlobsBundle BlobsBundleV2 Withdrawal WithdrawalRequest DepositRequest ConsolidationRequest
  ExecutionRequests)
gen_ssz proto/engine/v1 engine.ssz.go "consensus-types/primitives" "" "$(join "${engine_objs[@]}")"

eth_v1_objs=(AggregateAttestationAndProof Attestation AttestationData AttesterSlashing BeaconBlock
  BeaconBlockHeader Checkpoint Deposit DepositData Eth1Data IndexedAttestation ProposerSlashing
  SignedAggregateAttestationAndProof SignedBeaconBlock SignedBeaconBlockHeader SignedVoluntaryExit
  SyncAggregate Validator VoluntaryExit)
gen_ssz proto/eth/v1 gateway.ssz.go "consensus-types/primitives" "proto/engine/v1" "$(join "${eth_v1_objs[@]}")"

gen_ssz proto/ssz_query response.ssz.go "" "" "SSZQueryProof,SSZQueryResponse,SSZQueryResponseWithProof"

gen_ssz proto/ssz_query/testing test_containers.ssz.go "" "" "FixedTestContainer,FixedNestedContainer,VariableTestContainer"
