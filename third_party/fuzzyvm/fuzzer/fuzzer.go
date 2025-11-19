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

// Package fuzzer is the entry point for go-fuzz.
package fuzzer

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/eth/tracers/logger"
	"github.com/ethereum/go-ethereum/tests"
	"github.com/holiman/goevmlab/fuzzing"
	"golang.org/x/crypto/sha3"

	"github.com/MariusVanDerWijden/FuzzyVM/filler"
	"github.com/MariusVanDerWijden/FuzzyVM/generator"
)

var (
	outputDir   = "out"
	EnvKey      = "FUZZYDIR"
	shouldTrace = false
)

// SetFuzzyVMDir sets the output directory for FuzzyVM
// If the environment variable FUZZYDIR is set, the output directory
// will be set to that, otherwise it will be set to a temp dir (for unit tests)
func SetFuzzyVMDir() {
	if dir, ok := os.LookupEnv(EnvKey); ok {
		outputDir = dir
	} else {
		outputDir = os.TempDir()
	}
}

func FuzzStateless(data []byte) int {
	if len(data) < 32 {
		return -1
	}
	f := filler.NewFiller(data)
	generator.GenerateProgram(f)
	return 0
}

// Fuzz is the entry point for go-fuzz
func Fuzz(data []byte) int {
	// Too little data destroys our performance and makes it hard for the generator
	if len(data) < 32 {
		return -1
	}
	f := filler.NewFiller(data)
	testMaker, _ := generator.GenerateProgram(f)
	// minimize test
	minimized, _, err := MinimizeProgram(testMaker)
	if err == nil {
		testMaker = minimized
	}
	hashed := hash(testMaker.ToGeneralStateTest("hashName"))
	finalName := fmt.Sprintf("FuzzyVM-%v", common.Bytes2Hex(hashed))
	// Execute the test and write out the resulting trace
	var traceFile *os.File
	if shouldTrace {
		traceFile = setupTrace(finalName)
		defer traceFile.Close()
	}
	if err := testMaker.Fill(traceFile); err != nil {
		panic(err)
	}
	// Save the test
	test := testMaker.ToGeneralStateTest(finalName)
	if storeTest(test, hashed, finalName) {
		return 0
	}
	if f.UsedUp() {
		return 0
	}
	return 1
}

func setupTrace(name string) *os.File {
	path := fmt.Sprintf("%v/%v-trace.jsonl", outputDir, name)
	traceFile, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0755)
	if err != nil {
		panic("Could not write out trace file")
	}
	return traceFile
}

func MinimizeProgram(test *fuzzing.GstMaker) (*fuzzing.GstMaker, []byte, error) {
	original := new(bytes.Buffer)
	if err := test.Fill(original); err != nil {
		return nil, nil, err
	}
	name := ""
	gstPtr := test.ToGeneralStateTest(name)
	gst := (*gstPtr)
	var addr common.Address
	var code []byte
	for ad, acc := range gst[name].Pre {
		if len(acc.Code) > len(code) {
			code = acc.Code
			addr = ad
		}
	}
	orgs := original.Bytes()
	idx := strings.LastIndex(string(orgs), "{")
	if idx <= 0 {
		idx = 0
	} else {
		idx -= 1
	}
	orgs = orgs[0:idx]
	foundLength := sort.Search(len(code), func(i int) bool {
		// Set the code
		acc := gst[name].Pre[addr]
		acc.Code = code[0:i]
		gst[name].Pre[addr] = acc
		// Run and see if the trace still matches
		var gethStateTest tests.StateTest
		data, err := json.Marshal(gst[name])
		if err != nil {
			panic(err)
		}
		if err := json.Unmarshal(data, &gethStateTest); err != nil {
			panic(err)
		}
		newOutput := new(bytes.Buffer)
		cfg := vm.Config{}
		cfg.Tracer = logger.NewJSONLogger(&logger.Config{}, newOutput)
		subtest := gethStateTest.Subtests()[0]
		gethStateTest.RunNoVerify(subtest, cfg, false, rawdb.HashScheme)
		newB := newOutput.Bytes()
		newIdx := strings.LastIndex(string(newB), "{")
		if newIdx <= 0 {
			newIdx = 0
		} else {
			newIdx -= 1
		}
		newB = newB[0:newIdx]
		//fmt.Printf("%v: %v %v\n", i, len(newB), len(orgs))
		//fmt.Printf(string(newB))
		return bytes.Equal(newB, orgs)
	})
	if foundLength+100 < len(code) {
		// Add some bytes to make it easier to proof differences in execution
		foundLength += 100
	}
	test.SetCode(addr, code[0:foundLength])
	return test, code[0:foundLength], nil
}

// storeTest saves a testcase to disk
// returns true if a duplicate test was found
func storeTest(test *fuzzing.GeneralStateTest, hashed []byte, testName string) bool {
	path := fmt.Sprintf("%v/%02x/%v.json", outputDir, hashed[0], testName)
	// check if the test is already on disk
	if _, err := os.Stat(path); err == nil {
		fmt.Println("Duplicate test found")
		return true
	} else if !os.IsNotExist(err) {
		panic(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0755)
	if err != nil {
		panic(fmt.Sprintf("Could not open test file %q: %v", testName, err))
	}
	defer f.Close()
	// Write to file
	encoder := json.NewEncoder(f)
	if err = encoder.Encode(test); err != nil {
		panic(fmt.Sprintf("Could not encode state test %q: %v", testName, err))
	}
	return false
}

func randTestName(data []byte) string {
	var seedData [8]byte
	copy(seedData[:], data)
	seed := int64(binary.BigEndian.Uint64(seedData[:]))
	rand := rand.New(rand.NewSource(time.Now().UnixNano() ^ seed))
	return fmt.Sprintf("FuzzyVM-%v-%v", rand.Int31(), rand.Int31())
}

func hash(test *fuzzing.GeneralStateTest) []byte {
	h := sha3.New256()
	encoder := json.NewEncoder(h)
	if err := encoder.Encode(test); err != nil {
		panic(fmt.Sprintf("Could not hash state test: %v", err))
	}
	return h.Sum(nil)
}
