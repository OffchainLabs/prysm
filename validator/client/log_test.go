package client

import (
	"testing"
	"time"

	field_params "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/validator/pqkeys"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func TestLogSubmittedAtts(t *testing.T) {
	t.Run("phase0 attestations", func(t *testing.T) {
		logHook := logTest.NewGlobal()
		v := validator{
			submittedAtts:             make(map[submittedAttKey]*submittedAtt),
			attestedSlotsByKeyByEpoch: make(map[primitives.Epoch]map[[field_params.BLSPubkeyLength]byte]primitives.Slot),
		}
		att := util.HydrateAttestation(&ethpb.Attestation{})
		att.Data.CommitteeIndex = 12
		require.NoError(t, v.saveSubmittedAtt(att, make([]byte, field_params.BLSPubkeyLength), false))
		v.logSubmittedAtts(0)
		assert.LogsContain(t, logHook, "committeeIndices=\"[12]\"")
	})
	t.Run("electra attestations", func(t *testing.T) {
		logHook := logTest.NewGlobal()
		v := validator{
			submittedAtts:             make(map[submittedAttKey]*submittedAtt),
			attestedSlotsByKeyByEpoch: make(map[primitives.Epoch]map[[field_params.BLSPubkeyLength]byte]primitives.Slot),
		}
		att := util.HydrateAttestationElectra(&ethpb.AttestationElectra{})
		att.Data.CommitteeIndex = 0
		att.CommitteeBits = primitives.NewAttestationCommitteeBits()
		att.CommitteeBits.SetBitAt(44, true)
		require.NoError(t, v.saveSubmittedAtt(att, make([]byte, field_params.BLSPubkeyLength), false))
		v.logSubmittedAtts(0)
		assert.LogsContain(t, logHook, "committeeIndices=\"[44]\"")
	})
	t.Run("PQ attestations", func(t *testing.T) {
		logHook := logTest.NewGlobal()
		v := validator{
			pqKeys:                    pqkeys.NewRegistry(t.Context(), nil),
			submittedAtts:             make(map[submittedAttKey]*submittedAtt),
			attestedSlotsByKeyByEpoch: make(map[primitives.Epoch]map[[field_params.BLSPubkeyLength]byte]primitives.Slot),
		}

		// newAtt returns a PQ attestation in committee `committee` with payload status `payloadStatus`.
		newAtt := func(committee, payloadStatus primitives.CommitteeIndex) *ethpb.SingleAttestationPQ {
			data := util.HydrateAttestationData(&ethpb.AttestationData{CommitteeIndex: payloadStatus})
			return &ethpb.SingleAttestationPQ{CommitteeId: committee, Data: data}
		}

		require.NoError(t, v.saveSubmittedAttPQ(newAtt(3, 0), bytesutil.PadTo([]byte{1}, field_params.BLSPubkeyLength), 200*time.Millisecond))
		require.NoError(t, v.saveSubmittedAttPQ(newAtt(5, 0), bytesutil.PadTo([]byte{2}, field_params.BLSPubkeyLength), 600400*time.Microsecond))
		require.NoError(t, v.saveSubmittedAttPQ(newAtt(7, 1), bytesutil.PadTo([]byte{3}, field_params.BLSPubkeyLength), 300*time.Millisecond))
		v.logSubmittedAtts(0)

		require.Equal(t, 2, len(logHook.AllEntries()))
		assert.LogsContain(t, logHook, "Submitted new PQ attestations")
		assert.LogsContain(t, logHook, "committeeIndices=\"[3 5]\" package=validator/client payloadStatus=0 pubkeys=\"[0x010000000000 0x020000000000]\" signingTime=\"[min: 200ms, avg: 400ms, max: 600ms]\"")
		assert.LogsContain(t, logHook, "committeeIndices=\"[7]\" package=validator/client payloadStatus=1 pubkeys=\"[0x030000000000]\" signingTime=\"[min: 300ms, avg: 300ms, max: 300ms]\"")
	})
	t.Run("electra attestations multiple saved", func(t *testing.T) {
		logHook := logTest.NewGlobal()
		v := validator{
			submittedAtts:             make(map[submittedAttKey]*submittedAtt),
			attestedSlotsByKeyByEpoch: make(map[primitives.Epoch]map[[field_params.BLSPubkeyLength]byte]primitives.Slot),
		}
		att := util.HydrateAttestationElectra(&ethpb.AttestationElectra{})
		att.Data.CommitteeIndex = 0
		att.CommitteeBits = primitives.NewAttestationCommitteeBits()
		att.CommitteeBits.SetBitAt(23, true)
		require.NoError(t, v.saveSubmittedAtt(att, make([]byte, field_params.BLSPubkeyLength), false))
		att2 := util.HydrateAttestationElectra(&ethpb.AttestationElectra{})
		att2.Data.CommitteeIndex = 0
		att2.CommitteeBits = primitives.NewAttestationCommitteeBits()
		att2.CommitteeBits.SetBitAt(2, true)
		require.NoError(t, v.saveSubmittedAtt(att2, make([]byte, field_params.BLSPubkeyLength), false))
		v.logSubmittedAtts(0)
		assert.LogsContain(t, logHook, "committeeIndices=\"[23 2]\"")
	})
	t.Run("phase0 aggregates", func(t *testing.T) {
		logHook := logTest.NewGlobal()
		v := validator{
			submittedAggregates: make(map[submittedAttKey]*submittedAtt),
		}
		agg := &ethpb.AggregateAttestationAndProof{}
		agg.Aggregate = util.HydrateAttestation(&ethpb.Attestation{})
		agg.Aggregate.Data.CommitteeIndex = 12
		require.NoError(t, v.saveSubmittedAtt(agg.AggregateVal(), make([]byte, field_params.BLSPubkeyLength), true))
		v.logSubmittedAtts(0)
		assert.LogsContain(t, logHook, "committeeIndices=\"[12]\"")
	})
	t.Run("electra aggregates", func(t *testing.T) {
		logHook := logTest.NewGlobal()
		v := validator{
			submittedAggregates: make(map[submittedAttKey]*submittedAtt),
		}
		agg := &ethpb.AggregateAttestationAndProofElectra{}
		agg.Aggregate = util.HydrateAttestationElectra(&ethpb.AttestationElectra{})
		agg.Aggregate.Data.CommitteeIndex = 0
		agg.Aggregate.CommitteeBits = primitives.NewAttestationCommitteeBits()
		agg.Aggregate.CommitteeBits.SetBitAt(63, true)
		require.NoError(t, v.saveSubmittedAtt(agg.AggregateVal(), make([]byte, field_params.BLSPubkeyLength), true))
		v.logSubmittedAtts(0)
		assert.LogsContain(t, logHook, "committeeIndices=\"[63]\"")
	})
}

func TestFromAttData(t *testing.T) {
	att := util.HydrateAttestation(&ethpb.Attestation{})
	key := submittedAttKey{}
	require.NoError(t, key.FromAttData(att.Data))
	assert.NotEqual(t, submittedAttKey{}, key, "key not populated from att data")

	att2 := util.HydrateAttestation(&ethpb.Attestation{})
	att2.Data.BeaconBlockRoot = bytesutil.PadTo([]byte("different root"), 32)
	key2 := submittedAttKey{}
	require.NoError(t, key2.FromAttData(att2.Data))
	assert.NotEqual(t, key, key2, "distinct att data must produce distinct keys")
}

func TestLogSubmittedSyncCommitteeMessages(t *testing.T) {
	logHook := logTest.NewGlobal()
	v := validator{}
	blockRoot := bytesutil.PadTo([]byte("root"), field_params.RootLength)
	for _, idx := range []primitives.ValidatorIndex{9, 7, 8} {
		v.saveSubmittedSyncMessage(&ethpb.SyncCommitteeMessage{Slot: 12, BlockRoot: blockRoot, ValidatorIndex: idx})
	}

	v.logSubmittedSyncCommitteeMessages(12)

	assert.LogsContain(t, logHook, "msg=\"Submitted sync committee messages\"")
	assert.LogsContain(t, logHook, "validatorIndices=7-9")
	assert.LogsContain(t, logHook, "messages=3")
	assert.LogsContain(t, logHook, "dataSlot=12")
	logHook.Reset()
	v.logSubmittedSyncCommitteeMessages(12)
	assert.Equal(t, 0, len(logHook.AllEntries()))
}

func TestLogSubmittedSyncCommitteeContributions(t *testing.T) {
	logHook := logTest.NewGlobal()
	v := validator{}
	blockRoot := bytesutil.PadTo([]byte("root"), field_params.RootLength)
	bitsSub3 := ethpb.NewSyncCommitteeAggregationBits()
	bitsSub3.SetBitAt(0, true)
	contributionSub3 := &ethpb.SyncCommitteeContribution{
		BlockRoot:         blockRoot,
		Slot:              12,
		SubcommitteeIndex: 3,
		AggregationBits:   bitsSub3,
	}
	bitsSub1 := ethpb.NewSyncCommitteeAggregationBits()
	bitsSub1.SetBitAt(0, true)
	bitsSub1.SetBitAt(1, true)
	contributionSub1 := &ethpb.SyncCommitteeContribution{
		BlockRoot:         blockRoot,
		Slot:              12,
		SubcommitteeIndex: 1,
		AggregationBits:   bitsSub1,
	}
	v.saveSubmittedSyncContribution(&ethpb.ContributionAndProof{AggregatorIndex: 8, Contribution: contributionSub3})
	v.saveSubmittedSyncContribution(&ethpb.ContributionAndProof{AggregatorIndex: 7, Contribution: contributionSub3})
	v.saveSubmittedSyncContribution(&ethpb.ContributionAndProof{AggregatorIndex: 5, Contribution: contributionSub1})

	v.logSubmittedSyncCommitteeContributions(12)

	require.Equal(t, 1, len(logHook.AllEntries()))
	assert.LogsContain(t, logHook, "msg=\"Submitted sync committee contributions and proofs\"")
	assert.LogsContain(t, logHook, "aggregatorIndices=\"5,7-8\"")
	assert.LogsContain(t, logHook, "contributions=3")
	assert.LogsContain(t, logHook, "subcommittees=\"1,3\"")
	assert.LogsContain(t, logHook, "totalBits=3")
	logHook.Reset()
	v.logSubmittedSyncCommitteeContributions(12)
	assert.Equal(t, 0, len(logHook.AllEntries()))
}

func TestLogSubmittedPayloadAttestations(t *testing.T) {
	logHook := logTest.NewGlobal()
	v := validator{}
	data := &ethpb.PayloadAttestationData{
		BeaconBlockRoot:   bytesutil.PadTo([]byte("root"), field_params.RootLength),
		Slot:              12,
		PayloadPresent:    true,
		BlobDataAvailable: true,
	}
	v.saveSubmittedPayloadAtt(data, 8)
	v.saveSubmittedPayloadAtt(data, 7)

	v.logSubmittedPayloadAttestations(12)

	assert.LogsContain(t, logHook, "msg=\"Submitted payload attestations\"")
	assert.LogsContain(t, logHook, "validatorIndices=7-8")
	assert.LogsContain(t, logHook, "attestations=2")
	assert.LogsContain(t, logHook, "payloadPresent=true")
	assert.LogsContain(t, logHook, "blobDataAvailable=true")
	logHook.Reset()
	v.logSubmittedPayloadAttestations(12)
	assert.Equal(t, 0, len(logHook.AllEntries()))
}
