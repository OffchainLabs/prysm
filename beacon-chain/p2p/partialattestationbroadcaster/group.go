package partialattestationbroadcaster

import (
	"slices"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// All state in this file is owned by the Start loop; nothing here is safe
// for concurrent use.

const (
	// heartbeatInterval is the cadence of the Start loop's cleanup tick.
	heartbeatInterval = time.Second
	// pushInterval is the cadence of the publish tick; sending is tick-driven
	// only, so bursts of commits batch into one push per data group.
	pushInterval = 20 * time.Millisecond
	// sigTTLHeartbeats is how long a validated signature stays servable,
	// per attestation, sized to cover the advertise -> request -> serve trip.
	sigTTLHeartbeats = 6
	// maxIndicesPerClaim caps claim sets and wire index lists (= ssz_max).
	maxIndicesPerClaim = 2048
	// maxAttsPerBundle caps attestations per outgoing bundle (spec SHOULD);
	// a full bundle rolls over into a fresh one for the same key.
	maxAttsPerBundle        = 50
	maxGroupsPerTopic       = 128 // we need 1 per slot
	maxAttDataGroupsPerSlot = 2048
	// seenEntriesWarn is the seen-cache size above which cleanup warns;
	// honest all-subnet load holds ~2M entries.
	seenEntriesWarn = 5_000_000
)

// seenTTLHeartbeats mirrors gossipsub's seen-message TTL of two epochs (see
// setPubSubParameters).
func seenTTLHeartbeats() uint64 {
	cfg := params.BeaconConfig()
	return 2 * uint64(cfg.SlotsPerEpoch) * cfg.SecondsPerSlot
}

// validatedAtt is one validator's live validated signature this slot.
type validatedAtt struct {
	CommitteeIndex primitives.CommitteeIndex
	Signature      []byte
	// AttHash is the HTR of the attested data, a key into slotAtts.attData.
	AttHash string
}

// attDataEntry is one live AttestationData and the number of validated
// signatures referencing it; the entry dies with its last reference.
type attDataEntry struct {
	data *ethpb.AttestationData
	refs int
}

// sigExpiry schedules one validated signature's expiry. Entries append in
// commit order with a constant TTL, so remaining TTLs never decrease front
// to back and expiry only ever pops the front.
type sigExpiry struct {
	validatorIdx primitives.ValidatorIndex
	ttl          uint64
}

// slotAtts holds one slot's state on one subnet topic.
type slotAtts struct {
	// attData holds the live datas by string(HTR(AttestationData)).
	attData map[string]*attDataEntry
	// validated holds each validator's live signature this slot (one vote per
	// validator per slot); the entry is deleted when the signature expires.
	validated map[primitives.ValidatorIndex]validatedAtt
	// expiry drives signature expiry in commit order.
	expiry []sigExpiry
	// dirty is the freshly validated, not yet broadcast validators in commit
	// order; the push tick drains it into sent.
	dirty []primitives.ValidatorIndex
	// sent marks validators already broadcast: pushes are never replayed,
	// late mesh joiners catch up via requests, and once the validated entry
	// expires it is the remaining replay filter.
	sent map[primitives.ValidatorIndex]struct{}
}

// known reports whether the validator is already accounted for this slot:
// anything further from it is a replay or a slashable equivocation.
func (g *slotAtts) known(idx primitives.ValidatorIndex) bool {
	if _, ok := g.validated[idx]; ok {
		return true
	}
	_, ok := g.sent[idx]
	return ok
}

// ensureSlotGroup returns the slot group, creating it as needed; nil when the
// cap prevented it.
func (b *Broadcaster) ensureSlotGroup(topic string, slot primitives.Slot) *slotAtts {
	byTopic := b.groups[topic]
	if byTopic == nil {
		byTopic = make(map[primitives.Slot]*slotAtts)
		b.groups[topic] = byTopic
	}
	g := byTopic[slot]
	if g == nil {
		if len(byTopic) >= maxGroupsPerTopic {
			return nil
		}
		g = &slotAtts{
			attData:   make(map[string]*attDataEntry),
			validated: make(map[primitives.ValidatorIndex]validatedAtt),
			sent:      make(map[primitives.ValidatorIndex]struct{}),
		}
		byTopic[slot] = g
	}
	return g
}

// commitSig stores one validated signature and schedules its expiry; false
// when the data cap prevented tracking. The caller must have checked known.
func (g *slotAtts) commitSig(
	root string, data *ethpb.AttestationData, committee primitives.CommitteeIndex,
	idx primitives.ValidatorIndex, sig []byte, ttl uint64,
) bool {
	if g.known(idx) {
		return true
	}

	e := g.attData[root]
	if e == nil {
		if len(g.attData) >= maxAttDataGroupsPerSlot {
			return false
		}
		e = &attDataEntry{data: data}
		g.attData[root] = e
	}
	e.refs++
	g.validated[idx] = validatedAtt{CommitteeIndex: committee, Signature: sig, AttHash: root}
	g.expiry = append(g.expiry, sigExpiry{validatorIdx: idx, ttl: ttl})
	g.dirty = append(g.dirty, idx)
	return true
}

// takeDirty drains the dirty list straight into sent, returning it sorted.
// Taken entries are sent immediately: a lost push heals via requests.
func (g *slotAtts) takeDirty() []primitives.ValidatorIndex {
	indices := g.dirty
	g.dirty = nil
	for _, idx := range indices {
		g.sent[idx] = struct{}{}
	}
	slices.Sort(indices)
	return indices
}

// cleanup runs once per heartbeat: it expires signatures and drops slots
// outside the propagation window; a slot's sent set stays for replay
// filtering until the slot itself is dropped.
func (b *Broadcaster) cleanup(current primitives.Slot) {
	for topic, byTopic := range b.groups {
		for slot, g := range byTopic {
			if slots.ToEpoch(slot)+1 < slots.ToEpoch(current) {
				delete(byTopic, slot)
				continue
			}
			g.expireSigs()
		}
		if len(byTopic) == 0 {
			delete(b.groups, topic)
		}
	}
	for id, ttl := range b.seen {
		if ttl <= 1 {
			delete(b.seen, id)
			continue
		}
		b.seen[id] = ttl - 1
	}
	if len(b.seen) > seenEntriesWarn {
		log.WithField("entries", len(b.seen)).Warn("Seen-tuple cache far above the honest envelope")
	}
}

// expireSigs pops the expired front of the expiry queue, deleting each
// signature from validated and dropping its data with the last reference.
func (g *slotAtts) expireSigs() {
	for i := range g.expiry {
		g.expiry[i].ttl--
	}
	n := 0
	for n < len(g.expiry) && g.expiry[n].ttl == 0 {
		idx := g.expiry[n].validatorIdx
		v := g.validated[idx]
		delete(g.validated, idx)
		if e := g.attData[v.AttHash]; e != nil {
			e.refs--
			if e.refs == 0 {
				delete(g.attData, v.AttHash)
			}
		}
		n++
	}
	g.expiry = g.expiry[n:]
}

// sortedIndices returns the map's keys sorted ascending.
func sortedIndices[V any](set map[primitives.ValidatorIndex]V) []primitives.ValidatorIndex {
	indices := make([]primitives.ValidatorIndex, 0, len(set))
	for idx := range set {
		indices = append(indices, idx)
	}
	slices.Sort(indices)
	return indices
}

// capClaim truncates a sorted index list to the wire claim cap.
func capClaim(indices []primitives.ValidatorIndex) []primitives.ValidatorIndex {
	if len(indices) > maxIndicesPerClaim {
		return indices[:maxIndicesPerClaim]
	}
	return indices
}
