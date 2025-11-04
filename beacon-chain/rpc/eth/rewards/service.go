package rewards

import (
	"context"
	"net/http"
	"strconv"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/altair"
	coreblocks "github.com/OffchainLabs/prysm/v6/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/validators"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen"
	consensusblocks "github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	"github.com/OffchainLabs/prysm/v6/time/slots"
)

// BlockRewardsFetcher is a interface that provides access to reward related responses
type BlockRewardsFetcher interface {
	GetBlockRewardsData(context.Context, interfaces.ReadOnlyBeaconBlock) (*structs.BlockRewards, *httputil.DefaultJsonError)
	GetStateForRewards(context.Context, interfaces.ReadOnlyBeaconBlock) (state.BeaconState, *httputil.DefaultJsonError)
}

// BlockRewardService implements BlockRewardsFetcher and can be declared to access the underlying functions
type BlockRewardService struct {
	Replayer stategen.ReplayerBuilder
	DB       db.HeadAccessDatabase
}

// GetBlockRewardsData returns the BlockRewards object which is used for the BlockRewardsResponse and ProduceBlockV3.
// Rewards are denominated in Gwei.
func (rs *BlockRewardService) GetBlockRewardsData(ctx context.Context, blk interfaces.ReadOnlyBeaconBlock) (*structs.BlockRewards, *httputil.DefaultJsonError) {
	if blk == nil || blk.IsNil() {
		return nil, &httputil.DefaultJsonError{
			Message: consensusblocks.ErrNilBeaconBlock.Error(),
			Code:    http.StatusInternalServerError,
		}
	}

	st, httpErr := rs.GetStateForRewards(ctx, blk)
	if httpErr != nil {
		return nil, httpErr
	}

	proposerIndex := blk.ProposerIndex()
	initBalance, err := st.BalanceAtIndex(proposerIndex)
	if err != nil {
		return nil, &httputil.DefaultJsonError{
			Message: "Could not get proposer's balance: " + err.Error(),
			Code:    http.StatusInternalServerError,
		}
	}
	st, err = altair.ProcessAttestationsNoVerifySignature(ctx, st, blk)
	if err != nil {
		return nil, &httputil.DefaultJsonError{
			Message: "Could not get attestation rewards: " + err.Error(),
			Code:    http.StatusInternalServerError,
		}
	}
	attBalance, err := st.BalanceAtIndex(proposerIndex)
	if err != nil {
		return nil, &httputil.DefaultJsonError{
			Message: "Could not get proposer's balance: " + err.Error(),
			Code:    http.StatusInternalServerError,
		}
	}
	var exitInfo *validators.ExitInfo
	if len(blk.Body().ProposerSlashings()) > 0 || len(blk.Body().AttesterSlashings()) > 0 {
		// ExitInformation is expensive to compute, only do it if we need it.
		exitInfo = validators.ExitInformation(st)
	}
	st, err = coreblocks.ProcessAttesterSlashingsNoVerify(ctx, st, blk.Body().AttesterSlashings(), exitInfo)
	if err != nil {
		return nil, &httputil.DefaultJsonError{
			Message: "Could not get attester slashing rewards: " + err.Error(),
			Code:    http.StatusInternalServerError,
		}
	}
	attSlashingsBalance, err := st.BalanceAtIndex(proposerIndex)
	if err != nil {
		return nil, &httputil.DefaultJsonError{
			Message: "Could not get proposer's balance: " + err.Error(),
			Code:    http.StatusInternalServerError,
		}
	}
	st, err = coreblocks.ProcessProposerSlashingsNoVerify(ctx, st, blk.Body().ProposerSlashings(), exitInfo)
	if err != nil {
		return nil, &httputil.DefaultJsonError{
			Message: "Could not get proposer slashing rewards: " + err.Error(),
			Code:    http.StatusInternalServerError,
		}
	}
	proposerSlashingsBalance, err := st.BalanceAtIndex(proposerIndex)
	if err != nil {
		return nil, &httputil.DefaultJsonError{
			Message: "Could not get proposer's balance: " + err.Error(),
			Code:    http.StatusInternalServerError,
		}
	}
	sa, err := blk.Body().SyncAggregate()
	if err != nil {
		return nil, &httputil.DefaultJsonError{
			Message: "Could not get sync aggregate: " + err.Error(),
			Code:    http.StatusInternalServerError,
		}
	}
	var syncCommitteeReward uint64
	_, syncCommitteeReward, err = altair.ProcessSyncAggregateNoVerifySig(ctx, st, sa)
	if err != nil {
		return nil, &httputil.DefaultJsonError{
			Message: "Could not get sync aggregate rewards: " + err.Error(),
			Code:    http.StatusInternalServerError,
		}
	}

	return &structs.BlockRewards{
		ProposerIndex:     strconv.FormatUint(uint64(proposerIndex), 10),
		Total:             strconv.FormatUint(proposerSlashingsBalance-initBalance+syncCommitteeReward, 10),
		Attestations:      strconv.FormatUint(attBalance-initBalance, 10),
		SyncAggregate:     strconv.FormatUint(syncCommitteeReward, 10),
		ProposerSlashings: strconv.FormatUint(proposerSlashingsBalance-attSlashingsBalance, 10),
		AttesterSlashings: strconv.FormatUint(attSlashingsBalance-attBalance, 10),
	}, nil
}

// GetStateForRewards returns the state replayed up to the block's slot
func (rs *BlockRewardService) GetStateForRewards(ctx context.Context, blk interfaces.ReadOnlyBeaconBlock) (state.BeaconState, *httputil.DefaultJsonError) {
	// We want to run several block processing functions that update the proposer's balance.
	// This will allow us to calculate proposer rewards for each operation (atts, slashings etc).
	// To do this, we replay the state up to the block's slot, but before processing the block.

	// Try getting the state from the next slot cache first.
	_, prevSlotRoots, err := rs.DB.BlockRootsBySlot(ctx, slots.PrevSlot(blk.Slot()))
	if err != nil {
		return nil, &httputil.DefaultJsonError{
			Message: "Could not get roots for previous slot: " + err.Error(),
			Code:    http.StatusInternalServerError,
		}
	}
	for _, r := range prevSlotRoots {
		s := transition.NextSlotState(r[:], blk.Slot())
		if s != nil {
			return s, nil
		}
	}

	st, err := rs.Replayer.ReplayerForSlot(slots.PrevSlot(blk.Slot())).ReplayToSlot(ctx, blk.Slot())
	if err != nil {
		return nil, &httputil.DefaultJsonError{
			Message: "Could not get state: " + err.Error(),
			Code:    http.StatusInternalServerError,
		}
	}
	return st, nil
}
