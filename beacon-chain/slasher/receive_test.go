package slasher

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v6/async/event"
	mock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	dbtest "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	slashertypes "github.com/OffchainLabs/prysm/v6/beacon-chain/slasher/types"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	params2 "github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func TestSlasher_receiveAttestations_OK(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	s := &Service{
		serviceCfg: &ServiceConfig{
			IndexedAttestationsFeed: new(event.Feed),
			StateNotifier:           &mock.MockStateNotifier{},
			ClockWaiter:             startup.NewClockSynchronizer(),
		},
		attsQueue: newAttestationsQueue(),
	}
	indexedAttsChan := make(chan *slashertypes.WrappedIndexedAtt)
	defer close(indexedAttsChan)

	s.wg.Add(1)
	go func() {
		s.receiveAttestations(ctx, indexedAttsChan)
	}()
	firstIndices := []uint64{1, 2, 3}
	secondIndices := []uint64{4, 5, 6}
	att1 := createAttestationWrapperEmptySig(t, version.Phase0, 1, 2, firstIndices, nil)
	att2 := createAttestationWrapperEmptySig(t, version.Phase0, 1, 2, secondIndices, nil)
	wrappedAtt1 := &slashertypes.WrappedIndexedAtt{IndexedAtt: att1.IndexedAttestation}
	wrappedAtt2 := &slashertypes.WrappedIndexedAtt{IndexedAtt: att2.IndexedAttestation}
	indexedAttsChan <- wrappedAtt1
	indexedAttsChan <- wrappedAtt2
	cancel()
	s.wg.Wait()
	wanted := []*slashertypes.IndexedAttestationWrapper{
		{IndexedAttestation: att1.IndexedAttestation, DataRoot: att1.DataRoot},
		{IndexedAttestation: att2.IndexedAttestation, DataRoot: att2.DataRoot},
	}
	require.DeepEqual(t, wanted, s.attsQueue.dequeue())
}

func TestService_pruneSlasherDataWithinSlidingWindow_AttestationsPruned(t *testing.T) {
	ctx := t.Context()
	params := DefaultParams()
	params.historyLength = 4 // 4 epochs worth of history.
	slasherDB := dbtest.SetupSlasherDB(t)
	s := &Service{
		serviceCfg: &ServiceConfig{
			Database: slasherDB,
		},
		params: params,
	}

	// Setup attestations for 2 validators at each epoch for epochs 0, 1, 2, 3.
	err := slasherDB.SaveAttestationRecordsForValidators(ctx, []*slashertypes.IndexedAttestationWrapper{
		createAttestationWrapperEmptySig(t, version.Phase0, 0, 0, []uint64{0}, bytesutil.PadTo([]byte("0a"), 32)),
		createAttestationWrapperEmptySig(t, version.Phase0, 0, 0, []uint64{1}, bytesutil.PadTo([]byte("0b"), 32)),
		createAttestationWrapperEmptySig(t, version.Phase0, 0, 1, []uint64{0}, bytesutil.PadTo([]byte("1a"), 32)),
		createAttestationWrapperEmptySig(t, version.Phase0, 0, 1, []uint64{1}, bytesutil.PadTo([]byte("1b"), 32)),
		createAttestationWrapperEmptySig(t, version.Phase0, 0, 2, []uint64{0}, bytesutil.PadTo([]byte("2a"), 32)),
		createAttestationWrapperEmptySig(t, version.Phase0, 0, 2, []uint64{1}, bytesutil.PadTo([]byte("2b"), 32)),
		createAttestationWrapperEmptySig(t, version.Phase0, 0, 3, []uint64{0}, bytesutil.PadTo([]byte("3a"), 32)),
		createAttestationWrapperEmptySig(t, version.Phase0, 0, 3, []uint64{1}, bytesutil.PadTo([]byte("3b"), 32)),
	})
	require.NoError(t, err)

	// Attempt to prune and discover that all data is still intact.
	currentEpoch := primitives.Epoch(3)
	err = s.pruneSlasherDataWithinSlidingWindow(ctx, currentEpoch)
	require.NoError(t, err)

	epochs := []primitives.Epoch{0, 1, 2, 3}
	for _, epoch := range epochs {
		att, err := slasherDB.AttestationRecordForValidator(ctx, primitives.ValidatorIndex(0), epoch)
		require.NoError(t, err)
		require.NotNil(t, att)
		att, err = slasherDB.AttestationRecordForValidator(ctx, primitives.ValidatorIndex(1), epoch)
		require.NoError(t, err)
		require.NotNil(t, att)
	}

	// Setup attestations for 2 validators at epoch 4.
	err = slasherDB.SaveAttestationRecordsForValidators(ctx, []*slashertypes.IndexedAttestationWrapper{
		createAttestationWrapperEmptySig(t, version.Phase0, 0, 4, []uint64{0}, bytesutil.PadTo([]byte("4a"), 32)),
		createAttestationWrapperEmptySig(t, version.Phase0, 0, 4, []uint64{1}, bytesutil.PadTo([]byte("4b"), 32)),
	})
	require.NoError(t, err)

	// Attempt to prune again by setting current epoch to 4.
	currentEpoch = primitives.Epoch(4)
	err = s.pruneSlasherDataWithinSlidingWindow(ctx, currentEpoch)
	require.NoError(t, err)

	// We should now only have data for epochs 1, 2, 3, 4. We should
	// have pruned data from epoch 0.
	att, err := slasherDB.AttestationRecordForValidator(ctx, primitives.ValidatorIndex(0), 0)
	require.NoError(t, err)
	require.Equal(t, true, att == nil)
	att, err = slasherDB.AttestationRecordForValidator(ctx, primitives.ValidatorIndex(1), 0)
	require.NoError(t, err)
	require.Equal(t, true, att == nil)

	epochs = []primitives.Epoch{1, 2, 3, 4}
	for _, epoch := range epochs {
		att, err := slasherDB.AttestationRecordForValidator(ctx, primitives.ValidatorIndex(0), epoch)
		require.NoError(t, err)
		require.NotNil(t, att)
		att, err = slasherDB.AttestationRecordForValidator(ctx, primitives.ValidatorIndex(1), epoch)
		require.NoError(t, err)
		require.NotNil(t, att)
	}
}

func TestService_pruneSlasherDataWithinSlidingWindow_ProposalsPruned(t *testing.T) {
	ctx := t.Context()

	// Override beacon config to 1 slot per epoch for easier testing.
	params2.SetupTestConfigCleanup(t)
	config := params2.BeaconConfig().Copy()
	config.SlotsPerEpoch = 1
	params2.OverrideBeaconConfig(config)

	params := DefaultParams()
	params.historyLength = 4 // 4 epochs worth of history.
	slasherDB := dbtest.SetupSlasherDB(t)
	s := &Service{
		serviceCfg: &ServiceConfig{
			Database: slasherDB,
		},
		params: params,
	}

	// Setup block proposals for 2 validators at each epoch for epochs 0, 1, 2, 3.
	err := slasherDB.SaveBlockProposals(ctx, []*slashertypes.SignedBlockHeaderWrapper{
		createProposalWrapper(t, 0, 0, bytesutil.PadTo([]byte("0a"), 32)),
		createProposalWrapper(t, 0, 1, bytesutil.PadTo([]byte("0b"), 32)),
		createProposalWrapper(t, 1, 0, bytesutil.PadTo([]byte("1a"), 32)),
		createProposalWrapper(t, 1, 1, bytesutil.PadTo([]byte("1b"), 32)),
		createProposalWrapper(t, 2, 0, bytesutil.PadTo([]byte("2a"), 32)),
		createProposalWrapper(t, 2, 1, bytesutil.PadTo([]byte("2b"), 32)),
		createProposalWrapper(t, 3, 0, bytesutil.PadTo([]byte("3a"), 32)),
		createProposalWrapper(t, 3, 1, bytesutil.PadTo([]byte("3b"), 32)),
	})
	require.NoError(t, err)

	// Attempt to prune and discover that all data is still intact.
	currentEpoch := primitives.Epoch(3)
	err = s.pruneSlasherDataWithinSlidingWindow(ctx, currentEpoch)
	require.NoError(t, err)

	slots := []primitives.Slot{0, 1, 2, 3}
	for _, slot := range slots {
		blk, err := slasherDB.BlockProposalForValidator(ctx, primitives.ValidatorIndex(0), slot)
		require.NoError(t, err)
		require.NotNil(t, blk)
		blk, err = slasherDB.BlockProposalForValidator(ctx, primitives.ValidatorIndex(1), slot)
		require.NoError(t, err)
		require.NotNil(t, blk)
	}

	// Setup block proposals for 2 validators at epoch 4.
	err = slasherDB.SaveBlockProposals(ctx, []*slashertypes.SignedBlockHeaderWrapper{
		createProposalWrapper(t, 4, 0, bytesutil.PadTo([]byte("4a"), 32)),
		createProposalWrapper(t, 4, 1, bytesutil.PadTo([]byte("4b"), 32)),
	})
	require.NoError(t, err)

	// Attempt to prune again by setting current epoch to 4.
	currentEpoch = primitives.Epoch(4)
	err = s.pruneSlasherDataWithinSlidingWindow(ctx, currentEpoch)
	require.NoError(t, err)

	// We should now only have data for epochs 1, 2, 3, 4. We should
	// have pruned data from epoch 0.
	blk, err := slasherDB.BlockProposalForValidator(ctx, primitives.ValidatorIndex(0), 0)
	require.NoError(t, err)
	require.Equal(t, true, blk == nil)
	blk, err = slasherDB.BlockProposalForValidator(ctx, primitives.ValidatorIndex(1), 0)
	require.NoError(t, err)
	require.Equal(t, true, blk == nil)

	slots = []primitives.Slot{1, 2, 3, 4}
	for _, slot := range slots {
		blk, err := slasherDB.BlockProposalForValidator(ctx, primitives.ValidatorIndex(0), slot)
		require.NoError(t, err)
		require.NotNil(t, blk)
		blk, err = slasherDB.BlockProposalForValidator(ctx, primitives.ValidatorIndex(1), slot)
		require.NoError(t, err)
		require.NotNil(t, blk)
	}
}

func TestSlasher_receiveAttestations_OnlyValidAttestations(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	s := &Service{
		serviceCfg: &ServiceConfig{
			IndexedAttestationsFeed: new(event.Feed),
			StateNotifier:           &mock.MockStateNotifier{},
			ClockWaiter:             startup.NewClockSynchronizer(),
		},
		attsQueue: newAttestationsQueue(),
	}
	indexedAttsChan := make(chan *slashertypes.WrappedIndexedAtt)
	defer close(indexedAttsChan)

	s.wg.Add(1)
	go func() {
		s.receiveAttestations(ctx, indexedAttsChan)
	}()
	firstIndices := []uint64{1, 2, 3}
	secondIndices := []uint64{4, 5, 6}
	// Add a valid attestation.
	validAtt := createAttestationWrapperEmptySig(t, version.Phase0, 1, 2, firstIndices, nil)
	wrappedValidAtt := &slashertypes.WrappedIndexedAtt{IndexedAtt: validAtt.IndexedAttestation}
	indexedAttsChan <- wrappedValidAtt
	// Send an invalid, bad attestation which will not
	// pass integrity checks at it has invalid attestation data.
	indexedAttsChan <- &slashertypes.WrappedIndexedAtt{
		IndexedAtt: &ethpb.IndexedAttestation{
			AttestingIndices: secondIndices,
		},
	}
	cancel()
	s.wg.Wait()
	// Expect only a single, valid attestation was added to the queue.
	require.Equal(t, 1, s.attsQueue.size())
	wanted := []*slashertypes.IndexedAttestationWrapper{
		{IndexedAttestation: validAtt.IndexedAttestation, DataRoot: validAtt.DataRoot},
	}
	require.DeepEqual(t, wanted, s.attsQueue.dequeue())
}

func TestSlasher_receiveBlocks_OK(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	s := &Service{
		serviceCfg: &ServiceConfig{
			BeaconBlockHeadersFeed: new(event.Feed),
			StateNotifier:          &mock.MockStateNotifier{},
			ClockWaiter:            startup.NewClockSynchronizer(),
		},
		blksQueue: newBlocksQueue(),
	}
	beaconBlockHeadersChan := make(chan *ethpb.SignedBeaconBlockHeader)
	defer close(beaconBlockHeadersChan)
	s.wg.Add(1)
	go func() {
		s.receiveBlocks(ctx, beaconBlockHeadersChan)
	}()

	block1 := createProposalWrapper(t, 0, 1, nil).SignedBeaconBlockHeader
	block2 := createProposalWrapper(t, 0, 2, nil).SignedBeaconBlockHeader
	beaconBlockHeadersChan <- block1
	beaconBlockHeadersChan <- block2
	cancel()
	s.wg.Wait()
	wanted := []*slashertypes.SignedBlockHeaderWrapper{
		createProposalWrapper(t, 0, block1.Header.ProposerIndex, nil),
		createProposalWrapper(t, 0, block2.Header.ProposerIndex, nil),
	}
	require.DeepEqual(t, wanted, s.blksQueue.dequeue())
}

func TestService_processQueuedBlocks(t *testing.T) {
	hook := logTest.NewGlobal()
	slasherDB := dbtest.SetupSlasherDB(t)

	beaconState, err := util.NewBeaconState()
	require.NoError(t, err)
	currentSlot := primitives.Slot(0)
	require.NoError(t, beaconState.SetSlot(currentSlot))
	mockChain := &mock.ChainService{
		State: beaconState,
		Slot:  &currentSlot,
	}

	s := &Service{
		params: DefaultParams(),
		serviceCfg: &ServiceConfig{
			Database:         slasherDB,
			StateNotifier:    &mock.MockStateNotifier{},
			HeadStateFetcher: mockChain,
			ClockWaiter:      startup.NewClockSynchronizer(),
		},
		blksQueue: newBlocksQueue(),
	}
	s.blksQueue.extend([]*slashertypes.SignedBlockHeaderWrapper{
		createProposalWrapper(t, 0, 1, nil),
	})
	ctx, cancel := context.WithCancel(t.Context())
	tickerChan := make(chan primitives.Slot)
	s.wg.Add(1)
	go func() {
		s.processQueuedBlocks(ctx, tickerChan)
	}()

	// Send a value over the ticker.
	tickerChan <- 0
	cancel()
	s.wg.Wait()
	assert.LogsContain(t, hook, "Processing queued")
}
