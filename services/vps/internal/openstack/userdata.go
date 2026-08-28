package openstack

import (
	"bytes"
	"strings"
)

func buildCloudInitUserData(rootPassword string, sshKeys []string) []byte {
	rootPassword = strings.TrimSpace(rootPassword)
	var keys []string
	for _, k := range sshKeys {
		k = strings.TrimSpace(k)
		if k != "" {
			keys = append(keys, k)
		}
	}
	if rootPassword == "" && len(keys) == 0 {
		return nil
	}

	var b bytes.Buffer
	b.WriteString("#cloud-config\n")
	if rootPassword != "" {
		b.WriteString("chpasswd:\n")
		b.WriteString("  expire: false\n")
		b.WriteString("  list: |\n")
		b.WriteString("    root:" + rootPassword + "\n")
		b.WriteString("ssh_pwauth: true\n")
	}
	if len(keys) > 0 {
		b.WriteString("ssh_authorized_keys:\n")
		for _, k := range keys {
			b.WriteString("  - ")
			b.WriteString(k)
			b.WriteByte('\n')
		}
	}
	return b.Bytes()
}
