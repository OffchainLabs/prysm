package gloas_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"slices"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
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
	t.Helper()

	st, err := testutil.NewBeaconStateGloas(func(seed *eth.BeaconStateGloas) error {
		seed.Slot = slot
		seed.Validators = vals
		seed.Balances = make([]uint64, len(vals))
		for i, v := range vals {
			seed.Balances[i] = v.EffectiveBalance
		}
		seed.PtcWindow = deterministicPTCWindow(len(vals))
		return nil
	})
	require.NoError(t, err)
	return st
}

func newPhase0TestState(t *testing.T, vals []*eth.Validator, slot primitives.Slot) state.BeaconState {
	t.Helper()

	st, err := testutil.NewBeaconState()
	require.NoError(t, err)
	for _, v := range vals {
		require.NoError(t, st.AppendValidator(v))
		require.NoError(t, st.AppendBalance(v.EffectiveBalance))
	}
	require.NoError(t, st.SetSlot(slot))
	return st
}

func deterministicPTCWindow(validatorCount int) []*eth.PTCs {
	window := make([]*eth.PTCs, 3*params.BeaconConfig().SlotsPerEpoch)
	indices := make([]primitives.ValidatorIndex, fieldparams.PTCSize)
	if validatorCount > 0 {
		for i := range indices {
			indices[i] = primitives.ValidatorIndex(i % validatorCount)
		}
	}
	for i := range window {
		window[i] = &eth.PTCs{
			ValidatorIndices: slices.Clone(indices),
		}
	}
	return window
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

func TestProcessPTCWindow(t *testing.T) {
	fuluSt, _ := testutil.DeterministicGenesisStateFulu(t, 256)
	st, err := gloas.UpgradeToGloas(t.Context(), fuluSt)
	require.NoError(t, err)

	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch

	// Get original window.
	origWindow, err := st.PTCWindow()
	require.NoError(t, err)
	windowSize := int(slotsPerEpoch.Mul(uint64(2 + params.BeaconConfig().MinSeedLookahead)))
	require.Equal(t, windowSize, len(origWindow))

	// Advance state to next epoch boundary so process_ptc_window sees a new epoch.
	require.NoError(t, st.SetSlot(slotsPerEpoch))

	// Process PTC window — should rotate.
	require.NoError(t, gloas.ProcessPTCWindow(t.Context(), st))

	newWindow, err := st.PTCWindow()
	require.NoError(t, err)
	require.Equal(t, windowSize, len(newWindow))

	// The first two epochs should be the old epochs 1 and 2 (shifted left by one epoch).
	for i := range 2 * slotsPerEpoch {
		require.DeepEqual(t, origWindow[slotsPerEpoch+i], newWindow[i])
	}

	// The last epoch should be freshly computed — not all zeros.
	lastStart := 2 * slotsPerEpoch
	for i := range slotsPerEpoch {
		ptcSlot := newWindow[lastStart+i]
		require.NotNil(t, ptcSlot)
		nonZero := false
		for _, idx := range ptcSlot.ValidatorIndices {
			if idx != 0 {
				nonZero = true
				break
			}
		}
		require.Equal(t, true, nonZero, "last epoch slot %d should have non-zero validator indices", i)
	}
}

// TestProcessPTCWindow_GoldenVector pins a fingerprint of the validator
// indices produced by the balance-weighted PTC selection for a deterministic
// state, guarding against unintended behavioral drift.
func TestProcessPTCWindow_GoldenVector(t *testing.T) {
	fuluSt, _ := testutil.DeterministicGenesisStateFulu(t, 256)
	st, err := gloas.UpgradeToGloas(t.Context(), fuluSt)
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(params.BeaconConfig().SlotsPerEpoch))
	require.NoError(t, gloas.ProcessPTCWindow(t.Context(), st))

	window, err := st.PTCWindow()
	require.NoError(t, err)

	// Fingerprint the newly-computed (last) epoch by sha256'ing the little-endian
	// encoding of every validator index, slot-by-slot.
	slotsPerEpoch := int(params.BeaconConfig().SlotsPerEpoch)
	lastStart := len(window) - slotsPerEpoch
	h := sha256.New()
	var buf [8]byte
	for i := range slotsPerEpoch {
		for _, idx := range window[lastStart+i].ValidatorIndices {
			binary.LittleEndian.PutUint64(buf[:], uint64(idx))
			h.Write(buf[:])
		}
	}
	const expected = "bfdb357dbb3f2abe4bba9a0d5d0d6d8ae9e19335e83f64f21b5a2f727a0f3ee9"
	require.Equal(t, expected, hex.EncodeToString(h.Sum(nil)))
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

// BenchmarkProcessPTCWindow measures end-to-end PTC window rotation, which
// is dominated by the balance-weighted selection loop.
//
// Apple M4 Pro, mainnet config, 2048 validators:
//
//	before caching: 87.64 ms/op   136.9 MiB/op   1.230M allocs/op
//	with caching:   47.20 ms/op   136.9 MiB/op   1.230M allocs/op
//
// The caching optimization (hash(seed || i/16) reused across 16 rounds)
// cuts wall time by ~46% with no change to allocation profile — exactly
// what's expected: we skipped SHA256 work, not memory traffic.
func BenchmarkProcessPTCWindow(b *testing.B) {
	fuluSt, _ := testutil.DeterministicGenesisStateFulu(b, 2048)
	st, err := gloas.UpgradeToGloas(b.Context(), fuluSt)
	require.NoError(b, err)
	require.NoError(b, st.SetSlot(params.BeaconConfig().SlotsPerEpoch))

	ctx := b.Context()
	b.ResetTimer()
	for b.Loop() {
		if err := gloas.ProcessPTCWindow(ctx, st); err != nil {
			b.Fatal(err)
		}
	}
}
