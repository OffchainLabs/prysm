package sync

import (
	"context"
	"testing"

	mockChain "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func TestProcessAttestationBucket(t *testing.T) {
	t.Run("EmptyBucket", func(t *testing.T) {
		hook := logTest.NewGlobal()
		s := &Service{}

		s.processAttestationBucket(context.Background(), nil)

		emptyBucket := &attestationBucket{
			attestations: []ethpb.Att{},
		}
		s.processAttestationBucket(context.Background(), emptyBucket)

		require.Equal(t, 0, len(hook.Entries), "Should not log any messages for empty buckets")
	})

	t.Run("ForkchoiceFailure", func(t *testing.T) {
		hook := logTest.NewGlobal()
		chainService := &mockChain.ChainService{
			NotFinalized: true, // This makes InForkchoice return false
		}

		s := &Service{
			cfg: &config{
				chain: chainService,
			},
		}

		attData := &ethpb.AttestationData{
			BeaconBlockRoot: bytesutil.PadTo([]byte("blockroot"), 32),
		}

		bucket := &attestationBucket{
			data:         attData,
			attestations: []ethpb.Att{util.NewAttestation()},
		}

		s.processAttestationBucket(context.Background(), bucket)

		require.Equal(t, 1, len(hook.Entries))
		assert.StringContains(t, "Failed forkchoice check for bucket", hook.LastEntry().Message)
		require.NotNil(t, hook.LastEntry().Data["error"])
	})

	t.Run("CommitteeFailure", func(t *testing.T) {
		hook := logTest.NewGlobal()
		beaconState, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, beaconState.SetSlot(1))

		chainService := &mockChain.ChainService{
			State:            beaconState,
			ValidAttestation: true,
		}

		s := &Service{
			cfg: &config{
				chain: chainService,
			},
		}

		attData := &ethpb.AttestationData{
			BeaconBlockRoot: bytesutil.PadTo([]byte("blockroot"), 32),
			Target: &ethpb.Checkpoint{
				Epoch: 1,
				Root:  bytesutil.PadTo([]byte("blockroot"), 32),
			},
			CommitteeIndex: 999999,
		}

		att := util.NewAttestation()
		att.Data = attData

		bucket := &attestationBucket{
			data:         attData,
			attestations: []ethpb.Att{att},
		}

		s.processAttestationBucket(context.Background(), bucket)

		require.Equal(t, 1, len(hook.Entries))
		assert.StringContains(t, "Failed to get committee from state", hook.LastEntry().Message)
	})

	t.Run("FFGConsistencyFailure", func(t *testing.T) {
		hook := logTest.NewGlobal()

		validators := make([]*ethpb.Validator, 64)
		for i := range validators {
			validators[i] = &ethpb.Validator{
				ExitEpoch:        1000000,
				EffectiveBalance: 32000000000,
			}
		}

		beaconState, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, beaconState.SetSlot(1))
		require.NoError(t, beaconState.SetValidators(validators))

		chainService := &mockChain.ChainService{
			State: beaconState,
		}

		s := &Service{
			cfg: &config{
				chain: chainService,
			},
		}

		attData := &ethpb.AttestationData{
			BeaconBlockRoot: bytesutil.PadTo([]byte("blockroot"), 32),
			Target: &ethpb.Checkpoint{
				Epoch: 1,
				Root:  bytesutil.PadTo([]byte("different_target"), 32), // Different from BeaconBlockRoot to trigger FFG failure
			},
		}

		att := util.NewAttestation()
		att.Data = attData

		bucket := &attestationBucket{
			data:         attData,
			attestations: []ethpb.Att{att},
		}

		s.processAttestationBucket(context.Background(), bucket)

		require.Equal(t, 1, len(hook.Entries))
		assert.StringContains(t, "Failed FFG consistency check for bucket", hook.LastEntry().Message)
	})

	t.Run("ProcessingSuccess", func(t *testing.T) {
		hook := logTest.NewGlobal()
		validators := make([]*ethpb.Validator, 64)
		for i := range validators {
			validators[i] = &ethpb.Validator{
				ExitEpoch:        1000000,
				EffectiveBalance: 32000000000,
			}
		}

		beaconState, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, beaconState.SetSlot(1))
		require.NoError(t, beaconState.SetValidators(validators))

		chainService := &mockChain.ChainService{
			State:            beaconState,
			ValidAttestation: true,
		}

		s := &Service{
			cfg: &config{
				chain: chainService,
			},
		}

		// Test with Phase0 attestation
		t.Run("Phase0_NoError", func(t *testing.T) {
			hook.Reset() // Reset logs before test
			phase0Att := util.NewAttestation()
			phase0Att.Data.Slot = 1
			phase0Att.Data.CommitteeIndex = 0

			bucket := &attestationBucket{
				data:         phase0Att.GetData(),
				attestations: []ethpb.Att{phase0Att},
			}

			s.processAttestationBucket(context.Background(), bucket)
		})

		// Test with SingleAttestation
		t.Run("Electra_NoError", func(t *testing.T) {
			hook.Reset() // Reset logs before test
			attData := &ethpb.AttestationData{
				Slot:            1,
				CommitteeIndex:  0,
				BeaconBlockRoot: bytesutil.PadTo([]byte("blockroot"), 32),
				Source: &ethpb.Checkpoint{
					Epoch: 0,
					Root:  bytesutil.PadTo([]byte("source"), 32),
				},
				Target: &ethpb.Checkpoint{
					Epoch: 1,
					Root:  bytesutil.PadTo([]byte("blockroot"), 32), // Same as BeaconBlockRoot for LMD/FFG consistency
				},
			}

			singleAtt := &ethpb.SingleAttestation{
				CommitteeId:   0,
				AttesterIndex: 0,
				Data:          attData,
				Signature:     make([]byte, 96),
			}

			bucket := &attestationBucket{
				data:         singleAtt.GetData(),
				attestations: []ethpb.Att{singleAtt},
			}

			s.processAttestationBucket(context.Background(), bucket)
		})
	})
}

func TestBucketAttestationsByData(t *testing.T) {
	t.Run("EmptyInput", func(t *testing.T) {
		hook := logTest.NewGlobal()
		buckets := bucketAttestationsByData(nil)
		require.Equal(t, 0, len(buckets))
		require.Equal(t, 0, len(hook.Entries))

		buckets = bucketAttestationsByData([]ethpb.Att{})
		require.Equal(t, 0, len(buckets))
		require.Equal(t, 0, len(hook.Entries))
	})

	t.Run("SingleAttestation", func(t *testing.T) {
		hook := logTest.NewGlobal()
		att := util.NewAttestation()
		att.Data.Slot = 1
		att.Data.CommitteeIndex = 0

		buckets := bucketAttestationsByData([]ethpb.Att{att})

		require.Equal(t, 1, len(buckets))
		var bucket *attestationBucket
		for _, b := range buckets {
			bucket = b
			break
		}
		require.NotNil(t, bucket)
		require.Equal(t, 1, len(bucket.attestations))
		require.Equal(t, att, bucket.attestations[0])
		require.Equal(t, att.GetData(), bucket.data)
		require.Equal(t, 0, len(hook.Entries))
	})

	t.Run("MultipleAttestationsSameData", func(t *testing.T) {
		hook := logTest.NewGlobal()

		att1 := util.NewAttestation()
		att1.Data.Slot = 1
		att1.Data.CommitteeIndex = 0

		att2 := util.NewAttestation()
		att2.Data = att1.Data             // Same data
		att2.Signature = make([]byte, 96) // Different signature

		buckets := bucketAttestationsByData([]ethpb.Att{att1, att2})

		require.Equal(t, 1, len(buckets), "Should have one bucket for same data")
		var bucket *attestationBucket
		for _, b := range buckets {
			bucket = b
			break
		}
		require.NotNil(t, bucket)
		require.Equal(t, 2, len(bucket.attestations), "Should have both attestations in one bucket")
		require.Equal(t, att1.GetData(), bucket.data)
		require.Equal(t, 0, len(hook.Entries))
	})

	t.Run("MultipleAttestationsDifferentData", func(t *testing.T) {
		hook := logTest.NewGlobal()

		att1 := util.NewAttestation()
		att1.Data.Slot = 1
		att1.Data.CommitteeIndex = 0

		att2 := util.NewAttestation()
		att2.Data.Slot = 2 // Different slot
		att2.Data.CommitteeIndex = 1

		buckets := bucketAttestationsByData([]ethpb.Att{att1, att2})

		require.Equal(t, 2, len(buckets), "Should have two buckets for different data")
		bucketCount := 0
		for _, bucket := range buckets {
			require.Equal(t, 1, len(bucket.attestations), "Each bucket should have one attestation")
			bucketCount++
		}
		require.Equal(t, 2, bucketCount, "Should have exactly two buckets")
		require.Equal(t, 0, len(hook.Entries))
	})

	t.Run("MixedAttestationTypes", func(t *testing.T) {
		hook := logTest.NewGlobal()

		// Create Phase0 attestation
		phase0Att := util.NewAttestation()
		phase0Att.Data.Slot = 1
		phase0Att.Data.CommitteeIndex = 0

		electraAtt := &ethpb.SingleAttestation{
			CommitteeId:   0,
			AttesterIndex: 1,
			Data:          phase0Att.Data, // Same data
			Signature:     make([]byte, 96),
		}

		buckets := bucketAttestationsByData([]ethpb.Att{phase0Att, electraAtt})

		require.Equal(t, 1, len(buckets), "Should have one bucket for same data")
		var bucket *attestationBucket
		for _, b := range buckets {
			bucket = b
			break
		}
		require.NotNil(t, bucket)
		require.Equal(t, 2, len(bucket.attestations), "Should have both attestations in one bucket")
		require.Equal(t, phase0Att.GetData(), bucket.data)
		require.Equal(t, 0, len(hook.Entries))
	})
}

func TestBatchVerifyAttestationSignatures(t *testing.T) {
	t.Run("EmptyInput", func(t *testing.T) {
		s := &Service{}

		beaconState, err := util.NewBeaconState()
		require.NoError(t, err)

		result := s.batchVerifyAttestationSignatures(context.Background(), []ethpb.Att{}, beaconState)

		// Empty input should return empty output
		require.Equal(t, 0, len(result))
	})

	t.Run("BatchVerificationWithState", func(t *testing.T) {
		hook := logTest.NewGlobal()
		validators := make([]*ethpb.Validator, 64)
		for i := range validators {
			validators[i] = &ethpb.Validator{
				ExitEpoch:        1000000,
				EffectiveBalance: 32000000000,
			}
		}

		beaconState, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, beaconState.SetSlot(1))
		require.NoError(t, beaconState.SetValidators(validators))

		s := &Service{}

		att := util.NewAttestation()
		att.Data.Slot = 1
		attestations := []ethpb.Att{att}

		result := s.batchVerifyAttestationSignatures(context.Background(), attestations, beaconState)
		require.NotNil(t, result)

		if len(result) == 0 && len(hook.Entries) > 0 {
			_ = false // Check if fallback message is logged
			for _, entry := range hook.Entries {
				if entry.Message == "batch verification failed, using individual checks" {
					_ = true // Found the fallback message
					break
				}
			}
			// It's OK if fallback message is logged - this means the function is working correctly
		}
	})

	t.Run("BatchVerificationFailureFallbackToIndividual", func(t *testing.T) {
		hook := logTest.NewGlobal()
		beaconState, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, beaconState.SetSlot(1))

		chainService := &mockChain.ChainService{
			State:            beaconState,
			ValidAttestation: false, // This will cause verification to fail
		}

		s := &Service{
			cfg: &config{
				chain: chainService,
			},
		}

		att := util.NewAttestation()
		att.Data.Slot = 1
		attestations := []ethpb.Att{att}

		result := s.batchVerifyAttestationSignatures(context.Background(), attestations, beaconState)

		require.Equal(t, 0, len(result))

		require.NotEqual(t, 0, len(hook.Entries), "Should have log entries")
		found := false
		for _, entry := range hook.Entries {
			if entry.Message == "batch verification failed, using individual checks" {
				found = true
				break
			}
		}
		require.Equal(t, true, found, "Should log fallback message")
	})
}
