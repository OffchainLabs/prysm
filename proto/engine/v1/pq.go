package enginev1

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

// Spec: types 0x05 and 0x06 are not used.
const (
	ValidatorPreregistrationRequestType = 7
	BuilderPreregistrationRequestType   = 8
	VoluntaryExitRequestType            = 9
)

const (
	validatorPreregistrationRequestSize = 96
	builderPreregistrationRequestSize   = 64
	voluntaryExitRequestSize            = 32
)

func (e *ExecutionRequestsPQ) FlattenRequests() ([]hexutil.Bytes, error) {
	return EncodeExecutionRequestsPQ(e)
}

// The request types an execution client on Gloas also knows.
func (e *ExecutionRequestsPQ) Gloas() *ExecutionRequestsGloas {
	return &ExecutionRequestsGloas{
		Deposits:        e.Deposits,
		Withdrawals:     e.Withdrawals,
		Consolidations:  e.Consolidations,
		BuilderDeposits: e.BuilderDeposits,
		BuilderExits:    e.BuilderExits,
	}
}

// Spec: get_execution_requests_list
func EncodeExecutionRequestsPQ(requests *ExecutionRequestsPQ) ([]hexutil.Bytes, error) {
	if requests == nil {
		return nil, errors.New("invalid execution requests")
	}
	out, err := EncodeExecutionRequestsGloas(requests.Gloas())
	if err != nil {
		return nil, err
	}
	out, err = appendRequests(out, ValidatorPreregistrationRequestType, requests.ValidatorPreregistrations)
	if err != nil {
		return nil, err
	}
	out, err = appendRequests(out, BuilderPreregistrationRequestType, requests.BuilderPreregistrations)
	if err != nil {
		return nil, err
	}
	return appendRequests(out, VoluntaryExitRequestType, requests.VoluntaryExits)
}

func appendRequests[T sszMarshaler](out []hexutil.Bytes, requestType byte, items []T) ([]hexutil.Bytes, error) {
	if len(items) == 0 {
		return out, nil
	}
	enc, err := marshalItems(items)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal requests of type %d", requestType)
	}
	return append(out, append([]byte{requestType}, enc...)), nil
}

// Spec: get_execution_requests
func DecodeExecutionRequestsPQ(raw [][]byte, limits ExecutionRequestLimits) (*ExecutionRequestsPQ, error) {
	split := len(raw)
	for i, r := range raw {
		if len(r) > 0 && r[0] >= ValidatorPreregistrationRequestType {
			split = i
			break
		}
	}
	g, err := decodeExecutionRequestListGloas(raw[:split], limits)
	if err != nil {
		return nil, err
	}
	requests := &ExecutionRequestsPQ{
		Deposits:        g.Deposits,
		Withdrawals:     g.Withdrawals,
		Consolidations:  g.Consolidations,
		BuilderDeposits: g.BuilderDeposits,
		BuilderExits:    g.BuilderExits,
	}
	var prev uint8
	for _, r := range raw[split:] {
		requestType, data, err := decodeExecutionRequest(r)
		if err != nil {
			return nil, err
		}
		if requestType <= prev {
			return nil, errors.New("invalid execution request type order or duplicate requests, requests should be in sorted order and unique")
		}
		prev = requestType
		switch requestType {
		case ValidatorPreregistrationRequestType:
			requests.ValidatorPreregistrations, err = unmarshalRequests(data, validatorPreregistrationRequestSize, limits.ValidatorPreregistrations, func() *ValidatorPreregistrationRequest { return &ValidatorPreregistrationRequest{} })
		case BuilderPreregistrationRequestType:
			requests.BuilderPreregistrations, err = unmarshalRequests(data, builderPreregistrationRequestSize, limits.BuilderPreregistrations, func() *BuilderPreregistrationRequest { return &BuilderPreregistrationRequest{} })
		case VoluntaryExitRequestType:
			requests.VoluntaryExits, err = unmarshalRequests(data, voluntaryExitRequestSize, limits.VoluntaryExits, func() *VoluntaryExitRequest { return &VoluntaryExitRequest{} })
		default:
			return nil, fmt.Errorf("unsupported execution request type %d", requestType)
		}
		if err != nil {
			return nil, err
		}
	}
	return requests, nil
}

func unmarshalRequests[T sszUnmarshaler](data []byte, itemSize int, limit uint64, newItem func() T) ([]T, error) {
	if len(data) < itemSize {
		return nil, fmt.Errorf("invalid requests SSZ size, got %d expected at least %d", len(data), itemSize)
	}
	if uint64(len(data)) > uint64(itemSize)*limit {
		return nil, fmt.Errorf("invalid requests SSZ size, got %d max %d", len(data), uint64(itemSize)*limit)
	}
	return unmarshalItems(data, itemSize, newItem)
}

func (ebe *ExecutionBundleGloas) GetDecodedExecutionRequestsPQ(limits ExecutionRequestLimits) (*ExecutionRequestsPQ, error) {
	return DecodeExecutionRequestsPQ(ebe.ExecutionRequests, limits)
}
