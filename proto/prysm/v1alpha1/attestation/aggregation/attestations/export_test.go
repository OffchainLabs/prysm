package attestations

// export attList for the attestations_test package
type AttList attList

func (a AttList) ValidateForTesting() error {
	return attList(a).validate()
}

var RearrangeProcessedAttestations = rearrangeProcessedAttestations
var AggregateAttestations = aggregateAttestations
