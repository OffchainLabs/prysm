package blocks_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v6/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func TestVerifyBlockHeaderSignature(t *testing.T) {
	beaconState, err := util.NewBeaconState()
	require.NoError(t, err)

	privKey, err := bls.RandKey()
	require.NoError(t, err)
	validators := make([]*ethpb.Validator, 1)
	validators[0] = &ethpb.Validator{
		PublicKey:             privKey.PublicKey().Marshal(),
		WithdrawalCredentials: make([]byte, 32),
		EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
	}
	err = beaconState.SetValidators(validators)
	require.NoError(t, err)

	// Sign the block header.
	blockHeader := util.HydrateSignedBeaconHeader(&ethpb.SignedBeaconBlockHeader{
		Header: &ethpb.BeaconBlockHeader{
			Slot:          0,
			ProposerIndex: 0,
		},
	})
	domain, err := signing.Domain(
		beaconState.Fork(),
		0,
		params.BeaconConfig().DomainBeaconProposer,
		beaconState.GenesisValidatorsRoot(),
	)
	require.NoError(t, err)
	htr, err := blockHeader.Header.HashTreeRoot()
	require.NoError(t, err)
	container := &ethpb.SigningData{
		ObjectRoot: htr[:],
		Domain:     domain,
	}
	require.NoError(t, err)
	signingRoot, err := container.HashTreeRoot()
	require.NoError(t, err)

	// Set the signature in the block header.
	blockHeader.Signature = privKey.Sign(signingRoot[:]).Marshal()

	// Sig should verify.
	err = blocks.VerifyBlockHeaderSignature(beaconState, blockHeader)
	require.NoError(t, err)
}

func TestVerifyBlockSignatureUsingCurrentFork(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	bCfg := params.BeaconConfig()
	bCfg.AltairForkEpoch = 100
	bCfg.ForkVersionSchedule[bytesutil.ToBytes4(bCfg.AltairForkVersion)] = 100
	params.OverrideBeaconConfig(bCfg)
	bState, keys := util.DeterministicGenesisState(t, 100)
	altairBlk := util.NewBeaconBlockAltair()
	altairBlk.Block.ProposerIndex = 0
	altairBlk.Block.Slot = params.BeaconConfig().SlotsPerEpoch * 100
	fData := &ethpb.Fork{
		Epoch:           100,
		CurrentVersion:  params.BeaconConfig().AltairForkVersion,
		PreviousVersion: params.BeaconConfig().GenesisForkVersion,
	}
	domain, err := signing.Domain(fData, 100, params.BeaconConfig().DomainBeaconProposer, bState.GenesisValidatorsRoot())
	assert.NoError(t, err)
	blkRoot, err := altairBlk.Block.HashTreeRoot()
	assert.NoError(t, err)
	rt, err := signing.ComputeSigningRoot(altairBlk.Block, domain)
	assert.NoError(t, err)
	sig := keys[0].Sign(rt[:]).Marshal()
	altairBlk.Signature = sig
	wsb, err := consensusblocks.NewSignedBeaconBlock(altairBlk)
	require.NoError(t, err)
	assert.NoError(t, blocks.VerifyBlockSignatureUsingCurrentFork(bState, wsb, blkRoot))
}

func TestVerifyBlockSignatureUsingCurrentFork_InvalidSignature(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	bCfg := params.BeaconConfig()
	bCfg.AltairForkEpoch = 100
	bCfg.ForkVersionSchedule[bytesutil.ToBytes4(bCfg.AltairForkVersion)] = 100
	params.OverrideBeaconConfig(bCfg)
	bState, keys := util.DeterministicGenesisState(t, 100)
	altairBlk := util.NewBeaconBlockAltair()
	altairBlk.Block.ProposerIndex = 0
	altairBlk.Block.Slot = params.BeaconConfig().SlotsPerEpoch * 100
	blkRoot, err := altairBlk.Block.HashTreeRoot()
	assert.NoError(t, err)

	// Sign with wrong key (proposer index 0, but using key 1)
	fData := &ethpb.Fork{
		Epoch:           100,
		CurrentVersion:  params.BeaconConfig().AltairForkVersion,
		PreviousVersion: params.BeaconConfig().GenesisForkVersion,
	}
	domain, err := signing.Domain(fData, 100, params.BeaconConfig().DomainBeaconProposer, bState.GenesisValidatorsRoot())
	assert.NoError(t, err)
	rt, err := signing.ComputeSigningRoot(altairBlk.Block, domain)
	assert.NoError(t, err)
	wrongSig := keys[1].Sign(rt[:]).Marshal()
	altairBlk.Signature = wrongSig

	wsb, err := consensusblocks.NewSignedBeaconBlock(altairBlk)
	require.NoError(t, err)

	err = blocks.VerifyBlockSignatureUsingCurrentFork(bState, wsb, blkRoot)
	require.ErrorIs(t, err, blocks.ErrInvalidSignature, "Expected ErrInvalidSignature for invalid signature")
}
