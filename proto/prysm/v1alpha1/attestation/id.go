package attestation

import (
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/crypto/hash"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// IdSource represents the part of attestation that will be used to generate the Id.
type IdSource uint8

const (
	// Full generates the Id from the whole attestation.
	Full IdSource = iota
	// Data generates the Id from the tuple (slot, committee index, beacon block root, source, target).
	Data
)

// Id represents an attestation ID. Its uniqueness depends on the IdSource provided when constructing the Id.
type Id [32]byte

// NewId --
func NewId(att ethpb.Att, source IdSource) (Id, error) {
	if att.IsNil() {
		return Id{}, errors.New("nil attestation")
	}
	if att.Version() < 0 || att.Version() > 255 {
		return Id{}, errors.New("attestation version must be between 0 and 255")
	}

	var id Id
	id[0] = byte(att.Version())

	switch source {
	case Full:
		h, err := att.HashTreeRoot()
		if err != nil {
			return Id{}, err
		}
		copy(id[1:], h[1:])
		return id, nil
	case Data:
		dataHash, err := att.GetData().HashTreeRoot()
		if err != nil {
			return Id{}, err
		}
		h := dataHash
		if att.Version() >= version.Electra {
			committeeIndices := att.CommitteeBitsVal().BitIndices()
			if len(committeeIndices) == 0 {
				return Id{}, errors.New("no committee bits are set")
			}
			stringCommitteeIndices := make([]string, len(committeeIndices))
			for i, ix := range committeeIndices {
				stringCommitteeIndices[i] = strconv.Itoa(ix)
			}
			h = hash.Hash(append(dataHash[:], []byte(strings.Join(stringCommitteeIndices, ","))...))
		}
		copy(id[1:], h[1:])
		return id, nil
	default:
		return Id{}, errors.New("invalid source requested")
	}
}

// String --
func (id Id) String() string {
	return string(id[:])
}
