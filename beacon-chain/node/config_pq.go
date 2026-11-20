package node

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

func configurePQProver(cliCtx *cli.Context) error {
	socket := cliCtx.String(flags.PQProverSocket.Name)
	if socket == "" {
		return nil
	}
	backend, err := pq.NewSidecarBackend(socket)
	if err != nil {
		return errors.Wrap(err, "could not start the PQ proof backend")
	}
	pq.Proofs = backend
	log.WithField("socket", socket).Info("Using the leanVM sidecar for PQ attestation proofs")
	return nil
}
