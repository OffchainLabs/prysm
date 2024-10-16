"""
SSZ proto templating rules.

These rules allow for variable substitution for hardcoded tag values like ssz-size and ssz-max.
"""

####### Configuration #######

mainnet = {
    "block_roots.size": "8192,32",  # SLOTS_PER_HISTORICAL_ROOT, [32]byte
    "state_roots.size": "8192,32",  # SLOTS_PER_HISTORICAL_ROOT, [32]byte
    "eth1_data_votes.size": "2048",  # SLOTS_PER_ETH1_VOTING_PERIOD
    "randao_mixes.size": "65536,32",  # EPOCHS_PER_HISTORICAL_VECTOR, [32]byte
    "previous_epoch_attestations.max": "4096",  # MAX_ATTESTATIONS * SLOTS_PER_EPOCH
    "current_epoch_attestations.max": "4096",  # MAX_ATTESTATIONS * SLOTS_PER_EPOCH
    "slashings.size": "8192",  # EPOCHS_PER_SLASHINGS_VECTOR
    "sync_committee_bits.size": "512",  # SYNC_COMMITTEE_SIZE
    "sync_committee_bytes.size": "64",
    "sync_committee_bits.type": "github.com/prysmaticlabs/go-bitfield.Bitvector512",
    "sync_committee_aggregate_bytes.size": "16",
    "sync_committee_aggregate_bits.type": "github.com/prysmaticlabs/go-bitfield.Bitvector128",
    "withdrawal.size": "16",
    "blob.size": "131072",  # BYTES_PER_FIELD_ELEMENT * FIELD_ELEMENTS_PER_BLOB
    "logs_bloom.size": "256",
    "extra_data.size": "32",
    "max_blobs_per_block.size": "6",
    "max_blob_commitments.size": "4096",
    "kzg_commitment_inclusion_proof_depth.size": "17",
    "max_withdrawal_requests_per_payload.size":"16",
    "max_deposit_requests_per_payload.size": "8192",
    "max_attesting_indices.size": "131072",
    "max_committees_per_slot.size": "64",
    "committee_bits.size": "8",
    "committee_bits.type": "github.com/prysmaticlabs/go-bitfield.Bitvector64",
    "pending_deposits_limit": "134217728",
    "pending_partial_withdrawals_limit": "134217728",
    "pending_consolidations_limit": "262144",
    "max_consolidation_requests_per_payload.size": "1",
    "max_inclusion_list.size": "1024", #MAX_TRANSACTIONS_PER_INCLUSION_LIST
    "ptc.size": "64", #PTC_SIZE
    "ptc.type": "github.com/prysmaticlabs/go-bitfield.Bitvector512",
    "payload_attestation.size": "4",
}

minimal = {
    "block_roots.size": "64,32",
    "state_roots.size": "64,32",
    "eth1_data_votes.size": "32",
    "randao_mixes.size": "64,32",
    "previous_epoch_attestations.max": "1024",
    "current_epoch_attestations.max": "1024",
    "slashings.size": "64",
    "sync_committee_bits.size": "32",
    "sync_committee_bytes.size": "4",
    "sync_committee_bits.type": "github.com/prysmaticlabs/go-bitfield.Bitvector32",
    "sync_committee_aggregate_bytes.size": "1",
    "sync_committee_aggregate_bits.type": "github.com/prysmaticlabs/go-bitfield.Bitvector8",
    "withdrawal.size": "4",
    "blob.size": "131072",
    "logs_bloom.size": "256",
    "extra_data.size": "32",
    "max_blobs_per_block.size": "6",
    "max_blob_commitments.size": "16",
    "kzg_commitment_inclusion_proof_depth.size": "9",
    "max_withdrawal_requests_per_payload.size":"2",
    "max_deposit_requests_per_payload.size": "4",
    "max_attesting_indices.size": "8192",
    "max_committees_per_slot.size": "4",
    "committee_bits.size": "1",
    "committee_bits.type": "github.com/prysmaticlabs/go-bitfield.Bitvector4",
    "pending_deposits_limit": "134217728",
    "pending_partial_withdrawals_limit": "64",
    "pending_consolidations_limit": "64",
    "max_consolidation_requests_per_payload.size": "1",
    "max_inclusion_list.size": "16", #MAX_TRANSACTIONS_PER_INCLUSION_LIST
    "ptc.size": "4", #PTC_SIZE
    "ptc.type": "github.com/prysmaticlabs/go-bitfield.Bitvector32",
    "payload_attestation.size": "4",
}

###### Rules definitions #######

def _ssz_proto_files_impl(ctx):
    """
    ssz_proto_files implementation performs expand_template based on the value of "config".
    """
    outputs = []
    if (ctx.attr.config.lower() == "mainnet"):
        subs = mainnet
    elif (ctx.attr.config.lower() == "minimal"):
        subs = minimal
    else:
        fail("%s is an unknown configuration" % ctx.attr.config)

    for src in ctx.attr.srcs:
        output = ctx.actions.declare_file(src.files.to_list()[0].basename)
        outputs.append(output)
        ctx.actions.expand_template(
            template = src.files.to_list()[0],
            output = output,
            substitutions = subs,
        )

    return [DefaultInfo(files = depset(outputs))]

ssz_proto_files = rule(
    implementation = _ssz_proto_files_impl,
    attrs = {
        "srcs": attr.label_list(mandatory = True, allow_files = [".proto"]),
        "config": attr.string(mandatory = True),
    },
)
