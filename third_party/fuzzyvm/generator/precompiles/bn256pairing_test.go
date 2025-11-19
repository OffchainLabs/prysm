// Copyright 2020 Marius van der Wijden
// This file is part of the fuzzy-vm library.
//
// The fuzzy-vm library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The fuzzy-vm library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the fuzzy-vm library. If not, see <http://www.gnu.org/licenses/>.

package precompiles

import (
	"testing"

	"github.com/MariusVanDerWijden/FuzzyVM/filler"
	bn256 "github.com/ethereum/go-ethereum/crypto/bn256/cloudflare"
)

func TestPairing(t *testing.T) {
	i := 1
	data := []byte{1, 2, 3, 3, 4, 5, 6, 7, 7, 8, 8, 2, 2, 3}
	f := filler.NewFiller(data)
	a, b := pairing(i, f)
	out := bn256.PairingCheck(a, b)
	if !out {
		t.Fail()
	}
}
