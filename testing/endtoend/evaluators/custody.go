package evaluators

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"regexp"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	e2e "github.com/OffchainLabs/prysm/v7/testing/endtoend/params"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/policies"
	e2etypes "github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// CustodyInfoNonDecreasing verifies that earliestAvailableSlot never decreases
// across custody info updates. This is used to detect the bug where restarting
// with --semi-supernode before Fulu fork overwrites a higher slot with a lower one.
var CustodyInfoNonDecreasing = e2etypes.Evaluator{
	Name:       "custody_info_non_decreasing_%d",
	Policy:     policies.AllEpochs,
	Evaluation: custodyInfoNonDecreasing,
}

// custodyInfoNonDecreasing parses beacon node logs for custody info updates
// and verifies that earliestAvailableSlot never decreases.
func custodyInfoNonDecreasing(ec *e2etypes.EvaluationContext, _ ...*grpc.ClientConn) error {
	for i := 0; i < e2e.TestParams.BeaconNodeCount; i++ {
		logPath := path.Join(e2e.TestParams.LogPath, fmt.Sprintf(e2e.BeaconNodeLogFileName, i))
		custodyInfo, err := ParseCustodyInfoFromLog(logPath)
		if err != nil {
			// If we can't find custody info, that's OK - just means Fulu isn't enabled or no updates yet
			continue
		}

		// Check if we have a previous value
		if prev, exists := ec.CustodyInfo[i]; exists {
			if custodyInfo.EarliestAvailableSlot < prev.EarliestAvailableSlot {
				return fmt.Errorf(
					"node %d: earliestAvailableSlot decreased from %d to %d (bug detected!)",
					i, prev.EarliestAvailableSlot, custodyInfo.EarliestAvailableSlot,
				)
			}
		}

		// Update the stored value
		ec.CustodyInfo[i] = custodyInfo
	}

	return nil
}

// ParseCustodyInfoFromLog parses custody info from beacon node log file.
// It looks for log lines containing "Updated custody info in database" and extracts
// the earliestAvailableSlot and custodyGroupCount values.
// Returns the most recent custody info found in the log.
func ParseCustodyInfoFromLog(logPath string) (*e2etypes.CustodyInfoState, error) {
	file, err := os.Open(logPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open log file")
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.WithError(err).Error("Failed to close log file")
		}
	}()

	// Pattern to match: earliestAvailableSlot=1234 custodyGroupCount=64
	// Log format: time="..." level=info msg="Updated custody info in database" earliestAvailableSlot=123 custodyGroupCount=64 ...
	slotPattern := regexp.MustCompile(`earliestAvailableSlot=(\d+)`)
	countPattern := regexp.MustCompile(`custodyGroupCount=(\d+)`)
	updatePattern := regexp.MustCompile(`Updated custody info in database`)

	var latestInfo *e2etypes.CustodyInfoState

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Only process lines with custody info updates
		if !updatePattern.MatchString(line) {
			continue
		}

		slotMatch := slotPattern.FindStringSubmatch(line)
		countMatch := countPattern.FindStringSubmatch(line)

		if len(slotMatch) < 2 || len(countMatch) < 2 {
			continue
		}

		slot, err := strconv.ParseUint(slotMatch[1], 10, 64)
		if err != nil {
			continue
		}

		count, err := strconv.ParseUint(countMatch[1], 10, 64)
		if err != nil {
			continue
		}

		latestInfo = &e2etypes.CustodyInfoState{
			EarliestAvailableSlot: primitives.Slot(slot),
			CustodyGroupCount:     count,
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, errors.Wrap(err, "error reading log file")
	}

	if latestInfo == nil {
		return nil, errors.New("no custody info found in log")
	}

	return latestInfo, nil
}

// ParseAllCustodyInfoFromLog parses all custody info entries from beacon node log file.
// Returns a slice of all custody info states in chronological order.
func ParseAllCustodyInfoFromLog(logPath string) ([]*e2etypes.CustodyInfoState, error) {
	file, err := os.Open(logPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open log file")
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.WithError(err).Error("Failed to close log file")
		}
	}()

	slotPattern := regexp.MustCompile(`earliestAvailableSlot=(\d+)`)
	countPattern := regexp.MustCompile(`custodyGroupCount=(\d+)`)
	updatePattern := regexp.MustCompile(`Updated custody info in database`)

	var allInfo []*e2etypes.CustodyInfoState

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		if !updatePattern.MatchString(line) {
			continue
		}

		slotMatch := slotPattern.FindStringSubmatch(line)
		countMatch := countPattern.FindStringSubmatch(line)

		if len(slotMatch) < 2 || len(countMatch) < 2 {
			continue
		}

		slot, err := strconv.ParseUint(slotMatch[1], 10, 64)
		if err != nil {
			continue
		}

		count, err := strconv.ParseUint(countMatch[1], 10, 64)
		if err != nil {
			continue
		}

		allInfo = append(allInfo, &e2etypes.CustodyInfoState{
			EarliestAvailableSlot: primitives.Slot(slot),
			CustodyGroupCount:     count,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, errors.Wrap(err, "error reading log file")
	}

	return allInfo, nil
}

// VerifyCustodyInfoMonotonicity checks that earliestAvailableSlot is monotonically
// non-decreasing across all custody info updates for a given node.
// It checks both the current log file and any backup log file (from before restart).
func VerifyCustodyInfoMonotonicity(logPath string) error {
	var allInfo []*e2etypes.CustodyInfoState

	// First, try to parse from the pre-restart backup log if it exists
	// The backup log path follows the pattern: beacon-N-pre-restart.log
	backupLogPath := logPath[:len(logPath)-4] + "-pre-restart.log" // Replace .log with -pre-restart.log
	preRestartInfo, err := ParseAllCustodyInfoFromLog(backupLogPath)
	if err == nil && len(preRestartInfo) > 0 {
		log.WithField("count", len(preRestartInfo)).Info("Found pre-restart custody info entries")
		allInfo = append(allInfo, preRestartInfo...)
	}

	// Then parse from the current log file
	currentInfo, err := ParseAllCustodyInfoFromLog(logPath)
	if err != nil {
		if len(allInfo) == 0 {
			return err
		}
		// If we have pre-restart info but no current info, that's OK
	} else {
		allInfo = append(allInfo, currentInfo...)
	}

	if len(allInfo) < 2 {
		return nil // Not enough data points to check monotonicity
	}

	log.WithField("totalEntries", len(allInfo)).Info("Checking custody info monotonicity")

	for i := 1; i < len(allInfo); i++ {
		if allInfo[i].EarliestAvailableSlot < allInfo[i-1].EarliestAvailableSlot {
			return fmt.Errorf(
				"earliestAvailableSlot decreased at update %d: %d -> %d (pre-restart entries: %d)",
				i, allInfo[i-1].EarliestAvailableSlot, allInfo[i].EarliestAvailableSlot, len(preRestartInfo),
			)
		}
	}

	return nil
}
