package proofgeneration

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	statefeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	zkvmexecutionlayer "github.com/OffchainLabs/prysm/v7/zkvm-execution-layer"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("prefix", "proof-generation")

type Service struct {
	ctx               context.Context
	cancel            context.CancelFunc
	cfg               *Config
	GeneratorRegistry *zkvmexecutionlayer.GeneratorRegistry
}

func NewService(ctx context.Context, cfg *Config) (*Service, error) {
	ctx, cancel := context.WithCancel(ctx)

	enabledProofTypes := make(map[primitives.ExecutionProofId]struct{})
	for _, proofType := range cfg.ProofTypes {
		enabledProofTypes[proofType] = struct{}{}
	}

	// Create a proof generator registry with dummy one.
	generatorRegistry := zkvmexecutionlayer.NewGeneratorRegistryWithDummyGenerators(enabledProofTypes)

	return &Service{
		ctx:               ctx,
		cancel:            cancel,
		cfg:               cfg,
		GeneratorRegistry: generatorRegistry,
	}, nil
}

func (s *Service) Start() {
	log.Info("Starting Proof Generation Service")

	stateChannel := make(chan *feed.Event, 1)
	stateSub := s.cfg.StateNotifier.StateFeed().Subscribe(stateChannel)
	defer stateSub.Unsubscribe()

	for {
		select {
		case e := <-stateChannel:
			if e.Type == statefeed.BlockProcessed {
				data, ok := e.Data.(*statefeed.BlockProcessedData)
				if !ok {
					log.Error("Event feed data is not of type *statefeed.BlockProcessedData")
				} else if data.Verified {
					log.WithFields(logrus.Fields{
						"blockRoot": fmt.Sprintf("%x", data.BlockRoot),
						"slot":      data.Slot,
					}).Info("New verified block processed - generating proofs")

					proofs, err := s.GenerateProofs()
					if err != nil {
						log.WithError(err).Error("Failed to generate proofs")
						continue
					}
					if proofs == nil || len(proofs) == 0 {
						log.Info("No proofs generated for this block")
						continue
					}

					// Broadcast the generated proofs
					for _, proof := range proofs {
						if err := s.cfg.Broadcaster.Broadcast(s.ctx, proof); err != nil {
							log.WithError(err).Error("Failed to broadcast execution proof")
						} else {
							log.WithField("proofType", proof.ProofId).Info("Broadcasted execution proof")
						}
					}
				}
			}
		case <-s.ctx.Done():
			log.Info("Stopping Proof Generation Service")
			return
		case err := <-stateSub.Err():
			log.WithError(err).Error("Could not subscribe to state notifier")
			return
		}
	}
}

func (s *Service) Stop() error {
	s.cancel()
	return nil
}

func (*Service) Status() error {
	return nil
}
