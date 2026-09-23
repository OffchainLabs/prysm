package client

import (
	"context"
	stderrors "errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"github.com/pkg/errors"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestSubmitPayloadAttestation_DataFailure(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		log  string
	}{
		{"request failed", errors.New("request failed"), "Could not request payload attestation data"},
		{"data unavailable", unavailableErr(), "Skipping payload attestation: data unavailable"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hook := logTest.NewGlobal()
			v, m, key, finish := setup(t, false)
			defer finish()
			ptcRetrySetup(t, v, key)
			v.genesisTime = time.Time{}

			calls := 0
			m.validatorClient.EXPECT().PayloadAttestationData(gomock.Any(), primitives.Slot(1)).
				DoAndReturn(func(ctx context.Context, _ primitives.Slot) (*ethpb.PayloadAttestationData, error) {
					calls++
					if calls == 1 {
						return nil, tt.err
					}
					<-ctx.Done()
					return nil, ctx.Err()
				}).Times(2)

			v.SubmitPayloadAttestation(t.Context(), 1, bytesutil.ToBytes48(key.PublicKey().Marshal()))
			require.LogsContain(t, hook, tt.log)
			require.LogsDoNotContain(t, hook, "Submitted new payload attestation")
		})
	}
}

func ptcRetrySetup(t *testing.T, v *validator, key bls.SecretKey) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	cfg.SlotDurationMilliseconds = 200
	params.OverrideBeaconConfig(cfg)
	v.genesisTime = time.Now().Add(-cfg.SlotDuration())

	root := [32]byte{'b'}
	v.payloadAvailability.notify(1, &root)
	v.duties = &dutyStore{}
	var data dutyStoreData
	data.setFromContainer(&ethpb.ValidatorDutiesContainer{CurrentEpochDuties: []*ethpb.ValidatorDuty{
		{PublicKey: key.PublicKey().Marshal(), ValidatorIndex: 7},
	}})
	v.duties.write(data)
}

func unavailableErr() error {
	return errors.Wrap(
		status.Error(codes.Unavailable, "payload attestation data not yet final for slot 1"),
		"PayloadAttestationData",
	)
}

func TestSubmitPayloadAttestation_RetryRecovers(t *testing.T) {
	for _, minimal := range [...]bool{false, true} {
		t.Run(fmt.Sprintf("SlashingProtectionMinimal:%v", minimal), func(t *testing.T) {
			hook := logTest.NewGlobal()
			v, m, key, finish := setup(t, minimal)
			defer finish()
			ptcRetrySetup(t, v, key)
			want := &ethpb.PayloadAttestationData{
				BeaconBlockRoot: bytesutil.PadTo([]byte{'b'}, 32), Slot: 1,
				PayloadPresent: false, BlobDataAvailable: true,
			}
			gomock.InOrder(
				m.validatorClient.EXPECT().PayloadAttestationData(gomock.Any(), primitives.Slot(1)).Return(nil, unavailableErr()),
				m.validatorClient.EXPECT().PayloadAttestationData(gomock.Any(), primitives.Slot(1)).
					DoAndReturn(func(ctx context.Context, _ primitives.Slot) (*ethpb.PayloadAttestationData, error) {
						v.waitUntilSlotComponent(ctx, 1, params.BeaconConfig().PayloadAttestationDueBPS)
						return want, nil
					}),
			)
			m.validatorClient.EXPECT().DomainData(gomock.Any(), gomock.Any()).
				Return(&ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)
			var got *ethpb.PayloadAttestationMessage
			m.validatorClient.EXPECT().SubmitPayloadAttestation(gomock.Any(), gomock.Any()).
				Do(func(_ context.Context, msg *ethpb.PayloadAttestationMessage) { got = msg }).
				Return(&emptypb.Empty{}, nil)

			v.SubmitPayloadAttestation(t.Context(), 1, bytesutil.ToBytes48(key.PublicKey().Marshal()))
			require.LogsContain(t, hook, "Submitted new payload attestation")
			require.LogsDoNotContain(t, hook, "Skipping payload attestation")
			require.NotNil(t, got)
			require.DeepEqual(t, want, got.Data)
			require.Equal(t, 96, len(got.Signature))
		})
	}
}

func TestPayloadAttestationDataWithRetry_Recovery(t *testing.T) {
	for _, tt := range []struct {
		name          string
		dueIn         time.Duration
		responseDelay time.Duration
		err           error
	}{
		{"before due", time.Second, 0, unavailableErr()},
		{"at due", 0, 0, unavailableErr()},
		{"after due", -time.Second, 0, unavailableErr()},
		{"response crosses due", 100 * time.Millisecond, 110 * time.Millisecond, unavailableErr()},
		{"rest unavailable", -time.Second, 0, &httputil.DefaultJsonError{Code: http.StatusServiceUnavailable}},
		{"no block", -time.Second, 0, status.Error(codes.NotFound, "no block")},
		{"other failure", -time.Second, 0, errors.New("request failed")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v, m, key, finish := setup(t, false)
			defer finish()
			ptcRetrySetup(t, v, key)
			cfg := params.BeaconConfig()
			due := time.Now().Add(tt.dueIn)
			v.genesisTime = due.Add(-cfg.SlotDuration()).Add(-cfg.SlotComponentDuration(cfg.PayloadAttestationDueBPS))
			want := &ethpb.PayloadAttestationData{Slot: 1, PayloadPresent: true, BlobDataAvailable: true}
			var cutoff, firstAttempt time.Time
			calls := 0
			start := time.Now()
			m.validatorClient.EXPECT().PayloadAttestationData(gomock.Any(), primitives.Slot(1)).
				DoAndReturn(func(ctx context.Context, _ primitives.Slot) (*ethpb.PayloadAttestationData, error) {
					deadline, ok := ctx.Deadline()
					require.Equal(t, true, ok)
					calls++
					if calls == 1 {
						cutoff, firstAttempt = deadline, time.Now()
						if due.After(start) {
							require.Equal(t, true, cutoff.Equal(due.Add(500*time.Millisecond)))
						} else {
							require.Equal(t, true, !cutoff.Before(start.Add(500*time.Millisecond)))
							require.Equal(t, true, !cutoff.After(firstAttempt.Add(500*time.Millisecond)))
						}
						time.Sleep(tt.responseDelay)
						return nil, tt.err
					}
					require.Equal(t, true, cutoff.Equal(deadline), "retries must share one cutoff")
					require.Equal(t, true, time.Since(firstAttempt) >= 50*time.Millisecond)
					if tt.name == "before due" {
						require.Equal(t, true, time.Now().Before(due))
					}
					if calls < 3 {
						return nil, tt.err
					}
					return want, nil
				}).Times(3)

			got, retried, err := v.payloadAttestationDataWithRetry(t.Context(), 1)
			require.NoError(t, err)
			require.Equal(t, true, retried)
			require.DeepEqual(t, want, got)
		})
	}
}

func TestPayloadAttestationDataWithRetry_CallerCancellation(t *testing.T) {
	for _, tt := range []struct {
		name     string
		before   bool
		deadline bool
		fallback bool
	}{
		{name: "already cancelled", before: true},
		{name: "cancelled after failure"},
		{name: "parent deadline", deadline: true},
		{name: "cancelled with fallback", fallback: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v, m, key, finish := setup(t, false)
			defer finish()
			ptcRetrySetup(t, v, key)
			ctx, cancel := context.WithCancel(t.Context())
			if tt.deadline {
				cancel()
				ctx, cancel = context.WithTimeout(t.Context(), 25*time.Millisecond)
			}
			defer cancel()
			ctx, err := v.withPayloadHeadHint(ctx, 1)
			require.NoError(t, err)
			if tt.before {
				cancel()
			} else {
				m.validatorClient.EXPECT().PayloadAttestationData(gomock.Any(), primitives.Slot(1)).
					DoAndReturn(func(readCtx context.Context, _ primitives.Slot) (*ethpb.PayloadAttestationData, error) {
						if tt.deadline {
							want, _ := ctx.Deadline()
							got, _ := readCtx.Deadline()
							require.Equal(t, true, want.Equal(got))
						} else {
							cancel()
						}
						if tt.fallback {
							return &ethpb.PayloadAttestationData{Slot: 1, BeaconBlockRoot: bytesutil.PadTo([]byte{'a'}, 32)}, nil
						}
						return nil, unavailableErr()
					})
			}

			got, retried, err := v.payloadAttestationDataWithRetry(ctx, 1)
			require.ErrorIs(t, err, ctx.Err())
			require.Equal(t, true, got == nil)
			require.Equal(t, false, retried)
		})
	}
}

func TestPayloadAttestationDataWithRetry_PreferredRoot(t *testing.T) {
	for _, tt := range []struct {
		name      string
		preferred bool
		firstRoot byte
		attempts  int
	}{
		{"no preferred root", false, 'a', 1},
		{"matching root", true, 'b', 1},
		{"mismatch then match", true, 'a', 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v, m, key, finish := setup(t, false)
			defer finish()
			ptcRetrySetup(t, v, key)
			ctx := t.Context()
			if tt.preferred {
				var err error
				ctx, err = v.withPayloadHeadHint(ctx, 1)
				require.NoError(t, err)
			}
			first := &ethpb.PayloadAttestationData{Slot: 1, BeaconBlockRoot: bytesutil.PadTo([]byte{tt.firstRoot}, 32), PayloadPresent: true, BlobDataAvailable: true}
			want := first
			m.validatorClient.EXPECT().PayloadAttestationData(gomock.Any(), primitives.Slot(1)).Return(first, nil)
			if tt.attempts == 2 {
				want = &ethpb.PayloadAttestationData{Slot: 1, BeaconBlockRoot: bytesutil.PadTo([]byte{'b'}, 32), PayloadPresent: true, BlobDataAvailable: true}
				m.validatorClient.EXPECT().PayloadAttestationData(gomock.Any(), primitives.Slot(1)).Return(want, nil)
			}

			got, retried, err := v.payloadAttestationDataWithRetry(ctx, 1)
			require.NoError(t, err)
			require.Equal(t, tt.attempts > 1, retried)
			require.DeepEqual(t, want, got)
		})
	}
}

func TestSubmitPayloadAttestation_FallbackPublicationDeadline(t *testing.T) {
	for _, tt := range []struct {
		name      string
		remaining time.Duration
		publish   bool
	}{
		{"read expires with live slot", 2 * time.Second, true},
		{"slot expires with fallback", 300 * time.Millisecond, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v, m, key, finish := setup(t, false)
			defer finish()
			ptcRetrySetup(t, v, key)
			cfg := params.BeaconConfig().Copy()
			cfg.SlotDurationMilliseconds = 12000
			params.OverrideBeaconConfig(cfg)
			v.genesisTime = time.Now().Add(-2 * cfg.SlotDuration()).Add(tt.remaining)
			ctx, cancel := context.WithDeadline(t.Context(), v.SlotDeadline(1))
			defer cancel()
			parentDeadline, _ := ctx.Deadline()
			want := &ethpb.PayloadAttestationData{Slot: 1, BeaconBlockRoot: bytesutil.PadTo([]byte{'a'}, 32)}
			var readCtx context.Context
			gomock.InOrder(
				m.validatorClient.EXPECT().PayloadAttestationData(gomock.Any(), primitives.Slot(1)).Return(want, nil),
				m.validatorClient.EXPECT().PayloadAttestationData(gomock.Any(), primitives.Slot(1)).
					DoAndReturn(func(ctx context.Context, _ primitives.Slot) (*ethpb.PayloadAttestationData, error) {
						readCtx = ctx
						<-ctx.Done()
						return nil, ctx.Err()
					}),
			)
			if tt.publish {
				m.validatorClient.EXPECT().DomainData(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, _ *ethpb.DomainRequest) (*ethpb.DomainResponse, error) {
						require.ErrorIs(t, readCtx.Err(), context.DeadlineExceeded)
						require.NoError(t, ctx.Err())
						deadline, _ := ctx.Deadline()
						require.Equal(t, true, parentDeadline.Equal(deadline))
						return &ethpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil
					})
				m.validatorClient.EXPECT().SubmitPayloadAttestation(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, msg *ethpb.PayloadAttestationMessage) (*emptypb.Empty, error) {
						require.NoError(t, ctx.Err())
						require.DeepEqual(t, want, msg.Data)
						require.Equal(t, 96, len(msg.Signature))
						return &emptypb.Empty{}, nil
					})
			}

			v.SubmitPayloadAttestation(ctx, 1, bytesutil.ToBytes48(key.PublicKey().Marshal()))
			require.Equal(t, tt.publish, v.km.(*mockKeymanager).lastSignRequest() != nil)
			if tt.publish {
				require.NoError(t, ctx.Err())
			} else {
				require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
			}
		})
	}
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
			name:     "grpc unavailable joined with cancellation",
			err:      stderrors.Join(unavailableErr(), context.Canceled),
			expected: payloadAttestationSkippedUnavailable,
		},
		{
			name:     "grpc not found joined with deadline",
			err:      stderrors.Join(status.Error(codes.NotFound, "no block"), context.DeadlineExceeded),
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
	payloadRoot := [32]byte{0xaa}
	hinted := iface.WithHint(t.Context(), iface.Hint{Head: func() (iface.Head, bool) {
		return iface.Head{Root: payloadRoot, Slot: 1}, true
	}})
	tests := []struct {
		name     string
		ctx      context.Context
		data     *ethpb.PayloadAttestationData
		err      error
		expected string
	}{
		{name: "no hint", ctx: t.Context(), data: &ethpb.PayloadAttestationData{BeaconBlockRoot: make([]byte, 32)}, expected: payloadAttestationRecovered},
		{name: "matches payload root", ctx: hinted, data: &ethpb.PayloadAttestationData{BeaconBlockRoot: payloadRoot[:]}, expected: payloadAttestationRecovered},
		{name: "mismatched payload root", ctx: hinted, data: &ethpb.PayloadAttestationData{BeaconBlockRoot: make([]byte, 32)}, expected: payloadAttestationFallback},
		{name: "unavailable", ctx: hinted, err: unavailableErr(), expected: payloadAttestationSkippedUnavailable},
		{name: "other error", ctx: hinted, err: errors.New("boom"), expected: payloadAttestationFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, payloadAttestationRetryOutcome(tt.ctx, tt.data, tt.err))
		})
	}
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
