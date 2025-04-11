package checkpoint

import (
	"context"
	"fmt"
	"path"

	"github.com/OffchainLabs/prysm/v6/api/client"
	"github.com/OffchainLabs/prysm/v6/api/client/beacon"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz/detect"
	"github.com/OffchainLabs/prysm/v6/io/file"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var errCheckpointBlockMismatch = errors.New("mismatch between checkpoint sync state and block")

// APIInitializer manages initializing the beacon node using checkpoint sync, retrieving the checkpoint state and root
// from the remote beacon node api.
type APIInitializer struct {
	c *beacon.Client
}

// NewAPIInitializer creates an APIInitializer, handling the set up of a beacon node api client
// using the provided host string.
func NewAPIInitializer(beaconNodeHost string) (*APIInitializer, error) {
	c, err := beacon.NewClient(beaconNodeHost, client.WithMaxBodySize(client.MaxBodySizeState))
	if err != nil {
		return nil, errors.Wrapf(err, "unable to parse beacon node url or hostname - %s", beaconNodeHost)
	}
	return &APIInitializer{c: c}, nil
}

// Initialize downloads origin state and block for checkpoint sync and initializes database records to
// prepare the node to begin syncing from that point.
func (dl *APIInitializer) Initialize(ctx context.Context, d db.Database) error {
	origin, err := d.OriginCheckpointBlockRoot(ctx)
	if err == nil && origin != params.BeaconConfig().ZeroHash {
		log.Warnf("Origin checkpoint root %#x found in db, ignoring checkpoint sync flags", origin)
		return nil
	}
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		return errors.Wrap(err, "error while checking database for origin root")
	}
	od, err := DownloadFinalizedData(ctx, dl.c)
	if err != nil {
		return errors.Wrap(err, "Error retrieving checkpoint origin state and block")
	}
	return d.SaveOrigin(ctx, od.StateBytes(), od.BlockBytes())
}

// OriginData represents the BeaconState and ReadOnlySignedBeaconBlock necessary to start an empty Beacon Node
// using Checkpoint Sync.
type OriginData struct {
	sb []byte
	bb []byte
	st state.BeaconState
	b  interfaces.ReadOnlySignedBeaconBlock
	vu *detect.VersionedUnmarshaler
	br [32]byte
	sr [32]byte
}

// SaveBlock saves the downloaded block to a unique file in the given path.
// For readability and collision avoidance, the file name includes: type, config name, slot and root
func (o *OriginData) SaveBlock(dir string) (string, error) {
	blockPath := path.Join(dir, fname("block", o.vu, o.b.Block().Slot(), o.br))
	return blockPath, file.WriteFile(blockPath, o.BlockBytes())
}

// SaveState saves the downloaded state to a unique file in the given path.
// For readability and collision avoidance, the file name includes: type, config name, slot and root
func (o *OriginData) SaveState(dir string) (string, error) {
	statePath := path.Join(dir, fname("state", o.vu, o.st.Slot(), o.sr))
	return statePath, file.WriteFile(statePath, o.StateBytes())
}

// StateBytes returns the ssz-encoded bytes of the downloaded BeaconState value.
func (o *OriginData) StateBytes() []byte {
	return o.sb
}

// BlockBytes returns the ssz-encoded bytes of the downloaded ReadOnlySignedBeaconBlock value.
func (o *OriginData) BlockBytes() []byte {
	return o.bb
}

func fname(prefix string, vu *detect.VersionedUnmarshaler, slot primitives.Slot, root [32]byte) string {
	return fmt.Sprintf("%s_%s_%s_%d-%#x.ssz", prefix, vu.Config.ConfigName, version.String(vu.Fork), slot, root)
}

// DownloadFinalizedData downloads the most recently finalized state, and the block most recently applied to that state.
// This pair can be used to initialize a new beacon node via checkpoint sync.
func DownloadFinalizedData(ctx context.Context, client *beacon.Client) (*OriginData, error) {
	sb, err := client.GetState(ctx, beacon.IdFinalized)
	if err != nil {
		return nil, err
	}
	vu, err := detect.FromState(sb)
	if err != nil {
		return nil, errors.Wrap(err, "error detecting chain config for finalized state")
	}

	log.WithFields(logrus.Fields{
		"name": vu.Config.ConfigName,
		"fork": version.String(vu.Fork),
	}).Info("Detected supported config in remote finalized state")

	s, err := vu.UnmarshalBeaconState(sb)
	if err != nil {
		return nil, errors.Wrap(err, "error unmarshaling finalized state to correct version")
	}

	slot := s.LatestBlockHeader().Slot
	bb, err := client.GetBlock(ctx, beacon.IdFromSlot(slot))
	if err != nil {
		return nil, errors.Wrapf(err, "error requesting block by slot = %d", slot)
	}
	b, err := vu.UnmarshalBeaconBlock(bb)
	if err != nil {
		return nil, errors.Wrap(err, "unable to unmarshal block to a supported type using the detected fork schedule")
	}
	br, err := b.Block().HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "error computing hash_tree_root of retrieved block")
	}
	bodyRoot, err := b.Block().Body().HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "error computing hash_tree_root of retrieved block body")
	}

	sbr := bytesutil.ToBytes32(s.LatestBlockHeader().BodyRoot)
	if sbr != bodyRoot {
		return nil, errors.Wrapf(errCheckpointBlockMismatch, "state body root = %#x, block body root = %#x", sbr, bodyRoot)
	}
	sr, err := s.HashTreeRoot(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to compute htr for finalized state at slot=%d", s.Slot())
	}

	log.
		WithField("blockSlot", b.Block().Slot()).
		WithField("stateSlot", s.Slot()).
		WithField("stateRoot", hexutil.Encode(sr[:])).
		WithField("blockRoot", hexutil.Encode(br[:])).
		Info("Downloaded checkpoint sync state and block.")
	return &OriginData{
		st: s,
		b:  b,
		sb: sb,
		bb: bb,
		vu: vu,
		br: br,
		sr: sr,
	}, nil
}
