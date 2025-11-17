package beaconapi

import (
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
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
	getPOSTObj() any
	setPOSTObj(obj any)
	getPResp() any  // retrieves the Prysm JSON response
	getLHResp() any // retrieves the Lighthouse JSON response
	getParams(currentEpoch primitives.Epoch) []string
	setParams(f func(currentEpoch primitives.Epoch) []string)
	getQueryParams(currentEpoch primitives.Epoch) []string
	setQueryParams(f func(currentEpoch primitives.Epoch) []string)
	getCustomEval() func(any, any) error
	setCustomEval(f func(any, any) error)
}

type apiEndpoint[Resp any] struct {
	basePath    string
	sanity      bool
	ssz         bool
	start       primitives.Epoch
	postObj     any
	pResp       *Resp  // Prysm JSON response
	lhResp      *Resp  // Lighthouse JSON response
	sszResp     []byte // Prysm SSZ response
	params      func(currentEpoch primitives.Epoch) []string
	queryParams func(currentEpoch primitives.Epoch) []string
	customEval  func(any, any) error
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

func (e *apiEndpoint[Resp]) getPOSTObj() any {
	return e.postObj
}

func (e *apiEndpoint[Resp]) setPOSTObj(obj any) {
	e.postObj = obj
}

func (e *apiEndpoint[Resp]) getPResp() any {
	return e.pResp
}

func (e *apiEndpoint[Resp]) getLHResp() any {
	return e.lhResp
}

func (e *apiEndpoint[Resp]) getParams(currentEpoch primitives.Epoch) []string {
	if e.params == nil {
		return nil
	}
	return e.params(currentEpoch)
}

func (e *apiEndpoint[Resp]) setParams(f func(currentEpoch primitives.Epoch) []string) {
	e.params = f
}

func (e *apiEndpoint[Resp]) getQueryParams(currentEpoch primitives.Epoch) []string {
	if e.queryParams == nil {
		return nil
	}
	return e.queryParams(currentEpoch)
}

func (e *apiEndpoint[Resp]) setQueryParams(f func(currentEpoch primitives.Epoch) []string) {
	e.queryParams = f
}

func (e *apiEndpoint[Resp]) getCustomEval() func(any, any) error {
	return e.customEval
}

func (e *apiEndpoint[Resp]) setCustomEval(f func(any, any) error) {
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
func withPOSTObj(obj any) endpointOpt {
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

// We specify query parameters.
func withQueryParams(f func(currentEpoch primitives.Epoch) []string) endpointOpt {
	return func(e endpoint) {
		e.setQueryParams(f)
	}
}

// We perform custom evaluation on responses.
func withCustomEval(f func(any, any) error) endpointOpt {
	return func(e endpoint) {
		e.setCustomEval(f)
	}
}
