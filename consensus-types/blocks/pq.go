package blocks

import (
	"bytes"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

func initSignedBlockFromProtoPQ(pb *eth.SignedBeaconBlockPQ) (*SignedBeaconBlock, error) {
	if pb == nil {
		return nil, errNilBlock
	}
	block, err := initBlockFromProtoPQ(pb.Block)
	if err != nil {
		return nil, fmt.Errorf("initialize PQ block: %w", err)
	}
	return &SignedBeaconBlock{
		version:       version.PQ,
		block:         block,
		xmssSignature: bytes.Clone(pb.Signature),
	}, nil
}

func initBlockFromProtoPQ(pb *eth.BeaconBlockPQ) (*BeaconBlock, error) {
	if pb == nil {
		return nil, errNilBlock
	}
	body, err := initBlockBodyFromProtoPQ(pb.Body)
	if err != nil {
		return nil, fmt.Errorf("initialize PQ block body: %w", err)
	}
	return &BeaconBlock{
		version:       version.PQ,
		slot:          pb.Slot,
		proposerIndex: pb.ProposerIndex,
		parentRoot:    bytesutil.ToBytes32(pb.ParentRoot),
		stateRoot:     bytesutil.ToBytes32(pb.StateRoot),
		body:          body,
	}, nil
}

func initBlockBodyFromProtoPQ(pb *eth.BeaconBlockBodyPQ) (*BeaconBlockBody, error) {
	if pb == nil {
		return nil, errNilBlockBody
	}
	per := pb.ParentExecutionRequests
	if per == nil {
		per = &enginev1.ExecutionRequestsPQ{}
	}
	return &BeaconBlockBody{
		version:                     version.PQ,
		vrfProof:                    pb.RandaoReveal,
		eth1Data:                    pb.Eth1Data,
		graffiti:                    bytesutil.ToBytes32(pb.Graffiti),
		proposerSlashingsPQ:         pb.ProposerSlashings,
		attesterSlashingsPQ:         pb.AttesterSlashings,
		attestationsPQ:              pb.Attestations,
		deposits:                    pb.Deposits,
		signedExecutionPayloadBidPQ: pb.SignedExecutionPayloadBid,
		parentExecutionRequestsPQ:   per,
		attestationProof:            pb.Proof,
	}, nil
}

func (b *SignedBeaconBlock) XMSSSignature() []byte {
	return b.xmssSignature
}

func (b *BeaconBlockBody) VRFProof() []byte {
	return b.vrfProof
}

func (b *BeaconBlockBody) AttestationProof() []byte {
	return b.attestationProof
}

func (b *BeaconBlockBody) ProposerSlashingsPQ() []*eth.ProposerSlashingPQ {
	return b.proposerSlashingsPQ
}

func (b *BeaconBlockBody) ParentExecutionRequestsPQ() *enginev1.ExecutionRequestsPQ {
	return b.parentExecutionRequestsPQ
}

func (b *BeaconBlockBody) XMSSBidSignature() []byte {
	return b.signedExecutionPayloadBidPQ.GetSignature()
}
