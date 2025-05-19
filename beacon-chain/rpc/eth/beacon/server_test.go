package beacon

import (
	"fmt"
	"net/http"
	"net/http/httptest"
)

// Add helper function before test cases
func startServer(s *Server) *httptest.Server {
	router := http.NewServeMux()
	endpoints := Endpoints(s)
	for _, e := range endpoints {
		for i := range e.Methods {
			router.HandleFunc(
				fmt.Sprintf("%s %s", e.Methods[i], e.Template),
				e.HandlerWithMiddleware(),
			)
		}
	}
	return httptest.NewServer(router)
}
