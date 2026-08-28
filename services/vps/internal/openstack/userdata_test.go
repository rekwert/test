package openstack

import (
	"strings"
	"testing"
)

func TestBuildCloudInitUserData(t *testing.T) {
	ud := buildCloudInitUserData("secret", []string{"ssh-rsa AAA"})
	if len(ud) == 0 {
		t.Fatal("expected cloud-init payload")
	}
	text := string(ud)
	if !strings.HasPrefix(text, "#cloud-config\n") {
		t.Fatalf("unexpected prefix: %q", text[:min(20, len(text))])
	}
	if !strings.Contains(text, "root:secret") {
		t.Fatal("expected root password in chpasswd")
	}
	if !strings.Contains(text, "ssh-rsa AAA") {
		t.Fatal("expected ssh key")
	}
}
