package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/epoch/precompute"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz/detect"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz/equality"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	prefixed "github.com/OffchainLabs/prysm/v6/runtime/logging/logrus-prefixed-formatter"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/kr/pretty"
	"github.com/pkg/errors"
	fssz "github.com/prysmaticlabs/fastssz"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"gopkg.in/d4l3k/messagediff.v1"
)

var blockPath string
var preStatePath string
var expectedPostStatePath string
var network string
var sszPath string
var sszType string
var prettyCommand = &cli.Command{
	Name:    "pretty",
	Aliases: []string{"p"},
	Usage:   "pretty-print SSZ data",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "ssz-path",
			Usage:       "Path to file(ssz)",
			Required:    true,
			Destination: &sszPath,
		},
		&cli.StringFlag{
			Name: "data-type",
			Usage: "ssz file data type: " +
				"block|" +
				"blinded_block|" +
				"signed_block|" +
				"attestation|" +
				"block_header|" +
				"deposit|" +
				"proposer_slashing|" +
				"signed_block_header|" +
				"signed_voluntary_exit|" +
				"voluntary_exit|" +
				"state_capella",
			Required:    true,
			Destination: &sszType,
		},
	},
	Action: func(c *cli.Context) error {
		var data fssz.Unmarshaler
		switch sszType {
		case "block":
			data = &ethpb.BeaconBlock{}
		case "signed_block":
			data = &ethpb.SignedBeaconBlock{}
		case "blinded_block":
			data = &ethpb.BlindedBeaconBlockBellatrix{}
		case "attestation":
			data = &ethpb.Attestation{}
		case "block_header":
			data = &ethpb.BeaconBlockHeader{}
		case "deposit":
			data = &ethpb.Deposit{}
		case "deposit_message":
			data = &ethpb.DepositMessage{}
		case "proposer_slashing":
			data = &ethpb.ProposerSlashing{}
		case "signed_block_header":
			data = &ethpb.SignedBeaconBlockHeader{}
		case "signed_voluntary_exit":
			data = &ethpb.SignedVoluntaryExit{}
		case "voluntary_exit":
			data = &ethpb.VoluntaryExit{}
		case "state_capella":
			data = &ethpb.BeaconStateCapella{}
		default:
			log.Fatal("Invalid type")
		}
		prettyPrint(sszPath, data)
		return nil
	},
}

var benchmarkHashCommand = &cli.Command{
	Name:    "benchmark-hash",
	Aliases: []string{"b"},
	Usage:   "benchmark-hash SSZ data",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "ssz-path",
			Usage:       "Path to file(ssz)",
			Required:    true,
			Destination: &sszPath,
		},
		&cli.StringFlag{
			Name: "data-type",
			Usage: "ssz file data type: " +
				"block_capella|" +
				"blinded_block_capella|" +
				"signed_block_capella|" +
				"attestation|" +
				"block_header|" +
				"deposit|" +
				"proposer_slashing|" +
				"signed_block_header|" +
				"signed_voluntary_exit|" +
				"voluntary_exit|" +
				"state_capella",
			Required:    true,
			Destination: &sszType,
		},
	},
	Action: func(c *cli.Context) error {
		benchmarkHash(sszPath, sszType)
		return nil
	},
}

var unrealizedCheckpointsCommand = &cli.Command{
	Name:     "unrealized-checkpoints",
	Category: "state-computations",
	Usage:    "Subcommand to compute manually the unrealized checkpoints",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "state-path",
			Usage:       "Path to state file(ssz)",
			Destination: &preStatePath,
		},
	},
	Action: func(c *cli.Context) error {
		if preStatePath == "" {
			log.Info("State path not provided, please provide path")
			reader := bufio.NewReader(os.Stdin)
			text, err := reader.ReadString('\n')
			if err != nil {
				log.Fatal(err)
			}
			if text = strings.ReplaceAll(text, "\n", ""); text == "" {
				log.Fatal("Empty state path given")
			}
			preStatePath = text
		}
		stateObj, err := detectState(preStatePath)
		if err != nil {
			log.Fatal(err)
		}
		preStateRoot, err := stateObj.HashTreeRoot(context.Background())
		if err != nil {
			log.Fatal(err)
		}
		log.Infof(
			"Computing unrealized justification for state at slot %d and root %#x",
			stateObj.Slot(),
			preStateRoot,
		)
		uj, uf, err := precompute.UnrealizedCheckpoints(stateObj)
		if err != nil {
			log.Fatal(err)
		}
		log.Infof("Computed:\nUnrealized Justified: (Root: %#x, Epoch: %d)\nUnrealized Finalized: (Root: %#x, Epoch: %d).", uj.Root, uj.Epoch, uf.Root, uf.Epoch)
		return nil
	},
}

var stateTransitionCommand = &cli.Command{
	Name:     "state-transition",
	Category: "state-computations",
	Usage:    "Subcommand to run manual state transitions",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "block-path",
			Usage:       "Path to block file(ssz)",
			Destination: &blockPath,
		},
		&cli.StringFlag{
			Name:        "pre-state-path",
			Usage:       "Path to pre state file(ssz)",
			Destination: &preStatePath,
		},
		&cli.StringFlag{
			Name:        "expected-post-state-path",
			Usage:       "Path to expected post state file(ssz)",
			Destination: &expectedPostStatePath,
		},
		&cli.StringFlag{
			Name:        "network",
			Usage:       "Network to run the state transition in",
			Destination: &network,
		},
	},
	Action: func(c *cli.Context) error {
		if network != "" {
			switch network {
			case params.SepoliaName:
				if err := params.SetActive(params.SepoliaConfig()); err != nil {
					log.Fatal(err)
				}
			case params.HoleskyName:
				if err := params.SetActive(params.HoleskyConfig()); err != nil {
					log.Fatal(err)
				}
			case params.HoodiName:
				if err := params.SetActive(params.HoodiConfig()); err != nil {
					log.Fatal(err)
				}
			default:
				log.Fatalf("Unknown network provided: %s", network)
			}
		}

		if blockPath == "" {
			log.Info("Block path not provided for state transition. " +
				"Please provide path")
			reader := bufio.NewReader(os.Stdin)
			text, err := reader.ReadString('\n')
			if err != nil {
				log.Fatal(err)
			}
			if text = strings.ReplaceAll(text, "\n", ""); text == "" {
				log.Fatal("Empty block path given")
			}
			blockPath = text
		}
		block, err := detectBlock(blockPath)
		if err != nil {
			log.Fatal(err)
		}
		blkRoot, err := block.Block().HashTreeRoot()
		if err != nil {
			log.Fatal(err)
		}
		if preStatePath == "" {
			log.Info("Pre State path not provided for state transition. " +
				"Please provide path")
			reader := bufio.NewReader(os.Stdin)
			text, err := reader.ReadString('\n')
			if err != nil {
				log.Fatal(err)
			}
			if text = strings.ReplaceAll(text, "\n", ""); text == "" {
				log.Fatal("Empty state path given")
			}
			preStatePath = text
		}
		stateObj, err := detectState(preStatePath)
		if err != nil {
			log.Fatal(err)
		}
		preStateRoot, err := stateObj.HashTreeRoot(context.Background())
		if err != nil {
			log.Fatal(err)
		}
		log.WithFields(log.Fields{
			"blockSlot":    fmt.Sprintf("%d", block.Block().Slot()),
			"preStateSlot": fmt.Sprintf("%d", stateObj.Slot()),
		}).Infof(
			"Performing state transition with a block root of %#x and pre state root of %#x",
			blkRoot,
			preStateRoot,
		)
		postState, err := debugStateTransition(context.Background(), stateObj, block)
		if err != nil {
			log.Fatal(err)
		}
		postRoot, err := postState.HashTreeRoot(context.Background())
		if err != nil {
			log.Fatal(err)
		}
		log.Infof("Finished state transition with post state root of %#x", postRoot)

		// Diff the state if a post state is provided.
		if expectedPostStatePath != "" {
			expectedState, err := detectState(expectedPostStatePath)
			if err != nil {
				log.Fatal(err)
			}
			if !equality.DeepEqual(expectedState.ToProtoUnsafe(), postState.ToProtoUnsafe()) {
				diff, _ := messagediff.PrettyDiff(expectedState.ToProtoUnsafe(), postState.ToProtoUnsafe())
				log.Errorf("Derived state differs from provided post state: %s", diff)
			}
		}
		return nil
	},
}

func main() {
	customFormatter := new(prefixed.TextFormatter)
	customFormatter.TimestampFormat = "2006-01-02 15:04:05.00"
	customFormatter.FullTimestamp = true
	log.SetFormatter(customFormatter)
	app := cli.App{}
	app.Name = "pcli"
	app.Usage = "A command line utility to run Ethereum consensus specific commands"
	app.Version = version.Version()
	app.Commands = []*cli.Command{
		prettyCommand,
		benchmarkHashCommand,
		unrealizedCheckpointsCommand,
		stateTransitionCommand,
	}
	if err := app.Run(os.Args); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
}

// dataFetcher fetches and unmarshals data from file to provided data structure.
func dataFetcher(fPath string, data fssz.Unmarshaler) error {
	rawFile, err := os.ReadFile(fPath) // #nosec G304
	if err != nil {
		return err
	}
	return data.UnmarshalSSZ(rawFile)
}

func detectState(fPath string) (state.BeaconState, error) {
	rawFile, err := os.ReadFile(fPath) // #nosec G304
	if err != nil {
		return nil, err
	}
	vu, err := detect.FromState(rawFile)
	if err != nil {
		return nil, errors.Wrap(err, "error detecting state from file")
	}
	s, err := vu.UnmarshalBeaconState(rawFile)
	if err != nil {
		return nil, errors.Wrap(err, "error unmarshalling state")
	}
	return s, nil
}

func detectBlock(fPath string) (interfaces.SignedBeaconBlock, error) {
	rawFile, err := os.ReadFile(fPath) // #nosec G304
	if err != nil {
		return nil, err
	}
	vu, err := detect.FromBlock(rawFile)
	if err != nil {
		return nil, err
	}
	return vu.UnmarshalBeaconBlock(rawFile)
}

func prettyPrint(sszPath string, data fssz.Unmarshaler) {
	if err := dataFetcher(sszPath, data); err != nil {
		log.Fatal(err)
	}
	str := pretty.Sprint(data)
	re := regexp.MustCompile("(?m)[\r\n]+^.*XXX_.*$")
	str = re.ReplaceAllString(str, "")
	fmt.Print(str)
}

func benchmarkHash(sszPath string, sszType string) {
	switch sszType {
	case "state_capella":
		st := &ethpb.BeaconStateCapella{}
		rawFile, err := os.ReadFile(sszPath) // #nosec G304
		if err != nil {
			log.Fatal(err)
		}

		startDeserialize := time.Now()
		if err := st.UnmarshalSSZ(rawFile); err != nil {
			log.Fatal(err)
		}
		deserializeDuration := time.Since(startDeserialize)

		stateTrieState, err := state_native.InitializeFromProtoCapella(st)
		if err != nil {
			log.Fatal(err)
		}
		start := time.Now()
		stat := &runtime.MemStats{}
		runtime.ReadMemStats(stat)
		root, err := stateTrieState.HashTreeRoot(context.Background())
		if err != nil {
			log.Fatal("Couldn't hash")
		}
		newStat := &runtime.MemStats{}
		runtime.ReadMemStats(newStat)
		fmt.Printf("Deserialize Duration: %v, Hashing Duration: %v HTR: %#x\n", deserializeDuration, time.Since(start), root)
		fmt.Printf("Total Memory Allocation Differential: %d bytes, Heap Memory Allocation Differential: %d bytes\n", int64(newStat.TotalAlloc)-int64(stat.TotalAlloc), int64(newStat.HeapAlloc)-int64(stat.HeapAlloc))
		return
	default:
		log.Fatal("Invalid type")
	}
}

func debugStateTransition(
	ctx context.Context,
	st state.BeaconState,
	signed interfaces.ReadOnlySignedBeaconBlock,
) (state.BeaconState, error) {
	var err error

	parentRoot := signed.Block().ParentRoot()
	st, err = transition.ProcessSlotsUsingNextSlotCache(ctx, st, parentRoot[:], signed.Block().Slot())
	if err != nil {
		return st, errors.Wrap(err, "could not process slots")
	}

	// Execute per block transition.
	set, st, err := transition.ProcessBlockNoVerifyAnySig(ctx, st, signed)
	if err != nil {
		return st, errors.Wrap(err, "could not process block")
	}
	var valid bool
	valid, err = set.VerifyVerbosely()
	if err != nil {
		return st, errors.Wrap(err, "could not batch verify signature")
	}
	if !valid {
		return st, errors.New("signature in block failed to verify")
	}
	return st, nil
}
