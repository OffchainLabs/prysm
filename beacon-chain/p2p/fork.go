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

var (
	errForkScheduleMismatch  = errors.New("peer fork schedule incompatible")
	errCurrentDigestMismatch = errors.Wrap(errForkScheduleMismatch, "current_fork_digest mismatch")
	errNextVersionMismatch   = errors.Wrap(errForkScheduleMismatch, "next_fork_version mismatch")
	errNextDigestMismatch    = errors.Wrap(errForkScheduleMismatch, "nfd (next fork digest) mismatch")
)

const (
	eth2EnrKey = "eth2" // The `eth2` ENR entry advertizes the node's view of the fork schedule with an ssz-encoded ENRForkID value.
	nfdEnrKey  = "nfd"  // The `nfd` ENR entry separately advertizes the "next fork digest" aspect of the fork schedule.
)

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
	peerEntry, err := forkEntry(peer)
	if err != nil {
		return err
	}
	selfEntry, err := forkEntry(self)
	if err != nil {
		return err
	}
	peerString, err := SerializeENR(peer)
	if err != nil {
		return err
	}
	// Clients SHOULD connect to peers with current_fork_digest, next_fork_version,
	// and next_fork_epoch that match local values.
	if !bytes.Equal(peerEntry.CurrentForkDigest, selfEntry.CurrentForkDigest) {
		return errors.Wrapf(errCurrentDigestMismatch,
			"fork digest of peer with ENR %s: %v, does not match local value: %v",
			peerString,
			peerEntry.CurrentForkDigest,
			selfEntry.CurrentForkDigest,
		)
	}

	// Clients MAY connect to peers with the same current_fork_version but a
	// different next_fork_version/next_fork_epoch. Unless ENRForkID is manually
	// updated to matching prior to the earlier next_fork_epoch of the two clients,
	// these type of connecting clients will be unable to successfully interact
	// starting at the earlier next_fork_epoch.
	if peerEntry.NextForkEpoch != selfEntry.NextForkEpoch {
		log.WithFields(logrus.Fields{
			"peerNextForkEpoch":   peerEntry.NextForkEpoch,
			"peerNextForkVersion": peerEntry.NextForkVersion,
			"peerENR":             peerString,
		}).Trace("Peer matches fork digest but has different next fork epoch")
		// We allow the connection because we have a different view of the next fork epoch. This
		// could be due to peers that have no upgraded ahead of a fork or BPO schedule change, so
		// we allow the connection to continue until the fork boundary.
		return nil
	}
	if selfEntry.NextForkEpoch == params.BeaconConfig().FarFutureEpoch {
		return nil
	}

	// Since we agree on the next fork epoch, we require next fork version to also be in agreement.
	if !bytes.Equal(peerEntry.NextForkVersion, selfEntry.NextForkVersion) {
		return errors.Wrapf(errNextVersionMismatch,
			"next fork version of peer with ENR %s: %#x, does not match local value: %#x",
			peerString, peerEntry.NextForkVersion, selfEntry.NextForkVersion)
	}

	// Fulu adds the following to the spec:
	// ---
	// A new entry is added to the ENR under the key nfd, short for next fork digest. This entry
	// communicates the digest of the next scheduled fork, regardless of whether it is a regular
	// or a Blob-Parameters-Only fork. This new entry MUST be added once FULU_FORK_EPOCH is assigned
	// any value other than FAR_FUTURE_EPOCH. Adding this entry prior to the Fulu fork will not
	// impact peering as nodes will ignore unknown ENR entries and nfd mismatches do not cause
	// disconnects.
	// When discovering and interfacing with peers, nodes MUST evaluate nfd alongside their existing
	// consideration of the ENRForkID::next_* fields under the eth2 key, to form a more accurate
	// view of the peer's intended next fork for the purposes of sustained peering. If there is a
	// mismatch, the node MUST NOT disconnect before the fork boundary, but it MAY disconnect
	// at/after the fork boundary.

	// Nodes unprepared to follow the Fulu fork will be unaware of nfd entries. However, their
	// existing comparison of eth2 entries (concretely next_fork_epoch) is sufficient to detect
	// upcoming divergence.
	// ---

	// Because this is a new in-bound connection, we lean into the pre-fulu point that clients
	// MAY connect to peers with the same current_fork_version but a different
	// next_fork_version/next_fork_epoch, which implies we can chose to not connect to them when these
	// don't match.
	//
	// Given that the next_fork_epoch matches, we will require the next_fork_digest to match.
	if !params.FuluEnabled() {
		return nil
	}
	peerNFD, selfNFD := nfd(peer), nfd(self)
	if peerNFD != selfNFD {
		return errors.Wrapf(errNextDigestMismatch,
			"next fork digest of peer with ENR %s: %v, does not match local value: %v",
			peerString, peerNFD, selfNFD)
	}
	return nil
}

func updateENR(node *enode.LocalNode, entry, next params.NetworkScheduleEntry) error {
	enrForkID := &pb.ENRForkID{
		CurrentForkDigest: entry.ForkDigest[:],
		NextForkVersion:   next.ForkVersion[:],
		NextForkEpoch:     next.Epoch,
	}
	if entry.Epoch == next.Epoch {
		enrForkID.NextForkEpoch = params.BeaconConfig().FarFutureEpoch
	}
	logFields := logrus.Fields{
		"CurrentForkDigest": fmt.Sprintf("%#x", enrForkID.CurrentForkDigest),
		"NextForkVersion":   fmt.Sprintf("%#x", enrForkID.NextForkVersion),
		"NextForkEpoch":     fmt.Sprintf("%d", enrForkID.NextForkEpoch),
	}
	if params.BeaconConfig().FuluForkEpoch != params.BeaconConfig().FarFutureEpoch {
		if entry.ForkDigest == next.ForkDigest {
			node.Set(enr.WithEntry(nfdEnrKey, make([]byte, len(next.ForkDigest))))
		} else {
			node.Set(enr.WithEntry(nfdEnrKey, next.ForkDigest[:]))
		}
		logFields["NextForkDigest"] = fmt.Sprintf("%#x", next.ForkDigest)
	}
	log.WithFields(logFields).Info("Updating ENR Fork ID")
	enc, err := enrForkID.MarshalSSZ()
	if err != nil {
		return err
	}
	forkEntry := enr.WithEntry(eth2EnrKey, enc)
	node.Set(forkEntry)
	return nil
}

// Retrieves an enrForkID from an ENR record by key lookup
// under the Ethereum consensus EnrKey
func forkEntry(record *enr.Record) (*pb.ENRForkID, error) {
	sszEncodedForkEntry := make([]byte, 16)
	entry := enr.WithEntry(eth2EnrKey, &sszEncodedForkEntry)
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

// nfd retrieves the value of the `nfd` ("next fork digest") key from an ENR record.
func nfd(record *enr.Record) [4]byte {
	digest := [4]byte{}
	entry := enr.WithEntry(nfdEnrKey, &digest)
	if err := record.Load(entry); err != nil {
		// Treat a missing nfd entry as an empty digest.
		// We do this to avoid errors when checking peers that have not upgraded for fulu.
		return [4]byte{}
	}
	return digest
}
