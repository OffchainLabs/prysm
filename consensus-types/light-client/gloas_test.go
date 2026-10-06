package light_client

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/OffchainLabs/methodical-ssz/ssz"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	engine "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"google.golang.org/protobuf/proto"
)

func testRoots(count int) [][]byte {
	roots := make([][]byte, count)
	for i := range roots {
		roots[i] = make([]byte, 32)
		roots[i][0] = byte(i + 1)
	}
	return roots
}

func testGloasHeader() *pb.LightClientHeaderGloas {
	return &pb.LightClientHeaderGloas{
		Beacon: &pb.BeaconBlockHeader{
			Slot: 1, ProposerIndex: 2,
			ParentRoot: testRoots(1)[0], StateRoot: testRoots(2)[1], BodyRoot: testRoots(3)[2],
		},
		ExecutionBlockHash: testRoots(4)[3], ExecutionBranch: testRoots(11),
	}
}

func testSyncCommittee() *pb.SyncCommittee {
	pubkeys := make([][]byte, fieldparams.SyncCommitteeLength)
	for i := range pubkeys {
		pubkeys[i] = make([]byte, 48)
		pubkeys[i][0] = byte(i)
	}
	return &pb.SyncCommittee{Pubkeys: pubkeys, AggregatePubkey: make([]byte, 48)}
}

func testGloasUpdate() *pb.LightClientUpdateGloas {
	return &pb.LightClientUpdateGloas{
		AttestedHeader: testGloasHeader(), FinalizedHeader: testGloasHeader(),
		NextSyncCommittee: testSyncCommittee(), NextSyncCommitteeBranch: testRoots(11),
		FinalityBranch: testRoots(9), SignatureSlot: 3,
		SyncAggregate: &pb.SyncAggregate{
			SyncCommitteeBits: make([]byte, fieldparams.SyncCommitteeLength/8), SyncCommitteeSignature: make([]byte, 96),
		},
	}
}

func testGloasBootstrap() *pb.LightClientBootstrapGloas {
	return &pb.LightClientBootstrapGloas{
		Header: testGloasHeader(), CurrentSyncCommittee: testSyncCommittee(), CurrentSyncCommitteeBranch: testRoots(11),
	}
}

func testGloasFinalityUpdate() *pb.LightClientFinalityUpdateGloas {
	u := testGloasUpdate()
	return &pb.LightClientFinalityUpdateGloas{
		AttestedHeader: u.AttestedHeader, FinalizedHeader: u.FinalizedHeader, FinalityBranch: u.FinalityBranch,
		SyncAggregate: u.SyncAggregate, SignatureSlot: u.SignatureSlot,
	}
}

type testSSZProto interface {
	proto.Message
	ssz.Marshaler
	ssz.Unmarshaler
	HashTreeRoot() ([32]byte, error)
}

func TestGloasSSZRoundTrip(t *testing.T) {
	u := testGloasUpdate()
	headerSize := 112 + 32 + 11*32
	committeeSize := fieldparams.SyncCommitteeLength*48 + 48
	aggregateSize := fieldparams.SyncCommitteeLength/8 + 96
	tests := []struct {
		name  string
		value testSSZProto
		empty testSSZProto
		wrap  func(proto.Message) (ssz.Marshaler, error)
		size  int
	}{
		{"header", u.AttestedHeader, &pb.LightClientHeaderGloas{}, func(p proto.Message) (ssz.Marshaler, error) { return NewWrappedHeader(p) }, headerSize},
		{"bootstrap", testGloasBootstrap(), &pb.LightClientBootstrapGloas{}, func(p proto.Message) (ssz.Marshaler, error) { return NewWrappedBootstrap(p) }, headerSize + committeeSize + 11*32},
		{"update", u, &pb.LightClientUpdateGloas{}, func(p proto.Message) (ssz.Marshaler, error) { return NewWrappedUpdate(p) }, 2*headerSize + committeeSize + 11*32 + 9*32 + aggregateSize + 8},
		{"finality", testGloasFinalityUpdate(), &pb.LightClientFinalityUpdateGloas{}, func(p proto.Message) (ssz.Marshaler, error) { return NewWrappedFinalityUpdate(p) }, 2*headerSize + 9*32 + aggregateSize + 8},
		{"optimistic", &pb.LightClientOptimisticUpdateGloas{AttestedHeader: u.AttestedHeader, SyncAggregate: u.SyncAggregate, SignatureSlot: u.SignatureSlot}, &pb.LightClientOptimisticUpdateGloas{}, func(p proto.Message) (ssz.Marshaler, error) { return NewWrappedOptimisticUpdate(p) }, headerSize + aggregateSize + 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped, err := tt.wrap(tt.value)
			require.NoError(t, err)
			encoded, err := wrapped.MarshalSSZ()
			require.NoError(t, err)
			require.Equal(t, tt.size, len(encoded))
			require.Equal(t, tt.size, wrapped.SizeSSZ())
			withPrefix, err := wrapped.MarshalSSZTo([]byte{99})
			require.NoError(t, err)
			require.DeepEqual(t, append([]byte{99}, encoded...), withPrefix)
			require.NoError(t, tt.empty.UnmarshalSSZ(encoded))
			require.Equal(t, true, proto.Equal(tt.value, tt.empty))
			root, err := tt.value.HashTreeRoot()
			require.NoError(t, err)
			decodedRoot, err := tt.empty.HashTreeRoot()
			require.NoError(t, err)
			require.Equal(t, root, decodedRoot)
			require.NotNil(t, tt.empty.UnmarshalSSZ(encoded[:len(encoded)-1]))
		})
	}
}

func merkleizeTestRoots(roots [][32]byte) [32]byte {
	size := 1
	for size < len(roots) {
		size *= 2
	}
	layer := make([][32]byte, size)
	copy(layer, roots)
	for len(layer) > 1 {
		for i := 0; i < len(layer)/2; i++ {
			var pair [64]byte
			copy(pair[:32], layer[2*i][:])
			copy(pair[32:], layer[2*i+1][:])
			layer[i] = sha256.Sum256(pair[:])
		}
		layer = layer[:len(layer)/2]
	}
	return layer[0]
}

func TestGloasHeader(t *testing.T) {
	p := testGloasHeader()
	h, err := NewWrappedHeader(p)
	require.NoError(t, err)
	require.Equal(t, version.Gloas, h.Version())
	require.Equal(t, p, h.Proto())
	require.Equal(t, p.Beacon, h.Beacon())
	hash, err := h.ExecutionBlockHash()
	require.NoError(t, err)
	require.DeepEqual(t, p.ExecutionBlockHash, hash[:])
	branch, err := h.ExecutionBranchGloas()
	require.NoError(t, err)
	for i := range branch {
		require.DeepEqual(t, p.ExecutionBranch[i], branch[i][:])
	}
	_, err = h.Execution()
	require.ErrorContains(t, "not supported", err)
	_, err = h.ExecutionBranch()
	require.ErrorContains(t, "not supported", err)

	t.Run("regular container root and layout", func(t *testing.T) {
		beaconRoot, err := p.Beacon.HashTreeRoot()
		require.NoError(t, err)
		wantRoot := merkleizeTestRoots([][32]byte{beaconRoot, hash, merkleizeTestRoots(branch[:])})
		root, err := p.HashTreeRoot()
		require.NoError(t, err)
		require.Equal(t, wantRoot, root)
		wantBytes, err := p.Beacon.MarshalSSZ()
		require.NoError(t, err)
		wantBytes = append(wantBytes, p.ExecutionBlockHash...)
		for _, leaf := range p.ExecutionBranch {
			wantBytes = append(wantBytes, leaf...)
		}
		encoded, err := h.MarshalSSZ()
		require.NoError(t, err)
		require.DeepEqual(t, wantBytes, encoded)
	})

	t.Run("invalid fields", func(t *testing.T) {
		for _, mutate := range []func(*pb.LightClientHeaderGloas){
			func(p *pb.LightClientHeaderGloas) { p.Beacon = nil },
			func(p *pb.LightClientHeaderGloas) { p.ExecutionBlockHash = nil },
			func(p *pb.LightClientHeaderGloas) { p.ExecutionBlockHash = make([]byte, 31) },
			func(p *pb.LightClientHeaderGloas) { p.ExecutionBlockHash = make([]byte, 33) },
			func(p *pb.LightClientHeaderGloas) { p.ExecutionBranch = testRoots(10) },
			func(p *pb.LightClientHeaderGloas) { p.ExecutionBranch = testRoots(12) },
			func(p *pb.LightClientHeaderGloas) { p.ExecutionBranch[5] = make([]byte, 31) },
			func(p *pb.LightClientHeaderGloas) { p.ExecutionBranch[5] = make([]byte, 33) },
		} {
			p := testGloasHeader()
			mutate(p)
			_, err := NewWrappedHeaderGloas(p)
			require.NotNil(t, err)
		}
	})
}

func TestGloasSetters(t *testing.T) {
	bp := testGloasBootstrap()
	b, err := NewWrappedBootstrap(bp)
	require.NoError(t, err)
	p := testGloasUpdate()
	u, err := NewWrappedUpdate(p)
	require.NoError(t, err)
	require.Equal(t, version.Gloas, b.Version())
	require.Equal(t, version.Gloas, u.Version())
	require.Equal(t, bp, b.Proto())
	require.Equal(t, p, u.Proto())
	require.Equal(t, false, u.IsNil())

	t.Run("headers", func(t *testing.T) {
		newHeader, err := NewWrappedHeaderGloas(testGloasHeader())
		require.NoError(t, err)
		legacy, err := NewWrappedHeaderAltair(&pb.LightClientHeaderAltair{Beacon: testGloasHeader().Beacon})
		require.NoError(t, err)
		for _, tt := range []struct {
			name     string
			set      func(interfaces.LightClientHeader) error
			get      func() interfaces.LightClientHeader
			getProto func() *pb.LightClientHeaderGloas
		}{
			{"bootstrap", b.SetHeader, b.Header, func() *pb.LightClientHeaderGloas { return bp.Header }},
			{"attested", u.SetAttestedHeader, u.AttestedHeader, func() *pb.LightClientHeaderGloas { return p.AttestedHeader }},
			{"finalized", u.SetFinalizedHeader, u.FinalizedHeader, func() *pb.LightClientHeaderGloas { return p.FinalizedHeader }},
		} {
			t.Run(tt.name, func(t *testing.T) {
				require.NoError(t, tt.set(newHeader))
				require.Equal(t, newHeader, tt.get())
				require.Equal(t, newHeader.Proto(), tt.getProto())
				require.NotNil(t, tt.set(legacy))
				require.NotNil(t, tt.set(nil))
				require.Equal(t, newHeader, tt.get())
				require.Equal(t, newHeader.Proto(), tt.getProto())
			})
		}
	})

	t.Run("branches", func(t *testing.T) {
		for _, tt := range []struct {
			name     string
			depth    int
			set      func([][]byte) error
			get      func() ([][]byte, error)
			getProto func() [][]byte
		}{
			{"current committee", 11, b.SetCurrentSyncCommitteeBranch, func() ([][]byte, error) {
				branch, err := b.CurrentSyncCommitteeBranchGloas()
				return branchSlices(branch[:]), err
			}, func() [][]byte { return bp.CurrentSyncCommitteeBranch }},
			{"next committee", 11, u.SetNextSyncCommitteeBranch, func() ([][]byte, error) {
				branch, err := u.NextSyncCommitteeBranchGloas()
				return branchSlices(branch[:]), err
			}, func() [][]byte { return p.NextSyncCommitteeBranch }},
			{"finality", 9, u.SetFinalityBranch, func() ([][]byte, error) { branch, err := u.FinalityBranchGloas(); return branchSlices(branch[:]), err }, func() [][]byte { return p.FinalityBranch }},
		} {
			t.Run(tt.name, func(t *testing.T) {
				valid := testRoots(tt.depth)
				valid[0][0] = 99
				require.NoError(t, tt.set(valid))
				shortRoot, longRoot := testRoots(tt.depth), testRoots(tt.depth)
				shortRoot[0], longRoot[0] = make([]byte, 31), make([]byte, 33)
				for _, invalid := range [][][]byte{nil, testRoots(tt.depth - 1), testRoots(tt.depth + 1), shortRoot, longRoot} {
					require.NotNil(t, tt.set(invalid))
					got, err := tt.get()
					require.NoError(t, err)
					require.DeepEqual(t, valid, got)
					require.DeepEqual(t, valid, tt.getProto())
				}
			})
		}
	})

	t.Run("committee aggregate slot", func(t *testing.T) {
		sc := testSyncCommittee()
		require.NoError(t, b.SetCurrentSyncCommittee(sc))
		require.Equal(t, sc, b.CurrentSyncCommittee())
		require.Equal(t, sc, bp.CurrentSyncCommittee)
		u.SetNextSyncCommittee(sc)
		require.Equal(t, sc, u.NextSyncCommittee())
		require.Equal(t, sc, p.NextSyncCommittee)
		sa := testGloasUpdate().SyncAggregate
		u.SetSyncAggregate(sa)
		require.Equal(t, sa, u.SyncAggregate())
		require.Equal(t, sa, p.SyncAggregate)
		u.SetSignatureSlot(42)
		require.Equal(t, primitives.Slot(42), u.SignatureSlot())
		require.Equal(t, primitives.Slot(42), p.SignatureSlot)
	})

	t.Run("legacy branch accessors unsupported", func(t *testing.T) {
		_, err := b.CurrentSyncCommitteeBranch()
		require.ErrorContains(t, "not supported", err)
		_, err = b.CurrentSyncCommitteeBranchElectra()
		require.ErrorContains(t, "not supported", err)
		_, err = u.NextSyncCommitteeBranch()
		require.ErrorContains(t, "not supported", err)
		_, err = u.NextSyncCommitteeBranchElectra()
		require.ErrorContains(t, "not supported", err)
		_, err = u.FinalityBranch()
		require.ErrorContains(t, "not supported", err)
		_, err = u.FinalityBranchElectra()
		require.ErrorContains(t, "not supported", err)
	})
}

func branchSlices(branch [][32]byte) [][]byte {
	result := make([][]byte, len(branch))
	for i := range branch {
		result[i] = branch[i][:]
	}
	return result
}

func TestGloasUpdateConversions(t *testing.T) {
	p := testGloasUpdate()
	u, err := NewWrappedUpdate(p)
	require.NoError(t, err)
	f, err := NewFinalityUpdateFromUpdate(u)
	require.NoError(t, err)
	o, err := NewOptimisticUpdateFromUpdate(u)
	require.NoError(t, err)
	require.Equal(t, version.Gloas, f.Version())
	require.Equal(t, version.Gloas, o.Version())
	require.Equal(t, false, f.IsNil())
	require.Equal(t, false, o.IsNil())
	require.Equal(t, u.AttestedHeader(), f.AttestedHeader())
	require.Equal(t, u.AttestedHeader(), o.AttestedHeader())
	require.Equal(t, u.FinalizedHeader(), f.FinalizedHeader())
	require.Equal(t, p.SyncAggregate, f.SyncAggregate())
	require.Equal(t, p.SyncAggregate, o.SyncAggregate())
	require.Equal(t, p.SignatureSlot, f.SignatureSlot())
	require.Equal(t, p.SignatureSlot, o.SignatureSlot())
	fp := f.Proto().(*pb.LightClientFinalityUpdateGloas)
	op := o.Proto().(*pb.LightClientOptimisticUpdateGloas)
	require.Equal(t, p.AttestedHeader, fp.AttestedHeader)
	require.Equal(t, p.FinalizedHeader, fp.FinalizedHeader)
	require.Equal(t, p.AttestedHeader, op.AttestedHeader)
	require.DeepEqual(t, p.FinalityBranch, fp.FinalityBranch)
	branch, err := f.FinalityBranchGloas()
	require.NoError(t, err)
	require.DeepEqual(t, p.FinalityBranch, branchSlices(branch[:]))
	_, err = f.FinalityBranch()
	require.ErrorContains(t, "not supported", err)
	_, err = f.FinalityBranchElectra()
	require.ErrorContains(t, "not supported", err)

	t.Run("decode refreshes caches", func(t *testing.T) {
		decodedFinality := NewEmptyFinalityUpdateGloas()
		decodedOptimistic := NewEmptyOptimisticUpdateGloas()
		require.Equal(t, true, decodedFinality.IsNil())
		require.Equal(t, true, decodedOptimistic.IsNil())
		for i := byte(50); i < 52; i++ {
			fp.FinalityBranch[0][0] = i
			p.AttestedHeader.ExecutionBlockHash[0] = i
			for _, tt := range []struct {
				source ssz.Marshaler
				dest   interface {
					ssz.Unmarshaler
					Proto() proto.Message
				}
			}{
				{f, decodedFinality}, {o, decodedOptimistic},
			} {
				encoded, err := tt.source.MarshalSSZ()
				require.NoError(t, err)
				require.NoError(t, tt.dest.UnmarshalSSZ(encoded))
				require.NotNil(t, tt.dest.UnmarshalSSZ(encoded[:10]))
			}
			branch, err := decodedFinality.FinalityBranchGloas()
			require.NoError(t, err)
			require.Equal(t, i, branch[0][0])
			for _, h := range []interfaces.LightClientHeader{decodedFinality.AttestedHeader(), decodedOptimistic.AttestedHeader()} {
				hash, err := h.ExecutionBlockHash()
				require.NoError(t, err)
				require.Equal(t, i, hash[0])
			}
			require.Equal(t, true, proto.Equal(fp, decodedFinality.Proto()))
			require.Equal(t, true, proto.Equal(op, decodedOptimistic.Proto()))
			require.Equal(t, false, decodedFinality.IsNil())
			require.Equal(t, false, decodedOptimistic.IsNil())
		}
	})
}

func TestGloasConstructorValidation(t *testing.T) {
	t.Run("bootstrap", func(t *testing.T) {
		p := testGloasBootstrap()
		p.Header = nil
		b, err := NewWrappedBootstrapGloas(p)
		require.NoError(t, err)
		require.IsNil(t, b.Header())
		p.Header = &pb.LightClientHeaderGloas{}
		_, err = NewWrappedBootstrapGloas(p)
		require.NotNil(t, err)
		p.Header = testGloasHeader()
		p.CurrentSyncCommitteeBranch = testRoots(6)
		_, err = NewWrappedBootstrapGloas(p)
		require.NotNil(t, err)
	})
	t.Run("update", func(t *testing.T) {
		p := testGloasUpdate()
		p.FinalizedHeader = nil
		u, err := NewWrappedUpdateGloas(p)
		require.NoError(t, err)
		require.IsNil(t, u.FinalizedHeader())
		for i, mutate := range []func(*pb.LightClientUpdateGloas){
			func(p *pb.LightClientUpdateGloas) { p.AttestedHeader = nil },
			func(p *pb.LightClientUpdateGloas) { p.FinalizedHeader = &pb.LightClientHeaderGloas{} },
			func(p *pb.LightClientUpdateGloas) { p.NextSyncCommitteeBranch = testRoots(6) },
			func(p *pb.LightClientUpdateGloas) { p.FinalityBranch = testRoots(7) },
		} {
			t.Run(fmt.Sprint(i), func(t *testing.T) {
				p := testGloasUpdate()
				mutate(p)
				_, err := NewWrappedUpdateGloas(p)
				require.NotNil(t, err)
			})
		}
	})
	t.Run("finality", func(t *testing.T) {
		for _, mutate := range []func(*pb.LightClientFinalityUpdateGloas){
			func(p *pb.LightClientFinalityUpdateGloas) { p.AttestedHeader = nil },
			func(p *pb.LightClientFinalityUpdateGloas) { p.FinalizedHeader = nil },
			func(p *pb.LightClientFinalityUpdateGloas) { p.FinalityBranch = testRoots(7) },
		} {
			p := testGloasFinalityUpdate()
			mutate(p)
			_, err := NewWrappedFinalityUpdateGloas(p)
			require.NotNil(t, err)
		}
	})
	t.Run("optimistic", func(t *testing.T) {
		_, err := NewWrappedOptimisticUpdateGloas(&pb.LightClientOptimisticUpdateGloas{})
		require.NotNil(t, err)
	})
	t.Run("nil receivers", func(t *testing.T) {
		require.Equal(t, true, (*updateGloas)(nil).IsNil())
		require.Equal(t, true, (&updateGloas{}).IsNil())
		require.Equal(t, true, (*finalityUpdateGloas)(nil).IsNil())
		require.Equal(t, true, (*optimisticUpdateGloas)(nil).IsNil())
	})
}

func TestLightClientFactoriesRejectInvalidTypes(t *testing.T) {
	for _, tt := range []struct {
		name      string
		wrap      func(proto.Message) error
		typedNils []proto.Message
	}{
		{"header", func(p proto.Message) error { _, err := NewWrappedHeader(p); return err }, []proto.Message{(*pb.LightClientHeaderAltair)(nil), (*pb.LightClientHeaderCapella)(nil), (*pb.LightClientHeaderGloas)(nil)}},
		{"bootstrap", func(p proto.Message) error { _, err := NewWrappedBootstrap(p); return err }, []proto.Message{(*pb.LightClientBootstrapAltair)(nil), (*pb.LightClientBootstrapCapella)(nil), (*pb.LightClientBootstrapDeneb)(nil), (*pb.LightClientBootstrapElectra)(nil), (*pb.LightClientBootstrapGloas)(nil)}},
		{"update", func(p proto.Message) error { _, err := NewWrappedUpdate(p); return err }, []proto.Message{(*pb.LightClientUpdateAltair)(nil), (*pb.LightClientUpdateCapella)(nil), (*pb.LightClientUpdateDeneb)(nil), (*pb.LightClientUpdateElectra)(nil), (*pb.LightClientUpdateGloas)(nil)}},
		{"finality", func(p proto.Message) error { _, err := NewWrappedFinalityUpdate(p); return err }, []proto.Message{(*pb.LightClientFinalityUpdateAltair)(nil), (*pb.LightClientFinalityUpdateCapella)(nil), (*pb.LightClientFinalityUpdateDeneb)(nil), (*pb.LightClientFinalityUpdateElectra)(nil), (*pb.LightClientFinalityUpdateGloas)(nil)}},
		{"optimistic", func(p proto.Message) error { _, err := NewWrappedOptimisticUpdate(p); return err }, []proto.Message{(*pb.LightClientOptimisticUpdateAltair)(nil), (*pb.LightClientOptimisticUpdateCapella)(nil), (*pb.LightClientOptimisticUpdateDeneb)(nil), (*pb.LightClientOptimisticUpdateGloas)(nil)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, tt.wrap(nil))
			require.ErrorContains(t, "cannot construct", tt.wrap(&pb.BeaconBlockHeader{}))
			for _, p := range tt.typedNils {
				require.NotNil(t, tt.wrap(p))
			}
		})
	}
	_, err := NewFinalityUpdateFromUpdate(nil)
	require.ErrorContains(t, "unsupported type", err)
	_, err = NewOptimisticUpdateFromUpdate(nil)
	require.ErrorContains(t, "unsupported type", err)
}

func TestLegacyLightClientTypes(t *testing.T) {
	t.Run("unsupported branch without header", func(t *testing.T) {
		for _, b := range []interfaces.LightClientBootstrap{&bootstrapAltair{}, &bootstrapCapella{}, &bootstrapDeneb{}, &bootstrapElectra{}} {
			_, err := b.CurrentSyncCommitteeBranchGloas()
			require.ErrorContains(t, "not supported", err)
		}
	})
	beacon := testGloasHeader().Beacon
	altair := &pb.LightClientHeaderAltair{Beacon: beacon}
	capella := &pb.LightClientHeaderCapella{Beacon: beacon, Execution: &engine.ExecutionPayloadHeaderCapella{}, ExecutionBranch: testRoots(4)}
	deneb := &pb.LightClientHeaderDeneb{Beacon: beacon, Execution: &engine.ExecutionPayloadHeaderDeneb{}, ExecutionBranch: testRoots(4)}
	sa := testGloasUpdate().SyncAggregate
	for _, tt := range []struct {
		name      string
		update    proto.Message
		bootstrap proto.Message
	}{
		{"altair", &pb.LightClientUpdateAltair{AttestedHeader: altair, FinalizedHeader: altair, NextSyncCommitteeBranch: testRoots(5), FinalityBranch: testRoots(6), SyncAggregate: sa, SignatureSlot: 3}, &pb.LightClientBootstrapAltair{Header: altair, CurrentSyncCommitteeBranch: testRoots(5)}},
		{"capella", &pb.LightClientUpdateCapella{AttestedHeader: capella, FinalizedHeader: capella, NextSyncCommitteeBranch: testRoots(5), FinalityBranch: testRoots(6), SyncAggregate: sa, SignatureSlot: 3}, &pb.LightClientBootstrapCapella{Header: capella, CurrentSyncCommitteeBranch: testRoots(5)}},
		{"deneb", &pb.LightClientUpdateDeneb{AttestedHeader: deneb, FinalizedHeader: deneb, NextSyncCommitteeBranch: testRoots(5), FinalityBranch: testRoots(6), SyncAggregate: sa, SignatureSlot: 3}, &pb.LightClientBootstrapDeneb{Header: deneb, CurrentSyncCommitteeBranch: testRoots(5)}},
		{"electra", &pb.LightClientUpdateElectra{AttestedHeader: deneb, FinalizedHeader: deneb, NextSyncCommitteeBranch: testRoots(6), FinalityBranch: testRoots(7), SyncAggregate: sa, SignatureSlot: 3}, &pb.LightClientBootstrapElectra{Header: deneb, CurrentSyncCommitteeBranch: testRoots(6)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			u, err := NewWrappedUpdate(tt.update)
			require.NoError(t, err)
			b, err := NewWrappedBootstrap(tt.bootstrap)
			require.NoError(t, err)
			f, err := NewFinalityUpdateFromUpdate(u)
			require.NoError(t, err)
			o, err := NewOptimisticUpdateFromUpdate(u)
			require.NoError(t, err)
			_, err = NewWrappedFinalityUpdate(f.Proto())
			require.NoError(t, err)
			_, err = NewWrappedOptimisticUpdate(o.Proto())
			require.NoError(t, err)
			require.Equal(t, u.AttestedHeader(), f.AttestedHeader())
			require.Equal(t, u.AttestedHeader(), o.AttestedHeader())
			require.Equal(t, u.FinalizedHeader(), f.FinalizedHeader())
			require.Equal(t, u.SyncAggregate(), f.SyncAggregate())
			require.Equal(t, u.SyncAggregate(), o.SyncAggregate())
			require.Equal(t, u.SignatureSlot(), f.SignatureSlot())
			require.Equal(t, u.SignatureSlot(), o.SignatureSlot())
			_, err = u.AttestedHeader().ExecutionBlockHash()
			require.ErrorContains(t, "not supported", err)
			_, err = u.AttestedHeader().ExecutionBranchGloas()
			require.ErrorContains(t, "not supported", err)
			_, err = u.NextSyncCommitteeBranchGloas()
			require.ErrorContains(t, "not supported", err)
			_, err = u.FinalityBranchGloas()
			require.ErrorContains(t, "not supported", err)
			_, err = f.FinalityBranchGloas()
			require.ErrorContains(t, "not supported", err)
			_, err = b.CurrentSyncCommitteeBranchGloas()
			require.ErrorContains(t, "not supported", err)
		})
	}
	t.Run("electra header", func(t *testing.T) {
		h, err := NewWrappedHeaderElectra(deneb)
		require.NoError(t, err)
		_, err = h.ExecutionBlockHash()
		require.ErrorContains(t, "not supported", err)
		_, err = h.ExecutionBranchGloas()
		require.ErrorContains(t, "not supported", err)
	})
}
