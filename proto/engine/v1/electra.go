package enginev1

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/config/params"
)

var (
	drExample = &DepositRequest{}
	drSize    = drExample.SizeSSZ()
	wrExample = &WithdrawalRequest{}
	wrSize    = wrExample.SizeSSZ()
	crExample = &ConsolidationRequest{}
	crSize    = crExample.SizeSSZ()
)

const (
	DepositRequestType = iota
	WithdrawalRequestType
	ConsolidationRequestType
)

func (ebe *ExecutionBundleElectra) GetDecodedExecutionRequests() (*ExecutionRequests, error) {
	requests := &ExecutionRequests{}
	var prevTypeNum *uint8
	for i := range ebe.ExecutionRequests {
		requestType, requestListInSSZBytes, err := decodeExecutionRequest(ebe.ExecutionRequests[i])
		if err != nil {
			return nil, err
		}
		if prevTypeNum != nil && *prevTypeNum >= requestType {
			return nil, errors.New("invalid execution request type order or duplicate requests, requests should be in sorted order and unique")
		}
		prevTypeNum = &requestType
		switch requestType {
		case DepositRequestType:
			drs, err := unmarshalDeposits(requestListInSSZBytes)
			if err != nil {
				return nil, err
			}
			requests.Deposits = drs
		case WithdrawalRequestType:
			wrs, err := unmarshalWithdrawals(requestListInSSZBytes)
			if err != nil {
				return nil, err
			}
			requests.Withdrawals = wrs
		case ConsolidationRequestType:
			crs, err := unmarshalConsolidations(requestListInSSZBytes)
			if err != nil {
				return nil, err
			}
			requests.Consolidations = crs
		default:
			return nil, errors.Errorf("unsupported request type %d", requestType)
		}
	}
	return requests, nil
}

func unmarshalDeposits(requestListInSSZBytes []byte) ([]*DepositRequest, error) {
	if len(requestListInSSZBytes) < drSize {
		return nil, fmt.Errorf("invalid deposit requests SSZ size, got %d expected at least %d", len(requestListInSSZBytes), drSize)
	}
	maxSSZsize := uint64(drSize) * params.BeaconConfig().MaxDepositRequestsPerPayload
	if uint64(len(requestListInSSZBytes)) > maxSSZsize {
		return nil, fmt.Errorf("invalid deposit requests SSZ size, requests should not be more than the max per payload, got %d max %d", len(requestListInSSZBytes), maxSSZsize)
	}
	return unmarshalItems(requestListInSSZBytes, drSize, func() *DepositRequest { return &DepositRequest{} })
}

func unmarshalWithdrawals(requestListInSSZBytes []byte) ([]*WithdrawalRequest, error) {
	if len(requestListInSSZBytes) < wrSize {
		return nil, fmt.Errorf("invalid withdrawal requests SSZ size, got %d expected at least %d", len(requestListInSSZBytes), wrSize)
	}
	maxSSZsize := uint64(wrSize) * params.BeaconConfig().MaxWithdrawalRequestsPerPayload
	if uint64(len(requestListInSSZBytes)) > maxSSZsize {
		return nil, fmt.Errorf("invalid withdrawal requests SSZ size, requests should not be more than the max per payload, got %d max %d", len(requestListInSSZBytes), maxSSZsize)
	}
	return unmarshalItems(requestListInSSZBytes, wrSize, func() *WithdrawalRequest { return &WithdrawalRequest{} })
}

func unmarshalConsolidations(requestListInSSZBytes []byte) ([]*ConsolidationRequest, error) {
	if len(requestListInSSZBytes) < crSize {
		return nil, fmt.Errorf("invalid consolidation requests SSZ size, got %d expected at least %d", len(requestListInSSZBytes), crSize)
	}
	maxSSZsize := uint64(crSize) * params.BeaconConfig().MaxConsolidationsRequestsPerPayload
	if uint64(len(requestListInSSZBytes)) > maxSSZsize {
		return nil, fmt.Errorf("invalid consolidation requests SSZ size, requests should not be more than the max per payload, got %d max %d", len(requestListInSSZBytes), maxSSZsize)
	}
	return unmarshalItems(requestListInSSZBytes, crSize, func() *ConsolidationRequest { return &ConsolidationRequest{} })
}

func decodeExecutionRequest(req []byte) (typ uint8, data []byte, err error) {
	if len(req) < 1 {
		return 0, nil, errors.New("invalid execution request, length less than 1")
	}
	return req[0], req[1:], nil
}

func EncodeExecutionRequests(requests *ExecutionRequests) ([]hexutil.Bytes, error) {
	if requests == nil {
		return nil, errors.New("invalid execution requests")
	}

	requestsData := make([]hexutil.Bytes, 0)

	// request types MUST be in sorted order starting from 0
	if len(requests.Deposits) > 0 {
		drBytes, err := marshalItems(requests.Deposits)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal deposit requests")
		}
		requestData := []byte{DepositRequestType}
		requestData = append(requestData, drBytes...)
		requestsData = append(requestsData, requestData)
	}
	if len(requests.Withdrawals) > 0 {
		wrBytes, err := marshalItems(requests.Withdrawals)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal withdrawal requests")
		}
		requestData := []byte{WithdrawalRequestType}
		requestData = append(requestData, wrBytes...)
		requestsData = append(requestsData, requestData)
	}
	if len(requests.Consolidations) > 0 {
		crBytes, err := marshalItems(requests.Consolidations)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal consolidation requests")
		}
		requestData := []byte{ConsolidationRequestType}
		requestData = append(requestData, crBytes...)
		requestsData = append(requestsData, requestData)
	}

	return requestsData, nil
}

type sszUnmarshaler interface {
	UnmarshalSSZ([]byte) error
}

type sszMarshaler interface {
	MarshalSSZTo(buf []byte) ([]byte, error)
	SizeSSZ() int
}

func marshalItems[T sszMarshaler](items []T) ([]byte, error) {
	if len(items) == 0 {
		return []byte{}, nil
	}
	size := items[0].SizeSSZ()
	buf := make([]byte, 0, size*len(items))
	var err error
	for i, item := range items {
		buf, err = item.MarshalSSZTo(buf)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal item at index %d: %w", i, err)
		}
	}
	return buf, nil
}

// Generic function to unmarshal items
func unmarshalItems[T sszUnmarshaler](data []byte, itemSize int, newItem func() T) ([]T, error) {
	if len(data)%itemSize != 0 {
		return nil, fmt.Errorf("invalid data length: data size (%d) is not a multiple of item size (%d)", len(data), itemSize)
	}
	numItems := len(data) / itemSize
	items := make([]T, numItems)
	for i := range items {
		itemBytes := data[i*itemSize : (i+1)*itemSize]
		item := newItem()
		if err := item.UnmarshalSSZ(itemBytes); err != nil {
			return nil, fmt.Errorf("failed to unmarshal item at index %d: %w", i, err)
		}
		items[i] = item
	}
	return items, nil
}
