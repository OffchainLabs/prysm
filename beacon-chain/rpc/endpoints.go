package rpc

import (
	"net/http"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/api/server/middleware"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/beacon"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/blob"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/builder"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/config"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/debug"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/events"
	lightclient "github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/light-client"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/node"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/rewards"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/eth/validator"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/lookup"
	beaconprysm "github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/prysm/beacon"
	nodeprysm "github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/prysm/node"
	validatorv1alpha1 "github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/prysm/v1alpha1/validator"
	validatorprysm "github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/prysm/validator"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type endpoint struct {
	template   string
	name       string
	middleware []middleware.Middleware
	handler    http.HandlerFunc
	methods    []string
}

// responseWriter is the wrapper to http Response writer.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader wraps the WriteHeader method of the underlying http.ResponseWriter to capture the status code.
// Refer for WriteHeader doc: https://pkg.go.dev/net/http@go1.23.3#ResponseWriter.
func (w *responseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (e *endpoint) handlerWithMiddleware() http.HandlerFunc {
	handler := http.Handler(e.handler)
	for _, m := range e.middleware {
		handler = m(handler)
	}

	handler = promhttp.InstrumentHandlerDuration(
		httpRequestLatency.MustCurryWith(prometheus.Labels{"endpoint": e.name}),
		promhttp.InstrumentHandlerCounter(
			httpRequestCount.MustCurryWith(prometheus.Labels{"endpoint": e.name}),
			handler,
		),
	)

	return func(w http.ResponseWriter, r *http.Request) {
		// SSE errors are handled separately to avoid interference with the streaming
		// mechanism and ensure accurate error tracking.
		if e.template == "/eth/v1/events" {
			handler.ServeHTTP(w, r)
			return
		}

		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		handler.ServeHTTP(rw, r)

		if rw.statusCode >= 400 {
			httpErrorCount.WithLabelValues(r.URL.Path, http.StatusText(rw.statusCode), r.Method).Inc()
		}
	}
}

func (s *Service) endpoints(
	enableDebug bool,
	blocker lookup.Blocker,
	stater lookup.Stater,
	rewardFetcher rewards.BlockRewardsFetcher,
	validatorServer *validatorv1alpha1.Server,
	coreService *core.Service,
	ch *stategen.CanonicalHistory,
) []endpoint {
	endpoints := make([]endpoint, 0)
	endpoints = append(endpoints, s.rewardsEndpoints(blocker, stater, rewardFetcher)...)
	endpoints = append(endpoints, s.builderEndpoints(stater)...)
	endpoints = append(endpoints, s.blobEndpoints(blocker)...)
	endpoints = append(endpoints, s.validatorEndpoints(validatorServer, stater, coreService, rewardFetcher)...)
	endpoints = append(endpoints, s.nodeEndpoints()...)
	endpoints = append(endpoints, s.beaconEndpoints(ch, stater, blocker, validatorServer, coreService)...)
	endpoints = append(endpoints, s.configEndpoints()...)
	endpoints = append(endpoints, s.eventsEndpoints()...)
	endpoints = append(endpoints, s.prysmBeaconEndpoints(ch, stater, blocker, coreService)...)
	endpoints = append(endpoints, s.prysmNodeEndpoints()...)
	endpoints = append(endpoints, s.prysmValidatorEndpoints(stater, coreService)...)

	if features.Get().EnableLightClient {
		endpoints = append(endpoints, s.lightClientEndpoints()...)
	}

	if enableDebug {
		endpoints = append(endpoints, s.debugEndpoints(stater, blocker)...)
	}

	return endpoints
}

func (s *Service) rewardsEndpoints(blocker lookup.Blocker, stater lookup.Stater, rewardFetcher rewards.BlockRewardsFetcher) []endpoint {
	server := &rewards.Server{
		Blocker:               blocker,
		OptimisticModeFetcher: s.cfg.OptimisticModeFetcher,
		FinalizationFetcher:   s.cfg.FinalizationFetcher,
		TimeFetcher:           s.cfg.GenesisTimeFetcher,
		Stater:                stater,
		HeadFetcher:           s.cfg.HeadFetcher,
		BlockRewardFetcher:    rewardFetcher,
	}

	const namespace = "rewards"
	return []endpoint{
		{
			template: "/eth/v1/beacon/rewards/blocks/{block_id}",
			name:     namespace + ".BlockRewards",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.BlockRewards,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/rewards/attestations/{epoch}",
			name:     namespace + ".AttestationRewards",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.AttestationRewards,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/beacon/rewards/sync_committee/{block_id}",
			name:     namespace + ".SyncCommitteeRewards",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.SyncCommitteeRewards,
			methods: []string{http.MethodPost},
		},
	}
}

func (s *Service) builderEndpoints(stater lookup.Stater) []endpoint {
	server := &builder.Server{
		FinalizationFetcher:   s.cfg.FinalizationFetcher,
		OptimisticModeFetcher: s.cfg.OptimisticModeFetcher,
		Stater:                stater,
	}

	const namespace = "builder"
	return []endpoint{
		{
			// Deprecated: use SSE from /eth/v1/events for `Payload Attributes` instead
			template: "/eth/v1/builder/states/{state_id}/expected_withdrawals",
			name:     namespace + ".ExpectedWithdrawals",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.ExpectedWithdrawals,
			methods: []string{http.MethodGet},
		},
	}
}

func (s *Service) blobEndpoints(blocker lookup.Blocker) []endpoint {
	server := &blob.Server{
		Blocker:               blocker,
		OptimisticModeFetcher: s.cfg.OptimisticModeFetcher,
		FinalizationFetcher:   s.cfg.FinalizationFetcher,
		TimeFetcher:           s.cfg.GenesisTimeFetcher,
	}

	const namespace = "blob"
	return []endpoint{
		{
			// Deprecated: /eth/v1/beacon/blob_sidecars/{block_id} in favor of /eth/v1/beacon/blobs/{block_id}
			// the endpoint will continue to work post fulu for some time however
			template: "/eth/v1/beacon/blob_sidecars/{block_id}",
			name:     namespace + ".Blobs",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.Blobs,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/blobs/{block_id}",
			name:     namespace + ".GetBlobs",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetBlobs,
			methods: []string{http.MethodGet},
		},
	}
}

func (s *Service) validatorEndpoints(
	validatorServer *validatorv1alpha1.Server,
	stater lookup.Stater,
	coreService *core.Service,
	rewardFetcher rewards.BlockRewardsFetcher,
) []endpoint {
	server := &validator.Server{
		HeadFetcher:            s.cfg.HeadFetcher,
		TimeFetcher:            s.cfg.GenesisTimeFetcher,
		SyncChecker:            s.cfg.SyncService,
		OptimisticModeFetcher:  s.cfg.OptimisticModeFetcher,
		AttestationCache:       s.cfg.AttestationCache,
		AttestationsPool:       s.cfg.AttestationsPool,
		PeerManager:            s.cfg.PeerManager,
		Broadcaster:            s.cfg.Broadcaster,
		V1Alpha1Server:         validatorServer,
		Stater:                 stater,
		SyncCommitteePool:      s.cfg.SyncCommitteeObjectPool,
		ChainInfoFetcher:       s.cfg.ChainInfoFetcher,
		BeaconDB:               s.cfg.BeaconDB,
		BlockBuilder:           s.cfg.BlockBuilder,
		OperationNotifier:      s.cfg.OperationNotifier,
		TrackedValidatorsCache: s.cfg.TrackedValidatorsCache,
		PayloadIDCache:         s.cfg.PayloadIDCache,
		CoreService:            coreService,
		BlockRewardFetcher:     rewardFetcher,
	}

	const namespace = "validator"
	return []endpoint{
		{
			// Deprecated: use /eth/v2/validator/aggregate_attestation instead
			template: "/eth/v1/validator/aggregate_attestation",
			name:     namespace + ".GetAggregateAttestation",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetAggregateAttestation,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v2/validator/aggregate_attestation",
			name:     namespace + ".GetAggregateAttestationV2",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetAggregateAttestationV2,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/validator/contribution_and_proofs",
			name:     namespace + ".SubmitContributionAndProofs",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.SubmitContributionAndProofs,
			methods: []string{http.MethodPost},
		},
		{
			// Deprecated: use /eth/v2/validator/aggregate_and_proofs instead
			template: "/eth/v1/validator/aggregate_and_proofs",
			name:     namespace + ".SubmitAggregateAndProofs",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.SubmitAggregateAndProofs,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v2/validator/aggregate_and_proofs",
			name:     namespace + ".SubmitAggregateAndProofsV2",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.SubmitAggregateAndProofsV2,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/validator/sync_committee_contribution",
			name:     namespace + ".ProduceSyncCommitteeContribution",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.ProduceSyncCommitteeContribution,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/validator/sync_committee_subscriptions",
			name:     namespace + ".SubmitSyncCommitteeSubscription",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.SubmitSyncCommitteeSubscription,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/validator/beacon_committee_subscriptions",
			name:     namespace + ".SubmitBeaconCommitteeSubscription",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.SubmitBeaconCommitteeSubscription,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/validator/attestation_data",
			name:     namespace + ".GetAttestationData",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetAttestationData,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/validator/register_validator",
			name:     namespace + ".RegisterValidator",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.RegisterValidator,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/validator/duties/attester/{epoch}",
			name:     namespace + ".GetAttesterDuties",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetAttesterDuties,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/validator/duties/proposer/{epoch}",
			name:     namespace + ".GetProposerDuties",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetProposerDuties,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/validator/duties/sync/{epoch}",
			name:     namespace + ".GetSyncCommitteeDuties",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetSyncCommitteeDuties,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/validator/prepare_beacon_proposer",
			name:     namespace + ".PrepareBeaconProposer",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.PrepareBeaconProposer,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/validator/liveness/{epoch}",
			name:     namespace + ".GetLiveness",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetLiveness,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v3/validator/blocks/{slot}",
			name:     namespace + ".ProduceBlockV3",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.ProduceBlockV3,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/validator/beacon_committee_selections",
			name:     namespace + ".BeaconCommitteeSelections",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
			},
			handler: server.BeaconCommitteeSelections,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/validator/sync_committee_selections",
			name:     namespace + ".SyncCommittee Selections",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
			},
			handler: server.SyncCommitteeSelections,
			methods: []string{http.MethodPost},
		},
	}
}

func (s *Service) nodeEndpoints() []endpoint {
	server := &node.Server{
		BeaconDB:                  s.cfg.BeaconDB,
		Server:                    s.grpcServer,
		SyncChecker:               s.cfg.SyncService,
		OptimisticModeFetcher:     s.cfg.OptimisticModeFetcher,
		GenesisTimeFetcher:        s.cfg.GenesisTimeFetcher,
		PeersFetcher:              s.cfg.PeersFetcher,
		PeerManager:               s.cfg.PeerManager,
		MetadataProvider:          s.cfg.MetadataProvider,
		HeadFetcher:               s.cfg.HeadFetcher,
		ExecutionChainInfoFetcher: s.cfg.ExecutionChainInfoFetcher,
	}

	const namespace = "node"
	return []endpoint{
		{
			template: "/eth/v1/node/syncing",
			name:     namespace + ".GetSyncStatus",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetSyncStatus,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/node/identity",
			name:     namespace + ".GetIdentity",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetIdentity,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/node/peers/{peer_id}",
			name:     namespace + ".GetPeer",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetPeer,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/node/peers",
			name:     namespace + ".GetPeers",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetPeers,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/node/peer_count",
			name:     namespace + ".GetPeerCount",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetPeerCount,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/node/version",
			name:     namespace + ".GetVersion",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetVersion,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/node/health",
			name:     namespace + ".GetHealth",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetHealth,
			methods: []string{http.MethodGet},
		},
	}
}

func (s *Service) beaconEndpoints(
	ch *stategen.CanonicalHistory,
	stater lookup.Stater,
	blocker lookup.Blocker,
	validatorServer *validatorv1alpha1.Server,
	coreService *core.Service,
) []endpoint {
	server := &beacon.Server{
		CanonicalHistory:        ch,
		BeaconDB:                s.cfg.BeaconDB,
		AttestationCache:        s.cfg.AttestationCache,
		AttestationsPool:        s.cfg.AttestationsPool,
		SlashingsPool:           s.cfg.SlashingsPool,
		ChainInfoFetcher:        s.cfg.ChainInfoFetcher,
		GenesisTimeFetcher:      s.cfg.GenesisTimeFetcher,
		BlockNotifier:           s.cfg.BlockNotifier,
		OperationNotifier:       s.cfg.OperationNotifier,
		Broadcaster:             s.cfg.Broadcaster,
		BlockReceiver:           s.cfg.BlockReceiver,
		StateGenService:         s.cfg.StateGen,
		Stater:                  stater,
		Blocker:                 blocker,
		OptimisticModeFetcher:   s.cfg.OptimisticModeFetcher,
		HeadFetcher:             s.cfg.HeadFetcher,
		TimeFetcher:             s.cfg.GenesisTimeFetcher,
		VoluntaryExitsPool:      s.cfg.ExitPool,
		V1Alpha1ValidatorServer: validatorServer,
		SyncChecker:             s.cfg.SyncService,
		ExecutionReconstructor:  s.cfg.ExecutionReconstructor,
		BLSChangesPool:          s.cfg.BLSChangesPool,
		FinalizationFetcher:     s.cfg.FinalizationFetcher,
		ForkchoiceFetcher:       s.cfg.ForkchoiceFetcher,
		CoreService:             coreService,
		AttestationStateFetcher: s.cfg.AttestationReceiver,
	}

	const namespace = "beacon"
	return []endpoint{
		{
			template: "/eth/v1/beacon/states/{state_id}/committees",
			name:     namespace + ".GetCommittees",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetCommittees,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/states/{state_id}/fork",
			name:     namespace + ".GetStateFork",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetStateFork,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/states/{state_id}/root",
			name:     namespace + ".GetStateRoot",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetStateRoot,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/states/{state_id}/sync_committees",
			name:     namespace + ".GetSyncCommittees",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetSyncCommittees,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/states/{state_id}/randao",
			name:     namespace + ".GetRandao",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetRandao,
			methods: []string{http.MethodGet},
		},
		{
			// Deprecated: use /eth/v2/beacon/blocks instead
			template: "/eth/v1/beacon/blocks",
			name:     namespace + ".PublishBlock",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.PublishBlock,
			methods: []string{http.MethodPost},
		},
		{
			// Deprecated: use /eth/v2/beacon/blinded_blocks instead
			template: "/eth/v1/beacon/blinded_blocks",
			name:     namespace + ".PublishBlindedBlock",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.PublishBlindedBlock,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v2/beacon/blocks",
			name:     namespace + ".PublishBlockV2",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.PublishBlockV2,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v2/beacon/blinded_blocks",
			name:     namespace + ".PublishBlindedBlockV2",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.PublishBlindedBlockV2,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v2/beacon/blocks/{block_id}",
			name:     namespace + ".GetBlockV2",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetBlockV2,
			methods: []string{http.MethodGet},
		},
		{
			// Deprecated: use /eth/v2/beacon/blocks/{block_id}/attestations instead
			template: "/eth/v1/beacon/blocks/{block_id}/attestations",
			name:     namespace + ".GetBlockAttestations",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetBlockAttestations,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v2/beacon/blocks/{block_id}/attestations",
			name:     namespace + ".GetBlockAttestationsV2",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetBlockAttestationsV2,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/blinded_blocks/{block_id}",
			name:     namespace + ".GetBlindedBlock",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetBlindedBlock,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/blocks/{block_id}/root",
			name:     namespace + ".GetBlockRoot",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetBlockRoot,
			methods: []string{http.MethodGet},
		},
		{
			// Deprecated: use /eth/v2/beacon/pool/attestations instead
			template: "/eth/v1/beacon/pool/attestations",
			name:     namespace + ".ListAttestations",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.ListAttestations,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v2/beacon/pool/attestations",
			name:     namespace + ".ListAttestationsV2",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.ListAttestationsV2,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/pool/attestations",
			name:     namespace + ".SubmitAttestations",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.SubmitAttestations,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v2/beacon/pool/attestations",
			name:     namespace + ".SubmitAttestationsV2",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.SubmitAttestationsV2,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/beacon/pool/voluntary_exits",
			name:     namespace + ".ListVoluntaryExits",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.ListVoluntaryExits,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/pool/voluntary_exits",
			name:     namespace + ".SubmitVoluntaryExit",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.SubmitVoluntaryExit,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/beacon/pool/sync_committees",
			name:     namespace + ".SubmitSyncCommitteeSignatures",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.SubmitSyncCommitteeSignatures,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/beacon/pool/bls_to_execution_changes",
			name:     namespace + ".ListBLSToExecutionChanges",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.ListBLSToExecutionChanges,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/pool/bls_to_execution_changes",
			name:     namespace + ".SubmitBLSToExecutionChanges",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.SubmitBLSToExecutionChanges,
			methods: []string{http.MethodPost},
		},
		{
			// Deprecated: use /eth/v2/beacon/pool/attester_slashings instead
			template: "/eth/v1/beacon/pool/attester_slashings",
			name:     namespace + ".GetAttesterSlashings",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetAttesterSlashings,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v2/beacon/pool/attester_slashings",
			name:     namespace + ".GetAttesterSlashingsV2",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetAttesterSlashingsV2,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/pool/attester_slashings",
			name:     namespace + ".SubmitAttesterSlashings",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.SubmitAttesterSlashings,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v2/beacon/pool/attester_slashings",
			name:     namespace + ".SubmitAttesterSlashingsV2",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.SubmitAttesterSlashingsV2,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/beacon/pool/proposer_slashings",
			name:     namespace + ".GetProposerSlashings",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetProposerSlashings,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/pool/proposer_slashings",
			name:     namespace + ".SubmitProposerSlashing",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.SubmitProposerSlashing,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/beacon/headers",
			name:     namespace + ".GetBlockHeaders",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetBlockHeaders,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/headers/{block_id}",
			name:     namespace + ".GetBlockHeader",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetBlockHeader,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/genesis",
			name:     namespace + ".GetGenesis",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetGenesis,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/states/{state_id}/finality_checkpoints",
			name:     namespace + ".GetFinalityCheckpoints",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetFinalityCheckpoints,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/states/{state_id}/validators",
			name:     namespace + ".GetValidators",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetValidators,
			methods: []string{http.MethodGet, http.MethodPost},
		},
		{
			template: "/eth/v1/beacon/states/{state_id}/validators/{validator_id}",
			name:     namespace + ".GetValidator",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetValidator,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/states/{state_id}/validator_balances",
			name:     namespace + ".GetValidatorBalances",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetValidatorBalances,
			methods: []string{http.MethodGet, http.MethodPost},
		},
		{
			template: "/eth/v1/beacon/states/{state_id}/validator_identities",
			name:     namespace + ".GetValidatorIdentities",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			handler: server.GetValidatorIdentities,
			methods: []string{http.MethodPost},
		},
		{
			// Deprecated: no longer needed post Electra
			template: "/eth/v1/beacon/deposit_snapshot",
			name:     namespace + ".GetDepositSnapshot",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetDepositSnapshot,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/states/{state_id}/pending_deposits",
			name:     namespace + ".GetPendingDeposits",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetPendingDeposits,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/states/{state_id}/pending_consolidations",
			name:     namespace + ".GetPendingConsolidations",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			handler: server.GetPendingConsolidations,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/states/{state_id}/pending_partial_withdrawals",
			name:     namespace + ".GetPendingPartialWithdrawals",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetPendingPartialWithdrawals,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/states/{state_id}/proposer_lookahead",
			name:     namespace + ".GetProposerLookahead",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetProposerLookahead,
			methods: []string{http.MethodGet},
		},
	}
}

func (*Service) configEndpoints() []endpoint {
	const namespace = "config"
	return []endpoint{
		{
			template: "/eth/v1/config/deposit_contract",
			name:     namespace + ".GetDepositContract",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: config.GetDepositContract,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/config/fork_schedule",
			name:     namespace + ".GetForkSchedule",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: config.GetForkSchedule,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/config/spec",
			name:     namespace + ".GetSpec",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: config.GetSpec,
			methods: []string{http.MethodGet},
		},
	}
}

func (s *Service) lightClientEndpoints() []endpoint {
	server := &lightclient.Server{
		LCStore:     s.cfg.LCStore,
		HeadFetcher: s.cfg.HeadFetcher,
	}

	const namespace = "lightclient"
	return []endpoint{
		{
			template: "/eth/v1/beacon/light_client/bootstrap/{block_root}",
			name:     namespace + ".GetLightClientBootstrap",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetLightClientBootstrap,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/light_client/updates",
			name:     namespace + ".GetLightClientUpdatesByRange",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetLightClientUpdatesByRange,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/light_client/finality_update",
			name:     namespace + ".GetLightClientFinalityUpdate",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetLightClientFinalityUpdate,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/light_client/optimistic_update",
			name:     namespace + ".GetLightClientOptimisticUpdate",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetLightClientOptimisticUpdate,
			methods: []string{http.MethodGet},
		},
	}
}

func (s *Service) debugEndpoints(stater lookup.Stater, blocker lookup.Blocker) []endpoint {
	server := &debug.Server{
		BeaconDB:              s.cfg.BeaconDB,
		HeadFetcher:           s.cfg.HeadFetcher,
		Stater:                stater,
		OptimisticModeFetcher: s.cfg.OptimisticModeFetcher,
		ForkFetcher:           s.cfg.ForkFetcher,
		ForkchoiceFetcher:     s.cfg.ForkchoiceFetcher,
		FinalizationFetcher:   s.cfg.FinalizationFetcher,
		ChainInfoFetcher:      s.cfg.ChainInfoFetcher,
		GenesisTimeFetcher:    s.cfg.GenesisTimeFetcher,
		Blocker:               blocker,
	}

	const namespace = "debug"
	return []endpoint{
		{
			template: "/eth/v2/debug/beacon/states/{state_id}",
			name:     namespace + ".GetBeaconStateV2",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetBeaconStateV2,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v2/debug/beacon/heads",
			name:     namespace + ".GetForkChoiceHeadsV2",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetForkChoiceHeadsV2,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/debug/fork_choice",
			name:     namespace + ".GetForkChoice",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetForkChoice,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/debug/beacon/data_column_sidecars/{block_id}",
			name:     namespace + ".GetDataColumnSidecars",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.DataColumnSidecars,
			methods: []string{http.MethodGet},
		},
	}
}

func (s *Service) eventsEndpoints() []endpoint {
	server := &events.Server{
		StateNotifier:          s.cfg.StateNotifier,
		OperationNotifier:      s.cfg.OperationNotifier,
		HeadFetcher:            s.cfg.HeadFetcher,
		ChainInfoFetcher:       s.cfg.ChainInfoFetcher,
		TrackedValidatorsCache: s.cfg.TrackedValidatorsCache,
		StateGen:               s.cfg.StateGen,
	}

	const namespace = "events"
	return []endpoint{
		{
			template: "/eth/v1/events",
			name:     namespace + ".StreamEvents",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.EventStreamMediaType}),
			},
			handler: server.StreamEvents,
			methods: []string{http.MethodGet},
		},
	}
}

// Prysm custom endpoints
func (s *Service) prysmBeaconEndpoints(
	ch *stategen.CanonicalHistory,
	stater lookup.Stater,
	blocker lookup.Blocker,
	coreService *core.Service,
) []endpoint {
	server := &beaconprysm.Server{
		SyncChecker:           s.cfg.SyncService,
		HeadFetcher:           s.cfg.HeadFetcher,
		TimeFetcher:           s.cfg.GenesisTimeFetcher,
		OptimisticModeFetcher: s.cfg.OptimisticModeFetcher,
		CanonicalHistory:      ch,
		BeaconDB:              s.cfg.BeaconDB,
		Stater:                stater,
		Blocker:               blocker,
		ChainInfoFetcher:      s.cfg.ChainInfoFetcher,
		FinalizationFetcher:   s.cfg.FinalizationFetcher,
		CoreService:           coreService,
		Broadcaster:           s.cfg.Broadcaster,
		BlobReceiver:          s.cfg.BlobReceiver,
	}

	const namespace = "prysm.beacon"
	return []endpoint{
		{
			template: "/prysm/v1/beacon/weak_subjectivity",
			name:     namespace + ".GetWeakSubjectivity",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetWeakSubjectivity,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/states/{state_id}/validator_count",
			name:     namespace + ".GetValidatorCount",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetValidatorCount,
			methods: []string{http.MethodGet},
		},
		{
			template: "/prysm/v1/beacon/states/{state_id}/validator_count",
			name:     namespace + ".GetValidatorCount",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetValidatorCount,
			methods: []string{http.MethodGet},
		},
		{
			template: "/prysm/v1/beacon/individual_votes",
			name:     namespace + ".GetIndividualVotes",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetIndividualVotes,
			methods: []string{http.MethodPost},
		},
		{
			template: "/prysm/v1/beacon/chain_head",
			name:     namespace + ".GetChainHead",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetChainHead,
			methods: []string{http.MethodGet},
		},
		{
			// Warning: no longer supported post Fulu fork
			template: "/prysm/v1/beacon/blobs",
			name:     namespace + ".PublishBlobs",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.PublishBlobs,
			methods: []string{http.MethodPost},
		},
		{
			template: "/prysm/v1/beacon/states/{state_id}/query",
			name:     namespace + ".QueryBeaconState",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.OctetStreamMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.QueryBeaconState,
			methods: []string{http.MethodPost},
		},
		{
			template: "/prysm/v1/beacon/blocks/{block_id}/query",
			name:     namespace + ".QueryBeaconBlock",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.OctetStreamMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.QueryBeaconBlock,
			methods: []string{http.MethodPost},
		},
	}
}

func (s *Service) prysmNodeEndpoints() []endpoint {
	server := &nodeprysm.Server{
		BeaconDB:                  s.cfg.BeaconDB,
		SyncChecker:               s.cfg.SyncService,
		OptimisticModeFetcher:     s.cfg.OptimisticModeFetcher,
		GenesisTimeFetcher:        s.cfg.GenesisTimeFetcher,
		PeersFetcher:              s.cfg.PeersFetcher,
		PeerManager:               s.cfg.PeerManager,
		MetadataProvider:          s.cfg.MetadataProvider,
		HeadFetcher:               s.cfg.HeadFetcher,
		ExecutionChainInfoFetcher: s.cfg.ExecutionChainInfoFetcher,
	}

	const namespace = "prysm.node"
	return []endpoint{
		{
			template: "/prysm/node/trusted_peers",
			name:     namespace + ".ListTrustedPeer",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.ListTrustedPeer,
			methods: []string{http.MethodGet},
		},
		{
			template: "/prysm/v1/node/trusted_peers",
			name:     namespace + ".ListTrustedPeer",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.ListTrustedPeer,
			methods: []string{http.MethodGet},
		},
		{
			template: "/prysm/node/trusted_peers",
			name:     namespace + ".AddTrustedPeer",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.AddTrustedPeer,
			methods: []string{http.MethodPost},
		},
		{
			template: "/prysm/v1/node/trusted_peers",
			name:     namespace + ".AddTrustedPeer",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.AddTrustedPeer,
			methods: []string{http.MethodPost},
		},
		{
			template: "/prysm/node/trusted_peers/{peer_id}",
			name:     namespace + ".RemoveTrustedPeer",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.RemoveTrustedPeer,
			methods: []string{http.MethodDelete},
		},
		{
			template: "/prysm/v1/node/trusted_peers/{peer_id}",
			name:     namespace + ".RemoveTrustedPeer",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.RemoveTrustedPeer,
			methods: []string{http.MethodDelete},
		},
	}
}

func (s *Service) prysmValidatorEndpoints(stater lookup.Stater, coreService *core.Service) []endpoint {
	server := &validatorprysm.Server{
		ChainInfoFetcher: s.cfg.ChainInfoFetcher,
		Stater:           stater,
		CoreService:      coreService,
	}

	const namespace = "prysm.validator"
	return []endpoint{
		{
			template: "/prysm/validators/performance",
			name:     namespace + ".GetPerformance",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetPerformance,
			methods: []string{http.MethodPost},
		},
		{
			template: "/prysm/v1/validators/performance",
			name:     namespace + ".GetPerformance",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetPerformance,
			methods: []string{http.MethodPost},
		},
		{
			template: "/prysm/v1/validators/{state_id}/participation",
			name:     namespace + ".GetParticipation",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetParticipation,
			methods: []string{http.MethodGet},
		},
		{
			template: "/prysm/v1/validators/{state_id}/active_set_changes",
			name:     namespace + ".GetActiveSetChanges",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
				middleware.AcceptEncodingHeaderHandler(),
			},
			handler: server.GetActiveSetChanges,
			methods: []string{http.MethodGet},
		},
	}
}
