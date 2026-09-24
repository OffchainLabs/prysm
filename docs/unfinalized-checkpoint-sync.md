# Syncing from an unfinalized checkpoint

Checkpoint sync normally starts from the network's last finalized checkpoint.
During a long period without finality that checkpoint can be many epochs old, so
a fresh node would have to replay a large amount of chain before it is useful.
Prysm can instead be started from a recent, *unfinalized* anchor.

This is a manual procedure. `--checkpoint-sync-url` always downloads the
finalized checkpoint; to use an unfinalized one you fetch the state and block
yourself and pass them with `--checkpoint-state` and `--checkpoint-block`.

## Read this first

The anchor you supply is **trusted absolutely**. It becomes the fork choice tree
root and the database's lowest state, and Prysm reports it to the execution
client as final. If the block you pick is later reorged out, the node cannot
recover: every block on the real chain will be rejected as not descending from
the origin. The only remedy is to delete the database and sync again.

Always cross-check the block root against a second, independent source before
using it.

## Procedure

Pick an anchor epoch `E`. Two constraints apply:

- `E` must be at most `current_epoch - 2`. Prysm advertises `E` as its finalized
  epoch, and peers reject a status message claiming finality more recent than
  that.
- The state should sit on an epoch boundary slot, i.e. `E * SLOTS_PER_EPOCH`.
  Prysm accepts an unaligned state but logs a warning.

Download the state at the start slot of `E`:

```sh
SLOT=$((E * 32))
curl -H 'Accept: application/octet-stream' \
  "$BN/eth/v2/debug/beacon/states/$SLOT" -o state.ssz
```

Read `latest_block_header` from that state and compute its hash tree root, filling
in `state_root` only if it is zero. That root identifies the anchor block. Fetch
the block **by root**, never by slot — below finality several blocks can exist at
the same slot and the server may not return the one the state was built on:

```sh
curl -H 'Accept: application/octet-stream' \
  "$BN/eth/v2/beacon/blocks/0x<root>" -o block.ssz
```

Start the node:

```sh
beacon-chain --checkpoint-state=state.ssz --checkpoint-block=block.ssz ...
```

Prysm verifies that the block matches the state's `latest_block_header` and
refuses to start if it does not. When the state's own finalized checkpoint is
older than `E` it logs a warning that the anchor is unfinalized.

## What the node does with the anchor

The origin is recorded as finalized at epoch `E`, which is what stops fork choice
from ever reorging below it. The justified checkpoint keeps the anchor state's
*real* justified epoch, because fork choice compares that epoch against the
voting source of every block it imports; synthesizing it forward to `E` would
make every block non-viable and strand the head at the anchor for as long as the
chain stays unjustified. Both checkpoints point at the origin block root, since
no earlier block or state is in the database until backfill runs.

As a result `justified_epoch < finalized_epoch` until the network justifies epoch
`E` or later. This is expected and self-corrects once finality resumes.

## If the anchor is orphaned

The node logs an error naming the origin root once enough peers report finality
on a chain that does not contain it, exports
`sync_origin_orphaned_suspected 1`, and reports an unhealthy status. Delete the
database directory and start again from a new checkpoint.
