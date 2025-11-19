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

// Package filler can fill objects based on a provided data source.
package filler

import (
	"encoding/binary"
	"math/big"
)

// Filler can be used to fill objects from a data source.
type Filler struct {
	data    []byte
	pointer int
	usedUp  bool
}

// NewFiller creates a new Filler.
func NewFiller(data []byte) *Filler {
	if len(data) == 0 {
		data = make([]byte, 1)
	}
	return &Filler{
		data:    data,
		pointer: 0,
		usedUp:  false,
	}
}

// incPointer increments the internal pointer
// to the next position to be read.
func (f *Filler) incPointer(i int) {
	if f.pointer+i >= len(f.data) {
		f.usedUp = true
	}
	f.pointer = (f.pointer + i) % len(f.data)
}

// Bool returns a new bool.
func (f *Filler) Bool() bool {
	b := f.Byte()
	return b > 127
}

// Byte returns a new byte.
func (f *Filler) Byte() byte {
	b := f.data[f.pointer]
	f.incPointer(1)
	return b
}

// Read implements the io.Reader interface.
func (f *Filler) Read(b []byte) (n int, err error) {
	// TODO (MariusVanDerWijden) this can be done more efficiently
	tmp := f.ByteSlice(len(b))
	for i := 0; i < len(b); i++ {
		b[i] = tmp[i]
	}
	return len(b), nil
}

// BigInt16 returns a new big int in [0, 2^16).
func (f *Filler) BigInt16() *big.Int {
	i := f.Uint16()
	return big.NewInt(int64(i))
}

// BigInt32 returns a new big int in [0, 2^32).
func (f *Filler) BigInt32() *big.Int {
	i := f.Uint32()
	return big.NewInt(int64(i))
}

// BigInt64 returns a new big int in [0, 2^64).
func (f *Filler) BigInt64() *big.Int {
	i := f.ByteSlice(8)
	return new(big.Int).SetBytes(i)
}

// BigInt256 returns a new big int in [0, 2^256).
func (f *Filler) BigInt256() *big.Int {
	i := f.ByteSlice(32)
	return new(big.Int).SetBytes(i)
}

// GasInt returns a new big int to be used as a gas value.
// With probability 254/255 its in [0, 20.000.000].
// With probability 1/255 its in [0, 2^32].
func (f *Filler) GasInt() *big.Int {
	b := f.Byte()
	if b == 253 {
		return f.BigInt32()
	} else if b == 254 {
		return f.BigInt64()
	} else if b == 255 {
		return f.BigInt256()
	}
	i := f.BigInt32()
	return i.Mod(i, big.NewInt(20000000))
}

// MemInt returns a new big int to be used as a memory or offset value.
// With probability 252/255 its in [0, 256].
// With probability 1/255 its in [0, 2^32].
// With probability 1/255 its in [0, 2^64].
// With probability 1/255 its in [0, 2^256].
func (f *Filler) MemInt() *big.Int {
	b := f.Byte()
	if b == 253 {
		return f.BigInt32()
	} else if b == 254 {
		return f.BigInt64()
	} else if b == 255 {
		return f.BigInt256()
	}
	return big.NewInt(int64(f.Byte()))
}

// ByteSlice returns a byteslice with `items` values.
func (f *Filler) ByteSlice(items int) []byte {
	// TODO (MariusVanDerWijden) this can be done way more efficiently
	b := make([]byte, items)
	if f.pointer+items < len(f.data) {
		copy(b, f.data[f.pointer:])
	} else {
		// Not enough data available
		for i := 0; i < items; {
			it := copy(b[i:], f.data[f.pointer:])
			if it == 0 {
				panic("should not happen, infinite loop")
			}
			i += it
			f.pointer = 0
		}
		f.usedUp = true
	}
	f.incPointer(items)
	return b
}

// ByteSlice256 returns a byteslice with 0..255 values.
func (f *Filler) ByteSlice256() []byte {
	return f.ByteSlice(int(f.Byte()))
}

// Uint16 returns a new uint16.
func (f *Filler) Uint16() uint16 {
	return binary.BigEndian.Uint16(f.ByteSlice(2))
}

// Uint32 returns a new uint32.
func (f *Filler) Uint32() uint32 {
	return binary.BigEndian.Uint32(f.ByteSlice(4))
}

// Uint64 returns a new uint64.
func (f *Filler) Uint64() uint64 {
	return binary.BigEndian.Uint64(f.ByteSlice(8))
}

// Reset resets a filler.
func (f *Filler) Reset() {
	f.pointer = 0
	f.usedUp = false
}

// UsedUp returns wether all bytes from the source have been used.
func (f *Filler) UsedUp() bool {
	return f.usedUp
}
