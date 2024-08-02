package payloadattestation

import (
	"testing"

	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestValidatePayload(t *testing.T) {
	tests := []struct {
		name    string
		bfunc   func(t *testing.T) *ethpb.PayloadAttestationMessage
		wanterr error
	}{
		{
			name: "nil PayloadAttestationMessage",
			bfunc: func(t *testing.T) *ethpb.PayloadAttestationMessage {
				return nil
			},
			wanterr: errNilPayloadAttMessage,
		},
		{
			name: "nil data",
			bfunc: func(t *testing.T) *ethpb.PayloadAttestationMessage {
				return &ethpb.PayloadAttestationMessage{
					Data: nil,
				}
			},
			wanterr: errNilPayloadAttData,
		},
		{
			name: "nil signature",
			bfunc: func(t *testing.T) *ethpb.PayloadAttestationMessage {
				return &ethpb.PayloadAttestationMessage{
					Data: &ethpb.PayloadAttestationData{
						BeaconBlockRoot: make([]byte, fieldparams.RootLength),
					},
					Signature: nil,
				}
			},
			wanterr: errNilPayloadAttSignature,
		},
		{
			name: "Correct PayloadAttestationMessage",
			bfunc: func(t *testing.T) *ethpb.PayloadAttestationMessage {
				return &ethpb.PayloadAttestationMessage{
					Signature: make([]byte, fieldparams.BLSSignatureLength),
					Data: &ethpb.PayloadAttestationData{
						BeaconBlockRoot: make([]byte, fieldparams.RootLength),
					},
				}
			},
			wanterr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name+" ReadOnly", func(t *testing.T) {
			m := tt.bfunc(t)
			err := validatePayloadAtt(m)
			if tt.wanterr != nil {
				require.ErrorIs(t, err, tt.wanterr)
			} else {
				roMess, err := NewReadOnly(m)
				require.NoError(t, err)
				require.Equal(t, roMess.m.Data, m.Data)
				require.DeepEqual(t, roMess.m.Signature, m.Signature)
			}
		})
	}
}

func TestValidatorIndex(t *testing.T) {
	valIdx := primitives.ValidatorIndex(1)
	m := &ROMessage{
		m: &ethpb.PayloadAttestationMessage{
			ValidatorIndex: valIdx,
		},
	}
	require.Equal(t, valIdx, m.ValidatorIndex())
}

func TestSignature(t *testing.T) {
	sig := [96]byte{}
	m := &ROMessage{
		m: &ethpb.PayloadAttestationMessage{
			Signature: sig[:],
		},
	}
	require.Equal(t, sig, m.Signature())
}

func TestBeaconBlockRoot(t *testing.T) {
	root := [32]byte{}
	m := &ROMessage{
		m: &ethpb.PayloadAttestationMessage{
			Data: &ethpb.PayloadAttestationData{
				BeaconBlockRoot: root[:],
			},
		},
	}
	require.Equal(t, root, m.BeaconBlockRoot())
}

func TestSlot(t *testing.T) {
	slot := primitives.Slot(1)
	m := &ROMessage{
		m: &ethpb.PayloadAttestationMessage{
			Data: &ethpb.PayloadAttestationData{
				Slot: slot,
			},
		},
	}
	require.Equal(t, slot, m.Slot())
}

func TestPayloadStatus(t *testing.T) {
	for status := primitives.PAYLOAD_ABSENT; status < primitives.PAYLOAD_INVALID_STATUS; status++ {
		m := &ROMessage{
			m: &ethpb.PayloadAttestationMessage{
				Data: &ethpb.PayloadAttestationData{
					PayloadStatus: status,
				},
				Signature: make([]byte, fieldparams.BLSSignatureLength),
			},
		}
		require.NoError(t, validatePayloadAtt(m.m))
		require.Equal(t, status, m.PayloadStatus())
	}
}
