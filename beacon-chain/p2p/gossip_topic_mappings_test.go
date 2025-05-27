package p2p

import (
	"reflect"
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
)

func TestMappingHasNoDuplicates(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	m := make(map[reflect.Type]bool)
	for _, v := range gossipTopicMappings {
		if _, ok := m[reflect.TypeOf(v())]; ok {
			t.Errorf("%T is duplicated in the topic mapping", v)
		}
		m[reflect.TypeOf(v)] = true
	}
}

func TestGossipTopicMappings_CorrectType(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	bCfg := params.BeaconConfig().Copy()
	altairForkEpoch := primitives.Epoch(100)
	bellatrixForkEpoch := primitives.Epoch(200)
	capellaForkEpoch := primitives.Epoch(300)
	denebForkEpoch := primitives.Epoch(400)
	electraForkEpoch := primitives.Epoch(500)
	fuluForkEpoch := primitives.Epoch(600)

	bCfg.AltairForkEpoch = altairForkEpoch
	bCfg.BellatrixForkEpoch = bellatrixForkEpoch
	bCfg.CapellaForkEpoch = capellaForkEpoch
	bCfg.DenebForkEpoch = denebForkEpoch
	bCfg.ElectraForkEpoch = electraForkEpoch
	bCfg.FuluForkEpoch = fuluForkEpoch
	bCfg.ForkVersionSchedule[bytesutil.ToBytes4(bCfg.AltairForkVersion)] = primitives.Epoch(100)
	bCfg.ForkVersionSchedule[bytesutil.ToBytes4(bCfg.BellatrixForkVersion)] = primitives.Epoch(200)
	bCfg.ForkVersionSchedule[bytesutil.ToBytes4(bCfg.CapellaForkVersion)] = primitives.Epoch(300)
	bCfg.ForkVersionSchedule[bytesutil.ToBytes4(bCfg.DenebForkVersion)] = primitives.Epoch(400)
	bCfg.ForkVersionSchedule[bytesutil.ToBytes4(bCfg.ElectraForkVersion)] = primitives.Epoch(500)
	bCfg.ForkVersionSchedule[bytesutil.ToBytes4(bCfg.FuluForkVersion)] = primitives.Epoch(600)
	params.OverrideBeaconConfig(bCfg)

	// Phase 0
	pMessage := GossipTopicMappings(BlockSubnetTopicFormat, 0)
	_, ok := pMessage.(*ethpb.SignedBeaconBlock)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(AttestationSubnetTopicFormat, 0)
	_, ok = pMessage.(*ethpb.Attestation)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(AttesterSlashingSubnetTopicFormat, 0)
	_, ok = pMessage.(*ethpb.AttesterSlashing)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(AggregateAndProofSubnetTopicFormat, 0)
	_, ok = pMessage.(*ethpb.SignedAggregateAttestationAndProof)
	assert.Equal(t, true, ok)

	// Altair Fork
	pMessage = GossipTopicMappings(BlockSubnetTopicFormat, altairForkEpoch)
	_, ok = pMessage.(*ethpb.SignedBeaconBlockAltair)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(AttestationSubnetTopicFormat, altairForkEpoch)
	_, ok = pMessage.(*ethpb.Attestation)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(AttesterSlashingSubnetTopicFormat, altairForkEpoch)
	_, ok = pMessage.(*ethpb.AttesterSlashing)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(AggregateAndProofSubnetTopicFormat, altairForkEpoch)
	_, ok = pMessage.(*ethpb.SignedAggregateAttestationAndProof)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(LightClientOptimisticUpdateTopicFormat, altairForkEpoch)
	_, ok = pMessage.(*ethpb.LightClientOptimisticUpdateAltair)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(LightClientFinalityUpdateTopicFormat, altairForkEpoch)
	_, ok = pMessage.(*ethpb.LightClientFinalityUpdateAltair)
	assert.Equal(t, true, ok)

	// Bellatrix Fork
	pMessage = GossipTopicMappings(BlockSubnetTopicFormat, bellatrixForkEpoch)
	_, ok = pMessage.(*ethpb.SignedBeaconBlockBellatrix)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(AttestationSubnetTopicFormat, bellatrixForkEpoch)
	_, ok = pMessage.(*ethpb.Attestation)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(AttesterSlashingSubnetTopicFormat, bellatrixForkEpoch)
	_, ok = pMessage.(*ethpb.AttesterSlashing)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(AggregateAndProofSubnetTopicFormat, bellatrixForkEpoch)
	_, ok = pMessage.(*ethpb.SignedAggregateAttestationAndProof)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(LightClientOptimisticUpdateTopicFormat, bellatrixForkEpoch)
	_, ok = pMessage.(*ethpb.LightClientOptimisticUpdateAltair)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(LightClientFinalityUpdateTopicFormat, bellatrixForkEpoch)
	_, ok = pMessage.(*ethpb.LightClientFinalityUpdateAltair)
	assert.Equal(t, true, ok)

	// Capella Fork
	pMessage = GossipTopicMappings(BlockSubnetTopicFormat, capellaForkEpoch)
	_, ok = pMessage.(*ethpb.SignedBeaconBlockCapella)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(AttestationSubnetTopicFormat, capellaForkEpoch)
	_, ok = pMessage.(*ethpb.Attestation)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(AttesterSlashingSubnetTopicFormat, capellaForkEpoch)
	_, ok = pMessage.(*ethpb.AttesterSlashing)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(AggregateAndProofSubnetTopicFormat, capellaForkEpoch)
	_, ok = pMessage.(*ethpb.SignedAggregateAttestationAndProof)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(LightClientOptimisticUpdateTopicFormat, capellaForkEpoch)
	_, ok = pMessage.(*ethpb.LightClientOptimisticUpdateCapella)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(LightClientFinalityUpdateTopicFormat, capellaForkEpoch)
	_, ok = pMessage.(*ethpb.LightClientFinalityUpdateCapella)
	assert.Equal(t, true, ok)

	// Deneb Fork
	pMessage = GossipTopicMappings(BlockSubnetTopicFormat, denebForkEpoch)
	_, ok = pMessage.(*ethpb.SignedBeaconBlockDeneb)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(AttestationSubnetTopicFormat, denebForkEpoch)
	_, ok = pMessage.(*ethpb.Attestation)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(AttesterSlashingSubnetTopicFormat, denebForkEpoch)
	_, ok = pMessage.(*ethpb.AttesterSlashing)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(AggregateAndProofSubnetTopicFormat, denebForkEpoch)
	_, ok = pMessage.(*ethpb.SignedAggregateAttestationAndProof)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(LightClientOptimisticUpdateTopicFormat, denebForkEpoch)
	_, ok = pMessage.(*ethpb.LightClientOptimisticUpdateDeneb)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(LightClientFinalityUpdateTopicFormat, denebForkEpoch)
	_, ok = pMessage.(*ethpb.LightClientFinalityUpdateDeneb)
	assert.Equal(t, true, ok)

	// Electra Fork
	pMessage = GossipTopicMappings(BlockSubnetTopicFormat, electraForkEpoch)
	_, ok = pMessage.(*ethpb.SignedBeaconBlockElectra)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(AttestationSubnetTopicFormat, electraForkEpoch)
	_, ok = pMessage.(*ethpb.SingleAttestation)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(AttesterSlashingSubnetTopicFormat, electraForkEpoch)
	_, ok = pMessage.(*ethpb.AttesterSlashingElectra)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(AggregateAndProofSubnetTopicFormat, electraForkEpoch)
	_, ok = pMessage.(*ethpb.SignedAggregateAttestationAndProofElectra)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(LightClientOptimisticUpdateTopicFormat, electraForkEpoch)
	_, ok = pMessage.(*ethpb.LightClientOptimisticUpdateDeneb)
	assert.Equal(t, true, ok)
	pMessage = GossipTopicMappings(LightClientFinalityUpdateTopicFormat, electraForkEpoch)
	_, ok = pMessage.(*ethpb.LightClientFinalityUpdateElectra)
	assert.Equal(t, true, ok)
}
