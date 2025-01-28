package client

import (
	"bytes"
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/encoder"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync/rlnc"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	validatorpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1/validator-client"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var numChunks = uint(10)

// rlncBlockSuffix is a byte that is added to the end of the block to mark it's end.
var rlncBlockSuffix = byte(0xfe)

func (v *validator) createSignedChunks(
	ctx context.Context,
	pubKey [fieldparams.BLSPubkeyLength]byte,
	epoch primitives.Epoch,
	slot primitives.Slot,
	b interfaces.ReadOnlySignedBeaconBlock,
) ([]byte, [32]byte, *rlnc.Node, error) {
	ctx, span := trace.StartSpan(ctx, "validator.createSignedChunks")
	defer span.End()

	domain, err := v.domainData(ctx, epoch, params.BeaconConfig().DomainBeaconProposer[:])
	if err != nil {
		return nil, [32]byte{}, nil, errors.Wrap(err, domainDataErr)
	}
	if domain == nil {
		return nil, [32]byte{}, nil, errors.New(domainDataErr)
	}
	e := &encoder.SszNetworkEncoder{}
	buf := new(bytes.Buffer)
	if _, err := e.EncodeGossip(buf, b); err != nil {
		logrus.WithError(err).Error("Could not encode block data")
		return nil, [32]byte{}, nil, errors.Wrap(err, domainDataErr)
	}
	buf.WriteByte(rlncBlockSuffix)

	node, err := rlnc.NewSource(v.committer, numChunks, buf.Bytes())
	if err != nil {
		return nil, [32]byte{}, nil, errors.Wrap(err, "could not create source node")
	}
	node.SetProposerIndex(b.Block().ProposerIndex())
	node.SetSlot(slot)
	parentRoot := b.Block().ParentRoot()
	node.SetParentRoot(parentRoot[:])
	header := &ethpb.BeaconBlockChunkHeader{
		Slot:          slot,
		ProposerIndex: b.Block().ProposerIndex(),
		ParentRoot:    parentRoot[:],
		Commitments:   node.Commitments(),
	}
	signingRoot, err := signing.ComputeSigningRoot(header, domain.SignatureDomain)
	if err != nil {
		return nil, [32]byte{}, nil, errors.Wrap(err, signingRootErr)
	}
	req := &validatorpb.SignRequest_BeaconBlockChunkHeader{
		BeaconBlockChunkHeader: header,
	}
	sig, err := v.km.Sign(ctx, &validatorpb.SignRequest{
		PublicKey:       pubKey[:],
		SigningRoot:     signingRoot[:],
		SignatureDomain: domain.SignatureDomain,
		Object:          req,
		SigningSlot:     slot,
	})
	if err != nil {
		return nil, [32]byte{}, nil, errors.Wrap(err, "could not sign block proposal")
	}
	node.SetSignature(sig.Marshal())
	return sig.Marshal(), signingRoot, node, nil
}
