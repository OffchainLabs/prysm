package db

import (
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/slasher"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/slasher/types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/urfave/cli/v2"
)

const DefaultChunkKind = types.MinSpan

var (
	f = struct {
		Path                          string
		ValidatorIndex                uint64
		Epoch                         uint64
		ChunkKind                     string
		ChunkSize                     uint64
		ValidatorChunkSize            uint64
		HistoryLength                 uint64
		IsDisplayAllValidatorsInChunk bool
		IsDisplayAllEpochsInChunk     bool
	}{}

	slasherDefaultParams = slasher.DefaultParams()
)

var spanCmd = &cli.Command{
	Name:  "slasher-span-display",
	Usage: "visualise values in db span bucket",
	Action: func(c *cli.Context) error {
		if err := spanAction(c); err != nil {
			return errors.Wrapf(err, "visualise values in db span bucket failed")
		}
		return nil
	},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "db-path-directory",
			Usage:       "path to directory containing slasher.db",
			Destination: &f.Path,
			Required:    true,
		},
		&cli.Uint64Flag{
			Name:        "validator-index",
			Usage:       "filter by validator index",
			Destination: &f.ValidatorIndex,
			Required:    true,
		},
		&cli.Uint64Flag{
			Name:        "epoch",
			Usage:       "filter by epoch",
			Destination: &f.Epoch,
			Required:    true,
		},
		&cli.StringFlag{
			Name:        "chunk-kind",
			Usage:       "chunk kind to query (maxspan|minspan)",
			Destination: &f.ChunkKind,
			Value:       DefaultChunkKind.String(),
			DefaultText: DefaultChunkKind.String(),
		},
		&cli.Uint64Flag{
			Name:        "chunk-size",
			Usage:       "chunk size to query",
			Destination: &f.ChunkSize,
			DefaultText: fmt.Sprintf("%d", slasherDefaultParams.ChunkSize()),
		},
		&cli.Uint64Flag{
			Name:        "validator-chunk-size",
			Usage:       "validator chunk size to query",
			Destination: &f.ValidatorChunkSize,
			DefaultText: fmt.Sprintf("%d", slasherDefaultParams.ValidatorChunkSize()),
		},
		&cli.Uint64Flag{
			Name:        "history-length",
			Usage:       "history length to query",
			Destination: &f.HistoryLength,
			DefaultText: fmt.Sprintf("%d", slasherDefaultParams.HistoryLength()),
		},
		&cli.BoolFlag{
			Name:        "display-all-validators-in-chunk",
			Usage:       "display all validators in chunk",
			Destination: &f.IsDisplayAllValidatorsInChunk,
		},
		&cli.BoolFlag{
			Name:        "display-all-epochs-in-chunk",
			Usage:       "display all epochs in chunk",
			Destination: &f.IsDisplayAllEpochsInChunk,
		},
	},
}

func spanAction(cliCtx *cli.Context) error {
	var (
		chunk                      slasher.Chunker
		validatorChunkIdx          uint64
		lastEpochForValidatorIndex primitives.Epoch

		err error
	)

	// context
	ctx := cliCtx.Context

	// variables
	chunkKind := getChunkKind()
	params := getSlasherParams()
	i := primitives.ValidatorIndex(f.ValidatorIndex)
	epoch := primitives.Epoch(f.Epoch)

	// display configuration
	fmt.Printf("############################# SLASHER PARAMS ###############################\n")
	fmt.Printf("# Chunk Size: %d\n", params.ChunkSize())
	fmt.Printf("# Validator Chunk Size: %d\n", params.ValidatorChunkSize())
	fmt.Printf("# History Length: %d\n", params.HistoryLength())
	fmt.Printf("# DB: %s\n", f.Path)
	fmt.Printf("# Chunk Kind: %s\n", chunkKind)
	fmt.Printf("# Validator: %d\n", i)
	fmt.Printf("# Epoch: %d\n", epoch)
	fmt.Printf("############################################################################\n")

	// fetch chunk in database
	if lastEpochForValidatorIndex, _, validatorChunkIdx, chunk, err = slasher.GetChunkFromDatabase(
		ctx,
		f.Path,
		slasher.GetChunkFromDatabaseFilters{
			ChunkKind:                     chunkKind,
			ValidatorIndex:                i,
			SourceEpoch:                   epoch,
			IsDisplayAllValidatorsInChunk: f.IsDisplayAllValidatorsInChunk,
			IsDisplayAllEpochsInChunk:     f.IsDisplayAllEpochsInChunk,
		},
		params,
	); err != nil {
		return errors.Wrapf(err, "could not get chunk from database")
	}

	// fetch information related to chunk
	fmt.Printf("\n################################ CHUNK #####################################\n")
	firstValidator := params.ValidatorIndexesInChunk(validatorChunkIdx)[0]
	firstEpoch := epoch - (epoch.Mod(params.ChunkSize()))
	fmt.Printf("# First validator in chunk: %d\n", firstValidator)
	fmt.Printf("# First epoch in chunk: %d\n\n", firstEpoch)
	fmt.Printf("# Last epoch found in database for validator(%d): %d\n", i, lastEpochForValidatorIndex)
	fmt.Printf("############################################################################\n\n")

	// init table
	tw := table.NewWriter()

	minLowerBound := lastEpochForValidatorIndex.Sub(uint64(params.HistoryLength()))
	if f.IsDisplayAllValidatorsInChunk {
		if f.IsDisplayAllEpochsInChunk {
			// display all validators and epochs in chunk

			// headers
			addEpochsHeader(tw, params.ChunkSize(), firstEpoch)

			// rows
			b := chunk.Chunk()
			c := uint64(0)
			for z := uint64(0); z < uint64(len(b)); z += params.ChunkSize() {
				end := z + params.ChunkSize()
				if end > uint64(len(b)) {
					end = uint64(len(b))
				}
				subChunk := b[z:end]

				row := make(table.Row, params.ChunkSize()+1)
				title := firstValidator + primitives.ValidatorIndex(c)
				row[0] = title
				for y, span := range subChunk {
					row[y+1] = getSpanOrNonApplicable(firstEpoch, y, minLowerBound, lastEpochForValidatorIndex, span)
				}
				tw.AppendRow(row)

				c++
			}
		} else {
			// display all validators but only the requested epoch in chunk
			indexEpochInChunk := epoch - firstEpoch

			// headers
			addEpochsHeader(tw, 1, firstEpoch)

			// rows
			b := chunk.Chunk()
			c := uint64(0)
			for z := uint64(0); z < uint64(len(b)); z += params.ChunkSize() {
				end := z + params.ChunkSize()
				if end > uint64(len(b)) {
					end = uint64(len(b))
				}
				subChunk := b[z:end]

				row := make(table.Row, 2)
				title := firstValidator + primitives.ValidatorIndex(c)
				row[0] = title
				row[1] = subChunk[indexEpochInChunk]
				tw.AppendRow(row)

				c++
			}
		}
	} else {
		if f.IsDisplayAllEpochsInChunk {
			// display only the requested validator with all epochs in chunk

			// headers
			addEpochsHeader(tw, params.ChunkSize(), firstEpoch)

			// rows
			b := chunk.Chunk()
			validatorFirstEpochIdx := uint64(i.Mod(params.ValidatorChunkSize())) * params.ChunkSize()
			subChunk := b[validatorFirstEpochIdx : validatorFirstEpochIdx+params.ChunkSize()]
			row := make(table.Row, params.ChunkSize()+1)
			title := i
			row[0] = title
			for y, span := range subChunk {
				row[y+1] = getSpanOrNonApplicable(firstEpoch, y, minLowerBound, lastEpochForValidatorIndex, span)
			}
			tw.AppendRow(row)
		} else {
			// display only the requested validator and epoch in chunk

			// headers
			addEpochsHeader(tw, 1, epoch)

			// rows
			b := chunk.Chunk()
			validatorFirstEpochIdx := uint64(i.Mod(params.ValidatorChunkSize())) * params.ChunkSize()
			subChunk := b[validatorFirstEpochIdx : validatorFirstEpochIdx+params.ChunkSize()]
			row := make(table.Row, 2)
			title := i
			row[0] = title
			indexEpochInChunk := epoch - firstEpoch
			row[1] = subChunk[indexEpochInChunk]
			tw.AppendRow(row)
		}
	}

	// display table
	displayTable(tw)

	return nil
}

// getSpanOrNonApplicable checks if there's some epoch that are not correct in chunk due to the round robin
// nature of 2D chunking when an epoch gets overwritten by an epoch eg. (params.historyLength + next_epoch) > params.historyLength
// if we are out of the range, we display a n/a value otherwise the span value
func getSpanOrNonApplicable(firstEpoch primitives.Epoch, y int, minLowerBound primitives.Epoch, lastEpochForValidatorIndex primitives.Epoch, span uint16) string {
	if firstEpoch.Add(uint64(y)) < minLowerBound || firstEpoch.Add(uint64(y)) > lastEpochForValidatorIndex {
		return "-"
	}
	return fmt.Sprintf("%d", span)
}

func displayTable(tw table.Writer) {
	tw.AppendSeparator()
	fmt.Println(tw.Render())
}

func addEpochsHeader(tw table.Writer, nbEpoch uint64, firstEpoch primitives.Epoch) {
	header := table.Row{"Validator / Epoch"}
	for y := 0; uint64(y) < nbEpoch; y++ {
		header = append(header, firstEpoch+primitives.Epoch(y))
	}
	tw.AppendHeader(header)
}

func getChunkKind() types.ChunkKind {
	chunkKind := types.MinSpan
	if f.ChunkKind == "maxspan" {
		chunkKind = types.MaxSpan
	}
	return chunkKind
}

func getSlasherParams() *slasher.Parameters {
	var (
		chunkSize, validatorChunkSize uint64
		historyLength                 primitives.Epoch
	)
	if f.ChunkSize != 0 && f.ChunkSize != slasherDefaultParams.ChunkSize() {
		chunkSize = f.ChunkSize
	} else {
		chunkSize = slasherDefaultParams.ChunkSize()
	}
	if f.ValidatorChunkSize != 0 && f.ValidatorChunkSize != slasherDefaultParams.ValidatorChunkSize() {
		validatorChunkSize = f.ValidatorChunkSize
	} else {
		validatorChunkSize = slasherDefaultParams.ValidatorChunkSize()
	}
	if f.HistoryLength != 0 && f.HistoryLength != uint64(slasherDefaultParams.HistoryLength()) {
		historyLength = primitives.Epoch(f.HistoryLength)
	} else {
		historyLength = slasherDefaultParams.HistoryLength()
	}
	return slasher.NewParams(chunkSize, validatorChunkSize, historyLength)
}
