package rpc

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prysmaticlabs/prysm/v5/api"
	"github.com/prysmaticlabs/prysm/v5/api/server/middleware"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/core"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/beacon"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/blob"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/builder"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/config"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/debug"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/events"
	lightclient "github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/light-client"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/node"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/rewards"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/validator"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/lookup"
	beaconprysm "github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/prysm/beacon"
	nodeprysm "github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/prysm/node"
	validatorv1alpha1 "github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/prysm/v1alpha1/validator"
	validatorprysm "github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/prysm/validator"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state/stategen"
)

type endpoint struct {
	template   string
	name       string
	middleware []middleware.Middleware
	handler    http.HandlerFunc
	methods    []string
}

func (e *endpoint) handlerWithMiddleware() http.HandlerFunc {
	handler := http.Handler(e.handler)
	for _, m := range e.middleware {
		handler = m(handler)
	}
	return promhttp.InstrumentHandlerDuration(
		httpRequestLatency.MustCurryWith(prometheus.Labels{"endpoint": e.name}),
		promhttp.InstrumentHandlerCounter(
			httpRequestCount.MustCurryWith(prometheus.Labels{"endpoint": e.name}),
			handler,
		),
	)
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
	endpoints = append(endpoints, s.lightClientEndpoints(blocker, stater)...)
	endpoints = append(endpoints, s.eventsEndpoints()...)
	endpoints = append(endpoints, s.prysmBeaconEndpoints(ch, stater, coreService)...)
	endpoints = append(endpoints, s.prysmNodeEndpoints()...)
	endpoints = append(endpoints, s.prysmValidatorEndpoints(stater, coreService)...)
	if enableDebug {
		endpoints = append(endpoints, s.debugEndpoints(stater)...)
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
			template: "/eth/v1/builder/states/{state_id}/expected_withdrawals",
			name:     namespace + ".ExpectedWithdrawals",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			handler: server.ExpectedWithdrawals,
			methods: []string{http.MethodGet},
		},
	}
}

func (*Service) blobEndpoints(blocker lookup.Blocker) []endpoint {
	server := &blob.Server{
		Blocker: blocker,
	}

	const namespace = "blob"
	return []endpoint{
		{
			template: "/eth/v1/beacon/blob_sidecars/{block_id}",
			name:     namespace + ".Blobs",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			handler: server.Blobs,
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
			template: "/eth/v1/validator/aggregate_attestation",
			name:     namespace + ".GetAggregateAttestation",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetAggregateAttestation,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/validator/contribution_and_proofs",
			name:     namespace + ".SubmitContributionAndProofs",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.SubmitContributionAndProofs,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/validator/aggregate_and_proofs",
			name:     namespace + ".SubmitAggregateAndProofs",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
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
			},
			handler: server.SubmitAggregateAndProofsV2,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/validator/sync_committee_contribution",
			name:     namespace + ".ProduceSyncCommitteeContribution",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
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
			},
			handler: server.SubmitBeaconCommitteeSubscription,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/validator/attestation_data",
			name:     namespace + ".GetAttestationData",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
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
			},
			handler: server.GetAttesterDuties,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/validator/duties/proposer/{epoch}",
			name:     namespace + ".GetProposerDuties",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
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
			},
			handler: server.GetLiveness,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v2/validator/blocks/{slot}",
			name:     namespace + ".ProduceBlockV2",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			handler: server.ProduceBlockV2,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/validator/blinded_blocks/{slot}",
			name:     namespace + ".ProduceBlindedBlock",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			handler: server.ProduceBlindedBlock,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v3/validator/blocks/{slot}",
			name:     namespace + ".ProduceBlockV3",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
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
			},
			handler: server.GetSyncStatus,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/node/identity",
			name:     namespace + ".GetIdentity",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetIdentity,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/node/peers/{peer_id}",
			name:     namespace + ".GetPeer",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetPeer,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/node/peers",
			name:     namespace + ".GetPeers",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetPeers,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/node/peer_count",
			name:     namespace + ".GetPeerCount",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetPeerCount,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/node/version",
			name:     namespace + ".GetVersion",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetVersion,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/node/health",
			name:     namespace + ".GetHealth",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
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
	}

	const namespace = "beacon"
	return []endpoint{
		{
			template: "/eth/v1/beacon/states/{state_id}/committees",
			name:     namespace + ".GetCommittees",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetCommittees,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/states/{state_id}/fork",
			name:     namespace + ".GetStateFork",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetStateFork,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/states/{state_id}/root",
			name:     namespace + ".GetStateRoot",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetStateRoot,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/states/{state_id}/sync_committees",
			name:     namespace + ".GetSyncCommittees",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetSyncCommittees,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/states/{state_id}/randao",
			name:     namespace + ".GetRandao",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetRandao,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/blocks",
			name:     namespace + ".PublishBlock",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.PublishBlock,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/beacon/blinded_blocks",
			name:     namespace + ".PublishBlindedBlock",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
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
			},
			handler: server.PublishBlindedBlockV2,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v2/beacon/blocks/{block_id}",
			name:     namespace + ".GetBlockV2",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			handler: server.GetBlockV2,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/blocks/{block_id}/attestations",
			name:     namespace + ".GetBlockAttestations",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetBlockAttestations,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v2/beacon/blocks/{block_id}/attestations",
			name:     namespace + ".GetBlockAttestationsV2",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetBlockAttestations,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/blinded_blocks/{block_id}",
			name:     namespace + ".GetBlindedBlock",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			handler: server.GetBlindedBlock,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/blocks/{block_id}/root",
			name:     namespace + ".GetBlockRoot",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetBlockRoot,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/pool/attestations",
			name:     namespace + ".ListAttestations",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.ListAttestations,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v2/beacon/pool/attestations",
			name:     namespace + ".ListAttestationsV2",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
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
			},
			handler: server.SubmitAttestations,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/beacon/pool/voluntary_exits",
			name:     namespace + ".ListVoluntaryExits",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
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
			},
			handler: server.SubmitSyncCommitteeSignatures,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/beacon/pool/bls_to_execution_changes",
			name:     namespace + ".ListBLSToExecutionChanges",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
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
			},
			handler: server.SubmitBLSToExecutionChanges,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/beacon/pool/attester_slashings",
			name:     namespace + ".GetAttesterSlashings",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetAttesterSlashings,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v2/beacon/pool/attester_slashings",
			name:     namespace + ".GetAttesterSlashingsV2",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
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
			},
			handler: server.SubmitAttesterSlashingsV2,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/beacon/pool/proposer_slashings",
			name:     namespace + ".GetProposerSlashings",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
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
			},
			handler: server.SubmitProposerSlashing,
			methods: []string{http.MethodPost},
		},
		{
			template: "/eth/v1/beacon/headers",
			name:     namespace + ".GetBlockHeaders",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetBlockHeaders,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/headers/{block_id}",
			name:     namespace + ".GetBlockHeader",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetBlockHeader,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/genesis",
			name:     namespace + ".GetGenesis",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetGenesis,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/states/{state_id}/finality_checkpoints",
			name:     namespace + ".GetFinalityCheckpoints",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
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
			},
			handler: server.GetValidators,
			methods: []string{http.MethodGet, http.MethodPost},
		},
		{
			template: "/eth/v1/beacon/states/{state_id}/validators/{validator_id}",
			name:     namespace + ".GetValidator",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
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
			},
			handler: server.GetValidatorBalances,
			methods: []string{http.MethodGet, http.MethodPost},
		},
		{
			template: "/eth/v1/beacon/deposit_snapshot",
			name:     namespace + ".GetDepositSnapshot",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetDepositSnapshot,
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
			},
			handler: config.GetDepositContract,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/config/fork_schedule",
			name:     namespace + ".GetForkSchedule",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: config.GetForkSchedule,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/config/spec",
			name:     namespace + ".GetSpec",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: config.GetSpec,
			methods: []string{http.MethodGet},
		},
	}
}

func (s *Service) lightClientEndpoints(blocker lookup.Blocker, stater lookup.Stater) []endpoint {
	server := &lightclient.Server{
		Blocker:     blocker,
		Stater:      stater,
		HeadFetcher: s.cfg.HeadFetcher,
		BeaconDB:    s.cfg.BeaconDB,
	}

	const namespace = "lightclient"
	return []endpoint{
		{
			template: "/eth/v1/beacon/light_client/bootstrap/{block_root}",
			name:     namespace + ".GetLightClientBootstrap",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			handler: server.GetLightClientBootstrap,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/light_client/updates",
			name:     namespace + ".GetLightClientUpdatesByRange",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			handler: server.GetLightClientUpdatesByRange,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/light_client/finality_update",
			name:     namespace + ".GetLightClientFinalityUpdate",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			handler: server.GetLightClientFinalityUpdate,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/light_client/optimistic_update",
			name:     namespace + ".GetLightClientOptimisticUpdate",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			handler: server.GetLightClientOptimisticUpdate,
			methods: []string{http.MethodGet},
		},
	}
}

func (s *Service) debugEndpoints(stater lookup.Stater) []endpoint {
	server := &debug.Server{
		BeaconDB:              s.cfg.BeaconDB,
		HeadFetcher:           s.cfg.HeadFetcher,
		Stater:                stater,
		OptimisticModeFetcher: s.cfg.OptimisticModeFetcher,
		ForkFetcher:           s.cfg.ForkFetcher,
		ForkchoiceFetcher:     s.cfg.ForkchoiceFetcher,
		FinalizationFetcher:   s.cfg.FinalizationFetcher,
		ChainInfoFetcher:      s.cfg.ChainInfoFetcher,
	}

	const namespace = "debug"
	return []endpoint{
		{
			template: "/eth/v2/debug/beacon/states/{state_id}",
			name:     namespace + ".GetBeaconStateV2",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			handler: server.GetBeaconStateV2,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v2/debug/beacon/heads",
			name:     namespace + ".GetForkChoiceHeadsV2",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetForkChoiceHeadsV2,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/debug/fork_choice",
			name:     namespace + ".GetForkChoice",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetForkChoice,
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
			},
			handler: server.GetWeakSubjectivity,
			methods: []string{http.MethodGet},
		},
		{
			template: "/eth/v1/beacon/states/{state_id}/validator_count",
			name:     namespace + ".GetValidatorCount",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetValidatorCount,
			methods: []string{http.MethodGet},
		},
		{
			template: "/prysm/v1/beacon/states/{state_id}/validator_count",
			name:     namespace + ".GetValidatorCount",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
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
			},
			handler: server.GetIndividualVotes,
			methods: []string{http.MethodPost},
		},
		{
			template: "/prysm/v1/beacon/chain_head",
			name:     namespace + ".GetChainHead",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetChainHead,
			methods: []string{http.MethodGet},
		},
		{
			template: "/prysm/v1/beacon/blobs",
			name:     namespace + ".PublishBlobs",
			middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.PublishBlobs,
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
			},
			handler: server.ListTrustedPeer,
			methods: []string{http.MethodGet},
		},
		{
			template: "/prysm/v1/node/trusted_peers",
			name:     namespace + ".ListTrustedPeer",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
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
			},
			handler: server.AddTrustedPeer,
			methods: []string{http.MethodPost},
		},
		{
			template: "/prysm/node/trusted_peers/{peer_id}",
			name:     namespace + ".RemoveTrustedPeer",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.RemoveTrustedPeer,
			methods: []string{http.MethodDelete},
		},
		{
			template: "/prysm/v1/node/trusted_peers/{peer_id}",
			name:     namespace + ".RemoveTrustedPeer",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
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
			},
			handler: server.GetPerformance,
			methods: []string{http.MethodPost},
		},
		{
			template: "/prysm/v1/validators/participation",
			name:     namespace + ".GetParticipation",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetParticipation,
			methods: []string{http.MethodGet},
		},
		{
			template: "/prysm/v1/validators/active_set_changes",
			name:     namespace + ".GetActiveSetChanges",
			middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			handler: server.GetActiveSetChanges,
			methods: []string{http.MethodGet},
		},
	}
}
