package proofengine

import (
	"embed"
	"encoding/binary"
	"fmt"
	"path"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// Mock proofs, for devnets only (--zkvm-mock-proofs).
//
// The mock zkVMs of zkboost (github.com/eth-act/zkboost, v0.12.0 and later) prove nothing. In place of a proof, they
// send the SSZ container
//
//	MockProof { public_values: List[byte], proof: List[byte] }
//
// whose public values are the ones a real proof of the payload would commit to, and whose proof is random bytes.
// The proof cannot be verified, so in mock mode the engine accepts a MockProof without a cryptographic check, but
// still checks its public values against the payload, as it does for a real proof.
//
// To keep the cost of a real verification, which a devnet should reflect, the engine still calls the ere verifier
// for each mock proof, and discards the result. Verifying the random bytes of the mock proof would fail when
// decoding them, almost instantly, so it verifies instead a genuine proof of the same proof type: a stand-in from
// mockproofs/, embedded in the binary. It spends the CPU time, the memory and the verification slot of a real
// verification, through the very verifier and verification key the node uses. The stand-ins are genuine proofs of
// another block (the fixtures of zkboost v0.11.1, crates/server/src/proof/zkvm/mock), so their result says nothing
// about the payload, hence it is discarded. They are verified once when the engine starts, so that a verification
// key that does not match them fails at startup rather than going unnoticed.

//go:embed mockproofs/*.proof
var standInProofFiles embed.FS

// standInProofNames maps each proof type to its stand-in proof in mockproofs/.
var standInProofNames = map[ethpb.ProofType]string{
	ethpb.ProofTypeEthrexOpenVM: "stateless-validator-ethrex-openvm-v2.1.0-preview.proof",
	ethpb.ProofTypeEthrexSP1:    "stateless-validator-ethrex-sp1-v6.4.0.proof",
	ethpb.ProofTypeEthrexZisk:   "stateless-validator-ethrex-zisk-v1.1.0-alpha.proof",
}

// standInProofs returns the stand-in proof of every proof type.
func standInProofs() (map[ethpb.ProofType][]byte, error) {
	proofs := make(map[ethpb.ProofType][]byte, len(standInProofNames))
	for proofType, name := range standInProofNames {
		proof, err := standInProofFiles.ReadFile(path.Join("mockproofs", name))
		if err != nil {
			return nil, fmt.Errorf("read stand-in proof of %s: %w", proofType, err)
		}

		proofs[proofType] = proof
	}

	return proofs, nil
}

// enableMockProofs makes the engine accept mock proofs, with these stand-in proofs. Every proof type the engine
// verifies needs a stand-in, which must verify with the verification key of its proof type.
func (e *Engine) enableMockProofs(standIns map[ethpb.ProofType][]byte) error {
	for proofType, verifier := range e.verifiers {
		standIn, ok := standIns[proofType]
		if !ok {
			return fmt.Errorf("no stand-in proof for %s", proofType)
		}

		if _, err := verifier.zkVM.verify(standIn); err != nil {
			return fmt.Errorf("stand-in proof of %s does not verify with its verification key: %w", proofType, err)
		}

		verifier.standIn = standIn
		e.verifiers[proofType] = verifier
	}

	return nil
}

// decodeMockProof returns the public values of a proof that decodes as a MockProof: the offsets of its two lists,
// then the public values and the proof.
func decodeMockProof(proof []byte) ([]byte, bool) {
	const offsetsLength = 8

	if len(proof) < offsetsLength {
		return nil, false
	}

	publicValuesOffset := binary.LittleEndian.Uint32(proof[0:4])
	proofOffset := binary.LittleEndian.Uint32(proof[4:8])
	if publicValuesOffset != offsetsLength || proofOffset < publicValuesOffset || uint64(proofOffset) > uint64(len(proof)) {
		return nil, false
	}

	return proof[publicValuesOffset:proofOffset], true
}
