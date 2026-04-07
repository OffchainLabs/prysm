package integration

import (
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	methodPublishEnvelope     = "/ethereum.eth.v1alpha1.BeaconNodeValidator/PublishExecutionPayloadEnvelope"
	methodSubmitPayloadAttest = "/ethereum.eth.v1alpha1.BeaconNodeValidator/SubmitPayloadAttestation"
)

// SlotRule defines per-slot interception behavior for validator operations.
type SlotRule struct {
	DelayEnvelope time.Duration // Delay before forwarding the envelope.
	DelayPTCVotes time.Duration // Delay before forwarding PTC votes.
	DropEnvelope  bool          // Silently drop the envelope.
	DropPTCVotes  bool          // Suppress payload attestation messages.
}

// ValidatorInterceptor is a transparent gRPC proxy between validators and a
// beacon node. All RPCs are forwarded as-is except for envelope publishing
// and PTC votes, which can be delayed or dropped per-slot.
type ValidatorInterceptor struct {
	t          *testing.T
	listenPort int
	targetAddr string

	rules map[primitives.Slot]*SlotRule
	mu    sync.RWMutex

	rawConn *grpc.ClientConn // Uses rawCodec for transparent byte forwarding.
	server  *grpc.Server
}

// frame holds raw bytes for transparent proxying.
type frame struct{ data []byte }

func (f *frame) Reset()         {}
func (f *frame) String() string { return fmt.Sprintf("%d bytes", len(f.data)) }
func (f *frame) ProtoMessage()  {}

// rawCodec passes bytes through without proto marshalling.
// Used only on the proxy's outbound client connection.
type rawCodec struct{}

func (rawCodec) Marshal(v any) ([]byte, error) {
	if f, ok := v.(*frame); ok {
		return f.data, nil
	}
	return nil, fmt.Errorf("rawCodec: unexpected type %T", v)
}

func (rawCodec) Unmarshal(data []byte, v any) error {
	if f, ok := v.(*frame); ok {
		f.data = data
		return nil
	}
	return fmt.Errorf("rawCodec: unexpected type %T", v)
}

func (rawCodec) Name() string { return "raw-proxy" }

// NewValidatorInterceptor creates a proxy listening on listenPort that
// forwards to the beacon node at targetAddr.
func NewValidatorInterceptor(t *testing.T, listenPort int, targetAddr string) *ValidatorInterceptor {
	return &ValidatorInterceptor{
		t:          t,
		listenPort: listenPort,
		targetAddr: targetAddr,
		rules:      make(map[primitives.Slot]*SlotRule),
	}
}

// SetRule configures interception behavior for a specific slot.
func (vi *ValidatorInterceptor) SetRule(slot primitives.Slot, rule *SlotRule) {
	vi.mu.Lock()
	defer vi.mu.Unlock()
	vi.rules[slot] = rule
}

func (vi *ValidatorInterceptor) getRule(slot primitives.Slot) *SlotRule {
	vi.mu.RLock()
	defer vi.mu.RUnlock()
	return vi.rules[slot]
}

// Start connects to the real beacon node and starts the proxy.
func (vi *ValidatorInterceptor) Start() error {
	rawConn, err := grpc.NewClient(vi.targetAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(rawCodec{})),
	)
	if err != nil {
		return fmt.Errorf("connect to beacon (raw) %s: %w", vi.targetAddr, err)
	}
	vi.rawConn = rawConn

	vi.server = grpc.NewServer(
		grpc.ForceServerCodec(rawCodec{}),
		grpc.UnknownServiceHandler(vi.handler),
	)

	lis, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", vi.listenPort))
	if err != nil {
		return fmt.Errorf("listen on port %d: %w", vi.listenPort, err)
	}

	go func() {
		if err := vi.server.Serve(lis); err != nil {
			vi.t.Logf("validator interceptor stopped: %v", err)
		}
	}()

	vi.t.Logf("Validator interceptor :%d → %s", vi.listenPort, vi.targetAddr)
	return nil
}

// Stop gracefully shuts down the proxy.
func (vi *ValidatorInterceptor) Stop() {
	if vi.server != nil {
		vi.server.GracefulStop()
	}
	if vi.rawConn != nil {
		if err := vi.rawConn.Close(); err != nil {
			vi.t.Logf("validator interceptor conn close: %v", err)
		}
	}
}

// handler routes every gRPC call.
func (vi *ValidatorInterceptor) handler(_ any, serverStream grpc.ServerStream) error {
	method, ok := grpc.MethodFromServerStream(serverStream)
	if !ok {
		return fmt.Errorf("could not get method from stream")
	}

	switch method {
	case methodPublishEnvelope:
		return vi.handleEnvelope(serverStream)
	case methodSubmitPayloadAttest:
		return vi.handlePTCVote(serverStream)
	default:
		return vi.forward(method, serverStream)
	}
}

func (vi *ValidatorInterceptor) handleEnvelope(serverStream grpc.ServerStream) error {
	// Receive raw bytes (server uses rawCodec).
	var raw frame
	if err := serverStream.RecvMsg(&raw); err != nil {
		return err
	}

	// Decode to check slot for interception rules.
	var req ethpb.SignedExecutionPayloadEnvelope
	if err := proto.Unmarshal(raw.data, &req); err != nil {
		return fmt.Errorf("unmarshal envelope: %w", err)
	}

	slot := primitives.Slot(req.Message.Slot)
	if rule := vi.getRule(slot); rule != nil {
		if rule.DropEnvelope {
			vi.t.Logf("[interceptor] Dropping envelope for slot %d", slot)
			resp, _ := proto.Marshal(&emptypb.Empty{})
			return serverStream.SendMsg(&frame{data: resp})
		}
		if rule.DelayEnvelope > 0 {
			vi.t.Logf("[interceptor] Delaying envelope for slot %d by %s", slot, rule.DelayEnvelope)
			time.Sleep(rule.DelayEnvelope)
		}
	}

	// Forward raw bytes to backend via rawConn.
	ctx := serverStream.Context()
	md, _ := metadata.FromIncomingContext(ctx)
	outCtx := metadata.NewOutgoingContext(ctx, md)
	desc := &grpc.StreamDesc{}
	cs, err := vi.rawConn.NewStream(outCtx, desc, methodPublishEnvelope)
	if err != nil {
		return err
	}
	if err := cs.SendMsg(&raw); err != nil {
		return err
	}
	if err := cs.CloseSend(); err != nil {
		return err
	}
	var resp frame
	if err := cs.RecvMsg(&resp); err != nil {
		return err
	}
	return serverStream.SendMsg(&resp)
}

func (vi *ValidatorInterceptor) handlePTCVote(serverStream grpc.ServerStream) error {
	var raw frame
	if err := serverStream.RecvMsg(&raw); err != nil {
		return err
	}

	var req ethpb.PayloadAttestationMessage
	if err := proto.Unmarshal(raw.data, &req); err != nil {
		return fmt.Errorf("unmarshal PTC vote: %w", err)
	}

	slot := req.Data.Slot
	if rule := vi.getRule(slot); rule != nil {
		if rule.DropPTCVotes {
			vi.t.Logf("[interceptor] Dropping PTC vote for slot %d", slot)
			resp, _ := proto.Marshal(&emptypb.Empty{})
			return serverStream.SendMsg(&frame{data: resp})
		}
		if rule.DelayPTCVotes > 0 {
			vi.t.Logf("[interceptor] Delaying PTC vote for slot %d by %s", slot, rule.DelayPTCVotes)
			time.Sleep(rule.DelayPTCVotes)
		}
	}

	ctx := serverStream.Context()
	md, _ := metadata.FromIncomingContext(ctx)
	outCtx := metadata.NewOutgoingContext(ctx, md)
	desc := &grpc.StreamDesc{}
	cs, err := vi.rawConn.NewStream(outCtx, desc, methodSubmitPayloadAttest)
	if err != nil {
		return err
	}
	if err := cs.SendMsg(&raw); err != nil {
		return err
	}
	if err := cs.CloseSend(); err != nil {
		return err
	}
	var resp frame
	if err := cs.RecvMsg(&resp); err != nil {
		return err
	}
	return serverStream.SendMsg(&resp)
}

// forward transparently proxies a gRPC call to the real beacon node.
func (vi *ValidatorInterceptor) forward(method string, serverStream grpc.ServerStream) error {
	ctx := serverStream.Context()
	md, _ := metadata.FromIncomingContext(ctx)
	outCtx := metadata.NewOutgoingContext(ctx, md)

	desc := &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}
	clientStream, err := vi.rawConn.NewStream(outCtx, desc, method)
	if err != nil {
		return err
	}

	errCh := make(chan error, 2)

	// client → server (forward request)
	go func() {
		for {
			f := &frame{}
			if err := serverStream.RecvMsg(f); err != nil {
				if err == io.EOF {
					_ = clientStream.CloseSend()
					errCh <- nil
					return
				}
				errCh <- err
				return
			}
			if err := clientStream.SendMsg(f); err != nil {
				errCh <- err
				return
			}
		}
	}()

	// server → client (forward response)
	go func() {
		// Forward response headers.
		header, err := clientStream.Header()
		if err == nil && len(header) > 0 {
			_ = serverStream.SendHeader(header)
		}
		for {
			f := &frame{}
			if err := clientStream.RecvMsg(f); err != nil {
				if err == io.EOF {
					// Forward trailing metadata.
					serverStream.SetTrailer(clientStream.Trailer())
					errCh <- nil
					return
				}
				errCh <- err
				return
			}
			if err := serverStream.SendMsg(f); err != nil {
				errCh <- err
				return
			}
		}
	}()

	for range 2 {
		if err := <-errCh; err != nil {
			return err
		}
	}
	return nil
}

// interceptorProxyPort returns a dedicated port for the interceptor proxy.
const interceptorProxyOffset = 30

func interceptorProxyPort(index int) int {
	return basePort + index*100 + interceptorProxyOffset
}
