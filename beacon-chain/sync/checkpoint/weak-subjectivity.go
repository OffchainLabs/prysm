package checkpoint

import (
	"context"

	base "github.com/OffchainLabs/prysm/v6/api/client"
	"github.com/OffchainLabs/prysm/v6/api/client/beacon"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz/detect"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
)

// ComputeWeakSubjectivityCheckpoint attempts to use the prysm weak_subjectivity api
// to obtain the current weak_subjectivity checkpoint.
// For non-prysm nodes, the same computation will be performed with extra steps,
// using the head state downloaded from the beacon node api.
func ComputeWeakSubjectivityCheckpoint(ctx context.Context, client *beacon.Client) (*beacon.WeakSubjectivityData, error) {
	ws, err := client.GetWeakSubjectivity(ctx)
	if err != nil {
		// a 404/405 is expected if querying an endpoint that doesn't support the weak subjectivity checkpoint api
		if !errors.Is(err, base.ErrNotOK) {
			return nil, errors.Wrap(err, "unexpected API response for prysm-only weak subjectivity checkpoint API")
		}
		// fall back to vanilla Beacon Node API method
		return computeBackwardsCompatible(ctx, client)
	}
	log.Printf("server weak subjectivity checkpoint response - epoch=%d, block_root=%#x, state_root=%#x", ws.Epoch, ws.BlockRoot, ws.StateRoot)
	return ws, nil
}

// for clients that do not support the weak_subjectivity api method we gather the necessary data for a checkpoint sync by:
// - inspecting the remote server's head state and computing the weak subjectivity epoch locally
// - requesting the state at the first slot of the epoch
// - using hash_tree_root(state.latest_block_header) to compute the block the state integrates
// - requesting that block by its root
func computeBackwardsCompatible(ctx context.Context, client *beacon.Client) (*beacon.WeakSubjectivityData, error) {
	log.Print("falling back to generic checkpoint derivation, weak_subjectivity API not supported by server")
	epoch, err := getWeakSubjectivityEpochFromHead(ctx, client)
	if err != nil {
		return nil, errors.Wrap(err, "error computing weak subjectivity epoch via head state inspection")
	}

	// use first slot of the epoch for the state slot
	slot, err := slots.EpochStart(epoch)
	if err != nil {
		return nil, errors.Wrapf(err, "error computing first slot of epoch=%d", epoch)
	}

	log.Printf("requesting checkpoint state at slot %d", slot)
	// get the state at the first slot of the epoch
	sb, err := client.GetState(ctx, beacon.IdFromSlot(slot))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to request state by slot from api, slot=%d", slot)
	}

	// ConfigFork is used to unmarshal the BeaconState so we can read the block root in latest_block_header
	vu, err := detect.FromState(sb)
	if err != nil {
		return nil, errors.Wrap(err, "error detecting chain config for beacon state")
	}
	log.Printf("detected supported config in checkpoint state, name=%s, fork=%s", vu.Config.ConfigName, version.String(vu.Fork))

	s, err := vu.UnmarshalBeaconState(sb)
	if err != nil {
		return nil, errors.Wrap(err, "error using detected config fork to unmarshal state bytes")
	}

	// compute state and block roots
	sr, err := s.HashTreeRoot(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "error computing hash_tree_root of state")
	}

	h := s.LatestBlockHeader()
	h.StateRoot = sr[:]
	br, err := h.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "error while computing block root using state data")
	}

	bb, err := client.GetBlock(ctx, beacon.IdFromRoot(br))
	if err != nil {
		return nil, errors.Wrapf(err, "error requesting block by root = %d", br)
	}
	b, err := vu.UnmarshalBeaconBlock(bb)
	if err != nil {
		return nil, errors.Wrap(err, "unable to unmarshal block to a supported type using the detected fork schedule")
	}
	br, err = b.Block().HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "error computing hash_tree_root for block obtained via root")
	}

	return &beacon.WeakSubjectivityData{
		Epoch:     epoch,
		BlockRoot: br,
		StateRoot: sr,
	}, nil
}

// this method downloads the head state, which can be used to find the correct chain config
// and use prysm's helper methods to compute the latest weak subjectivity epoch.
func getWeakSubjectivityEpochFromHead(ctx context.Context, client *beacon.Client) (primitives.Epoch, error) {
	headBytes, err := client.GetState(ctx, beacon.IdHead)
	if err != nil {
		return 0, err
	}
	vu, err := detect.FromState(headBytes)
	if err != nil {
		return 0, errors.Wrap(err, "error detecting chain config for beacon state")
	}
	log.Printf("detected supported config in remote head state, name=%s, fork=%s", vu.Config.ConfigName, version.String(vu.Fork))
	headState, err := vu.UnmarshalBeaconState(headBytes)
	if err != nil {
		return 0, errors.Wrap(err, "error unmarshaling state to correct version")
	}

	epoch, err := helpers.LatestWeakSubjectivityEpoch(ctx, headState, vu.Config)
	if err != nil {
		return 0, errors.Wrap(err, "error computing the weak subjectivity epoch from head state")
	}

	log.Printf("(computed client-side) weak subjectivity epoch = %d", epoch)
	return epoch, nil
}
