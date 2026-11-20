# Signing protection

This package is the signing protection database of the leanXMSS validator client.
It replaces the EIP-3076 slashing protection database after the PQ fork.

It refuses to sign two kinds of messages:

- **A message at an already used leaf.** leanXMSS is stateful: signing two
  different messages at the same leaf exposes the leaf and lets anyone forge
  signatures at it. This also prevents two slashing offenses: double proposals
  (two different blocks for the same slot, signed at the same leaf) and double
  votes (two different attestations for the same target epoch, signed at the
  same leaf).
- **A surround vote.** An attestation that surrounds, or is surrounded by, a
  previously signed attestation is slashable. The two attestations have
  different target epochs, so they are signed at different leaves, and leaf
  protection alone does not prevent it.

## Leaves

The PQ specification assigns a leaf to every message a validator signs:

| Leaf                                | Message                                 | Order                      |
| ----------------------------------- | --------------------------------------- | -------------------------- |
| `0`                                 | `ValidatorPreregistration`              | fixed by convention        |
| `[1, 2^15)`                         | `VoluntaryExit`                         | allocated by this database |
| `2^15 + target_epoch`               | `AttestationData`                       | target epoch               |
| `2^15 + 2^29 + 2^30 + slot * 8 + 0` | `BeaconBlock`                           | slot                       |
| `2^15 + 2^29 + 2^30 + slot * 8 + 1` | `AggregateAndProof`                     | slot                       |
| `2^15 + 2^29 + 2^30 + slot * 8 + 2` | `PayloadAttestationData`                | slot                       |
| `2^15 + 2^29 + 2^30 + slot * 8 + 3` | `ProposerPreferences`                   | proposal slot              |
| `2^15 + 2^29 + 2^30 + slot * 8 + 4` | `ExecutionPayloadEnvelope` (self-build) | slot of its beacon block   |

Offsets `0` to `4` are the spec's `LEAF_OFFSET_BEACON_PROPOSER`,
`LEAF_OFFSET_AGGREGATE_AND_PROOF`, `LEAF_OFFSET_PTC_ATTESTER`,
`LEAF_OFFSET_PROPOSER_PREFERENCES` and `LEAF_OFFSET_EXECUTION_PAYLOAD_ENVELOPE`.
Offsets `5` to `7` are unused.

## File format

Each XMSS public key has its own YAML file:

```text
<datadir>/validator-client-data/signing-protection/0x<xmss pubkey>.yaml
```

```yaml
genesisValidatorsRoot: 0x4b363db94e286120d76eb905340fdd4e54bfe9f06bf33ff6cf5ad27f511bfe95 # chain of the file
lastSignedReservedLeaf: 1 # reserved leaves
lastSignedAttestationSourceEpoch: 1200 # surround protection
lastSignedAttestationTargetEpoch: 1201 # surround protection + attestation leaves
lastSignedBlockSlot: 38450 # LEAF_OFFSET_BEACON_PROPOSER
lastSignedAggregateAndProofSlot: 38449 # LEAF_OFFSET_AGGREGATE_AND_PROOF
lastSignedPayloadAttestationSlot: 38449 # LEAF_OFFSET_PTC_ATTESTER
lastSignedProposerPreferencesSlot: 38470 # LEAF_OFFSET_PROPOSER_PREFERENCES
lastSignedExecutionPayloadEnvelopeSlot: 38450 # LEAF_OFFSET_EXECUTION_PAYLOAD_ENVELOPE
```

`genesisValidatorsRoot` is required. The store is created for one chain, and
refuses to read or write a file of another chain. Any other missing field means
nothing was signed in that class yet. A key without a file has signed nothing,
except reserved leaf 0.

## Rules

| Function                  | Refuses when                                                         | Records        |
| ------------------------- | -------------------------------------------------------------------- | -------------- |
| `CheckAndSaveAttestation` | source > target, source < recorded source, target <= recorded target | source, target |
| `CheckAndSaveSlotMessage` | slot <= recorded slot of the same message type                       | slot           |
| `NextReservedLeaf`        | all `2^15 - 1` usable reserved leaves are used                       | allocated leaf |

All refusals wrap `ErrSigningRefused` and leave the file unchanged.

The attestation rule is the EIP-3076 minimal rule. The strictly increasing
target epoch both prevents double votes and protects the attestation leaves;
the non-decreasing source epoch prevents surround votes.

Each slot-based message type has its own watermark. A shared one would not
work: proposer preferences are signed ahead of time for a slot of the next
epoch, and would then block the blocks and aggregates of the current epoch.

### Reserved leaves

Messages without a leaf of their own, such as voluntary exits, are signed at a
reserved leaf. `NextReservedLeaf` allocates the next one, records it, and
returns it; the caller must sign at exactly that leaf.

Leaf 0 is never allocated: by convention, it is the leaf of the key's
preregistration, which is signed before the validator exists, usually by
another tool. Renewing an expired preregistration resubmits the same signed
request, so it does not use a new leaf. This way, a key imported without its
signing protection file can never reuse its preregistration leaf.

Pre-signed voluntary exits each use a reserved leaf as well.

## Usage

The caller must check and record **before** signing, and must not sign if the
check fails:

```go
if err := store.CheckAndSaveSlotMessage(pubkey, signingprotection.Block, slot); err != nil {
	return err // Do not sign.
}
signature := key.Sign(leafIndex, signingRoot)
```

Recording before signing means a crash between the two loses a duty, but never
reuses a leaf. It also avoids spending a signature (about 70 ms) on a message
that would be refused.

## Safety properties

- **Atomic check and record.** Each public key has a mutex held across the
  read, the check and the write, so two concurrent calls for the same leaf
  cannot both succeed.
- **Durable writes.** A file is written to a temporary file, synced, renamed
  over the previous one, and its directory is synced. A crash leaves either the
  previous or the new content, never a partial one.
- **Fail closed.** An empty, unparsable file, a file with unknown fields, or a
  file without the genesis validators root of the store is an error, never an
  empty history. Signing is refused until it is fixed.
- **Only stricter.** `Merge` raises each watermark to the highest of the two
  values and never lowers one.

## Fork transition

Before the PQ fork, a validator signs with BLS and is protected by the EIP-3076
database. At the fork, its XMSS file should be seeded with `Merge`, using the
attestation source and target epochs of its BLS history. Slashing evidence
cannot cross the fork, so this is not strictly required, but it costs nothing.
