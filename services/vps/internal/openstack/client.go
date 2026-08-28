package openstack

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/images"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
)

type Clients struct {
	Provider *gophercloud.ProviderClient
	Compute  *gophercloud.ServiceClient
	Network  *gophercloud.ServiceClient
	Image    *gophercloud.ServiceClient
	Region   string
}

func NewClients(_ context.Context, cfg Config) (*Clients, error) {
	if cfg.AuthURL == "" {
		return nil, fmt.Errorf("openstack: OPENSTACK_AUTH_URL (or OS_AUTH_URL) is required")
	}
	opts := authOptions(cfg)
	provider, err := openstack.AuthenticatedClient(opts)
	if err != nil {
		return nil, fmt.Errorf("openstack: authenticate: %w", err)
	}
	if cfg.InsecureTLS {
		provider.HTTPClient.Transport = insecureTransport(provider.HTTPClient.Transport)
	}
	endpoint := gophercloud.EndpointOpts{Region: cfg.Region}
	compute, err := openstack.NewComputeV2(provider, endpoint)
	if err != nil {
		return nil, fmt.Errorf("openstack: compute client: %w", err)
	}
	network, err := openstack.NewNetworkV2(provider, endpoint)
	if err != nil {
		return nil, fmt.Errorf("openstack: network client: %w", err)
	}
	image, err := openstack.NewImageServiceV2(provider, endpoint)
	if err != nil {
		return nil, fmt.Errorf("openstack: image client: %w", err)
	}
	return &Clients{
		Provider: provider,
		Compute:  compute,
		Network:  network,
		Image:    image,
		Region:   cfg.Region,
	}, nil
}

func authOptions(cfg Config) gophercloud.AuthOptions {
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: cfg.AuthURL,
		Username:         cfg.Username,
		Password:         cfg.Password,
		TenantName:       cfg.TenantName,
		DomainName:       cfg.DomainName,
		AllowReauth:      true,
	}
	if cfg.ApplicationCredID != "" && cfg.ApplicationCredSecret != "" {
		opts.ApplicationCredentialID = cfg.ApplicationCredID
		opts.ApplicationCredentialSecret = cfg.ApplicationCredSecret
		opts.Username = ""
		opts.Password = ""
	}
	return opts
}

func insecureTransport(base http.RoundTripper) http.RoundTripper {
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

func (c *Clients) resolveFlavorRef(_ context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("openstack: empty flavor ref")
	}
	if looksLikeUUID(ref) {
		return ref, nil
	}
	page, err := flavors.ListDetail(c.Compute, flavors.ListOpts{}).AllPages()
	if err != nil {
		return "", err
	}
	all, err := flavors.ExtractFlavors(page)
	if err != nil {
		return "", err
	}
	for _, f := range all {
		if f.Name == ref || f.ID == ref {
			return f.ID, nil
		}
	}
	return "", fmt.Errorf("openstack: flavor %q not found", ref)
}

func (c *Clients) resolveImageRef(_ context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("openstack: empty image ref")
	}
	if looksLikeUUID(ref) {
		return ref, nil
	}
	page, err := images.List(c.Image, images.ListOpts{Name: ref}).AllPages()
	if err != nil {
		return "", err
	}
	all, err := images.ExtractImages(page)
	if err != nil {
		return "", err
	}
	for _, img := range all {
		if img.Name == ref || img.ID == ref {
			return img.ID, nil
		}
	}
	return "", fmt.Errorf("openstack: image %q not found", ref)
}

func (c *Clients) resolveNetworkID(_ context.Context, cfg Config) (string, error) {
	if cfg.NetworkID != "" {
		return cfg.NetworkID, nil
	}
	page, err := networks.List(c.Network, networks.ListOpts{}).AllPages()
	if err != nil {
		return "", err
	}
	all, err := networks.ExtractNetworks(page)
	if err != nil {
		return "", err
	}
	for _, n := range all {
		if n.Name == "private" || n.Name == "internal" {
			return n.ID, nil
		}
	}
	for _, n := range all {
		if !n.Shared && n.Status == "ACTIVE" {
			return n.ID, nil
		}
	}
	if len(all) > 0 {
		return all[0].ID, nil
	}
	return "", fmt.Errorf("openstack: no neutron network found; set OPENSTACK_NETWORK_ID")
}

func serverByID(compute *gophercloud.ServiceClient, id string) (*servers.Server, error) {
	return servers.Get(compute, id).Extract()
}

func looksLikeUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
				return false
			}
		}
	}
	return true
}
