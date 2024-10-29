package validator

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prysmaticlabs/prysm/v5/api/client/builder"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/encoding/ssz"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	"github.com/prysmaticlabs/prysm/v5/network/forks"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
)

var (
	builderValueGweiGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "builder_value_gwei",
		Help: "Builder payload value in gwei",
	})
	localValueGweiGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "local_value_gwei",
		Help: "Local payload value in gwei",
	})
	builderGetPayloadMissCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "builder_get_payload_miss_count",
		Help: "The number of get payload misses for validator requests to builder",
	})
)

// emptyTransactionsRoot represents the returned value of ssz.TransactionsRoot([][]byte{}) and
// can be used as a constant to avoid recomputing this value in every call.
var emptyTransactionsRoot = [32]byte{127, 254, 36, 30, 166, 1, 135, 253, 176, 24, 123, 250, 34, 222, 53, 209, 249, 190, 215, 171, 6, 29, 148, 1, 253, 71, 227, 74, 84, 251, 237, 225}

// blockBuilderTimeout is the maximum amount of time allowed for a block builder to respond to a
// block request. This value is known as `BUILDER_PROPOSAL_DELAY_TOLERANCE` in builder spec.
const blockBuilderTimeout = 1 * time.Second

// Sets the execution data for the block. Execution data can come from local EL client or remote builder depends on validator registration and circuit breaker conditions.
func setExecutionData(ctx context.Context, blk interfaces.SignedBeaconBlock, local *blocks.GetPayloadResponse, bid builder.Bid, builderBoostFactor primitives.Gwei) (primitives.Wei, *enginev1.BlobsBundle, error) {
	_, span := trace.StartSpan(ctx, "ProposerServer.setExecutionData")
	defer span.End()

	slot := blk.Block().Slot()
	if slots.ToEpoch(slot) < params.BeaconConfig().BellatrixForkEpoch {
		return primitives.ZeroWei(), nil, nil
	}

	if local == nil {
		return primitives.ZeroWei(), nil, errors.New("local payload is nil")
	}

	// Use local payload if builder payload is nil.
	if bid == nil {
		return local.Bid, local.BlobsBundle, setLocalExecution(blk, local)
	}

	var builderKzgCommitments [][]byte
	builderPayload, err := bid.Header()
	if err != nil {
		log.WithError(err).Warn("Proposer: failed to retrieve header from BuilderBid")
		return local.Bid, local.BlobsBundle, setLocalExecution(blk, local)
	}
	//TODO: add builder execution requests here.
	if bid.Version() >= version.Deneb {
		builderKzgCommitments, err = bid.BlobKzgCommitments()
		if err != nil {
			log.WithError(err).Warn("Proposer: failed to retrieve kzg commitments from BuilderBid")
		}
	}

	switch {
	case blk.Version() >= version.Capella:
		withdrawalsMatched, err := matchingWithdrawalsRoot(local.ExecutionData, builderPayload)
		if err != nil {
			tracing.AnnotateError(span, err)
			log.WithError(err).Warn("Proposer: failed to match withdrawals root")
			return local.Bid, local.BlobsBundle, setLocalExecution(blk, local)
		}

		// Compare payload values between local and builder. Default to the local value if it is higher.
		localValueGwei := primitives.WeiToGwei(local.Bid)
		builderValueGwei := primitives.WeiToGwei(bid.Value())
		minBid := primitives.Gwei(params.BeaconConfig().MinBuilderBid)
		// Use local block if min bid is not attained
		if builderValueGwei < minBid {
			log.WithFields(logrus.Fields{
				"minBuilderBid":    minBid,
				"builderGweiValue": builderValueGwei,
			}).Warn("Proposer: using local execution payload because min bid not attained")
			return local.Bid, local.BlobsBundle, setLocalExecution(blk, local)
		}

		// Use local block if min difference is not attained
		minDiff := localValueGwei + primitives.Gwei(params.BeaconConfig().MinBuilderDiff)
		if builderValueGwei < minDiff {
			log.WithFields(logrus.Fields{
				"localGweiValue":   localValueGwei,
				"minBidDiff":       minDiff,
				"builderGweiValue": builderValueGwei,
			}).Warn("Proposer: using local execution payload because min difference with local value was not attained")
			return local.Bid, local.BlobsBundle, setLocalExecution(blk, local)
		}

		// Use builder payload if the following in true:
		// builder_bid_value * builderBoostFactor(default 100) > local_block_value * (local-block-value-boost + 100)
		boost := primitives.Gwei(params.BeaconConfig().LocalBlockValueBoost)
		higherValueBuilder := builderValueGwei*builderBoostFactor > localValueGwei*(100+boost)
		if boost > 0 && builderBoostFactor != defaultBuilderBoostFactor {
			log.WithFields(logrus.Fields{
				"localGweiValue":       localValueGwei,
				"localBoostPercentage": boost,
				"builderGweiValue":     builderValueGwei,
				"builderBoostFactor":   builderBoostFactor,
			}).Warn("Proposer: both local boost and builder boost are using non default values")
		}
		builderValueGweiGauge.Set(float64(builderValueGwei))
		localValueGweiGauge.Set(float64(localValueGwei))

		// If we can't get the builder value, just use local block.
		if higherValueBuilder && withdrawalsMatched { // Builder value is higher and withdrawals match.
			if err := setBuilderExecution(blk, builderPayload, builderKzgCommitments); err != nil {
				log.WithError(err).Warn("Proposer: failed to set builder payload")
				return local.Bid, local.BlobsBundle, setLocalExecution(blk, local)
			} else {
				return bid.Value(), nil, nil
			}
		}
		if !higherValueBuilder {
			log.WithFields(logrus.Fields{
				"localGweiValue":       localValueGwei,
				"localBoostPercentage": boost,
				"builderGweiValue":     builderValueGwei,
				"builderBoostFactor":   builderBoostFactor,
			}).Warn("Proposer: using local execution payload because higher value")
		}
		span.SetAttributes(
			trace.BoolAttribute("higherValueBuilder", higherValueBuilder),
			trace.Int64Attribute("localGweiValue", int64(localValueGwei)),         // lint:ignore uintcast -- This is OK for tracing.
			trace.Int64Attribute("localBoostPercentage", int64(boost)),            // lint:ignore uintcast -- This is OK for tracing.
			trace.Int64Attribute("builderGweiValue", int64(builderValueGwei)),     // lint:ignore uintcast -- This is OK for tracing.
			trace.Int64Attribute("builderBoostFactor", int64(builderBoostFactor)), // lint:ignore uintcast -- This is OK for tracing.
		)
		return local.Bid, local.BlobsBundle, setLocalExecution(blk, local)
	default: // Bellatrix case.
		if err := setBuilderExecution(blk, builderPayload, builderKzgCommitments); err != nil {
			log.WithError(err).Warn("Proposer: failed to set builder payload")
			return local.Bid, local.BlobsBundle, setLocalExecution(blk, local)
		} else {
			return bid.Value(), nil, nil
		}
	}
}

// This function retrieves the payload header and kzg commitments given the slot number and the validator index.
// It's a no-op if the latest head block is not versioned bellatrix.
func (vs *Server) getPayloadHeaderFromBuilder(ctx context.Context, slot primitives.Slot, idx primitives.ValidatorIndex) (builder.Bid, error) {
	ctx, span := trace.StartSpan(ctx, "ProposerServer.getPayloadHeaderFromBuilder")
	defer span.End()

	if slots.ToEpoch(slot) < params.BeaconConfig().BellatrixForkEpoch {
		return nil, errors.New("can't get payload header from builder before bellatrix epoch")
	}

	b, err := vs.HeadFetcher.HeadBlock(ctx)
	if err != nil {
		return nil, err
	}

	h, err := b.Block().Body().Execution()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get execution header")
	}
	pk, err := vs.HeadFetcher.HeadValidatorIndexToPublicKey(ctx, idx)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, blockBuilderTimeout)
	defer cancel()

	signedBid, err := vs.BlockBuilder.GetHeader(ctx, slot, bytesutil.ToBytes32(h.BlockHash()), pk)
	if err != nil {
		return nil, err
	}
	if signedBid == nil || signedBid.IsNil() {
		return nil, errors.New("builder returned nil bid")
	}
	fork, err := forks.Fork(slots.ToEpoch(slot))
	if err != nil {
		return nil, errors.Wrap(err, "unable to get fork information")
	}
	forkName, ok := params.BeaconConfig().ForkVersionNames[bytesutil.ToBytes4(fork.CurrentVersion)]
	if !ok {
		return nil, errors.New("unable to find current fork in schedule")
	}
	if !strings.EqualFold(version.String(signedBid.Version()), forkName) {
		return nil, fmt.Errorf("builder bid response version: %d is different from head block version: %d for epoch %d", signedBid.Version(), b.Version(), slots.ToEpoch(slot))
	}

	bid, err := signedBid.Message()
	if err != nil {
		return nil, errors.Wrap(err, "could not get bid")
	}
	if bid == nil || bid.IsNil() {
		return nil, errors.New("builder returned nil bid")
	}

	v := bid.Value()
	if big.NewInt(0).Cmp(v) == 0 {
		return nil, errors.New("builder returned header with 0 bid amount")
	}

	header, err := bid.Header()
	if err != nil {
		return nil, errors.Wrap(err, "could not get bid header")
	}
	txRoot, err := header.TransactionsRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not get transaction root")
	}
	if bytesutil.ToBytes32(txRoot) == emptyTransactionsRoot {
		return nil, errors.New("builder returned header with an empty tx root")
	}

	if !bytes.Equal(header.ParentHash(), h.BlockHash()) {
		return nil, fmt.Errorf("incorrect parent hash %#x != %#x", header.ParentHash(), h.BlockHash())
	}

	reg, err := vs.BlockBuilder.RegistrationByValidatorID(ctx, idx)
	if err != nil {
		log.WithError(err).Warn("Proposer: failed to get registration by validator ID, could not check gas limit")
	} else {
		if reg.GasLimit != header.GasLimit() {
			return nil, fmt.Errorf("incorrect header gas limit %d != %d", reg.GasLimit, header.GasLimit())
		}
	}

	t, err := slots.ToTime(uint64(vs.TimeFetcher.GenesisTime().Unix()), slot)
	if err != nil {
		return nil, err
	}
	if header.Timestamp() != uint64(t.Unix()) {
		return nil, fmt.Errorf("incorrect timestamp %d != %d", header.Timestamp(), uint64(t.Unix()))
	}

	if err := validateBuilderSignature(signedBid); err != nil {
		return nil, errors.Wrap(err, "could not validate builder signature")
	}

	var kzgCommitments [][]byte
	if bid.Version() >= version.Deneb {
		kzgCommitments, err = bid.BlobKzgCommitments()
		if err != nil {
			return nil, errors.Wrap(err, "could not get blob kzg commitments")
		}
		if len(kzgCommitments) > fieldparams.MaxBlobsPerBlock {
			return nil, fmt.Errorf("builder returned too many kzg commitments: %d", len(kzgCommitments))
		}
		for _, c := range kzgCommitments {
			if len(c) != fieldparams.BLSPubkeyLength {
				return nil, fmt.Errorf("builder returned invalid kzg commitment length: %d", len(c))
			}
		}
	}

	l := log.WithFields(logrus.Fields{
		"gweiValue":          primitives.WeiToGwei(v),
		"builderPubKey":      fmt.Sprintf("%#x", bid.Pubkey()),
		"blockHash":          fmt.Sprintf("%#x", header.BlockHash()),
		"slot":               slot,
		"validator":          idx,
		"sinceSlotStartTime": time.Since(t),
	})
	if len(kzgCommitments) > 0 {
		l = l.WithField("kzgCommitmentCount", len(kzgCommitments))
	}
	l.Info("Received header with bid")

	span.SetAttributes(
		trace.StringAttribute("value", primitives.WeiToBigInt(v).String()),
		trace.StringAttribute("builderPubKey", fmt.Sprintf("%#x", bid.Pubkey())),
		trace.StringAttribute("blockHash", fmt.Sprintf("%#x", header.BlockHash())),
	)

	return bid, nil
}

// Validates builder signature and returns an error if the signature is invalid.
func validateBuilderSignature(signedBid builder.SignedBid) error {
	d, err := signing.ComputeDomain(params.BeaconConfig().DomainApplicationBuilder,
		nil, /* fork version */
		nil /* genesis val root */)
	if err != nil {
		return err
	}
	if signedBid == nil || signedBid.IsNil() {
		return errors.New("nil builder bid")
	}
	bid, err := signedBid.Message()
	if err != nil {
		return errors.Wrap(err, "could not get bid")
	}
	if bid == nil || bid.IsNil() {
		return errors.New("builder returned nil bid")
	}
	return signing.VerifySigningRoot(bid, bid.Pubkey(), signedBid.Signature(), d)
}

func matchingWithdrawalsRoot(local, builder interfaces.ExecutionData) (bool, error) {
	wds, err := local.Withdrawals()
	if err != nil {
		return false, errors.Wrap(err, "could not get local withdrawals")
	}
	br, err := builder.WithdrawalsRoot()
	if err != nil {
		return false, errors.Wrap(err, "could not get builder withdrawals root")
	}
	wr, err := ssz.WithdrawalSliceRoot(wds, fieldparams.MaxWithdrawalsPerPayload)
	if err != nil {
		return false, errors.Wrap(err, "could not compute local withdrawals root")
	}

	if !bytes.Equal(br, wr[:]) {
		log.WithFields(logrus.Fields{
			"local":   fmt.Sprintf("%#x", wr),
			"builder": fmt.Sprintf("%#x", br),
		}).Warn("Proposer: withdrawal roots don't match, using local block")
		return false, nil
	}
	return true, nil
}

// setLocalExecution sets the execution context for a local beacon block.
// It delegates to setExecution for the actual work.
func setLocalExecution(blk interfaces.SignedBeaconBlock, local *blocks.GetPayloadResponse) error {
	var kzgCommitments [][]byte
	if local.BlobsBundle != nil {
		kzgCommitments = local.BlobsBundle.KzgCommitments
	}
	if local.ExecutionRequests != nil {
		if err := blk.SetExecutionRequests(local.ExecutionRequests); err != nil {
			return errors.Wrap(err, "could not set execution requests")
		}
	}

	return setExecution(blk, local.ExecutionData, false, kzgCommitments)
}

// setBuilderExecution sets the execution context for a builder's beacon block.
// It delegates to setExecution for the actual work.
func setBuilderExecution(blk interfaces.SignedBeaconBlock, execution interfaces.ExecutionData, builderKzgCommitments [][]byte) error {
	// TODO #14344: add execution requests for electra
	return setExecution(blk, execution, true, builderKzgCommitments)
}

// setExecution sets the execution context for a beacon block. It also sets KZG commitments based on the block version.
// The function is designed to be flexible and handle both local and builder executions.
func setExecution(blk interfaces.SignedBeaconBlock, execution interfaces.ExecutionData, isBlinded bool, kzgCommitments [][]byte) error {
	if execution == nil {
		return errors.New("execution is nil")
	}

	// Set the execution data for the block
	errMessage := "failed to set local execution"
	if isBlinded {
		errMessage = "failed to set builder execution"
	}
	if err := blk.SetExecution(execution); err != nil {
		return errors.Wrap(err, errMessage)
	}

	// If the block version is below Deneb, no further actions are needed
	if blk.Version() < version.Deneb {
		return nil
	}

	// Set the KZG commitments for the block
	errMessage = "failed to set local kzg commitments"
	if isBlinded {
		errMessage = "failed to set builder kzg commitments"
	}
	if err := blk.SetBlobKzgCommitments(kzgCommitments); err != nil {
		return errors.Wrap(err, errMessage)
	}

	return nil
}
