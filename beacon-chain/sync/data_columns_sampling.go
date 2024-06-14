package sync

import (
	"context"
	"fmt"
	"sort"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed"
	statefeed "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed/state"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/crypto/rand"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// reandomIntegers returns a map of `count` random integers in the range [0, max[.
func randomIntegers(count uint64, max uint64) map[uint64]bool {
	result := make(map[uint64]bool, count)
	randGenerator := rand.NewGenerator()

	for uint64(len(result)) < count {
		n := randGenerator.Uint64() % max
		result[n] = true
	}

	return result
}

// sortedListFromMap returns a sorted list of keys from a map.
func sortedListFromMap(m map[uint64]bool) []uint64 {
	result := make([]uint64, 0, len(m))
	for k := range m {
		result = append(result, k)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i] < result[j]
	})

	return result
}

// extractNodeID extracts the node ID from a peer ID.
func extractNodeID(pid peer.ID) ([32]byte, error) {
	var nodeID [32]byte

	// Retrieve the public key object of the peer under "crypto" form.
	pubkeyObjCrypto, err := pid.ExtractPublicKey()
	if err != nil {
		return nodeID, errors.Wrap(err, "extract public key")
	}

	// Extract the bytes representation of the public key.
	compressedPubKeyBytes, err := pubkeyObjCrypto.Raw()
	if err != nil {
		return nodeID, errors.Wrap(err, "public key raw")
	}

	// Retrieve the public key object of the peer under "SECP256K1" form.
	pubKeyObjSecp256k1, err := btcec.ParsePubKey(compressedPubKeyBytes)
	if err != nil {
		return nodeID, errors.Wrap(err, "parse public key")
	}

	// Concatenate the X and Y coordinates represented in bytes.
	buf := make([]byte, 64)
	math.ReadBits(pubKeyObjSecp256k1.X(), buf[:32])
	math.ReadBits(pubKeyObjSecp256k1.Y(), buf[32:])

	// Get the node ID by hashing the concatenated X and Y coordinates.
	nodeIDBytes := crypto.Keccak256(buf)
	copy(nodeID[:], nodeIDBytes)

	return nodeID, nil
}

// sampleDataColumnFromPeer samples data columns from a peer.
// It returns the missing columns after sampling.
func (s *Service) sampleDataColumnFromPeer(
	pid peer.ID,
	columnsToSample map[uint64]bool,
	requestedRoot [fieldparams.RootLength]byte,
) (map[uint64]bool, error) {
	// Define missing columns.
	missingColumns := make(map[uint64]bool, len(columnsToSample))
	for index := range columnsToSample {
		missingColumns[index] = true
	}

	// Retrieve the custody count of the peer.
	peerCustodiedSubnetCount := s.cfg.p2p.CustodyCountFromRemotePeer(pid)

	// Extract the node ID from the peer ID.
	nodeID, err := extractNodeID(pid)
	if err != nil {
		return nil, errors.Wrap(err, "extract node ID")
	}

	// Determine which columns the peer should custody.
	peerCustodiedColumns, err := peerdas.CustodyColumns(nodeID, peerCustodiedSubnetCount)
	if err != nil {
		return nil, errors.Wrap(err, "custody columns")
	}

	peerCustodiedColumnsList := sortedListFromMap(peerCustodiedColumns)

	// Compute the intersection of the columns to sample and the columns the peer should custody.
	peerRequestedColumns := make(map[uint64]bool, len(columnsToSample))
	for column := range columnsToSample {
		if peerCustodiedColumns[column] {
			peerRequestedColumns[column] = true
		}
	}

	peerRequestedColumnsList := sortedListFromMap(peerRequestedColumns)

	// Get the data column identifiers to sample from this peer.
	dataColumnIdentifiers := make(types.DataColumnSidecarsByRootReq, 0, len(peerRequestedColumns))
	for index := range peerRequestedColumns {
		dataColumnIdentifiers = append(dataColumnIdentifiers, &eth.DataColumnIdentifier{
			BlockRoot:   requestedRoot[:],
			ColumnIndex: index,
		})
	}

	// Return early if there are no data columns to sample.
	if len(dataColumnIdentifiers) == 0 {
		log.WithFields(logrus.Fields{
			"peerID":           pid,
			"custodiedColumns": peerCustodiedColumnsList,
			"requestedColumns": peerRequestedColumnsList,
		}).Debug("Peer does not custody any of the requested columns")
		return columnsToSample, nil
	}

	// Sample data columns.
	roDataColumns, err := SendDataColumnSidecarByRoot(s.ctx, s.cfg.clock, s.cfg.p2p, pid, s.ctxMap, &dataColumnIdentifiers)
	if err != nil {
		return nil, errors.Wrap(err, "send data column sidecar by root")
	}

	peerRetrievedColumns := make(map[uint64]bool, len(roDataColumns))

	// Remove retrieved items from rootsByDataColumnIndex.
	for _, roDataColumn := range roDataColumns {
		retrievedColumn := roDataColumn.ColumnIndex

		actualRoot := roDataColumn.BlockRoot()
		if actualRoot != requestedRoot {
			// TODO: Should we decrease the peer score here?
			log.WithFields(logrus.Fields{
				"peerID":        pid,
				"requestedRoot": fmt.Sprintf("%#x", requestedRoot),
				"actualRoot":    fmt.Sprintf("%#x", actualRoot),
			}).Warning("Actual root does not match requested root")

			continue
		}

		peerRetrievedColumns[retrievedColumn] = true

		if !columnsToSample[retrievedColumn] {
			// TODO: Should we decrease the peer score here?
			log.WithFields(logrus.Fields{
				"peerID":           pid,
				"retrievedColumn":  retrievedColumn,
				"requestedColumns": peerRequestedColumnsList,
			}).Warning("Retrieved column is was not requested")
		}

		delete(missingColumns, retrievedColumn)
	}

	peerRetrievedColumnsList := sortedListFromMap(peerRetrievedColumns)
	remainingMissingColumnsList := sortedListFromMap(missingColumns)

	log.WithFields(logrus.Fields{
		"peerID":                  pid,
		"custodiedColumns":        peerCustodiedColumnsList,
		"requestedColumns":        peerRequestedColumnsList,
		"retrievedColumns":        peerRetrievedColumnsList,
		"remainingMissingColumns": remainingMissingColumnsList,
	}).Debug("Peer data column sampling summary")

	return missingColumns, nil
}

// sampleDataColumns samples data columns from active peers.
func (s *Service) sampleDataColumns(requestedRoot [fieldparams.RootLength]byte, samplesCount uint64) error {
	// Determine `samplesCount` random column indexes.
	requestedColumns := randomIntegers(samplesCount, params.BeaconConfig().NumberOfColumns)

	missingColumns := make(map[uint64]bool, len(requestedColumns))
	for index := range requestedColumns {
		missingColumns[index] = true
	}

	// Get the active peers from the p2p service.
	activePeers := s.cfg.p2p.Peers().Active()

	var err error

	// Sampling is done sequentially peer by peer.
	// TODO: Add parallelism if (probably) needed.
	for _, pid := range activePeers {
		// Early exit if all needed columns are already sampled. (This is the happy path.)
		if len(missingColumns) == 0 {
			break
		}

		// Sample data columns from the peer.
		missingColumns, err = s.sampleDataColumnFromPeer(pid, missingColumns, requestedRoot)
		if err != nil {
			return errors.Wrap(err, "sample data column from peer")
		}
	}

	requestedColumnsList := sortedListFromMap(requestedColumns)

	if len(missingColumns) == 0 {
		log.WithField("requestedColumns", requestedColumnsList).Debug("Successfully sampled all requested columns")
		return nil
	}

	missingColumnsList := sortedListFromMap(missingColumns)
	log.WithFields(logrus.Fields{
		"requestedColumns": requestedColumnsList,
		"missingColumns":   missingColumnsList,
	}).Warning("Failed to sample some requested columns")

	return nil
}

func (s *Service) dataColumnSampling(ctx context.Context) {
	// Create a subscription to the state feed.
	stateChannel := make(chan *feed.Event, 1)
	stateSub := s.cfg.stateNotifier.StateFeed().Subscribe(stateChannel)

	// Unsubscribe from the state feed when the function returns.
	defer stateSub.Unsubscribe()

	for {
		select {
		case e := <-stateChannel:
			if e.Type != statefeed.BlockProcessed {
				continue
			}

			data, ok := e.Data.(*statefeed.BlockProcessedData)
			if !ok {
				log.Error("Event feed data is not of type *statefeed.BlockProcessedData")
				continue
			}

			if !data.Verified {
				// We only process blocks that have been verified
				log.Error("Data is not verified")
				continue
			}

			if data.SignedBlock.Version() < version.Deneb {
				log.Debug("Pre Deneb block, skipping data column sampling")
				continue
			}

			// Get the commitments for this block.
			commitments, err := data.SignedBlock.Block().Body().BlobKzgCommitments()
			if err != nil {
				log.WithError(err).Error("Failed to get blob KZG commitments")
				continue
			}

			// Skip if there are no commitments.
			if len(commitments) == 0 {
				log.Debug("No commitments in block, skipping data column sampling")
				continue
			}

			dataColumnSamplingCount := params.BeaconConfig().SamplesPerSlot

			// Sample data columns.
			if err := s.sampleDataColumns(data.BlockRoot, dataColumnSamplingCount); err != nil {
				log.WithError(err).Error("Failed to sample data columns")
			}

		case <-s.ctx.Done():
			log.Debug("Context closed, exiting goroutine")
			return

		case err := <-stateSub.Err():
			log.WithError(err).Error("Subscription to state feed failed")
		}
	}
}
