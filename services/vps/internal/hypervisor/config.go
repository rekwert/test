package hypervisor

import (
	"log"
	"os"
	"strings"

	"github.com/borishru-boop/testVPStrade/packages/shared-go/prodenv"
)

func MockEnabled() bool {
	for _, key := range []string{"OPENSTACK_USE_MOCK", "HYPERVISOR_USE_MOCK"} {
		v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
		switch v {
		case "1", "true", "yes", "on":
			return true
		}
	}
	return false
}

func RegionEnabled(region string) bool {
	regions := parseRegionSet(os.Getenv("OPENSTACK_PROVISION_REGIONS"))
	if len(regions) == 0 {
		return true
	}
	_, ok := regions[strings.ToLower(strings.TrimSpace(region))]
	return ok
}

func HasExplicitPlan(planID string) bool {
	planID = strings.TrimSpace(planID)
	if planID == "" {
		return false
	}
	ref, ok := parseStringMap(os.Getenv("OPENSTACK_PLAN_MAP"))[planID]
	return ok && ref != ""
}

func ActivePlanMapLen() int {
	return len(parseStringMap(os.Getenv("OPENSTACK_PLAN_MAP")))
}

func OSTemplateConfigured(osID string) bool {
	osID = strings.TrimSpace(osID)
	raw := strings.TrimSpace(os.Getenv("OPENSTACK_OS_MAP"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("OPENSTACK_IMAGE_MAP"))
	}
	_, ok := parseStringMap(raw)[osID]
	return ok
}

func ActiveOSTemplateMap() map[string]int {
	raw := strings.TrimSpace(os.Getenv("OPENSTACK_OS_MAP"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("OPENSTACK_IMAGE_MAP"))
	}
	src := parseStringMap(raw)
	out := make(map[string]int, len(src))
	i := 1
	for k := range src {
		out[k] = i
		i++
	}
	return out
}

func InsecureTLS() bool {
	return envBool("OPENSTACK_INSECURE_TLS", false)
}

func AssertProductionHypervisor() {
	if !prodenv.IsProduction() {
		return
	}
	if MockEnabled() {
		log.Fatal("OPENSTACK_USE_MOCK must not be enabled when APP_ENV=production")
	}
	if !OpenStackConfigured() {
		log.Fatal("OPENSTACK_AUTH_URL must be set when APP_ENV=production")
	}
	prodenv.AssertOpenStackTLS(InsecureTLS())
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
		if k != "" && v != "" {
			out[k] = v
		}
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
