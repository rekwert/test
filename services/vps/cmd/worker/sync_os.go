package main

import (
	"context"
	"log"
	"time"

	"github.com/borishru-boop/testVPStrade/services/vps/internal/catalog"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/hypervisor"
	"github.com/borishru-boop/testVPStrade/services/vps/internal/store"
)

func syncOS(ctx context.Context, st *store.Store, src hypervisor.OSSyncSource) error {
	if src == nil || st == nil {
		return nil
	}
	images, err := src.FetchOSImages(ctx)
	if err != nil {
		return err
	}
	rows := make([]catalog.OSImageVersion, 0, len(images))
	for _, img := range images {
		rows = append(rows, catalog.OSImageVersion{
			Name:      img.Name,
			Version:   img.Version,
			VersionID: img.VersionID,
		})
	}
	matched := catalog.BuildOSCatalogFromImages(rows)
	now := time.Now().UTC()
	if err := st.SyncOSTemplatesFull(ctx, matched, now); err != nil {
		return err
	}
	if len(matched) > 0 {
		idMap := make(map[string]int, len(matched))
		for _, row := range matched {
			idMap[row.ID] = row.VersionID
		}
		hypervisor.SetDynamicOSTemplates(idMap)
		log.Printf("os sync: %d templates active", len(matched))
	}
	return nil
}

func loadOSMapFromDB(ctx context.Context, st *store.Store) {
	if st == nil {
		return
	}
	m, err := st.ListOSExternalMap(ctx)
	if err != nil {
		log.Printf("os map load: %v", err)
		return
	}
	if len(m) > 0 {
		hypervisor.SetDynamicOSTemplates(m)
	}
}
