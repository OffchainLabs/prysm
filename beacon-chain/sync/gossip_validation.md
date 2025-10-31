# Gossip validation

**Note:** This design doc currently details some topics of gossip validation. Additional topics about gossip validation will be added in the future. When the document is complete we will remove this note. 

## State usage in gossip validation

The beacon node needs to verify different objects that arrive via gossipsub: beacon blocks, attestations, aggregated attestations, sync committee messages, data column sidecars, slashings, etc. Each of these objects requires a different validation path. However, they all have in common that in order for them to be verified, one needs access to information from a beacon state. The question is *what beacon state should we use?*. 

### Beacon Blocks
Before we get into implementation details, let us analyze some explicit checks that we need to perform. Suppose this is the forkchoice state of the node

```
A <--- B <--- C <----------- E <-- .... <--- Y       (<--- head of the chain)
               \
                ----- D
``` 
Here the block `A` is finalized, the block `B` is justified and the head of the chain (from the point of view of this beacon node) is at `Y`. Suppose moreover that many slots and even epochs have happened between `D` and `Y`. The node now receives a block based on `D`. 
```
D <--- Z
```
How can we validate that the proposer index was indeed supposed to propose during this slot? Which state should we use to check what is the proposer index? If we take the post state of `Y`, which is this blocks current head state, and advance it to the slot of `Z`, the proposer index may be different than if you take the post state of `D` and advance it accordingly. Now we put ourselves in the shoes of the proposer of `Z`. This validator may have honestly not seen the chain `E <-- ... <--- Y` and instead kept `D` all the time as head, simply processing slots. Eventually she finds in the position of proposing a block. She needs to base it on `D`. Hence the phrasing on the p2p-spec:

```
- _[REJECT]_ The block is proposed by the expected `proposer_index` for the
  block's slot in the context of the current shuffling (defined by
  `parent_root`/`slot`). If the `proposer_index` cannot immediately be verified
  against the expected shuffling, the block MAY be queued for later processing
  while proposers for the block's branch are calculated -- in such a case _do
  not_ `REJECT`, instead `IGNORE` this message.
```

### Head state is often good enough

So when is the head state good enough to validate the proposer index in the above case? The situation is slightly different pre-Fulu than post-Fulu with the proposer lookahead, but essentially what we need to verify, is that there couldn't have been different shufflings when considering the post-state of `Y` and the post-state of `D` when advanced to the current slot.

Let `S = 32 E + k` be `Z`'s slot, with `0 <= k < 32` so that `E` is `Z`'s epoch. The proposer shuffling for `Z` was determined at slot `32 (E - 1)`. Let `X` be the latest ancestor of `Z` with slot less or equal to `32 (E - 1)`. If `X` is an ancestor of `C` or `C` itself, then the shuffling on the `Z` branch will be the same as on the `Y` branch for slot `S`. This for example is forced to happen if all `Z`, `Y` and `D` are in the same epoch `E`.

This takes care of the shuffling. However, the actual computation for the proposer index requires also the active validator indices, and this slice is determined at the latest epoch transition into `Z`'s epoch. 

So a good algorithm is as follows when importing `Z` at slot `S` and epoch `E`. 
1. Check if the head state is at epoch `E`
2. Check if the target checkpoint for `Y` in `E` equals the target checkpoint for `Z` at `E`. 
If both these points hold, then the head state already has the right proposer index. 
3. If either 1) or 2) does not hold, then the checkpoint state on the branch of `Z`, at `E` will hold the right proposer index for `Z`'s slot. Often times this state is faster to get than that of `D`, since being a checkpoint it will be cached in case that this checkpoint was canonical at some point. 

This takes care of most reorgs that happen on mainnet, and the only problem occurs when deep forks are attempted (usually by struggling nodes building on some old block). In these cases, often times the parent block is already finalized and therefore we don't even attempt to import those blocks. But this problem is exacerbated when the chain is not finalizing because any such struggling block will cause a fork and will fail the above checks to use the head state to consider the proposer index. 

### Attestations

Something similar happens for attestations. When receiving an attestation
```
AttestationData(
    slot,
    index,
    beacon_block_root,
    source,
    target=Checkpoint(
        epoch: E,
        root: R,
    )
)
``` 
We make sure that we know the block with root `beacon_block_root`. We also check that the target checkpoint is consistent. In particular, we know that the beacon state of `R` (possibly advanced) at the slot `32 E` is at the same epoch as `slot` and has the right beacon committees to check if the attester was supposed to attest at `slot` or not. Indeed the ingredients to compute the Beacon committee at the given slot are built out of the `randao_mixes` of the epoch `E - 2` (it's `E - MIN_SEED_LOOKAHEAD - 1`) and the active validator indices of the epoch `E`. Therefore any state that belongs to the same chain containing `R` and `beacon_block_root` and has epoch greater or equal than `E-2` will contain all the information necessary to validate the randao mix, and it needs to be exactly `E` to validate the active validator indices. We thus always take the checkpoint state, that is `R` advanced to `32 E`. 

### Head is good again

Now when is the head state good enough to validate an attestation as above? We already have the answer in the previous paragraph: the state needs to have the right active validator indices and the same randao mix. The mix is rarely a problem, this requires that the head state's checkpoint at `E-2` coincides with the `beacon_block_root` checkpoint at `E-2`. But the active validator indices are more likely to differ, the check here is very simple, if:
1. The head state's epoch is `E`. 
2. The head target at `E` has root `R`.

Then the head state is good to validate this attestation. If the above two conditions fail, then the right state to validate it is `R` advanced to `32 E`, which is likely to be cached if this state happened to be a checkpoint in a canonical chain. 

### Other verifications and caches

So we see we have two types of verifications: verifications related to randao mix, seeds and such to determine committees. These typically require a state from 1 or 2 epochs ago where the seed was fixed. And verifications related to active validator indices which require a state at the start of the current epoch (or the epoch of the object being validated). This applies to all verifications: proposer index, beacon committee attester index, sync committee index, PTC index, etc. 

Since computing active validator indices, proposer indices, beacon committees, etc. is very expensive, we keep several caches (more than what we actually need and some need to be removed from our codebase) for these. Since these are updated at epoch transition they are keyed by either the latest state root before the epoch transition or by the checkpoint root itself. 

In addition, forkchoice keeps an O(1) cache for each block, it gives the corresponding target checkpoint. So a general algorithm to perform verifications for arriving gossip elements is as follows:

1. Does the gossiped element belong to the head state or is it a descendant of the head state? If yes, use the head state.
2. If the parent of the gossiped element is not head, then is the target the same as the head's target for the current epoch? If yes, use the head state. 
3. If the targets differ, then get the gossiped element target state. This can be done as follows:
3.a. If the parent is in the same epoch, then use forkchoice to get the parent's target; this equals the gossiped element's target. Then use the checkpoint state cache to get the state.
3.b. If the parent is in a different epoch, then take the parent state and advance to the current epoch; that is the gossiped element target state.

### Dropping expensive computations

If the checkpoint cache misses (for example if the checkpoint was not really a checkpoint in our canonical chain ever), then regenerating the checkpoint state could be very expensive. In this case we should consider dropping or queueing the gossiped object. For attestations we have some heuristics for this to avoid validating old useless attestations. For beacon blocks this is not the case and we will try to always import a block that we receive over gossip. This is dangerous in case of non-finality as this can lead to very old regeneration of states. 
