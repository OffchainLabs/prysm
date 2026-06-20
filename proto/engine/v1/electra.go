package enginev1

import (
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

var (
	drExample  = &DepositRequest{}
	drSize     = drExample.SizeSSZ()
	wrExample  = &WithdrawalRequest{}
	wrSize     = wrExample.SizeSSZ()
	crExample  = &ConsolidationRequest{}
	crSize     = crExample.SizeSSZ()
	bdrExample = &BuilderDepositRequest{}
	bdrSize    = bdrExample.SizeSSZ()
	berExample = &BuilderExitRequest{}
	berSize    = berExample.SizeSSZ()
)

// emptyRequestsRootOnce merkleizes a zero-value gloas ExecutionRequests.
var emptyRequestsRootOnce = sync.OnceValues(func() ([32]byte, error) {
	return (&ExecutionRequestsGloas{}).HashTreeRoot()
})

// EmptyExecutionRequestsHashTreeRoot returns the merkle root of an empty gloas ExecutionRequests.
func EmptyExecutionRequestsHashTreeRoot() ([32]byte, error) {
	return emptyRequestsRootOnce()
}

const (
	DepositRequestType = iota
	WithdrawalRequestType
	ConsolidationRequestType
	BuilderDepositRequestType
	BuilderExitRequestType
)

// ExecutionRequestConfig ensures that we don't mix up the execution request params
type ExecutionRequestLimits struct {
	Deposits        uint64
	Withdrawals     uint64
	Consolidations  uint64
	BuilderDeposits uint64
	BuilderExits    uint64
}

func (ebe *ExecutionBundleElectra) GetDecodedExecutionRequests(limits ExecutionRequestLimits) (*ExecutionRequests, error) {
	return decodeExecutionRequestList(ebe.ExecutionRequests, limits)
}

func decodeExecutionRequestList(raw [][]byte, limits ExecutionRequestLimits) (*ExecutionRequests, error) {
	requests := &ExecutionRequests{}
	var prevTypeNum *uint8
	for i := range raw {
		requestType, requestListInSSZBytes, err := decodeExecutionRequest(raw[i])
		if err != nil {
			return nil, err
		}
		if prevTypeNum != nil && *prevTypeNum >= requestType {
			return nil, errors.New("invalid execution request type order or duplicate requests, requests should be in sorted order and unique")
		}
		prevTypeNum = &requestType
		switch requestType {
		case DepositRequestType:
			drs, err := unmarshalDeposits(requestListInSSZBytes, limits.Deposits)
			if err != nil {
				return nil, err
			}
			requests.Deposits = drs
		case WithdrawalRequestType:
			wrs, err := unmarshalWithdrawals(requestListInSSZBytes, limits.Withdrawals)
			if err != nil {
				return nil, err
			}
			requests.Withdrawals = wrs
		case ConsolidationRequestType:
			crs, err := unmarshalConsolidations(requestListInSSZBytes, limits.Consolidations)
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

// decodeExecutionRequestListGloas decodes the EIP-7685 flat request list into a
// gloas ExecutionRequests, including builder deposit/exit requests (EIP-8282).
func decodeExecutionRequestListGloas(raw [][]byte, limits ExecutionRequestLimits) (*ExecutionRequestsGloas, error) {
	requests := &ExecutionRequestsGloas{}
	var prevTypeNum *uint8
	for i := range raw {
		requestType, requestListInSSZBytes, err := decodeExecutionRequest(raw[i])
		if err != nil {
			return nil, err
		}
		if prevTypeNum != nil && *prevTypeNum >= requestType {
			return nil, errors.New("invalid execution request type order or duplicate requests, requests should be in sorted order and unique")
		}
		prevTypeNum = &requestType
		switch requestType {
		case DepositRequestType:
			drs, err := unmarshalDeposits(requestListInSSZBytes, limits.Deposits)
			if err != nil {
				return nil, err
			}
			requests.Deposits = drs
		case WithdrawalRequestType:
			wrs, err := unmarshalWithdrawals(requestListInSSZBytes, limits.Withdrawals)
			if err != nil {
				return nil, err
			}
			requests.Withdrawals = wrs
		case ConsolidationRequestType:
			crs, err := unmarshalConsolidations(requestListInSSZBytes, limits.Consolidations)
			if err != nil {
				return nil, err
			}
			requests.Consolidations = crs
		case BuilderDepositRequestType:
			bds, err := unmarshalBuilderDeposits(requestListInSSZBytes, limits.BuilderDeposits)
			if err != nil {
				return nil, err
			}
			requests.BuilderDeposits = bds
		case BuilderExitRequestType:
			bes, err := unmarshalBuilderExits(requestListInSSZBytes, limits.BuilderExits)
			if err != nil {
				return nil, err
			}
			requests.BuilderExits = bes
		default:
			return nil, errors.Errorf("unsupported request type %d", requestType)
		}
	}
	return requests, nil
}

func unmarshalDeposits(requestListInSSZBytes []byte, maxDepositRequests uint64) ([]*DepositRequest, error) {
	if len(requestListInSSZBytes) < drSize {
		return nil, fmt.Errorf("invalid deposit requests SSZ size, got %d expected at least %d", len(requestListInSSZBytes), drSize)
	}
	maxSSZsize := uint64(drSize) * maxDepositRequests
	if uint64(len(requestListInSSZBytes)) > maxSSZsize {
		return nil, fmt.Errorf("invalid deposit requests SSZ size, requests should not be more than the max per payload, got %d max %d", len(requestListInSSZBytes), maxSSZsize)
	}
	return unmarshalItems(requestListInSSZBytes, drSize, func() *DepositRequest { return &DepositRequest{} })
}

func unmarshalWithdrawals(requestListInSSZBytes []byte, maxWithdrawals uint64) ([]*WithdrawalRequest, error) {
	if len(requestListInSSZBytes) < wrSize {
		return nil, fmt.Errorf("invalid withdrawal requests SSZ size, got %d expected at least %d", len(requestListInSSZBytes), wrSize)
	}
	maxSSZsize := uint64(wrSize) * maxWithdrawals
	if uint64(len(requestListInSSZBytes)) > maxSSZsize {
		return nil, fmt.Errorf("invalid withdrawal requests SSZ size, requests should not be more than the max per payload, got %d max %d", len(requestListInSSZBytes), maxSSZsize)
	}
	return unmarshalItems(requestListInSSZBytes, wrSize, func() *WithdrawalRequest { return &WithdrawalRequest{} })
}

func unmarshalConsolidations(requestListInSSZBytes []byte, maxConsolidations uint64) ([]*ConsolidationRequest, error) {
	if len(requestListInSSZBytes) < crSize {
		return nil, fmt.Errorf("invalid consolidation requests SSZ size, got %d expected at least %d", len(requestListInSSZBytes), crSize)
	}
	maxSSZsize := uint64(crSize) * maxConsolidations
	if uint64(len(requestListInSSZBytes)) > maxSSZsize {
		return nil, fmt.Errorf("invalid consolidation requests SSZ size, requests should not be more than the max per payload, got %d max %d", len(requestListInSSZBytes), maxSSZsize)
	}
	return unmarshalItems(requestListInSSZBytes, crSize, func() *ConsolidationRequest { return &ConsolidationRequest{} })
}

func unmarshalBuilderDeposits(requestListInSSZBytes []byte, maxBuilderDeposits uint64) ([]*BuilderDepositRequest, error) {
	if len(requestListInSSZBytes) < bdrSize {
		return nil, fmt.Errorf("invalid builder deposit requests SSZ size, got %d expected at least %d", len(requestListInSSZBytes), bdrSize)
	}
	maxSSZsize := uint64(bdrSize) * maxBuilderDeposits
	if uint64(len(requestListInSSZBytes)) > maxSSZsize {
		return nil, fmt.Errorf("invalid builder deposit requests SSZ size, requests should not be more than the max per payload, got %d max %d", len(requestListInSSZBytes), maxSSZsize)
	}
	return unmarshalItems(requestListInSSZBytes, bdrSize, func() *BuilderDepositRequest { return &BuilderDepositRequest{} })
}

func unmarshalBuilderExits(requestListInSSZBytes []byte, maxBuilderExits uint64) ([]*BuilderExitRequest, error) {
	if len(requestListInSSZBytes) < berSize {
		return nil, fmt.Errorf("invalid builder exit requests SSZ size, got %d expected at least %d", len(requestListInSSZBytes), berSize)
	}
	maxSSZsize := uint64(berSize) * maxBuilderExits
	if uint64(len(requestListInSSZBytes)) > maxSSZsize {
		return nil, fmt.Errorf("invalid builder exit requests SSZ size, requests should not be more than the max per payload, got %d max %d", len(requestListInSSZBytes), maxSSZsize)
	}
	return unmarshalItems(requestListInSSZBytes, berSize, func() *BuilderExitRequest { return &BuilderExitRequest{} })
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

// EncodeExecutionRequestsGloas encodes a gloas ExecutionRequests into the
// EIP-7685 flat request list, including builder deposit/exit requests (EIP-8282).
func EncodeExecutionRequestsGloas(requests *ExecutionRequestsGloas) ([]hexutil.Bytes, error) {
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
	if len(requests.BuilderDeposits) > 0 {
		bdBytes, err := marshalItems(requests.BuilderDeposits)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal builder deposit requests")
		}
		requestData := []byte{BuilderDepositRequestType}
		requestData = append(requestData, bdBytes...)
		requestsData = append(requestsData, requestData)
	}
	if len(requests.BuilderExits) > 0 {
		beBytes, err := marshalItems(requests.BuilderExits)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal builder exit requests")
		}
		requestData := []byte{BuilderExitRequestType}
		requestData = append(requestData, beBytes...)
		requestsData = append(requestsData, requestData)
	}

	return requestsData, nil
}

// ExecutionRequester is implemented by the fork-specific ExecutionRequests
// containers that can be flattened into the EIP-7685 engine-API request list.
type ExecutionRequester interface {
	FlattenRequests() ([]hexutil.Bytes, error)
}

// FlattenRequests encodes the Electra/Fulu execution requests for the engine API.
func (e *ExecutionRequests) FlattenRequests() ([]hexutil.Bytes, error) {
	return EncodeExecutionRequests(e)
}

// FlattenRequests encodes the Gloas execution requests (including builder
// requests) for the engine API.
func (e *ExecutionRequestsGloas) FlattenRequests() ([]hexutil.Bytes, error) {
	return EncodeExecutionRequestsGloas(e)
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
