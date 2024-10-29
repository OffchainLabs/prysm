package execution

import (
	"context"
	"net/http"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	pb "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

type versioner struct {
	version int
}

func (v versioner) Version() int {
	return v.version
}

func payloadToBody(t *testing.T, ed interfaces.ExecutionData) *pb.ExecutionPayloadBody {
	body := &pb.ExecutionPayloadBody{}
	txs, err := ed.Transactions()
	require.NoError(t, err)
	wd, err := ed.Withdrawals()
	// Bellatrix does not have withdrawals and will return an error.
	if err == nil {
		body.Withdrawals = wd
	}
	for i := range txs {
		body.Transactions = append(body.Transactions, txs[i])
	}
	return body
}

type blindedBlockFixtures struct {
	denebBlock      *fullAndBlinded
	emptyDenebBlock *fullAndBlinded
	afterSkipDeneb  *fullAndBlinded
	electra         *fullAndBlinded
}

type fullAndBlinded struct {
	full    interfaces.ReadOnlySignedBeaconBlock
	blinded *blockWithHeader
}

func blindedBlockWithHeader(t *testing.T, b interfaces.ReadOnlySignedBeaconBlock) *fullAndBlinded {
	header, err := b.Block().Body().Execution()
	require.NoError(t, err)
	blinded, err := b.ToBlinded()
	require.NoError(t, err)
	return &fullAndBlinded{
		full: b,
		blinded: &blockWithHeader{
			block:  blinded,
			header: header,
		}}
}

func denebSlot(t *testing.T) primitives.Slot {
	s, err := slots.EpochStart(params.BeaconConfig().DenebForkEpoch)
	require.NoError(t, err)
	return s
}

func electraSlot(t *testing.T) primitives.Slot {
	s, err := slots.EpochStart(params.BeaconConfig().ElectraForkEpoch)
	require.NoError(t, err)
	return s
}

func testBlindedBlockFixtures(t *testing.T) *blindedBlockFixtures {
	pfx := fixturesStruct()
	fx := &blindedBlockFixtures{}
	full := pfx.ExecutionPayloadDeneb
	// this func overrides fixture blockhashes to ensure they are unique
	full.BlockHash = bytesutil.PadTo([]byte("full"), 32)
	denebBlock, _ := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, denebSlot(t), 0, util.WithPayloadSetter(full))
	fx.denebBlock = blindedBlockWithHeader(t, denebBlock)

	empty := pfx.EmptyExecutionPayloadDeneb
	empty.BlockHash = bytesutil.PadTo([]byte("empty"), 32)
	empty.BlockNumber = 2
	emptyDenebBlock, _ := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, denebSlot(t)+1, 0, util.WithPayloadSetter(empty))
	fx.emptyDenebBlock = blindedBlockWithHeader(t, emptyDenebBlock)

	afterSkip := fixturesStruct().ExecutionPayloadDeneb
	// this func overrides fixture blockhashes to ensure they are unique
	afterSkip.BlockHash = bytesutil.PadTo([]byte("afterSkip"), 32)
	afterSkip.BlockNumber = 4
	afterSkipBlock, _ := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, denebSlot(t)+3, 0, util.WithPayloadSetter(afterSkip))
	fx.afterSkipDeneb = blindedBlockWithHeader(t, afterSkipBlock)

	electra := fixturesStruct().ExecutionPayloadDeneb
	electra.BlockHash = bytesutil.PadTo([]byte("electra"), 32)
	electra.BlockNumber = 5
	electraBlock, _ := util.GenerateTestElectraBlockWithSidecar(t, [32]byte{}, electraSlot(t), 0, util.WithElectraPayload(electra))
	fx.electra = blindedBlockWithHeader(t, electraBlock)
	return fx
}

func TestPayloadBodiesViaUnblinder(t *testing.T) {
	defer util.HackElectraMaxuint(t)()
	fx := testBlindedBlockFixtures(t)
	t.Run("mix of non-empty and empty", func(t *testing.T) {
		cli, srv := newMockEngine(t)
		srv.register(GetPayloadBodiesByHashV1, func(msg *jsonrpcMessage, w http.ResponseWriter, r *http.Request) {
			executionPayloadBodies := []*pb.ExecutionPayloadBody{
				payloadToBody(t, fx.denebBlock.blinded.header),
				payloadToBody(t, fx.emptyDenebBlock.blinded.header),
			}
			mockWriteResult(t, w, msg, executionPayloadBodies)
		})
		ctx := context.Background()

		toUnblind := []interfaces.ReadOnlySignedBeaconBlock{
			fx.denebBlock.blinded.block,
			fx.emptyDenebBlock.blinded.block,
		}
		bbr, err := newBlindedBlockReconstructor(toUnblind)
		require.NoError(t, err)
		require.NoError(t, bbr.requestBodies(ctx, cli))

		payload, err := bbr.payloadForHeader(fx.denebBlock.blinded.header, fx.denebBlock.blinded.block.Version())
		require.NoError(t, err)
		require.Equal(t, version.Deneb, fx.denebBlock.blinded.block.Version())
		unblindFull, err := blocks.BuildSignedBeaconBlockFromExecutionPayload(fx.denebBlock.blinded.block, payload)
		require.NoError(t, err)
		testAssertReconstructedEquivalent(t, fx.denebBlock.full, unblindFull)

		emptyPayload, err := bbr.payloadForHeader(fx.emptyDenebBlock.blinded.header, fx.emptyDenebBlock.blinded.block.Version())
		require.NoError(t, err)
		unblindEmpty, err := blocks.BuildSignedBeaconBlockFromExecutionPayload(fx.emptyDenebBlock.blinded.block, emptyPayload)
		require.NoError(t, err)
		testAssertReconstructedEquivalent(t, fx.emptyDenebBlock.full, unblindEmpty)
	})
}

func TestFixtureEquivalence(t *testing.T) {
	defer util.HackElectraMaxuint(t)()
	fx := testBlindedBlockFixtures(t)
	t.Run("full and blinded block equivalence", func(t *testing.T) {
		testAssertReconstructedEquivalent(t, fx.denebBlock.blinded.block, fx.denebBlock.full)
		testAssertReconstructedEquivalent(t, fx.emptyDenebBlock.blinded.block, fx.emptyDenebBlock.full)
	})
}

func testAssertReconstructedEquivalent(t *testing.T, b, ogb interfaces.ReadOnlySignedBeaconBlock) {
	bHtr, err := b.Block().HashTreeRoot()
	require.NoError(t, err)
	ogbHtr, err := ogb.Block().HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, bHtr, ogbHtr)
}

func TestComputeRanges(t *testing.T) {
	cases := []struct {
		name string
		hbns []hashBlockNumber
		want []byRangeReq
	}{
		{
			name: "3 contiguous, 1 not",
			hbns: []hashBlockNumber{
				{h: [32]byte{5}, n: 5},
				{h: [32]byte{3}, n: 3},
				{h: [32]byte{2}, n: 2},
				{h: [32]byte{1}, n: 1},
			},
			want: []byRangeReq{
				{start: 1, count: 3, hbns: []hashBlockNumber{{h: [32]byte{1}, n: 1}, {h: [32]byte{2}, n: 2}, {h: [32]byte{3}, n: 3}}},
				{start: 5, count: 1, hbns: []hashBlockNumber{{h: [32]byte{5}, n: 5}}},
			},
		},
		{
			name: "1 element",
			hbns: []hashBlockNumber{
				{h: [32]byte{1}, n: 1},
			},
			want: []byRangeReq{
				{start: 1, count: 1, hbns: []hashBlockNumber{{h: [32]byte{1}, n: 1}}},
			},
		},
		{
			name: "2 contiguous",
			hbns: []hashBlockNumber{
				{h: [32]byte{2}, n: 2},
				{h: [32]byte{1}, n: 1},
			},
			want: []byRangeReq{
				{start: 1, count: 2, hbns: []hashBlockNumber{{h: [32]byte{1}, n: 1}, {h: [32]byte{2}, n: 2}}},
			},
		},
		{
			name: "2 non-contiguous",
			hbns: []hashBlockNumber{
				{h: [32]byte{3}, n: 3},
				{h: [32]byte{1}, n: 1},
			},
			want: []byRangeReq{
				{start: 1, count: 1, hbns: []hashBlockNumber{{h: [32]byte{1}, n: 1}}},
				{start: 3, count: 1, hbns: []hashBlockNumber{{h: [32]byte{3}, n: 3}}},
			},
		},
		{
			name: "3 contiguous",
			hbns: []hashBlockNumber{
				{h: [32]byte{2}, n: 2},
				{h: [32]byte{1}, n: 1},
				{h: [32]byte{3}, n: 3},
			},
			want: []byRangeReq{
				{start: 1, count: 3, hbns: []hashBlockNumber{{h: [32]byte{1}, n: 1}, {h: [32]byte{2}, n: 2}, {h: [32]byte{3}, n: 3}}},
			},
		},
		{
			name: "3 non-contiguous",
			hbns: []hashBlockNumber{
				{h: [32]byte{5}, n: 5},
				{h: [32]byte{3}, n: 3},
				{h: [32]byte{1}, n: 1},
			},
			want: []byRangeReq{
				{start: 1, count: 1, hbns: []hashBlockNumber{{h: [32]byte{1}, n: 1}}},
				{start: 3, count: 1, hbns: []hashBlockNumber{{h: [32]byte{3}, n: 3}}},
				{start: 5, count: 1, hbns: []hashBlockNumber{{h: [32]byte{5}, n: 5}}},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := computeRanges(c.hbns)
			for i := range got {
				require.Equal(t, c.want[i].start, got[i].start)
				require.Equal(t, c.want[i].count, got[i].count)
				require.DeepEqual(t, c.want[i].hbns, got[i].hbns)
			}
		})
	}
}

func TestReconstructBlindedBlockBatchFallbackToRange(t *testing.T) {
	defer util.HackElectraMaxuint(t)()
	ctx := context.Background()
	t.Run("fallback fails", func(t *testing.T) {
		cli, srv := newMockEngine(t)
		fx := testBlindedBlockFixtures(t)
		srv.register(GetPayloadBodiesByHashV1, func(msg *jsonrpcMessage, w http.ResponseWriter, r *http.Request) {
			executionPayloadBodies := []*pb.ExecutionPayloadBody{nil, nil}
			mockWriteResult(t, w, msg, executionPayloadBodies)
		})
		srv.register(GetPayloadBodiesByRangeV1, func(msg *jsonrpcMessage, w http.ResponseWriter, r *http.Request) {
			executionPayloadBodies := []*pb.ExecutionPayloadBody{nil, nil}
			mockWriteResult(t, w, msg, executionPayloadBodies)
		})
		toUnblind := []interfaces.ReadOnlySignedBeaconBlock{
			fx.denebBlock.blinded.block,
			fx.emptyDenebBlock.blinded.block,
		}
		_, err := reconstructBlindedBlockBatch(ctx, cli, toUnblind)
		require.ErrorIs(t, err, errNilPayloadBody)
		require.Equal(t, 1, srv.callCount(GetPayloadBodiesByHashV1))
		require.Equal(t, 1, srv.callCount(GetPayloadBodiesByRangeV1))
	})
	t.Run("fallback succeeds", func(t *testing.T) {
		cli, srv := newMockEngine(t)
		fx := testBlindedBlockFixtures(t)
		srv.register(GetPayloadBodiesByHashV1, func(msg *jsonrpcMessage, w http.ResponseWriter, r *http.Request) {
			executionPayloadBodies := []*pb.ExecutionPayloadBody{nil, nil}
			mockWriteResult(t, w, msg, executionPayloadBodies)
		})
		srv.register(GetPayloadBodiesByRangeV1, func(msg *jsonrpcMessage, w http.ResponseWriter, r *http.Request) {
			executionPayloadBodies := []*pb.ExecutionPayloadBody{
				payloadToBody(t, fx.denebBlock.blinded.header),
				payloadToBody(t, fx.emptyDenebBlock.blinded.header),
			}
			mockWriteResult(t, w, msg, executionPayloadBodies)
		})
		unblind := []interfaces.ReadOnlySignedBeaconBlock{
			fx.denebBlock.blinded.block,
			fx.emptyDenebBlock.blinded.block,
		}
		_, err := reconstructBlindedBlockBatch(ctx, cli, unblind)
		require.NoError(t, err)
	})
	t.Run("separated by block number gap", func(t *testing.T) {
		cli, srv := newMockEngine(t)
		fx := testBlindedBlockFixtures(t)
		srv.register(GetPayloadBodiesByHashV1, func(msg *jsonrpcMessage, w http.ResponseWriter, r *http.Request) {
			executionPayloadBodies := []*pb.ExecutionPayloadBody{nil, nil, nil}
			mockWriteResult(t, w, msg, executionPayloadBodies)
		})
		srv.register(GetPayloadBodiesByRangeV1, func(msg *jsonrpcMessage, w http.ResponseWriter, r *http.Request) {
			p := mockParseUintList(t, msg.Params)
			require.Equal(t, 2, len(p))
			start, count := p[0], p[1]
			// Return first 2 blocks by number, which are contiguous.
			if start == fx.denebBlock.blinded.header.BlockNumber() {
				require.Equal(t, uint64(2), count)
				executionPayloadBodies := []*pb.ExecutionPayloadBody{
					payloadToBody(t, fx.denebBlock.blinded.header),
					payloadToBody(t, fx.emptyDenebBlock.blinded.header),
				}
				mockWriteResult(t, w, msg, executionPayloadBodies)
				return
			}
			// Assume it's the second request
			require.Equal(t, fx.afterSkipDeneb.blinded.header.BlockNumber(), start)
			require.Equal(t, uint64(1), count)
			executionPayloadBodies := []*pb.ExecutionPayloadBody{
				payloadToBody(t, fx.afterSkipDeneb.blinded.header),
			}
			mockWriteResult(t, w, msg, executionPayloadBodies)
		})
		blind := []interfaces.ReadOnlySignedBeaconBlock{
			fx.denebBlock.blinded.block,
			fx.emptyDenebBlock.blinded.block,
			fx.afterSkipDeneb.blinded.block,
		}
		unblind, err := reconstructBlindedBlockBatch(ctx, cli, blind)
		require.NoError(t, err)
		for i := range unblind {
			testAssertReconstructedEquivalent(t, blind[i], unblind[i])
		}
	})
}

func TestReconstructBlindedBlockBatchDenebAndElectra(t *testing.T) {
	defer util.HackElectraMaxuint(t)()
	t.Run("deneb and electra", func(t *testing.T) {
		cli, srv := newMockEngine(t)
		fx := testBlindedBlockFixtures(t)
		srv.register(GetPayloadBodiesByHashV1, func(msg *jsonrpcMessage, w http.ResponseWriter, r *http.Request) {
			executionPayloadBodies := []*pb.ExecutionPayloadBody{payloadToBody(t, fx.denebBlock.blinded.header), payloadToBody(t, fx.electra.blinded.header)}
			mockWriteResult(t, w, msg, executionPayloadBodies)
		})
		blinded := []interfaces.ReadOnlySignedBeaconBlock{
			fx.denebBlock.blinded.block,
			fx.electra.blinded.block,
		}
		unblinded, err := reconstructBlindedBlockBatch(context.Background(), cli, blinded)
		require.NoError(t, err)
		require.Equal(t, len(blinded), len(unblinded))
		for i := range unblinded {
			testAssertReconstructedEquivalent(t, blinded[i], unblinded[i])
		}
	})
}
