package main

import (
	"encoding/binary"

	"github.com/MariusVanDerWijden/FuzzyVM/filler"
	"github.com/MariusVanDerWijden/FuzzyVM/generator"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/core/vm/program"
	"github.com/holiman/uint256"
)

func createModexpStateTest() {
	base := common.FromHex("0xffffffffffffffff")
	exp := []byte{}
	for range 0x51 {
		exp = append(exp, 0xff)
	}
	mod := common.FromHex("0xffffffffffffffff")
	code := createModTestcase(base, exp, mod)
	f := filler.NewFiller([]byte("\x5a\x5a\x5a\x5a\x5a\x5a\x5a"))
	testMaker := generator.CreateGstMaker(f, code)
	if err := testMaker.Fill(nil); err != nil {
		panic(err)
	}
	// Save the test
	test := testMaker.ToGeneralStateTest("statetest.json")
	storeTest(test, "statetest.json")
}

func createModTestcase(base, exp, mod []byte) []byte {
	p := program.New()
	baseLen := make([]byte, 4)
	binary.BigEndian.PutUint32(baseLen, uint32(len(base)))
	expLen := make([]byte, 4)
	binary.BigEndian.PutUint32(expLen, uint32(len(expLen)))
	modLen := make([]byte, 4)
	binary.BigEndian.PutUint32(modLen, uint32(len(mod)))

	p.MstoreSmall(baseLen, 0x00)      // base size
	p.MstoreSmall([]byte{0x51}, 0x20) // exponent size
	p.MstoreSmall([]byte{0x08}, 0x40) // modulo size

	p.Mstore(base, 0x60)
	offset := 0x60 + uint32(len(base))
	p.Mstore(exp, offset)
	offset += uint32(len(exp))
	p.Mstore(mod, offset)
	offset += uint32(len(mod))

	_, dest := p.Jumpdest()
	p.StaticCall(uint256.NewInt(0xffff), 0x5, 0, offset, offset, len(mod))
	p.Op(vm.POP)
	p.Jump(dest)

	return p.Bytes()
}
