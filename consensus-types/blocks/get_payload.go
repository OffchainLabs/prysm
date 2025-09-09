package blocks

import (
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	pb "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// GetPayloadResponse represents the result of unmarshaling an execution engine
// GetPayloadResponseV(1|2|3|4|5) value.
type GetPayloadResponse struct {
	ExecutionData   interfaces.ExecutionData
	BlobsBundler    pb.BlobsBundler
	OverrideBuilder bool
	// todo: should we convert this to Gwei up front?
	Bid               primitives.Wei
	ExecutionRequests *pb.ExecutionRequests
}

// bundleGetter is an interface satisfied by get payload responses that have a blobs bundle.
type bundleGetter interface {
	GetBlobsBundle() *pb.BlobsBundle
}

type bundleV2Getter interface {
	GetBlobsBundle() *pb.BlobsBundleV2
}

// bidValueGetter is an interface satisfied by get payload responses that have a bid value.
type bidValueGetter interface {
	GetValue() []byte
}

type shouldOverrideBuilderGetter interface {
	GetShouldOverrideBuilder() bool
}

type executionRequestsGetter interface {
	GetDecodedExecutionRequests(pb.ExecutionRequestLimits) (*pb.ExecutionRequests, error)
}

func NewGetPayloadResponse(msg proto.Message) (*GetPayloadResponse, error) {
	r := &GetPayloadResponse{}
	bundleGetter, hasBundle := msg.(bundleGetter)
	if hasBundle {
		r.BlobsBundler = bundleGetter.GetBlobsBundle()
	}
	bundleV2Getter, hasBundle := msg.(bundleV2Getter)
	if hasBundle {
		r.BlobsBundler = bundleV2Getter.GetBlobsBundle()
	}
	bidValueGetter, hasBid := msg.(bidValueGetter)
	wei := primitives.ZeroWei()
	if hasBid {
		// The protobuf types that engine api responses unmarshal into store their values in little endian form.
		// This is done for consistency with other uint256 values stored in protobufs for SSZ values.
		// Long term we should move away from protobuf types for these values and just keep the bid as a big.Int as soon
		// as we unmarshal it from the engine api response.
		wei = primitives.LittleEndianBytesToWei(bidValueGetter.GetValue())
	}
	r.Bid = wei
	shouldOverride, hasShouldOverride := msg.(shouldOverrideBuilderGetter)
	if hasShouldOverride {
		r.OverrideBuilder = shouldOverride.GetShouldOverrideBuilder()
	}
	ed, err := NewWrappedExecutionData(msg)
	if err != nil {
		return nil, errors.Wrap(err, "new wrapped execution data")
	}
	r.ExecutionData = ed

	executionRequestsGetter, hasExecutionRequests := msg.(executionRequestsGetter)
	if hasExecutionRequests {
		requests, err := executionRequestsGetter.GetDecodedExecutionRequests(params.BeaconConfig().ExecutionRequestLimits())
		if err != nil {
			return nil, errors.Wrap(err, "get decoded execution requests")
		}
		r.ExecutionRequests = requests
	}
	return r, nil
}
