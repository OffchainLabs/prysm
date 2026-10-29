package blocks_test

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/attestation"
	"github.com/OffchainLabs/prysm/v7/runtime/interop"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
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

// attestationTestState returns an Electra state with n active validators at slot 2*SlotsPerEpoch+6, their secret keys,
// and an unsigned attestation that covers every committee of the previous slot at the given participation.
func attestationTestState(tb testing.TB, n uint64, participation float64) (state.BeaconState, []bls.SecretKey, *ethpb.AttestationElectra) {
	secs, pubs, err := interop.DeterministicallyGenerateKeys(0, n)
	require.NoError(tb, err)
	vals := make([]*ethpb.Validator, n)
	bals := make([]uint64, n)
	for i := range vals {
		vals[i] = &ethpb.Validator{
			PublicKey:             pubs[i].Marshal(),
			WithdrawalCredentials: make([]byte, 32),
			EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			WithdrawableEpoch:     params.BeaconConfig().FarFutureEpoch,
		}
		bals[i] = params.BeaconConfig().MaxEffectiveBalance
	}
	st, err := util.NewBeaconStateElectra()
	require.NoError(tb, err)
	require.NoError(tb, st.SetValidators(vals))
	require.NoError(tb, st.SetBalances(bals))
	attSlot := 2*params.BeaconConfig().SlotsPerEpoch + 5
	require.NoError(tb, st.SetSlot(attSlot+1))

	committees, err := helpers.BeaconCommittees(context.Background(), st, attSlot)
	require.NoError(tb, err)
	r := rand.New(rand.NewSource(1))
	cb := primitives.NewAttestationCommitteeBits()
	var total uint64
	for i, c := range committees {
		cb.SetBitAt(uint64(i), true)
		total += uint64(len(c))
	}
	aggBits := bitfield.NewBitlist(total)
	for i := range total {
		if r.Float64() < participation {
			aggBits.SetBitAt(i, true)
		}
	}
	att := util.HydrateAttestationElectra(&ethpb.AttestationElectra{
		AggregationBits: aggBits,
		CommitteeBits:   cb,
		Data:            &ethpb.AttestationData{Slot: attSlot},
	})
	return st, secs, att
}

func TestAttestationSignatureBatch_CommitteeAggregates(t *testing.T) {
	// 16384 validators give 4 committees of 128 per slot on the mainnet config.
	const n = 16384
	helpers.ClearCache()
	t.Cleanup(helpers.ClearCache)
	st, secs, att := attestationTestState(t, n, 0.9)
	ctx := context.Background()

	// attesting returns the sorted attesting indices of a, as the spec defines them.
	attesting := func(t *testing.T, ctx context.Context, a ethpb.Att) []uint64 {
		committees, err := helpers.AttestationCommitteesFromState(ctx, st, a)
		require.NoError(t, err)
		ia, err := attestation.ConvertToIndexed(ctx, a, committees...)
		require.NoError(t, err)
		return ia.GetAttestingIndices()
	}
	// requireAttestingKey asserts that the batch for a holds the aggregate of the attesting keys.
	requireAttestingKey := func(t *testing.T, ctx context.Context, a ethpb.Att) *bls.SignatureBatch {
		set, err := blocks.AttestationSignatureBatch(ctx, st, []ethpb.Att{a})
		require.NoError(t, err)
		require.Equal(t, 1, len(set.PublicKeys))
		want, err := st.AggregateKeyFromIndices(attesting(t, ctx, a))
		require.NoError(t, err)
		require.Equal(t, true, want.Equals(set.PublicKeys[0]))
		return set
	}
	withBits := func(bits bitfield.Bitlist) *ethpb.AttestationElectra {
		a := att.Copy()
		a.AggregationBits = bits
		return a
	}

	t.Run("fallback without cached aggregates", func(t *testing.T) {
		ctx := helpers.WithIsolatedCaches(ctx)
		keys, err := helpers.AttestationCommitteeAggregatesFromCache(ctx, st, att)
		require.NoError(t, err)
		require.Equal(t, true, keys == nil)
		requireAttestingKey(t, ctx, att)
	})

	_, err := blocks.AttestationSignatureBatch(ctx, st, []ethpb.Att{att})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		keys, err := helpers.AttestationCommitteeAggregatesFromCache(ctx, st, att)
		return err == nil && keys != nil
	}, time.Minute, time.Millisecond)

	t.Run("subtract path signature verifies", func(t *testing.T) {
		domain, err := signing.Domain(st.Fork(), 2, params.BeaconConfig().DomainBeaconAttester, st.GenesisValidatorsRoot())
		require.NoError(t, err)
		root, err := signing.ComputeSigningRoot(att.Data, domain)
		require.NoError(t, err)
		indices := attesting(t, ctx, att)
		sigs := make([]bls.Signature, len(indices))
		for i, idx := range indices {
			sigs[i] = secs[idx].Sign(root[:])
		}
		a := att.Copy()
		a.Signature = bls.AggregateSignatures(sigs).Marshal()
		set := requireAttestingKey(t, ctx, a)
		ok, err := set.Verify()
		require.NoError(t, err)
		require.Equal(t, true, ok)
	})
	t.Run("full participation", func(t *testing.T) {
		bits := bitfield.NewBitlist(att.AggregationBits.Len())
		for i := range bits.Len() {
			bits.SetBitAt(i, true)
		}
		requireAttestingKey(t, ctx, withBits(bits))
	})
	t.Run("one absent", func(t *testing.T) {
		bits := bitfield.NewBitlist(att.AggregationBits.Len())
		for i := uint64(1); i < bits.Len(); i++ {
			bits.SetBitAt(i, true)
		}
		requireAttestingKey(t, ctx, withBits(bits))
	})
	t.Run("low participation adds present keys", func(t *testing.T) {
		bits := bitfield.NewBitlist(att.AggregationBits.Len())
		for i := uint64(0); i < bits.Len(); i += 3 {
			bits.SetBitAt(i, true)
		}
		requireAttestingKey(t, ctx, withBits(bits))
	})
	t.Run("subset of committees", func(t *testing.T) {
		committees, err := helpers.BeaconCommittees(ctx, st, att.Data.Slot)
		require.NoError(t, err)
		a := att.Copy()
		a.CommitteeBits = primitives.NewAttestationCommitteeBits()
		a.CommitteeBits.SetBitAt(1, true)
		a.CommitteeBits.SetBitAt(3, true)
		a.AggregationBits = bitfield.NewBitlist(uint64(len(committees[1]) + len(committees[3])))
		for i := uint64(1); i < a.AggregationBits.Len(); i++ {
			a.AggregationBits.SetBitAt(i, true)
		}
		requireAttestingKey(t, ctx, a)
	})
	t.Run("pre-Electra attestation", func(t *testing.T) {
		committee, err := helpers.BeaconCommitteeFromState(ctx, st, att.Data.Slot, 2)
		require.NoError(t, err)
		bits := bitfield.NewBitlist(uint64(len(committee)))
		for i := uint64(1); i < bits.Len(); i++ {
			bits.SetBitAt(i, true)
		}
		a := util.HydrateAttestation(&ethpb.Attestation{
			AggregationBits: bits,
			Data:            &ethpb.AttestationData{Slot: att.Data.Slot, CommitteeIndex: 2},
		})
		requireAttestingKey(t, ctx, a)
	})
	t.Run("zero participants fails", func(t *testing.T) {
		_, err := blocks.AttestationSignatureBatch(ctx, st, []ethpb.Att{withBits(bitfield.NewBitlist(att.AggregationBits.Len()))})
		require.ErrorContains(t, "no attesting indices", err)
	})
	t.Run("aggregation bits longer than committees fail", func(t *testing.T) {
		bits := bitfield.NewBitlist(att.AggregationBits.Len() + 1)
		for i := range bits.Len() {
			bits.SetBitAt(i, true)
		}
		_, err := blocks.AttestationSignatureBatch(ctx, st, []ethpb.Att{withBits(bits)})
		require.ErrorContains(t, "bitfield length", err)
	})
}
