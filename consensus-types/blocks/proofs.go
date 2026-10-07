package blocks

import (
	"context"
	"encoding/binary"
	"fmt"

	methodicalssz "github.com/OffchainLabs/methodical-ssz/ssz"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/crypto/hash/htr"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

const (
	payloadFieldIndex                   = 9
	signedExecutionPayloadBidFieldIndex = 10
	parentBlockHashFieldIndex           = 0
	executionPayloadBidFieldCount       = 12
	beaconBlockBodyFieldCount           = 13
)

type progressiveContainerType uint8

const (
	executionPayloadBidProgressiveContainer progressiveContainerType = iota
	beaconBlockBodyProgressiveContainer
)

func ComputeBlockBodyFieldRoots(ctx context.Context, blockBody *BeaconBlockBody) ([][]byte, error) {
	_, span := trace.StartSpan(ctx, "blocks.ComputeBlockBodyFieldRoots")
	defer span.End()

	if blockBody == nil {
		return nil, errNilBlockBody
	}

	var fieldRoots [][]byte
	switch blockBody.version {
	case version.Phase0:
		fieldRoots = make([][]byte, 8)
	case version.Altair:
		fieldRoots = make([][]byte, 9)
	case version.Bellatrix:
		fieldRoots = make([][]byte, 10)
	case version.Capella:
		fieldRoots = make([][]byte, 11)
	case version.Deneb:
		fieldRoots = make([][]byte, 12)
	case version.Electra:
		fieldRoots = make([][]byte, 13)
	case version.Fulu:
		fieldRoots = make([][]byte, 13)
	case version.Gloas:
		fieldRoots = make([][]byte, 13)
	default:
		return nil, fmt.Errorf("unknown block body version %s", version.String(blockBody.version))
	}

	for i := range fieldRoots {
		fieldRoots[i] = make([]byte, 32)
	}

	// Randao Reveal
	randao := blockBody.RandaoReveal()
	root, err := ssz.MerkleizeByteSliceSSZ(randao[:])
	if err != nil {
		return nil, err
	}
	copy(fieldRoots[0], root[:])

	// eth1_data
	eth1 := blockBody.Eth1Data()
	root, err = eth1.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	copy(fieldRoots[1], root[:])

	// graffiti
	root = blockBody.Graffiti()
	copy(fieldRoots[2], root[:])

	// Proposer slashings
	ps := blockBody.ProposerSlashings()
	root, err = blockBodyListRoot(blockBody.version, ps, params.BeaconConfig().MaxProposerSlashings)
	if err != nil {
		return nil, err
	}
	copy(fieldRoots[3], root[:])

	// Attester slashings
	as := blockBody.AttesterSlashings()
	bodyVersion := blockBody.Version()
	if bodyVersion < version.Electra {
		root, err = blockBodyListRoot(bodyVersion, as, params.BeaconConfig().MaxAttesterSlashings)
	} else {
		root, err = blockBodyListRoot(bodyVersion, as, params.BeaconConfig().MaxAttesterSlashingsElectra)
	}
	if err != nil {
		return nil, err
	}
	copy(fieldRoots[4], root[:])

	// Attestations
	att := blockBody.Attestations()
	if bodyVersion < version.Electra {
		root, err = blockBodyListRoot(bodyVersion, att, params.BeaconConfig().MaxAttestations)
	} else {
		root, err = blockBodyListRoot(bodyVersion, att, params.BeaconConfig().MaxAttestationsElectra)
	}
	if err != nil {
		return nil, err
	}
	copy(fieldRoots[5], root[:])

	// Deposits
	dep := blockBody.Deposits()
	root, err = blockBodyListRoot(blockBody.version, dep, params.BeaconConfig().MaxDeposits)
	if err != nil {
		return nil, err
	}
	copy(fieldRoots[6], root[:])

	// Voluntary Exits
	ve := blockBody.VoluntaryExits()
	root, err = blockBodyListRoot(blockBody.version, ve, params.BeaconConfig().MaxVoluntaryExits)
	if err != nil {
		return nil, err
	}
	copy(fieldRoots[7], root[:])

	if blockBody.version >= version.Altair {
		// Sync Aggregate
		sa, err := blockBody.SyncAggregate()
		if err != nil {
			return nil, err
		}
		root, err = sa.HashTreeRoot()
		if err != nil {
			return nil, err
		}
		copy(fieldRoots[8], root[:])
	}

	if blockBody.version >= version.Bellatrix && blockBody.version < version.Gloas {
		// Execution Payload
		ep, err := blockBody.Execution()
		if err != nil {
			return nil, err
		}
		root, err = ep.HashTreeRoot()
		if err != nil {
			return nil, err
		}
		copy(fieldRoots[9], root[:])
	}

	if blockBody.version >= version.Capella && blockBody.version < version.Gloas {
		// BLS Changes
		bls, err := blockBody.BLSToExecutionChanges()
		if err != nil {
			return nil, err
		}
		root, err = blockBodyListRoot(blockBody.version, bls, params.BeaconConfig().MaxBlsToExecutionChanges)
		if err != nil {
			return nil, err
		}
		copy(fieldRoots[10], root[:])
	}

	if blockBody.version >= version.Deneb && blockBody.version < version.Gloas {
		// KZG commitments
		roots := make([][32]byte, len(blockBody.blobKzgCommitments))
		for i, commitment := range blockBody.blobKzgCommitments {
			chunks, err := ssz.PackByChunk([][]byte{commitment})
			if err != nil {
				return nil, err
			}
			roots[i] = htr.VectorizedSha256(chunks)[0]
		}
		commitmentsRoot, err := ssz.BitwiseMerkleize(roots, uint64(len(roots)), 4096)
		if err != nil {
			return nil, err
		}
		length := make([]byte, 32)
		binary.LittleEndian.PutUint64(length[:8], uint64(len(roots)))
		root = ssz.MixInLength(commitmentsRoot, length)
		copy(fieldRoots[11], root[:])
	}

	if blockBody.version >= version.Electra && blockBody.version < version.Gloas {
		// Execution Requests
		er, err := blockBody.ExecutionRequests()
		if err != nil {
			return nil, err
		}
		root, err := er.HashTreeRoot()
		if err != nil {
			return nil, err
		}
		copy(fieldRoots[12], root[:])
	}
	if blockBody.version >= version.Gloas {
		if err := computeGloasBlockBodyFieldRoots(blockBody, fieldRoots); err != nil {
			return nil, err
		}
	}
	return fieldRoots, nil
}

func blockBodyListRoot[T ssz.Hashable](bodyVersion int, elements []T, limit uint64) ([32]byte, error) {
	if bodyVersion >= version.Gloas {
		if uint64(len(elements)) > limit {
			return [32]byte{}, fmt.Errorf("slice exceeds max length %d", limit)
		}
		return ssz.MerkleizeListSSZProgressive(elements)
	}
	return ssz.MerkleizeListSSZ(elements, limit)
}

func computeGloasBlockBodyFieldRoots(blockBody *BeaconBlockBody, fieldRoots [][]byte) error {
	bls, err := blockBody.BLSToExecutionChanges()
	if err != nil {
		return err
	}
	root, err := blockBodyListRoot(blockBody.version, bls, params.BeaconConfig().MaxBlsToExecutionChanges)
	if err != nil {
		return err
	}
	copy(fieldRoots[9], root[:])

	bid, err := blockBody.SignedExecutionPayloadBid()
	if err != nil {
		return err
	}
	root, err = bid.HashTreeRoot()
	if err != nil {
		return err
	}
	copy(fieldRoots[10], root[:])

	payloadAttestations, err := blockBody.PayloadAttestations()
	if err != nil {
		return err
	}
	root, err = blockBodyListRoot(blockBody.version, payloadAttestations, fieldparams.MaxPayloadAttestations)
	if err != nil {
		return err
	}
	copy(fieldRoots[11], root[:])

	parentExecutionRequests, err := blockBody.ParentExecutionRequests()
	if err != nil {
		return err
	}
	root, err = parentExecutionRequests.HashTreeRoot()
	if err != nil {
		return err
	}
	copy(fieldRoots[12], root[:])

	return nil
}

func PayloadProof(ctx context.Context, block interfaces.ReadOnlyBeaconBlock) ([][]byte, error) {
	if block.Version() >= version.Gloas {
		return nil, errors.New("payload proof is not supported post Gloas; use ExecutionBlockHashProof")
	}

	i := block.Body()
	blockBody, ok := i.(*BeaconBlockBody)
	if !ok {
		return nil, errors.New("failed to cast block body")
	}

	fieldRoots, err := ComputeBlockBodyFieldRoots(ctx, blockBody)
	if err != nil {
		return nil, err
	}

	fieldRootsTrie := stateutil.Merkleize(fieldRoots)
	proof := trie.ProofFromMerkleLayers(fieldRootsTrie, payloadFieldIndex)

	return proof, nil
}

// ExecutionBlockHashProof returns the Progressive Merkle branch for
// BeaconBlockBody.signed_execution_payload_bid.message.parent_block_hash.
func ExecutionBlockHashProof(ctx context.Context, block interfaces.ReadOnlyBeaconBlock) ([][]byte, error) {
	if block.Version() < version.Gloas {
		return nil, fmt.Errorf("execution block hash proof is not supported for %s", version.String(block.Version()))
	}

	i := block.Body()
	blockBody, ok := i.(*BeaconBlockBody)
	if !ok {
		return nil, errors.New("failed to cast block body")
	}
	signedBid, err := blockBody.SignedExecutionPayloadBid()
	if err != nil {
		return nil, err
	}
	if signedBid == nil || signedBid.Message == nil {
		return nil, errors.New("signed execution payload bid is nil")
	}

	bidFieldRoots, err := executionPayloadBidFieldRoots(signedBid.Message)
	if err != nil {
		return nil, err
	}
	proof, err := progressiveContainerFieldProof(
		bidFieldRoots,
		parentBlockHashFieldIndex,
		block.Version(),
		executionPayloadBidProgressiveContainer,
	)
	if err != nil {
		return nil, err
	}

	signatureRoot, err := fixedByteVectorRoot(signedBid.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, fmt.Errorf("signature: %w", err)
	}
	proof = append(proof, signatureRoot[:])

	bodyFieldRoots, err := ComputeBlockBodyFieldRoots(ctx, blockBody)
	if err != nil {
		return nil, err
	}
	blockBodyProof, err := progressiveContainerFieldProof(
		bodyFieldRoots,
		signedExecutionPayloadBidFieldIndex,
		block.Version(),
		beaconBlockBodyProgressiveContainer,
	)
	if err != nil {
		return nil, err
	}
	return append(proof, blockBodyProof...), nil
}

func executionPayloadBidFieldRoots(bid *eth.ExecutionPayloadBid) ([][]byte, error) {
	fieldRoots := make([][]byte, executionPayloadBidFieldCount)
	fixedFields := []struct {
		index  int
		value  []byte
		length int
	}{
		{index: 0, value: bid.ParentBlockHash, length: fieldparams.RootLength},
		{index: 1, value: bid.ParentBlockRoot, length: fieldparams.RootLength},
		{index: 2, value: bid.BlockHash, length: fieldparams.RootLength},
		{index: 3, value: bid.PrevRandao, length: fieldparams.RootLength},
		{index: 4, value: bid.FeeRecipient, length: fieldparams.FeeRecipientLength},
		{index: 11, value: bid.ExecutionRequestsRoot, length: fieldparams.RootLength},
	}
	for _, field := range fixedFields {
		root, err := fixedByteVectorRoot(field.value, field.length)
		if err != nil {
			return nil, err
		}
		fieldRoots[field.index] = root[:]
	}

	for index, value := range []uint64{
		bid.GasLimit,
		uint64(bid.BuilderIndex),
		uint64(bid.Slot),
		uint64(bid.Value),
		uint64(bid.ExecutionPayment),
	} {
		root := ssz.Uint64Root(value)
		fieldRoots[index+5] = root[:]
	}

	commitmentsRoot, err := blobKzgCommitmentsProgressiveRoot(bid.BlobKzgCommitments)
	if err != nil {
		return nil, err
	}
	fieldRoots[10] = commitmentsRoot[:]
	return fieldRoots, nil
}

func blobKzgCommitmentsProgressiveRoot(commitments [][]byte) ([32]byte, error) {
	if uint64(len(commitments)) > params.BeaconConfig().MaxBlobCommitmentsPerBlock {
		return [32]byte{}, fmt.Errorf("slice exceeds max length %d", params.BeaconConfig().MaxBlobCommitmentsPerBlock)
	}

	roots := make([][32]byte, len(commitments))
	for i, commitment := range commitments {
		root, err := fixedByteVectorRoot(commitment, fieldparams.BLSPubkeyLength)
		if err != nil {
			return [32]byte{}, err
		}
		roots[i] = root
	}

	body := ssz.MerkleizeProgressiveChunks(roots)
	var length [32]byte
	binary.LittleEndian.PutUint64(length[:8], uint64(len(commitments)))
	return ssz.MixInLength(body, length[:]), nil
}

func progressiveContainerFieldProof(
	fieldRoots [][]byte,
	fieldIndex int,
	v int,
	container progressiveContainerType,
) ([][]byte, error) {
	branch, err := stateutil.MerkleizeProgressive(fieldRoots).Proof(fieldIndex)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate progressive merkle proof")
	}
	activeFields, err := progressiveContainerActiveFields(v, container)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't get active fields bitlist for progressive container")
	}
	activeFieldsRoot, err := ssz.PackActiveFields(activeFields)
	if err != nil {
		return nil, err
	}
	branch = append(branch, activeFieldsRoot)

	proof := make([][]byte, len(branch))
	for i := range branch {
		proof[i] = branch[i][:]
	}
	return proof, nil
}

func progressiveContainerActiveFields(v int, container progressiveContainerType) ([]bool, error) {
	if v < version.Gloas {
		return nil, fmt.Errorf("progressive container active fields are not defined for %s", version.String(v))
	}

	switch container {
	case executionPayloadBidProgressiveContainer:
		af := make([]bool, executionPayloadBidFieldCount)
		for i := range af {
			af[i] = true
		}
		return af, nil
	case beaconBlockBodyProgressiveContainer:
		af := make([]bool, beaconBlockBodyFieldCount)
		for i := range af {
			af[i] = true
		}
		return af, nil
	default:
		return nil, fmt.Errorf("unknown progressive container type %d", container)
	}
}

func fixedByteVectorRoot(value []byte, length int) ([32]byte, error) {
	if len(value) != length {
		return [32]byte{}, methodicalssz.ErrBytesLength
	}
	return ssz.MerkleizeByteSliceSSZ(value)
}
