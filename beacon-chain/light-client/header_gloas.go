package light_client

import (
	"context"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz/query"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

func makeExecutionAndProofGloas(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock) ([]byte, [][]byte, error) {
	if block.Version() < version.Capella {
		return make([]byte, fieldparams.RootLength), emptyGloasExecutionProof(), nil
	}
	if block.Version() < version.Gloas {
		return makePreGloasExecutionAndProof(ctx, block)
	}

	bid, err := block.Block().Body().ExecutionPayloadBid()
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not get execution payload bid")
	}
	proof, err := blocks.ExecutionBlockHashProof(ctx, block.Block())
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not get execution block hash proof")
	}
	parentBlockHash := bid.ParentBlockHash()
	return parentBlockHash[:], proof, nil
}

func makePreGloasExecutionAndProof(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock) ([]byte, [][]byte, error) {
	var payloadHeader query.SSZObject
	var payloadProof [][]byte
	var err error
	if block.Version() == version.Capella {
		payloadHeader, payloadProof, err = makeExecutionAndProofCapella(ctx, block)
	} else {
		payloadHeader, payloadProof, err = makeExecutionAndProofDeneb(ctx, block)
	}
	if err != nil {
		return nil, nil, err
	}

	info, err := query.AnalyzeObject(payloadHeader)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not analyze execution payload header")
	}
	index, err := query.GetGeneralizedIndexFromPath(info, query.Path{Elements: []query.PathElement{{Name: "block_hash"}}})
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not get execution block hash index")
	}
	proof, err := info.Prove(index)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not prove execution block hash")
	}

	// Normalize with leading zero nodes; the payload proof remains at the end.
	branch := append(proof.Hashes, payloadProof...)
	normalized := emptyGloasExecutionProof()
	if len(branch) > len(normalized) {
		return nil, nil, errors.New("execution block hash proof exceeds Gloas branch depth")
	}
	copy(normalized[len(normalized)-len(branch):], branch)
	return proof.Leaf, normalized, nil
}

func emptyGloasExecutionProof() [][]byte {
	branch := make([][]byte, fieldparams.ExecutionBranchDepthGloas)
	for i := range branch {
		branch[i] = make([]byte, fieldparams.RootLength)
	}
	return branch
}
