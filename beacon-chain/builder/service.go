package builder

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/api/client/builder"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	v1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// ErrNoBuilder is used when builder endpoint is not configured.
var ErrNoBuilder = errors.New("builder endpoint not configured")

// BlockBuilder defines the interface for interacting with the block builder
type BlockBuilder interface {
	SubmitBlindedBlock(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock) (interfaces.ExecutionData, v1.BlobsBundler, error)
	SubmitBlindedBlockPostFulu(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock) error
	GetHeader(ctx context.Context, slot primitives.Slot, parentHash [32]byte, pubKey [48]byte) (builder.SignedBid, error)
	GetExecutionPayloadBid(ctx context.Context, slot primitives.Slot, parentHash, parentRoot [32]byte, proposerPubkey [48]byte, auths []*ethpb.SignedRequestAuthV1) ([]PayloadBid, error)
	SubmitSignedBeaconBlock(ctx context.Context, builderURL string, block interfaces.ReadOnlySignedBeaconBlock) error
	SubmitBuilderPreferences(ctx context.Context, validatorPubkey [48]byte, req *ethpb.BuilderPreferencesRequestV1) error
	RegisterValidator(ctx context.Context, reg []*ethpb.SignedValidatorRegistrationV1) error
	RegistrationByValidatorID(ctx context.Context, id primitives.ValidatorIndex) (*ethpb.ValidatorRegistrationV1, error)
	Configured() bool
}

// PayloadBid pairs a builder's signed execution payload bid with the URL it came
// from, so the proposer can route the revealed block back to the winning builder.
type PayloadBid struct {
	BuilderURL string
	Bid        *ethpb.SignedExecutionPayloadBid
}

// config defines a config struct for dependencies into the service.
type config struct {
	builderClient builder.BuilderClient
	beaconDB      db.HeadAccessDatabase
	headFetcher   blockchain.HeadFetcher
}

// Service defines a service that provides a client for interacting with the beacon chain and MEV relay network.
type Service struct {
	cfg               *config
	c                 builder.BuilderClient
	ctx               context.Context
	cancel            context.CancelFunc
	registrationCache *cache.RegistrationCache
	clientOpts        []builder.ClientOpt
	// dial builds a per-URL builder client; overridable in tests.
	dial func(url string) (builder.BuilderClient, error)
	// clients maps a builder URL to its client. For Gloas the builder set is
	// driven by the validator-signed request auths, so clients are dialed lazily
	// per URL rather than from a single endpoint flag.
	clients   map[string]builder.BuilderClient
	clientsMu sync.RWMutex
}

// NewService instantiates a new service.
func NewService(ctx context.Context, opts ...Option) (*Service, error) {
	ctx, cancel := context.WithCancel(ctx)
	s := &Service{
		ctx:     ctx,
		cancel:  cancel,
		cfg:     &config{},
		clients: make(map[string]builder.BuilderClient),
	}
	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}
	if s.dial == nil {
		s.dial = func(url string) (builder.BuilderClient, error) {
			return builder.NewClient(url, s.clientOpts...)
		}
	}
	if s.cfg.builderClient != nil && !reflect.ValueOf(s.cfg.builderClient).IsNil() {
		s.c = s.cfg.builderClient
		s.clients[s.c.NodeURL()] = s.c

		// Is the builder up?
		if err := s.c.Status(ctx); err != nil {
			log.WithError(err).Error("Failed to check builder status")
		} else {
			log.WithField("endpoint", s.c.NodeURL()).Info("Builder has been configured")
			log.Warn("Outsourcing block construction to external builders adds non-trivial delay to block propagation time. " +
				"Builder-constructed blocks or fallback blocks may get orphaned. Use at your own risk!")
		}
	}
	return s, nil
}

// clientFor returns the builder client for url, lazily dialing and caching one
// (matching the configured client options) if not already present.
func (s *Service) clientFor(url string) (builder.BuilderClient, error) {
	s.clientsMu.RLock()
	c, ok := s.clients[url]
	s.clientsMu.RUnlock()
	if ok {
		return c, nil
	}
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	if c, ok := s.clients[url]; ok {
		return c, nil
	}
	c, err := s.dial(url)
	if err != nil {
		return nil, errors.Wrapf(err, "could not create builder client for %s", url)
	}
	if s.clients == nil {
		s.clients = make(map[string]builder.BuilderClient)
	}
	s.clients[url] = c
	return c, nil
}

// Start initializes the service.
func (s *Service) Start() {
	go s.pollRelayerStatus(s.ctx)
}

// Stop halts the service.
func (s *Service) Stop() error {
	s.cancel()
	return nil
}

// SubmitBlindedBlock submits a blinded block to the builder relay network.
func (s *Service) SubmitBlindedBlock(ctx context.Context, b interfaces.ReadOnlySignedBeaconBlock) (interfaces.ExecutionData, v1.BlobsBundler, error) {
	ctx, span := trace.StartSpan(ctx, "builder.SubmitBlindedBlock")
	defer span.End()
	start := time.Now()
	defer func() {
		submitBlindedBlockLatency.Observe(float64(time.Since(start).Milliseconds()))
	}()
	if s.c == nil {
		return nil, nil, ErrNoBuilder
	}

	return s.c.SubmitBlindedBlock(ctx, b)
}

// SubmitBlindedBlockPostFulu submits a blinded block to the builder relay network post-Fulu.
// After Fulu, relays only return status codes (no payload).
func (s *Service) SubmitBlindedBlockPostFulu(ctx context.Context, b interfaces.ReadOnlySignedBeaconBlock) error {
	ctx, span := trace.StartSpan(ctx, "builder.SubmitBlindedBlockPostFulu")
	defer span.End()
	start := time.Now()
	defer func() {
		submitBlindedBlockLatency.Observe(float64(time.Since(start).Milliseconds()))
	}()
	if s.c == nil {
		return ErrNoBuilder
	}

	return s.c.SubmitBlindedBlockPostFulu(ctx, b)
}

// GetExecutionPayloadBid requests a bid from every builder the proposer signed a
// request auth for, querying them concurrently and returning each non-empty bid
// tagged with the builder it came from.
func (s *Service) GetExecutionPayloadBid(ctx context.Context, slot primitives.Slot, parentHash, parentRoot [32]byte, proposerPubkey [48]byte, auths []*ethpb.SignedRequestAuthV1) ([]PayloadBid, error) {
	ctx, span := trace.StartSpan(ctx, "builder.GetExecutionPayloadBid")
	defer span.End()

	byURL := make(map[string]*ethpb.SignedRequestAuthV1, len(auths))
	urls := make([]string, 0, len(auths))
	for _, a := range auths {
		url := string(a.GetMessage().GetData())
		if url == "" {
			continue
		}
		if _, ok := byURL[url]; !ok {
			byURL[url] = a
			urls = append(urls, url)
		}
	}
	if len(urls) == 0 {
		return nil, nil
	}

	var (
		mu   sync.Mutex
		bids []PayloadBid
		wg   sync.WaitGroup
	)
	for _, url := range urls {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			c, err := s.clientFor(url)
			if err != nil {
				log.WithError(err).WithField("builder", url).Warn("Could not get builder client")
				return
			}
			bid, err := c.GetExecutionPayloadBid(ctx, slot, parentHash, parentRoot, proposerPubkey, byURL[url])
			if err != nil {
				log.WithError(err).WithField("builder", url).Warn("Could not get builder execution payload bid")
				return
			}
			if bid == nil {
				return
			}
			mu.Lock()
			bids = append(bids, PayloadBid{BuilderURL: url, Bid: bid})
			mu.Unlock()
		}(url)
	}
	wg.Wait()
	return bids, nil
}

// SubmitSignedBeaconBlock sends a signed Gloas beacon block to the winning builder so it can reveal the envelope.
func (s *Service) SubmitSignedBeaconBlock(ctx context.Context, builderURL string, b interfaces.ReadOnlySignedBeaconBlock) error {
	ctx, span := trace.StartSpan(ctx, "builder.SubmitSignedBeaconBlock")
	defer span.End()
	if builderURL == "" {
		tracing.AnnotateError(span, ErrNoBuilder)
		return ErrNoBuilder
	}
	c, err := s.clientFor(builderURL)
	if err != nil {
		tracing.AnnotateError(span, err)
		return err
	}
	return c.SubmitSignedBeaconBlock(ctx, b)
}

// SubmitBuilderPreferences submits a proposer's preferences to the builder named
// in the signed request auth, dialing a client for it if not already known.
func (s *Service) SubmitBuilderPreferences(ctx context.Context, validatorPubkey [48]byte, req *ethpb.BuilderPreferencesRequestV1) error {
	ctx, span := trace.StartSpan(ctx, "builder.SubmitBuilderPreferences")
	defer span.End()
	url := string(req.GetAuth().GetMessage().GetData())
	if url == "" {
		return errors.New("builder preferences missing builder url")
	}
	c, err := s.clientFor(url)
	if err != nil {
		tracing.AnnotateError(span, err)
		return err
	}
	return c.SubmitBuilderPreferences(ctx, validatorPubkey, req)
}

// GetHeader retrieves the header for a given slot and parent hash from the builder relay network.
func (s *Service) GetHeader(ctx context.Context, slot primitives.Slot, parentHash [32]byte, pubKey [48]byte) (builder.SignedBid, error) {
	ctx, span := trace.StartSpan(ctx, "builder.GetHeader")
	defer span.End()
	start := time.Now()
	defer func() {
		getHeaderLatency.Observe(float64(time.Since(start).Milliseconds()))
	}()
	if s.c == nil {
		tracing.AnnotateError(span, ErrNoBuilder)
		return nil, ErrNoBuilder
	}

	h, err := s.c.GetHeader(ctx, slot, parentHash, pubKey)
	tracing.AnnotateError(span, err)
	return h, err
}

// Status retrieves the status of the builder relay network.
func (s *Service) Status() error {
	// Return early if builder isn't initialized in service.
	if s.c == nil {
		return nil
	}

	return nil
}

// RegisterValidator registers a validator with the builder relay network.
// It also saves the registration object to the DB.
func (s *Service) RegisterValidator(ctx context.Context, reg []*ethpb.SignedValidatorRegistrationV1) error {
	ctx, span := trace.StartSpan(ctx, "builder.RegisterValidator")
	defer span.End()
	start := time.Now()
	defer func() {
		registerValidatorLatency.Observe(float64(time.Since(start).Milliseconds()))
	}()
	if s.c == nil {
		return ErrNoBuilder
	}

	// should be removed if db is removed
	idxs := make([]primitives.ValidatorIndex, 0)
	msgs := make([]*ethpb.ValidatorRegistrationV1, 0)

	indexToRegistration := make(map[primitives.ValidatorIndex]*ethpb.ValidatorRegistrationV1)

	valid := make([]*ethpb.SignedValidatorRegistrationV1, 0)
	for i := range reg {
		r := reg[i]
		nx, exists := s.cfg.headFetcher.HeadPublicKeyToValidatorIndex(bytesutil.ToBytes48(r.Message.Pubkey))
		if !exists {
			// we want to allow validators to set up keys that haven't been added to the beaconstate validator list yet,
			// so we should tolerate keys that do not seem to be valid by skipping past them.
			log.Warnf("Skipping validator registration for pubkey=%#x - not in current validator set.", r.Message.Pubkey)
			continue
		}
		idxs = append(idxs, nx)
		msgs = append(msgs, r.Message)
		valid = append(valid, r)
		indexToRegistration[nx] = r.Message
	}
	if err := s.c.RegisterValidator(ctx, valid); err != nil {
		return errors.Wrap(err, "could not register validator(s)")
	}

	if len(indexToRegistration) != len(msgs) {
		return errors.New("ids and registrations must be the same length")
	}
	if s.registrationCache != nil {
		s.registrationCache.UpdateIndexToRegisteredMap(ctx, indexToRegistration)
		return nil
	} else {
		return s.cfg.beaconDB.SaveRegistrationsByValidatorIDs(ctx, idxs, msgs)
	}
}

// RegistrationByValidatorID returns either the values from the cache or db.
func (s *Service) RegistrationByValidatorID(ctx context.Context, id primitives.ValidatorIndex) (*ethpb.ValidatorRegistrationV1, error) {
	if s.registrationCache != nil {
		return s.registrationCache.RegistrationByIndex(id)
	} else {
		if s.cfg == nil || s.cfg.beaconDB == nil {
			return nil, errors.New("nil beacon db")
		}
		return s.cfg.beaconDB.RegistrationByValidatorID(ctx, id)
	}
}

// Configured returns true if the user has configured a builder client.
func (s *Service) Configured() bool {
	return s.c != nil && !reflect.ValueOf(s.c).IsNil()
}

func (s *Service) pollRelayerStatus(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if s.c != nil {
				if err := s.c.Status(ctx); err != nil {
					log.WithError(err).Error("Failed to call relayer status endpoint, perhaps mev-boost or relayers are down")
				}
			}
		case <-ctx.Done():
			return
		}
	}
}
