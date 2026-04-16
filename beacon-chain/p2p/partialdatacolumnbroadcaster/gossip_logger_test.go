package partialdatacolumnbroadcaster

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// TestGossipUsesInjectedLogger verifies that the gossip() method logs through
// the broadcaster's injected logger (p.logger), not the package-level `log`
// variable. It triggers an error inside gossip's publishPartialCol to produce
// a log line, then asserts that line appears in the injected logger's buffer.
func TestGossipUsesInjectedLogger(t *testing.T) {
	var buf bytes.Buffer
	injectedLogger := &logrus.Logger{
		Out:       &buf,
		Formatter: &logrus.TextFormatter{DisableTimestamp: true},
		Level:     logrus.DebugLevel,
	}

	topic := "/eth2/00000000/data_column_sidecar_7/ssz_snappy"
	groupID := []byte{0, 1, 2, 3}

	b := NewBroadcaster(injectedLogger)
	// Wire up a publishPartialCol that always errors so gossip() logs the failure.
	b.publishPartialCol = func(t string, g []byte, c *blocks.PartialDataColumn) error {
		return errors.New("publish failed")
	}
	b.peerFeedback = func(_ string, _ peer.ID, _ pubsub.PeerFeedbackKind) error { return nil }

	// Create a partial column in the store so gossip() has something to publish.
	col := createPartialColumn(t, 2, map[uint64][]byte{0: {0xAA}})
	col.Published = true
	verifier := &verification.PartialColumnVerifier{Column: col}
	b.partialMsgStore[topic] = map[string]*verification.PartialColumnVerifier{
		string(groupID): verifier,
	}

	// Run gossip directly (it's called from the event loop).
	b.gossip(topic, groupID)

	// Give a moment for any async log to flush.
	time.Sleep(10 * time.Millisecond)

	output := buf.String()
	if !strings.Contains(output, "publish") {
		t.Errorf("gossip() error log did not appear in the injected logger.\n"+
			"Injected logger output: %q\n"+
			"This means gossip() is using the package-level `log` instead of `p.logger`.", output)
	}
}

// TestGossipUsesInjectedLoggerSuccess verifies that on success, the gossip()
// method does not log an error to the injected logger.
func TestGossipUsesInjectedLoggerSuccess(t *testing.T) {
	var buf bytes.Buffer
	injectedLogger := &logrus.Logger{
		Out:       &buf,
		Formatter: &logrus.TextFormatter{DisableTimestamp: true},
		Level:     logrus.DebugLevel,
	}

	topic := "/eth2/00000000/data_column_sidecar_7/ssz_snappy"
	groupID := []byte{0, 1, 2, 3}

	b := NewBroadcaster(injectedLogger)
	b.publishPartialCol = func(t string, g []byte, c *blocks.PartialDataColumn) error {
		return nil // success
	}

	col := createPartialColumn(t, 2, map[uint64][]byte{0: {0xAA}})
	col.Published = true
	verifier := &verification.PartialColumnVerifier{Column: col}
	b.partialMsgStore[topic] = map[string]*verification.PartialColumnVerifier{
		string(groupID): verifier,
	}

	b.gossip(topic, groupID)

	output := buf.String()
	require.Equal(t, "", output, "no log output expected on successful gossip")
}
