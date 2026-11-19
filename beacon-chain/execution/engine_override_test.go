package execution

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	pb "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

func TestGetPayload_Override(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	fix := fixtures()
	payload, ok := fix["ExecutionPayloadDeneb"].(*pb.ExecutionPayloadDeneb)
	require.Equal(t, true, ok)
	wantValue := bytesutil.PadTo([]byte{42}, 32)
	wantBundle := &pb.BlobsBundleV2{
		KzgCommitments: [][]byte{bytesutil.PadTo([]byte{0xc}, 48)},
		Proofs:         [][]byte{bytesutil.PadTo([]byte{0xd}, 48)},
		Blobs:          [][]byte{{0xb}},
	}
	payloadId := [8]byte{1}

	t.Run("override populates the response, rpc client unused", func(t *testing.T) {
		var gotMethod string
		srv := &Service{
			rpcClient: RPCClientBad{}, // any CallContext would fail the test
			cfg: &config{getPayloadOverride: func(_ context.Context, id [8]byte, _ primitives.Slot, method string, result proto.Message) error {
				gotMethod = method
				require.Equal(t, payloadId, id)
				bundle, ok := result.(*pb.ExecutionBundleFulu)
				require.Equal(t, true, ok)
				bundle.Payload = payload
				bundle.Value = wantValue
				bundle.BlobsBundle = wantBundle
				bundle.ShouldOverrideBuilder = true
				bundle.ExecutionRequests = [][]byte{}
				return nil
			}},
		}
		resp, err := srv.GetPayload(t.Context(), payloadId, 1)
		require.NoError(t, err)
		require.Equal(t, GetPayloadMethodV5, gotMethod)
		require.Equal(t, true, resp.OverrideBuilder)
		require.DeepEqual(t, primitives.LittleEndianBytesToWei(wantValue), resp.Bid)
		require.DeepEqual(t, payload, resp.ExecutionData.Proto())
		gotBundle, ok := resp.BlobsBundler.(*pb.BlobsBundleV2)
		require.Equal(t, true, ok)
		require.DeepEqual(t, wantBundle, gotBundle)
	})

	t.Run("override error propagates", func(t *testing.T) {
		srv := &Service{
			rpcClient: RPCClientBad{},
			cfg: &config{getPayloadOverride: func(context.Context, [8]byte, primitives.Slot, string, proto.Message) error {
				return errors.New("typed path failed")
			}},
		}
		_, err := srv.GetPayload(t.Context(), payloadId, 1)
		require.ErrorContains(t, "typed path failed", err)
	})
}

func TestGetBlobs_Override(t *testing.T) {
	hashes := []common.Hash{{0xaa}}
	want := []*pb.BlobAndProofV2{{Blob: []byte{1}, KzgProofs: [][]byte{{2}}}}
	var gotMethod string
	srv := &Service{
		rpcClient:       RPCClientBad{},
		capabilityCache: &capabilityCache{}, // empty: the JSON path would reject GetBlobsV2 as unsupported
		cfg: &config{getBlobsOverride: func(_ context.Context, method string, got []common.Hash) ([]*pb.BlobAndProofV2, error) {
			gotMethod = method
			require.DeepEqual(t, hashes, got)
			return want, nil
		}},
	}

	got, err := srv.GetBlobsV2(t.Context(), hashes)
	require.NoError(t, err)
	require.Equal(t, GetBlobsV2, gotMethod)
	require.DeepEqual(t, want, got)

	got, err = srv.GetBlobsV3(t.Context(), hashes)
	require.NoError(t, err)
	require.Equal(t, GetBlobsV3, gotMethod)
	require.DeepEqual(t, want, got)
}

func TestGetBlobsV2_OverrideRespectsDisableFlag(t *testing.T) {
	old := flags.Get()
	flags.Init(&flags.GlobalFlags{DisableGetBlobsV2: true})
	t.Cleanup(func() { flags.Init(old) })

	called := false
	srv := &Service{
		rpcClient:       RPCClientBad{},
		capabilityCache: &capabilityCache{capabilities: map[string]any{GetBlobsV2: nil}},
		cfg: &config{getBlobsOverride: func(context.Context, string, []common.Hash) ([]*pb.BlobAndProofV2, error) {
			called = true
			return nil, nil
		}},
	}
	got, err := srv.GetBlobsV2(t.Context(), []common.Hash{{0xaa}})
	require.NoError(t, err)
	require.Equal(t, 0, len(got))
	require.Equal(t, false, called)
}
