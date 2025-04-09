package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

func (c *beaconApiValidatorClient) submitSignedContributionAndProof(ctx context.Context, in *ethpb.SignedContributionAndProof) error {
	if in == nil {
		return errors.New("signed contribution and proof is nil")
	}

	if in.Message == nil {
		return errors.New("signed contribution and proof message is nil")
	}

	if in.Message.Contribution == nil {
		return errors.New("signed contribution and proof contribution is nil")
	}

	jsonContributionAndProofs := []structs.SignedContributionAndProof{
		{
			Message: &structs.ContributionAndProof{
				AggregatorIndex: strconv.FormatUint(uint64(in.Message.AggregatorIndex), 10),
				Contribution: &structs.SyncCommitteeContribution{
					Slot:              strconv.FormatUint(uint64(in.Message.Contribution.Slot), 10),
					BeaconBlockRoot:   hexutil.Encode(in.Message.Contribution.BlockRoot),
					SubcommitteeIndex: strconv.FormatUint(in.Message.Contribution.SubcommitteeIndex, 10),
					AggregationBits:   hexutil.Encode(in.Message.Contribution.AggregationBits),
					Signature:         hexutil.Encode(in.Message.Contribution.Signature),
				},
				SelectionProof: hexutil.Encode(in.Message.SelectionProof),
			},
			Signature: hexutil.Encode(in.Signature),
		},
	}

	jsonContributionAndProofsBytes, err := json.Marshal(jsonContributionAndProofs)
	if err != nil {
		return errors.Wrap(err, "failed to marshall signed contribution and proof")
	}

	return c.jsonRestHandler.Post(
		ctx,
		"/eth/v1/validator/contribution_and_proofs",
		nil,
		bytes.NewBuffer(jsonContributionAndProofsBytes),
		nil,
	)
}
