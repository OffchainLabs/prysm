package enginev1

import (
	"github.com/pkg/errors"
)

func (ebe *ExecutionBundleFulu) GetDecodedExecutionRequests(limits ExecutionRequestLimits) (*ExecutionRequests, error) {
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
