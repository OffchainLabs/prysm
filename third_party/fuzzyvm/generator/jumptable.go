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
	"encoding/binary"
)

type destination struct {
	pc       uint64
	jumpdest uint64
}

type Jumptable struct {
	minDist uint64
	dests   []destination
	toFill  []uint64
}

// NewJumptable creates a new Jumptable with a minimum jump distance.
func NewJumptable(minDist uint64) *Jumptable {
	return &Jumptable{
		minDist: minDist,
		dests:   make([]destination, 0),
	}
}

// Push pushes a new destination on the jumptable.
func (j *Jumptable) Push(pc, dest uint64) {
	j.dests = append(j.dests, destination{pc: pc, jumpdest: dest})
}

// Pop stores where a destination from the jumptable is needed.
// Returns a full uint64 as a placeholder
func (j *Jumptable) Pop(pc uint64) uint64 {
	j.toFill = append(j.toFill, pc)
	return ^uint64(0)
}

func (j *Jumptable) rem(index int) destination {
	dest := j.dests[index]
	j.dests[index] = j.dests[len(j.dests)-1]
	j.dests = j.dests[:len(j.dests)-1]
	return dest
}

func (j *Jumptable) InsertJumps(bytecode []byte) []byte {
	// The destination to fill starts either at pc + 1 (JUMP)
	// or at pc + 3
	for _, pc := range j.toFill {
		if pc, ok := checkCond(bytecode, pc); ok {
			set := false
			for i, dest := range j.dests {
				// allow jump if enough instructions passed
				if pc-dest.pc > j.minDist {
					bytecode = insertJumpdest(bytecode, pc, j.rem(i).jumpdest)
					set = true
					break
				}
				// allow forward jumps
				if pc < dest.jumpdest {
					bytecode = insertJumpdest(bytecode, pc, j.rem(i).jumpdest)
					set = true
					break
				}
			}
			if !set {
				// if no suitable destination found, set jumpdest to 0
				bytecode = insertJumpdest(bytecode, pc, 0)
			}
		}
	}
	return bytecode
}

func checkCond(bytecode []byte, pc uint64) (uint64, bool) {
	for i := uint64(1); i < 5; i++ {
		if bytecode[pc+i] == bytecode[pc+i+1] &&
			bytecode[pc+i] == bytecode[pc+i+2] &&
			bytecode[pc+i] == bytecode[pc+i+3] &&
			bytecode[pc+i] == bytecode[pc+i+4] &&
			bytecode[pc+i] == bytecode[pc+i+5] &&
			bytecode[pc+i] == bytecode[pc+i+6] &&
			bytecode[pc+i] == bytecode[pc+i+7] &&
			bytecode[pc+i] == byte(255) {
			return pc + i, true
		}
	}
	return 0, false
}

func insertJumpdest(bytecode []byte, pc, dest uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, dest)
	for i := uint64(0); i < 8; i++ {
		bytecode[pc+i] = b[i]
	}
	return bytecode
}
