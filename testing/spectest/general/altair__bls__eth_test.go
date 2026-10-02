package general

import (
	"encoding/hex"
	"path"
	"testing"

	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/spectest/utils"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// ethBLSSources are the two copies of the eth_* BLS vectors: cryptography-specs and consensus-specs general.
var ethBLSSources = []struct{ config, fork, suffix string }{
	{"bls", "", ""},
	{"general", "altair", "bls"},
}

func ethBLSFolders(t *testing.T, config, fork, handler, suffix string) ([]string, string) {
	dir := path.Join("bls", handler, suffix)
	if config == "bls" {
		fork, dir = handler, ""
	}
	folders, folderPath := utils.TestFolders(t, config, fork, dir)
	names := make([]string, 0, len(folders))
	for _, f := range folders {
		names = append(names, f.Name())
	}
	return names, folderPath
}

func TestEthAggregatePubkeys(t *testing.T) {
	if utils.FakeCrypto {
		t.Skip("BLS vectors need the real backend")
	}
	type data struct {
		Input  []string `json:"input"`
		Output *string  `json:"output"`
	}
	for _, src := range ethBLSSources {
		t.Run(src.config, func(t *testing.T) {
			names, folderPath := ethBLSFolders(t, src.config, src.fork, "eth_aggregate_pubkeys", src.suffix)
			for _, name := range names {
				t.Run(name, func(t *testing.T) {
					raw, err := util.BazelFileBytes(folderPath, name, "data.yaml")
					require.NoError(t, err)
					test := &data{}
					require.NoError(t, utils.UnmarshalYaml(raw, test))

					pubkeys := make([][]byte, len(test.Input))
					for i, pk := range test.Input {
						pubkeys[i], err = hex.DecodeString(pk[2:])
						require.NoError(t, err)
					}
					agg, err := bls.AggregatePublicKeys(pubkeys)
					if test.Output == nil {
						require.NotNil(t, err)
						return
					}
					require.NoError(t, err)
					require.Equal(t, *test.Output, "0x"+hex.EncodeToString(agg.Marshal()))
				})
			}
		})
	}
}

func TestEthFastAggregateVerify(t *testing.T) {
	if utils.FakeCrypto {
		t.Skip("BLS vectors need the real backend")
	}
	type data struct {
		Input struct {
			Pubkeys   []string `json:"pubkeys"`
			Message   string   `json:"message"`
			Signature string   `json:"signature"`
		} `json:"input"`
		Output bool `json:"output"`
	}
	for _, src := range ethBLSSources {
		t.Run(src.config, func(t *testing.T) {
			names, folderPath := ethBLSFolders(t, src.config, src.fork, "eth_fast_aggregate_verify", src.suffix)
			for _, name := range names {
				t.Run(name, func(t *testing.T) {
					raw, err := util.BazelFileBytes(folderPath, name, "data.yaml")
					require.NoError(t, err)
					test := &data{}
					require.NoError(t, utils.UnmarshalYaml(raw, test))

					pubkeys := make([]common.PublicKey, len(test.Input.Pubkeys))
					for i, pk := range test.Input.Pubkeys {
						b, err := hex.DecodeString(pk[2:])
						require.NoError(t, err)
						pubkeys[i], err = bls.PublicKeyFromBytes(b)
						if err != nil {
							require.Equal(t, false, test.Output)
							return
						}
					}
					msg, err := hex.DecodeString(test.Input.Message[2:])
					require.NoError(t, err)
					sigBytes, err := hex.DecodeString(test.Input.Signature[2:])
					require.NoError(t, err)
					sig, err := bls.SignatureFromBytes(sigBytes)
					if err != nil {
						require.Equal(t, false, test.Output)
						return
					}
					require.Equal(t, test.Output, sig.Eth2FastAggregateVerify(pubkeys, bytesutil.ToBytes32(msg)))
				})
			}
		})
	}
}
