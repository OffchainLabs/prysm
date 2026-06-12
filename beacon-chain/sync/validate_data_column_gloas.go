package sync

import (
	"context"
	"fmt"
	"strings"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/logging"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// maxPendingGloasRoots caps the number of distinct block roots in the pending queue.
const maxPendingGloasRoots = 8

type pendingColumnEntry struct {
	sidecar *ethpb.DataColumnSidecarGloas
	peer    peer.ID
}

type pendingGloasEntry struct {
	slot    primitives.Slot
	columns [fieldparams.NumberOfColumns]*pendingColumnEntry
}

func (s *Service) validateDataColumnGloas(
	ctx context.Context,
	pid peer.ID,
	msg *pubsub.Message,
	roDataColumn blocks.RODataColumn,
	dataColumnSidecarSubTopic string,
) (blocks.VerifiedRODataColumn, error) {
	// data_column_sidecar_{subnet_id}
	// [Modified in Gloas:EIP7732]
	//
	// [IGNORE] A valid block for the sidecar's slot has been seen (via gossip or non-gossip sources).
	// If not yet seen, a client MUST queue the sidecar for deferred validation and possible processing once
	// the block is received or retrieved.
	if s.cfg.chain == nil || !s.cfg.chain.HasBlock(ctx, roDataColumn.BlockRoot()) {
		actualSubnet := peerdas.ComputeSubnetForDataColumnSidecar(roDataColumn.Index())
		expectedSubTopic := fmt.Sprintf(dataColumnSidecarSubTopic, actualSubnet)
		if msg.Topic == nil || !strings.Contains(*msg.Topic+"/", expectedSubTopic) {
			return blocks.VerifiedRODataColumn{}, errors.New("gloas data column on wrong subnet")
		}
		if err := s.queuePendingGloasColumn(roDataColumn, pid); err != nil {
			return blocks.VerifiedRODataColumn{}, err
		}
		return blocks.VerifiedRODataColumn{}, ignoreValidation(errors.New("gloas data column block not yet seen"))
	}

	block, err := s.cfg.beaconDB.Block(ctx, roDataColumn.BlockRoot())
	if err != nil {
		return blocks.VerifiedRODataColumn{}, ignoreValidation(err)
	}
	verifier := verification.NewGloasDataColumnVerifier(roDataColumn, block.Block(), verification.GossipDataColumnSidecarRequirementsGloas)
	verifier.SatisfyRequirement(verification.RequireBlockSeenGloas)

	// [REJECT] The sidecar's slot matches the slot of the block with root beacon_block_root.
	if err := verifier.VerifyDataColumnSidecarSlotMatchesBlockGloas(); err != nil {
		return blocks.VerifiedRODataColumn{}, errors.Wrap(err, "gloas data column validation")
	}

	// [REJECT] The sidecar is valid as verified by verify_data_column_sidecar(sidecar, bid.blob_kzg_commitments).
	if err := verifier.VerifyDataColumnSidecarGloas(); err != nil {
		return blocks.VerifiedRODataColumn{}, errors.Wrap(err, "gloas data column validation")
	}

	// [REJECT] The sidecar is for the correct subnet -- i.e.
	// compute_subnet_for_data_column_sidecar(sidecar.index) == subnet_id.
	if err := verifier.CorrectSubnet(dataColumnSidecarSubTopic, []string{*msg.Topic}); err != nil {
		return blocks.VerifiedRODataColumn{}, errors.Wrap(err, "gloas data column validation")
	}

	// [REJECT] The sidecar's column data is valid as verified by
	// verify_data_column_sidecar_kzg_proofs(sidecar, bid.blob_kzg_commitments).
	if err := verifier.VerifyDataColumnSidecarKzgProofsGloas(); err != nil {
		return blocks.VerifiedRODataColumn{}, errors.Wrap(err, "gloas data column validation")
	}

	// [IGNORE] The sidecar is the first sidecar for the tuple
	// (sidecar.beacon_block_root, sidecar.index) with valid kzg proof.
	//
	// Note: If the sidecar fails deferred validation, its forwarding peers MUST be downscored
	// retroactively. If validation succeeds, the client MUST re-broadcast the sidecar.
	if s.hasSeenDataColumnRootIndex(roDataColumn.BlockRoot(), roDataColumn.Index()) {
		return blocks.VerifiedRODataColumn{}, ignoreValidation(errors.New("data column sidecar already seen for block root"))
	}
	verifier.SatisfyRequirement(verification.RequireNotSeenGloas)

	verifiedRODataColumn, err := verifier.VerifiedRODataColumn()
	if err != nil {
		log.WithError(err).WithFields(logging.DataColumnFields(roDataColumn)).Error("Failed to get verified gloas data columns")
		return blocks.VerifiedRODataColumn{}, ignoreValidation(err)
	}

	commitments, err := block.Block().Body().BlobKzgCommitments()
	if err != nil {
		return blocks.VerifiedRODataColumn{}, ignoreValidation(errors.Wrap(err, "get bid blob kzg commitments"))
	}
	verifiedRODataColumn.SetBidCommitments(commitments)

	s.setSeenDataColumnRootIndex(verifiedRODataColumn.BlockRoot(), verifiedRODataColumn.Index(), verifiedRODataColumn.Slot())
	return verifiedRODataColumn, nil
}

func (s *Service) hasSeenDataColumnRootIndex(root [fieldparams.RootLength]byte, index uint64) bool {
	key := computeRootIndexCacheKey(root, index)
	_, seen := s.seenDataColumnCache.Get(key)
	return seen
}

func (s *Service) setSeenDataColumnRootIndex(root [fieldparams.RootLength]byte, index uint64, slot primitives.Slot) {
	key := computeRootIndexCacheKey(root, index)
	s.seenDataColumnCache.Add(slot, key, true)
}

// queuePendingGloasColumn returns a non-nil error for malformed sidecars (the caller propagates it as ValidationReject).
func (s *Service) queuePendingGloasColumn(roCol blocks.RODataColumn, pid peer.ID) error {
	dc := roCol.DataColumnSidecarGloas()
	if dc == nil {
		return errors.New("nil gloas data column sidecar")
	}
	cells := len(dc.Column)
	if cells == 0 || len(dc.KzgProofs) != cells {
		return errors.Errorf("gloas data column length mismatch: cells=%d proofs=%d", cells, len(dc.KzgProofs))
	}
	cfg := params.BeaconConfig()
	currentEpoch := slots.ToEpoch(s.cfg.clock.CurrentSlot())
	if cells > max(cfg.MaxBlobsPerBlockAtEpoch(currentEpoch), cfg.MaxBlobsPerBlockAtEpoch(currentEpoch+1)) {
		return errors.Errorf("gloas data column cell count %d exceeds network blob limit", cells)
	}
	idx := roCol.Index()
	if idx >= fieldparams.NumberOfColumns {
		return errors.Errorf("gloas data column index %d out of range", idx)
	}

	root := roCol.BlockRoot()
	slot := roCol.Slot()

	s.pendingGloasColumnsLock.Lock()
	defer s.pendingGloasColumnsLock.Unlock()

	entry := s.pendingGloasColumns[root]
	if entry == nil {
		if len(s.pendingGloasColumns) >= maxPendingGloasRoots {
			return nil
		}
		entry = &pendingGloasEntry{slot: slot}
		s.pendingGloasColumns[root] = entry
	}

	if entry.columns[idx] != nil {
		return nil
	}
	entry.columns[idx] = &pendingColumnEntry{sidecar: dc, peer: pid}
	return nil
}

func (s *Service) processPendingGloasColumns(root [fieldparams.RootLength]byte, blk interfaces.ReadOnlySignedBeaconBlock) {
	if blk == nil || blk.IsNil() {
		return
	}

	s.pendingGloasColumnsLock.Lock()
	entry := s.pendingGloasColumns[root]
	delete(s.pendingGloasColumns, root)
	s.pendingGloasColumnsLock.Unlock()

	if entry == nil {
		return
	}

	commitments, err := blk.Block().Body().BlobKzgCommitments()
	if err != nil {
		log.WithError(err).WithField("root", fmt.Sprintf("%#x", root)).Warn("Failed to get bid commitments for pending Gloas columns")
		return
	}

	// Count pending sidecars for pre-allocation.
	count := 0
	for _, pe := range entry.columns {
		if pe != nil {
			count++
		}
	}

	verified := make([]blocks.VerifiedRODataColumn, 0, count)
	var skipped int
	badPeers := make(map[peer.ID]bool)
	for _, pe := range entry.columns {
		if pe == nil {
			continue
		}
		roCol, err := blocks.NewRODataColumnGloasWithRoot(pe.sidecar, root)
		if err != nil {
			log.WithError(err).WithField("root", fmt.Sprintf("%#x", root)).Error("Failed to wrap pending Gloas column")
			skipped++
			continue
		}
		roCol.SetBidCommitments(commitments)

		if s.hasSeenDataColumnRootIndex(root, roCol.Index()) {
			continue
		}

		verifier := verification.NewGloasDataColumnVerifier(roCol, blk.Block(), verification.PendingGloasColumnRequirements)

		if err := verifier.VerifyDataColumnSidecarSlotMatchesBlockGloas(); err != nil {
			skipped++
			badPeers[pe.peer] = true
			continue
		}
		if err := verifier.VerifyDataColumnSidecarGloas(); err != nil {
			skipped++
			badPeers[pe.peer] = true
			continue
		}
		if err := verifier.VerifyDataColumnSidecarKzgProofsGloas(); err != nil {
			skipped++
			badPeers[pe.peer] = true
			continue
		}

		v, err := verifier.VerifiedRODataColumn()
		if err != nil {
			log.WithError(err).WithField("root", fmt.Sprintf("%#x", root)).Error("Failed to get verified pending Gloas column")
			skipped++
			continue
		}
		v.SetBidCommitments(commitments)

		s.setSeenDataColumnRootIndex(root, v.Index(), v.Slot())
		verified = append(verified, v)
	}

	for pid := range badPeers {
		s.cfg.p2p.Peers().Scorers().BadResponsesScorer().Increment(pid)
	}

	if len(verified) > 0 {
		if err := s.cfg.dataColumnStorage.Save(verified); err != nil {
			log.WithError(err).WithField("root", fmt.Sprintf("%#x", root)).Warn("Failed to save pending Gloas columns")
			return
		}

		log.WithFields(logrus.Fields{
			"root":    fmt.Sprintf("%#x", root),
			"count":   len(verified),
			"skipped": skipped,
			"slot":    entry.slot,
		}).Debug("Processed pending Gloas data columns")
	}
}

func (s *Service) hasPendingGloasColumns(root [fieldparams.RootLength]byte) bool {
	s.pendingGloasColumnsLock.RLock()
	defer s.pendingGloasColumnsLock.RUnlock()
	_, ok := s.pendingGloasColumns[root]
	return ok
}

// prunePendingGloasColumns removes stale entries every slot.
func (s *Service) prunePendingGloasColumns() {
	slotTicker := slots.NewSlotTicker(s.cfg.clock.GenesisTime(), params.BeaconConfig().SecondsPerSlot)
	defer slotTicker.Done()
	for {
		select {
		case currentSlot := <-slotTicker.C():
			s.pendingGloasColumnsLock.Lock()
			for r, e := range s.pendingGloasColumns {
				if e.slot+1 < currentSlot {
					delete(s.pendingGloasColumns, r)
				}
			}
			s.pendingGloasColumnsLock.Unlock()
		case <-s.ctx.Done():
			return
		}
	}
}

func computeRootIndexCacheKey(root [fieldparams.RootLength]byte, index uint64) string {
	key := make([]byte, 0, fieldparams.RootLength+32)
	key = append(key, root[:]...)
	key = append(key, bytesutil.Bytes32(index)...)
	return string(key)
}
