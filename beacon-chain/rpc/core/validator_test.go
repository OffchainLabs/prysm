package core

import (
	"encoding/binary"
	"sync"
	"testing"
	"time"

	mockChain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	p2pmock "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	mockstategen "github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen/mock"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/consensus-types/validator"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestRegisterSyncSubnetProto(t *testing.T) {
	k := pubKey(3)
	committee := make([][]byte, 0)

	for i := range 100 {
		committee = append(committee, pubKey(uint64(i)))
	}
	sCommittee := &ethpb.SyncCommittee{
		Pubkeys: committee,
	}
	registerSyncSubnetProto(0, 0, k, sCommittee, ethpb.ValidatorStatus_ACTIVE)
	coms, _, ok, exp := cache.SyncSubnetIDs.GetSyncCommitteeSubnets(k, 0)
	require.Equal(t, true, ok, "No cache entry found for validator")
	assert.Equal(t, uint64(1), uint64(len(coms)))
	epochDuration := time.Duration(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))
	totalTime := time.Duration(params.BeaconConfig().EpochsPerSyncCommitteePeriod) * epochDuration * time.Second
	receivedTime := time.Until(exp.Round(time.Second)).Round(time.Second)
	if receivedTime < totalTime {
		t.Fatalf("Expiration time of %f was less than expected duration of %f ", receivedTime.Seconds(), totalTime.Seconds())
	}
}

func TestRegisterSyncSubnet(t *testing.T) {
	k := pubKey(3)
	committee := make([][]byte, 0)

	for i := range 100 {
		committee = append(committee, pubKey(uint64(i)))
	}
	sCommittee := &ethpb.SyncCommittee{
		Pubkeys: committee,
	}
	registerSyncSubnet(0, 0, k, sCommittee, validator.Active)
	coms, _, ok, exp := cache.SyncSubnetIDs.GetSyncCommitteeSubnets(k, 0)
	require.Equal(t, true, ok, "No cache entry found for validator")
	assert.Equal(t, uint64(1), uint64(len(coms)))
	epochDuration := time.Duration(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))
	totalTime := time.Duration(params.BeaconConfig().EpochsPerSyncCommitteePeriod) * epochDuration * time.Second
	receivedTime := time.Until(exp.Round(time.Second)).Round(time.Second)
	if receivedTime < totalTime {
		t.Fatalf("Expiration time of %f was less than expected duration of %f ", receivedTime.Seconds(), totalTime.Seconds())
	}
}

// pubKey is a helper to generate a well-formed public key.
func pubKey(i uint64) []byte {
	pubKey := make([]byte, params.BeaconConfig().BLSPubkeyLength)
	binary.LittleEndian.PutUint64(pubKey, i)
	return pubKey
}

func TestService_SubmitSignedAggregateSelectionProof(t *testing.T) {
	slot := primitives.Slot(0)
	mock := &mockChain.ChainService{Slot: &slot, Genesis: time.Now().Add(-75 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)}
	s := &Service{GenesisTimeFetcher: mock}
	var err error
	t.Run("Happy path electra", func(t *testing.T) {
		slot, err = slots.EpochEnd(params.BeaconConfig().ElectraForkEpoch)
		require.NoError(t, err)
		broadcaster := &p2pmock.MockBroadcaster{}
		s.Broadcaster = broadcaster
		fakeSig, err := hexutil.Decode("0x1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505cc411d61252fb6cb3fa0017b679f8bb2305b26a285fa2737f175668d0dff91cc1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505")
		require.NoError(t, err)
		agg := &ethpb.SignedAggregateAttestationAndProofElectra{
			Message: &ethpb.AggregateAttestationAndProofElectra{
				AggregatorIndex: 72,
				Aggregate: &ethpb.AttestationElectra{
					AggregationBits: make([]byte, 4),
					Data: &ethpb.AttestationData{
						Slot:            75,
						CommitteeIndex:  76,
						BeaconBlockRoot: make([]byte, 32),
						Source: &ethpb.Checkpoint{
							Epoch: 78,
							Root:  make([]byte, 32),
						},
						Target: &ethpb.Checkpoint{
							Epoch: 80,
							Root:  make([]byte, 32),
						},
					},
					Signature:     fakeSig,
					CommitteeBits: make([]byte, 8),
				},
				SelectionProof: fakeSig,
			},
			Signature: fakeSig,
		}
		rpcError := s.SubmitSignedAggregateSelectionProof(t.Context(), agg)
		t.Log(rpcError)
		assert.Equal(t, true, rpcError == nil)
	})

	t.Run("Phase 0 post electra", func(t *testing.T) {
		slot, err = slots.EpochEnd(params.BeaconConfig().ElectraForkEpoch)
		require.NoError(t, err)
		agg := &ethpb.SignedAggregateAttestationAndProof{
			Message: &ethpb.AggregateAttestationAndProof{
				Aggregate: &ethpb.Attestation{
					Data: &ethpb.AttestationData{},
				},
			},
			Signature: make([]byte, 96),
		}
		rpcError := s.SubmitSignedAggregateSelectionProof(t.Context(), agg)
		assert.ErrorContains(t, "old aggregate and proof", rpcError.Err)
	})

	t.Run("electra agg pre electra", func(t *testing.T) {
		slot = primitives.Slot(0)
		agg := &ethpb.SignedAggregateAttestationAndProofElectra{
			Message: &ethpb.AggregateAttestationAndProofElectra{
				Aggregate: &ethpb.AttestationElectra{
					Data: &ethpb.AttestationData{},
				},
			},
			Signature: make([]byte, 96),
		}
		rpcError := s.SubmitSignedAggregateSelectionProof(t.Context(), agg)
		assert.ErrorContains(t, "electra aggregate and proof not supported yet", rpcError.Err)
	})
}

func TestPayloadAttestationData(t *testing.T) {
	t.Run("pre-gloas → BadRequest", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 100
		params.OverrideBeaconConfig(cfg)

		slot := primitives.Slot(0)
		chain := &mockChain.ChainService{Slot: &slot}
		s := &Service{GenesisTimeFetcher: chain, ForkchoiceFetcher: chain}

		_, rpcErr := s.PayloadAttestationData(t.Context(), slot)
		require.NotNil(t, rpcErr)
		assert.Equal(t, ErrorReason(BadRequest), rpcErr.Reason)
		assert.ErrorContains(t, "Gloas fork", rpcErr.Err)
	})
	t.Run("slot mismatch → BadRequest", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		current := primitives.Slot(5)
		chain := &mockChain.ChainService{Slot: &current}
		s := &Service{GenesisTimeFetcher: chain, ForkchoiceFetcher: chain}

		_, rpcErr := s.PayloadAttestationData(t.Context(), primitives.Slot(10))
		require.NotNil(t, rpcErr)
		assert.Equal(t, ErrorReason(BadRequest), rpcErr.Reason)
		assert.ErrorContains(t, "current slot", rpcErr.Err)
	})
	t.Run("no block received for slot → Unavailable", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		slot := primitives.Slot(5)
		timeChain := &mockChain.ChainService{Slot: &slot}
		fcChain := &mockChain.ChainService{BlockSlot: primitives.Slot(4)}
		s := &Service{GenesisTimeFetcher: timeChain, ForkchoiceFetcher: fcChain}

		_, rpcErr := s.PayloadAttestationData(t.Context(), slot)
		require.NotNil(t, rpcErr)
		assert.Equal(t, ErrorReason(Unavailable), rpcErr.Reason)
		assert.ErrorContains(t, "no valid block root for slot 5", rpcErr.Err)
	})
	t.Run("empty highest received root → Internal", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		slot := primitives.Slot(5)
		timeChain := &mockChain.ChainService{Slot: &slot}
		fcChain := &mockChain.ChainService{BlockSlot: slot}
		s := &Service{GenesisTimeFetcher: timeChain, ForkchoiceFetcher: fcChain}

		_, rpcErr := s.PayloadAttestationData(t.Context(), slot)
		require.NotNil(t, rpcErr)
		assert.Equal(t, ErrorReason(Internal), rpcErr.Reason)
		assert.ErrorContains(t, "could not retrieve highest received block root", rpcErr.Err)
	})
	t.Run("ok with payload absent", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		slot := primitives.Slot(5)
		root := bytesutil.PadTo([]byte("head-root"), 32)
		chain := &mockChain.ChainService{Slot: &slot, Root: root}
		s := &Service{GenesisTimeFetcher: chain, ForkchoiceFetcher: chain, HeadFetcher: chain}

		data, rpcErr := s.PayloadAttestationData(t.Context(), slot)
		require.IsNil(t, rpcErr)
		assert.DeepEqual(t, root, data.BeaconBlockRoot)
		assert.Equal(t, slot, data.Slot)
		assert.Equal(t, false, data.PayloadPresent)
		assert.Equal(t, false, data.BlobDataAvailable)
	})
	t.Run("ok with payload present", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		slot := primitives.Slot(5)
		root := bytesutil.PadTo([]byte("head-root"), 32)
		chain := &mockChain.ChainService{
			Slot:               &slot,
			Root:               root,
			MockCanonicalRoots: map[primitives.Slot][32]byte{slot: bytesutil.ToBytes32(root)},
			MockCanonicalFull:  map[primitives.Slot]bool{slot: true},
			MockPayloadEarly:   map[[32]byte]bool{bytesutil.ToBytes32(root): true},
		}
		s := &Service{GenesisTimeFetcher: chain, ForkchoiceFetcher: chain, HeadFetcher: chain}

		data, rpcErr := s.PayloadAttestationData(t.Context(), slot)
		require.IsNil(t, rpcErr)
		assert.DeepEqual(t, root, data.BeaconBlockRoot)
		assert.Equal(t, slot, data.Slot)
		assert.Equal(t, true, data.PayloadPresent)
		assert.Equal(t, true, data.BlobDataAvailable)
	})
	t.Run("before PTC deadline and not final → Unavailable", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		slot := primitives.Slot(0)
		chain := &mockChain.ChainService{
			Slot:    &slot,
			Genesis: time.Now(),
			Root:    bytesutil.PadTo([]byte{0xAA}, 32),
		}
		s := &Service{GenesisTimeFetcher: chain, ForkchoiceFetcher: chain, HeadFetcher: chain}

		_, rpcErr := s.PayloadAttestationData(t.Context(), slot)
		require.NotNil(t, rpcErr)
		assert.Equal(t, ErrorReason(Unavailable), rpcErr.Reason)
		assert.ErrorContains(t, "not yet final", rpcErr.Err)
		assert.Equal(t, (*ethpb.PayloadAttestationData)(nil), s.payloadAttestationData.Load())
	})
	t.Run("before PTC deadline but final → returns data early", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		slot := primitives.Slot(0)
		root := bytesutil.PadTo([]byte{0xAA}, 32)
		chain := &mockChain.ChainService{
			Slot:               &slot,
			Genesis:            time.Now(),
			Root:               root,
			MockCanonicalRoots: map[primitives.Slot][32]byte{slot: bytesutil.ToBytes32(root)},
			MockCanonicalFull:  map[primitives.Slot]bool{slot: true},
			MockPayloadEarly:   map[[32]byte]bool{bytesutil.ToBytes32(root): true},
		}
		s := &Service{GenesisTimeFetcher: chain, ForkchoiceFetcher: chain, HeadFetcher: chain}

		data, rpcErr := s.PayloadAttestationData(t.Context(), slot)
		require.IsNil(t, rpcErr)
		assert.DeepEqual(t, root, data.BeaconBlockRoot)
		assert.Equal(t, true, data.PayloadPresent)
		assert.Equal(t, true, data.BlobDataAvailable)
		require.NotNil(t, s.payloadAttestationData.Load())
	})
	t.Run("result is cached per slot and bypassed on slot change", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		slot := primitives.Slot(7)
		root := bytesutil.PadTo([]byte{0xAA}, 32)
		chain := &mockChain.ChainService{
			Slot:               &slot,
			Root:               root,
			MockCanonicalRoots: map[primitives.Slot][32]byte{slot: bytesutil.ToBytes32(root)},
			MockCanonicalFull:  map[primitives.Slot]bool{slot: false},
		}
		s := &Service{GenesisTimeFetcher: chain, ForkchoiceFetcher: chain, HeadFetcher: chain}

		first, rpcErr := s.PayloadAttestationData(t.Context(), slot)
		require.IsNil(t, rpcErr)
		require.DeepEqual(t, root, first.BeaconBlockRoot)

		// Mutate the underlying mock; same-slot call must hit the cache.
		newRoot := bytesutil.PadTo([]byte{0xBB}, 32)
		chain.Root = newRoot
		chain.MockCanonicalRoots[slot] = bytesutil.ToBytes32(newRoot)
		chain.MockCanonicalFull[slot] = true

		second, rpcErr := s.PayloadAttestationData(t.Context(), slot)
		require.IsNil(t, rpcErr)
		assert.Equal(t, true, first == second)
		require.DeepEqual(t, root, second.BeaconBlockRoot)

		// Advance to a new slot; cache must be bypassed.
		nextSlot := slot + 1
		chain.Slot = &nextSlot
		chain.BlockSlot = nextSlot
		chain.MockCanonicalRoots[nextSlot] = bytesutil.ToBytes32(newRoot)
		chain.MockCanonicalFull[nextSlot] = true
		chain.MockPayloadEarly = map[[32]byte]bool{bytesutil.ToBytes32(newRoot): true}

		third, rpcErr := s.PayloadAttestationData(t.Context(), nextSlot)
		require.IsNil(t, rpcErr)
		assert.Equal(t, false, first == third)
		require.DeepEqual(t, newRoot, third.BeaconBlockRoot)
		assert.Equal(t, nextSlot, third.Slot)
		assert.Equal(t, true, third.PayloadPresent)
	})
	t.Run("concurrent callers share a single computation", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		slot := primitives.Slot(7)
		root := bytesutil.PadTo([]byte{0xAA}, 32)
		chain := &mockChain.ChainService{
			Slot:               &slot,
			Root:               root,
			MockCanonicalRoots: map[primitives.Slot][32]byte{slot: bytesutil.ToBytes32(root)},
			MockCanonicalFull:  map[primitives.Slot]bool{slot: false},
		}
		s := &Service{GenesisTimeFetcher: chain, ForkchoiceFetcher: chain, HeadFetcher: chain}

		const callers = 16
		results := make([]*ethpb.PayloadAttestationData, callers)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range callers {
			wg.Go(func() {
				<-start
				resp, rpcErr := s.PayloadAttestationData(t.Context(), slot)
				require.IsNil(t, rpcErr)
				results[i] = resp
			})
		}
		close(start)
		wg.Wait()

		for i := 1; i < callers; i++ {
			assert.Equal(t, true, results[0] == results[i])
		}
	})
	t.Run("non-canonical shuffling → Unavailable", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		slot := primitives.Slot(5)
		root := bytesutil.PadTo([]byte("head-root"), 32)
		chain := &mockChain.ChainService{
			Slot:               &slot,
			Root:               root,
			MockCanonicalRoots: map[primitives.Slot][32]byte{slot: bytesutil.ToBytes32(root)},
			MockCanonicalFull:  map[primitives.Slot]bool{slot: true},
			MockPayloadEarly:   map[[32]byte]bool{bytesutil.ToBytes32(root): true},
			HeadDependentRoot:  bytesutil.ToBytes32(bytesutil.PadTo([]byte{0x01}, 32)),
			DependentRootCB: func(_ [32]byte, _ primitives.Epoch) ([32]byte, error) {
				return bytesutil.ToBytes32(bytesutil.PadTo([]byte{0x02}, 32)), nil
			},
		}
		s := &Service{GenesisTimeFetcher: chain, ForkchoiceFetcher: chain, HeadFetcher: chain}

		_, rpcErr := s.PayloadAttestationData(t.Context(), slot)
		require.NotNil(t, rpcErr)
		assert.Equal(t, ErrorReason(Unavailable), rpcErr.Reason)
		assert.ErrorContains(t, "no canonical shuffling", rpcErr.Err)
		assert.Equal(t, (*ethpb.PayloadAttestationData)(nil), s.payloadAttestationData.Load())
	})
	t.Run("canonical shuffling with non-zero roots → ok", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		slot := primitives.Slot(5)
		root := bytesutil.PadTo([]byte("head-root"), 32)
		dependent := bytesutil.ToBytes32(bytesutil.PadTo([]byte{0x07}, 32))
		chain := &mockChain.ChainService{
			Slot:               &slot,
			Root:               root,
			MockCanonicalRoots: map[primitives.Slot][32]byte{slot: bytesutil.ToBytes32(root)},
			MockCanonicalFull:  map[primitives.Slot]bool{slot: true},
			MockPayloadEarly:   map[[32]byte]bool{bytesutil.ToBytes32(root): true},
			HeadDependentRoot:  dependent,
			DependentRootCB: func(_ [32]byte, _ primitives.Epoch) ([32]byte, error) {
				return dependent, nil
			},
		}
		s := &Service{GenesisTimeFetcher: chain, ForkchoiceFetcher: chain, HeadFetcher: chain}

		data, rpcErr := s.PayloadAttestationData(t.Context(), slot)
		require.IsNil(t, rpcErr)
		assert.DeepEqual(t, root, data.BeaconBlockRoot)
		assert.Equal(t, true, data.PayloadPresent)
		assert.Equal(t, true, data.BlobDataAvailable)
	})
	t.Run("dependent root uses previous epoch", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		slot, err := slots.EpochStart(primitives.Epoch(3))
		require.NoError(t, err)
		root := bytesutil.PadTo([]byte("head-root"), 32)
		dependent := bytesutil.ToBytes32(bytesutil.PadTo([]byte{0x07}, 32))
		chain := &mockChain.ChainService{
			Slot:               &slot,
			Root:               root,
			MockCanonicalRoots: map[primitives.Slot][32]byte{slot: bytesutil.ToBytes32(root)},
			MockCanonicalFull:  map[primitives.Slot]bool{slot: true},
			MockPayloadEarly:   map[[32]byte]bool{bytesutil.ToBytes32(root): true},
			HeadDependentRoot:  dependent,
			DependentRootCB: func(_ [32]byte, epoch primitives.Epoch) ([32]byte, error) {
				assert.Equal(t, slots.ToEpoch(slot)-1, epoch)
				return dependent, nil
			},
		}
		s := &Service{GenesisTimeFetcher: chain, ForkchoiceFetcher: chain, HeadFetcher: chain}

		data, rpcErr := s.PayloadAttestationData(t.Context(), slot)
		require.IsNil(t, rpcErr)
		assert.DeepEqual(t, root, data.BeaconBlockRoot)
	})
}

func TestValidatorActiveSetChanges(t *testing.T) {
	t.Run("future epoch", func(t *testing.T) {
		currentSlot, err := slots.EpochStart(primitives.Epoch(3))
		require.NoError(t, err)
		chain := &mockChain.ChainService{Slot: &currentSlot}
		s := &Service{GenesisTimeFetcher: chain}

		_, rpcErr := s.ValidatorActiveSetChanges(t.Context(), primitives.Epoch(4))
		require.NotNil(t, rpcErr)
		assert.Equal(t, ErrorReason(BadRequest), rpcErr.Reason)
		assert.ErrorContains(t, "cannot retrieve information about an epoch in the future, current epoch 3, requesting 4", rpcErr.Err)
	})

	t.Run("nominal", func(t *testing.T) {
		const numValidators = 8

		validators := make([]*ethpb.Validator, numValidators)
		for i := range validators {
			activationEpoch := params.BeaconConfig().FarFutureEpoch
			withdrawableEpoch := params.BeaconConfig().FarFutureEpoch
			exitEpoch := params.BeaconConfig().FarFutureEpoch
			slashed := false
			balance := params.BeaconConfig().MaxEffectiveBalance
			switch {
			case i%2 == 0:
				// Activated at epoch 0.
				activationEpoch = 0
			case i%3 == 0:
				// Slashed.
				withdrawableEpoch = params.BeaconConfig().EpochsPerSlashingsVector
				slashed = true
			case i%5 == 0:
				// Exited at epoch 0.
				exitEpoch = 0
				withdrawableEpoch = params.BeaconConfig().MinValidatorWithdrawabilityDelay
			case i%7 == 0:
				// Ejected at epoch 0 (effective balance at ejection threshold).
				exitEpoch = 0
				withdrawableEpoch = params.BeaconConfig().MinValidatorWithdrawabilityDelay
				balance = params.BeaconConfig().EjectionBalance
			}
			validators[i] = &ethpb.Validator{
				ActivationEpoch:       activationEpoch,
				PublicKey:             pubKey(uint64(i)),
				EffectiveBalance:      balance,
				WithdrawalCredentials: make([]byte, 32),
				WithdrawableEpoch:     withdrawableEpoch,
				Slashed:               slashed,
				ExitEpoch:             exitEpoch,
			}
		}

		headState, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, headState.SetSlot(0))
		require.NoError(t, headState.SetValidators(validators))

		slot := primitives.Slot(0)
		chain := &mockChain.ChainService{Slot: &slot}
		s := &Service{
			GenesisTimeFetcher: chain,
			ReplayerBuilder:    mockstategen.NewReplayerBuilder(mockstategen.WithMockState(headState)),
		}

		res, rpcErr := s.ValidatorActiveSetChanges(t.Context(), primitives.Epoch(0))
		require.IsNil(t, rpcErr)

		assert.Equal(t, primitives.Epoch(0), res.Epoch)
		assert.DeepEqual(t, []primitives.ValidatorIndex{0, 2, 4, 6}, res.ActivatedIndices)
		assert.DeepEqual(t, [][]byte{pubKey(0), pubKey(2), pubKey(4), pubKey(6)}, res.ActivatedPublicKeys)
		assert.DeepEqual(t, []primitives.ValidatorIndex{5}, res.ExitedIndices)
		assert.DeepEqual(t, [][]byte{pubKey(5)}, res.ExitedPublicKeys)
		assert.DeepEqual(t, []primitives.ValidatorIndex{3}, res.SlashedIndices)
		assert.DeepEqual(t, [][]byte{pubKey(3)}, res.SlashedPublicKeys)
		assert.DeepEqual(t, []primitives.ValidatorIndex{7}, res.EjectedIndices)
		assert.DeepEqual(t, [][]byte{pubKey(7)}, res.EjectedPublicKeys)
	})
}
