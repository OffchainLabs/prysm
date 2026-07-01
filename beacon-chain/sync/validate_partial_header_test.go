package sync

import (
	"context"
	"testing"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
)

func TestService_PartialVerifierFromTrustedColumn(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		col          *blocks.PartialDataColumn
		verifier     verification.MockDataColumnsVerifier
		wantErr      error
		expectResult bool
		verify       func(t *testing.T, v *verification.PartialColumnVerifier)
	}{
		{
			name:    "nil column",
			col:     nil,
			wantErr: errHeaderNil,
		},
		{
			name:    "empty commitments",
			col:     buildPartialColumn(t, 0, nil),
			wantErr: errHeaderEmptyCommitments,
		},
		{
			name:         "marks included cells as verified",
			col:          buildPartialColumn(t, 2, []uint64{0, 1}),
			verifier:     verification.MockDataColumnsVerifier{},
			expectResult: true,
			verify: func(t *testing.T, v *verification.PartialColumnVerifier) {
				require.NoError(t, v.SidecarKzgProofVerified())
				_, ok, err := v.Complete()
				require.NoError(t, err)
				require.Equal(t, true, ok)
			},
		},
		{
			name: "propagates verifier field errors on completion",
			col:  buildPartialColumn(t, 1, []uint64{0}),
			verifier: verification.MockDataColumnsVerifier{
				ErrValidFields: errors.New("invalid fields"),
			},
			expectResult: true,
			verify: func(t *testing.T, v *verification.PartialColumnVerifier) {
				_, _, err := v.Complete()
				require.ErrorContains(t, "invalid fields", err)
			},
		},
		{
			name:         "gloas column builds verifier and completes",
			col:          buildGloasPartialColumn(t, 2, []uint64{0, 1}),
			verifier:     verification.MockDataColumnsVerifier{},
			expectResult: true,
			verify: func(t *testing.T, v *verification.PartialColumnVerifier) {
				_, ok, err := v.Complete()
				require.NoError(t, err)
				require.Equal(t, true, ok)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service := &Service{
				newColumnsVerifier: testNewColumnsVerifier(tc.verifier),
			}
			got, err := service.partialVerifierFromTrustedColumn(ctx, tc.col)
			require.ErrorIs(t, err, tc.wantErr)
			require.Equal(t, tc.expectResult, got != nil)
			if tc.verify != nil {
				tc.verify(t, got)
			}
		})
	}
}

func TestService_ValidatePartialDataColumnHeader(t *testing.T) {
	ctx := context.Background()
	genericErr := errors.New("generic error")
	unavailableParentSlotErr := errors.Wrap(verification.ErrSidecarParentUnknown, "slot lookup failed")
	invalidVerifierErr := errors.Wrap(verification.ErrInvalid, "invalid verification")

	db := dbtest.SetupDB(t)

	// chainWithParent returns a mock chain where HasBlock returns true for the zero parent root.
	chainWithParent := func() *mock.ChainService {
		return &mock.ChainService{
			DB: db,
			InitSyncBlockRoots: map[[32]byte]bool{
				{}: true, // zero root matches buildPartialColumn's parent root
			},
		}
	}

	// chainWithoutParent returns a mock chain where HasBlock returns false.
	chainWithoutParent := func() *mock.ChainService {
		return &mock.ChainService{DB: db}
	}

	tests := []struct {
		name         string
		col          *blocks.PartialDataColumn
		chain        *mock.ChainService
		verifier     verification.MockDataColumnsVerifier
		wantErr      error
		wantResult   pubsub.ValidationResult
		expectResult bool
	}{
		{
			name:       "nil column",
			col:        nil,
			wantErr:    errHeaderNil,
			wantResult: pubsub.ValidationIgnore,
		},
		{
			name:       "gloas column rejected by fulu header path",
			col:        buildGloasPartialColumn(t, 2, nil),
			wantErr:    errColumnNotFulu,
			wantResult: pubsub.ValidationIgnore,
		},
		{
			name:       "empty commitments is reject",
			col:        buildPartialColumn(t, 0, nil),
			wantErr:    errHeaderEmptyCommitments,
			wantResult: pubsub.ValidationReject,
		},
		{
			name:         "not from future slot is ignore",
			col:          buildPartialColumn(t, 1, nil),
			verifier:     verification.MockDataColumnsVerifier{ErrNotFromFutureSlot: genericErr},
			wantErr:      genericErr,
			wantResult:   pubsub.ValidationIgnore,
			expectResult: true,
		},
		{
			name:         "slot above finalized is ignore",
			col:          buildPartialColumn(t, 1, nil),
			verifier:     verification.MockDataColumnsVerifier{ErrSlotAboveFinalized: genericErr},
			wantErr:      genericErr,
			wantResult:   pubsub.ValidationIgnore,
			expectResult: true,
		},
		{
			name:         "parent not seen is ignore",
			col:          buildPartialColumn(t, 1, nil),
			chain:        chainWithoutParent(),
			wantErr:      errHeaderParentNotSeen,
			wantResult:   pubsub.ValidationIgnore,
			expectResult: true,
		},
		{
			name:         "parent seen is ignore",
			col:          buildPartialColumn(t, 1, nil),
			chain:        chainWithParent(),
			verifier:     verification.MockDataColumnsVerifier{ErrSidecarParentSeen: genericErr},
			wantErr:      genericErr,
			wantResult:   pubsub.ValidationIgnore,
			expectResult: true,
		},
		{
			name:         "parent valid is reject",
			col:          buildPartialColumn(t, 1, nil),
			chain:        chainWithParent(),
			verifier:     verification.MockDataColumnsVerifier{ErrSidecarParentValid: genericErr},
			wantErr:      genericErr,
			wantResult:   pubsub.ValidationReject,
			expectResult: true,
		},
		{
			name:         "parent slot unavailable is ignore",
			col:          buildPartialColumn(t, 1, nil),
			chain:        chainWithParent(),
			verifier:     verification.MockDataColumnsVerifier{ErrSidecarParentSlotLower: unavailableParentSlotErr},
			wantErr:      unavailableParentSlotErr,
			wantResult:   pubsub.ValidationIgnore,
			expectResult: true,
		},
		{
			name:         "parent slot lower invalid is reject",
			col:          buildPartialColumn(t, 1, nil),
			chain:        chainWithParent(),
			verifier:     verification.MockDataColumnsVerifier{ErrSidecarParentSlotLower: genericErr},
			wantErr:      genericErr,
			wantResult:   pubsub.ValidationReject,
			expectResult: true,
		},
		{
			name:         "proposer expected verification failure is reject",
			col:          buildPartialColumn(t, 1, nil),
			chain:        chainWithParent(),
			verifier:     verification.MockDataColumnsVerifier{ErrSidecarProposerExpected: invalidVerifierErr},
			wantErr:      invalidVerifierErr,
			wantResult:   pubsub.ValidationReject,
			expectResult: true,
		},
		{
			name:         "proposer expected non verification failure is reject",
			col:          buildPartialColumn(t, 1, nil),
			chain:        chainWithParent(),
			verifier:     verification.MockDataColumnsVerifier{ErrSidecarProposerExpected: genericErr},
			wantErr:      genericErr,
			wantResult:   pubsub.ValidationReject,
			expectResult: true,
		},
		{
			name:         "invalid proposer signature is reject",
			col:          buildPartialColumn(t, 1, nil),
			chain:        chainWithParent(),
			verifier:     verification.MockDataColumnsVerifier{ErrValidProposerSignature: verification.ErrInvalidProposerSignature},
			wantErr:      verification.ErrInvalidProposerSignature,
			wantResult:   pubsub.ValidationReject,
			expectResult: true,
		},
		{
			name:         "signature infra failure is reject",
			col:          buildPartialColumn(t, 1, nil),
			chain:        chainWithParent(),
			verifier:     verification.MockDataColumnsVerifier{ErrValidProposerSignature: genericErr},
			wantErr:      genericErr,
			wantResult:   pubsub.ValidationReject,
			expectResult: true,
		},
		{
			name:         "nominal",
			col:          buildPartialColumn(t, 1, nil),
			chain:        chainWithParent(),
			verifier:     verification.MockDataColumnsVerifier{},
			wantErr:      nil,
			wantResult:   pubsub.ValidationAccept,
			expectResult: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service := &Service{
				newColumnsVerifier: testNewColumnsVerifier(tc.verifier),
			}
			if tc.chain != nil {
				service.cfg = &config{chain: tc.chain}
			}
			got, result, err := service.validatePartialDataColumnHeader(ctx, tc.col)
			require.ErrorIs(t, err, tc.wantErr)
			require.Equal(t, tc.wantResult, result)
			require.Equal(t, tc.expectResult, got != nil)
		})
	}
}

func testNewColumnsVerifier(v verification.MockDataColumnsVerifier) verification.NewDataColumnsVerifier {
	return func(cols []blocks.RODataColumn, _ []verification.Requirement) verification.DataColumnsVerifier {
		for _, col := range cols {
			v.AppendRODataColumns(col)
		}
		return &v
	}
}

func buildPartialColumn(t *testing.T, nCommitments int, included []uint64) *blocks.PartialDataColumn {
	t.Helper()

	commitments := make([][]byte, nCommitments)
	for i := range nCommitments {
		commitments[i] = make([]byte, fieldparams.KzgCommitmentSize)
		commitments[i][0] = byte(i + 1)
	}

	inclusionProof := [][]byte{
		make([]byte, 32),
		make([]byte, 32),
		make([]byte, 32),
		make([]byte, 32),
	}

	col, err := blocks.NewPartialDataColumn(
		[fieldparams.RootLength]byte{},
		&ethpb.SignedBeaconBlockHeader{
			Header: &ethpb.BeaconBlockHeader{
				ParentRoot: make([]byte, fieldparams.RootLength),
				StateRoot:  make([]byte, fieldparams.RootLength),
				BodyRoot:   make([]byte, fieldparams.RootLength),
			},
			Signature: make([]byte, fieldparams.BLSSignatureLength),
		},
		0,
		commitments,
		inclusionProof,
	)
	require.NoError(t, err)

	for _, idx := range included {
		extended := col.ExtendFromVerifiedCell(idx, []byte{byte(idx + 1)}, []byte{byte(idx + 2)})
		require.Equal(t, true, extended)
	}

	return &col
}

func buildGloasPartialColumn(t *testing.T, nCommitments int, included []uint64) *blocks.PartialDataColumn {
	t.Helper()

	commitments := make([][]byte, nCommitments)
	for i := range nCommitments {
		commitments[i] = make([]byte, fieldparams.KzgCommitmentSize)
		commitments[i][0] = byte(i + 1)
	}

	col, err := blocks.NewPartialDataColumnGloas([fieldparams.RootLength]byte{}, 0, 0, commitments)
	require.NoError(t, err)

	for _, idx := range included {
		extended := col.ExtendFromVerifiedCell(idx, []byte{byte(idx + 1)}, []byte{byte(idx + 2)})
		require.Equal(t, true, extended)
	}

	return &col
}
