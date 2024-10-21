package light_client

import (
	"fmt"

	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	consensustypes "github.com/prysmaticlabs/prysm/v5/consensus-types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	pb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"google.golang.org/protobuf/proto"
)

func NewWrappedUpdate(m proto.Message) (interfaces.LightClientUpdate, error) {
	if m == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	switch t := m.(type) {
	case *pb.LightClientUpdateAltair:
		return NewWrappedUpdateAltair(t)
	case *pb.LightClientUpdateCapella:
		return NewWrappedUpdateCapella(t)
	case *pb.LightClientUpdateDeneb:
		return NewWrappedUpdateDeneb(t)
	case *pb.LightClientUpdateElectra:
		return NewWrappedUpdateElectra(t)
	default:
		return nil, fmt.Errorf("cannot construct light client update from type %T", t)
	}
}

type updateAltair struct {
	p                       *pb.LightClientUpdateAltair
	attestedHeader          interfaces.LightClientHeader
	nextSyncCommitteeBranch interfaces.LightClientSyncCommitteeBranch
	finalizedHeader         interfaces.LightClientHeader
	finalityBranch          interfaces.LightClientFinalityBranch
}

var _ interfaces.LightClientUpdate = &updateAltair{}

func NewWrappedUpdateAltair(p *pb.LightClientUpdateAltair) (interfaces.LightClientUpdate, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	attestedHeader, err := NewWrappedHeaderAltair(p.AttestedHeader)
	if err != nil {
		return nil, err
	}

	var finalizedHeader interfaces.LightClientHeader
	if p.FinalizedHeader != nil {
		finalizedHeader, err = NewWrappedHeaderAltair(p.FinalizedHeader)
		if err != nil {
			return nil, err
		}
	}

	scBranch, err := createBranch[interfaces.LightClientSyncCommitteeBranch](
		"sync committee",
		p.NextSyncCommitteeBranch,
		fieldparams.SyncCommitteeBranchDepth,
	)
	if err != nil {
		return nil, err
	}
	finalityBranch, err := createBranch[interfaces.LightClientFinalityBranch](
		"finality",
		p.FinalityBranch,
		fieldparams.FinalityBranchDepth,
	)
	if err != nil {
		return nil, err
	}

	return &updateAltair{
		p:                       p,
		attestedHeader:          attestedHeader,
		nextSyncCommitteeBranch: scBranch,
		finalizedHeader:         finalizedHeader,
		finalityBranch:          finalityBranch,
	}, nil
}

func (u *updateAltair) MarshalSSZTo(dst []byte) ([]byte, error) {
	return u.p.MarshalSSZTo(dst)
}

func (u *updateAltair) MarshalSSZ() ([]byte, error) {
	return u.p.MarshalSSZ()
}

func (u *updateAltair) SizeSSZ() int {
	return u.p.SizeSSZ()
}

func (u *updateAltair) Proto() proto.Message {
	return u.p
}

func (u *updateAltair) Version() int {
	return version.Altair
}

func (u *updateAltair) AttestedHeader() interfaces.LightClientHeader {
	return u.attestedHeader
}

func (u *updateAltair) SetAttestedHeader(header interfaces.LightClientHeader) {
	u.attestedHeader = header
}

func (u *updateAltair) NextSyncCommittee() *pb.SyncCommittee {
	return u.p.NextSyncCommittee
}

func (u *updateAltair) SetNextSyncCommittee(sc *pb.SyncCommittee) {
	u.p.NextSyncCommittee = sc
}

func (u *updateAltair) NextSyncCommitteeBranch() (interfaces.LightClientSyncCommitteeBranch, error) {
	return u.nextSyncCommitteeBranch, nil
}

func (u *updateAltair) SetNextSyncCommitteeBranch(branch [][]byte) error {
	b, err := createBranch[interfaces.LightClientSyncCommitteeBranch]("sync committee", branch, fieldparams.SyncCommitteeBranchDepth)
	if err != nil {
		return err
	}
	u.nextSyncCommitteeBranch = b
	return nil
}

func (u *updateAltair) NextSyncCommitteeBranchElectra() (interfaces.LightClientSyncCommitteeBranchElectra, error) {
	return [6][32]byte{}, consensustypes.ErrNotSupported("NextSyncCommitteeBranchElectra", version.Altair)
}

func (u *updateAltair) SetNextSyncCommitteeBranchElectra([][]byte) error {
	return consensustypes.ErrNotSupported("SetNextSyncCommitteeBranchElectra", version.Altair)
}

func (u *updateAltair) FinalizedHeader() interfaces.LightClientHeader {
	return u.finalizedHeader
}

func (u *updateAltair) SetFinalizedHeader(header interfaces.LightClientHeader) {
	u.finalizedHeader = header
}

func (u *updateAltair) FinalityBranch() interfaces.LightClientFinalityBranch {
	return u.finalityBranch
}

func (u *updateAltair) SetFinalityBranch(branch [][]byte) error {
	b, err := createBranch[interfaces.LightClientFinalityBranch]("finality", branch, fieldparams.FinalityBranchDepth)
	if err != nil {
		return err
	}
	u.finalityBranch = b
	return nil
}

func (u *updateAltair) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *updateAltair) SetSyncAggregate(sa *pb.SyncAggregate) {
	u.p.SyncAggregate = sa
}

func (u *updateAltair) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}

func (u *updateAltair) SetSignatureSlot(slot primitives.Slot) {
	u.p.SignatureSlot = slot
}

type updateCapella struct {
	p                       *pb.LightClientUpdateCapella
	attestedHeader          interfaces.LightClientHeader
	nextSyncCommitteeBranch interfaces.LightClientSyncCommitteeBranch
	finalizedHeader         interfaces.LightClientHeader
	finalityBranch          interfaces.LightClientFinalityBranch
}

var _ interfaces.LightClientUpdate = &updateCapella{}

func NewWrappedUpdateCapella(p *pb.LightClientUpdateCapella) (interfaces.LightClientUpdate, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	attestedHeader, err := NewWrappedHeaderCapella(p.AttestedHeader)
	if err != nil {
		return nil, err
	}

	var finalizedHeader interfaces.LightClientHeader
	if p.FinalizedHeader != nil {
		finalizedHeader, err = NewWrappedHeaderCapella(p.FinalizedHeader)
		if err != nil {
			return nil, err
		}
	}

	scBranch, err := createBranch[interfaces.LightClientSyncCommitteeBranch](
		"sync committee",
		p.NextSyncCommitteeBranch,
		fieldparams.SyncCommitteeBranchDepth,
	)
	if err != nil {
		return nil, err
	}
	finalityBranch, err := createBranch[interfaces.LightClientFinalityBranch](
		"finality",
		p.FinalityBranch,
		fieldparams.FinalityBranchDepth,
	)
	if err != nil {
		return nil, err
	}

	return &updateCapella{
		p:                       p,
		attestedHeader:          attestedHeader,
		nextSyncCommitteeBranch: scBranch,
		finalizedHeader:         finalizedHeader,
		finalityBranch:          finalityBranch,
	}, nil
}

func (u *updateCapella) MarshalSSZTo(dst []byte) ([]byte, error) {
	return u.p.MarshalSSZTo(dst)
}

func (u *updateCapella) MarshalSSZ() ([]byte, error) {
	return u.p.MarshalSSZ()
}

func (u *updateCapella) SizeSSZ() int {
	return u.p.SizeSSZ()
}

func (u *updateCapella) Proto() proto.Message {
	return u.p
}

func (u *updateCapella) Version() int {
	return version.Capella
}

func (u *updateCapella) AttestedHeader() interfaces.LightClientHeader {
	return u.attestedHeader
}

func (u *updateCapella) SetAttestedHeader(header interfaces.LightClientHeader) {
	u.attestedHeader = header
}

func (u *updateCapella) NextSyncCommittee() *pb.SyncCommittee {
	return u.p.NextSyncCommittee
}

func (u *updateCapella) SetNextSyncCommittee(sc *pb.SyncCommittee) {
	u.p.NextSyncCommittee = sc
}

func (u *updateCapella) NextSyncCommitteeBranch() (interfaces.LightClientSyncCommitteeBranch, error) {
	return u.nextSyncCommitteeBranch, nil
}

func (u *updateCapella) SetNextSyncCommitteeBranch(branch [][]byte) error {
	b, err := createBranch[interfaces.LightClientSyncCommitteeBranch]("sync committee", branch, fieldparams.SyncCommitteeBranchDepth)
	if err != nil {
		return err
	}
	u.nextSyncCommitteeBranch = b
	return nil
}

func (u *updateCapella) NextSyncCommitteeBranchElectra() (interfaces.LightClientSyncCommitteeBranchElectra, error) {
	return [6][32]byte{}, consensustypes.ErrNotSupported("NextSyncCommitteeBranchElectra", version.Capella)
}

func (u *updateCapella) SetNextSyncCommitteeBranchElectra([][]byte) error {
	return consensustypes.ErrNotSupported("SetNextSyncCommitteeBranchElectra", version.Capella)
}

func (u *updateCapella) FinalizedHeader() interfaces.LightClientHeader {
	return u.finalizedHeader
}

func (u *updateCapella) SetFinalizedHeader(header interfaces.LightClientHeader) {
	u.finalizedHeader = header
}

func (u *updateCapella) FinalityBranch() interfaces.LightClientFinalityBranch {
	return u.finalityBranch
}

func (u *updateCapella) SetFinalityBranch(branch [][]byte) error {
	b, err := createBranch[interfaces.LightClientFinalityBranch]("finality", branch, fieldparams.FinalityBranchDepth)
	if err != nil {
		return err
	}
	u.finalityBranch = b
	return nil
}

func (u *updateCapella) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *updateCapella) SetSyncAggregate(sa *pb.SyncAggregate) {
	u.p.SyncAggregate = sa
}

func (u *updateCapella) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}

func (u *updateCapella) SetSignatureSlot(slot primitives.Slot) {
	u.p.SignatureSlot = slot
}

type updateDeneb struct {
	p                       *pb.LightClientUpdateDeneb
	attestedHeader          interfaces.LightClientHeader
	nextSyncCommitteeBranch interfaces.LightClientSyncCommitteeBranch
	finalizedHeader         interfaces.LightClientHeader
	finalityBranch          interfaces.LightClientFinalityBranch
}

var _ interfaces.LightClientUpdate = &updateDeneb{}

func NewWrappedUpdateDeneb(p *pb.LightClientUpdateDeneb) (interfaces.LightClientUpdate, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	attestedHeader, err := NewWrappedHeaderDeneb(p.AttestedHeader)
	if err != nil {
		return nil, err
	}

	var finalizedHeader interfaces.LightClientHeader
	if p.FinalizedHeader != nil {
		finalizedHeader, err = NewWrappedHeaderDeneb(p.FinalizedHeader)
		if err != nil {
			return nil, err
		}
	}

	scBranch, err := createBranch[interfaces.LightClientSyncCommitteeBranch](
		"sync committee",
		p.NextSyncCommitteeBranch,
		fieldparams.SyncCommitteeBranchDepth,
	)
	if err != nil {
		return nil, err
	}
	finalityBranch, err := createBranch[interfaces.LightClientFinalityBranch](
		"finality",
		p.FinalityBranch,
		fieldparams.FinalityBranchDepth,
	)
	if err != nil {
		return nil, err
	}

	return &updateDeneb{
		p:                       p,
		attestedHeader:          attestedHeader,
		nextSyncCommitteeBranch: scBranch,
		finalizedHeader:         finalizedHeader,
		finalityBranch:          finalityBranch,
	}, nil
}

func (u *updateDeneb) MarshalSSZTo(dst []byte) ([]byte, error) {
	return u.p.MarshalSSZTo(dst)
}

func (u *updateDeneb) MarshalSSZ() ([]byte, error) {
	return u.p.MarshalSSZ()
}

func (u *updateDeneb) SizeSSZ() int {
	return u.p.SizeSSZ()
}

func (u *updateDeneb) Proto() proto.Message {
	return u.p
}

func (u *updateDeneb) Version() int {
	return version.Deneb
}

func (u *updateDeneb) AttestedHeader() interfaces.LightClientHeader {
	return u.attestedHeader
}

func (u *updateDeneb) SetAttestedHeader(header interfaces.LightClientHeader) {
	u.attestedHeader = header
}

func (u *updateDeneb) NextSyncCommittee() *pb.SyncCommittee {
	return u.p.NextSyncCommittee
}

func (u *updateDeneb) SetNextSyncCommittee(sc *pb.SyncCommittee) {
	u.p.NextSyncCommittee = sc
}

func (u *updateDeneb) NextSyncCommitteeBranch() (interfaces.LightClientSyncCommitteeBranch, error) {
	return u.nextSyncCommitteeBranch, nil
}

func (u *updateDeneb) SetNextSyncCommitteeBranch(branch [][]byte) error {
	b, err := createBranch[interfaces.LightClientSyncCommitteeBranch]("sync committee", branch, fieldparams.SyncCommitteeBranchDepth)
	if err != nil {
		return err
	}
	u.nextSyncCommitteeBranch = b
	return nil
}

func (u *updateDeneb) NextSyncCommitteeBranchElectra() (interfaces.LightClientSyncCommitteeBranchElectra, error) {
	return [6][32]byte{}, consensustypes.ErrNotSupported("NextSyncCommitteeBranchElectra", version.Deneb)
}

func (u *updateDeneb) SetNextSyncCommitteeBranchElectra([][]byte) error {
	return consensustypes.ErrNotSupported("SetNextSyncCommitteeBranchElectra", version.Deneb)
}

func (u *updateDeneb) FinalizedHeader() interfaces.LightClientHeader {
	return u.finalizedHeader
}

func (u *updateDeneb) SetFinalizedHeader(header interfaces.LightClientHeader) {
	u.finalizedHeader = header
}

func (u *updateDeneb) FinalityBranch() interfaces.LightClientFinalityBranch {
	return u.finalityBranch
}

func (u *updateDeneb) SetFinalityBranch(branch [][]byte) error {
	b, err := createBranch[interfaces.LightClientFinalityBranch]("finality", branch, fieldparams.FinalityBranchDepth)
	if err != nil {
		return err
	}
	u.finalityBranch = b
	return nil
}

func (u *updateDeneb) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *updateDeneb) SetSyncAggregate(sa *pb.SyncAggregate) {
	u.p.SyncAggregate = sa
}

func (u *updateDeneb) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}

func (u *updateDeneb) SetSignatureSlot(slot primitives.Slot) {
	u.p.SignatureSlot = slot
}

type updateElectra struct {
	p                       *pb.LightClientUpdateElectra
	attestedHeader          interfaces.LightClientHeader
	nextSyncCommitteeBranch interfaces.LightClientSyncCommitteeBranchElectra
	finalizedHeader         interfaces.LightClientHeader
	finalityBranch          interfaces.LightClientFinalityBranch
}

var _ interfaces.LightClientUpdate = &updateElectra{}

func NewWrappedUpdateElectra(p *pb.LightClientUpdateElectra) (interfaces.LightClientUpdate, error) {
	if p == nil {
		return nil, consensustypes.ErrNilObjectWrapped
	}
	attestedHeader, err := NewWrappedHeaderDeneb(p.AttestedHeader)
	if err != nil {
		return nil, err
	}

	var finalizedHeader interfaces.LightClientHeader
	if p.FinalizedHeader != nil {
		finalizedHeader, err = NewWrappedHeaderDeneb(p.FinalizedHeader)
		if err != nil {
			return nil, err
		}
	}

	scBranch, err := createBranch[interfaces.LightClientSyncCommitteeBranchElectra](
		"sync committee",
		p.NextSyncCommitteeBranch,
		fieldparams.SyncCommitteeBranchDepthElectra,
	)
	if err != nil {
		return nil, err
	}
	finalityBranch, err := createBranch[interfaces.LightClientFinalityBranch](
		"finality",
		p.FinalityBranch,
		fieldparams.FinalityBranchDepth,
	)
	if err != nil {
		return nil, err
	}

	return &updateElectra{
		p:                       p,
		attestedHeader:          attestedHeader,
		nextSyncCommitteeBranch: scBranch,
		finalizedHeader:         finalizedHeader,
		finalityBranch:          finalityBranch,
	}, nil
}

func (u *updateElectra) MarshalSSZTo(dst []byte) ([]byte, error) {
	return u.p.MarshalSSZTo(dst)
}

func (u *updateElectra) MarshalSSZ() ([]byte, error) {
	return u.p.MarshalSSZ()
}

func (u *updateElectra) SizeSSZ() int {
	return u.p.SizeSSZ()
}

func (u *updateElectra) Proto() proto.Message {
	return u.p
}

func (u *updateElectra) Version() int {
	return version.Electra
}

func (u *updateElectra) AttestedHeader() interfaces.LightClientHeader {
	return u.attestedHeader
}

func (u *updateElectra) SetAttestedHeader(header interfaces.LightClientHeader) {
	u.attestedHeader = header
}

func (u *updateElectra) NextSyncCommittee() *pb.SyncCommittee {
	return u.p.NextSyncCommittee
}

func (u *updateElectra) SetNextSyncCommittee(sc *pb.SyncCommittee) {
	u.p.NextSyncCommittee = sc
}

func (u *updateElectra) NextSyncCommitteeBranch() (interfaces.LightClientSyncCommitteeBranch, error) {
	return [5][32]byte{}, consensustypes.ErrNotSupported("NextSyncCommitteeBranch", version.Electra)
}

func (u *updateElectra) SetNextSyncCommitteeBranch([][]byte) error {
	return consensustypes.ErrNotSupported("SetNextSyncCommitteeBranch", version.Electra)
}

func (u *updateElectra) NextSyncCommitteeBranchElectra() (interfaces.LightClientSyncCommitteeBranchElectra, error) {
	return u.nextSyncCommitteeBranch, nil
}

func (u *updateElectra) SetNextSyncCommitteeBranchElectra(branch [][]byte) error {
	b, err := createBranch[interfaces.LightClientSyncCommitteeBranchElectra]("sync committee", branch, fieldparams.SyncCommitteeBranchDepthElectra)
	if err != nil {
		return err
	}
	u.nextSyncCommitteeBranch = b
	return nil
}

func (u *updateElectra) FinalizedHeader() interfaces.LightClientHeader {
	return u.finalizedHeader
}

func (u *updateElectra) SetFinalizedHeader(header interfaces.LightClientHeader) {
	u.finalizedHeader = header
}

func (u *updateElectra) FinalityBranch() interfaces.LightClientFinalityBranch {
	return u.finalityBranch
}

func (u *updateElectra) SetFinalityBranch(branch [][]byte) error {
	b, err := createBranch[interfaces.LightClientFinalityBranch]("finality", branch, fieldparams.FinalityBranchDepth)
	if err != nil {
		return err
	}
	u.finalityBranch = b
	return nil
}

func (u *updateElectra) SyncAggregate() *pb.SyncAggregate {
	return u.p.SyncAggregate
}

func (u *updateElectra) SetSyncAggregate(sa *pb.SyncAggregate) {
	u.p.SyncAggregate = sa
}

func (u *updateElectra) SignatureSlot() primitives.Slot {
	return u.p.SignatureSlot
}

func (u *updateElectra) SetSignatureSlot(slot primitives.Slot) {
	u.p.SignatureSlot = slot
}
