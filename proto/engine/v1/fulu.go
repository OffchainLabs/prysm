package enginev1

func (ebe *ExecutionBundleFulu) GetDecodedExecutionRequests(limits ExecutionRequestLimits) (*ExecutionRequests, error) {
	return decodeExecutionRequestList(ebe.ExecutionRequests, limits)
}
