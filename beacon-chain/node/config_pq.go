package node

import (
	"path/filepath"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations/pqpool"
	initialsync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync"
	"github.com/OffchainLabs/prysm/v7/cmd"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
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

func (b *BeaconNode) registerPQTimingService() error {
	measure := b.cliCtx.Bool(flags.PQMeasurement.Name)
	output := b.cliCtx.String(flags.PQTimingOutput.Name)
	if measure && output == "" {
		output = filepath.Join(b.cliCtx.String(cmd.DataDirFlag.Name), "pq-timing")
	}
	if output == "" {
		return nil
	}
	var chainService *blockchain.Service
	if err := b.services.FetchService(&chainService); err != nil {
		return err
	}
	var initSync *initialsync.Service
	if err := b.services.FetchService(&initSync); err != nil {
		return err
	}
	peers := b.fetchP2P().Peers()
	backend := "concatenation"
	if pq.Proofs.Succinct() {
		backend = "leanvm-sidecar"
	}
	return b.services.RegisterService(pqpool.NewObserver(b.ctx, &pqpool.ObserverConfig{
		Pool:        b.pqAttestationPool,
		Head:        chainService,
		ClockWaiter: b.ClockWaiter,
		Syncing:     initSync.Syncing,
		Peers:       func() int { return len(peers.Connected()) },
		Output:      output,
		Measure:     measure,
		Network:     params.BeaconConfig().ConfigName,
		Version:     version.Version(),
		Backend:     backend,
	}))
}
