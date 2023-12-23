package server

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/basicrum/front_basicrum_go/backup"
	"github.com/basicrum/front_basicrum_go/service"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

const (
	defaultHTTPPort  = "80"
	defaultHTTPSPort = "443"
	cacheDirPath     = "cache-dir"
)

// SSLType is the type of http SSL configuration to use
type SSLType string

const (
	// SSLTypeFile file configuration
	SSLTypeFile SSLType = "FILE"
	// SSLTypeLetsEncrypt let's encrypt configuration
	SSLTypeLetsEncrypt SSLType = "LETS_ENCRYPT"
)

// Factory is server factory
type Factory struct {
	processService *service.Service
	backupService  backup.IBackup
}

// NewFactory returns server factory
func NewFactory(
	processService *service.Service,
	backupService backup.IBackup,
) *Factory {
	return &Factory{
		processService: processService,
		backupService:  backupService,
	}
}

type configType struct {
	Port            string  `json:"BRUM_SERVER_PORT"`
	SSL             bool    `json:"BRUM_SERVER_SSL" default:"false"`
	SSLType         SSLType `json:"BRUM_SERVER_SSL_TYPE" default:"FILE"`
	SSLFileCertFile string  `json:"BRUM_SERVER_SSL_CERT_FILE"`
	SSLFileKeyFile  string  `json:"BRUM_SERVER_SSL_KEY_FILE"`
	Domain          string  `json:"BRUM_SERVER_SSL_LETS_ENCRYPT_DOMAIN"`
	Token           string  `json:"BRUM_PRIVATE_API_TOKEN"`
}

// Build creates http/https server(s) based on startup configuration
func (f *Factory) Build(jsonconfig json.RawMessage) ([]*Server, error) {

	if len(jsonconfig) < 2 {
		return nil, errors.New("server missing config")
	}

	var err error
	var sConf configType
	if err = json.Unmarshal(jsonconfig, &sConf); err != nil {
		return nil, errors.New("server failed to parse config: " + err.Error())
	}
	httpPort := defaultValue(sConf.Port, defaultHTTPPort)
	httpsPort := defaultValue(sConf.Port, defaultHTTPSPort)

	if !sConf.SSL {
		log.Println("HTTP configuration enabled")
		httpServer := New(
			f.processService,
			f.backupService,
			sConf.Token,
			WithHTTP(httpPort),
		)
		return []*Server{httpServer}, nil
	}

	log.Printf("SSL configuration enabled type[%v]\n", sConf.SSLType)
	switch sConf.SSLType {
	case SSLTypeLetsEncrypt:
		allowedHost := sConf.Domain
		log.Printf("SSL Let's Encrypt allowedHost[%v]\n", allowedHost)
		tlsConfig := makeLetsEncryptTLSConfig(allowedHost)
		httpsServer := New(
			f.processService,
			f.backupService,
			sConf.Token,
			WithTLSConfig(defaultHTTPSPort, tlsConfig),
		)
		httpServer := New(
			f.processService,
			f.backupService,
			sConf.Token,
			WithHTTP(httpPort),
		)
		return []*Server{httpsServer, httpServer}, nil
	case SSLTypeFile:
		log.Println("SSL files configuration enabled")
		httpsServer := New(
			f.processService,
			f.backupService,
			sConf.Token,
			WithSSL(httpsPort, sConf.SSLFileCertFile, sConf.SSLFileKeyFile),
		)
		httpServer := New(
			f.processService,
			f.backupService,
			sConf.Token,
			WithHTTP(httpPort),
		)
		return []*Server{httpsServer, httpServer}, nil
	default:
		return nil, fmt.Errorf("unsupported SSL type[%v]", sConf.SSLType)
	}
}

func makeLetsEncryptTLSConfig(allowedHost string) *tls.Config {
	client := makeACMEClient()
	m := &autocert.Manager{
		Cache:      autocert.DirCache(cacheDirPath),
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(allowedHost),
		Client:     client,
	}
	// nolint: gosec
	return &tls.Config{GetCertificate: m.GetCertificate}
}

func makeACMEClient() *acme.Client {
	directoryURL := os.Getenv("TEST_DIRECTORY_URL")
	if directoryURL == "" {
		return nil
	}
	insecureSkipVerify := os.Getenv("TEST_INSECURE_SKIP_VERIFYy") == "true"
	return &acme.Client{
		DirectoryURL: directoryURL,
		HTTPClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					// nolint: gosec
					InsecureSkipVerify: insecureSkipVerify,
				},
			},
		},
	}
}

func defaultValue(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}
