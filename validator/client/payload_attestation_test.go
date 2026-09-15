package client

import (
	"context"
	stderrors "errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/crypto/bls"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/pkg/errors"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestSubmitPayloadAttestation_PayloadAttestationDataFailure(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()

			m.validatorClient.EXPECT().
				PayloadAttestationData(gomock.Any(), gomock.Any()).
				Return(nil, errors.New("request failed"))

			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.SubmitPayloadAttestation(t.Context(), 1, pubKey)
			require.LogsContain(t, hook, "Could not request payload attestation data")
		})
	}
}

func TestSubmitPayloadAttestation_NoHeadBlockForSlot(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()

			unavailable := errors.Wrap(
				status.Error(codes.Unavailable, "no valid block root for slot 1, highest received block slot is 0"),
				"PayloadAttestationData",
			)
			m.validatorClient.EXPECT().
				PayloadAttestationData(gomock.Any(), gomock.Any()).
				Return(nil, unavailable)

			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.SubmitPayloadAttestation(t.Context(), 1, pubKey)
			require.LogsContain(t, hook, "Skipping payload attestation: data unavailable")
			require.LogsDoNotContain(t, hook, "Could not request payload attestation data")
		})
	}
}

// ptcRetrySetup puts the slot 1 payload attestation deadline shortly ahead with the
// availability event already delivered, and returns the deadline lead time.
func ptcRetrySetup(t *testing.T, validator *validator, validatorKey bls.SecretKey) time.Duration {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	// PayloadAttestationDueBPS is 7500, so the deadline sits 150ms into the slot.
	cfg.SlotDurationMilliseconds = 200
	params.OverrideBeaconConfig(cfg)

	lead := cfg.SlotComponentDuration(cfg.PayloadAttestationDueBPS)
	// Place genesis so that slot 1 starts now and its deadline is lead away.
	validator.genesisTime = time.Now().Add(-time.Duration(cfg.SlotDurationMilliseconds) * time.Millisecond)

	// Release the per-slot waiter as the execution_payload_available event would.
	root := bytesutil.ToBytes32(bytesutil.PadTo([]byte{'b'}, 32))
	validator.payloadAvailability.notify(1, &root)

	validatorIndex := primitives.ValidatorIndex(7)
	validator.duties = &dutyStore{}
	var data dutyStoreData
	data.setFromContainer(&ethpb.ValidatorDutiesContainer{CurrentEpochDuties: []*ethpb.ValidatorDuty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			ValidatorIndex: validatorIndex,
		},
	}})
	validator.duties.write(data)

	return lead
}

// unavailableErr mirrors the gRPC error for a node withholding non-final data.
func unavailableErr() error {
	return errors.Wrap(
		status.Error(codes.Unavailable, "payload attestation data not yet final for slot 1"),
		"PayloadAttestationData",
	)
}

// Regression test for issue #17464: a request released early by the availability event
// is retried at the deadline rather than abstaining.
func TestSubmitPayloadAttestation_RetriesAtDeadlineAfterUnavailable(t *testing.T) {
	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			lead := ptcRetrySetup(t, validator, validatorKey)

			blockRoot := bytesutil.PadTo([]byte{'b'}, 32)
			gomock.InOrder(
				m.validatorClient.EXPECT().
					PayloadAttestationData(gomock.Any(), primitives.Slot(1)).
					Return(nil, unavailableErr()).
					Times(1),
				m.validatorClient.EXPECT().
					PayloadAttestationData(gomock.Any(), primitives.Slot(1)).
					Return(&ethpb.PayloadAttestationData{
						BeaconBlockRoot:   blockRoot,
						Slot:              1,
						PayloadPresent:    false,
						BlobDataAvailable: true,
					}, nil).
					Times(1),
			)

			m.validatorClient.EXPECT().
				DomainData(gomock.Any(), gomock.Any()).
				Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

			var generatedMsg *ethpb.PayloadAttestationMessage
			m.validatorClient.EXPECT().
				SubmitPayloadAttestation(gomock.Any(), gomock.AssignableToTypeOf(&ethpb.PayloadAttestationMessage{})).
				Do(func(_ context.Context, msg *ethpb.PayloadAttestationMessage) {
					generatedMsg = msg
				}).
				Return(&emptypb.Empty{}, nil)

			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			start := time.Now()
			validator.SubmitPayloadAttestation(t.Context(), 1, pubKey)
			elapsed := time.Since(start)

			require.LogsContain(t, hook, "Submitted new payload attestation")
			require.LogsDoNotContain(t, hook, "Skipping payload attestation")
			require.NotNil(t, generatedMsg)
			// The honest late vote is cast instead of abstaining.
			require.Equal(t, false, generatedMsg.Data.PayloadPresent)
			require.Equal(t, true, generatedMsg.Data.BlobDataAvailable)
			// The second request really waited for the deadline.
			require.Equal(t, true, elapsed >= lead-10*time.Millisecond,
				fmt.Sprintf("expected to wait about %s for the deadline, waited %s", lead, elapsed))
		})
	}
}

// A pre-deadline failure that only arrives after the deadline is still retried.
func TestPayloadAttestationDataWithRetry_ResponseCrossesDeadline(t *testing.T) {
	v, m, key, finish := setup(t, false)
	defer finish()
	ptcRetrySetup(t, v, key)

	const slot primitives.Slot = 1
	want := &ethpb.PayloadAttestationData{
		Slot: slot, PayloadPresent: false, BlobDataAvailable: true,
	}
	var deadline time.Time
	gomock.InOrder(
		m.validatorClient.EXPECT().
			PayloadAttestationData(gomock.Any(), slot).
			DoAndReturn(func(_ context.Context, _ primitives.Slot) (*ethpb.PayloadAttestationData, error) {
				require.Equal(t, true, time.Now().Before(deadline))
				// Deliver a pre-deadline failure after the deadline has passed.
				err := unavailableErr()
				time.Sleep(time.Until(deadline.Add(time.Millisecond)))
				return nil, err
			}),
		m.validatorClient.EXPECT().
			PayloadAttestationData(gomock.Any(), slot).
			Return(want, nil),
	)

	// Start the slot after mock setup to preserve the pre-deadline window.
	cfg := params.BeaconConfig()
	v.genesisTime = time.Now().Add(-cfg.SlotDuration())
	deadline, err := v.slotComponentDeadline(slot, cfg.PayloadAttestationDueBPS)
	require.NoError(t, err)

	got, retried, err := v.payloadAttestationDataWithRetry(t.Context(), slot)
	require.NoError(t, err)
	require.Equal(t, true, retried)
	require.DeepEqual(t, want, got)
}

// After the deadline there is nothing to wait for, so the request is not repeated.
func TestSubmitPayloadAttestation_NoRetryAfterDeadline(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	hook := logTest.NewGlobal()
	// setup leaves genesisTime at the zero value, so the deadline is long past.
	validator, m, validatorKey, finish := setup(t, false)
	defer finish()

	m.validatorClient.EXPECT().
		PayloadAttestationData(gomock.Any(), primitives.Slot(1)).
		Return(nil, unavailableErr()).
		Times(1)

	var pubKey [fieldparams.BLSPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.SubmitPayloadAttestation(t.Context(), 1, pubKey)

	require.LogsContain(t, hook, "Skipping payload attestation: data unavailable")
	require.LogsDoNotContain(t, hook, "Could not request payload attestation data")
}

// A node still withholding at the deadline skips the slot after a second attempt.
func TestSubmitPayloadAttestation_RetryStillUnavailable(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, m, validatorKey, finish := setup(t, false)
	defer finish()
	ptcRetrySetup(t, validator, validatorKey)

	// No DomainData or SubmitPayloadAttestation expectations: an unexpected call fails.
	m.validatorClient.EXPECT().
		PayloadAttestationData(gomock.Any(), primitives.Slot(1)).
		Return(nil, unavailableErr()).
		Times(2)

	var pubKey [fieldparams.BLSPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.SubmitPayloadAttestation(t.Context(), 1, pubKey)

	require.LogsContain(t, hook, "Skipping payload attestation: data unavailable")
	require.LogsDoNotContain(t, hook, "Submitted new payload attestation")
}

// A block reaching the queried node only after the retry still yields a vote.
func TestSubmitPayloadAttestation_RetryOnNoBlockThenBlockArrives(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, m, validatorKey, finish := setup(t, false)
	defer finish()
	ptcRetrySetup(t, validator, validatorKey)

	blockRoot := bytesutil.PadTo([]byte{'b'}, 32)
	gomock.InOrder(
		m.validatorClient.EXPECT().
			PayloadAttestationData(gomock.Any(), primitives.Slot(1)).
			Return(nil, errors.Wrap(status.Error(codes.NotFound, "no block found at slot=1"), "PayloadAttestationData")).
			Times(1),
		m.validatorClient.EXPECT().
			PayloadAttestationData(gomock.Any(), primitives.Slot(1)).
			Return(&ethpb.PayloadAttestationData{
				BeaconBlockRoot:   blockRoot,
				Slot:              1,
				PayloadPresent:    true,
				BlobDataAvailable: true,
			}, nil).
			Times(1),
	)
	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)
	m.validatorClient.EXPECT().
		SubmitPayloadAttestation(gomock.Any(), gomock.AssignableToTypeOf(&ethpb.PayloadAttestationMessage{})).
		Return(&emptypb.Empty{}, nil)

	var pubKey [fieldparams.BLSPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.SubmitPayloadAttestation(t.Context(), 1, pubKey)

	require.LogsContain(t, hook, "Submitted new payload attestation")
	require.LogsDoNotContain(t, hook, "Skipping payload attestation")
}

// A cancelled context aborts the wait instead of retrying on a dead context.
func TestSubmitPayloadAttestation_RetryAbortsOnContextCancel(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, m, validatorKey, finish := setup(t, false)
	defer finish()
	lead := ptcRetrySetup(t, validator, validatorKey)

	m.validatorClient.EXPECT().
		PayloadAttestationData(gomock.Any(), primitives.Slot(1)).
		Return(nil, unavailableErr()).
		Times(1)

	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(lead / 4)
		cancel()
	}()

	var pubKey [fieldparams.BLSPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	start := time.Now()
	validator.SubmitPayloadAttestation(ctx, 1, pubKey)

	require.Equal(t, true, time.Since(start) < lead,
		"cancellation should cut the wait short instead of running to the deadline")
	require.LogsContain(t, hook, "Skipping payload attestation: data unavailable")
	require.LogsDoNotContain(t, hook, "Submitted new payload attestation")
}

func TestPayloadAttestationDataFailure(t *testing.T) {
	// The wrap chain a beacon API multi-node read actually produces.
	restErr := func(code int) error {
		return errors.Wrap(
			fmt.Errorf("read until: %w",
				stderrors.Join(fmt.Errorf("get ssz: %w", &httputil.DefaultJsonError{Code: code}))),
			"could not get execution payload attestation data",
		)
	}

	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "grpc unavailable",
			err:      status.Error(codes.Unavailable, "not yet final"),
			expected: payloadAttestationSkippedUnavailable,
		},
		{
			name:     "wrapped grpc not found",
			err:      errors.Wrap(status.Error(codes.NotFound, "no block"), "PayloadAttestationData"),
			expected: payloadAttestationSkippedNoBlock,
		},
		{
			name:     "beacon api 503",
			err:      restErr(http.StatusServiceUnavailable),
			expected: payloadAttestationSkippedUnavailable,
		},
		{
			name:     "beacon api 204",
			err:      restErr(http.StatusNoContent),
			expected: payloadAttestationSkippedNoBlock,
		},
		{
			name:     "beacon api 404",
			err:      restErr(http.StatusNotFound),
			expected: payloadAttestationSkippedNoBlock,
		},
		{
			name: "joined 503 and 204 prefers unavailable",
			err: stderrors.Join(
				&httputil.DefaultJsonError{Code: http.StatusNoContent},
				&httputil.DefaultJsonError{Code: http.StatusServiceUnavailable},
			),
			expected: payloadAttestationSkippedUnavailable,
		},
		{
			name:     "grpc invalid argument",
			err:      status.Error(codes.InvalidArgument, "wrong slot"),
			expected: payloadAttestationFailed,
		},
		{
			name:     "unclassified",
			err:      errors.New("boom"),
			expected: payloadAttestationFailed,
		},
		{
			name:     "context cancelled",
			err:      context.Canceled,
			expected: payloadAttestationFailed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, payloadAttestationDataFailure(tt.err))
		})
	}
}

func TestPayloadAttestationRetryOutcome(t *testing.T) {
	require.Equal(t, payloadAttestationRecovered, payloadAttestationRetryOutcome(nil))
	require.Equal(t, payloadAttestationSkippedUnavailable, payloadAttestationRetryOutcome(unavailableErr()))
	require.Equal(t, payloadAttestationFailed, payloadAttestationRetryOutcome(errors.New("boom")))
}

func TestSubmitPayloadAttestation_ValidatorDutiesRequestFailure(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			validator.duties = &dutyStore{}
			{
				var data dutyStoreData
				data.setFromContainer(&ethpb.ValidatorDutiesContainer{CurrentEpochDuties: []*ethpb.ValidatorDuty{}})
				validator.duties.write(data)
			}
			defer finish()

			m.validatorClient.EXPECT().
				PayloadAttestationData(gomock.Any(), gomock.Any()).
				Return(&ethpb.PayloadAttestationData{
					BeaconBlockRoot: bytesutil.PadTo([]byte{'a'}, 32),
					Slot:            1,
					PayloadPresent:  true,
				}, nil)

			m.validatorClient.EXPECT().
				DomainData(gomock.Any(), gomock.Any()).
				Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.SubmitPayloadAttestation(t.Context(), 1, pubKey)
			require.LogsContain(t, hook, "Could not fetch validator assignment")
		})
	}
}

func TestSubmitPayloadAttestation_BadDomainData(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			validatorIndex := primitives.ValidatorIndex(7)
			validator.duties = &dutyStore{}
			{
				var data dutyStoreData
				data.setFromContainer(&ethpb.ValidatorDutiesContainer{CurrentEpochDuties: []*ethpb.ValidatorDuty{
					{
						PublicKey:      validatorKey.PublicKey().Marshal(),
						ValidatorIndex: validatorIndex,
					},
				}})
				validator.duties.write(data)
			}

			m.validatorClient.EXPECT().
				PayloadAttestationData(gomock.Any(), gomock.Any()).
				Return(&ethpb.PayloadAttestationData{
					BeaconBlockRoot: bytesutil.PadTo([]byte{'a'}, 32),
					Slot:            1,
					PayloadPresent:  true,
				}, nil)

			m.validatorClient.EXPECT().
				DomainData(gomock.Any(), gomock.Any()).
				Return(nil, errors.New("uh oh"))

			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.SubmitPayloadAttestation(t.Context(), 1, pubKey)
			require.LogsContain(t, hook, "Could not get PTC attester domain data")
		})
	}
}

func TestSubmitPayloadAttestation_CouldNotSubmit(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			validatorIndex := primitives.ValidatorIndex(7)
			validator.duties = &dutyStore{}
			{
				var data dutyStoreData
				data.setFromContainer(&ethpb.ValidatorDutiesContainer{CurrentEpochDuties: []*ethpb.ValidatorDuty{
					{
						PublicKey:      validatorKey.PublicKey().Marshal(),
						ValidatorIndex: validatorIndex,
					},
				}})
				validator.duties.write(data)
			}

			m.validatorClient.EXPECT().
				PayloadAttestationData(gomock.Any(), gomock.Any()).
				Return(&ethpb.PayloadAttestationData{
					BeaconBlockRoot: bytesutil.PadTo([]byte{'a'}, 32),
					Slot:            1,
					PayloadPresent:  true,
				}, nil)

			m.validatorClient.EXPECT().
				DomainData(gomock.Any(), gomock.Any()).
				Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

			m.validatorClient.EXPECT().
				SubmitPayloadAttestation(gomock.Any(), gomock.AssignableToTypeOf(&ethpb.PayloadAttestationMessage{})).
				Return(&emptypb.Empty{}, errors.New("submit failed"))

			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.SubmitPayloadAttestation(t.Context(), 1, pubKey)
			require.LogsContain(t, hook, "Could not submit payload attestation")
		})
	}
}

func TestSubmitPayloadAttestation_OK(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	for _, isSlashingProtectionMinimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", isSlashingProtectionMinimal), func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t, isSlashingProtectionMinimal)
			defer finish()
			validatorIndex := primitives.ValidatorIndex(7)
			validator.duties = &dutyStore{}
			{
				var data dutyStoreData
				data.setFromContainer(&ethpb.ValidatorDutiesContainer{CurrentEpochDuties: []*ethpb.ValidatorDuty{
					{
						PublicKey:      validatorKey.PublicKey().Marshal(),
						ValidatorIndex: validatorIndex,
					},
				}})
				validator.duties.write(data)
			}

			blockRoot := bytesutil.PadTo([]byte{'b'}, 32)
			m.validatorClient.EXPECT().
				PayloadAttestationData(gomock.Any(), gomock.Any()).
				Return(&ethpb.PayloadAttestationData{
					BeaconBlockRoot: blockRoot,
					Slot:            1,
					PayloadPresent:  true,
				}, nil)

			m.validatorClient.EXPECT().
				DomainData(gomock.Any(), gomock.Any()).
				Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

			var generatedMsg *ethpb.PayloadAttestationMessage
			m.validatorClient.EXPECT().
				SubmitPayloadAttestation(gomock.Any(), gomock.AssignableToTypeOf(&ethpb.PayloadAttestationMessage{})).
				Do(func(_ context.Context, msg *ethpb.PayloadAttestationMessage) {
					generatedMsg = msg
				}).
				Return(&emptypb.Empty{}, nil)

			var pubKey [fieldparams.BLSPubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())
			validator.SubmitPayloadAttestation(t.Context(), 1, pubKey)

			require.LogsDoNotContain(t, hook, "Could not")
			require.LogsContain(t, hook, "Submitted new payload attestation")
			require.Equal(t, validatorIndex, generatedMsg.ValidatorIndex)
			require.DeepEqual(t, blockRoot, generatedMsg.Data.BeaconBlockRoot)
			require.Equal(t, true, generatedMsg.Data.PayloadPresent)
			require.Equal(t, 96, len(generatedMsg.Signature))
		})
	}
}
