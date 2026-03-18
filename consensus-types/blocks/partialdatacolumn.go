package blocks

import (
	"bytes"
	"iter"
	"slices"

	"github.com/OffchainLabs/go-bitfield"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/libp2p/go-libp2p-pubsub/partialmessages"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

var _ partialmessages.PublishActionsFn[PartialDataColumnPeerState] = (*PartialDataColumn)(nil).PublishActions

// CellProofBundle contains a cell, its proof, and the corresponding
// commitment/index information.
type CellProofBundle struct {
	ColumnIndex uint64
	Commitment  []byte
	Cell        []byte
	Proof       []byte
}

type PartialDataColumnPeerState struct {
	Sent  *ethpb.PartialDataColumnPartsMetadata
	Recvd *ethpb.PartialDataColumnPartsMetadata
}

// PartialDataColumn is a partially populated DataColumnSidecar used for
// exchanging cells with peers.
type PartialDataColumn struct {
	*ethpb.DataColumnSidecar
	root    [fieldparams.RootLength]byte
	groupID []byte

	Included bitfield.Bitlist

	// set to true when the node itself has Published this column. We only want
	// to republish in response to an incoming RPC after we publish this column
	// ourselves, as that is the point we know what cells we have or are
	// missing.
	Published bool
}

func NewPartialDataColumnFromVerifiedRODataColumn(c VerifiedRODataColumn) PartialDataColumn {
	included := bitfield.NewBitlist(uint64(len(c.KzgCommitments)))
	included = included.Not()

	return PartialDataColumn{
		DataColumnSidecar: c.DataColumnSidecar,
		root:              c.root,
		Included:          included,
		groupID:           groupIdFromRoot(c.root),
	}
}

func groupIdFromRoot(root [fieldparams.RootLength]byte) []byte {
	groupID := make([]byte, len(root)+1)
	copy(groupID[1:], root[:])
	// Version 0
	groupID[0] = 0
	return groupID
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

	c := PartialDataColumn{
		DataColumnSidecar: sidecar,
		root:              root,
		groupID:           groupIdFromRoot(root),
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

// ClonePeerState creates a deep copy of the given PeerState. It clones the
// RecvdState and SentState fields if they are of type *PartialDataColumnPartsMetadata,
// ensuring that modifications to the returned state do not affect the original.
func ClonePeerState(peerState PartialDataColumnPeerState) PartialDataColumnPeerState {
	clonePartsMetadataF := func(meta *ethpb.PartialDataColumnPartsMetadata) *ethpb.PartialDataColumnPartsMetadata {
		if meta == nil {
			return nil
		}
		return &ethpb.PartialDataColumnPartsMetadata{
			Available: slices.Clone(meta.Available),
			Requests:  slices.Clone(meta.Requests),
		}
	}

	var nextPeerState PartialDataColumnPeerState
	nextPeerState.Sent = clonePartsMetadataF(peerState.Sent)
	nextPeerState.Recvd = clonePartsMetadataF(peerState.Recvd)
	return nextPeerState
}

// NKzgCommitments returns the number of commitments in the block header for this column which
// in turn will be equal to the number of cells in this column.
func (p *PartialDataColumn) NKzgCommitments() uint64 {
	return p.Included.Len()
}

func (p *PartialDataColumn) cellsToSendForPeer(peerMeta *ethpb.PartialDataColumnPartsMetadata) (encodedMsg []byte, cellsSent bitfield.Bitlist, err error) {
	// We have it and the peer requested it.
	meetsRequests, err := peerMeta.Requests.And(p.Included)
	if err != nil {
		return nil, nil, errors.Wrap(err, "peer metadata bitmap length mismatch - requests")
	}
	// Even though the bitmaps provide the flexibility for the peer to request cells it has, we will still save bandwidth by filtering those out.
	meetsNeeds, err := meetsRequests.And(peerMeta.Available.Not())
	if err != nil {
		return nil, nil, errors.Wrap(err, "peer metadata bitmap length mismatch - available")
	}

	size := meetsNeeds.Len()

	// Nothing to send
	if meetsNeeds.Count() == 0 {
		return nil, nil, nil
	}

	nCells := meetsNeeds.Count()
	out := ethpb.PartialDataColumnSidecar{
		PartialColumn:      make([][]byte, 0, nCells),
		KzgProofs:          make([][]byte, 0, nCells),
		CellsPresentBitmap: meetsNeeds,
	}
	for i := range size {
		if meetsNeeds.BitAt(i) {
			out.PartialColumn = append(out.PartialColumn, p.Column[i])
			out.KzgProofs = append(out.KzgProofs, p.KzgProofs[i])
		}
	}

	marshalled, err := out.MarshalSSZ()
	if err != nil {
		return nil, nil, err
	}
	return marshalled, meetsNeeds, nil
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

// MergeAvailableIntoPartsMetadata merges additional available cells into the base partsmetadata's available cells.
func MergeAvailableIntoPartsMetadata(base *ethpb.PartialDataColumnPartsMetadata, additionalAvailable bitfield.Bitlist) (*ethpb.PartialDataColumnPartsMetadata, error) {
	if base == nil {
		return nil, errors.New("base is nil")
	}
	if base.Requests.Len() != additionalAvailable.Len() {
		return nil, errors.New("requests length mismatch")
	}
	merged, err := base.Available.Or(additionalAvailable)
	if err != nil {
		return nil, err
	}
	base.Available = merged
	return base, nil
}

func (p *PartialDataColumn) PublishActions(peerStates map[peer.ID]PartialDataColumnPeerState, peerRequestsPartial func(peer.ID) bool) iter.Seq2[peer.ID, partialmessages.PublishAction] {
	return func(yield func(peer.ID, partialmessages.PublishAction) bool) {
		for peer, peerState := range peerStates {
			nextState, action := p.forPeer(peer, peerRequestsPartial(peer), peerState)
			if action.Err == nil {
				// Only update state if there was no error.
				peerStates[peer] = nextState
			}
			if !yield(peer, action) {
				return
			}
		}
	}
}

// forPeer returns the next peer state and the publish action for this peer
func (p *PartialDataColumn) forPeer(remote peer.ID, requestedMessage bool, peerState PartialDataColumnPeerState) (PartialDataColumnPeerState, partialmessages.PublishAction) {
	peerState = ClonePeerState(peerState)

	// Eager push header - we don't know what the peer has and message has been requested.
	// Set RecvdState so subsequent calls skip the eager push path.
	if requestedMessage && peerState.Recvd == nil {
		encoded, err := p.eagerPushBytes()
		if err != nil {
			return peerState, partialmessages.PublishAction{Err: err}
		}
		peerState.Recvd = NewPartsMetaWithNoAvailableAndNoRequests(p.NKzgCommitments())
		myPartsMeta, err := marshalPartsMetadata(p.newPartsMetadata())
		if err != nil {
			return peerState, partialmessages.PublishAction{Err: err}
		}
		return peerState, partialmessages.PublishAction{
			EncodedPartialMessage: encoded,
			EncodedPartsMetadata:  myPartsMeta,
		}
	}

	var cellsSent bitfield.Bitlist
	sentMeta := peerState.Sent
	recvdMeta := peerState.Recvd
	var encodedMsg []byte

	//  Normal - message requested and we have RecvdState.
	if requestedMessage && recvdMeta != nil {
		var err error
		encodedMsg, cellsSent, err = p.cellsToSendForPeer(recvdMeta)
		if err != nil {
			return peerState, partialmessages.PublishAction{Err: err}
		}
		if cellsSent != nil && cellsSent.Count() != 0 {
			newRecvd, err := MergeAvailableIntoPartsMetadata(recvdMeta, cellsSent)
			if err != nil {
				return peerState, partialmessages.PublishAction{Err: err}
			}
			peerState.Recvd = newRecvd
		}
	}

	//  Check if we need to send partsMetadata.
	var partsMetadataToSend partialmessages.PartsMetadata
	myPartsMeta := p.newPartsMetadata()
	var shouldSendPartsMetadata bool

	if sentMeta != nil {
		if !bytes.Equal(sentMeta.Requests, myPartsMeta.Requests) {
			shouldSendPartsMetadata = true
		} else {
			contains, err := sentMeta.Available.Contains(myPartsMeta.Available)
			if err != nil {
				return peerState, partialmessages.PublishAction{Err: err}
			}
			shouldSendPartsMetadata = !contains
		}
	}

	if sentMeta == nil || shouldSendPartsMetadata {
		var err error
		partsMetadataToSend, err = marshalPartsMetadata(myPartsMeta)
		if err != nil {
			return peerState, partialmessages.PublishAction{Err: err}
		}
		if sentMeta == nil {
			peerState.Sent = myPartsMeta
		} else {
			sentMeta, err = MergeAvailableIntoPartsMetadata(sentMeta, myPartsMeta.Available)
			if err != nil {
				return peerState, partialmessages.PublishAction{Err: err}
			}
			sentMeta.Requests = myPartsMeta.Requests
			peerState.Sent = sentMeta
		}
	}

	return peerState, partialmessages.PublishAction{
		EncodedPartialMessage: encodedMsg,
		EncodedPartsMetadata:  partsMetadataToSend,
	}
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

// IsComplete returns true if all cells are now present in this column.
func (p *PartialDataColumn) IsComplete() bool {
	return uint64(len(p.KzgCommitments)) == p.Included.Count()
}
