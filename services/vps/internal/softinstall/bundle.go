package softinstall

import (
	"encoding/json"
	"fmt"
	"strings"
)

const bundlePrefix = "BUNDLE_JSON:"

// Bundle is persisted in instances.provider_meta.software_bundle for the portal.
type Bundle struct {
	Profile   string          `json:"profile"`
	Panel     PanelAccess     `json:"panel,omitempty"`
	Claude    ClaudeAccess    `json:"claude,omitempty"`
	Amnezia   AmneziaAccess   `json:"amnezia,omitempty"`
	VLESS     VLESSClient     `json:"vless,omitempty"`
	Hysteria2 Hysteria2Client `json:"hysteria2,omitempty"`
}

type AmneziaAccess struct {
	VpnURI     string `json:"vpn_uri"`
	ClientName string `json:"client_name,omitempty"`
	Port       int    `json:"port,omitempty"`
}

type PanelAccess struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type VLESSClient struct {
	URI       string `json:"uri"`
	Port      int    `json:"port"`
	SNI       string `json:"sni"`
	UUID      string `json:"uuid"`
	PublicKey string `json:"public_key"`
	ShortID   string `json:"short_id"`
	Email     string `json:"email"`
}

type Hysteria2Client struct {
	URI      string `json:"uri"`
	Port     int    `json:"port"`
	SNI      string `json:"sni"`
	Password string `json:"password"`
	Email    string `json:"email"`
}

func parseBundleOutput(stdout string) (*Bundle, error) {
	for i := len(stdout) - 1; i >= 0; i-- {
		if stdout[i] != '\n' {
			end := i + 1
			start := i
			for start > 0 && stdout[start-1] != '\n' {
				start--
			}
			line := strings.TrimSpace(stdout[start:end])
			if strings.HasPrefix(line, bundlePrefix) {
				raw := strings.TrimPrefix(line, bundlePrefix)
				var bundle Bundle
				if err := json.Unmarshal([]byte(raw), &bundle); err != nil {
					return nil, fmt.Errorf("parse bundle json: %w", err)
				}
				if err := validateBundle(&bundle); err != nil {
					return nil, err
				}
				return &bundle, nil
			}
			i = start
		}
	}
	tail := stdout
	if len(tail) > 800 {
		tail = tail[len(tail)-800:]
	}
	return nil, fmt.Errorf("bundle json not found in installer output: %s", tail)
}

func validateBundle(bundle *Bundle) error {
	profile := strings.TrimSpace(strings.ToLower(bundle.Profile))
	switch profile {
	case "amnezia":
		if strings.TrimSpace(bundle.Amnezia.VpnURI) == "" {
			return fmt.Errorf("bundle missing amnezia vpn_uri")
		}
		return nil
	default:
		if strings.TrimSpace(bundle.Panel.URL) == "" {
			return fmt.Errorf("bundle missing panel url")
		}
		return nil
	}
}
