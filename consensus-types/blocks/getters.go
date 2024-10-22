package blocks

import (
	"fmt"

	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
	field_params "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	consensus_types "github.com/prysmaticlabs/prysm/v5/consensus-types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	validatorpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/validator-client"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

var (
	// ErrAlreadyUnblinded is returned when trying to unblind a full block.
	ErrAlreadyUnblinded = errors.New("cannot unblind if a full block")
)

// BeaconBlockIsNil checks if any composite field of input signed beacon block is nil.
// Access to these nil fields will result in run time panic,
// it is recommended to run these checks as first line of defense.
func BeaconBlockIsNil(b interfaces.ReadOnlySignedBeaconBlock) error {
	if b == nil || b.IsNil() {
		return ErrNilSignedBeaconBlock
	}
	return nil
}

// Signature returns the respective block signature.
func (b *SignedBeaconBlock) Signature() [field_params.BLSSignatureLength]byte {
	return b.signature
}

// Block returns the underlying beacon block object.
func (b *SignedBeaconBlock) Block() interfaces.ReadOnlyBeaconBlock {
	return b.block
}

// IsNil checks if the underlying beacon block is nil.
func (b *SignedBeaconBlock) IsNil() bool {
	return b == nil || b.block.IsNil()
}

// Copy performs a deep copy of the signed beacon block object.
func (b *SignedBeaconBlock) Copy() (interfaces.SignedBeaconBlock, error) {
	if b == nil {
		return nil, nil
	}

	pb, err := b.Proto()
	if err != nil {
		return nil, err
	}
	switch b.version {
	case version.Phase0:
		return initSignedBlockFromProtoPhase0(pb.(*eth.SignedBeaconBlock).Copy())
	case version.Altair:
		return initSignedBlockFromProtoAltair(pb.(*eth.SignedBeaconBlockAltair).Copy())
	case version.Bellatrix:
		if b.IsBlinded() {
			return initBlindedSignedBlockFromProtoBellatrix(pb.(*eth.SignedBlindedBeaconBlockBellatrix).Copy())
		}
		return initSignedBlockFromProtoBellatrix(pb.(*eth.SignedBeaconBlockBellatrix).Copy())
	case version.Capella:
		if b.IsBlinded() {
			return initBlindedSignedBlockFromProtoCapella(pb.(*eth.SignedBlindedBeaconBlockCapella).Copy())
		}
		return initSignedBlockFromProtoCapella(pb.(*eth.SignedBeaconBlockCapella).Copy())
	case version.Deneb:
		if b.IsBlinded() {
			return initBlindedSignedBlockFromProtoDeneb(pb.(*eth.SignedBlindedBeaconBlockDeneb).Copy())
		}
		return initSignedBlockFromProtoDeneb(pb.(*eth.SignedBeaconBlockDeneb).Copy())
	case version.Electra:
		if b.IsBlinded() {
			return initBlindedSignedBlockFromProtoElectra(pb.(*eth.SignedBlindedBeaconBlockElectra).Copy())
		}
		return initSignedBlockFromProtoElectra(pb.(*eth.SignedBeaconBlockElectra).Copy())
	case version.EPBS:
		cp := eth.CopySignedBeaconBlockEPBS(pb.(*eth.SignedBeaconBlockEpbs))
		return initSignedBlockFromProtoEPBS(cp)
	default:
		return nil, errIncorrectBlockVersion
	}
}

// PbGenericBlock returns a generic signed beacon block.
func (b *SignedBeaconBlock) PbGenericBlock() (*eth.GenericSignedBeaconBlock, error) {
	pb, err := b.Proto()
	if err != nil {
		return nil, err
	}
	switch b.version {
	case version.Phase0:
		return &eth.GenericSignedBeaconBlock{
			Block: &eth.GenericSignedBeaconBlock_Phase0{Phase0: pb.(*eth.SignedBeaconBlock)},
		}, nil
	case version.Altair:
		return &eth.GenericSignedBeaconBlock{
			Block: &eth.GenericSignedBeaconBlock_Altair{Altair: pb.(*eth.SignedBeaconBlockAltair)},
		}, nil
	case version.Bellatrix:
		if b.IsBlinded() {
			return &eth.GenericSignedBeaconBlock{
				Block: &eth.GenericSignedBeaconBlock_BlindedBellatrix{BlindedBellatrix: pb.(*eth.SignedBlindedBeaconBlockBellatrix)},
			}, nil
		}
		return &eth.GenericSignedBeaconBlock{
			Block: &eth.GenericSignedBeaconBlock_Bellatrix{Bellatrix: pb.(*eth.SignedBeaconBlockBellatrix)},
		}, nil
	case version.Capella:
		if b.IsBlinded() {
			return &eth.GenericSignedBeaconBlock{
				Block: &eth.GenericSignedBeaconBlock_BlindedCapella{BlindedCapella: pb.(*eth.SignedBlindedBeaconBlockCapella)},
			}, nil
		}
		return &eth.GenericSignedBeaconBlock{
			Block: &eth.GenericSignedBeaconBlock_Capella{Capella: pb.(*eth.SignedBeaconBlockCapella)},
		}, nil
	case version.Deneb:
		if b.IsBlinded() {
			return &eth.GenericSignedBeaconBlock{
				Block: &eth.GenericSignedBeaconBlock_BlindedDeneb{BlindedDeneb: pb.(*eth.SignedBlindedBeaconBlockDeneb)},
			}, nil
		}
		return &eth.GenericSignedBeaconBlock{
			Block: &eth.GenericSignedBeaconBlock_Deneb{Deneb: pb.(*eth.SignedBeaconBlockContentsDeneb)},
		}, nil
	case version.Electra:
		if b.IsBlinded() {
			return &eth.GenericSignedBeaconBlock{
				Block: &eth.GenericSignedBeaconBlock_BlindedElectra{BlindedElectra: pb.(*eth.SignedBlindedBeaconBlockElectra)},
			}, nil
		}
		return &eth.GenericSignedBeaconBlock{
			Block: &eth.GenericSignedBeaconBlock_Electra{Electra: pb.(*eth.SignedBeaconBlockContentsElectra)},
		}, nil
	case version.EPBS:
		return &eth.GenericSignedBeaconBlock{
			Block: &eth.GenericSignedBeaconBlock_Epbs{Epbs: pb.(*eth.SignedBeaconBlockEpbs)},
		}, nil
	default:
		return nil, errIncorrectBlockVersion
	}
}

// ToBlinded converts a non-blinded block to its blinded equivalent.
func (b *SignedBeaconBlock) ToBlinded() (interfaces.ReadOnlySignedBeaconBlock, error) {
	if b.version < version.Bellatrix || b.version >= version.EPBS {
		return nil, ErrUnsupportedVersion
	}
	if b.IsBlinded() {
		return b, nil
	}
	if b.block.IsNil() {
		return nil, errors.New("cannot convert nil block to blinded format")
	}
	payload, err := b.block.Body().Execution()
	if err != nil {
		return nil, err
	}

	if b.version >= version.Electra {
		p, ok := payload.Proto().(*enginev1.ExecutionPayloadElectra)
		if !ok {
			return nil, fmt.Errorf("%T is not an execution payload header of Deneb version", p)
		}
		header, err := PayloadToHeaderElectra(payload)
		if err != nil {
			return nil, err
		}
		return initBlindedSignedBlockFromProtoElectra(
			&eth.SignedBlindedBeaconBlockElectra{
				Message: &eth.BlindedBeaconBlockElectra{
					Slot:          b.block.slot,
					ProposerIndex: b.block.proposerIndex,
					ParentRoot:    b.block.parentRoot[:],
					StateRoot:     b.block.stateRoot[:],
					Body: &eth.BlindedBeaconBlockBodyElectra{
						RandaoReveal:           b.block.body.randaoReveal[:],
						Eth1Data:               b.block.body.eth1Data,
						Graffiti:               b.block.body.graffiti[:],
						ProposerSlashings:      b.block.body.proposerSlashings,
						AttesterSlashings:      b.block.body.attesterSlashingsElectra,
						Attestations:           b.block.body.attestationsElectra,
						Deposits:               b.block.body.deposits,
						VoluntaryExits:         b.block.body.voluntaryExits,
						SyncAggregate:          b.block.body.syncAggregate,
						ExecutionPayloadHeader: header,
						BlsToExecutionChanges:  b.block.body.blsToExecutionChanges,
						BlobKzgCommitments:     b.block.body.blobKzgCommitments,
						ExecutionRequests:      b.block.body.executionRequests,
					},
				},
				Signature: b.signature[:],
			})
	}

	switch p := payload.Proto().(type) {
	case *enginev1.ExecutionPayload:
		header, err := PayloadToHeader(payload)
		if err != nil {
			return nil, err
		}
		return initBlindedSignedBlockFromProtoBellatrix(
			&eth.SignedBlindedBeaconBlockBellatrix{
				Block: &eth.BlindedBeaconBlockBellatrix{
					Slot:          b.block.slot,
					ProposerIndex: b.block.proposerIndex,
					ParentRoot:    b.block.parentRoot[:],
					StateRoot:     b.block.stateRoot[:],
					Body: &eth.BlindedBeaconBlockBodyBellatrix{
						RandaoReveal:           b.block.body.randaoReveal[:],
						Eth1Data:               b.block.body.eth1Data,
						Graffiti:               b.block.body.graffiti[:],
						ProposerSlashings:      b.block.body.proposerSlashings,
						AttesterSlashings:      b.block.body.attesterSlashings,
						Attestations:           b.block.body.attestations,
						Deposits:               b.block.body.deposits,
						VoluntaryExits:         b.block.body.voluntaryExits,
						SyncAggregate:          b.block.body.syncAggregate,
						ExecutionPayloadHeader: header,
					},
				},
				Signature: b.signature[:],
			})
	case *enginev1.ExecutionPayloadCapella:
		header, err := PayloadToHeaderCapella(payload)
		if err != nil {
			return nil, err
		}
		return initBlindedSignedBlockFromProtoCapella(
			&eth.SignedBlindedBeaconBlockCapella{
				Block: &eth.BlindedBeaconBlockCapella{
					Slot:          b.block.slot,
					ProposerIndex: b.block.proposerIndex,
					ParentRoot:    b.block.parentRoot[:],
					StateRoot:     b.block.stateRoot[:],
					Body: &eth.BlindedBeaconBlockBodyCapella{
						RandaoReveal:           b.block.body.randaoReveal[:],
						Eth1Data:               b.block.body.eth1Data,
						Graffiti:               b.block.body.graffiti[:],
						ProposerSlashings:      b.block.body.proposerSlashings,
						AttesterSlashings:      b.block.body.attesterSlashings,
						Attestations:           b.block.body.attestations,
						Deposits:               b.block.body.deposits,
						VoluntaryExits:         b.block.body.voluntaryExits,
						SyncAggregate:          b.block.body.syncAggregate,
						ExecutionPayloadHeader: header,
						BlsToExecutionChanges:  b.block.body.blsToExecutionChanges,
					},
				},
				Signature: b.signature[:],
			})
	case *enginev1.ExecutionPayloadDeneb:
		header, err := PayloadToHeaderDeneb(payload)
		if err != nil {
			return nil, err
		}
		return initBlindedSignedBlockFromProtoDeneb(
			&eth.SignedBlindedBeaconBlockDeneb{
				Message: &eth.BlindedBeaconBlockDeneb{
					Slot:          b.block.slot,
					ProposerIndex: b.block.proposerIndex,
					ParentRoot:    b.block.parentRoot[:],
					StateRoot:     b.block.stateRoot[:],
					Body: &eth.BlindedBeaconBlockBodyDeneb{
						RandaoReveal:           b.block.body.randaoReveal[:],
						Eth1Data:               b.block.body.eth1Data,
						Graffiti:               b.block.body.graffiti[:],
						ProposerSlashings:      b.block.body.proposerSlashings,
						AttesterSlashings:      b.block.body.attesterSlashings,
						Attestations:           b.block.body.attestations,
						Deposits:               b.block.body.deposits,
						VoluntaryExits:         b.block.body.voluntaryExits,
						SyncAggregate:          b.block.body.syncAggregate,
						ExecutionPayloadHeader: header,
						BlsToExecutionChanges:  b.block.body.blsToExecutionChanges,
						BlobKzgCommitments:     b.block.body.blobKzgCommitments,
					},
				},
				Signature: b.signature[:],
			})
	default:
		return nil, fmt.Errorf("%T is not an execution payload header", p)
	}
}

func (b *SignedBeaconBlock) Unblind(e interfaces.ExecutionData) error {
	if e == nil || e.IsNil() {
		return errors.New("cannot unblind with nil execution data")
	}
	if !b.IsBlinded() {
		return ErrAlreadyUnblinded
	}
	payloadRoot, err := e.HashTreeRoot()
	if err != nil {
		return err
	}
	header, err := b.Block().Body().Execution()
	if err != nil {
		return err
	}
	headerRoot, err := header.HashTreeRoot()
	if err != nil {
		return err
	}
	if payloadRoot != headerRoot {
		return errors.New("cannot unblind with different execution data")
	}
	if err := b.SetExecution(e); err != nil {
		return err
	}
	return nil
}

// Version of the underlying protobuf object.
func (b *SignedBeaconBlock) Version() int {
	return b.version
}

// IsBlinded metadata on whether a block is blinded
func (b *SignedBeaconBlock) IsBlinded() bool {
	preEPBS := b.version < version.EPBS
	return preEPBS && b.version >= version.Bellatrix && b.block.body.executionPayload == nil
}

// Header converts the underlying protobuf object from blinded block to header format.
func (b *SignedBeaconBlock) Header() (*eth.SignedBeaconBlockHeader, error) {
	if b.IsNil() {
		return nil, errNilBlock
	}
	root, err := b.block.body.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrapf(err, "could not hash block body")
	}

	return &eth.SignedBeaconBlockHeader{
		Header: &eth.BeaconBlockHeader{
			Slot:          b.block.slot,
			ProposerIndex: b.block.proposerIndex,
			ParentRoot:    b.block.parentRoot[:],
			StateRoot:     b.block.stateRoot[:],
			BodyRoot:      root[:],
		},
		Signature: b.signature[:],
	}, nil
}

// MarshalSSZ marshals the signed beacon block to its relevant ssz form.
func (b *SignedBeaconBlock) MarshalSSZ() ([]byte, error) {
	pb, err := b.Proto()
	if err != nil {
		return []byte{}, err
	}
	switch b.version {
	case version.Phase0:
		return pb.(*eth.SignedBeaconBlock).MarshalSSZ()
	case version.Altair:
		return pb.(*eth.SignedBeaconBlockAltair).MarshalSSZ()
	case version.Bellatrix:
		if b.IsBlinded() {
			return pb.(*eth.SignedBlindedBeaconBlockBellatrix).MarshalSSZ()
		}
		return pb.(*eth.SignedBeaconBlockBellatrix).MarshalSSZ()
	case version.Capella:
		if b.IsBlinded() {
			return pb.(*eth.SignedBlindedBeaconBlockCapella).MarshalSSZ()
		}
		return pb.(*eth.SignedBeaconBlockCapella).MarshalSSZ()
	case version.Deneb:
		if b.IsBlinded() {
			return pb.(*eth.SignedBlindedBeaconBlockDeneb).MarshalSSZ()
		}
		return pb.(*eth.SignedBeaconBlockDeneb).MarshalSSZ()
	case version.Electra:
		if b.IsBlinded() {
			return pb.(*eth.SignedBlindedBeaconBlockElectra).MarshalSSZ()
		}
		return pb.(*eth.SignedBeaconBlockElectra).MarshalSSZ()
	case version.EPBS:
		return pb.(*eth.SignedBeaconBlockEpbs).MarshalSSZ()
	default:
		return []byte{}, errIncorrectBlockVersion
	}
}

// MarshalSSZTo marshals the signed beacon block's ssz
// form to the provided byte buffer.
func (b *SignedBeaconBlock) MarshalSSZTo(dst []byte) ([]byte, error) {
	pb, err := b.Proto()
	if err != nil {
		return []byte{}, err
	}
	switch b.version {
	case version.Phase0:
		return pb.(*eth.SignedBeaconBlock).MarshalSSZTo(dst)
	case version.Altair:
		return pb.(*eth.SignedBeaconBlockAltair).MarshalSSZTo(dst)
	case version.Bellatrix:
		if b.IsBlinded() {
			return pb.(*eth.SignedBlindedBeaconBlockBellatrix).MarshalSSZTo(dst)
		}
		return pb.(*eth.SignedBeaconBlockBellatrix).MarshalSSZTo(dst)
	case version.Capella:
		if b.IsBlinded() {
			return pb.(*eth.SignedBlindedBeaconBlockCapella).MarshalSSZTo(dst)
		}
		return pb.(*eth.SignedBeaconBlockCapella).MarshalSSZTo(dst)
	case version.Deneb:
		if b.IsBlinded() {
			return pb.(*eth.SignedBlindedBeaconBlockDeneb).MarshalSSZTo(dst)
		}
		return pb.(*eth.SignedBeaconBlockDeneb).MarshalSSZTo(dst)
	case version.Electra:
		if b.IsBlinded() {
			return pb.(*eth.SignedBlindedBeaconBlockElectra).MarshalSSZTo(dst)
		}
		return pb.(*eth.SignedBeaconBlockElectra).MarshalSSZTo(dst)
	case version.EPBS:
		return pb.(*eth.SignedBeaconBlockEpbs).MarshalSSZTo(dst)
	default:
		return []byte{}, errIncorrectBlockVersion
	}
}

// SizeSSZ returns the size of the serialized signed block
//
// WARNING: This function panics. It is required to change the signature
// of fastssz's SizeSSZ() interface function to avoid panicking.
// Changing the signature causes very problematic issues with wealdtech deps.
// For the time being panicking is preferable.
func (b *SignedBeaconBlock) SizeSSZ() int {
	pb, err := b.Proto()
	if err != nil {
		panic(err)
	}
	switch b.version {
	case version.Phase0:
		return pb.(*eth.SignedBeaconBlock).SizeSSZ()
	case version.Altair:
		return pb.(*eth.SignedBeaconBlockAltair).SizeSSZ()
	case version.Bellatrix:
		if b.IsBlinded() {
			return pb.(*eth.SignedBlindedBeaconBlockBellatrix).SizeSSZ()
		}
		return pb.(*eth.SignedBeaconBlockBellatrix).SizeSSZ()
	case version.Capella:
		if b.IsBlinded() {
			return pb.(*eth.SignedBlindedBeaconBlockCapella).SizeSSZ()
		}
		return pb.(*eth.SignedBeaconBlockCapella).SizeSSZ()
	case version.Deneb:
		if b.IsBlinded() {
			return pb.(*eth.SignedBlindedBeaconBlockDeneb).SizeSSZ()
		}
		return pb.(*eth.SignedBeaconBlockDeneb).SizeSSZ()
	case version.Electra:
		if b.IsBlinded() {
			return pb.(*eth.SignedBlindedBeaconBlockElectra).SizeSSZ()
		}
		return pb.(*eth.SignedBeaconBlockElectra).SizeSSZ()
	case version.EPBS:
		return pb.(*eth.SignedBeaconBlockEpbs).SizeSSZ()
	default:
		panic(incorrectBlockVersion)
	}
}

// UnmarshalSSZ unmarshals the signed beacon block from its relevant ssz form.
func (b *SignedBeaconBlock) UnmarshalSSZ(buf []byte) error {
	var newBlock *SignedBeaconBlock
	switch b.version {
	case version.Phase0:
		pb := &eth.SignedBeaconBlock{}
		if err := pb.UnmarshalSSZ(buf); err != nil {
			return err
		}
		var err error
		newBlock, err = initSignedBlockFromProtoPhase0(pb)
		if err != nil {
			return err
		}
	case version.Altair:
		pb := &eth.SignedBeaconBlockAltair{}
		if err := pb.UnmarshalSSZ(buf); err != nil {
			return err
		}
		var err error
		newBlock, err = initSignedBlockFromProtoAltair(pb)
		if err != nil {
			return err
		}
	case version.Bellatrix:
		if b.IsBlinded() {
			pb := &eth.SignedBlindedBeaconBlockBellatrix{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initBlindedSignedBlockFromProtoBellatrix(pb)
			if err != nil {
				return err
			}
		} else {
			pb := &eth.SignedBeaconBlockBellatrix{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initSignedBlockFromProtoBellatrix(pb)
			if err != nil {
				return err
			}
		}
	case version.Capella:
		if b.IsBlinded() {
			pb := &eth.SignedBlindedBeaconBlockCapella{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initBlindedSignedBlockFromProtoCapella(pb)
			if err != nil {
				return err
			}
		} else {
			pb := &eth.SignedBeaconBlockCapella{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initSignedBlockFromProtoCapella(pb)
			if err != nil {
				return err
			}
		}
	case version.Deneb:
		if b.IsBlinded() {
			pb := &eth.SignedBlindedBeaconBlockDeneb{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initBlindedSignedBlockFromProtoDeneb(pb)
			if err != nil {
				return err
			}
		} else {
			pb := &eth.SignedBeaconBlockDeneb{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initSignedBlockFromProtoDeneb(pb)
			if err != nil {
				return err
			}
		}
	case version.Electra:
		if b.IsBlinded() {
			pb := &eth.SignedBlindedBeaconBlockElectra{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initBlindedSignedBlockFromProtoElectra(pb)
			if err != nil {
				return err
			}
		} else {
			pb := &eth.SignedBeaconBlockElectra{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initSignedBlockFromProtoElectra(pb)
			if err != nil {
				return err
			}
		}
	case version.EPBS:
		pb := &eth.SignedBeaconBlockEpbs{}
		if err := pb.UnmarshalSSZ(buf); err != nil {
			return err
		}
		var err error
		newBlock, err = initSignedBlockFromProtoEPBS(pb)
		if err != nil {
			return err
		}
	default:
		return errIncorrectBlockVersion
	}
	*b = *newBlock
	return nil
}

// Slot returns the respective slot of the block.
func (b *BeaconBlock) Slot() primitives.Slot {
	return b.slot
}

// ProposerIndex returns the proposer index of the beacon block.
func (b *BeaconBlock) ProposerIndex() primitives.ValidatorIndex {
	return b.proposerIndex
}

// ParentRoot returns the parent root of beacon block.
func (b *BeaconBlock) ParentRoot() [field_params.RootLength]byte {
	return b.parentRoot
}

// StateRoot returns the state root of the beacon block.
func (b *BeaconBlock) StateRoot() [field_params.RootLength]byte {
	return b.stateRoot
}

// Body returns the underlying block body.
func (b *BeaconBlock) Body() interfaces.ReadOnlyBeaconBlockBody {
	return b.body
}

// IsNil checks if the beacon block is nil.
func (b *BeaconBlock) IsNil() bool {
	return b == nil || b.Body().IsNil()
}

// IsBlinded checks if the beacon block is a blinded block.
func (b *BeaconBlock) IsBlinded() bool {
	preEPBS := b.version < version.EPBS
	return preEPBS && b.version >= version.Bellatrix && b.body.executionPayload == nil
}

// Version of the underlying protobuf object.
func (b *BeaconBlock) Version() int {
	return b.version
}

// HashTreeRoot returns the ssz root of the block.
func (b *BeaconBlock) HashTreeRoot() ([field_params.RootLength]byte, error) {
	pb, err := b.Proto()
	if err != nil {
		return [field_params.RootLength]byte{}, err
	}
	switch b.version {
	case version.Phase0:
		return pb.(*eth.BeaconBlock).HashTreeRoot()
	case version.Altair:
		return pb.(*eth.BeaconBlockAltair).HashTreeRoot()
	case version.Bellatrix:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockBellatrix).HashTreeRoot()
		}
		return pb.(*eth.BeaconBlockBellatrix).HashTreeRoot()
	case version.Capella:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockCapella).HashTreeRoot()
		}
		return pb.(*eth.BeaconBlockCapella).HashTreeRoot()
	case version.Deneb:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockDeneb).HashTreeRoot()
		}
		return pb.(*eth.BeaconBlockDeneb).HashTreeRoot()
	case version.Electra:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockElectra).HashTreeRoot()
		}
		return pb.(*eth.BeaconBlockElectra).HashTreeRoot()
	case version.EPBS:
		return pb.(*eth.BeaconBlockEpbs).HashTreeRoot()
	default:
		return [field_params.RootLength]byte{}, errIncorrectBlockVersion
	}
}

// HashTreeRootWith ssz hashes the BeaconBlock object with a hasher.
func (b *BeaconBlock) HashTreeRootWith(h *ssz.Hasher) error {
	pb, err := b.Proto()
	if err != nil {
		return err
	}
	switch b.version {
	case version.Phase0:
		return pb.(*eth.BeaconBlock).HashTreeRootWith(h)
	case version.Altair:
		return pb.(*eth.BeaconBlockAltair).HashTreeRootWith(h)
	case version.Bellatrix:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockBellatrix).HashTreeRootWith(h)
		}
		return pb.(*eth.BeaconBlockBellatrix).HashTreeRootWith(h)
	case version.Capella:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockCapella).HashTreeRootWith(h)
		}
		return pb.(*eth.BeaconBlockCapella).HashTreeRootWith(h)
	case version.Deneb:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockDeneb).HashTreeRootWith(h)
		}
		return pb.(*eth.BeaconBlockDeneb).HashTreeRootWith(h)
	case version.Electra:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockElectra).HashTreeRootWith(h)
		}
		return pb.(*eth.BeaconBlockElectra).HashTreeRootWith(h)
	case version.EPBS:
		return pb.(*eth.BeaconBlockEpbs).HashTreeRootWith(h)
	default:
		return errIncorrectBlockVersion
	}
}

// MarshalSSZ marshals the block into its respective
// ssz form.
func (b *BeaconBlock) MarshalSSZ() ([]byte, error) {
	pb, err := b.Proto()
	if err != nil {
		return []byte{}, err
	}
	switch b.version {
	case version.Phase0:
		return pb.(*eth.BeaconBlock).MarshalSSZ()
	case version.Altair:
		return pb.(*eth.BeaconBlockAltair).MarshalSSZ()
	case version.Bellatrix:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockBellatrix).MarshalSSZ()
		}
		return pb.(*eth.BeaconBlockBellatrix).MarshalSSZ()
	case version.Capella:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockCapella).MarshalSSZ()
		}
		return pb.(*eth.BeaconBlockCapella).MarshalSSZ()
	case version.Deneb:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockDeneb).MarshalSSZ()
		}
		return pb.(*eth.BeaconBlockDeneb).MarshalSSZ()
	case version.Electra:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockElectra).MarshalSSZ()
		}
		return pb.(*eth.BeaconBlockElectra).MarshalSSZ()
	case version.EPBS:
		return pb.(*eth.BeaconBlockEpbs).MarshalSSZ()
	default:
		return []byte{}, errIncorrectBlockVersion
	}
}

// MarshalSSZTo marshals the beacon block's ssz
// form to the provided byte buffer.
func (b *BeaconBlock) MarshalSSZTo(dst []byte) ([]byte, error) {
	pb, err := b.Proto()
	if err != nil {
		return []byte{}, err
	}
	switch b.version {
	case version.Phase0:
		return pb.(*eth.BeaconBlock).MarshalSSZTo(dst)
	case version.Altair:
		return pb.(*eth.BeaconBlockAltair).MarshalSSZTo(dst)
	case version.Bellatrix:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockBellatrix).MarshalSSZTo(dst)
		}
		return pb.(*eth.BeaconBlockBellatrix).MarshalSSZTo(dst)
	case version.Capella:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockCapella).MarshalSSZTo(dst)
		}
		return pb.(*eth.BeaconBlockCapella).MarshalSSZTo(dst)
	case version.Deneb:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockDeneb).MarshalSSZTo(dst)
		}
		return pb.(*eth.BeaconBlockDeneb).MarshalSSZTo(dst)
	case version.Electra:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockElectra).MarshalSSZTo(dst)
		}
		return pb.(*eth.BeaconBlockElectra).MarshalSSZTo(dst)
	case version.EPBS:
		return pb.(*eth.BeaconBlockEpbs).MarshalSSZTo(dst)
	default:
		return []byte{}, errIncorrectBlockVersion
	}
}

// SizeSSZ returns the size of the serialized block.
//
// WARNING: This function panics. It is required to change the signature
// of fastssz's SizeSSZ() interface function to avoid panicking.
// Changing the signature causes very problematic issues with wealdtech deps.
// For the time being panicking is preferable.
func (b *BeaconBlock) SizeSSZ() int {
	pb, err := b.Proto()
	if err != nil {
		panic(err)
	}
	switch b.version {
	case version.Phase0:
		return pb.(*eth.BeaconBlock).SizeSSZ()
	case version.Altair:
		return pb.(*eth.BeaconBlockAltair).SizeSSZ()
	case version.Bellatrix:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockBellatrix).SizeSSZ()
		}
		return pb.(*eth.BeaconBlockBellatrix).SizeSSZ()
	case version.Capella:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockCapella).SizeSSZ()
		}
		return pb.(*eth.BeaconBlockCapella).SizeSSZ()
	case version.Deneb:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockDeneb).SizeSSZ()
		}
		return pb.(*eth.BeaconBlockDeneb).SizeSSZ()
	case version.Electra:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockElectra).SizeSSZ()
		}
		return pb.(*eth.BeaconBlockElectra).SizeSSZ()
	case version.EPBS:
		return pb.(*eth.BeaconBlockEpbs).SizeSSZ()
	default:
		panic(incorrectBodyVersion)
	}
}

// UnmarshalSSZ unmarshals the beacon block from its relevant ssz form.
func (b *BeaconBlock) UnmarshalSSZ(buf []byte) error {
	var newBlock *BeaconBlock
	switch b.version {
	case version.Phase0:
		pb := &eth.BeaconBlock{}
		if err := pb.UnmarshalSSZ(buf); err != nil {
			return err
		}
		var err error
		newBlock, err = initBlockFromProtoPhase0(pb)
		if err != nil {
			return err
		}
	case version.Altair:
		pb := &eth.BeaconBlockAltair{}
		if err := pb.UnmarshalSSZ(buf); err != nil {
			return err
		}
		var err error
		newBlock, err = initBlockFromProtoAltair(pb)
		if err != nil {
			return err
		}
	case version.Bellatrix:
		if b.IsBlinded() {
			pb := &eth.BlindedBeaconBlockBellatrix{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initBlindedBlockFromProtoBellatrix(pb)
			if err != nil {
				return err
			}
		} else {
			pb := &eth.BeaconBlockBellatrix{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initBlockFromProtoBellatrix(pb)
			if err != nil {
				return err
			}
		}
	case version.Capella:
		if b.IsBlinded() {
			pb := &eth.BlindedBeaconBlockCapella{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initBlindedBlockFromProtoCapella(pb)
			if err != nil {
				return err
			}
		} else {
			pb := &eth.BeaconBlockCapella{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initBlockFromProtoCapella(pb)
			if err != nil {
				return err
			}
		}
	case version.Deneb:
		if b.IsBlinded() {
			pb := &eth.BlindedBeaconBlockDeneb{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initBlindedBlockFromProtoDeneb(pb)
			if err != nil {
				return err
			}
		} else {
			pb := &eth.BeaconBlockDeneb{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initBlockFromProtoDeneb(pb)
			if err != nil {
				return err
			}
		}
	case version.Electra:
		if b.IsBlinded() {
			pb := &eth.BlindedBeaconBlockElectra{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initBlindedBlockFromProtoElectra(pb)
			if err != nil {
				return err
			}
		} else {
			pb := &eth.BeaconBlockElectra{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initBlockFromProtoElectra(pb)
			if err != nil {
				return err
			}
		}
	case version.EPBS:
		pb := &eth.BeaconBlockEpbs{}
		if err := pb.UnmarshalSSZ(buf); err != nil {
			return err
		}
		var err error
		newBlock, err = initBlockFromProtoEpbs(pb)
		if err != nil {
			return err
		}
	default:
		return errIncorrectBlockVersion
	}
	*b = *newBlock
	return nil
}

// AsSignRequestObject returns the underlying sign request object.
func (b *BeaconBlock) AsSignRequestObject() (validatorpb.SignRequestObject, error) {
	pb, err := b.Proto()
	if err != nil {
		return nil, err
	}
	switch b.version {
	case version.Phase0:
		return &validatorpb.SignRequest_Block{Block: pb.(*eth.BeaconBlock)}, nil
	case version.Altair:
		return &validatorpb.SignRequest_BlockAltair{BlockAltair: pb.(*eth.BeaconBlockAltair)}, nil
	case version.Bellatrix:
		if b.IsBlinded() {
			return &validatorpb.SignRequest_BlindedBlockBellatrix{BlindedBlockBellatrix: pb.(*eth.BlindedBeaconBlockBellatrix)}, nil
		}
		return &validatorpb.SignRequest_BlockBellatrix{BlockBellatrix: pb.(*eth.BeaconBlockBellatrix)}, nil
	case version.Capella:
		if b.IsBlinded() {
			return &validatorpb.SignRequest_BlindedBlockCapella{BlindedBlockCapella: pb.(*eth.BlindedBeaconBlockCapella)}, nil
		}
		return &validatorpb.SignRequest_BlockCapella{BlockCapella: pb.(*eth.BeaconBlockCapella)}, nil
	case version.Deneb:
		if b.IsBlinded() {
			return &validatorpb.SignRequest_BlindedBlockDeneb{BlindedBlockDeneb: pb.(*eth.BlindedBeaconBlockDeneb)}, nil
		}
		return &validatorpb.SignRequest_BlockDeneb{BlockDeneb: pb.(*eth.BeaconBlockDeneb)}, nil
	case version.Electra:
		if b.IsBlinded() {
			return &validatorpb.SignRequest_BlindedBlockElectra{BlindedBlockElectra: pb.(*eth.BlindedBeaconBlockElectra)}, nil
		}
		return &validatorpb.SignRequest_BlockElectra{BlockElectra: pb.(*eth.BeaconBlockElectra)}, nil
	case version.EPBS:
		return &validatorpb.SignRequest_BlockEpbs{BlockEpbs: pb.(*eth.BeaconBlockEpbs)}, nil
	default:
		return nil, errIncorrectBlockVersion
	}
}

func (b *BeaconBlock) Copy() (interfaces.ReadOnlyBeaconBlock, error) {
	if b == nil {
		return nil, nil
	}

	pb, err := b.Proto()
	if err != nil {
		return nil, err
	}
	switch b.version {
	case version.Phase0:
		return initBlockFromProtoPhase0(pb.(*eth.BeaconBlock).Copy())
	case version.Altair:
		return initBlockFromProtoAltair(pb.(*eth.BeaconBlockAltair).Copy())
	case version.Bellatrix:
		if b.IsBlinded() {
			return initBlindedBlockFromProtoBellatrix(pb.(*eth.BlindedBeaconBlockBellatrix).Copy())
		}
		return initBlockFromProtoBellatrix(pb.(*eth.BeaconBlockBellatrix).Copy())
	case version.Capella:
		if b.IsBlinded() {
			return initBlindedBlockFromProtoCapella(pb.(*eth.BlindedBeaconBlockCapella).Copy())
		}
		return initBlockFromProtoCapella(pb.(*eth.BeaconBlockCapella).Copy())
	case version.Deneb:
		if b.IsBlinded() {
			return initBlindedBlockFromProtoDeneb(pb.(*eth.BlindedBeaconBlockDeneb).Copy())
		}
		return initBlockFromProtoDeneb(pb.(*eth.BeaconBlockDeneb).Copy())
	case version.Electra:
		if b.IsBlinded() {
			return initBlindedBlockFromProtoElectra(pb.(*eth.BlindedBeaconBlockElectra).Copy())
		}
		return initBlockFromProtoElectra(pb.(*eth.BeaconBlockElectra).Copy())
	case version.EPBS:
		cp := eth.CopyBeaconBlockEPBS(pb.(*eth.BeaconBlockEpbs))
		return initBlockFromProtoEpbs(cp)
	default:
		return nil, errIncorrectBlockVersion
	}
}

// IsNil checks if the block body is nil.
func (b *BeaconBlockBody) IsNil() bool {
	return b == nil
}

// RandaoReveal returns the randao reveal from the block body.
func (b *BeaconBlockBody) RandaoReveal() [field_params.BLSSignatureLength]byte {
	return b.randaoReveal
}

// Eth1Data returns the eth1 data in the block.
func (b *BeaconBlockBody) Eth1Data() *eth.Eth1Data {
	return b.eth1Data
}

// Graffiti returns the graffiti in the block.
func (b *BeaconBlockBody) Graffiti() [field_params.RootLength]byte {
	return b.graffiti
}

// ProposerSlashings returns the proposer slashings in the block.
func (b *BeaconBlockBody) ProposerSlashings() []*eth.ProposerSlashing {
	return b.proposerSlashings
}

// AttesterSlashings returns the attester slashings in the block.
func (b *BeaconBlockBody) AttesterSlashings() []eth.AttSlashing {
	var slashings []eth.AttSlashing
	if b.version < version.Electra {
		if b.attesterSlashings == nil {
			return nil
		}
		slashings = make([]eth.AttSlashing, len(b.attesterSlashings))
		for i, s := range b.attesterSlashings {
			slashings[i] = s
		}
	} else {
		if b.attesterSlashingsElectra == nil {
			return nil
		}
		slashings = make([]eth.AttSlashing, len(b.attesterSlashingsElectra))
		for i, s := range b.attesterSlashingsElectra {
			slashings[i] = s
		}
	}
	return slashings
}

// Attestations returns the stored attestations in the block.
func (b *BeaconBlockBody) Attestations() []eth.Att {
	var atts []eth.Att
	if b.version < version.Electra {
		if b.attestations == nil {
			return nil
		}
		atts = make([]eth.Att, len(b.attestations))
		for i, a := range b.attestations {
			atts[i] = a
		}
	} else {
		if b.attestationsElectra == nil {
			return nil
		}
		atts = make([]eth.Att, len(b.attestationsElectra))
		for i, a := range b.attestationsElectra {
			atts[i] = a
		}
	}
	return atts
}

// Deposits returns the stored deposits in the block.
func (b *BeaconBlockBody) Deposits() []*eth.Deposit {
	return b.deposits
}

// VoluntaryExits returns the voluntary exits in the block.
func (b *BeaconBlockBody) VoluntaryExits() []*eth.SignedVoluntaryExit {
	return b.voluntaryExits
}

// SyncAggregate returns the sync aggregate in the block.
func (b *BeaconBlockBody) SyncAggregate() (*eth.SyncAggregate, error) {
	if b.version == version.Phase0 {
		return nil, consensus_types.ErrNotSupported("SyncAggregate", b.version)
	}
	return b.syncAggregate, nil
}

// Execution returns the execution payload of the block body.
func (b *BeaconBlockBody) Execution() (interfaces.ExecutionData, error) {
	switch b.version {
	case version.Phase0, version.Altair, version.EPBS:
		return nil, consensus_types.ErrNotSupported("Execution", b.version)
	default:
		if b.IsBlinded() {
			return b.executionPayloadHeader, nil
		}
		return b.executionPayload, nil
	}
}

func (b *BeaconBlockBody) BLSToExecutionChanges() ([]*eth.SignedBLSToExecutionChange, error) {
	if b.version < version.Capella {
		return nil, consensus_types.ErrNotSupported("BLSToExecutionChanges", b.version)
	}
	return b.blsToExecutionChanges, nil
}

// BlobKzgCommitments returns the blob kzg commitments in the block.
func (b *BeaconBlockBody) BlobKzgCommitments() ([][]byte, error) {
	switch b.version {
	case version.Phase0, version.Altair, version.Bellatrix, version.Capella:
		return nil, consensus_types.ErrNotSupported("BlobKzgCommitments", b.version)
	case version.Deneb, version.Electra:
		return b.blobKzgCommitments, nil
	default:
		return nil, errIncorrectBlockVersion
	}
}

// ExecutionRequests returns the execution requests
func (b *BeaconBlockBody) ExecutionRequests() (*enginev1.ExecutionRequests, error) {
	if b.version < version.Electra {
		return nil, consensus_types.ErrNotSupported("ExecutionRequests", b.version)
	}
	return b.executionRequests, nil
}

// PayloadAttestations returns the payload attestations in the block.
func (b *BeaconBlockBody) PayloadAttestations() []*eth.PayloadAttestation {
	return b.payloadAttestations
}

// SignedExecutionPayloadHeader returns the signed execution payload header in the block.
func (b *BeaconBlockBody) SignedExecutionPayloadHeader() *enginev1.SignedExecutionPayloadHeader {
	return b.signedExecutionPayloadHeader
}

// Version returns the version of the beacon block body
func (b *BeaconBlockBody) Version() int {
	return b.version
}

// HashTreeRoot returns the ssz root of the block body.
func (b *BeaconBlockBody) HashTreeRoot() ([field_params.RootLength]byte, error) {
	pb, err := b.Proto()
	if err != nil {
		return [field_params.RootLength]byte{}, err
	}
	switch b.version {
	case version.Phase0:
		return pb.(*eth.BeaconBlockBody).HashTreeRoot()
	case version.Altair:
		return pb.(*eth.BeaconBlockBodyAltair).HashTreeRoot()
	case version.Bellatrix:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockBodyBellatrix).HashTreeRoot()
		}
		return pb.(*eth.BeaconBlockBodyBellatrix).HashTreeRoot()
	case version.Capella:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockBodyCapella).HashTreeRoot()
		}
		return pb.(*eth.BeaconBlockBodyCapella).HashTreeRoot()
	case version.Deneb:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockBodyDeneb).HashTreeRoot()
		}
		return pb.(*eth.BeaconBlockBodyDeneb).HashTreeRoot()
	case version.Electra:
		if b.IsBlinded() {
			return pb.(*eth.BlindedBeaconBlockBodyElectra).HashTreeRoot()
		}
		return pb.(*eth.BeaconBlockBodyElectra).HashTreeRoot()
	case version.EPBS:
		return pb.(*eth.BeaconBlockBodyEpbs).HashTreeRoot()
	default:
		return [field_params.RootLength]byte{}, errIncorrectBodyVersion
	}
}

// IsBlinded checks if the beacon block body is a blinded block body.
func (b *BeaconBlockBody) IsBlinded() bool {
	preEPBS := b.version < version.EPBS
	return preEPBS && b.version >= version.Bellatrix && b.executionPayload == nil
}
