package structs

import (
	"fmt"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/server"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// maxRequestAuthDataSize mirrors MAX_DATA_SIZE from the Gloas builder-api spec; RequestAuthV1.data
// (the builder URL) is a ByteList[MAX_DATA_SIZE].
const maxRequestAuthDataSize = 4096

type RequestAuthV1 struct {
	Data string `json:"data"`
	Slot string `json:"slot"`
}

type SignedRequestAuthV1 struct {
	Message   *RequestAuthV1 `json:"message"`
	Signature string         `json:"signature"`
}

type BuilderPreferencesV1 struct {
	MaxExecutionPayment string `json:"max_execution_payment"`
}

type BuilderPreferencesRequestV1 struct {
	Preferences *BuilderPreferencesV1 `json:"preferences"`
	Auth        *SignedRequestAuthV1  `json:"auth"`
}

func RequestAuthV1FromConsensus(a *eth.RequestAuthV1) *RequestAuthV1 {
	return &RequestAuthV1{
		Data: hexutil.Encode(a.Data),
		Slot: fmt.Sprintf("%d", a.Slot),
	}
}

func SignedRequestAuthV1FromConsensus(a *eth.SignedRequestAuthV1) *SignedRequestAuthV1 {
	return &SignedRequestAuthV1{
		Message:   RequestAuthV1FromConsensus(a.Message),
		Signature: hexutil.Encode(a.Signature),
	}
}

func BuilderPreferencesV1FromConsensus(p *eth.BuilderPreferencesV1) *BuilderPreferencesV1 {
	return &BuilderPreferencesV1{
		MaxExecutionPayment: fmt.Sprintf("%d", p.MaxExecutionPayment),
	}
}

func BuilderPreferencesRequestV1FromConsensus(r *eth.BuilderPreferencesRequestV1) *BuilderPreferencesRequestV1 {
	return &BuilderPreferencesRequestV1{
		Preferences: BuilderPreferencesV1FromConsensus(r.Preferences),
		Auth:        SignedRequestAuthV1FromConsensus(r.Auth),
	}
}

func (a *RequestAuthV1) ToConsensus() (*eth.RequestAuthV1, error) {
	if a == nil {
		return nil, errNilValue
	}
	data, err := hexutil.Decode(a.Data)
	if err != nil {
		return nil, server.NewDecodeError(err, "Data")
	}
	if len(data) > maxRequestAuthDataSize {
		return nil, server.NewDecodeError(fmt.Errorf("data length %d exceeds max %d", len(data), maxRequestAuthDataSize), "Data")
	}
	slot, err := strconv.ParseUint(a.Slot, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "Slot")
	}
	return &eth.RequestAuthV1{
		Data: data,
		Slot: primitives.Slot(slot),
	}, nil
}

func (a *SignedRequestAuthV1) ToConsensus() (*eth.SignedRequestAuthV1, error) {
	if a == nil {
		return nil, errNilValue
	}
	message, err := a.Message.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Message")
	}
	sig, err := bytesutil.DecodeHexWithLength(a.Signature, fieldparams.BLSSignatureLength)
	if err != nil {
		return nil, server.NewDecodeError(err, "Signature")
	}
	return &eth.SignedRequestAuthV1{
		Message:   message,
		Signature: sig,
	}, nil
}

func (p *BuilderPreferencesV1) ToConsensus() (*eth.BuilderPreferencesV1, error) {
	if p == nil {
		return nil, errNilValue
	}
	maxExecutionPayment, err := strconv.ParseUint(p.MaxExecutionPayment, 10, 64)
	if err != nil {
		return nil, server.NewDecodeError(err, "MaxExecutionPayment")
	}
	return &eth.BuilderPreferencesV1{
		MaxExecutionPayment: primitives.Gwei(maxExecutionPayment),
	}, nil
}

func (r *BuilderPreferencesRequestV1) ToConsensus() (*eth.BuilderPreferencesRequestV1, error) {
	if r == nil {
		return nil, errNilValue
	}
	preferences, err := r.Preferences.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Preferences")
	}
	auth, err := r.Auth.ToConsensus()
	if err != nil {
		return nil, server.NewDecodeError(err, "Auth")
	}
	return &eth.BuilderPreferencesRequestV1{
		Preferences: preferences,
		Auth:        auth,
	}, nil
}
