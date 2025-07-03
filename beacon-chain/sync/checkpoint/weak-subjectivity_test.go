package checkpoint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/OffchainLabs/prysm/v6/api/client"
	"github.com/OffchainLabs/prysm/v6/api/client/beacon"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	blocktest "github.com/OffchainLabs/prysm/v6/consensus-types/blocks/testing"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz/detect"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
)

func marshalToEnvelope(val interface{}) ([]byte, error) {
	raw, err := json.Marshal(val)
	if err != nil {
		return nil, errors.Wrap(err, "error marshaling value to place in data envelope")
	}
	env := struct {
		Data json.RawMessage `json:"data"`
	}{
		Data: raw,
	}
	return json.Marshal(env)
}

func TestMarshalToEnvelope(t *testing.T) {
	d := struct {
		Version string `json:"version"`
	}{
		Version: "Prysm/v2.0.5 (linux amd64)",
	}
	encoded, err := marshalToEnvelope(d)
	require.NoError(t, err)
	expected := `{"data":{"version":"Prysm/v2.0.5 (linux amd64)"}}`
	require.Equal(t, expected, string(encoded))
}

func TestFname(t *testing.T) {
	vu := &detect.VersionedUnmarshaler{
		Config: params.MainnetConfig(),
		Fork:   version.Phase0,
	}
	slot := primitives.Slot(23)
	prefix := "block"
	var root [32]byte
	copy(root[:], []byte{0x23, 0x23, 0x23})
	expected := "block_mainnet_phase0_23-0x2323230000000000000000000000000000000000000000000000000000000000.ssz"
	actual := fname(prefix, vu, slot, root)
	require.Equal(t, expected, actual)

	vu.Config = params.MinimalSpecConfig()
	vu.Fork = version.Altair
	slot = 17
	prefix = "state"
	copy(root[29:], []byte{0x17, 0x17, 0x17})
	expected = "state_minimal_altair_17-0x2323230000000000000000000000000000000000000000000000000000171717.ssz"
	actual = fname(prefix, vu, slot, root)
	require.Equal(t, expected, actual)
}

func TestDownloadWeakSubjectivityCheckpoint(t *testing.T) {
	ctx := t.Context()
	cfg := params.MainnetConfig()

	epoch := cfg.AltairForkEpoch - 1
	// set up checkpoint state, using the epoch that will be computed as the ws checkpoint state based on the head state
	wSlot, err := slots.EpochStart(epoch)
	require.NoError(t, err)
	wst, err := util.NewBeaconState()
	require.NoError(t, err)
	fork, err := params.Fork(epoch)
	require.NoError(t, err)
	require.NoError(t, wst.SetFork(fork))

	// set up checkpoint block
	b, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlock())
	require.NoError(t, err)
	b, err = blocktest.SetBlockParentRoot(b, cfg.ZeroHash)
	require.NoError(t, err)
	b, err = blocktest.SetBlockSlot(b, wSlot)
	require.NoError(t, err)
	b, err = blocktest.SetProposerIndex(b, 0)
	require.NoError(t, err)

	// set up state header pointing at checkpoint block - this is how the block is downloaded by root
	header, err := b.Header()
	require.NoError(t, err)
	require.NoError(t, wst.SetLatestBlockHeader(header.Header))

	// order of operations can be confusing here:
	// - when computing the state root, make sure block header is complete, EXCEPT the state root should be zero-value
	// - before computing the block root (to match the request route), the block should include the state root
	//   *computed from the state with a header that does not have a state root set yet*
	wRoot, err := wst.HashTreeRoot(ctx)
	require.NoError(t, err)

	b, err = blocktest.SetBlockStateRoot(b, wRoot)
	require.NoError(t, err)
	serBlock, err := b.MarshalSSZ()
	require.NoError(t, err)
	bRoot, err := b.Block().HashTreeRoot()
	require.NoError(t, err)

	wsSerialized, err := wst.MarshalSSZ()
	require.NoError(t, err)
	expectedWSD := beacon.WeakSubjectivityData{
		BlockRoot: bRoot,
		StateRoot: wRoot,
		Epoch:     epoch,
	}

	trans := &testRT{rt: func(req *http.Request) (*http.Response, error) {
		res := &http.Response{Request: req}
		switch req.URL.Path {
		case beacon.GetWeakSubjectivityPath:
			res.StatusCode = http.StatusOK
			cp := struct {
				Epoch string `json:"epoch"`
				Root  string `json:"root"`
			}{
				Epoch: fmt.Sprintf("%d", slots.ToEpoch(b.Block().Slot())),
				Root:  fmt.Sprintf("%#x", bRoot),
			}
			wsr := struct {
				Checkpoint interface{} `json:"ws_checkpoint"`
				StateRoot  string      `json:"state_root"`
			}{
				Checkpoint: cp,
				StateRoot:  fmt.Sprintf("%#x", wRoot),
			}
			rb, err := marshalToEnvelope(wsr)
			require.NoError(t, err)
			res.Body = io.NopCloser(bytes.NewBuffer(rb))
		case beacon.RenderGetStatePath(beacon.IdFromSlot(wSlot)):
			res.StatusCode = http.StatusOK
			res.Body = io.NopCloser(bytes.NewBuffer(wsSerialized))
		case beacon.RenderGetBlockPath(beacon.IdFromRoot(bRoot)):
			res.StatusCode = http.StatusOK
			res.Body = io.NopCloser(bytes.NewBuffer(serBlock))
		}

		return res, nil
	}}

	c, err := beacon.NewClient("http://localhost:3500", client.WithRoundTripper(trans))
	require.NoError(t, err)

	wsd, err := ComputeWeakSubjectivityCheckpoint(ctx, c)
	require.NoError(t, err)
	require.Equal(t, expectedWSD.Epoch, wsd.Epoch)
	require.Equal(t, expectedWSD.StateRoot, wsd.StateRoot)
	require.Equal(t, expectedWSD.BlockRoot, wsd.BlockRoot)
}

// runs computeBackwardsCompatible directly
// and via ComputeWeakSubjectivityCheckpoint with a round tripper that triggers the backwards compatible code path
func TestDownloadBackwardsCompatibleCombined(t *testing.T) {
	ctx := t.Context()
	cfg := params.MainnetConfig()

	st, expectedEpoch := defaultTestHeadState(t, cfg)
	serialized, err := st.MarshalSSZ()
	require.NoError(t, err)

	// set up checkpoint state, using the epoch that will be computed as the ws checkpoint state based on the head state
	wSlot, err := slots.EpochStart(expectedEpoch)
	require.NoError(t, err)
	wst, err := util.NewBeaconState()
	require.NoError(t, err)
	fork, err := params.Fork(cfg.GenesisEpoch)
	require.NoError(t, err)
	require.NoError(t, wst.SetFork(fork))

	// set up checkpoint block
	b, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlock())
	require.NoError(t, err)
	b, err = blocktest.SetBlockParentRoot(b, cfg.ZeroHash)
	require.NoError(t, err)
	b, err = blocktest.SetBlockSlot(b, wSlot)
	require.NoError(t, err)
	b, err = blocktest.SetProposerIndex(b, 0)
	require.NoError(t, err)

	// set up state header pointing at checkpoint block - this is how the block is downloaded by root
	header, err := b.Header()
	require.NoError(t, err)
	require.NoError(t, wst.SetLatestBlockHeader(header.Header))

	// order of operations can be confusing here:
	// - when computing the state root, make sure block header is complete, EXCEPT the state root should be zero-value
	// - before computing the block root (to match the request route), the block should include the state root
	//   *computed from the state with a header that does not have a state root set yet*
	wRoot, err := wst.HashTreeRoot(ctx)
	require.NoError(t, err)

	b, err = blocktest.SetBlockStateRoot(b, wRoot)
	require.NoError(t, err)
	serBlock, err := b.MarshalSSZ()
	require.NoError(t, err)
	bRoot, err := b.Block().HashTreeRoot()
	require.NoError(t, err)

	wsSerialized, err := wst.MarshalSSZ()
	require.NoError(t, err)

	trans := &testRT{rt: func(req *http.Request) (*http.Response, error) {
		res := &http.Response{Request: req}
		switch req.URL.Path {
		case beacon.GetNodeVersionPath:
			res.StatusCode = http.StatusOK
			b := bytes.NewBuffer(nil)
			d := struct {
				Version string `json:"version"`
			}{
				Version: "Lighthouse/v0.1.5 (Linux x86_64)",
			}
			encoded, err := marshalToEnvelope(d)
			require.NoError(t, err)
			b.Write(encoded)
			res.Body = io.NopCloser(b)
		case beacon.GetWeakSubjectivityPath:
			res.StatusCode = http.StatusNotFound
		case beacon.RenderGetStatePath(beacon.IdHead):
			res.StatusCode = http.StatusOK
			res.Body = io.NopCloser(bytes.NewBuffer(serialized))
		case beacon.RenderGetStatePath(beacon.IdFromSlot(wSlot)):
			res.StatusCode = http.StatusOK
			res.Body = io.NopCloser(bytes.NewBuffer(wsSerialized))
		case beacon.RenderGetBlockPath(beacon.IdFromRoot(bRoot)):
			res.StatusCode = http.StatusOK
			res.Body = io.NopCloser(bytes.NewBuffer(serBlock))
		}

		return res, nil
	}}

	c, err := beacon.NewClient("http://localhost:3500", client.WithRoundTripper(trans))
	require.NoError(t, err)

	wsPub, err := ComputeWeakSubjectivityCheckpoint(ctx, c)
	require.NoError(t, err)

	wsPriv, err := computeBackwardsCompatible(ctx, c)
	require.NoError(t, err)
	require.DeepEqual(t, wsPriv, wsPub)
}

func TestGetWeakSubjectivityEpochFromHead(t *testing.T) {
	st, expectedEpoch := defaultTestHeadState(t, params.MainnetConfig())
	serialized, err := st.MarshalSSZ()
	require.NoError(t, err)
	trans := &testRT{rt: func(req *http.Request) (*http.Response, error) {
		res := &http.Response{Request: req}
		if req.URL.Path == beacon.RenderGetStatePath(beacon.IdHead) {
			res.StatusCode = http.StatusOK
			res.Body = io.NopCloser(bytes.NewBuffer(serialized))
		}
		return res, nil
	}}
	c, err := beacon.NewClient("http://localhost:3500", client.WithRoundTripper(trans))
	require.NoError(t, err)
	actualEpoch, err := getWeakSubjectivityEpochFromHead(t.Context(), c)
	require.NoError(t, err)
	require.Equal(t, expectedEpoch, actualEpoch)
}

func defaultTestHeadState(t *testing.T, cfg *params.BeaconChainConfig) (state.BeaconState, primitives.Epoch) {
	st, err := util.NewBeaconStateAltair()
	require.NoError(t, err)

	fork, err := params.Fork(cfg.AltairForkEpoch)
	require.NoError(t, err)
	require.NoError(t, st.SetFork(fork))

	slot, err := slots.EpochStart(cfg.AltairForkEpoch)
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(slot))

	var validatorCount, avgBalance uint64 = 100, 35
	require.NoError(t, populateValidators(cfg, st, validatorCount, avgBalance))
	require.NoError(t, st.SetFinalizedCheckpoint(&ethpb.Checkpoint{
		Epoch: fork.Epoch - 10,
		Root:  make([]byte, 32),
	}))
	// to see the math for this, look at helpers.LatestWeakSubjectivityEpoch
	// and for the values use mainnet config values, the validatorCount and avgBalance above, and altair fork epoch
	expectedEpoch := slots.ToEpoch(st.Slot()) - 224
	return st, expectedEpoch
}

// TODO(10429): refactor beacon state options in testing/util to take a state.BeaconState so this can become an option
func populateValidators(cfg *params.BeaconChainConfig, st state.BeaconState, valCount, avgBalance uint64) error {
	validators := make([]*ethpb.Validator, valCount)
	balances := make([]uint64, len(validators))
	for i := uint64(0); i < valCount; i++ {
		validators[i] = &ethpb.Validator{
			PublicKey:             make([]byte, cfg.BLSPubkeyLength),
			WithdrawalCredentials: make([]byte, 32),
			EffectiveBalance:      avgBalance * 1e9,
			ExitEpoch:             cfg.FarFutureEpoch,
		}
		balances[i] = validators[i].EffectiveBalance
	}

	if err := st.SetValidators(validators); err != nil {
		return err
	}
	return st.SetBalances(balances)
}
