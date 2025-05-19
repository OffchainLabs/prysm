package rpc

import (
	"net/http"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/api/apiutil"
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
)

func (s *Service) endpoints(
	enableDebug bool,
	blocker lookup.Blocker,
	stater lookup.Stater,
	rewardFetcher rewards.BlockRewardsFetcher,
	validatorServer *validatorv1alpha1.Server,
	coreService *core.Service,
	ch *stategen.CanonicalHistory,
) []apiutil.Endpoint {
	endpoints := make([]apiutil.Endpoint, 0)
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

func (s *Service) rewardsEndpoints(blocker lookup.Blocker, stater lookup.Stater, rewardFetcher rewards.BlockRewardsFetcher) []apiutil.Endpoint {
	return rewardsEndpoints(&rewards.Server{
		Blocker:               blocker,
		OptimisticModeFetcher: s.cfg.OptimisticModeFetcher,
		FinalizationFetcher:   s.cfg.FinalizationFetcher,
		TimeFetcher:           s.cfg.GenesisTimeFetcher,
		Stater:                stater,
		HeadFetcher:           s.cfg.HeadFetcher,
		BlockRewardFetcher:    rewardFetcher,
	})
}

func rewardsEndpoints(server *rewards.Server) []apiutil.Endpoint {
	const namespace = "rewards"
	return []apiutil.Endpoint{
		{
			Template: "/eth/v1/beacon/rewards/blocks/{block_id}",
			Name:     namespace + ".BlockRewards",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.BlockRewards,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/rewards/attestations/{epoch}",
			Name:     namespace + ".AttestationRewards",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.AttestationRewards,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/eth/v1/beacon/rewards/sync_committee/{block_id}",
			Name:     namespace + ".SyncCommitteeRewards",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.SyncCommitteeRewards,
			Methods: []string{http.MethodPost},
		},
	}
}

func (s *Service) builderEndpoints(stater lookup.Stater) []apiutil.Endpoint {
	return builderEndpoints(&builder.Server{
		FinalizationFetcher:   s.cfg.FinalizationFetcher,
		OptimisticModeFetcher: s.cfg.OptimisticModeFetcher,
		Stater:                stater,
	})
}

func builderEndpoints(server *builder.Server) []apiutil.Endpoint {
	const namespace = "builder"
	return []apiutil.Endpoint{
		{
			// Deprecated: use SSE from /eth/v1/events for `Payload Attributes` instead
			Template: "/eth/v1/builder/states/{state_id}/expected_withdrawals",
			Name:     namespace + ".ExpectedWithdrawals",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			Handler: server.ExpectedWithdrawals,
			Methods: []string{http.MethodGet},
		},
	}
}

func (s *Service) blobEndpoints(blocker lookup.Blocker) []apiutil.Endpoint {
	return blobEndpoints(&blob.Server{
		Blocker:               blocker,
		OptimisticModeFetcher: s.cfg.OptimisticModeFetcher,
		FinalizationFetcher:   s.cfg.FinalizationFetcher,
		TimeFetcher:           s.cfg.GenesisTimeFetcher,
	})
}
func blobEndpoints(server *blob.Server) []apiutil.Endpoint {
	const namespace = "blob"
	return []apiutil.Endpoint{
		{
			Template: "/eth/v1/beacon/blob_sidecars/{block_id}",
			Name:     namespace + ".Blobs",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			Handler: server.Blobs,
			Methods: []string{http.MethodGet},
		},
	}
}

func (s *Service) validatorEndpoints(
	validatorServer *validatorv1alpha1.Server,
	stater lookup.Stater,
	coreService *core.Service,
	rewardFetcher rewards.BlockRewardsFetcher,
) []apiutil.Endpoint {
	return validatorEndpoints(&validator.Server{
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
	})
}

func validatorEndpoints(server *validator.Server) []apiutil.Endpoint {
	const namespace = "validator"
	return []apiutil.Endpoint{
		{
			// Deprecated: use /eth/v2/validator/aggregate_attestation instead
			Template: "/eth/v1/validator/aggregate_attestation",
			Name:     namespace + ".GetAggregateAttestation",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetAggregateAttestation,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v2/validator/aggregate_attestation",
			Name:     namespace + ".GetAggregateAttestationV2",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetAggregateAttestationV2,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/validator/contribution_and_proofs",
			Name:     namespace + ".SubmitContributionAndProofs",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.SubmitContributionAndProofs,
			Methods: []string{http.MethodPost},
		},
		{
			// Deprecated: use /eth/v2/validator/aggregate_and_proofs instead
			Template: "/eth/v1/validator/aggregate_and_proofs",
			Name:     namespace + ".SubmitAggregateAndProofs",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.SubmitAggregateAndProofs,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/eth/v2/validator/aggregate_and_proofs",
			Name:     namespace + ".SubmitAggregateAndProofsV2",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.SubmitAggregateAndProofsV2,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/eth/v1/validator/sync_committee_contribution",
			Name:     namespace + ".ProduceSyncCommitteeContribution",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.ProduceSyncCommitteeContribution,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/validator/sync_committee_subscriptions",
			Name:     namespace + ".SubmitSyncCommitteeSubscription",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.SubmitSyncCommitteeSubscription,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/eth/v1/validator/beacon_committee_subscriptions",
			Name:     namespace + ".SubmitBeaconCommitteeSubscription",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.SubmitBeaconCommitteeSubscription,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/eth/v1/validator/attestation_data",
			Name:     namespace + ".GetAttestationData",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetAttestationData,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/validator/register_validator",
			Name:     namespace + ".RegisterValidator",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.RegisterValidator,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/eth/v1/validator/duties/attester/{epoch}",
			Name:     namespace + ".GetAttesterDuties",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetAttesterDuties,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/eth/v1/validator/duties/proposer/{epoch}",
			Name:     namespace + ".GetProposerDuties",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetProposerDuties,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/validator/duties/sync/{epoch}",
			Name:     namespace + ".GetSyncCommitteeDuties",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetSyncCommitteeDuties,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/eth/v1/validator/prepare_beacon_proposer",
			Name:     namespace + ".PrepareBeaconProposer",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.PrepareBeaconProposer,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/eth/v1/validator/liveness/{epoch}",
			Name:     namespace + ".GetLiveness",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetLiveness,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/eth/v3/validator/blocks/{slot}",
			Name:     namespace + ".ProduceBlockV3",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			Handler: server.ProduceBlockV3,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/validator/beacon_committee_selections",
			Name:     namespace + ".BeaconCommitteeSelections",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
			},
			Handler: server.BeaconCommitteeSelections,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/eth/v1/validator/sync_committee_selections",
			Name:     namespace + ".SyncCommittee Selections",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
			},
			Handler: server.SyncCommitteeSelections,
			Methods: []string{http.MethodPost},
		},
	}
}

func (s *Service) nodeEndpoints() []apiutil.Endpoint {
	return nodeEndpoints(&node.Server{
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
	})
}

func nodeEndpoints(server *node.Server) []apiutil.Endpoint {
	const namespace = "node"
	return []apiutil.Endpoint{
		{
			Template: "/eth/v1/node/syncing",
			Name:     namespace + ".GetSyncStatus",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetSyncStatus,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/node/identity",
			Name:     namespace + ".GetIdentity",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetIdentity,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/node/peers/{peer_id}",
			Name:     namespace + ".GetPeer",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetPeer,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/node/peers",
			Name:     namespace + ".GetPeers",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetPeers,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/node/peer_count",
			Name:     namespace + ".GetPeerCount",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetPeerCount,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/node/version",
			Name:     namespace + ".GetVersion",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetVersion,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/node/health",
			Name:     namespace + ".GetHealth",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetHealth,
			Methods: []string{http.MethodGet},
		},
	}
}

func (s *Service) beaconEndpoints(
	ch *stategen.CanonicalHistory,
	stater lookup.Stater,
	blocker lookup.Blocker,
	validatorServer *validatorv1alpha1.Server,
	coreService *core.Service,
) []apiutil.Endpoint {
	return beacon.Endpoints(&beacon.Server{
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
	})
}

func (*Service) configEndpoints() []apiutil.Endpoint {
	const namespace = "config"
	return []apiutil.Endpoint{
		{
			Template: "/eth/v1/config/deposit_contract",
			Name:     namespace + ".GetDepositContract",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: config.GetDepositContract,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/config/fork_schedule",
			Name:     namespace + ".GetForkSchedule",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: config.GetForkSchedule,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/config/spec",
			Name:     namespace + ".GetSpec",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: config.GetSpec,
			Methods: []string{http.MethodGet},
		},
	}
}

func (s *Service) lightClientEndpoints(blocker lookup.Blocker, stater lookup.Stater) []apiutil.Endpoint {
	return lightClientEndpoints(&lightclient.Server{
		Blocker:          blocker,
		Stater:           stater,
		HeadFetcher:      s.cfg.HeadFetcher,
		ChainInfoFetcher: s.cfg.ChainInfoFetcher,
		BeaconDB:         s.cfg.BeaconDB,
		LCStore:          s.cfg.LCStore,
	})
}

func lightClientEndpoints(server *lightclient.Server) []apiutil.Endpoint {
	const namespace = "lightclient"
	return []apiutil.Endpoint{
		{
			Template: "/eth/v1/beacon/light_client/bootstrap/{block_root}",
			Name:     namespace + ".GetLightClientBootstrap",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			Handler: server.GetLightClientBootstrap,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/light_client/updates",
			Name:     namespace + ".GetLightClientUpdatesByRange",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			Handler: server.GetLightClientUpdatesByRange,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/light_client/finality_update",
			Name:     namespace + ".GetLightClientFinalityUpdate",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			Handler: server.GetLightClientFinalityUpdate,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/light_client/optimistic_update",
			Name:     namespace + ".GetLightClientOptimisticUpdate",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			Handler: server.GetLightClientOptimisticUpdate,
			Methods: []string{http.MethodGet},
		},
	}
}

func (s *Service) debugEndpoints(stater lookup.Stater) []apiutil.Endpoint {
	return debugEndpoints(&debug.Server{
		BeaconDB:              s.cfg.BeaconDB,
		HeadFetcher:           s.cfg.HeadFetcher,
		Stater:                stater,
		OptimisticModeFetcher: s.cfg.OptimisticModeFetcher,
		ForkFetcher:           s.cfg.ForkFetcher,
		ForkchoiceFetcher:     s.cfg.ForkchoiceFetcher,
		FinalizationFetcher:   s.cfg.FinalizationFetcher,
		ChainInfoFetcher:      s.cfg.ChainInfoFetcher,
	})
}

func debugEndpoints(server *debug.Server) []apiutil.Endpoint {
	const namespace = "debug"
	return []apiutil.Endpoint{
		{
			Template: "/eth/v2/debug/beacon/states/{state_id}",
			Name:     namespace + ".GetBeaconStateV2",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			Handler: server.GetBeaconStateV2,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v2/debug/beacon/heads",
			Name:     namespace + ".GetForkChoiceHeadsV2",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetForkChoiceHeadsV2,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/debug/fork_choice",
			Name:     namespace + ".GetForkChoice",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetForkChoice,
			Methods: []string{http.MethodGet},
		},
	}
}

func (s *Service) eventsEndpoints() []apiutil.Endpoint {
	return eventsEndpoints(&events.Server{
		StateNotifier:          s.cfg.StateNotifier,
		OperationNotifier:      s.cfg.OperationNotifier,
		HeadFetcher:            s.cfg.HeadFetcher,
		ChainInfoFetcher:       s.cfg.ChainInfoFetcher,
		TrackedValidatorsCache: s.cfg.TrackedValidatorsCache,
		StateGen:               s.cfg.StateGen,
	})
}

func eventsEndpoints(server *events.Server) []apiutil.Endpoint {
	const namespace = "events"
	return []apiutil.Endpoint{
		{
			Template: "/eth/v1/events",
			Name:     namespace + ".StreamEvents",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.EventStreamMediaType}),
			},
			Handler: server.StreamEvents,
			Methods: []string{http.MethodGet},
		},
	}
}

// Prysm custom endpoints
func (s *Service) prysmBeaconEndpoints(
	ch *stategen.CanonicalHistory,
	stater lookup.Stater,
	coreService *core.Service,
) []apiutil.Endpoint {
	return prysmBeaconEndpoints(&beaconprysm.Server{
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
	})
}

func prysmBeaconEndpoints(server *beaconprysm.Server) []apiutil.Endpoint {
	const namespace = "prysm.beacon"
	return []apiutil.Endpoint{
		{
			Template: "/prysm/v1/beacon/weak_subjectivity",
			Name:     namespace + ".GetWeakSubjectivity",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetWeakSubjectivity,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/states/{state_id}/validator_count",
			Name:     namespace + ".GetValidatorCount",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetValidatorCount,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/prysm/v1/beacon/states/{state_id}/validator_count",
			Name:     namespace + ".GetValidatorCount",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetValidatorCount,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/prysm/v1/beacon/individual_votes",
			Name:     namespace + ".GetIndividualVotes",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetIndividualVotes,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/prysm/v1/beacon/chain_head",
			Name:     namespace + ".GetChainHead",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetChainHead,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/prysm/v1/beacon/blobs",
			Name:     namespace + ".PublishBlobs",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.PublishBlobs,
			Methods: []string{http.MethodPost},
		},
	}
}

func (s *Service) prysmNodeEndpoints() []apiutil.Endpoint {
	return prysmNodeEndpoints(&nodeprysm.Server{
		BeaconDB:                  s.cfg.BeaconDB,
		SyncChecker:               s.cfg.SyncService,
		OptimisticModeFetcher:     s.cfg.OptimisticModeFetcher,
		GenesisTimeFetcher:        s.cfg.GenesisTimeFetcher,
		PeersFetcher:              s.cfg.PeersFetcher,
		PeerManager:               s.cfg.PeerManager,
		MetadataProvider:          s.cfg.MetadataProvider,
		HeadFetcher:               s.cfg.HeadFetcher,
		ExecutionChainInfoFetcher: s.cfg.ExecutionChainInfoFetcher,
	})
}

func prysmNodeEndpoints(server *nodeprysm.Server) []apiutil.Endpoint {
	const namespace = "prysm.node"
	return []apiutil.Endpoint{
		{
			Template: "/prysm/node/trusted_peers",
			Name:     namespace + ".ListTrustedPeer",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.ListTrustedPeer,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/prysm/v1/node/trusted_peers",
			Name:     namespace + ".ListTrustedPeer",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.ListTrustedPeer,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/prysm/node/trusted_peers",
			Name:     namespace + ".AddTrustedPeer",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.AddTrustedPeer,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/prysm/v1/node/trusted_peers",
			Name:     namespace + ".AddTrustedPeer",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.AddTrustedPeer,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/prysm/node/trusted_peers/{peer_id}",
			Name:     namespace + ".RemoveTrustedPeer",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.RemoveTrustedPeer,
			Methods: []string{http.MethodDelete},
		},
		{
			Template: "/prysm/v1/node/trusted_peers/{peer_id}",
			Name:     namespace + ".RemoveTrustedPeer",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.RemoveTrustedPeer,
			Methods: []string{http.MethodDelete},
		},
	}
}

func (s *Service) prysmValidatorEndpoints(stater lookup.Stater, coreService *core.Service) []apiutil.Endpoint {
	return prysmValidatorEndpoints(&validatorprysm.Server{
		ChainInfoFetcher: s.cfg.ChainInfoFetcher,
		Stater:           stater,
		CoreService:      coreService,
	})
}

func prysmValidatorEndpoints(server *validatorprysm.Server) []apiutil.Endpoint {
	const namespace = "prysm.validator"
	return []apiutil.Endpoint{
		{
			Template: "/prysm/validators/performance",
			Name:     namespace + ".GetPerformance",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetPerformance,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/prysm/v1/validators/performance",
			Name:     namespace + ".GetPerformance",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetPerformance,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/prysm/v1/validators/{state_id}/participation",
			Name:     namespace + ".GetParticipation",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetParticipation,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/prysm/v1/validators/{state_id}/active_set_changes",
			Name:     namespace + ".GetActiveSetChanges",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetActiveSetChanges,
			Methods: []string{http.MethodGet},
		},
	}
}
