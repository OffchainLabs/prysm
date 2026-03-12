package validator

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	builderapi "github.com/OffchainLabs/prysm/v7/api/client/builder"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/builder"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	blockfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/block"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/kv"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	emptypb "github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// eth1DataNotification is a latch to stop flooding logs with the same warning.
var eth1DataNotification bool

const (
	eth1dataTimeout           = 2 * time.Second
	defaultBuilderBoostFactor = primitives.Gwei(100)
)

func debugContextFields(ctx context.Context) logrus.Fields {
	fields := logrus.Fields{}
	if deadline, ok := ctx.Deadline(); ok {
		fields["deadline"] = deadline
		fields["timeUntilDeadline"] = time.Until(deadline)
	}
	if err := ctx.Err(); err != nil {
		fields["ctxErr"] = err
	}
	return fields
}

func genericBeaconBlockFields(block *ethpb.GenericBeaconBlock) logrus.Fields {
	fields := logrus.Fields{}
	switch {
	case block == nil:
		fields["responseType"] = "nil"
	case block.GetPhase0() != nil:
		fields["responseType"] = "phase0"
	case block.GetAltair() != nil:
		fields["responseType"] = "altair"
	case block.GetBellatrix() != nil:
		fields["responseType"] = "bellatrix"
	case block.GetBlindedBellatrix() != nil:
		fields["responseType"] = "blinded_bellatrix"
	case block.GetCapella() != nil:
		fields["responseType"] = "capella"
	case block.GetBlindedCapella() != nil:
		fields["responseType"] = "blinded_capella"
	case block.GetDeneb() != nil:
		fields["responseType"] = "deneb"
		fields["blobCount"] = len(block.GetDeneb().Blobs)
		fields["kzgProofCount"] = len(block.GetDeneb().KzgProofs)
	case block.GetBlindedDeneb() != nil:
		fields["responseType"] = "blinded_deneb"
	case block.GetElectra() != nil:
		fields["responseType"] = "electra"
		fields["blobCount"] = len(block.GetElectra().Blobs)
		fields["kzgProofCount"] = len(block.GetElectra().KzgProofs)
	case block.GetBlindedElectra() != nil:
		fields["responseType"] = "blinded_electra"
	case block.GetFulu() != nil:
		fields["responseType"] = "fulu"
		fields["blobCount"] = len(block.GetFulu().Blobs)
		fields["kzgProofCount"] = len(block.GetFulu().KzgProofs)
	case block.GetBlindedFulu() != nil:
		fields["responseType"] = "blinded_fulu"
	default:
		fields["responseType"] = "unknown"
	}
	return fields
}

func signedBlockFields(block interfaces.ReadOnlySignedBeaconBlock, root [fieldparams.RootLength]byte) logrus.Fields {
	fields := logrus.Fields{
		"slot":          block.Block().Slot(),
		"root":          fmt.Sprintf("%#x", root),
		"fork":          version.String(block.Version()),
		"blinded":       block.IsBlinded(),
		"proposerIndex": block.Block().ProposerIndex(),
	}
	body := block.Block().Body()
	if body == nil {
		return fields
	}
	if kzgs, err := body.BlobKzgCommitments(); err == nil {
		fields["kzgCommitmentCount"] = len(kzgs)
	}
	return fields
}

func genericSignedBlockFields(block *ethpb.GenericSignedBeaconBlock) logrus.Fields {
	fields := logrus.Fields{}
	switch {
	case block == nil:
		fields["requestType"] = "nil"
	case block.GetPhase0() != nil:
		fields["requestType"] = "phase0"
	case block.GetAltair() != nil:
		fields["requestType"] = "altair"
	case block.GetBellatrix() != nil:
		fields["requestType"] = "bellatrix"
	case block.GetBlindedBellatrix() != nil:
		fields["requestType"] = "blinded_bellatrix"
	case block.GetCapella() != nil:
		fields["requestType"] = "capella"
	case block.GetBlindedCapella() != nil:
		fields["requestType"] = "blinded_capella"
	case block.GetDeneb() != nil:
		fields["requestType"] = "deneb"
		fields["blobCount"] = len(block.GetDeneb().Blobs)
		fields["kzgProofCount"] = len(block.GetDeneb().KzgProofs)
	case block.GetBlindedDeneb() != nil:
		fields["requestType"] = "blinded_deneb"
	case block.GetElectra() != nil:
		fields["requestType"] = "electra"
		fields["blobCount"] = len(block.GetElectra().Blobs)
		fields["kzgProofCount"] = len(block.GetElectra().KzgProofs)
	case block.GetBlindedElectra() != nil:
		fields["requestType"] = "blinded_electra"
	case block.GetFulu() != nil:
		fields["requestType"] = "fulu"
		fields["blobCount"] = len(block.GetFulu().Blobs)
		fields["kzgProofCount"] = len(block.GetFulu().KzgProofs)
	case block.GetBlindedFulu() != nil:
		fields["requestType"] = "blinded_fulu"
	default:
		fields["requestType"] = "unknown"
	}
	return fields
}

func dataColumnSidecarFields(sidecars []blocks.RODataColumn, partialColumns []blocks.PartialDataColumn) logrus.Fields {
	fields := logrus.Fields{
		"dataColumnSidecarCount": len(sidecars),
		"partialColumnCount":     len(partialColumns),
	}
	if len(sidecars) > 0 {
		fields["slot"] = sidecars[0].Slot()
		fields["root"] = fmt.Sprintf("%#x", sidecars[0].BlockRoot())
		fields["proposerIndex"] = sidecars[0].ProposerIndex()
		return fields
	}
	if len(partialColumns) > 0 && partialColumns[0].SignedBlockHeader != nil && partialColumns[0].SignedBlockHeader.Header != nil {
		fields["slot"] = partialColumns[0].SignedBlockHeader.Header.Slot
		fields["proposerIndex"] = partialColumns[0].SignedBlockHeader.Header.ProposerIndex
	}
	return fields
}

// Deprecated: The gRPC API will remain the default and fully supported through v8 (expected in 2026) but will be eventually removed in favor of REST API.
//
// GetBeaconBlock is called by a proposer during its assigned slot to request a block to sign
// by passing in the slot and the signed randao reveal of the slot.
func (vs *Server) GetBeaconBlock(ctx context.Context, req *ethpb.BlockRequest) (*ethpb.GenericBeaconBlock, error) {
	ctx, span := trace.StartSpan(ctx, "ProposerServer.GetBeaconBlock")
	defer span.End()
	span.SetAttributes(trace.Int64Attribute("slot", int64(req.Slot)))
	buildStarted := time.Now()

	t, err := slots.StartTime(vs.TimeFetcher.GenesisTime(), req.Slot)
	if err != nil {
		log.WithError(err).Error("Could not convert slot to time")
	}

	log := log.WithField("slot", req.Slot)
	log.WithField("sinceSlotStartTime", time.Since(t)).Info("Begin building block")

	// A syncing validator should not produce a block.
	if vs.SyncChecker.Syncing() {
		log.Error("Fail to build block: node is syncing")
		return nil, status.Error(codes.Unavailable, "Syncing to latest head, not ready to respond")
	}
	// An optimistic validator MUST NOT produce a block (i.e., sign across the DOMAIN_BEACON_PROPOSER domain).
	if slots.ToEpoch(req.Slot) >= params.BeaconConfig().BellatrixForkEpoch {
		if err := vs.optimisticStatus(ctx); err != nil {
			log.WithError(err).Error("Fail to build block: node is optimistic")
			return nil, status.Errorf(codes.Unavailable, "Validator is not ready to propose: %v", err)
		}
	}

	head, parentRoot, err := vs.getParentState(ctx, req.Slot)
	if err != nil {
		log.WithError(err).Error("Fail to build block: could not get parent state")
		return nil, err
	}
	sBlk, err := getEmptyBlock(req.Slot)
	if err != nil {
		log.WithError(err).Error("Fail to build block: could not get empty block")
		return nil, status.Errorf(codes.Internal, "Could not prepare block: %v", err)
	}
	// Set slot, graffiti, randao reveal, and parent root.
	sBlk.SetSlot(req.Slot)
	// Generate graffiti with client version info using flexible standard
	if vs.GraffitiInfo != nil {
		graffiti := vs.GraffitiInfo.GenerateGraffiti(req.Graffiti)
		sBlk.SetGraffiti(graffiti[:])
	} else {
		sBlk.SetGraffiti(req.Graffiti)
	}
	sBlk.SetRandaoReveal(req.RandaoReveal)
	sBlk.SetParentRoot(parentRoot[:])

	// Set proposer index.
	idx, err := helpers.BeaconProposerIndex(ctx, head)
	if err != nil {
		return nil, fmt.Errorf("could not calculate proposer index %w", err)
	}
	sBlk.SetProposerIndex(idx)

	builderBoostFactor := defaultBuilderBoostFactor
	if req.BuilderBoostFactor != nil {
		builderBoostFactor = primitives.Gwei(req.BuilderBoostFactor.Value)
	}
	if log.Logger.IsLevelEnabled(logrus.DebugLevel) {
		fields := debugContextFields(ctx)
		fields["slot"] = req.Slot
		fields["headStateSlot"] = head.Slot()
		fields["parentRoot"] = fmt.Sprintf("%#x", parentRoot)
		fields["currentSlot"] = vs.TimeFetcher.CurrentSlot()
		fields["skipMevBoost"] = req.SkipMevBoost
		fields["builderBoostFactor"] = builderBoostFactor
		fields["buildElapsed"] = time.Since(buildStarted)
		if !t.IsZero() {
			fields["sinceSlotStartTime"] = time.Since(t)
		}
		log.WithFields(fields).Debug("Starting beacon block build with selected parent state")
	}

	resp, err := vs.BuildBlockParallel(ctx, sBlk, head, req.SkipMevBoost, builderBoostFactor)
	log = log.WithFields(logrus.Fields{
		"sinceSlotStartTime": time.Since(t),
		"validator":          sBlk.Block().ProposerIndex(),
	})

	if err != nil {
		log.WithError(err).Error("Finished building block")
		return nil, errors.Wrap(err, "could not build block in parallel")
	}
	if log.Logger.IsLevelEnabled(logrus.DebugLevel) {
		fields := debugContextFields(ctx)
		fields["slot"] = req.Slot
		fields["buildElapsed"] = time.Since(buildStarted)
		for key, value := range genericBeaconBlockFields(resp) {
			fields[key] = value
		}
		if !t.IsZero() {
			fields["sinceSlotStartTime"] = time.Since(t)
		}
		log.WithFields(fields).Debug("Built beacon block response for validator")
	}

	log.Info("Finished building block")
	return resp, nil
}

func (vs *Server) handleSuccesfulReorgAttempt(ctx context.Context, slot primitives.Slot, parentRoot, _ [32]byte) (state.BeaconState, error) {
	// Try to get the state from the NSC
	head := transition.NextSlotState(parentRoot[:], slot)
	if head != nil {
		return head, nil
	}
	// cache miss
	head, err := vs.StateGen.StateByRoot(ctx, parentRoot)
	if err != nil {
		return nil, status.Error(codes.Unavailable, "could not obtain head state")
	}
	return head, nil
}

func logFailedReorgAttempt(slot primitives.Slot, oldHeadRoot, headRoot [32]byte) {
	blockchain.LateBlockAttemptedReorgCount.Inc()
	log.WithFields(logrus.Fields{
		"slot":        slot,
		"oldHeadRoot": fmt.Sprintf("%#x", oldHeadRoot),
		"headRoot":    fmt.Sprintf("%#x", headRoot),
	}).Warn("Late block attempted reorg failed")
}

func (vs *Server) getHeadNoReorg(ctx context.Context, slot primitives.Slot, parentRoot [32]byte) (state.BeaconState, error) {
	// Try to get the state from the NSC
	head := transition.NextSlotState(parentRoot[:], slot)
	if head != nil {
		return head, nil
	}
	head, err := vs.HeadFetcher.HeadState(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get head state: %v", err)
	}
	return head, nil
}

func (vs *Server) getParentStateFromReorgData(ctx context.Context, slot primitives.Slot, oldHeadRoot, parentRoot, headRoot [32]byte) (head state.BeaconState, err error) {
	if parentRoot != headRoot {
		head, err = vs.handleSuccesfulReorgAttempt(ctx, slot, parentRoot, headRoot)
	} else {
		if oldHeadRoot != headRoot {
			logFailedReorgAttempt(slot, oldHeadRoot, headRoot)
		}
		head, err = vs.getHeadNoReorg(ctx, slot, parentRoot)
	}
	if err != nil {
		return nil, err
	}
	if head.Slot() >= slot {
		return head, nil
	}
	head, err = transition.ProcessSlotsUsingNextSlotCache(ctx, head, parentRoot[:], slot)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not process slots up to %d: %v", slot, err)
	}
	return head, nil
}

func (vs *Server) getParentState(ctx context.Context, slot primitives.Slot) (state.BeaconState, [32]byte, error) {
	// process attestations and update head in forkchoice
	oldHeadRoot := vs.ForkchoiceFetcher.CachedHeadRoot()
	vs.ForkchoiceFetcher.UpdateHead(ctx, vs.TimeFetcher.CurrentSlot())
	headRoot := vs.ForkchoiceFetcher.CachedHeadRoot()
	parentRoot := vs.ForkchoiceFetcher.GetProposerHead()
	head, err := vs.getParentStateFromReorgData(ctx, slot, oldHeadRoot, parentRoot, headRoot)
	if err == nil && log.Logger.IsLevelEnabled(logrus.DebugLevel) {
		fields := debugContextFields(ctx)
		fields["slot"] = slot
		fields["oldHeadRoot"] = fmt.Sprintf("%#x", oldHeadRoot)
		fields["headRoot"] = fmt.Sprintf("%#x", headRoot)
		fields["parentRoot"] = fmt.Sprintf("%#x", parentRoot)
		fields["headStateSlot"] = head.Slot()
		fields["currentSlot"] = vs.TimeFetcher.CurrentSlot()
		fields["reorgedForProposal"] = parentRoot != headRoot
		log.WithFields(fields).Debug("Selected parent state for proposal")
	}
	return head, parentRoot, err
}

func (vs *Server) BuildBlockParallel(ctx context.Context, sBlk interfaces.SignedBeaconBlock, head state.BeaconState, skipMevBoost bool, builderBoostFactor primitives.Gwei) (*ethpb.GenericBeaconBlock, error) {
	// Build consensus fields in background
	var wg sync.WaitGroup
	wg.Go(func() {

		// Set eth1 data.
		eth1Data, err := vs.eth1DataMajorityVote(ctx, head)
		if err != nil {
			eth1Data = &ethpb.Eth1Data{DepositRoot: params.BeaconConfig().ZeroHash[:], BlockHash: params.BeaconConfig().ZeroHash[:]}
			log.WithError(err).Error("Could not get eth1data")
		}
		sBlk.SetEth1Data(eth1Data)

		// Set deposit and attestation.
		deposits, atts, err := vs.packDepositsAndAttestations(ctx, head, sBlk.Block().Slot(), eth1Data) // TODO: split attestations and deposits
		if err != nil {
			sBlk.SetDeposits([]*ethpb.Deposit{})
			if err := sBlk.SetAttestations([]ethpb.Att{}); err != nil {
				log.WithError(err).Error("Could not set attestations on block")
			}
			log.WithError(err).Error("Could not pack deposits and attestations")
		} else {
			sBlk.SetDeposits(deposits)
			if err := sBlk.SetAttestations(atts); err != nil {
				log.WithError(err).Error("Could not set attestations on block")
			}
		}

		// Set slashings.
		validProposerSlashings, validAttSlashings := vs.getSlashings(ctx, head)
		sBlk.SetProposerSlashings(validProposerSlashings)
		if err := sBlk.SetAttesterSlashings(validAttSlashings); err != nil {
			log.WithError(err).Error("Could not set attester slashings on block")
		}

		// Set exits.
		sBlk.SetVoluntaryExits(vs.getExits(head, sBlk.Block().Slot()))

		// Set sync aggregate. New in Altair.
		vs.setSyncAggregate(ctx, sBlk, head)

		// Set bls to execution change. New in Capella.
		vs.setBlsToExecData(sBlk, head)

		// Set payload attestations. New in Gloas.
		if sBlk.Version() >= version.Gloas {
			if err := sBlk.SetPayloadAttestations(vs.getPayloadAttestations(ctx, head)); err != nil {
				log.WithError(err).Error("Could not set payload attestations")
			}
		}
	})

	winningBid := primitives.ZeroWei()
	var bundle enginev1.BlobsBundler
	var local *blocks.GetPayloadResponse
	if sBlk.Version() >= version.Bellatrix {
		var err error
		local, err = vs.getLocalPayload(ctx, sBlk.Block(), head)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not get local payload: %v", err)
		}

		if sBlk.Version() < version.Gloas {
			// There's no reason to try to get a builder bid if local override is true.
			var builderBid builderapi.Bid
			if !(local.OverrideBuilder || skipMevBoost) {
				latestHeader, err := head.LatestExecutionPayloadHeader()
				if err != nil {
					return nil, status.Errorf(codes.Internal, "Could not get latest execution payload header: %v", err)
				}
				parentGasLimit := latestHeader.GasLimit()
				builderBid, err = vs.getBuilderPayloadAndBlobs(ctx, sBlk.Block().Slot(), sBlk.Block().ProposerIndex(), parentGasLimit)
				if err != nil {
					builderGetPayloadMissCount.Inc()
					log.WithError(err).Error("Could not get builder payload")
				}
			}

			winningBid, bundle, err = setExecutionData(ctx, sBlk, local, builderBid, builderBoostFactor)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "Could not set execution data: %v", err)
			}
		} else {
			if err := vs.setSelfBuildExecutionPayloadBid(ctx, sBlk, local); err != nil {
				return nil, status.Errorf(codes.Internal, "Could not set execution data for Gloas: %v", err)
			}
		}
	}

	wg.Wait()

	sr, err := vs.computeStateRoot(ctx, sBlk)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not compute state root: %v", err)
	}
	sBlk.SetStateRoot(sr)

	// For Gloas, build and cache the execution payload envelope now that the block
	// is fully built (state root set). The envelope needs the final block HTR as
	// BeaconBlockRoot and the post-payload state root as StateRoot.
	if sBlk.Version() >= version.Gloas {
		if err := vs.storeExecutionPayloadEnvelope(sBlk, local); err != nil {
			return nil, status.Errorf(codes.Internal, "Could not build execution payload envelope: %v", err)
		}
	}

	return vs.constructGenericBeaconBlock(sBlk, bundle, winningBid)
}

// Deprecated: The gRPC API will remain the default and fully supported through v8 (expected in 2026) but will be eventually removed in favor of REST API.
//
// ProposeBeaconBlock handles the proposal of beacon blocks.
func (vs *Server) ProposeBeaconBlock(ctx context.Context, req *ethpb.GenericSignedBeaconBlock) (*ethpb.ProposeResponse, error) {
	var (
		blobSidecars       []*ethpb.BlobSidecar
		dataColumnSidecars []blocks.RODataColumn
	)

	ctx, span := trace.StartSpan(ctx, "ProposerServer.ProposeBeaconBlock")
	defer span.End()
	proposalStarted := time.Now()

	if req == nil {
		return nil, status.Errorf(codes.InvalidArgument, "empty request")
	}

	block, err := blocks.NewSignedBeaconBlock(req.Block)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s: %v", "decode block failed", err)
	}
	root, err := block.Block().HashTreeRoot()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not hash tree root: %v", err)
	}
	reqLog := log.WithFields(signedBlockFields(block, root)).WithFields(debugContextFields(ctx)).WithFields(genericSignedBlockFields(req))
	if reqLog.Logger.IsLevelEnabled(logrus.DebugLevel) {
		reqLog.WithField("proposalElapsed", time.Since(proposalStarted)).Debug("Received signed block proposal from validator")
	}

	// For post-Fulu blinded blocks, submit to relay and return early
	if block.IsBlinded() && slots.ToEpoch(block.Block().Slot()) >= params.BeaconConfig().FuluForkEpoch {
		submitStart := time.Now()
		err := vs.BlockBuilder.SubmitBlindedBlockPostFulu(ctx, block)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not submit blinded block post-Fulu: %v", err)
		}
		if reqLog.Logger.IsLevelEnabled(logrus.DebugLevel) {
			reqLog.WithFields(logrus.Fields{
				"submitBlindedBlockElapsed": time.Since(submitStart),
				"proposalElapsed":           time.Since(proposalStarted),
			}).Debug("Submitted blinded post-Fulu block to relay")
		}
		return &ethpb.ProposeResponse{BlockRoot: root[:]}, nil
	}

	rob, err := blocks.NewROBlockWithRoot(block, root)
	var partialColumns []blocks.PartialDataColumn
	handleBlockStart := time.Now()
	if block.IsBlinded() {
		block, blobSidecars, err = vs.handleBlindedBlock(ctx, block)
		if errors.Is(err, builderapi.ErrBadGateway) {
			log.WithError(err).Info("Optimistically proposed block - builder relay temporarily unavailable, block may arrive over P2P")
			return &ethpb.ProposeResponse{BlockRoot: root[:]}, nil
		}
	} else if block.Version() >= version.Deneb && block.Version() < version.Gloas {
		blobSidecars, dataColumnSidecars, partialColumns, err = vs.handleUnblindedBlock(rob, req)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%s: %v", "handle block failed", err)
	}
	if reqLog.Logger.IsLevelEnabled(logrus.DebugLevel) {
		reqLog.WithFields(logrus.Fields{
			"handleBlockElapsed":     time.Since(handleBlockStart),
			"blobSidecarCount":       len(blobSidecars),
			"dataColumnSidecarCount": len(dataColumnSidecars),
			"partialColumnCount":     len(partialColumns),
			"proposalElapsed":        time.Since(proposalStarted),
		}).Debug("Prepared block sidecars for proposal processing")
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	wg.Add(1)
	go func() {
		if err := vs.broadcastReceiveBlock(ctx, &wg, block, root); err != nil {
			errChan <- errors.Wrap(err, "broadcast/receive block failed")
			return
		}
		errChan <- nil
	}()

	waitForBroadcastStart := time.Now()
	wg.Wait()
	if reqLog.Logger.IsLevelEnabled(logrus.DebugLevel) {
		reqLog.WithFields(logrus.Fields{
			"waitForBroadcastElapsed": time.Since(waitForBroadcastStart),
			"proposalElapsed":         time.Since(proposalStarted),
		}).Debug("Finished waiting for block gossip broadcast before sidecar handling")
	}

	if block.Version() < version.Gloas {
		sidecarStart := time.Now()
		if err := vs.broadcastAndReceiveSidecars(ctx, block, root, blobSidecars, dataColumnSidecars, partialColumns); err != nil {
			return nil, status.Errorf(codes.Internal, "Could not broadcast/receive sidecars: %v", err)
		}
		if reqLog.Logger.IsLevelEnabled(logrus.DebugLevel) {
			reqLog.WithFields(logrus.Fields{
				"broadcastAndReceiveSidecarsElapsed": time.Since(sidecarStart),
				"proposalElapsed":                    time.Since(proposalStarted),
			}).Debug("Finished sidecar broadcast and local receive")
		}
	}
	if err := <-errChan; err != nil {
		return nil, status.Errorf(codes.Internal, "Could not broadcast/receive block: %v", err)
	}
	if reqLog.Logger.IsLevelEnabled(logrus.DebugLevel) {
		reqLog.WithField("proposalElapsed", time.Since(proposalStarted)).Debug("Finished proposer submission path")
	}

	return &ethpb.ProposeResponse{BlockRoot: root[:]}, nil
}

// broadcastAndReceiveSidecars broadcasts and receives sidecars.
func (vs *Server) broadcastAndReceiveSidecars(
	ctx context.Context,
	block interfaces.SignedBeaconBlock,
	root [fieldparams.RootLength]byte,
	blobSidecars []*ethpb.BlobSidecar,
	dataColumnSidecars []blocks.RODataColumn,
	partialColumns []blocks.PartialDataColumn,
) error {
	if block.Version() >= version.Fulu {
		if err := vs.broadcastAndReceiveDataColumns(ctx, dataColumnSidecars, partialColumns); err != nil {
			return errors.Wrap(err, "broadcast and receive data columns")
		}
		return nil
	}

	if err := vs.broadcastAndReceiveBlobs(ctx, blobSidecars, root); err != nil {
		return errors.Wrap(err, "broadcast and receive blobs")
	}

	return nil
}

// handleBlindedBlock processes blinded beacon blocks (pre-Fulu only).
// Post-Fulu blinded blocks are handled directly in ProposeBeaconBlock.
func (vs *Server) handleBlindedBlock(ctx context.Context, block interfaces.SignedBeaconBlock) (interfaces.SignedBeaconBlock, []*ethpb.BlobSidecar, error) {
	if block.Version() < version.Bellatrix {
		return nil, nil, errors.New("pre-Bellatrix blinded block")
	}

	if vs.BlockBuilder == nil || !vs.BlockBuilder.Configured() {
		return nil, nil, errors.New("unconfigured block builder")
	}

	copiedBlock, err := block.Copy()
	if err != nil {
		return nil, nil, err
	}

	payload, bundle, err := vs.BlockBuilder.SubmitBlindedBlock(ctx, block)
	if err != nil {
		return nil, nil, errors.Wrap(err, "submit blinded block failed")
	}

	if err := copiedBlock.Unblind(payload); err != nil {
		return nil, nil, errors.Wrap(err, "unblind failed")
	}

	sidecars, err := unblindBlobsSidecars(copiedBlock, bundle)
	if err != nil {
		return nil, nil, errors.Wrap(err, "unblind blobs sidecars: commitment value doesn't match block")
	}

	return copiedBlock, sidecars, nil
}

func (vs *Server) handleUnblindedBlock(
	block blocks.ROBlock,
	req *ethpb.GenericSignedBeaconBlock,
) ([]*ethpb.BlobSidecar, []blocks.RODataColumn, []blocks.PartialDataColumn, error) {
	handleLog := log.WithFields(logrus.Fields{
		"slot": block.Block().Slot(),
		"root": fmt.Sprintf("%#x", block.Root()),
		"fork": version.String(block.Version()),
	})
	handleStarted := time.Now()
	rawBlobs, proofs, err := blobsAndProofs(req)
	if err != nil {
		return nil, nil, nil, err
	}
	if handleLog.Logger.IsLevelEnabled(logrus.DebugLevel) {
		handleLog.WithFields(logrus.Fields{
			"blobCount":          len(rawBlobs),
			"kzgProofCount":      len(proofs),
			"handleBlockElapsed": time.Since(handleStarted),
		}).Debug("Decoded blobs and proofs from proposed block")
	}

	if block.Version() >= version.Fulu {
		// Compute cells and proofs from the blobs and cell proofs.
		computeCellsStart := time.Now()
		cellsPerBlob, proofsPerBlob, err := peerdas.ComputeCellsAndProofsFromFlat(rawBlobs, proofs)
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "compute cells and proofs")
		}
		if handleLog.Logger.IsLevelEnabled(logrus.DebugLevel) {
			handleLog.WithFields(logrus.Fields{
				"blobCount":                    len(rawBlobs),
				"cellsPerBlobCount":            len(cellsPerBlob),
				"proofGroupsCount":             len(proofsPerBlob),
				"computeCellsAndProofsElapsed": time.Since(computeCellsStart),
			}).Debug("Computed cells and proofs from flat blob data")
		}

		// Construct data column sidecars from the signed block and cells and proofs.
		dataColumnsStart := time.Now()
		roDataColumnSidecars, err := peerdas.DataColumnSidecars(cellsPerBlob, proofsPerBlob, peerdas.PopulateFromBlock(block))
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "data column sidcars")
		}
		if handleLog.Logger.IsLevelEnabled(logrus.DebugLevel) {
			handleLog.WithFields(logrus.Fields{
				"dataColumnSidecarCount": len(roDataColumnSidecars),
				"dataColumnBuildElapsed": time.Since(dataColumnsStart),
			}).Debug("Constructed full data column sidecars from proposed block")
		}

		if len(cellsPerBlob) == 0 {
			return nil, roDataColumnSidecars, nil, nil
		}

		included := bitfield.NewBitlist(uint64(len(cellsPerBlob)))
		included = included.Not() // all bits set to 1
		partialColumnsStart := time.Now()
		partialColumns, err := peerdas.PartialColumns(included, cellsPerBlob, proofsPerBlob, peerdas.PopulateFromBlock(block))
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "data column sidcars")
		}
		if handleLog.Logger.IsLevelEnabled(logrus.DebugLevel) {
			handleLog.WithFields(logrus.Fields{
				"partialColumnCount":        len(partialColumns),
				"partialColumnBuildElapsed": time.Since(partialColumnsStart),
				"handleBlockElapsed":        time.Since(handleStarted),
			}).Debug("Constructed partial data columns from proposed block")
		}

		return nil, roDataColumnSidecars, partialColumns, nil
	}

	blobSidecarsStart := time.Now()
	blobSidecars, err := BuildBlobSidecars(block, rawBlobs, proofs)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "build blob sidecars")
	}
	if handleLog.Logger.IsLevelEnabled(logrus.DebugLevel) {
		handleLog.WithFields(logrus.Fields{
			"blobSidecarCount":        len(blobSidecars),
			"blobSidecarBuildElapsed": time.Since(blobSidecarsStart),
			"handleBlockElapsed":      time.Since(handleStarted),
		}).Debug("Constructed blob sidecars from proposed block")
	}

	return blobSidecars, nil, nil, nil
}

// broadcastReceiveBlock broadcasts a block and handles its reception.
func (vs *Server) broadcastReceiveBlock(ctx context.Context, wg *sync.WaitGroup, block interfaces.SignedBeaconBlock, root [fieldparams.RootLength]byte) error {
	stageLog := log.WithFields(signedBlockFields(block, root)).WithFields(debugContextFields(ctx))
	stageStarted := time.Now()
	if stageLog.Logger.IsLevelEnabled(logrus.DebugLevel) {
		stageLog.Debug("Starting block gossip broadcast and local self-import")
	}
	broadcastStarted := time.Now()
	if err := vs.broadcastBlock(ctx, wg, block, root); err != nil {
		return errors.Wrap(err, "broadcast block")
	}
	if stageLog.Logger.IsLevelEnabled(logrus.DebugLevel) {
		stageLog.WithFields(logrus.Fields{
			"broadcastElapsed": time.Since(broadcastStarted),
			"totalElapsed":     time.Since(stageStarted),
		}).Debug("Finished block gossip broadcast")
	}

	vs.BlockNotifier.BlockFeed().Send(&feed.Event{
		Type: blockfeed.ReceivedBlock,
		Data: &blockfeed.ReceivedBlockData{SignedBlock: block},
	})

	receiveStarted := time.Now()
	if err := vs.BlockReceiver.ReceiveBlock(ctx, block, root, nil); err != nil {
		return errors.Wrap(err, "receive block")
	}
	if stageLog.Logger.IsLevelEnabled(logrus.DebugLevel) {
		stageLog.WithFields(logrus.Fields{
			"receiveBlockElapsed": time.Since(receiveStarted),
			"totalElapsed":        time.Since(stageStarted),
		}).Debug("Finished local block self-import")
	}

	return nil
}

func (vs *Server) broadcastBlock(ctx context.Context, wg *sync.WaitGroup, block interfaces.SignedBeaconBlock, root [fieldparams.RootLength]byte) error {
	defer wg.Done()

	protoBlock, err := block.Proto()
	if err != nil {
		return errors.Wrap(err, "protobuf conversion failed")
	}
	if err := vs.P2P.Broadcast(ctx, protoBlock); err != nil {
		return errors.Wrap(err, "broadcast failed")
	}

	log.WithFields(logrus.Fields{
		"slot": block.Block().Slot(),
		"root": fmt.Sprintf("%#x", root),
	}).Debug("Broadcasted block")

	return nil
}

// broadcastAndReceiveBlobs handles the broadcasting and reception of blob sidecars.
func (vs *Server) broadcastAndReceiveBlobs(ctx context.Context, sidecars []*ethpb.BlobSidecar, root [fieldparams.RootLength]byte) error {
	start := time.Now()
	blobLog := log.WithFields(logrus.Fields{
		"root":             fmt.Sprintf("%#x", root),
		"blobSidecarCount": len(sidecars),
	}).WithFields(debugContextFields(ctx))
	if blobLog.Logger.IsLevelEnabled(logrus.DebugLevel) {
		blobLog.Debug("Starting blob sidecar broadcast and local receive")
	}
	eg, eCtx := errgroup.WithContext(ctx)
	for subIdx, sc := range sidecars {
		eg.Go(func() error {
			if err := vs.P2P.BroadcastBlob(eCtx, uint64(subIdx), sc); err != nil {
				return errors.Wrap(err, "broadcast blob failed")
			}
			readOnlySc, err := blocks.NewROBlobWithRoot(sc, root)
			if err != nil {
				return errors.Wrap(err, "ROBlob creation failed")
			}
			verifiedBlob := blocks.NewVerifiedROBlob(readOnlySc)
			if err := vs.BlobReceiver.ReceiveBlob(ctx, verifiedBlob); err != nil {
				return errors.Wrap(err, "receive blob failed")
			}
			vs.OperationNotifier.OperationFeed().Send(&feed.Event{
				Type: operation.BlobSidecarReceived,
				Data: &operation.BlobSidecarReceivedData{Blob: &verifiedBlob},
			})
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	if blobLog.Logger.IsLevelEnabled(logrus.DebugLevel) {
		blobLog.WithField("elapsed", time.Since(start)).Debug("Finished blob sidecar broadcast and local receive")
	}
	return nil
}

// broadcastAndReceiveDataColumns handles the broadcasting and reception of data columns sidecars.
func (vs *Server) broadcastAndReceiveDataColumns(ctx context.Context, roSidecars []blocks.RODataColumn, partialColumns []blocks.PartialDataColumn) error {
	dataColumnLog := log.WithFields(dataColumnSidecarFields(roSidecars, partialColumns)).WithFields(debugContextFields(ctx))
	totalStart := time.Now()
	if dataColumnLog.Logger.IsLevelEnabled(logrus.DebugLevel) {
		dataColumnLog.Debug("Starting data column broadcast and local receive")
	}

	// We built this block ourselves, so we can upgrade the read only data column sidecar into a verified one.
	verifiedSidecars := make([]blocks.VerifiedRODataColumn, 0, len(roSidecars))
	for _, sidecar := range roSidecars {
		verifiedSidecar := blocks.NewVerifiedRODataColumn(sidecar)
		verifiedSidecars = append(verifiedSidecars, verifiedSidecar)
	}

	// Broadcast sidecars (non blocking).
	broadcastStart := time.Now()
	if err := vs.P2P.BroadcastDataColumnSidecars(ctx, verifiedSidecars, partialColumns); err != nil {
		return errors.Wrap(err, "broadcast data column sidecars")
	}
	if dataColumnLog.Logger.IsLevelEnabled(logrus.DebugLevel) {
		dataColumnLog.WithFields(logrus.Fields{
			"queueBroadcastElapsed": time.Since(broadcastStart),
			"totalElapsed":          time.Since(totalStart),
		}).Debug("Queued data column sidecars for gossip broadcast")
	}

	// In parallel, receive sidecars.
	receiveStart := time.Now()
	if err := vs.DataColumnReceiver.ReceiveDataColumns(verifiedSidecars); err != nil {
		return errors.Wrap(err, "receive data columns")
	}
	if dataColumnLog.Logger.IsLevelEnabled(logrus.DebugLevel) {
		dataColumnLog.WithFields(logrus.Fields{
			"localReceiveElapsed": time.Since(receiveStart),
			"totalElapsed":        time.Since(totalStart),
		}).Debug("Saved data column sidecars locally")
	}

	return nil
}

// Deprecated: The gRPC API will remain the default and fully supported through v8 (expected in 2026) but will be eventually removed in favor of REST API.
//
// PrepareBeaconProposer caches and updates the fee recipient for the given proposer.
func (vs *Server) PrepareBeaconProposer(
	_ context.Context, request *ethpb.PrepareBeaconProposerRequest,
) (*emptypb.Empty, error) {
	var validatorIndices []primitives.ValidatorIndex

	for _, r := range request.Recipients {
		recipient := hexutil.Encode(r.FeeRecipient)
		if !common.IsHexAddress(recipient) {
			return nil, status.Errorf(codes.InvalidArgument, "Invalid fee recipient address: %v", recipient)
		}
		// Use default address if the burn address is return
		feeRecipient := primitives.ExecutionAddress(r.FeeRecipient)
		if feeRecipient == primitives.ExecutionAddress([20]byte{}) {
			feeRecipient = primitives.ExecutionAddress(params.BeaconConfig().DefaultFeeRecipient)
			if feeRecipient == primitives.ExecutionAddress([20]byte{}) {
				log.WithField("validatorIndex", r.ValidatorIndex).Warn("Fee recipient is the burn address")
			}
		}
		val := cache.TrackedValidator{
			Active:       true, // TODO: either check or add the field in the request
			Index:        r.ValidatorIndex,
			FeeRecipient: feeRecipient,
		}
		vs.TrackedValidatorsCache.Set(val)
		validatorIndices = append(validatorIndices, r.ValidatorIndex)
	}

	if len(validatorIndices) == 0 {
		return &emptypb.Empty{}, nil

	}

	log := log.WithField("validatorCount", len(validatorIndices))
	if logrus.GetLevel() >= logrus.TraceLevel {
		log = log.WithField("validatorIndices", validatorIndices)
	}

	log.Debug("Updated fee recipient addresses")

	return &emptypb.Empty{}, nil
}

// Deprecated: The gRPC API will remain the default and fully supported through v8 (expected in 2026) but will be eventually removed in favor of REST API.
//
// GetFeeRecipientByPubKey returns a fee recipient from the beacon node's settings or db based on a given public key
func (vs *Server) GetFeeRecipientByPubKey(ctx context.Context, request *ethpb.FeeRecipientByPubKeyRequest) (*ethpb.FeeRecipientByPubKeyResponse, error) {
	ctx, span := trace.StartSpan(ctx, "validator.GetFeeRecipientByPublicKey")
	defer span.End()
	if request == nil {
		return nil, status.Errorf(codes.InvalidArgument, "request was empty")
	}

	resp, err := vs.ValidatorIndex(ctx, &ethpb.ValidatorIndexRequest{PublicKey: request.PublicKey})
	if err != nil {
		if strings.Contains(err.Error(), "Could not find validator index") {
			return &ethpb.FeeRecipientByPubKeyResponse{
				FeeRecipient: params.BeaconConfig().DefaultFeeRecipient.Bytes(),
			}, nil
		} else {
			log.WithError(err).Error("An error occurred while retrieving validator index")
			return nil, err
		}
	}
	address, err := vs.BeaconDB.FeeRecipientByValidatorID(ctx, resp.GetIndex())
	if err != nil {
		if errors.Is(err, kv.ErrNotFoundFeeRecipient) {
			return &ethpb.FeeRecipientByPubKeyResponse{
				FeeRecipient: params.BeaconConfig().DefaultFeeRecipient.Bytes(),
			}, nil
		} else {
			log.WithError(err).Error("An error occurred while retrieving fee recipient from db")
			return nil, status.Errorf(codes.Internal, "error=%s", err)
		}
	}
	return &ethpb.FeeRecipientByPubKeyResponse{
		FeeRecipient: address.Bytes(),
	}, nil
}

// computeStateRoot computes the state root after a block has been processed through a state transition and
// returns it to the validator client.
func (vs *Server) computeStateRoot(ctx context.Context, block interfaces.SignedBeaconBlock) ([]byte, error) {
	roblock, err := blocks.NewROBlockWithRoot(block, [32]byte{}) // root is not used
	if err != nil {
		return nil, errors.Wrap(err, "could not create ROBlock")
	}
	beaconState, err := vs.BlockReceiver.GetPrestateToPropose(ctx, roblock)
	if err != nil {
		return nil, errors.Wrap(err, "could not retrieve beacon state")
	}
	root, err := transition.CalculateStateRoot(
		ctx,
		beaconState,
		block,
	)
	if err != nil {
		return vs.handleStateRootError(ctx, block, err)
	}

	log.WithField("beaconStateRoot", fmt.Sprintf("%#x", root)).Debugf("Computed state root")
	return root[:], nil
}

type computeStateRootAttemptsKeyType string

const computeStateRootAttemptsKey = computeStateRootAttemptsKeyType("compute-state-root-attempts")
const maxComputeStateRootAttempts = 3

// handleStateRootError retries block construction in some error cases.
func (vs *Server) handleStateRootError(ctx context.Context, block interfaces.SignedBeaconBlock, err error) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, status.Errorf(codes.Canceled, "context error: %v", ctx.Err())
	}
	switch {
	case errors.Is(err, transition.ErrAttestationsSignatureInvalid),
		errors.Is(err, transition.ErrProcessAttestationsFailed):
		log.WithError(err).Warn("Retrying block construction without attestations")
		if err := block.SetAttestations([]ethpb.Att{}); err != nil {
			return nil, errors.Wrap(err, "could not set attestations")
		}
	case errors.Is(err, transition.ErrProcessBLSChangesFailed), errors.Is(err, transition.ErrBLSToExecutionChangesSignatureInvalid):
		log.WithError(err).Warn("Retrying block construction without BLS to execution changes")
		if err := block.SetBLSToExecutionChanges([]*ethpb.SignedBLSToExecutionChange{}); err != nil {
			return nil, errors.Wrap(err, "could not set BLS to execution changes")
		}
	case errors.Is(err, transition.ErrProcessProposerSlashingsFailed):
		log.WithError(err).Warn("Retrying block construction without proposer slashings")
		block.SetProposerSlashings([]*ethpb.ProposerSlashing{})
	case errors.Is(err, transition.ErrProcessAttesterSlashingsFailed):
		log.WithError(err).Warn("Retrying block construction without attester slashings")
		if err := block.SetAttesterSlashings([]ethpb.AttSlashing{}); err != nil {
			return nil, errors.Wrap(err, "could not set attester slashings")
		}
	case errors.Is(err, transition.ErrProcessVoluntaryExitsFailed):
		log.WithError(err).Warn("Retrying block construction without voluntary exits")
		block.SetVoluntaryExits([]*ethpb.SignedVoluntaryExit{})
	case errors.Is(err, transition.ErrProcessSyncAggregateFailed):
		log.WithError(err).Warn("Retrying block construction without sync aggregate")
		emptySig := [96]byte{0xC0}
		emptyAggregate := &ethpb.SyncAggregate{
			SyncCommitteeBits:      make([]byte, params.BeaconConfig().SyncCommitteeSize/8),
			SyncCommitteeSignature: emptySig[:],
		}
		if err := block.SetSyncAggregate(emptyAggregate); err != nil {
			log.WithError(err).Error("Could not set sync aggregate")
		}

	default:
		return nil, errors.Wrap(err, "could not compute state root")
	}
	// prevent deep recursion by limiting max attempts.
	if v, ok := ctx.Value(computeStateRootAttemptsKey).(int); !ok {
		ctx = context.WithValue(ctx, computeStateRootAttemptsKey, int(1))
	} else if v >= maxComputeStateRootAttempts {
		return nil, fmt.Errorf("attempted max compute state root attempts %d", maxComputeStateRootAttempts)
	} else {
		ctx = context.WithValue(ctx, computeStateRootAttemptsKey, v+1)
	}
	// recursive call to compute state root again
	return vs.computeStateRoot(ctx, block)
}

// Deprecated: The gRPC API will remain the default and fully supported through v8 (expected in 2026) but will be eventually removed in favor of REST API.
//
// SubmitValidatorRegistrations submits validator registrations.
func (vs *Server) SubmitValidatorRegistrations(ctx context.Context, reg *ethpb.SignedValidatorRegistrationsV1) (*emptypb.Empty, error) {
	if vs.BlockBuilder == nil || !vs.BlockBuilder.Configured() {
		return &emptypb.Empty{}, status.Errorf(codes.InvalidArgument, "Could not register block builder: %v", builder.ErrNoBuilder)
	}

	if err := vs.BlockBuilder.RegisterValidator(ctx, reg.Messages); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Could not register block builder: %v", err)
	}

	return &emptypb.Empty{}, nil
}

func blobsAndProofs(req *ethpb.GenericSignedBeaconBlock) ([][]byte, [][]byte, error) {
	switch {
	case req.GetDeneb() != nil:
		dbBlockContents := req.GetDeneb()
		return dbBlockContents.Blobs, dbBlockContents.KzgProofs, nil
	case req.GetElectra() != nil:
		dbBlockContents := req.GetElectra()
		return dbBlockContents.Blobs, dbBlockContents.KzgProofs, nil
	case req.GetFulu() != nil:
		dbBlockContents := req.GetFulu()
		return dbBlockContents.Blobs, dbBlockContents.KzgProofs, nil
	default:
		return nil, nil, errors.Errorf("unknown request type provided: %T", req)
	}
}
