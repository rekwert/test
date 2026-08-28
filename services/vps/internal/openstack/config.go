package openstack

import (
	"os"
	"sort"
	"strings"
)

// Config holds OpenStack mapping and connectivity settings for the VPS adapter.
type Config struct {
	AuthURL            string
	Region             string
	Username           string
	Password           string
	TenantName         string
	DomainName         string
	ApplicationCredID  string
	ApplicationCredSecret string
	InsecureTLS        bool
	// PlanFlavors maps catalog plan UUID -> Nova flavor ID or name.
	PlanFlavors map[string]string
	// OSTemplates maps catalog os_template_id -> Glance image ID or name.
	OSTemplates map[string]string
	// NetworkID is the Neutron network for VM NICs (optional; auto-pick when empty).
	NetworkID string
	// FloatingNetworkID is the external pool for floating IPs (optional).
	FloatingNetworkID string
	ProvisionRegions  map[string]struct{}
	DefaultAvailabilityZone string
	// NodeHosts maps portal node UUID -> Nova compute hostname (OPENSTACK_NODE_MAP).
	NodeHosts map[string]string
	// HypervisorHosts maps Nova hypervisor UUID -> compute hostname (OPENSTACK_HV_MAP).
	HypervisorHosts map[string]string
}

func LoadConfig() Config {
	cfg := Config{
		AuthURL:           firstEnv("OPENSTACK_AUTH_URL", "OS_AUTH_URL"),
		Region:            firstEnv("OPENSTACK_REGION", "OS_REGION_NAME"),
		Username:          firstEnv("OPENSTACK_USERNAME", "OS_USERNAME"),
		Password:          firstEnv("OPENSTACK_PASSWORD", "OS_PASSWORD"),
		TenantName:        firstEnv("OPENSTACK_PROJECT_NAME", "OPENSTACK_TENANT_NAME", "OS_PROJECT_NAME"),
		DomainName:        firstEnv("OPENSTACK_DOMAIN_NAME", "OS_USER_DOMAIN_NAME", "OS_PROJECT_DOMAIN_NAME"),
		ApplicationCredID: firstEnv("OPENSTACK_APPLICATION_CREDENTIAL_ID", "OS_APPLICATION_CREDENTIAL_ID"),
		ApplicationCredSecret: firstEnv("OPENSTACK_APPLICATION_CREDENTIAL_SECRET", "OS_APPLICATION_CREDENTIAL_SECRET"),
		InsecureTLS:       envBool("OPENSTACK_INSECURE_TLS", false),
		PlanFlavors:       parseStringMap(os.Getenv("OPENSTACK_PLAN_MAP")),
		OSTemplates:       parseStringMap(firstEnv("OPENSTACK_OS_MAP", "OPENSTACK_IMAGE_MAP")),
		NetworkID:         strings.TrimSpace(os.Getenv("OPENSTACK_NETWORK_ID")),
		FloatingNetworkID: strings.TrimSpace(os.Getenv("OPENSTACK_FLOATING_NETWORK_ID")),
		ProvisionRegions:  parseRegionSet(os.Getenv("OPENSTACK_PROVISION_REGIONS")),
		DefaultAvailabilityZone: strings.TrimSpace(os.Getenv("OPENSTACK_AVAILABILITY_ZONE")),
		NodeHosts:         parseStringMap(os.Getenv("OPENSTACK_NODE_MAP")),
		HypervisorHosts:   parseStringMap(os.Getenv("OPENSTACK_HV_MAP")),
	}
	if len(cfg.PlanFlavors) == 0 {
		if fallback := strings.TrimSpace(os.Getenv("OPENSTACK_DEFAULT_FLAVOR")); fallback != "" {
			cfg.PlanFlavors["default"] = fallback
		}
	}
	return cfg
}

func (c Config) FlavorRef(planID string) (string, bool) {
	planID = strings.TrimSpace(planID)
	if ref, ok := c.PlanFlavors[planID]; ok && ref != "" {
		return ref, true
	}
	if ref, ok := c.PlanFlavors["default"]; ok && ref != "" {
		return ref, true
	}
	return "", false
}

func (c Config) HasExplicitFlavor(planID string) bool {
	planID = strings.TrimSpace(planID)
	ref, ok := c.PlanFlavors[planID]
	return ok && ref != ""
}

func (c Config) ImageRef(catalogOS string) (string, bool) {
	ref, ok := c.OSTemplates[strings.TrimSpace(catalogOS)]
	return ref, ok && ref != ""
}

// ComputeHost resolves Nova scheduler host hint for a portal node.
func (c Config) ComputeHost(nodeID, hypervisorExternalID string) string {
	nodeID = strings.TrimSpace(nodeID)
	if host, ok := c.NodeHosts[nodeID]; ok && host != "" {
		return host
	}
	ext := strings.TrimSpace(hypervisorExternalID)
	if ext != "" {
		if host, ok := c.HypervisorHosts[ext]; ok && host != "" {
			return host
		}
		if !isNumericExternalID(ext) {
			return ext
		}
	}
	return ""
}

func isNumericExternalID(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	for _, r := range raw {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (c Config) UniqueFlavorRefs() []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(c.PlanFlavors))
	for _, ref := range c.PlanFlavors {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

func RegionEnabled(region string) bool {
	cfg := LoadConfig()
	if len(cfg.ProvisionRegions) == 0 {
		return true
	}
	_, ok := cfg.ProvisionRegions[strings.ToLower(strings.TrimSpace(region))]
	return ok
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func parseStringMap(raw string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		out[k] = v
	}
	return out
}

func parseRegionSet(raw string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part != "" {
			out[part] = struct{}{}
		}
	}
	return out
}

func envBool(key string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
