package slashings

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestConvertToElectraWithTimer(t *testing.T) {
	ctx := t.Context()

	cfg := params.BeaconConfig().Copy()
	cfg.ElectraForkEpoch = 1
	params.OverrideBeaconConfig(cfg)
	params.SetupTestConfigCleanup(t)

	indices := []uint64{0, 1}
	data := &ethpb.AttestationData{
		Slot:            1,
		CommitteeIndex:  1,
		BeaconBlockRoot: make([]byte, fieldparams.RootLength),
		Source: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, fieldparams.RootLength),
		},
		Target: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, fieldparams.RootLength),
		},
	}
	sig := make([]byte, fieldparams.BLSSignatureLength)

	phase0Slashing := &PendingAttesterSlashing{
		attesterSlashing: &ethpb.AttesterSlashing{
			Attestation_1: &ethpb.IndexedAttestation{
				AttestingIndices: indices,
				Data:             data,
				Signature:        sig,
			},
			Attestation_2: &ethpb.IndexedAttestation{
				AttestingIndices: indices,
				Data:             data,
				Signature:        sig,
			},
		},
	}

	// We need run() to execute the conversion immediately, otherwise we'd need a time.Sleep to wait for the Electra fork.
	// To do that we need a timer with the current time being at the Electra fork.
	now := time.Now()
	electraTime := now.Add(time.Duration(uint64(cfg.ElectraForkEpoch)*uint64(params.BeaconConfig().SlotsPerEpoch)*params.BeaconConfig().SecondsPerSlot) * time.Second)
	c := startup.NewClock(now, [32]byte{}, startup.WithNower(func() time.Time { return electraTime }))
	cw := startup.NewClockSynchronizer()
	require.NoError(t, cw.SetClock(c))
	p := NewPool()
	// The service has to think that the current slot is before Electra
	// because run() exits early after Electra.
	s := NewPoolService(ctx, p, WithElectraTimer(cw, func() primitives.Slot {
		return primitives.Slot(cfg.ElectraForkEpoch)*params.BeaconConfig().SlotsPerEpoch - 1
	}))
	p.pendingAttesterSlashing = append(p.pendingAttesterSlashing, phase0Slashing)

	s.run()

	electraSlashing, ok := p.pendingAttesterSlashing[0].attesterSlashing.(*ethpb.AttesterSlashingElectra)
	require.Equal(t, true, ok, "Slashing was not converted to Electra")
	assert.DeepEqual(t, phase0Slashing.attesterSlashing.FirstAttestation().GetAttestingIndices(), electraSlashing.FirstAttestation().GetAttestingIndices())
	assert.DeepEqual(t, phase0Slashing.attesterSlashing.FirstAttestation().GetData(), electraSlashing.FirstAttestation().GetData())
	assert.DeepEqual(t, phase0Slashing.attesterSlashing.FirstAttestation().GetSignature(), electraSlashing.FirstAttestation().GetSignature())
	assert.DeepEqual(t, phase0Slashing.attesterSlashing.SecondAttestation().GetAttestingIndices(), electraSlashing.SecondAttestation().GetAttestingIndices())
	assert.DeepEqual(t, phase0Slashing.attesterSlashing.SecondAttestation().GetData(), electraSlashing.SecondAttestation().GetData())
	assert.DeepEqual(t, phase0Slashing.attesterSlashing.SecondAttestation().GetSignature(), electraSlashing.SecondAttestation().GetSignature())
}
