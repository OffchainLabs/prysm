package sync

import (
	"context"
	"fmt"
	"iter"
	"slices"
	"sync"
	"testing"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/partialdatacolumnbroadcaster"
	mockp2p "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

func TestAllDataColumnSubnets(t *testing.T) {
	t.Run("returns nil when no validators tracked", func(t *testing.T) {
		// Service with no tracked validators
		svc := &Service{
			ctx: t.Context(),
		}

		result := svc.allDataColumnSubnets(primitives.Slot(0))
		assert.Equal(t, true, len(result) == 0, "Expected nil or empty map when no validators are tracked")
	})

	t.Run("returns all subnets logic test", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		ctx := t.Context()

		beaconDB := dbtest.SetupDB(t)

		// Create and save genesis state
		genesisState, _ := util.DeterministicGenesisState(t, 64)
		require.NoError(t, beaconDB.SaveGenesisData(ctx, genesisState))

		// Create stategen and initialize with genesis state
		stateGen := stategen.New(beaconDB, doublylinkedtree.New())
		_, err := stateGen.Resume(ctx, genesisState)
		require.NoError(t, err)

		// At least one attached validator.
		svc := cache.NewSubscribedValidatorsCache()
		svc.Add(1)

		s := &Service{
			ctx:                       ctx,
			subscribedValidatorsCache: svc,
			cfg: &config{
				stateGen: stateGen,
				beaconDB: beaconDB,
			},
		}

		dataColumnSidecarSubnetCount := params.BeaconConfig().DataColumnSidecarSubnetCount
		result := s.allDataColumnSubnets(0)
		assert.Equal(t, dataColumnSidecarSubnetCount, uint64(len(result)))

		for i := range dataColumnSidecarSubnetCount {
			assert.Equal(t, true, result[i])
		}
	})
}

// TestProcessDataColumnSidecarsFromReconstruction_GloasSkipsProposerIndex is a regression test:
// Gloas sidecars don't expose a proposer index, so reconstruction must not call ProposerIndex().
// With no stored columns, reconstruction is unnecessary and the call returns nil instead of erroring.
func TestProcessDataColumnSidecarsFromReconstruction_GloasSkipsProposerIndex(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	s := &Service{
		ctx: t.Context(),
		cfg: &config{
			p2p:               mockp2p.NewTestP2P(t),
			clock:             startup.NewClock(time.Now(), [32]byte{}),
			dataColumnStorage: filesystem.NewEphemeralDataColumnStorage(t),
		},
		seenDataColumnCache: newSlotAwareCache(seenDataColumnSize),
	}

	var root [fieldparams.RootLength]byte
	root[0] = 0xEE
	gdc, err := blocks.NewRODataColumnGloasWithRoot(&ethpb.DataColumnSidecarGloas{
		Index:           0,
		Slot:            1,
		BeaconBlockRoot: root[:],
	}, root)
	require.NoError(t, err)
	v := blocks.NewVerifiedRODataColumn(gdc)

	require.NoError(t, s.processDataColumnSidecarsFromReconstruction(t.Context(), v))
}

// --- Gloas partial-column publish/republish tests ---

type recordedPartial struct {
	topic string
	col   blocks.PartialDataColumn
}

// recordingPartialBroadcaster is a partial-column Broadcaster that records the
// (topic, column) pairs passed to Publish so tests can assert on what was published.
type recordingPartialBroadcaster struct {
	mu        sync.Mutex
	published []recordedPartial
}

var _ partialdatacolumnbroadcaster.Broadcaster = (*recordingPartialBroadcaster)(nil)

func (r *recordingPartialBroadcaster) Publish(_ context.Context, seq iter.Seq2[string, blocks.PartialDataColumn]) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for topic, col := range seq {
		r.published = append(r.published, recordedPartial{topic: topic, col: col})
	}
	return nil
}

func (r *recordingPartialBroadcaster) columns() []recordedPartial {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.published)
}

func (*recordingPartialBroadcaster) Start(partialdatacolumnbroadcaster.ColumnCallbacks) {}
func (*recordingPartialBroadcaster) AppendPubSubOpts(opts []pubsub.Option) []pubsub.Option {
	return opts
}
func (*recordingPartialBroadcaster) Subscribe(context.Context, *pubsub.Topic) error { return nil }
func (*recordingPartialBroadcaster) Unsubscribe(context.Context, string) error      { return nil }

// newGloasPartialColumnService builds a sync Service wired with a recording partial-column
// broadcaster, an in-memory beacon DB, a mock chain, a clock and ephemeral column storage —
// enough to exercise the Gloas partial-column publish/republish paths.
func newGloasPartialColumnService(t *testing.T) (*Service, *recordingPartialBroadcaster, *mock.ChainService) {
	fake := &recordingPartialBroadcaster{}
	wrapped := &p2pWithPartialBroadcaster{P2P: mockp2p.NewTestP2P(t), broadcaster: fake}
	chain := &mock.ChainService{Genesis: time.Now()}
	s := &Service{
		ctx: t.Context(),
		cfg: &config{
			p2p:               wrapped,
			beaconDB:          dbtest.SetupDB(t),
			chain:             chain,
			clock:             startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			operationNotifier: &mock.MockOperationNotifier{},
			dataColumnStorage: filesystem.NewEphemeralDataColumnStorage(t),
		},
		seenDataColumnCache: newSlotAwareCache(seenDataColumnSize),
	}
	return s, fake, chain
}

// enableGloasForks sets the Deneb..Gloas fork epochs to 0 so Gloas blocks can be
// built, saved and read back within a test.
func enableGloasForks(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.DenebForkEpoch = 0
	cfg.ElectraForkEpoch = 0
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
}

// buildGloasBlockWithCommitments builds a Gloas signed block whose execution payload bid
// carries the given blob KZG commitments.
func buildGloasBlockWithCommitments(t *testing.T, slot primitives.Slot, commitments [][]byte) interfaces.SignedBeaconBlock {
	t.Helper()
	pb := util.NewBeaconBlockGloas()
	pb.Block.Slot = slot
	bid := util.GenerateTestSignedExecutionPayloadBid(slot)
	bid.Message.BlobKzgCommitments = commitments
	pb.Block.Body.SignedExecutionPayloadBid = bid
	signedBlock, err := blocks.NewSignedBeaconBlock(pb)
	require.NoError(t, err)
	return signedBlock
}

// buildGloasColumnFixture builds a Gloas block carrying the given bid commitments and a
// matching Gloas data column sidecar (its BeaconBlockRoot equals the block root).
func buildGloasColumnFixture(t *testing.T, slot primitives.Slot, index uint64, commitments [][]byte) (interfaces.SignedBeaconBlock, *ethpb.DataColumnSidecarGloas) {
	t.Helper()
	block := buildGloasBlockWithCommitments(t, slot, commitments)
	root, err := block.Block().HashTreeRoot()
	require.NoError(t, err)
	sidecar := &ethpb.DataColumnSidecarGloas{
		Index:           index,
		Slot:            slot,
		BeaconBlockRoot: root[:],
	}
	return block, sidecar
}

func gloasCommitments() [][]byte {
	return [][]byte{
		bytesutil.PadTo([]byte{'a'}, fieldparams.KzgCommitmentSize),
		bytesutil.PadTo([]byte{'b'}, fieldparams.KzgCommitmentSize),
	}
}

func TestBidCommitmentsForRoot(t *testing.T) {
	ctx := t.Context()

	t.Run("returns bid commitments for a stored block", func(t *testing.T) {
		enableGloasForks(t)

		commitments := gloasCommitments()
		block := buildGloasBlockWithCommitments(t, 1, commitments)
		beaconDB := dbtest.SetupDB(t)
		require.NoError(t, beaconDB.SaveBlock(ctx, block))
		root, err := block.Block().HashTreeRoot()
		require.NoError(t, err)

		s := &Service{ctx: ctx, cfg: &config{beaconDB: beaconDB}}
		got, err := s.bidCommitmentsForRoot(ctx, root)
		require.NoError(t, err)
		require.DeepEqual(t, commitments, got)
	})

	t.Run("errors when the block is missing", func(t *testing.T) {
		beaconDB := dbtest.SetupDB(t)
		s := &Service{ctx: ctx, cfg: &config{beaconDB: beaconDB}}

		var root [fieldparams.RootLength]byte
		root[0] = 0xAB
		_, err := s.bidCommitmentsForRoot(ctx, root)
		require.ErrorContains(t, "nil block", err)
	})
}

func TestPublishColumnAsPartial(t *testing.T) {
	t.Run("no-op when the broadcaster is nil", func(t *testing.T) {
		s := &Service{ctx: t.Context(), cfg: &config{p2p: mockp2p.NewTestP2P(t)}}
		_, verified := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{{
			Index:          1,
			KzgCommitments: [][]byte{bytesutil.PadTo([]byte{'c'}, fieldparams.KzgCommitmentSize)},
		}})
		// With no broadcaster the function must return without publishing (and without panicking).
		s.publishColumnAsPartial(t.Context(), verified[0])
	})

	t.Run("publishes a full column to the correct subnet topic", func(t *testing.T) {
		const index = uint64(5)
		commitments := [][]byte{bytesutil.PadTo([]byte{'c'}, fieldparams.KzgCommitmentSize)}
		_, verified := util.CreateTestVerifiedRoDataColumnSidecars(t, []util.DataColumnParam{{
			Index:          index,
			Slot:           2,
			KzgCommitments: commitments,
		}})

		s, fake, _ := newGloasPartialColumnService(t)
		s.publishColumnAsPartial(t.Context(), verified[0])

		published := fake.columns()
		require.Equal(t, 1, len(published))
		require.Equal(t, index, published[0].col.Index())

		gotComms, err := published[0].col.KzgCommitments()
		require.NoError(t, err)
		require.DeepEqual(t, commitments, gotComms)

		digest, err := s.currentForkDigest()
		require.NoError(t, err)
		subnet := peerdas.ComputeSubnetForDataColumnSidecar(index)
		wantTopic := fmt.Sprintf(p2p.DataColumnSubnetTopicFormat, digest, subnet) + s.cfg.p2p.Encoding().ProtocolSuffix()
		require.Equal(t, wantTopic, published[0].topic)
	})

	t.Run("does not publish when the partial column cannot be built", func(t *testing.T) {
		// A Gloas sidecar without bid commitments cannot produce a partial column,
		// so the publish iterator yields nothing.
		var root [fieldparams.RootLength]byte
		root[0] = 0x01
		ro, err := blocks.NewRODataColumnGloasWithRoot(&ethpb.DataColumnSidecarGloas{
			Index:           0,
			Slot:            1,
			BeaconBlockRoot: root[:],
		}, root)
		require.NoError(t, err)

		s, fake, _ := newGloasPartialColumnService(t)
		s.publishColumnAsPartial(t.Context(), blocks.NewVerifiedRODataColumn(ro))
		require.Equal(t, 0, len(fake.columns()))
	})
}

func TestRepublishGloasColumnAsPartial(t *testing.T) {
	ctx := t.Context()

	t.Run("no-op when the broadcaster is nil", func(t *testing.T) {
		s := &Service{ctx: ctx, cfg: &config{p2p: mockp2p.NewTestP2P(t)}}

		var root [fieldparams.RootLength]byte
		root[0] = 0x01
		ro, err := blocks.NewRODataColumnGloasWithRoot(&ethpb.DataColumnSidecarGloas{
			Index:           0,
			Slot:            1,
			BeaconBlockRoot: root[:],
		}, root)
		require.NoError(t, err)
		// Returns before touching the (nil) beaconDB.
		s.republishGloasColumnAsPartial(ctx, blocks.NewVerifiedRODataColumn(ro))
	})

	t.Run("does not publish when the block is missing", func(t *testing.T) {
		s, fake, _ := newGloasPartialColumnService(t)

		var root [fieldparams.RootLength]byte
		root[0] = 0xEE
		ro, err := blocks.NewRODataColumnGloasWithRoot(&ethpb.DataColumnSidecarGloas{
			Index:           0,
			Slot:            1,
			BeaconBlockRoot: root[:],
		}, root)
		require.NoError(t, err)

		s.republishGloasColumnAsPartial(ctx, blocks.NewVerifiedRODataColumn(ro))
		require.Equal(t, 0, len(fake.columns()))
	})

	t.Run("publishes the column with the block's bid commitments", func(t *testing.T) {
		enableGloasForks(t)

		const (
			slot  = primitives.Slot(3)
			index = uint64(7)
		)
		commitments := gloasCommitments()
		block, sidecarPb := buildGloasColumnFixture(t, slot, index, commitments)

		s, fake, _ := newGloasPartialColumnService(t)
		require.NoError(t, s.cfg.beaconDB.SaveBlock(ctx, block))

		ro, err := blocks.NewRODataColumnGloas(sidecarPb)
		require.NoError(t, err)
		s.republishGloasColumnAsPartial(ctx, blocks.NewVerifiedRODataColumn(ro))

		published := fake.columns()
		require.Equal(t, 1, len(published))
		require.Equal(t, index, published[0].col.Index())

		gotComms, err := published[0].col.KzgCommitments()
		require.NoError(t, err)
		require.DeepEqual(t, commitments, gotComms)

		digest, err := s.currentForkDigest()
		require.NoError(t, err)
		subnet := peerdas.ComputeSubnetForDataColumnSidecar(index)
		wantTopic := fmt.Sprintf(p2p.DataColumnSubnetTopicFormat, digest, subnet) + s.cfg.p2p.Encoding().ProtocolSuffix()
		require.Equal(t, wantTopic, published[0].topic)
	})
}

// TestDataColumnSubscriber_GloasRepublishFlow exercises the end-to-end Gloas path of
// dataColumnSubscriber: a received Gloas column is republished as a partial column carrying
// the block's bid commitments, and the column is received (marked seen + handed to the chain).
func TestDataColumnSubscriber_GloasRepublishFlow(t *testing.T) {
	enableGloasForks(t)
	ctx := t.Context()

	const (
		slot  = primitives.Slot(4)
		index = uint64(9)
	)
	commitments := gloasCommitments()
	block, sidecarPb := buildGloasColumnFixture(t, slot, index, commitments)

	s, fake, chain := newGloasPartialColumnService(t)
	require.NoError(t, s.cfg.beaconDB.SaveBlock(ctx, block))

	require.NoError(t, s.dataColumnSubscriber(ctx, sidecarPb))

	// Republished as a partial column carrying the block's bid commitments.
	published := fake.columns()
	require.Equal(t, 1, len(published))
	require.Equal(t, index, published[0].col.Index())
	gotComms, err := published[0].col.KzgCommitments()
	require.NoError(t, err)
	require.DeepEqual(t, commitments, gotComms)

	// Received: handed to the chain service and marked seen.
	require.Equal(t, 1, len(chain.DataColumns))
	require.Equal(t, index, chain.DataColumns[0].Index())

	root := bytesutil.ToBytes32(sidecarPb.BeaconBlockRoot)
	require.Equal(t, true, s.hasSeenDataColumnRootIndex(root, index))
}
