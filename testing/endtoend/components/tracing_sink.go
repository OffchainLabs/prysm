package components

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/OffchainLabs/prysm/v7/testing/endtoend/helpers"
	e2e "github.com/OffchainLabs/prysm/v7/testing/endtoend/params"
	"github.com/OffchainLabs/prysm/v7/testing/endtoend/types"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

var _ types.ComponentRunner = &TracingSink{}

// TracingSink to capture HTTP requests from opentracing pushes. This is meant
// to capture all opentracing spans from Prysm during an end-to-end test. Spans
// are normally sent to a jaeger (https://www.jaegertracing.io/docs/1.25/getting-started/)
// endpoint, but here we instead replace that with our own http request sink.
// The request sink receives any requests, raw marshals them and base64-encodes them,
// then writes them newline-delimited into a file.
//
// The output file from this component can then be used by tools/replay-http in
// the Prysm repository to replay requests to a jaeger collector endpoint. This
// can then be used to visualize the spans themselves in the jaeger UI.
type TracingSink struct {
	cancel   context.CancelFunc
	started  chan struct{}
	stopped  chan struct{}
	endpoint string
	server   *http.Server
}

// NewTracingSink initializes the tracing sink component.
func NewTracingSink(endpoint string) *TracingSink {
	return &TracingSink{
		started:  make(chan struct{}, 1),
		stopped:  make(chan struct{}),
		endpoint: endpoint,
	}
}

// Start the tracing sink.
func (ts *TracingSink) Start(ctx context.Context) error {
	if ts.endpoint == "" {
		return errors.New("empty endpoint provided")
	}
	ctx, cancelF := context.WithCancel(ctx)
	ts.cancel = cancelF
	go ts.initializeSink(ctx)
	close(ts.started)
	return nil
}

// Started checks whether a tracing sink is started and ready to be queried.
func (ts *TracingSink) Started() <-chan struct{} {
	return ts.started
}

// Pause pauses the component and its underlying process.
func (ts *TracingSink) Pause() error {
	return nil
}

// Resume resumes the component and its underlying process.
func (ts *TracingSink) Resume() error {
	return nil
}

// Stop stops the component and its underlying process.
func (ts *TracingSink) Stop() error {
	if ts.cancel != nil {
		ts.cancel()
	}
	// Wait for server to actually shut down before returning
	<-ts.stopped
	return nil
}

// reusePortListener creates a TCP listener with SO_REUSEADDR set, allowing
// the port to be reused immediately after the previous listener closes.
// This is essential for sequential E2E tests that reuse the same port.
func reusePortListener(addr string) (net.Listener, error) {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var setSockOptErr error
			err := c.Control(func(fd uintptr) {
				setSockOptErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
			})
			if err != nil {
				return err
			}
			return setSockOptErr
		},
	}
	return lc.Listen(context.Background(), "tcp", addr)
}

// Initialize an http handler that writes all requests to a file.
func (ts *TracingSink) initializeSink(ctx context.Context) {
	defer close(ts.stopped)

	mux := &http.ServeMux{}
	ts.server = &http.Server{
		Addr:              ts.endpoint,
		Handler:           mux,
		ReadHeaderTimeout: time.Second,
	}
	// Disable keep-alives to ensure connections close immediately
	ts.server.SetKeepAlivesEnabled(false)

	// Create listener with SO_REUSEADDR to allow port reuse between tests
	listener, err := reusePortListener(ts.endpoint)
	if err != nil {
		log.WithError(err).Error("Failed to create listener")
		return
	}

	stdOutFile, err := helpers.DeleteAndCreateFile(e2e.TestParams.LogPath, e2e.TracingRequestSinkFileName)
	if err != nil {
		log.WithError(err).Error("Failed to create stdout file")
		if closeErr := listener.Close(); closeErr != nil {
			log.WithError(closeErr).Error("Failed to close listener after file creation error")
		}
		return
	}

	cleanup := func() {
		if err := stdOutFile.Close(); err != nil {
			log.WithError(err).Error("Could not close stdout file")
		}
		// Use Shutdown for graceful shutdown that releases the port
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := ts.server.Shutdown(shutdownCtx); err != nil {
			log.WithError(err).Error("Could not gracefully shutdown http server")
			// Force close if shutdown times out
			if err := ts.server.Close(); err != nil {
				log.WithError(err).Error("Could not close http server")
			}
		}
	}
	defer cleanup()

	mux.HandleFunc("/", func(_ http.ResponseWriter, r *http.Request) {
		if err := captureRequest(stdOutFile, r); err != nil {
			log.WithError(err).Error("Failed to capture http request")
			return
		}
	})

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-sigs:
			return
		}
	}()

	// Use Serve with our custom listener instead of ListenAndServe
	if err := ts.server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.WithError(err).Error("Failed to serve http")
	}
}

// Captures raw requests in base64 encoded form in a line-delimited file.
func captureRequest(f io.Writer, r *http.Request) error {
	buf := bytes.NewBuffer(nil)
	err := r.Write(buf)
	if err != nil {
		return err
	}
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(buf.Bytes())))
	base64.StdEncoding.Encode(encoded, buf.Bytes())
	encoded = append(encoded, []byte("\n")...)
	_, err = f.Write(encoded)
	if err != nil {
		return err
	}
	return nil
}

func (ts *TracingSink) UnderlyingProcess() *os.Process {
	return nil // No subprocess for this component.
}
