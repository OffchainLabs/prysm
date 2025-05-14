package operations

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/electra"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	common "github.com/OffchainLabs/prysm/v6/testing/spectest/shared/common/operations"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func blockWithAttestation(attestationSSZ []byte) (interfaces.SignedBeaconBlock, error) {
	att := &ethpb.AttestationElectra{}
	if err := att.UnmarshalSSZ(attestationSSZ); err != nil {
		return nil, err
	}
	b := util.NewBeaconBlockElectra()
	b.Block.Body = &ethpb.BeaconBlockBodyElectra{Attestations: []*ethpb.AttestationElectra{att}}
	return blocks.NewSignedBeaconBlock(b)
}

func RunAttestationTest(t *testing.T, config string) {
	common.RunAttestationTest(t, config, version.String(version.Electra), blockWithAttestation, electra.ProcessAttestationsNoVerifySignature, sszToState)
}
