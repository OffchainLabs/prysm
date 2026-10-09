package proofengine

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// fixtureNewPayloadRequestRoot is the new payload request root the fixture proofs of testdata commit to: block 93354
// of glamsterdam-devnet-8, from github.com/eth-act/zkboost v0.11.1, crates/server/src/proof/zkvm/mock.
var fixtureNewPayloadRequestRoot = [32]byte(hexutil.MustDecode("0x68e2250786a622ab175a8149c63d220704e7ed9610a12b7864047510235e7bb7"))

// fakeVerifier returns publicValues for any proof, or err if set.
type fakeVerifier struct {
	publicValues []byte
	err          error
}

func (f fakeVerifier) verify([]byte) ([]byte, error) {
	return f.publicValues, f.err
}

// recordingVerifier returns publicValues for any proof, and records the proofs it verifies.
type recordingVerifier struct {
	publicValues []byte
	proofs       *[][]byte
}

func (r recordingVerifier) verify(proof []byte) ([]byte, error) {
	*r.proofs = append(*r.proofs, proof)
	return r.publicValues, nil
}

// mockProof encodes the MockProof a zkboost mock zkVM sends: the offsets of its two lists, the public values, then
// random bytes in place of a proof.
func mockProof(publicValues []byte) []byte {
	const offsetsLength = 8

	proof := binary.LittleEndian.AppendUint32(nil, offsetsLength)
	proof = binary.LittleEndian.AppendUint32(proof, uint32(offsetsLength+len(publicValues)))
	proof = append(proof, publicValues...)
	return append(proof, bytes.Repeat([]byte{0xbb}, 64)...)
}

func TestPublicInput(t *testing.T) {
	t.Run("mainnet matches the fixture public values", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		params.OverrideBeaconConfig(params.MainnetConfig())

		want, err := os.ReadFile(filepath.Join("testdata", "public_values.bin"))
		require.NoError(t, err)

		require.DeepEqual(t, want, PublicInput(fixtureNewPayloadRequestRoot))
	})

	t.Run("encodes the deposit chain ID", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.MainnetConfig()
		cfg.DepositChainID = 0x0102030405060708
		params.OverrideBeaconConfig(cfg)

		publicInput := PublicInput([32]byte{})
		require.Equal(t, publicInputLength, len(publicInput))
		require.DeepEqual(t, []byte{0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01}, publicInput[33:41])
	})
}

func TestPublicValuesMatch(t *testing.T) {
	publicInput := PublicInput(fixtureNewPayloadRequestRoot)
	padded := append(append([]byte{}, publicInput...), make([]byte, 256-len(publicInput))...)
	otherRoot := PublicInput([32]byte{0x01})

	t.Run("sp1 exact", func(t *testing.T) {
		require.Equal(t, true, publicValuesMatch(zkVMSP1, publicInput, publicInput))
	})

	t.Run("sp1 padded", func(t *testing.T) {
		require.Equal(t, false, publicValuesMatch(zkVMSP1, padded, publicInput))
	})

	t.Run("openvm padded", func(t *testing.T) {
		require.Equal(t, true, publicValuesMatch(zkVMOpenVM, padded, publicInput))
	})

	t.Run("zisk exact", func(t *testing.T) {
		require.Equal(t, true, publicValuesMatch(zkVMZisk, publicInput, publicInput))
	})

	t.Run("zisk non-zero padding", func(t *testing.T) {
		dirty := append([]byte{}, padded...)
		dirty[len(dirty)-1] = 1
		require.Equal(t, false, publicValuesMatch(zkVMZisk, dirty, publicInput))
	})

	t.Run("truncated", func(t *testing.T) {
		require.Equal(t, false, publicValuesMatch(zkVMOpenVM, publicInput[:len(publicInput)-1], publicInput))
	})

	t.Run("other root", func(t *testing.T) {
		require.Equal(t, false, publicValuesMatch(zkVMSP1, otherRoot, publicInput))
		require.Equal(t, false, publicValuesMatch(zkVMOpenVM, otherRoot, publicInput))
	})
}

func TestDefaultVerificationKeys(t *testing.T) {
	keys := defaultVerificationKeys()

	for proofType := range proofTypes() {
		key, ok := keys[proofType]
		require.Equal(t, true, ok, "no key for %s", proofType)

		kind, err := zkVMKindOf(proofType)
		require.NoError(t, err)

		wantLength := 32
		if kind == zkVMOpenVM {
			wantLength = 367
		}
		require.Equal(t, wantLength, len(key), "key length of %s", proofType)
	}
}

func TestParseVerificationKeys(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		keys, err := parseVerificationKeys([]byte(`{"execution_proofs":[{"proof_type":1,"program_vk":"0x0102"},{"proof_type":2,"program_vk":"0x03"}]}`))
		require.NoError(t, err)
		require.Equal(t, 2, len(keys))
		require.DeepEqual(t, []byte{0x01, 0x02}, keys[ethpb.ProofTypeEthrexOpenVM])
		require.DeepEqual(t, []byte{0x03}, keys[ethpb.ProofTypeEthrexSP1])
	})

	t.Run("malformed json", func(t *testing.T) {
		_, err := parseVerificationKeys([]byte(`{`))
		require.ErrorContains(t, "unmarshal", err)
	})

	t.Run("no execution proof", func(t *testing.T) {
		_, err := parseVerificationKeys([]byte(`{"execution_proofs":[]}`))
		require.ErrorContains(t, "no execution proof configured", err)
	})

	t.Run("unsupported proof type", func(t *testing.T) {
		_, err := parseVerificationKeys([]byte(`{"execution_proofs":[{"proof_type":0,"program_vk":"0x01"}]}`))
		require.ErrorContains(t, "unsupported proof type 0", err)
	})

	t.Run("duplicate proof type", func(t *testing.T) {
		_, err := parseVerificationKeys([]byte(`{"execution_proofs":[{"proof_type":1,"program_vk":"0x01"},{"proof_type":1,"program_vk":"0x02"}]}`))
		require.ErrorContains(t, "duplicate proof type", err)
	})

	t.Run("empty key", func(t *testing.T) {
		_, err := parseVerificationKeys([]byte(`{"execution_proofs":[{"proof_type":1,"program_vk":"0x"}]}`))
		require.ErrorContains(t, "empty program verification key", err)
	})

	t.Run("bad hex", func(t *testing.T) {
		_, err := parseVerificationKeys([]byte(`{"execution_proofs":[{"proof_type":1,"program_vk":"zz"}]}`))
		require.ErrorContains(t, "decode program verification key", err)
	})
}

func TestEngine_Verify(t *testing.T) {
	ctx := context.Background()
	publicInput := PublicInput(fixtureNewPayloadRequestRoot)

	newTestEngine := func(t *testing.T, verifier zkVMVerifier) *Engine {
		keys := map[ethpb.ProofType][]byte{ethpb.ProofTypeEthrexSP1: {0x01}}
		engine, err := newEngine(keys, func(zkVMKind, []byte) (zkVMVerifier, error) { return verifier, nil })
		require.NoError(t, err)
		return engine
	}

	t.Run("valid", func(t *testing.T) {
		engine := newTestEngine(t, fakeVerifier{publicValues: publicInput})
		require.NoError(t, engine.Verify(ctx, ethpb.ProofTypeEthrexSP1, []byte{0x01}, publicInput))
	})

	t.Run("unsupported proof type", func(t *testing.T) {
		engine := newTestEngine(t, fakeVerifier{publicValues: publicInput})
		require.ErrorIs(t, engine.Verify(ctx, ethpb.ProofTypeEthrexZisk, []byte{0x01}, publicInput), ErrUnsupportedProofType)
	})

	t.Run("invalid proof", func(t *testing.T) {
		engine := newTestEngine(t, fakeVerifier{err: ErrProofInvalid})
		require.ErrorIs(t, engine.Verify(ctx, ethpb.ProofTypeEthrexSP1, []byte{0x01}, publicInput), ErrProofInvalid)
	})

	t.Run("verifier failure", func(t *testing.T) {
		failure := errors.New("failure")
		engine := newTestEngine(t, fakeVerifier{err: failure})
		err := engine.Verify(ctx, ethpb.ProofTypeEthrexSP1, []byte{0x01}, publicInput)
		require.ErrorIs(t, err, failure)
		require.Equal(t, false, errors.Is(err, ErrProofInvalid))
	})

	t.Run("public values of another payload", func(t *testing.T) {
		engine := newTestEngine(t, fakeVerifier{publicValues: PublicInput([32]byte{0x01})})
		require.ErrorIs(t, engine.Verify(ctx, ethpb.ProofTypeEthrexSP1, []byte{0x01}, publicInput), ErrProofInvalid)
	})

	t.Run("mock proof", func(t *testing.T) {
		var proofs [][]byte
		standIn := []byte{0x42}
		engine := newTestEngine(t, recordingVerifier{proofs: &proofs})
		require.NoError(t, engine.enableMockProofs(map[ethpb.ProofType][]byte{ethpb.ProofTypeEthrexSP1: standIn}))
		proofs = nil

		require.NoError(t, engine.Verify(ctx, ethpb.ProofTypeEthrexSP1, mockProof(publicInput), publicInput))
		// The stand-in is verified, and its result discarded: the verifier returns no public values here.
		require.DeepEqual(t, [][]byte{standIn}, proofs)
	})

	t.Run("mock proof of another payload", func(t *testing.T) {
		var proofs [][]byte
		engine := newTestEngine(t, recordingVerifier{proofs: &proofs})
		require.NoError(t, engine.enableMockProofs(map[ethpb.ProofType][]byte{ethpb.ProofTypeEthrexSP1: {0x42}}))

		err := engine.Verify(ctx, ethpb.ProofTypeEthrexSP1, mockProof(PublicInput([32]byte{0x01})), publicInput)
		require.ErrorIs(t, err, ErrProofInvalid)
	})

	t.Run("real proof in mock mode", func(t *testing.T) {
		var proofs [][]byte
		engine := newTestEngine(t, recordingVerifier{publicValues: publicInput, proofs: &proofs})
		require.NoError(t, engine.enableMockProofs(map[ethpb.ProofType][]byte{ethpb.ProofTypeEthrexSP1: {0x42}}))
		proofs = nil

		require.NoError(t, engine.Verify(ctx, ethpb.ProofTypeEthrexSP1, []byte{0x01}, publicInput))
		require.DeepEqual(t, [][]byte{{0x01}}, proofs)
	})

	t.Run("mock proof without mock mode", func(t *testing.T) {
		engine := newTestEngine(t, fakeVerifier{err: ErrProofInvalid})
		require.ErrorIs(t, engine.Verify(ctx, ethpb.ProofTypeEthrexSP1, mockProof(publicInput), publicInput), ErrProofInvalid)
	})

	t.Run("mock mode without a stand-in", func(t *testing.T) {
		engine := newTestEngine(t, fakeVerifier{publicValues: publicInput})
		require.ErrorContains(t, "no stand-in proof", engine.enableMockProofs(nil))
	})

	t.Run("mock mode with a stand-in that does not verify", func(t *testing.T) {
		engine := newTestEngine(t, fakeVerifier{err: ErrProofInvalid})
		err := engine.enableMockProofs(map[ethpb.ProofType][]byte{ethpb.ProofTypeEthrexSP1: {0x42}})
		require.ErrorIs(t, err, ErrProofInvalid)
	})

	t.Run("cancelled context", func(t *testing.T) {
		engine := newTestEngine(t, fakeVerifier{publicValues: publicInput})
		for range cap(engine.slots) {
			engine.slots <- struct{}{}
		}

		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()
		require.ErrorIs(t, engine.Verify(cancelledCtx, ethpb.ProofTypeEthrexSP1, []byte{0x01}, publicInput), context.Canceled)
	})
}

// proofTypes returns every supported proof type.
func proofTypes() map[ethpb.ProofType]bool {
	types := make(map[ethpb.ProofType]bool)
	for i := range 256 {
		if proofType := ethpb.ProofType(i); proofType.Supported() {
			types[proofType] = true
		}
	}
	return types
}

func TestDecodeMockProof(t *testing.T) {
	t.Run("mock proof", func(t *testing.T) {
		publicValues, ok := decodeMockProof(mockProof([]byte{0xaa, 0xbb, 0xcc}))
		require.Equal(t, true, ok)
		require.DeepEqual(t, []byte{0xaa, 0xbb, 0xcc}, publicValues)
	})

	t.Run("empty lists", func(t *testing.T) {
		publicValues, ok := decodeMockProof([]byte{8, 0, 0, 0, 8, 0, 0, 0})
		require.Equal(t, true, ok)
		require.Equal(t, 0, len(publicValues))
	})

	t.Run("too short", func(t *testing.T) {
		_, ok := decodeMockProof([]byte{8, 0, 0, 0})
		require.Equal(t, false, ok)
	})

	t.Run("wrong first offset", func(t *testing.T) {
		_, ok := decodeMockProof([]byte{9, 0, 0, 0, 9, 0, 0, 0, 0})
		require.Equal(t, false, ok)
	})

	t.Run("second offset before the first", func(t *testing.T) {
		_, ok := decodeMockProof([]byte{8, 0, 0, 0, 4, 0, 0, 0})
		require.Equal(t, false, ok)
	})

	t.Run("second offset past the end", func(t *testing.T) {
		_, ok := decodeMockProof([]byte{8, 0, 0, 0, 9, 0, 0, 0})
		require.Equal(t, false, ok)
	})
}
