package core

import (
	"encoding/binary"
	"testing"
	"time"

	mockChain "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	p2pmock "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/consensus-types/validator"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func TestRegisterSyncSubnetProto(t *testing.T) {
	k := pubKey(3)
	committee := make([][]byte, 0)

	for i := 0; i < 100; i++ {
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

	for i := 0; i < 100; i++ {
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
