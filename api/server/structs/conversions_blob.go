package structs

import (
	"strconv"

	"github.com/prysmaticlabs/prysm/v5/api/server"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

func (sc *Sidecar) ToConsensus() (*eth.BlobSidecar, error) {
	if sc == nil {
		return nil, errNilValue
	}

	index, err := strconv.ParseUint(sc.Index, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Index")
	}

	blob, err := bytesutil.DecodeHexWithLength(sc.Blob, 131072)
	if err != nil {
		return nil, server.NewDecodeError(err, "Blob")
	}

	kzgCommitment, err := bytesutil.DecodeHexWithLength(sc.KzgCommitment, 48)
	if err != nil {
		return nil, server.NewDecodeError(err, "KzgCommitment")
	}

	kzgProof, err := bytesutil.DecodeHexWithLength(sc.KzgProof, 48)
	if err != nil {
		return nil, server.NewDecodeError(err, "KzgProof")
	}

	header, err := sc.SignedBeaconBlockHeader.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "SignedBeaconBlockHeader")
	}

	// decode the commitment inclusion proof
	var commitmentInclusionProof [][]byte
	for _, proof := range sc.CommitmentInclusionProof {
		proofBytes, err := bytesutil.DecodeHexWithLength(proof, 32)
		if err != nil {
			return nil, server.NewDecodeError(err, "CommitmentInclusionProof")
		}
		commitmentInclusionProof = append(commitmentInclusionProof, proofBytes)
	}

	bsc := &eth.BlobSidecar{
		Index:                    index,
		Blob:                     blob,
		KzgCommitment:            kzgCommitment,
		KzgProof:                 kzgProof,
		SignedBlockHeader:        header,
		CommitmentInclusionProof: commitmentInclusionProof,
	}

	return bsc, nil
}
