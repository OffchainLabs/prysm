package blocks

import (
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	pb "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	"google.golang.org/protobuf/proto"
)

// GetPayloadResponse represents the result of unmarshaling an execution engine
// GetPayloadResponseV(1|2|3|4) value.
type GetPayloadResponse struct {
	ExecutionData   interfaces.ExecutionData
	BlobsBundle     *pb.BlobsBundle
	OverrideBuilder bool
	// todo: should we convert this to Gwei up front?
	Bid               primitives.Wei
	ExecutionRequests *pb.ExecutionRequests
}

// bundleGetter is an interface satisfied by get payload responses that have a blobs bundle.
type bundleGetter interface {
	GetBlobsBundle() *pb.BlobsBundle
}

// bidValueGetter is an interface satisfied by get payload responses that have a bid value.
type bidValueGetter interface {
	GetValue() []byte
}

type shouldOverrideBuilderGetter interface {
	GetShouldOverrideBuilder() bool
}

type executionRequestsGetter interface {
	GetDecodedExecutionRequests() (*pb.ExecutionRequests, error)
}

func NewGetPayloadResponse(msg proto.Message) (*GetPayloadResponse, error) {
	r := &GetPayloadResponse{}
	bundleGetter, hasBundle := msg.(bundleGetter)
	if hasBundle {
		r.BlobsBundle = bundleGetter.GetBlobsBundle()
	}
	bidValueGetter, hasBid := msg.(bidValueGetter)
	executionRequestsGetter, hasExecutionRequests := msg.(executionRequestsGetter)
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
		return nil, err
	}
	r.ExecutionData = ed
	if hasExecutionRequests {
		requests, err := executionRequestsGetter.GetDecodedExecutionRequests()
		if err != nil {
			return nil, err
		}
		r.ExecutionRequests = requests
	}
	return r, nil
}
