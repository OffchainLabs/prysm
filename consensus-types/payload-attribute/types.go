package payloadattribute

import (
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	field_params "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

var (
	_ = Attributer(&data{})
)

type data struct {
	version               int
	timeStamp             uint64
	prevRandao            []byte
	suggestedFeeRecipient []byte
	withdrawals           []*enginev1.Withdrawal
	parentBeaconBlockRoot []byte
}

var (
	errNilPayloadAttribute         = errors.New("received nil payload attribute")
	errUnsupportedPayloadAttribute = errors.New("unsupported payload attribute")
	errNoParentRoot                = errors.New("parent root is empty")
)

// New returns a new payload attribute with the given input object.
func New(i interface{}) (Attributer, error) {
	switch a := i.(type) {
	case nil:
		return nil, blocks.ErrNilObject
	case *enginev1.PayloadAttributes:
		return initPayloadAttributeFromV1(a)
	case *enginev1.PayloadAttributesV2:
		return initPayloadAttributeFromV2(a)
	case *enginev1.PayloadAttributesV3:
		return initPayloadAttributeFromV3(a)
	default:
		return nil, errors.Wrapf(errUnsupportedPayloadAttribute, "unable to create payload attribute from type %T", i)
	}
}

// EmptyWithVersion returns an empty payload attribute with the given version.
func EmptyWithVersion(version int) Attributer {
	return &data{
		version: version,
	}
}

func initPayloadAttributeFromV1(a *enginev1.PayloadAttributes) (Attributer, error) {
	if a == nil {
		return nil, errNilPayloadAttribute
	}

	return &data{
		version:               version.Bellatrix,
		prevRandao:            a.PrevRandao,
		timeStamp:             a.Timestamp,
		suggestedFeeRecipient: a.SuggestedFeeRecipient,
	}, nil
}

func initPayloadAttributeFromV2(a *enginev1.PayloadAttributesV2) (Attributer, error) {
	if a == nil {
		return nil, errNilPayloadAttribute
	}

	return &data{
		version:               version.Capella,
		prevRandao:            a.PrevRandao,
		timeStamp:             a.Timestamp,
		suggestedFeeRecipient: a.SuggestedFeeRecipient,
		withdrawals:           a.Withdrawals,
	}, nil
}

func initPayloadAttributeFromV3(a *enginev1.PayloadAttributesV3) (Attributer, error) {
	if a == nil {
		return nil, errNilPayloadAttribute
	}

	return &data{
		version:               version.Deneb,
		prevRandao:            a.PrevRandao,
		timeStamp:             a.Timestamp,
		suggestedFeeRecipient: a.SuggestedFeeRecipient,
		withdrawals:           a.Withdrawals,
		parentBeaconBlockRoot: a.ParentBeaconBlockRoot,
	}, nil
}

// EventData holds the values for a PayloadAttributes event.
type EventData struct {
	ProposerIndex     primitives.ValidatorIndex
	ProposalSlot      primitives.Slot
	ParentBlockNumber uint64
	ParentBlockRoot   []byte
	ParentBlockHash   []byte
	Attributer        Attributer
	HeadState         state.BeaconState
	HeadBlock         interfaces.ReadOnlySignedBeaconBlock
	HeadRoot          [field_params.RootLength]byte
}
