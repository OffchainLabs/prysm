package sync

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/d4l3k/messagediff"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain"
	mock "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/signing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	p2ptesting "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/startup"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/wrapper"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/metadata"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func TestService_decodePubsubMessage(t *testing.T) {
	digest, err := signing.ComputeForkDigest(params.BeaconConfig().GenesisForkVersion, make([]byte, 32))
	require.NoError(t, err)
	tests := []struct {
		name    string
		topic   string
		input   *pubsub.Message
		want    interface{}
		wantErr error
	}{
		{
			name:    "Nil message",
			input:   nil,
			wantErr: errNilPubsubMessage,
		},
		{
			name: "nil topic",
			input: &pubsub.Message{
				Message: &pb.Message{
					Topic: nil,
				},
			},
			wantErr: errNilPubsubMessage,
		},
		{
			name:    "invalid topic format",
			topic:   "foo",
			wantErr: errInvalidTopic,
		},
		{
			name:    "topic not mapped to any message type",
			topic:   "/eth2/abababab/foo/ssz_snappy",
			wantErr: p2p.ErrMessageNotMapped,
		},
		{
			name:  "valid message -- beacon block",
			topic: fmt.Sprintf(p2p.GossipTypeMapping[reflect.TypeOf(&ethpb.SignedBeaconBlock{})], digest),
			input: &pubsub.Message{
				Message: &pb.Message{
					Data: func() []byte {
						buf := new(bytes.Buffer)
						if _, err := p2ptesting.NewTestP2P(t).Encoding().EncodeGossip(buf, util.NewBeaconBlock()); err != nil {
							t.Fatal(err)
						}
						return buf.Bytes()
					}(),
				},
			},
			wantErr: nil,
			want: func() interfaces.ReadOnlySignedBeaconBlock {
				wsb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlock())
				require.NoError(t, err)
				return wsb
			}(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := &mock.ChainService{ValidatorsRoot: [32]byte{}, Genesis: time.Now()}
			s := &Service{
				cfg: &config{p2p: p2ptesting.NewTestP2P(t), chain: chain, clock: startup.NewClock(chain.Genesis, chain.ValidatorsRoot)},
			}
			if tt.topic != "" {
				if tt.input == nil {
					tt.input = &pubsub.Message{Message: &pb.Message{}}
				} else if tt.input.Message == nil {
					tt.input.Message = &pb.Message{}
				}
				// reassign because tt is a loop variable
				topic := tt.topic
				tt.input.Message.Topic = &topic
			}
			got, err := s.decodePubsubMessage(tt.input)
			if err != nil && err != tt.wantErr && !strings.Contains(err.Error(), tt.wantErr.Error()) {
				t.Errorf("decodePubsubMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				diff, _ := messagediff.PrettyDiff(got, tt.want)
				t.Log(diff)
				t.Errorf("decodePubsubMessage() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractDataType(t *testing.T) {
	// Precompute digests
	genDigest, err := signing.ComputeForkDigest(params.BeaconConfig().GenesisForkVersion, params.BeaconConfig().ZeroHash[:])
	require.NoError(t, err)
	altairDigest, err := signing.ComputeForkDigest(params.BeaconConfig().AltairForkVersion, params.BeaconConfig().ZeroHash[:])
	require.NoError(t, err)
	bellatrixDigest, err := signing.ComputeForkDigest(params.BeaconConfig().BellatrixForkVersion, params.BeaconConfig().ZeroHash[:])
	require.NoError(t, err)
	capellaDigest, err := signing.ComputeForkDigest(params.BeaconConfig().CapellaForkVersion, params.BeaconConfig().ZeroHash[:])
	require.NoError(t, err)
	denebDigest, err := signing.ComputeForkDigest(params.BeaconConfig().DenebForkVersion, params.BeaconConfig().ZeroHash[:])
	require.NoError(t, err)
	electraDigest, err := signing.ComputeForkDigest(params.BeaconConfig().ElectraForkVersion, params.BeaconConfig().ZeroHash[:])
	require.NoError(t, err)

	type args struct {
		digest []byte
		chain  blockchain.ChainInfoFetcher
	}
	tests := []struct {
		name          string
		args          args
		wantBlock     interfaces.ReadOnlySignedBeaconBlock
		wantMd        metadata.Metadata
		wantAtt       ethpb.Att
		wantAggregate ethpb.SignedAggregateAttAndProof
		wantErr       bool
	}{
		{
			name: "no digest",
			args: args{
				digest: []byte{},
				chain:  &mock.ChainService{ValidatorsRoot: [32]byte{}},
			},
			wantBlock: func() interfaces.ReadOnlySignedBeaconBlock {
				wsb, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlock{Block: &ethpb.BeaconBlock{Body: &ethpb.BeaconBlockBody{}}})
				require.NoError(t, err)
				return wsb
			}(),
			wantMd:        wrapper.WrappedMetadataV0(&ethpb.MetaDataV0{}),
			wantAtt:       &ethpb.Attestation{},
			wantAggregate: &ethpb.SignedAggregateAttestationAndProof{},
			wantErr:       false,
		},
		{
			name: "invalid digest",
			args: args{
				digest: []byte{0x00, 0x01},
				chain:  &mock.ChainService{ValidatorsRoot: [32]byte{}},
			},
			wantBlock:     nil,
			wantMd:        nil,
			wantAtt:       nil,
			wantAggregate: nil,
			wantErr:       true,
		},
		{
			name: "non existent digest",
			args: args{
				digest: []byte{0x00, 0x01, 0x02, 0x03},
				chain:  &mock.ChainService{ValidatorsRoot: [32]byte{}},
			},
			wantBlock:     nil,
			wantMd:        nil,
			wantAtt:       nil,
			wantAggregate: nil,
			wantErr:       true,
		},
		{
			name: "genesis fork version",
			args: args{
				digest: genDigest[:],
				chain:  &mock.ChainService{ValidatorsRoot: [32]byte{}},
			},
			wantBlock: func() interfaces.ReadOnlySignedBeaconBlock {
				wsb, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlock{Block: &ethpb.BeaconBlock{Body: &ethpb.BeaconBlockBody{}}})
				require.NoError(t, err)
				return wsb
			}(),
			wantAtt:       &ethpb.Attestation{},
			wantAggregate: &ethpb.SignedAggregateAttestationAndProof{},
			wantErr:       false,
		},
		{
			name: "altair fork version",
			args: args{
				digest: altairDigest[:],
				chain:  &mock.ChainService{ValidatorsRoot: [32]byte{}},
			},
			wantBlock: func() interfaces.ReadOnlySignedBeaconBlock {
				wsb, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockAltair{Block: &ethpb.BeaconBlockAltair{Body: &ethpb.BeaconBlockBodyAltair{}}})
				require.NoError(t, err)
				return wsb
			}(),
			wantMd:        wrapper.WrappedMetadataV1(&ethpb.MetaDataV1{}),
			wantAtt:       &ethpb.Attestation{},
			wantAggregate: &ethpb.SignedAggregateAttestationAndProof{},
			wantErr:       false,
		},
		{
			name: "bellatrix fork version",
			args: args{
				digest: bellatrixDigest[:],
				chain:  &mock.ChainService{ValidatorsRoot: [32]byte{}},
			},
			wantBlock: func() interfaces.ReadOnlySignedBeaconBlock {
				wsb, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockBellatrix{Block: &ethpb.BeaconBlockBellatrix{Body: &ethpb.BeaconBlockBodyBellatrix{ExecutionPayload: &enginev1.ExecutionPayload{}}}})
				require.NoError(t, err)
				return wsb
			}(),
			wantMd:        wrapper.WrappedMetadataV1(&ethpb.MetaDataV1{}),
			wantAtt:       &ethpb.Attestation{},
			wantAggregate: &ethpb.SignedAggregateAttestationAndProof{},
			wantErr:       false,
		},
		{
			name: "capella fork version",
			args: args{
				digest: capellaDigest[:],
				chain:  &mock.ChainService{ValidatorsRoot: [32]byte{}},
			},
			wantBlock: func() interfaces.ReadOnlySignedBeaconBlock {
				wsb, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockCapella{Block: &ethpb.BeaconBlockCapella{Body: &ethpb.BeaconBlockBodyCapella{ExecutionPayload: &enginev1.ExecutionPayloadCapella{}}}})
				require.NoError(t, err)
				return wsb
			}(),
			wantMd:        wrapper.WrappedMetadataV1(&ethpb.MetaDataV1{}),
			wantAtt:       &ethpb.Attestation{},
			wantAggregate: &ethpb.SignedAggregateAttestationAndProof{},
			wantErr:       false,
		},
		{
			name: "deneb fork version",
			args: args{
				digest: denebDigest[:],
				chain:  &mock.ChainService{ValidatorsRoot: [32]byte{}},
			},
			wantBlock: func() interfaces.ReadOnlySignedBeaconBlock {
				wsb, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockDeneb{Block: &ethpb.BeaconBlockDeneb{Body: &ethpb.BeaconBlockBodyDeneb{ExecutionPayload: &enginev1.ExecutionPayloadDeneb{}}}})
				require.NoError(t, err)
				return wsb
			}(),
			wantMd:        wrapper.WrappedMetadataV1(&ethpb.MetaDataV1{}),
			wantAtt:       &ethpb.Attestation{},
			wantAggregate: &ethpb.SignedAggregateAttestationAndProof{},
			wantErr:       false,
		},
		{
			name: "electra fork version",
			args: args{
				digest: electraDigest[:],
				chain:  &mock.ChainService{ValidatorsRoot: [32]byte{}},
			},
			wantBlock: func() interfaces.ReadOnlySignedBeaconBlock {
				wsb, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockElectra{Block: &ethpb.BeaconBlockElectra{Body: &ethpb.BeaconBlockBodyElectra{ExecutionPayload: &enginev1.ExecutionPayloadDeneb{}}}})
				require.NoError(t, err)
				return wsb
			}(),
			wantMd:        wrapper.WrappedMetadataV1(&ethpb.MetaDataV1{}),
			wantAtt:       &ethpb.SingleAttestation{},
			wantAggregate: &ethpb.SignedAggregateAttestationAndProofElectra{},
			wantErr:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBlock, err := extractDataTypeFromTypeMap(types.BlockMap, tt.args.digest, tt.args.chain)
			if (err != nil) != tt.wantErr {
				t.Errorf("block: error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotBlock, tt.wantBlock) {
				t.Errorf("block: got = %v, want %v", gotBlock, tt.wantBlock)
			}
			gotAtt, err := extractDataTypeFromTypeMap(types.AttestationMap, tt.args.digest, tt.args.chain)
			if (err != nil) != tt.wantErr {
				t.Errorf("attestation: error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotAtt, tt.wantAtt) {
				t.Errorf("attestation: got = %v, want %v", gotAtt, tt.wantAtt)
			}
			gotAggregate, err := extractDataTypeFromTypeMap(types.AggregateAttestationMap, tt.args.digest, tt.args.chain)
			if (err != nil) != tt.wantErr {
				t.Errorf("aggregate: error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotAggregate, tt.wantAggregate) {
				t.Errorf("aggregate: got = %v, want %v", gotAggregate, tt.wantAggregate)
			}
		})
	}
}
