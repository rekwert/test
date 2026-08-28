package hypervisor

import "sync"

var (
	osMapMu sync.RWMutex
	osMap   map[string]int
)

// SetDynamicOSTemplates replaces runtime OS map synced from Glance (merged with env on lookup).
func SetDynamicOSTemplates(m map[string]int) {
	osMapMu.Lock()
	defer osMapMu.Unlock()
	if len(m) == 0 {
		osMap = nil
		return
	}
	osMap = make(map[string]int, len(m))
	for k, v := range m {
		osMap[k] = v
	}
}

func lookupOSImageID(catalogOS string) (int, bool) {
	osMapMu.RLock()
	if id, ok := osMap[catalogOS]; ok && id > 0 {
		osMapMu.RUnlock()
		return id, true
	}
	osMapMu.RUnlock()
	id, ok := ActiveOSTemplateMap()[catalogOS]
	return id, ok && id > 0
}
