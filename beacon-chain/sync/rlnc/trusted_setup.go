package rlnc

import (
	_ "embed"
	"encoding/json"

	ristretto "github.com/gtank/ristretto255"
	"github.com/pkg/errors"
)

var (
	//go:embed trusted_setup.json
	embeddedTrustedSetup []byte // 311KB
)

func LoadTrustedSetup() (*Committer, error) {
	var elements []*ristretto.Element
	err := json.Unmarshal(embeddedTrustedSetup, &elements)
	if err != nil {
		return nil, errors.Wrap(err, "could not parse trusted setup JSON")
	}
	return &Committer{elements}, nil
}
