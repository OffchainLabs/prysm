package builder

import (
	"bytes"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	builderClient "github.com/OffchainLabs/prysm/v7/api/client/builder"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	v1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	gethRPC "github.com/ethereum/go-ethereum/rpc"
	"github.com/sirupsen/logrus"
)

type testEngineService struct {
	payload  *engine.ExecutableData
	envelope *engine.ExecutionPayloadEnvelope
}

func (s *testEngineService) GetPayloadV1(v1.PayloadIDBytes) (*engine.ExecutableData, error) {
	return s.payload, nil
}

func (s *testEngineService) GetPayloadV2(v1.PayloadIDBytes) (*engine.ExecutionPayloadEnvelope, error) {
	return s.envelope, nil
}

func (s *testEngineService) GetPayloadV3(v1.PayloadIDBytes) (*engine.ExecutionPayloadEnvelope, error) {
	return s.envelope, nil
}

func (s *testEngineService) GetPayloadV4(v1.PayloadIDBytes) (*engine.ExecutionPayloadEnvelope, error) {
	return s.envelope, nil
}

func TestGetHeaderBaselineJSON(t *testing.T) {
	builder, server, _ := headerTestServer(t)
	client, err := builderClient.NewClient(server.URL)
	require.NoError(t, err)

	bid, err := client.GetHeader(t.Context(), 1, [32]byte{1}, [48]byte{2})

	require.NoError(t, err)
	require.NotNil(t, bid)
	require.Equal(t, version.Bellatrix, builder.currVersion)
	t.Logf("endpoint=%s fork=bellatrix accept=%s status=%d content_type=%s version=%s decoded_type=%T", headerPath, api.JsonMediaType, http.StatusOK, api.JsonMediaType, "bellatrix", bid)
}

func TestHandleBlindedBlockBaselineJSON(t *testing.T) {
	config := params.BeaconConfig().Copy()
	params.SetActiveTestCleanup(t, config)
	params.SetGenesisFork(t, config, version.Bellatrix)
	builder, server, observed := headerTestServer(t)
	client, err := builderClient.NewClient(server.URL)
	require.NoError(t, err)
	_, err = client.GetHeader(t.Context(), 1, [32]byte{1}, [48]byte{2})
	require.NoError(t, err)
	block, err := blocks.NewSignedBeaconBlock(util.NewBlindedBeaconBlockBellatrix())
	require.NoError(t, err)

	payload, _, err := client.SubmitBlindedBlock(t.Context(), block)

	require.NoError(t, err)
	require.NotNil(t, payload)
	require.DeepEqual(t, builder.currPayload.BlockHash(), payload.BlockHash())
	require.Equal(t, http.StatusOK, observed.status)
	t.Logf("endpoint=%s fork=bellatrix encoding=json status=%d content_type=%s version=%s decoded_type=%T body_length=%d", blindedPath, observed.status, observed.header.Get("Content-Type"), observed.header.Get(api.VersionHeader), payload, observed.body.Len())
}

func TestGetHeader(t *testing.T) {
	tests := []struct {
		name          string
		forkVersion   int
		decodeJSON    func(*testing.T, []byte)
		decodeSSZ     func(*testing.T, []byte)
		assertBidType func(*testing.T, builderClient.Bid)
	}{
		{
			name:        "Bellatrix",
			forkVersion: version.Bellatrix,
			decodeJSON: func(t *testing.T, body []byte) {
				require.NoError(t, json.Unmarshal(body, &builderClient.ExecHeaderResponse{}))
			},
			decodeSSZ: func(t *testing.T, body []byte) {
				require.NoError(t, (&eth.SignedBuilderBid{}).UnmarshalSSZ(body))
			},
		},
		{
			name:        "Capella",
			forkVersion: version.Capella,
			decodeJSON: func(t *testing.T, body []byte) {
				require.NoError(t, json.Unmarshal(body, &builderClient.ExecHeaderResponseCapella{}))
			},
			decodeSSZ: func(t *testing.T, body []byte) {
				require.NoError(t, (&eth.SignedBuilderBidCapella{}).UnmarshalSSZ(body))
			},
		},
		{
			name:        "Deneb",
			forkVersion: version.Deneb,
			decodeJSON: func(t *testing.T, body []byte) {
				require.NoError(t, json.Unmarshal(body, &builderClient.ExecHeaderResponseDeneb{}))
			},
			decodeSSZ: func(t *testing.T, body []byte) {
				require.NoError(t, (&eth.SignedBuilderBidDeneb{}).UnmarshalSSZ(body))
			},
			assertBidType: func(t *testing.T, bid builderClient.Bid) {
				_, ok := bid.(builderClient.BidDeneb)
				require.Equal(t, true, ok)
			},
		},
		{
			name:        "Electra",
			forkVersion: version.Electra,
			decodeJSON: func(t *testing.T, body []byte) {
				require.NoError(t, json.Unmarshal(body, &builderClient.ExecHeaderResponseElectra{}))
			},
			decodeSSZ: func(t *testing.T, body []byte) {
				require.NoError(t, (&eth.SignedBuilderBidElectra{}).UnmarshalSSZ(body))
			},
			assertBidType: func(t *testing.T, bid builderClient.Bid) {
				_, ok := bid.(builderClient.BidElectra)
				require.Equal(t, true, ok)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := params.BeaconConfig().Copy()
			params.SetActiveTestCleanup(t, config)
			params.SetGenesisFork(t, config, test.forkVersion)
			for _, encoding := range []string{api.JsonMediaType, api.OctetStreamMediaType} {
				t.Run(encoding, func(t *testing.T) {
					builder, server, observed := headerTestServer(t)
					client, err := builderClient.NewClient(server.URL, clientOptions(encoding)...)
					require.NoError(t, err)

					bid, err := client.GetHeader(t.Context(), 1, [32]byte{1}, [48]byte{2})

					require.NoError(t, err)
					require.Equal(t, http.StatusOK, observed.status)
					require.Equal(t, encoding, observed.header.Get("Content-Type"))
					require.Equal(t, version.String(test.forkVersion), observed.header.Get(api.VersionHeader))
					require.Equal(t, test.forkVersion, builder.currVersion)
					if encoding == api.JsonMediaType {
						test.decodeJSON(t, observed.body.Bytes())
					} else {
						test.decodeSSZ(t, observed.body.Bytes())
					}
					require.Equal(t, test.forkVersion, bid.Version())
					message, err := bid.Message()
					require.NoError(t, err)
					require.Equal(t, test.forkVersion, message.Version())
					require.NotNil(t, message.Value())
					require.Equal(t, 96, len(bid.Signature()))
					header, err := message.Header()
					require.NoError(t, err)
					require.Equal(t, 32, len(header.BlockHash()))
					if test.assertBidType != nil {
						test.assertBidType(t, message)
					}
					t.Logf("endpoint=%s fork=%s accept=%s status=%d content_type=%s version=%s decoded_type=%T", headerPath, version.String(test.forkVersion), encoding, observed.status, observed.header.Get("Content-Type"), observed.header.Get(api.VersionHeader), bid)
				})
			}
		})
	}
}

func TestWriteBuilderResponse(t *testing.T) {
	tests := []struct {
		name        string
		forkVersion int
		accept      string
		contentType string
	}{
		{name: "Bellatrix JSON", forkVersion: version.Bellatrix, accept: api.JsonMediaType, contentType: api.JsonMediaType},
		{name: "Bellatrix SSZ", forkVersion: version.Bellatrix, accept: api.OctetStreamMediaType, contentType: api.OctetStreamMediaType},
		{name: "Capella JSON", forkVersion: version.Capella, accept: api.JsonMediaType, contentType: api.JsonMediaType},
		{name: "Capella SSZ", forkVersion: version.Capella, accept: api.OctetStreamMediaType, contentType: api.OctetStreamMediaType},
		{name: "Deneb JSON", forkVersion: version.Deneb, accept: api.JsonMediaType, contentType: api.JsonMediaType},
		{name: "Deneb SSZ", forkVersion: version.Deneb, accept: api.OctetStreamMediaType, contentType: api.OctetStreamMediaType},
		{name: "Electra JSON", forkVersion: version.Electra, accept: api.JsonMediaType, contentType: api.JsonMediaType},
		{name: "Electra SSZ", forkVersion: version.Electra, accept: api.OctetStreamMediaType, contentType: api.OctetStreamMediaType},
		{name: "Absent Accept defaults JSON", forkVersion: version.Bellatrix, contentType: api.JsonMediaType},
		{name: "Unrelated Accept defaults JSON", forkVersion: version.Bellatrix, accept: "text/plain", contentType: api.JsonMediaType},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/header", nil)
			if test.accept != "" {
				req.Header.Set("Accept", test.accept)
			}

			err := writeBuilderResponse(recorder, req, test.forkVersion, map[string]string{"encoding": "json"}, testSSZResponse{body: []byte("ssz")})

			require.NoError(t, err)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, test.contentType, recorder.Header().Get("Content-Type"))
			require.Equal(t, version.String(test.forkVersion), recorder.Header().Get(api.VersionHeader))
			if test.contentType == api.OctetStreamMediaType {
				require.Equal(t, true, bytes.Equal([]byte("ssz"), recorder.Body.Bytes()))
			} else {
				var response map[string]string
				require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
				require.Equal(t, "json", response["encoding"])
			}
			t.Logf("endpoint=%s fork=%s accept=%s status=%d content_type=%s version=%s decoded_type=%T", headerPath, version.String(test.forkVersion), test.accept, recorder.Code, recorder.Header().Get("Content-Type"), recorder.Header().Get(api.VersionHeader), testSSZResponse{})
		})
	}
}

type testSSZResponse struct {
	body []byte
}

func (r testSSZResponse) MarshalSSZ() ([]byte, error) {
	return r.body, nil
}

func (r testSSZResponse) MarshalSSZTo(dst []byte) ([]byte, error) {
	return append(dst, r.body...), nil
}

func (r testSSZResponse) SizeSSZ() int {
	return len(r.body)
}

func clientOptions(accept string) []builderClient.ClientOpt {
	if accept == api.OctetStreamMediaType {
		return []builderClient.ClientOpt{builderClient.WithSSZ()}
	}
	return nil
}

type observedHeaderResponse struct {
	http.ResponseWriter
	status        int
	header        http.Header
	requestHeader http.Header
	body          bytes.Buffer
}

func (w *observedHeaderResponse) reset(responseWriter http.ResponseWriter, request *http.Request) {
	w.ResponseWriter = responseWriter
	w.status = 0
	w.header = nil
	w.requestHeader = request.Header.Clone()
	w.body.Reset()
}

func (w *observedHeaderResponse) WriteHeader(status int) {
	w.status = status
	w.header = w.ResponseWriter.Header().Clone()
	w.ResponseWriter.WriteHeader(status)
}

func (w *observedHeaderResponse) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	_, _ = w.body.Write(body)
	return w.ResponseWriter.Write(body)
}

func headerTestServer(t *testing.T) (*Builder, *httptest.Server, *observedHeaderResponse) {
	t.Helper()
	rpcServer := gethRPC.NewServer()
	blobGasUsed := uint64(0)
	excessBlobGas := uint64(0)
	payload := &engine.ExecutableData{
		ParentHash:    common.Hash{1},
		FeeRecipient:  common.Address{2},
		StateRoot:     common.Hash{3},
		ReceiptsRoot:  common.Hash{4},
		LogsBloom:     bytes.Repeat([]byte{5}, 256),
		Random:        common.Hash{6},
		Number:        1,
		GasLimit:      30_000_000,
		GasUsed:       1,
		Timestamp:     1,
		ExtraData:     []byte{7},
		BaseFeePerGas: big.NewInt(1),
		BlockHash:     common.Hash{8},
		Transactions:  [][]byte{},
		BlobGasUsed:   &blobGasUsed,
		ExcessBlobGas: &excessBlobGas,
	}
	envelope := &engine.ExecutionPayloadEnvelope{
		ExecutionPayload: payload,
		BlockValue:       big.NewInt(1),
		BlobsBundle:      &engine.BlobsBundle{},
	}
	require.NoError(t, rpcServer.RegisterName("engine", &testEngineService{payload: payload, envelope: envelope}))
	t.Cleanup(rpcServer.Stop)
	builder := &Builder{
		cfg:            &config{logger: logrus.New()},
		currId:         &v1.PayloadIDBytes{1},
		prevBeaconRoot: common.Hash{9}.Bytes(),
		execClient:     gethRPC.DialInProc(rpcServer),
	}
	mux := http.NewServeMux()
	observed := &observedHeaderResponse{}
	mux.HandleFunc(http.MethodGet+" "+headerPath, func(w http.ResponseWriter, req *http.Request) {
		observed.reset(w, req)
		builder.handleHeaderRequest(observed, req)
	})
	mux.HandleFunc(http.MethodPost+" "+blindedPath, func(w http.ResponseWriter, req *http.Request) {
		observed.reset(w, req)
		builder.handleBlindedBlock(observed, req)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return builder, server, observed
}

func TestRegisterValidatorsBaselineJSON(t *testing.T) {
	builder, server := registrationTestServer(t)
	registrations := validRegistrations()
	client, err := builderClient.NewClient(server.URL)
	require.NoError(t, err)

	err = client.RegisterValidator(t.Context(), registrations)

	require.NoError(t, err)
	require.Equal(t, 2, len(builder.validatorMap))
	for _, registration := range registrations {
		require.DeepEqual(t, registration.Message, builder.validatorMap[hexutil.Encode(registration.Message.Pubkey)])
	}
	t.Logf("endpoint=%s encoding=json status=%d map_size=%d", registerPath, http.StatusOK, len(builder.validatorMap))
}

func TestRegisterValidators(t *testing.T) {
	registrations := validRegistrations()

	t.Run("accepts two JSON registrations", func(t *testing.T) {
		builder, server := registrationTestServer(t)
		client, err := builderClient.NewClient(server.URL)
		require.NoError(t, err)

		err = client.RegisterValidator(t.Context(), registrations)

		require.NoError(t, err)
		assertRegistrations(t, builder, registrations)
		t.Logf("endpoint=%s encoding=json status=%d map_size=%d", registerPath, http.StatusOK, len(builder.validatorMap))
	})

	t.Run("accepts two concatenated SSZ registrations", func(t *testing.T) {
		builder, server := registrationTestServer(t)
		client, err := builderClient.NewClient(server.URL, builderClient.WithSSZ())
		require.NoError(t, err)

		err = client.RegisterValidator(t.Context(), registrations)

		require.NoError(t, err)
		assertRegistrations(t, builder, registrations)
		t.Logf("endpoint=%s encoding=ssz status=%d map_size=%d", registerPath, http.StatusOK, len(builder.validatorMap))
	})

	validJSON := registrationJSON(t, registrations)
	validSSZ, err := registrations[0].MarshalSSZ()
	require.NoError(t, err)
	invalidMessage := registrationAPI(registrations[1])
	invalidMessage.Message.Pubkey = "0x01"
	invalidSignature := registrationAPI(registrations[1])
	invalidSignature.Signature = "0x01"
	nilMessage := registrationAPI(registrations[1])
	nilMessage.Message = nil
	invalidSecond, err := json.Marshal([]*structs.SignedValidatorRegistration{registrationAPI(registrations[0]), invalidMessage})
	require.NoError(t, err)

	tests := []struct {
		name        string
		contentType string
		body        []byte
	}{
		{name: "rejects zero-length SSZ", contentType: api.OctetStreamMediaType},
		{name: "rejects empty JSON array", contentType: api.JsonMediaType, body: []byte("[]")},
		{name: "rejects JSON null", contentType: api.JsonMediaType, body: []byte("null")},
		{name: "rejects truncated SSZ", contentType: api.OctetStreamMediaType, body: validSSZ[:len(validSSZ)-1]},
		{name: "rejects syntax-invalid JSON", contentType: api.JsonMediaType, body: []byte("[{")},
		{name: "rejects a second JSON document", contentType: api.JsonMediaType, body: append(validJSON, []byte(" []")...)},
		{name: "rejects malformed trailing JSON", contentType: api.JsonMediaType, body: append(validJSON, []byte(" trailing")...)},
		{name: "rejects nil message", contentType: api.JsonMediaType, body: marshalJSON(t, []*structs.SignedValidatorRegistration{nilMessage})},
		{name: "rejects wrong fixed-length message field", contentType: api.JsonMediaType, body: marshalJSON(t, []*structs.SignedValidatorRegistration{invalidMessage})},
		{name: "rejects wrong-length signature", contentType: api.JsonMediaType, body: marshalJSON(t, []*structs.SignedValidatorRegistration{invalidSignature})},
		{name: "does not partially mutate after invalid second record", contentType: api.JsonMediaType, body: invalidSecond},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			builder, server := registrationTestServer(t)
			existing := validRegistration(0xff).Message
			expected := map[string]*eth.ValidatorRegistrationV1{hexutil.Encode(existing.Pubkey): existing}
			builder.validatorMap = map[string]*eth.ValidatorRegistrationV1{hexutil.Encode(existing.Pubkey): existing}

			status := postRegistration(t, server, test.contentType, test.body)

			require.Equal(t, http.StatusBadRequest, status)
			require.DeepEqual(t, expected, builder.validatorMap)
			t.Logf("endpoint=%s encoding=%s status=%d map_size=%d", registerPath, test.contentType, status, len(builder.validatorMap))
		})
	}
}

func registrationTestServer(t *testing.T) (*Builder, *httptest.Server) {
	t.Helper()
	builder := &Builder{validatorMap: make(map[string]*eth.ValidatorRegistrationV1)}
	mux := http.NewServeMux()
	mux.HandleFunc(http.MethodPost+" "+registerPath, builder.registerValidators)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return builder, server
}

func validRegistrations() []*eth.SignedValidatorRegistrationV1 {
	return []*eth.SignedValidatorRegistrationV1{
		validRegistration(0x41),
		validRegistration(0x42),
	}
}

func validRegistration(pubkeyByte byte) *eth.SignedValidatorRegistrationV1 {
	return &eth.SignedValidatorRegistrationV1{
		Message: &eth.ValidatorRegistrationV1{
			FeeRecipient: bytes.Repeat([]byte{0x24}, 20),
			GasLimit:     30_000_000,
			Timestamp:    1,
			Pubkey:       bytes.Repeat([]byte{pubkeyByte}, 48),
		},
		Signature: bytes.Repeat([]byte{0x11}, 96),
	}
}

func assertRegistrations(t *testing.T, builder *Builder, registrations []*eth.SignedValidatorRegistrationV1) {
	t.Helper()
	require.Equal(t, len(registrations), len(builder.validatorMap))
	for _, registration := range registrations {
		require.DeepEqual(t, registration.Message, builder.validatorMap[hexutil.Encode(registration.Message.Pubkey)])
	}
}

func registrationJSON(t *testing.T, registrations []*eth.SignedValidatorRegistrationV1) []byte {
	t.Helper()
	jsonRegistrations := make([]*structs.SignedValidatorRegistration, len(registrations))
	for index, registration := range registrations {
		jsonRegistrations[index] = registrationAPI(registration)
	}
	return marshalJSON(t, jsonRegistrations)
}

func registrationAPI(registration *eth.SignedValidatorRegistrationV1) *structs.SignedValidatorRegistration {
	return structs.SignedValidatorRegistrationFromConsensus(registration)
}

func marshalJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return encoded
}

func postRegistration(t *testing.T, server *httptest.Server, contentType string, body []byte) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+registerPath, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", contentType)
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	status := resp.StatusCode
	require.NoError(t, resp.Body.Close())
	return status
}

func TestHandleBlindedBlock(t *testing.T) {
	tests := []struct {
		name    string
		fork    int
		block   func() interfaces.ReadOnlySignedBeaconBlock
		decode  func(*testing.T, []byte)
		bundles bool
	}{
		{"bellatrix", version.Bellatrix, func() interfaces.ReadOnlySignedBeaconBlock {
			return blindedBlock(t, util.NewBlindedBeaconBlockBellatrix())
		}, func(t *testing.T, body []byte) { require.NoError(t, (&v1.ExecutionPayload{}).UnmarshalSSZ(body)) }, false},
		{"capella", version.Capella, func() interfaces.ReadOnlySignedBeaconBlock {
			return blindedBlock(t, util.NewBlindedBeaconBlockCapella())
		}, func(t *testing.T, body []byte) {
			require.NoError(t, (&v1.ExecutionPayloadCapella{}).UnmarshalSSZ(body))
		}, false},
		{"deneb", version.Deneb, func() interfaces.ReadOnlySignedBeaconBlock { return blindedBlock(t, util.NewBlindedBeaconBlockDeneb()) }, func(t *testing.T, body []byte) {
			require.NoError(t, (&v1.ExecutionPayloadDenebAndBlobsBundle{}).UnmarshalSSZ(body))
		}, true},
		{"electra", version.Electra, func() interfaces.ReadOnlySignedBeaconBlock {
			return blindedBlock(t, util.NewBlindedBeaconBlockElectra())
		}, func(t *testing.T, body []byte) {
			require.NoError(t, (&v1.ExecutionPayloadDenebAndBlobsBundle{}).UnmarshalSSZ(body))
		}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := params.BeaconConfig().Copy()
			params.SetActiveTestCleanup(t, config)
			params.SetGenesisFork(t, config, test.fork)
			for _, encoding := range []string{api.JsonMediaType, api.OctetStreamMediaType} {
				t.Run(encoding, func(t *testing.T) {
					builder, server, observed := headerTestServer(t)
					client, err := builderClient.NewClient(server.URL, clientOptions(encoding)...)
					require.NoError(t, err)
					_, err = client.GetHeader(t.Context(), 1, [32]byte{1}, [48]byte{2})
					require.NoError(t, err)

					payload, bundle, err := client.SubmitBlindedBlock(t.Context(), test.block())

					require.NoError(t, err)
					require.Equal(t, encoding, observed.requestHeader.Get("Content-Type"))
					require.Equal(t, http.StatusOK, observed.status)
					require.Equal(t, encoding, observed.header.Get("Content-Type"))
					require.Equal(t, version.String(test.fork), observed.header.Get(api.VersionHeader))
					require.DeepEqual(t, builder.currPayload.BlockHash(), payload.BlockHash())
					if test.bundles {
						require.NotNil(t, bundle)
					}
					if encoding == api.OctetStreamMediaType {
						test.decode(t, observed.body.Bytes())
					} else {
						var response builderClient.ExecutionPayloadResponse
						require.NoError(t, json.Unmarshal(observed.body.Bytes(), &response))
					}
					t.Logf("endpoint=%s fork=%s encoding=%s status=%d content_type=%s version=%s decoded_type=%T body_length=%d", blindedPath, test.name, encoding, observed.status, observed.header.Get("Content-Type"), observed.header.Get(api.VersionHeader), payload, observed.body.Len())
				})
			}
		})
	}
}

func TestHandleBlindedBlockRejectsMalformedInput(t *testing.T) {
	for _, test := range []struct {
		name  string
		fork  int
		block func() interfaces.ReadOnlySignedBeaconBlock
	}{
		{"bellatrix", version.Bellatrix, func() interfaces.ReadOnlySignedBeaconBlock {
			return blindedBlock(t, util.NewBlindedBeaconBlockBellatrix())
		}},
		{"capella", version.Capella, func() interfaces.ReadOnlySignedBeaconBlock {
			return blindedBlock(t, util.NewBlindedBeaconBlockCapella())
		}},
		{"deneb", version.Deneb, func() interfaces.ReadOnlySignedBeaconBlock { return blindedBlock(t, util.NewBlindedBeaconBlockDeneb()) }},
		{"electra", version.Electra, func() interfaces.ReadOnlySignedBeaconBlock {
			return blindedBlock(t, util.NewBlindedBeaconBlockElectra())
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			valid := blindedBlockJSON(t, test.block())
			for _, input := range []struct {
				name, contentType string
				body              []byte
			}{
				{"malformed ssz", api.OctetStreamMediaType, []byte{1}},
				{"syntax invalid json", api.JsonMediaType, []byte(`{"message":`)},
				{"nil message", api.JsonMediaType, mutateBlindedBlockJSON(t, valid, func(message map[string]json.RawMessage) { message["message"] = []byte("null") })},
				{"wrong fixed length field", api.JsonMediaType, mutateBlindedBlockMessageJSON(t, valid, func(message map[string]json.RawMessage) { message["parent_root"] = []byte(`"0x01"`) })},
				{"second json document", api.JsonMediaType, append(valid, []byte(" {}")...)},
				{"malformed trailing bytes", api.JsonMediaType, append(valid, []byte(" trailing")...)},
			} {
				t.Run(input.name, func(t *testing.T) {
					builder := &Builder{cfg: &config{logger: logrus.New()}, currVersion: test.fork}
					mux := http.NewServeMux()
					mux.HandleFunc(http.MethodPost+" "+blindedPath, builder.handleBlindedBlock)
					server := httptest.NewServer(mux)
					t.Cleanup(server.Close)
					status, response := postBlindedBlock(t, server.URL, blindedPath, input.contentType, input.body)
					require.Equal(t, http.StatusBadRequest, status)
					require.Equal(t, 0, len(response))
					t.Logf("endpoint=%s fork=%s encoding=%s status=%d body_length=%d", blindedPath, test.name, input.contentType, status, len(response))
				})
			}
		})
	}
}

func TestSubmitBlindedBlockPostFulu(t *testing.T) {
	for _, encoding := range []string{api.JsonMediaType, api.OctetStreamMediaType} {
		t.Run(encoding, func(t *testing.T) {
			builder, server, fallbackHits := fuluTestServer(t)
			client, err := builderClient.NewClient(server.URL, clientOptions(encoding)...)
			require.NoError(t, err)

			err = client.SubmitBlindedBlockPostFulu(t.Context(), blindedBlock(t, util.NewBlindedBeaconBlockFulu()))

			require.NoError(t, err)
			require.Equal(t, int32(0), fallbackHits.Load())
			t.Logf("endpoint=/eth/v2/builder/blinded_blocks fork=fulu encoding=%s status=%d body_length=%d fallback_hits=%d", encoding, http.StatusAccepted, 0, fallbackHits.Load())
			_ = builder
		})
	}
}

func TestServeHTTPRoutesBuilderV2(t *testing.T) {
	for _, test := range []struct {
		path         string
		wantStatus   int
		wantFallback int32
	}{
		{"/eth/v2/builder/blinded_blocks", http.StatusAccepted, 0},
		{"/eth/v2/builder/unknown", http.StatusOK, 1},
		{"/not-builder", http.StatusOK, 1},
		{"/eth/v1/builder/header/1/0x01", http.StatusOK, 1},
	} {
		t.Run(test.path, func(t *testing.T) {
			_, server, fallbackHits := fuluTestServer(t)
			body := []byte(`{}`)
			if test.path == "/eth/v2/builder/blinded_blocks" {
				body = blindedBlockJSON(t, blindedBlock(t, util.NewBlindedBeaconBlockFulu()))
			}
			status, _ := postBlindedBlock(t, server.URL, test.path, api.JsonMediaType, body)
			require.Equal(t, test.wantStatus, status)
			require.Equal(t, test.wantFallback, fallbackHits.Load())
			t.Logf("endpoint=%s status=%d fallback_hits=%d", test.path, status, fallbackHits.Load())
		})
	}
}

func TestServeHTTPRoutesBuilderReturnsMethodNotAllowedForRegisteredPaths(t *testing.T) {
	for _, test := range []struct {
		name, method, path string
	}{
		{"status", http.MethodPost, "/eth/v1/builder/status"},
		{"validators", http.MethodGet, "/eth/v1/builder/validators"},
		{"header", http.MethodPost, "/eth/v1/builder/header/1/0x01/0x02"},
		{"v1 blinded block", http.MethodGet, "/eth/v1/builder/blinded_blocks"},
		{"v2 blinded block", http.MethodGet, "/eth/v2/builder/blinded_blocks"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, server, fallbackHits := fuluTestServer(t)
			req, err := http.NewRequestWithContext(t.Context(), test.method, server.URL+test.path, bytes.NewReader([]byte(`{}`)))
			require.NoError(t, err)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer func() { require.NoError(t, resp.Body.Close()) }()
			require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
			require.Equal(t, int32(0), fallbackHits.Load())
			t.Logf("endpoint=%s method=%s status=%d fallback_hits=%d", test.path, test.method, resp.StatusCode, fallbackHits.Load())
		})
	}
}

func TestIsBuilderCall(t *testing.T) {
	builder, _, _ := fuluTestServer(t)
	for _, test := range []struct {
		name, method, path string
		want               bool
	}{
		{"status", http.MethodGet, "/eth/v1/builder/status", true},
		{"status method mismatch", http.MethodPost, "/eth/v1/builder/status", true},
		{"validators", http.MethodPost, "/eth/v1/builder/validators", true},
		{"validators method mismatch", http.MethodGet, "/eth/v1/builder/validators", true},
		{"header", http.MethodGet, "/eth/v1/builder/header/1/0x01/0x02", true},
		{"header method mismatch", http.MethodPost, "/eth/v1/builder/header/1/0x01/0x02", true},
		{"v1 blinded block", http.MethodPost, "/eth/v1/builder/blinded_blocks", true},
		{"v1 blinded block method mismatch", http.MethodGet, "/eth/v1/builder/blinded_blocks", true},
		{"v2 blinded block", http.MethodPost, "/eth/v2/builder/blinded_blocks", true},
		{"v2 blinded block method mismatch", http.MethodGet, "/eth/v2/builder/blinded_blocks", true},
		{"unknown v1 builder path", http.MethodGet, "/eth/v1/builder/unknown", false},
		{"unknown v2 builder path", http.MethodPost, "/eth/v2/builder/unknown", false},
		{"engine path", http.MethodPost, "/", false},
		{"header missing segments", http.MethodGet, "/eth/v1/builder/header/1/0x01", false},
		{"header extra segments", http.MethodGet, "/eth/v1/builder/header/1/0x01/0x02/extra", false},
		{"header empty segment", http.MethodGet, "/eth/v1/builder/header/1//0x02", false},
		{"header trailing slash", http.MethodGet, "/eth/v1/builder/header/1/0x01/0x02/", false},
		{"header prefix only", http.MethodGet, "/eth/v1/builder/header/", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(test.method, test.path, nil)
			require.Equal(t, test.want, builder.isBuilderCall(req))
		})
	}
}

func TestSubmitBlindedBlockPostFuluRejectsMalformedInput(t *testing.T) {
	valid := blindedBlockJSON(t, blindedBlock(t, util.NewBlindedBeaconBlockFulu()))
	for _, input := range []struct {
		name, contentType string
		body              []byte
	}{
		{"malformed ssz", api.OctetStreamMediaType, []byte{1}},
		{"syntax invalid json", api.JsonMediaType, []byte(`{"message":`)},
		{"nil message", api.JsonMediaType, mutateBlindedBlockJSON(t, valid, func(message map[string]json.RawMessage) { message["message"] = []byte("null") })},
		{"wrong fixed length field", api.JsonMediaType, mutateBlindedBlockMessageJSON(t, valid, func(message map[string]json.RawMessage) { message["parent_root"] = []byte(`"0x01"`) })},
		{"second json document", api.JsonMediaType, append(valid, []byte(" {}")...)},
		{"malformed trailing bytes", api.JsonMediaType, append(valid, []byte(" trailing")...)},
	} {
		t.Run(input.name, func(t *testing.T) {
			_, server, fallbackHits := fuluTestServer(t)
			status, response := postBlindedBlock(t, server.URL, "/eth/v2/builder/blinded_blocks", input.contentType, input.body)
			require.Equal(t, http.StatusBadRequest, status)
			require.Equal(t, 0, len(response))
			require.Equal(t, int32(0), fallbackHits.Load())
			t.Logf("endpoint=/eth/v2/builder/blinded_blocks fork=fulu encoding=%s status=%d body_length=%d fallback_hits=%d", input.contentType, status, len(response), fallbackHits.Load())
		})
	}
}

func blindedBlock(t *testing.T, proto any) interfaces.ReadOnlySignedBeaconBlock {
	t.Helper()
	block, err := blocks.NewSignedBeaconBlock(proto)
	require.NoError(t, err)
	return block
}

func blindedBlockJSON(t *testing.T, block interfaces.ReadOnlySignedBeaconBlock) []byte {
	t.Helper()
	jsonBlock, err := structs.SignedBeaconBlockMessageJsoner(block)
	require.NoError(t, err)
	return marshalJSON(t, jsonBlock)
}

func mutateBlindedBlockJSON(t *testing.T, body []byte, mutate func(map[string]json.RawMessage)) []byte {
	t.Helper()
	var message map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &message))
	mutate(message)
	return marshalJSON(t, message)
}

func mutateBlindedBlockMessageJSON(t *testing.T, body []byte, mutate func(map[string]json.RawMessage)) []byte {
	t.Helper()
	var root map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &root))
	var message map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(root["message"], &message))
	mutate(message)
	root["message"] = marshalJSON(t, message)
	return marshalJSON(t, root)
}

func postBlindedBlock(t *testing.T, baseURL, path, contentType string, body []byte) (int, []byte) {
	t.Helper()
	path = strings.TrimPrefix(path, http.MethodPost+" ")
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, baseURL+path, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", contentType)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	response, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, response
}

func fuluTestServer(t *testing.T) (*Builder, *httptest.Server, *atomic.Int32) {
	t.Helper()
	fallbackHits := &atomic.Int32{}
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fallbackHits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(fallback.Close)
	builder, err := New(WithDestinationAddress(fallback.URL))
	require.NoError(t, err)
	server := httptest.NewServer(builder)
	t.Cleanup(server.Close)
	return builder, server, fallbackHits
}
