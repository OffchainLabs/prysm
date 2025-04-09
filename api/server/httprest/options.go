package httprest

import (
	"time"

	"net/http"

	"github.com/prysmaticlabs/prysm/v5/api/server/middleware"
)

// Option is a http rest server functional parameter type.
type Option func(g *Server) error

// WithMiddlewares sets the list of middlewares to be applied on routes.
func WithMiddlewares(mw []middleware.Middleware) Option {
	return func(g *Server) error {
		g.cfg.middlewares = mw
		return nil
	}
}

// WithHTTPAddr sets the full address ( host and port ) of the server.
func WithHTTPAddr(addr string) Option {
	return func(g *Server) error {
		g.cfg.httpAddr = addr
		return nil
	}
}

// WithRouter sets the internal router of the server, this is required.
func WithRouter(r *http.ServeMux) Option {
	return func(g *Server) error {
		g.cfg.router = r
		return nil
	}
}

// WithTimeout allows changing the timeout value for API calls.
func WithTimeout(duration time.Duration) Option {
	return func(g *Server) error {
		g.cfg.timeout = duration
		return nil
	}
}
