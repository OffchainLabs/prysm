package beacon_api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/OffchainLabs/prysm/v7/api/rest"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
)

const (
	blockFreshnessBudget = 500 * time.Millisecond // Max time to wait for a node serving a block built on the announced head.
	readFreshnessBudget  = 500 * time.Millisecond // Floor for a read's deadline, so a lagging node still gets time to import the announced head.
)

type headExtractor struct {
	extract func(raw json.RawMessage) (root [32]byte, ok bool)
	poll    bool
}

var (
	attestationRootExtractor = headExtractor{extract: rootExtractor("beacon_block_root"), poll: true}
	syncBlockRootExtractor   = headExtractor{extract: rootExtractor("root"), poll: false}
)

// readFreshnessOptions builds the read options that steer a JSON read toward a node
// that already imported the head announced on ctx, or nil if ctx has no hint. It
// uses:
//   - WithRace: query every node concurrently.
//   - WithAccept: among those responses, prefer the one whose extracted root
//     matches the announced head.
//   - WithDeadline: bound the read by the hint deadline (floored by
//     readFreshnessBudget so a lagging node still gets time to catch up).
//   - WithRepoll: for poll extractors only, keep re-polling until a node reports
//     the head or the deadline fires.
func readFreshnessOptions(ctx context.Context, extractor headExtractor) []rest.GetOption {
	hint, ok := freshnessHint(ctx)
	if !ok {
		return nil
	}

	accept := func(raw json.RawMessage) bool {
		wantRoot, _, known := hint.Head()
		if !known {
			// No head expectation yet: we cannot do better than first-success.
			return true
		}

		gotRoot, ok := extractor.extract(raw)
		return ok && gotRoot == wantRoot
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

	// Manage repolling.
	opts = append(opts, rest.WithDeadline(deadline))
	if extractor.poll {
		// Zero interval uses the default poll backoff.
		opts = append(opts, rest.WithRepoll(0))
	}

	return opts
}

// blockFreshnessOptions builds the read options that steer an SSZ block read
// toward a node whose block builds on the head announced on ctx, or nil if ctx
// has no hint. It uses:
//   - WithRace: query every node concurrently.
//   - WithSSZAccept: among those responses, prefer the block whose parent root
//     matches the announced head (decoded via decode).
//   - WithDeadline: bound the read by blockFreshnessBudget, tightened to the hint
//     deadline when that is sooner.
func blockFreshnessOptions(ctx context.Context, decode func([]byte, http.Header) (*ethpb.GenericBeaconBlock, error)) []rest.GetOption {
	hint, ok := freshnessHint(ctx)
	if !ok {
		return nil
	}

	accept := func(body []byte, hdr http.Header) bool {
		wantRoot, _, known := hint.Head()
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

		return wrapped.ParentRoot() == wantRoot
	}

	deadline := time.Now().Add(blockFreshnessBudget)
	if !hint.Deadline.IsZero() && hint.Deadline.Before(deadline) {
		deadline = hint.Deadline
	}

	// Race the nodes to select the one whose block builds on the announced head.
	return []rest.GetOption{rest.WithRace(), rest.WithSSZAccept(accept), rest.WithDeadline(deadline)}
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
