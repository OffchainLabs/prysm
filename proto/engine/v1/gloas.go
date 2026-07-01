package enginev1

func (ebe *ExecutionBundleGloas) GetDecodedExecutionRequests(limits ExecutionRequestLimits) (*ExecutionRequestsGloas, error) {
	return decodeExecutionRequestListGloas(ebe.ExecutionRequests, limits)
}
