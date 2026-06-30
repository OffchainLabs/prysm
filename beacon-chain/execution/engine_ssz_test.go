package execution

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/execution/enginehttp"
	payloadattribute "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attribute"
	pb "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	enginev2 "github.com/OffchainLabs/prysm/v7/proto/engine/v2"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common"
)

func TestPayloadStatusResult(t *testing.T) {
	lvh := make([]byte, 32)
	lvh[0] = 0xab

	t.Run("VALID returns latest valid hash and no error", func(t *testing.T) {
		out, err := payloadStatusResult(&enginev2.PayloadStatus{
			Status:          enginev2.StatusByte(enginev2.PayloadStatusValid),
			LatestValidHash: enginev2.PresentBytes(lvh),
		})
		require.NoError(t, err)
		assert.DeepEqual(t, lvh, out)
	})

	t.Run("INVALID returns latest valid hash and the INVALID sentinel", func(t *testing.T) {
		out, err := payloadStatusResult(&enginev2.PayloadStatus{
			Status:          enginev2.StatusByte(enginev2.PayloadStatusInvalid),
			LatestValidHash: enginev2.PresentBytes(lvh),
		})
		require.ErrorIs(t, err, ErrInvalidPayloadStatus)
		assert.DeepEqual(t, lvh, out)
	})

	t.Run("SYNCING maps to the accepted/syncing sentinel", func(t *testing.T) {
		out, err := payloadStatusResult(&enginev2.PayloadStatus{Status: enginev2.StatusByte(enginev2.PayloadStatusSyncing)})
		require.ErrorIs(t, err, ErrAcceptedSyncingPayloadStatus)
		require.IsNil(t, out)
	})

	t.Run("ACCEPTED maps to the accepted/syncing sentinel", func(t *testing.T) {
		_, err := payloadStatusResult(&enginev2.PayloadStatus{Status: enginev2.StatusByte(enginev2.PayloadStatusAccepted)})
		require.ErrorIs(t, err, ErrAcceptedSyncingPayloadStatus)
	})

	t.Run("unknown status maps to the unknown sentinel", func(t *testing.T) {
		_, err := payloadStatusResult(&enginev2.PayloadStatus{Status: enginev2.StatusByte(9)})
		require.ErrorIs(t, err, ErrUnknownPayloadStatus)
	})
}

func TestForkchoiceResult(t *testing.T) {
	lvh := make([]byte, 32)
	lvh[0] = 0xcd
	id := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	t.Run("VALID echoes the opaque payload id verbatim", func(t *testing.T) {
		gotID, gotLVH, err := forkchoiceResult(&enginev2.ForkchoiceUpdateResponse{
			PayloadStatus: &enginev2.PayloadStatus{Status: enginev2.StatusByte(enginev2.PayloadStatusValid), LatestValidHash: enginev2.PresentBytes(lvh)},
			PayloadId:     enginev2.PresentBytes(id),
		})
		require.NoError(t, err)
		require.NotNil(t, gotID)
		assert.DeepEqual(t, id, gotID[:])
		assert.DeepEqual(t, lvh, gotLVH)
	})

	t.Run("VALID with no build started has a nil payload id", func(t *testing.T) {
		gotID, _, err := forkchoiceResult(&enginev2.ForkchoiceUpdateResponse{
			PayloadStatus: &enginev2.PayloadStatus{Status: enginev2.StatusByte(enginev2.PayloadStatusValid)},
		})
		require.NoError(t, err)
		require.IsNil(t, gotID)
	})

	t.Run("INVALID returns latest valid hash and the INVALID sentinel", func(t *testing.T) {
		gotID, gotLVH, err := forkchoiceResult(&enginev2.ForkchoiceUpdateResponse{
			PayloadStatus: &enginev2.PayloadStatus{Status: enginev2.StatusByte(enginev2.PayloadStatusInvalid), LatestValidHash: enginev2.PresentBytes(lvh)},
		})
		require.ErrorIs(t, err, ErrInvalidPayloadStatus)
		require.IsNil(t, gotID)
		assert.DeepEqual(t, lvh, gotLVH)
	})

	t.Run("SYNCING maps to the accepted/syncing sentinel", func(t *testing.T) {
		_, _, err := forkchoiceResult(&enginev2.ForkchoiceUpdateResponse{
			PayloadStatus: &enginev2.PayloadStatus{Status: enginev2.StatusByte(enginev2.PayloadStatusSyncing)},
		})
		require.ErrorIs(t, err, ErrAcceptedSyncingPayloadStatus)
	})

	t.Run("nil payload status returns ErrNilResponse", func(t *testing.T) {
		_, _, err := forkchoiceResult(&enginev2.ForkchoiceUpdateResponse{})
		require.ErrorIs(t, err, ErrNilResponse)
	})

	t.Run("ACCEPTED on forkchoice is a protocol error", func(t *testing.T) {
		_, _, err := forkchoiceResult(&enginev2.ForkchoiceUpdateResponse{
			PayloadStatus: &enginev2.PayloadStatus{Status: enginev2.StatusByte(enginev2.PayloadStatusAccepted)},
		})
		require.ErrorIs(t, err, ErrUnknownPayloadStatus)
	})
}

func TestMapEngineError(t *testing.T) {
	cases := []struct {
		problemType string
		want        error
	}{
		{enginehttp.ProblemInvalidForkchoice, ErrInvalidForkchoiceState},
		{enginehttp.ProblemInvalidAttributes, ErrInvalidPayloadAttributes},
		{enginehttp.ProblemUnknownPayload, ErrUnknownPayload},
		{enginehttp.ProblemRequestTooLarge, ErrRequestTooLarge},
		{enginehttp.ProblemUnsupportedFork, ErrUnsupportedFork},
		{enginehttp.ProblemInvalidBody, ErrInvalidParams},
	}
	for _, tc := range cases {
		err := mapEngineError(&enginehttp.Error{Status: 409, Problem: enginehttp.Problem{Type: tc.problemType}})
		require.ErrorIs(t, err, tc.want)
	}

	require.NoError(t, mapEngineError(nil))

	ioErr := errors.New("io failure")
	assert.Equal(t, ioErr, mapEngineError(ioErr)) // non-*Error passes through

	other := &enginehttp.Error{Status: 500, Problem: enginehttp.Problem{Type: "/engine-api/errors/teapot"}}
	assert.Equal(t, other, mapEngineError(other)) // unmapped problem type passes through
}

func TestBuiltPayloadToBundle(t *testing.T) {
	val := []byte{0xaa, 0xbb}
	reqs := [][]byte{{0x01, 0x02}}

	t.Run("Fulu", func(t *testing.T) {
		bundle, err := builtPayloadToBundle(&enginev2.BuiltPayloadFulu{
			BlockValue:            val,
			ShouldOverrideBuilder: true,
			ExecutionRequests:     reqs,
		})
		require.NoError(t, err)
		fb, ok := bundle.(*pb.ExecutionBundleFulu)
		require.Equal(t, true, ok)
		assert.DeepEqual(t, val, fb.Value)
		assert.Equal(t, true, fb.ShouldOverrideBuilder)
		assert.DeepEqual(t, reqs, fb.ExecutionRequests)
	})

	t.Run("Gloas", func(t *testing.T) {
		bundle, err := builtPayloadToBundle(&enginev2.BuiltPayloadGloas{
			BlockValue:            val,
			ShouldOverrideBuilder: false,
			ExecutionRequests:     reqs,
		})
		require.NoError(t, err)
		gb, ok := bundle.(*pb.ExecutionBundleGloas)
		require.Equal(t, true, ok)
		assert.DeepEqual(t, val, gb.Value)
		assert.Equal(t, false, gb.ShouldOverrideBuilder)
		assert.DeepEqual(t, reqs, gb.ExecutionRequests)
	})

	t.Run("unexpected type errors", func(t *testing.T) {
		_, err := builtPayloadToBundle(&enginev2.PayloadStatus{})
		require.ErrorContains(t, "unexpected BuiltPayload type", err)
	})
}

func TestSupportsBlob(t *testing.T) {
	e := &sszEngine{caps: &enginehttp.Capabilities{
		IndependentlyVersioned: map[string][]string{"blobs": {"v1", "v2", "v3", "v4"}},
	}}
	assert.Equal(t, true, e.supportsBlob("v1"))
	assert.Equal(t, true, e.supportsBlob("v2"))

	none := &sszEngine{caps: &enginehttp.Capabilities{IndependentlyVersioned: map[string][]string{}}}
	assert.Equal(t, false, none.supportsBlob("v1"))

	// No capability document (defensive): permit the request to surface support.
	assert.Equal(t, true, (&sszEngine{}).supportsBlob("v1"))
}

func TestRejectIfUnsupportedFork(t *testing.T) {
	e := &sszEngine{caps: &enginehttp.Capabilities{SupportedForks: []string{"osaka", "amsterdam"}}}
	require.NoError(t, e.rejectIfUnsupportedFork(version.Fulu))
	require.NoError(t, e.rejectIfUnsupportedFork(version.Gloas))

	unsupported := &sszEngine{caps: &enginehttp.Capabilities{SupportedForks: []string{"amsterdam"}}}
	err := unsupported.rejectIfUnsupportedFork(version.Fulu)
	require.ErrorIs(t, err, ErrUnsupportedFork)
	require.ErrorContains(t, "osaka", err)

	none := &sszEngine{caps: &enginehttp.Capabilities{}}
	require.ErrorIs(t, none.rejectIfUnsupportedFork(version.Fulu), ErrUnsupportedFork)

	// No capability document (defensive): permit the request to surface support.
	require.NoError(t, (&sszEngine{}).rejectIfUnsupportedFork(version.Fulu))
}

func TestBodiesEntries(t *testing.T) {
	tx := []byte{0xde, 0xad}
	wd := []*pb.Withdrawal{{Index: 7}}
	resp := &enginev2.BodiesResponseGloas{
		Entries: []*enginev2.BodyEntryGloas{
			{Available: true, Body: &enginev2.ExecutionPayloadBodyGloas{
				Transactions:    [][]byte{tx},
				Withdrawals:     wd,
				BlockAccessList: []byte{0x01, 0x02},
			}},
			{Available: false, Body: &enginev2.ExecutionPayloadBodyGloas{}},
		},
	}

	out, err := bodiesEntries(resp)
	require.NoError(t, err)
	require.Equal(t, 2, len(out))
	require.NotNil(t, out[0])

	transactions, err := out[0].Transactions()
	require.NoError(t, err)
	assert.DeepEqual(t, tx, transactions[0])

	withdrawals, err := out[0].Withdrawals()
	require.NoError(t, err)
	assert.DeepEqual(t, wd, withdrawals)
	require.IsNil(t, out[1]) // available=false -> nil body

	_, err = bodiesEntries(&enginev2.PayloadStatus{})
	require.ErrorContains(t, "unexpected BodiesResponse type", err)
}

func TestSSZEngineLimit(t *testing.T) {
	e := &sszEngine{caps: &enginehttp.Capabilities{Limits: map[string]uint64{limitBodiesMaxCount: 32}}}

	got, ok := e.limit(limitBodiesMaxCount)
	assert.Equal(t, true, ok)
	assert.Equal(t, uint64(32), got)

	_, ok = e.limit(limitPayloadMaxBytes) // absent key
	assert.Equal(t, false, ok)

	_, ok = (&sszEngine{caps: &enginehttp.Capabilities{Limits: map[string]uint64{limitBodiesMaxCount: 0}}}).limit(limitBodiesMaxCount)
	assert.Equal(t, false, ok) // zero == unbounded

	_, ok = (&sszEngine{}).limit(limitBodiesMaxCount) // nil caps
	assert.Equal(t, false, ok)

	require.NoError(t, e.rejectIfOverLimit(limitBodiesMaxCount, 32)) // at cap
	require.NoError(t, e.rejectIfOverLimit(limitBodiesMaxCount, 1))
	require.NoError(t, e.rejectIfOverLimit(limitPayloadMaxBytes, 1<<40)) // uncapped key
	require.ErrorIs(t, e.rejectIfOverLimit(limitBodiesMaxCount, 33), ErrRequestTooLarge)
}

func TestGetBlobs_RejectsOverLimit(t *testing.T) {
	e := &sszEngine{caps: &enginehttp.Capabilities{
		IndependentlyVersioned: map[string][]string{"blobs": {"v1", "v2"}},
		Limits:                 map[string]uint64{limitBlobsMaxVersionedHashes: 2},
	}}
	hashes := []common.Hash{{1}, {2}, {3}}

	_, err := e.GetBlobs(context.Background(), hashes)
	require.ErrorIs(t, err, ErrRequestTooLarge)

	_, err = e.GetBlobsV2(context.Background(), hashes)
	require.ErrorIs(t, err, ErrRequestTooLarge)
}

func newTestSSZEngine(t *testing.T, srvURL string, caps *enginehttp.Capabilities) *sszEngine {
	c, err := enginehttp.New(enginehttp.Config{
		BaseURL:   srvURL,
		JWTSecret: []byte("0123456789abcdef0123456789abcdef"),
	})
	require.NoError(t, err)
	return &sszEngine{client: c, caps: caps}
}

func sszBodiesGloas(t *testing.T, n uint64) []byte {
	resp := &enginev2.BodiesResponseGloas{Entries: make([]*enginev2.BodyEntryGloas, n)}
	for i := range resp.Entries {
		resp.Entries[i] = &enginev2.BodyEntryGloas{Body: &enginev2.ExecutionPayloadBodyGloas{}}
	}
	b, err := resp.MarshalSSZ()
	require.NoError(t, err)
	return b
}

func TestGetPayloadBodiesByHash_Chunks(t *testing.T) {
	var sizes []int
	srv := h2cServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		req := &enginev2.BodiesByHashRequest{}
		require.NoError(t, req.UnmarshalSSZ(body))
		sizes = append(sizes, len(req.BlockHashes))
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(sszBodiesGloas(t, uint64(len(req.BlockHashes))))
	})
	e := newTestSSZEngine(t, srv.URL, &enginehttp.Capabilities{
		SupportedForks: []string{"amsterdam"},
		Limits:         map[string]uint64{limitBodiesMaxCount: 2},
	})

	hashes := []common.Hash{{1}, {2}, {3}, {4}, {5}}
	result, err := e.GetPayloadBodiesByHash(context.Background(), version.Gloas, hashes)
	require.NoError(t, err)
	assert.Equal(t, len(hashes), len(result))  // request-aligned across chunks
	assert.DeepEqual(t, []int{2, 2, 1}, sizes) // 5 hashes, cap 2 -> 3 calls
}

func TestGetPayloadBodiesByRange_Chunks(t *testing.T) {
	type call struct{ from, count string }
	var calls []call
	srv := h2cServer(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		calls = append(calls, call{from: q.Get("from"), count: q.Get("count")})
		cnt, err := strconv.ParseUint(q.Get("count"), 10, 64)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(sszBodiesGloas(t, cnt))
	})
	e := newTestSSZEngine(t, srv.URL, &enginehttp.Capabilities{
		SupportedForks: []string{"amsterdam"},
		Limits:         map[string]uint64{limitBodiesMaxCount: 2},
	})

	result, err := e.GetPayloadBodiesByRange(context.Background(), version.Gloas, 100, 5)
	require.NoError(t, err)
	assert.Equal(t, 5, len(result))
	assert.DeepEqual(t, []call{{"100", "2"}, {"102", "2"}, {"104", "1"}}, calls)
}

func fcuResponseSSZ(t *testing.T) []byte {
	b, err := (&enginev2.ForkchoiceUpdateResponse{
		PayloadStatus: &enginev2.PayloadStatus{Status: enginev2.StatusByte(enginev2.PayloadStatusValid)},
	}).MarshalSSZ()
	require.NoError(t, err)
	return b
}

func TestForkchoiceUpdated_SerializesPerConnection(t *testing.T) {
	var inFlight, maxInFlight atomic.Int32
	resp := fcuResponseSSZ(t)
	srv := h2cServer(t, func(w http.ResponseWriter, r *http.Request) {
		cur := inFlight.Add(1)
		for { // record the high-water mark of concurrent requests
			m := maxInFlight.Load()
			if cur <= m || maxInFlight.CompareAndSwap(m, cur) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond) // widen the overlap window
		inFlight.Add(-1)
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(resp)
	})
	e := newTestSSZEngine(t, srv.URL, &enginehttp.Capabilities{SupportedForks: []string{"osaka"}})

	// Shared read-only state across goroutines; buildForkchoiceUpdate only reads it.
	state := &pb.ForkchoiceState{
		HeadBlockHash:      make([]byte, 32),
		SafeBlockHash:      make([]byte, 32),
		FinalizedBlockHash: make([]byte, 32),
	}

	const goroutines = 16
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for range goroutines {
		wg.Go(func() {
			_, _, err := e.ForkchoiceUpdated(context.Background(), state, payloadattribute.EmptyWithVersion(version.Fulu))
			errs <- err
		})
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, int32(1), maxInFlight.Load()) // never more than one in flight
}

func TestForkchoiceUpdated_RejectsUnadvertisedForkBeforeRequest(t *testing.T) {
	var requests atomic.Int32
	srv := h2cServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	})
	e := newTestSSZEngine(t, srv.URL, &enginehttp.Capabilities{SupportedForks: []string{"amsterdam"}})
	state := &pb.ForkchoiceState{
		HeadBlockHash:      make([]byte, 32),
		SafeBlockHash:      make([]byte, 32),
		FinalizedBlockHash: make([]byte, 32),
	}

	_, _, err := e.ForkchoiceUpdated(context.Background(), state, payloadattribute.EmptyWithVersion(version.Fulu))
	require.ErrorIs(t, err, ErrUnsupportedFork)
	assert.Equal(t, int32(0), requests.Load())
}
