"""
SSZ proto templating rules.

These rules allow for variable substitution for hardcoded tag values like ssz-size and ssz-max.
"""

####### Configuration #######

mainnet = {
    "block_roots.size": "8192,32",  # SLOTS_PER_HISTORICAL_ROOT, [32]byte
    "state_roots.size": "8192,32",  # SLOTS_PER_HISTORICAL_ROOT, [32]byte
    "sync_committee_bits.size": "512",  # SYNC_COMMITTEE_SIZE
    "sync_committee_bytes.size": "64",
    "sync_committee_bits.type": "github.com/OffchainLabs/go-bitfield.Bitvector512",
    "sync_committee_aggregate_bytes.size": "16",
    "sync_committee_aggregate_bits.type": "github.com/OffchainLabs/go-bitfield.Bitvector128",
    "withdrawal.size": "16",
    "blob.size": "131072",  # BYTES_PER_FIELD_ELEMENT * FIELD_ELEMENTS_PER_BLOB
    "logs_bloom.size": "256",
    "extra_data.size": "32",
    "max_blobs_per_block.size": "6",
    "max_blob_commitments.size": "4096",
    "max_cell_proofs_length.size": "33554432",  # FIELD_ELEMENTS_PER_EXT_BLOB * MAX_BLOB_COMMITMENTS_PER_BLOCK
    "kzg_commitment_inclusion_proof_depth.size": "17",
    "max_withdrawal_requests_per_payload.size": "16",
    "max_deposit_requests_per_payload.size": "8192",
    "max_builder_deposit_requests_per_payload.size": "256",  # MAX_BUILDER_DEPOSIT_REQUESTS_PER_PAYLOAD (2**8)
    "max_builder_exit_requests_per_payload.size": "16",  # MAX_BUILDER_EXIT_REQUESTS_PER_PAYLOAD (2**4)
    "max_attesting_indices.size": "131072",
    "max_committees_per_slot.size": "64",
    "committee_bits.size": "8",
    "committee_bits.type": "github.com/OffchainLabs/go-bitfield.Bitvector64",
    "max_consolidation_requests_per_payload.size": "2",
    "field_elements_per_cell.size": "64",
    "field_elements_per_ext_blob.size": "8192",
    "bytes_per_cell.size": "2048",  # FIELD_ELEMENTS_PER_CELL * BYTES_PER_FIELD_ELEMENT
    "cells_per_blob.size": "128",
    "kzg_commitments_inclusion_proof_depth.size": "4",
    "ptc_committee_indices.size": "512",  # PTC_SIZE
    "ptc.size": "64",  # Gloas: Payload Timeliness Committee aggregation bits (PTC_SIZE = 512)
    "ptc.type": "github.com/OffchainLabs/go-bitfield.Bitvector512",
    "payload_attestation.size": "4",  # Gloas: MAX_PAYLOAD_ATTESTATIONS defined in block body
}

minimal = {
    "block_roots.size": "64,32",
    "state_roots.size": "64,32",
    "sync_committee_bits.size": "32",
    "sync_committee_bytes.size": "4",
    "sync_committee_bits.type": "github.com/OffchainLabs/go-bitfield.Bitvector32",
    "sync_committee_aggregate_bytes.size": "1",
    "sync_committee_aggregate_bits.type": "github.com/OffchainLabs/go-bitfield.Bitvector8",
    "withdrawal.size": "4",
    "blob.size": "131072",
    "logs_bloom.size": "256",
    "extra_data.size": "32",
    "max_blobs_per_block.size": "6",
    "max_blob_commitments.size": "4096",
    "max_cell_proofs_length.size": "33554432",  # FIELD_ELEMENTS_PER_EXT_BLOB * MAX_BLOB_COMMITMENTS_PER_BLOCK
    "kzg_commitment_inclusion_proof_depth.size": "17",
    "max_withdrawal_requests_per_payload.size": "16",
    "max_deposit_requests_per_payload.size": "8192",
    "max_builder_deposit_requests_per_payload.size": "256",  # MAX_BUILDER_DEPOSIT_REQUESTS_PER_PAYLOAD (2**8)
    "max_builder_exit_requests_per_payload.size": "16",  # MAX_BUILDER_EXIT_REQUESTS_PER_PAYLOAD (2**4)
    "max_attesting_indices.size": "8192",
    "max_committees_per_slot.size": "4",
    "committee_bits.size": "1",
    "committee_bits.type": "github.com/OffchainLabs/go-bitfield.Bitvector4",
    "max_consolidation_requests_per_payload.size": "2",
    "field_elements_per_cell.size": "64",
    "field_elements_per_ext_blob.size": "8192",
    "bytes_per_cell.size": "2048",  # FIELD_ELEMENTS_PER_CELL * BYTES_PER_FIELD_ELEMENT
    "cells_per_blob.size": "128",
    "kzg_commitments_inclusion_proof_depth.size": "4",
    "ptc_committee_indices.size": "16",  # PTC_SIZE
    "ptc.size": "2",  # Gloas: Payload Timeliness Committee aggregation bits (PTC_SIZE = 16)
    "ptc.type": "github.com/OffchainLabs/go-bitfield.Bitvector16",
    "payload_attestation.size": "4",  # Gloas: MAX_PAYLOAD_ATTESTATIONS defined in block body
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
