package verification

type LazyHeadStateProvider struct {
	HeadStateProvider
}

var _ HeadStateProvider = &LazyHeadStateProvider{}
