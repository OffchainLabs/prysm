package sync

import (
	"testing"

	chainMock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
	mockp2p "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time"
)

func TestAllDataColumnSubnets(t *testing.T) {
	t.Run("returns nil when no validators tracked", func(t *testing.T) {
		// Service with no tracked validators
		svc := &Service{
			ctx:                    t.Context(),
			trackedValidatorsCache: cache.NewTrackedValidatorsCache(),
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

		// At least one tracked validator.
		tvc := cache.NewTrackedValidatorsCache()
		tvc.Set(cache.TrackedValidator{Active: true, Index: 1})

		svc := &Service{
			ctx:                    ctx,
			trackedValidatorsCache: tvc,
			cfg: &config{
				stateGen: stateGen,
				beaconDB: beaconDB,
			},
		}

		dataColumnSidecarSubnetCount := params.BeaconConfig().DataColumnSidecarSubnetCount
		result := svc.allDataColumnSubnets(0)
		assert.Equal(t, dataColumnSidecarSubnetCount, uint64(len(result)))

		for i := range dataColumnSidecarSubnetCount {
			assert.Equal(t, true, result[i])
		}
	})
}

// saveGloasBlockWithCommitments builds a Gloas block carrying the given bid KZG commitments,
// persists it, and returns it as an ROBlock.
func saveGloasBlockWithCommitments(t *testing.T, database db.Database, slot primitives.Slot, commitments [][]byte) blocks.ROBlock {
	t.Helper()
	b := util.NewBeaconBlockGloas()
	b.Block.Slot = slot
	b.Block.Body.SignedExecutionPayloadBid.Message.BlobKzgCommitments = commitments
	sb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	roBlock, err := blocks.NewROBlock(sb)
	require.NoError(t, err)
	require.NoError(t, database.SaveBlock(t.Context(), sb))
	return roBlock
}

// TestConstructionPopulatorForSidecar verifies the populator selection: Fulu sidecars populate
// from the self-describing sidecar, while Gloas sidecars populate from the block's execution
// payload bid (looked up by block root in the DB).
func TestConstructionPopulatorForSidecar(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	ctx := t.Context()
	beaconDB := dbtest.SetupDB(t)
	s := &Service{ctx: ctx, cfg: &config{beaconDB: beaconDB}}

	t.Run("fulu sidecar populates from sidecar", func(t *testing.T) {
		dc := &ethpb.DataColumnSidecar{
			Index: 3,
			SignedBlockHeader: &ethpb.SignedBeaconBlockHeader{
				Header: &ethpb.BeaconBlockHeader{
					Slot:          1,
					ProposerIndex: 7,
					ParentRoot:    make([]byte, fieldparams.RootLength),
					StateRoot:     make([]byte, fieldparams.RootLength),
					BodyRoot:      make([]byte, fieldparams.RootLength),
				},
				Signature: make([]byte, fieldparams.BLSSignatureLength),
			},
		}
		ro, err := blocks.NewRODataColumn(dc)
		require.NoError(t, err)
		v := blocks.NewVerifiedRODataColumn(ro)

		source, err := s.constructionPopulatorForSidecar(ctx, v)
		require.NoError(t, err)
		require.Equal(t, peerdas.SidecarType, source.Type())
		require.Equal(t, v.BlockRoot(), source.Root())
	})

	t.Run("gloas sidecar populates from block bid", func(t *testing.T) {
		commitment := make([]byte, 48)
		commitment[0] = 0xAA
		roBlock := saveGloasBlockWithCommitments(t, beaconDB, 1, [][]byte{commitment})
		root := roBlock.Root()

		gdc, err := blocks.NewRODataColumnGloasWithRoot(&ethpb.DataColumnSidecarGloas{
			Index:           0,
			Slot:            1,
			BeaconBlockRoot: root[:],
		}, root)
		require.NoError(t, err)
		v := blocks.NewVerifiedRODataColumn(gdc)

		source, err := s.constructionPopulatorForSidecar(ctx, v)
		require.NoError(t, err)
		require.Equal(t, peerdas.BidType, source.Type())
		require.Equal(t, root, source.Root())
		require.Equal(t, primitives.Slot(1), source.Slot())

		proposerIndex, err := source.ProposerIndex()
		require.NoError(t, err)
		require.Equal(t, roBlock.Block().ProposerIndex(), proposerIndex)

		comms, err := source.Commitments()
		require.NoError(t, err)
		require.Equal(t, 1, len(comms))
		require.DeepEqual(t, commitment, comms[0])
	})

	t.Run("gloas sidecar without block returns error", func(t *testing.T) {
		var missing [fieldparams.RootLength]byte
		missing[0] = 0xCD
		gdc, err := blocks.NewRODataColumnGloasWithRoot(&ethpb.DataColumnSidecarGloas{
			Index:           0,
			Slot:            1,
			BeaconBlockRoot: missing[:],
		}, missing)
		require.NoError(t, err)
		v := blocks.NewVerifiedRODataColumn(gdc)

		_, err = s.constructionPopulatorForSidecar(ctx, v)
		require.ErrorContains(t, "not available", err)
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

// TestDataColumnSubscriber_GloasReconstructsFromExecution verifies the end-to-end wiring: receiving
// a single Gloas data column via gossip drives EL reconstruction from the block's bid, so the gossip
// column plus the EL-reconstructed columns are all received.
func TestDataColumnSubscriber_GloasReconstructsFromExecution(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.CapellaForkEpoch = 0
	cfg.DenebForkEpoch = 0
	cfg.ElectraForkEpoch = 0
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	ctx := t.Context()
	beaconDB := dbtest.SetupDB(t)

	// Persist a Gloas block with a single bid commitment.
	roBlock := saveGloasBlockWithCommitments(t, beaconDB, 1, [][]byte{make([]byte, 48)})
	root := roBlock.Root()

	// The EL returns the full set of Gloas columns for this root.
	elColumns := make([]blocks.VerifiedRODataColumn, fieldparams.NumberOfColumns)
	for i := range elColumns {
		gdc, err := blocks.NewRODataColumnGloasWithRoot(&ethpb.DataColumnSidecarGloas{
			Index:           uint64(i),
			Slot:            1,
			BeaconBlockRoot: root[:],
		}, root)
		require.NoError(t, err)
		elColumns[i] = blocks.NewVerifiedRODataColumn(gdc)
	}

	chainService := &chainMock.ChainService{Genesis: time.Now()}
	s := &Service{
		ctx: ctx,
		cfg: &config{
			p2p:                    mockp2p.NewTestP2P(t),
			chain:                  chainService,
			beaconDB:               beaconDB,
			clock:                  startup.NewClock(time.Now(), [32]byte{}),
			dataColumnStorage:      filesystem.NewEphemeralDataColumnStorage(t),
			executionReconstructor: &mockExecution.EngineClient{DataColumnSidecars: elColumns},
			operationNotifier:      &chainMock.MockOperationNotifier{},
		},
		seenDataColumnCache: newSlotAwareCache(seenDataColumnSize),
	}

	// Full custody so every column index is sampled.
	_, _, err := s.cfg.p2p.UpdateCustodyInfo(0, params.BeaconConfig().NumberOfCustodyGroups)
	require.NoError(t, err)

	// Receive a single Gloas column (index 0) via the subscriber.
	msg := &ethpb.DataColumnSidecarGloas{
		Index:           0,
		Slot:            1,
		BeaconBlockRoot: root[:],
	}
	require.NoError(t, s.dataColumnSubscriber(ctx, msg))

	// Gossip column (index 0) plus the 127 EL-reconstructed columns must be received,
	// proving the Gloas sidecar path drove EL reconstruction from the block bid.
	require.Equal(t, fieldparams.NumberOfColumns, len(chainService.DataColumns))
}
