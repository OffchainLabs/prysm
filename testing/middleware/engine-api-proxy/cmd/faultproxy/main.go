// Command faultproxy runs the engine-api proxy as a standalone service with an
// HTTP admin port to toggle an EL "SYNCING" fault on/off. It exists so a
// Kurtosis/Assertoor scenario can inject the same optimistic-sync fault the Go
// E2E injects via EngineProxy.AddRequestInterceptor.
//
// The flags mirror ethereum-package's json_rpc_snoop (-b host, -p port,
// positional destination URL) so it is a drop-in snooper image; the CL's
// --execution-endpoint then points at this proxy. With no --jwt-secret-file it
// forwards the caller's Authorization header (transparent passthrough), so no
// secret needs mounting. Toggle the fault with:
//
//	curl -fsS -X POST   http://<host>:<admin>/fault/syncing   # enable
//	curl -fsS -X DELETE http://<host>:<admin>/fault/syncing   # disable
package main

import (
	"context"
	"encoding/hex"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	proxy "github.com/OffchainLabs/prysm/v7/testing/middleware/engine-api-proxy"
	"github.com/sirupsen/logrus"
)

// All engine method versions we may need to fault. Registering every version
// means the caller does not have to know which fork is active.
// ponytail: static list, extend when a new newPayload/FCU version ships.
var faultMethods = []string{
	"engine_newPayloadV1", "engine_newPayloadV2", "engine_newPayloadV3",
	"engine_newPayloadV4", "engine_newPayloadV5",
	"engine_forkchoiceUpdatedV1", "engine_forkchoiceUpdatedV2",
	"engine_forkchoiceUpdatedV3",
}

func main() {
	bind := flag.String("b", "0.0.0.0", "host the CL-facing proxy listens on")
	port := flag.Int("p", 8551, "port the CL connects to (its execution endpoint)")
	jwtFile := flag.String("jwt-secret-file", "", "engine JWT secret file (hex); empty = forward caller's Authorization header")
	adminAddr := flag.String("admin-addr", ":8552", "listen address for the fault toggle admin server")
	flag.Parse()
	destination := flag.Arg(0) // EL engine RPC URL, e.g. http://el:8551 (snooper passes it positionally)

	log := logrus.New()

	var secret []byte
	if *jwtFile != "" {
		s, err := readJWTSecret(*jwtFile)
		if err != nil {
			log.WithError(err).Fatal("could not read jwt secret")
		}
		secret = s
	}

	p, err := proxy.New(
		proxy.WithHost(*bind),
		proxy.WithPort(*port),
		proxy.WithDestinationAddress(destination),
		proxy.WithJwtSecret(string(secret)),
		proxy.WithLogger(log),
	)
	if err != nil {
		log.WithError(err).Fatal("could not create proxy")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Match the Go E2E injection: newPayload SYNCING carries a 32-byte zero
	// latestValidHash (make([]byte,32)); FCU SYNCING carries null. Prysm's
	// payloadStatusJSON has no validationError field, so it is omitted.
	const zeroHash = "0x0000000000000000000000000000000000000000000000000000000000000000"
	syncing := func() any {
		return map[string]any{"status": "SYNCING", "latestValidHash": zeroHash}
	}
	syncingFCU := func() any {
		return map[string]any{
			"payloadStatus": map[string]any{"status": "SYNCING", "latestValidHash": nil},
			"payloadId":     nil,
		}
	}
	always := func() bool { return true }

	mux := http.NewServeMux()
	mux.HandleFunc("/fault/syncing", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			for _, m := range faultMethods {
				if strings.Contains(m, "forkchoiceUpdated") {
					p.AddRequestInterceptor(m, syncingFCU, always)
				} else {
					p.AddRequestInterceptor(m, syncing, always)
				}
			}
			log.Info("optimistic-sync fault ENABLED")
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			for _, m := range faultMethods {
				p.RemoveRequestInterceptor(m)
				p.ReleaseBackedUpRequests(m)
			}
			log.Info("optimistic-sync fault DISABLED")
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	admin := &http.Server{Addr: *adminAddr, Handler: mux, ReadHeaderTimeout: time.Second}
	go func() {
		log.Infof("fault admin listening on %s", *adminAddr)
		if err := admin.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Error("admin server error")
		}
	}()
	go func() { <-ctx.Done(); _ = admin.Shutdown(context.Background()) }()

	if err := p.Start(ctx); err != nil {
		log.WithError(err).Fatal("proxy stopped")
	}
}

// readJWTSecret decodes a hex JWT secret file (with optional 0x prefix) to raw
// bytes, matching the e2e proxy component's handling.
func readJWTSecret(pathStr string) ([]byte, error) {
	enc, err := os.ReadFile(pathStr) // #nosec G304 -- operator-supplied path
	if err != nil {
		return nil, err
	}
	return hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(string(enc)), "0x"))
}
