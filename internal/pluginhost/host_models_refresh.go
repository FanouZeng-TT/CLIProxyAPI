package pluginhost

import (
	"context"
	"fmt"
)

// SetModelCatalogRefresh installs the service's serialized model-only refresh.
// Plugins call it from a background task, outside their catalog locks: the
// refresh reads model.static again, without reconfiguring or reloading plugins.
func (h *Host) SetModelCatalogRefresh(refresh func(context.Context, string) (string, error)) {
	h.mu.Lock()
	h.modelCatalogRefresh = refresh
	h.mu.Unlock()
}

func (h *Host) supportsModelCatalogRefresh() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.modelCatalogRefresh != nil
}

func (h *Host) callHostModelsRefresh(ctx context.Context) ([]byte, error) {
	id := hostCallbackPluginIDFromContext(ctx)
	if id == "" {
		return nil, fmt.Errorf("model catalog refresh requires plugin identity")
	}
	h.mu.Lock()
	refresh := h.modelCatalogRefresh
	h.mu.Unlock()
	if refresh == nil {
		return nil, fmt.Errorf("model catalog refresh is unavailable")
	}
	var caller capabilityRecord
	found := false
	for _, record := range h.activeRecords() {
		if record.id == id {
			caller, found = record, true
			break
		}
	}
	if !found || caller.plugin.Capabilities.ModelProvider == nil {
		return nil, fmt.Errorf("model provider is not active")
	}
	revision, errRefresh := refresh(ctx, id)
	if errRefresh != nil {
		return nil, errRefresh
	}
	if !h.recordCurrent(caller) {
		return nil, fmt.Errorf("model provider changed during refresh")
	}
	if instance := hostCallbackInstanceFromContext(ctx); instance != nil && instance.closed.Load() {
		return nil, fmt.Errorf("model provider instance is closed")
	}
	return marshalRPCResult(struct {
		Revision string `json:"catalog_revision"`
	}{Revision: revision})
}

// ModelCatalogRevision returns the last catalog collected from model.static.
// The service reads it while holding its model runtime synchronization lock.
func (h *Host) ModelCatalogRevision(id string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.modelCatalogRevisions[id]
}
