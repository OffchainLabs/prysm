package api

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/crypto/rand"
)

// GenerateRandomHexString generates a random hex string that follows the standards for jwt token
// used for beacon node -> execution client
// used for web client -> validator client
func GenerateRandomHexString() (string, error) {
	secret := make([]byte, 32)
	randGen := rand.NewGenerator()
	n, err := randGen.Read(secret)
	if err != nil {
		return "", err
	} else if n != 32 {
		return "", errors.New("rand: unexpected length")
	}
	return hexutil.Encode(secret), nil
}

// ValidateAuthToken validating auth token for web
func ValidateAuthToken(token string) error {
	b, err := hexutil.Decode(token)
	// token should be hex-encoded and at least 256 bits
	if err != nil || len(b) < 32 {
		return errors.New("invalid auth token: token should be hex-encoded and at least 256 bits")
	}
	return nil
}
