package beacon_api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/rest"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
)

const (
	readFreshnessBudget = 500 * time.Millisecond // Floor for a read's deadline.
	blockPublishMargin  = 250 * time.Millisecond // Time reserved to sign, publish and gossip a block in hand before the committee votes if no accepted block is returned by the deadline.
)

var (
	attestationRootExtractor = rootExtractor("beacon_block_root")
	syncBlockRootExtractor   = rootExtractor("root")

	attestationMatcher   = attestationMatches
	syncCommitteeMatcher = rootMatcher(syncBlockRootExtractor)
)

// attestationFreshnessOptions builds the read options that steer an attestation
// data read toward a node that already imported the head announced on ctx, or nil
// if ctx has no hint. See readFreshnessOptions.
func attestationFreshnessOptions(ctx context.Context) []rest.GetOption {
	return readFreshnessOptions(ctx, attestationMatcher)
}

// syncCommitteeFreshnessOptions builds the read options that steer a sync
// committee read toward a node that already imported the head announced on ctx, or
// nil if ctx has no hint. See readFreshnessOptions.
func syncCommitteeFreshnessOptions(ctx context.Context) []rest.GetOption {
	return readFreshnessOptions(ctx, syncCommitteeMatcher)
}

// rootMatcher builds a matcher accepting a response whose extracted root is the
// announced head.
func rootMatcher(extract func(json.RawMessage) ([32]byte, bool)) func(json.RawMessage, iface.Head) bool {
	return func(raw json.RawMessage, want iface.Head) bool {
		got, ok := extract(raw)
		return ok && got == want.Root
	}
}

// attestationMatches accepts an attestation data response reporting the
// announced head.
func attestationMatches(raw json.RawMessage, want iface.Head) bool {
	if !rootMatcher(attestationRootExtractor)(raw, want) {
		return false
	}

	wantIndex, known := attestationIndexFor(want)
	if !known {
		// No payload status criterion applies to the head: the root alone is the criterion.
		return true
	}

	gotIndex, ok := attestationIndexExtractor(raw)

	return ok && gotIndex == wantIndex
}

func attestationIndexFor(head iface.Head) (uint64, bool) {
	if slots.ToEpoch(head.Slot) < params.BeaconConfig().GloasForkEpoch {
		return 0, false
	}

	switch head.PayloadStatus {
	case api.PayloadStatusFull:
		return 1, true
	case api.PayloadStatusEmpty:
		return 0, true
	default:
		return 0, false
	}
}

// readFreshnessOptions builds the read options that steer a JSON read toward a node
// that already imported the head announced on ctx, or nil if ctx has no hint. It
// uses:
//   - WithRace: query every node concurrently.
//   - WithAccept: among those responses, prefer the one matches reports as the
//     announced head.
//   - WithDeadline: bound the read by the hint deadline (floored by
//     readFreshnessBudget so a lagging node still gets time to catch up).
//   - WithRepoll (UntilAccepted): keep re-polling every node until one
//     reports the announced head or the deadline fires.
func readFreshnessOptions(ctx context.Context, matches func(json.RawMessage, iface.Head) bool) []rest.GetOption {
	hint, ok := freshnessHint(ctx)
	if !ok {
		return nil
	}

	accept := func(raw json.RawMessage) bool {
		want, known := hint.Head()
		if !known {
			// No head expectation yet: we cannot do better than first-success.
			return true
		}

		return matches(raw, want)
	}

	// Race the nodes to select the one that already imported the announced head.
	opts := []rest.GetOption{rest.WithRace(), rest.WithAccept(accept)}

	// Manage deadline.
	if hint.Deadline.IsZero() {
		return opts
	}

	deadline := hint.Deadline
	if floor := time.Now().Add(readFreshnessBudget); deadline.Before(floor) {
		deadline = floor
	}

	// Keep re-polling every node until one reports the announced head or the
	// deadline fires.
	opts = append(opts, rest.WithDeadline(deadline), rest.WithRepoll(rest.UntilAccepted))

	return opts
}

// blockFreshnessOptions builds the read options that steer an SSZ block read
// toward a node whose block builds on the head announced on ctx, or nil if ctx
// has no hint. It uses:
//   - WithRace: query every node concurrently.
//   - WithSSZAccept: among those responses, prefer the block whose parent root
//     matches the announced head (decoded via decode).
//   - WithRepoll(UntilAny2xx): keep re-polling until at least one node returns a
//     block (any 2xx), then use it.
//   - WithDeadline: bound the read by the caller's context deadline.
//   - WithFallbackDeadline: stop waiting on the nodes that have not answered at
//     the hint deadline.
func blockFreshnessOptions(ctx context.Context, decode func([]byte, http.Header) (*ethpb.GenericBeaconBlock, error)) []rest.GetOption {
	hint, ok := freshnessHint(ctx)
	if !ok {
		return nil
	}

	accept := func(body []byte, hdr http.Header) bool {
		want, known := hint.Head()
		if !known {
			// No head expectation yet: we cannot do better than first-success.
			return true
		}

		block, err := decode(body, hdr)
		if err != nil {
			return false
		}

		wrapped, err := blocks.NewBeaconBlock(block.Block)
		if err != nil {
			return false
		}

		return wrapped.ParentRoot() == want.Root
	}

	// Race the nodes to select the one whose block builds on the announced head,
	// re-polling until at least one node returns a block.
	opts := []rest.GetOption{
		rest.WithRace(),
		rest.WithSSZAccept(accept),
		rest.WithRepoll(rest.UntilAny2xx),
	}

	// Bound the read (and the re-polling) by the caller's slot deadline.
	if deadline, ok := ctx.Deadline(); ok {
		opts = append(opts, rest.WithDeadline(deadline))
	}

	// Once a node has returned a block, only wait on the others up to
	// blockPublishMargin before the hint deadline.
	if !hint.Deadline.IsZero() {
		opts = append(opts, rest.WithFallbackDeadline(hint.Deadline.Add(-blockPublishMargin)))
	}

	return opts
}

// payloadAttestationFreshnessOptions builds the read options that steer an SSZ
// payload attestation data read toward a node that already imported the head
// announced on ctx, or nil if ctx has no hint. It uses:
//   - WithRace: query every node concurrently.
//   - WithSSZAccept: among those responses, prefer the one whose beacon_block_root
//     matches the announced head (decoded via payloadAttestationBeaconBlockRoot).
//   - WithDeadline: bound the read by the hint deadline (floored by
//     readFreshnessBudget so a lagging node still gets time to catch up).
//   - WithRepoll: keep re-polling until a node reports the head or the deadline
//     fires.
func payloadAttestationFreshnessOptions(ctx context.Context) []rest.GetOption {
	hint, ok := freshnessHint(ctx)
	if !ok {
		return nil
	}

	accept := func(body []byte, hdr http.Header) bool {
		want, known := hint.Head()
		if !known {
			// No head expectation yet: we cannot do better than first-success.
			return true
		}

		gotRoot, ok := payloadAttestationBeaconBlockRoot(body, hdr)
		return ok && gotRoot == want.Root
	}

	// Race the nodes to select the one that already imported the announced head.
	opts := []rest.GetOption{rest.WithRace(), rest.WithSSZAccept(accept)}

	if hint.Deadline.IsZero() {
		return opts
	}

	deadline := hint.Deadline
	if floor := time.Now().Add(readFreshnessBudget); deadline.Before(floor) {
		deadline = floor
	}

	// Keep re-polling until a node reports the head or the deadline fires.
	opts = append(opts, rest.WithDeadline(deadline), rest.WithRepoll(rest.UntilAccepted))
	return opts
}

// payloadAttestationBeaconBlockRoot extracts the beacon_block_root from a payload
// attestation data response, which GetSSZ may return as SSZ or JSON.
func payloadAttestationBeaconBlockRoot(body []byte, hdr http.Header) ([32]byte, bool) {
	if strings.Contains(hdr.Get("Content-Type"), api.OctetStreamMediaType) {
		d := &ethpb.PayloadAttestationData{}
		if err := d.UnmarshalSSZ(body); err != nil {
			return [32]byte{}, false
		}

		return bytesutil.ToBytes32(d.BeaconBlockRoot), true
	}

	return rootExtractor("beacon_block_root")(json.RawMessage(body))
}

// freshnessHint returns the freshness hint on ctx, if one usable for head
// matching is present.
func freshnessHint(ctx context.Context) (iface.Hint, bool) {
	hint, ok := iface.FromContext(ctx)
	if !ok || hint.Head == nil {
		return iface.Hint{}, false
	}

	return hint, true
}

// rootExtractor returns an extractor that reads a 32-byte hex root from
// data.<field> of a JSON response.
func rootExtractor(field string) func(json.RawMessage) ([32]byte, bool) {
	return func(raw json.RawMessage) ([32]byte, bool) {
		var body struct {
			Data map[string]json.RawMessage `json:"data"`
		}

		if err := json.Unmarshal(raw, &body); err != nil {
			return [32]byte{}, false
		}

		var hexRoot string
		if err := json.Unmarshal(body.Data[field], &hexRoot); err != nil || hexRoot == "" {
			return [32]byte{}, false
		}

		root, err := bytesutil.DecodeHex32(hexRoot)
		if err != nil {
			return [32]byte{}, false
		}

		return root, true
	}
}

// attestationIndexExtractor reads data.index from an attestation data response.
func attestationIndexExtractor(raw json.RawMessage) (uint64, bool) {
	var body struct {
		Data struct {
			Index string `json:"index"`
		} `json:"data"`
	}

	if err := json.Unmarshal(raw, &body); err != nil {
		return 0, false
	}

	index, err := strconv.ParseUint(body.Data.Index, 10, 64)
	if err != nil {
		return 0, false
	}

	return index, true
}
