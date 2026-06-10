package blocks

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/crypto/hash/htr"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

const (
	payloadFieldIndex = 9
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
	if progressiveSSZEnabledForVersion(bodyVersion) {
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
	root, err = hashTreeRootForVersion(blockBody.version, bid)
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
	root, err = hashTreeRootForVersion(blockBody.version, parentExecutionRequests)
	if err != nil {
		return err
	}
	copy(fieldRoots[12], root[:])

	return nil
}

func PayloadProof(ctx context.Context, block interfaces.ReadOnlyBeaconBlock) ([][]byte, error) {
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
