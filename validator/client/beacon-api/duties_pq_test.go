package beacon_api

import (
	"context"
	"errors"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// testDutiesProvider is a duties provider returning fixed duties, or failing with `err`.
type testDutiesProvider struct {
	attester *structs.GetAttesterDutiesResponse
	proposer *structs.GetProposerDutiesResponse
	ptc      *structs.GetPTCDutiesResponse
	err      error
}

func (p testDutiesProvider) AttesterDuties(context.Context, primitives.Epoch, []primitives.ValidatorIndex) (*structs.GetAttesterDutiesResponse, error) {
	return p.attester, p.err
}

func (p testDutiesProvider) ProposerDuties(context.Context, primitives.Epoch) (*structs.GetProposerDutiesResponse, error) {
	return p.proposer, p.err
}

func (p testDutiesProvider) SyncDuties(context.Context, primitives.Epoch, []primitives.ValidatorIndex) ([]*structs.SyncCommitteeDuty, error) {
	return nil, errors.New("sync duties requested in the PQ fork")
}

func (p testDutiesProvider) PTCDuties(context.Context, primitives.Epoch, []primitives.ValidatorIndex) (*structs.GetPTCDutiesResponse, error) {
	return p.ptc, p.err
}

func (p testDutiesProvider) Committees(context.Context, primitives.Epoch) ([]*structs.Committee, error) {
	return []*structs.Committee{{Index: "1"}}, p.err
}

func TestPQDutiesProvider(t *testing.T) {
	registry, blsKey := testPQRegistry(t, nil)
	xmssKey, ok := registry.XMSSPubkey(blsKey)
	require.Equal(t, true, ok)

	blsHex, xmssHex := hexutil.Encode(blsKey[:]), hexutil.Encode(xmssKey[:])
	otherHex := hexutil.Encode(make([]byte, fieldparams.XMSSPubkeyLength))
	isAggregator := true

	newProvider := func(inner testDutiesProvider) pqDutiesProvider {
		return pqDutiesProvider{inner: inner, keys: registry, aggregators: newPQAggregators()}
	}

	t.Run("attester duties", func(t *testing.T) {
		provider := newProvider(testDutiesProvider{attester: &structs.GetAttesterDutiesResponse{Data: []*structs.AttesterDuty{
			{Pubkey: xmssHex, ValidatorIndex: "3", Slot: "10", IsAggregator: &isAggregator},
			{Pubkey: otherHex, ValidatorIndex: "4", Slot: "10"},
		}}})

		resp, err := provider.AttesterDuties(t.Context(), 1, []primitives.ValidatorIndex{3, 4})
		require.NoError(t, err)

		// The XMSS public key of a validator of the validator client becomes its BLS public key.
		require.Equal(t, blsHex, resp.Data[0].Pubkey)
		require.Equal(t, otherHex, resp.Data[1].Pubkey)

		// The beacon node selects the aggregators.
		require.Equal(t, true, provider.aggregators.isAggregator(10, 3))
		require.Equal(t, false, provider.aggregators.isAggregator(10, 4))
		require.Equal(t, false, provider.aggregators.isAggregator(11, 3))
	})

	t.Run("proposer duties", func(t *testing.T) {
		provider := newProvider(testDutiesProvider{proposer: &structs.GetProposerDutiesResponse{Data: []*structs.ProposerDuty{
			{Pubkey: xmssHex, ValidatorIndex: "3", Slot: "10"},
		}}})

		resp, err := provider.ProposerDuties(t.Context(), 1)
		require.NoError(t, err)
		require.Equal(t, blsHex, resp.Data[0].Pubkey)
	})

	t.Run("PTC duties", func(t *testing.T) {
		provider := newProvider(testDutiesProvider{ptc: &structs.GetPTCDutiesResponse{Data: []*structs.PTCDuty{
			{Pubkey: xmssHex, ValidatorIndex: "3", Slot: "10"},
		}}})

		resp, err := provider.PTCDuties(t.Context(), 1, []primitives.ValidatorIndex{3})
		require.NoError(t, err)
		require.Equal(t, blsHex, resp.Data[0].Pubkey)
	})

	t.Run("no sync committee", func(t *testing.T) {
		duties, err := newProvider(testDutiesProvider{}).SyncDuties(t.Context(), 1, []primitives.ValidatorIndex{3})
		require.NoError(t, err)
		require.Equal(t, 0, len(duties))
	})

	t.Run("committees", func(t *testing.T) {
		committees, err := newProvider(testDutiesProvider{}).Committees(t.Context(), 1)
		require.NoError(t, err)
		require.Equal(t, 1, len(committees))
	})

	t.Run("invalid XMSS public key", func(t *testing.T) {
		provider := newProvider(testDutiesProvider{proposer: &structs.GetProposerDutiesResponse{Data: []*structs.ProposerDuty{
			{Pubkey: blsHex, ValidatorIndex: "3", Slot: "10"},
		}}})

		_, err := provider.ProposerDuties(t.Context(), 1)
		require.ErrorContains(t, "decode XMSS public key", err)
	})

	t.Run("inner error", func(t *testing.T) {
		_, err := newProvider(testDutiesProvider{err: errors.New("boom")}).AttesterDuties(t.Context(), 1, nil)
		require.ErrorContains(t, "boom", err)
	})
}

func TestPQAggregators(t *testing.T) {
	isAggregator := true

	t.Run("invalid slot", func(t *testing.T) {
		err := newPQAggregators().set(&structs.AttesterDuty{Slot: "a", ValidatorIndex: "1"})
		require.ErrorContains(t, "parse attester slot", err)
	})

	t.Run("invalid validator index", func(t *testing.T) {
		err := newPQAggregators().set(&structs.AttesterDuty{Slot: "1", ValidatorIndex: "a"})
		require.ErrorContains(t, "parse attester validator index", err)
	})

	t.Run("forgets old slots", func(t *testing.T) {
		aggregators := newPQAggregators()
		require.NoError(t, aggregators.set(&structs.AttesterDuty{Slot: "1", ValidatorIndex: "1", IsAggregator: &isAggregator}))
		require.Equal(t, true, aggregators.isAggregator(1, 1))

		require.NoError(t, aggregators.set(&structs.AttesterDuty{Slot: "1000", ValidatorIndex: "2", IsAggregator: &isAggregator}))
		require.Equal(t, false, aggregators.isAggregator(1, 1))
		require.Equal(t, true, aggregators.isAggregator(1000, 2))
	})
}
