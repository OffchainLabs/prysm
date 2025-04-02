package verification

import (
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/spf13/afero"
)

func VerifiedROBlobFromDisk(fs afero.Fs, root [32]byte, path string) (blocks.VerifiedROBlob, error) {
	encoded, err := afero.ReadFile(fs, path)
	if err != nil {
		return VerifiedROBlobError(err)
	}
	s := &ethpb.BlobSidecar{}
	if err := s.UnmarshalSSZ(encoded); err != nil {
		return VerifiedROBlobError(err)
	}
	ro, err := blocks.NewROBlobWithRoot(s, root)
	if err != nil {
		return VerifiedROBlobError(err)
	}
	return blocks.NewVerifiedROBlob(ro), nil
}
