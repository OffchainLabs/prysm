# Bundled attestation propagation — design and plan

Reimplementation of attestation broadcast over the gossipsub partial-messages
extension (EIP draft: `../EIPs/EIPS/eip-draft_attestation_propagation_redesign.md`),
behind `--partial-attestations`.

## Design decisions

**Group = slot.** The partial-messages group ID on `beacon_attestation_{subnet}`
is the slot number, little-endian uint64. Everything finer — committee,
attestation data, positions — is sub-information inside the group. The slot in
the group ID is checkable on arrival: groups outside the attestation
propagation window are ignored before any state is created.

**RPCs are batches of upserts, never snapshots.** A partial message carries
`AttestationBundle`s; parts metadata carries `CommitteeAttestationPartsMetadata`.
Every entry is self-keyed (committee_index + full attestation_data), so any
subset in any RPC is meaningful alone. Absence of an entry means "no update",
never "I have nothing" — that is what makes multiple RPCs per tick, chunked
flushes, and mid-tick eager pushes all correct. Per sub-info, `available` is a
full idempotent snapshot; losing an RPC only delays, the next one repairs.

**Validation is the only admission bound.** A valid attestation is never
rate-limited; what we bound is unvalidated junk:

- Group-ID slot window check at arrival — free rejection, no state.
- Expensive state (signatures, committee-sized bitmaps) is allocated only
  after validation.
- Reconciliation splits into two kinds of state. OUR forwardable state (the
  pristine store, tombstones) advances only on validated handoff — a drop
  anywhere (queue full, validation overload, cap) must be invisible to it, so
  it heals via the next exchange instead of becoming a permanent hole.
  PEER-knowledge state (per-peer `Available` claims) instead updates
  immediately on RPC receipt on the GS event loop, before any handoff: a claim
  only ever suppresses sends to the claimant, never makes us forward junk, so
  it is safe to trust a peer's word about itself even for a dropped RPC.
  Drop-invisibility still holds for our forwardable state.
- The pubsub library's peer-initiated group caps (default 8/peer/topic,
  255/topic) bound only groups we never republish, i.e. junk; they release
  when we publish for a group. With slot groups the counts are tiny; bump the
  per-peer limit to ~64 (the propagation window in slots) so late-reveal
  bursts across many old slots can't trip it.
- Junk (undecodable, wrong-slot entries, invalid signatures) is reported via
  gossipsub PeerFeedback so spammers score themselves out of the mesh.
- RPC count per tick is sender-controlled — no receive-side property depends
  on batching. Per-peer fairness comes from per-peer stream serialization;
  per-peer quotas on queued work are mostly subsumed by scoring (below) — a
  spammer occupies the queue at most once per identity.

**Concurrency: three lock-free lanes, no locks anywhere.**

- The gossipsub event loop owns the per-peer state (`peerStates`, the library's
  per-(topic, group) `PartialAttestationPeerState.Available` map). It runs
  `OnIncomingRPC`, `OnEmitGossip`, and every `PublishActionsFn` closure. Its
  callbacks never touch chain state, disk, or block on the Start loop.
- The Start loop owns the group store (`b.groups`, seen cache) — pure memory.
- The validation goroutine is the only lane allowed to touch chain state, disk,
  or chain locks (committee resolution, signature checks).
- `publishPartial` is the lock-free rendezvous: the library ships the closure to
  the pubsub event loop and BLOCKS the Start-loop caller until it has run, so
  the closure may read Start-loop-owned data by value while its single writer is
  parked. The one rule that keeps this deadlock-free is that GS-loop callbacks
  never wait on the Start loop.

**Validation pipeline sizing.** The cost unit is the signature, not the job:
a bundle carries up to 2048 sigs, so bounds counted in jobs bound neither
memory nor CPU. Honest load needs no per-peer accounting — the seen-tuple
cache is global and content-keyed, and an honest attestation has one
canonical signature, so N peers sending the same attestation collapse to one
validation (~31k unique sigs/slot network-wide, ~2.6k/s on all subnets).
Sizing rules:

- Queue depth = validation throughput × useful lifetime, and no bigger. An
  attestation is worth validating for a few seconds; anything queued behind
  more work than that comes out stale. Drops are cheap by design (retryable,
  reconciliation heals), so small queue + drop, counted in signatures
  (~1000, drains in well under a second). The current 20k-job buffer with a
  "~20MB" comment is wrong in both unit and conclusion.
- Parallelism = target throughput × 5 ms. All gossip validation funnels into
  the shared batch verifier, which collects waiters for 5 ms (cap 1000, flag
  `batch-verifier-limit`); in-flight partial-path sigs per window is the
  throughput knob. The reference implementation validates one signature at
  a time; production wants ~32 in flight → ~6.4k sigs/s.
- Seen-cache sizing: entries are a 32-byte hash + TTL. Like gossipsub's
  seen cache it is unbounded and purely time-based; honest all-subnet load
  holds ~2M entries (~100MB) over the two-epoch TTL (~2.6k tuples/s).
  Cleanup warns above 5M entries — that level means a sustained flood, and
  the flood already pays validation (BLS) for every fresh tuple it inserts.
  If memory ever needs a hard bound: evict a tuple once its position
  validates — the bitmap covers it from then on.

**Blast radius of invalid signatures.** One bad signature fails the shared
5 ms batch and every co-batched waiter — classic gossip included — falls
back to individual verification. A typical window holds only ~13 honest sigs,
so a single poisoned window is cheap; the threat is *sustained* poisoning
(~200 garbage sigs/s poisons every window). Attribution is clean: honest
partial peers forward only validated positions out of their pristine store,
so any invalid signature implicates the direct sender — there is no innocent
relay. Containment, in order:

- Abort a bundle on its first ValidationReject: any reject already proves
  the sender is bad. Jobs are one signature each, so this means dropping the
  bundle's still-queued jobs (needs `from` for attribution), cutting
  per-bundle damage from ~64 poisoned windows (2048 sigs / 32 in flight) to
  ~one. No dependency on scoring.
- Thread `from` through valJob/valDone so failures can be attributed and the
  sender skipped on push (the reference implementation dropped it: it is only
  needed at validation time, not after).
- PeerFeedback-reject on first failure → score → disconnect. The attacker
  then pays a fresh connection per poisoned window: the attack degenerates
  to Sybil churn, which inbound connection limits already govern.
- Scoring nuance: the seen-tuple cache means a replayed garbage tuple from a
  second peer never reaches validation, so only the first sender is scored.
  Correct behavior, but the decode/hash load of replayed junk stays on the
  Start loop regardless.

**Late-reveal cost accounting (accepted).** Late reveals cannot amplify
volume — a signature must be a valid attestation from a real validator, one
per epoch, so withholding only time-shifts the sender's own budget (and
forfeits their head+source rewards). Worst case (an adversary dripping one
withheld attestation per in-window slot): each reveal costs a cache-hit
committee re-resolution, one BLS check, ~2s re-hydration of its own data
group, and ~4.5KB egress per node — a 1-sig bundle to mesh peers plus one
change-driven metadata entry (full attestation data + committee bitmap,
~350B) to gossip peers. That metadata entry is ~5x classic gossip's 20-byte
IHAVE ID, ~1.6x classic all-in; the trade is accepted because on the honest
bulk path one metadata entry amortizes over a whole data group (~2.4KB vs
~180KB of per-message IHAVE for a 500-attestation committee). If the tail
ever matters: flush old-group bundles/metadata on a slow cadence (per
heartbeat or slower) so drip-fed reveals re-batch — lateness has already
forfeited all time-sensitive rewards, so only the current slot needs the
fast tick.

**Requests are identity-addressed.** Metadata carries no attestation data:
an advertisement is `(slot [the group ID], committee_index, available:
[validator indices])`, complete because a validator attests at most once per
slot. Requests echo the same shape. Serving reads only live signatures and
never stores: a request is answered immediately from live signatures or
forgotten, so a junk request costs one map miss, never pending state.
Fetching is likewise oneshot:
filtered by the slot's known (validated or sent) set, the request goes out
immediately with no requested state — with nothing to inspect pre-fetch,
there is no data gate and no validation-lane hop; the response is a regular
bundle that pays full validation, and duplicate responses from concurrent
advertisers die in the known/seen filters. The claim space is (slot,
committee, validator), so a junk claim buys the attacker one bounded
request per advertisement — unlike a classic IHAVE, which is an opaque message
ID fetched blind with no identity to bound it.

**Validator indices end to end (implemented).** Group ID stays the slot;
both containers keep their committee_index field. Committee positions and
bitmaps are gone — everything speaks global validator indices:

- Bundle: `committee_index + attestation_data + attester_indices
  (validator indices) + signatures`.
- Parts metadata: `committee_index + available (validator index list) +
  requests (validator index list)` — no attestation data.

Validator indices are the node's native identifier post-Electra
(SingleAttestation, the classic seen-cache key, pool, fork choice), so
classic-compat bridging is field copying — and the broadcaster stops
needing committees altogether: no committee cache on data groups, no
tombstone revival, no position/index boundary. Validated, sent, peer
claims, wants — all sets of validator indices; serve is set intersection,
fetch is set difference, and peer state keeps one representation folded
directly on the GS loop. The broadcaster performs no validation of its own:
a bundle is translated into SingleAttestations and replayed through the
classic gossip validator, which owns every check — data, chain consistency,
topic, membership, signature — including its pending-attestation queue for
unknown blocks. Submit commits inline with no validation-lane hop (its
callers already validated). This also enables verifying signatures FIRST
later: the registry maps index to pubkey without any target state, so junk
from non-validators dies at one BLS check and sig-valid junk is slashably
bounded to one data per validator per epoch (dos_protection rules 1/3
shrink accordingly).

Costs, accepted: an index is 8B on the wire (4B if uint32) versus 1 bit, so
a full ~500-member committee's advertisement grows ~64B -> ~4KB — still far
under classic IHAVE for the same committee. Data diversity does not inflate
it: validators partition across distinct datas, so one advertisement covers
the slot regardless of camps. Claim/request lists are attacker-sized where
bitmaps were self-capping, so per-committee set caps carry that bound.
uint16 indices are permanently off the table (the validator index space
needs 32 bits). The EIP draft is updated to this design.

**Reconsider HashTreeRoot for internal keys.** dataRoot is used only for
process-local keying (sub-info keys, seen-tuple IDs, peer availability keys)
— the wire carries the full attestation_data, never a root, so nothing
requires the canonical SSZ HTR. A flat sha256 over the serialized data pins
target/fork/shuffle just as well at a fraction of the hashing (one pass over
~128B instead of ~7-9 Merkleization compressions), and it runs on the GS
event loop for every incoming RPC. Swap when convenient; the only rule is
that every key derivation in the process uses the same function.

The current implementation is a reference implementation — the sizing above
is recorded for the production pass, not yet applied precisely.

## Production pass

Deferred hardening and cleanup, to be applied before this leaves prototype.
The validation items condense dos_protection.md (fake-data spam); see there
for the threat model and full rationale.

- ~~**Pin data to the chain before touching state** (dos_protection rule
  1).~~ Done by construction: the partial path replays every attestation
  through the classic gossip validator, whose check order already pins the
  block and target before resolving state — fake data naming a real block
  with a crafted target never reaches stategen.
- **Per-dataRoot verdict cache** (rule 2): cache
  (topic, committeeIndex, dataRoot) → Accept/Reject on the Start loop so one
  garbage AttestationData costs one entry, not up to 2048 seen-tuples.
  Tuples enter the seen cache only for data-accepted groups, bounding its
  growth by slashable-to-vary real data instead of attacker bandwidth.
  Ignore caches nothing — it must stay retryable. This cache doubles as the
  fetch path's admission gate ("Fetch gating").
- **Pending pool for unknown-root data** (rule 2): Ignored attestations must
  be held, not just retryable — mesh pushes are never replayed and serving
  peers' groups tombstone in seconds, so an attestation that beats its block
  by more than a few seconds is recoverable only from a local hold. Hard
  per-peer allowance of 2-3 distinct unknown attestation_data (a bundle under
  one unknown data occupies one slot), no cross-peer eviction, short TTL,
  never penalized — a spammer can never evict an honest racer, and spam
  degenerates to Sybil. Do NOT copy the classic pending queue's shape
  (global peer-blind cap, pre-validation saves, per-slot refetch of garbage
  roots). Open decision: whether the pool triggers block-by-root fetches at
  all or waits for block sync; if it fetches, per-peer bounded and
  attributable.
- **Validation-lane split** (rule 3): jobs for an existing data group
  (committee cached) are BLS-only; jobs needing `GroupCommittee` resolution
  go to a separate small per-peer-bounded queue so a spammer (or one slow
  honest disk hit) cannot wedge the lane honest bundles ride on. Open
  decision: full split vs a per-peer bound on the single lane (rule 1 makes
  the slow path rare).
- **Score data-level Rejects** (rule 4) plus the junk-signature containment
  from "Blast radius": wire `PeerFeedbackFn` (currently discarded in
  `InitPubSub`); thread the Reject/Ignore verdict through `valDone` (today a
  reject is indistinguishable from Ignore); abort a bundle on first
  signature Reject instead of tallying; confirm structural errors from
  `OnIncomingRPC` reach PeerFeedback. Never penalize Ignore.
- **Library cap bump**: per-peer group limit to ~64 (propagation window).
- **Sizing**: the queue is already per-signature (one job = one
  attestation); confirm the depth and parallelism against the numbers from
  "Validation pipeline sizing".
- **Keys**: swap HTR for a flat sha256 on internal keys ("Reconsider
  HashTreeRoot").

**Group lifecycle: hot → sent set → gone.** A validated signature is
kept ~6s from its validation (6 ticks of the Start loop's 1s cleanup
heartbeat, sized to cover the advertise → request → serve round trip), per
attestation; its validated entry expires with it, and an attestation data
whose last signature expired is deleted. What persists until the slot leaves
the current/previous-epoch validity window is the slot's sent set — the
validator indices already broadcast (all validated indices fold into it on
the next push tick, well before the ~6s expiry) — which, with the seen-tuple
cache, answers two questions: which advertised validators are worth
requesting, and which of a replayed bundle can skip BLS entirely. Requests
against expired slots are not served (spec: requests are not persistent).
Expiry is per attestation, not per slot — a late reveal must never refresh
the whole slot's state (griefing). The attestation pool remains the
authoritative store for block packing; the validated set is broadcaster-only
bookkeeping. An attacker must reveal a fresh valid signature (slashable to
vary per epoch) to occupy any storage: ~150 bytes for seconds, then ~8 bytes.

**Per-slot validator dedup is safe** post-validation: a validator attests at
most once per slot, so once one of its signatures passes BLS, any other
attestation from it this slot is a replay or a slashable equivocation — the
same shape as the classic seen-cache key (slot, committee, attester). The
EIP's MUST NOT on data-blind dedup is about deduping *before* validation;
the pre-validation seen-tuple still includes the data root and signature, so
a racing garbage signature cannot suppress the honest one.

## Implementation steps

1. ~~Mux plumbing: single partial-messages extension shared via partialmsgmux
   (union peer state, one column handler + one attestation handler),
   `--partial-attestations` flag, subnets join with RequestPartialMessages.~~
   (committed: "Plug the gossipsub partial-messages extension into attestation
   subnets")
2. **SSZ containers** matching the EIP: `AttestationBundle`,
   `CommitteeAttestationPartsMetadata`, as protobufs with sszgen codecs like
   the partial column containers (`partial_attestations.proto`; regenerate
   with `make gen proto ssz`). attester_indices are global validator
   indices as List[uint64] (uint16 is impossible for the validator index
   space; uint32 would suffice and is an EIP question).
   MAX_VALIDATORS_PER_COMMITTEE = 2048 in both presets.
3. ~~**Receive path**: OnIncomingRPC decodes group ID (slot) + bundle +
   metadata, structural checks only (length, slot match, propagation
   window), hands decoded values to the broadcaster channel.~~ (committed:
   "Add partial attestation wire containers and the receive path")
4. **Validation handoff**: the broadcaster's event loop resolves the
   committee via sync (`partialAttestationCommittee`: data-level classic
   checks incl. the topic fork digest, then the committee from the target
   state) and replays each attestation through the classic gossip validator
   as a synthetic pubsub message (`processPartialAttestations`), so pool,
   feeds, and seen-caches all behave exactly as for classic gossip; accepted
   ones are rebroadcast as SingleAttestation for non-partial peers. No
   PeerFeedback scoring yet. The committee resolution (`GroupCommittee`) runs
   on the validation goroutine, never on the Start loop: it can hit BoltDB and
   regenerate state from disk, so junk data now occupies a validation-queue
   slot (retryable, drops heal) instead of being rejected synchronously.
5. **Group store**: groups keyed topic → slot, sub-infos keyed
   (committee_index, HTR(data)) holding committee, per-position signatures,
   and the validated-position bitmap. Signatures live sigTTLHeartbeats (~2s)
   from validation, per position — a late reveal gets its own TTL and never
   refreshes earlier signatures — then the data group tombstones to bitmaps
   only, deleted when the slot leaves the propagation window.
   Deduplication is two-level, mirroring classic gossip: the
   pristine store (validated positions only — the sole forwardable state)
   drops any signature for an already-validated position (BLS is
   deterministic, it is necessarily invalid); everything earlier is one
   seen-tuple cache keyed hash(data root, committee, position, signature)
   with a TTL matching gossipsub's seen-message TTL (two epochs), counted
   down on each heartbeat — exact replays, junk
   retries, and in-flight duplicates all hit the cache, while a different
   signature for the same position is a different tuple and validates
   independently, so a racing garbage signature cannot suppress the honest
   one. Duplicate positions inside a bundle are a wire error. Tuples are
   marked seen only when their job is queued, and the store is written only
   post-validation, so drops stay invisible. Validation runs on a dedicated
   goroutine (jobs/done channels); the Start loop owns all state. The committee is
   cached on hot data groups. Data groups are constructed after the
   data-level checks pass (known block, topic match) but stored only once
   an attestation in them validates. Per-topic group and per-slot
   data-group caps as flood backstops, enforced at store time. Cleanup runs
   on the Start loop's 1s heartbeat ticker.
6. Publish path: on every push tick (20 ms), flush dirty sub-infos — only
   positions not yet broadcast (tracked by a group-level `sent` bitmap) are
   pushed via `publishPartial` to partial peers not already known to have them
   (diffed against their `Available`), the sent positions fold back into
   `Available`, and the pushed positions fold into `sent` so they are never
   re-offered. A peer that joins the mesh late therefore gets nothing
   retroactively — catching up is the metadata/request path's job (step 7).
   Sending is tick-driven only, never per-commit. Classic-path interop is
   done: `Broadcaster.Submit` is a pre-validated ingress that commits without
   ProcessAttestations, fed by two hooks — local origination
   (`BroadcastAttestation`) and classic gossip acceptance (the subnet
   validator's accept path). The rebroadcast each hook triggers re-enters
   Submit and is neutralized by the pristine store's validated-position drop.
   With this, `--partial-attestations` is viable end to end (locally produced
   and classic-accepted attestations now reach partial peers). Metadata
   emission still open.
7. Metadata/gossip path, library cap bump, PeerFeedback scoring for junk.
   Advertisement is oneshot gossip via OnEmitGossip: the library's gossip
   emissions (non-mesh peers) get the slot's per-committee Available once,
   deleted from the peer states and never tracked; there is no heartbeat
   advertisement, and late mesh joiners are covered by ongoing traffic.
   Request serving and fetching are done immediately, with no queued state,
   and never mix the two metadata sides: a peer's Available triggers a
   Requests-only fetch back (no available list of our own), a peer's
   Requests is answered from live signatures with bundles, diffed against
   the requester's recorded availability and folded back into it so a
   replayed request goes quiet; duplicate fetch responses die in the
   known/seen filters. Still open: scoring —
   abort a bundle on first reject, reject-by-peer on failed validation (see
   "Blast radius of invalid signatures"; `from` must be re-threaded through
   valJob/valDone first) — and the library cap bump.
