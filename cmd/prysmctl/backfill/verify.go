package backfill

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var verifyFlags = struct {
	BeaconNodeHost string
	Verbose        bool
}{}

var verifyCmd = &cli.Command{
	Name:  "verify",
	Usage: "Verify that backfill successfully retrieved blobs and data columns",
	Action: func(cliCtx *cli.Context) error {
		if err := cliActionVerify(cliCtx); err != nil {
			log.WithError(err).Fatal("Could not verify backfill")
		}
		return nil
	},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "beacon-node-host",
			Usage:       "Beacon node API endpoint (e.g., http://localhost:3500)",
			Destination: &verifyFlags.BeaconNodeHost,
			Value:       "http://localhost:3500",
		},
		&cli.BoolFlag{
			Name:        "verbose",
			Usage:       "Print detailed output for each slot checked",
			Destination: &verifyFlags.Verbose,
			Value:       false,
		},
	},
}

type verificationStats struct {
	slotsChecked    int
	blocksFound     int
	blocksWithBlobs int
	blobsExpected   int
	blobsFound      int
	blobsMissing    int
	columnsExpected int
	columnsFound    int
	columnsMissing  int
	errors          int
}

func detectAndLoadNetworkConfig(ctx context.Context, baseURL string) error {
	url := fmt.Sprintf("%s/eth/v1/beacon/genesis", baseURL)

	var resp structs.GetGenesisResponse
	if err := doJSONGetRequest(ctx, url, &resp); err != nil {
		return err
	}

	if resp.Data == nil {
		return errors.New("no genesis data returned from beacon node")
	}

	genesisValidatorsRoot := resp.Data.GenesisValidatorsRoot

	log.WithField("genesisValidatorsRoot", genesisValidatorsRoot).Info("Detected network")

	// Hoodi testnet genesis validators root
	hoodiGenesisRoot := "0x212f13fc4df078b6cb7db228f1c8307566dcecf900867401a92023d7ba99cb5f"

	if genesisValidatorsRoot == hoodiGenesisRoot {
		log.Info("Detected Hoodi testnet, loading Hoodi configuration")
		params.OverrideBeaconConfig(params.HoodiConfig())
		params.UseHoodiNetworkConfig()
		return nil
	}

	// Default to mainnet
	log.Info("Using mainnet configuration")
	return nil
}

func cliActionVerify(cliCtx *cli.Context) error {
	ctx := context.Background()
	baseURL := verifyFlags.BeaconNodeHost

	log.Info("Starting backfill verification")
	log.WithField("endpoint", baseURL).Info("Connecting to beacon node")

	// Detect network by querying genesis
	if err := detectAndLoadNetworkConfig(ctx, baseURL); err != nil {
		return errors.Wrap(err, "failed to detect network configuration")
	}

	// Get current slot
	currentSlot, err := getCurrentSlot(ctx, baseURL)
	if err != nil {
		return errors.Wrap(err, "failed to get current slot")
	}
	log.WithField("currentSlot", currentSlot).Info("Retrieved current slot")

	// Calculate retention window
	denebForkEpoch := params.BeaconConfig().DenebForkEpoch
	fuluForkEpoch := params.BeaconConfig().FuluForkEpoch

	minBlobSlot := calculateBlobRetentionStart(currentSlot, denebForkEpoch)

	log.WithFields(log.Fields{
		"denebForkEpoch": denebForkEpoch,
		"fuluForkEpoch":  fuluForkEpoch,
		"startSlot":      minBlobSlot,
		"endSlot":        currentSlot,
		"totalSlots":     currentSlot - minBlobSlot + 1,
	}).Info("Calculated verification range")

	// Verify slots
	stats := &verificationStats{}

	log.Info("Beginning slot verification...")
	if verifyFlags.Verbose {
		fmt.Println()
	}

	totalSlots := int(currentSlot - minBlobSlot + 1)

	for slot := minBlobSlot; slot <= currentSlot; slot++ {
		if err := verifySlot(ctx, baseURL, slot, fuluForkEpoch, stats, verifyFlags.Verbose); err != nil {
			stats.errors++
			if stats.errors > 10 {
				return errors.Wrap(err, "too many errors during verification")
			}
		}
		stats.slotsChecked++

		// Print progress every 1000 slots
		if !verifyFlags.Verbose && stats.slotsChecked%1000 == 0 {
			percentComplete := float64(stats.slotsChecked) / float64(totalSlots) * 100
			log.WithFields(log.Fields{
				"slotsChecked":    stats.slotsChecked,
				"totalSlots":      totalSlots,
				"progress":        fmt.Sprintf("%.1f%%", percentComplete),
				"blocksFound":     stats.blocksFound,
				"blobsFound":      stats.blobsFound,
				"blobsExpected":   stats.blobsExpected,
				"blobsMissing":    stats.blobsMissing,
				"columnsFound":    stats.columnsFound,
				"columnsExpected": stats.columnsExpected,
				"columnsMissing":  stats.columnsMissing,
			}).Info("Progress update")
		}
	}

	// Print summary
	fmt.Println()
	log.Info("Verification complete")
	printSummary(stats, minBlobSlot, currentSlot)

	// Check for failures
	if stats.blobsMissing > 0 || stats.columnsMissing > 0 {
		return fmt.Errorf("verification failed: %d blobs missing, %d data columns missing",
			stats.blobsMissing, stats.columnsMissing)
	}

	return nil
}

func getCurrentSlot(ctx context.Context, baseURL string) (primitives.Slot, error) {
	// HACK: I only want to test backfill right now. The problem is that regular sync will retain 8 data columns while backfill wants to retain the cdc amount which is probably 4.
	if true {
		return 1830335, nil
	}
	url := fmt.Sprintf("%s/eth/v1/beacon/headers/head", baseURL)

	var resp structs.GetBlockHeaderResponse
	if err := doJSONGetRequest(ctx, url, &resp); err != nil {
		return 0, err
	}

	if resp.Data == nil || resp.Data.Header == nil || resp.Data.Header.Message == nil {
		return 0, errors.New("invalid response from beacon node")
	}

	slot, err := strconv.ParseUint(resp.Data.Header.Message.Slot, 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse slot")
	}

	return primitives.Slot(slot), nil
}

func calculateBlobRetentionStart(currentSlot primitives.Slot, denebForkEpoch primitives.Epoch) primitives.Slot {
	// MIN_EPOCHS_FOR_BLOB_SIDECARS_REQUEST is typically 4096
	minEpochsForBlobSidecars := params.BeaconConfig().MinEpochsForBlobsSidecarsRequest
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch

	currentEpoch := primitives.Epoch(currentSlot / primitives.Slot(slotsPerEpoch))

	// Blob retention starts at max(DenebForkEpoch, currentEpoch - MIN_EPOCHS_FOR_BLOB_SIDECARS_REQUEST)
	var startEpoch primitives.Epoch
	if currentEpoch > primitives.Epoch(minEpochsForBlobSidecars) {
		//startEpoch = currentEpoch - primitives.Epoch(minEpochsForBlobSidecars)
		startEpoch = params.BeaconConfig().FuluForkEpoch.Sub(100)
	} else {
		startEpoch = 0
	}

	if startEpoch < denebForkEpoch {
		startEpoch = denebForkEpoch
	}

	return primitives.Slot(startEpoch) * primitives.Slot(slotsPerEpoch)
}

func verifySlot(ctx context.Context, baseURL string, slot primitives.Slot, fuluForkEpoch primitives.Epoch, stats *verificationStats, verbose bool) error {
	// Check if block exists
	blockURL := fmt.Sprintf("%s/eth/v2/beacon/blocks/%d", baseURL, slot)

	var blockResp structs.GetBlockV2Response
	err := doJSONGetRequest(ctx, blockURL, &blockResp)
	if err != nil {
		// 404 is expected for empty slots
		if isNotFoundError(err) {
			if verbose {
				fmt.Printf("Slot %d: ⊘ Empty slot (no block)\n", slot)
			}
			return nil
		}
		// Always report errors
		fmt.Printf("Slot %d: ✗ Error fetching block: %v\n", slot, err)
		return err
	}

	stats.blocksFound++
	sbb := &structs.SignedBeaconBlockFulu{Message: &structs.BeaconBlockElectra{}}
	if err := json.Unmarshal(blockResp.Data.Message, sbb.Message); err != nil {
		return err
	}

	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	slotEpoch := primitives.Epoch(slot / primitives.Slot(slotsPerEpoch))

	var blobError bool
	var numBlobs int
	if slotEpoch < params.BeaconConfig().FuluForkEpoch && len(sbb.Message.Body.BlobKzgCommitments) > 0 {
		// Check for blobs
		blobURL := fmt.Sprintf("%s/eth/v1/beacon/blob_sidecars/%d", baseURL, slot)

		var blobResp structs.SidecarsResponse
		err = doJSONGetRequest(ctx, blobURL, &blobResp)

		stats.blobsExpected += len(sbb.Message.Body.BlobKzgCommitments)
		if err == nil && blobResp.Data != nil {
			numBlobs = len(blobResp.Data)
			stats.blobsFound += numBlobs
		} else if err != nil && !isNotFoundError(err) {
			blobError = true
		}
		stats.blobsMissing += max(len(sbb.Message.Body.BlobKzgCommitments)-numBlobs, 0)
	}
	// Check for data columns if post-Fulu
	if slotEpoch >= fuluForkEpoch && len(sbb.Message.Body.BlobKzgCommitments) > 0 {
		columnURL := fmt.Sprintf("%s/eth/v1/debug/beacon/data_column_sidecars/%d", baseURL, slot)

		var columnResp structs.GetDebugDataColumnSidecarsResponse
		err = doJSONGetRequest(ctx, columnURL, &columnResp)

		numColumns := 0
		columnError := false
		expectedColumns := 4 // TODO: Get from node identity cdc
		if err == nil && columnResp.Data != nil {
			// Compute the number of columns
			numColumns += len(columnResp.Data)
			if numColumns > expectedColumns {
				log.WithField("slot", slot).WithField("columns", numColumns).WithField("url", columnURL).WithField("expected", expectedColumns).Error("Too many columns!")
			} else if missing := expectedColumns - numColumns; missing > 0 && (numColumns > 0 || verbose) {
				log.WithField("slot", slot).WithField("columns", numColumns).WithField("url", columnURL).WithField("missing", missing).Warnf("Too few columns, expected %d", expectedColumns)
			}
			stats.columnsMissing += max(expectedColumns-numColumns, 0)

			stats.columnsExpected += expectedColumns
			stats.columnsFound += numColumns
		} else if err != nil && !isNotFoundError(err) {
			columnError = true
		}

		// Report errors or verbose output
		if blobError || columnError {
			fmt.Printf("Slot %d: ✗ Block found, but errors: blob_error=%v, column_error=%v\n",
				slot, blobError, columnError)
		} else if verbose {
			if numBlobs > 0 {
				stats.blocksWithBlobs++
				fmt.Printf("Slot %d: ✓ Block found, %d blobs verified, %d data columns verified\n",
					slot, numBlobs, numColumns)
			} else {
				fmt.Printf("Slot %d: ✓ Block found, 0 blobs (expected), %d data columns\n",
					slot, numColumns)
			}
		} else if numBlobs > 0 {
			stats.blocksWithBlobs++
		}
	} else {
		// Pre-Fulu, only check blobs
		if blobError {
			fmt.Printf("Slot %d: ✗ Block found, but blob error occurred\n", slot)
		} else if verbose {
			if numBlobs > 0 {
				stats.blocksWithBlobs++
				fmt.Printf("Slot %d: ✓ Block found, %d blobs verified\n", slot, numBlobs)
			} else {
				fmt.Printf("Slot %d: ✓ Block found, 0 blobs (expected)\n", slot)
			}
		} else if numBlobs > 0 {
			stats.blocksWithBlobs++
		}
	}

	return nil
}

func doJSONGetRequest(ctx context.Context, url string, resp any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return errors.Wrap(err, "failed to create request")
	}

	req.Header.Set("Accept", "application/json")

	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "request failed")
	}
	defer closeBody(httpResp.Body)

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("request failed with status %d: %s", httpResp.StatusCode, string(body))
	}

	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return errors.Wrap(err, "failed to decode response")
	}

	return nil
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return contains(err.Error(), "404") || contains(err.Error(), "not found")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			len(s) > len(substr)+1 && containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func closeBody(body io.Closer) {
	if err := body.Close(); err != nil {
		log.WithError(err).Error("Could not close response body")
	}
}

func printSummary(stats *verificationStats, startSlot, endSlot primitives.Slot) {
	fmt.Println()
	fmt.Println("====================================================================")
	fmt.Println("                    VERIFICATION SUMMARY")
	fmt.Println("====================================================================")
	fmt.Println()
	fmt.Printf("Slot Range:            %d to %d (%d slots)\n", startSlot, endSlot, endSlot-startSlot+1)
	fmt.Printf("Slots Checked:         %d\n", stats.slotsChecked)
	fmt.Printf("Blocks Found:          %d\n", stats.blocksFound)
	fmt.Printf("Blocks with Blobs:     %d\n", stats.blocksWithBlobs)
	fmt.Println()
	fmt.Println("Blobs:")
	fmt.Printf("  Expected:            %d\n", stats.blobsExpected)
	fmt.Printf("  Found:               %d\n", stats.blobsFound)
	fmt.Printf("  Missing:             %d\n", stats.blobsMissing)
	fmt.Println()

	if stats.columnsExpected > 0 {
		fmt.Println("Data Columns:")
		fmt.Printf("  Expected:            %d\n", stats.columnsExpected)
		fmt.Printf("  Found:               %d\n", stats.columnsFound)
		fmt.Printf("  Missing:             %d\n", stats.columnsMissing)
		fmt.Println()
	}

	if stats.errors > 0 {
		fmt.Printf("Errors Encountered:    %d\n", stats.errors)
		fmt.Println()
	}

	if stats.blobsMissing == 0 && stats.columnsMissing == 0 {
		fmt.Println("✓ RESULT: All backfilled data verified successfully!")
	} else {
		fmt.Println("✗ RESULT: Verification failed - missing data detected")
	}
	fmt.Println("====================================================================")
}
