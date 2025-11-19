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

package filler

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
)

type testCase struct {
	data []byte
	used bool
}

var tests = []testCase{
	{[]byte{}, true},
	{[]byte{1, 2, 3, 4}, true},
	{[]byte{2, 1, 1, 2, 0, 0, 1, 1, 1, 1, 2, 2, 2, 2, 2, 2, 2, 2}, true},
	{[]byte{2, 1, 1, 2, 0, 0, 1, 1, 1, 1, 2, 2, 2, 2, 2, 2, 2, 2, 3}, false},
}

func TestFiller(t *testing.T) {
	for _, test := range tests {
		testFiller(t, test.data, test.used)
	}
}

func testFiller(t *testing.T, data []byte, usedUp bool) {
	filler := NewFiller(data)
	if filler == nil {
		return
	}
	filler.Byte()
	filler.ByteSlice(2)
	filler.ByteSlice256()
	filler.Uint32()
	filler.Uint64()
	if filler.usedUp != usedUp {
		t.Errorf("Filler failed: want %v, got %v", usedUp, filler.usedUp)
	}
}

func testFunc(t *testing.T, data []byte, exp []byte, fut func(f *Filler, exp []byte) bool) {
	filler := NewFiller(data)
	if !fut(filler, exp) {
		t.Errorf("test failed: input: %v exp: %v", data, exp)
	}
}

func TestByte(t *testing.T) {
	fut := func(f *Filler, exp []byte) bool {
		return f.Byte() == byte(exp[0])
	}
	for i := 0; i < 256; i++ {
		testFunc(t, []byte{byte(i)}, []byte{byte(i)}, fut)
	}
}

func TestBytes(t *testing.T) {
	fut := func(f *Filler, exp []byte) bool {
		got := f.ByteSlice256()
		fmt.Printf("got: %v", got)
		return bytes.Equal(got, exp)
	}
	type testCase struct {
		data []byte
		exp  []byte
	}
	tests := []testCase{
		{[]byte{1}, []byte{1}},
		{[]byte{0}, []byte{}},
		{[]byte{4}, []byte{4, 4, 4, 4}},
		{[]byte{2, 2, 3}, []byte{2, 3}},
		{[]byte{1, 2, 3, 4}, []byte{2}},
		{[]byte{6, 2, 3, 4}, []byte{2, 3, 4, 6, 2, 3}},
		{[]byte{2, 2, 3, 4}, []byte{2, 3}},
	}
	for _, test := range tests {
		testFunc(t, test.data, test.exp, fut)
	}
}

func TestBytesVBytes(t *testing.T) {
	fut := func(f *Filler, _ []byte) bool {
		a := f.ByteSlice256()
		aUsed := f.usedUp
		f.Reset()
		items := int(f.Byte())
		b := f.ByteSlice(items)
		bUsed := f.usedUp
		return bytes.Equal(a, b) && aUsed == bUsed
	}
	for _, test := range tests[1:] {
		testFunc(t, test.data, test.data, fut)
	}
}

func TestInts(t *testing.T) {
	filler := NewFiller([]byte{0, 1, 2, 3, 4, 5, 6})
	if filler.Uint32() != binary.BigEndian.Uint32([]byte{0, 1, 2, 3}) {
		t.Errorf("Uint32 wrong")
	}

	if filler.Uint64() != binary.BigEndian.Uint64([]byte{4, 5, 6, 0, 1, 2, 3, 4}) {
		t.Errorf("uint64 wrong")
	}

	if !filler.UsedUp() {
		t.Errorf("filler should been used up")
	}
}
