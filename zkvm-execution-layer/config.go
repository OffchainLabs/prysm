package zkvmexecutionlayer

import (
	"errors"
	"fmt"
	"time"
	executionproof "github.com/OffchainLabs/prysm/v6/consensus-types/execution-proof"
)

const (
	// defaultProofRequestTimeout is the default timeout for proof requests via RPC.
	defaultProofRequestTimeout = 5 * time.Second
	// defaultGossipGracePeriod is the default delay before falling back to RPC.
	defaultGossipGracePeriod = 4000 * time.Millisecond
)

// ZKVMExecutionLayerConfig is the configuration for the zkVM Execution Layer.
type ZKVMExecutionLayerConfig struct {
	// SubscribedSubnets are the subnets/proofs that we are subscribed to
	// and therefore need to know how to verify.
	SubscribedSubnets map[executionproof.ExecutionProofSubnetId]struct{} `json:"subscribed_subnets"`

	// MinProofsRequired is the minimum number of proofs required from _different_ subnets
	// in order for the node to mark an execution payload as VALID.
	MinProofsRequired int `json:"min_proofs_required"`

	// GenerationSubnets are the subnets to generate proofs for (empty if not generating proofs).
	GenerationSubnets map[executionproof.ExecutionProofSubnetId]struct{} `json:"generation_subnets"`

	// ProofCacheSize is the proof cache size (number of execution block hashes to cache proofs for).
	ProofCacheSize int `json:"proof_cache_size"`

	// ProofRequestTimeout is the timeout for proof requests via RPC.
	ProofRequestTimeout time.Duration `json:"proof_request_timeout"`

	// GossipGracePeriod is the delay before falling back to RPC (gossip grace period).
	GossipGracePeriod time.Duration `json:"gossip_grace_period"`
}

// NewDefaultZKVMExecutionLayerConfig creates a new config with default values.
func NewDefaultZKVMExecutionLayerConfig() *ZKVMExecutionLayerConfig {
	return &ZKVMExecutionLayerConfig{
		SubscribedSubnets:   make(map[executionproof.ExecutionProofSubnetId]struct{}),
		MinProofsRequired:   1,
		GenerationSubnets:   make(map[executionproof.ExecutionProofSubnetId]struct{}),
		ProofCacheSize:      64 * 8, // 512
		ProofRequestTimeout: defaultProofRequestTimeout,
		GossipGracePeriod:   defaultGossipGracePeriod,
	}
}

// Validate checks if the configuration is valid.
func (c *ZKVMExecutionLayerConfig) Validate() error {
	if c.MinProofsRequired == 0 {
		return errors.New("min_proofs_required must be at least 1")
	}

	if c.ProofCacheSize == 0 {
		return errors.New("proof_cache_size must be at least 1")
	}

	// Ensure we subscribe to enough subnets to meet min_proofs_required
	if len(c.SubscribedSubnets) < c.MinProofsRequired {
		return fmt.Errorf(
			"subscribed_subnets (%d) must be >= min_proofs_required (%d)",
			len(c.SubscribedSubnets),
			c.MinProofsRequired,
		)
	}

	// Node can only generate proofs for subnets they are subscribed to
	for subnet := range c.GenerationSubnets {
		if _, ok := c.SubscribedSubnets[subnet]; !ok {
			return fmt.Errorf(
				"generation_subnets must be a subset of subscribed_subnets (subnet %s not subscribed)",
				subnet,
			)
		}
	}

	return nil
}


// ZKVMExecutionLayerConfigBuilder is a builder for ZKVMExecutionLayerConfig.
type ZKVMExecutionLayerConfigBuilder struct {
	subscribedSubnets   map[executionproof.ExecutionProofSubnetId]struct{}
	minProofsRequired   *int
	generationSubnets   map[executionproof.ExecutionProofSubnetId]struct{}
	proofCacheSize      *int
	proofRequestTimeout *time.Duration
	gossipGracePeriod   *time.Duration
}

// NewZKVMExecutionLayerConfigBuilder creates a new, empty builder.
func NewZKVMExecutionLayerConfigBuilder() *ZKVMExecutionLayerConfigBuilder {
	return &ZKVMExecutionLayerConfigBuilder{
		subscribedSubnets: make(map[executionproof.ExecutionProofSubnetId]struct{}),
		generationSubnets: make(map[executionproof.ExecutionProofSubnetId]struct{}),
	}
}

// SubscribedSubnets sets the entire set of subscribed subnets.
func (b *ZKVMExecutionLayerConfigBuilder) SubscribedSubnets(subnets map[executionproof.ExecutionProofSubnetId]struct{}) *ZKVMExecutionLayerConfigBuilder {
	b.subscribedSubnets = subnets
	return b
}

// AddSubscribedSubnet adds a single subnet to the subscribed set.
func (b *ZKVMExecutionLayerConfigBuilder) AddSubscribedSubnet(subnet executionproof.ExecutionProofSubnetId) *ZKVMExecutionLayerConfigBuilder {
	b.subscribedSubnets[subnet] = struct{}{}
	return b
}

// MinProofsRequired sets the minimum proofs required.
func (b *ZKVMExecutionLayerConfigBuilder) MinProofsRequired(min int) *ZKVMExecutionLayerConfigBuilder {
	b.minProofsRequired = &min
	return b
}

// GenerationSubnets sets the entire set of generation subnets.
func (b *ZKVMExecutionLayerConfigBuilder) GenerationSubnets(subnets map[executionproof.ExecutionProofSubnetId]struct{}) *ZKVMExecutionLayerConfigBuilder {
	b.generationSubnets = subnets
	return b
}

// AddGenerationSubnet adds a single subnet to the generation set.
func (b *ZKVMExecutionLayerConfigBuilder) AddGenerationSubnet(subnet executionproof.ExecutionProofSubnetId) *ZKVMExecutionLayerConfigBuilder {
	b.generationSubnets[subnet] = struct{}{}
	return b
}

// ProofCacheSize sets the proof cache size.
func (b *ZKVMExecutionLayerConfigBuilder) ProofCacheSize(size int) *ZKVMExecutionLayerConfigBuilder {
	b.proofCacheSize = &size
	return b
}

// ProofRequestTimeout sets the proof request timeout.
func (b *ZKVMExecutionLayerConfigBuilder) ProofRequestTimeout(timeout time.Duration) *ZKVMExecutionLayerConfigBuilder {
	b.proofRequestTimeout = &timeout
	return b
}

// GossipGracePeriod sets the gossip grace period.
func (b *ZKVMExecutionLayerConfigBuilder) GossipGracePeriod(period time.Duration) *ZKVMExecutionLayerConfigBuilder {
	b.gossipGracePeriod = &period
	return b
}

// Build constructs the ZKVMExecutionLayerConfig from the builder.
func (b *ZKVMExecutionLayerConfigBuilder) Build() (*ZKVMExecutionLayerConfig, error) {
	minProofs := 1
	if b.minProofsRequired != nil {
		minProofs = *b.minProofsRequired
	}

	cacheSize := 1024
	if b.proofCacheSize != nil {
		cacheSize = *b.proofCacheSize
	}

	timeout := defaultProofRequestTimeout
	if b.proofRequestTimeout != nil {
		timeout = *b.proofRequestTimeout
	}

	gracePeriod := defaultGossipGracePeriod
	if b.gossipGracePeriod != nil {
		gracePeriod = *b.gossipGracePeriod
	}

	config := &ZKVMExecutionLayerConfig{
		SubscribedSubnets:   b.subscribedSubnets,
		MinProofsRequired:   minProofs,
		GenerationSubnets:   b.generationSubnets,
		ProofCacheSize:      cacheSize,
		ProofRequestTimeout: timeout,
		GossipGracePeriod:   gracePeriod,
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}
	return config, nil
}