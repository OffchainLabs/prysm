package helpers

import (
	"context"
	"encoding/hex"
	"io"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/proposer"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/validator/db/common"
	"github.com/prysmaticlabs/prysm/v5/validator/db/iface"
	"github.com/prysmaticlabs/prysm/v5/validator/slashing-protection-history/format"
)

type ValidatorDBMock struct {
	genesisValidatorsRoot []byte
}

func NewValidatorDBMock() *ValidatorDBMock {
	return &ValidatorDBMock{}
}

var _ iface.ValidatorDB = (*ValidatorDBMock)(nil)

func (db *ValidatorDBMock) Backup(ctx context.Context, outputPath string, permissionOverride bool) error {
	panic("not implemented")
}

func (db *ValidatorDBMock) Close() error { panic("not implemented") }

func (db *ValidatorDBMock) DatabasePath() string                        { panic("not implemented") }
func (db *ValidatorDBMock) ClearDB() error                              { panic("not implemented") }
func (db *ValidatorDBMock) RunUpMigrations(ctx context.Context) error   { panic("not implemented") }
func (db *ValidatorDBMock) RunDownMigrations(ctx context.Context) error { panic("not implemented") }
func (db *ValidatorDBMock) UpdatePublicKeysBuckets(publicKeys [][fieldparams.BLSPubkeyLength]byte) error {
	panic("not implemented")
}

// Genesis information related methods.
func (db *ValidatorDBMock) GenesisValidatorsRoot(ctx context.Context) ([]byte, error) {
	return db.genesisValidatorsRoot, nil
}
func (db *ValidatorDBMock) SaveGenesisValidatorsRoot(ctx context.Context, genValRoot []byte) error {
	db.genesisValidatorsRoot = genValRoot
	return nil
}

// Proposer protection related methods.
func (db *ValidatorDBMock) HighestSignedProposal(ctx context.Context, publicKey [fieldparams.BLSPubkeyLength]byte) (primitives.Slot, bool, error) {
	panic("not implemented")
}
func (db *ValidatorDBMock) LowestSignedProposal(ctx context.Context, publicKey [fieldparams.BLSPubkeyLength]byte) (primitives.Slot, bool, error) {
	panic("not implemented")
}
func (db *ValidatorDBMock) ProposalHistoryForPubKey(ctx context.Context, publicKey [fieldparams.BLSPubkeyLength]byte) ([]*common.Proposal, error) {
	panic("not implemented")
}
func (db *ValidatorDBMock) ProposalHistoryForSlot(ctx context.Context, publicKey [fieldparams.BLSPubkeyLength]byte, slot primitives.Slot) ([32]byte, bool, bool, error) {
	panic("not implemented")
}
func (db *ValidatorDBMock) SaveProposalHistoryForSlot(ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, slot primitives.Slot, signingRoot []byte) error {
	panic("not implemented")
}
func (db *ValidatorDBMock) ProposedPublicKeys(ctx context.Context) ([][fieldparams.BLSPubkeyLength]byte, error) {
	panic("not implemented")
}

func (db *ValidatorDBMock) SlashableProposalCheck(
	ctx context.Context,
	pubKey [fieldparams.BLSPubkeyLength]byte,
	signedBlock interfaces.ReadOnlySignedBeaconBlock,
	signingRoot [fieldparams.RootLength]byte,
	emitAccountMetrics bool,
	validatorProposeFailVec *prometheus.CounterVec,
) error {
	panic("not implemented")
}

// Attester protection related methods.
// Methods to store and read blacklisted public keys from EIP-3076
// slashing protection imports.
func (db *ValidatorDBMock) EIPImportBlacklistedPublicKeys(ctx context.Context) ([][fieldparams.BLSPubkeyLength]byte, error) {
	panic("not implemented")
}
func (db *ValidatorDBMock) SaveEIPImportBlacklistedPublicKeys(ctx context.Context, publicKeys [][fieldparams.BLSPubkeyLength]byte) error {
	panic("not implemented")
}
func (db *ValidatorDBMock) SigningRootAtTargetEpoch(ctx context.Context, publicKey [fieldparams.BLSPubkeyLength]byte, target primitives.Epoch) ([]byte, error) {
	panic("not implemented")
}
func (db *ValidatorDBMock) LowestSignedTargetEpoch(ctx context.Context, publicKey [fieldparams.BLSPubkeyLength]byte) (primitives.Epoch, bool, error) {
	panic("not implemented")
}
func (db *ValidatorDBMock) LowestSignedSourceEpoch(ctx context.Context, publicKey [fieldparams.BLSPubkeyLength]byte) (primitives.Epoch, bool, error) {
	panic("not implemented")
}
func (db *ValidatorDBMock) AttestedPublicKeys(ctx context.Context) ([][fieldparams.BLSPubkeyLength]byte, error) {
	panic("not implemented")
}

func (db *ValidatorDBMock) SlashableAttestationCheck(
	ctx context.Context, indexedAtt ethpb.IndexedAtt, pubKey [fieldparams.BLSPubkeyLength]byte,
	signingRoot32 [32]byte,
	emitAccountMetrics bool,
	validatorAttestFailVec *prometheus.CounterVec,
) error {
	panic("not implemented")
}

func (db *ValidatorDBMock) SaveAttestationForPubKey(
	ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, signingRoot [fieldparams.RootLength]byte, att ethpb.IndexedAtt,
) error {
	panic("not implemented")
}

func (db *ValidatorDBMock) SaveAttestationsForPubKey(
	ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte, signingRoots [][]byte, atts []*ethpb.IndexedAttestation,
) error {
	panic("not implemented")
}

func (db *ValidatorDBMock) AttestationHistoryForPubKey(
	ctx context.Context, pubKey [fieldparams.BLSPubkeyLength]byte,
) ([]*common.AttestationRecord, error) {
	panic("not implemented")
}

// Graffiti ordered index related methods
func (db *ValidatorDBMock) SaveGraffitiOrderedIndex(ctx context.Context, index uint64) error {
	panic("not implemented")
}
func (db *ValidatorDBMock) GraffitiOrderedIndex(ctx context.Context, fileHash [32]byte) (uint64, error) {
	panic("not implemented")
}
func (db *ValidatorDBMock) GraffitiFileHash() ([32]byte, bool, error) { panic("not implemented") }

// ProposerSettings related methods
func (db *ValidatorDBMock) ProposerSettings(context.Context) (*proposer.Settings, error) {
	panic("not implemented")
}
func (db *ValidatorDBMock) ProposerSettingsExists(ctx context.Context) (bool, error) {
	panic("not implemented")
}
func (db *ValidatorDBMock) UpdateProposerSettingsDefault(context.Context, *proposer.Option) error {
	panic("not implemented")
}
func (db *ValidatorDBMock) UpdateProposerSettingsForPubkey(context.Context, [fieldparams.BLSPubkeyLength]byte, *proposer.Option) error {
	panic("not implemented")
}
func (db *ValidatorDBMock) SaveProposerSettings(ctx context.Context, settings *proposer.Settings) error {
	panic("not implemented")
}

// EIP-3076 slashing protection related methods
func (db *ValidatorDBMock) ImportStandardProtectionJSON(ctx context.Context, r io.Reader) error {
	panic("not implemented")
}

func Test_validateMetadata(t *testing.T) {
	goodRoot := [32]byte{1}
	goodStr := make([]byte, hex.EncodedLen(len(goodRoot)))
	hex.Encode(goodStr, goodRoot[:])
	tests := []struct {
		name                    string
		interchangeJSON         *format.EIPSlashingProtectionFormat
		dbGenesisValidatorsRoot []byte
		wantErr                 bool
		wantFatal               string
	}{
		{
			name: "Incorrect version for EIP format should fail",
			interchangeJSON: &format.EIPSlashingProtectionFormat{
				Metadata: struct {
					InterchangeFormatVersion string `json:"interchange_format_version"`
					GenesisValidatorsRoot    string `json:"genesis_validators_root"`
				}{
					InterchangeFormatVersion: "1",
					GenesisValidatorsRoot:    string(goodStr),
				},
			},
			wantErr: true,
		},
		{
			name: "Junk data for version should fail",
			interchangeJSON: &format.EIPSlashingProtectionFormat{
				Metadata: struct {
					InterchangeFormatVersion string `json:"interchange_format_version"`
					GenesisValidatorsRoot    string `json:"genesis_validators_root"`
				}{
					InterchangeFormatVersion: "asdljas$d",
					GenesisValidatorsRoot:    string(goodStr),
				},
			},
			wantErr: true,
		},
		{
			name: "Proper version field should pass",
			interchangeJSON: &format.EIPSlashingProtectionFormat{
				Metadata: struct {
					InterchangeFormatVersion string `json:"interchange_format_version"`
					GenesisValidatorsRoot    string `json:"genesis_validators_root"`
				}{
					InterchangeFormatVersion: format.InterchangeFormatVersion,
					GenesisValidatorsRoot:    string(goodStr),
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateMetadata(context.Background(), NewValidatorDBMock(), tt.interchangeJSON); (err != nil) != tt.wantErr {
				t.Errorf("validateMetadata() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_validateMetadataGenesisValidatorsRoot(t *testing.T) {
	goodRoot := [32]byte{1}
	goodStr := make([]byte, hex.EncodedLen(len(goodRoot)))
	hex.Encode(goodStr, goodRoot[:])
	secondRoot := [32]byte{2}
	secondStr := make([]byte, hex.EncodedLen(len(secondRoot)))
	hex.Encode(secondStr, secondRoot[:])

	tests := []struct {
		name                    string
		interchangeJSON         *format.EIPSlashingProtectionFormat
		dbGenesisValidatorsRoot []byte
		wantErr                 bool
	}{
		{
			name: "Same genesis roots should not fail",
			interchangeJSON: &format.EIPSlashingProtectionFormat{
				Metadata: struct {
					InterchangeFormatVersion string `json:"interchange_format_version"`
					GenesisValidatorsRoot    string `json:"genesis_validators_root"`
				}{
					InterchangeFormatVersion: format.InterchangeFormatVersion,
					GenesisValidatorsRoot:    string(goodStr),
				},
			},
			dbGenesisValidatorsRoot: goodRoot[:],
			wantErr:                 false,
		},
		{
			name: "Different genesis roots should not fail",
			interchangeJSON: &format.EIPSlashingProtectionFormat{
				Metadata: struct {
					InterchangeFormatVersion string `json:"interchange_format_version"`
					GenesisValidatorsRoot    string `json:"genesis_validators_root"`
				}{
					InterchangeFormatVersion: format.InterchangeFormatVersion,
					GenesisValidatorsRoot:    string(secondStr),
				},
			},
			dbGenesisValidatorsRoot: goodRoot[:],
			wantErr:                 true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			validatorDB := NewValidatorDBMock()
			require.NoError(t, validatorDB.SaveGenesisValidatorsRoot(ctx, tt.dbGenesisValidatorsRoot))
			err := ValidateMetadata(ctx, validatorDB, tt.interchangeJSON)
			if tt.wantErr {
				require.ErrorContains(t, "genesis validators root doesn't match the one that is stored", err)
			} else {
				require.NoError(t, err)
			}

		})
	}
}
