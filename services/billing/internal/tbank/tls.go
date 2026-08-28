package tbank

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"
	"time"
)

func newHTTPClient() *http.Client {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	for _, path := range []string{
		"/etc/ssl/certs/russian_trusted_root_ca.pem",
		"/usr/local/share/ca-certificates/russian_trusted_root_ca.crt",
		"/usr/local/share/ca-certificates/russian_trusted_sub_ca.crt",
	} {
		if b, err := os.ReadFile(path); err == nil {
			pool.AppendCertsFromPEM(b)
		}
	}
	return &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    pool,
			},
		},
	}
}
