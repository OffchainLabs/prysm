package checkpoint

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/client"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	blocktest "github.com/prysmaticlabs/prysm/v5/consensus-types/blocks/testing"
	"github.com/prysmaticlabs/prysm/v5/encoding/ssz/detect"
	"github.com/prysmaticlabs/prysm/v5/network/forks"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

func TestDownloadFinalizedData(t *testing.T) {
	ctx := context.Background()
	cfg := params.MainnetConfig().Copy()

	// avoid the altair zone because genesis tests are easier to set up
	epoch := cfg.AltairForkEpoch - 1
	// set up checkpoint state, using the epoch that will be computed as the ws checkpoint state based on the head state
	slot, err := slots.EpochStart(epoch)
	require.NoError(t, err)
	st, err := util.NewBeaconState()
	require.NoError(t, err)
	fork, err := forks.ForkForEpochFromConfig(cfg, epoch)
	require.NoError(t, err)
	require.NoError(t, st.SetFork(fork))
	require.NoError(t, st.SetSlot(slot))

	// set up checkpoint block
	b, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlock())
	require.NoError(t, err)
	b, err = blocktest.SetBlockParentRoot(b, cfg.ZeroHash)
	require.NoError(t, err)
	b, err = blocktest.SetBlockSlot(b, slot)
	require.NoError(t, err)
	b, err = blocktest.SetProposerIndex(b, 0)
	require.NoError(t, err)

	// set up state header pointing at checkpoint block - this is how the block is downloaded by root
	header, err := b.Header()
	require.NoError(t, err)
	require.NoError(t, st.SetLatestBlockHeader(header.Header))

	// order of operations can be confusing here:
	// - when computing the state root, make sure block header is complete, EXCEPT the state root should be zero-value
	// - before computing the block root (to match the request route), the block should include the state root
	//   *computed from the state with a header that does not have a state root set yet*
	sr, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)

	b, err = blocktest.SetBlockStateRoot(b, sr)
	require.NoError(t, err)
	mb, err := b.MarshalSSZ()
	require.NoError(t, err)
	br, err := b.Block().HashTreeRoot()
	require.NoError(t, err)

	ms, err := st.MarshalSSZ()
	require.NoError(t, err)

	trans := &testRT{rt: func(req *http.Request) (*http.Response, error) {
		res := &http.Response{Request: req}
		switch req.URL.Path {
		case beacon.RenderGetStatePath(beacon.IdFinalized):
			res.StatusCode = http.StatusOK
			res.Body = io.NopCloser(bytes.NewBuffer(ms))
		case beacon.RenderGetBlockPath(beacon.IdFromSlot(b.Block().Slot())):
			res.StatusCode = http.StatusOK
			res.Body = io.NopCloser(bytes.NewBuffer(mb))
		default:
			res.StatusCode = http.StatusInternalServerError
			res.Body = io.NopCloser(bytes.NewBufferString(""))
		}

		return res, nil
	}}
	c, err := beacon.NewClient("http://localhost:3500", client.WithRoundTripper(trans))
	require.NoError(t, err)
	// sanity check before we go through checkpoint
	// make sure we can download the state and unmarshal it with the VersionedUnmarshaler
	sb, err := c.GetState(ctx, beacon.IdFinalized)
	require.NoError(t, err)
	require.Equal(t, true, bytes.Equal(sb, ms))
	vu, err := detect.FromState(sb)
	require.NoError(t, err)
	us, err := vu.UnmarshalBeaconState(sb)
	require.NoError(t, err)
	ushtr, err := us.HashTreeRoot(ctx)
	require.NoError(t, err)
	require.Equal(t, sr, ushtr)

	expected := &OriginData{
		sb: ms,
		bb: mb,
		br: br,
		sr: sr,
	}
	od, err := DownloadFinalizedData(ctx, c)
	require.NoError(t, err)
	require.Equal(t, true, bytes.Equal(expected.sb, od.sb))
	require.Equal(t, true, bytes.Equal(expected.bb, od.bb))
	require.Equal(t, expected.br, od.br)
	require.Equal(t, expected.sr, od.sr)
}

type testRT struct {
	rt func(*http.Request) (*http.Response, error)
}

func (rt *testRT) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt.rt != nil {
		return rt.rt(req)
	}
	return nil, errors.New("RoundTripper not implemented")
}

var _ http.RoundTripper = &testRT{}
