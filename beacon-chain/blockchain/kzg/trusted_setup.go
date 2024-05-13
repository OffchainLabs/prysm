package kzg

import (
	_ "embed"
	"encoding/json"

	GoKZG "github.com/crate-crypto/go-kzg-4844"
	CKZG "github.com/ethereum/c-kzg-4844/bindings/go"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
)

var (
	//go:embed trusted_setup.json
	embeddedTrustedSetup []byte // 1.2Mb
	kzgContext           *GoKZG.Context
	kzgLoaded            bool
)

func Start() error {
	parsedSetup := &GoKZG.JSONTrustedSetup{}
	err := json.Unmarshal(embeddedTrustedSetup, parsedSetup)
	if err != nil {
		return errors.Wrap(err, "could not parse trusted setup JSON")
	}
	kzgContext, err = GoKZG.NewContext4096(parsedSetup)
	if err != nil {
		return errors.Wrap(err, "could not initialize go-kzg context")
	}
	g1Lagrange := &parsedSetup.SetupG1Lagrange

	// Length of a G1 point, converted from hex to binary.
	g1s := make([]byte, len(g1Lagrange)*(len(g1Lagrange[0])-2)/2)
	for i, g1 := range g1Lagrange {
		copy(g1s[i*(len(g1)-2)/2:], hexutil.MustDecode(g1))
	}
	// Length of a G2 point, converted from hex to binary.
	g2s := make([]byte, len(parsedSetup.SetupG2)*(len(parsedSetup.SetupG2[0])-2)/2)
	for i, g2 := range parsedSetup.SetupG2 {
		copy(g2s[i*(len(g2)-2)/2:], hexutil.MustDecode(g2))
	}
	if !kzgLoaded {
		// Free the current trusted setup before running this method. CKZG
		// panics if the same setup is run multiple times.
		if err = CKZG.LoadTrustedSetup(g1s, g2s); err != nil {
			panic(err)
		}
	}
	kzgLoaded = true
	return nil
}
