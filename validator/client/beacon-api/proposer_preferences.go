package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

func (c *beaconApiValidatorClient) submitSignedProposerPreferences(ctx context.Context, prefs []*ethpb.SignedProposerPreferences) error {
	jsonPrefs := make([]*structs.SignedProposerPreferences, len(prefs))
	for i, p := range prefs {
		if p == nil || p.Message == nil {
			return errors.Errorf("signed proposer preferences at index %d is nil", i)
		}
		jsonPrefs[i] = &structs.SignedProposerPreferences{
			Message: &structs.ProposerPreferences{
				DependentRoot:  hexutil.Encode(p.Message.DependentRoot),
				ProposalSlot:   strconv.FormatUint(uint64(p.Message.ProposalSlot), 10),
				ValidatorIndex: strconv.FormatUint(uint64(p.Message.ValidatorIndex), 10),
				FeeRecipient:   hexutil.Encode(p.Message.FeeRecipient),
				TargetGasLimit: strconv.FormatUint(p.Message.TargetGasLimit, 10),
			},
			Signature: hexutil.Encode(p.Signature),
		}
	}

	body, err := json.Marshal(jsonPrefs)
	if err != nil {
		return errors.Wrap(err, "failed to marshal signed proposer preferences")
	}

	return c.handler.Post(ctx, "/eth/v1/validator/proposer_preferences", nil, bytes.NewBuffer(body), nil)
}
