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
	"github.com/sirupsen/logrus"
)

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

	// partsRequests overrides the request bitmap in parts metadata. This is used
	// when we know which parts to request from other peers before we actually fetch cells from the EL.
	partsRequests bitfield.Bitlist
}

// NewPartialDataColumnFromVerifiedRODataColumn builds a PartialDataColumn from
// an already verified RO data column, marking all of its cells as included.
func NewPartialDataColumnFromVerifiedRODataColumn(c VerifiedRODataColumn) (PartialDataColumn, error) {
	commitments, err := c.KzgCommitments()
	if err != nil {
		return PartialDataColumn{}, errors.Wrap(err, "get KZG commitments")
	}
	included := bitfield.NewBitlist(uint64(len(commitments)))
	included = included.Not()

	return PartialDataColumn{
		DataColumnSidecar: c.DataColumnSidecar(),
		root:              c.root,
		Included:          included,
		groupID:           groupIdFromRoot(c.root),
	}, nil
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
	root [fieldparams.RootLength]byte,
	signedBlockHeader *ethpb.SignedBeaconBlockHeader,
	columnIndex uint64,
	kzgCommitments [][]byte,
	kzgInclusionProof [][]byte,
) (PartialDataColumn, error) {
	if signedBlockHeader == nil {
		return PartialDataColumn{}, errors.New("signedBlockHeader is nil")
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

// GroupID returns the libp2p partial-messages group identifier. It returns a
// copy so callers cannot mutate the internal group identifier.
func (p *PartialDataColumn) GroupID() []byte {
	return slices.Clone(p.groupID)
}

func (p *PartialDataColumn) newPartsMetadata() (*ethpb.PartialDataColumnPartsMetadata, error) {
	available := slices.Clone(p.Included)
	missing := slices.Clone(p.Included).Not()
	requests := missing
	if p.partsRequests != nil {
		var err error
		requests, err = slices.Clone(p.partsRequests).And(missing)
		if err != nil {
			return nil, errors.Wrap(err, "intersect parts requests with missing cells")
		}
	}

	return &ethpb.PartialDataColumnPartsMetadata{
		Available: available,
		Requests:  requests,
	}, nil
}

// SetPartsRequests overrides the request bitmap emitted in parts metadata.
func (p *PartialDataColumn) SetPartsRequests(requests bitfield.Bitlist) error {
	if requests.Len() != uint64(len(p.KzgCommitments)) {
		return errors.Errorf("parts requests length mismatch: got %d, want %d", requests.Len(), len(p.KzgCommitments))
	}
	p.partsRequests = slices.Clone(requests)
	return nil
}

// ClearPartsRequests removes any request bitmap override.
func (p *PartialDataColumn) ClearPartsRequests() {
	p.partsRequests = nil
}

// PartsRequests returns a cloned request bitmap override, if one is set.
func (p *PartialDataColumn) PartsRequests() (bitfield.Bitlist, bool) {
	if p.partsRequests == nil {
		return nil, false
	}
	return slices.Clone(p.partsRequests), true
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
		return nil, errors.Wrap(err, "marshal parts metadata")
	}
	return partialmessages.PartsMetadata(b), nil
}

// Clone creates a deep copy of the PeerState. It clones the Recvd and Sent
// fields when they are non-nil, ensuring that modifications to the returned
// state do not affect the original.
func (peerState PartialDataColumnPeerState) Clone() PartialDataColumnPeerState {
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

// KzgCommitmentCount returns the number of KZG commitments in the block header
// for this column, which in turn is equal to the number of cells in this column.
func (p *PartialDataColumn) KzgCommitmentCount() uint64 {
	return uint64(len(p.KzgCommitments))
}

// cellsToSendToPeer computes the cells to send to a peer based on its parts
// metadata and returns them as an SSZ-encoded PartialDataColumnSidecar along
// with the bitmap of the cells actually sent.
//
// A cell is sent only if all of the following hold:
//   - the peer requested it (peerMeta.Requests),
//   - we have it (p.Included), and
//   - the peer does not already have it (peerMeta.Available).
//
// The peer is allowed to request cells it already has, but we filter those out
// to save bandwidth. If there is nothing to send, it returns (nil, nil, nil).
func (p *PartialDataColumn) cellsToSendToPeer(peerMeta *ethpb.PartialDataColumnPartsMetadata) (encodedMsg []byte, cellsSent bitfield.Bitlist, err error) {
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

	nCells := meetsNeeds.Count()

	// Nothing to send
	if nCells == 0 {
		return nil, nil, nil
	}

	size := meetsNeeds.Len()
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
		return nil, nil, errors.Wrap(err, "marshal partial data column sidecar")
	}
	return marshalled, meetsNeeds, nil
}

func (p *PartialDataColumn) buildPartialColumnHeader() (encoded []byte, err error) {
	outMessage := &ethpb.PartialDataColumnSidecar{
		Header: []*ethpb.PartialDataColumnHeader{{
			KzgCommitments:               p.KzgCommitments,
			SignedBlockHeader:            p.SignedBlockHeader,
			KzgCommitmentsInclusionProof: p.KzgCommitmentsInclusionProof,
		}},
		CellsPresentBitmap: bitfield.NewBitlist(uint64(len(p.KzgCommitments))),
	}
	encoded, err = outMessage.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "marshal partial column header")
	}
	return encoded, nil
}

// PartsMetadata returns SSZ-encoded PartialDataColumnPartsMetadata.
func (p *PartialDataColumn) PartsMetadata() (partialmessages.PartsMetadata, error) {
	meta, err := p.newPartsMetadata()
	if err != nil {
		return nil, errors.Wrap(err, "new parts metadata")
	}
	return marshalPartsMetadata(meta)
}

// MergeAvailableIntoPartsMetadata returns a new parts metadata whose available
// cells are the union of base's available cells and additionalAvailable. The
// base argument is not modified.
func MergeAvailableIntoPartsMetadata(base *ethpb.PartialDataColumnPartsMetadata, additionalAvailable bitfield.Bitlist) (*ethpb.PartialDataColumnPartsMetadata, error) {
	if base == nil {
		return nil, errors.New("base is nil")
	}

	if base.Available.Len() != additionalAvailable.Len() {
		return nil, errors.New("available length mismatch")
	}
	if base.Requests.Len() != additionalAvailable.Len() {
		return nil, errors.New("requests length mismatch")
	}
	merged, err := base.Available.Or(additionalAvailable)
	if err != nil {
		return nil, errors.Wrap(err, "merge available cells")
	}
	return &ethpb.PartialDataColumnPartsMetadata{
		Available: merged,
		Requests:  slices.Clone(base.Requests),
	}, nil
}

// PublishActionsFn returns a PublishActionsFn that, for each known peer, computes
// the next peer state and the publish action to send to that peer. headerSentCache
// tracks whether the block header has already been sent to a peer so it is only
// included once; it is updated as actions are produced. onEagerPush, if non-nil,
// is invoked for each peer that was eager pushed to.
func (p *PartialDataColumn) PublishActionsFn(headerSentCache map[peer.ID]bool, onEagerPush func(peer.ID)) partialmessages.PublishActionsFn[PartialDataColumnPeerState] {
	return func(peerStates map[peer.ID]PartialDataColumnPeerState, peerRequestsPartial func(peer.ID) bool) iter.Seq2[peer.ID, partialmessages.PublishAction] {
		return p.publishActions(peerStates, peerRequestsPartial, headerSentCache, onEagerPush)
	}
}

// publishActions yields the publish action for each peer in peerStates. On a
// successful action it updates peerStates with the next state and records sent
// headers in headerSentCache.
func (p *PartialDataColumn) publishActions(
	peerStates map[peer.ID]PartialDataColumnPeerState,
	peerRequestsPartial func(peer.ID) bool,
	headerSentCache map[peer.ID]bool,
	onEagerPush func(peer.ID),
) iter.Seq2[peer.ID, partialmessages.PublishAction] {
	return func(yield func(peer.ID, partialmessages.PublishAction) bool) {
		for peerID, peerState := range peerStates {
			requested := peerRequestsPartial(peerID)
			nextState, action, includeHeader := p.forPeer(peerID, requested, peerState, !headerSentCache[peerID])
			// Only update state if there was no error.
			if action.Err == nil {
				if onEagerPush != nil && isEagerPush(requested, peerState) {
					onEagerPush(peerID)
				}
				p.recordHeaderSent(peerID, includeHeader, headerSentCache)
				peerStates[peerID] = nextState
			}
			if !yield(peerID, action) {
				return
			}
		}
	}
}

// recordHeaderSent marks in headerSentCache that the header was sent to peerID if
// includeHeader is true, logging the transition when the cached value changes.
func (p *PartialDataColumn) recordHeaderSent(peerID peer.ID, includeHeader bool, headerSentCache map[peer.ID]bool) {
	prev := headerSentCache[peerID]
	headerSentCache[peerID] = prev || includeHeader
}

func isEagerPush(requestedMessage bool, peerState PartialDataColumnPeerState) bool {
	return requestedMessage && peerState.Recvd == nil
}

// forPeer returns the next peer state and the publish action for this peer
func (p *PartialDataColumn) forPeer(remote peer.ID, requestedMessage bool, peerState PartialDataColumnPeerState, includeHeader bool) (PartialDataColumnPeerState, partialmessages.PublishAction, bool) {
	peerState = peerState.Clone()

	// Eager push - we don't know what the peer has and message has been requested.
	// Set RecvdState so subsequent calls skip the eager push path.
	if isEagerPush(requestedMessage, peerState) {
		var encoded []byte
		if includeHeader {
			var err error
			encoded, err = p.buildPartialColumnHeader()
			if err != nil {
				return peerState, partialmessages.PublishAction{Err: err}, false
			}
		}
		myPartsMeta, err := p.newPartsMetadata()
		if err != nil {
			return peerState, partialmessages.PublishAction{Err: err}, false
		}
		peerState.Recvd = NewPartsMetaWithNoAvailableAndNoRequests(p.KzgCommitmentCount())
		// We're sending our parts metadata so update the sent state i.e. the peer's view of what we have.
		peerState.Sent = myPartsMeta
		encodedMeta, err := marshalPartsMetadata(myPartsMeta)
		if err != nil {
			return peerState, partialmessages.PublishAction{Err: err}, false
		}
		return peerState, partialmessages.PublishAction{
			EncodedPartialMessage: encoded,
			EncodedPartsMetadata:  encodedMeta,
		}, includeHeader
	}

	var cellsSent bitfield.Bitlist
	sentMeta := peerState.Sent
	recvdMeta := peerState.Recvd
	var encodedMsg []byte

	//  Normal - message requested and we have RecvdState.
	if requestedMessage && recvdMeta != nil {
		var err error
		encodedMsg, cellsSent, err = p.cellsToSendToPeer(recvdMeta)
		if err != nil {
			return peerState, partialmessages.PublishAction{Err: err}, false
		}
		if cellsSent != nil && cellsSent.Count() != 0 {
			newRecvd, err := MergeAvailableIntoPartsMetadata(recvdMeta, cellsSent)
			if err != nil {
				return peerState, partialmessages.PublishAction{Err: err}, false
			}
			peerState.Recvd = newRecvd
		}
	}

	//  Check if we need to send partsMetadata.
	var partsMetadataToSend partialmessages.PartsMetadata
	myPartsMeta, err := p.newPartsMetadata()
	if err != nil {
		return peerState, partialmessages.PublishAction{Err: err}, false
	}
	var shouldSendPartsMetadata bool

	if sentMeta != nil {
		if !bytes.Equal(sentMeta.Requests, myPartsMeta.Requests) {
			shouldSendPartsMetadata = true
		} else {
			contains, err := sentMeta.Available.Contains(myPartsMeta.Available)
			if err != nil {
				return peerState, partialmessages.PublishAction{Err: errors.Wrap(err, "check available parts metadata containment")}, false
			}
			shouldSendPartsMetadata = !contains
		}
	}

	if sentMeta == nil || shouldSendPartsMetadata {
		var err error
		partsMetadataToSend, err = marshalPartsMetadata(myPartsMeta)
		if err != nil {
			return peerState, partialmessages.PublishAction{Err: err}, false
		}
		if sentMeta == nil {
			peerState.Sent = myPartsMeta
		} else {
			sentMeta, err = MergeAvailableIntoPartsMetadata(sentMeta, myPartsMeta.Available)
			if err != nil {
				return peerState, partialmessages.PublishAction{Err: err}, false
			}
			sentMeta.Requests = myPartsMeta.Requests
			peerState.Sent = sentMeta
		}
	}

	return peerState, partialmessages.PublishAction{
		EncodedPartialMessage: encodedMsg,
		EncodedPartsMetadata:  partsMetadataToSend,
	}, false
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
	// Filter out cells we already have.
	// j tracks position in the compact PartialColumn/KzgProofs arrays.
	var j int
	for i := range included.Len() {
		if !included.BitAt(i) {
			continue
		}
		if j >= len(message.PartialColumn) {
			break
		}

		if !ourIncludedList.BitAt(i) {
			cellIndices = append(cellIndices, i)
			cellsToVerify = append(cellsToVerify, CellProofBundle{
				ColumnIndex: p.Index,
				Cell:        message.PartialColumn[j],
				Proof:       message.KzgProofs[j],
				// Use the commitment from our datacolumn, indexed by i since we
				// have all commitments.
				Commitment: p.KzgCommitments[i],
			})
		}
		j++
	}
	return cellIndices, cellsToVerify, nil
}

// ExtendFromVerifiedCell extends this partial column with one verified cell. It
// returns false without modifying the column if the cell is already present or
// if cellIndex is out of range.
func (p *PartialDataColumn) ExtendFromVerifiedCell(cellIndex uint64, cell, proof []byte) bool {
	if cellIndex >= uint64(len(p.Column)) || cellIndex >= uint64(len(p.KzgProofs)) {
		log.WithFields(logrus.Fields{
			"index":        p.Index,
			"cellIndex":    cellIndex,
			"columnLen":    len(p.Column),
			"kzgProofsLen": len(p.KzgProofs),
		}).Error("Cell index out of range for partial data column")
		return false
	}
	if p.Included.BitAt(cellIndex) {
		// We already have this cell
		return false
	}

	p.Included.SetBitAt(cellIndex, true)
	p.Column[cellIndex] = cell
	p.KzgProofs[cellIndex] = proof
	return true
}

// IsComplete returns true if all cells are now present in this column.
func (p *PartialDataColumn) IsComplete() bool {
	return uint64(len(p.KzgCommitments)) == p.Included.Count()
}
