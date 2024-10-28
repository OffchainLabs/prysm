package blocks

import (
	"testing"

	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func Test_initSignedBlockFromProtoEpbs(t *testing.T) {
	f := getFields()
	expectedBlock := &eth.SignedBeaconBlockEpbs{
		Block: &eth.BeaconBlockEpbs{
			Slot:          128,
			ProposerIndex: 128,
			ParentRoot:    f.root[:],
			StateRoot:     f.root[:],
			Body:          bodyPbEpbs(),
		},
		Signature: f.sig[:],
	}
	resultBlock, err := initSignedBlockFromProtoEPBS(expectedBlock)
	require.NoError(t, err)
	resultHTR, err := resultBlock.block.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
	assert.DeepEqual(t, expectedBlock.Signature, resultBlock.signature[:])
}

func Test_initBlockFromProtoEpbs(t *testing.T) {
	f := getFields()
	expectedBlock := &eth.BeaconBlockEpbs{
		Slot:          128,
		ProposerIndex: 128,
		ParentRoot:    f.root[:],
		StateRoot:     f.root[:],
		Body:          bodyPbEpbs(),
	}
	resultBlock, err := initBlockFromProtoEpbs(expectedBlock)
	require.NoError(t, err)
	resultHTR, err := resultBlock.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBlock.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
}

func Test_initBlockBodyFromProtoEpbs(t *testing.T) {
	expectedBody := bodyPbEpbs()
	resultBody, err := initBlockBodyFromProtoEpbs(expectedBody)
	require.NoError(t, err)
	resultHTR, err := resultBody.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBody.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
}

func bodyEpbs() *BeaconBlockBody {
	f := getFields()
	return &BeaconBlockBody{
		version:      version.EPBS,
		randaoReveal: f.sig,
		eth1Data: &eth.Eth1Data{
			DepositRoot:  f.root[:],
			DepositCount: 128,
			BlockHash:    f.root[:],
		},
		graffiti:                     f.root,
		proposerSlashings:            f.proposerSlashings,
		attesterSlashingsElectra:     f.attesterSlashingsElectra,
		attestationsElectra:          f.attsElectra,
		deposits:                     f.deposits,
		voluntaryExits:               f.voluntaryExits,
		syncAggregate:                f.syncAggregate,
		signedExecutionPayloadHeader: f.signedPayloadHeader,
		blsToExecutionChanges:        f.blsToExecutionChanges,
		blobKzgCommitments:           f.kzgCommitments,
		payloadAttestations:          f.payloadAttestation,
	}
}

func bodyPbEpbs() *eth.BeaconBlockBodyEpbs {
	f := getFields()
	return &eth.BeaconBlockBodyEpbs{
		RandaoReveal: f.sig[:],
		Eth1Data: &eth.Eth1Data{
			DepositRoot:  f.root[:],
			DepositCount: 128,
			BlockHash:    f.root[:],
		},
		Graffiti:                     f.root[:],
		ProposerSlashings:            f.proposerSlashings,
		AttesterSlashings:            f.attesterSlashingsElectra,
		Attestations:                 f.attsElectra,
		Deposits:                     f.deposits,
		VoluntaryExits:               f.voluntaryExits,
		SyncAggregate:                f.syncAggregate,
		BlsToExecutionChanges:        f.blsToExecutionChanges,
		SignedExecutionPayloadHeader: f.signedPayloadHeader,
		PayloadAttestations:          f.payloadAttestation,
	}
}
