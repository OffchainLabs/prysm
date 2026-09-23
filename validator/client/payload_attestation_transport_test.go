package client

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/rest"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	beaconApi "github.com/OffchainLabs/prysm/v7/validator/client/beacon-api"
	grpcApi "github.com/OffchainLabs/prysm/v7/validator/client/grpc-api"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type payloadAttestationTransportServer struct {
	ethpb.UnimplementedBeaconNodeValidatorServer
	read func(context.Context, primitives.Slot) (*ethpb.PayloadAttestationData, error)
}

func (s *payloadAttestationTransportServer) PayloadAttestationData(ctx context.Context, req *ethpb.PayloadAttestationDataRequest) (*ethpb.PayloadAttestationData, error) {
	return s.read(ctx, req.Slot)
}

func payloadAttestationTransportClient(t *testing.T, transport string, read func(context.Context, primitives.Slot) (*ethpb.PayloadAttestationData, error)) iface.ValidatorClient {
	t.Helper()
	if transport == "grpc" {
		listener := bufconn.Listen(1024 * 1024)
		server := grpc.NewServer()
		ethpb.RegisterBeaconNodeValidatorServer(server, &payloadAttestationTransportServer{read: read})
		go func() { _ = server.Serve(listener) }()
		t.Cleanup(server.Stop)
		conn, err := validatorHelpers.NewNodeConnection(validatorHelpers.WithGRPC(t.Context(), "passthrough:///payload-attestation", []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return listener.DialContext(ctx)
			}),
		}))
		require.NoError(t, err)
		t.Cleanup(conn.GetGrpcConnectionProvider().Close)
		return grpcApi.NewGrpcValidatorClient(conn)
	}

	server := httptest.NewServer(payloadAttestationTransportHandler(t, transport, read))
	t.Cleanup(server.Close)
	provider, err := rest.NewRestConnectionProvider(server.URL)
	require.NoError(t, err)
	return beaconApi.NewBeaconApiValidatorClient(provider)
}

func payloadAttestationTransportHandler(t *testing.T, transport string, read func(context.Context, primitives.Slot) (*ethpb.PayloadAttestationData, error)) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slot, err := strconv.ParseUint(r.URL.Query().Get("slot"), 10, 64)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		data, err := read(r.Context(), primitives.Slot(slot))
		if err != nil {
			code := http.StatusInternalServerError
			switch status.Code(err) {
			case codes.Unavailable:
				code = http.StatusServiceUnavailable
			case codes.InvalidArgument:
				code = http.StatusBadRequest
			case codes.NotFound:
				w.WriteHeader(http.StatusNoContent)
				return
			}
			httputil.HandleError(w, status.Convert(err).Message(), code)
			return
		}
		var body []byte
		if transport == "rest-ssz" {
			w.Header().Set("Content-Type", api.OctetStreamMediaType)
			body, err = data.MarshalSSZ()
		} else {
			w.Header().Set("Content-Type", api.JsonMediaType)
			body, err = json.Marshal(&structs.GetPayloadAttestationDataResponse{Version: "gloas", Data: structs.PayloadAttestationDataFromConsensus(data)})
		}
		if err != nil {
			t.Error(err)
			return
		}
		_, _ = w.Write(body)
	})
}

func payloadAttestationTransportValidator(client iface.ValidatorClient, due time.Time) *validator {
	cfg := params.BeaconConfig()
	return &validator{
		validatorClient:     client,
		genesisTime:         due.Add(-cfg.SlotDuration() - cfg.SlotComponentDuration(cfg.PayloadAttestationDueBPS)),
		payloadAvailability: newPayloadAvailability(),
	}
}

func TestPayloadAttestationDataWithRetry_TransportReadiness(t *testing.T) {
	for _, transport := range []string{"grpc", "rest-json", "rest-ssz"} {
		for _, tt := range []struct {
			name           string
			dueLead        time.Duration
			readyAfter     time.Duration
			payloadPresent bool
		}{
			{name: "ready before due", dueLead: 2 * time.Second, payloadPresent: true},
			{name: "ready 100ms after due", dueLead: -time.Millisecond, readyAfter: 100 * time.Millisecond},
		} {
			t.Run(transport+"/"+tt.name, func(t *testing.T) {
				var calls atomic.Int32
				var readyAt atomic.Int64
				want := &ethpb.PayloadAttestationData{Slot: 1, BeaconBlockRoot: make([]byte, 32), PayloadPresent: tt.payloadPresent, BlobDataAvailable: true}
				client := payloadAttestationTransportClient(t, transport, func(_ context.Context, slot primitives.Slot) (*ethpb.PayloadAttestationData, error) {
					if calls.Add(1) == 1 || time.Now().UnixNano() < readyAt.Load() {
						return nil, status.Error(codes.Unavailable, "not yet final")
					}
					return want, nil
				})
				start := time.Now()
				readyAt.Store(start.Add(tt.readyAfter).UnixNano())
				v := payloadAttestationTransportValidator(client, start.Add(tt.dueLead))
				root := [32]byte{}
				v.payloadAvailability.notify(1, &root)
				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()
				ctx, err := v.withPayloadHeadHint(ctx, 1)
				require.NoError(t, err)

				got, retried, err := v.payloadAttestationDataWithRetry(ctx, 1)
				require.NoError(t, err)
				require.DeepEqual(t, want, got)
				require.Equal(t, true, retried)
				require.Equal(t, true, calls.Load() >= 2)
				require.Equal(t, true, time.Now().UnixNano() >= readyAt.Load())
				require.NoError(t, ctx.Err())
			})
		}
	}
}

func TestPayloadAttestationDataWithRetry_TransportBudget(t *testing.T) {
	for _, transport := range []string{"grpc", "rest-json", "rest-ssz"} {
		for _, tt := range []struct {
			code     codes.Code
			httpCode int
			outcome  string
		}{
			{codes.Unavailable, http.StatusServiceUnavailable, payloadAttestationSkippedUnavailable},
			{codes.NotFound, http.StatusNoContent, payloadAttestationSkippedNoBlock},
			{codes.InvalidArgument, http.StatusBadRequest, payloadAttestationFailed},
			{codes.Internal, http.StatusInternalServerError, payloadAttestationFailed},
		} {
			t.Run(transport+"/"+tt.code.String(), func(t *testing.T) {
				var calls atomic.Int32
				client := payloadAttestationTransportClient(t, transport, func(context.Context, primitives.Slot) (*ethpb.PayloadAttestationData, error) {
					calls.Add(1)
					return nil, status.Error(tt.code, "test failure")
				})
				v := payloadAttestationTransportValidator(client, time.Now().Add(-time.Second))
				ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
				defer cancel()
				ctx, err := v.withPayloadHeadHint(ctx, 1)
				require.NoError(t, err)
				start := time.Now()
				got, retried, err := v.payloadAttestationDataWithRetry(ctx, 1)
				require.NotNil(t, err)
				if transport == "grpc" {
					require.Equal(t, tt.code, status.Code(err))
				} else {
					require.ErrorIs(t, err, &httputil.DefaultJsonError{Code: tt.httpCode})
				}
				require.Equal(t, (*ethpb.PayloadAttestationData)(nil), got)
				require.Equal(t, tt.outcome, payloadAttestationDataFailure(err))
				require.Equal(t, true, retried)
				require.Equal(t, true, calls.Load() >= 2)
				require.Equal(t, true, calls.Load() <= int32(payloadAttestationReadGrace/payloadAttestationPollInterval))
				require.Equal(t, true, time.Since(start) >= payloadAttestationReadGrace)
				require.NoError(t, ctx.Err())
			})
		}
	}
}

func TestPayloadAttestationDataWithRetry_RESTIndependentNodes(t *testing.T) {
	for _, transport := range []string{"rest-json", "rest-ssz"} {
		for _, tt := range []struct {
			name       string
			late       bool
			slowWinner bool
		}{
			{name: "negative vote at due with stalled peer", late: true},
			{name: "slow winner survives peer retry", slowWinner: true},
			{name: "initial fanout is not a retry"},
		} {
			t.Run(transport+"/"+tt.name, func(t *testing.T) {
				var calls, peerCalls, earlyCalls atomic.Int32
				peerStarted := make(chan struct{})
				peerCanceled := make(chan struct{}, 1)
				root := [32]byte{0xab}
				want := &ethpb.PayloadAttestationData{Slot: 1, BeaconBlockRoot: root[:], PayloadPresent: !tt.late && !tt.slowWinner, BlobDataAvailable: true}
				var due time.Time
				ready := httptest.NewServer(payloadAttestationTransportHandler(t, transport, func(ctx context.Context, _ primitives.Slot) (*ethpb.PayloadAttestationData, error) {
					calls.Add(1)
					select {
					case <-peerStarted:
					case <-ctx.Done():
						return nil, ctx.Err()
					}
					if tt.late && time.Now().Before(due) {
						earlyCalls.Add(1)
						return nil, status.Error(codes.Unavailable, "not yet final")
					}
					if tt.slowWinner {
						select {
						case <-time.After(time.Until(due)):
						case <-ctx.Done():
							return nil, ctx.Err()
						}
					}
					return want, nil
				}))
				t.Cleanup(ready.Close)
				peer := httptest.NewServer(payloadAttestationTransportHandler(t, transport, func(ctx context.Context, _ primitives.Slot) (*ethpb.PayloadAttestationData, error) {
					if peerCalls.Add(1) == 1 {
						close(peerStarted)
						if tt.slowWinner {
							return nil, status.Error(codes.Unavailable, "not yet final")
						}
					}
					<-ctx.Done()
					peerCanceled <- struct{}{}
					return nil, ctx.Err()
				}))
				t.Cleanup(peer.Close)
				provider, err := rest.NewRestConnectionProvider(ready.URL + "," + peer.URL)
				require.NoError(t, err)
				due = time.Now().Add(200 * time.Millisecond)
				v := payloadAttestationTransportValidator(beaconApi.NewBeaconApiValidatorClient(provider), due)
				v.payloadAvailability.notify(1, &root)
				ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
				defer cancel()
				ctx, err = v.withPayloadHeadHint(ctx, 1)
				require.NoError(t, err)

				got, retried, err := v.payloadAttestationDataWithRetry(ctx, 1)
				require.NoError(t, err)
				require.DeepEqual(t, want, got)
				require.Equal(t, tt.late || tt.slowWinner, retried)
				require.NoError(t, ctx.Err())
				if tt.late {
					require.Equal(t, true, earlyCalls.Load() > 0)
					require.Equal(t, true, calls.Load() >= 2)
				} else {
					require.Equal(t, int32(1), calls.Load())
				}
				wantPeerCalls := int32(1)
				if tt.slowWinner {
					wantPeerCalls = 2
				}
				require.Equal(t, wantPeerCalls, peerCalls.Load())
				select {
				case <-peerCanceled:
				case <-time.After(time.Second):
					t.Fatal("winning response did not cancel the stalled peer")
				}
			})
		}
	}
}

func TestPayloadAttestationDataWithRetry_RESTMalformedResponsePreservesFallback(t *testing.T) {
	for _, transport := range []string{"rest-json", "rest-ssz"} {
		t.Run(transport, func(t *testing.T) {
			var calls atomic.Int32
			root := [32]byte{0xaa}
			want := &ethpb.PayloadAttestationData{Slot: 1, BeaconBlockRoot: root[:], BlobDataAvailable: true}
			valid := payloadAttestationTransportHandler(t, transport, func(context.Context, primitives.Slot) (*ethpb.PayloadAttestationData, error) {
				return want, nil
			})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					valid.ServeHTTP(w, r)
					return
				}
				contentType := api.JsonMediaType
				if transport == "rest-ssz" {
					contentType = api.OctetStreamMediaType
				}
				w.Header().Set("Content-Type", contentType)
				_, _ = w.Write([]byte("malformed"))
			}))
			t.Cleanup(server.Close)
			provider, err := rest.NewRestConnectionProvider(server.URL)
			require.NoError(t, err)
			v := payloadAttestationTransportValidator(beaconApi.NewBeaconApiValidatorClient(provider), time.Now().Add(-time.Second))
			announced := [32]byte{0xbb}
			v.payloadAvailability.notify(1, &announced)
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			ctx, err = v.withPayloadHeadHint(ctx, 1)
			require.NoError(t, err)

			got, retried, err := v.payloadAttestationDataWithRetry(ctx, 1)
			require.NoError(t, err)
			require.DeepEqual(t, want, got)
			require.Equal(t, true, retried)
			require.Equal(t, true, calls.Load() >= 2)
			require.NoError(t, ctx.Err())
		})
	}
}
