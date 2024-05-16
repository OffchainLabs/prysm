package sync

import (
	"context"
	"sort"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed"
	statefeed "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed/state"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/crypto/rand"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/sirupsen/logrus"
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

func (s *Service) sampleDataColumns(requestedRoot [fieldparams.RootLength]byte, samplesCount uint64) (map[uint64]bool, error) {
	// Determine `samplesCount` random column indexes.
	missingIndices := randomIntegers(samplesCount, params.BeaconConfig().NumberOfColumns)

	// Get the active peers from the p2p service.
	activePeers := s.cfg.p2p.Peers().Active()

	// Sampling is done sequentially peer by peer.
	// TODO: Add parallelism if (probably) needed.
	for _, peer := range activePeers {
		// Early exit if all needed columns are already sampled.
		// This is the happy path.
		if len(missingIndices) == 0 {
			return nil, nil
		}

		// Retrieve the ENR of the peer.
		peerRecord, err := s.cfg.p2p.Peers().ENR(peer)
		if err != nil {
			return nil, errors.Wrap(err, "ENR")
		}

		peerCustodiedSubnetCount := params.BeaconConfig().CustodyRequirement

		if peerRecord != nil {
			// Load the `custody_subnet_count`
			// TODO: Do not harcode `custody_subnet_count`
			custodyBytes := make([]byte, 8)
			if err := peerRecord.Load(p2p.CustodySubnetCount(custodyBytes)); err != nil {
				return nil, errors.Wrap(err, "load custody_subnet_count")
			}
			actualCustodyCount := ssz.UnmarshallUint64(custodyBytes)

			if actualCustodyCount > peerCustodiedSubnetCount {
				peerCustodiedSubnetCount = actualCustodyCount
			}
		}

		// Retrieve the public key object of the peer under "crypto" form.
		pubkeyObjCrypto, err := peer.ExtractPublicKey()
		if err != nil {
			return nil, errors.Wrap(err, "extract public key")
		}

		// Extract the bytes representation of the public key.
		compressedPubKeyBytes, err := pubkeyObjCrypto.Raw()
		if err != nil {
			return nil, errors.Wrap(err, "public key raw")
		}

		// Retrieve the public key object of the peer under "SECP256K1" form.
		pubKeyObjSecp256k1, err := btcec.ParsePubKey(compressedPubKeyBytes)
		if err != nil {
			return nil, errors.Wrap(err, "parse public key")
		}

		// Concatenate the X and Y coordinates represented in bytes.
		buf := make([]byte, 64)
		math.ReadBits(pubKeyObjSecp256k1.X(), buf[:32])
		math.ReadBits(pubKeyObjSecp256k1.Y(), buf[32:])

		// Get the peer ID by hashing the concatenated X and Y coordinates.
		peerIDBytes := crypto.Keccak256(buf)

		var peerID [32]byte
		copy(peerID[:], peerIDBytes)

		// Determine which columns the peer should custody.
		peerCustodiedColumns, err := peerdas.CustodyColumns(peerID, peerCustodiedSubnetCount)
		if err != nil {
			return nil, errors.Wrap(err, "custody columns")
		}

		// Determine how many columns are yet missing.
		missingColumnsCount := len(missingIndices)

		// Get the data column identifiers to sample from this particular peer.
		dataColumnIdentifiers := make(types.BlobSidecarsByRootReq, 0, missingColumnsCount)

		for index := range missingIndices {
			if peerCustodiedColumns[index] {
				dataColumnIdentifiers = append(dataColumnIdentifiers, &eth.BlobIdentifier{
					BlockRoot: requestedRoot[:],
					Index:     index,
				})
			}
		}

		// Skip the peer if there are no data columns to sample.
		if len(dataColumnIdentifiers) == 0 {
			continue
		}

		// Sample data columns.
		roDataColumns, err := SendDataColumnSidecarByRoot(s.ctx, s.cfg.clock, s.cfg.p2p, peer, s.ctxMap, &dataColumnIdentifiers)
		if err != nil {
			return nil, errors.Wrap(err, "send data column sidecar by root")
		}

		// Remove retrieved items from rootsByDataColumnIndex.
		for _, roDataColumn := range roDataColumns {
			index := roDataColumn.ColumnIndex

			actualRoot := roDataColumn.BlockRoot()
			if actualRoot != requestedRoot {
				return nil, errors.Errorf("actual root (%#x) does not match requested root (%#x)", actualRoot, requestedRoot)
			}

			delete(missingIndices, index)
		}
	}

	// We tried all our active peers and some columns are still missing.
	// This is the unhappy path.
	return missingIndices, nil
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
			missingColumns, err := s.sampleDataColumns(data.BlockRoot, dataColumnSamplingCount)
			if err != nil {
				log.WithError(err).Error("Failed to sample data columns")
				continue
			}

			missingColumnsCount := len(missingColumns)

			missingColumnsList := make([]uint64, 0, missingColumnsCount)
			for column := range missingColumns {
				missingColumnsList = append(missingColumnsList, column)
			}

			// Sort the missing columns list.
			sort.Slice(missingColumnsList, func(i, j int) bool {
				return missingColumnsList[i] < missingColumnsList[j]
			})

			if missingColumnsCount > 0 {
				log.WithFields(logrus.Fields{
					"missingColumns":      missingColumnsList,
					"sampledColumnsCount": dataColumnSamplingCount,
				}).Warning("Failed to sample some data columns")
				continue
			}

			log.WithField("sampledColumnsCount", dataColumnSamplingCount).Info("Successfully sampled all data columns")

		case <-s.ctx.Done():
			log.Debug("Context closed, exiting goroutine")
			return

		case err := <-stateSub.Err():
			log.WithError(err).Error("Subscription to state feed failed")
		}
	}
}
