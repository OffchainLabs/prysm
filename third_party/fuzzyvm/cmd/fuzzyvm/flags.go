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

package main

import (
	"runtime"

	"github.com/urfave/cli/v2"
)

var (
	countFlag = &cli.IntFlag{
		Name:  "count",
		Usage: "Number of tests that should be benched/executed/generated",
	}

	threadsFlag = &cli.IntFlag{
		Name:  "threads",
		Usage: "Number of generator threads started (default = NUMCPU)",
		Value: runtime.NumCPU(),
	}
)
