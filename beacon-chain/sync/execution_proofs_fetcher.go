package sync

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/sirupsen/logrus"
)

// knownExecutionProofTypes lists every proof type the background fetcher will
// request. See https://github.com/eth-act/zkboost#configuration.
var knownExecutionProofTypes = []uint8{
	// ethpb.ProofTypeEthrexRisc0,
	// ethpb.ProofTypeEthrexSP1,
	// ethpb.ProofTypeEthrexZisk,
	ethpb.ProofTypeRethOpenVM,
	ethpb.ProofTypeRethRisc0,
	ethpb.ProofTypeRethSP1,
	ethpb.ProofTypeRethZisk,
}

// executionProofsFetcherLoop runs until context cancellation, once per slot,
// asking every zkvm-enabled peer for the proof types missing from unfinalized
// blocks.
func (s *Service) executionProofsFetcherLoop() {
	slotTicker := slots.NewSlotTicker(s.cfg.clock.GenesisTime(), params.BeaconConfig().SecondsPerSlot)
	defer slotTicker.Done()

	for {
		select {
		case <-slotTicker.C():
			s.fetchMissingExecutionProofs()
		case <-s.ctx.Done():
			log.Debug("Context closed, exiting execution proofs fetcher loop")
			return
		}
	}
}

// fetchMissingExecutionProofs performs one pass over unfinalized blocks that
// do not yet have enough execution proofs, sending a single batched request to
// each zkvm-enabled peer until storage is satisfied or peers are exhausted.
func (s *Service) fetchMissingExecutionProofs() {
	if !params.FuluEnabled() {
		return
	}

	roots, err := s.cfg.chain.RootsMissingExecutionProofs()
	if err != nil {
		log.WithError(err).Error("Could not list unfinalized blocks missing execution proofs")
		return
	}
	if len(roots) == 0 {
		return
	}

	// TODO: We don't take into account the latest root/slot with proof of the peer
	peers := s.cfg.p2p.Peers().ZkvmEnabledPeers()
	if len(peers) == 0 {
		log.Warning("No zkvm-enabled peers available to fetch execution proofs")
		return
	}

	for peer := range peers {
		request := s.buildMissingExecutionProofsRequest(roots)
		if len(request) == 0 {
			log.Debug("No execution proofs to fetch")
			return
		}

		log := log.WithField("peer", peer)
		requestedByRoot := make(map[string][]string, len(request))
		for _, identifier := range request {
			rootKey := fmt.Sprintf("%#x", identifier.BlockRoot)
			names := make([]string, 0, len(identifier.ProofTypes))
			for _, pt := range identifier.ProofTypes {
				names = append(names, ethpb.ProofTypeName(pt))
			}
			requestedByRoot[rootKey] = names
		}
		log.WithFields(logrus.Fields{
			"blocks":          len(request),
			"requestedByRoot": requestedByRoot,
		}).Debug("Fetching missing execution proofs from peer")

		proofs, err := s.sendExecutionProofsByRootRequest(s.ctx, peer, request)
		if err != nil {
			log.WithError(err).Warning("Could not fetch execution proofs from peer")
			continue
		}

		for _, proof := range proofs {
			if err := s.verifyAndReceiveProof(proof); err != nil {
				log.WithError(err).WithField("root", fmt.Sprintf("%#x", proof.BlockRoot())).Debug("Could not verify and receive execution proof")
			}
		}
	}
}

// verifyAndReceiveProof runs the same verification pipeline as the gossip
// validator (active validator, prover signature, proof data bounds, ZK
// verifier) and, on success, routes the verified proof through ReceiveProof.
// Proofs whose (newPayloadRequestRoot, proofType, proverPubkey) tuple was
// already seen are skipped, matching gossip dedup semantics.
func (s *Service) verifyAndReceiveProof(proof blocks.ROSignedExecutionProof) error {
	if s.hasSeenProof(&proof) {
		return nil
	}

	s.setSeenProof(&proof)

	verifier := s.newSignedExecutionProofsVerifier(
		[]blocks.ROSignedExecutionProof{proof},
		verification.GossipSignedExecutionProofRequirements,
	)

	if err := verifier.IsFromActiveValidator(); err != nil {
		return fmt.Errorf("is from active validator: %w", err)
	}

	if err := verifier.ValidProverSignature(s.ctx); err != nil {
		return fmt.Errorf("valid prover signature: %w", err)
	}

	if err := verifier.ProofDataNonEmpty(); err != nil {
		return fmt.Errorf("proof data non empty: %w", err)
	}

	if err := verifier.ProofDataNotTooLarge(); err != nil {
		return fmt.Errorf("proof data not too large: %w", err)
	}

	if err := verifier.ProofVerified(); err != nil {
		return fmt.Errorf("proof verified: %w", err)
	}

	if s.hasSeenValidProof(&proof) {
		return nil
	}

	verified, err := verifier.VerifiedROSignedExecutionProofs()
	if err != nil {
		return fmt.Errorf("verified ro signed execution proofs: %w", err)
	}

	s.setSeenValidProof(&proof)

	if err := s.cfg.chain.ReceiveProof(verified[0]); err != nil {
		return fmt.Errorf("receive proof: %w", err)
	}
	return nil
}

// buildMissingExecutionProofsRequest assembles one ExecutionProofsByRoot
// request covering every root that still has missing proofs in storage.
func (s *Service) buildMissingExecutionProofsRequest(roots [][fieldparams.RootLength]byte) types.ExecutionProofsByRootReq {
	cfg := params.BeaconConfig()
	maxProofs := cfg.MaxRequestBlocksDeneb * cfg.MaxExecutionProofsPerPayload

	request := make(types.ExecutionProofsByRootReq, 0, len(roots))
	totalProofs := uint64(0)
	for _, root := range roots {
		missing := s.missingProofTypes(root)
		if len(missing) == 0 {
			continue
		}

		if totalProofs+uint64(len(missing)) > maxProofs {
			break
		}

		request = append(request, &ethpb.ProofByRootIdentifier{
			BlockRoot:  root[:],
			ProofTypes: missing,
		})

		totalProofs += uint64(len(missing))
	}

	return request
}

// missingProofTypes returns the proof types from knownExecutionProofTypes that
// are not yet in storage for the given block root. An empty result means the
// block already has every known proof type (and MinProofsRequired is met).
// The slice is capped at MAX_EXECUTION_PROOFS_PER_PAYLOAD since that is both
// the maximum number of proofs a payload can carry and the SSZ list bound on
// ProofByRootIdentifier.ProofTypes.
func (s *Service) missingProofTypes(root [fieldparams.RootLength]byte) []uint8 {
	summary := s.cfg.proofStorage.Summary(root)
	if uint64(len(summary.All())) >= params.BeaconConfig().MinProofsRequired {
		return nil
	}
	maxPerPayload := params.BeaconConfig().MaxExecutionProofsPerPayload
	missing := make([]uint8, 0, maxPerPayload)
	for _, pt := range knownExecutionProofTypes {
		if summary.HasProof(pt) {
			continue
		}
		missing = append(missing, pt)
		if uint64(len(missing)) >= maxPerPayload {
			break
		}
	}
	return missing
}
