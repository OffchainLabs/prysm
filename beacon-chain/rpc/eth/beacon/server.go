// Package beacon defines a gRPC beacon service implementation,
// following the official API standards https://ethereum.github.io/beacon-apis/#/.
// This package includes the beacon and config endpoints.
package beacon

import (
	"net/http"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/api/apiutil"
	"github.com/OffchainLabs/prysm/v6/api/server/middleware"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	blockfeed "github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/block"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/execution"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/blstoexec"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/slashings"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/voluntaryexits"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/rpc/lookup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
)

// Server defines a server implementation of the gRPC Beacon Chain service,
// providing RPC endpoints to access data relevant to the Ethereum Beacon Chain.
type Server struct {
	BeaconDB                db.ReadOnlyDatabase
	ChainInfoFetcher        blockchain.ChainInfoFetcher
	GenesisTimeFetcher      blockchain.TimeFetcher
	BlockReceiver           blockchain.BlockReceiver
	BlockNotifier           blockfeed.Notifier
	OperationNotifier       operation.Notifier
	Broadcaster             p2p.Broadcaster
	AttestationCache        *cache.AttestationCache
	AttestationsPool        attestations.Pool
	SlashingsPool           slashings.PoolManager
	VoluntaryExitsPool      voluntaryexits.PoolManager
	StateGenService         stategen.StateManager
	Stater                  lookup.Stater
	Blocker                 lookup.Blocker
	HeadFetcher             blockchain.HeadFetcher
	TimeFetcher             blockchain.TimeFetcher
	OptimisticModeFetcher   blockchain.OptimisticModeFetcher
	V1Alpha1ValidatorServer eth.BeaconNodeValidatorServer
	SyncChecker             sync.Checker
	CanonicalHistory        *stategen.CanonicalHistory
	ExecutionReconstructor  execution.Reconstructor
	FinalizationFetcher     blockchain.FinalizationFetcher
	BLSChangesPool          blstoexec.PoolManager
	ForkchoiceFetcher       blockchain.ForkchoiceFetcher
	CoreService             *core.Service
	AttestationStateFetcher blockchain.AttestationStateFetcher
}

func Endpoints(server *Server) []apiutil.Endpoint {
	const namespace = "beacon"
	return []apiutil.Endpoint{
		{
			Template: "/eth/v1/beacon/states/{state_id}/committees",
			Name:     namespace + ".GetCommittees",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetCommittees,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/states/{state_id}/fork",
			Name:     namespace + ".GetStateFork",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetStateFork,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/states/{state_id}/root",
			Name:     namespace + ".GetStateRoot",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetStateRoot,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/states/{state_id}/sync_committees",
			Name:     namespace + ".GetSyncCommittees",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetSyncCommittees,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/states/{state_id}/randao",
			Name:     namespace + ".GetRandao",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetRandao,
			Methods: []string{http.MethodGet},
		},
		{
			// Deprecated: use /eth/v2/beacon/blocks instead
			Template: "/eth/v1/beacon/blocks",
			Name:     namespace + ".PublishBlock",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.PublishBlock,
			Methods: []string{http.MethodPost},
		},
		{
			// Deprecated: use /eth/v2/beacon/blinded_blocks instead
			Template: "/eth/v1/beacon/blinded_blocks",
			Name:     namespace + ".PublishBlindedBlock",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.PublishBlindedBlock,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/eth/v2/beacon/blocks",
			Name:     namespace + ".PublishBlockV2",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.PublishBlockV2,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/eth/v2/beacon/blinded_blocks",
			Name:     namespace + ".PublishBlindedBlockV2",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.PublishBlindedBlockV2,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/eth/v2/beacon/blocks/{block_id}",
			Name:     namespace + ".GetBlockV2",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			Handler: server.GetBlockV2,
			Methods: []string{http.MethodGet},
		},
		{
			// Deprecated: use /eth/v2/beacon/blocks/{block_id}/attestations instead
			Template: "/eth/v1/beacon/blocks/{block_id}/attestations",
			Name:     namespace + ".GetBlockAttestations",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetBlockAttestations,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v2/beacon/blocks/{block_id}/attestations",
			Name:     namespace + ".GetBlockAttestationsV2",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetBlockAttestationsV2,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/blinded_blocks/{block_id}",
			Name:     namespace + ".GetBlindedBlock",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			Handler: server.GetBlindedBlock,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/blocks/{block_id}/root",
			Name:     namespace + ".GetBlockRoot",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetBlockRoot,
			Methods: []string{http.MethodGet},
		},
		{
			// Deprecated: use /eth/v2/beacon/pool/attestations instead
			Template: "/eth/v1/beacon/pool/attestations",
			Name:     namespace + ".ListAttestations",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.ListAttestations,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v2/beacon/pool/attestations",
			Name:     namespace + ".ListAttestationsV2",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.ListAttestationsV2,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/pool/attestations",
			Name:     namespace + ".SubmitAttestations",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.SubmitAttestations,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/eth/v2/beacon/pool/attestations",
			Name:     namespace + ".SubmitAttestationsV2",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.SubmitAttestationsV2,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/eth/v1/beacon/pool/voluntary_exits",
			Name:     namespace + ".ListVoluntaryExits",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.ListVoluntaryExits,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/pool/voluntary_exits",
			Name:     namespace + ".SubmitVoluntaryExit",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.SubmitVoluntaryExit,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/eth/v1/beacon/pool/sync_committees",
			Name:     namespace + ".SubmitSyncCommitteeSignatures",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.SubmitSyncCommitteeSignatures,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/eth/v1/beacon/pool/bls_to_execution_changes",
			Name:     namespace + ".ListBLSToExecutionChanges",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.ListBLSToExecutionChanges,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/pool/bls_to_execution_changes",
			Name:     namespace + ".SubmitBLSToExecutionChanges",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.SubmitBLSToExecutionChanges,
			Methods: []string{http.MethodPost},
		},
		{
			// Deprecated: use /eth/v2/beacon/pool/attester_slashings instead
			Template: "/eth/v1/beacon/pool/attester_slashings",
			Name:     namespace + ".GetAttesterSlashings",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetAttesterSlashings,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v2/beacon/pool/attester_slashings",
			Name:     namespace + ".GetAttesterSlashingsV2",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetAttesterSlashingsV2,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/pool/attester_slashings",
			Name:     namespace + ".SubmitAttesterSlashings",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.SubmitAttesterSlashings,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/eth/v2/beacon/pool/attester_slashings",
			Name:     namespace + ".SubmitAttesterSlashingsV2",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.SubmitAttesterSlashingsV2,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/eth/v1/beacon/pool/proposer_slashings",
			Name:     namespace + ".GetProposerSlashings",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetProposerSlashings,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/pool/proposer_slashings",
			Name:     namespace + ".SubmitProposerSlashing",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.SubmitProposerSlashing,
			Methods: []string{http.MethodPost},
		},
		{
			Template: "/eth/v1/beacon/headers",
			Name:     namespace + ".GetBlockHeaders",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetBlockHeaders,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/headers/{block_id}",
			Name:     namespace + ".GetBlockHeader",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetBlockHeader,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/genesis",
			Name:     namespace + ".GetGenesis",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetGenesis,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/states/{state_id}/finality_checkpoints",
			Name:     namespace + ".GetFinalityCheckpoints",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetFinalityCheckpoints,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/states/{state_id}/validators",
			Name:     namespace + ".GetValidators",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetValidators,
			Methods: []string{http.MethodGet, http.MethodPost},
		},
		{
			Template: "/eth/v1/beacon/states/{state_id}/validators/{validator_id}",
			Name:     namespace + ".GetValidator",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetValidator,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/states/{state_id}/validator_balances",
			Name:     namespace + ".GetValidatorBalances",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetValidatorBalances,
			Methods: []string{http.MethodGet, http.MethodPost},
		},
		{
			Template: "/eth/v1/beacon/states/{state_id}/validator_identities",
			Name:     namespace + ".GetValidatorIdentities",
			Middleware: []middleware.Middleware{
				middleware.ContentTypeHandler([]string{api.JsonMediaType}),
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType, api.OctetStreamMediaType}),
			},
			Handler: server.GetValidatorIdentities,
			Methods: []string{http.MethodPost},
		},
		{
			// Deprecated: no longer needed post Electra
			Template: "/eth/v1/beacon/deposit_snapshot",
			Name:     namespace + ".GetDepositSnapshot",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetDepositSnapshot,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/states/{state_id}/pending_deposits",
			Name:     namespace + ".GetPendingDeposits",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetPendingDeposits,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/states/{state_id}/pending_consolidations",
			Name:     namespace + ".GetPendingConsolidations",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetPendingDeposits,
			Methods: []string{http.MethodGet},
		},
		{
			Template: "/eth/v1/beacon/states/{state_id}/pending_partial_withdrawals",
			Name:     namespace + ".GetPendingPartialWithdrawals",
			Middleware: []middleware.Middleware{
				middleware.AcceptHeaderHandler([]string{api.JsonMediaType}),
			},
			Handler: server.GetPendingPartialWithdrawals,
			Methods: []string{http.MethodGet},
		},
	}
}
