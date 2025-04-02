package eth_test

import (
	"math/rand"
	"reflect"
	"testing"

	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	v1alpha1 "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
)

func TestCopySignedBeaconBlock(t *testing.T) {
	blk := genSignedBeaconBlock()

	got := blk.Copy()
	if !reflect.DeepEqual(got, blk) {
		t.Errorf("CopySignedBeaconBlock() = %v, want %v", got, blk)
	}
	assert.NotEmpty(t, got, "Copied signed beacon block has empty fields")
}

func TestCopyBeaconBlock(t *testing.T) {
	blk := genBeaconBlock()

	got := blk.Copy()
	if !reflect.DeepEqual(got, blk) {
		t.Errorf("CopyBeaconBlock() = %v, want %v", got, blk)
	}
	assert.NotEmpty(t, got, "Copied beacon block has empty fields")
}

func TestCopyBeaconBlockBody(t *testing.T) {
	body := genBeaconBlockBody()

	got := body.Copy()
	if !reflect.DeepEqual(got, body) {
		t.Errorf("CopyBeaconBlockBody() = %v, want %v", got, body)
	}
	assert.NotEmpty(t, got, "Copied beacon block body has empty fields")
}

func TestCopySignedBeaconBlockAltair(t *testing.T) {
	sbb := genSignedBeaconBlockAltair()

	got := sbb.Copy()
	if !reflect.DeepEqual(got, sbb) {
		t.Errorf("CopySignedBeaconBlockAltair() = %v, want %v", got, sbb)
	}
	assert.NotEmpty(t, sbb, "Copied signed beacon block altair has empty fields")
}

func TestCopyBeaconBlockAltair(t *testing.T) {
	b := genBeaconBlockAltair()

	got := b.Copy()
	if !reflect.DeepEqual(got, b) {
		t.Errorf("CopyBeaconBlockAltair() = %v, want %v", got, b)
	}
	assert.NotEmpty(t, b, "Copied beacon block altair has empty fields")
}

func TestCopyBeaconBlockBodyAltair(t *testing.T) {
	bb := genBeaconBlockBodyAltair()

	got := bb.Copy()
	if !reflect.DeepEqual(got, bb) {
		t.Errorf("CopyBeaconBlockBodyAltair() = %v, want %v", got, bb)
	}
	assert.NotEmpty(t, bb, "Copied beacon block body altair has empty fields")
}

func TestCopyValidator(t *testing.T) {
	v := genValidator()

	got := v1alpha1.CopyValidator(v)
	if !reflect.DeepEqual(got, v) {
		t.Errorf("CopyValidator() = %v, want %v", got, v)
	}
	assert.NotEmpty(t, got, "Copied validator has empty fields")
}

func TestCopySyncCommitteeMessage(t *testing.T) {
	scm := genSyncCommitteeMessage()

	got := v1alpha1.CopySyncCommitteeMessage(scm)
	if !reflect.DeepEqual(got, scm) {
		t.Errorf("CopySyncCommitteeMessage() = %v, want %v", got, scm)
	}
	assert.NotEmpty(t, got, "Copied sync committee message has empty fields")
}

func TestCopySyncCommitteeContribution(t *testing.T) {
	scc := genSyncCommitteeContribution()

	got := v1alpha1.CopySyncCommitteeContribution(scc)
	if !reflect.DeepEqual(got, scc) {
		t.Errorf("CopySyncCommitteeContribution() = %v, want %v", got, scc)
	}
	assert.NotEmpty(t, got, "Copied sync committee contribution has empty fields")
}

func TestCopyPendingAttestationSlice(t *testing.T) {
	tests := []struct {
		name  string
		input []*v1alpha1.PendingAttestation
	}{
		{
			name:  "nil",
			input: nil,
		},
		{
			name:  "empty",
			input: []*v1alpha1.PendingAttestation{},
		},
		{
			name: "correct copy",
			input: []*v1alpha1.PendingAttestation{
				genPendingAttestation(),
				genPendingAttestation(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.input; !reflect.DeepEqual(got, tt.input) {
				t.Errorf("CopyPendingAttestationSlice() = %v, want %v", got, tt.input)
			}
		})
	}
}

func TestCopySignedBeaconBlockBellatrix(t *testing.T) {
	sbb := genSignedBeaconBlockBellatrix()

	got := sbb.Copy()
	if !reflect.DeepEqual(got, sbb) {
		t.Errorf("CopySignedBeaconBlockBellatrix() = %v, want %v", got, sbb)
	}
	assert.NotEmpty(t, sbb, "Copied signed beacon block Bellatrix has empty fields")
}

func TestCopyBeaconBlockBellatrix(t *testing.T) {
	b := genBeaconBlockBellatrix()

	got := b.Copy()
	if !reflect.DeepEqual(got, b) {
		t.Errorf("CopyBeaconBlockBellatrix() = %v, want %v", got, b)
	}
	assert.NotEmpty(t, b, "Copied beacon block Bellatrix has empty fields")
}

func TestCopyBeaconBlockBodyBellatrix(t *testing.T) {
	bb := genBeaconBlockBodyBellatrix()

	got := bb.Copy()
	if !reflect.DeepEqual(got, bb) {
		t.Errorf("CopyBeaconBlockBodyBellatrix() = %v, want %v", got, bb)
	}
	assert.NotEmpty(t, bb, "Copied beacon block body Bellatrix has empty fields")
}

func TestCopySignedBlindedBeaconBlockBellatrix(t *testing.T) {
	sbb := genSignedBeaconBlockBellatrix()

	got := sbb.Copy()
	if !reflect.DeepEqual(got, sbb) {
		t.Errorf("CopySignedBeaconBlockBellatrix() = %v, want %v", got, sbb)
	}
	assert.NotEmpty(t, sbb, "Copied signed blinded beacon block Bellatrix has empty fields")
}

func TestCopyBlindedBeaconBlockBellatrix(t *testing.T) {
	b := genBeaconBlockBellatrix()

	got := b.Copy()
	if !reflect.DeepEqual(got, b) {
		t.Errorf("CopyBeaconBlockBellatrix() = %v, want %v", got, b)
	}
	assert.NotEmpty(t, b, "Copied blinded beacon block Bellatrix has empty fields")
}

func TestCopyBlindedBeaconBlockBodyBellatrix(t *testing.T) {
	bb := genBeaconBlockBodyBellatrix()

	got := bb.Copy()
	if !reflect.DeepEqual(got, bb) {
		t.Errorf("CopyBeaconBlockBodyBellatrix() = %v, want %v", got, bb)
	}
	assert.NotEmpty(t, bb, "Copied blinded beacon block body Bellatrix has empty fields")
}

func TestCopySignedBeaconBlockCapella(t *testing.T) {
	sbb := genSignedBeaconBlockCapella()

	got := sbb.Copy()
	if !reflect.DeepEqual(got, sbb) {
		t.Errorf("CopySignedBeaconBlockCapella() = %v, want %v", got, sbb)
	}
	assert.NotEmpty(t, sbb, "Copied signed beacon block Capella has empty fields")
}

func TestCopyBeaconBlockCapella(t *testing.T) {
	b := genBeaconBlockCapella()

	got := b.Copy()
	if !reflect.DeepEqual(got, b) {
		t.Errorf("CopyBeaconBlockCapella() = %v, want %v", got, b)
	}
	assert.NotEmpty(t, b, "Copied beacon block Capella has empty fields")
}

func TestCopyBeaconBlockBodyCapella(t *testing.T) {
	bb := genBeaconBlockBodyCapella()

	got := bb.Copy()
	if !reflect.DeepEqual(got, bb) {
		t.Errorf("CopyBeaconBlockBodyCapella() = %v, want %v", got, bb)
	}
	assert.NotEmpty(t, bb, "Copied beacon block body Capella has empty fields")
}

func TestCopySignedBlindedBeaconBlockCapella(t *testing.T) {
	sbb := genSignedBlindedBeaconBlockCapella()

	got := sbb.Copy()
	if !reflect.DeepEqual(got, sbb) {
		t.Errorf("CopySignedBlindedBeaconBlockCapella() = %v, want %v", got, sbb)
	}
	assert.NotEmpty(t, sbb, "Copied signed blinded beacon block Capella has empty fields")
}

func TestCopyBlindedBeaconBlockCapella(t *testing.T) {
	b := genBlindedBeaconBlockCapella()

	got := b.Copy()
	if !reflect.DeepEqual(got, b) {
		t.Errorf("CopyBlindedBeaconBlockCapella() = %v, want %v", got, b)
	}
	assert.NotEmpty(t, b, "Copied blinded beacon block Capella has empty fields")
}

func TestCopyBlindedBeaconBlockBodyCapella(t *testing.T) {
	bb := genBlindedBeaconBlockBodyCapella()

	got := bb.Copy()
	if !reflect.DeepEqual(got, bb) {
		t.Errorf("CopyBlindedBeaconBlockBodyCapella() = %v, want %v", got, bb)
	}
	assert.NotEmpty(t, bb, "Copied blinded beacon block body Capella has empty fields")
}

func TestCopySignedBeaconBlockDeneb(t *testing.T) {
	sbb := genSignedBeaconBlockDeneb()

	got := sbb.Copy()
	if !reflect.DeepEqual(got, sbb) {
		t.Errorf("CopySignedBeaconBlockDeneb() = %v, want %v", got, sbb)
	}
	assert.NotEmpty(t, sbb, "Copied signed beacon block Deneb has empty fields")
}

func TestCopyBeaconBlockDeneb(t *testing.T) {
	b := genBeaconBlockDeneb()

	got := b.Copy()
	if !reflect.DeepEqual(got, b) {
		t.Errorf("CopyBeaconBlockDeneb() = %v, want %v", got, b)
	}
	assert.NotEmpty(t, b, "Copied beacon block Deneb has empty fields")
}

func TestCopyBeaconBlockBodyDeneb(t *testing.T) {
	bb := genBeaconBlockBodyDeneb()

	got := bb.Copy()
	if !reflect.DeepEqual(got, bb) {
		t.Errorf("CopyBeaconBlockBodyDeneb() = %v, want %v", got, bb)
	}
	assert.NotEmpty(t, bb, "Copied beacon block body Deneb has empty fields")
}

func TestCopySignedBlindedBeaconBlockDeneb(t *testing.T) {
	sbb := genSignedBlindedBeaconBlockDeneb()

	got := sbb.Copy()
	if !reflect.DeepEqual(got, sbb) {
		t.Errorf("CopySignedBlindedBeaconBlockDeneb() = %v, want %v", got, sbb)
	}
	assert.NotEmpty(t, sbb, "Copied signed blinded beacon block Deneb has empty fields")
}

func TestCopyBlindedBeaconBlockDeneb(t *testing.T) {
	b := genBlindedBeaconBlockDeneb()

	got := b.Copy()
	if !reflect.DeepEqual(got, b) {
		t.Errorf("CopyBlindedBeaconBlockDeneb() = %v, want %v", got, b)
	}
	assert.NotEmpty(t, b, "Copied blinded beacon block Deneb has empty fields")
}

func TestCopyBlindedBeaconBlockBodyDeneb(t *testing.T) {
	bb := genBlindedBeaconBlockBodyDeneb()

	got := bb.Copy()
	if !reflect.DeepEqual(got, bb) {
		t.Errorf("CopyBlindedBeaconBlockBodyDeneb() = %v, want %v", got, bb)
	}
	assert.NotEmpty(t, bb, "Copied blinded beacon block body Deneb has empty fields")
}

func bytes(length int) []byte {
	b := make([]byte, length)
	for i := 0; i < length; i++ {
		b[i] = uint8(rand.Int31n(255) + 1)
	}
	return b
}

func TestCopySignedBlindedBeaconBlockElectra(t *testing.T) {
	sbb := genSignedBlindedBeaconBlockElectra()

	got := sbb.Copy()
	if !reflect.DeepEqual(got, sbb) {
		t.Errorf("TestCopySignedBlindedBeaconBlockElectra() = %v, want %v", got, sbb)
	}
}

func TestCopyBlindedBeaconBlockElectra(t *testing.T) {
	b := genBlindedBeaconBlockElectra()

	got := b.Copy()
	if !reflect.DeepEqual(got, b) {
		t.Errorf("TestCopyBlindedBeaconBlockElectra() = %v, want %v", got, b)
	}
}

func TestCopyBlindedBeaconBlockBodyElectra(t *testing.T) {
	bb := genBlindedBeaconBlockBodyElectra()

	got := bb.Copy()
	if !reflect.DeepEqual(got, bb) {
		t.Errorf("TestCopyBlindedBeaconBlockBodyElectra() = %v, want %v", got, bb)
	}
}

func TestCopySignedBeaconBlockElectra(t *testing.T) {
	sbb := genSignedBeaconBlockElectra()

	got := sbb.Copy()
	if !reflect.DeepEqual(got, sbb) {
		t.Errorf("TestCopySignedBeaconBlockElectra() = %v, want %v", got, sbb)
	}
}

func TestCopyBeaconBlockElectra(t *testing.T) {
	b := genBeaconBlockElectra()

	got := b.Copy()
	if !reflect.DeepEqual(got, b) {
		t.Errorf("TestCopyBeaconBlockElectra() = %v, want %v", got, b)
	}
}

func TestCopyBeaconBlockBodyElectra(t *testing.T) {
	bb := genBeaconBlockBodyElectra()

	got := bb.Copy()
	if !reflect.DeepEqual(got, bb) {
		t.Errorf("TestCopyBeaconBlockBodyElectra() = %v, want %v", got, bb)
	}
}

func genAttestation() *v1alpha1.Attestation {
	return &v1alpha1.Attestation{
		AggregationBits: bytes(32),
		Data:            genAttData(),
		Signature:       bytes(32),
	}
}

func genAttestations(num int) []*v1alpha1.Attestation {
	atts := make([]*v1alpha1.Attestation, num)
	for i := 0; i < num; i++ {
		atts[i] = genAttestation()
	}
	return atts
}

func genAttData() *v1alpha1.AttestationData {
	return &v1alpha1.AttestationData{
		Slot:            1,
		CommitteeIndex:  2,
		BeaconBlockRoot: bytes(32),
		Source:          genCheckpoint(),
		Target:          genCheckpoint(),
	}
}

func genCheckpoint() *v1alpha1.Checkpoint {
	return &v1alpha1.Checkpoint{
		Epoch: 1,
		Root:  bytes(32),
	}
}

func genEth1Data() *v1alpha1.Eth1Data {
	return &v1alpha1.Eth1Data{
		DepositRoot:  bytes(32),
		DepositCount: 4,
		BlockHash:    bytes(32),
	}
}

func genPendingAttestation() *v1alpha1.PendingAttestation {
	return &v1alpha1.PendingAttestation{
		AggregationBits: bytes(32),
		Data:            genAttData(),
		InclusionDelay:  3,
		ProposerIndex:   5,
	}
}

func genSignedBeaconBlock() *v1alpha1.SignedBeaconBlock {
	return &v1alpha1.SignedBeaconBlock{
		Block:     genBeaconBlock(),
		Signature: bytes(32),
	}
}

func genBeaconBlock() *v1alpha1.BeaconBlock {
	return &v1alpha1.BeaconBlock{
		Slot:          4,
		ProposerIndex: 5,
		ParentRoot:    bytes(32),
		StateRoot:     bytes(32),
		Body:          genBeaconBlockBody(),
	}
}

func genBeaconBlockBody() *v1alpha1.BeaconBlockBody {
	return &v1alpha1.BeaconBlockBody{
		RandaoReveal:      bytes(32),
		Eth1Data:          genEth1Data(),
		Graffiti:          bytes(32),
		ProposerSlashings: genProposerSlashings(5),
		AttesterSlashings: genAttesterSlashings(5),
		Attestations:      genAttestations(5),
		Deposits:          genDeposits(5),
		VoluntaryExits:    genSignedVoluntaryExits(5),
	}
}

func genProposerSlashing() *v1alpha1.ProposerSlashing {
	return &v1alpha1.ProposerSlashing{
		Header_1: genSignedBeaconBlockHeader(),
		Header_2: genSignedBeaconBlockHeader(),
	}
}

func genProposerSlashings(num int) []*v1alpha1.ProposerSlashing {
	ps := make([]*v1alpha1.ProposerSlashing, num)
	for i := 0; i < num; i++ {
		ps[i] = genProposerSlashing()
	}
	return ps
}

func genAttesterSlashing() *v1alpha1.AttesterSlashing {
	return &v1alpha1.AttesterSlashing{
		Attestation_1: genIndexedAttestation(),
		Attestation_2: genIndexedAttestation(),
	}
}

func genIndexedAttestation() *v1alpha1.IndexedAttestation {
	return &v1alpha1.IndexedAttestation{
		AttestingIndices: []uint64{1, 2, 3},
		Data:             genAttData(),
		Signature:        bytes(32),
	}
}

func genAttesterSlashings(num int) []*v1alpha1.AttesterSlashing {
	as := make([]*v1alpha1.AttesterSlashing, num)
	for i := 0; i < num; i++ {
		as[i] = genAttesterSlashing()
	}
	return as
}

func genBeaconBlockHeader() *v1alpha1.BeaconBlockHeader {
	return &v1alpha1.BeaconBlockHeader{
		Slot:          10,
		ProposerIndex: 15,
		ParentRoot:    bytes(32),
		StateRoot:     bytes(32),
		BodyRoot:      bytes(32),
	}
}

func genSignedBeaconBlockHeader() *v1alpha1.SignedBeaconBlockHeader {
	return &v1alpha1.SignedBeaconBlockHeader{
		Header:    genBeaconBlockHeader(),
		Signature: bytes(32),
	}
}

func genDepositData() *v1alpha1.Deposit_Data {
	return &v1alpha1.Deposit_Data{
		PublicKey:             bytes(32),
		WithdrawalCredentials: bytes(32),
		Amount:                20000,
		Signature:             bytes(32),
	}
}

func genDeposit() *v1alpha1.Deposit {
	return &v1alpha1.Deposit{
		Data:  genDepositData(),
		Proof: [][]byte{bytes(32), bytes(32), bytes(32), bytes(32)},
	}
}

func genDeposits(num int) []*v1alpha1.Deposit {
	d := make([]*v1alpha1.Deposit, num)
	for i := 0; i < num; i++ {
		d[i] = genDeposit()
	}
	return d
}

func genVoluntaryExit() *v1alpha1.VoluntaryExit {
	return &v1alpha1.VoluntaryExit{
		Epoch:          5432,
		ValidatorIndex: 888888,
	}
}

func genSignedVoluntaryExit() *v1alpha1.SignedVoluntaryExit {
	return &v1alpha1.SignedVoluntaryExit{
		Exit:      genVoluntaryExit(),
		Signature: bytes(32),
	}
}

func genSignedVoluntaryExits(num int) []*v1alpha1.SignedVoluntaryExit {
	sv := make([]*v1alpha1.SignedVoluntaryExit, num)
	for i := 0; i < num; i++ {
		sv[i] = genSignedVoluntaryExit()
	}
	return sv
}

func genValidator() *v1alpha1.Validator {
	return &v1alpha1.Validator{
		PublicKey:                  bytes(32),
		WithdrawalCredentials:      bytes(32),
		EffectiveBalance:           12345,
		Slashed:                    true,
		ActivationEligibilityEpoch: 14322,
		ActivationEpoch:            14325,
		ExitEpoch:                  23425,
		WithdrawableEpoch:          30000,
	}
}

func genSyncCommitteeContribution() *v1alpha1.SyncCommitteeContribution {
	return &v1alpha1.SyncCommitteeContribution{
		Slot:              12333,
		BlockRoot:         bytes(32),
		SubcommitteeIndex: 4444,
		AggregationBits:   bytes(32),
		Signature:         bytes(32),
	}
}

func genSyncAggregate() *v1alpha1.SyncAggregate {
	return &v1alpha1.SyncAggregate{
		SyncCommitteeBits:      bytes(32),
		SyncCommitteeSignature: bytes(32),
	}
}

func genBeaconBlockBodyAltair() *v1alpha1.BeaconBlockBodyAltair {
	return &v1alpha1.BeaconBlockBodyAltair{
		RandaoReveal:      bytes(32),
		Eth1Data:          genEth1Data(),
		Graffiti:          bytes(32),
		ProposerSlashings: genProposerSlashings(5),
		AttesterSlashings: genAttesterSlashings(5),
		Attestations:      genAttestations(10),
		Deposits:          genDeposits(5),
		VoluntaryExits:    genSignedVoluntaryExits(12),
		SyncAggregate:     genSyncAggregate(),
	}
}

func genBeaconBlockAltair() *v1alpha1.BeaconBlockAltair {
	return &v1alpha1.BeaconBlockAltair{
		Slot:          123455,
		ProposerIndex: 55433,
		ParentRoot:    bytes(32),
		StateRoot:     bytes(32),
		Body:          genBeaconBlockBodyAltair(),
	}
}

func genSignedBeaconBlockAltair() *v1alpha1.SignedBeaconBlockAltair {
	return &v1alpha1.SignedBeaconBlockAltair{
		Block:     genBeaconBlockAltair(),
		Signature: bytes(32),
	}
}

func genBeaconBlockBodyBellatrix() *v1alpha1.BeaconBlockBodyBellatrix {
	return &v1alpha1.BeaconBlockBodyBellatrix{
		RandaoReveal:      bytes(32),
		Eth1Data:          genEth1Data(),
		Graffiti:          bytes(32),
		ProposerSlashings: genProposerSlashings(5),
		AttesterSlashings: genAttesterSlashings(5),
		Attestations:      genAttestations(10),
		Deposits:          genDeposits(5),
		VoluntaryExits:    genSignedVoluntaryExits(12),
		SyncAggregate:     genSyncAggregate(),
		ExecutionPayload:  genPayload(),
	}
}

func genBeaconBlockBellatrix() *v1alpha1.BeaconBlockBellatrix {
	return &v1alpha1.BeaconBlockBellatrix{
		Slot:          123455,
		ProposerIndex: 55433,
		ParentRoot:    bytes(32),
		StateRoot:     bytes(32),
		Body:          genBeaconBlockBodyBellatrix(),
	}
}

func genSignedBeaconBlockBellatrix() *v1alpha1.SignedBeaconBlockBellatrix {
	return &v1alpha1.SignedBeaconBlockBellatrix{
		Block:     genBeaconBlockBellatrix(),
		Signature: bytes(32),
	}
}

func genBeaconBlockBodyCapella() *v1alpha1.BeaconBlockBodyCapella {
	return &v1alpha1.BeaconBlockBodyCapella{
		RandaoReveal:          bytes(96),
		Eth1Data:              genEth1Data(),
		Graffiti:              bytes(32),
		ProposerSlashings:     genProposerSlashings(5),
		AttesterSlashings:     genAttesterSlashings(5),
		Attestations:          genAttestations(10),
		Deposits:              genDeposits(5),
		VoluntaryExits:        genSignedVoluntaryExits(12),
		SyncAggregate:         genSyncAggregate(),
		ExecutionPayload:      genPayloadCapella(),
		BlsToExecutionChanges: genBLSToExecutionChanges(10),
	}
}

func genBeaconBlockCapella() *v1alpha1.BeaconBlockCapella {
	return &v1alpha1.BeaconBlockCapella{
		Slot:          123455,
		ProposerIndex: 55433,
		ParentRoot:    bytes(32),
		StateRoot:     bytes(32),
		Body:          genBeaconBlockBodyCapella(),
	}
}

func genSignedBeaconBlockCapella() *v1alpha1.SignedBeaconBlockCapella {
	return &v1alpha1.SignedBeaconBlockCapella{
		Block:     genBeaconBlockCapella(),
		Signature: bytes(96),
	}
}

func genBlindedBeaconBlockBodyCapella() *v1alpha1.BlindedBeaconBlockBodyCapella {
	return &v1alpha1.BlindedBeaconBlockBodyCapella{
		RandaoReveal:           bytes(96),
		Eth1Data:               genEth1Data(),
		Graffiti:               bytes(32),
		ProposerSlashings:      genProposerSlashings(5),
		AttesterSlashings:      genAttesterSlashings(5),
		Attestations:           genAttestations(10),
		Deposits:               genDeposits(5),
		VoluntaryExits:         genSignedVoluntaryExits(12),
		SyncAggregate:          genSyncAggregate(),
		ExecutionPayloadHeader: genPayloadHeaderCapella(),
		BlsToExecutionChanges:  genBLSToExecutionChanges(10),
	}
}

func genBlindedBeaconBlockCapella() *v1alpha1.BlindedBeaconBlockCapella {
	return &v1alpha1.BlindedBeaconBlockCapella{
		Slot:          123455,
		ProposerIndex: 55433,
		ParentRoot:    bytes(32),
		StateRoot:     bytes(32),
		Body:          genBlindedBeaconBlockBodyCapella(),
	}
}

func genSignedBlindedBeaconBlockCapella() *v1alpha1.SignedBlindedBeaconBlockCapella {
	return &v1alpha1.SignedBlindedBeaconBlockCapella{
		Block:     genBlindedBeaconBlockCapella(),
		Signature: bytes(32),
	}
}

func genBeaconBlockBodyDeneb() *v1alpha1.BeaconBlockBodyDeneb {
	return &v1alpha1.BeaconBlockBodyDeneb{
		RandaoReveal:          bytes(96),
		Eth1Data:              genEth1Data(),
		Graffiti:              bytes(32),
		ProposerSlashings:     genProposerSlashings(5),
		AttesterSlashings:     genAttesterSlashings(5),
		Attestations:          genAttestations(10),
		Deposits:              genDeposits(5),
		VoluntaryExits:        genSignedVoluntaryExits(12),
		SyncAggregate:         genSyncAggregate(),
		ExecutionPayload:      genPayloadDeneb(),
		BlsToExecutionChanges: genBLSToExecutionChanges(10),
		BlobKzgCommitments:    getKZGCommitments(4),
	}
}

func genBeaconBlockDeneb() *v1alpha1.BeaconBlockDeneb {
	return &v1alpha1.BeaconBlockDeneb{
		Slot:          123455,
		ProposerIndex: 55433,
		ParentRoot:    bytes(32),
		StateRoot:     bytes(32),
		Body:          genBeaconBlockBodyDeneb(),
	}
}

func genSignedBeaconBlockDeneb() *v1alpha1.SignedBeaconBlockDeneb {
	return &v1alpha1.SignedBeaconBlockDeneb{
		Block:     genBeaconBlockDeneb(),
		Signature: bytes(96),
	}
}

func genBlindedBeaconBlockBodyDeneb() *v1alpha1.BlindedBeaconBlockBodyDeneb {
	return &v1alpha1.BlindedBeaconBlockBodyDeneb{
		RandaoReveal:           bytes(96),
		Eth1Data:               genEth1Data(),
		Graffiti:               bytes(32),
		ProposerSlashings:      genProposerSlashings(5),
		AttesterSlashings:      genAttesterSlashings(5),
		Attestations:           genAttestations(10),
		Deposits:               genDeposits(5),
		VoluntaryExits:         genSignedVoluntaryExits(12),
		SyncAggregate:          genSyncAggregate(),
		ExecutionPayloadHeader: genPayloadHeaderDeneb(),
		BlsToExecutionChanges:  genBLSToExecutionChanges(10),
		BlobKzgCommitments:     getKZGCommitments(4),
	}
}

func getKZGCommitments(n int) [][]byte {
	kzgs := make([][]byte, n)
	for i := 0; i < n; i++ {
		kzgs[i] = bytes(48)
	}
	return kzgs
}

func genBlindedBeaconBlockDeneb() *v1alpha1.BlindedBeaconBlockDeneb {
	return &v1alpha1.BlindedBeaconBlockDeneb{
		Slot:          123455,
		ProposerIndex: 55433,
		ParentRoot:    bytes(32),
		StateRoot:     bytes(32),
		Body:          genBlindedBeaconBlockBodyDeneb(),
	}
}

func genSignedBlindedBeaconBlockDeneb() *v1alpha1.SignedBlindedBeaconBlockDeneb {
	return &v1alpha1.SignedBlindedBeaconBlockDeneb{
		Message:   genBlindedBeaconBlockDeneb(),
		Signature: bytes(32),
	}
}

func genSyncCommitteeMessage() *v1alpha1.SyncCommitteeMessage {
	return &v1alpha1.SyncCommitteeMessage{
		Slot:           424555,
		BlockRoot:      bytes(32),
		ValidatorIndex: 5443,
		Signature:      bytes(32),
	}
}

func genPayload() *enginev1.ExecutionPayload {
	return &enginev1.ExecutionPayload{
		ParentHash:    bytes(32),
		FeeRecipient:  bytes(32),
		StateRoot:     bytes(32),
		ReceiptsRoot:  bytes(32),
		LogsBloom:     bytes(32),
		PrevRandao:    bytes(32),
		BlockNumber:   1,
		GasLimit:      2,
		GasUsed:       3,
		Timestamp:     4,
		ExtraData:     bytes(32),
		BaseFeePerGas: bytes(32),
		BlockHash:     bytes(32),
		Transactions:  [][]byte{{'a'}, {'b'}, {'c'}},
	}
}

func genPayloadCapella() *enginev1.ExecutionPayloadCapella {
	return &enginev1.ExecutionPayloadCapella{
		ParentHash:    bytes(32),
		FeeRecipient:  bytes(20),
		StateRoot:     bytes(32),
		ReceiptsRoot:  bytes(32),
		LogsBloom:     bytes(256),
		PrevRandao:    bytes(32),
		BlockNumber:   1,
		GasLimit:      2,
		GasUsed:       3,
		Timestamp:     4,
		ExtraData:     bytes(32),
		BaseFeePerGas: bytes(32),
		BlockHash:     bytes(32),
		Transactions:  [][]byte{{'a'}, {'b'}, {'c'}},
		Withdrawals: []*enginev1.Withdrawal{
			{
				Index:          123,
				ValidatorIndex: 123,
				Address:        bytes(20),
				Amount:         123,
			},
			{
				Index:          124,
				ValidatorIndex: 456,
				Address:        bytes(20),
				Amount:         456,
			},
		},
	}
}

func genPayloadDeneb() *enginev1.ExecutionPayloadDeneb {
	return &enginev1.ExecutionPayloadDeneb{
		ParentHash:    bytes(32),
		FeeRecipient:  bytes(20),
		StateRoot:     bytes(32),
		ReceiptsRoot:  bytes(32),
		LogsBloom:     bytes(256),
		PrevRandao:    bytes(32),
		BlockNumber:   1,
		GasLimit:      2,
		GasUsed:       3,
		Timestamp:     4,
		ExtraData:     bytes(32),
		BaseFeePerGas: bytes(32),
		BlockHash:     bytes(32),
		Transactions:  [][]byte{{'a'}, {'b'}, {'c'}},
		Withdrawals: []*enginev1.Withdrawal{
			{
				Index:          123,
				ValidatorIndex: 123,
				Address:        bytes(20),
				Amount:         123,
			},
			{
				Index:          124,
				ValidatorIndex: 456,
				Address:        bytes(20),
				Amount:         456,
			},
		},
		BlobGasUsed:   5,
		ExcessBlobGas: 6,
	}
}

var genPayloadElectra = genPayloadDeneb

func genPayloadHeader() *enginev1.ExecutionPayloadHeader {
	return &enginev1.ExecutionPayloadHeader{
		ParentHash:       bytes(32),
		FeeRecipient:     bytes(32),
		StateRoot:        bytes(32),
		ReceiptsRoot:     bytes(32),
		LogsBloom:        bytes(32),
		PrevRandao:       bytes(32),
		BlockNumber:      1,
		GasLimit:         2,
		GasUsed:          3,
		Timestamp:        4,
		ExtraData:        bytes(32),
		BaseFeePerGas:    bytes(32),
		BlockHash:        bytes(32),
		TransactionsRoot: bytes(32),
	}
}

func genPayloadHeaderCapella() *enginev1.ExecutionPayloadHeaderCapella {
	return &enginev1.ExecutionPayloadHeaderCapella{
		ParentHash:       bytes(32),
		FeeRecipient:     bytes(20),
		StateRoot:        bytes(32),
		ReceiptsRoot:     bytes(32),
		LogsBloom:        bytes(256),
		PrevRandao:       bytes(32),
		BlockNumber:      1,
		GasLimit:         2,
		GasUsed:          3,
		Timestamp:        4,
		ExtraData:        bytes(32),
		BaseFeePerGas:    bytes(32),
		BlockHash:        bytes(32),
		TransactionsRoot: bytes(32),
		WithdrawalsRoot:  bytes(32),
	}
}

func genPayloadHeaderDeneb() *enginev1.ExecutionPayloadHeaderDeneb {
	return &enginev1.ExecutionPayloadHeaderDeneb{
		ParentHash:       bytes(32),
		FeeRecipient:     bytes(20),
		StateRoot:        bytes(32),
		ReceiptsRoot:     bytes(32),
		LogsBloom:        bytes(256),
		PrevRandao:       bytes(32),
		BlockNumber:      1,
		GasLimit:         2,
		GasUsed:          3,
		Timestamp:        4,
		ExtraData:        bytes(32),
		BaseFeePerGas:    bytes(32),
		BlockHash:        bytes(32),
		TransactionsRoot: bytes(32),
		WithdrawalsRoot:  bytes(32),
		BlobGasUsed:      5,
		ExcessBlobGas:    6,
	}
}

var genPayloadHeaderElectra = genPayloadHeaderDeneb

func genWithdrawals(num int) []*enginev1.Withdrawal {
	ws := make([]*enginev1.Withdrawal, num)
	for i := 0; i < num; i++ {
		ws[i] = genWithdrawal()
	}
	return ws
}

func genWithdrawal() *enginev1.Withdrawal {
	return &enginev1.Withdrawal{
		Index:          123456,
		ValidatorIndex: 654321,
		Address:        bytes(20),
		Amount:         55555,
	}
}

func genBLSToExecutionChanges(num int) []*v1alpha1.SignedBLSToExecutionChange {
	changes := make([]*v1alpha1.SignedBLSToExecutionChange, num)
	for i := 0; i < num; i++ {
		changes[i] = genBLSToExecutionChange()
	}
	return changes
}

func genBLSToExecutionChange() *v1alpha1.SignedBLSToExecutionChange {
	return &v1alpha1.SignedBLSToExecutionChange{
		Message: &v1alpha1.BLSToExecutionChange{
			ValidatorIndex:     123456,
			FromBlsPubkey:      bytes(48),
			ToExecutionAddress: bytes(20),
		},
		Signature: bytes(96),
	}
}

func genAttestationElectra() *v1alpha1.AttestationElectra {
	return &v1alpha1.AttestationElectra{
		AggregationBits: bytes(32),
		CommitteeBits:   bytes(8),
		Data:            genAttData(),
		Signature:       bytes(96),
	}
}

func genAttesterSlashingsElectra(num int) []*v1alpha1.AttesterSlashingElectra {
	as := make([]*v1alpha1.AttesterSlashingElectra, num)
	for i := 0; i < num; i++ {
		as[i] = genAttesterSlashingElectra()
	}
	return as
}

func genAttesterSlashingElectra() *v1alpha1.AttesterSlashingElectra {
	return &v1alpha1.AttesterSlashingElectra{
		Attestation_1: genIndexedAttestationElectra(),
		Attestation_2: genIndexedAttestationElectra(),
	}
}

func genIndexedAttestationElectra() *v1alpha1.IndexedAttestationElectra {
	return &v1alpha1.IndexedAttestationElectra{
		AttestingIndices: []uint64{1, 2, 3},
		Data:             genAttData(),
		Signature:        bytes(96),
	}
}

func genAttestationsElectra(num int) []*v1alpha1.AttestationElectra {
	atts := make([]*v1alpha1.AttestationElectra, num)
	for i := 0; i < num; i++ {
		atts[i] = genAttestationElectra()
	}
	return atts
}

func genSignedBlindedBeaconBlockElectra() *v1alpha1.SignedBlindedBeaconBlockElectra {
	return &v1alpha1.SignedBlindedBeaconBlockElectra{
		Message:   genBlindedBeaconBlockElectra(),
		Signature: bytes(96),
	}
}

func genBlindedBeaconBlockElectra() *v1alpha1.BlindedBeaconBlockElectra {
	return &v1alpha1.BlindedBeaconBlockElectra{
		Slot:          123455,
		ProposerIndex: 55433,
		ParentRoot:    bytes(32),
		StateRoot:     bytes(32),
		Body:          genBlindedBeaconBlockBodyElectra(),
	}
}

func genBlindedBeaconBlockBodyElectra() *v1alpha1.BlindedBeaconBlockBodyElectra {
	return &v1alpha1.BlindedBeaconBlockBodyElectra{
		RandaoReveal:           bytes(96),
		Eth1Data:               genEth1Data(),
		Graffiti:               bytes(32),
		ProposerSlashings:      genProposerSlashings(5),
		AttesterSlashings:      genAttesterSlashingsElectra(5),
		Attestations:           genAttestationsElectra(10),
		Deposits:               genDeposits(5),
		VoluntaryExits:         genSignedVoluntaryExits(12),
		SyncAggregate:          genSyncAggregate(),
		ExecutionPayloadHeader: genPayloadHeaderElectra(),
		BlsToExecutionChanges:  genBLSToExecutionChanges(10),
		BlobKzgCommitments:     getKZGCommitments(4),
		ExecutionRequests:      genExecutionRequests(),
	}
}

func genSignedBeaconBlockElectra() *v1alpha1.SignedBeaconBlockElectra {
	return &v1alpha1.SignedBeaconBlockElectra{
		Block:     genBeaconBlockElectra(),
		Signature: bytes(96),
	}
}

func genBeaconBlockElectra() *v1alpha1.BeaconBlockElectra {
	return &v1alpha1.BeaconBlockElectra{
		Slot:          123455,
		ProposerIndex: 55433,
		ParentRoot:    bytes(32),
		StateRoot:     bytes(32),
		Body:          genBeaconBlockBodyElectra(),
	}
}

func genBeaconBlockBodyElectra() *v1alpha1.BeaconBlockBodyElectra {
	return &v1alpha1.BeaconBlockBodyElectra{
		RandaoReveal:          bytes(96),
		Eth1Data:              genEth1Data(),
		Graffiti:              bytes(32),
		ProposerSlashings:     genProposerSlashings(5),
		AttesterSlashings:     genAttesterSlashingsElectra(5),
		Attestations:          genAttestationsElectra(10),
		Deposits:              genDeposits(5),
		VoluntaryExits:        genSignedVoluntaryExits(12),
		SyncAggregate:         genSyncAggregate(),
		ExecutionPayload:      genPayloadElectra(),
		BlsToExecutionChanges: genBLSToExecutionChanges(10),
		BlobKzgCommitments:    getKZGCommitments(4),
		ExecutionRequests:     genExecutionRequests(),
	}
}

func genExecutionRequests() *enginev1.ExecutionRequests {
	return &enginev1.ExecutionRequests{
		Deposits:       genDepositRequests(10),
		Withdrawals:    genWithdrawalRequests(10),
		Consolidations: genConsolidationRequests(10),
	}
}

func genDepositRequests(num int) []*enginev1.DepositRequest {
	drs := make([]*enginev1.DepositRequest, num)
	for i := 0; i < num; i++ {
		drs[i] = genDepositRequest()
	}
	return drs
}

func genDepositRequest() *enginev1.DepositRequest {
	return &enginev1.DepositRequest{
		Pubkey:                bytes(48),
		WithdrawalCredentials: bytes(32),
		Amount:                55555,
		Signature:             bytes(96),
		Index:                 123444,
	}
}

func genWithdrawalRequests(num int) []*enginev1.WithdrawalRequest {
	wrs := make([]*enginev1.WithdrawalRequest, num)
	for i := 0; i < num; i++ {
		wrs[i] = genWithdrawalRequest()
	}
	return wrs
}

func genWithdrawalRequest() *enginev1.WithdrawalRequest {
	return &enginev1.WithdrawalRequest{
		SourceAddress:   bytes(20),
		ValidatorPubkey: bytes(48),
		Amount:          55555,
	}
}

func genConsolidationRequests(num int) []*enginev1.ConsolidationRequest {
	crs := make([]*enginev1.ConsolidationRequest, num)
	for i := 0; i < num; i++ {
		crs[i] = genConsolidationRequest()
	}
	return crs
}

func genConsolidationRequest() *enginev1.ConsolidationRequest {
	return &enginev1.ConsolidationRequest{
		SourceAddress: bytes(20),
		SourcePubkey:  bytes(48),
		TargetPubkey:  bytes(48),
	}
}
