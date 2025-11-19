package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/holiman/goevmlab/fuzzing"
)

func main() {
	createModexpStateTest()
}

// storeTest saves a testcase to disk
// returns true if a duplicate test was found
func storeTest(test *fuzzing.GeneralStateTest, path string) bool {
	// check if the test is already on disk
	if _, err := os.Stat(path); err == nil {
		fmt.Println("Duplicate test found")
		return true
	} else if !os.IsNotExist(err) {
		panic(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0755)
	if err != nil {
		panic(fmt.Sprintf("Could not open test file %q: %v", path, err))
	}
	defer f.Close()
	// Write to file
	encoder := json.NewEncoder(f)
	if err = encoder.Encode(test); err != nil {
		panic(fmt.Sprintf("Could not encode state test %q: %v", path, err))
	}
	return false
}
