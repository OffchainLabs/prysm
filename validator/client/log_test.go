package client

import (
	"testing"

	field_params "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func TestLogSubmittedAtts(t *testing.T) {
	t.Run("phase0 attestations", func(t *testing.T) {
		logHook := logTest.NewGlobal()
		v := validator{
			submittedAtts: make(map[submittedAttKey]*submittedAtt),
		}
		att := util.HydrateAttestation(&ethpb.Attestation{})
		att.Data.CommitteeIndex = 12
		require.NoError(t, v.saveSubmittedAtt(att, make([]byte, field_params.BLSPubkeyLength), false))
		v.LogSubmittedAtts(0)
		assert.LogsContain(t, logHook, "committeeIndices=\"[12]\"")
	})
	t.Run("electra attestations", func(t *testing.T) {
		logHook := logTest.NewGlobal()
		v := validator{
			submittedAtts: make(map[submittedAttKey]*submittedAtt),
		}
		att := util.HydrateAttestationElectra(&ethpb.AttestationElectra{})
		att.Data.CommitteeIndex = 0
		att.CommitteeBits = primitives.NewAttestationCommitteeBits()
		att.CommitteeBits[0] = 44
		att.CommitteeBits[1] = 73
		require.NoError(t, v.saveSubmittedAtt(att, make([]byte, field_params.BLSPubkeyLength), false))
		v.LogSubmittedAtts(0)
		assert.LogsContain(t, logHook, "committeeIndices=\"[2]\"")
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
		v.LogSubmittedAtts(0)
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
		agg.Aggregate.CommitteeBits[0] = 66
		require.NoError(t, v.saveSubmittedAtt(agg.AggregateVal(), make([]byte, field_params.BLSPubkeyLength), true))
		v.LogSubmittedAtts(0)
		assert.LogsContain(t, logHook, "committeeIndices=\"[1]\"")
	})
}
