package hvinit

import (
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/openstack"
)

func NewAdapter() hypervisor.Adapter {
	if hypervisor.MockEnabled() {
		return hypervisor.NewMock()
	}
	return openstack.NewAdapter(openstack.LoadConfig())
}

func NewNodeSync() hypervisor.NodeSyncSource {
	if hypervisor.MockEnabled() {
		return hypervisor.NewMock()
	}
	return openstack.NewAdapter(openstack.LoadConfig())
}

func NewOSSync() hypervisor.OSSyncSource {
	if hypervisor.MockEnabled() {
		return hypervisor.NewMock()
	}
	return openstack.NewAdapter(openstack.LoadConfig())
}
