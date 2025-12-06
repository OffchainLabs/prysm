package evaluators

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	e2e "github.com/OffchainLabs/prysm/v7/testing/endtoend/params"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/policies"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// DependentRootEvaluators returns evaluators for testing dependent root handling.
// These evaluators verify that the dependent root bug does NOT occur:
// - When a block at the first slot of an epoch becomes finalized and its parent pruned
// - The dependent root query should still return correct data (from the previous epoch)
//
// The evaluator checks for "E2E_DEPENDENT_ROOT_BUG" error log which indicates
// a dependent root was returned from the wrong epoch.
//
// WITH the fix: No bug log appears → test PASSES
// WITHOUT the fix: Bug log appears (wrong epoch data) → test FAILS
func DependentRootEvaluators(afterEpoch primitives.Epoch) []types.Evaluator {
	return []types.Evaluator{
		{
			Name:       "no_dependent_root_bug_epoch_%d",
			Policy:     policies.AfterNthEpoch(afterEpoch + 2), // Allow time for finalization
			Evaluation: checkNoDependentRootBug,
		},
	}
}

// checkNoDependentRootBug scans beacon node logs to verify that the dependent root
// bug did NOT occur. The bug would manifest as returning a dependent root from
// the wrong epoch (current epoch instead of previous epoch).
//
// The beacon node logs "E2E_DEPENDENT_ROOT_BUG" when it detects this condition.
//
// WITH the fix: No bug log appears → test PASSES
// WITHOUT the fix: Bug log appears → test FAILS
func checkNoDependentRootBug(_ *types.EvaluationContext, _ ...*grpc.ClientConn) error {
	// This log message indicates the bug was triggered
	bugLogMessage := "E2E_DEPENDENT_ROOT_BUG"

	for i := 0; i < e2e.TestParams.BeaconNodeCount; i++ {
		logFile := path.Join(e2e.TestParams.LogPath, fmt.Sprintf(e2e.BeaconNodeLogFileName, i))
		found, err := searchLogForMessages(logFile, []string{bugLogMessage})
		if err != nil {
			return errors.Wrapf(err, "failed to search beacon node %d log file", i)
		}
		if found {
			// Bug was detected - test FAILS
			return errors.New("E2E_DEPENDENT_ROOT_BUG detected: dependent root returned from wrong epoch - the fix is not working or not present")
		}
	}

	// No bug detected - test PASSES
	log.Info("No dependent root bug detected - fix is working correctly")
	return nil
}

// searchLogForMessages searches a log file for any of the given messages.
func searchLogForMessages(logPath string, messages []string) (bool, error) {
	file, err := os.Open(logPath) // #nosec G304 -- test code only
	if err != nil {
		return false, errors.Wrap(err, "failed to open log file")
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.WithError(err).Error("Failed to close log file")
		}
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		for _, msg := range messages {
			if strings.Contains(line, msg) {
				return true, nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return false, errors.Wrap(err, "error scanning log file")
	}

	return false, nil
}
