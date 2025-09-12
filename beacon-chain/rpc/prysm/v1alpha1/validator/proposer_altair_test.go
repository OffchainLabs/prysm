package validator

import (
	"testing"

	chainmock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/synccommittee"
	mockSync "github.com/OffchainLabs/prysm/v6/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/prysmaticlabs/go-bitfield"
)

func TestProposer_GetSyncAggregate_OK(t *testing.T) {
	st, err := util.NewBeaconStateAltair()
	require.NoError(t, err)
	proposerServer := &Server{
		HeadFetcher:       &chainmock.ChainService{State: st},
		SyncChecker:       &mockSync.Sync{IsSyncing: false},
		SyncCommitteePool: synccommittee.NewStore(),
	}

	r := params.BeaconConfig().ZeroHash
	conts := []*ethpb.SyncCommitteeContribution{
		{Slot: 1, SubcommitteeIndex: 0, Signature: bls.NewAggregateSignature().Marshal(), AggregationBits: []byte{0b0001}, BlockRoot: r[:]},
		{Slot: 1, SubcommitteeIndex: 0, Signature: bls.NewAggregateSignature().Marshal(), AggregationBits: []byte{0b1001}, BlockRoot: r[:]},
		{Slot: 1, SubcommitteeIndex: 0, Signature: bls.NewAggregateSignature().Marshal(), AggregationBits: []byte{0b1110}, BlockRoot: r[:]},
		{Slot: 1, SubcommitteeIndex: 1, Signature: bls.NewAggregateSignature().Marshal(), AggregationBits: []byte{0b0001}, BlockRoot: r[:]},
		{Slot: 1, SubcommitteeIndex: 1, Signature: bls.NewAggregateSignature().Marshal(), AggregationBits: []byte{0b1001}, BlockRoot: r[:]},
		{Slot: 1, SubcommitteeIndex: 1, Signature: bls.NewAggregateSignature().Marshal(), AggregationBits: []byte{0b1110}, BlockRoot: r[:]},
		{Slot: 1, SubcommitteeIndex: 2, Signature: bls.NewAggregateSignature().Marshal(), AggregationBits: []byte{0b0001}, BlockRoot: r[:]},
		{Slot: 1, SubcommitteeIndex: 2, Signature: bls.NewAggregateSignature().Marshal(), AggregationBits: []byte{0b1001}, BlockRoot: r[:]},
		{Slot: 1, SubcommitteeIndex: 2, Signature: bls.NewAggregateSignature().Marshal(), AggregationBits: []byte{0b1110}, BlockRoot: r[:]},
		{Slot: 1, SubcommitteeIndex: 3, Signature: bls.NewAggregateSignature().Marshal(), AggregationBits: []byte{0b0001}, BlockRoot: r[:]},
		{Slot: 1, SubcommitteeIndex: 3, Signature: bls.NewAggregateSignature().Marshal(), AggregationBits: []byte{0b1001}, BlockRoot: r[:]},
		{Slot: 1, SubcommitteeIndex: 3, Signature: bls.NewAggregateSignature().Marshal(), AggregationBits: []byte{0b1110}, BlockRoot: r[:]},
		{Slot: 2, SubcommitteeIndex: 0, Signature: bls.NewAggregateSignature().Marshal(), AggregationBits: []byte{0b10101010}, BlockRoot: r[:]},
		{Slot: 2, SubcommitteeIndex: 1, Signature: bls.NewAggregateSignature().Marshal(), AggregationBits: []byte{0b10101010}, BlockRoot: r[:]},
		{Slot: 2, SubcommitteeIndex: 2, Signature: bls.NewAggregateSignature().Marshal(), AggregationBits: []byte{0b10101010}, BlockRoot: r[:]},
		{Slot: 2, SubcommitteeIndex: 3, Signature: bls.NewAggregateSignature().Marshal(), AggregationBits: []byte{0b10101010}, BlockRoot: r[:]},
	}

	for _, cont := range conts {
		require.NoError(t, proposerServer.SyncCommitteePool.SaveSyncCommitteeContribution(cont))
	}

	aggregate, err := proposerServer.getSyncAggregate(t.Context(), 1, bytesutil.ToBytes32(conts[0].BlockRoot))
	require.NoError(t, err)
	require.DeepEqual(t, bitfield.Bitvector32{0xf, 0xf, 0xf, 0xf}, aggregate.SyncCommitteeBits)

	aggregate, err = proposerServer.getSyncAggregate(t.Context(), 2, bytesutil.ToBytes32(conts[0].BlockRoot))
	require.NoError(t, err)
	require.DeepEqual(t, bitfield.Bitvector32{0xaa, 0xaa, 0xaa, 0xaa}, aggregate.SyncCommitteeBits)

	aggregate, err = proposerServer.getSyncAggregate(t.Context(), 3, bytesutil.ToBytes32(conts[0].BlockRoot))
	require.NoError(t, err)
	require.DeepEqual(t, bitfield.NewBitvector32(), aggregate.SyncCommitteeBits)
}

func TestServer_SetSyncAggregate_EmptyCase(t *testing.T) {
	b, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockAltair())
	require.NoError(t, err)
	s := &Server{} // Sever is not initialized with sync committee pool.
	s.setSyncAggregate(t.Context(), b)
	agg, err := b.Block().Body().SyncAggregate()
	require.NoError(t, err)

	emptySig := [96]byte{0xC0}
	want := &ethpb.SyncAggregate{
		SyncCommitteeBits:      make([]byte, params.BeaconConfig().SyncCommitteeSize/8),
		SyncCommitteeSignature: emptySig[:],
	}
	require.DeepEqual(t, want, agg)
}

func TestProposer_GetSyncAggregate_IncludesSyncCommitteeMessages(t *testing.T) {
	// TEST SETUP
	// - validator 0 is selected twice in subcommittee 0 (indexes [0,1])
	// - validator 1 is selected once in subcommittee 0 (index 2)
	// - validator 2 is selected twice in subcommittee 1 (indexes [0,1])
	// - validator 3 is selected once in subcommittee 1 (index 2)
	// - sync committee aggregates in the pool have index 3 set for both subcommittees

	subcommitteeSize := params.BeaconConfig().SyncCommitteeSize / params.BeaconConfig().SyncCommitteeSubnetCount

	helpers.ClearCache()
	st, err := util.NewBeaconStateAltair()
	require.NoError(t, err)
	vals := make([]*ethpb.Validator, 4)
	vals[0] = &ethpb.Validator{PublicKey: bytesutil.PadTo([]byte{0xf0}, 48)}
	vals[1] = &ethpb.Validator{PublicKey: bytesutil.PadTo([]byte{0xf1}, 48)}
	vals[2] = &ethpb.Validator{PublicKey: bytesutil.PadTo([]byte{0xf2}, 48)}
	vals[3] = &ethpb.Validator{PublicKey: bytesutil.PadTo([]byte{0xf3}, 48)}
	require.NoError(t, st.SetValidators(vals))
	sc := &ethpb.SyncCommittee{
		Pubkeys: make([][]byte, params.BeaconConfig().SyncCommitteeSize),
	}
	sc.Pubkeys[0] = vals[0].PublicKey
	sc.Pubkeys[1] = vals[0].PublicKey
	sc.Pubkeys[2] = vals[1].PublicKey
	sc.Pubkeys[subcommitteeSize] = vals[2].PublicKey
	sc.Pubkeys[subcommitteeSize+1] = vals[2].PublicKey
	sc.Pubkeys[subcommitteeSize+2] = vals[3].PublicKey
	require.NoError(t, st.SetCurrentSyncCommittee(sc))
	proposerServer := &Server{
		HeadFetcher:       &chainmock.ChainService{State: st},
		SyncChecker:       &mockSync.Sync{IsSyncing: false},
		SyncCommitteePool: synccommittee.NewStore(),
	}

	r := params.BeaconConfig().ZeroHash
	msgs := []*ethpb.SyncCommitteeMessage{
		{Slot: 1, BlockRoot: r[:], ValidatorIndex: 0, Signature: bls.NewAggregateSignature().Marshal()},
		{Slot: 1, BlockRoot: r[:], ValidatorIndex: 1, Signature: bls.NewAggregateSignature().Marshal()},
		{Slot: 1, BlockRoot: r[:], ValidatorIndex: 2, Signature: bls.NewAggregateSignature().Marshal()},
		{Slot: 1, BlockRoot: r[:], ValidatorIndex: 3, Signature: bls.NewAggregateSignature().Marshal()},
	}
	for _, msg := range msgs {
		require.NoError(t, proposerServer.SyncCommitteePool.SaveSyncCommitteeMessage(msg))
	}
	subcommittee0AggBits := ethpb.NewSyncCommitteeAggregationBits()
	subcommittee0AggBits.SetBitAt(3, true)
	subcommittee1AggBits := ethpb.NewSyncCommitteeAggregationBits()
	subcommittee1AggBits.SetBitAt(3, true)
	conts := []*ethpb.SyncCommitteeContribution{
		{Slot: 1, SubcommitteeIndex: 0, Signature: bls.NewAggregateSignature().Marshal(), AggregationBits: subcommittee0AggBits, BlockRoot: r[:]},
		{Slot: 1, SubcommitteeIndex: 1, Signature: bls.NewAggregateSignature().Marshal(), AggregationBits: subcommittee1AggBits, BlockRoot: r[:]},
	}
	for _, cont := range conts {
		require.NoError(t, proposerServer.SyncCommitteePool.SaveSyncCommitteeContribution(cont))
	}

	// The final sync aggregates must have indexes [0,1,2,3] set for both subcommittees
	sa, err := proposerServer.getSyncAggregate(t.Context(), 1, r)
	require.NoError(t, err)
	assert.Equal(t, true, sa.SyncCommitteeBits.BitAt(0))
	assert.Equal(t, true, sa.SyncCommitteeBits.BitAt(1))
	assert.Equal(t, true, sa.SyncCommitteeBits.BitAt(2))
	assert.Equal(t, true, sa.SyncCommitteeBits.BitAt(3))
	assert.Equal(t, true, sa.SyncCommitteeBits.BitAt(subcommitteeSize))
	assert.Equal(t, true, sa.SyncCommitteeBits.BitAt(subcommitteeSize+1))
	assert.Equal(t, true, sa.SyncCommitteeBits.BitAt(subcommitteeSize+2))
	assert.Equal(t, true, sa.SyncCommitteeBits.BitAt(subcommitteeSize+3))
}

func Test_aggregatedSyncCommitteeMessages_NoIntersectionWithPoolContributions(t *testing.T) {
	helpers.ClearCache()
	st, err := util.NewBeaconStateAltair()
	require.NoError(t, err)
	vals := make([]*ethpb.Validator, 4)
	vals[0] = &ethpb.Validator{PublicKey: bytesutil.PadTo([]byte{0xf0}, 48)}
	vals[1] = &ethpb.Validator{PublicKey: bytesutil.PadTo([]byte{0xf1}, 48)}
	vals[2] = &ethpb.Validator{PublicKey: bytesutil.PadTo([]byte{0xf2}, 48)}
	vals[3] = &ethpb.Validator{PublicKey: bytesutil.PadTo([]byte{0xf3}, 48)}
	require.NoError(t, st.SetValidators(vals))
	sc := &ethpb.SyncCommittee{
		Pubkeys: make([][]byte, params.BeaconConfig().SyncCommitteeSize),
	}
	sc.Pubkeys[0] = vals[0].PublicKey
	sc.Pubkeys[1] = vals[1].PublicKey
	sc.Pubkeys[2] = vals[2].PublicKey
	sc.Pubkeys[3] = vals[3].PublicKey
	require.NoError(t, st.SetCurrentSyncCommittee(sc))
	proposerServer := &Server{
		HeadFetcher:       &chainmock.ChainService{State: st},
		SyncChecker:       &mockSync.Sync{IsSyncing: false},
		SyncCommitteePool: synccommittee.NewStore(),
	}

	r := params.BeaconConfig().ZeroHash
	msgs := []*ethpb.SyncCommitteeMessage{
		{Slot: 1, BlockRoot: r[:], ValidatorIndex: 0, Signature: bls.NewAggregateSignature().Marshal()},
		{Slot: 1, BlockRoot: r[:], ValidatorIndex: 1, Signature: bls.NewAggregateSignature().Marshal()},
		{Slot: 1, BlockRoot: r[:], ValidatorIndex: 2, Signature: bls.NewAggregateSignature().Marshal()},
		{Slot: 1, BlockRoot: r[:], ValidatorIndex: 3, Signature: bls.NewAggregateSignature().Marshal()},
	}
	for _, msg := range msgs {
		require.NoError(t, proposerServer.SyncCommitteePool.SaveSyncCommitteeMessage(msg))
	}
	subcommitteeAggBits := ethpb.NewSyncCommitteeAggregationBits()
	subcommitteeAggBits.SetBitAt(3, true)
	cont := &ethpb.SyncCommitteeContribution{
		Slot:              1,
		SubcommitteeIndex: 0,
		Signature:         bls.NewAggregateSignature().Marshal(),
		AggregationBits:   subcommitteeAggBits,
		BlockRoot:         r[:],
	}

	aggregated, err := proposerServer.aggregatedSyncCommitteeMessages(t.Context(), 1, r, []*ethpb.SyncCommitteeContribution{cont})
	require.NoError(t, err)
	require.Equal(t, 1, len(aggregated))
	assert.Equal(t, false, aggregated[0].AggregationBits.BitAt(3))
}
