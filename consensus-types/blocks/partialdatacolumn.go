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
func (p *PartialDataColumn) PartialMessageBytes(metadata partialmessages.PartsMetadata) ([]byte, error) {
	numCommitments := p.Included.Len()
	peerAvailable, peerRequests, isNewFormat := ParseMetadata(metadata, numCommitments)
	if peerAvailable.Len() != numCommitments {
		return nil, errors.New("metadata length does not match expected length")
	}
	if isNewFormat && peerRequests.Len() != numCommitments {
		return nil, errors.New("metadata length does not match expected length")
	}

	// shouldSend returns true if we should send cell i to this peer.
	shouldSend := func(i uint64) bool {
		if !p.Included.BitAt(i) {
			return false
		}
		if peerAvailable.BitAt(i) {
			return false
		}
		if isNewFormat && !peerRequests.BitAt(i) {
			return false
		}
		return true
	}

	var cellsToReturn int
	for i := range numCommitments {
		if shouldSend(i) {
			cellsToReturn++
		}
	}
	if cellsToReturn == 0 {
		return nil, nil
	}

	included := bitfield.NewBitlist(numCommitments)
	outMessage := ethpb.PartialDataColumnSidecar{
		CellsPresentBitmap: included,
		PartialColumn:      make([][]byte, 0, cellsToReturn),
		KzgProofs:          make([][]byte, 0, cellsToReturn),
	}
	for i := range numCommitments {
		if !shouldSend(i) {
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

// TODO: This method will be removed after upgrading to the latest Gossipsub.
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
	// Empty bitlist since we aren't including any cells here
	peersNextParts := partialmessages.PartsMetadata(bitfield.NewBitlist(uint64(len(p.KzgCommitments))))

	return marshalled, peersNextParts, nil
}

func (p *PartialDataColumn) PartsMetadata() partialmessages.PartsMetadata {
	n := p.Included.Len()
	requests := bitfield.NewBitlist(n)
	for i := range n {
		requests.SetBitAt(i, true)
	}
	return combinedMetadata(p.Included, requests)
}

// ParseMetadata splits PartsMetadata into available and request bitlists.
// Old format (len==N): returns (metadata, nil, false)
// New format (len==2N): returns (first N bits, next N bits, true)
func ParseMetadata(metadata partialmessages.PartsMetadata, numCommitments uint64) (available bitfield.Bitlist, requests bitfield.Bitlist, isNewFormat bool) {
	bl := bitfield.Bitlist(metadata)
	if bl.Len() == 2*numCommitments {
		available = bitfield.NewBitlist(numCommitments)
		requests = bitfield.NewBitlist(numCommitments)
		for i := range numCommitments {
			available.SetBitAt(i, bl.BitAt(i))
			requests.SetBitAt(i, bl.BitAt(i+numCommitments))
		}
		return available, requests, true
	}
	return bl, nil, false
}

func combinedMetadata(available, requests bitfield.Bitlist) partialmessages.PartsMetadata {
	n := available.Len()
	combined := bitfield.NewBitlist(2 * n)
	for i := range n {
		combined.SetBitAt(i, available.BitAt(i))
		combined.SetBitAt(i+n, requests.BitAt(i))
	}
	return partialmessages.PartsMetadata(combined)
}

// MergePartsMetadata merges two PartsMetadata values, handling old (N) and new (2N) formats.
// If lengths differ, the old-format (N) is extended to new-format (2N) with all request bits set to 1.
// TODO: This method will be removed after upgrading to the latest Gossipsub.
func MergePartsMetadata(left, right partialmessages.PartsMetadata) (partialmessages.PartsMetadata, error) {
	if len(left) == 0 {
		return right, nil
	}
	if len(right) == 0 {
		return left, nil
	}
	leftBl := bitfield.Bitlist(left)
	rightBl := bitfield.Bitlist(right)
	if leftBl.Len() != rightBl.Len() {
		leftBl, rightBl = normalizeMetadataLengths(leftBl, rightBl)
	}
	merged, err := leftBl.Or(rightBl)
	if err != nil {
		return nil, err
	}
	return partialmessages.PartsMetadata(merged), nil
}

func normalizeMetadataLengths(left, right bitfield.Bitlist) (bitfield.Bitlist, bitfield.Bitlist) {
	if left.Len() < right.Len() {
		left = extendToNewFormat(left)
	} else {
		right = extendToNewFormat(right)
	}
	return left, right
}

func extendToNewFormat(bl bitfield.Bitlist) bitfield.Bitlist {
	n := bl.Len()
	extended := bitfield.NewBitlist(2 * n)
	for i := range n {
		extended.SetBitAt(i, bl.BitAt(i))
		extended.SetBitAt(i+n, true)
	}
	return extended
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
