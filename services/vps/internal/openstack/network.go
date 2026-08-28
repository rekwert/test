package openstack

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/hypervisors"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/images"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
)

func (a *Adapter) FetchComputeSnapshots(ctx context.Context) ([]hypervisor.ComputeSnapshot, error) {
	cli, err := a.clients(ctx)
	if err != nil {
		return nil, err
	}
	page, err := hypervisors.List(cli.Compute, nil).AllPages()
	if err != nil {
		return nil, fmt.Errorf("openstack: list hypervisors: %w", err)
	}
	all, err := hypervisors.ExtractHypervisors(page)
	if err != nil {
		return nil, err
	}
	out := make([]hypervisor.ComputeSnapshot, 0, len(all))
	for _, hv := range all {
		cpuPct := 0.0
		if hv.VCPUs > 0 {
			cpuPct = float64(hv.VCPUsUsed) / float64(hv.VCPUs) * 100
		}
		memPct := 0.0
		if hv.MemoryMB > 0 {
			memPct = float64(hv.MemoryMBUsed) / float64(hv.MemoryMB) * 100
		}
		commissioned := 0
		if hv.State == "up" {
			commissioned = 3
		}
		out = append(out, hypervisor.ComputeSnapshot{
			ExternalID:        hv.ID,
			Name:              hv.HypervisorHostname,
			IP:                hv.HostIP,
			Enabled:           hv.State == "up",
			Commissioned:      commissioned,
			MaxCPU:            hv.VCPUs,
			MaxMemoryMB:       hv.MemoryMB,
			CPUAllocated:      hv.VCPUsUsed,
			CPUUsedPercent:    cpuPct,
			MemoryAllocatedMB: hv.MemoryMBUsed,
			MemoryUsedPercent: memPct,
			ServerCount:       hv.RunningVMs,
		})
	}
	return out, nil
}

func (a *Adapter) FetchOSImages(ctx context.Context) ([]hypervisor.OSImageRow, error) {
	cli, err := a.clients(ctx)
	if err != nil {
		return nil, err
	}
	page, err := images.List(cli.Image, images.ListOpts{Status: images.ImageStatusActive}).AllPages()
	if err != nil {
		return nil, fmt.Errorf("openstack: list images: %w", err)
	}
	all, err := images.ExtractImages(page)
	if err != nil {
		return nil, err
	}
	out := make([]hypervisor.OSImageRow, 0, len(all))
	for catalogID, ref := range a.cfg.OSTemplates {
		for _, img := range all {
			if img.ID == ref || img.Name == ref {
				out = append(out, hypervisor.OSImageRow{
					Name:      img.Name,
					Version:   catalogID,
					VersionID: 0,
				})
				break
			}
		}
	}
	if len(out) == 0 {
		for _, img := range all {
			out = append(out, hypervisor.OSImageRow{Name: img.Name, Version: img.ID})
		}
	}
	return out, nil
}
