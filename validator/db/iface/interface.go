// Package iface defines an interface for the validator database.
package iface

import (
	"context"
	"io"

	"github.com/prometheus/client_golang/prometheus"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/proposer"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/monitoring/backup"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/validator/db/common"
)

// ValidatorDB defines the necessary methods for a Prysm validator DB.
type ValidatorDB interface {
	io.Closer
	backup.Exporter
	DatabasePath() string
	ClearDB() error
	RunUpMigrations(ctx context.Context) error
	RunDownMigrations(ctx context.Context) error
	UpdatePublicKeysBuckets(publicKeys [][fieldparams.BLSPubkeyLength]byte) error

	// Genesis information related methods.
	GenesisValidatorsRoot(ctx context.Context) ([]byte, error)
	SaveGenesisValidatorsRoot(ctx context.Context, genValRoot []byte) error

	// Proposer protection related methods.
	HighestSignedProposal(ctx context.Context, publicKey [fieldparams.BLSPubkeyLength]byte) (primitives.Slot, bool, error)
	LowestSignedProposal(ctx context.Context, publicKey [fieldparams.BLSPubkeyLength]byte) (primitives.Slot, bool, error)
	ProposalHistoryForPubKey(ctx context.Context, publicKey [fieldparams.BLSPubkeyLength]byte) ([]*common.Proposal, error)
	ProposalHistoryForSlot(ctx context.Context, publicKey [fieldparams.BLSPubkeyLength]byte, slot primitives.Slot) ([32]byte, bool, bool, error)
	SaveProposalHistoryForSlot(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, slot primitives.Slot, signingRoot []byte) error
	ProposedPublicKeys(ctx context.Context) ([][fieldparams.BLSPubkeyLength]byte, error)
	SlashableProposalCheck(
		ctx context.Context,
		pubKey [fieldparams.BLSPubkeyLength]byte,
		signedBlock interfaces.ReadOnlySignedBeaconBlock,
		signingRoot [fieldparams.RootLength]byte,
		emitAccountMetrics bool,
		validatorProposeFailVec *prometheus.CounterVec,
	) error

	// Attester protection related methods.
	// Methods to store and read blacklisted public keys from EIP-3076
	// slashing protection imports.
	EIPImportBlacklistedPublicKeys(ctx context.Context) ([][fieldparams.BLSPubkeyLength]byte, error)
	SaveEIPImportBlacklistedPublicKeys(ctx context.Context, publicKeys [][fieldparams.BLSPubkeyLength]byte) error
	SigningRootAtTargetEpoch(ctx context.Context, publicKey [fieldparams.BLSPubkeyLength]byte, target primitives.Epoch) ([]byte, error)
	LowestSignedTargetEpoch(ctx context.Context, publicKey [fieldparams.BLSPubkeyLength]byte) (primitives.Epoch, bool, error)
	LowestSignedSourceEpoch(ctx context.Context, publicKey [fieldparams.BLSPubkeyLength]byte) (primitives.Epoch, bool, error)
	AttestedPublicKeys(ctx context.Context) ([][fieldparams.BLSPubkeyLength]byte, error)
	SlashableAttestationCheck(
		ctx context.Context, indexedAtt ethpb.IndexedAtt, pubKey [fieldparams.BLSPubkeyLength]byte,
		signingRoot32 [32]byte,
		emitAccountMetrics bool,
		validatorAttestFailVec *prometheus.CounterVec,
	) error
	SaveAttestationForPubKey(
		ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, signingRoot [fieldparams.RootLength]byte, att ethpb.IndexedAtt,
	) error
	SaveAttestationsForPubKey(
		ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, signingRoots [][]byte, atts []*ethpb.IndexedAttestation,
	) error
	AttestationHistoryForPubKey(
		ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte,
	) ([]*common.AttestationRecord, error)

	// Graffiti ordered index related methods
	SaveGraffitiOrderedIndex(ctx context.Context, index uint64) error
	GraffitiOrderedIndex(ctx context.Context, fileHash [32]byte) (uint64, error)
	GraffitiFileHash() ([32]byte, bool, error)

	// ProposerSettings related methods
	ProposerSettings(context.Context) (*proposer.Settings, error)
	ProposerSettingsExists(ctx context.Context) (bool, error)
	SaveProposerSettings(ctx context.Context, settings *proposer.Settings) error

	// EIP-3076 slashing protection related methods
	ImportStandardProtectionJSON(ctx context.Context, r io.Reader) error
}
