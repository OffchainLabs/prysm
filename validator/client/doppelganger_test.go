package client

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	validatormock "github.com/OffchainLabs/prysm/v7/testing/validator-mock"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	dbTest "github.com/OffchainLabs/prysm/v7/validator/db/testing"
	"github.com/pkg/errors"
	"go.uber.org/mock/gomock"
)

// doppelTestValidator returns a validator whose genesis time puts the current
// slot 3/4 into epoch epochsElapsed, past MaybeCheckDoppelGanger's poll gate.
func doppelTestValidator(epochsElapsed uint64) *validator {
	spe := uint64(params.BeaconConfig().SlotsPerEpoch)
	slotsElapsed := epochsElapsed*spe + spe*3/4
	return &validator{
		genesisTime: time.Now().Add(-time.Duration(slotsElapsed*params.BeaconConfig().SecondsPerSlot) * time.Second),
	}
}

func enableDoppelGanger(t *testing.T) {
	flgs := *features.Get() // copy: mutating the live pointer would leak past reset
	flgs.EnableDoppelGanger = true
	reset := features.InitWithReset(&flgs)
	t.Cleanup(reset)
}

func TestTrackReloadedKeysForDoppelGanger(t *testing.T) {
	enableDoppelGanger(t)
	v := doppelTestValidator(4)
	keyA := bytesutil.ToBytes48([]byte{0xaa})
	keyB := bytesutil.ToBytes48([]byte{0xbb})
	v.markDoppelGangerChecked([][fieldparams.BLSPubkeyLength]byte{keyA})

	// Only the never-checked key is quarantined.
	v.trackReloadedKeysForDoppelGanger([][fieldparams.BLSPubkeyLength]byte{keyA, keyB})
	assert.Equal(t, false, v.isDoppelGangerPending(keyA))
	assert.Equal(t, true, v.isDoppelGangerPending(keyB))

	// Removing a key forgets it; re-adding quarantines it again.
	v.trackReloadedKeysForDoppelGanger([][fieldparams.BLSPubkeyLength]byte{keyA})
	assert.Equal(t, false, v.isDoppelGangerPending(keyB))
	v.trackReloadedKeysForDoppelGanger([][fieldparams.BLSPubkeyLength]byte{keyA, keyB})
	assert.Equal(t, true, v.isDoppelGangerPending(keyB))
}

func TestTrackReloadedKeysForDoppelGanger_GenesisEpochQuarantines(t *testing.T) {
	enableDoppelGanger(t)
	v := doppelTestValidator(0) // current epoch is the genesis epoch
	keyA := bytesutil.ToBytes48([]byte{0xaa})
	v.trackReloadedKeysForDoppelGanger([][fieldparams.BLSPubkeyLength]byte{keyA})
	// A duplicate can already be attesting within the genesis epoch, so keys
	// added mid-epoch-0 are quarantined like any other reload.
	assert.Equal(t, true, v.isDoppelGangerPending(keyA))
}

func TestTrackReloadedKeysForDoppelGanger_FlagOff(t *testing.T) {
	flgs := *features.Get()
	flgs.EnableDoppelGanger = false
	reset := features.InitWithReset(&flgs)
	t.Cleanup(reset)
	v := doppelTestValidator(4)
	keyB := bytesutil.ToBytes48([]byte{0xbb})
	v.trackReloadedKeysForDoppelGanger([][fieldparams.BLSPubkeyLength]byte{keyB})
	assert.Equal(t, false, v.isDoppelGangerPending(keyB))
}

func TestFilteredKeysAndIndices_ExcludesDoppelGangerPending(t *testing.T) {
	enableDoppelGanger(t)
	v := doppelTestValidator(4)
	keyA := bytesutil.ToBytes48([]byte{0xaa})
	keyB := bytesutil.ToBytes48([]byte{0xbb})
	v.pubkeyToStatus = map[pubkey]*validatorStatus{
		keyA: {publicKey: keyA[:], status: &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE}, index: 1},
		keyB: {publicKey: keyB[:], status: &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE}, index: 2},
	}
	v.markDoppelGangerChecked([][fieldparams.BLSPubkeyLength]byte{keyA})
	v.trackReloadedKeysForDoppelGanger([][fieldparams.BLSPubkeyLength]byte{keyA, keyB})

	keys, indices := v.filteredKeysAndIndices([][fieldparams.BLSPubkeyLength]byte{keyA, keyB}, 4)
	require.Equal(t, 1, len(keys))
	assert.Equal(t, keyA, keys[0])
	require.Equal(t, 1, len(indices))
	assert.Equal(t, 1, int(indices[0]))
}

func waitForDoppelCheck(t *testing.T, v *validator) {
	deadline := time.Now().Add(2 * time.Second)
	for v.doppelGanger.inFlight.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	require.Equal(t, false, v.doppelGanger.inFlight.Load(), "doppelganger check did not finish")
}

// mockSyncedChainHead points the validator at a chain whose head is at headEpoch,
// so clearElapsed sees an up-to-date beacon node.
func mockSyncedChainHead(ctrl *gomock.Controller, v *validator, headEpoch primitives.Epoch) {
	chain := validatormock.NewMockChainClient(ctrl)
	chain.EXPECT().ChainHead(gomock.Any(), gomock.Any()).Return(&ethpb.ChainHead{HeadEpoch: headEpoch}, nil).AnyTimes()
	v.chainClient = chain
}

func TestMaybeCheckDoppelGanger_ClearsAndBlocks(t *testing.T) {
	enableDoppelGanger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	keyA := bytesutil.ToBytes48([]byte{0xaa})
	keyB := bytesutil.ToBytes48([]byte{0xbb})
	db := dbTest.SetupDB(t, t.TempDir(), [][fieldparams.BLSPubkeyLength]byte{keyA, keyB}, false)
	v := doppelTestValidator(4)
	v.validatorClient = client
	v.db = db
	v.doppelGanger.pending = map[pubkey]*doppelGangerPendingKey{
		keyA: {addedEpoch: 1},
		keyB: {addedEpoch: 1},
	}
	v.doppelGanger.pendingCount.Store(2)
	mockSyncedChainHead(ctrl, v, 4)

	client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(&ethpb.DoppelGangerResponse{
		Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{
			{PublicKey: keyA[:], DuplicateExists: false},
			{PublicKey: keyB[:], DuplicateExists: true},
		},
	}, nil)

	v.MaybeCheckDoppelGanger(t.Context(), slots.CurrentSlot(v.genesisTime))
	waitForDoppelCheck(t, v)

	// Clean key cleared; duplicate stays excluded and is never polled again.
	assert.Equal(t, false, v.isDoppelGangerPending(keyA))
	assert.Equal(t, true, v.isDoppelGangerPending(keyB))
	assert.Equal(t, 0, len(v.doppelGanger.pollDue(100)))
}

func TestMaybeCheckDoppelGanger_EarlyPoll(t *testing.T) {
	enableDoppelGanger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	keyA := bytesutil.ToBytes48([]byte{0xaa})
	db := dbTest.SetupDB(t, t.TempDir(), [][fieldparams.BLSPubkeyLength]byte{keyA}, false)
	v := doppelTestValidator(4)
	v.validatorClient = client
	v.db = db
	// Added this epoch: polled immediately, but a clean result must NOT clear
	// before the quarantine elapses.
	v.doppelGanger.pending = map[pubkey]*doppelGangerPendingKey{keyA: {addedEpoch: 4}}
	v.doppelGanger.pendingCount.Store(1)
	mockSyncedChainHead(ctrl, v, 4)

	client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(&ethpb.DoppelGangerResponse{
		Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{
			{PublicKey: keyA[:], DuplicateExists: false},
		},
	}, nil)

	slot := slots.CurrentSlot(v.genesisTime)
	v.MaybeCheckDoppelGanger(t.Context(), slot)
	waitForDoppelCheck(t, v)
	assert.Equal(t, true, v.isDoppelGangerPending(keyA)) // clean but not elapsed: still quarantined

	// Same epoch: already polled, no second RPC (the single EXPECT enforces it).
	v.MaybeCheckDoppelGanger(t.Context(), slot)
	assert.Equal(t, false, v.doppelGanger.inFlight.Load())
}

func TestMaybeCheckDoppelGanger_EarlyPollBlocksDuplicate(t *testing.T) {
	enableDoppelGanger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	keyA := bytesutil.ToBytes48([]byte{0xaa})
	db := dbTest.SetupDB(t, t.TempDir(), [][fieldparams.BLSPubkeyLength]byte{keyA}, false)
	v := doppelTestValidator(4)
	v.validatorClient = client
	v.db = db
	v.doppelGanger.pending = map[pubkey]*doppelGangerPendingKey{keyA: {addedEpoch: 4}}
	v.doppelGanger.pendingCount.Store(1)

	client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(&ethpb.DoppelGangerResponse{
		Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{
			{PublicKey: keyA[:], DuplicateExists: true},
		},
	}, nil)

	v.MaybeCheckDoppelGanger(t.Context(), slots.CurrentSlot(v.genesisTime))
	waitForDoppelCheck(t, v)

	// Duplicate is blocked at the first poll, well before the quarantine ends.
	assert.Equal(t, true, v.isDoppelGangerPending(keyA))
	assert.Equal(t, 0, len(v.doppelGanger.pollDue(100)))
}

func TestMaybeCheckDoppelGanger_FailureRetriesSameEpoch(t *testing.T) {
	enableDoppelGanger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	keyA := bytesutil.ToBytes48([]byte{0xaa})
	db := dbTest.SetupDB(t, t.TempDir(), [][fieldparams.BLSPubkeyLength]byte{keyA}, false)
	v := doppelTestValidator(4)
	v.validatorClient = client
	v.db = db
	v.doppelGanger.pending = map[pubkey]*doppelGangerPendingKey{keyA: {addedEpoch: 1}}
	v.doppelGanger.pendingCount.Store(1)
	mockSyncedChainHead(ctrl, v, 4)

	// First check fails: the poll epoch must NOT be consumed, so the very next
	// slot retries and the second (successful) check clears the key.
	gomock.InOrder(
		client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(nil, errors.New("bn down")),
		client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(&ethpb.DoppelGangerResponse{
			Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{{PublicKey: keyA[:], DuplicateExists: false}},
		}, nil),
	)

	slot := slots.CurrentSlot(v.genesisTime)
	v.MaybeCheckDoppelGanger(t.Context(), slot)
	waitForDoppelCheck(t, v)
	assert.Equal(t, true, v.isDoppelGangerPending(keyA)) // failure: still quarantined

	v.MaybeCheckDoppelGanger(t.Context(), slot) // same epoch retry succeeds
	waitForDoppelCheck(t, v)
	assert.Equal(t, false, v.isDoppelGangerPending(keyA))
}

func TestMaybeCheckDoppelGanger_PartialResponseKeepsQuarantine(t *testing.T) {
	enableDoppelGanger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	keyA := bytesutil.ToBytes48([]byte{0xaa})
	keyB := bytesutil.ToBytes48([]byte{0xbb})
	db := dbTest.SetupDB(t, t.TempDir(), [][fieldparams.BLSPubkeyLength]byte{keyA, keyB}, false)
	v := doppelTestValidator(4)
	v.validatorClient = client
	v.db = db
	v.doppelGanger.pending = map[pubkey]*doppelGangerPendingKey{
		keyA: {addedEpoch: 1},
		keyB: {addedEpoch: 1},
	}
	v.doppelGanger.pendingCount.Store(2)
	mockSyncedChainHead(ctrl, v, 4)

	// Response omits keyB entirely: only the explicitly-clean keyA may clear.
	client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(&ethpb.DoppelGangerResponse{
		Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{{PublicKey: keyA[:], DuplicateExists: false}},
	}, nil)

	v.MaybeCheckDoppelGanger(t.Context(), slots.CurrentSlot(v.genesisTime))
	waitForDoppelCheck(t, v)
	assert.Equal(t, false, v.isDoppelGangerPending(keyA))
	assert.Equal(t, true, v.isDoppelGangerPending(keyB)) // absent from response: fail-closed
}

func TestMaybeCheckDoppelGanger_SingleFlightAndFlagOff(t *testing.T) {
	enableDoppelGanger(t)
	// No mock client: any RPC would panic, proving the guards short-circuit.
	v := doppelTestValidator(4)
	keyA := bytesutil.ToBytes48([]byte{0xaa})
	v.doppelGanger.pending = map[pubkey]*doppelGangerPendingKey{keyA: {addedEpoch: 1}}
	v.doppelGanger.pendingCount.Store(1)

	v.doppelGanger.inFlight.Store(true) // a check is already running
	v.MaybeCheckDoppelGanger(t.Context(), slots.CurrentSlot(v.genesisTime))
	v.doppelGanger.inFlight.Store(false)

	flgs := *features.Get()
	flgs.EnableDoppelGanger = false
	reset := features.InitWithReset(&flgs)
	defer reset()
	v.MaybeCheckDoppelGanger(t.Context(), slots.CurrentSlot(v.genesisTime)) // flag off
	assert.Equal(t, true, v.isDoppelGangerPending(keyA))
}

func TestMaybeCheckDoppelGanger_SkipsEarlyEpochSlots(t *testing.T) {
	enableDoppelGanger(t)
	// No mock client: an RPC would panic, proving the poll gate short-circuits.
	v := doppelTestValidator(4)
	keyA := bytesutil.ToBytes48([]byte{0xaa})
	v.doppelGanger.pending = map[pubkey]*doppelGangerPendingKey{keyA: {addedEpoch: 1}}
	v.doppelGanger.pendingCount.Store(1)

	// Slot 2 of epoch 4: before the 3/4-epoch poll point, no check may fire.
	earlySlot := primitives.Slot(4*uint64(params.BeaconConfig().SlotsPerEpoch) + 2)
	v.MaybeCheckDoppelGanger(t.Context(), earlySlot)
	assert.Equal(t, false, v.doppelGanger.inFlight.Load())
	assert.Equal(t, true, v.isDoppelGangerPending(keyA))
}

func TestMaybeCheckDoppelGanger_LaggingHeadDefersClear(t *testing.T) {
	enableDoppelGanger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	keyA := bytesutil.ToBytes48([]byte{0xaa})
	db := dbTest.SetupDB(t, t.TempDir(), [][fieldparams.BLSPubkeyLength]byte{keyA}, false)
	v := doppelTestValidator(4)
	v.validatorClient = client
	v.db = db
	v.doppelGanger.pending = map[pubkey]*doppelGangerPendingKey{keyA: {addedEpoch: 1}}
	v.doppelGanger.pendingCount.Store(1)
	// Head stalled inside the wait: its clean answers are unevaluated defaults.
	mockSyncedChainHead(ctrl, v, 3)

	client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(&ethpb.DoppelGangerResponse{
		Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{{PublicKey: keyA[:], DuplicateExists: false}},
	}, nil)

	v.MaybeCheckDoppelGanger(t.Context(), slots.CurrentSlot(v.genesisTime))
	waitForDoppelCheck(t, v)
	assert.Equal(t, true, v.isDoppelGangerPending(keyA)) // clear deferred until the head catches up
}

func TestMaybeCheckDoppelGanger_HeadErrorDefersClear(t *testing.T) {
	enableDoppelGanger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	keyA := bytesutil.ToBytes48([]byte{0xaa})
	db := dbTest.SetupDB(t, t.TempDir(), [][fieldparams.BLSPubkeyLength]byte{keyA}, false)
	v := doppelTestValidator(4)
	v.validatorClient = client
	v.db = db
	v.doppelGanger.pending = map[pubkey]*doppelGangerPendingKey{keyA: {addedEpoch: 1}}
	v.doppelGanger.pendingCount.Store(1)
	chain := validatormock.NewMockChainClient(ctrl)
	chain.EXPECT().ChainHead(gomock.Any(), gomock.Any()).Return(nil, errors.New("bn down"))
	v.chainClient = chain

	client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(&ethpb.DoppelGangerResponse{
		Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{{PublicKey: keyA[:], DuplicateExists: false}},
	}, nil)

	v.MaybeCheckDoppelGanger(t.Context(), slots.CurrentSlot(v.genesisTime))
	waitForDoppelCheck(t, v)
	assert.Equal(t, true, v.isDoppelGangerPending(keyA)) // unknown head: fail-closed, no clear
	// The poll is left unconsumed so the next slot in this epoch retries.
	epoch := slots.ToEpoch(slots.CurrentSlot(v.genesisTime))
	assert.Equal(t, 1, len(v.doppelGanger.pollDue(epoch)))
}

func TestMaybeCheckDoppelGanger_EmptyResponseCountsPoll(t *testing.T) {
	enableDoppelGanger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	keyA := bytesutil.ToBytes48([]byte{0xaa})
	db := dbTest.SetupDB(t, t.TempDir(), [][fieldparams.BLSPubkeyLength]byte{keyA}, false)
	v := doppelTestValidator(4)
	v.validatorClient = client
	v.db = db
	v.doppelGanger.pending = map[pubkey]*doppelGangerPendingKey{keyA: {addedEpoch: 1}}
	v.doppelGanger.pendingCount.Store(1)

	// Key unknown to the node: a definitive empty answer, not a failure. The
	// single EXPECT proves the same epoch does not re-poll every slot.
	client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(&ethpb.DoppelGangerResponse{
		Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{},
	}, nil)

	slot := slots.CurrentSlot(v.genesisTime)
	v.MaybeCheckDoppelGanger(t.Context(), slot)
	waitForDoppelCheck(t, v)
	assert.Equal(t, true, v.isDoppelGangerPending(keyA)) // stays quarantined

	v.MaybeCheckDoppelGanger(t.Context(), slot) // same epoch: no second RPC
	assert.Equal(t, false, v.doppelGanger.inFlight.Load())
}

func TestRolesAt_ExcludesDoppelGangerPending(t *testing.T) {
	enableDoppelGanger(t)
	v := doppelTestValidator(4)
	v.duties = &dutyStore{}
	v.aggSelector = &distributedSelector{}
	keyA := bytesutil.ToBytes48([]byte{0xaa})
	keyB := bytesutil.ToBytes48([]byte{0xbb})
	var data dutyStoreData
	data.setFromContainer(&ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{
			{PublicKey: keyA[:], ValidatorIndex: 1},
			{PublicKey: keyB[:], ValidatorIndex: 2},
		},
	})
	v.duties.write(data)
	v.trackReloadedKeysForDoppelGanger([][fieldparams.BLSPubkeyLength]byte{keyA, keyB})
	v.markDoppelGangerChecked([][fieldparams.BLSPubkeyLength]byte{keyB})

	// Duties fetched before the reload must not grant the quarantined key roles.
	roles, err := v.RolesAt(t.Context(), 1)
	require.NoError(t, err)
	_, hasA := roles[keyA]
	assert.Equal(t, false, hasA)
	_, hasB := roles[keyB]
	assert.Equal(t, true, hasB)
}

func TestDoppelGangerTracker_BlockIgnoresUntrackedKeys(t *testing.T) {
	d := &doppelGangerTracker{}
	keyA := bytesutil.ToBytes48([]byte{0x01})
	// A key removed mid-check must not be resurrected or logged as excluded.
	d.block([][fieldparams.BLSPubkeyLength]byte{keyA})
	d.mu.RLock()
	assert.Equal(t, 0, len(d.pending))
	d.mu.RUnlock()
}

func TestDoppelGangerTracker_ClearElapsedFilters(t *testing.T) {
	d := &doppelGangerTracker{}
	elapsed := bytesutil.ToBytes48([]byte{0x01})
	tooNew := bytesutil.ToBytes48([]byte{0x02})
	blocked := bytesutil.ToBytes48([]byte{0x03})
	d.pending = map[pubkey]*doppelGangerPendingKey{
		elapsed: {addedEpoch: 1},
		tooNew:  {addedEpoch: 9},
		blocked: {addedEpoch: 1, blocked: true},
	}
	d.pendingCount.Store(3)

	cleared := d.clearElapsed([][fieldparams.BLSPubkeyLength]byte{elapsed, tooNew, blocked}, 10)
	require.Equal(t, 1, len(cleared))
	assert.Equal(t, elapsed, cleared[0])
	assert.Equal(t, false, d.isPending(elapsed))
	assert.Equal(t, true, d.isPending(tooNew))
	assert.Equal(t, true, d.isPending(blocked))
}

func TestDoppelGangerTracker_PollsAgainNextEpoch(t *testing.T) {
	d := &doppelGangerTracker{}
	keyA := bytesutil.ToBytes48([]byte{0x01})
	d.pending = map[pubkey]*doppelGangerPendingKey{keyA: {addedEpoch: 4}}
	d.pendingCount.Store(1)

	require.Equal(t, 1, len(d.pollDue(4)))
	d.markPolled(4)
	assert.Equal(t, 0, len(d.pollDue(4))) // consumed for this epoch
	assert.Equal(t, 1, len(d.pollDue(5))) // next epoch polls again
}

func TestTrackReloadedKeysForDoppelGanger_ReimportKeepsClock(t *testing.T) {
	enableDoppelGanger(t)
	v := doppelTestValidator(6)
	keyA := bytesutil.ToBytes48([]byte{0xaa})
	v.doppelGanger.pending = map[pubkey]*doppelGangerPendingKey{keyA: {addedEpoch: 2}}
	v.doppelGanger.pendingCount.Store(1)

	// Re-importing a still-quarantined key must not reset its wait.
	v.trackReloadedKeysForDoppelGanger([][fieldparams.BLSPubkeyLength]byte{keyA})
	v.doppelGanger.mu.RLock()
	assert.Equal(t, 2, int(v.doppelGanger.pending[keyA].addedEpoch))
	v.doppelGanger.mu.RUnlock()
}

func TestHandleKeyReload_QuarantinesNewKeys(t *testing.T) {
	enableDoppelGanger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	keyOld := bytesutil.ToBytes48([]byte{0xaa})
	keyNew := bytesutil.ToBytes48([]byte{0xbb})
	v := doppelTestValidator(4)
	v.validatorClient = client
	v.pubkeyToStatus = map[pubkey]*validatorStatus{}
	v.markDoppelGangerChecked([][fieldparams.BLSPubkeyLength]byte{keyOld})

	client.EXPECT().MultipleValidatorStatus(gomock.Any(), gomock.Any()).Return(&ethpb.MultipleValidatorStatusResponse{
		PublicKeys: [][]byte{keyOld[:], keyNew[:]},
		Statuses: []*ethpb.ValidatorStatusResponse{
			{Status: ethpb.ValidatorStatus_ACTIVE},
			{Status: ethpb.ValidatorStatus_ACTIVE},
		},
		Indices: []primitives.ValidatorIndex{1, 2},
	}, nil)

	_, err := v.HandleKeyReload(t.Context(), [][fieldparams.BLSPubkeyLength]byte{keyOld, keyNew})
	require.NoError(t, err)
	assert.Equal(t, false, v.isDoppelGangerPending(keyOld))
	assert.Equal(t, true, v.isDoppelGangerPending(keyNew))
}

func TestCheckDoppelGanger_StartupProceedsWhenKeysUnknown(t *testing.T) {
	enableDoppelGanger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	km := genMockKeymanager(t, 2)
	keys, err := km.FetchValidatingPublicKeys(t.Context())
	require.NoError(t, err)
	db := dbTest.SetupDB(t, t.TempDir(), keys, false)
	v := doppelTestValidator(4)
	v.validatorClient = client
	v.km = km
	v.db = db

	// Keys with no validator index yet: the node has nothing to evaluate, and no
	// duplicate can exist for an index-less key, so startup must proceed.
	client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(&ethpb.DoppelGangerResponse{
		Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{},
	}, nil)

	require.NoError(t, v.CheckDoppelGanger(t.Context()))
	v.trackReloadedKeysForDoppelGanger(keys)
	for _, k := range keys {
		assert.Equal(t, false, v.isDoppelGangerPending(k))
	}
}

func TestCheckDoppelGanger_MarksKeysChecked(t *testing.T) {
	enableDoppelGanger(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	km := genMockKeymanager(t, 3)
	keys, err := km.FetchValidatingPublicKeys(t.Context())
	require.NoError(t, err)
	db := dbTest.SetupDB(t, t.TempDir(), keys, false)
	v := doppelTestValidator(4)
	v.validatorClient = client
	v.km = km
	v.db = db

	resp := &ethpb.DoppelGangerResponse{}
	for _, k := range keys {
		resp.Responses = append(resp.Responses, &ethpb.DoppelGangerResponse_ValidatorResponse{PublicKey: k[:], DuplicateExists: false})
	}
	client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(resp, nil)

	require.NoError(t, v.CheckDoppelGanger(t.Context()))

	// Startup keys are recorded as checked, so a reload does not quarantine them.
	v.trackReloadedKeysForDoppelGanger(keys)
	for _, k := range keys {
		assert.Equal(t, false, v.isDoppelGangerPending(k))
	}
}
