package rpc

import (
	"fmt"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/config/proposer"
	"github.com/OffchainLabs/prysm/v7/consensus-types/validator"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

// local keymanager api
type ListKeystoresResponse struct {
	Data []*Keystore `json:"data"`
}

type Keystore struct {
	ValidatingPubkey string `json:"validating_pubkey"`
	DerivationPath   string `json:"derivation_path"`
}

type ImportKeystoresRequest struct {
	Keystores          []string `json:"keystores"`
	Passwords          []string `json:"passwords"`
	SlashingProtection string   `json:"slashing_protection"`
}

type ImportKeystoresResponse struct {
	Data []*keymanager.KeyStatus `json:"data"`
}

type DeleteKeystoresRequest struct {
	Pubkeys []string `json:"pubkeys"`
}

type DeleteKeystoresResponse struct {
	Data               []*keymanager.KeyStatus `json:"data"`
	SlashingProtection string                  `json:"slashing_protection"`
}

// voluntary exit keymanager api
type SetVoluntaryExitResponse struct {
	Data *structs.SignedVoluntaryExit `json:"data"`
}

// gas limit keymanager api
type GasLimitMetaData struct {
	Pubkey   string `json:"pubkey"`
	GasLimit string `json:"gas_limit"`
}

type GetGasLimitResponse struct {
	Data *GasLimitMetaData `json:"data"`
}

type SetGasLimitRequest struct {
	GasLimit string `json:"gas_limit"`
}

// remote keymanager api
type ListRemoteKeysResponse struct {
	Data []*RemoteKey `json:"data"`
}

type RemoteKey struct {
	Pubkey   string `json:"pubkey"`
	Url      string `json:"url"`
	Readonly bool   `json:"readonly"`
}

type ImportRemoteKeysRequest struct {
	RemoteKeys []*RemoteKey `json:"remote_keys"`
}

type DeleteRemoteKeysRequest struct {
	Pubkeys []string `json:"pubkeys"`
}

type RemoteKeysResponse struct {
	Data []*keymanager.KeyStatus `json:"data"`
}

// Fee Recipient keymanager api
type FeeRecipient struct {
	Pubkey     string `json:"pubkey"`
	Ethaddress string `json:"ethaddress"`
}

type GetFeeRecipientByPubkeyResponse struct {
	Data *FeeRecipient `json:"data"`
}

type SetFeeRecipientByPubkeyRequest struct {
	Ethaddress string `json:"ethaddress"`
}

// Graffiti keymanager api
type GetGraffitiResponse struct {
	Data *GraffitiData `json:"data"`
}

type GraffitiData struct {
	Pubkey   string `json:"pubkey"`
	Graffiti string `json:"graffiti"`
}

type GetBuilderConfigResponse struct {
	Data *BuilderConfig `json:"data"`
}

// BuilderConfig is the keymanager-APIs #88 wire form: integers are decimal
// strings, bytes 0x-hex. Builders nil = inherit, [] = use none (kept by no omitempty).
type BuilderConfig struct {
	MinBid             *string         `json:"min_bid,omitempty"`
	BuilderBoostFactor *string         `json:"builder_boost_factor,omitempty"`
	Builders           []*BuilderEntry `json:"builders"`
}

type BuilderEntry struct {
	Url                 string   `json:"url"`
	AuthData            *string  `json:"auth_data,omitempty"`
	BuilderPubkeys      []string `json:"builder_pubkeys"`
	MaxExecutionPayment *string  `json:"max_execution_payment,omitempty"`
	MinBid              *string  `json:"min_bid,omitempty"`
	BuilderBoostFactor  *string  `json:"builder_boost_factor,omitempty"`
}

func builderConfigFromConsensus(bc *proposer.BuilderConfig) *BuilderConfig {
	out := &BuilderConfig{
		MinBid:             new(strconv.FormatUint(uint64(bc.EffectiveMinBid()), 10)),
		BuilderBoostFactor: new(strconv.FormatUint(uint64(bc.EffectiveBuilderBoostFactor()), 10)),
	}
	if bc.Builders != nil {
		out.Builders = make([]*BuilderEntry, 0, len(bc.Builders))
		for _, b := range bc.Builders {
			out.Builders = append(out.Builders, builderEntryFromConsensus(b, bc))
		}
	}
	return out
}

func builderEntryFromConsensus(be *proposer.BuilderEntry, bc *proposer.BuilderConfig) *BuilderEntry {
	out := &BuilderEntry{
		// Omitted builder_pubkeys resolves to the empty list (accept any builder).
		BuilderPubkeys:      make([]string, 0, len(be.Pubkeys)),
		MaxExecutionPayment: new(strconv.FormatUint(uint64(be.EffectiveMaxExecutionPayment(bc)), 10)),
		MinBid:              new(strconv.FormatUint(uint64(be.EffectiveMinBid(bc)), 10)),
		BuilderBoostFactor:  new(strconv.FormatUint(uint64(be.EffectiveBuilderBoostFactor(bc)), 10)),
	}
	if be.URL != "" {
		out.Url = be.URL
		out.AuthData = new(hexutil.Encode(be.EffectiveAuthData()))
	}
	for _, pk := range be.Pubkeys {
		out.BuilderPubkeys = append(out.BuilderPubkeys, hexutil.Encode(pk))
	}
	return out
}

func (in *BuilderConfig) ToConsensus() (*proposer.BuilderConfig, error) {
	bc := &proposer.BuilderConfig{}
	if in.MinBid != nil {
		v, err := parseUint(*in.MinBid, "min_bid")
		if err != nil {
			return nil, err
		}
		bc.MinBid = &v
	}
	if in.BuilderBoostFactor != nil {
		v, err := parseUint(*in.BuilderBoostFactor, "builder_boost_factor")
		if err != nil {
			return nil, err
		}
		bc.BuilderBoostFactor = &v
	}
	if in.Builders == nil {
		return bc, nil
	}
	if len(in.Builders) > proposer.MaxBuilderEntries {
		return nil, errors.Errorf("builders exceeds %d entries", proposer.MaxBuilderEntries)
	}
	// Non-nil (possibly empty) list means "use exactly these builders", not "inherit".
	// Omitted auth_data compares as its derived value (the url's hostname), so it
	// collides with the explicit form.
	bc.Builders = make([]*proposer.BuilderEntry, 0, len(in.Builders))
	seen := make(map[proposer.EntryIdentity]bool, len(in.Builders))
	for i, entry := range in.Builders {
		if entry == nil {
			return nil, errors.Errorf("builders[%d] is null", i)
		}
		be, err := entry.ToConsensus(i)
		if err != nil {
			return nil, err
		}
		if seen[be.Identity()] {
			return nil, errors.Errorf("builders[%d]: two entries share the same url and auth_data", i)
		}
		seen[be.Identity()] = true
		bc.Builders = append(bc.Builders, be)
	}
	return bc, nil
}

func (in *BuilderEntry) ToConsensus(i int) (*proposer.BuilderEntry, error) {
	be := &proposer.BuilderEntry{URL: in.Url}
	for _, raw := range in.BuilderPubkeys {
		pk, err := hexutil.Decode(raw)
		if err != nil {
			return nil, errors.Errorf("builders[%d].builder_pubkeys contains an invalid BLS public key", i)
		}
		be.Pubkeys = append(be.Pubkeys, pk)
	}
	if in.AuthData != nil {
		ad, err := hexutil.Decode(*in.AuthData)
		if err != nil {
			return nil, errors.Errorf("builders[%d].auth_data is not valid hex", i)
		}
		// An explicit empty auth_data is rejected; omitting the field derives it from the URL.
		if len(ad) == 0 {
			return nil, errors.Errorf("builders[%d].auth_data must be 1 to %d bytes", i, proposer.MaxAuthDataSize)
		}
		be.AuthData = ad
	}
	if err := be.Validate(); err != nil {
		return nil, errors.Wrapf(err, "builders[%d]", i)
	}
	if in.MaxExecutionPayment != nil {
		v, err := parseUint(*in.MaxExecutionPayment, fmt.Sprintf("builders[%d].max_execution_payment", i))
		if err != nil {
			return nil, err
		}
		be.MaxExecutionPayment = &v
	}
	if in.MinBid != nil {
		v, err := parseUint(*in.MinBid, fmt.Sprintf("builders[%d].min_bid", i))
		if err != nil {
			return nil, err
		}
		be.MinBid = &v
	}
	if in.BuilderBoostFactor != nil {
		v, err := parseUint(*in.BuilderBoostFactor, fmt.Sprintf("builders[%d].builder_boost_factor", i))
		if err != nil {
			return nil, err
		}
		be.BuilderBoostFactor = &v
	}
	return be, nil
}

func parseUint(s, field string) (validator.Uint64, error) {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, errors.Errorf("%s is not a valid uint64", field)
	}
	return validator.Uint64(v), nil
}
