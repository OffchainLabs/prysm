package enginev1

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

type ExecutionPayloadElectra = ExecutionPayloadDeneb
type ExecutionPayloadHeaderElectra = ExecutionPayloadHeaderDeneb

var (
	drExample = &DepositRequest{}
	drSize    = drExample.SizeSSZ()
	wrExample = &WithdrawalRequest{}
	wrSize    = wrExample.SizeSSZ()
	crExample = &ConsolidationRequest{}
	crSize    = crExample.SizeSSZ()
)

const LenExecutionRequestsElectra = 3

func (ebe *ExecutionBundleElectra) GetDecodedExecutionRequests() (*ExecutionRequests, error) {
	requests := &ExecutionRequests{}

	if len(ebe.ExecutionRequests) != LenExecutionRequestsElectra /* types of requests */ {
		return nil, errors.Errorf("invalid execution request size: %d", len(ebe.ExecutionRequests))
	}

	// deposit requests
	drs, err := unmarshalItems(ebe.ExecutionRequests[0], drSize, func() *DepositRequest { return &DepositRequest{} })
	if err != nil {
		return nil, err
	}
	requests.Deposits = drs

	// withdrawal requests
	wrs, err := unmarshalItems(ebe.ExecutionRequests[1], wrSize, func() *WithdrawalRequest { return &WithdrawalRequest{} })
	if err != nil {
		return nil, err
	}
	requests.Withdrawals = wrs

	// consolidation requests
	crs, err := unmarshalItems(ebe.ExecutionRequests[2], crSize, func() *ConsolidationRequest { return &ConsolidationRequest{} })
	if err != nil {
		return nil, err
	}
	requests.Consolidations = crs

	return requests, nil
}

func EncodeExecutionRequests(requests *ExecutionRequests) ([]hexutil.Bytes, error) {
	if requests == nil {
		return nil, errors.New("invalid execution requests")
	}

	drBytes, err := marshalItems(requests.Deposits)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal deposit requests")
	}

	wrBytes, err := marshalItems(requests.Withdrawals)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal withdrawal requests")
	}

	crBytes, err := marshalItems(requests.Consolidations)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal consolidation requests")
	}
	return []hexutil.Bytes{drBytes, wrBytes, crBytes}, nil
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
