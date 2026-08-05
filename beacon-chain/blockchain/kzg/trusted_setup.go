package kzg

import (
	_ "embed"
	"encoding/json"
	"fmt"

	GoKZG "github.com/crate-crypto/go-kzg-4844"
	"github.com/pkg/errors"
)

var (
	// https://github.com/ethereum/consensus-specs/blob/master/presets/mainnet/trusted_setups/trusted_setup_4096.json
	//go:embed trusted_setup_4096.json
	embeddedTrustedSetup []byte // 1.2Mb
	kzgContext           *GoKZG.Context
)

type TrustedSetup struct {
	G1Monomial [GoKZG.ScalarsPerBlob]GoKZG.G1CompressedHexStr `json:"g1_monomial"`
	G1Lagrange [GoKZG.ScalarsPerBlob]GoKZG.G1CompressedHexStr `json:"g1_lagrange"`
	G2Monomial [65]GoKZG.G2CompressedHexStr                   `json:"g2_monomial"`
}

func Start() error {
	trustedSetup := &TrustedSetup{}
	err := json.Unmarshal(embeddedTrustedSetup, trustedSetup)
	if err != nil {
		return errors.Wrap(err, "could not parse trusted setup JSON")
	}

	kzgContext, err = GoKZG.NewContext4096(&GoKZG.JSONTrustedSetup{
		SetupG2:         trustedSetup.G2Monomial[:],
		SetupG1Lagrange: trustedSetup.G1Lagrange,
	})
	if err != nil {
		return errors.Wrap(err, "could not initialize go-kzg context")
	}

	if err := activeBackend.LoadTrustedSetup(trustedSetup); err != nil {
		return fmt.Errorf("load trusted setup: %w", err)
	}

	return nil
}
