# DoS protection for bundled attestation propagation

Design notes for hardening the partial-attestation broadcaster against fake
attestation data spam. Recommendations here will eventually move into the EIP /
specs; for now this file is the working record. Companion to plan.md's
"Validation is the only admission bound", "Blast radius of invalid signatures"
(the junk-*signature* axis), and "Fetch gating".

## Threat model

A connected peer fabricates `AttestationData` variants: every field tweak is a
fresh dataRoot, so — unlike junk signatures over real data — the key space is
unbounded and nothing about sending fake data is slashable. The attacker's
goals, in rough order of danger:

1. **Amplify cheap messages into expensive chain work** — worst case, a state
   regeneration from disk per message.
2. **Starve honest attestations** by occupying the validation lane. Extra bad
   here because mesh pushes are never replayed: until the request path exists,
   a dropped honest bundle is healed only by the classic-gossip bridge.
3. **Grow unbounded memory** — seen-tuple cache entries live two epochs.
4. **Burn the shared gossipsub event loop** with decode + hash work (it serves
   every topic, not just attestations).

## Cost ledger (what a fake-data bundle costs us per stage, current code)

- **GS event loop** (`OnIncomingRPC`): SSZ decode + one HTR + a capped
  peer-state write (64 sub-infos x 256B per peer). This is the floor cost,
  roughly symmetric with the attacker's own bandwidth. Acceptable, cheaper
  still after the planned HTR -> flat-sha256 swap for internal keys.
- **Start loop** (`handleIncoming`): up to 2048 tuple hashes, then up to 2048
  seen-cache entries held for two epochs once the job queues. Attacker-
  controlled memory: one ~200KB garbage bundle buys ~80KB of cache for ~13
  minutes, repeatable.
- **Validation lane** (`partialAttestationCommittee`): the real hole. We check
  `hasBlockAndState` and then go straight to `AttestationTargetState`. The
  classic validator runs `VerifyLmdFfgConsistency` (cheap, in-memory
  forkchoice) in between (`validate_beacon_attestation.go:157` before `:163`);
  we skip it. Fake data naming a *real* block root with a crafted target
  checkpoint therefore reaches stategen, which can regenerate a checkpoint
  state from disk — seconds of work per message, on a lane that runs jobs one
  at a time.
- **No penalty anywhere**: fake data is structurally valid, so
  `OnIncomingRPC` returns nil, and `PeerFeedbackFn` is currently discarded in
  `InitPubSub`. Spam is free to send.

## Design: four rules, in data-flow order

### Rule 1 — pin the data to the chain before touching state

Mirror the classic validator's check order in `partialAttestationCommittee`:

1. `hasBlockAndState` — unknown block -> Ignore, free (already in place; no
   pending queue, the classic path covers those).
2. **`VerifyLmdFfgConsistency`** — target/source must be ancestors of the
   named block per forkchoice. Pure in-memory map walk; Reject on mismatch.
   This is the missing check, and it is ~4 lines: the function takes an
   `ethpb.Att` and we already build the pseudo `SingleAttestation`.
3. Only then `AttestationTargetState`. Anything that survives step 2 names
   the canonical chain, where the checkpoint-state cache hits.

Result: fake data's cost ceiling drops to a few forkchoice lookups. The only
remaining state-touching path is real chain data, and the source-checkpoint
comparison against a cached target state is a trivial equality check.

### Rule 2 — cache the data-level verdict per dataRoot, not per tuple

One garbage `AttestationData` currently costs up to 2048 seen-tuple entries;
it should cost exactly one. Keep a small
`(topic, committeeIndex, dataRoot) -> verdict` cache on the Start loop:

- **Accept** caches the committee (the data group already does this).
- **Reject** caches the rejection: replays and same-data re-spam die at a map
  hit, with no validation-lane dispatch.
- **Ignore** (unknown block, syncing, overload) caches *nothing* — it must
  stay retryable. See "Seen-cache semantics" below.

Tuples enter the seen cache **only for data-accepted groups**. Seen-cache
growth is then bounded by validated data groups (real, slashable-to-vary
data), not by attacker bandwidth. The verdict cache itself is capped with
plain eviction: an evicted entry only means re-running the rule-1 check,
which is cheap. This cache is also exactly the admission gate the fetch path
(plan.md "Fetch gating") needs — build once, share.

**Seen-cache semantics.** Verdicts split three ways and only two burn the
tuple: *Reject* (permanent — the data or signature is provably bad) and
*Accept* (the validated bitmap covers it forever) may persist; *Ignore*
means "no verdict was reached" and must leave the tuple retryable. Today a
queued-then-Ignored tuple stays seen for two epochs, so an attestation whose
block arrives moments later can never validate via the partial path — the
classic bridge's pending-attestation queue papers over this.

**Ignored attestations must be held, not just retryable.** The network does
not re-offer a dropped undecidable for long: the sender pushes once and
folds the pushed positions into our recorded availability (never replays),
mesh duplicates only help if one arrives after the block does, and metadata
advertisement/request serving stop once the serving peers' signatures expire
(~2s) and their groups tombstone. An attestation that beats its block by
more than a few seconds is therefore recoverable only from a local hold —
exactly why classic gossip has the pending-attestation queue (which also
drives block-by-root discovery). A partial-only deployment needs the same
primitive: a small pending pool for unknown-root data, short-TTL,
never penalized, never triggering unbounded fetches. Junk with unknown
roots is never *detected*, it is *outlived*.

**The pending pool is itself the harm-1 surface, so it must be a hard
per-peer allowance — not a shared buffer.** Holding the undecidable only
relocates slot theft: the pool is finite, so a shared buffer with global
eviction lets an attacker fill it and evict honest racers. Dropping an
honest racer is *not* cheap — a lost unaggregated attestation is a missed
vote (the attester's reward and its participation weight, gone; "aggregators
hold it too" only if some other path delivered it, which the racing window
does not guarantee). So the design goal is to never evict an honest racer
at all, achieved structurally:

- *Fixed per-peer slots: 2-3 unknown attestation_data per peer, no
  cross-peer eviction.* The slot unit is a distinct unknown
  attestation_data, not a signature — a bundle carrying many positions under
  one unknown data occupies one slot, so a peer's entire pending footprint
  is at most 2-3 data entries however many signatures it packs. Each peer's
  own oldest entry is evicted only when it exceeds its own allowance. An
  attacker on identity P can only ever occupy P's slots and cannot touch any
  other peer's entries, so a spammer never causes an honest racer to be
  dropped. This turns spam (one identity, unbounded volume) into Sybil (many
  identities), governed by connection admission — the same endgame as every
  other branch here. Total pool is peers x 2-3, a trivial memory bound; a
  loose global cap is only a backstop.
- *Why 2-3 suffices.* A peer holds an undecidable only while it has seen a
  block we have not — a brief, narrow condition — so an honest peer's
  working set of racing data is one or two at a time. 2-3 covers it with
  margin; short TTL recycles the slots as races resolve.

### Rule 3 — split the validation lane by whether the committee is known

Jobs for an existing data group (committee cached) skip `GroupCommittee`
entirely and are BLS-only — that is the honest steady state. Jobs needing
data-level resolution go to a separate, small, per-peer-bounded queue.

Honest peers introduce ~1-2 new data groups per slot per subnet, so a
per-peer bound of a handful of in-flight new-data jobs costs them nothing,
while a spammer can no longer wedge the lane that honest bundles ride on.
This also fixes a pre-existing head-of-line problem: one slow
`GroupCommittee` (even an honest one hitting disk) currently stalls all BLS
work behind it.

Open decision: whether the full lane split is worth it now, or whether a
per-peer bound on the single lane suffices for the reference implementation
(rule 1 makes the slow path rare; rule 4 makes sustaining it expensive).

### Rule 4 — score data-level Rejects

Rule 1 turns crafted-target spam into a clean Reject with clean attribution:
honest peers forward only validated positions out of their pristine store, so
a data-level Reject always implicates the direct sender. Wire PeerFeedback so
Reject -> score -> disconnect; sustained spam then degenerates to Sybil churn,
governed by inbound connection limits (same endgame as the junk-signature
design in plan.md).

**Penalty taxonomy** (attribution differs by stage):

- *Structural* (GS event loop): bad SSZ, slot mismatch, duplicate positions —
  `OnIncomingRPC` already returns errors; confirm the mux/library feeds them
  into PeerFeedback rather than dropping them.
- *Data-level Reject* (validation lane): incomplete data, LMD/FFG
  inconsistency, topic/fork mismatch — one reject per bundle, sender is
  `valJob.from`. Needs the verdict threaded back through `valDone` (today a
  rejected job returns an empty `valDone{}` indistinguishable from Ignore).
- *Signature-level Reject*: abort the bundle on first reject (plan.md "Blast
  radius"); `processPartialAttestations` currently returns only a rejected
  *count*, so it must distinguish Reject from Ignore per position and abort
  instead of tallying.
- *Never penalized*: Ignore outcomes — unknown block, syncing, queue drops.
  Honest peers legitimately race block propagation.

Plumbing note: `PeerFeedbackFn` is discarded in `InitPubSub`; decide whether
it is safe to call from the validation goroutine directly or must route
through the Start loop.

## Fork resolution

**Fork length is irrelevant to attestation propagation.** Slots are
wall-clock, so both forks share a current slot, and the group-ID window admits
only current + previous epoch (`slotInPropagationWindow`). A fork that ran for
many epochs still produces at most ~2 epochs of *in-window* attestations;
everything older is rejected free at the group-ID check and arrives instead as
attestations embedded in the blocks we sync. So "really long-running fork"
collapses to "at most 2 epochs of attestation-side work." The length shows up
entirely in the block/state path (download the fork, replay state, import its
on-chain votes into forkchoice, reorg) — never this component's job.

**The catch-up burst fits the per-peer bound.** During resolution the
undecidables have a specific shape: *many peers referencing the same few
winning-fork heads*, not one peer referencing many data. Each honest peer
attests to one head (maybe two as the fork's last slots turn over), so its
distinct unknown attestation_data is 1-2 — inside the 2-3 allowance without
adjustment. Classification stays stable throughout: a checkpoint is
undecidable only when its block root is unknown; junk checkpoints still match
nothing in forkchoice and still Reject. As the fork's blocks import, its data
moves undecidable -> decidable -> validated (Ignore-retryability paying off),
and losing-fork attestations stay valid-but-non-canonical until finalization
prunes that branch (by which point they are out of window). Groups are keyed
by data root, independent of the canonical head, so a moving head never
invalidates stored state; stale losing-fork groups expire on TTL/window.

**Reference: the classic pending-attestation queue** (`pending_attestations_queue.go`)
is what heals unknown-block attestations today, and a fork is where it earns
its keep — but it is the shape we should *not* copy:

- Global cap `pendingAttsLimit = 32768`, peer-blind (the sender's ID is never
  recorded), and the save happens *before* signature/committee validation
  (both need the target state, which needs the missing block). So entries are
  cheap to manufacture: structurally-valid attestations naming random roots,
  no valid signature required.
- Unknown-root attestations drive block-by-root fetches: every processed
  block runs `processPendingAttsForBlock`, which re-requests every still-
  unknown root to `getBestPeers()` (honest high-score peers, not the sender).
  A garbage root never resolves, so it is re-requested ~once per slot for a
  full epoch until the staleness prune (`slot >= attSlot + SlotsPerEpoch`)
  drops it — ~32 fetch rounds per junk entry.
- Eviction piggybacks entirely on block arrival (the only `validatePendingAtts`
  call site), and the cap *refuses* rather than evicts: once full, new —
  possibly honest — attestations are silently dropped while garbage squats for
  ~6 minutes. During a fork, the one time the queue's head-discovery job
  matters, a pre-filled queue defeats it.

The useful half of this mechanism is real: "many validators vote for a root I
have never seen a block for -> go fetch it" recovers heads that block gossip
missed. So dropping attestation-driven fetch is a genuine tradeoff, not a free
win — block gossip + `Status`/range sync remain the primary fork-discovery
path, and giving up the attestation hint only forfeits a self-healing
optimization. The partial design improves on the rest for free: `from` is
already threaded through `valJob`/`valDone` (attributable), and the
2-3-unknown-data-per-peer allowance replaces the global peer-blind cap. Open
decision, same as rule 3's: whether the partial pending pool triggers fetches
at all, or waits for block sync — and if it does fetch, it must be per-peer
bounded and attributable, never the classic global amplifier.

## Non-issues (checked, no action)

- **Fake parts metadata**: only writes the sender's own capped claim state
  (64 sub-infos x 256B per peer). Self-polluting the cap just makes us send
  that peer more, bounded by our honest heartbeat traffic. Rule 2's verdict
  cache is what keeps metadata harmless once request-driven fetching exists.
- **Group-store occupancy**: data groups are stored only post-validation, so
  fake data never occupies the group store; varying valid data requires
  slashable equivocation (plan.md "Group lifecycle").
- **Network-partition heal**: attestation propagation has no reconciliation
  duty. Fork weight travels via on-chain attestations in the other side's
  blocks plus fresh post-heal votes (LMD latest-message supersedes; a vote on
  a new head weighs its whole ancestor branch). Nothing retro-attests
  (double-voting a target epoch is slashable), and partition-era attestations
  die out of gossip message caches in seconds (mcache: 6 x 700ms windows,
  IHAVE for 3). See "Fork resolution" for the catch-up-window handling.
