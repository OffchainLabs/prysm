package gloas_test

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	testutil "github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

func TestProcessPayloadAttestations_WrongParent(t *testing.T) {
	setupTestConfig(t)

	_, pk := newKey(t)
	st := newTestState(t, []*eth.Validator{activeValidator(pk)}, 1)
	require.NoError(t, st.SetSlot(2))
	parentRoot := bytes.Repeat([]byte{0xaa}, 32)
	require.NoError(t, st.SetLatestBlockHeader(&eth.BeaconBlockHeader{ParentRoot: parentRoot}))

	att := &eth.PayloadAttestation{
		Data: &eth.PayloadAttestationData{
			BeaconBlockRoot: bytes.Repeat([]byte{0xbb}, 32),
			Slot:            1,
		},
		AggregationBits: bitfield.NewBitvector512(),
		Signature:       make([]byte, 96),
	}
	body := buildBody(t, att)

	err := gloas.ProcessPayloadAttestations(t.Context(), st, body)
	require.ErrorContains(t, "wrong parent", err)
}

func TestProcessPayloadAttestations_WrongSlot(t *testing.T) {
	setupTestConfig(t)

	_, pk := newKey(t)
	st := newTestState(t, []*eth.Validator{activeValidator(pk)}, 1)
	require.NoError(t, st.SetSlot(3))
	parentRoot := bytes.Repeat([]byte{0xaa}, 32)
	require.NoError(t, st.SetLatestBlockHeader(&eth.BeaconBlockHeader{ParentRoot: parentRoot}))

	att := &eth.PayloadAttestation{
		Data: &eth.PayloadAttestationData{
			BeaconBlockRoot: parentRoot,
			Slot:            1,
		},
		AggregationBits: bitfield.NewBitvector512(),
		Signature:       make([]byte, 96),
	}
	body := buildBody(t, att)

	err := gloas.ProcessPayloadAttestations(t.Context(), st, body)
	require.ErrorContains(t, "wrong slot", err)
}

func TestProcessPayloadAttestations_InvalidSignature(t *testing.T) {
	setupTestConfig(t)

	_, pk1 := newKey(t)
	sk2, pk2 := newKey(t)
	vals := []*eth.Validator{activeValidator(pk1), activeValidator(pk2)}
	st := newTestState(t, vals, 2)
	parentRoot := bytes.Repeat([]byte{0xaa}, 32)
	require.NoError(t, st.SetLatestBlockHeader(&eth.BeaconBlockHeader{ParentRoot: parentRoot}))

	attData := &eth.PayloadAttestationData{
		BeaconBlockRoot: parentRoot,
		Slot:            1,
	}
	att := &eth.PayloadAttestation{
		Data:            attData,
		AggregationBits: setBits(bitfield.NewBitvector512(), 0),
		Signature:       signAttestation(t, st, attData, []common.SecretKey{sk2}),
	}
	body := buildBody(t, att)

	err := gloas.ProcessPayloadAttestations(t.Context(), st, body)
	require.ErrorContains(t, "failed to verify indexed form", err)
	require.ErrorContains(t, "invalid signature", err)
}

func TestProcessPayloadAttestations_EmptyAggregationBits(t *testing.T) {
	setupTestConfig(t)

	_, pk := newKey(t)
	st := newTestState(t, []*eth.Validator{activeValidator(pk)}, 1)
	require.NoError(t, st.SetSlot(2))
	parentRoot := bytes.Repeat([]byte{0xaa}, 32)
	require.NoError(t, st.SetLatestBlockHeader(&eth.BeaconBlockHeader{ParentRoot: parentRoot}))

	attData := &eth.PayloadAttestationData{
		BeaconBlockRoot: parentRoot,
		Slot:            1,
	}
	att := &eth.PayloadAttestation{
		Data:            attData,
		AggregationBits: bitfield.NewBitvector512(),
		Signature:       make([]byte, 96),
	}
	body := buildBody(t, att)

	err := gloas.ProcessPayloadAttestations(t.Context(), st, body)
	require.ErrorContains(t, "failed to verify indexed form", err)
	require.ErrorContains(t, "attesting indices empty or unsorted", err)
}

func TestProcessPayloadAttestations_HappyPath(t *testing.T) {
	helpers.ClearCache()
	setupTestConfig(t)

	sk1, pk1 := newKey(t)
	sk2, pk2 := newKey(t)
	vals := []*eth.Validator{activeValidator(pk1), activeValidator(pk2)}

	st := newTestState(t, vals, 2)
	parentRoot := bytes.Repeat([]byte{0xaa}, 32)
	require.NoError(t, st.SetLatestBlockHeader(&eth.BeaconBlockHeader{ParentRoot: parentRoot}))

	attData := &eth.PayloadAttestationData{
		BeaconBlockRoot: parentRoot,
		Slot:            1,
	}
	aggBits := bitfield.NewBitvector512()
	aggBits.SetBitAt(0, true)
	aggBits.SetBitAt(1, true)

	att := &eth.PayloadAttestation{
		Data:            attData,
		AggregationBits: aggBits,
		Signature:       signAttestation(t, st, attData, []common.SecretKey{sk1, sk2}),
	}
	body := buildBody(t, att)

	err := gloas.ProcessPayloadAttestations(t.Context(), st, body)
	require.NoError(t, err)
}

func TestProcessPayloadAttestations_MultipleAttestations(t *testing.T) {
	helpers.ClearCache()
	setupTestConfig(t)

	sk1, pk1 := newKey(t)
	sk2, pk2 := newKey(t)
	vals := []*eth.Validator{activeValidator(pk1), activeValidator(pk2)}

	st := newTestState(t, vals, 2)
	parentRoot := bytes.Repeat([]byte{0xaa}, 32)
	require.NoError(t, st.SetLatestBlockHeader(&eth.BeaconBlockHeader{ParentRoot: parentRoot}))

	attData1 := &eth.PayloadAttestationData{
		BeaconBlockRoot: parentRoot,
		Slot:            1,
	}
	attData2 := &eth.PayloadAttestationData{
		BeaconBlockRoot: parentRoot,
		Slot:            1,
	}

	att1 := &eth.PayloadAttestation{
		Data:            attData1,
		AggregationBits: setBits(bitfield.NewBitvector512(), 0),
		Signature:       signAttestation(t, st, attData1, []common.SecretKey{sk1}),
	}
	att2 := &eth.PayloadAttestation{
		Data:            attData2,
		AggregationBits: setBits(bitfield.NewBitvector512(), 1),
		Signature:       signAttestation(t, st, attData2, []common.SecretKey{sk2}),
	}

	body := buildBody(t, att1, att2)

	err := gloas.ProcessPayloadAttestations(t.Context(), st, body)
	require.NoError(t, err)
}

func TestProcessPayloadAttestations_IndexedVerificationError(t *testing.T) {
	setupTestConfig(t)

	_, pk := newKey(t)
	st := newTestState(t, []*eth.Validator{activeValidator(pk)}, 1)
	parentRoot := bytes.Repeat([]byte{0xaa}, 32)
	require.NoError(t, st.SetLatestBlockHeader(&eth.BeaconBlockHeader{ParentRoot: parentRoot}))

	attData := &eth.PayloadAttestationData{
		BeaconBlockRoot: parentRoot,
		Slot:            0,
	}
	att := &eth.PayloadAttestation{
		Data:            attData,
		AggregationBits: setBits(bitfield.NewBitvector512(), 0),
		Signature:       make([]byte, 96),
	}
	body := buildBody(t, att)

	errState := &validatorLookupErrState{
		BeaconState: st,
		errIndex:    0,
	}
	err := gloas.ProcessPayloadAttestations(t.Context(), errState, body)
	require.ErrorContains(t, "failed to verify indexed form", err)
	require.ErrorContains(t, "validator 0", err)
}

func newTestState(t *testing.T, vals []*eth.Validator, slot primitives.Slot) state.BeaconState {
	st, err := testutil.NewBeaconState()
	require.NoError(t, err)
	for _, v := range vals {
		require.NoError(t, st.AppendValidator(v))
		require.NoError(t, st.AppendBalance(v.EffectiveBalance))
	}
	require.NoError(t, st.SetSlot(slot))
	require.NoError(t, helpers.UpdateCommitteeCache(t.Context(), st, slots.ToEpoch(slot)))
	return st
}

func setupTestConfig(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotsPerEpoch = 1
	cfg.MaxEffectiveBalanceElectra = cfg.MaxEffectiveBalance
	params.OverrideBeaconConfig(cfg)
}

func buildBody(t *testing.T, atts ...*eth.PayloadAttestation) interfaces.ReadOnlyBeaconBlockBody {
	body := &eth.BeaconBlockBodyGloas{
		PayloadAttestations:   atts,
		RandaoReveal:          make([]byte, 96),
		Eth1Data:              &eth.Eth1Data{},
		Graffiti:              make([]byte, 32),
		ProposerSlashings:     []*eth.ProposerSlashing{},
		AttesterSlashings:     []*eth.AttesterSlashingElectra{},
		Attestations:          []*eth.AttestationElectra{},
		Deposits:              []*eth.Deposit{},
		VoluntaryExits:        []*eth.SignedVoluntaryExit{},
		SyncAggregate:         &eth.SyncAggregate{},
		BlsToExecutionChanges: []*eth.SignedBLSToExecutionChange{},
	}
	wrapped, err := blocks.NewBeaconBlockBody(body)
	require.NoError(t, err)
	return wrapped
}

func setBits(bits bitfield.Bitvector512, idx uint64) bitfield.Bitvector512 {
	bits.SetBitAt(idx, true)
	return bits
}

func activeValidator(pub []byte) *eth.Validator {
	return &eth.Validator{
		PublicKey:                  pub,
		EffectiveBalance:           params.BeaconConfig().MaxEffectiveBalance,
		WithdrawalCredentials:      make([]byte, 32),
		ActivationEligibilityEpoch: 0,
		ActivationEpoch:            0,
		ExitEpoch:                  params.BeaconConfig().FarFutureEpoch,
		WithdrawableEpoch:          params.BeaconConfig().FarFutureEpoch,
	}
}

func newKey(t *testing.T) (common.SecretKey, []byte) {
	sk, err := bls.RandKey()
	require.NoError(t, err)
	return sk, sk.PublicKey().Marshal()
}

func signAttestation(t *testing.T, st state.ReadOnlyBeaconState, data *eth.PayloadAttestationData, sks []common.SecretKey) []byte {
	domain, err := signing.Domain(st.Fork(), slots.ToEpoch(data.Slot), params.BeaconConfig().DomainPTCAttester, st.GenesisValidatorsRoot())
	require.NoError(t, err)
	root, err := signing.ComputeSigningRoot(data, domain)
	require.NoError(t, err)

	sigs := make([]common.Signature, len(sks))
	for i, sk := range sks {
		sigs[i] = sk.Sign(root[:])
	}
	agg := bls.AggregateSignatures(sigs)
	return agg.Marshal()
}

func TestPTCDuties(t *testing.T) {
	helpers.ClearCache()
	setupTestConfig(t)

	// Create state with enough validators.
	numVals := 100
	vals := make([]*eth.Validator, numVals)
	for i := range numVals {
		_, pk := newKey(t)
		vals[i] = activeValidator(pk)
	}
	st := newTestState(t, vals, 0)

	t.Run("returns duties for validators in PTC", func(t *testing.T) {
		// Request duties for all validators.
		requested := make(map[primitives.ValidatorIndex]struct{})
		for i := range numVals {
			requested[primitives.ValidatorIndex(i)] = struct{}{}
		}

		duties, err := gloas.PTCDuties(t.Context(), st, requested)
		require.NoError(t, err)
		require.NotEmpty(t, duties, "Should return some duties")

		// Verify all returned duties are for requested validators.
		for _, duty := range duties {
			_, ok := requested[duty.ValidatorIndex]
			require.Equal(t, true, ok, "Returned validator should be in requested set")
		}
	})

	t.Run("returns empty for empty request", func(t *testing.T) {
		requested := make(map[primitives.ValidatorIndex]struct{})
		duties, err := gloas.PTCDuties(t.Context(), st, requested)
		require.NoError(t, err)
		require.Equal(t, 0, len(duties), "Should return no duties for empty request")
	})

	t.Run("returns empty for validators not in any PTC", func(t *testing.T) {
		// Request duties for validators that don't exist.
		requested := make(map[primitives.ValidatorIndex]struct{})
		for i := 1000000; i < 1000010; i++ {
			requested[primitives.ValidatorIndex(i)] = struct{}{}
		}

		duties, err := gloas.PTCDuties(t.Context(), st, requested)
		require.NoError(t, err)
		require.Equal(t, 0, len(duties), "Non-existent validators should have no duties")
	})

	t.Run("each validator has at most one duty per epoch", func(t *testing.T) {
		requested := make(map[primitives.ValidatorIndex]struct{})
		for i := range numVals {
			requested[primitives.ValidatorIndex(i)] = struct{}{}
		}

		duties, err := gloas.PTCDuties(t.Context(), st, requested)
		require.NoError(t, err)

		// Check for duplicates.
		seen := make(map[primitives.ValidatorIndex]bool)
		for _, duty := range duties {
			if seen[duty.ValidatorIndex] {
				t.Errorf("Validator %d appears in duties multiple times", duty.ValidatorIndex)
			}
			seen[duty.ValidatorIndex] = true
		}
	})

	t.Run("duties are deterministic", func(t *testing.T) {
		requested := make(map[primitives.ValidatorIndex]struct{})
		for i := range 50 {
			requested[primitives.ValidatorIndex(i)] = struct{}{}
		}

		duties1, err := gloas.PTCDuties(t.Context(), st, requested)
		require.NoError(t, err)

		duties2, err := gloas.PTCDuties(t.Context(), st, requested)
		require.NoError(t, err)

		require.Equal(t, len(duties1), len(duties2), "Should return same number of duties")
		for i := range duties1 {
			require.Equal(t, duties1[i].ValidatorIndex, duties2[i].ValidatorIndex)
			require.Equal(t, duties1[i].Slot, duties2[i].Slot)
		}
	})
}

type validatorLookupErrState struct {
	state.BeaconState
	errIndex primitives.ValidatorIndex
}

// ValidatorAtIndexReadOnly is overridden to simulate a missing validator lookup.
func (s *validatorLookupErrState) ValidatorAtIndexReadOnly(idx primitives.ValidatorIndex) (state.ReadOnlyValidator, error) {
	if idx == s.errIndex {
		return nil, state.ErrNilValidatorsInState
	}
	return s.BeaconState.ValidatorAtIndexReadOnly(idx)
}
