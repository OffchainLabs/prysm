// Package optionalproofs implements the EIP-8025 prover role.
//
// Provers are active validators who volunteer to generate execution proofs for
// payloads that have already been revealed. The protocol neither assigns nor
// rewards the duty, and the role is optional for clients to implement. It is
// also transitional: in a future mandatory-proof fork, builders will produce
// proofs as part of block production and the prover role disappears.
//
// The flow follows EIP-8025 prover.md. When the beacon node announces that a
// payload is available, this service rebuilds the new payload request for it, asks
// the proof node to generate proofs, signs each finished proof as an execution
// proof envelope, and hands it back to the beacon node to broadcast.
package optionalproofs

import (
	"context"
	"fmt"
	"sync"

	"github.com/OffchainLabs/prysm/v7/api/client/proofnode"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/sirupsen/logrus"
)

// Signer signs execution proof envelopes with an active validator key. The
// validator client implements it; the prover holds no keys of its own.
type Signer interface {
	SignExecutionProofEnvelope(
		ctx context.Context,
		envelope *ethpb.ExecutionProofEnvelope,
		epoch primitives.Epoch,
	) ([]byte, primitives.ValidatorIndex, error)
}

// Config configures the prover service.
type Config struct {
	BeaconApiEndpoint string
	ProofNodeEndpoint string
	ProofTypes        []ethpb.ProofType
	Signer            Signer
}

// Service generates and broadcasts execution proofs.
type Service struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    *Config

	beacon    *beaconClient
	proofNode *proofnode.Client

	// proofTypes is resolved lazily against the proof node, and retried until
	// it succeeds. proveBlock runs concurrently, so it is mutex guarded.
	proofTypesMu sync.Mutex
	proofTypes   []ethpb.ProofType
}

// NewService creates a prover service.
func NewService(ctx context.Context, cfg *Config) (*Service, error) {
	if cfg.Signer == nil {
		return nil, fmt.Errorf("prover requires a signer")
	}
	beacon, err := newBeaconClient(cfg.BeaconApiEndpoint)
	if err != nil {
		return nil, err
	}
	proofNode, err := proofnode.NewClient(cfg.ProofNodeEndpoint)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	return &Service{
		ctx:       ctx,
		cancel:    cancel,
		cfg:       cfg,
		beacon:    beacon,
		proofNode: proofNode,
	}, nil
}

// Start runs the prover until the service is stopped.
func (s *Service) Start() {
	log.WithFields(logrus.Fields{
		"beaconApiEndpoint": s.cfg.BeaconApiEndpoint,
		"proofNodeEndpoint": s.cfg.ProofNodeEndpoint,
	}).Info("Starting execution proof prover")

	go s.run()
}

// Stop halts the prover.
func (s *Service) Stop() error {
	s.cancel()
	return nil
}

// Status reports whether the prover is healthy.
func (s *Service) Status() error {
	return nil
}

// run follows the beacon node's payload availability events, proving each
// payload as it becomes available.
func (s *Service) run() {
	events := make(chan payloadAvailable, 1)
	go s.beacon.subscribePayloadAvailable(s.ctx, events)

	for {
		select {
		case <-s.ctx.Done():
			return
		case event := <-events:
			// Each payload is proved independently, so a slow proof for one
			// block must not hold up the next.
			go s.proveBlock(s.ctx, event)
		}
	}
}

// proveBlock generates, signs and submits every configured proof type for one
// revealed payload.
func (s *Service) proveBlock(ctx context.Context, event payloadAvailable) {
	log := log.WithFields(logrus.Fields{
		"slot":      event.slot,
		"blockRoot": fmt.Sprintf("%#x", event.blockRoot),
	})

	proofTypes := s.resolveProofTypes(ctx)
	if len(proofTypes) == 0 {
		log.Debug("Proof node can generate no proof types, skipping payload")
		return
	}

	newPayloadRequest, err := s.beacon.newPayloadRequest(ctx, event.blockRoot)
	if err != nil {
		log.WithError(err).Error("Could not build new payload request for payload")
		return
	}

	chainConfig, err := s.beacon.chainConfig(ctx)
	if err != nil {
		log.WithError(err).Error("Could not determine the chain config for the proof node")
		return
	}

	newPayloadRequestRoot, err := s.proofNode.RequestProofs(ctx, newPayloadRequest, chainConfig, proofTypes)
	if err != nil {
		log.WithError(err).Error("Could not request execution proofs")
		return
	}
	log.WithField("newPayloadRequestRoot", fmt.Sprintf("%#x", newPayloadRequestRoot)).
		Debug("Requested execution proofs")

	epoch := slots.ToEpoch(event.slot)
	var wg sync.WaitGroup
	for _, proofType := range proofTypes {
		wg.Go(func() {
			s.submitProof(ctx, event, epoch, newPayloadRequestRoot, proofType)
		})
	}
	wg.Wait()
}

// submitProof waits for one proof to finish generating, signs its envelope and
// submits it to the beacon node for broadcast.
func (s *Service) submitProof(
	ctx context.Context,
	event payloadAvailable,
	epoch primitives.Epoch,
	newPayloadRequestRoot [32]byte,
	proofType ethpb.ProofType,
) {
	log := log.WithFields(logrus.Fields{
		"slot":      event.slot,
		"blockRoot": fmt.Sprintf("%#x", event.blockRoot),
		"proofType": proofType,
	})

	proofData, err := s.proofNode.AwaitProof(ctx, newPayloadRequestRoot, proofType)
	if err != nil {
		log.WithError(err).Warn("Could not obtain execution proof")
		return
	}

	envelope := &ethpb.ExecutionProofEnvelope{
		ProofData:       proofData,
		ProofType:       []byte{byte(proofType)},
		BeaconBlockRoot: event.blockRoot[:],
	}

	signature, validatorIndex, err := s.cfg.Signer.SignExecutionProofEnvelope(ctx, envelope, epoch)
	if err != nil {
		log.WithError(err).Error("Could not sign execution proof envelope")
		return
	}

	signed := &ethpb.SignedExecutionProofEnvelope{
		Message:        envelope,
		ValidatorIndex: validatorIndex,
		Signature:      signature,
	}
	if err := s.beacon.submitExecutionProof(ctx, signed); err != nil {
		log.WithError(err).Error("Could not submit execution proof")
		return
	}

	log.WithFields(logrus.Fields{
		"prover":    validatorIndex,
		"proofSize": len(proofData),
	}).Info("Submitted execution proof")
}

// resolveProofTypes determines which proof types to generate, once, by asking
// the proof node what it can produce and intersecting that with the configured
// selection.
func (s *Service) resolveProofTypes(ctx context.Context) []ethpb.ProofType {
	s.proofTypesMu.Lock()
	defer s.proofTypesMu.Unlock()

	if s.proofTypes != nil {
		return s.proofTypes
	}

	provable, err := s.proofNode.ProvableTypes(ctx)
	if err != nil {
		// Leave the resolution unset so the next payload retries.
		log.WithError(err).Error("Could not list the proof node's proof types")
		return nil
	}

	s.proofTypes = provable

	if len(s.cfg.ProofTypes) > 0 {
		s.proofTypes = intersect(s.cfg.ProofTypes, provable)
	}

	log.WithField("proofTypes", s.proofTypes).Info("Resolved execution proof types")
	return s.proofTypes
}

func intersect(requested, available []ethpb.ProofType) []ethpb.ProofType {
	set := make(map[ethpb.ProofType]bool, len(available))
	for _, proofType := range available {
		set[proofType] = true
	}

	out := make([]ethpb.ProofType, 0, len(requested))
	for _, proofType := range requested {
		if set[proofType] {
			out = append(out, proofType)
			continue
		}
		log.WithField("proofType", proofType).Warn("Proof node cannot generate the requested proof type")
	}

	return out
}
