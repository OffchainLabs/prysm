package blocks

import (
	"errors"

	"github.com/OffchainLabs/go-bitfield"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/libp2p/go-libp2p-pubsub/partialmessages"
	"github.com/sirupsen/logrus"
)

type CellProofBundle struct {
	ColumnIndex uint64
	Commitment  []byte
	Cell        []byte
	Proof       []byte
}

type PartialDataColumn struct {
	*ethpb.DataColumnSidecar
	root    [fieldparams.RootLength]byte
	groupID []byte

	Included bitfield.Bitlist

	// Parts we've received before we have any commitments to validate against.
	// Happens when a peer eager pushes to us.
	// TODO implement. For now, not bothering to handle the eager pushes.
	// quarantine []*ethpb.PartialDataColumnSidecar
}

// const quarantineSize = 3

// NewPartialDataColumn creates a new Partial Data Column for the given block.
// It does not validate the inputs. The caller is responsible for validating the
// block header and KZG Commitment Inclusion proof.
func NewPartialDataColumn(
	signedBlockHeader *ethpb.SignedBeaconBlockHeader,
	columnIndex uint64,
	kzgCommitments [][]byte,
	kzgInclusionProof [][]byte,
) (PartialDataColumn, error) {
	root, err := signedBlockHeader.Header.HashTreeRoot()
	if err != nil {
		return PartialDataColumn{}, err
	}

	sidecar := &ethpb.DataColumnSidecar{
		Index:                        columnIndex,
		KzgCommitments:               kzgCommitments,
		Column:                       make([][]byte, len(kzgCommitments)),
		KzgProofs:                    make([][]byte, len(kzgCommitments)),
		SignedBlockHeader:            signedBlockHeader,
		KzgCommitmentsInclusionProof: kzgInclusionProof,
	}

	groupID := make([]byte, len(root)+1)
	copy(groupID[1:], root[:])
	// Version 0
	groupID[0] = 0

	c := PartialDataColumn{
		DataColumnSidecar: sidecar,
		root:              root,
		groupID:           groupID,
		Included:          bitfield.NewBitlist(uint64(len(sidecar.KzgCommitments))),
	}
	if len(c.Column) != len(c.KzgCommitments) {
		return PartialDataColumn{}, errors.New("mismatch between number of cells and commitments")
	}
	if len(c.KzgProofs) != len(c.KzgCommitments) {
		return PartialDataColumn{}, errors.New("mismatch between number of proofs and commitments")
	}

	for i := range len(c.KzgCommitments) {
		if sidecar.Column[i] == nil {
			continue
		}
		c.Included.SetBitAt(uint64(i), true)
	}
	return c, nil
}

func (p *PartialDataColumn) GroupID() []byte {
	return p.groupID
}

// NewPartsMetadata creates SSZ-encoded PartialDataColumnPartsMetadata from the given bitmaps.
func NewPartsMetadata(available, requests bitfield.Bitlist) (partialmessages.PartsMetadata, error) {
	meta := &ethpb.PartialDataColumnPartsMetadata{
		Available: available,
		Requests:  requests,
	}
	marshalled, err := meta.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return partialmessages.PartsMetadata(marshalled), nil
}

// ParsePartsMetadata deserializes SSZ-encoded PartialDataColumnPartsMetadata.
func ParsePartsMetadata(data partialmessages.PartsMetadata) (*ethpb.PartialDataColumnPartsMetadata, error) {
	meta := &ethpb.PartialDataColumnPartsMetadata{}
	if err := meta.UnmarshalSSZ(data); err != nil {
		return nil, err
	}
	return meta, nil
}

// MergePartsMetadata merges two SSZ-encoded PartialDataColumnPartsMetadata by OR-ing
// both available and requests bitmaps.
// TODO: How do we handle the request bitmap here ?
func MergePartsMetadata(left, right partialmessages.PartsMetadata) (partialmessages.PartsMetadata, error) {
	if len(left) == 0 {
		return right, nil
	}
	if len(right) == 0 {
		return left, nil
	}

	leftMeta, err := ParsePartsMetadata(left)
	if err != nil {
		return left, err
	}
	rightMeta, err := ParsePartsMetadata(right)
	if err != nil {
		return left, err
	}

	mergedAvailable, err := bitfield.Bitlist(leftMeta.Available).Or(bitfield.Bitlist(rightMeta.Available))
	if err != nil {
		return left, err
	}
	mergedRequests, err := bitfield.Bitlist(leftMeta.Requests).Or(bitfield.Bitlist(rightMeta.Requests))
	if err != nil {
		return left, err
	}

	return NewPartsMetadata(mergedAvailable, mergedRequests)
}

func (p *PartialDataColumn) PartialMessageBytes(metadata partialmessages.PartsMetadata) ([]byte, error) {
	peerMeta, err := ParsePartsMetadata(metadata)
	if err != nil {
		return nil, err
	}

	peerAvailable := bitfield.Bitlist(peerMeta.Available)
	peerRequests := bitfield.Bitlist(peerMeta.Requests)

	if peerAvailable.Len() != p.Included.Len() {
		return nil, errors.New("available bitmap length does not match expected length")
	}
	if peerRequests.Len() != p.Included.Len() {
		return nil, errors.New("requests bitmap length does not match expected length")
	}

	var cellsToReturn int
	for i := range p.Included.Len() {
		// Send cell if: we have it, peer doesn't have it, and peer wants it.
		if p.Included.BitAt(i) && !peerAvailable.BitAt(i) && peerRequests.BitAt(i) {
			cellsToReturn++
		}
	}
	if cellsToReturn == 0 {
		return nil, nil
	}

	included := bitfield.NewBitlist(p.Included.Len())
	outMessage := ethpb.PartialDataColumnSidecar{
		CellsPresentBitmap: included,
		PartialColumn:      make([][]byte, 0, cellsToReturn),
		KzgProofs:          make([][]byte, 0, cellsToReturn),
	}
	for i := range p.Included.Len() {
		if !p.Included.BitAt(i) || peerAvailable.BitAt(i) || !peerRequests.BitAt(i) {
			continue
		}
		included.SetBitAt(i, true)
		outMessage.PartialColumn = append(outMessage.PartialColumn, p.Column[i])
		outMessage.KzgProofs = append(outMessage.KzgProofs, p.KzgProofs[i])
	}

	marshalled, err := outMessage.MarshalSSZ()
	if err != nil {
		return nil, err
	}

	return marshalled, nil
}

func (p *PartialDataColumn) EagerPartialMessageBytes() ([]byte, partialmessages.PartsMetadata, error) {
	// TODO: do we want to send this once per groupID per peer
	// Eagerly push the PartialDataColumnHeader
	outHeader := &ethpb.PartialDataColumnHeader{
		KzgCommitments:               p.KzgCommitments,
		SignedBlockHeader:            p.SignedBlockHeader,
		KzgCommitmentsInclusionProof: p.KzgCommitmentsInclusionProof,
	}
	outMessage := &ethpb.PartialDataColumnSidecar{
		CellsPresentBitmap: bitfield.NewBitlist(uint64(len(p.KzgCommitments))),
		Header:             []*ethpb.PartialDataColumnHeader{outHeader},
	}

	marshalled, err := outMessage.MarshalSSZ()
	if err != nil {
		return nil, nil, err
	}
	// Empty available (no cells sent), empty requests (we don't know what the peer wants)
	numCommitments := uint64(len(p.KzgCommitments))
	peersNextParts, err := NewPartsMetadata(
		bitfield.NewBitlist(numCommitments),
		bitfield.NewBitlist(numCommitments),
	)
	if err != nil {
		return nil, nil, err
	}

	return marshalled, peersNextParts, nil
}

func (p *PartialDataColumn) PartsMetadata() partialmessages.PartsMetadata {
	numCommitments := uint64(len(p.KzgCommitments))
	meta, err := NewPartsMetadata(p.Included, allOnesBitlist(numCommitments))
	if err != nil {
		logrus.Error("failed to create parts metadata", "err", err)
		return nil
	}
	return meta
}

// CellsToVerifyFromPartialMessage returns cells from the partial message that need to be verified.
func (p *PartialDataColumn) CellsToVerifyFromPartialMessage(message *ethpb.PartialDataColumnSidecar) ([]uint64, []CellProofBundle, error) {
	included := message.CellsPresentBitmap
	if included.Len() == 0 {
		return nil, nil, nil
	}

	// Some basic sanity checks
	includedCells := included.Count()
	if uint64(len(message.KzgProofs)) != includedCells {
		return nil, nil, errors.New("invalid message. Missing KZG proofs")
	}
	if uint64(len(message.PartialColumn)) != includedCells {
		return nil, nil, errors.New("invalid message. Missing cells")
	}

	ourIncludedList := p.Included
	if included.Len() != ourIncludedList.Len() {
		return nil, nil, errors.New("invalid message. Wrong bitmap length.")
	}

	cellIndices := make([]uint64, 0, includedCells)
	cellsToVerify := make([]CellProofBundle, 0, includedCells)
	// Filter out cells we already have
	for i := range included.Len() {
		if len(message.PartialColumn) == 0 {
			break
		}
		if !included.BitAt(i) {
			continue
		}

		if !ourIncludedList.BitAt(i) {
			cellIndices = append(cellIndices, i)
			cellsToVerify = append(cellsToVerify, CellProofBundle{
				ColumnIndex: p.Index,
				Cell:        message.PartialColumn[0],
				Proof:       message.KzgProofs[0],
				// Use the commitment from our datacolumn, indexed by i since we
				// have all commitments.
				Commitment: p.KzgCommitments[i],
			})
		}
		message.PartialColumn = message.PartialColumn[1:]
		message.KzgProofs = message.KzgProofs[1:]
	}
	return cellIndices, cellsToVerify, nil
}

// ExtendFromVerfifiedCells will extend this partial column with the provided verified cells
func (p *PartialDataColumn) ExtendFromVerfifiedCell(cellIndex uint64, cell, proof []byte) bool {
	if p.Included.BitAt(cellIndex) {
		// We already have this cell
		return false
	}

	p.Included.SetBitAt(cellIndex, true)
	p.Column[cellIndex] = cell
	p.KzgProofs[cellIndex] = proof
	return true
}

// ExtendFromVerfifiedCells will extend this partial column with the provided verified cells
func (p *PartialDataColumn) ExtendFromVerfifiedCells(cellIndices []uint64, cells []CellProofBundle) /* extended */ bool {
	var extended bool
	for i, bundle := range cells {
		if bundle.ColumnIndex != p.Index {
			// Invalid column index, shouldn't happen
			return false
		}
		if p.ExtendFromVerfifiedCell(cellIndices[i], bundle.Cell, bundle.Proof) {
			extended = true
		}
	}
	return extended
}

func (p *PartialDataColumn) Complete(logger *logrus.Logger) (VerifiedRODataColumn, bool) {
	if uint64(len(p.KzgCommitments)) != p.Included.Count() {
		return VerifiedRODataColumn{}, false
	}

	rodc, err := NewRODataColumn(p.DataColumnSidecar)
	if err != nil {
		// We shouldn't get an error, as we check the hash root when creating
		// the partial column
		logger.Error("failed to create RODataColumn", "err", err)
		return VerifiedRODataColumn{}, false
	}

	return NewVerifiedRODataColumn(rodc), true
}

func allOnesBitlist(length uint64) bitfield.Bitlist {
	bl := bitfield.NewBitlist(length)
	for i := range length {
		bl.SetBitAt(i, true)
	}
	return bl
}
