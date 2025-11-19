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

package generator

import (
	"crypto/rand"
	"testing"

	"github.com/MariusVanDerWijden/FuzzyVM/filler"
)

func TestGenerator(t *testing.T) {
	inputEscaped := "\xb7vx\x99\xee\xd8q~\xf0\xd1\x18\xb5\xcb\xf6\x87\x9b\"{\xad\xae" +
		"\xa6ߩ\xfdE$>\n%9\x84]\x9b\xb0;\xf2\x96\xc0\u0378" +
		"\xc0X\x00\x04\x00\x00\x1e7\n.\x88\x85u\x0e\xf6g\xb2}\x01\x02" +
		"ok\xef\n{FӃ\xf6\v\xc6C\x17\x1d\xbf\b\x83\xad\v\xa3" +
		"\x17\xd1q\x88{\xb5'Td\xd5ڛ#D|-ȳ\xadt" +
		"4\xcb|\x18\x14\xbfX\xf4J\x85\x11e\xa4\xb7\xcb\xf8K\x9e\xe5\x8a" +
		"\xdc\x14\r\xaas\b:\x17\x8b\xee\xda\xf7\xe7\xe6Ź\xf2 -'" +
		"\x87l\x82C\u007fx\x80\xb7t\x10\xb3\"\xcd\x1e\xfd\xb4-\xda\xf8\x8b" +
		"\xf5\x1b?v\xb8ތ\x93�\x0f\xc1S\r_G\xabz\x98" +
		"'\f\xe3&\x18\x87\x1b\x1f\u007f\xf2\xe6$\x86\xfa(\xab\xa2\xa7L~" +
		"\xe5V\x9d\x03\xbf\xb1\xb6\xcf\xf6\xab<\xd1\nq\xcenE}3U" +
		"\x99l\x93\x1a\xd1\xdd'\x1d\x13?K.\a~\xd0\xcb\x18b\xd5\xca" +
		"\x18>\x9e\x9dP%'Ͻ\x93\xbb8#,^y\x91\xe3\x8a\xc6" +
		"A\xee\x12\xc0\xf3\xac\xbdXA\xe1v\xed\x01\\\xdfL405\x89" +
		"NewTɬ\xf7\x16&,\x81\xbaO䣢\x1eE\x8b\xec" +
		"\xf8g\xffc\xdb\xf6\xa0\xce\xf9\xef\u007f\xfc\xa3i\xb8\xe1\xa7\xeb\xc5G" +
		"\xfc\xb4&\x9a\x04'~\x15h\xcb\xfc\u05fb"

	input := []byte(inputEscaped)
	filler := filler.NewFiller(input)
	_, p := GenerateProgram(filler)
	if len(p) == 0 {
		panic("Should generate a program")
	}
}

func TestRandomGenerator(t *testing.T) {
	input := make([]byte, 100000)
	rand.Read(input)
	filler := filler.NewFiller(input)
	GenerateProgram(filler)
}
