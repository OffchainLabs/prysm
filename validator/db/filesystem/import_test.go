package filesystem

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/validator/db/common"
	"github.com/prysmaticlabs/prysm/v5/validator/slashing-protection-history/format"
	valtest "github.com/prysmaticlabs/prysm/v5/validator/testing"
)

func TestStore_ImportInterchangeData_BadJSON(t *testing.T) {
	// Create a database path.
	databaseParentPath := t.TempDir()

	// Create a new store.
	s, err := NewStore(databaseParentPath, nil)
	require.NoError(t, err, "NewStore should not return an error")

	buf := bytes.NewBuffer([]byte("helloworld"))
	err = s.ImportStandardProtectionJSON(context.Background(), buf)
	require.ErrorContains(t, "could not unmarshal slashing protection JSON file", err)
}

func TestStore_ImportInterchangeData_NilData_FailsSilently(t *testing.T) {
	// Create a database path.
	databaseParentPath := t.TempDir()

	// Create a new store.
	s, err := NewStore(databaseParentPath, nil)
	require.NoError(t, err, "NewStore should not return an error")

	interchangeJSON := &format.EIPSlashingProtectionFormat{}
	encoded, err := json.Marshal(interchangeJSON)
	require.NoError(t, err)

	buf := bytes.NewBuffer(encoded)
	err = s.ImportStandardProtectionJSON(context.Background(), buf)
	require.NoError(t, err)
}

func TestStore_ImportInterchangeData_BadFormat_PreventsDBWrites(t *testing.T) {
	ctx := context.Background()
	numValidators := 10
	publicKeys, err := valtest.CreateRandomPubKeys(numValidators)
	require.NoError(t, err)

	// Create a database path.
	databaseParentPath := t.TempDir()

	// Create a new store.
	s, err := NewStore(databaseParentPath, &Config{PubKeys: publicKeys})
	require.NoError(t, err, "NewStore should not return an error")

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
	err = s.ImportStandardProtectionJSON(ctx, buf)
	assert.NotNil(t, err)

	// Next, we attempt to retrieve the attesting and proposals histories from our database and
	// verify nothing was saved to the DB. If there is an error in the import process, we need to make
	// sure writing is an atomic operation: either the import succeeds and saves the slashing protection
	// data to our DB, or it does not.
	for i := 0; i < len(publicKeys); i++ {
		receivedHistory, err := s.ProposalHistoryForPubKey(ctx, publicKeys[i])
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
	ctx := context.Background()
	numValidators := 10
	publicKeys, err := valtest.CreateRandomPubKeys(numValidators)
	require.NoError(t, err)

	// Create a database path.
	databaseParentPath := t.TempDir()

	// Create a new store.
	s, err := NewStore(databaseParentPath, &Config{PubKeys: publicKeys})
	require.NoError(t, err, "NewStore should not return an error")

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
	err = s.ImportStandardProtectionJSON(ctx, buf)
	require.NoError(t, err)

	// Next, we attempt to retrieve the attesting and proposals histories from our database and
	// verify those indeed match the originally generated mock histories.
	for i := 0; i < len(publicKeys); i++ {
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
			err := s.SaveAttestationForPubKey(ctx, publicKeys[i], [fieldparams.RootLength]byte{}, indexedAtt)
			require.ErrorContains(t, "could not sign attestation", err)
		}
	}
}
