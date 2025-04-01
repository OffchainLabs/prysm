package validator_api

import (
	"encoding/json"
	"strconv"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

type BeaconCommitteeSelection struct {
	SelectionProof []byte
	Slot           primitives.Slot
	ValidatorIndex primitives.ValidatorIndex
}

type beaconCommitteeSelectionJson struct {
	SelectionProof string `json:"selection_proof"`
	Slot           string `json:"slot"`
	ValidatorIndex string `json:"validator_index"`
}

func (b *BeaconCommitteeSelection) MarshalJSON() ([]byte, error) {
	return json.Marshal(beaconCommitteeSelectionJson{
		SelectionProof: hexutil.Encode(b.SelectionProof),
		Slot:           strconv.FormatUint(uint64(b.Slot), 10),
		ValidatorIndex: strconv.FormatUint(uint64(b.ValidatorIndex), 10),
	})
}

func (b *BeaconCommitteeSelection) UnmarshalJSON(input []byte) error {
	var bjson beaconCommitteeSelectionJson
	err := json.Unmarshal(input, &bjson)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal beacon committee selection")
	}

	slot, err := strconv.ParseUint(bjson.Slot, 10, 64)
	if err != nil {
		return errors.Wrap(err, "failed to parse slot")
	}

	vIdx, err := strconv.ParseUint(bjson.ValidatorIndex, 10, 64)
	if err != nil {
		return errors.Wrap(err, "failed to parse validator index")
	}

	selectionProof, err := hexutil.Decode(bjson.SelectionProof)
	if err != nil {
		return errors.Wrap(err, "failed to parse selection proof")
	}

	b.Slot = primitives.Slot(slot)
	b.SelectionProof = selectionProof
	b.ValidatorIndex = primitives.ValidatorIndex(vIdx)

	return nil
}

type SyncCommitteeSelection struct {
	SelectionProof    []byte
	Slot              primitives.Slot
	SubcommitteeIndex primitives.CommitteeIndex
	ValidatorIndex    primitives.ValidatorIndex
}

type syncCommitteeSelectionJson struct {
	SelectionProof    string `json:"selection_proof"`
	Slot              string `json:"slot"`
	SubcommitteeIndex string `json:"subcommittee_index"`
	ValidatorIndex    string `json:"validator_index"`
}

func (s *SyncCommitteeSelection) MarshalJSON() ([]byte, error) {
	return json.Marshal(syncCommitteeSelectionJson{
		SelectionProof:    hexutil.Encode(s.SelectionProof),
		Slot:              strconv.FormatUint(uint64(s.Slot), 10),
		SubcommitteeIndex: strconv.FormatUint(uint64(s.SubcommitteeIndex), 10),
		ValidatorIndex:    strconv.FormatUint(uint64(s.ValidatorIndex), 10),
	})
}

func (s *SyncCommitteeSelection) UnmarshalJSON(input []byte) error {
	var resJson syncCommitteeSelectionJson
	err := json.Unmarshal(input, &resJson)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal sync committee selection")
	}

	slot, err := strconv.ParseUint(resJson.Slot, 10, 64)
	if err != nil {
		return errors.Wrap(err, "failed to parse slot")
	}

	vIdx, err := strconv.ParseUint(resJson.ValidatorIndex, 10, 64)
	if err != nil {
		return errors.Wrap(err, "failed to parse validator index")
	}

	subcommIdx, err := strconv.ParseUint(resJson.SubcommitteeIndex, 10, 64)
	if err != nil {
		return errors.Wrap(err, "failed to parse subcommittee index")
	}

	selectionProof, err := hexutil.Decode(resJson.SelectionProof)
	if err != nil {
		return errors.Wrap(err, "failed to parse selection proof")
	}

	s.Slot = primitives.Slot(slot)
	s.SelectionProof = selectionProof
	s.ValidatorIndex = primitives.ValidatorIndex(vIdx)
	s.SubcommitteeIndex = primitives.CommitteeIndex(subcommIdx)

	return nil
}

type aggregatedSelectionResponse struct {
	Data []BeaconCommitteeSelection `json:"data"`
}
