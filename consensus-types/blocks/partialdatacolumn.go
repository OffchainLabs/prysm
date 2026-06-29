package blocks

import (
	"bytes"
	"iter"
	"slices"

	"github.com/OffchainLabs/go-bitfield"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/libp2p/go-libp2p-pubsub/partialmessages"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	partialColumnGroupIDVersionFulu  byte = 0x00
	partialColumnGroupIDVersionGloas byte = 0x01
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

// PartialDataColumn is a partially populated data column sidecar used for
// exchanging cells with peers.
type PartialDataColumn struct {
	RODataColumn

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
	if len(commitments) == 0 {
		return PartialDataColumn{}, errors.New("no KZG commitments")
	}

	included := bitfield.NewBitlist(uint64(len(commitments)))
	included = included.Not()

	groupID, err := partialColumnGroupID(c.RODataColumn)
	if err != nil {
		return PartialDataColumn{}, errors.Wrap(err, "compute group id")
	}

	return PartialDataColumn{
		RODataColumn: c.RODataColumn,
		Included:     included,
		groupID:      groupID,
	}, nil
}

func partialColumnGroupID(c RODataColumn) ([]byte, error) {
	if c.IsGloas() {
		return gloasGroupID(c.Slot(), c.BlockRoot())
	}
	return groupIDFromRoot(c.BlockRoot()), nil
}

func groupIDFromRoot(root [fieldparams.RootLength]byte) []byte {
	groupID := make([]byte, 0, len(root)+1)
	groupID = append(groupID, partialColumnGroupIDVersionFulu)
	groupID = append(groupID, root[:]...)
	return groupID
}

func gloasGroupID(slot primitives.Slot, root [fieldparams.RootLength]byte) ([]byte, error) {
	encoded, err := (&ethpb.PartialDataColumnGroupID{
		Slot:            slot,
		BeaconBlockRoot: root[:],
	}).MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "marshal gloas partial column group id")
	}
	groupID := make([]byte, 0, len(encoded)+1)
	groupID = append(groupID, partialColumnGroupIDVersionGloas)
	groupID = append(groupID, encoded...)
	return groupID, nil
}

// ParsePartialColumnGroupID decodes a partial-column group id into its fork,
// slot, and block root. Fulu group ids (0x00 || root) carry no slot, so slot is
// returned as 0 for them.
func ParsePartialColumnGroupID(b []byte) (isGloas bool, slot primitives.Slot, root [fieldparams.RootLength]byte, err error) {
	if len(b) == 0 {
		return false, 0, root, errors.New("empty partial column group id")
	}
	switch b[0] {
	case partialColumnGroupIDVersionFulu:
		if len(b) != fieldparams.RootLength+1 {
			return false, 0, root, errors.Errorf("invalid fulu group id length: got %d, want %d", len(b), fieldparams.RootLength+1)
		}
		copy(root[:], b[1:])
		return false, 0, root, nil
	case partialColumnGroupIDVersionGloas:
		id := &ethpb.PartialDataColumnGroupID{}
		if err := id.UnmarshalSSZ(b[1:]); err != nil {
			return false, 0, root, errors.Wrap(err, "unmarshal gloas group id")
		}
		copy(root[:], id.BeaconBlockRoot)
		return true, id.Slot, root, nil
	default:
		return false, 0, root, errors.Errorf("unknown partial column group id version: %d", b[0])
	}
}

// NewPartialDataColumn creates a new Fulu Partial Data Column for the given
// block.
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

	ro, err := NewRODataColumnWithRoot(sidecar, root)
	if err != nil {
		return PartialDataColumn{}, errors.Wrap(err, "new ro data column")
	}

	return PartialDataColumn{
		RODataColumn: ro,
		groupID:      groupIDFromRoot(root),
		Included:     bitfield.NewBitlist(uint64(len(kzgCommitments))),
	}, nil
}

// NewPartialDataColumnGloas creates a new Gloas Partial Data Column for the given
// block.
func NewPartialDataColumnGloas(
	root [fieldparams.RootLength]byte,
	slot primitives.Slot,
	columnIndex uint64,
	kzgCommitments [][]byte,
) (PartialDataColumn, error) {
	if len(kzgCommitments) == 0 {
		return PartialDataColumn{}, errors.New("kzgCommitments is empty")
	}

	sidecar := &ethpb.DataColumnSidecarGloas{
		Index:           columnIndex,
		Slot:            slot,
		BeaconBlockRoot: root[:],
		Column:          make([][]byte, len(kzgCommitments)),
		KzgProofs:       make([][]byte, len(kzgCommitments)),
	}

	ro, err := NewRODataColumnGloasWithRoot(sidecar, root)
	if err != nil {
		return PartialDataColumn{}, errors.Wrap(err, "new gloas ro data column")
	}
	ro.SetBidCommitments(kzgCommitments)

	groupID, err := gloasGroupID(slot, root)
	if err != nil {
		return PartialDataColumn{}, errors.Wrap(err, "compute group id")
	}

	return PartialDataColumn{
		RODataColumn: ro,
		groupID:      groupID,
		Included:     bitfield.NewBitlist(uint64(len(kzgCommitments))),
	}, nil
}

// GroupID returns the libp2p partial-messages group identifier. It returns a
// copy so callers cannot mutate the internal group identifier.
func (p *PartialDataColumn) GroupID() []byte {
	return slices.Clone(p.groupID)
}

func (p *PartialDataColumn) newPartsMetadata() (*ethpb.PartialDataColumnPartsMetadata, error) {
	available := slices.Clone(p.Included)
	missing := p.Included.Not()
	requests := missing
	if p.partsRequests != nil {
		var err error
		requests, err = p.partsRequests.And(missing)
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
	if requests.Len() != p.Included.Len() {
		return errors.Errorf("parts requests length mismatch: got %d, want %d", requests.Len(), p.Included.Len())
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

// KzgCommitmentCount returns the number of cells in this column, which equals the
// number of KZG commitments.
func (p *PartialDataColumn) KzgCommitmentCount() uint64 {
	return p.Included.Len()
}

// cellsToSendToPeer computes the cells to send to a peer based on its parts
// metadata and returns them as an SSZ-encoded cell message along with the bitmap
// of the cells actually sent. The wire message is fork-specific: Gloas columns
// emit a PartialDataColumnSidecarGloas, Fulu columns a PartialDataColumnSidecar.
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
	column := p.Column()
	kzgProofs := p.KzgProofs()
	cells := make([][]byte, 0, nCells)
	proofs := make([][]byte, 0, nCells)
	for i := range size {
		if meetsNeeds.BitAt(i) {
			cells = append(cells, column[i])
			proofs = append(proofs, kzgProofs[i])
		}
	}

	marshalled, err := p.marshalCellsMessage(cells, proofs, meetsNeeds)
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal cells message")
	}
	return marshalled, meetsNeeds, nil
}

func (p *PartialDataColumn) marshalCellsMessage(cells, proofs [][]byte, present bitfield.Bitlist) ([]byte, error) {
	if p.IsGloas() {
		out := &ethpb.PartialDataColumnSidecarGloas{
			PartialColumn:      cells,
			KzgProofs:          proofs,
			CellsPresentBitmap: present,
		}
		encoded, err := out.MarshalSSZ()
		if err != nil {
			return nil, errors.Wrap(err, "marshal gloas partial data column sidecar")
		}
		return encoded, nil
	}

	out := &ethpb.PartialDataColumnSidecar{
		PartialColumn:      cells,
		KzgProofs:          proofs,
		CellsPresentBitmap: present,
	}
	encoded, err := out.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "marshal partial data column sidecar")
	}
	return encoded, nil
}

// DecodePartialColumnSidecar SSZ-decodes an incoming partial-message body into the in-memory
// PartialDataColumnSidecar. It is the inverse of marshalCellsMessage: the Gloas wire is the 3-field
// PartialDataColumnSidecarGloas (no header), normalized into the 4-field struct with Header left nil
// so the receive path stays fork-neutral.
func DecodePartialColumnSidecar(raw []byte, isGloas bool) (*ethpb.PartialDataColumnSidecar, error) {
	if isGloas {
		gloas := &ethpb.PartialDataColumnSidecarGloas{}
		if err := gloas.UnmarshalSSZ(raw); err != nil {
			return nil, errors.Wrap(err, "unmarshal gloas partial data column sidecar")
		}
		return &ethpb.PartialDataColumnSidecar{
			CellsPresentBitmap: gloas.CellsPresentBitmap,
			PartialColumn:      gloas.PartialColumn,
			KzgProofs:          gloas.KzgProofs,
		}, nil
	}
	sidecar := &ethpb.PartialDataColumnSidecar{}
	if err := sidecar.UnmarshalSSZ(raw); err != nil {
		return nil, errors.Wrap(err, "unmarshal partial data column sidecar")
	}
	return sidecar, nil
}

func (p *PartialDataColumn) buildPartialColumnHeader() (encoded []byte, err error) {
	signedBlockHeader, err := p.SignedBlockHeader()
	if err != nil {
		return nil, errors.Wrap(err, "signed block header")
	}
	inclusionProof, err := p.KzgCommitmentsInclusionProof()
	if err != nil {
		return nil, errors.Wrap(err, "kzg commitments inclusion proof")
	}
	commitments, err := p.KzgCommitments()
	if err != nil {
		return nil, errors.Wrap(err, "kzg commitments")
	}
	outMessage := &ethpb.PartialDataColumnSidecar{
		Header: []*ethpb.PartialDataColumnHeader{{
			KzgCommitments:               commitments,
			SignedBlockHeader:            signedBlockHeader,
			KzgCommitmentsInclusionProof: inclusionProof,
		}},
		CellsPresentBitmap: bitfield.NewBitlist(uint64(len(commitments))),
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
		// Gloas columns have no header to exchange, so we never eager-push one;
		// the eager push then carries parts metadata only.
		includeHeader = includeHeader && !p.IsGloas()
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

	commitments, err := p.KzgCommitments()
	if err != nil {
		return nil, nil, errors.Wrap(err, "kzg commitments")
	}
	index := p.Index()
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
				ColumnIndex: index,
				Cell:        message.PartialColumn[j],
				Proof:       message.KzgProofs[j],
				// Use the commitment from our datacolumn, indexed by i since we
				// have all commitments.
				Commitment: commitments[i],
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
	column := p.Column()
	kzgProofs := p.KzgProofs()
	if cellIndex >= uint64(len(column)) || cellIndex >= uint64(len(kzgProofs)) {
		log.WithFields(logrus.Fields{
			"index":        p.Index(),
			"cellIndex":    cellIndex,
			"columnLen":    len(column),
			"kzgProofsLen": len(kzgProofs),
		}).Error("Cell index out of range for partial data column")
		return false
	}
	if p.Included.BitAt(cellIndex) {
		// We already have this cell
		return false
	}

	p.Included.SetBitAt(cellIndex, true)
	column[cellIndex] = cell
	kzgProofs[cellIndex] = proof
	return true
}

// IsComplete returns true if all cells are now present in this column.
func (p *PartialDataColumn) IsComplete() bool {
	return p.Included.Len() == p.Included.Count()
}
