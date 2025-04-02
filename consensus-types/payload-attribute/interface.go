package payloadattribute

import (
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
)

type Attributer interface {
	Version() int
	PrevRandao() []byte
	Timestamp() uint64
	SuggestedFeeRecipient() []byte
	Withdrawals() ([]*enginev1.Withdrawal, error)
	ParentBeaconBlockRoot() ([]byte, error)
	PbV1() (*enginev1.PayloadAttributes, error)
	PbV2() (*enginev1.PayloadAttributesV2, error)
	PbV3() (*enginev1.PayloadAttributesV3, error)
	IsEmpty() bool
}
