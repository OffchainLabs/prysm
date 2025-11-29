package proofgeneration

import (
	"context"

	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("prefix", "db-pruner")

type Service struct {
	ctx context.Context
}

func NewService(ctx context.Context) (*Service, error) {
	return &Service{
		ctx: ctx,
	}, nil
}

func (s *Service) Start() {
	log.Info("Starting Proof Generation Service")
}

func (*Service) Stop() error {
	return nil
}

func (*Service) Status() error {
	return nil
}
