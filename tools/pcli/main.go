package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"slices"
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
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
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
var sszObtained string
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
				"signed_block|" +
				"attestation|" +
				"block_header|" +
				"deposit|" +
				"proposer_slashing|" +
				"signed_block_header|" +
				"signed_voluntary_exit|" +
				"voluntary_exit|" +
				"state_electra" +
				"state_eip7732",
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
			data = &ethpb.SignedBeaconBlockEpbs{}
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
		case "state_electra":
			data = &ethpb.BeaconStateElectra{}
		case "state_eip7732":
			data = &ethpb.BeaconStateEPBS{}
		default:
			log.Fatal("Invalid type")
		}
		prettyPrint(sszPath, data)
		return nil
	},
}

var diffCommand = &cli.Command{
	Name:    "diff",
	Aliases: []string{"d"},
	Usage:   "diff --wanted file --obtained file --data-type type",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "wanted",
			Usage:       "Path to file",
			Required:    true,
			Destination: &sszPath,
		},
		&cli.StringFlag{
			Name:        "obtained",
			Usage:       "Path to file",
			Required:    true,
			Destination: &sszObtained,
		},
		&cli.StringFlag{
			Name: "data-type",
			Usage: "data type: " +
				"block|" +
				"blinded_block|" +
				"signed_block|" +
				"signed_block|" +
				"attestation|" +
				"block_header|" +
				"deposit|" +
				"proposer_slashing|" +
				"signed_block_header|" +
				"signed_voluntary_exit|" +
				"voluntary_exit|" +
				"state_eip7732",
			Required:    true,
			Destination: &sszType,
		},
	},
	Action: func(c *cli.Context) error {
		switch sszType {
		case "state_eip7732":
			diffStateEpbs(sszPath, sszObtained)
			return nil
		default:
			log.Fatal("Invalid type")
		}
		return nil
	},
}

var hashCommand = &cli.Command{
	Name:    "hash",
	Aliases: []string{"u"},
	Usage:   "hash SSZ",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "ssz-path",
			Usage:       "Path to file(ssz)",
			Required:    true,
			Destination: &sszPath,
		},
		&cli.StringFlag{
			Name: "data-type",
			Usage: "data type: " +
				"block|" +
				"blinded_block|" +
				"signed_block|" +
				"signed_block|" +
				"attestation|" +
				"block_header|" +
				"deposit|" +
				"proposer_slashing|" +
				"signed_block_header|" +
				"signed_voluntary_exit|" +
				"voluntary_exit|" +
				"state_eip7732",
			Required:    true,
			Destination: &sszType,
		}},
	Action: func(c *cli.Context) error {
		hash(sszPath, sszType)
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
		hashCommand,
		unrealizedCheckpointsCommand,
		stateTransitionCommand,
		diffCommand,
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

func hash(sszPath string, sszType string) {
	var data fssz.Unmarshaler
	switch sszType {
	case "signed_block":
		data = &ethpb.SignedBeaconBlockEpbs{}
	case "state_eip7732":
		data = &ethpb.BeaconStateEPBS{}
	default:
		log.Fatal("Invalid type")
	}
	if err := dataFetcher(sszPath, data); err != nil {
		log.Fatal(err)
	}
	hasher, ok := data.(fssz.HashRoot)
	if !ok {
		log.Fatal("Data does not implement HashRoot interface")
	}
	root, err := hasher.HashTreeRoot()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("HTR: %#x\n", root)
}

func diffStateEpbs(sszPath string, sszWanted string) {
	wanted := &ethpb.BeaconStateEPBS{}
	obtained := &ethpb.BeaconStateEPBS{}
	if err := dataFetcher(sszPath, wanted); err != nil {
		log.Fatal(err)
	}
	if err := dataFetcher(sszWanted, obtained); err != nil {
		log.Fatal(err)
	}

	if wanted.GenesisTime != obtained.GenesisTime {
		fmt.Printf("Genesis Time: Wanted: %d, Obtained: %d\n", wanted.GenesisTime, obtained.GenesisTime)
	}
	if !slices.Equal(wanted.GenesisValidatorsRoot, obtained.GenesisValidatorsRoot) {
		fmt.Printf("Genesis Validators Root: Wanted: %#x, Obtained: %#x\n", wanted.GenesisValidatorsRoot, obtained.GenesisValidatorsRoot)
	}
	if wanted.Slot != obtained.Slot {
		fmt.Printf("Slot: Wanted: %d, Obtained: %d\n", wanted.Slot, obtained.Slot)
	}
	compareForks(wanted.Fork, obtained.Fork)
	compareBlockHeaders(wanted.LatestBlockHeader, obtained.LatestBlockHeader)
	compareRootsSlices("Block Roots", wanted.BlockRoots, obtained.BlockRoots)
	compareRootsSlices("State Roots", wanted.StateRoots, obtained.StateRoots)
	compareRootsSlices("Historical Roots", wanted.HistoricalRoots, obtained.HistoricalRoots)
	if eth1DataDiffers(wanted.Eth1Data, obtained.Eth1Data) {
		printEth1Data(wanted.Eth1Data, obtained.Eth1Data)
	}
	compareEth1Slices(wanted.Eth1DataVotes, obtained.Eth1DataVotes)
	if wanted.Eth1DepositIndex != obtained.Eth1DepositIndex {
		fmt.Printf("Eth1 Deposit Index: Wanted: %d, Obtained: %d\n", wanted.Eth1DepositIndex, obtained.Eth1DepositIndex)
	}
	compareValidatorsSlices(wanted.Validators, obtained.Validators)
	compareUintSlices("Balances", wanted.Balances, obtained.Balances)
	compareRootsSlices("Randao Mixes", wanted.RandaoMixes, obtained.RandaoMixes)
	compareUintSlices("Slashings", wanted.Slashings, obtained.Slashings)
	compareByteSlices("Previous Epoch Participation", wanted.PreviousEpochParticipation, obtained.PreviousEpochParticipation)
	compareByteSlices("Current Epoch Participation", wanted.CurrentEpochParticipation, obtained.CurrentEpochParticipation)
	if !slices.Equal(wanted.JustificationBits, obtained.JustificationBits) {
		fmt.Printf("Justification Bits: Wanted: %#x, Obtained: %#x\n", wanted.JustificationBits, obtained.JustificationBits)
	}
	compareCheckpoints(wanted.PreviousJustifiedCheckpoint, obtained.PreviousJustifiedCheckpoint)
	compareCheckpoints(wanted.CurrentJustifiedCheckpoint, obtained.CurrentJustifiedCheckpoint)
	compareCheckpoints(wanted.FinalizedCheckpoint, obtained.FinalizedCheckpoint)
	compareUintSlices("Inactivity Scores", wanted.InactivityScores, obtained.InactivityScores)
	if !slices.Equal(wanted.CurrentSyncCommittee.AggregatePubkey, obtained.CurrentSyncCommittee.AggregatePubkey) {
		fmt.Printf("Current Sync Committee Aggregate Pubkey: Wanted: %#x, Obtained: %#x\n", wanted.CurrentSyncCommittee.AggregatePubkey, obtained.CurrentSyncCommittee.AggregatePubkey)
	}
	if !slices.Equal(wanted.NextSyncCommittee.AggregatePubkey, obtained.NextSyncCommittee.AggregatePubkey) {
		fmt.Printf("Next Sync Committee Aggregate Pubkey: Wanted: %#x, Obtained: %#x\n", wanted.NextSyncCommittee.AggregatePubkey, obtained.NextSyncCommittee.AggregatePubkey)
	}
	compareExecutionPayloadHeaderEpbs(wanted.LatestExecutionPayloadHeader, obtained.LatestExecutionPayloadHeader)
	if wanted.NextWithdrawalIndex != obtained.NextWithdrawalIndex {
		fmt.Printf("Next Withdrawal Index: Wanted: %d, Obtained: %d\n", wanted.NextWithdrawalIndex, obtained.NextWithdrawalIndex)
	}
	if wanted.NextWithdrawalValidatorIndex != obtained.NextWithdrawalValidatorIndex {
		fmt.Printf("Next Withdrawal Validator Index: Wanted: %d, Obtained: %d\n", wanted.NextWithdrawalValidatorIndex, obtained.NextWithdrawalValidatorIndex)
	}
	compareHistoricalSummaries(wanted.HistoricalSummaries, obtained.HistoricalSummaries)
	if wanted.DepositRequestsStartIndex != obtained.DepositRequestsStartIndex {
		fmt.Printf("Deposit Requests Start Index: Wanted: %d, Obtained: %d\n", wanted.DepositRequestsStartIndex, obtained.DepositRequestsStartIndex)
	}
	if wanted.DepositBalanceToConsume != obtained.DepositBalanceToConsume {
		fmt.Printf("Deposit Balance To Consumer: Wanted: %d, Obtained: %d\n", wanted.DepositBalanceToConsume, obtained.DepositBalanceToConsume)
	}
	if wanted.ExitBalanceToConsume != obtained.ExitBalanceToConsume {
		fmt.Printf("Exit Balance To Consumer: Wanted: %d, Obtained: %d\n", wanted.ExitBalanceToConsume, obtained.ExitBalanceToConsume)
	}
	if wanted.EarliestExitEpoch != obtained.EarliestExitEpoch {
		fmt.Printf("Earliest Exit Epoch: Wanted: %d, Obtained: %d\n", wanted.EarliestExitEpoch, obtained.EarliestExitEpoch)
	}
	if wanted.EarliestConsolidationEpoch != obtained.EarliestConsolidationEpoch {
		fmt.Printf("Earliest Consolidation Epoch: Wanted: %d, Obtained: %d\n", wanted.EarliestConsolidationEpoch, obtained.EarliestConsolidationEpoch)
	}
	comparePendingDeposits(wanted.PendingDeposits, obtained.PendingDeposits)
	comparePendingPartialWithdrawals(wanted.PendingPartialWithdrawals, obtained.PendingPartialWithdrawals)
	comparePendingConsolidations(wanted.PendingConsolidations, obtained.PendingConsolidations)
	if !slices.Equal(wanted.LatestBlockHash, obtained.LatestBlockHash) {
		fmt.Printf("Latest Block Hash: Wanted: %#x, Obtained: %#x\n", wanted.LatestBlockHash, obtained.LatestBlockHash)
	}
	if wanted.LatestFullSlot != obtained.LatestFullSlot {
		fmt.Printf("Latest Full Slot: Wanted: %d, Obtained: %d\n", wanted.LatestFullSlot, obtained.LatestFullSlot)
	}
	if !slices.Equal(wanted.LatestWithdrawalsRoot, obtained.LatestWithdrawalsRoot) {
		fmt.Printf("Last Withdrawals Root: Wanted: %#x, Obtained: %#x\n", wanted.LatestWithdrawalsRoot, obtained.LatestWithdrawalsRoot)
	}
}

func printPendingConsolidation(wanted, obtained *ethpb.PendingConsolidation) {
	fmt.Printf("Pending Concolidation Wanted:\n    Source Index: %d, Target Index: %d\n", wanted.SourceIndex, wanted.TargetIndex)
	fmt.Printf("Pending Concolidation Obtained:\n    Source Index: %d, Target Index: %d\n", obtained.SourceIndex, obtained.TargetIndex)
}

func comparePendingConsolidations(wanted, obtained []*ethpb.PendingConsolidation) {
	different := 0
	for i, consolidation := range wanted {
		if consolidation.SourceIndex != obtained[i].SourceIndex ||
			consolidation.TargetIndex != obtained[i].TargetIndex {
			if different == 0 {
				fmt.Println("Pending Consolidations differ:")
			}
			fmt.Printf("    %d: ", i)
			printPendingConsolidation(consolidation, obtained[i])
			different++
			if different > 10 {
				fmt.Println("    ...")
				break
			}
		}
	}
}

func printPendingPartialWithdrawal(wanted, obtained *ethpb.PendingPartialWithdrawal) {
	fmt.Printf("Pending Partial Withdrawal Wanted: \n   Index: %d, Amount: %d, Withdrawable Epoch: %d\n", wanted.Index, wanted.Amount, wanted.WithdrawableEpoch)
	fmt.Printf("Pending Partial Withdrawal Obtained: \n   Index: %d, Amount: %d, Withdrawable Epoch: %d\n", obtained.Index, obtained.Amount, obtained.WithdrawableEpoch)
}

func comparePendingPartialWithdrawals(wanted, obtained []*ethpb.PendingPartialWithdrawal) {
	different := 0
	for i, withdrawal := range wanted {
		if withdrawal.Index != obtained[i].Index ||
			withdrawal.Amount != obtained[i].Amount ||
			withdrawal.WithdrawableEpoch != obtained[i].WithdrawableEpoch {
			if different == 0 {
				fmt.Println("Pending Partial Withdrawals differ:")
			}
			fmt.Printf("    %d: ", i)
			printPendingPartialWithdrawal(withdrawal, obtained[i])
			different++
			if different > 10 {
				fmt.Println("    ...")
				break
			}
		}
	}
}

func printPendingDeposit(wanted, obtained *ethpb.PendingDeposit) {
	fmt.Printf("PEnding Deposit Wanted:\n    PublicKey: %#x, Withdrawal Credentials: %#x, Amount: %d, Signature: %#x, Slot: %d\n", wanted.PublicKey, wanted.WithdrawalCredentials, wanted.Amount, wanted.Signature, wanted.Slot)
	fmt.Printf("Pending Deposit Obtained:\n    PublicKey: %#x, Withdrawal Credentials: %#x, Amount: %d, Signature: %#x, Slot: %d\n", obtained.PublicKey, obtained.WithdrawalCredentials, obtained.Amount, obtained.Signature, obtained.Slot)
}
func comparePendingDeposits(wanted, obtained []*ethpb.PendingDeposit) {
	different := 0
	for i, deposit := range wanted {
		if !slices.Equal(deposit.PublicKey, obtained[i].PublicKey) ||
			!slices.Equal(deposit.WithdrawalCredentials, obtained[i].WithdrawalCredentials) ||
			deposit.Amount != obtained[i].Amount ||
			!slices.Equal(deposit.Signature, obtained[i].Signature) ||
			deposit.Slot != obtained[i].Slot {
			if different == 0 {
				fmt.Println("Pending Deposits differ:")
			}
			fmt.Printf("    %d: ", i)
			printPendingDeposit(deposit, obtained[i])
			different++
			if different > 10 {
				fmt.Println("    ...")
				break
			}
		}
	}
}

func printHistoricalSummary(wanted, obtained *ethpb.HistoricalSummary) {
	fmt.Printf("Historical Summary Wanted:\n    Block Summary Root: %#x\n    State Summary Root: %#x\n", wanted.BlockSummaryRoot, wanted.StateSummaryRoot)
	fmt.Printf("Historical Summary Obtained:\n    Block Summary Root: %#x\n    State Summary Root: %#x\n", obtained.BlockSummaryRoot, obtained.StateSummaryRoot)
}

func compareHistoricalSummaries(wanted, obtained []*ethpb.HistoricalSummary) {
	different := 0
	for i, summary := range wanted {
		if !slices.Equal(summary.BlockSummaryRoot, obtained[i].BlockSummaryRoot) ||
			!slices.Equal(summary.StateSummaryRoot, obtained[i].StateSummaryRoot) {
			if different == 0 {
				fmt.Println("Historical Summaries differ:")
			}
			fmt.Printf("    %d: ", i)
			printHistoricalSummary(summary, obtained[i])
			different++
			if different > 10 {
				fmt.Println("    ...")
				break
			}
		}
	}
}

func printExecutionPayloadHeaderEpbs(wanted, obtained *enginev1.ExecutionPayloadHeaderEPBS) {
	fmt.Printf("Execution Payload Header Wanted:\n  ParentBlockHash: %#x,  ParentBlockRoot: %#x,  BlockHash: %#x, Gaslimit: %d,  BuilderIndex: %d,  Slot: %d,  Value: %d,  BlobKzgCommitmentsRoot: %#x\n", wanted.ParentBlockHash, wanted.ParentBlockRoot, wanted.BlockHash, wanted.GasLimit, wanted.BuilderIndex, wanted.Slot, wanted.Value, wanted.BlobKzgCommitmentsRoot)
	fmt.Printf("Execution Payload Header Obtained:\n  ParentBlockHash: %#x,  ParentBlockRoot: %#x,  BlockHash: %#x, Gaslimit: %d,  BuilderIndex: %d,  Slot: %d,  Value: %d,  BlobKzgCommitmentsRoot: %#x\n", obtained.ParentBlockHash, obtained.ParentBlockRoot, obtained.BlockHash, obtained.GasLimit, obtained.BuilderIndex, obtained.Slot, obtained.Value, obtained.BlobKzgCommitmentsRoot)
}

func compareExecutionPayloadHeaderEpbs(wanted, obtained *enginev1.ExecutionPayloadHeaderEPBS) {
	if !slices.Equal(wanted.ParentBlockHash, obtained.ParentBlockHash) {
		printExecutionPayloadHeaderEpbs(wanted, obtained)
		return
	}
	if !slices.Equal(wanted.ParentBlockRoot, obtained.ParentBlockRoot) {
		printExecutionPayloadHeaderEpbs(wanted, obtained)
		return
	}
	if !slices.Equal(wanted.BlockHash, obtained.BlockHash) {
		printExecutionPayloadHeaderEpbs(wanted, obtained)
		return
	}
	if wanted.GasLimit != obtained.GasLimit {
		printExecutionPayloadHeaderEpbs(wanted, obtained)
		return
	}
	if wanted.BuilderIndex != obtained.BuilderIndex {
		printExecutionPayloadHeaderEpbs(wanted, obtained)
		return
	}
	if wanted.Slot != obtained.Slot {
		printExecutionPayloadHeaderEpbs(wanted, obtained)
		return
	}
	if wanted.Value != obtained.Value {
		printExecutionPayloadHeaderEpbs(wanted, obtained)
		return
	}
	if !slices.Equal(wanted.BlobKzgCommitmentsRoot, obtained.BlobKzgCommitmentsRoot) {
		printExecutionPayloadHeaderEpbs(wanted, obtained)
		return
	}
}

func printCheckpoint(wanted, obtained *ethpb.Checkpoint) {
	fmt.Printf("Checkpoint Wanted:\n    Root: %#x\n    Epoch: %d\n", wanted.Root, wanted.Epoch)
	fmt.Printf("Checkpoint Obtained:\n    Root: %#x\n    Epoch: %d\n", obtained.Root, obtained.Epoch)
}
func compareCheckpoints(wanted, obtained *ethpb.Checkpoint) {
	if !slices.Equal(wanted.Root, obtained.Root) {
		printCheckpoint(wanted, obtained)
		return
	}
	if wanted.Epoch != obtained.Epoch {
		printCheckpoint(wanted, obtained)
		return
	}
}

func compareByteSlices(label string, wanted, obtained []byte) {
	different := 0
	for i, val := range wanted {
		if val != obtained[i] {
			if different == 0 {
				fmt.Printf("%s differ:\n", label)
			}
			fmt.Printf("    %d: Wanted: %d, Obtained: %d\n", i, val, obtained[i])
			different++
			if different > 10 {
				fmt.Println("    ...")
				break
			}
		}
	}
}
func compareUintSlices(label string, wanted, obtained []uint64) {
	different := 0
	for i, val := range wanted {
		if val != obtained[i] {
			if different == 0 {
				fmt.Printf("%s differ:\n", label)
			}
			fmt.Printf("    %d: Wanted: %d, Obtained: %d\n", i, val, obtained[i])
			different++
			if different > 10 {
				fmt.Println("    ...")
				break
			}
		}
	}
}

func printValidator(wanted, obtained *ethpb.Validator) {
	fmt.Printf("Validator Wanted:\n    Pubkey: %#x\n    Withdrawable Epoch: %d\n    Effective Balance: %d\n", wanted.PublicKey, wanted.WithdrawableEpoch, wanted.EffectiveBalance)
	fmt.Printf("Validator Obtained:\n    Pubkey: %#x\n    Withdrawable Epoch: %d\n    Effective Balance: %d\n", obtained.PublicKey, obtained.WithdrawableEpoch, obtained.EffectiveBalance)
}

func validatorsDiffers(wanted, obtained *ethpb.Validator) bool {
	if !slices.Equal(wanted.PublicKey, obtained.PublicKey) {
		return true
	}
	if wanted.WithdrawableEpoch != obtained.WithdrawableEpoch {
		return true
	}
	if wanted.EffectiveBalance != obtained.EffectiveBalance {
		return true
	}
	return false
}

func compareValidatorsSlices(wanted, obtained []*ethpb.Validator) {
	different := 0
	for i, validator := range wanted {
		if validatorsDiffers(validator, obtained[i]) {
			if different == 0 {
				fmt.Println("Validators differ:")
			}
			fmt.Printf("    %d: ", i)
			printValidator(validator, obtained[i])
			different++
			if different > 10 {
				fmt.Println("    ...")
				break
			}
		}
	}
}

func compareEth1Slices(wanted, obtained []*ethpb.Eth1Data) {
	different := 0
	for i, eth1Data := range wanted {
		if eth1DataDiffers(eth1Data, obtained[i]) {
			if different == 0 {
				fmt.Println("Eth1 Data differ:")
			}
			fmt.Printf("    %d: ", i)
			printEth1Data(eth1Data, obtained[i])
			different++
			if different > 10 {
				fmt.Println("    ...")
				break
			}
		}
	}
}

func printEth1Data(wanted, obtained *ethpb.Eth1Data) {
	fmt.Printf("Eth1 Data Wanted:\n    Deposit Root: %#x\n    Deposit Count: %d\n    Block Hash: %#x\n", wanted.DepositRoot, wanted.DepositCount, wanted.BlockHash)
	fmt.Printf("Eth1 Data Obtained:\n    Deposit Root: %#x\n    Deposit Count: %d\n    Block Hash: %#x\n", obtained.DepositRoot, obtained.DepositCount, obtained.BlockHash)
}

func eth1DataDiffers(wanted, obtained *ethpb.Eth1Data) bool {
	if !slices.Equal(wanted.DepositRoot, obtained.DepositRoot) {
		return true
	}
	if wanted.DepositCount != obtained.DepositCount {
		return true
	}
	if !slices.Equal(wanted.BlockHash, obtained.BlockHash) {
		return true
	}
	return false
}

func compareRootsSlices(label string, wanted, obtained [][]byte) {
	different := 0
	for i, root := range wanted {
		if !slices.Equal(root, obtained[i]) {
			if different == 0 {
				fmt.Printf("%s differ:\n", label)
			}
			fmt.Printf("    %d: Wanted: %#x, Obtained: %#x\n", i, root, obtained[i])
			different++
			if different > 10 {
				fmt.Println("    ...")
				break
			}
		}
	}
}

func compareForks(wanted, obtained *ethpb.Fork) {
	if !slices.Equal(wanted.PreviousVersion, obtained.PreviousVersion) {
		printForks(wanted, obtained)
		return
	}
	if !slices.Equal(wanted.CurrentVersion, obtained.CurrentVersion) {
		printForks(wanted, obtained)
		return
	}
	if wanted.Epoch != obtained.Epoch {
		printForks(wanted, obtained)
		return
	}
}

func printBlockHeader(wanted, obtained *ethpb.BeaconBlockHeader) {
	fmt.Printf("Block Header Wanted:\n    Slot: %d\n    Proposer Index: %d\n    Parent Root: %#x\n    State Root: %#x\n    Body Root: %#x\n", wanted.Slot, wanted.ProposerIndex, wanted.ParentRoot, wanted.StateRoot, wanted.BodyRoot)
	fmt.Printf("Block Header Obtained:\n    Slot: %d\n    Proposer Index: %d\n    Parent Root: %#x\n    State Root: %#x\n    Body Root: %#x\n", obtained.Slot, obtained.ProposerIndex, obtained.ParentRoot, obtained.StateRoot, obtained.BodyRoot)
}

func compareBlockHeaders(wanted, obtained *ethpb.BeaconBlockHeader) {
	if wanted.Slot != obtained.Slot {
		printBlockHeader(wanted, obtained)
		return
	}
	if wanted.ProposerIndex != obtained.ProposerIndex {
		printBlockHeader(wanted, obtained)
		return
	}
	if !slices.Equal(wanted.ParentRoot, obtained.ParentRoot) {
		printBlockHeader(wanted, obtained)
		return
	}
	if !slices.Equal(wanted.StateRoot, obtained.StateRoot) {
		printBlockHeader(wanted, obtained)
		return
	}
	if !slices.Equal(wanted.BodyRoot, obtained.BodyRoot) {
		printBlockHeader(wanted, obtained)
		return
	}
}

func printForks(wanted, obtained *ethpb.Fork) {
	fmt.Printf("Fork Wanted:\n    Previous Version: %#x\n    Current Version: %#x\n    Epoch: %d\n", wanted.PreviousVersion, wanted.CurrentVersion, wanted.Epoch)
	fmt.Printf("Fork Obtained:\n    Previous Version: %#x\n    Current Version: %#x\n    Epoch: %d\n", obtained.PreviousVersion, obtained.CurrentVersion, obtained.Epoch)
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
