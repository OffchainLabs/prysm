package proofgeneration

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	statefeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/state"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("prefix", "proof-generation")

type Service struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    *Config
}

func NewService(ctx context.Context, cfg *Config) (*Service, error) {
	ctx, cancel := context.WithCancel(ctx)

	return &Service{
		ctx:    ctx,
		cancel: cancel,
		cfg:    cfg,
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
