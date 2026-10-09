package proofengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// defaultVerificationKeysHex are the program verification keys of the guests of eth-act/ere-guests v0.17.0, for the
// ere v0.17.2 verifiers (shipped by github.com/nalepae/go-ere, bump both together). They are the built-in keys of
// Lighthouse (eth-act/lighthouse, branch optional-proofs, beacon_node/proof_engine/src/config.rs).
// They are the Ethrex stateless-validator guests from ethrex v26.0.0, the only proof types the specification supports.
var defaultVerificationKeysHex = map[ethpb.ProofType]string{
	ethpb.ProofTypeEthrexOpenVM: "0x" +
		"005793a02300aef9526800ee5a881400a80e7d2600ba7f8b4500f002973500b2c72d61005e74577006030619068000a2" +
		"c21b53000a2fee4f0036169c2800aaaa8d6c0087fbce5b00328dc26f009fe7de5a004686562400e77e894500a128f20f" +
		"00674f7c2400b38df01800309c530900a487cf0400725bac510051af497500e4abff6e00a58ac939000775b41a001a76" +
		"e84100c5e8944400c94e8e1600330e6b39001cacbc5a00ca47cd51001b418e02000fe02a480009a32070002554164500" +
		"d7069403007d07bf3000290ccf21008726523b00e5fd1112003d03bd4c001c6831680016a3fe4200ad7ec6300028529e" +
		"3c005710de1700349b6a77004b13962f00cff00054001483f65100ab05ce6b0034174b6000bc041c0900a9b5a11a00b2" +
		"6f160300615de46100935f922800d39e4a2700596ea87000ca5764770023df7b57000b1ee85e004c456d61000bdad13b" +
		"003de28a5f008584cc2a00033ab1020025f59e4a00c3a9f64a00b8ef166500",
	ethpb.ProofTypeEthrexSP1:  "0x00662ca6c9db4ecb22d9ac4c320612caeac9c320f92e3ab019af4f544e35c88b",
	ethpb.ProofTypeEthrexZisk: "0xbe8b29b013077f82a411403e9389ae7464b152db764e4333fdbd2c81fa157b0d",
}

// defaultVerificationKeys returns the built-in verification key of every proof type.
func defaultVerificationKeys() map[ethpb.ProofType][]byte {
	keys := make(map[ethpb.ProofType][]byte, len(defaultVerificationKeysHex))
	for proofType, key := range defaultVerificationKeysHex {
		keys[proofType] = hexutil.MustDecode(key)
	}

	return keys
}

// verificationKeysConfig is the proof engine configuration file. It uses the format of Lighthouse's --proof-engine
// file, so that devnet configurations serve both clients:
//
//	{"execution_proofs": [{"proof_type": 1, "program_vk": "0x..."}]}
type verificationKeysConfig struct {
	ExecutionProofs []struct {
		ProofType uint8  `json:"proof_type"`
		ProgramVK string `json:"program_vk"`
	} `json:"execution_proofs"`
}

// loadVerificationKeys reads the verification keys of the configuration file at path. Only the proof types it lists
// are verified.
func loadVerificationKeys(path string) (map[ethpb.ProofType][]byte, error) {
	content, err := os.ReadFile(path) // #nosec G304 -- The path is set by the node operator.
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	return parseVerificationKeys(content)
}

func parseVerificationKeys(content []byte) (map[ethpb.ProofType][]byte, error) {
	var config verificationKeysConfig
	if err := json.Unmarshal(content, &config); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	if len(config.ExecutionProofs) == 0 {
		return nil, errors.New("no execution proof configured")
	}

	keys := make(map[ethpb.ProofType][]byte, len(config.ExecutionProofs))
	for _, entry := range config.ExecutionProofs {
		proofType := ethpb.ProofType(entry.ProofType)
		if !proofType.Supported() {
			return nil, fmt.Errorf("unsupported proof type %d", entry.ProofType)
		}

		if _, ok := keys[proofType]; ok {
			return nil, fmt.Errorf("duplicate proof type %s", proofType)
		}

		key, err := bytesutil.DecodeHexWithMaxLength(entry.ProgramVK, 1<<20)
		if err != nil {
			return nil, fmt.Errorf("decode program verification key of %s: %w", proofType, err)
		}

		if len(key) == 0 {
			return nil, fmt.Errorf("empty program verification key for %s", proofType)
		}

		keys[proofType] = key
	}

	return keys, nil
}
