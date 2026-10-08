package initialsync

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	prysmsync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

func (s *Service) ensureParentPayload(ctx context.Context, child blocks.ROBlock, envelopes []interfaces.ROSignedExecutionPayloadEnvelope, preferred peer.ID) ([]interfaces.ROSignedExecutionPayloadEnvelope, error) {
	root := child.Block().ParentRoot()
	if child.Version() < version.Gloas || s.cfg.DB.HasExecutionPayloadEnvelope(ctx, root) {
		return envelopes, nil
	}
	signed, err := s.cfg.DB.Block(ctx, root)
	if err != nil {
		return nil, err
	}
	if signed == nil {
		// A failed batch save can leave the previous head only in memory.
		signed, err = s.cfg.Chain.HeadBlock(ctx)
		if err != nil {
			return nil, err
		}
		head, err := blocks.NewROBlock(signed)
		if err != nil {
			return nil, err
		}
		if head.Root() != root {
			return nil, errors.Errorf("parent block %#x unavailable", root)
		}
	}
	parent, err := blocks.NewROBlockWithRoot(signed, root)
	if err != nil {
		return nil, err
	}
	if parent.Version() < version.Gloas || parent.Block().Slot() == 0 {
		return envelopes, nil
	}
	full, err := blocks.BlockBuiltOnParentPayload(parent.Block(), child.Block())
	if err != nil || !full {
		return envelopes, err
	}
	supplied := false
	if len(envelopes) > 0 {
		supplied, err = blocks.BlockBuiltOnParentEnvelope(envelopes[0], child)
		if err != nil {
			return nil, err
		}
	}
	if !supplied {
		envelope, err := s.fetchParentPayload(ctx, parent, child, preferred)
		if err != nil {
			return nil, err
		}
		envelopes = append([]interfaces.ROSignedExecutionPayloadEnvelope{envelope}, envelopes...)
	}
	// Prefetch may have received the envelope before its block was local.
	if err := s.fetchParentColumns(ctx, parent); err != nil {
		return nil, err
	}
	return envelopes, nil
}

func (s *Service) fetchParentPayload(ctx context.Context, parent, child blocks.ROBlock, preferred peer.ID) (interfaces.ROSignedExecutionPayloadEnvelope, error) {
	const maxAttempts = 3
	_, peers := s.cfg.P2P.Peers().BestNonFinalized(params.BeaconConfig().MaxPeersToSync, s.cfg.Chain.FinalizedCheckpt().Epoch)
	shufflePeers(peers)
	if preferred != "" {
		peers = append([]peer.ID{preferred}, peers...)
	}
	peers = dedupPeers(peers)
	peers = peers[:min(len(peers), maxAttempts)]
	request := p2ptypes.ExecutionPayloadEnvelopesByRootReq{parent.Root()}
	for _, pid := range peers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		requestCtx, cancel := context.WithTimeout(ctx, params.BeaconConfig().RespTimeoutDuration())
		response, err := prysmsync.SendExecutionPayloadEnvelopesByRootRequest(requestCtx, s.clock, s.cfg.P2P, pid, s.ctxMap, &request)
		cancel()
		if err != nil || len(response) != 1 {
			continue
		}
		envelope, err := blocks.WrappedROSignedExecutionPayloadEnvelope(response[0])
		if err != nil {
			continue
		}
		matches, err := blocks.BlockBuiltOnParentEnvelope(envelope, child)
		if err != nil || !matches || response[0].Message.Payload.SlotNumber != parent.Block().Slot() {
			continue
		}
		return envelope, nil
	}
	return nil, errors.Errorf("no peer could provide required parent execution payload envelope %#x", parent.Root())
}

func (s *Service) fetchParentColumns(ctx context.Context, parent blocks.ROBlock) error {
	commitments, err := parent.Block().Body().BlobKzgCommitments()
	if err != nil {
		return err
	}
	if len(commitments) == 0 || !params.WithinDAPeriod(slots.ToEpoch(parent.Block().Slot()), slots.ToEpoch(s.clock.CurrentSlot())) {
		return nil
	}
	custodyGroupCount, err := s.cfg.P2P.CustodyGroupCount(ctx)
	if err != nil {
		return err
	}
	info, _, err := peerdas.Info(s.cfg.P2P.NodeID(), max(custodyGroupCount, params.BeaconConfig().SamplesPerSlot))
	if err != nil {
		return err
	}
	columns, missing, err := prysmsync.FetchDataColumnSidecars(prysmsync.DataColumnSidecarsParams{
		Ctx: ctx, Tor: s.clock, P2P: s.cfg.P2P, CtxMap: s.ctxMap,
		Storage: s.cfg.DataColumnStorage, NewVerifier: s.newDataColumnsVerifier,
		RequestByRoot: true,
	}, []blocks.ROBlock{parent}, info.CustodyColumns)
	if err != nil {
		return errors.Wrap(err, "fetch parent data columns")
	}
	if len(missing) != 0 {
		return errors.Errorf("data columns unavailable for parent %#x", parent.Root())
	}
	return s.cfg.DataColumnStorage.Save(columns[parent.Root()])
}
