package sync

import (
	"context"
	"errors"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/sirupsen/logrus"
)

// executionProofStatusRPCHandler handles incoming ExecutionProofStatus RPC
// requests per EIP-8025. The peer reports its most recent proof-validated
// block; we reply with ours.
// spec: https://github.com/ethereum/consensus-specs/blob/master/specs/_features/eip8025/p2p-interface.md#executionproofstatus
func (s *Service) executionProofStatusRPCHandler(ctx context.Context, msg any, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.executionProofStatusRPCHandler")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, ttfbTimeout)
	defer cancel()
	SetRPCStreamDeadlines(stream)
	log := log.WithField("handler", p2p.ExecutionProofStatusName[1:]) // drop leading slash

	peerStatus, ok := msg.(*p2ptypes.ExecutionProofStatus)
	if !ok {
		return errors.New("message is not type *ExecutionProofStatus")
	}

	// Charge one rate-limit token per request.
	if err := s.rateLimiter.validateRequest(stream, 1); err != nil {
		return fmt.Errorf("validator request: %w", err)
	}
	s.rateLimiter.add(stream, 1)
	defer closeStream(stream, log)

	remotePeer := stream.Conn().RemotePeer()
	log = log.WithField("peer", remotePeer.String())
	log.WithFields(logrus.Fields{
		"peerBlockRoot": fmt.Sprintf("%#x", peerStatus.BlockRoot),
		"peerSlot":      peerStatus.Slot,
	}).Debug("Received execution proof status")

	// Remember the peer's status. Inbound handler is our only observation
	// point for peers we did not dial (we typically don't have their ENR).
	s.cfg.p2p.Peers().SetExecutionProofStatus(remotePeer, peerStatus)

	local := s.localExecutionProofStatus(ctx)
	if err := WriteExecutionProofStatusChunk(stream, s.cfg.p2p.Encoding(), local); err != nil {
		s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
		return fmt.Errorf("write execution proof status chunk: %w", err)
	}

	log.WithFields(logrus.Fields{
		"localBlockRoot": fmt.Sprintf("%#x", local.BlockRoot),
		"localSlot":      local.Slot,
	}).Debug("Responded with execution proof status")
	return nil
}

// localExecutionProofStatus reports the most recent block for which we have
// at least MIN_PROOFS_REQUIRED execution proofs persisted. Starting from the
// current head, we walk back through parents via beaconDB and check
// proofStorage.Summary(root) on each, returning the first match. Walk stops
// at the finalized checkpoint (proof storage is pruned below finality).
// If nothing qualifies, both fields are zero — per the spec that just means
// "nothing verified yet".
func (s *Service) localExecutionProofStatus(ctx context.Context) *p2ptypes.ExecutionProofStatus {
	status := &p2ptypes.ExecutionProofStatus{}

	headRoot, err := s.cfg.chain.HeadRoot(ctx)
	if err != nil {
		return status
	}
	root := bytesutil.ToBytes32(headRoot)
	if root == ([fieldparams.RootLength]byte{}) {
		return status
	}

	finalized := s.cfg.chain.FinalizedCheckpt()
	var finalizedRoot [fieldparams.RootLength]byte
	if finalized != nil {
		finalizedRoot = bytesutil.ToBytes32(finalized.Root)
	}
	required := params.BeaconConfig().MinProofsRequired

	for {
		signed, err := s.cfg.beaconDB.Block(ctx, root)
		if err != nil {
			return status
		}
		if err := blocks.BeaconBlockIsNil(signed); err != nil {
			return status
		}

		summary := s.cfg.proofStorage.Summary(root)
		if uint64(summary.Count()) >= required {
			status.BlockRoot = root
			status.Slot = signed.Block().Slot()
			return status
		}

		if root == finalizedRoot {
			return status
		}
		parent := signed.Block().ParentRoot()
		if parent == ([fieldparams.RootLength]byte{}) || parent == root {
			return status
		}
		root = parent
	}
}
