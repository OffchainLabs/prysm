// Package coverage implements the standalone node-owned execution payload
// envelope coverage runtime (Gloas+). Its coordinator is the only writer of
// the durable EnvelopeCoverage record and of the canonical-revealed envelope
// slot index; producers (forward sync, the live envelope path, head updates)
// only send coalescing notifications, and the envelopes-by-range RPC handler
// consumes generation-coherent read snapshots.
package coverage

import "github.com/sirupsen/logrus"

var log = logrus.WithField("prefix", "coverage")
