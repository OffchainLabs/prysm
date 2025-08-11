package p2p

import (
	"bytes"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/config/params"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var errEth2ENRDigestMismatch = errors.New("fork digest of peer does not match local value")

// ENR key used for Ethereum consensus-related fork data.
var eth2ENRKey = params.BeaconNetworkConfig().ETH2Key

// ForkDigest returns the current fork digest of
// the node according to the local clock.
func (s *Service) currentForkDigest() ([4]byte, error) {
	if !s.isInitialized() {
		return [4]byte{}, errors.New("state is not initialized")
	}

	currentSlot := slots.CurrentSlot(s.genesisTime)
	currentEpoch := slots.ToEpoch(currentSlot)
	return params.ForkDigest(currentEpoch), nil
}

// Compares fork ENRs between an incoming peer's record and our node's
// local record values for current and next fork version/epoch.
func compareForkENR(self, peer *enr.Record) error {
	peerForkENR, err := forkEntry(peer)
	if err != nil {
		return err
	}
	currentForkENR, err := forkEntry(self)
	if err != nil {
		return err
	}
	enrString, err := SerializeENR(peer)
	if err != nil {
		return err
	}
	// Clients SHOULD connect to peers with current_fork_digest, next_fork_version,
	// and next_fork_epoch that match local values.
	if !bytes.Equal(peerForkENR.CurrentForkDigest, currentForkENR.CurrentForkDigest) {
		return errors.Wrapf(errEth2ENRDigestMismatch,
			"fork digest of peer with ENR %s: %v, does not match local value: %v",
			enrString,
			peerForkENR.CurrentForkDigest,
			currentForkENR.CurrentForkDigest,
		)
	}
	// Clients MAY connect to peers with the same current_fork_version but a
	// different next_fork_version/next_fork_epoch. Unless ENRForkID is manually
	// updated to matching prior to the earlier next_fork_epoch of the two clients,
	// these type of connecting clients will be unable to successfully interact
	// starting at the earlier next_fork_epoch.
	if peerForkENR.NextForkEpoch != currentForkENR.NextForkEpoch {
		log.WithFields(logrus.Fields{
			"peerNextForkEpoch": peerForkENR.NextForkEpoch,
			"peerENR":           enrString,
		}).Trace("Peer matches fork digest but has different next fork epoch")
	}
	if !bytes.Equal(peerForkENR.NextForkVersion, currentForkENR.NextForkVersion) {
		log.WithFields(logrus.Fields{
			"peerNextForkVersion": peerForkENR.NextForkVersion,
			"peerENR":             enrString,
		}).Trace("Peer matches fork digest but has different next fork version")
	}
	return nil
}

func updateENR(node *enode.LocalNode, entry, next params.NetworkScheduleEntry) error {
	enrForkID := &pb.ENRForkID{
		CurrentForkDigest: entry.ForkDigest[:],
		NextForkVersion:   next.ForkVersion[:],
		NextForkEpoch:     next.Epoch,
	}
	log.
		WithField("CurrentForkDigest", fmt.Sprintf("%#x", enrForkID.CurrentForkDigest)).
		WithField("NextForkVersion", fmt.Sprintf("%#x", enrForkID.NextForkVersion)).
		WithField("NextForkEpoch", fmt.Sprintf("%d", enrForkID.NextForkEpoch)).
		Info("Updating ENR Fork ID")
	enc, err := enrForkID.MarshalSSZ()
	if err != nil {
		return err
	}
	forkEntry := enr.WithEntry(eth2ENRKey, enc)
	node.Set(forkEntry)
	return nil
}

// Retrieves an enrForkID from an ENR record by key lookup
// under the Ethereum consensus EnrKey
func forkEntry(record *enr.Record) (*pb.ENRForkID, error) {
	sszEncodedForkEntry := make([]byte, 16)
	entry := enr.WithEntry(eth2ENRKey, &sszEncodedForkEntry)
	err := record.Load(entry)
	if err != nil {
		return nil, err
	}
	forkEntry := &pb.ENRForkID{}
	if err := forkEntry.UnmarshalSSZ(sszEncodedForkEntry); err != nil {
		return nil, err
	}
	return forkEntry, nil
}
