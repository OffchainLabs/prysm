package blocks

import (
	"fmt"

	consensus_types "github.com/prysmaticlabs/prysm/v5/consensus-types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// SetSignature sets the signature of the signed beacon block.
// This function is not thread safe, it is only used during block creation.
func (b *SignedBeaconBlock) SetSignature(sig []byte) {
	copy(b.signature[:], sig)
}

// SetSlot sets the respective slot of the block.
// This function is not thread safe, it is only used during block creation.
func (b *SignedBeaconBlock) SetSlot(slot primitives.Slot) {
	b.block.slot = slot
}

// SetProposerIndex sets the proposer index of the beacon block.
// This function is not thread safe, it is only used during block creation.
func (b *SignedBeaconBlock) SetProposerIndex(proposerIndex primitives.ValidatorIndex) {
	b.block.proposerIndex = proposerIndex
}

// SetParentRoot sets the parent root of beacon block.
// This function is not thread safe, it is only used during block creation.
func (b *SignedBeaconBlock) SetParentRoot(parentRoot []byte) {
	copy(b.block.parentRoot[:], parentRoot)
}

// SetStateRoot sets the state root of the underlying beacon block
// This function is not thread safe, it is only used during block creation.
func (b *SignedBeaconBlock) SetStateRoot(root []byte) {
	copy(b.block.stateRoot[:], root)
}

// SetRandaoReveal sets the randao reveal in the block body.
// This function is not thread safe, it is only used during block creation.
func (b *SignedBeaconBlock) SetRandaoReveal(r []byte) {
	copy(b.block.body.randaoReveal[:], r)
}

// SetGraffiti sets the graffiti in the block.
// This function is not thread safe, it is only used during block creation.
func (b *SignedBeaconBlock) SetGraffiti(g []byte) {
	copy(b.block.body.graffiti[:], g)
}

// SetEth1Data sets the eth1 data in the block.
// This function is not thread safe, it is only used during block creation.
func (b *SignedBeaconBlock) SetEth1Data(e *eth.Eth1Data) {
	b.block.body.eth1Data = e
}

// SetProposerSlashings sets the proposer slashings in the block.
// This function is not thread safe, it is only used during block creation.
func (b *SignedBeaconBlock) SetProposerSlashings(p []*eth.ProposerSlashing) {
	b.block.body.proposerSlashings = p
}

// SetAttesterSlashings sets the attester slashings in the block.
// This function is not thread safe, it is only used during block creation.
func (b *SignedBeaconBlock) SetAttesterSlashings(slashings []eth.AttSlashing) error {
	if b.version < version.Electra {
		blockSlashings := make([]*eth.AttesterSlashing, 0, len(slashings))
		for _, slashing := range slashings {
			s, ok := slashing.(*eth.AttesterSlashing)
			if !ok {
				return fmt.Errorf("slashing of type %T is not *eth.AttesterSlashing", slashing)
			}
			blockSlashings = append(blockSlashings, s)
		}
		b.block.body.attesterSlashings = blockSlashings
	} else {
		blockSlashings := make([]*eth.AttesterSlashingElectra, 0, len(slashings))
		for _, slashing := range slashings {
			s, ok := slashing.(*eth.AttesterSlashingElectra)
			if !ok {
				return fmt.Errorf("slashing of type %T is not *eth.AttesterSlashingElectra", slashing)
			}
			blockSlashings = append(blockSlashings, s)
		}
		b.block.body.attesterSlashingsElectra = blockSlashings
	}
	return nil
}

// SetAttestations sets the attestations in the block.
// This function is not thread safe, it is only used during block creation.
func (b *SignedBeaconBlock) SetAttestations(atts []eth.Att) error {
	if b.version < version.Electra {
		blockAtts := make([]*eth.Attestation, 0, len(atts))
		for _, att := range atts {
			a, ok := att.(*eth.Attestation)
			if !ok {
				return fmt.Errorf("attestation of type %T is not *eth.Attestation", att)
			}
			blockAtts = append(blockAtts, a)
		}
		b.block.body.attestations = blockAtts
	} else {
		blockAtts := make([]*eth.AttestationElectra, 0, len(atts))
		for _, att := range atts {
			a, ok := att.(*eth.AttestationElectra)
			if !ok {
				return fmt.Errorf("attestation of type %T is not *eth.AttestationElectra", att)
			}
			blockAtts = append(blockAtts, a)
		}
		b.block.body.attestationsElectra = blockAtts
	}
	return nil
}

// SetDeposits sets the deposits in the block.
// This function is not thread safe, it is only used during block creation.
func (b *SignedBeaconBlock) SetDeposits(d []*eth.Deposit) {
	b.block.body.deposits = d
}

// SetVoluntaryExits sets the voluntary exits in the block.
// This function is not thread safe, it is only used during block creation.
func (b *SignedBeaconBlock) SetVoluntaryExits(v []*eth.SignedVoluntaryExit) {
	b.block.body.voluntaryExits = v
}

// SetSyncAggregate sets the sync aggregate in the block.
// This function is not thread safe, it is only used during block creation.
func (b *SignedBeaconBlock) SetSyncAggregate(s *eth.SyncAggregate) error {
	if b.version == version.Phase0 {
		return consensus_types.ErrNotSupported("SyncAggregate", b.version)
	}
	b.block.body.syncAggregate = s
	return nil
}

// SetExecution sets the execution payload of the block body.
// This function is not thread safe, it is only used during block creation.
func (b *SignedBeaconBlock) SetExecution(e interfaces.ExecutionData) error {
	if b.version >= version.EPBS || b.version < version.Bellatrix {
		return consensus_types.ErrNotSupported("Execution", b.version)
	}
	if e.IsBlinded() {
		b.block.body.executionPayloadHeader = e
		return nil
	}
	b.block.body.executionPayload = e
	return nil
}

// SetBLSToExecutionChanges sets the BLS to execution changes in the block.
// This function is not thread safe, it is only used during block creation.
func (b *SignedBeaconBlock) SetBLSToExecutionChanges(blsToExecutionChanges []*eth.SignedBLSToExecutionChange) error {
	if b.version < version.Capella {
		return consensus_types.ErrNotSupported("BLSToExecutionChanges", b.version)
	}
	b.block.body.blsToExecutionChanges = blsToExecutionChanges
	return nil
}

// SetBlobKzgCommitments sets the blob kzg commitments in the block.
func (b *SignedBeaconBlock) SetBlobKzgCommitments(c [][]byte) error {
	if b.version < version.Deneb {
		return consensus_types.ErrNotSupported("SetBlobKzgCommitments", b.version)
	}
	b.block.body.blobKzgCommitments = c
	return nil
}

// SetExecutionRequests sets the execution requests in the block.
func (b *SignedBeaconBlock) SetExecutionRequests(req *enginev1.ExecutionRequests) error {
	if b.version < version.Electra {
		return consensus_types.ErrNotSupported("SetExecutionRequests", b.version)
	}
	b.block.body.executionRequests = req
	return nil
}

// SetPayloadAttestations sets the payload attestations in the block.
func (b *SignedBeaconBlock) SetPayloadAttestations(p []*eth.PayloadAttestation) error {
	if b.version < version.EPBS {
		return consensus_types.ErrNotSupported("PayloadAttestations", b.version)
	}
	b.block.body.payloadAttestations = p
	return nil
}

// SetSignedExecutionPayloadHeader sets the signed execution payload header of the block body.
func (b *SignedBeaconBlock) SetSignedExecutionPayloadHeader(h *enginev1.SignedExecutionPayloadHeader) error {
	if b.version < version.EPBS {
		return consensus_types.ErrNotSupported("SetSignedExecutionPayloadHeader", b.version)
	}
	b.block.body.signedExecutionPayloadHeader = h
	return nil
}
