package remote_web3signer

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
)

// urlPollTimeout bounds a single public-keys fetch so a hung request cannot stall
// the poll loop indefinitely.
const urlPollTimeout = 30 * time.Second

// pollRemoteKeysFromURL re-fetches the public keys from url every interval and
// applies any change if needed.
func (km *Keymanager) pollRemoteKeysFromURL(ctx context.Context, url string, interval time.Duration) {
	if url == "" || interval <= 0 {
		return
	}

	log.
		WithField("url", api.RedactEndpoint(url)).
		WithField("interval", interval).
		Info("Starting Web3Signer public key poller")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pollCtx, cancel := context.WithTimeout(ctx, urlPollTimeout)
			raw, err := km.client.GetPublicKeys(pollCtx, url)
			cancel()
			if err != nil {
				erroredResponsesTotal.Inc()
				log.
					WithError(err).
					WithField("url", api.RedactEndpoint(url)).
					Warn("Could not refresh Web3Signer public keys from URL; keeping current keys")
				continue
			}

			keys, err := decodePublicKeys(raw)
			if err != nil {
				log.WithError(err).Warn("Web3Signer URL returned an undecodable public key; keeping current keys")
				continue
			}
			// Treat an empty response as transient rather than clearing all keys.
			if len(keys) == 0 {
				log.Warn("Web3Signer URL returned no public keys; keeping current keys")
				continue
			}

			km.replaceKeys(sourceURL, keys)
		}
	}
}
