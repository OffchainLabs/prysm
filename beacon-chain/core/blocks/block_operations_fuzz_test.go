package blocks

import (
	"testing"

	v "github.com/OffchainLabs/prysm/v6/beacon-chain/core/validators"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/fuzz"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	gofuzz "github.com/google/gofuzz"
)

func TestFuzzProcessAttestationNoVerify_10000(t *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	ctx := t.Context()
	state := &ethpb.BeaconState{}
	att := &ethpb.Attestation{}

	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(att)
		s, err := state_native.InitializeFromProtoUnsafePhase0(state)
		require.NoError(t, err)
		_, err = ProcessAttestationNoVerifySignature(ctx, s, att)
		_ = err
		fuzz.FreeMemory(i)
	}
}

func TestFuzzProcessBlockHeader_10000(t *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	state := &ethpb.BeaconState{}
	block := &ethpb.SignedBeaconBlock{}

	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(block)

		s, err := state_native.InitializeFromProtoUnsafePhase0(state)
		require.NoError(t, err)
		if block.Block == nil || block.Block.Body == nil || block.Block.Body.Eth1Data == nil {
			continue
		}
		wsb, err := blocks.NewSignedBeaconBlock(block)
		require.NoError(t, err)
		_, err = ProcessBlockHeader(t.Context(), s, wsb)
		_ = err
		fuzz.FreeMemory(i)
	}
}

func TestFuzzverifyDepositDataSigningRoot_10000(_ *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	var ba []byte
	var pubkey [fieldparams.BLSPubkeyLength]byte
	var sig [96]byte
	var domain [4]byte
	var p []byte
	var s []byte
	var d []byte
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(&ba)
		fuzzer.Fuzz(&pubkey)
		fuzzer.Fuzz(&sig)
		fuzzer.Fuzz(&domain)
		fuzzer.Fuzz(&p)
		fuzzer.Fuzz(&s)
		fuzzer.Fuzz(&d)
		err := verifySignature(ba, pubkey[:], sig[:], domain[:])
		_ = err
		err = verifySignature(ba, p, s, d)
		_ = err
	}
}

func TestFuzzProcessEth1DataInBlock_10000(t *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	e := &ethpb.Eth1Data{}
	state, err := state_native.InitializeFromProtoUnsafePhase0(&ethpb.BeaconState{})
	require.NoError(t, err)
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(e)
		s, err := ProcessEth1DataInBlock(t.Context(), state, e)
		if err != nil && s != nil {
			t.Fatalf("state should be nil on err. found: %v on error: %v for state: %v and eth1data: %v", s, err, state, e)
		}
	}
}

func TestFuzzareEth1DataEqual_10000(_ *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	eth1data := &ethpb.Eth1Data{}
	eth1data2 := &ethpb.Eth1Data{}

	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(eth1data)
		fuzzer.Fuzz(eth1data2)
		AreEth1DataEqual(eth1data, eth1data2)
		AreEth1DataEqual(eth1data, eth1data)
	}
}

func TestFuzzEth1DataHasEnoughSupport_10000(t *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	eth1data := &ethpb.Eth1Data{}
	var stateVotes []*ethpb.Eth1Data
	for i := 0; i < 100000; i++ {
		fuzzer.Fuzz(eth1data)
		fuzzer.Fuzz(&stateVotes)
		s, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{
			Eth1DataVotes: stateVotes,
		})
		require.NoError(t, err)
		_, err = Eth1DataHasEnoughSupport(s, eth1data)
		_ = err
		fuzz.FreeMemory(i)
	}

}

func TestFuzzProcessBlockHeaderNoVerify_10000(t *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	state := &ethpb.BeaconState{}
	block := &ethpb.BeaconBlock{}

	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(block)
		s, err := state_native.InitializeFromProtoUnsafePhase0(state)
		require.NoError(t, err)
		_, err = ProcessBlockHeaderNoVerify(t.Context(), s, block.Slot, block.ProposerIndex, block.ParentRoot, []byte{})
		_ = err
		fuzz.FreeMemory(i)
	}
}

func TestFuzzProcessRandao_10000(t *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	state := &ethpb.BeaconState{}
	b := &ethpb.SignedBeaconBlock{}

	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(b)
		s, err := state_native.InitializeFromProtoUnsafePhase0(state)
		require.NoError(t, err)
		if b.Block == nil || b.Block.Body == nil {
			continue
		}
		wsb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		r, err := ProcessRandao(t.Context(), s, wsb)
		if err != nil && r != nil {
			t.Fatalf("return value should be nil on err. found: %v on error: %v for state: %v and block: %v", r, err, state, b)
		}
		fuzz.FreeMemory(i)
	}
}

func TestFuzzProcessRandaoNoVerify_10000(t *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	state := &ethpb.BeaconState{}
	blockBody := &ethpb.BeaconBlockBody{}

	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(blockBody)
		s, err := state_native.InitializeFromProtoUnsafePhase0(state)
		require.NoError(t, err)
		r, err := ProcessRandaoNoVerify(s, blockBody.RandaoReveal)
		if err != nil && r != nil {
			t.Fatalf("return value should be nil on err. found: %v on error: %v for state: %v and block: %v", r, err, state, blockBody)
		}
		fuzz.FreeMemory(i)
	}
}

func TestFuzzProcessProposerSlashings_10000(t *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	state := &ethpb.BeaconState{}
	p := &ethpb.ProposerSlashing{}
	ctx := t.Context()
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(p)
		s, err := state_native.InitializeFromProtoUnsafePhase0(state)
		require.NoError(t, err)
		r, err := ProcessProposerSlashings(ctx, s, []*ethpb.ProposerSlashing{p}, v.ExitInformation(s))
		if err != nil && r != nil {
			t.Fatalf("return value should be nil on err. found: %v on error: %v for state: %v and slashing: %v", r, err, state, p)
		}
		fuzz.FreeMemory(i)
	}
}

func TestFuzzVerifyProposerSlashing_10000(t *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	state := &ethpb.BeaconState{}
	proposerSlashing := &ethpb.ProposerSlashing{}
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(proposerSlashing)
		s, err := state_native.InitializeFromProtoUnsafePhase0(state)
		require.NoError(t, err)
		err = VerifyProposerSlashing(s, proposerSlashing)
		_ = err
		fuzz.FreeMemory(i)
	}
}

func TestFuzzProcessAttesterSlashings_10000(t *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	state := &ethpb.BeaconState{}
	a := &ethpb.AttesterSlashing{}
	ctx := t.Context()
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(a)
		s, err := state_native.InitializeFromProtoUnsafePhase0(state)
		require.NoError(t, err)
		r, err := ProcessAttesterSlashings(ctx, s, []ethpb.AttSlashing{a}, v.ExitInformation(s))
		if err != nil && r != nil {
			t.Fatalf("return value should be nil on err. found: %v on error: %v for state: %v and slashing: %v", r, err, state, a)
		}
		fuzz.FreeMemory(i)
	}
}

func TestFuzzVerifyAttesterSlashing_10000(t *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	state := &ethpb.BeaconState{}
	attesterSlashing := &ethpb.AttesterSlashing{}
	ctx := t.Context()
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(attesterSlashing)
		s, err := state_native.InitializeFromProtoUnsafePhase0(state)
		require.NoError(t, err)
		err = VerifyAttesterSlashing(ctx, s, attesterSlashing)
		_ = err
		fuzz.FreeMemory(i)
	}
}

func TestFuzzIsSlashableAttestationData_10000(_ *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	attestationData := &ethpb.AttestationData{}
	attestationData2 := &ethpb.AttestationData{}

	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(attestationData)
		fuzzer.Fuzz(attestationData2)
		IsSlashableAttestationData(attestationData, attestationData2)
	}
}

func TestFuzzslashableAttesterIndices_10000(_ *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	attesterSlashing := &ethpb.AttesterSlashing{}

	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(attesterSlashing)
		SlashableAttesterIndices(attesterSlashing)
	}
}

func TestFuzzProcessAttestationsNoVerify_10000(t *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	state := &ethpb.BeaconState{}
	b := &ethpb.SignedBeaconBlock{}
	ctx := t.Context()
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(b)
		s, err := state_native.InitializeFromProtoUnsafePhase0(state)
		require.NoError(t, err)
		if b.Block == nil || b.Block.Body == nil {
			continue
		}
		wsb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		r, err := ProcessAttestationsNoVerifySignature(ctx, s, wsb.Block())
		if err != nil && r != nil {
			t.Fatalf("return value should be nil on err. found: %v on error: %v for state: %v and block: %v", r, err, state, b)
		}
		fuzz.FreeMemory(i)
	}
}

func TestFuzzVerifyIndexedAttestationn_10000(t *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	state := &ethpb.BeaconState{}
	idxAttestation := &ethpb.IndexedAttestation{}
	ctx := t.Context()
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(idxAttestation)
		s, err := state_native.InitializeFromProtoUnsafePhase0(state)
		require.NoError(t, err)
		err = VerifyIndexedAttestation(ctx, s, idxAttestation)
		_ = err
		fuzz.FreeMemory(i)
	}
}

func TestFuzzverifyDeposit_10000(t *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	state := &ethpb.BeaconState{}
	deposit := &ethpb.Deposit{}
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(deposit)
		s, err := state_native.InitializeFromProtoUnsafePhase0(state)
		require.NoError(t, err)
		err = VerifyDeposit(s, deposit)
		_ = err
		fuzz.FreeMemory(i)
	}
}

func TestFuzzProcessVoluntaryExits_10000(t *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	state := &ethpb.BeaconState{}
	e := &ethpb.SignedVoluntaryExit{}
	ctx := t.Context()
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(e)
		s, err := state_native.InitializeFromProtoUnsafePhase0(state)
		require.NoError(t, err)
		r, err := ProcessVoluntaryExits(ctx, s, []*ethpb.SignedVoluntaryExit{e}, v.ExitInformation(s))
		if err != nil && r != nil {
			t.Fatalf("return value should be nil on err. found: %v on error: %v for state: %v and exit: %v", r, err, state, e)
		}
		fuzz.FreeMemory(i)
	}
}

func TestFuzzProcessVoluntaryExitsNoVerify_10000(t *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	state := &ethpb.BeaconState{}
	e := &ethpb.SignedVoluntaryExit{}
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(e)
		s, err := state_native.InitializeFromProtoUnsafePhase0(state)
		require.NoError(t, err)
		r, err := ProcessVoluntaryExits(t.Context(), s, []*ethpb.SignedVoluntaryExit{e}, v.ExitInformation(s))
		if err != nil && r != nil {
			t.Fatalf("return value should be nil on err. found: %v on error: %v for state: %v and block: %v", r, err, state, e)
		}
		fuzz.FreeMemory(i)
	}
}

func TestFuzzVerifyExit_10000(t *testing.T) {
	fuzzer := gofuzz.NewWithSeed(0)
	ve := &ethpb.SignedVoluntaryExit{}
	rawVal := &ethpb.Validator{}
	fork := &ethpb.Fork{}
	var slot primitives.Slot

	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(ve)
		fuzzer.Fuzz(rawVal)
		fuzzer.Fuzz(fork)
		fuzzer.Fuzz(&slot)

		state := &ethpb.BeaconState{
			Slot:                  slot,
			Fork:                  fork,
			GenesisValidatorsRoot: params.BeaconConfig().ZeroHash[:],
		}
		s, err := state_native.InitializeFromProtoUnsafePhase0(state)
		require.NoError(t, err)

		val, err := state_native.NewValidator(&ethpb.Validator{})
		_ = err
		err = VerifyExitAndSignature(val, s, ve)
		_ = err
		fuzz.FreeMemory(i)
	}
}
