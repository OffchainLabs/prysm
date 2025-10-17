package blockchain

import (
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	coreTime "github.com/OffchainLabs/prysm/v6/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	forkchoicetypes "github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1/attestation"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/go-bitfield"
	"github.com/sirupsen/logrus"
)

// A custom slot deadline for processing state slots in our cache.
const slotDeadline = 5 * time.Second

// A custom deadline for deposit trie insertion.
const depositDeadline = 20 * time.Second

// This defines size of the upper bound for initial sync block cache.
var initialSyncBlockCacheSize = uint64(2 * params.BeaconConfig().SlotsPerEpoch)

// postBlockProcessConfig is a structure that contains the data needed to
// process the beacon block after validating the state transition function
type postBlockProcessConfig struct {
	ctx            context.Context
	roblock        consensusblocks.ROBlock
	headRoot       [32]byte
	postState      state.BeaconState
	isValidPayload bool
}

// postBlockProcess is called when a gossip block is received. This function performs
// several duties most importantly informing the engine if head was updated,
// saving the new head information to the blockchain package and
// handling attestations, slashings and similar included in the block.
func (s *Service) postBlockProcess(cfg *postBlockProcessConfig) error {
	ctx, span := trace.StartSpan(cfg.ctx, "blockChain.onBlock")
	defer span.End()
	cfg.ctx = ctx
	if err := consensusblocks.BeaconBlockIsNil(cfg.roblock); err != nil {
		return invalidBlock{error: err}
	}
	startTime := time.Now()
	fcuArgs := &fcuConfig{}

	if s.inRegularSync() {
		defer s.handleSecondFCUCall(cfg, fcuArgs)
	}
	if features.Get().EnableLightClient && slots.ToEpoch(s.CurrentSlot()) >= params.BeaconConfig().AltairForkEpoch {
		defer s.processLightClientUpdates(cfg)
	}

	defer reportProcessingTime(startTime)
	defer reportAttestationInclusion(cfg.roblock.Block())

	err := s.cfg.ForkChoiceStore.InsertNode(ctx, cfg.postState, cfg.roblock)
	if err != nil {
		// Do not use parent context in the event it deadlined
		ctx = trace.NewContext(context.Background(), span)
		s.rollbackBlock(ctx, cfg.roblock.Root())
		return errors.Wrapf(err, "could not insert block %d to fork choice store", cfg.roblock.Block().Slot())
	}
	if err := s.handleBlockAttestations(ctx, cfg.roblock.Block(), cfg.postState); err != nil {
		return errors.Wrap(err, "could not handle block's attestations")
	}

	s.InsertSlashingsToForkChoiceStore(ctx, cfg.roblock.Block().Body().AttesterSlashings())
	if cfg.isValidPayload {
		if err := s.cfg.ForkChoiceStore.SetOptimisticToValid(ctx, cfg.roblock.Root()); err != nil {
			return errors.Wrap(err, "could not set optimistic block to valid")
		}
	}

	defer s.sendStateFeedOnBlock(cfg) // only send event after successful insertion
	start := time.Now()
	cfg.headRoot, err = s.cfg.ForkChoiceStore.Head(ctx)
	if err != nil {
		log.WithError(err).Warn("Could not update head")
	}
	newBlockHeadElapsedTime.Observe(float64(time.Since(start).Milliseconds()))
	if cfg.headRoot != cfg.roblock.Root() {
		s.logNonCanonicalBlockReceived(cfg.roblock.Root(), cfg.headRoot)
		return nil
	}
	if err := s.getFCUArgs(cfg, fcuArgs); err != nil {
		log.WithError(err).Error("Could not get forkchoice update argument")
		return nil
	}
	if err := s.sendFCU(cfg, fcuArgs); err != nil {
		return errors.Wrap(err, "could not send FCU to engine")
	}

	return nil
}

func getStateVersionAndPayload(st state.BeaconState) (int, interfaces.ExecutionData, error) {
	if st == nil {
		return 0, nil, errors.New("nil state")
	}
	var preStateHeader interfaces.ExecutionData
	var err error
	preStateVersion := st.Version()
	switch preStateVersion {
	case version.Phase0, version.Altair:
	default:
		preStateHeader, err = st.LatestExecutionPayloadHeader()
		if err != nil {
			return 0, nil, err
		}
	}
	return preStateVersion, preStateHeader, nil
}

func (s *Service) onBlockBatch(ctx context.Context, blks []consensusblocks.ROBlock, avs das.AvailabilityStore) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.onBlockBatch")
	defer span.End()

	if len(blks) == 0 {
		return errors.New("no blocks provided")
	}

	if err := consensusblocks.BeaconBlockIsNil(blks[0]); err != nil {
		return invalidBlock{error: err}
	}
	b := blks[0].Block()

	// Retrieve incoming block's pre state.
	if err := s.verifyBlkPreState(ctx, b.ParentRoot()); err != nil {
		return err
	}
	preState, err := s.cfg.StateGen.StateByRootInitialSync(ctx, b.ParentRoot())
	if err != nil {
		return err
	}
	if preState == nil || preState.IsNil() {
		return fmt.Errorf("nil pre state for slot %d", b.Slot())
	}

	// Fill in missing blocks
	if err := s.fillInForkChoiceMissingBlocks(ctx, blks[0], preState.FinalizedCheckpoint(), preState.CurrentJustifiedCheckpoint()); err != nil {
		return errors.Wrap(err, "could not fill in missing blocks to forkchoice")
	}

	jCheckpoints := make([]*ethpb.Checkpoint, len(blks))
	fCheckpoints := make([]*ethpb.Checkpoint, len(blks))
	sigSet := bls.NewSet()
	type versionAndHeader struct {
		version int
		header  interfaces.ExecutionData
	}
	preVersionAndHeaders := make([]*versionAndHeader, len(blks))
	postVersionAndHeaders := make([]*versionAndHeader, len(blks))
	var set *bls.SignatureBatch
	boundaries := make(map[[32]byte]state.BeaconState)
	for i, b := range blks {
		if features.BlacklistedBlock(b.Root()) {
			return errBlacklistedRoot
		}
		v, h, err := getStateVersionAndPayload(preState)
		if err != nil {
			return err
		}
		preVersionAndHeaders[i] = &versionAndHeader{
			version: v,
			header:  h,
		}

		set, preState, err = transition.ExecuteStateTransitionNoVerifyAnySig(ctx, preState, b)
		if err != nil {
			return invalidBlock{error: err}
		}
		// Save potential boundary states.
		if slots.IsEpochStart(preState.Slot()) {
			boundaries[b.Root()] = preState.Copy()
		}
		jCheckpoints[i] = preState.CurrentJustifiedCheckpoint()
		fCheckpoints[i] = preState.FinalizedCheckpoint()

		v, h, err = getStateVersionAndPayload(preState)
		if err != nil {
			return err
		}
		postVersionAndHeaders[i] = &versionAndHeader{
			version: v,
			header:  h,
		}
		sigSet.Join(set)
	}

	var verify bool
	if features.Get().EnableVerboseSigVerification {
		verify, err = sigSet.VerifyVerbosely()
	} else {
		verify, err = sigSet.Verify()
	}
	if err != nil {
		return invalidBlock{error: err}
	}
	if !verify {
		return errors.New("batch block signature verification failed")
	}

	// blocks have been verified, save them and call the engine
	pendingNodes := make([]*forkchoicetypes.BlockAndCheckpoints, len(blks))
	var isValidPayload bool
	for i, b := range blks {
		root := b.Root()
		isValidPayload, err = s.notifyNewPayload(ctx,
			postVersionAndHeaders[i].version,
			postVersionAndHeaders[i].header, b)
		if err != nil {
			return s.handleInvalidExecutionError(ctx, err, root, b.Block().ParentRoot())
		}
		if isValidPayload {
			if err := s.validateMergeTransitionBlock(ctx, preVersionAndHeaders[i].version,
				preVersionAndHeaders[i].header, b); err != nil {
				return err
			}
		}

		if err := s.areSidecarsAvailable(ctx, avs, b); err != nil {
			return errors.Wrapf(err, "could not validate sidecar availability for block %#x at slot %d", b.Root(), b.Block().Slot())
		}

		args := &forkchoicetypes.BlockAndCheckpoints{Block: b,
			JustifiedCheckpoint: jCheckpoints[i],
			FinalizedCheckpoint: fCheckpoints[i]}
		pendingNodes[i] = args
		if err := s.saveInitSyncBlock(ctx, root, b); err != nil {
			tracing.AnnotateError(span, err)
			return err
		}
		if err := s.cfg.BeaconDB.SaveStateSummary(ctx, &ethpb.StateSummary{
			Slot: b.Block().Slot(),
			Root: root[:],
		}); err != nil {
			tracing.AnnotateError(span, err)
			return err
		}
		if i > 0 && jCheckpoints[i].Epoch > jCheckpoints[i-1].Epoch {
			if err := s.cfg.BeaconDB.SaveJustifiedCheckpoint(ctx, jCheckpoints[i]); err != nil {
				tracing.AnnotateError(span, err)
				return err
			}
		}
		if i > 0 && fCheckpoints[i].Epoch > fCheckpoints[i-1].Epoch {
			if err := s.updateFinalized(ctx, fCheckpoints[i]); err != nil {
				tracing.AnnotateError(span, err)
				return err
			}
		}
	}
	// Save boundary states that will be useful for forkchoice
	for r, st := range boundaries {
		if err := s.cfg.StateGen.SaveState(ctx, r, st); err != nil {
			return err
		}
	}
	lastB := blks[len(blks)-1]
	lastBR := lastB.Root()
	// Also saves the last post state which to be used as pre state for the next batch.
	if err := s.cfg.StateGen.SaveState(ctx, lastBR, preState); err != nil {
		return err
	}
	// Insert all nodes to forkchoice
	if err := s.cfg.ForkChoiceStore.InsertChain(ctx, pendingNodes); err != nil {
		return errors.Wrap(err, "could not insert batch to forkchoice")
	}
	// Set their optimistic status
	if isValidPayload {
		if err := s.cfg.ForkChoiceStore.SetOptimisticToValid(ctx, lastBR); err != nil {
			return errors.Wrap(err, "could not set optimistic block to valid")
		}
	}
	arg := &fcuConfig{
		headState: preState,
		headRoot:  lastBR,
		headBlock: lastB,
	}
	if _, err := s.notifyForkchoiceUpdate(ctx, arg); err != nil {
		return err
	}
	return s.saveHeadNoDB(ctx, lastB, lastBR, preState, !isValidPayload)
}

func (s *Service) areSidecarsAvailable(ctx context.Context, avs das.AvailabilityStore, roBlock consensusblocks.ROBlock) error {
	blockVersion := roBlock.Version()
	block := roBlock.Block()
	slot := block.Slot()

	if blockVersion >= version.Fulu {
		if err := s.areDataColumnsAvailable(ctx, roBlock.Root(), block); err != nil {
			return errors.Wrapf(err, "are data columns available for block %#x with slot %d", roBlock.Root(), slot)
		}

		return nil
	}

	if blockVersion >= version.Deneb {
		if err := avs.IsDataAvailable(ctx, s.CurrentSlot(), roBlock); err != nil {
			return errors.Wrapf(err, "could not validate sidecar availability at slot %d", slot)
		}

		return nil
	}

	return nil
}

func (s *Service) updateEpochBoundaryCaches(ctx context.Context, st state.BeaconState) error {
	e := coreTime.CurrentEpoch(st)
	if err := helpers.UpdateCommitteeCache(ctx, st, e); err != nil {
		return errors.Wrap(err, "could not update committee cache")
	}
	if err := helpers.UpdateProposerIndicesInCache(ctx, st, e); err != nil {
		return errors.Wrap(err, "could not update proposer index cache")
	}
	go func(ep primitives.Epoch) {
		// Use a custom deadline here, since this method runs asynchronously.
		// We ignore the parent method's context and instead create a new one
		// with a custom deadline, therefore using the background context instead.
		slotCtx, cancel := context.WithTimeout(context.Background(), slotDeadline)
		defer cancel()
		if err := helpers.UpdateCommitteeCache(slotCtx, st, ep+1); err != nil {
			log.WithError(err).Warn("Could not update committee cache")
		}
	}(e)
	// The latest block header is from the previous epoch
	r, err := st.LatestBlockHeader().HashTreeRoot()
	if err != nil {
		log.WithError(err).Error("Could not update proposer index state-root map")
		return nil
	}
	// The proposer indices cache takes the target root for the previous
	// epoch as key
	if e > 0 {
		e = e - 1
	}
	target, err := s.cfg.ForkChoiceStore.TargetRootForEpoch(r, e)
	if err != nil {
		log.WithError(err).Error("Could not update proposer index state-root map")
		return nil
	}
	err = helpers.UpdateCachedCheckpointToStateRoot(st, &forkchoicetypes.Checkpoint{Epoch: e, Root: target})
	if err != nil {
		log.WithError(err).Error("Could not update proposer index state-root map")
	}
	return nil
}

// Epoch boundary tasks: it copies the headState and updates the epoch boundary
// caches.
func (s *Service) handleEpochBoundary(ctx context.Context, slot primitives.Slot, headState state.BeaconState, blockRoot []byte) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.handleEpochBoundary")
	defer span.End()
	// return early if we are advancing to a past epoch
	if slot < headState.Slot() {
		return nil
	}
	if !slots.IsEpochEnd(slot) {
		return nil
	}
	copied := headState.Copy()
	copied, err := transition.ProcessSlotsUsingNextSlotCache(ctx, copied, blockRoot, slot+1)
	if err != nil {
		return err
	}
	return s.updateEpochBoundaryCaches(ctx, copied)
}

// This feeds in the attestations included in the block to fork choice store. It's allows fork choice store
// to gain information on the most current chain.
func (s *Service) handleBlockAttestations(ctx context.Context, blk interfaces.ReadOnlyBeaconBlock, st state.BeaconState) error {
	// Feed in block's attestations to fork choice store.
	for _, a := range blk.Body().Attestations() {
		committees, err := helpers.AttestationCommitteesFromState(ctx, st, a)
		if err != nil {
			return err
		}
		indices, err := attestation.AttestingIndices(a, committees...)
		if err != nil {
			return err
		}
		r := bytesutil.ToBytes32(a.GetData().BeaconBlockRoot)
		if s.cfg.ForkChoiceStore.HasNode(r) {
			s.cfg.ForkChoiceStore.ProcessAttestation(ctx, indices, r, a.GetData().Target.Epoch)
		} else if features.Get().EnableExperimentalAttestationPool {
			if err = s.cfg.AttestationCache.Add(a); err != nil {
				return err
			}
		} else if err = s.cfg.AttPool.SaveBlockAttestation(a); err != nil {
			return err
		}
	}
	return nil
}

// InsertSlashingsToForkChoiceStore inserts attester slashing indices to fork choice store.
// To call this function, it's caller's responsibility to ensure the slashing object is valid.
// This function requires a write lock on forkchoice.
func (s *Service) InsertSlashingsToForkChoiceStore(ctx context.Context, slashings []ethpb.AttSlashing) {
	for _, slashing := range slashings {
		indices := blocks.SlashableAttesterIndices(slashing)
		for _, index := range indices {
			s.cfg.ForkChoiceStore.InsertSlashedIndex(ctx, primitives.ValidatorIndex(index))
		}
	}
}

// This saves post state info to DB or cache. This also saves post state info to fork choice store.
// Post state info consists of processed block and state. Do not call this method unless the block and state are verified.
func (s *Service) savePostStateInfo(ctx context.Context, r [32]byte, b interfaces.ReadOnlySignedBeaconBlock, st state.BeaconState) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.savePostStateInfo")
	defer span.End()
	if err := s.cfg.BeaconDB.SaveBlock(ctx, b); err != nil {
		return errors.Wrapf(err, "could not save block from slot %d", b.Block().Slot())
	}
	if err := s.cfg.StateGen.SaveState(ctx, r, st); err != nil {
		// Do not use parent context in the event it deadlined
		ctx = trace.NewContext(context.Background(), span)
		s.rollbackBlock(ctx, r)
		return errors.Wrap(err, "could not save state")
	}
	return nil
}

// pruneAttsFromPool removes these attestations from the attestation pool
// which are covered by attestations from the received block.
func (s *Service) pruneAttsFromPool(ctx context.Context, headState state.BeaconState, headBlock interfaces.ReadOnlySignedBeaconBlock) {
	for _, att := range headBlock.Block().Body().Attestations() {
		if err := s.pruneCoveredAttsFromPool(ctx, headState, att); err != nil {
			log.WithError(err).Warn("Could not prune attestations covered by a received block's attestation")
		}
	}
}

func (s *Service) pruneCoveredAttsFromPool(ctx context.Context, headState state.BeaconState, att ethpb.Att) error {
	switch {
	case !att.IsAggregated():
		return s.cfg.AttPool.DeleteUnaggregatedAttestation(att)
	case att.Version() == version.Phase0:
		if features.Get().EnableExperimentalAttestationPool {
			return errors.Wrap(s.cfg.AttestationCache.DeleteCovered(att), "could not delete covered attestation")
		}
		return errors.Wrap(s.cfg.AttPool.DeleteAggregatedAttestation(att), "could not delete aggregated attestation")
	default:
		return s.pruneCoveredElectraAttsFromPool(ctx, headState, att)
	}
}

// pruneCoveredElectraAttsFromPool handles removing aggregated Electra attestations from the pool after receiving a block.
// Because in Electra block attestations can combine aggregates for multiple committees, comparing attestation bits
// of a block attestation with attestations bits of an aggregate can cause unexpected results, leading to covered
// aggregates not being removed from the pool.
//
// To make sure aggregates are removed, we decompose the block attestation into dummy aggregates, with each
// aggregate accounting for one committee. This allows us to compare aggregates in the same way it's done for
// Phase0. Even though we can't provide a valid signature for the dummy aggregate, it does not matter because
// signatures play no part in pruning attestations.
func (s *Service) pruneCoveredElectraAttsFromPool(ctx context.Context, headState state.BeaconState, att ethpb.Att) error {
	if att.Version() == version.Phase0 {
		log.Error("Called pruneCoveredElectraAttsFromPool with a Phase0 attestation")
		return nil
	}

	// We don't want to recompute committees. If they are not cached already,
	// we allow attestations to stay in the pool. If these attestations are
	// included in a later block, they will be redundant. But given that
	// they were not cached in the first place, it's unlikely that they
	// will be chosen into a block.
	ok, committees, err := helpers.AttestationCommitteesFromCache(ctx, headState, att)
	if err != nil {
		return errors.Wrap(err, "could not get attestation committees")
	}
	if !ok {
		log.Debug("Attestation committees are not cached. Skipping attestation pruning.")
		return nil
	}

	committeeIndices := att.CommitteeBitsVal().BitIndices()
	offset := uint64(0)

	// Sanity check as this should never happen
	if len(committeeIndices) != len(committees) {
		return errors.New("committee indices and committees have different lengths")
	}

	for i, c := range committees {
		ab := bitfield.NewBitlist(uint64(len(c)))
		for j := uint64(0); j < uint64(len(c)); j++ {
			ab.SetBitAt(j, att.GetAggregationBits().BitAt(j+offset))
		}

		cb := primitives.NewAttestationCommitteeBits()
		cb.SetBitAt(uint64(committeeIndices[i]), true)

		a := &ethpb.AttestationElectra{
			AggregationBits: ab,
			Data:            att.GetData(),
			CommitteeBits:   cb,
			Signature:       make([]byte, fieldparams.BLSSignatureLength),
		}

		if features.Get().EnableExperimentalAttestationPool {
			if err = s.cfg.AttestationCache.DeleteCovered(a); err != nil {
				return errors.Wrap(err, "could not delete covered attestation")
			}
		} else if !a.IsAggregated() {
			if err = s.cfg.AttPool.DeleteUnaggregatedAttestation(a); err != nil {
				return errors.Wrap(err, "could not delete unaggregated attestation")
			}
		} else if err = s.cfg.AttPool.DeleteAggregatedAttestation(a); err != nil {
			return errors.Wrap(err, "could not delete aggregated attestation")
		}

		offset += uint64(len(c))
	}

	return nil
}

// validateMergeTransitionBlock validates the merge transition block.
func (s *Service) validateMergeTransitionBlock(ctx context.Context, stateVersion int, stateHeader interfaces.ExecutionData, blk interfaces.ReadOnlySignedBeaconBlock) error {
	// Skip validation if block is older than Bellatrix.
	if blocks.IsPreBellatrixVersion(blk.Block().Version()) {
		return nil
	}

	// Skip validation if block has an empty payload.
	payload, err := blk.Block().Body().Execution()
	if err != nil {
		return invalidBlock{error: err}
	}
	isEmpty, err := consensusblocks.IsEmptyExecutionData(payload)
	if err != nil {
		return err
	}
	if isEmpty {
		return nil
	}

	// Handle case where pre-state is Altair but block contains payload.
	// To reach here, the block must have contained a valid payload.
	if blocks.IsPreBellatrixVersion(stateVersion) {
		return s.validateMergeBlock(ctx, blk)
	}

	// Skip validation if the block is not a merge transition block.
	// To reach here. The payload must be non-empty. If the state header is empty then it's at transition.
	empty, err := consensusblocks.IsEmptyExecutionData(stateHeader)
	if err != nil {
		return err
	}
	if !empty {
		return nil
	}
	return s.validateMergeBlock(ctx, blk)
}

// This routine checks if there is a cached proposer payload ID available for the next slot proposer.
// If there is not, it will call forkchoice updated with the correct payload attribute then cache the payload ID.
func (s *Service) runLateBlockTasks() {
	if err := s.waitForSync(); err != nil {
		log.WithError(err).Error("Failed to wait for initial sync")
		return
	}

	attThreshold := params.BeaconConfig().SecondsPerSlot / 3
	ticker := slots.NewSlotTickerWithOffset(s.genesisTime, time.Duration(attThreshold)*time.Second, params.BeaconConfig().SecondsPerSlot)
	for {
		select {
		case <-ticker.C():
			s.lateBlockTasks(s.ctx)
		case <-s.ctx.Done():
			log.Debug("Context closed, exiting routine")
			return
		}
	}
}

// missingBlobIndices uses the expected commitments from the block to determine
// which BlobSidecar indices would need to be in the database for DA success.
// It returns a map where each key represents a missing BlobSidecar index.
// An empty map means we have all indices; a non-empty map can be used to compare incoming
// BlobSidecars against the set of known missing sidecars.
func missingBlobIndices(store *filesystem.BlobStorage, root [fieldparams.RootLength]byte, expected [][]byte, slot primitives.Slot) (map[uint64]bool, error) {
	maxBlobsPerBlock := params.BeaconConfig().MaxBlobsPerBlock(slot)
	if len(expected) == 0 {
		return nil, nil
	}
	if len(expected) > maxBlobsPerBlock {
		return nil, errMaxBlobsExceeded
	}
	indices := store.Summary(root)
	missing := make(map[uint64]bool, len(expected))
	for i := range expected {
		if len(expected[i]) > 0 && !indices.HasIndex(uint64(i)) {
			missing[uint64(i)] = true
		}
	}
	return missing, nil
}

// missingDataColumnIndices uses the expected data columns from the block to determine
// which DataColumnSidecar indices would need to be in the database for DA success.
// It returns a map where each key represents a missing DataColumnSidecar index.
// An empty map means we have all indices; a non-empty map can be used to compare incoming
// DataColumns against the set of known missing sidecars.
func missingDataColumnIndices(store *filesystem.DataColumnStorage, root [fieldparams.RootLength]byte, expected map[uint64]bool) (map[uint64]bool, error) {
	if len(expected) == 0 {
		return nil, nil
	}

	numberOfColumns := params.BeaconConfig().NumberOfColumns

	if uint64(len(expected)) > numberOfColumns {
		return nil, errMaxDataColumnsExceeded
	}

	// Get a summary of the data columns stored in the database.
	summary := store.Summary(root)

	// Check all expected data columns against the summary.
	missing := make(map[uint64]bool)
	for column := range expected {
		if !summary.HasIndex(column) {
			missing[column] = true
		}
	}

	return missing, nil
}

// isDataAvailable blocks until all sidecars committed to in the block are available,
// or an error or context cancellation occurs. A nil result means that the data availability check is successful.
// The function will first check the database to see if all sidecars have been persisted. If any
// sidecars are missing, it will then read from the sidecar notifier channel for the given root until the channel is
// closed, the context hits cancellation/timeout, or notifications have been received for all the missing sidecars.
func (s *Service) isDataAvailable(
	ctx context.Context,
	roBlock consensusblocks.ROBlock,
) error {
	block := roBlock.Block()
	if block == nil {
		return errors.New("invalid nil beacon block")
	}

	root := roBlock.Root()
	blockVersion := block.Version()
	if blockVersion >= version.Fulu {
		return s.areDataColumnsAvailable(ctx, root, block)
	}

	if blockVersion >= version.Deneb {
		return s.areBlobsAvailable(ctx, root, block)
	}

	return nil
}

// areDataColumnsAvailable blocks until all data columns committed to in the block are available,
// or an error or context cancellation occurs. A nil result means that the data availability check is successful.
func (s *Service) areDataColumnsAvailable(
	ctx context.Context,
	root [fieldparams.RootLength]byte,
	block interfaces.ReadOnlyBeaconBlock,
) error {
	// We are only required to check within MIN_EPOCHS_FOR_DATA_COLUMN_SIDECARS_REQUESTS
	blockSlot, currentSlot := block.Slot(), s.CurrentSlot()
	blockEpoch, currentEpoch := slots.ToEpoch(blockSlot), slots.ToEpoch(currentSlot)
	if !params.WithinDAPeriod(blockEpoch, currentEpoch) {
		return nil
	}

	body := block.Body()
	if body == nil {
		return errors.New("invalid nil beacon block body")
	}

	kzgCommitments, err := body.BlobKzgCommitments()
	if err != nil {
		return errors.Wrap(err, "blob KZG commitments")
	}

	// If block has not commitments there is nothing to wait for.
	if len(kzgCommitments) == 0 {
		return nil
	}

	// All columns to sample need to be available for the block to be considered available.
	nodeID := s.cfg.P2P.NodeID()

	// Get the custody group sampling size for the node.
	custodyGroupCount, err := s.cfg.P2P.CustodyGroupCount(ctx)
	if err != nil {
		return errors.Wrap(err, "custody group count")
	}

	// Compute the sampling size.
	// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/das-core.md#custody-sampling
	samplesPerSlot := params.BeaconConfig().SamplesPerSlot
	samplingSize := max(samplesPerSlot, custodyGroupCount)

	// Get the peer info for the node.
	peerInfo, _, err := peerdas.Info(nodeID, samplingSize)
	if err != nil {
		return errors.Wrap(err, "peer info")
	}

	// Subscribe to newly data columns stored in the database.
	subscription, identsChan := s.dataColumnStorage.Subscribe()
	defer subscription.Unsubscribe()

	// Get the count of data columns we already have in the store.
	summary := s.dataColumnStorage.Summary(root)
	storedDataColumnsCount := summary.Count()

	minimumColumnCountToReconstruct := peerdas.MinimumColumnCountToReconstruct()

	// As soon as we have enough data column sidecars, we can reconstruct the missing ones.
	// We don't need to wait for the rest of the data columns to declare the block as available.
	if storedDataColumnsCount >= minimumColumnCountToReconstruct {
		return nil
	}

	// Get a map of data column indices that are not currently available.
	missing, err := missingDataColumnIndices(s.dataColumnStorage, root, peerInfo.CustodyColumns)
	if err != nil {
		return errors.Wrap(err, "missing data columns")
	}

	// If there are no missing indices, all data column sidecars are available.
	// This is the happy path.
	if len(missing) == 0 {
		return nil
	}

	if s.startWaitingDataColumnSidecars != nil {
		s.startWaitingDataColumnSidecars <- true
	}

	// Log for DA checks that cross over into the next slot; helpful for debugging.
	nextSlot, err := slots.StartTime(s.genesisTime, block.Slot()+1)
	if err != nil {
		return fmt.Errorf("unable to determine slot start time: %w", err)
	}

	// Avoid logging if DA check is called after next slot start.
	if nextSlot.After(time.Now()) {
		timer := time.AfterFunc(time.Until(nextSlot), func() {
			missingCount := uint64(len(missing))

			if missingCount == 0 {
				return
			}

			log.WithFields(logrus.Fields{
				"slot":            block.Slot(),
				"root":            fmt.Sprintf("%#x", root),
				"columnsExpected": helpers.SortedPrettySliceFromMap(peerInfo.CustodyColumns),
				"columnsWaiting":  helpers.SortedPrettySliceFromMap(missing),
			}).Warning("Data columns still missing at slot end")
		})
		defer timer.Stop()
	}

	for {
		select {
		case idents := <-identsChan:
			if idents.Root != root {
				// This is not the root we are looking for.
				continue
			}

			for _, index := range idents.Indices {
				// This is a data column we are expecting.
				if _, ok := missing[index]; ok {
					storedDataColumnsCount++
				}

				// As soon as we have more than half of the data columns, we can reconstruct the missing ones.
				// We don't need to wait for the rest of the data columns to declare the block as available.
				if storedDataColumnsCount >= minimumColumnCountToReconstruct {
					return nil
				}

				// Remove the index from the missing map.
				delete(missing, index)

				// Return if there is no more missing data columns.
				if len(missing) == 0 {
					return nil
				}
			}

		case <-ctx.Done():
			var missingIndices interface{} = "all"
			numberOfColumns := params.BeaconConfig().NumberOfColumns
			missingIndicesCount := uint64(len(missing))

			if missingIndicesCount < numberOfColumns {
				missingIndices = helpers.SortedPrettySliceFromMap(missing)
			}

			return errors.Wrapf(ctx.Err(), "data column sidecars slot: %d, BlockRoot: %#x, missing: %v", block.Slot(), root, missingIndices)
		}
	}
}

// areBlobsAvailable blocks until all BlobSidecars committed to in the block are available,
// or an error or context cancellation occurs. A nil result means that the data availability check is successful.
func (s *Service) areBlobsAvailable(ctx context.Context, root [fieldparams.RootLength]byte, block interfaces.ReadOnlyBeaconBlock) error {
	blockSlot := block.Slot()

	// We are only required to check within MIN_EPOCHS_FOR_BLOB_SIDECARS_REQUESTS
	if !params.WithinDAPeriod(slots.ToEpoch(block.Slot()), slots.ToEpoch(s.CurrentSlot())) {
		return nil
	}

	body := block.Body()
	if body == nil {
		return errors.New("invalid nil beacon block body")
	}
	kzgCommitments, err := body.BlobKzgCommitments()
	if err != nil {
		return errors.Wrap(err, "could not get KZG commitments")
	}
	// expected is the number of kzg commitments observed in the block.
	expected := len(kzgCommitments)
	if expected == 0 {
		return nil
	}
	// get a map of BlobSidecar indices that are not currently available.
	missing, err := missingBlobIndices(s.blobStorage, root, kzgCommitments, block.Slot())
	if err != nil {
		return errors.Wrap(err, "missing indices")
	}
	// If there are no missing indices, all BlobSidecars are available.
	if len(missing) == 0 {
		return nil
	}

	// The gossip handler for blobs writes the index of each verified blob referencing the given
	// root to the channel returned by blobNotifiers.forRoot.
	nc := s.blobNotifiers.forRoot(root, block.Slot())

	// Log for DA checks that cross over into the next slot; helpful for debugging.
	nextSlot, err := slots.StartTime(s.genesisTime, block.Slot()+1)
	if err != nil {
		return fmt.Errorf("unable to determine slot start time: %w", err)
	}
	// Avoid logging if DA check is called after next slot start.
	if nextSlot.After(time.Now()) {
		nst := time.AfterFunc(time.Until(nextSlot), func() {
			if len(missing) == 0 {
				return
			}

			log.WithFields(logrus.Fields{
				"slot":          blockSlot,
				"root":          fmt.Sprintf("%#x", root),
				"blobsExpected": expected,
				"blobsWaiting":  len(missing),
			}).Error("Still waiting for blobs DA check at slot end.")
		})
		defer nst.Stop()
	}
	for {
		select {
		case idx := <-nc:
			// Delete each index seen in the notification channel.
			delete(missing, idx)
			// Read from the channel until there are no more missing sidecars.
			if len(missing) > 0 {
				continue
			}
			// Once all sidecars have been observed, clean up the notification channel.
			s.blobNotifiers.delete(root)
			return nil
		case <-ctx.Done():
			return errors.Wrapf(ctx.Err(), "context deadline waiting for blob sidecars slot: %d, BlockRoot: %#x", block.Slot(), root)
		}
	}
}

// lateBlockTasks  is called 4 seconds into the slot and performs tasks
// related to late blocks. It emits a MissedSlot state feed event.
// It calls FCU and sets the right attributes if we are proposing next slot
// it also updates the next slot cache and the proposer index cache to deal with skipped slots.
func (s *Service) lateBlockTasks(ctx context.Context) {
	currentSlot := s.CurrentSlot()
	if currentSlot == s.HeadSlot() {
		return
	}
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	// return early if we are in init sync
	if !s.inRegularSync() {
		return
	}
	s.headLock.RLock()
	headRoot := s.headRoot()
	headState := s.headState(ctx)
	s.headLock.RUnlock()
	lastRoot, lastState := transition.LastCachedState()
	if lastState == nil {
		lastRoot, lastState = headRoot[:], headState
	}
	// Copy all the field tries in our cached state in the event of late
	// blocks.
	lastState.CopyAllTries()
	if err := transition.UpdateNextSlotCache(ctx, lastRoot, lastState); err != nil {
		log.WithError(err).Debug("Could not update next slot state cache")
	}
	if err := s.handleEpochBoundary(ctx, currentSlot, headState, headRoot[:]); err != nil {
		log.WithError(err).Error("Could not update epoch boundary caches")
	}
	// return early if we already started building a block for the current
	// head root
	_, has := s.cfg.PayloadIDCache.PayloadID(s.CurrentSlot()+1, headRoot)
	if has {
		return
	}

	attribute := s.getPayloadAttribute(ctx, headState, s.CurrentSlot()+1, headRoot[:])
	// return early if we are not proposing next slot
	if attribute.IsEmpty() {
		headBlock, err := s.headBlock()
		if err != nil {
			log.WithError(err).WithField("head_root", headRoot).Error("Unable to retrieve head block to fire payload attributes event")
		}
		// notifyForkchoiceUpdate fires the payload attribute event. But in this case, we won't
		// call notifyForkchoiceUpdate, so the event is fired here.
		go s.firePayloadAttributesEvent(s.cfg.StateNotifier.StateFeed(), headBlock, headRoot, s.CurrentSlot()+1)
		return
	}

	s.headLock.RLock()
	headBlock, err := s.headBlock()
	if err != nil {
		s.headLock.RUnlock()
		log.WithError(err).Debug("could not perform late block tasks: failed to retrieve head block")
		return
	}
	s.headLock.RUnlock()

	fcuArgs := &fcuConfig{
		headState:  headState,
		headRoot:   headRoot,
		headBlock:  headBlock,
		attributes: attribute,
	}
	_, err = s.notifyForkchoiceUpdate(ctx, fcuArgs)
	if err != nil {
		log.WithError(err).Debug("could not perform late block tasks: failed to update forkchoice with engine")
	}
}

// waitForSync blocks until the node is synced to the head.
func (s *Service) waitForSync() error {
	select {
	case <-s.syncComplete:
		return nil
	case <-s.ctx.Done():
		return errors.New("context closed, exiting goroutine")
	}
}

func (s *Service) handleInvalidExecutionError(ctx context.Context, err error, blockRoot, parentRoot [fieldparams.RootLength]byte) error {
	if IsInvalidBlock(err) && InvalidBlockLVH(err) != [32]byte{} {
		return s.pruneInvalidBlock(ctx, blockRoot, parentRoot, InvalidBlockLVH(err))
	}
	return err
}

// In the event of an issue processing a block we rollback changes done to the db and our caches
// to always ensure that the node's internal state is consistent.
func (s *Service) rollbackBlock(ctx context.Context, blockRoot [32]byte) {
	log.Warnf("Rolling back insertion of block with root %#x due to processing error", blockRoot)
	if err := s.cfg.StateGen.DeleteStateFromCaches(ctx, blockRoot); err != nil {
		log.WithError(err).Errorf("Could not delete state from caches with block root %#x", blockRoot)
	}
	if err := s.cfg.BeaconDB.DeleteBlock(ctx, blockRoot); err != nil {
		log.WithError(err).Errorf("Could not delete block with block root %#x", blockRoot)
	}
}
