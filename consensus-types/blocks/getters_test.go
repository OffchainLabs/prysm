package blocks

import (
	"testing"

	ssz "github.com/prysmaticlabs/fastssz"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	pb "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	validatorpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/validator-client"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func Test_BeaconBlockIsNil(t *testing.T) {
	t.Run("not nil", func(t *testing.T) {
		assert.NoError(t, BeaconBlockIsNil(&SignedBeaconBlock{block: &BeaconBlock{body: &BeaconBlockBody{}}}))
	})
	t.Run("nil interface", func(t *testing.T) {
		err := BeaconBlockIsNil(nil)
		assert.NotNil(t, err)
	})
	t.Run("nil signed block", func(t *testing.T) {
		var i interfaces.ReadOnlySignedBeaconBlock
		var sb *SignedBeaconBlock
		i = sb
		err := BeaconBlockIsNil(i)
		assert.NotNil(t, err)
	})
	t.Run("nil block", func(t *testing.T) {
		err := BeaconBlockIsNil(&SignedBeaconBlock{})
		assert.NotNil(t, err)
	})
	t.Run("nil block body", func(t *testing.T) {
		err := BeaconBlockIsNil(&SignedBeaconBlock{block: &BeaconBlock{}})
		assert.NotNil(t, err)
	})
}

func Test_SignedBeaconBlock_Signature(t *testing.T) {
	sb := &SignedBeaconBlock{}
	sb.SetSignature([]byte("signature"))
	assert.DeepEqual(t, bytesutil.ToBytes96([]byte("signature")), sb.Signature())
}

func Test_SignedBeaconBlock_Block(t *testing.T) {
	b := &BeaconBlock{}
	sb := &SignedBeaconBlock{block: b}
	assert.Equal(t, b, sb.Block())
}

func Test_SignedBeaconBlock_IsNil(t *testing.T) {
	t.Run("nil signed block", func(t *testing.T) {
		var sb *SignedBeaconBlock
		assert.Equal(t, true, sb.IsNil())
	})
	t.Run("nil block", func(t *testing.T) {
		sb := &SignedBeaconBlock{}
		assert.Equal(t, true, sb.IsNil())
	})
	t.Run("nil body", func(t *testing.T) {
		sb := &SignedBeaconBlock{block: &BeaconBlock{}}
		assert.Equal(t, true, sb.IsNil())
	})
	t.Run("not nil", func(t *testing.T) {
		sb := &SignedBeaconBlock{block: &BeaconBlock{body: &BeaconBlockBody{}}}
		assert.Equal(t, false, sb.IsNil())
	})
}

func Test_SignedBeaconBlock_Copy(t *testing.T) {
	bb := &BeaconBlockBody{}
	b := &BeaconBlock{body: bb}
	sb := &SignedBeaconBlock{block: b}
	cp, err := sb.Copy()
	require.NoError(t, err)
	assert.NotEqual(t, cp, sb)
	assert.NotEqual(t, cp.Block(), sb.block)
	assert.NotEqual(t, cp.Block().Body(), sb.block.body)
}

func Test_SignedBeaconBlock_Version(t *testing.T) {
	sb := &SignedBeaconBlock{version: 128}
	assert.Equal(t, 128, sb.Version())
}

func Test_SignedBeaconBlock_Header(t *testing.T) {
	bb := &BeaconBlockBody{
		version:      version.Phase0,
		randaoReveal: [96]byte{},
		eth1Data: &eth.Eth1Data{
			DepositRoot: make([]byte, 32),
			BlockHash:   make([]byte, 32),
		},
		graffiti: [32]byte{},
	}
	sb := &SignedBeaconBlock{
		version: version.Phase0,
		block: &BeaconBlock{
			version:       version.Phase0,
			slot:          128,
			proposerIndex: 128,
			parentRoot:    bytesutil.ToBytes32([]byte("parentroot")),
			stateRoot:     bytesutil.ToBytes32([]byte("stateroot")),
			body:          bb,
		},
		signature: bytesutil.ToBytes96([]byte("signature")),
	}
	h, err := sb.Header()
	require.NoError(t, err)
	assert.DeepEqual(t, sb.signature[:], h.Signature)
	assert.Equal(t, sb.block.slot, h.Header.Slot)
	assert.Equal(t, sb.block.proposerIndex, h.Header.ProposerIndex)
	assert.DeepEqual(t, sb.block.parentRoot[:], h.Header.ParentRoot)
	assert.DeepEqual(t, sb.block.stateRoot[:], h.Header.StateRoot)
	expectedHTR, err := bb.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR[:], h.Header.BodyRoot)
}

func Test_SignedBeaconBlock_UnmarshalSSZ(t *testing.T) {
	pb := hydrateSignedBeaconBlock()
	buf, err := pb.MarshalSSZ()
	require.NoError(t, err)
	expectedHTR, err := pb.HashTreeRoot()
	require.NoError(t, err)
	sb := &SignedBeaconBlock{}
	require.NoError(t, sb.UnmarshalSSZ(buf))
	msg, err := sb.Proto()
	require.NoError(t, err)
	actualPb, ok := msg.(*eth.SignedBeaconBlock)
	require.Equal(t, true, ok)
	actualHTR, err := actualPb.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, actualHTR)
}

func Test_BeaconBlock_Slot(t *testing.T) {
	b := &SignedBeaconBlock{block: &BeaconBlock{}}
	b.SetSlot(128)
	assert.Equal(t, primitives.Slot(128), b.Block().Slot())
}

func Test_BeaconBlock_ProposerIndex(t *testing.T) {
	b := &SignedBeaconBlock{block: &BeaconBlock{}}
	b.SetProposerIndex(128)
	assert.Equal(t, primitives.ValidatorIndex(128), b.Block().ProposerIndex())
}

func Test_BeaconBlock_ParentRoot(t *testing.T) {
	b := &SignedBeaconBlock{block: &BeaconBlock{}}
	b.SetParentRoot([]byte("parentroot"))
	assert.DeepEqual(t, bytesutil.ToBytes32([]byte("parentroot")), b.Block().ParentRoot())
}

func Test_BeaconBlock_StateRoot(t *testing.T) {
	b := &SignedBeaconBlock{block: &BeaconBlock{}}
	b.SetStateRoot([]byte("stateroot"))
	assert.DeepEqual(t, bytesutil.ToBytes32([]byte("stateroot")), b.Block().StateRoot())
}

func Test_BeaconBlock_Body(t *testing.T) {
	bb := &BeaconBlockBody{}
	b := &BeaconBlock{body: bb}
	assert.Equal(t, bb, b.Body())
}

func Test_BeaconBlock_Copy(t *testing.T) {
	bb := &BeaconBlockBody{randaoReveal: bytesutil.ToBytes96([]byte{246}), graffiti: bytesutil.ToBytes32([]byte("graffiti"))}
	b := &BeaconBlock{body: bb, slot: 123, proposerIndex: 456, parentRoot: bytesutil.ToBytes32([]byte("parentroot")), stateRoot: bytesutil.ToBytes32([]byte("stateroot"))}
	cp, err := b.Copy()
	require.NoError(t, err)
	assert.NotEqual(t, cp, b)
	assert.NotEqual(t, cp.Body(), bb)

	b.version = version.Altair
	b.body.version = b.version
	cp, err = b.Copy()
	require.NoError(t, err)
	assert.NotEqual(t, cp, b)
	assert.NotEqual(t, cp.Body(), bb)

	b.version = version.Bellatrix
	b.body.version = b.version
	cp, err = b.Copy()
	require.NoError(t, err)
	assert.NotEqual(t, cp, b)
	assert.NotEqual(t, cp.Body(), bb)

	b.version = version.Capella
	b.body.version = b.version
	cp, err = b.Copy()
	require.NoError(t, err)
	assert.NotEqual(t, cp, b)
	assert.NotEqual(t, cp.Body(), bb)

	b.version = version.Bellatrix
	b.body.version = b.version
	cp, err = b.Copy()
	require.NoError(t, err)
	assert.NotEqual(t, cp, b)
	assert.NotEqual(t, cp.Body(), bb)

	b.version = version.Capella
	b.body.version = b.version
	cp, err = b.Copy()
	require.NoError(t, err)
	assert.NotEqual(t, cp, b)
	assert.NotEqual(t, cp.Body(), bb)

	payload := &pb.ExecutionPayloadDeneb{ExcessBlobGas: 123}
	header := &pb.ExecutionPayloadHeaderDeneb{ExcessBlobGas: 223}
	payloadInterface, err := WrappedExecutionPayloadDeneb(payload)
	require.NoError(t, err)
	headerInterface, err := WrappedExecutionPayloadHeaderDeneb(header)
	require.NoError(t, err)
	bb = &BeaconBlockBody{executionPayload: payloadInterface, executionPayloadHeader: headerInterface, randaoReveal: bytesutil.ToBytes96([]byte{246}), graffiti: bytesutil.ToBytes32([]byte("graffiti"))}
	b = &BeaconBlock{body: bb, slot: 123, proposerIndex: 456, parentRoot: bytesutil.ToBytes32([]byte("parentroot")), stateRoot: bytesutil.ToBytes32([]byte("stateroot"))}
	b.version = version.Deneb
	b.body.version = b.version
	cp, err = b.Copy()
	require.NoError(t, err)
	assert.NotEqual(t, cp, b)
	assert.NotEqual(t, cp.Body(), bb)
	e, err := cp.Body().Execution()
	require.NoError(t, err)
	gas, err := e.ExcessBlobGas()
	require.NoError(t, err)
	require.DeepEqual(t, gas, uint64(123))
}

func Test_BeaconBlock_IsNil(t *testing.T) {
	t.Run("nil block", func(t *testing.T) {
		var b *BeaconBlock
		assert.Equal(t, true, b.IsNil())
	})
	t.Run("nil block body", func(t *testing.T) {
		b := &BeaconBlock{}
		assert.Equal(t, true, b.IsNil())
	})
	t.Run("not nil", func(t *testing.T) {
		b := &BeaconBlock{body: &BeaconBlockBody{}}
		assert.Equal(t, false, b.IsNil())
	})
}

func Test_BeaconBlock_IsBlinded(t *testing.T) {
	b := &SignedBeaconBlock{block: &BeaconBlock{body: &BeaconBlockBody{}}}
	assert.Equal(t, false, b.IsBlinded())

	b1 := &SignedBeaconBlock{version: version.Bellatrix, block: &BeaconBlock{body: &BeaconBlockBody{executionPayloadHeader: executionPayloadHeader{}}}}
	assert.Equal(t, true, b1.IsBlinded())
}

func Test_BeaconBlock_Version(t *testing.T) {
	b := &BeaconBlock{version: 128}
	assert.Equal(t, 128, b.Version())
}

func Test_BeaconBlock_HashTreeRoot(t *testing.T) {
	pb := hydrateBeaconBlock()
	expectedHTR, err := pb.HashTreeRoot()
	require.NoError(t, err)
	b, err := initBlockFromProtoPhase0(pb)
	require.NoError(t, err)
	actualHTR, err := b.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, actualHTR)
}

func Test_BeaconBlock_HashTreeRootWith(t *testing.T) {
	pb := hydrateBeaconBlock()
	expectedHTR, err := pb.HashTreeRoot()
	require.NoError(t, err)
	b, err := initBlockFromProtoPhase0(pb)
	require.NoError(t, err)
	h := ssz.DefaultHasherPool.Get()
	require.NoError(t, b.HashTreeRootWith(h))
	actualHTR, err := h.HashRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, actualHTR)
}

func Test_BeaconBlock_UnmarshalSSZ(t *testing.T) {
	pb := hydrateBeaconBlock()
	buf, err := pb.MarshalSSZ()
	require.NoError(t, err)
	expectedHTR, err := pb.HashTreeRoot()
	require.NoError(t, err)
	b := &BeaconBlock{}
	require.NoError(t, b.UnmarshalSSZ(buf))
	msg, err := b.Proto()
	require.NoError(t, err)
	actualPb, ok := msg.(*eth.BeaconBlock)
	require.Equal(t, true, ok)
	actualHTR, err := actualPb.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, actualHTR)
}

func Test_BeaconBlock_AsSignRequestObject(t *testing.T) {
	pb := hydrateBeaconBlock()
	expectedHTR, err := pb.HashTreeRoot()
	require.NoError(t, err)
	b, err := initBlockFromProtoPhase0(pb)
	require.NoError(t, err)
	signRequestObj, err := b.AsSignRequestObject()
	require.NoError(t, err)
	actualSignRequestObj, ok := signRequestObj.(*validatorpb.SignRequest_Block)
	require.Equal(t, true, ok)
	actualHTR, err := actualSignRequestObj.Block.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, actualHTR)
}

func Test_BeaconBlockBody_IsNil(t *testing.T) {
	t.Run("nil block body", func(t *testing.T) {
		var bb *BeaconBlockBody
		assert.Equal(t, true, bb.IsNil())
	})
	t.Run("not nil", func(t *testing.T) {
		bb := &BeaconBlockBody{}
		assert.Equal(t, false, bb.IsNil())
	})
}

func Test_BeaconBlockBody_RandaoReveal(t *testing.T) {
	bb := &SignedBeaconBlock{block: &BeaconBlock{body: &BeaconBlockBody{}}}
	bb.SetRandaoReveal([]byte("randaoreveal"))
	assert.DeepEqual(t, bytesutil.ToBytes96([]byte("randaoreveal")), bb.Block().Body().RandaoReveal())
}

func Test_BeaconBlockBody_Eth1Data(t *testing.T) {
	e := &eth.Eth1Data{DepositRoot: []byte("depositroot")}
	bb := &SignedBeaconBlock{block: &BeaconBlock{body: &BeaconBlockBody{}}}
	bb.SetEth1Data(e)
	assert.DeepEqual(t, e, bb.Block().Body().Eth1Data())
}

func Test_BeaconBlockBody_Graffiti(t *testing.T) {
	bb := &SignedBeaconBlock{block: &BeaconBlock{body: &BeaconBlockBody{}}}
	bb.SetGraffiti([]byte("graffiti"))
	assert.DeepEqual(t, bytesutil.ToBytes32([]byte("graffiti")), bb.Block().Body().Graffiti())
}

func Test_BeaconBlockBody_ProposerSlashings(t *testing.T) {
	ps := make([]*eth.ProposerSlashing, 0)
	bb := &SignedBeaconBlock{block: &BeaconBlock{body: &BeaconBlockBody{}}}
	bb.SetProposerSlashings(ps)
	assert.DeepSSZEqual(t, ps, bb.Block().Body().ProposerSlashings())
}

func Test_BeaconBlockBody_AttesterSlashings(t *testing.T) {
	as := make([]eth.AttSlashing, 0)
	bb := &SignedBeaconBlock{block: &BeaconBlock{body: &BeaconBlockBody{}}}
	require.NoError(t, bb.SetAttesterSlashings(as))
	assert.DeepSSZEqual(t, as, bb.Block().Body().AttesterSlashings())
}

func Test_BeaconBlockBody_Attestations(t *testing.T) {
	a := make([]eth.Att, 0)
	bb := &SignedBeaconBlock{block: &BeaconBlock{body: &BeaconBlockBody{}}}
	require.NoError(t, bb.SetAttestations(a))
	assert.DeepSSZEqual(t, a, bb.Block().Body().Attestations())
}

func Test_BeaconBlockBody_ElectraAttestations(t *testing.T) {
	bb := &SignedBeaconBlock{
		block: &BeaconBlock{body: &BeaconBlockBody{
			version: version.Electra,
			attestationsElectra: []*eth.AttestationElectra{{
				Signature: []byte("electra"),
			}},
		}}}
	a := bb.Block().Body().Attestations()
	require.Equal(t, 1, len(a))
	require.DeepEqual(t, a[0].GetSignature(), []byte("electra"))
}

func Test_BeaconBlockBody_Deposits(t *testing.T) {
	d := make([]*eth.Deposit, 0)
	bb := &SignedBeaconBlock{block: &BeaconBlock{body: &BeaconBlockBody{}}}
	bb.SetDeposits(d)
	assert.DeepSSZEqual(t, d, bb.Block().Body().Deposits())
}

func Test_BeaconBlockBody_VoluntaryExits(t *testing.T) {
	ve := make([]*eth.SignedVoluntaryExit, 0)
	bb := &SignedBeaconBlock{block: &BeaconBlock{body: &BeaconBlockBody{}}}
	bb.SetVoluntaryExits(ve)
	assert.DeepSSZEqual(t, ve, bb.Block().Body().VoluntaryExits())
}

func Test_BeaconBlockBody_SyncAggregate(t *testing.T) {
	sa := &eth.SyncAggregate{}
	bb := &SignedBeaconBlock{version: version.Altair, block: &BeaconBlock{version: version.Altair, body: &BeaconBlockBody{version: version.Altair}}}
	require.NoError(t, bb.SetSyncAggregate(sa))
	result, err := bb.Block().Body().SyncAggregate()
	require.NoError(t, err)
	assert.DeepEqual(t, result, sa)
}

func Test_BeaconBlockBody_BLSToExecutionChanges(t *testing.T) {
	changes := []*eth.SignedBLSToExecutionChange{{Message: &eth.BLSToExecutionChange{ToExecutionAddress: []byte("address")}}}
	bb := &SignedBeaconBlock{version: version.Capella, block: &BeaconBlock{body: &BeaconBlockBody{version: version.Capella}}}
	require.NoError(t, bb.SetBLSToExecutionChanges(changes))
	result, err := bb.Block().Body().BLSToExecutionChanges()
	require.NoError(t, err)
	assert.DeepSSZEqual(t, result, changes)
}

func Test_BeaconBlockBody_Execution(t *testing.T) {
	execution := &pb.ExecutionPayload{BlockNumber: 1}
	e, err := WrappedExecutionPayload(execution)
	require.NoError(t, err)
	bb := &SignedBeaconBlock{version: version.Bellatrix, block: &BeaconBlock{body: &BeaconBlockBody{version: version.Bellatrix}}}
	require.NoError(t, bb.SetExecution(e))
	result, err := bb.Block().Body().Execution()
	require.NoError(t, err)
	assert.DeepEqual(t, result, e)

	executionCapella := &pb.ExecutionPayloadCapella{BlockNumber: 1}
	eCapella, err := WrappedExecutionPayloadCapella(executionCapella)
	require.NoError(t, err)
	bb = &SignedBeaconBlock{version: version.Capella, block: &BeaconBlock{body: &BeaconBlockBody{version: version.Capella}}}
	require.NoError(t, bb.SetExecution(eCapella))
	result, err = bb.Block().Body().Execution()
	require.NoError(t, err)
	assert.DeepEqual(t, result, eCapella)

	executionCapellaHeader := &pb.ExecutionPayloadHeaderCapella{BlockNumber: 1}
	eCapellaHeader, err := WrappedExecutionPayloadHeaderCapella(executionCapellaHeader)
	require.NoError(t, err)
	bb = &SignedBeaconBlock{version: version.Capella, block: &BeaconBlock{version: version.Capella, body: &BeaconBlockBody{version: version.Capella}}}
	require.NoError(t, bb.SetExecution(eCapellaHeader))
	result, err = bb.Block().Body().Execution()
	require.NoError(t, err)
	assert.DeepEqual(t, result, eCapellaHeader)

	executionDeneb := &pb.ExecutionPayloadDeneb{BlockNumber: 1, ExcessBlobGas: 123}
	eDeneb, err := WrappedExecutionPayloadDeneb(executionDeneb)
	require.NoError(t, err)
	bb = &SignedBeaconBlock{version: version.Deneb, block: &BeaconBlock{body: &BeaconBlockBody{version: version.Deneb}}}
	require.NoError(t, bb.SetExecution(eDeneb))
	result, err = bb.Block().Body().Execution()
	require.NoError(t, err)
	assert.DeepEqual(t, result, eDeneb)
	gas, err := eDeneb.ExcessBlobGas()
	require.NoError(t, err)
	require.DeepEqual(t, gas, uint64(123))

	executionDenebHeader := &pb.ExecutionPayloadHeaderDeneb{BlockNumber: 1, ExcessBlobGas: 223}
	eDenebHeader, err := WrappedExecutionPayloadHeaderDeneb(executionDenebHeader)
	require.NoError(t, err)
	bb = &SignedBeaconBlock{version: version.Deneb, block: &BeaconBlock{version: version.Deneb, body: &BeaconBlockBody{version: version.Deneb}}}
	require.NoError(t, bb.SetExecution(eDenebHeader))
	result, err = bb.Block().Body().Execution()
	require.NoError(t, err)
	assert.DeepEqual(t, result, eDenebHeader)
	gas, err = eDenebHeader.ExcessBlobGas()
	require.NoError(t, err)
	require.DeepEqual(t, gas, uint64(223))

	bb = &SignedBeaconBlock{version: version.EPBS, block: &BeaconBlock{version: version.EPBS, body: &BeaconBlockBody{version: version.EPBS}}}
	require.ErrorContains(t, "Execution is not supported for epbs: unsupported getter", bb.SetExecution(nil))
}

func Test_BeaconBlockBody_HashTreeRoot(t *testing.T) {
	pb := hydrateBeaconBlockBody()
	expectedHTR, err := pb.HashTreeRoot()
	require.NoError(t, err)
	b, err := initBlockBodyFromProtoPhase0(pb)
	require.NoError(t, err)
	actualHTR, err := b.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, actualHTR)
}

func hydrateSignedBeaconBlock() *eth.SignedBeaconBlock {
	return &eth.SignedBeaconBlock{
		Signature: make([]byte, fieldparams.BLSSignatureLength),
		Block:     hydrateBeaconBlock(),
	}
}

func hydrateBeaconBlock() *eth.BeaconBlock {
	return &eth.BeaconBlock{
		ParentRoot: make([]byte, fieldparams.RootLength),
		StateRoot:  make([]byte, fieldparams.RootLength),
		Body:       hydrateBeaconBlockBody(),
	}
}

func hydrateBeaconBlockBody() *eth.BeaconBlockBody {
	return &eth.BeaconBlockBody{
		RandaoReveal: make([]byte, fieldparams.BLSSignatureLength),
		Graffiti:     make([]byte, fieldparams.RootLength),
		Eth1Data: &eth.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, fieldparams.RootLength),
		},
	}
}

func hydrateBeaconBlockBodyAltair() *eth.BeaconBlockBodyAltair {
	return &eth.BeaconBlockBodyAltair{
		RandaoReveal: make([]byte, fieldparams.BLSSignatureLength),
		Graffiti:     make([]byte, fieldparams.RootLength),
		Eth1Data: &eth.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, fieldparams.RootLength),
		},
		SyncAggregate: &eth.SyncAggregate{
			SyncCommitteeBits:      make([]byte, 64),
			SyncCommitteeSignature: make([]byte, fieldparams.BLSSignatureLength),
		},
	}
}

func hydrateBeaconBlockBodyBellatrix() *eth.BeaconBlockBodyBellatrix {
	return &eth.BeaconBlockBodyBellatrix{
		RandaoReveal: make([]byte, fieldparams.BLSSignatureLength),
		Graffiti:     make([]byte, fieldparams.RootLength),
		Eth1Data: &eth.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, fieldparams.RootLength),
		},
		SyncAggregate: &eth.SyncAggregate{
			SyncCommitteeBits:      make([]byte, 64),
			SyncCommitteeSignature: make([]byte, fieldparams.BLSSignatureLength),
		},
		ExecutionPayload: &pb.ExecutionPayload{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			ExtraData:     make([]byte, 0),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
			Transactions:  make([][]byte, 0),
		},
	}
}

func hydrateBeaconBlockBodyCapella() *eth.BeaconBlockBodyCapella {
	return &eth.BeaconBlockBodyCapella{
		RandaoReveal: make([]byte, fieldparams.BLSSignatureLength),
		Graffiti:     make([]byte, fieldparams.RootLength),
		Eth1Data: &eth.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, fieldparams.RootLength),
		},
		SyncAggregate: &eth.SyncAggregate{
			SyncCommitteeBits:      make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength),
			SyncCommitteeSignature: make([]byte, fieldparams.BLSSignatureLength),
		},
		ExecutionPayload: &pb.ExecutionPayloadCapella{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			ExtraData:     make([]byte, 0),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
			Transactions:  make([][]byte, 0),
			Withdrawals:   make([]*pb.Withdrawal, 0),
		},
	}
}

func hydrateBeaconBlockBodyDeneb() *eth.BeaconBlockBodyDeneb {
	return &eth.BeaconBlockBodyDeneb{
		RandaoReveal: make([]byte, fieldparams.BLSSignatureLength),
		Graffiti:     make([]byte, fieldparams.RootLength),
		Eth1Data: &eth.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, fieldparams.RootLength),
		},
		SyncAggregate: &eth.SyncAggregate{
			SyncCommitteeBits:      make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength),
			SyncCommitteeSignature: make([]byte, fieldparams.BLSSignatureLength),
		},
		ExecutionPayload: &pb.ExecutionPayloadDeneb{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			ExtraData:     make([]byte, 0),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
			Transactions:  make([][]byte, 0),
			Withdrawals:   make([]*pb.Withdrawal, 0),
		},
	}
}

func hydrateBeaconBlockBodyElectra() *eth.BeaconBlockBodyElectra {
	return &eth.BeaconBlockBodyElectra{
		RandaoReveal: make([]byte, fieldparams.BLSSignatureLength),
		Graffiti:     make([]byte, fieldparams.RootLength),
		Eth1Data: &eth.Eth1Data{
			DepositRoot: make([]byte, fieldparams.RootLength),
			BlockHash:   make([]byte, fieldparams.RootLength),
		},
		SyncAggregate: &eth.SyncAggregate{
			SyncCommitteeBits:      make([]byte, fieldparams.SyncAggregateSyncCommitteeBytesLength),
			SyncCommitteeSignature: make([]byte, fieldparams.BLSSignatureLength),
		},
		ExecutionPayload: &pb.ExecutionPayloadElectra{
			ParentHash:    make([]byte, fieldparams.RootLength),
			FeeRecipient:  make([]byte, 20),
			StateRoot:     make([]byte, fieldparams.RootLength),
			ReceiptsRoot:  make([]byte, fieldparams.RootLength),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, fieldparams.RootLength),
			ExtraData:     make([]byte, 0),
			BaseFeePerGas: make([]byte, fieldparams.RootLength),
			BlockHash:     make([]byte, fieldparams.RootLength),
			Transactions:  make([][]byte, 0),
			Withdrawals:   make([]*pb.Withdrawal, 0),
		},
		ExecutionRequests: &pb.ExecutionRequests{
			Deposits:       make([]*pb.DepositRequest, 0),
			Withdrawals:    make([]*pb.WithdrawalRequest, 0),
			Consolidations: make([]*pb.ConsolidationRequest, 0),
		},
	}
}
