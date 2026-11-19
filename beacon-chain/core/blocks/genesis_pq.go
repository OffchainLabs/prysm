package blocks

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// The bid mirrors the state so the body root matches the genesis block header.
func pqGenesisBlock(root [fieldparams.RootLength]byte, latestBid *ethpb.ExecutionPayloadBid) *ethpb.SignedBeaconBlockPQ {
	bid := latestBid.Copy()
	if bid == nil {
		bid = &ethpb.ExecutionPayloadBid{
			ParentBlockHash:       make([]byte, 32),
			ParentBlockRoot:       make([]byte, 32),
			BlockHash:             make([]byte, 32),
			PrevRandao:            make([]byte, 32),
			FeeRecipient:          make([]byte, 20),
			BlobKzgCommitments:    make([][]byte, 0),
			ExecutionRequestsRoot: make([]byte, 32),
		}
	}
	return &ethpb.SignedBeaconBlockPQ{
		Block: &ethpb.BeaconBlockPQ{
			ParentRoot: params.BeaconConfig().ZeroHash[:],
			StateRoot:  root[:],
			Body: &ethpb.BeaconBlockBodyPQ{
				RandaoReveal: make([]byte, 1024),
				Eth1Data: &ethpb.Eth1Data{
					DepositRoot: make([]byte, 32),
					BlockHash:   make([]byte, 32),
				},
				Graffiti: make([]byte, 32),
				SignedExecutionPayloadBid: &ethpb.SignedExecutionPayloadBidPQ{
					Message:   bid,
					Signature: make([]byte, pq.XMSSSignatureLength),
				},
				ParentExecutionRequests: &enginev1.ExecutionRequestsPQ{},
			},
		},
		Signature: make([]byte, pq.XMSSSignatureLength),
	}
}
