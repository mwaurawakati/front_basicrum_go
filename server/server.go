package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/basicrum/front_basicrum_go/backup"
	"github.com/basicrum/front_basicrum_go/service"
	"github.com/rs/cors"
)

// Server represents http or https server
type Server struct {
	port            string
	service         service.IService
	backup          backup.IBackup
	certFile        string
	keyFile         string
	server          *http.Server
	tlsConfig       *tls.Config
	privateAPIToken string
}

// WithHTTP creates server with port
func WithHTTP(port string) func(*Server) {
	return func(s *Server) {
		s.port = port
	}
}

// WithSSL creates server with SSL port and certificate/key files
func WithSSL(port, certFile, keyFile string) func(*Server) {
	return func(s *Server) {
		s.port = port
		s.certFile = certFile
		s.keyFile = keyFile
	}
}

// WithTLSConfig creates server with SSL port and tls config
func WithTLSConfig(port string, tlsConfig *tls.Config) func(*Server) {
	return func(s *Server) {
		s.port = port
		s.tlsConfig = tlsConfig
	}
}

// New creates a new http or https server
func New(
	processService service.IService,
	backupService backup.IBackup,
	privateAPIToken string,
	options ...func(*Server),
) *Server {
	result := &Server{
		service:         processService,
		backup:          backupService,
		privateAPIToken: privateAPIToken,
	}
	for _, o := range options {
		o(result)
	}
	return result
}

// Serve start http or https server and blocks
func (s *Server) Serve() error {
	mux := http.NewServeMux()
	s.setupRoutes(mux)
	handler := cors.Default().Handler(mux)
	s.server = &http.Server{
		Addr:    ":" + s.port,
		Handler: handler,
		// https://deepsource.io/directory/analyzers/go/issues/GO-S2114
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       120 * time.Second,
		TLSConfig:         s.tlsConfig,
	}
	if s.certFile != "" || s.keyFile != "" || s.tlsConfig != nil {
		slog.Info(fmt.Sprintf("starting https server on port[%v] with certFile[%v] keyFile[%v]", s.port, s.certFile, s.keyFile))
		return s.server.ListenAndServeTLS(s.certFile, s.keyFile)
	}
	slog.Info(fmt.Sprintf("starting http server on port[%v]", s.port))
	return s.server.ListenAndServe()
}

// Shutdown gracefully shutdowns the http server
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return errors.New("server is not started")
	}
	return s.server.Shutdown(ctx)
}
