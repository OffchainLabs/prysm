package light_client

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/execution"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	light_client "github.com/OffchainLabs/prysm/v6/consensus-types/light-client"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

func createDefaultLightClientBootstrap(currentSlot primitives.Slot) (interfaces.LightClientBootstrap, error) {
	currentEpoch := slots.ToEpoch(currentSlot)
	syncCommitteeSize := params.BeaconConfig().SyncCommitteeSize
	pubKeys := make([][]byte, syncCommitteeSize)
	for i := uint64(0); i < syncCommitteeSize; i++ {
		pubKeys[i] = make([]byte, fieldparams.BLSPubkeyLength)
	}
	currentSyncCommittee := &pb.SyncCommittee{
		Pubkeys:         pubKeys,
		AggregatePubkey: make([]byte, fieldparams.BLSPubkeyLength),
	}

	var currentSyncCommitteeBranch [][]byte
	if currentEpoch >= params.BeaconConfig().ElectraForkEpoch {
		currentSyncCommitteeBranch = make([][]byte, fieldparams.SyncCommitteeBranchDepthElectra)
	} else {
		currentSyncCommitteeBranch = make([][]byte, fieldparams.SyncCommitteeBranchDepth)
	}
	for i := 0; i < len(currentSyncCommitteeBranch); i++ {
		currentSyncCommitteeBranch[i] = make([]byte, fieldparams.RootLength)
	}

	executionBranch := make([][]byte, fieldparams.ExecutionBranchDepth)
	for i := 0; i < fieldparams.ExecutionBranchDepth; i++ {
		executionBranch[i] = make([]byte, 32)
	}

	var m proto.Message
	if currentEpoch < params.BeaconConfig().CapellaForkEpoch {
		m = &pb.LightClientBootstrapAltair{
			Header: &pb.LightClientHeaderAltair{
				Beacon: &pb.BeaconBlockHeader{},
			},
			CurrentSyncCommittee:       currentSyncCommittee,
			CurrentSyncCommitteeBranch: currentSyncCommitteeBranch,
		}
	} else if currentEpoch < params.BeaconConfig().DenebForkEpoch {
		m = &pb.LightClientBootstrapCapella{
			Header: &pb.LightClientHeaderCapella{
				Beacon:          &pb.BeaconBlockHeader{},
				Execution:       &enginev1.ExecutionPayloadHeaderCapella{},
				ExecutionBranch: executionBranch,
			},
			CurrentSyncCommittee:       currentSyncCommittee,
			CurrentSyncCommitteeBranch: currentSyncCommitteeBranch,
		}
	} else if currentEpoch < params.BeaconConfig().ElectraForkEpoch {
		m = &pb.LightClientBootstrapDeneb{
			Header: &pb.LightClientHeaderDeneb{
				Beacon:          &pb.BeaconBlockHeader{},
				Execution:       &enginev1.ExecutionPayloadHeaderDeneb{},
				ExecutionBranch: executionBranch,
			},
			CurrentSyncCommittee:       currentSyncCommittee,
			CurrentSyncCommitteeBranch: currentSyncCommitteeBranch,
		}
	} else {
		m = &pb.LightClientBootstrapElectra{
			Header: &pb.LightClientHeaderDeneb{
				Beacon:          &pb.BeaconBlockHeader{},
				Execution:       &enginev1.ExecutionPayloadHeaderDeneb{},
				ExecutionBranch: executionBranch,
			},
			CurrentSyncCommittee:       currentSyncCommittee,
			CurrentSyncCommitteeBranch: currentSyncCommitteeBranch,
		}
	}

	return light_client.NewWrappedBootstrap(m)
}

func makeExecutionAndProofDeneb(ctx context.Context, blk interfaces.ReadOnlySignedBeaconBlock) (*enginev1.ExecutionPayloadHeaderDeneb, [][]byte, error) {
	if blk.Version() < version.Capella {
		p, err := execution.EmptyExecutionPayloadHeader(version.Deneb)
		if err != nil {
			return nil, nil, errors.Wrap(err, "could not get payload header")
		}
		payloadHeader, ok := p.(*enginev1.ExecutionPayloadHeaderDeneb)
		if !ok {
			return nil, nil, fmt.Errorf("payload header type %T is not %T", p, &enginev1.ExecutionPayloadHeaderDeneb{})
		}
		payloadProof := emptyPayloadProof()

		return payloadHeader, payloadProof, nil
	}

	payload, err := blk.Block().Body().Execution()
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not get execution payload")
	}
	transactionsRoot, err := ComputeTransactionsRoot(payload)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not get transactions root")
	}
	withdrawalsRoot, err := ComputeWithdrawalsRoot(payload)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not get withdrawals root")
	}

	payloadHeader := &enginev1.ExecutionPayloadHeaderDeneb{
		ParentHash:       payload.ParentHash(),
		FeeRecipient:     payload.FeeRecipient(),
		StateRoot:        payload.StateRoot(),
		ReceiptsRoot:     payload.ReceiptsRoot(),
		LogsBloom:        payload.LogsBloom(),
		PrevRandao:       payload.PrevRandao(),
		BlockNumber:      payload.BlockNumber(),
		GasLimit:         payload.GasLimit(),
		GasUsed:          payload.GasUsed(),
		Timestamp:        payload.Timestamp(),
		ExtraData:        payload.ExtraData(),
		BaseFeePerGas:    payload.BaseFeePerGas(),
		BlockHash:        payload.BlockHash(),
		TransactionsRoot: transactionsRoot,
		WithdrawalsRoot:  withdrawalsRoot,
		BlobGasUsed:      0,
		ExcessBlobGas:    0,
	}

	if blk.Version() >= version.Deneb {
		blobGasUsed, err := payload.BlobGasUsed()
		if err != nil {
			return nil, nil, errors.Wrap(err, "could not get blob gas used")
		}
		excessBlobGas, err := payload.ExcessBlobGas()
		if err != nil {
			return nil, nil, errors.Wrap(err, "could not get excess blob gas")
		}

		payloadHeader.BlobGasUsed = blobGasUsed
		payloadHeader.ExcessBlobGas = excessBlobGas
	}

	payloadProof, err := blocks.PayloadProof(ctx, blk.Block())
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not get execution payload proof")
	}

	return payloadHeader, payloadProof, nil
}

func makeExecutionAndProofCapella(ctx context.Context, blk interfaces.ReadOnlySignedBeaconBlock) (*enginev1.ExecutionPayloadHeaderCapella, [][]byte, error) {
	if blk.Version() > version.Capella {
		return nil, nil, fmt.Errorf("unsupported block version %s for capella execution payload", version.String(blk.Version()))
	}
	if blk.Version() < version.Capella {
		p, err := execution.EmptyExecutionPayloadHeader(version.Capella)
		if err != nil {
			return nil, nil, errors.Wrap(err, "could not get payload header")
		}
		payloadHeader, ok := p.(*enginev1.ExecutionPayloadHeaderCapella)
		if !ok {
			return nil, nil, fmt.Errorf("payload header type %T is not %T", p, &enginev1.ExecutionPayloadHeaderCapella{})
		}
		payloadProof := emptyPayloadProof()

		return payloadHeader, payloadProof, nil
	}

	payload, err := blk.Block().Body().Execution()
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not get execution payload")
	}
	transactionsRoot, err := ComputeTransactionsRoot(payload)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not get transactions root")
	}
	withdrawalsRoot, err := ComputeWithdrawalsRoot(payload)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not get withdrawals root")
	}

	payloadHeader := &enginev1.ExecutionPayloadHeaderCapella{
		ParentHash:       payload.ParentHash(),
		FeeRecipient:     payload.FeeRecipient(),
		StateRoot:        payload.StateRoot(),
		ReceiptsRoot:     payload.ReceiptsRoot(),
		LogsBloom:        payload.LogsBloom(),
		PrevRandao:       payload.PrevRandao(),
		BlockNumber:      payload.BlockNumber(),
		GasLimit:         payload.GasLimit(),
		GasUsed:          payload.GasUsed(),
		Timestamp:        payload.Timestamp(),
		ExtraData:        payload.ExtraData(),
		BaseFeePerGas:    payload.BaseFeePerGas(),
		BlockHash:        payload.BlockHash(),
		TransactionsRoot: transactionsRoot,
		WithdrawalsRoot:  withdrawalsRoot,
	}

	payloadProof, err := blocks.PayloadProof(ctx, blk.Block())
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not get execution payload proof")
	}

	return payloadHeader, payloadProof, nil
}

func makeBeaconBlockHeader(blk interfaces.ReadOnlySignedBeaconBlock) (*pb.BeaconBlockHeader, error) {
	parentRoot := blk.Block().ParentRoot()
	stateRoot := blk.Block().StateRoot()
	bodyRoot, err := blk.Block().Body().HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not get body root")
	}
	return &pb.BeaconBlockHeader{
		Slot:          blk.Block().Slot(),
		ProposerIndex: blk.Block().ProposerIndex(),
		ParentRoot:    parentRoot[:],
		StateRoot:     stateRoot[:],
		BodyRoot:      bodyRoot[:],
	}, nil
}

func emptyPayloadProof() [][]byte {
	branch := interfaces.LightClientExecutionBranch{}
	proof := make([][]byte, len(branch))
	for i, b := range branch {
		proof[i] = b[:]
	}
	return proof
}
