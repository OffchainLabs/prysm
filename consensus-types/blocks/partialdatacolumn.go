package blocks

import (
	"bytes"
	"errors"
	"slices"

	"github.com/OffchainLabs/go-bitfield"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/libp2p/go-libp2p-pubsub/partialmessages"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

var _ partialmessages.Message = (*PartialDataColumn)(nil)

// CellProofBundle contains a cell, its proof, and the corresponding
// commitment/index information.
type CellProofBundle struct {
	ColumnIndex uint64
	Commitment  []byte
	Cell        []byte
	Proof       []byte
}

// PartialDataColumn is a partially populated DataColumnSidecar used for
// exchanging cells with peers.
type PartialDataColumn struct {
	*ethpb.DataColumnSidecar
	root    [fieldparams.RootLength]byte
	groupID []byte

	Included bitfield.Bitlist
}

// NewPartialDataColumn creates a new Partial Data Column for the given block.
// It does not validate the inputs. The caller is responsible for validating the
// block header and KZG Commitment Inclusion proof.
func NewPartialDataColumn(
	signedBlockHeader *ethpb.SignedBeaconBlockHeader,
	columnIndex uint64,
	kzgCommitments [][]byte,
	kzgInclusionProof [][]byte,
) (PartialDataColumn, error) {
	if signedBlockHeader == nil {
		return PartialDataColumn{}, errors.New("signedBlockHeader is nil")
	}
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
	return c, nil
}

// GroupID returns the libp2p partial-messages group identifier.
func (p *PartialDataColumn) GroupID() []byte {
	return p.groupID
}

func (p *PartialDataColumn) newPartsMetadata() *ethpb.PartialDataColumnPartsMetadata {
	n := uint64(len(p.KzgCommitments))
	available := slices.Clone(p.Included)
	requests := bitfield.NewBitlist(n)
	requests = requests.Not()
	return &ethpb.PartialDataColumnPartsMetadata{
		Available: available,
		Requests:  requests,
	}
}

// NewPartsMetaWithNoAvailableAndNoRequests creates metadata for n parts where
// no parts are marked as available and no requests are set.
func NewPartsMetaWithNoAvailableAndNoRequests(n uint64) *ethpb.PartialDataColumnPartsMetadata {
	return &ethpb.PartialDataColumnPartsMetadata{
		Available: bitfield.NewBitlist(n),
		Requests:  bitfield.NewBitlist(n),
	}
}

func marshalPartsMetadata(meta *ethpb.PartialDataColumnPartsMetadata) (partialmessages.PartsMetadata, error) {
	b, err := meta.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return partialmessages.PartsMetadata(b), nil
}

// NKzgCommitments returns the number of commitments in the block header for this column which
// in turn will be equal to the number of cells in this column.
func (p *PartialDataColumn) NKzgCommitments() uint64 {
	return p.Included.Len()
}

// ParsePartsMetadata SSZ-decodes bytes back to PartialDataColumnPartsMetadata.
func ParsePartsMetadata(pm partialmessages.PartsMetadata, expectedLength uint64) (*ethpb.PartialDataColumnPartsMetadata, error) {
	meta := &ethpb.PartialDataColumnPartsMetadata{}
	if err := meta.UnmarshalSSZ(pm); err != nil {
		return nil, err
	}
	if meta.Available.Len() != expectedLength || meta.Requests.Len() != expectedLength {
		return nil, errors.New("invalid parts metadata length")
	}
	return meta, nil
}

func (p *PartialDataColumn) cellsToSendForPeer(peerMeta *ethpb.PartialDataColumnPartsMetadata) (encodedMsg []byte, cellsSent bitfield.Bitlist, err error) {
	peerAvailable := bitfield.Bitlist(peerMeta.Available)
	peerRequests := bitfield.Bitlist(peerMeta.Requests)

	n := p.Included.Len()
	if peerAvailable.Len() != n || peerRequests.Len() != n {
		return nil, nil, errors.New("peer metadata bitmap length mismatch")
	}

	var cellsToReturn int
	for i := range n {
		if p.Included.BitAt(i) && !peerAvailable.BitAt(i) && peerRequests.BitAt(i) {
			cellsToReturn++
		}
	}
	if cellsToReturn == 0 {
		return nil, nil, nil
	}

	included := bitfield.NewBitlist(n)
	outMessage := ethpb.PartialDataColumnSidecar{
		PartialColumn: make([][]byte, 0, cellsToReturn),
		KzgProofs:     make([][]byte, 0, cellsToReturn),
	}
	for i := range n {
		if !p.Included.BitAt(i) || peerAvailable.BitAt(i) || !peerRequests.BitAt(i) {
			continue
		}
		included.SetBitAt(i, true)
		outMessage.PartialColumn = append(outMessage.PartialColumn, p.Column[i])
		outMessage.KzgProofs = append(outMessage.KzgProofs, p.KzgProofs[i])
	}
	outMessage.CellsPresentBitmap = included

	marshalled, err := outMessage.MarshalSSZ()
	if err != nil {
		return nil, nil, err
	}
	return marshalled, included, nil
}

// eagerPushBytes builds SSZ-encoded PartialDataColumnSidecar with header only (no cells).
func (p *PartialDataColumn) eagerPushBytes() ([]byte, error) {
	outHeader := &ethpb.PartialDataColumnHeader{
		KzgCommitments:               p.KzgCommitments,
		SignedBlockHeader:            p.SignedBlockHeader,
		KzgCommitmentsInclusionProof: p.KzgCommitmentsInclusionProof,
	}
	outMessage := &ethpb.PartialDataColumnSidecar{
		CellsPresentBitmap: bitfield.NewBitlist(uint64(len(p.KzgCommitments))),
		Header:             []*ethpb.PartialDataColumnHeader{outHeader},
	}
	return outMessage.MarshalSSZ()
}

// PartsMetadata returns SSZ-encoded PartialDataColumnPartsMetadata.
func (p *PartialDataColumn) PartsMetadata() (partialmessages.PartsMetadata, error) {
	meta := p.newPartsMetadata()
	return marshalPartsMetadata(meta)
}

// MergeAvailableIntoPartsMetadata merges additional availabe cells into the base partsmetadata's available cells and
// returns the marshaled metadata.
func MergeAvailableIntoPartsMetadata(base *ethpb.PartialDataColumnPartsMetadata, additionalAvailable bitfield.Bitlist) (partialmessages.PartsMetadata, error) {
	if base == nil {
		return nil, errors.New("base is nil")
	}
	if base.Requests.Len() != additionalAvailable.Len() {
		return nil, errors.New("requests length mismatch")
	}
	merged, err := bitfield.Bitlist(base.Available).Or(additionalAvailable)
	if err != nil {
		return nil, err
	}
	base.Available = merged
	return marshalPartsMetadata(base)
}

// merge available, keep request unchanged, if my parts are different, simply over write with myparts
func (p *PartialDataColumn) updateReceivedStateOutgoing(receivedMeta partialmessages.PartsMetadata, cellsSent bitfield.Bitlist) (partialmessages.PartsMetadata, error) {
	if len(receivedMeta) == 0 {
		return nil, errors.New("receivedMeta is empty")
	}
	peerMeta, err := ParsePartsMetadata(receivedMeta, p.Included.Len())
	if err != nil {
		return nil, err
	}
	return MergeAvailableIntoPartsMetadata(peerMeta, cellsSent)
}

// ForPeer implements partialmessages.Message.
func (p *PartialDataColumn) ForPeer(remote peer.ID, requestedMessage bool, peerState partialmessages.PeerState) (partialmessages.PeerState, []byte,
	partialmessages.PartsMetadata, error) {
	// Eager push header - we don't know what the peer has and message has been requested.
	// Set RecvdState so subsequent calls skip the eager push path.
	if requestedMessage && peerState.RecvdState == nil {
		encoded, err := p.eagerPushBytes()
		if err != nil {
			return peerState, nil, nil, err
		}
		noAvailNoReq := NewPartsMetaWithNoAvailableAndNoRequests(p.NKzgCommitments())
		recvdMeta, err := marshalPartsMetadata(noAvailNoReq)
		if err != nil {
			return peerState, nil, nil, err
		}
		peerState.RecvdState = recvdMeta
		return peerState, encoded, nil, nil
	}

	var encodedMsg []byte
	var cellsSent bitfield.Bitlist
	var sentMeta partialmessages.PartsMetadata
	var recvdMeta partialmessages.PartsMetadata
	if peerState.SentState != nil {
		var ok bool
		sentMeta, ok = peerState.SentState.(partialmessages.PartsMetadata)
		// should never happen but checking this for safety
		if !ok {
			return peerState, nil, nil, errors.New("SentState is not PartsMetadata")
		}
	}
	if peerState.RecvdState != nil {
		var ok bool
		recvdMeta, ok = peerState.RecvdState.(partialmessages.PartsMetadata)
		if !ok {
			return peerState, nil, nil, errors.New("RecvdState is not PartsMetadata")
		}
	}

	//  Normal - message requested and we have RecvdState.
	if requestedMessage && peerState.RecvdState != nil {
		peerMeta, err := ParsePartsMetadata(recvdMeta, p.Included.Len())
		if err != nil {
			return peerState, nil, nil, err
		}

		encodedMsg, cellsSent, err = p.cellsToSendForPeer(peerMeta)
		if err != nil {
			return peerState, nil, nil, err
		}
		if cellsSent != nil && cellsSent.Count() != 0 {
			newRecvd, err := p.updateReceivedStateOutgoing(recvdMeta, cellsSent)
			if err != nil {
				return peerState, nil, nil, err
			}
			peerState.RecvdState = newRecvd
		}
	}

	//  Check if we need to send partsMetadata.
	var partsMetadataToSend partialmessages.PartsMetadata
	myPartsMetadata, err := p.PartsMetadata()
	if err != nil {
		return peerState, nil, nil, err
	}

	if !bytes.Equal(myPartsMetadata, sentMeta) {
		partsMetadataToSend = myPartsMetadata
		sentMeta = partialmessages.PartsMetadata(slices.Clone([]byte(myPartsMetadata)))
		peerState.SentState = sentMeta
	}

	return peerState, encodedMsg, partsMetadataToSend, nil
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
		return nil, nil, errors.New("invalid message: wrong bitmap length")
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

// ExtendFromVerifiedCell extends this partial column with one verified cell.
func (p *PartialDataColumn) ExtendFromVerifiedCell(cellIndex uint64, cell, proof []byte) bool {
	if p.Included.BitAt(cellIndex) {
		// We already have this cell
		return false
	}

	p.Included.SetBitAt(cellIndex, true)
	p.Column[cellIndex] = cell
	p.KzgProofs[cellIndex] = proof
	return true
}

// ExtendFromVerifiedCells extends this partial column with the provided verified cells.
func (p *PartialDataColumn) ExtendFromVerifiedCells(cellIndices []uint64, cells []CellProofBundle) /* extended */ bool {
	var extended bool
	for i, bundle := range cells {
		if bundle.ColumnIndex != p.Index {
			// Invalid column index, shouldn't happen
			return false
		}
		if p.ExtendFromVerifiedCell(cellIndices[i], bundle.Cell, bundle.Proof) {
			extended = true
		}
	}
	return extended
}

// Complete returns a verified read-only column if all cells are now present in this column.
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
