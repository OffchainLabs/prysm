package httprest

import (
	"context"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/server/middleware"
	"github.com/prysmaticlabs/prysm/v5/runtime"
)

var _ runtime.Service = (*Server)(nil)

// Config parameters for setting up the http-rest service.
type config struct {
	httpAddr    string
	middlewares []middleware.Middleware
	router      http.Handler
	timeout     time.Duration
}

// Server serves HTTP traffic.
type Server struct {
	cfg          *config
	server       *http.Server
	cancel       context.CancelFunc
	ctx          context.Context
	startFailure error
}

// New returns a new instance of the Server.
func New(ctx context.Context, opts ...Option) (*Server, error) {
	g := &Server{
		ctx: ctx,
		cfg: &config{},
	}
	for _, opt := range opts {
		if err := opt(g); err != nil {
			return nil, err
		}
	}
	if g.cfg.router == nil {
		return nil, errors.New("router option not configured")
	}
	var handler http.Handler
	defaultReadHeaderTimeout := time.Second
	handler = middleware.MiddlewareChain(g.cfg.router, g.cfg.middlewares)
	if g.cfg.timeout > 0*time.Second {
		defaultReadHeaderTimeout = g.cfg.timeout
		handler = http.TimeoutHandler(handler, g.cfg.timeout, "request timed out")
	}
	g.server = &http.Server{
		Addr:              g.cfg.httpAddr,
		Handler:           handler,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
	}

	return g, nil
}

// Start the http rest service.
func (g *Server) Start() {
	g.ctx, g.cancel = context.WithCancel(g.ctx)

	go func() {
		log.WithField("address", g.cfg.httpAddr).Info("Starting HTTP server")
		if err := g.server.ListenAndServe(); err != http.ErrServerClosed {
			log.WithError(err).Error("Failed to start HTTP server")
			g.startFailure = err
			return
		}
	}()
}

// Status of the HTTP server. Returns an error if this service is unhealthy.
func (g *Server) Status() error {
	if g.startFailure != nil {
		return g.startFailure
	}
	return nil
}

// Stop the HTTP server with a graceful shutdown.
func (g *Server) Stop() error {
	if g.server != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(g.ctx, 2*time.Second)
		defer shutdownCancel()
		if err := g.server.Shutdown(shutdownCtx); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				log.Warn("Existing connections terminated")
			} else {
				log.WithError(err).Error("Failed to gracefully shut down server")
			}
		}
	}
	if g.cancel != nil {
		g.cancel()
	}
	return nil
}
