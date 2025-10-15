package sync

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/async"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	p2ptypes "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	prysmTime "github.com/OffchainLabs/prysm/v6/time"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/sirupsen/logrus"
)

// maintainPeerStatuses maintains peer statuses by polling peers for their latest status twice per epoch.
func (s *Service) maintainPeerStatuses() {
	// Run twice per epoch.
	interval := time.Duration(params.BeaconConfig().SlotsPerEpoch.Div(2).Mul(params.BeaconConfig().SecondsPerSlot)) * time.Second
	async.RunEvery(s.ctx, interval, func() {
		wg := new(sync.WaitGroup)
		for _, pid := range s.cfg.p2p.Peers().Connected() {
			wg.Add(1)
			go func(id peer.ID) {
				defer wg.Done()

				log := log.WithFields(logrus.Fields{
					"peer":  id,
					"agent": agentString(id, s.cfg.p2p.Host()),
				})

				// If our peer status has not been updated correctly we disconnect over here
				// and set the connection state over here instead.
				if s.cfg.p2p.Host().Network().Connectedness(id) != network.Connected {
					s.cfg.p2p.Peers().SetConnectionState(id, peers.Disconnecting)
					if err := s.cfg.p2p.Disconnect(id); err != nil {
						log.WithError(err).Debug("Error when disconnecting with peer")
					}
					s.cfg.p2p.Peers().SetConnectionState(id, peers.Disconnected)
					log.WithField("reason", "maintainPeerStatusesNotConnectedPeer").Debug("Initiate peer disconnection")
					return
				}

				// Disconnect from peers that are considered bad by any of the registered scorers.
				if err := s.cfg.p2p.Peers().IsBad(id); err != nil {
					s.disconnectBadPeer(s.ctx, id, err)
					return
				}

				// If the status hasn't been updated in the recent interval time.
				lastUpdated, err := s.cfg.p2p.Peers().ChainStateLastUpdated(id)
				if err != nil {
					// Peer has vanished; nothing to do.
					return
				}

				if prysmTime.Now().After(lastUpdated.Add(interval)) {
					if err := s.reValidatePeer(s.ctx, id); err != nil {
						log.WithError(err).Debug("Cannot re-validate peer")
					}
				}
			}(pid)
		}
		// Wait for all status checks to finish and then proceed onwards to
		// pruning excess peers.
		wg.Wait()
		peerIds := s.cfg.p2p.Peers().PeersToPrune()
		peerIds = s.filterNeededPeers(peerIds)
		for _, id := range peerIds {
			if err := s.sendGoodByeAndDisconnect(s.ctx, p2ptypes.GoodbyeCodeTooManyPeers, id); err != nil {
				log.WithField("peer", id).WithError(err).Debug("Could not disconnect with peer")
			}

			log.WithFields(logrus.Fields{
				"peer":   id,
				"reason": "to be pruned",
			}).Debug("Initiate peer disconnection")
		}
	})
}

// resyncIfBehind checks periodically to see if we are in normal sync but have fallen behind our peers
// by more than an epoch, in which case we attempt a resync using the initial sync method to catch up.
func (s *Service) resyncIfBehind() {
	millisecondsPerEpoch := params.BeaconConfig().SlotsPerEpoch.Mul(1000).Mul(params.BeaconConfig().SecondsPerSlot)
	// Run sixteen times per epoch.
	interval := time.Duration(millisecondsPerEpoch/16) * time.Millisecond
	async.RunEvery(s.ctx, interval, func() {
		if s.shouldReSync() {
			syncedEpoch := slots.ToEpoch(s.cfg.chain.HeadSlot())
			// Factor number of expected minimum sync peers, to make sure that enough peers are
			// available to resync (some peers may go away between checking non-finalized peers and
			// actual resyncing).
			highestEpoch, _ := s.cfg.p2p.Peers().BestNonFinalized(flags.Get().MinimumSyncPeers*2, syncedEpoch)
			// Check if the current node is more than 1 epoch behind.
			if highestEpoch > (syncedEpoch + 1) {
				log.WithFields(logrus.Fields{
					"currentEpoch": slots.ToEpoch(s.cfg.clock.CurrentSlot()),
					"syncedEpoch":  syncedEpoch,
					"peersEpoch":   highestEpoch,
				}).Info("Fallen behind peers; reverting to initial sync to catch up")
				numberOfTimesResyncedCounter.Inc()
				s.clearPendingSlots()
				if err := s.cfg.initialSync.Resync(); err != nil {
					log.WithError(err).Errorf("Could not resync chain")
				}
			}
		}
	})
}

// shouldReSync returns true if the node is not syncing and falls behind two epochs.
func (s *Service) shouldReSync() bool {
	syncedEpoch := slots.ToEpoch(s.cfg.chain.HeadSlot())
	currentEpoch := slots.ToEpoch(s.cfg.clock.CurrentSlot())
	prevEpoch := primitives.Epoch(0)
	if currentEpoch > 1 {
		prevEpoch = currentEpoch - 1
	}
	return s.cfg.initialSync != nil && !s.cfg.initialSync.Syncing() && syncedEpoch < prevEpoch
}

// sendRPCStatusRequest for a given topic with an expected protobuf message type.
func (s *Service) sendRPCStatusRequest(ctx context.Context, peer peer.ID) error {
	ctx, cancel := context.WithTimeout(ctx, respTimeout)
	defer cancel()

	headRoot, err := s.cfg.chain.HeadRoot(ctx)
	if err != nil {
		return errors.Wrap(err, "chain head root")
	}

	forkDigest, err := s.currentForkDigest()
	if err != nil {
		return errors.Wrap(err, "current fork digest")
	}

	// Compute the current epoch.
	currentSlot := s.cfg.clock.CurrentSlot()
	currentEpoch := slots.ToEpoch(currentSlot)

	// Compute the topic for the status request regarding the current epoch.
	topic, err := p2p.TopicFromMessage(p2p.StatusMessageName, currentEpoch)
	if err != nil {
		return errors.Wrap(err, "topic from message")
	}

	cp := s.cfg.chain.FinalizedCheckpt()
	status, err := s.buildStatusFromEpoch(ctx, currentEpoch, forkDigest, cp.Root, cp.Epoch, headRoot)
	if err != nil {
		return errors.Wrap(err, "build status from epoch")
	}

	stream, err := s.cfg.p2p.Send(ctx, status, topic, peer)
	if err != nil {
		return errors.Wrap(err, "p2p send")
	}

	defer closeStream(stream, log)

	code, errMsg, err := ReadStatusCode(stream, s.cfg.p2p.Encoding())
	if err != nil {
		s.downscorePeer(peer, "statusRequestReadStatusCodeError")
		return errors.Wrap(err, "read status code")
	}
	if code != 0 {
		s.downscorePeer(peer, "statusRequestNonNullStatusCode")
		return errors.New(errMsg)
	}

	msg, err := s.decodeStatus(stream, currentEpoch)
	if err != nil {
		return errors.Wrap(err, "decode status")
	}

	// If validation fails, validation error is logged, and peer status scorer will mark peer as bad.
	err = s.validateStatusMessage(ctx, msg)
	s.cfg.p2p.Peers().Scorers().PeerStatusScorer().SetPeerStatus(peer, msg, err)
	if err := s.cfg.p2p.Peers().IsBad(peer); err != nil {
		s.disconnectBadPeer(s.ctx, peer, err)
	}

	return err
}

func (s *Service) decodeStatus(stream network.Stream, epoch primitives.Epoch) (*pb.StatusV2, error) {
	if epoch >= params.BeaconConfig().FuluForkEpoch {
		msg := new(pb.StatusV2)
		if err := s.cfg.p2p.Encoding().DecodeWithMaxLength(stream, msg); err != nil {
			s.cfg.p2p.Peers().Scorers().BadResponsesScorer().Increment(stream.Conn().RemotePeer())
			return nil, errors.Wrap(err, "decode with max length")
		}

		return msg, nil
	}

	msg := new(pb.Status)
	if err := s.cfg.p2p.Encoding().DecodeWithMaxLength(stream, msg); err != nil {
		s.cfg.p2p.Peers().Scorers().BadResponsesScorer().Increment(stream.Conn().RemotePeer())
		return nil, errors.Wrap(err, "decode with max length")
	}

	status, err := statusV2(msg)
	if err != nil {
		return nil, errors.Wrap(err, "status data")
	}

	return status, nil
}

func (s *Service) reValidatePeer(ctx context.Context, id peer.ID) error {
	s.cfg.p2p.Peers().Scorers().PeerStatusScorer().SetHeadSlot(s.cfg.chain.HeadSlot())
	if err := s.sendRPCStatusRequest(ctx, id); err != nil {
		return err
	}
	// Do not return an error for ping requests.
	if err := s.sendPingRequest(ctx, id); err != nil && !isUnwantedError(err) {
		log.WithError(err).WithField("pid", id).Debug("Could not ping peer")
	}
	return nil
}

// statusRPCHandler reads the incoming Status RPC from the peer and responds with our version of a status message.
// This handler will disconnect any peer that does not match our fork version.
func (s *Service) statusRPCHandler(ctx context.Context, msg interface{}, stream libp2pcore.Stream) error {
	ctx, cancel := context.WithTimeout(ctx, ttfbTimeout)
	defer cancel()
	SetRPCStreamDeadlines(stream)
	log := log.WithField("handler", "status")
	m, err := statusV2(msg)
	if err != nil {
		return errors.Wrap(err, "status data")
	}

	if err := s.rateLimiter.validateRequest(stream, 1); err != nil {
		return err
	}
	s.rateLimiter.add(stream, 1)

	remotePeer := stream.Conn().RemotePeer()
	if err := s.validateStatusMessage(ctx, m); err != nil {
		log.WithFields(logrus.Fields{
			"peer":  remotePeer,
			"error": err,
			"agent": agentString(remotePeer, s.cfg.p2p.Host()),
		}).Debug("Invalid status message from peer")

		var respCode byte
		switch {
		case errors.Is(err, p2ptypes.ErrGeneric):
			respCode = responseCodeServerError
		case errors.Is(err, p2ptypes.ErrWrongForkDigestVersion):
			// Respond with our status and disconnect with the peer.
			s.cfg.p2p.Peers().SetChainState(remotePeer, m)
			if err := s.respondWithStatus(ctx, stream); err != nil {
				return err
			}
			// Close before disconnecting, and wait for the other end to ack our response.
			closeStreamAndWait(stream, log)
			if err := s.sendGoodByeAndDisconnect(ctx, p2ptypes.GoodbyeCodeWrongNetwork, remotePeer); err != nil {
				return err
			}
			return nil
		default:
			respCode = responseCodeInvalidRequest
			s.downscorePeer(remotePeer, "statusRpcHandlerInvalidMessage")
		}

		originalErr := err
		resp, err := s.generateErrorResponse(respCode, err.Error())
		if err != nil {
			log.WithError(err).Debug("Could not generate a response error")
		} else if _, err := stream.Write(resp); err != nil && !isUnwantedError(err) {
			// The peer may already be ignoring us, as we disagree on fork version, so log this as debug only.
			log.WithError(err).Debug("Could not write to stream")
		}
		closeStreamAndWait(stream, log)
		if err := s.sendGoodByeAndDisconnect(ctx, p2ptypes.GoodbyeCodeGenericError, remotePeer); err != nil {
			return err
		}
		return originalErr
	}
	s.cfg.p2p.Peers().SetChainState(remotePeer, m)

	if err := s.respondWithStatus(ctx, stream); err != nil {
		return err
	}
	closeStream(stream, log)
	return nil
}

func (s *Service) respondWithStatus(ctx context.Context, stream network.Stream) error {
	headRoot, err := s.cfg.chain.HeadRoot(ctx)
	if err != nil {
		return errors.Wrap(err, "chain head root")
	}

	forkDigest, err := s.currentForkDigest()
	if err != nil {
		return errors.Wrap(err, "current fork digest")
	}

	cp := s.cfg.chain.FinalizedCheckpt()
	status, err := s.buildStatusFromStream(ctx, stream, forkDigest, cp.Root, cp.Epoch, headRoot)
	if err != nil {
		return errors.Wrap(err, "build status")
	}

	if _, err := stream.Write([]byte{responseCodeSuccess}); err != nil && !isUnwantedError(err) {
		log.WithError(err).Debug("Could not write to stream")
	}

	if _, err := s.cfg.p2p.Encoding().EncodeWithMaxLength(stream, status); err != nil {
		return errors.Wrap(err, "encode with max length")
	}

	return nil
}

func (s *Service) buildStatusFromStream(
	ctx context.Context,
	stream libp2pcore.Stream,
	forkDigest [4]byte,
	finalizedRoot []byte,
	FinalizedEpoch primitives.Epoch,
	headRoot []byte,
) (ssz.Marshaler, error) {
	// Get the stream version from the protocol.
	_, _, streamVersion, err := p2p.TopicDeconstructor(string(stream.Protocol()))
	if err != nil {
		err := errors.Wrap(err, "topic deconstructor")

		resp, err2 := s.generateErrorResponse(responseCodeServerError, types.ErrGeneric.Error())
		if err2 != nil {
			log.WithError(err2).Debug("Could not write to stream")
			return nil, err
		}

		if _, err2 := stream.Write(resp); err2 != nil {
			log.WithError(err2).Debug("Could not write to stream")
		}

		return nil, err
	}

	if params.FuluEnabled() && streamVersion == p2p.SchemaVersionV2 {
		earliestAvailableSlot, err := s.cfg.p2p.EarliestAvailableSlot(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "earliest available slot")
		}

		status := &pb.StatusV2{
			ForkDigest:            forkDigest[:],
			FinalizedRoot:         finalizedRoot,
			FinalizedEpoch:        FinalizedEpoch,
			HeadRoot:              headRoot,
			HeadSlot:              s.cfg.chain.HeadSlot(),
			EarliestAvailableSlot: earliestAvailableSlot,
		}

		return status, nil
	}

	status := &pb.Status{
		ForkDigest:     forkDigest[:],
		FinalizedRoot:  finalizedRoot,
		FinalizedEpoch: FinalizedEpoch,
		HeadRoot:       headRoot,
		HeadSlot:       s.cfg.chain.HeadSlot(),
	}

	return status, nil
}

func (s *Service) buildStatusFromEpoch(
	ctx context.Context,
	epoch primitives.Epoch,
	forkDigest [4]byte,
	finalizedRoot []byte,
	FinalizedEpoch primitives.Epoch,
	headRoot []byte,
) (ssz.Marshaler, error) {
	// Get the stream version from the protocol.
	if epoch >= params.BeaconConfig().FuluForkEpoch {
		earliestAvailableSlot, err := s.cfg.p2p.EarliestAvailableSlot(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "earliest available slot")
		}

		status := &pb.StatusV2{
			ForkDigest:            forkDigest[:],
			FinalizedRoot:         finalizedRoot,
			FinalizedEpoch:        FinalizedEpoch,
			HeadRoot:              headRoot,
			HeadSlot:              s.cfg.chain.HeadSlot(),
			EarliestAvailableSlot: earliestAvailableSlot,
		}

		return status, nil
	}

	status := &pb.Status{
		ForkDigest:     forkDigest[:],
		FinalizedRoot:  finalizedRoot,
		FinalizedEpoch: FinalizedEpoch,
		HeadRoot:       headRoot,
		HeadSlot:       s.cfg.chain.HeadSlot(),
	}

	return status, nil
}

func (s *Service) validateStatusMessage(ctx context.Context, genericMsg interface{}) error {
	msg, err := statusV2(genericMsg)
	if err != nil {
		return errors.Wrap(err, "status data")
	}

	forkDigest, err := s.currentForkDigest()
	if err != nil {
		return err
	}
	if !bytes.Equal(forkDigest[:], msg.ForkDigest) {
		return fmt.Errorf("mismatch fork digest: expected %#x, got %#x: %w", forkDigest[:], msg.ForkDigest, p2ptypes.ErrWrongForkDigestVersion)
	}
	genesis := s.cfg.clock.GenesisTime()
	cp := s.cfg.chain.FinalizedCheckpt()
	finalizedEpoch := cp.Epoch
	maxEpoch := slots.EpochsSinceGenesis(genesis)
	// It would take a minimum of 2 epochs to finalize a
	// previous epoch
	maxFinalizedEpoch := primitives.Epoch(0)
	if maxEpoch > 2 {
		maxFinalizedEpoch = maxEpoch - 2
	}
	if msg.FinalizedEpoch > maxFinalizedEpoch {
		return p2ptypes.ErrInvalidEpoch
	}
	// Exit early if the peer's finalized epoch
	// is less than that of the remote peer's.
	if finalizedEpoch < msg.FinalizedEpoch {
		return nil
	}
	finalizedAtGenesis := msg.FinalizedEpoch == 0
	rootIsEqual := bytes.Equal(params.BeaconConfig().ZeroHash[:], msg.FinalizedRoot)
	// If peer is at genesis with the correct genesis root hash we exit.
	if finalizedAtGenesis && rootIsEqual {
		return nil
	}
	if !s.cfg.chain.IsFinalized(ctx, bytesutil.ToBytes32(msg.FinalizedRoot)) {
		log.WithField("root", fmt.Sprintf("%#x", msg.FinalizedRoot)).Debug("Could not validate finalized root")
		return p2ptypes.ErrInvalidFinalizedRoot
	}
	blk, err := s.cfg.beaconDB.Block(ctx, bytesutil.ToBytes32(msg.FinalizedRoot))
	if err != nil {
		return p2ptypes.ErrGeneric
	}
	if blk == nil || blk.IsNil() {
		return p2ptypes.ErrGeneric
	}
	if slots.ToEpoch(blk.Block().Slot()) == msg.FinalizedEpoch {
		return nil
	}

	startSlot, err := slots.EpochStart(msg.FinalizedEpoch)
	if err != nil {
		return p2ptypes.ErrGeneric
	}
	if startSlot > blk.Block().Slot() {
		childBlock, err := s.cfg.beaconDB.FinalizedChildBlock(ctx, bytesutil.ToBytes32(msg.FinalizedRoot))
		if err != nil {
			return p2ptypes.ErrGeneric
		}
		// Is a valid finalized block if no
		// other child blocks exist yet.
		if childBlock == nil || childBlock.IsNil() {
			return nil
		}
		// If child finalized block also has a smaller or
		// equal slot number we return an error.
		if startSlot >= childBlock.Block().Slot() {
			return p2ptypes.ErrInvalidEpoch
		}
		return nil
	}
	return p2ptypes.ErrInvalidEpoch
}

func statusV2(msg interface{}) (*pb.StatusV2, error) {
	if status, ok := msg.(*pb.StatusV2); ok {
		return status, nil
	}

	if status, ok := msg.(*pb.Status); ok {
		status := &pb.StatusV2{
			ForkDigest:            status.ForkDigest,
			FinalizedRoot:         status.FinalizedRoot,
			FinalizedEpoch:        status.FinalizedEpoch,
			HeadRoot:              status.HeadRoot,
			HeadSlot:              status.HeadSlot,
			EarliestAvailableSlot: 0, // Default value for StatusV2
		}

		return status, nil
	}

	return nil, errors.New("message is not type *pb.Status or *pb.StatusV2")
}
