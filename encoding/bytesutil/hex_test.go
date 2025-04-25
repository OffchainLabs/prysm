package bytesutil_test

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
)

func TestIsHex(t *testing.T) {
	tests := []struct {
		a []byte
		b bool
	}{
		{nil, false},
		{[]byte(""), false},
		{[]byte("0x"), false},
		{[]byte("0x0"), true},
		{[]byte("foo"), false},
		{[]byte("1234567890abcDEF"), false},
		{[]byte("XYZ4567890abcDEF1234567890abcDEF1234567890abcDEF1234567890abcDEF"), false},
		{[]byte("0x1234567890abcDEF1234567890abcDEF1234567890abcDEF1234567890abcDEF"), true},
		{[]byte("1234567890abcDEF1234567890abcDEF1234567890abcDEF1234567890abcDEF"), false},
	}
	for _, tt := range tests {
		isHex := bytesutil.IsHex(tt.a)
		assert.Equal(t, tt.b, isHex)
	}
}

func TestDecodeHexWithLength_Root(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{
			name:  "correct format",
			input: "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2",
			valid: true,
		},
		{
			name:  "root too small",
			input: "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f",
			valid: false,
		},
		{
			name:  "root too big",
			input: "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f22",
			valid: false,
		},
		{
			name:  "empty root",
			input: "",
			valid: false,
		},
		{
			name:  "no 0x prefix",
			input: "cf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2",
			valid: false,
		},
		{
			name:  "invalid characters",
			input: "0xzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := bytesutil.DecodeHexWithLength(tt.input, fieldparams.RootLength)
			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Equal(t, true, err != nil)
			}
		})
	}
}
