package enginev1

func (ebe *ExecutionBundleGloas) GetDecodedExecutionRequests(limits ExecutionRequestLimits) (*ExecutionRequests, error) {
	return decodeExecutionRequestList(ebe.ExecutionRequests, limits)
}
