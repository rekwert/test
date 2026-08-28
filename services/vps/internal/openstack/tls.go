package openstack

import (
	"crypto/tls"
	"net/http"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
)

func TLSConfig() *tls.Config {
	if hypervisor.InsecureTLS() {
		return &tls.Config{InsecureSkipVerify: true}
	}
	return nil
}

func InsecureHTTPTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if tr, ok := base.(*http.Transport); ok {
		cp := tr.Clone()
		if cp.TLSClientConfig == nil {
			cp.TLSClientConfig = &tls.Config{}
		}
		cp.TLSClientConfig.InsecureSkipVerify = true
		return cp
	}
	return &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
}
