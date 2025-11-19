package main

import (
	"crypto/sha256"
	"fmt"
	"sync"

	"github.com/MariusVanDerWijden/FuzzyVM/filler"
	"github.com/MariusVanDerWijden/FuzzyVM/fuzzer"
	"github.com/MariusVanDerWijden/FuzzyVM/generator"
	"github.com/cockroachdb/pebble"
	"github.com/pkg/errors"
)

var dbFile = "fuzzyvm-db.pebble"

func main() {
	db, err := createDB(dbFile)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	inner := db.(*pebbleDB)
	metrics := inner.db.Metrics()
	fmt.Printf("Reading metrics for %v\n", dbFile)
	fmt.Printf("Estimated disk usage: %.2fM\n", float64(metrics.DiskSpaceUsage())/1024/1024)
	fmt.Printf("Key count: %v\n", countKeys(inner))
}

func countKeys(db *pebbleDB) int {
	iter, err := db.db.NewIter(&pebble.IterOptions{LowerBound: []byte{}, UpperBound: []byte{}})
	if err != nil {
		panic(err)
	}
	keys := 0
	iter.First()
	for iter.Next() {
		keys++
	}
	return keys
}

func createDB(file string) (db, error) {
	db, err := pebble.Open(file, &pebble.Options{})
	return &pebbleDB{
		db: db,
	}, err
}

func makeKey(code []byte) []byte {
	hash := sha256.Sum256(code)
	return hash[:]
}

type db interface {
	Get(key []byte) ([]byte, error)
	Set(key, value []byte) error
	Close() error
}

type pebbleDB struct {
	db *pebble.DB
	mu sync.RWMutex
}

func (db *pebbleDB) Get(key []byte) ([]byte, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	val, closer, err := db.db.Get(key)
	if closer != nil {
		defer closer.Close()
	}
	return val, err
}

func (db *pebbleDB) Set(key, value []byte) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	return db.db.Set(key, value, &pebble.WriteOptions{})
}

func (db *pebbleDB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	return db.db.Close()
}

func hasCode(db db, code []byte) bool {
	key := makeKey(code)
	if _, err := db.Get(key); err == pebble.ErrNotFound {
		return false
	} else if err != nil {
		panic(err)
	}
	return true
}

func putCode(db db, code []byte) error {
	key := makeKey(code)
	return db.Set(key, code)
}

func run(db db, input []byte) error {
	f := filler.NewFiller(input)
	gst, bytecode := generator.GenerateProgram(f)
	if hasCode(db, bytecode) {
		// already have this code in our db, skip
		return nil
	}
	_, minCode, err := fuzzer.MinimizeProgram(gst)
	if err != nil {
		return errors.Wrap(err, "error minimizing program")
	}

	if err := putCode(db, bytecode); err != nil {
		return err
	}
	return putCode(db, minCode)
}
