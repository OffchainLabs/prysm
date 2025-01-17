package slashings

import (
	"context"
	"testing"
	"time"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/operations/slashings/mock"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/startup"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

var (
	_ = PoolManager(&Pool{})
	_ = PoolInserter(&Pool{})
	_ = PoolManager(&mock.PoolMock{})
	_ = PoolInserter(&mock.PoolMock{})
)

func TestPool_validatorSlashingPreconditionCheck_requiresLock(t *testing.T) {
	p := &Pool{}
	_, err := p.validatorSlashingPreconditionCheck(nil, 0)
	require.ErrorContains(t, "caller must hold read/write lock", err)
}

func Test_convertToElectraWithTimer(t *testing.T) {
	ctx := context.Background()

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

	// TODO: doc
	now := time.Now()
	electraTime := now.Add(time.Duration(uint64(cfg.ElectraForkEpoch)*uint64(params.BeaconConfig().SlotsPerEpoch)*params.BeaconConfig().SecondsPerSlot) * time.Second)
	c := startup.NewClock(now, [32]byte{}, startup.WithNower(func() time.Time { return electraTime }))
	cw := startup.NewClockSynchronizer()
	require.NoError(t, cw.SetClock(c))
	p := NewPool(ctx, WithElectraTimer(cw, func() primitives.Slot { return 31 }))
	p.pendingAttesterSlashing = append(p.pendingAttesterSlashing, phase0Slashing)

	p.run()

	electraSlashing, ok := p.pendingAttesterSlashing[0].attesterSlashing.(*ethpb.AttesterSlashingElectra)
	require.Equal(t, true, ok, "Slashing was not converted to Electra")
	assert.DeepEqual(t, phase0Slashing.attesterSlashing.FirstAttestation().GetAttestingIndices(), electraSlashing.FirstAttestation().GetAttestingIndices())
	assert.DeepEqual(t, phase0Slashing.attesterSlashing.FirstAttestation().GetData(), electraSlashing.FirstAttestation().GetData())
	assert.DeepEqual(t, phase0Slashing.attesterSlashing.FirstAttestation().GetSignature(), electraSlashing.FirstAttestation().GetSignature())
	assert.DeepEqual(t, phase0Slashing.attesterSlashing.SecondAttestation().GetAttestingIndices(), electraSlashing.SecondAttestation().GetAttestingIndices())
	assert.DeepEqual(t, phase0Slashing.attesterSlashing.SecondAttestation().GetData(), electraSlashing.SecondAttestation().GetData())
	assert.DeepEqual(t, phase0Slashing.attesterSlashing.SecondAttestation().GetSignature(), electraSlashing.SecondAttestation().GetSignature())
}
