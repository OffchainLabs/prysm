package core

import (
	"context"
	"sync/atomic"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	opfeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/synccommittee"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"golang.org/x/sync/singleflight"
)

type Service struct {
	BeaconDB                         db.ReadOnlyDatabase
	ChainInfoFetcher                 blockchain.ChainInfoFetcher
	HeadFetcher                      blockchain.HeadFetcher
	ForkchoiceFetcher                blockchain.ForkchoiceFetcher
	FinalizedFetcher                 blockchain.FinalizationFetcher
	GenesisTimeFetcher               blockchain.TimeFetcher
	SyncChecker                      sync.Checker
	Broadcaster                      p2p.Broadcaster
	SyncCommitteePool                synccommittee.Pool
	OperationNotifier                opfeed.Notifier
	AttestationCache                 *cache.AttestationDataCache
	StateGen                         stategen.StateManager
	P2P                              p2p.Broadcaster
	ReplayerBuilder                  stategen.ReplayerBuilder
	OptimisticModeFetcher            blockchain.OptimisticModeFetcher
	Ctx                              context.Context
	ExecutionPayloadEnvelopeCache    *cache.ExecutionPayloadEnvelopeCache
	ProposerPreferencesCache         *cache.ProposerPreferencesCache
	HighestBidCache                  *cache.HighestExecutionPayloadBidCache
	DataColumnReceiver               blockchain.DataColumnReceiver
	ExecutionPayloadEnvelopeReceiver blockchain.ExecutionPayloadEnvelopeReceiver
	NewExecutionPayloadBidVerifier   verification.NewExecutionPayloadBidVerifier

	payloadAttestationData   atomic.Pointer[ethpb.PayloadAttestationData]
	payloadAttestationFlight singleflight.Group
}
