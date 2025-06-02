package common

import (
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
)

const FailedBlockSignLocalErr = "block rejected by local protection"

// Proposal representation for a validator public key.
type Proposal struct {
	Slot        primitives.Slot `json:"slot"`
	SigningRoot []byte          `json:"signing_root"`
}

// ProposalHistoryForPubkey for a validator public key.
type ProposalHistoryForPubkey struct {
	Proposals []Proposal
}

// AttestationRecord which can be represented by these simple values
// for manipulation by database methods.
type AttestationRecord struct {
	PubKey      [fieldparams.BLSPubkeyLength]byte
	Source      primitives.Epoch
	Target      primitives.Epoch
	SigningRoot []byte
}
