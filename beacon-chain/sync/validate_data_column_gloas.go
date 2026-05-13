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

// pendingColumnEntry holds every (peer, sidecar) pair we've received for a given
// (block_root, column_index) tuple. Retaining one slot per peer lets us:
//   - downscore every forwarding peer whose offering fails verification once the
//     block arrives, and
//   - keep per-peer-per-subnet queue coverage, subject to the queue caps above,
//     as recommended by consensus-specs #5199.
type pendingColumnEntry struct {
	sidecars map[peer.ID]*ethpb.DataColumnSidecarGloas
}

type pendingGloasEntry struct {
	slot    primitives.Slot
	columns [fieldparams.NumberOfColumns]*pendingColumnEntry
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
//	H1 -> (R1, 0)  accepted. H1: cols=1, roots=1.
//	H1 -> (R1, 5)  accepted (same root). H1: cols=2, roots=1.
//	H1 -> (R2, 0)  accepted. H1: cols=3, roots=2.
//	H1 -> (R3, 0)  DROP — H1 root cap.
//	H2 -> (R1, 0)  accepted alongside H1. H2: cols=1, roots=1.
//	H2 -> (R1, 0)  DROP — same-peer cell dedup.
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
	// Example. Existing entry for R1:
	//   columns[0].sidecars = {H1, H2}
	//   columns[5].sidecars = {H1}
	// Then:
	//   H1 -> (R1, 9) inserts -> newRootForPeer = false (H1 already at col 0 and 5).
	//   H3 -> (R1, 9) inserts -> newRootForPeer = true  (H3 nowhere under R1).
	newRootForPeer := true
	if exists {
		for _, pe := range entry.columns {
			if pe == nil {
				continue
			}
			if _, ok := pe.sidecars[pid]; ok {
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

	pe := entry.columns[idx]
	if pe == nil {
		pe = &pendingColumnEntry{sidecars: make(map[peer.ID]*ethpb.DataColumnSidecarGloas)}
		entry.columns[idx] = pe
	}
	// Same-peer dedup: must not double-charge quotas.
	if _, dup := pe.sidecars[pid]; dup {
		return
	}
	pe.sidecars[pid] = dc
	s.pendingGloasPeerColumnCounts[pid]++
	if newRootForPeer {
		s.pendingGloasPeerRootCounts[pid]++
	}
}

// processPendingGloasColumns drains the queued sidecars for `root` after the block
// arrives. For each (root, index), KZG-verify every peer's offering; first pass wins
// and is saved; failing peers are downscored.
//
// Example for one cell (R1, 0) with offerings {H1: good, M: bad, H2: good}:
//
//	H1 verify pass -> winner = H1
//	M  verify fail -> badPeers[M] = true
//	H2 verify pass -> discarded (already have winner)
//	-> save H1, Increment(M).
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

	// Pre-allocate based on number of populated indices; each contributes at
	// most one verified sidecar to the saved set.
	count := 0
	for _, pe := range entry.columns {
		if pe != nil {
			count++
		}
	}

	verified := make([]blocks.VerifiedRODataColumn, 0, count)
	var skipped int
	badPeers := make(map[peer.ID]bool)
	// Outer: each column index. Inner: each peer's offering for that cell.
	for _, pe := range entry.columns {
		if pe == nil {
			continue
		}
		var winner blocks.VerifiedRODataColumn
		haveWinner := false
		alreadySeen := false
		for pid, sidecar := range pe.sidecars {
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
		} else if len(pe.sidecars) > 0 && !alreadySeen {
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

// releasePendingGloasPeerCounts undoes per-peer bookkeeping for a removed entry.
// Column count: -1 per cell the peer was in. Root count: -1 per peer (once per entry).
//
// Example. Entry holds:
//
//	col[0] = {H1, H2}, col[5] = {H1}, col[9] = {H2, M}
//	Before: cols{H1:2, H2:2, M:1}, roots{H1:1, H2:1, M:1}
//	After:  cols{}, roots{}
//
// Caller must hold pendingGloasColumnsLock for writing.
func (s *Service) releasePendingGloasPeerCounts(entry *pendingGloasEntry) {
	rootSeen := make(map[peer.ID]bool)
	for _, pe := range entry.columns {
		if pe == nil {
			continue
		}
		for pid := range pe.sidecars {
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
