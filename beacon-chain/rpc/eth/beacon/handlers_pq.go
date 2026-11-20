package beacon

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func writeGRPCErrorPQ(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	msg := err.Error()
	if st, ok := status.FromError(err); ok {
		msg = st.Message()
		switch st.Code() {
		case codes.NotFound:
			code = http.StatusNotFound
		case codes.InvalidArgument, codes.FailedPrecondition:
			code = http.StatusBadRequest
		case codes.Unavailable:
			code = http.StatusServiceUnavailable
		}
	}
	httputil.HandleError(w, msg, code)
}

func decodePQJSON(body []byte) (*eth.GenericSignedBeaconBlock, error) {
	block := &structs.SignedBeaconBlockPQ{}
	if err := unmarshalStrict(body, block); err != nil {
		return nil, decodingError(version.String(version.PQ), err)
	}
	consensus, err := block.ToConsensus()
	if err != nil {
		return nil, decodingError(version.String(version.PQ), err)
	}
	return &eth.GenericSignedBeaconBlock{Block: &eth.GenericSignedBeaconBlock_Pq{Pq: consensus}}, nil
}

func (s *Server) submitAttestationsPQ(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	var (
		atts     []*eth.SingleAttestationPQ
		failures []*server.IndexedError
		err      error
	)
	if httputil.IsRequestSsz(r) {
		atts, failures, err = server.DecodeSSZList[eth.SingleAttestationPQ](r.Body)
	} else {
		atts, failures, err = server.DecodeJSONList(r.Body, (*structs.SingleAttestation).ToConsensusPQ)
	}
	if err != nil {
		writeDecodeError(w, err)
		return
	}
	if len(atts) == 0 && len(failures) == 0 {
		httputil.HandleError(w, "no data submitted", http.StatusBadRequest)
		return
	}
	for i, att := range atts {
		if att == nil {
			continue
		}
		if _, err := s.V1Alpha1ValidatorServer.ProposeAttestationPQ(ctx, att); err != nil {
			failures = append(failures, &server.IndexedError{Index: i, Message: err.Error()})
		}
	}
	if len(failures) > 0 {
		httputil.WriteError(w, &server.IndexedErrorContainer{
			Code:     http.StatusBadRequest,
			Message:  server.ErrIndexedValidationFail,
			Failures: failures,
		})
	}
}

// SSZ container of a signed PQ envelope, the cell proofs and the blobs, as Lighthouse sends it.
func decodeEnvelopeContentsPQSSZ(body []byte) (*eth.SignedExecutionPayloadEnvelopePQ, [][]byte, [][]byte, error) {
	if len(body) < 12 {
		return nil, nil, nil, errors.New("envelope contents too short")
	}
	envelopeStart := uint64(binary.LittleEndian.Uint32(body[0:4]))
	proofsStart := uint64(binary.LittleEndian.Uint32(body[4:8]))
	blobsStart := uint64(binary.LittleEndian.Uint32(body[8:12]))
	if envelopeStart != 12 || proofsStart < envelopeStart || blobsStart < proofsStart || blobsStart > uint64(len(body)) {
		return nil, nil, nil, errors.New("invalid envelope contents offsets")
	}
	signed := &eth.SignedExecutionPayloadEnvelopePQ{}
	if err := signed.UnmarshalSSZ(body[envelopeStart:proofsStart]); err != nil {
		return nil, nil, nil, err
	}
	proofs, err := splitFixed(body[proofsStart:blobsStart], fieldparams.BLSPubkeyLength)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "kzg proofs")
	}
	blobs, err := splitFixed(body[blobsStart:], fieldparams.BlobLength)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "blobs")
	}
	return signed, proofs, blobs, nil
}

func splitFixed(b []byte, size int) ([][]byte, error) {
	if len(b)%size != 0 {
		return nil, errors.Errorf("length %d is not a multiple of %d", len(b), size)
	}
	out := make([][]byte, 0, len(b)/size)
	for i := 0; i < len(b); i += size {
		out = append(out, b[i:i+size])
	}
	return out, nil
}

func (s *Server) publishEnvelopePQ(ctx context.Context, w http.ResponseWriter, r *http.Request, body []byte, blobDataIncluded bool) {
	var (
		signed           *eth.SignedExecutionPayloadEnvelopePQ
		kzgProofs, blobs [][]byte
		err              error
	)
	switch {
	case httputil.IsRequestSsz(r) && blobDataIncluded:
		signed, kzgProofs, blobs, err = decodeEnvelopeContentsPQSSZ(body)
	case httputil.IsRequestSsz(r):
		signed = &eth.SignedExecutionPayloadEnvelopePQ{}
		err = signed.UnmarshalSSZ(body)
	case blobDataIncluded:
		contents := &structs.SignedExecutionPayloadEnvelopeContentsPQ{}
		if err = json.Unmarshal(body, contents); err == nil {
			signed, kzgProofs, blobs, err = contents.ToConsensus()
		}
	default:
		envelope := &structs.SignedExecutionPayloadEnvelopePQ{}
		if err = json.Unmarshal(body, envelope); err == nil {
			signed, err = envelope.ToConsensus()
		}
	}
	if err != nil {
		httputil.HandleError(w, "could not decode PQ envelope: "+err.Error(), http.StatusBadRequest)
		return
	}
	if blobDataIncluded {
		err = s.PQValidator.PublishExecutionPayloadEnvelopeContentsPQ(ctx, signed, kzgProofs, blobs)
	} else {
		_, err = s.V1Alpha1ValidatorServer.PublishExecutionPayloadEnvelopePQ(ctx, signed)
	}
	if err != nil {
		writeGRPCErrorPQ(w, err)
	}
}

// The shared record has no PQ requests or signature, those come from the PQ record.
func (s *Server) envelopePQ(ctx context.Context, root [32]byte) (*eth.SignedExecutionPayloadEnvelopePQ, error) {
	blinded, err := s.BeaconDB.ExecutionPayloadEnvelope(ctx, root)
	if err != nil {
		return nil, err
	}
	full, err := s.ExecutionReconstructor.ReconstructExecutionPayloadEnvelope(ctx, blinded)
	if err != nil {
		return nil, err
	}
	record, err := s.BeaconDB.ExecutionPayloadEnvelopePQ(ctx, root)
	if err != nil {
		return nil, err
	}
	return &eth.SignedExecutionPayloadEnvelopePQ{
		Message: &eth.ExecutionPayloadEnvelopePQ{
			Payload:               full.Message.Payload,
			ExecutionRequests:     record.Message.ExecutionRequests,
			BuilderIndex:          record.Message.BuilderIndex,
			BeaconBlockRoot:       record.Message.BeaconBlockRoot,
			ParentBeaconBlockRoot: record.Message.ParentBeaconBlockRoot,
		},
		Signature: record.Signature,
	}, nil
}

func (s *Server) writeEnvelopePQ(ctx context.Context, w http.ResponseWriter, r *http.Request, root [32]byte) {
	signed, err := s.envelopePQ(ctx, root)
	if err != nil {
		httputil.HandleError(w, "could not retrieve execution payload envelope: "+err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set(api.VersionHeader, version.String(version.PQ))
	if httputil.RespondWithSsz(r) {
		sszBytes, err := signed.MarshalSSZ()
		if err != nil {
			httputil.HandleError(w, "could not marshal envelope to SSZ: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httputil.WriteSsz(w, sszBytes)
		return
	}
	isOptimistic, err := s.OptimisticModeFetcher.IsOptimisticForRoot(ctx, root)
	if err != nil {
		httputil.HandleError(w, "could not check optimistic status: "+err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := structs.SignedExecutionPayloadEnvelopePQFromConsensus(signed)
	if err != nil {
		httputil.HandleError(w, "could not convert envelope to JSON: "+err.Error(), http.StatusInternalServerError)
		return
	}
	httputil.WriteJson(w, &structs.GetExecutionPayloadEnvelopeResponsePQ{
		Version:             version.String(version.PQ),
		ExecutionOptimistic: isOptimistic,
		Finalized:           s.FinalizationFetcher.IsFinalized(ctx, root),
		Data:                data,
	})
}

// The registry stores XMSS keys with a zero prefix, the API shows the 32 byte key and the VRF root.
func setValidatorPQFields(st state.ReadOnlyBeaconState, c *structs.ValidatorContainer, idx primitives.ValidatorIndex) {
	if st.Version() < version.PQ {
		return
	}
	pubkey := st.PubkeyAtIndex(idx)
	c.Validator.Pubkey = hexutil.Encode(pubkey[stateutil.XMSSPubkeyPrefixLength:])
	if root, err := st.VrfRootAtIndex(idx); err == nil {
		c.Validator.VrfRoot = hexutil.Encode(root[:])
	}
}

// PQ blocks carry no payload attestations, so messages go to gossip and fork choice only.
func (s *Server) submitPayloadAttestationsPQ(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	var (
		msgs     []*eth.PayloadAttestationMessagePQ
		failures []*server.IndexedError
		err      error
	)
	if httputil.IsRequestSsz(r) {
		msgs, failures, err = server.DecodeSSZList[eth.PayloadAttestationMessagePQ](r.Body)
	} else {
		msgs, failures, err = server.DecodeJSONList(r.Body, (*structs.PayloadAttestationMessage).ToConsensusPQ)
	}
	if err != nil {
		writeDecodeError(w, err)
		return
	}
	if len(msgs) == 0 && len(failures) == 0 {
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	}
	currentSlot := s.TimeFetcher.CurrentSlot()
	for i, msg := range msgs {
		if msg == nil || msg.Data == nil {
			continue
		}
		if err := s.submitPayloadAttestationPQ(ctx, msg, currentSlot); err != nil {
			failures = append(failures, &server.IndexedError{Index: i, Message: err.Error()})
		}
	}
	if len(failures) > 0 {
		httputil.WriteError(w, &server.IndexedErrorContainer{
			Code:     http.StatusBadRequest,
			Message:  server.ErrIndexedValidationFail,
			Failures: failures,
		})
	}
}

func (s *Server) submitPayloadAttestationPQ(ctx context.Context, msg *eth.PayloadAttestationMessagePQ, currentSlot primitives.Slot) error {
	if msg.Data.Slot != currentSlot {
		return errors.Errorf("payload attestation slot %d does not match current slot %d", msg.Data.Slot, currentSlot)
	}
	st, err := s.PayloadAttestationReceiver.PtcLookupState(ctx, bytesutil.ToBytes32(msg.Data.BeaconBlockRoot), msg.Data.Slot)
	if err != nil {
		return errors.Wrap(err, "could not find state for payload attestation")
	}
	if st == nil {
		return errors.New("could not find state for payload attestation")
	}
	if _, err := gloas.PayloadCommitteeIndices(ctx, st, msg.Data.Slot, msg.ValidatorIndex); err != nil {
		return errors.Wrap(err, "could not determine PTC committee index")
	}
	if err := pq.VerifyPayloadAttestationMessage(st, msg); err != nil {
		return errors.Wrap(err, "incorrect payload attestation signature")
	}
	if err := s.Broadcaster.Broadcast(ctx, msg); err != nil {
		return errors.Wrap(err, "could not broadcast payload attestation")
	}
	// Fork choice reads only the index and data, so a BLS shaped copy carries them.
	return s.PayloadAttestationReceiver.ReceivePayloadAttestationMessage(ctx, &eth.PayloadAttestationMessage{
		ValidatorIndex: msg.ValidatorIndex,
		Data:           msg.Data,
		Signature:      make([]byte, fieldparams.BLSSignatureLength),
	})
}
