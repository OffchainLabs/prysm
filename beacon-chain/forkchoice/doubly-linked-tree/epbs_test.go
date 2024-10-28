package doublylinkedtree

import (
	"context"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestStore_Insert_PayloadContent(t *testing.T) {
	ctx := context.Background()
	f := setup(0, 0)
	s := f.store
	// The tree root is full
	fr := [32]byte{}
	n := s.nodeByRoot[fr]
	require.Equal(t, true, n.isParentFull())

	// Insert a child with a payload
	cr := [32]byte{'a'}
	cp := [32]byte{'p'}
	n, err := s.insert(ctx, 1, cr, fr, cp, fr, 0, 0)
	require.NoError(t, err)
	require.Equal(t, true, n.isParentFull())
	require.Equal(t, s.treeRootNode, n.parent)
	require.Equal(t, s.nodeByRoot[cr], n)

	// Insert a grandchild without a payload
	gr := [32]byte{'b'}
	gn, err := s.insert(ctx, 2, gr, cr, fr, cp, 0, 0)
	require.NoError(t, err)
	require.Equal(t, true, gn.isParentFull())
	require.Equal(t, n, gn.parent)

	// Insert the payload of the same grandchild
	gp := [32]byte{'q'}
	gfn, err := s.insert(ctx, 2, gr, cr, gp, cp, 0, 0)
	require.NoError(t, err)
	require.Equal(t, true, gfn.isParentFull())
	require.Equal(t, n, gfn.parent)

	// Insert an empty great grandchild based on empty
	ggr := [32]byte{'c'}
	ggn, err := s.insert(ctx, 3, ggr, gr, fr, cp, 0, 0)
	require.NoError(t, err)
	require.Equal(t, false, ggn.isParentFull())
	require.Equal(t, gn, ggn.parent)

	// Insert an empty great grandchild based on full
	ggfr := [32]byte{'d'}
	ggfn, err := s.insert(ctx, 3, ggfr, gr, fr, gp, 0, 0)
	require.NoError(t, err)
	require.Equal(t, gfn, ggfn.parent)
	require.Equal(t, true, ggfn.isParentFull())

	// Insert the payload for the great grandchild based on empty
	ggp := [32]byte{'r'}
	n, err = s.insert(ctx, 3, ggr, gr, ggp, cp, 0, 0)
	require.NoError(t, err)
	require.Equal(t, false, n.isParentFull())
	require.Equal(t, gn, n.parent)

	// Insert the payload for the great grandchild based on full
	ggfp := [32]byte{'s'}
	n, err = s.insert(ctx, 3, ggfr, gr, ggfp, gp, 0, 0)
	require.NoError(t, err)
	require.Equal(t, true, n.isParentFull())
	require.Equal(t, gfn, n.parent)

	// Reinsert an empty node
	ggfn2, err := s.insert(ctx, 3, ggfr, gr, fr, gp, 0, 0)
	require.NoError(t, err)
	require.Equal(t, ggfn, ggfn2)

	// Reinsert a full node
	n2, err := s.insert(ctx, 3, ggfr, gr, ggfp, gp, 0, 0)
	require.NoError(t, err)
	require.Equal(t, n, n2)
}

func TestStore_Insert_PayloadEnvelope(t *testing.T) {
	ctx := context.Background()
	f := setup(0, 0)
	s := f.store
	// The tree root is full
	fr := [32]byte{}
	n := s.nodeByRoot[fr]
	require.Equal(t, true, n.isParentFull())

	// Insert a child
	cr := [32]byte{'a'}
	cp := [32]byte{'p'}
	n, err := s.insert(ctx, 1, cr, fr, [32]byte{}, fr, 0, 0)
	require.NoError(t, err)
	require.Equal(t, true, n.isParentFull())
	require.Equal(t, s.treeRootNode, n.parent)
	require.Equal(t, s.nodeByRoot[cr], n)
	// Insert its payload
	p := &enginev1.ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadElectra{
			BlockHash: cp[:],
		},
		BeaconBlockRoot:    cr[:],
		PayloadWithheld:    false,
		StateRoot:          fr[:],
		BlobKzgCommitments: make([][]byte, 0),
	}
	e, err := blocks.WrappedROExecutionPayloadEnvelope(p)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayloadEnvelope(e))
	np := s.nodeByPayload[cp]
	require.Equal(t, np.root, n.root)
	require.NotEqual(t, np, n)

	// Insert a grandchild without a payload, it's parent is the full node,
	// which is not the empty node
	gr := [32]byte{'b'}
	gn, err := s.insert(ctx, 2, gr, cr, fr, cp, 0, 0)
	require.NoError(t, err)
	require.Equal(t, true, gn.isParentFull())
	require.Equal(t, np, gn.parent)

	// Insert the payload of the same grandchild
	gp := [32]byte{'q'}
	p = &enginev1.ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadElectra{
			BlockHash: gp[:],
		},
		BeaconBlockRoot:    gr[:],
		PayloadWithheld:    false,
		StateRoot:          fr[:],
		BlobKzgCommitments: make([][]byte, 0),
	}
	e, err = blocks.WrappedROExecutionPayloadEnvelope(p)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayloadEnvelope(e))
	gfn := s.nodeByPayload[gp]
	require.Equal(t, true, gfn.isParentFull())
	require.Equal(t, np, gfn.parent)

	// Insert an empty great grandchild based on empty
	ggr := [32]byte{'c'}
	ggn, err := s.insert(ctx, 3, ggr, gr, fr, cp, 0, 0)
	require.NoError(t, err)
	require.Equal(t, false, ggn.isParentFull())
	require.Equal(t, gn, ggn.parent)

	// Insert an empty great grandchild based on full
	ggfr := [32]byte{'d'}
	ggfn, err := s.insert(ctx, 3, ggfr, gr, fr, gp, 0, 0)
	require.NoError(t, err)
	require.Equal(t, gfn, ggfn.parent)
	require.Equal(t, true, ggfn.isParentFull())

	// Insert the payload for the great grandchild based on empty
	ggp := [32]byte{'r'}
	p = &enginev1.ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadElectra{
			BlockHash: ggp[:],
		},
		BeaconBlockRoot:    ggr[:],
		PayloadWithheld:    false,
		StateRoot:          fr[:],
		BlobKzgCommitments: make([][]byte, 0),
	}
	e, err = blocks.WrappedROExecutionPayloadEnvelope(p)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayloadEnvelope(e))
	n = s.nodeByPayload[ggp]
	require.Equal(t, false, n.isParentFull())
	require.Equal(t, gn, n.parent)

	// Insert the payload for the great grandchild based on full
	ggfp := [32]byte{'s'}
	p = &enginev1.ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadElectra{
			BlockHash: ggfp[:],
		},
		BeaconBlockRoot:    ggfr[:],
		PayloadWithheld:    false,
		StateRoot:          fr[:],
		BlobKzgCommitments: make([][]byte, 0),
	}
	e, err = blocks.WrappedROExecutionPayloadEnvelope(p)
	require.NoError(t, err)
	require.NoError(t, f.InsertPayloadEnvelope(e))
	n = s.nodeByPayload[ggfp]
	require.Equal(t, true, n.isParentFull())
	require.Equal(t, gfn, n.parent)
}

func TestGetPTCVote(t *testing.T) {
	ctx := context.Background()
	f := setup(0, 0)
	s := f.store
	require.NotNil(t, s.highestReceivedNode)
	fr := [32]byte{}

	// Insert a child with a payload
	cr := [32]byte{'a'}
	cp := [32]byte{'p'}
	n, err := s.insert(ctx, 1, cr, fr, cp, fr, 0, 0)
	require.NoError(t, err)
	require.Equal(t, n, s.highestReceivedNode)
	require.Equal(t, primitives.PAYLOAD_ABSENT, f.GetPTCVote())
	driftGenesisTime(f, 1, 0)
	require.Equal(t, primitives.PAYLOAD_PRESENT, f.GetPTCVote())
	n.withheld = true
	require.Equal(t, primitives.PAYLOAD_WITHHELD, f.GetPTCVote())
}
