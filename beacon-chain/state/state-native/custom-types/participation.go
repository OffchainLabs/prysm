package customtypes

type ReadOnlyParticipation struct {
	p []byte
}

func NewReadOnlyParticipation(p []byte) ReadOnlyParticipation {
	return ReadOnlyParticipation{p}
}

func (r ReadOnlyParticipation) At(i uint64) byte {
	return r.p[i]
}

func (r ReadOnlyParticipation) Len() int {
	return len(r.p)
}
