package coverage

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	dbiface "github.com/OffchainLabs/prysm/v7/beacon-chain/db/iface"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/proto/dbval"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// Database is the narrow database dependency of the coverage runtime. It is
// satisfied by the beacon node database (db/dbiface.Database).
type Database interface {
	EnvelopeCoverage(ctx context.Context) (*dbval.EnvelopeCoverage, error)
	CommitEnvelopeCoverage(ctx context.Context, cov *dbval.EnvelopeCoverage, replacements []dbiface.EnvelopeIndexReplacement) error
	RevealedEnvelopeRoots(ctx context.Context, start, end primitives.Slot, limit uint64) ([]dbiface.RevealedEnvelopeSlotRoot, error)
	PruneRevealedEnvelopeIndexBelow(ctx context.Context, floor primitives.Slot, limit int) (int, error)
	BlockSlotIndexPageDescending(ctx context.Context, cursor dbiface.SlotIndexCursor, floor primitives.Slot, maxCandidates, maxBytes int) (*dbiface.SlotIndexPage, error)
	BlockSlotIndexPageAscending(ctx context.Context, cursor dbiface.SlotIndexCursor, ceil primitives.Slot, maxCandidates, maxBytes int) (*dbiface.SlotIndexPage, error)
	Block(ctx context.Context, blockRoot [32]byte) (interfaces.ReadOnlySignedBeaconBlock, error)
	ExecutionPayloadEnvelopeWithFingerprint(ctx context.Context, blockRoot [32]byte) (*ethpb.SignedBlindedExecutionPayloadEnvelope, [32]byte, error)
}

// ChainView is the narrow canonical/head view late-bound onto the runtime
// once the blockchain service exists. It is satisfied by
// blockchain.Service.
type ChainView interface {
	IsCanonical(ctx context.Context, blockRoot [32]byte) (bool, error)
	HeadRoot(ctx context.Context) ([]byte, error)
}

// Snapshot is an immutable copy of the published coverage state.
type Snapshot struct {
	// Initialized reports whether a durable coverage record exists. Serving
	// must refuse when false.
	Initialized   bool
	FormatVersion uint32
	// [Low, High) is the covered half-open slot interval; Low == High is a
	// valid empty interval.
	Low  primitives.Slot
	High primitives.Slot
	// AnchorRoot is the canonical beacon block at exactly High providing
	// child testimony for the last covered slot.
	AnchorRoot [32]byte
}

// store owns the published coverage snapshot. The coordinator is the only
// writer; it holds the exclusive lock through the combined Bolt commit and
// snapshot publication so a coherent serve read can never pair an old bound
// with post-commit index contents.
type store struct {
	mu         sync.RWMutex
	db         Database
	cov        *dbval.EnvelopeCoverage // nil ⇒ UNINITIALIZED
	serveEpoch atomic.Uint64
}

func snapshotFromProto(cov *dbval.EnvelopeCoverage) Snapshot {
	if cov == nil {
		return Snapshot{}
	}
	return Snapshot{
		Initialized:   true,
		FormatVersion: cov.FormatVersion,
		Low:           primitives.Slot(cov.LowSlot),
		High:          primitives.Slot(cov.HighSlot),
		AnchorRoot:    bytesutil.ToBytes32(cov.HighAnchorRoot),
	}
}

// load reads and verifies the durable record, then publishes it. No snapshot
// is published before this returns: a restart refuses serving until the
// durable record has been loaded and verified.
func (s *store) load(ctx context.Context) error {
	cov, err := s.db.EnvelopeCoverage(ctx)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil // stay UNINITIALIZED
		}
		return err
	}
	if cov.FormatVersion != 1 {
		return errors.Errorf("unsupported envelope coverage format_version %d; delete and resync the beacon database", cov.FormatVersion)
	}
	if cov.HighSlot < cov.LowSlot || len(cov.HighAnchorRoot) != 32 {
		return errors.Errorf("corrupt envelope coverage record [%d, %d); delete and resync the beacon database", cov.LowSlot, cov.HighSlot)
	}
	s.mu.Lock()
	s.cov = cov
	s.mu.Unlock()
	return nil
}

// snapshot returns the current published snapshot.
func (s *store) snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return snapshotFromProto(s.cov)
}

// commit serializes one coverage mutation: it validates the new record,
// commits it with its exact index range replacements in one Bolt
// transaction, bumps the in-process serve epoch when the commit invalidates
// already-published contents, and publishes the new snapshot before
// releasing the writer lock. The serve epoch is deliberately not persisted.
func (s *store) commit(ctx context.Context, cov *dbval.EnvelopeCoverage, replacements []dbiface.EnvelopeIndexReplacement, destructive bool) error {
	if cov == nil {
		return errors.New("cannot commit nil coverage")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.db.CommitEnvelopeCoverage(ctx, cov, replacements); err != nil {
		return err
	}
	if destructive {
		s.serveEpoch.Add(1)
	}
	s.cov = proto.Clone(cov).(*dbval.EnvelopeCoverage)
	coverageLowSlot.Set(float64(cov.LowSlot))
	coverageHighSlot.Set(float64(cov.HighSlot))
	return nil
}

// clone returns a deep copy of the current durable record for mutation, or
// nil when uninitialized.
func (s *store) clone() *dbval.EnvelopeCoverage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cov == nil {
		return nil
	}
	return proto.Clone(s.cov).(*dbval.EnvelopeCoverage)
}
