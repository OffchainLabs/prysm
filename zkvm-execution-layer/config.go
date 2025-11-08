package zkvmexecutionlayer

import (
	"errors"
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
	// MinProofsRequired is the minimum number of proofs required from _different_ subnets
	// in order for the node to mark an execution payload as VALID.
	MinProofsRequired int `json:"min_proofs_required"`

	// GenerationProofTypes are the subnets to generate proofs for (empty if not generating proofs).
	GenerationProofTypes map[executionproof.ExecutionProofId]struct{} `json:"generation_subnets"`

	// ProofCacheSize is the proof cache size (number of execution block hashes to cache proofs for).
	ProofCacheSize int `json:"proof_cache_size"`
}

// NewDefaultZKVMExecutionLayerConfig creates a new config with default values.
func NewDefaultZKVMExecutionLayerConfig() *ZKVMExecutionLayerConfig {
	return &ZKVMExecutionLayerConfig{
		MinProofsRequired:   1,
		GenerationProofTypes: make(map[executionproof.ExecutionProofId]struct{}),
		ProofCacheSize:      64 * 8,
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

	return nil
}


// ZKVMExecutionLayerConfigBuilder is a builder for ZKVMExecutionLayerConfig.
type ZKVMExecutionLayerConfigBuilder struct {
	minProofsRequired   *int
	generationProofTypes   map[executionproof.ExecutionProofId]struct{}
	proofCacheSize      *int
}

// NewZKVMExecutionLayerConfigBuilder creates a new, empty builder.
func NewZKVMExecutionLayerConfigBuilder() *ZKVMExecutionLayerConfigBuilder {
	return &ZKVMExecutionLayerConfigBuilder{
		generationProofTypes: make(map[executionproof.ExecutionProofId]struct{}),
	}
}

// MinProofsRequired sets the minimum proofs required.
func (b *ZKVMExecutionLayerConfigBuilder) MinProofsRequired(min int) *ZKVMExecutionLayerConfigBuilder {
	b.minProofsRequired = &min
	return b
}

// GenerationSubnets sets the entire set of generation subnets.
func (b *ZKVMExecutionLayerConfigBuilder) GenerationSubnets(subnets map[executionproof.ExecutionProofId]struct{}) *ZKVMExecutionLayerConfigBuilder {
	b.generationProofTypes = subnets
	return b
}

// AddGenerationSubnet adds a single subnet to the generation set.
func (b *ZKVMExecutionLayerConfigBuilder) AddGenerationSubnet(subnet executionproof.ExecutionProofId) *ZKVMExecutionLayerConfigBuilder {
	b.generationProofTypes[subnet] = struct{}{}
	return b
}

// ProofCacheSize sets the proof cache size.
func (b *ZKVMExecutionLayerConfigBuilder) ProofCacheSize(size int) *ZKVMExecutionLayerConfigBuilder {
	b.proofCacheSize = &size
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

	config := &ZKVMExecutionLayerConfig{
		MinProofsRequired:   minProofs,
		GenerationProofTypes:   b.generationProofTypes,
		ProofCacheSize:      cacheSize,
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}
	return config, nil
}