package blocks

import (
	field_params "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/container/trie"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/gohashtree"
)

const (
	bodyLength    = 13 // The number of elements in the BeaconBlockBody Container for Electra
	logBodyLength = 4  // The log 2 of bodyLength
	kzgPosition   = 11 // The index of the KZG commitment list in the Body
	kzgRootIndex  = 54 // The Merkle index of the KZG commitment list's root in the Body's Merkle tree
	KZGOffset     = kzgRootIndex * field_params.MaxBlobCommitmentsPerBlock
)

var (
	errInvalidIndex          = errors.New("index out of bounds")
	errInvalidBodyRoot       = errors.New("invalid Beacon Block Body root")
	errInvalidInclusionProof = errors.New("invalid KZG commitment inclusion proof")
)

// MerkleProofComponents contains pre-computed components for efficient proof generation
type MerkleProofComponents struct {
	kzgSubtree    *trie.SparseMerkleTrie
	topLevelProof [][]byte
}

// VerifyKZGInclusionProof verifies the Merkle proof in a Blob sidecar against
// the beacon block body root.
func VerifyKZGInclusionProof(blob ROBlob) error {
	if blob.SignedBlockHeader == nil {
		return errNilBlockHeader
	}
	if blob.SignedBlockHeader.Header == nil {
		return errNilBlockHeader
	}
	root := blob.SignedBlockHeader.Header.BodyRoot
	if len(root) != field_params.RootLength {
		return errInvalidBodyRoot
	}
	chunks := makeChunk(blob.KzgCommitment)
	gohashtree.HashChunks(chunks, chunks)
	verified := trie.VerifyMerkleProof(root, chunks[0][:], blob.Index+KZGOffset, blob.CommitmentInclusionProof)
	if !verified {
		return errInvalidInclusionProof
	}
	return nil
}

// MerkleProofKZGCommitment constructs a Merkle proof of inclusion of the KZG
// commitment of index `index` into the Beacon Block with the given `body`
func MerkleProofKZGCommitment(body interfaces.ReadOnlyBeaconBlockBody, index int) ([][]byte, error) {
	bodyVersion := body.Version()
	if bodyVersion < version.Deneb {
		return nil, errUnsupportedBeaconBlockBody
	}
	commitments, err := body.BlobKzgCommitments()
	if err != nil {
		return nil, err
	}
	proof, err := bodyProof(commitments, index)
	if err != nil {
		return nil, err
	}
	membersRoots, err := topLevelRoots(body)
	if err != nil {
		return nil, err
	}
	sparse, err := trie.GenerateTrieFromItems(membersRoots, logBodyLength)
	if err != nil {
		return nil, err
	}
	topProof, err := sparse.MerkleProof(kzgPosition)
	if err != nil {
		return nil, err
	}
	// sparse.MerkleProof always includes the length of the slice this is
	// why we remove the last element that is not needed in topProof
	proof = append(proof, topProof[:len(topProof)-1]...)
	return proof, nil
}

// PrecomputeMerkleProofComponents pre-computes the expensive parts of Merkle proof generation
// that are shared across all blob indices for a given block body.
func PrecomputeMerkleProofComponents(body interfaces.ReadOnlyBeaconBlockBody) (*MerkleProofComponents, error) {
	bodyVersion := body.Version()
	if bodyVersion < version.Deneb {
		return nil, errUnsupportedBeaconBlockBody
	}

	// Pre-compute KZG subtree
	commitments, err := body.BlobKzgCommitments()
	if err != nil {
		return nil, err
	}

	// No work needed if there are no commitments
	if len(commitments) == 0 {
		return nil, nil
	}

	leaves := LeavesFromCommitments(commitments)
	kzgSubtree, err := trie.GenerateTrieFromItems(leaves, field_params.LogMaxBlobCommitments)
	if err != nil {
		return nil, err
	}

	// Pre-compute top-level components
	membersRoots, err := topLevelRoots(body)
	if err != nil {
		return nil, err
	}
	topLevelTrie, err := trie.GenerateTrieFromItems(membersRoots, logBodyLength)
	if err != nil {
		return nil, err
	}
	topLevelProof, err := topLevelTrie.MerkleProof(kzgPosition)
	if err != nil {
		return nil, err
	}
	// Remove the last element that is not needed in topProof
	topLevelProof = topLevelProof[:len(topLevelProof)-1]

	return &MerkleProofComponents{
		kzgSubtree:    kzgSubtree,
		topLevelProof: topLevelProof,
	}, nil
}

// MerkleProofKZGCommitmentFromComponents constructs a Merkle proof for a specific index
// using pre-computed components, avoiding redundant calculations.
func MerkleProofKZGCommitmentFromComponents(components *MerkleProofComponents, index int) ([][]byte, error) {
	// Generate index-specific proof from pre-computed KZG subtree
	subtreeProof, err := components.kzgSubtree.MerkleProof(index)
	if err != nil {
		return nil, err
	}

	// Combine with pre-computed top-level proof
	proof := append(subtreeProof, components.topLevelProof...)
	return proof, nil
}

// MerkleProofKZGCommitments constructs a Merkle proof of inclusion of the KZG
// commitments into the Beacon Block with the given `body`
func MerkleProofKZGCommitments(body interfaces.ReadOnlyBeaconBlockBody) ([][]byte, error) {
	bodyVersion := body.Version()
	if bodyVersion < version.Deneb {
		return nil, errUnsupportedBeaconBlockBody
	}

	membersRoots, err := topLevelRoots(body)
	if err != nil {
		return nil, errors.Wrap(err, "top level roots")
	}

	sparse, err := trie.GenerateTrieFromItems(membersRoots, logBodyLength)
	if err != nil {
		return nil, errors.Wrap(err, "generate trie from items")
	}

	proof, err := sparse.MerkleProof(kzgPosition)
	if err != nil {
		return nil, errors.Wrap(err, "merkle proof")
	}

	// Remove the last element as it is a mix in with the number of
	// elements in the trie.
	proof = proof[:len(proof)-1]

	return proof, nil
}

// LeavesFromCommitments hashes each commitment to construct a slice of roots
func LeavesFromCommitments(commitments [][]byte) [][]byte {
	leaves := make([][]byte, len(commitments))
	for i, kzg := range commitments {
		chunk := makeChunk(kzg)
		gohashtree.HashChunks(chunk, chunk)
		leaves[i] = chunk[0][:]
	}
	return leaves
}

// makeChunk constructs a chunk from a KZG commitment.
func makeChunk(commitment []byte) [][32]byte {
	chunk := make([][32]byte, 2)
	copy(chunk[0][:], commitment)
	copy(chunk[1][:], commitment[field_params.RootLength:])
	return chunk
}

// bodyProof returns the Merkle proof of the subtree up to the root of the KZG
// commitment list.
func bodyProof(commitments [][]byte, index int) ([][]byte, error) {
	if index < 0 || index >= len(commitments) {
		return nil, errInvalidIndex
	}
	leaves := LeavesFromCommitments(commitments)
	sparse, err := trie.GenerateTrieFromItems(leaves, field_params.LogMaxBlobCommitments)
	if err != nil {
		return nil, err
	}
	proof, err := sparse.MerkleProof(index)
	if err != nil {
		return nil, err
	}
	return proof, err
}

// topLevelRoots computes the slice with the roots of each element in the
// BeaconBlockBody. Notice that the KZG commitments root is not needed for the
// proof computation thus it's omitted
func topLevelRoots(body interfaces.ReadOnlyBeaconBlockBody) ([][]byte, error) {
	layer := make([][]byte, bodyLength)
	for i := range layer {
		layer[i] = make([]byte, 32)
	}

	// Randao Reveal
	randao := body.RandaoReveal()
	root, err := ssz.MerkleizeByteSliceSSZ(randao[:])
	if err != nil {
		return nil, err
	}
	copy(layer[0], root[:])

	// eth1_data
	eth1 := body.Eth1Data()
	root, err = eth1.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	copy(layer[1], root[:])

	// graffiti
	root = body.Graffiti()
	copy(layer[2], root[:])

	// Proposer slashings
	ps := body.ProposerSlashings()
	root, err = ssz.MerkleizeListSSZ(ps, params.BeaconConfig().MaxProposerSlashings)
	if err != nil {
		return nil, err
	}
	copy(layer[3], root[:])

	// Attester slashings
	as := body.AttesterSlashings()
	bodyVersion := body.Version()
	if bodyVersion < version.Electra {
		root, err = ssz.MerkleizeListSSZ(as, params.BeaconConfig().MaxAttesterSlashings)
	} else {
		root, err = ssz.MerkleizeListSSZ(as, params.BeaconConfig().MaxAttesterSlashingsElectra)
	}
	if err != nil {
		return nil, err
	}
	copy(layer[4], root[:])

	// Attestations
	att := body.Attestations()
	if bodyVersion < version.Electra {
		root, err = ssz.MerkleizeListSSZ(att, params.BeaconConfig().MaxAttestations)
	} else {
		root, err = ssz.MerkleizeListSSZ(att, params.BeaconConfig().MaxAttestationsElectra)
	}
	if err != nil {
		return nil, err
	}
	copy(layer[5], root[:])

	// Deposits
	dep := body.Deposits()
	root, err = ssz.MerkleizeListSSZ(dep, params.BeaconConfig().MaxDeposits)
	if err != nil {
		return nil, err
	}
	copy(layer[6], root[:])

	// Voluntary Exits
	ve := body.VoluntaryExits()
	root, err = ssz.MerkleizeListSSZ(ve, params.BeaconConfig().MaxVoluntaryExits)
	if err != nil {
		return nil, err
	}
	copy(layer[7], root[:])

	// Sync Aggregate
	sa, err := body.SyncAggregate()
	if err != nil {
		return nil, err
	}
	root, err = sa.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	copy(layer[8], root[:])

	// Execution Payload
	ep, err := body.Execution()
	if err != nil {
		return nil, err
	}
	root, err = ep.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	copy(layer[9], root[:])

	// BLS Changes
	bls, err := body.BLSToExecutionChanges()
	if err != nil {
		return nil, err
	}
	root, err = ssz.MerkleizeListSSZ(bls, params.BeaconConfig().MaxBlsToExecutionChanges)
	if err != nil {
		return nil, err
	}
	copy(layer[10], root[:])

	// KZG commitments is not needed

	// Execution requests
	if body.Version() >= version.Electra {
		er, err := body.ExecutionRequests()
		if err != nil {
			return nil, err
		}
		root, err = er.HashTreeRoot()
		if err != nil {
			return nil, err
		}
		copy(layer[12], root[:])
	}
	return layer, nil
}
