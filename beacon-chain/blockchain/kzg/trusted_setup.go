package kzg

import (
	_ "embed"
	"encoding/json"

	goethkzg "github.com/crate-crypto/go-eth-kzg"
	GoKZG "github.com/crate-crypto/go-kzg-4844"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

var (
	//go:embed trusted_setup.json
	embeddedTrustedSetup []byte // 1.2Mb
	kzgContext           *GoKZG.Context
	goEthKZGContext      *goethkzg.Context
	kzgLoaded            bool
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
		SetupG1Lagrange: trustedSetup.G1Lagrange})
	if err != nil {
		return errors.Wrap(err, "could not initialize go-kzg context")
	}

	// Length of a G1 point, converted from hex to binary.
	g1MonomialBytes := make([]byte, len(trustedSetup.G1Monomial)*(len(trustedSetup.G1Monomial[0])-2)/2)
	for i, g1 := range &trustedSetup.G1Monomial {
		copy(g1MonomialBytes[i*(len(g1)-2)/2:], hexutil.MustDecode(g1))
	}
	// Length of a G1 point, converted from hex to binary.
	g1LagrangeBytes := make([]byte, len(trustedSetup.G1Lagrange)*(len(trustedSetup.G1Lagrange[0])-2)/2)
	for i, g1 := range &trustedSetup.G1Lagrange {
		copy(g1LagrangeBytes[i*(len(g1)-2)/2:], hexutil.MustDecode(g1))
	}
	// Length of a G2 point, converted from hex to binary.
	g2MonomialBytes := make([]byte, len(trustedSetup.G2Monomial)*(len(trustedSetup.G2Monomial[0])-2)/2)
	for i, g2 := range &trustedSetup.G2Monomial {
		copy(g2MonomialBytes[i*(len(g2)-2)/2:], hexutil.MustDecode(g2))
	}
	if !kzgLoaded {
		goEthKZGContext, err = goethkzg.NewContext4096Secure()
		if err != nil {
			return errors.Wrap(err, "could not initialize go-eth-kzg context")
		}
	}
	kzgLoaded = true
	return nil
}
