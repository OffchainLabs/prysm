package gloas_test

import (
	"bytes"
	"sync"
	"testing"
	"time"

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
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
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

// ptcSeed mirrors the seed derivation in PayloadCommittee so the test can
// pre-mark the seed as in-progress.
func ptcSeed(t *testing.T, st state.ReadOnlyBeaconState, slot primitives.Slot) [32]byte {
	epoch := slots.ToEpoch(slot)
	seed, err := helpers.Seed(st, epoch, params.BeaconConfig().DomainPTCAttester)
	require.NoError(t, err)
	return hash.Hash(append(seed[:], bytesutil.Bytes8(uint64(slot))...))
}

// TestPayloadCommittee_ConcurrentInProgress verifies that when another
// goroutine holds the in-progress lock and then releases WITHOUT populating
// the cache (simulating a failed computation), PayloadCommittee falls through
// and computes the result itself instead of returning an error.
func TestPayloadCommittee_ConcurrentInProgress(t *testing.T) {
	helpers.ClearCache()
	setupTestConfig(t)

	_, pk1 := newKey(t)
	_, pk2 := newKey(t)
	vals := []*eth.Validator{activeValidator(pk1), activeValidator(pk2)}
	st := newTestState(t, vals, 2)

	slot := primitives.Slot(1)
	seed := ptcSeed(t, st, slot)

	// Simulate another goroutine holding the lock.
	require.NoError(t, helpers.MarkPayloadCommitteeInProgress(seed))

	// Release the lock after a short delay WITHOUT adding to cache,
	// simulating the other goroutine failing.
	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = helpers.MarkPayloadCommitteeNotInProgress(seed)
	}()

	// PayloadCommittee should wait, see no cache entry, and compute itself.
	ptc, err := gloas.PayloadCommittee(t.Context(), st, slot)
	require.NoError(t, err)
	assert.Equal(t, true, len(ptc) > 0, "expected non-empty PTC")
}

// TestPayloadCommittee_ConcurrentCacheHit verifies that when another goroutine
// holds the in-progress lock and then populates the cache, a concurrent caller
// gets the cached result.
func TestPayloadCommittee_ConcurrentCacheHit(t *testing.T) {
	helpers.ClearCache()
	setupTestConfig(t)

	_, pk1 := newKey(t)
	_, pk2 := newKey(t)
	vals := []*eth.Validator{activeValidator(pk1), activeValidator(pk2)}
	st := newTestState(t, vals, 2)

	slot := primitives.Slot(1)
	seed := ptcSeed(t, st, slot)

	// First, compute the expected result.
	expected, err := gloas.PayloadCommittee(t.Context(), st, slot)
	require.NoError(t, err)
	helpers.ClearCache()

	// Simulate another goroutine that will populate the cache.
	require.NoError(t, helpers.MarkPayloadCommitteeInProgress(seed))
	go func() {
		time.Sleep(20 * time.Millisecond)
		helpers.AddPayloadCommittee(seed, expected)
		_ = helpers.MarkPayloadCommitteeNotInProgress(seed)
	}()

	ptc, err := gloas.PayloadCommittee(t.Context(), st, slot)
	require.NoError(t, err)
	assert.DeepEqual(t, expected, ptc)
}

// TestPayloadCommittee_ParallelCallers verifies that multiple concurrent
// callers all get the correct result without errors.
func TestPayloadCommittee_ParallelCallers(t *testing.T) {
	helpers.ClearCache()
	setupTestConfig(t)

	_, pk1 := newKey(t)
	_, pk2 := newKey(t)
	vals := []*eth.Validator{activeValidator(pk1), activeValidator(pk2)}
	st := newTestState(t, vals, 2)

	slot := primitives.Slot(1)

	// Compute expected result first.
	expected, err := gloas.PayloadCommittee(t.Context(), st, slot)
	require.NoError(t, err)
	helpers.ClearCache()

	const numCallers = 8
	var wg sync.WaitGroup
	errs := make([]error, numCallers)
	results := make([][]primitives.ValidatorIndex, numCallers)

	for i := 0; i < numCallers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = gloas.PayloadCommittee(t.Context(), st, slot)
		}(i)
	}

	wg.Wait()
	for i := 0; i < numCallers; i++ {
		require.NoError(t, errs[i], "caller %d returned error", i)
		assert.DeepEqual(t, expected, results[i], "caller %d got wrong result", i)
	}
}
