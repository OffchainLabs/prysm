package client

import (
	"context"
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
	logTest "github.com/sirupsen/logrus/hooks/test"
	"go.uber.org/mock/gomock"
)

// doppelTestValidator returns a validator whose genesis time puts the current
// slot at epoch epochsElapsed's poll slot, so CheckDoppelGangerMidEpoch may fire.
func doppelTestValidator(epochsElapsed uint64) *validator {
	spe := uint64(params.BeaconConfig().SlotsPerEpoch)
	slotsElapsed := epochsElapsed*spe + uint64(doppelGangerPollSlot())
	return &validator{
		genesisTime: time.Now().Add(-time.Duration(slotsElapsed*params.BeaconConfig().SecondsPerSlot) * time.Second),
	}
}

// markDoppelGangerChecked seeds keys as already vetted, via the same path a
// clean startup check takes (test helper).
func (v *validator) markDoppelGangerChecked(keys [][fieldparams.BLSPubkeyLength]byte) {
	responses := make([]*ethpb.DoppelGangerResponse_ValidatorResponse, len(keys))
	for i, pk := range keys {
		responses[i] = &ethpb.DoppelGangerResponse_ValidatorResponse{PublicKey: pk[:], DuplicateExists: false}
	}
	v.vetStartupKeys(keys, responses)
}

func setDoppelGangerFlag(t *testing.T, enabled bool) {
	flgs := *features.Get() // copy: mutating the live pointer would leak past reset
	flgs.EnableDoppelGanger = enabled
	reset := features.InitWithReset(&flgs)
	t.Cleanup(reset)
}

func enableDoppelGanger(t *testing.T) {
	setDoppelGangerFlag(t, true)
}

func waitForDoppelCheck(t *testing.T, v *validator) {
	require.Eventually(t, func() bool {
		return !v.doppelGanger.inFlight.Load()
	}, 2*time.Second, 10*time.Millisecond, "doppelganger check did not finish")
}

// mockSyncedChainHead points the validator at a chain whose head is at headEpoch,
// so clearElapsed sees an up-to-date beacon node.
func mockSyncedChainHead(ctrl *gomock.Controller, v *validator, headEpoch primitives.Epoch) {
	chain := validatormock.NewMockChainClient(ctrl)
	chain.EXPECT().ChainHead(gomock.Any(), gomock.Any()).Return(&ethpb.ChainHead{HeadEpoch: headEpoch}, nil).AnyTimes()
	v.chainClient = chain
}

func TestTrackReloadedKeysForDoppelGanger(t *testing.T) {
	keyA := bytesutil.ToBytes48([]byte{0xaa})
	keyB := bytesutil.ToBytes48([]byte{0xbb})

	t.Run("quarantines new keys and forgets removed ones", func(t *testing.T) {
		enableDoppelGanger(t)
		v := doppelTestValidator(4)
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
	})

	t.Run("genesis epoch quarantines like any other", func(t *testing.T) {
		enableDoppelGanger(t)
		v := doppelTestValidator(0) // current epoch is the genesis epoch
		// A duplicate can already be attesting within the genesis epoch, so keys
		// added mid-epoch-0 are quarantined like any other reload.
		v.trackReloadedKeysForDoppelGanger([][fieldparams.BLSPubkeyLength]byte{keyA})
		assert.Equal(t, true, v.isDoppelGangerPending(keyA))
	})

	t.Run("flag off is a no-op", func(t *testing.T) {
		setDoppelGangerFlag(t, false)
		v := doppelTestValidator(4)
		v.trackReloadedKeysForDoppelGanger([][fieldparams.BLSPubkeyLength]byte{keyB})
		assert.Equal(t, false, v.isDoppelGangerPending(keyB))
	})

	t.Run("removal forgets a blocked verdict", func(t *testing.T) {
		enableDoppelGanger(t)
		v := doppelTestValidator(6)
		v.doppelGanger.pending = map[pubkey]*doppelGangerPendingKey{keyA: {addedEpoch: 1, blocked: true}}
		v.doppelGanger.pendingCount.Store(1)

		// Removing the key is the operator's recovery path: the verdict is dropped,
		// and a re-add starts a fresh, unblocked quarantine on a new clock.
		v.trackReloadedKeysForDoppelGanger([][fieldparams.BLSPubkeyLength]byte{keyB})
		assert.Equal(t, false, v.isDoppelGangerPending(keyA))
		v.trackReloadedKeysForDoppelGanger([][fieldparams.BLSPubkeyLength]byte{keyA, keyB})
		assert.Equal(t, true, v.isDoppelGangerPending(keyA))
		v.doppelGanger.mu.RLock()
		assert.Equal(t, false, v.doppelGanger.pending[keyA].blocked)
		assert.Equal(t, 6, int(v.doppelGanger.pending[keyA].addedEpoch))
		v.doppelGanger.mu.RUnlock()
	})

	t.Run("boot keys are left for the startup check", func(t *testing.T) {
		enableDoppelGanger(t)
		v := doppelTestValidator(4)
		v.doppelGanger.beginStartup([]pubkey{keyA}, 4)

		// While the boot snapshot is active, only keys outside it quarantine;
		// the snapshot's keys belong to the one-shot startup check.
		v.trackReloadedKeysForDoppelGanger([][fieldparams.BLSPubkeyLength]byte{keyA, keyB})
		assert.Equal(t, false, v.isDoppelGangerPending(keyA))
		assert.Equal(t, true, v.isDoppelGangerPending(keyB))

		// After the startup check completes, boot membership no longer matters.
		v.doppelGanger.completeStartup()
		v.trackReloadedKeysForDoppelGanger([][fieldparams.BLSPubkeyLength]byte{keyA, keyB})
		assert.Equal(t, true, v.isDoppelGangerPending(keyA))
	})

	t.Run("re-import keeps the quarantine clock", func(t *testing.T) {
		enableDoppelGanger(t)
		v := doppelTestValidator(6)
		v.doppelGanger.pending = map[pubkey]*doppelGangerPendingKey{keyA: {addedEpoch: 2}}
		v.doppelGanger.pendingCount.Store(1)

		// Re-importing a still-quarantined key must not reset its wait.
		v.trackReloadedKeysForDoppelGanger([][fieldparams.BLSPubkeyLength]byte{keyA})
		v.doppelGanger.mu.RLock()
		assert.Equal(t, 2, int(v.doppelGanger.pending[keyA].addedEpoch))
		v.doppelGanger.mu.RUnlock()
	})
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

// pendingDoppelValidator returns a poll-ready validator with the given keys
// quarantined at addedEpoch and a slashing DB covering them.
func pendingDoppelValidator(t *testing.T, client *validatormock.MockValidatorClient, addedEpoch primitives.Epoch, keys ...pubkey) *validator {
	v := doppelTestValidator(4)
	v.validatorClient = client
	v.db = dbTest.SetupDB(t, t.TempDir(), keys, false)
	v.doppelGanger.pending = make(map[pubkey]*doppelGangerPendingKey, len(keys))
	for _, pk := range keys {
		v.doppelGanger.pending[pk] = &doppelGangerPendingKey{addedEpoch: addedEpoch}
	}
	v.doppelGanger.pendingCount.Store(int64(len(keys)))
	return v
}

func TestCheckDoppelGangerMidEpoch(t *testing.T) {
	keyA := bytesutil.ToBytes48([]byte{0xaa})
	keyB := bytesutil.ToBytes48([]byte{0xbb})

	t.Run("clears clean keys and blocks duplicates", func(t *testing.T) {
		enableDoppelGanger(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := validatormock.NewMockValidatorClient(ctrl)
		v := pendingDoppelValidator(t, client, 1, keyA, keyB)
		mockSyncedChainHead(ctrl, v, 4)

		client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(&ethpb.DoppelGangerResponse{
			Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{
				{PublicKey: keyA[:], DuplicateExists: false},
				{PublicKey: keyB[:], DuplicateExists: true},
			},
		}, nil)

		v.CheckDoppelGangerMidEpoch(t.Context(), slots.CurrentSlot(v.genesisTime))
		waitForDoppelCheck(t, v)

		// Clean key cleared; duplicate stays excluded and is never polled again.
		assert.Equal(t, false, v.isDoppelGangerPending(keyA))
		assert.Equal(t, true, v.isDoppelGangerPending(keyB))
		assert.Equal(t, 0, len(v.doppelGanger.pollDue(100)))
	})

	t.Run("early poll does not clear before the quarantine elapses", func(t *testing.T) {
		enableDoppelGanger(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := validatormock.NewMockValidatorClient(ctrl)
		// Added this epoch: polled immediately, but a clean result must NOT clear.
		v := pendingDoppelValidator(t, client, 4, keyA)
		mockSyncedChainHead(ctrl, v, 4)

		client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(&ethpb.DoppelGangerResponse{
			Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{
				{PublicKey: keyA[:], DuplicateExists: false},
			},
		}, nil)

		slot := slots.CurrentSlot(v.genesisTime)
		v.CheckDoppelGangerMidEpoch(t.Context(), slot)
		waitForDoppelCheck(t, v)
		assert.Equal(t, true, v.isDoppelGangerPending(keyA)) // clean but not elapsed: still quarantined

		// Same epoch: already polled, no second RPC (the single EXPECT enforces it).
		v.CheckDoppelGangerMidEpoch(t.Context(), slot)
		assert.Equal(t, false, v.doppelGanger.inFlight.Load())
	})

	t.Run("early poll blocks a duplicate immediately", func(t *testing.T) {
		enableDoppelGanger(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := validatormock.NewMockValidatorClient(ctrl)
		v := pendingDoppelValidator(t, client, 4, keyA)

		client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(&ethpb.DoppelGangerResponse{
			Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{
				{PublicKey: keyA[:], DuplicateExists: true},
			},
		}, nil)

		v.CheckDoppelGangerMidEpoch(t.Context(), slots.CurrentSlot(v.genesisTime))
		waitForDoppelCheck(t, v)

		// Duplicate is blocked at the first poll, well before the quarantine ends.
		assert.Equal(t, true, v.isDoppelGangerPending(keyA))
		assert.Equal(t, 0, len(v.doppelGanger.pollDue(100)))
	})

	t.Run("failure retries in the same epoch", func(t *testing.T) {
		enableDoppelGanger(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := validatormock.NewMockValidatorClient(ctrl)
		v := pendingDoppelValidator(t, client, 1, keyA)
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
		v.CheckDoppelGangerMidEpoch(t.Context(), slot)
		waitForDoppelCheck(t, v)
		assert.Equal(t, true, v.isDoppelGangerPending(keyA)) // failure: still quarantined

		v.CheckDoppelGangerMidEpoch(t.Context(), slot) // same epoch retry succeeds
		waitForDoppelCheck(t, v)
		assert.Equal(t, false, v.isDoppelGangerPending(keyA))
	})

	t.Run("partial response keeps absent keys quarantined", func(t *testing.T) {
		enableDoppelGanger(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := validatormock.NewMockValidatorClient(ctrl)
		v := pendingDoppelValidator(t, client, 1, keyA, keyB)
		mockSyncedChainHead(ctrl, v, 4)

		// Response omits keyB entirely: only the explicitly-clean keyA may clear.
		client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(&ethpb.DoppelGangerResponse{
			Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{{PublicKey: keyA[:], DuplicateExists: false}},
		}, nil)

		v.CheckDoppelGangerMidEpoch(t.Context(), slots.CurrentSlot(v.genesisTime))
		waitForDoppelCheck(t, v)
		assert.Equal(t, false, v.isDoppelGangerPending(keyA))
		assert.Equal(t, true, v.isDoppelGangerPending(keyB)) // absent from response: fail-closed
	})

	t.Run("single flight and flag off short-circuit", func(t *testing.T) {
		enableDoppelGanger(t)
		// No mock client: any RPC would panic, proving the guards short-circuit.
		v := doppelTestValidator(4)
		v.doppelGanger.pending = map[pubkey]*doppelGangerPendingKey{keyA: {addedEpoch: 1}}
		v.doppelGanger.pendingCount.Store(1)

		v.doppelGanger.inFlight.Store(true) // a check is already running
		v.CheckDoppelGangerMidEpoch(t.Context(), slots.CurrentSlot(v.genesisTime))
		v.doppelGanger.inFlight.Store(false)

		setDoppelGangerFlag(t, false)
		v.CheckDoppelGangerMidEpoch(t.Context(), slots.CurrentSlot(v.genesisTime)) // flag off
		assert.Equal(t, true, v.isDoppelGangerPending(keyA))
	})

	t.Run("skips slots before the poll point", func(t *testing.T) {
		enableDoppelGanger(t)
		// No mock client: an RPC would panic, proving the poll gate short-circuits.
		v := doppelTestValidator(4)
		v.doppelGanger.pending = map[pubkey]*doppelGangerPendingKey{keyA: {addedEpoch: 1}}
		v.doppelGanger.pendingCount.Store(1)

		// Slot 2 of epoch 4: before the poll point, no check may fire.
		earlySlot := primitives.Slot(4*uint64(params.BeaconConfig().SlotsPerEpoch) + 2)
		v.CheckDoppelGangerMidEpoch(t.Context(), earlySlot)
		assert.Equal(t, false, v.doppelGanger.inFlight.Load())
		assert.Equal(t, true, v.isDoppelGangerPending(keyA))
	})

	t.Run("lagging head defers clearing", func(t *testing.T) {
		enableDoppelGanger(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := validatormock.NewMockValidatorClient(ctrl)
		v := pendingDoppelValidator(t, client, 1, keyA)
		// Head stalled inside the wait: its clean answers are unevaluated defaults.
		mockSyncedChainHead(ctrl, v, 3)

		client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(&ethpb.DoppelGangerResponse{
			Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{{PublicKey: keyA[:], DuplicateExists: false}},
		}, nil)

		v.CheckDoppelGangerMidEpoch(t.Context(), slots.CurrentSlot(v.genesisTime))
		waitForDoppelCheck(t, v)
		assert.Equal(t, true, v.isDoppelGangerPending(keyA)) // clear deferred until the head catches up
	})

	t.Run("head error defers clearing and the poll", func(t *testing.T) {
		enableDoppelGanger(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := validatormock.NewMockValidatorClient(ctrl)
		v := pendingDoppelValidator(t, client, 1, keyA)
		chain := validatormock.NewMockChainClient(ctrl)
		chain.EXPECT().ChainHead(gomock.Any(), gomock.Any()).Return(nil, errors.New("bn down"))
		v.chainClient = chain

		client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(&ethpb.DoppelGangerResponse{
			Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{{PublicKey: keyA[:], DuplicateExists: false}},
		}, nil)

		v.CheckDoppelGangerMidEpoch(t.Context(), slots.CurrentSlot(v.genesisTime))
		waitForDoppelCheck(t, v)
		assert.Equal(t, true, v.isDoppelGangerPending(keyA)) // unknown head: fail-closed, no clear
		// The poll is left unconsumed so the next slot in this epoch retries.
		epoch := slots.ToEpoch(slots.CurrentSlot(v.genesisTime))
		assert.Equal(t, 1, len(v.doppelGanger.pollDue(epoch)))
	})

	t.Run("key with imported history is checked from its import epoch", func(t *testing.T) {
		tests := []struct {
			name        string
			recordEpoch primitives.Epoch
			addedEpoch  primitives.Epoch
			wantCleared bool
		}{
			// A record newer than the import would defer evaluation past the quarantine.
			{"record newer than the import", 9, 1, true},
			// A record older than the import would probe pre-import epochs; this key
			// also stays pending because its wait has not elapsed at head epoch 4.
			{"record older than the import", 1, 3, false},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				enableDoppelGanger(t)
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()
				client := validatormock.NewMockValidatorClient(ctrl)
				v := pendingDoppelValidator(t, client, tc.addedEpoch, keyA)
				mockSyncedChainHead(ctrl, v, 4)
				att := &ethpb.IndexedAttestation{Data: &ethpb.AttestationData{
					BeaconBlockRoot: make([]byte, 32),
					Source:          &ethpb.Checkpoint{Epoch: tc.recordEpoch - 1, Root: make([]byte, 32)},
					Target:          &ethpb.Checkpoint{Epoch: tc.recordEpoch, Root: make([]byte, 32)},
				}}
				require.NoError(t, v.db.SaveAttestationsForPubKey(t.Context(), keyA, [][]byte{make([]byte, 32)}, []*ethpb.IndexedAttestation{att}))

				client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, req *ethpb.DoppelGangerRequest) (*ethpb.DoppelGangerResponse, error) {
						require.Equal(t, 1, len(req.ValidatorRequests))
						assert.Equal(t, tc.addedEpoch, req.ValidatorRequests[0].Epoch, "request must carry the import epoch")
						return &ethpb.DoppelGangerResponse{Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{
							{PublicKey: keyA[:], DuplicateExists: false},
						}}, nil
					})

				v.CheckDoppelGangerMidEpoch(t.Context(), slots.CurrentSlot(v.genesisTime))
				waitForDoppelCheck(t, v)
				assert.Equal(t, !tc.wantCleared, v.isDoppelGangerPending(keyA))
			})
		}
	})

	t.Run("key with no attestation history is checked from its import epoch", func(t *testing.T) {
		enableDoppelGanger(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := validatormock.NewMockValidatorClient(ctrl)
		// keyA was imported at epoch 1 and has an empty slashing-protection DB,
		// as a key migrated from another client would.
		v := pendingDoppelValidator(t, client, 1, keyA)
		mockSyncedChainHead(ctrl, v, 4)

		// Requesting epoch 0 would have the node scan epochs before the import,
		// where the previous client's last attestations register as a duplicate.
		client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, req *ethpb.DoppelGangerRequest) (*ethpb.DoppelGangerResponse, error) {
				require.Equal(t, 1, len(req.ValidatorRequests))
				assert.Equal(t, primitives.Epoch(1), req.ValidatorRequests[0].Epoch, "request must carry the import epoch")
				return &ethpb.DoppelGangerResponse{Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{
					{PublicKey: keyA[:], DuplicateExists: false},
				}}, nil
			})

		v.CheckDoppelGangerMidEpoch(t.Context(), slots.CurrentSlot(v.genesisTime))
		waitForDoppelCheck(t, v)
		assert.Equal(t, false, v.isDoppelGangerPending(keyA))
	})

	t.Run("empty response counts the poll", func(t *testing.T) {
		enableDoppelGanger(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := validatormock.NewMockValidatorClient(ctrl)
		v := pendingDoppelValidator(t, client, 1, keyA)

		// Key unknown to the node: a definitive empty answer, not a failure. The
		// single EXPECT proves the same epoch does not re-poll every slot.
		client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(&ethpb.DoppelGangerResponse{
			Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{},
		}, nil)

		slot := slots.CurrentSlot(v.genesisTime)
		v.CheckDoppelGangerMidEpoch(t.Context(), slot)
		waitForDoppelCheck(t, v)
		assert.Equal(t, true, v.isDoppelGangerPending(keyA)) // stays quarantined

		v.CheckDoppelGangerMidEpoch(t.Context(), slot) // same epoch: no second RPC
		assert.Equal(t, false, v.doppelGanger.inFlight.Load())
	})
}

func TestUpdateDutiesSplit_AllKeysQuarantinedYieldsNoRoles(t *testing.T) {
	enableDoppelGanger(t)
	v := doppelTestValidator(4)
	v.duties = &dutyStore{}
	v.aggSelector = &distributedSelector{}
	keyA := bytesutil.ToBytes48([]byte{0xaa})
	keyB := bytesutil.ToBytes48([]byte{0xbb})
	v.pubkeyToStatus = map[pubkey]*validatorStatus{
		keyA: {publicKey: keyA[:], status: &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE}, index: 1},
		keyB: {publicKey: keyB[:], status: &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE}, index: 2},
	}
	v.trackReloadedKeysForDoppelGanger([][fieldparams.BLSPubkeyLength]byte{keyA, keyB})

	// Every key quarantined: the duty update stores an empty set and per-slot
	// role lookups stay quiet for the whole quarantine window.
	keys, indices := v.filteredKeysAndIndices([][fieldparams.BLSPubkeyLength]byte{keyA, keyB}, 4)
	require.Equal(t, 0, len(keys))
	require.NoError(t, v.updateDutiesSplit(t.Context(), 4, indices))

	roles, err := v.RolesAt(t.Context(), primitives.Slot(4*uint64(params.BeaconConfig().SlotsPerEpoch)))
	require.NoError(t, err)
	assert.Equal(t, 0, len(roles))
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

func TestDoppelGangerTracker(t *testing.T) {
	t.Run("block ignores untracked keys", func(t *testing.T) {
		d := &doppelGangerTracker{}
		keyA := bytesutil.ToBytes48([]byte{0x01})
		// A key removed mid-check must not be resurrected or logged as excluded.
		d.block([][fieldparams.BLSPubkeyLength]byte{keyA})
		d.mu.RLock()
		assert.Equal(t, 0, len(d.pending))
		d.mu.RUnlock()
	})

	t.Run("clearElapsed skips blocked and unelapsed keys", func(t *testing.T) {
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
	})

	t.Run("vetStartup only quarantines keys with no existing state", func(t *testing.T) {
		d := &doppelGangerTracker{}
		checked := bytesutil.ToBytes48([]byte{0x01})
		alreadyPending := bytesutil.ToBytes48([]byte{0x02})
		fresh := bytesutil.ToBytes48([]byte{0x03})
		d.checked = map[pubkey]bool{checked: true}
		d.pending = map[pubkey]*doppelGangerPendingKey{alreadyPending: {addedEpoch: 2, blocked: true}}
		d.pendingCount.Store(1)

		all := [][fieldparams.BLSPubkeyLength]byte{checked, alreadyPending, fresh}
		assert.Equal(t, 1, d.vetStartup(all, nil, 9)) // nothing evaluated: only the fresh key is newly held
		assert.Equal(t, false, d.isPending(checked))
		assert.Equal(t, true, d.isPending(fresh))
		d.mu.RLock()
		// The existing entry keeps its clock and verdict.
		assert.Equal(t, 2, int(d.pending[alreadyPending].addedEpoch))
		assert.Equal(t, true, d.pending[alreadyPending].blocked)
		assert.Equal(t, 9, int(d.pending[fresh].addedEpoch))
		d.mu.RUnlock()

		// A rerun holds nothing new, so callers stay quiet on restarts.
		assert.Equal(t, 0, d.vetStartup(all, nil, 10))
	})

	t.Run("vetStartup never releases a blocked key", func(t *testing.T) {
		d := &doppelGangerTracker{}
		blocked := bytesutil.ToBytes48([]byte{0x01})
		clean := bytesutil.ToBytes48([]byte{0x02})
		d.pending = map[pubkey]*doppelGangerPendingKey{
			blocked: {addedEpoch: 1, blocked: true},
			clean:   {addedEpoch: 1},
		}
		d.pendingCount.Store(2)

		// A rerun of the startup check must not override a duplicate verdict.
		keys := [][fieldparams.BLSPubkeyLength]byte{blocked, clean}
		assert.Equal(t, 0, d.vetStartup(keys, map[pubkey]bool{blocked: true, clean: true}, 5))
		assert.Equal(t, true, d.isPending(blocked))
		assert.Equal(t, false, d.isPending(clean))
	})

	t.Run("polls again next epoch", func(t *testing.T) {
		d := &doppelGangerTracker{}
		keyA := bytesutil.ToBytes48([]byte{0x01})
		d.pending = map[pubkey]*doppelGangerPendingKey{keyA: {addedEpoch: 4}}
		d.pendingCount.Store(1)

		require.Equal(t, 1, len(d.pollDue(4)))
		d.markPolled(4)
		assert.Equal(t, 0, len(d.pollDue(4))) // consumed for this epoch
		assert.Equal(t, 1, len(d.pollDue(5))) // next epoch polls again
	})

	t.Run("epoch zero is pollable", func(t *testing.T) {
		d := &doppelGangerTracker{}
		keyA := bytesutil.ToBytes48([]byte{0x01})
		d.pending = map[pubkey]*doppelGangerPendingKey{keyA: {addedEpoch: 0}}
		d.pendingCount.Store(1)

		require.Equal(t, 1, len(d.pollDue(0))) // the never-polled zero value must not mask epoch 0
		d.markPolled(0)
		assert.Equal(t, 0, len(d.pollDue(0)))
		assert.Equal(t, 1, len(d.pollDue(1)))
	})
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

// startupDoppelValidator returns a validator with a fresh keymanager, its keys,
// and a slashing DB, wired to the given mock client for startup checks.
func startupDoppelValidator(t *testing.T, client *validatormock.MockValidatorClient, numKeys int) (*validator, [][fieldparams.BLSPubkeyLength]byte) {
	km := genMockKeymanager(t, numKeys)
	keys, err := km.FetchValidatingPublicKeys(t.Context())
	require.NoError(t, err)
	v := doppelTestValidator(4)
	v.validatorClient = client
	v.km = km
	v.db = dbTest.SetupDB(t, t.TempDir(), keys, false)
	return v, keys
}

func TestCheckDoppelGangerAtStartup(t *testing.T) {
	t.Run("holds unknown keys pending instead of failing startup", func(t *testing.T) {
		enableDoppelGanger(t)
		hook := logTest.NewGlobal()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := validatormock.NewMockValidatorClient(ctrl)
		v, keys := startupDoppelValidator(t, client, 2)

		// Keys with no validator index yet: the node has nothing to evaluate, so
		// startup proceeds and the per-epoch polls own the keys until it can.
		client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(&ethpb.DoppelGangerResponse{
			Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{},
		}, nil)

		require.NoError(t, v.CheckDoppelGangerAtStartup(t.Context()))
		for _, k := range keys {
			assert.Equal(t, true, v.isDoppelGangerPending(k))
		}
		// Nothing evaluable at all means the validator will sit idle: warn loudly.
		require.LogsContain(t, hook, "could not evaluate any validating keys")
	})

	t.Run("keys imported during initialization are held, boot keys vetted", func(t *testing.T) {
		enableDoppelGanger(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := validatormock.NewMockValidatorClient(ctrl)
		v, keys := startupDoppelValidator(t, client, 3)
		// Boot snapshot covers keys[0] and keys[1]; keys[2] arrived mid-boot and
		// must be quarantined, not one-shot vetted with a zero watermark.
		v.doppelGanger.beginStartup(keys[:2], 4)

		client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, req *ethpb.DoppelGangerRequest) (*ethpb.DoppelGangerResponse, error) {
				require.Equal(t, 2, len(req.ValidatorRequests)) // the late add is never asked about
				return &ethpb.DoppelGangerResponse{Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{
					{PublicKey: keys[0][:], DuplicateExists: false},
					{PublicKey: keys[1][:], DuplicateExists: false},
				}}, nil
			})

		require.NoError(t, v.CheckDoppelGangerAtStartup(t.Context()))
		assert.Equal(t, false, v.isDoppelGangerPending(keys[0]))
		assert.Equal(t, false, v.isDoppelGangerPending(keys[1]))
		assert.Equal(t, true, v.isDoppelGangerPending(keys[2]))
		// The snapshot is consumed: a later reload treats boot keys normally.
		v.doppelGanger.mu.RLock()
		assert.Equal(t, true, v.doppelGanger.bootSet == nil)
		v.doppelGanger.mu.RUnlock()
	})

	t.Run("mixed set holds only the unevaluated keys", func(t *testing.T) {
		enableDoppelGanger(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := validatormock.NewMockValidatorClient(ctrl)
		v, keys := startupDoppelValidator(t, client, 2)

		// The node resolves keys[0] and omits keys[1]: only the evaluated key is
		// vetted; the omitted one waits for the polls.
		client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(&ethpb.DoppelGangerResponse{
			Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{
				{PublicKey: keys[0][:], DuplicateExists: false},
			},
		}, nil)

		require.NoError(t, v.CheckDoppelGangerAtStartup(t.Context()))
		assert.Equal(t, false, v.isDoppelGangerPending(keys[0]))
		assert.Equal(t, true, v.isDoppelGangerPending(keys[1]))
	})

	t.Run("marks keys checked so a reload does not quarantine them", func(t *testing.T) {
		enableDoppelGanger(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := validatormock.NewMockValidatorClient(ctrl)
		v, keys := startupDoppelValidator(t, client, 3)

		resp := &ethpb.DoppelGangerResponse{}
		for _, k := range keys {
			resp.Responses = append(resp.Responses, &ethpb.DoppelGangerResponse_ValidatorResponse{PublicKey: k[:], DuplicateExists: false})
		}
		client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(resp, nil)

		require.NoError(t, v.CheckDoppelGangerAtStartup(t.Context()))

		v.trackReloadedKeysForDoppelGanger(keys)
		for _, k := range keys {
			assert.Equal(t, false, v.isDoppelGangerPending(k))
		}
	})

	// A beacon-node health flap restarts the runner in-process, which reruns
	// this startup check while the tracker still holds quarantined keys.
	t.Run("rerun skips poll-owned keys and cannot release them", func(t *testing.T) {
		enableDoppelGanger(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := validatormock.NewMockValidatorClient(ctrl)
		v, keys := startupDoppelValidator(t, client, 3)
		// keys[0] was confirmed a duplicate, keys[1] is mid-quarantine; only the
		// untracked keys[2] belongs to the rerun.
		v.doppelGanger.pending = map[pubkey]*doppelGangerPendingKey{
			keys[0]: {addedEpoch: 1, blocked: true},
			keys[1]: {addedEpoch: 1},
		}
		v.doppelGanger.pendingCount.Store(2)

		client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, req *ethpb.DoppelGangerRequest) (*ethpb.DoppelGangerResponse, error) {
				// Quarantined keys must not even be asked about: an all-clean
				// answer could otherwise shortcut their wait or verdict.
				require.Equal(t, 1, len(req.ValidatorRequests))
				require.DeepEqual(t, keys[2][:], req.ValidatorRequests[0].PublicKey)
				return &ethpb.DoppelGangerResponse{Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{
					{PublicKey: keys[2][:], DuplicateExists: false},
				}}, nil
			})

		require.NoError(t, v.CheckDoppelGangerAtStartup(t.Context()))
		assert.Equal(t, true, v.isDoppelGangerPending(keys[0]))
		assert.Equal(t, true, v.isDoppelGangerPending(keys[1]))
		assert.Equal(t, false, v.isDoppelGangerPending(keys[2]))
	})

	t.Run("a failed check keeps the boot snapshot for the retry", func(t *testing.T) {
		enableDoppelGanger(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := validatormock.NewMockValidatorClient(ctrl)
		v, keys := startupDoppelValidator(t, client, 2)
		v.doppelGanger.beginStartup(keys[:1], 4) // keys[1] arrived mid-boot

		gomock.InOrder(
			client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(nil, errors.New("bn down")),
			client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(&ethpb.DoppelGangerResponse{
				Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{{PublicKey: keys[0][:], DuplicateExists: false}},
			}, nil),
		)

		// The failed attempt must not close the snapshot: the initialize retry
		// has to re-vet the same boot set, not a refreshed one.
		require.ErrorContains(t, "doppelganger check request to beacon node failed", v.CheckDoppelGangerAtStartup(t.Context()))
		v.doppelGanger.mu.RLock()
		require.Equal(t, false, v.doppelGanger.bootSet == nil)
		v.doppelGanger.mu.RUnlock()

		require.NoError(t, v.CheckDoppelGangerAtStartup(t.Context()))
		assert.Equal(t, false, v.isDoppelGangerPending(keys[0])) // boot key vetted
		assert.Equal(t, true, v.isDoppelGangerPending(keys[1]))  // mid-boot import held
		v.doppelGanger.mu.RLock()
		assert.Equal(t, true, v.doppelGanger.bootSet == nil) // success closes the snapshot
		v.doppelGanger.mu.RUnlock()
	})

	// A health flap restarts the runner and re-runs initialize on the same tracker;
	// a fresh boot snapshot must not admit outage-imported keys to the one-shot check.
	t.Run("key imported while the runner was down is quarantined", func(t *testing.T) {
		enableDoppelGanger(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := validatormock.NewMockValidatorClient(ctrl)
		v, keys := startupDoppelValidator(t, client, 2)

		// First boot: the wallet held only keys[0], and the startup check cleared it.
		v.doppelGanger.beginStartup(keys[:1], 2)
		v.markDoppelGangerChecked(keys[:1])
		v.doppelGanger.completeStartup()

		// Runner restart: keys[1] was imported during the outage, so the wallet
		// the new keymanager reads now contains both.
		v.doppelGanger.beginStartup(keys, 4)

		client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, req *ethpb.DoppelGangerRequest) (*ethpb.DoppelGangerResponse, error) {
				// Only the original boot key belongs to the one-shot check.
				require.Equal(t, 1, len(req.ValidatorRequests))
				require.DeepEqual(t, keys[0][:], req.ValidatorRequests[0].PublicKey)
				return &ethpb.DoppelGangerResponse{Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{
					{PublicKey: keys[0][:], DuplicateExists: false},
				}}, nil
			})

		require.NoError(t, v.CheckDoppelGangerAtStartup(t.Context()))
		assert.Equal(t, false, v.isDoppelGangerPending(keys[0]))
		assert.Equal(t, true, v.isDoppelGangerPending(keys[1]))
	})

	t.Run("blocked key re-imported while the runner was down stays blocked", func(t *testing.T) {
		enableDoppelGanger(t)
		v := doppelTestValidator(4)
		keyA := bytesutil.ToBytes48([]byte{0xaa})
		// Startup completed on an earlier boot; keyA was later blocked as a duplicate.
		v.doppelGanger.completeStartup()
		v.doppelGanger.pending = map[pubkey]*doppelGangerPendingKey{keyA: {addedEpoch: 1, blocked: true}}
		v.doppelGanger.pendingCount.Store(1)

		// Runner restart: the wallet still contains keyA, so the reload cannot
		// tell a remove+re-import apart from the key having been there all along.
		v.doppelGanger.beginStartup([]pubkey{keyA}, 4)

		assert.Equal(t, true, v.isDoppelGangerPending(keyA))
		v.doppelGanger.mu.RLock()
		assert.Equal(t, true, v.doppelGanger.pending[keyA].blocked)
		assert.Equal(t, 1, int(v.doppelGanger.pending[keyA].addedEpoch))
		v.doppelGanger.mu.RUnlock()
	})

	t.Run("rerun fail-stops when a checked key turns duplicate", func(t *testing.T) {
		enableDoppelGanger(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := validatormock.NewMockValidatorClient(ctrl)
		v, keys := startupDoppelValidator(t, client, 2)
		// A backup VC came alive during the outage: the reconnect re-check of
		// already-vetted keys is what catches it.
		v.markDoppelGangerChecked(keys)

		client.EXPECT().CheckDoppelGanger(gomock.Any(), gomock.Any()).Return(&ethpb.DoppelGangerResponse{
			Responses: []*ethpb.DoppelGangerResponse_ValidatorResponse{
				{PublicKey: keys[0][:], DuplicateExists: true},
				{PublicKey: keys[1][:], DuplicateExists: false},
			},
		}, nil)

		require.ErrorContains(t, "Duplicate instances exists", v.CheckDoppelGangerAtStartup(t.Context()))
	})
}
