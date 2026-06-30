package iface

// ClientConfig carries backend-agnostic validator client construction settings, shared by the REST
// and gRPC client constructors so the factory can forward one option set to either backend.
type ClientConfig struct {
	Stateless bool
}

// Option configures a validator client at construction, independent of the REST/gRPC backend.
type Option func(*ClientConfig)

// WithStateless enables the Gloas stateless self-build path: block production fetches the block and
// execution payload envelope together and caches the envelope for reuse by the self-build publisher.
func WithStateless(enabled bool) Option {
	return func(c *ClientConfig) { c.Stateless = enabled }
}
