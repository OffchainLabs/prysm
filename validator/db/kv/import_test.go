package kv

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/db/common"
	"github.com/OffchainLabs/prysm/v7/validator/slashing-protection-history/format"
	valtest "github.com/OffchainLabs/prysm/v7/validator/testing"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func TestStore_ImportInterchangeData_BadJSON(t *testing.T) {
	ctx := t.Context()
	validatorDB := setupDB(t, nil)

	buf := bytes.NewBuffer([]byte("helloworld"))
	err := validatorDB.ImportStandardProtectionJSON(ctx, buf)
	require.ErrorContains(t, "could not unmarshal slashing protection JSON file", err)
}

func TestStore_ImportInterchangeData_NilData_FailsSilently(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := t.Context()
	validatorDB := setupDB(t, nil)

	interchangeJSON := &format.EIPSlashingProtectionFormat{}
	encoded, err := json.Marshal(interchangeJSON)
	require.NoError(t, err)

	buf := bytes.NewBuffer(encoded)
	err = validatorDB.ImportStandardProtectionJSON(ctx, buf)
	require.NoError(t, err)
	require.LogsContain(t, hook, "No slashing protection data to import")
}

func TestStore_ImportInterchangeData_BadFormat_PreventsDBWrites(t *testing.T) {
	ctx := t.Context()
	numValidators := 10
	publicKeys, err := valtest.CreateRandomPubKeys(numValidators)
	require.NoError(t, err)
	validatorDB := setupDB(t, publicKeys)

	// First we setup some mock attesting and proposal histories and create a mock
	// standard slashing protection format JSON struct.
	attestingHistory, proposalHistory := valtest.MockAttestingAndProposalHistories(publicKeys)
	standardProtectionFormat, err := valtest.MockSlashingProtectionJSON(publicKeys, attestingHistory, proposalHistory)
	require.NoError(t, err)

	// We replace a slot of one of the blocks with junk data.
	standardProtectionFormat.Data[0].SignedBlocks[0].Slot = "BadSlot"

	// We encode the standard slashing protection struct into a JSON format.
	blob, err := json.Marshal(standardProtectionFormat)
	require.NoError(t, err)
	buf := bytes.NewBuffer(blob)

	// Next, we attempt to import it into our validator database and check that
	// we obtain an error during the import process.
	err = validatorDB.ImportStandardProtectionJSON(ctx, buf)
	assert.NotNil(t, err)

	// Next, we attempt to retrieve the attesting and proposals histories from our database and
	// verify nothing was saved to the DB. If there is an error in the import process, we need to make
	// sure writing is an atomic operation: either the import succeeds and saves the slashing protection
	// data to our DB, or it does not.
	for i := range publicKeys {
		for _, att := range attestingHistory[i] {
			indexedAtt := &ethpb.IndexedAttestation{
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{
						Epoch: att.Source,
					},
					Target: &ethpb.Checkpoint{
						Epoch: att.Target,
					},
				},
			}

			slashingKind, err := validatorDB.CheckSlashableAttestation(ctx, publicKeys[i], []byte{}, indexedAtt)
			// We expect we do not have an attesting history for each attestation
			require.NoError(t, err)
			require.Equal(t, NotSlashable, slashingKind)
		}

		receivedHistory, err := validatorDB.ProposalHistoryForPubKey(ctx, publicKeys[i])
		require.NoError(t, err)
		require.DeepEqual(
			t,
			make([]*common.Proposal, 0),
			receivedHistory,
			"Imported proposal signing root is different than the empty default",
		)
	}
}

func TestStore_ImportInterchangeData_OK(t *testing.T) {
	ctx := t.Context()
	numValidators := 10
	publicKeys, err := valtest.CreateRandomPubKeys(numValidators)
	require.NoError(t, err)
	validatorDB := setupDB(t, publicKeys)

	// First we setup some mock attesting and proposal histories and create a mock
	// standard slashing protection format JSON struct.
	attestingHistory, proposalHistory := valtest.MockAttestingAndProposalHistories(publicKeys)
	standardProtectionFormat, err := valtest.MockSlashingProtectionJSON(publicKeys, attestingHistory, proposalHistory)
	require.NoError(t, err)

	// We encode the standard slashing protection struct into a JSON format.
	blob, err := json.Marshal(standardProtectionFormat)
	require.NoError(t, err)
	buf := bytes.NewBuffer(blob)

	// Next, we attempt to import it into our validator database.
	err = validatorDB.ImportStandardProtectionJSON(ctx, buf)
	require.NoError(t, err)

	// Next, we attempt to retrieve the attesting and proposals histories from our database and
	// verify those indeed match the originally generated mock histories.
	for i := range publicKeys {
		for _, att := range attestingHistory[i] {
			indexedAtt := &ethpb.IndexedAttestation{
				Data: &ethpb.AttestationData{
					Source: &ethpb.Checkpoint{
						Epoch: att.Source,
					},
					Target: &ethpb.Checkpoint{
						Epoch: att.Target,
					},
				},
			}
			// We expect we have an attesting history for the attestation and when
			// attempting to verify the same att is slashable with a different signing root,
			// we expect to receive a double vote slashing kind.
			slashingKind, err := validatorDB.CheckSlashableAttestation(ctx, publicKeys[i], []byte{}, indexedAtt)
			require.NotNil(t, err)
			require.Equal(t, DoubleVote, slashingKind)
		}

		proposals := proposalHistory[i].Proposals

		receivedProposalHistory, err := validatorDB.ProposalHistoryForPubKey(ctx, publicKeys[i])
		require.NoError(t, err)
		rootsBySlot := make(map[primitives.Slot][]byte)
		for _, proposal := range receivedProposalHistory {
			rootsBySlot[proposal.Slot] = proposal.SigningRoot
		}
		for _, proposal := range proposals {
			receivedRoot, ok := rootsBySlot[proposal.Slot]
			require.DeepEqual(t, true, ok)
			require.DeepEqual(
				t,
				receivedRoot,
				proposal.SigningRoot,
				"Imported proposals are different then the generated ones",
			)
		}
	}
}

func Test_parseUniqueSignedBlocksByPubKey(t *testing.T) {
	numValidators := 4
	publicKeys, err := valtest.CreateRandomPubKeys(numValidators)
	require.NoError(t, err)
	roots := valtest.CreateMockRoots(numValidators)
	tests := []struct {
		name    string
		data    []*format.ProtectionData
		want    map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedBlock
		wantErr bool
	}{
		{
			name: "nil values are skipped",
			data: []*format.ProtectionData{
				{
					Pubkey: fmt.Sprintf("%x", publicKeys[0]),
					SignedBlocks: []*format.SignedBlock{
						{
							Slot:        "1",
							SigningRoot: fmt.Sprintf("%x", roots[0]),
						},
						nil,
					},
				},
				{
					Pubkey: fmt.Sprintf("%x", publicKeys[0]),
					SignedBlocks: []*format.SignedBlock{
						{
							Slot:        "3",
							SigningRoot: fmt.Sprintf("%x", roots[2]),
						},
					},
				},
			},
			want: map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedBlock{
				publicKeys[0]: {
					{
						Slot:        "1",
						SigningRoot: fmt.Sprintf("%x", roots[0]),
					},
					{
						Slot:        "3",
						SigningRoot: fmt.Sprintf("%x", roots[2]),
					},
				},
			},
		},
		{
			name: "same blocks but different public keys are parsed correctly",
			data: []*format.ProtectionData{
				{
					Pubkey: fmt.Sprintf("%x", publicKeys[0]),
					SignedBlocks: []*format.SignedBlock{
						{
							Slot:        "1",
							SigningRoot: fmt.Sprintf("%x", roots[0]),
						},
						{
							Slot:        "2",
							SigningRoot: fmt.Sprintf("%x", roots[1]),
						},
					},
				},
				{
					Pubkey: fmt.Sprintf("%x", publicKeys[1]),
					SignedBlocks: []*format.SignedBlock{
						{
							Slot:        "1",
							SigningRoot: fmt.Sprintf("%x", roots[0]),
						},
						{
							Slot:        "2",
							SigningRoot: fmt.Sprintf("%x", roots[1]),
						},
					},
				},
			},
			want: map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedBlock{
				publicKeys[0]: {
					{
						Slot:        "1",
						SigningRoot: fmt.Sprintf("%x", roots[0]),
					},
					{
						Slot:        "2",
						SigningRoot: fmt.Sprintf("%x", roots[1]),
					},
				},
				publicKeys[1]: {
					{
						Slot:        "1",
						SigningRoot: fmt.Sprintf("%x", roots[0]),
					},
					{
						Slot:        "2",
						SigningRoot: fmt.Sprintf("%x", roots[1]),
					},
				},
			},
		},
		{
			name: "disjoint sets of signed blocks by the same public key are parsed correctly",
			data: []*format.ProtectionData{
				{
					Pubkey: fmt.Sprintf("%x", publicKeys[0]),
					SignedBlocks: []*format.SignedBlock{
						{
							Slot:        "1",
							SigningRoot: fmt.Sprintf("%x", roots[0]),
						},
						{
							Slot:        "2",
							SigningRoot: fmt.Sprintf("%x", roots[1]),
						},
					},
				},
				{
					Pubkey: fmt.Sprintf("%x", publicKeys[0]),
					SignedBlocks: []*format.SignedBlock{
						{
							Slot:        "3",
							SigningRoot: fmt.Sprintf("%x", roots[2]),
						},
					},
				},
			},
			want: map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedBlock{
				publicKeys[0]: {
					{
						Slot:        "1",
						SigningRoot: fmt.Sprintf("%x", roots[0]),
					},
					{
						Slot:        "2",
						SigningRoot: fmt.Sprintf("%x", roots[1]),
					},
					{
						Slot:        "3",
						SigningRoot: fmt.Sprintf("%x", roots[2]),
					},
				},
			},
		},
		{
			name: "full duplicate entries are uniquely parsed",
			data: []*format.ProtectionData{
				{
					Pubkey: fmt.Sprintf("%x", publicKeys[0]),
					SignedBlocks: []*format.SignedBlock{
						{
							Slot:        "1",
							SigningRoot: fmt.Sprintf("%x", roots[0]),
						},
					},
				},
				{
					Pubkey: fmt.Sprintf("%x", publicKeys[0]),
					SignedBlocks: []*format.SignedBlock{
						{
							Slot:        "1",
							SigningRoot: fmt.Sprintf("%x", roots[0]),
						},
					},
				},
			},
			want: map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedBlock{
				publicKeys[0]: {
					{
						Slot:        "1",
						SigningRoot: fmt.Sprintf("%x", roots[0]),
					},
					{
						Slot:        "1",
						SigningRoot: fmt.Sprintf("%x", roots[0]),
					},
				},
			},
		},
		{
			name: "intersecting duplicate public key entries are handled properly",
			data: []*format.ProtectionData{
				{
					Pubkey: fmt.Sprintf("%x", publicKeys[0]),
					SignedBlocks: []*format.SignedBlock{
						{
							Slot:        "1",
							SigningRoot: fmt.Sprintf("%x", roots[0]),
						},
						{
							Slot:        "2",
							SigningRoot: fmt.Sprintf("%x", roots[1]),
						},
					},
				},
				{
					Pubkey: fmt.Sprintf("%x", publicKeys[0]),
					SignedBlocks: []*format.SignedBlock{
						{
							Slot:        "2",
							SigningRoot: fmt.Sprintf("%x", roots[1]),
						},
						{
							Slot:        "3",
							SigningRoot: fmt.Sprintf("%x", roots[2]),
						},
					},
				},
			},
			want: map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedBlock{
				publicKeys[0]: {
					{
						Slot:        "1",
						SigningRoot: fmt.Sprintf("%x", roots[0]),
					},
					{
						Slot:        "2",
						SigningRoot: fmt.Sprintf("%x", roots[1]),
					},
					{
						Slot:        "2",
						SigningRoot: fmt.Sprintf("%x", roots[1]),
					},
					{
						Slot:        "3",
						SigningRoot: fmt.Sprintf("%x", roots[2]),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBlocksForUniquePublicKeys(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseBlocksForUniquePublicKeys() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseBlocksForUniquePublicKeys() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_parseUniqueSignedAttestationsByPubKey(t *testing.T) {
	numValidators := 4
	publicKeys, err := valtest.CreateRandomPubKeys(numValidators)
	require.NoError(t, err)
	roots := valtest.CreateMockRoots(numValidators)
	tests := []struct {
		name    string
		data    []*format.ProtectionData
		want    map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedAttestation
		wantErr bool
	}{
		{
			name: "nil values are skipped",
			data: []*format.ProtectionData{
				{
					Pubkey: fmt.Sprintf("%x", publicKeys[0]),
					SignedAttestations: []*format.SignedAttestation{
						{
							SourceEpoch: "1",
							TargetEpoch: "3",
							SigningRoot: fmt.Sprintf("%x", roots[0]),
						},
						nil,
					},
				},
				{
					Pubkey: fmt.Sprintf("%x", publicKeys[0]),
					SignedAttestations: []*format.SignedAttestation{
						{
							SourceEpoch: "3",
							TargetEpoch: "5",
							SigningRoot: fmt.Sprintf("%x", roots[2]),
						},
					},
				},
			},
			want: map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedAttestation{
				publicKeys[0]: {
					{
						SourceEpoch: "1",
						TargetEpoch: "3",
						SigningRoot: fmt.Sprintf("%x", roots[0]),
					},
					{
						SourceEpoch: "3",
						TargetEpoch: "5",
						SigningRoot: fmt.Sprintf("%x", roots[2]),
					},
				},
			},
		},
		{
			name: "same attestations but different public keys are parsed correctly",
			data: []*format.ProtectionData{
				{
					Pubkey: fmt.Sprintf("%x", publicKeys[0]),
					SignedAttestations: []*format.SignedAttestation{
						{
							SourceEpoch: "1",
							SigningRoot: fmt.Sprintf("%x", roots[0]),
						},
						{
							SourceEpoch: "2",
							SigningRoot: fmt.Sprintf("%x", roots[1]),
						},
					},
				},
				{
					Pubkey: fmt.Sprintf("%x", publicKeys[1]),
					SignedAttestations: []*format.SignedAttestation{
						{
							SourceEpoch: "1",
							SigningRoot: fmt.Sprintf("%x", roots[0]),
						},
						{
							SourceEpoch: "2",
							SigningRoot: fmt.Sprintf("%x", roots[1]),
						},
					},
				},
			},
			want: map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedAttestation{
				publicKeys[0]: {
					{
						SourceEpoch: "1",
						SigningRoot: fmt.Sprintf("%x", roots[0]),
					},
					{
						SourceEpoch: "2",
						SigningRoot: fmt.Sprintf("%x", roots[1]),
					},
				},
				publicKeys[1]: {
					{
						SourceEpoch: "1",
						SigningRoot: fmt.Sprintf("%x", roots[0]),
					},
					{
						SourceEpoch: "2",
						SigningRoot: fmt.Sprintf("%x", roots[1]),
					},
				},
			},
		},
		{
			name: "disjoint sets of signed attestations by the same public key are parsed correctly",
			data: []*format.ProtectionData{
				{
					Pubkey: fmt.Sprintf("%x", publicKeys[0]),
					SignedAttestations: []*format.SignedAttestation{
						{
							SourceEpoch: "1",
							TargetEpoch: "3",
							SigningRoot: fmt.Sprintf("%x", roots[0]),
						},
						{
							SourceEpoch: "2",
							TargetEpoch: "4",
							SigningRoot: fmt.Sprintf("%x", roots[1]),
						},
					},
				},
				{
					Pubkey: fmt.Sprintf("%x", publicKeys[0]),
					SignedAttestations: []*format.SignedAttestation{
						{
							SourceEpoch: "3",
							TargetEpoch: "5",
							SigningRoot: fmt.Sprintf("%x", roots[2]),
						},
					},
				},
			},
			want: map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedAttestation{
				publicKeys[0]: {
					{
						SourceEpoch: "1",
						TargetEpoch: "3",
						SigningRoot: fmt.Sprintf("%x", roots[0]),
					},
					{
						SourceEpoch: "2",
						TargetEpoch: "4",
						SigningRoot: fmt.Sprintf("%x", roots[1]),
					},
					{
						SourceEpoch: "3",
						TargetEpoch: "5",
						SigningRoot: fmt.Sprintf("%x", roots[2]),
					},
				},
			},
		},
		{
			name: "full duplicate entries are uniquely parsed",
			data: []*format.ProtectionData{
				{
					Pubkey: fmt.Sprintf("%x", publicKeys[0]),
					SignedAttestations: []*format.SignedAttestation{
						{
							SourceEpoch: "1",
							SigningRoot: fmt.Sprintf("%x", roots[0]),
						},
					},
				},
				{
					Pubkey: fmt.Sprintf("%x", publicKeys[0]),
					SignedAttestations: []*format.SignedAttestation{
						{
							SourceEpoch: "1",
							SigningRoot: fmt.Sprintf("%x", roots[0]),
						},
					},
				},
			},
			want: map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedAttestation{
				publicKeys[0]: {
					{
						SourceEpoch: "1",
						SigningRoot: fmt.Sprintf("%x", roots[0]),
					},
					{
						SourceEpoch: "1",
						SigningRoot: fmt.Sprintf("%x", roots[0]),
					},
				},
			},
		},
		{
			name: "intersecting duplicate public key entries are handled properly",
			data: []*format.ProtectionData{
				{
					Pubkey: fmt.Sprintf("%x", publicKeys[0]),
					SignedAttestations: []*format.SignedAttestation{
						{
							SourceEpoch: "1",
							SigningRoot: fmt.Sprintf("%x", roots[0]),
						},
						{
							SourceEpoch: "2",
							SigningRoot: fmt.Sprintf("%x", roots[1]),
						},
					},
				},
				{
					Pubkey: fmt.Sprintf("%x", publicKeys[0]),
					SignedAttestations: []*format.SignedAttestation{
						{
							SourceEpoch: "2",
							SigningRoot: fmt.Sprintf("%x", roots[1]),
						},
						{
							SourceEpoch: "3",
							SigningRoot: fmt.Sprintf("%x", roots[2]),
						},
					},
				},
			},
			want: map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedAttestation{
				publicKeys[0]: {
					{
						SourceEpoch: "1",
						SigningRoot: fmt.Sprintf("%x", roots[0]),
					},
					{
						SourceEpoch: "2",
						SigningRoot: fmt.Sprintf("%x", roots[1]),
					},
					{
						SourceEpoch: "2",
						SigningRoot: fmt.Sprintf("%x", roots[1]),
					},
					{
						SourceEpoch: "3",
						SigningRoot: fmt.Sprintf("%x", roots[2]),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAttestationsForUniquePublicKeys(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseAttestationsForUniquePublicKeys() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseAttestationsForUniquePublicKeys() got = %v, want %v", got, tt.want)
			}
		})
	}
}

// requireSlashablePubKeys asserts that got holds exactly the public keys marked in want.
func requireSlashablePubKeys(t *testing.T, want map[[fieldparams.BLSPubkeyLength]byte]bool, got [][fieldparams.BLSPubkeyLength]byte) {
	t.Helper()

	gotByPubKey := make(map[[fieldparams.BLSPubkeyLength]byte]bool, len(got))
	for _, pubKey := range got {
		gotByPubKey[pubKey] = true
	}

	for pubKey := range want {
		require.Equal(t, true, gotByPubKey[pubKey], fmt.Sprintf("public key %#x should be slashable", pubKey))
	}

	for pubKey := range gotByPubKey {
		require.Equal(t, true, want[pubKey], fmt.Sprintf("public key %#x should not be slashable", pubKey))
	}
}

func Test_filterSlashablePubKeysFromBlocks(t *testing.T) {
	var tests = []struct {
		name     string
		expected [][fieldparams.BLSPubkeyLength]byte
		given    map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedBlock
	}{
		{
			name:     "No slashable keys returns empty",
			expected: make([][fieldparams.BLSPubkeyLength]byte, 0),
			given: map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedBlock{
				{1}: {
					{
						Slot: "1",
					},
					{
						Slot: "2",
					},
				},
				{2}: {
					{
						Slot: "2",
					},
					{
						Slot: "3",
					},
				},
			},
		},
		{
			name:     "Empty data returns empty",
			expected: make([][fieldparams.BLSPubkeyLength]byte, 0),
			given:    make(map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedBlock),
		},
		{
			name: "Properly finds public keys with slashable data",
			expected: [][fieldparams.BLSPubkeyLength]byte{
				{1}, {3},
			},
			given: map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedBlock{
				// Two different blocks proposed at the same slot.
				{1}: {
					{
						Slot:        "1",
						SigningRoot: fmt.Sprintf("%#x", [32]byte{1}),
					},
					{
						Slot:        "1",
						SigningRoot: fmt.Sprintf("%#x", [32]byte{2}),
					},
					{
						Slot: "2",
					},
				},
				// Two blocks proposed at different slots.
				{2}: {
					{
						Slot: "2",
					},
					{
						Slot: "3",
					},
				},
				// Two different blocks proposed at the same slot.
				{3}: {
					{
						Slot:        "3",
						SigningRoot: fmt.Sprintf("%#x", [32]byte{3}),
					},
					{
						Slot:        "3",
						SigningRoot: fmt.Sprintf("%#x", [32]byte{4}),
					},
				},
			},
		},
		{
			name: "Considers optional signing roots when determining slashable keys",
			expected: [][fieldparams.BLSPubkeyLength]byte{
				{3},
			},
			given: map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedBlock{
				// Same slot and same signing root: the same block listed twice, not slashable.
				{1}: {
					{
						Slot:        "1",
						SigningRoot: fmt.Sprintf("%#x", [32]byte{1}),
					},
					{
						Slot:        "1",
						SigningRoot: fmt.Sprintf("%#x", [32]byte{1}),
					},
				},
				// Same slot and no signing root at all: the same block listed twice, not slashable.
				{2}: {
					{
						Slot: "2",
					},
					{
						Slot: "2",
					},
				},
				// Same slot, but only one entry has a signing root: slashable.
				{3}: {
					{
						Slot: "3",
					},
					{
						Slot:        "3",
						SigningRoot: fmt.Sprintf("%#x", [32]byte{3}),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			historyByPubKey := make(map[[fieldparams.BLSPubkeyLength]byte]common.ProposalHistoryForPubkey)
			for pubKey, signedBlocks := range tt.given {
				proposalHistory, err := transformSignedBlocks(ctx, signedBlocks)
				require.NoError(t, err)
				historyByPubKey[pubKey] = *proposalHistory
			}
			slashablePubKeys := filterSlashablePubKeysFromBlocks(t.Context(), historyByPubKey)
			wantedPubKeys := make(map[[fieldparams.BLSPubkeyLength]byte]bool, len(tt.expected))
			for _, pk := range tt.expected {
				wantedPubKeys[pk] = true
			}
			requireSlashablePubKeys(t, wantedPubKeys, slashablePubKeys)
		})
	}
}

func Test_filterSlashablePubKeysFromAttestations(t *testing.T) {
	// filterSlashablePubKeysFromAttestations is used only for complete slashing protection.
	ctx := t.Context()

	const (
		firstRoot  = "0x4ff6f743a43f3b4f95350831aeaf0a122a1a392922c45db804ae16b4b6f2e850"
		secondRoot = "0x6a3b04f5a5d47b1e7fd6b0c29cd0e8b9a4f1c2d3e4f50617283940a1b2c3d4e5"
	)

	tests := []struct {
		name                 string
		previousAttsByPubKey map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedAttestation
		incomingAttsByPubKey map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedAttestation
		want                 map[[fieldparams.BLSPubkeyLength]byte]bool
		wantErr              bool
	}{
		{
			name: "Properly filters out double voting attester keys",
			incomingAttsByPubKey: map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedAttestation{
				// Same target epoch, different source epochs: two different attestations.
				{1}: {
					{SourceEpoch: "2", TargetEpoch: "4"},
					{SourceEpoch: "3", TargetEpoch: "4"},
				},
				{2}: {
					{SourceEpoch: "2", TargetEpoch: "4"},
					{SourceEpoch: "2", TargetEpoch: "5"},
				},
				// Same attestation, different signing roots: two different attestations.
				{3}: {
					{SourceEpoch: "2", TargetEpoch: "4", SigningRoot: firstRoot},
					{SourceEpoch: "2", TargetEpoch: "4", SigningRoot: secondRoot},
				},
			},
			want: map[[fieldparams.BLSPubkeyLength]byte]bool{
				{1}: true,
				{3}: true,
			},
		},
		{
			name: "Returns empty if no keys are slashable",
			incomingAttsByPubKey: map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedAttestation{
				{1}: {
					{SourceEpoch: "2", TargetEpoch: "4"},
				},
				{2}: {
					{SourceEpoch: "2", TargetEpoch: "4"},
					{SourceEpoch: "2", TargetEpoch: "5"},
				},
				{3}: {
					{SourceEpoch: "2", TargetEpoch: "4"},
					{SourceEpoch: "3", TargetEpoch: "6"},
				},
			},
			want: map[[fieldparams.BLSPubkeyLength]byte]bool{},
		},
		{
			name: "Returns empty if the same attestation is repeated",
			incomingAttsByPubKey: map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedAttestation{
				// Signing roots are optional, so both repetitions describe the same attestation.
				{1}: {
					{SourceEpoch: "2", TargetEpoch: "4"},
					{SourceEpoch: "2", TargetEpoch: "4"},
				},
				{2}: {
					{SourceEpoch: "2", TargetEpoch: "4", SigningRoot: firstRoot},
					{SourceEpoch: "2", TargetEpoch: "4", SigningRoot: firstRoot},
				},
			},
			want: map[[fieldparams.BLSPubkeyLength]byte]bool{},
		},
		{
			name: "Properly filters out surround voting attester keys",
			incomingAttsByPubKey: map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedAttestation{
				{1}: {
					{SourceEpoch: "2", TargetEpoch: "4"},
					{SourceEpoch: "1", TargetEpoch: "5"},
				},
				{2}: {
					{SourceEpoch: "2", TargetEpoch: "4"},
					{SourceEpoch: "2", TargetEpoch: "5"},
				},
				{3}: {
					{SourceEpoch: "2", TargetEpoch: "5"},
					{SourceEpoch: "3", TargetEpoch: "4"},
				},
			},
			want: map[[fieldparams.BLSPubkeyLength]byte]bool{
				{1}: true,
				{3}: true,
			},
		},
		{
			name: "Properly filters out keys slashable with respect to the database",
			previousAttsByPubKey: map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedAttestation{
				{1}: {
					{SourceEpoch: "2", TargetEpoch: "4", SigningRoot: firstRoot},
				},
				{2}: {
					{SourceEpoch: "2", TargetEpoch: "4", SigningRoot: firstRoot},
				},
			},
			incomingAttsByPubKey: map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedAttestation{
				// Same attestation as the one in the database, but with another signing root.
				{1}: {
					{SourceEpoch: "2", TargetEpoch: "4", SigningRoot: secondRoot},
				},
				// Exactly the same attestation as the one in the database.
				{2}: {
					{SourceEpoch: "2", TargetEpoch: "4", SigningRoot: firstRoot},
				},
			},
			want: map[[fieldparams.BLSPubkeyLength]byte]bool{
				{1}: true,
			},
		},
		{
			name: "Considers optional signing roots with respect to the database",
			previousAttsByPubKey: map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedAttestation{
				{1}: {
					{SourceEpoch: "2", TargetEpoch: "4"},
				},
				{2}: {
					{SourceEpoch: "2", TargetEpoch: "4"},
				},
				{3}: {
					{SourceEpoch: "2", TargetEpoch: "4"},
				},
				{4}: {
					{SourceEpoch: "2", TargetEpoch: "4", SigningRoot: firstRoot},
				},
				{5}: {
					{SourceEpoch: "1", TargetEpoch: "10"},
					{SourceEpoch: "2", TargetEpoch: "4"},
				},
			},
			incomingAttsByPubKey: map[[fieldparams.BLSPubkeyLength]byte][]*format.SignedAttestation{
				// Same attestation as the one in the database, both without signing root.
				{1}: {
					{SourceEpoch: "2", TargetEpoch: "4"},
				},
				// Same target epoch as the one in the database, but another source epoch.
				{2}: {
					{SourceEpoch: "3", TargetEpoch: "4"},
				},
				// Same attestation as the one in the database, with a signing root on only one of them.
				{3}: {
					{SourceEpoch: "2", TargetEpoch: "4", SigningRoot: firstRoot},
				},
				{4}: {
					{SourceEpoch: "2", TargetEpoch: "4"},
				},
				// Same attestation as one in the database, but surrounded by another one in the database.
				{5}: {
					{SourceEpoch: "2", TargetEpoch: "4"},
				},
			},
			want: map[[fieldparams.BLSPubkeyLength]byte]bool{
				{2}: true,
				{3}: true,
				{4}: true,
				{5}: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pubKeys := make([][fieldparams.BLSPubkeyLength]byte, 0, len(tt.incomingAttsByPubKey))
			for pubKey := range tt.incomingAttsByPubKey {
				pubKeys = append(pubKeys, pubKey)
			}

			validatorDB := setupDB(t, pubKeys)

			// Save into the database the attestations which are already known by the validator.
			for pubKey, signedAtts := range tt.previousAttsByPubKey {
				attestingHistory, err := transformSignedAttestations(pubKey, signedAtts)
				require.NoError(t, err)

				for _, att := range attestingHistory {
					indexedAtt := createAttestation(att.Source, att.Target)
					err := validatorDB.SaveAttestationForPubKey(ctx, pubKey, att.SigningRoot, indexedAtt)
					require.NoError(t, err)
				}
			}

			// Build the attesting histories of the imported JSON file.
			attestingHistoriesByPubKey := make(map[[fieldparams.BLSPubkeyLength]byte][]*common.AttestationRecord, len(tt.incomingAttsByPubKey))
			for pubKey, signedAtts := range tt.incomingAttsByPubKey {
				attestingHistory, err := transformSignedAttestations(pubKey, signedAtts)
				require.NoError(t, err)

				attestingHistoriesByPubKey[pubKey] = attestingHistory
			}

			got, err := filterSlashablePubKeysFromAttestations(ctx, validatorDB, attestingHistoriesByPubKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("filterSlashablePubKeysFromAttestations() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			requireSlashablePubKeys(t, tt.want, got)
		})
	}
}

func TestStore_ImportInterchangeData_ImportTwice(t *testing.T) {
	ctx := t.Context()

	const signingRoot = "0x4ff6f743a43f3b4f95350831aeaf0a122a1a392922c45db804ae16b4b6f2e850"

	tests := []struct {
		name        string
		signingRoot string
	}{
		{
			name:        "attestation with signing root",
			signingRoot: signingRoot,
		},
		{
			name:        "attestation without signing root",
			signingRoot: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			publicKeys, err := valtest.CreateRandomPubKeys(1)
			require.NoError(t, err, "could not create public key")
			validatorDB := setupDB(t, publicKeys)

			interchangeJSON := &format.EIPSlashingProtectionFormat{}
			interchangeJSON.Metadata.InterchangeFormatVersion = format.InterchangeFormatVersion
			interchangeJSON.Metadata.GenesisValidatorsRoot = fmt.Sprintf("%#x", bytesutil.PadTo([]byte{32}, 32))
			interchangeJSON.Data = []*format.ProtectionData{
				{
					Pubkey: fmt.Sprintf("%#x", publicKeys[0]),
					SignedAttestations: []*format.SignedAttestation{
						{SourceEpoch: "2290", TargetEpoch: "3007", SigningRoot: tt.signingRoot},
					},
				},
			}

			blob, err := json.Marshal(interchangeJSON)
			require.NoError(t, err, "could not marshal interchange JSON")

			// Importing the same file twice should not blacklist the public key.
			for range 2 {
				err = validatorDB.ImportStandardProtectionJSON(ctx, bytes.NewBuffer(blob))
				require.NoError(t, err, "could not import interchange JSON")

				blacklistedPublicKeys, err := validatorDB.EIPImportBlacklistedPublicKeys(ctx)
				require.NoError(t, err, "could not get blacklisted public keys")
				require.Equal(t, 0, len(blacklistedPublicKeys), "unexpected blacklisting of the public key")
			}
		})
	}
}

func TestStore_ImportInterchangeData_OptionalSigningRoots(t *testing.T) {
	ctx := t.Context()

	const (
		firstRoot  = "0x4ff6f743a43f3b4f95350831aeaf0a122a1a392922c45db804ae16b4b6f2e850"
		secondRoot = "0x6a3b04f5a5d47b1e7fd6b0c29cd0e8b9a4f1c2d3e4f50617283940a1b2c3d4e5"
	)

	signedAttestations := func(firstSigningRoot, secondSigningRoot string) []*format.SignedAttestation {
		return []*format.SignedAttestation{
			{SourceEpoch: "2290", TargetEpoch: "3007", SigningRoot: firstSigningRoot},
			{SourceEpoch: "2290", TargetEpoch: "3007", SigningRoot: secondSigningRoot},
		}
	}

	signedBlocks := func(firstSigningRoot, secondSigningRoot string) []*format.SignedBlock {
		return []*format.SignedBlock{
			{Slot: "81952", SigningRoot: firstSigningRoot},
			{Slot: "81952", SigningRoot: secondSigningRoot},
		}
	}

	tests := []struct {
		name               string
		signedAttestations []*format.SignedAttestation
		signedBlocks       []*format.SignedBlock
		wantBlacklisted    bool
	}{
		{
			name:               "attestations with the same signing root",
			signedAttestations: signedAttestations(firstRoot, firstRoot),
			wantBlacklisted:    false,
		},
		{
			name:               "attestations without signing root",
			signedAttestations: signedAttestations("", ""),
			wantBlacklisted:    false,
		},
		{
			name:               "attestations with a signing root on only one of them",
			signedAttestations: signedAttestations("", firstRoot),
			wantBlacklisted:    true,
		},
		{
			name:               "attestations with different signing roots",
			signedAttestations: signedAttestations(firstRoot, secondRoot),
			wantBlacklisted:    true,
		},
		{
			name: "attestations with the same target epoch and different source epochs",
			signedAttestations: []*format.SignedAttestation{
				{SourceEpoch: "2290", TargetEpoch: "3007"},
				{SourceEpoch: "2291", TargetEpoch: "3007"},
			},
			wantBlacklisted: true,
		},
		{
			name:            "blocks with the same signing root",
			signedBlocks:    signedBlocks(firstRoot, firstRoot),
			wantBlacklisted: false,
		},
		{
			name:            "blocks without signing root",
			signedBlocks:    signedBlocks("", ""),
			wantBlacklisted: false,
		},
		{
			name:            "blocks with a signing root on only one of them",
			signedBlocks:    signedBlocks("", firstRoot),
			wantBlacklisted: true,
		},
		{
			name:            "blocks with different signing roots",
			signedBlocks:    signedBlocks(firstRoot, secondRoot),
			wantBlacklisted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			publicKeys, err := valtest.CreateRandomPubKeys(1)
			require.NoError(t, err, "could not create public key")
			validatorDB := setupDB(t, publicKeys)

			interchangeJSON := &format.EIPSlashingProtectionFormat{}
			interchangeJSON.Metadata.InterchangeFormatVersion = format.InterchangeFormatVersion
			interchangeJSON.Metadata.GenesisValidatorsRoot = fmt.Sprintf("%#x", bytesutil.PadTo([]byte{32}, 32))
			interchangeJSON.Data = []*format.ProtectionData{
				{
					Pubkey:             fmt.Sprintf("%#x", publicKeys[0]),
					SignedAttestations: tt.signedAttestations,
					SignedBlocks:       tt.signedBlocks,
				},
			}

			blob, err := json.Marshal(interchangeJSON)
			require.NoError(t, err, "could not marshal interchange JSON")

			err = validatorDB.ImportStandardProtectionJSON(ctx, bytes.NewBuffer(blob))
			require.NoError(t, err, "could not import interchange JSON")

			blacklistedPublicKeys, err := validatorDB.EIPImportBlacklistedPublicKeys(ctx)
			require.NoError(t, err, "could not get blacklisted public keys")

			blacklisted := len(blacklistedPublicKeys) > 0
			require.Equal(t, tt.wantBlacklisted, blacklisted, "unexpected blacklisting of the public key")
		})
	}
}
