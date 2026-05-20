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

// maxPendingGloasColumnsPerPeer caps the number of queued columns attributable to a single
// peer at any time, so a malicious peer cannot fill the queue and starve honest peers
// (consensus-specs #5199).
const maxPendingGloasColumnsPerPeer = fieldparams.NumberOfColumns

// maxPendingGloasRootsPerPeer caps the number of distinct block roots a single peer can
// introduce into the pending queue, so one peer cannot occupy every root slot and shut
// other peers out. This is stricter than consensus-specs #5199's per-peer-per-subnet
// recommendation, but preserves space for multiple peers under the global root cap.
const maxPendingGloasRootsPerPeer = 2

// pendingGloasEntry holds every queued (peer, sidecar) for one block root: each
// column index maps peer.ID -> sidecar. Keeping a separate sidecar per peer per cell
// lets us downscore every forwarding peer whose offering fails verification once the
// block arrives, and keep per-peer-per-subnet queue coverage as recommended by
// consensus-specs #5199.
type pendingGloasEntry struct {
	slot    primitives.Slot
	columns [fieldparams.NumberOfColumns]map[peer.ID]*ethpb.DataColumnSidecarGloas
}

// validateDataColumnGloas validates a Gloas data column sidecar from gossip.
//
// Two paths:
//   - block not seen -> queue, return Ignore. Verified later when block arrives.
//   - block seen     -> KZG-verify inline. Pass -> Accept, fail -> Reject.
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
	// If not yet seen, a client SHOULD queue the sidecar for deferred validation and possible processing
	// once the block is received or retrieved. Per consensus-specs #5199, queueing is per peer per
	// subnet so a single peer cannot occupy all queue slots.
	if s.cfg.chain == nil || !s.cfg.chain.HasBlock(ctx, roDataColumn.BlockRoot()) {
		actualSubnet := peerdas.ComputeSubnetForDataColumnSidecar(roDataColumn.Index())
		expectedSubTopic := fmt.Sprintf(dataColumnSidecarSubTopic, actualSubnet)
		if msg.Topic == nil || !strings.Contains(*msg.Topic+"/", expectedSubTopic) {
			return blocks.VerifiedRODataColumn{}, errors.New("gloas data column on wrong subnet")
		}
		s.queuePendingGloasColumn(roDataColumn, pid)
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

// queuePendingGloasColumn parks a sidecar until its block arrives. Caps:
//   - global: 8 roots
//   - per peer: 128 columns, 2 roots
//
// Each (root, index) holds map[peer.ID]*sidecar so multiple peers' offerings are
// all retained for later verification + per-peer downscoring.
//
// Example (empty queue):
//
//	peer1 -> (root1, 0)  accepted. peer1: cols=1, roots=1.
//	peer1 -> (root1, 5)  accepted (same root). peer1: cols=2, roots=1.
//	peer1 -> (root2, 0)  accepted. peer1: cols=3, roots=2.
//	peer1 -> (root3, 0)  DROP — peer1 root cap.
//	peer2 -> (root1, 0)  accepted alongside peer1. peer2: cols=1, roots=1.
//	peer2 -> (root1, 0)  DROP — same-peer cell dedup.
func (s *Service) queuePendingGloasColumn(roCol blocks.RODataColumn, pid peer.ID) {
	dc := roCol.DataColumnSidecarGloas()
	if dc == nil {
		return
	}
	idx := roCol.Index()
	if idx >= fieldparams.NumberOfColumns {
		return
	}

	root := roCol.BlockRoot()
	slot := roCol.Slot()

	s.pendingGloasColumnsLock.Lock()
	defer s.pendingGloasColumnsLock.Unlock()

	// Per-peer column cap.
	if s.pendingGloasPeerColumnCounts[pid] >= maxPendingGloasColumnsPerPeer {
		return
	}

	entry, exists := s.pendingGloasColumns[root]

	// newRootForPeer asks: would inserting this column cause pid to claim this
	// root for the first time? If yes, it consumes one of pid's 2 root slots;
	// if no, it just adds a column under a root pid already owns.
	//
	// Example. Existing entry for root1:
	//   columns[0] = {peer1, peer2}
	//   columns[5] = {peer1}
	// Then:
	//   peer1 -> (root1, 9) inserts -> newRootForPeer = false (peer1 already at col 0 and 5).
	//   peer3 -> (root1, 9) inserts -> newRootForPeer = true  (peer3 nowhere under root1).
	newRootForPeer := true
	if exists {
		for _, cell := range entry.columns {
			if _, ok := cell[pid]; ok {
				newRootForPeer = false
				break
			}
		}
	}
	// Per-peer root cap.
	if newRootForPeer && s.pendingGloasPeerRootCounts[pid] >= maxPendingGloasRootsPerPeer {
		return
	}

	if !exists {
		// Global root cap.
		if len(s.pendingGloasColumns) >= maxPendingGloasRoots {
			return
		}
		entry = &pendingGloasEntry{slot: slot}
		s.pendingGloasColumns[root] = entry
	}

	cell := entry.columns[idx]
	if cell == nil {
		cell = make(map[peer.ID]*ethpb.DataColumnSidecarGloas)
		entry.columns[idx] = cell
	}
	// Same-peer dedup: must not double-charge quotas.
	if _, dup := cell[pid]; dup {
		return
	}
	cell[pid] = dc
	s.pendingGloasPeerColumnCounts[pid]++
	if newRootForPeer {
		s.pendingGloasPeerRootCounts[pid]++
	}
}

// processPendingGloasColumns drains the queued sidecars for `root` after the block
// arrives. For each (root, index), KZG-verify every peer's offering; first pass wins
// and is saved; failing peers are downscored.
//
// Example for one cell (root1, 0) with offerings {goodpeer1: good, attacker: bad, goodpeer2: good}:
//
//	goodpeer1 verify pass -> winner = goodpeer1
//	attacker  verify fail -> badPeers[attacker] = true
//	goodpeer2 verify pass -> discarded (already have winner)
//	-> save goodpeer1's sidecar, Increment(attacker).
//
// alreadySeen avoids logging "skipped" when the column was saved via another path.
func (s *Service) processPendingGloasColumns(root [fieldparams.RootLength]byte, blk interfaces.ReadOnlySignedBeaconBlock) {
	if blk == nil || blk.IsNil() {
		return
	}

	s.pendingGloasColumnsLock.Lock()
	entry := s.pendingGloasColumns[root]
	delete(s.pendingGloasColumns, root)
	if entry != nil {
		s.releasePendingGloasPeerCounts(entry)
	}
	s.pendingGloasColumnsLock.Unlock()

	if entry == nil {
		return
	}

	commitments, err := blk.Block().Body().BlobKzgCommitments()
	if err != nil {
		log.WithError(err).WithField("root", fmt.Sprintf("%#x", root)).Warn("Failed to get bid commitments for pending Gloas columns")
		return
	}

	verified := make([]blocks.VerifiedRODataColumn, 0, fieldparams.NumberOfColumns)
	var skipped int
	badPeers := make(map[peer.ID]bool)
	// Outer: each column index. Inner: each peer's offering for that cell.
	for _, cell := range entry.columns {
		if cell == nil {
			continue
		}
		var winner blocks.VerifiedRODataColumn
		haveWinner := false
		alreadySeen := false
		for pid, sidecar := range cell {
			roCol, err := blocks.NewRODataColumnGloasWithRoot(sidecar, root)
			if err != nil {
				log.WithError(err).WithField("root", fmt.Sprintf("%#x", root)).Error("Failed to wrap pending Gloas column")
				skipped++
				continue
			}
			roCol.SetBidCommitments(commitments)

			verifier := verification.NewGloasDataColumnVerifier(roCol, blk.Block(), verification.PendingGloasColumnRequirements)

			if err := verifier.VerifyDataColumnSidecarSlotMatchesBlockGloas(); err != nil {
				badPeers[pid] = true
				continue
			}
			if err := verifier.VerifyDataColumnSidecarGloas(); err != nil {
				badPeers[pid] = true
				continue
			}
			if err := verifier.VerifyDataColumnSidecarKzgProofsGloas(); err != nil {
				badPeers[pid] = true
				continue
			}
			if haveWinner {
				continue
			}
			if s.hasSeenDataColumnRootIndex(root, roCol.Index()) {
				alreadySeen = true
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
			winner = v
			haveWinner = true
		}
		if haveWinner {
			verified = append(verified, winner)
		} else if len(cell) > 0 && !alreadySeen {
			// Every peer's offering for this index was rejected.
			skipped++
		}
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
					s.releasePendingGloasPeerCounts(e)
					delete(s.pendingGloasColumns, r)
				}
			}
			s.pendingGloasColumnsLock.Unlock()
		case <-s.ctx.Done():
			return
		}
	}
}

// releasePendingGloasPeerCounts undoes per-peer bookkeeping for ONE removed entry.
// It does NOT reset s.pendingGloasPeerColumnCounts / s.pendingGloasPeerRootCounts
// wholesale — those maps span every entry, and other entries' contributions stay live.
//
// Per peer in this entry:
//   - ColumnCounts[pid]: -1 per cell the peer appears in.
//   - RootCounts[pid]:   -1 once (rootSeen guards against double-decrement).
//
// Example. s.pendingGloasColumns holds:
//
//	s.pendingGloasColumns[root1].columns[0] = {peer1, peer2}
//	s.pendingGloasColumns[root1].columns[5] = {peer1}
//	s.pendingGloasColumns[root2].columns[3] = {peer1, peer3}
//
//	                          s.pendingGloasPeerColumnCounts             s.pendingGloasPeerRootCounts
//	before release of root1:  {peer1:3, peer2:1, peer3:1}                {peer1:2, peer2:1, peer3:1}
//	after  release of root1:  {peer1:1, peer3:1}                         {peer1:1, peer3:1}
//
// peer1's ColumnCounts drops 3 -> 1 (two cells in root1) but RootCounts only -1 (still
// owns root2). peer2 hits zero and is deleted. peer3 untouched (only in root2).
//
// Caller must hold pendingGloasColumnsLock for writing.
func (s *Service) releasePendingGloasPeerCounts(entry *pendingGloasEntry) {
	rootSeen := make(map[peer.ID]bool)
	for _, cell := range entry.columns {
		for pid := range cell {
			if s.pendingGloasPeerColumnCounts[pid] <= 1 {
				delete(s.pendingGloasPeerColumnCounts, pid)
			} else {
				s.pendingGloasPeerColumnCounts[pid]--
			}
			if rootSeen[pid] {
				continue
			}
			rootSeen[pid] = true
			if s.pendingGloasPeerRootCounts[pid] <= 1 {
				delete(s.pendingGloasPeerRootCounts, pid)
			} else {
				s.pendingGloasPeerRootCounts[pid]--
			}
		}
	}
}

func computeRootIndexCacheKey(root [fieldparams.RootLength]byte, index uint64) string {
	key := make([]byte, 0, fieldparams.RootLength+32)
	key = append(key, root[:]...)
	key = append(key, bytesutil.Bytes32(index)...)
	return string(key)
}
