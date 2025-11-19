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

// Package main creates a fuzzer for Ethereum Virtual Machine (evm) implementations.
package main

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/urfave/cli/v2"

	"github.com/MariusVanDerWijden/FuzzyVM/benchmark"
	"github.com/MariusVanDerWijden/FuzzyVM/fuzzer"
	"github.com/ethereum/go-ethereum/common"
)

var benchCommand = &cli.Command{
	Name:   "bench",
	Usage:  "Starts a benchmarking run",
	Action: bench,
	Flags: []cli.Flag{
		countFlag,
	},
}

var corpusCommand = &cli.Command{
	Name:   "corpus",
	Usage:  "Generate corpus elements",
	Action: corpus,
	Flags: []cli.Flag{
		countFlag,
	},
}

var minCorpusCommand = &cli.Command{
	Name:   "minCorpus",
	Usage:  "Minimizes the corpus by removing duplicate elements",
	Action: minimizeCorpus,
}

var runCommand = &cli.Command{
	Name:   "run",
	Usage:  "Runs the fuzzer",
	Action: run,
	Flags: []cli.Flag{
		threadsFlag,
	},
}

func initApp() *cli.App {
	app := cli.NewApp()
	app.Name = "FuzzyVM"
	app.Usage = "Generator for Ethereum Virtual Machine tests"
	app.Commands = []*cli.Command{
		benchCommand,
		corpusCommand,
		minCorpusCommand,
		runCommand,
	}
	return app
}

var app = initApp()

const (
	outputRootDir = "out"
	crashesDir    = "crashes"
)

func main() {
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func bench(c *cli.Context) error {
	benchmark.RunFullBench(c.Int(countFlag.Name))
	return nil
}

func corpus(c *cli.Context) error {
	const dir = "corpus"
	ensureDirs(dir)
	n := c.Int(countFlag.Name)

	for i := 0; i < n; i++ {
		elem, err := fuzzer.CreateNewCorpusElement()
		if err != nil {
			fmt.Printf("Error while creating corpus: %v\n", err)
			return err
		}
		hash := sha1.Sum(elem)
		filename := fmt.Sprintf("%v/%v", dir, common.Bytes2Hex(hash[:]))
		if err := ioutil.WriteFile(filename, elem, 0755); err != nil {
			fmt.Printf("Error while writing corpus element: %v\n", err)
			return err
		}
	}
	return nil
}

func run(c *cli.Context) error {
	directories := []string{
		outputRootDir,
		crashesDir,
	}
	for i := 0; i < 256; i++ {
		directories = append(directories, fmt.Sprintf("%v/%v", outputRootDir, common.Bytes2Hex([]byte{byte(i)})))
	}
	ensureDirs(directories...)
	genThreads := c.Int(threadsFlag.Name)
	cmd := startGenerator(genThreads)
	return cmd.Wait()
}

func startGenerator(genThreads int) *exec.Cmd {
	var (
		cmdName = "go"
		target  = "FuzzVMBasic"
		dir     = "./fuzzer/..."
	)
	cmd := exec.Command(cmdName, "test", "--fuzz", target, "--parallel", fmt.Sprint(genThreads), dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Set the output directory
	path, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	directory := filepath.Join(path, outputRootDir)
	env := append(os.Environ(), fmt.Sprintf("%v=%v", fuzzer.EnvKey, directory))
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		panic(err)
	}
	return cmd
}

func minimizeCorpus(c *cli.Context) error {
	const dir = "corpus"
	ensureDirs(dir)
	infos, err := ioutil.ReadDir(outputRootDir)
	if err != nil {
		return err
	}
	toDelete := make(map[string]struct{})
	for i, info := range infos {
		f, err := ioutil.ReadFile(info.Name())
		if err != nil {
			continue
		}
		for k, info2 := range infos {
			if k == i {
				continue
			}
			h, err := ioutil.ReadFile(info2.Name())
			if err != nil {
				continue
			}
			if bytes.HasPrefix(h, f) {
				toDelete[info2.Name()] = struct{}{}
			}
		}
	}
	for name := range toDelete {
		fmt.Printf("Removing corpus file: %v\n", name)
		if err := os.Remove(name); err != nil {
			return err
		}
	}
	return nil
}

func ensureDirs(dirs ...string) {
	for _, dir := range dirs {
		_, err := os.Stat(dir)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("Creating directory: %v\n", dir)
				if err = os.Mkdir(dir, 0777); err != nil {
					fmt.Printf("Error while making the dir %q: %v\n", dir, err)
					return
				}
			} else {
				fmt.Printf("Error while using os.Stat dir %q: %v\n", dir, err)
			}
		}
	}
}
