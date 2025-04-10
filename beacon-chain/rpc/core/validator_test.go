package core

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	mockChain "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	p2pmock "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/validator"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	testhelpers "github.com/OffchainLabs/prysm/v6/validator/client/beacon-api/test-helpers"
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
	mock := &mockChain.ChainService{}
	s := &Service{GenesisTimeFetcher: mock}

	t.Run("Happy path electra", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		config := params.BeaconConfig()
		config.ElectraForkEpoch = 0
		params.OverrideBeaconConfig(config)
		broadcaster := &p2pmock.MockBroadcaster{}
		s.Broadcaster = broadcaster
		agg := &ethpb.SignedAggregateAttestationAndProofElectra{
			Message: &ethpb.AggregateAttestationAndProofElectra{
				AggregatorIndex: 72,
				Aggregate: &ethpb.AttestationElectra{
					AggregationBits: testhelpers.FillByteSlice(4, 74),
					Data: &ethpb.AttestationData{
						Slot:            75,
						CommitteeIndex:  76,
						BeaconBlockRoot: testhelpers.FillByteSlice(32, 38),
						Source: &ethpb.Checkpoint{
							Epoch: 78,
							Root:  testhelpers.FillByteSlice(32, 79),
						},
						Target: &ethpb.Checkpoint{
							Epoch: 80,
							Root:  testhelpers.FillByteSlice(32, 81),
						},
					},
					Signature:     testhelpers.FillByteSlice(96, 82),
					CommitteeBits: testhelpers.FillByteSlice(8, 83),
				},
				SelectionProof: testhelpers.FillByteSlice(96, 84),
			},
			Signature: testhelpers.FillByteSlice(96, 85),
		}
		rpcError := s.SubmitSignedAggregateSelectionProof(context.Background(), agg)
		assert.Equal(t, true, rpcError == nil)
	})

	t.Run("Phase 0 post electra", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		config := params.BeaconConfig()
		config.ElectraForkEpoch = 0
		params.OverrideBeaconConfig(config)

		agg := &ethpb.SignedAggregateAttestationAndProof{
			Message: &ethpb.AggregateAttestationAndProof{
				Aggregate: &ethpb.Attestation{
					Data: &ethpb.AttestationData{},
				},
			},
			Signature: make([]byte, 96),
		}
		rpcError := s.SubmitSignedAggregateSelectionProof(context.Background(), agg)
		assert.ErrorContains(t, "old aggregate and proof", rpcError.Err)
	})

	t.Run("electra agg pre electra", func(t *testing.T) {
		agg := &ethpb.SignedAggregateAttestationAndProofElectra{
			Message: &ethpb.AggregateAttestationAndProofElectra{
				Aggregate: &ethpb.AttestationElectra{
					Data: &ethpb.AttestationData{},
				},
			},
			Signature: make([]byte, 96),
		}
		rpcError := s.SubmitSignedAggregateSelectionProof(context.Background(), agg)
		assert.ErrorContains(t, "electra aggregate and proof not supported yet", rpcError.Err)
	})
}
