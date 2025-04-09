package beaconapi

import (
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

type endpoint interface {
	getBasePath() string
	sanityCheckOnlyEnabled() bool
	enableSanityCheckOnly()
	sszEnabled() bool
	enableSsz()
	getSszResp() []byte     // retrieves the Prysm SSZ response
	setSszResp(resp []byte) // sets the Prysm SSZ response
	getStart() primitives.Epoch
	setStart(start primitives.Epoch)
	getPOSTObj() interface{}
	setPOSTObj(obj interface{})
	getPResp() interface{}  // retrieves the Prysm JSON response
	getLHResp() interface{} // retrieves the Lighthouse JSON response
	getParams(epoch primitives.Epoch) []string
	setParams(f func(primitives.Epoch) []string)
	getCustomEval() func(interface{}, interface{}) error
	setCustomEval(f func(interface{}, interface{}) error)
}

type apiEndpoint[Resp any] struct {
	basePath   string
	sanity     bool
	ssz        bool
	start      primitives.Epoch
	postObj    interface{}
	pResp      *Resp  // Prysm JSON response
	lhResp     *Resp  // Lighthouse JSON response
	sszResp    []byte // Prysm SSZ response
	params     func(currentEpoch primitives.Epoch) []string
	customEval func(interface{}, interface{}) error
}

func (e *apiEndpoint[Resp]) getBasePath() string {
	return e.basePath
}

func (e *apiEndpoint[Resp]) sanityCheckOnlyEnabled() bool {
	return e.sanity
}

func (e *apiEndpoint[Resp]) enableSanityCheckOnly() {
	e.sanity = true
}

func (e *apiEndpoint[Resp]) sszEnabled() bool {
	return e.ssz
}

func (e *apiEndpoint[Resp]) enableSsz() {
	e.ssz = true
}

func (e *apiEndpoint[Resp]) getSszResp() []byte {
	return e.sszResp
}

func (e *apiEndpoint[Resp]) setSszResp(resp []byte) {
	e.sszResp = resp
}

func (e *apiEndpoint[Resp]) getStart() primitives.Epoch {
	return e.start
}

func (e *apiEndpoint[Resp]) setStart(start primitives.Epoch) {
	e.start = start
}

func (e *apiEndpoint[Resp]) getPOSTObj() interface{} {
	return e.postObj
}

func (e *apiEndpoint[Resp]) setPOSTObj(obj interface{}) {
	e.postObj = obj
}

func (e *apiEndpoint[Resp]) getPResp() interface{} {
	return e.pResp
}

func (e *apiEndpoint[Resp]) getLHResp() interface{} {
	return e.lhResp
}

func (e *apiEndpoint[Resp]) getParams(epoch primitives.Epoch) []string {
	if e.params == nil {
		return nil
	}
	return e.params(epoch)
}

func (e *apiEndpoint[Resp]) setParams(f func(currentEpoch primitives.Epoch) []string) {
	e.params = f
}

func (e *apiEndpoint[Resp]) getCustomEval() func(interface{}, interface{}) error {
	return e.customEval
}

func (e *apiEndpoint[Resp]) setCustomEval(f func(interface{}, interface{}) error) {
	e.customEval = f
}

func newMetadata[Resp any](basePath string, opts ...endpointOpt) *apiEndpoint[Resp] {
	m := &apiEndpoint[Resp]{
		basePath: basePath,
		pResp:    new(Resp),
		lhResp:   new(Resp),
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

type endpointOpt func(endpoint)

// We only care if the request was successful, without comparing responses.
func withSanityCheckOnly() endpointOpt {
	return func(e endpoint) {
		e.enableSanityCheckOnly()
	}
}

// We request SSZ data too.
func withSsz() endpointOpt {
	return func(e endpoint) {
		e.enableSsz()
	}
}

// We begin issuing the request at a particular epoch.
func withStart(start primitives.Epoch) endpointOpt {
	return func(e endpoint) {
		e.setStart(start)
	}
}

// We perform a POST instead of GET, sending an object.
func withPOSTObj(obj interface{}) endpointOpt {
	return func(e endpoint) {
		e.setPOSTObj(obj)
	}
}

// We specify URL parameters.
func withParams(f func(currentEpoch primitives.Epoch) []string) endpointOpt {
	return func(e endpoint) {
		e.setParams(f)
	}
}

// We perform custom evaluation on responses.
func withCustomEval(f func(interface{}, interface{}) error) endpointOpt {
	return func(e endpoint) {
		e.setCustomEval(f)
	}
}
