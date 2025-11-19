package main

import (
	"testing"
)

func FuzzEVM(f *testing.F) {
	f.Fuzz(func(t *testing.T, a []byte) {
		db, err := createDB(dbFile)
		if err != nil {
			panic(err)
		}
		defer db.Close()
		if err := run(db, a); err != nil {
			panic(err)
		}
	})
}

func TestEVM(t *testing.T) {
	db, err := createDB(dbFile)
	if err != nil {
		panic(err)
	}
	db.Close()

	db, err = createDB(dbFile)
	if err != nil {
		panic(err)
	}
	defer db.Close()
}

func TestRepro(t *testing.T) {
	input := []byte("0¨\x99ž\xb8>\xb0]\xd17\b*\xe9ן:\xd1")
	db, err := createDB(dbFile)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	if err := run(db, input); err != nil {
		panic(err)
	}
}
