package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/mock"
	"github.com/OffchainLabs/prysm/v7/validator/pqkeys"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.uber.org/mock/gomock"
)

func TestPQStateValidatorsProvider_StateValidators(t *testing.T) {
	source := &testVRFStateSource{roots: make(chan [32]byte, 10)}
	registry, blsKey := testPQRegistry(t, source)

	xmssKey, ok := registry.XMSSPubkey(blsKey)
	require.Equal(t, true, ok)

	vrfRoot := [32]byte{0x01}
	blsHex, xmssHex, vrfRootHex := hexutil.Encode(blsKey[:]), hexutil.Encode(xmssKey[:]), hexutil.Encode(vrfRoot[:])
	otherXMSSHex := hexutil.Encode(make([]byte, fieldparams.XMSSPubkeyLength))

	t.Run("translates the public keys", func(t *testing.T) {
		handler := mock.NewMockHandler(gomock.NewController(t))
		expectPost(t, handler, "/eth/v1/beacon/states/head/validators", []string{"7", xmssHex}, []*structs.ValidatorContainerPQ{
			testValidatorPQ("3", xmssHex, vrfRootHex),
			testValidatorPQ("7", otherXMSSHex, vrfRootHex),
		})

		provider := pqStateValidatorsProvider{handler: handler, keys: registry}
		resp, err := provider.StateValidators(t.Context(), []string{blsHex}, []primitives.ValidatorIndex{7}, nil)
		require.NoError(t, err)
		require.Equal(t, true, resp.Finalized)
		require.Equal(t, 2, len(resp.Data))

		// One of the validators of the validator client: its BLS public key.
		require.Equal(t, "3", resp.Data[0].Index)
		require.Equal(t, "active_ongoing", resp.Data[0].Status)
		require.Equal(t, blsHex, resp.Data[0].Validator.Pubkey)
		require.Equal(t, "32000000000", resp.Data[0].Validator.EffectiveBalance)
		require.Equal(t, "4", resp.Data[0].Validator.ActivationEpoch)

		// Another validator, asked by index: its XMSS public key.
		require.Equal(t, "7", resp.Data[1].Index)
		require.Equal(t, otherXMSSHex, resp.Data[1].Validator.Pubkey)
	})

	t.Run("hands the VRF root to the registry", func(t *testing.T) {
		handler := mock.NewMockHandler(gomock.NewController(t))
		otherRoot := [32]byte{0x02}
		expectPost(t, handler, "/eth/v1/beacon/states/head/validators", []string{xmssHex}, []*structs.ValidatorContainerPQ{
			testValidatorPQ("3", xmssHex, hexutil.Encode(otherRoot[:])),
		})

		provider := pqStateValidatorsProvider{handler: handler, keys: registry}
		resp, err := provider.StateValidators(t.Context(), []string{blsHex}, nil, nil)
		require.NoError(t, err)
		require.Equal(t, 1, len(resp.Data))

		// The registry looks the root up in the VRF state, in the background. It may first look up the roots of the
		// other subtests.
		timeout := time.After(10 * time.Second)
		for {
			select {
			case root := <-source.roots:
				if root == otherRoot {
					return
				}
			case <-timeout:
				t.Fatal("the registry did not look the VRF root up")
			}
		}
	})

	t.Run("invalid VRF root", func(t *testing.T) {
		handler := mock.NewMockHandler(gomock.NewController(t))
		expectPost(t, handler, "/eth/v1/beacon/states/head/validators", []string{xmssHex}, []*structs.ValidatorContainerPQ{
			testValidatorPQ("3", xmssHex, "0x01"),
		})

		provider := pqStateValidatorsProvider{handler: handler, keys: registry}
		_, err := provider.StateValidators(t.Context(), []string{blsHex}, nil, nil)
		require.ErrorContains(t, "decode VRF root", err)
	})

	t.Run("asks nothing for unknown public keys", func(t *testing.T) {
		// The mock fails the test on any request.
		handler := mock.NewMockHandler(gomock.NewController(t))

		unknown := hexutil.Encode(make([]byte, fieldparams.BLSPubkeyLength))
		provider := pqStateValidatorsProvider{handler: handler, keys: registry}
		resp, err := provider.StateValidators(t.Context(), []string{unknown}, nil, nil)
		require.NoError(t, err)
		require.Equal(t, 0, len(resp.Data))
	})

	t.Run("invalid BLS public key", func(t *testing.T) {
		handler := mock.NewMockHandler(gomock.NewController(t))
		provider := pqStateValidatorsProvider{handler: handler, keys: registry}
		_, err := provider.StateValidators(t.Context(), []string{xmssHex}, nil, nil)
		require.ErrorContains(t, "decode BLS public key", err)
	})

	t.Run("state at a slot", func(t *testing.T) {
		handler := mock.NewMockHandler(gomock.NewController(t))
		expectPost(t, handler, "/eth/v1/beacon/states/5/validators", []string{xmssHex}, []*structs.ValidatorContainerPQ{
			testValidatorPQ("3", xmssHex, vrfRootHex),
		})

		provider := pqStateValidatorsProvider{handler: handler, keys: registry}
		resp, err := provider.StateValidatorsForSlot(t.Context(), 5, []string{blsHex}, nil, nil)
		require.NoError(t, err)
		require.Equal(t, 1, len(resp.Data))
		require.Equal(t, blsHex, resp.Data[0].Validator.Pubkey)
	})
}

// testPQRegistry returns a registry holding the keys of one validator, reading the VRF state from `source`,
// with the BLS public key of the validator.
func testPQRegistry(t *testing.T, source pqkeys.VRFStateSource) (*pqkeys.Registry, [fieldparams.BLSPubkeyLength]byte) {
	secretKey, err := bls.RandKey()
	require.NoError(t, err)

	registry := pqkeys.NewRegistry(t.Context(), source)
	require.NoError(t, registry.Load(t.Context(), []bls.SecretKey{secretKey}))

	return registry, [fieldparams.BLSPubkeyLength]byte(secretKey.PublicKey().Marshal())
}

// expectPost expects one state validators request to `endpoint` for `ids`, answered with `validators`.
func expectPost(t *testing.T, handler *mock.MockHandler, endpoint string, ids []string, validators []*structs.ValidatorContainerPQ) {
	handler.EXPECT().Post(gomock.Any(), endpoint, nil, gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, _ map[string]string, data *bytes.Buffer, resp any) error {
			var req structs.GetValidatorsRequest
			require.NoError(t, json.Unmarshal(data.Bytes(), &req))
			require.DeepEqual(t, ids, req.Ids)

			*resp.(*structs.GetValidatorsResponsePQ) = structs.GetValidatorsResponsePQ{Finalized: true, Data: validators}
			return nil
		},
	)
}

func testValidatorPQ(index, pubkey, vrfRoot string) *structs.ValidatorContainerPQ {
	return &structs.ValidatorContainerPQ{
		Index:   index,
		Balance: "32000000000",
		Status:  "active_ongoing",
		Validator: &structs.ValidatorPQ{
			Pubkey:                     pubkey,
			VrfRoot:                    vrfRoot,
			WithdrawalCredentials:      hexutil.Encode(make([]byte, fieldparams.RootLength)),
			EffectiveBalance:           "32000000000",
			ActivationEligibilityEpoch: "3",
			ActivationEpoch:            "4",
			ExitEpoch:                  "18446744073709551615",
			WithdrawableEpoch:          "18446744073709551615",
		},
	}
}

// testVRFStateSource is an empty VRF state, which sends the VRF roots it is asked for to `roots`.
type testVRFStateSource struct {
	roots chan [32]byte
}

func (s *testVRFStateSource) Records(_ context.Context, roots map[[32]byte]bool) (map[[32]byte]pqkeys.VRFRecord, error) {
	for root := range roots {
		s.roots <- root
	}

	return nil, nil
}
