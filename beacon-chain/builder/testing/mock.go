package testing

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/api/client/builder"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	v1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
)

// Config defines a config struct for dependencies into the service.
type Config struct {
	BeaconDB db.HeadAccessDatabase
}

// MockBuilderService to mock builder.
type MockBuilderService struct {
	HasConfigured                 bool
	Payload                       *v1.ExecutionPayload
	PayloadCapella                *v1.ExecutionPayloadCapella
	PayloadDeneb                  *v1.ExecutionPayloadDeneb
	BlobBundle                    *v1.BlobsBundle
	BlobBundleV2                  *v1.BlobsBundleV2
	ErrSubmitBlindedBlock         error
	ErrSubmitBlindedBlockPostFulu error
	Bid                           *ethpb.SignedBuilderBid
	BidCapella                    *ethpb.SignedBuilderBidCapella
	BidDeneb                      *ethpb.SignedBuilderBidDeneb
	BidElectra                    *ethpb.SignedBuilderBidElectra
	RegistrationCache             *cache.RegistrationCache
	ErrGetHeader                  error
	ErrRegisterValidator          error
	Cfg                           *Config
}

// Configured for mocking.
func (s *MockBuilderService) Configured() bool {
	return s.HasConfigured
}

// SubmitBlindedBlock for mocking.
func (s *MockBuilderService) SubmitBlindedBlock(_ context.Context, b interfaces.ReadOnlySignedBeaconBlock) (interfaces.ExecutionData, v1.BlobsBundler, error) {
	switch b.Version() {
	case version.Bellatrix:
		w, err := blocks.WrappedExecutionPayload(s.Payload)
		if err != nil {
			return nil, nil, errors.Wrap(err, "could not wrap payload")
		}
		return w, nil, s.ErrSubmitBlindedBlock
	case version.Capella:
		w, err := blocks.WrappedExecutionPayloadCapella(s.PayloadCapella)
		if err != nil {
			return nil, nil, errors.Wrap(err, "could not wrap capella payload")
		}
		return w, nil, s.ErrSubmitBlindedBlock
	case version.Deneb, version.Electra:
		w, err := blocks.WrappedExecutionPayloadDeneb(s.PayloadDeneb)
		if err != nil {
			return nil, nil, errors.Wrap(err, "could not wrap deneb payload")
		}
		return w, s.BlobBundle, s.ErrSubmitBlindedBlock
	case version.Fulu:
		w, err := blocks.WrappedExecutionPayloadDeneb(s.PayloadDeneb)
		if err != nil {
			return nil, nil, errors.Wrap(err, "could not wrap deneb payload for fulu")
		}
		// For Fulu, return BlobsBundleV2 if available, otherwise regular BlobsBundle
		if s.BlobBundleV2 != nil {
			return w, s.BlobBundleV2, s.ErrSubmitBlindedBlock
		}
		return w, s.BlobBundle, s.ErrSubmitBlindedBlock
	default:
		return nil, nil, errors.New("unknown block version for mocking")
	}
}

// GetHeader for mocking.
func (s *MockBuilderService) GetHeader(_ context.Context, slot primitives.Slot, _ [32]byte, _ [48]byte) (builder.SignedBid, error) {
	if slots.ToEpoch(slot) >= params.BeaconConfig().ElectraForkEpoch || s.BidElectra != nil {
		return builder.WrappedSignedBuilderBidElectra(s.BidElectra)
	}
	if slots.ToEpoch(slot) >= params.BeaconConfig().DenebForkEpoch || s.BidDeneb != nil {
		return builder.WrappedSignedBuilderBidDeneb(s.BidDeneb)
	}
	if slots.ToEpoch(slot) >= params.BeaconConfig().CapellaForkEpoch || s.BidCapella != nil {
		return builder.WrappedSignedBuilderBidCapella(s.BidCapella)
	}
	w, err := builder.WrappedSignedBuilderBid(s.Bid)
	if err != nil {
		return nil, errors.Wrap(err, "could not wrap capella bid")
	}
	return w, s.ErrGetHeader
}

// RegistrationByValidatorID returns either the values from the cache or db.
func (s *MockBuilderService) RegistrationByValidatorID(ctx context.Context, id primitives.ValidatorIndex) (*ethpb.ValidatorRegistrationV1, error) {
	if s.RegistrationCache != nil {
		return s.RegistrationCache.RegistrationByIndex(id)
	}
	if s.Cfg.BeaconDB != nil {
		return s.Cfg.BeaconDB.RegistrationByValidatorID(ctx, id)
	}
	return nil, cache.ErrNotFoundRegistration
}

// RegisterValidator for mocking.
func (s *MockBuilderService) RegisterValidator(context.Context, []*ethpb.SignedValidatorRegistrationV1) error {
	return s.ErrRegisterValidator
}

// SubmitBlindedBlockPostFulu for mocking.
func (s *MockBuilderService) SubmitBlindedBlockPostFulu(_ context.Context, _ interfaces.ReadOnlySignedBeaconBlock) error {
	return s.ErrSubmitBlindedBlockPostFulu
}
