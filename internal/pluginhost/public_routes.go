package pluginhost

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	log "github.com/sirupsen/logrus"
)

const publicPluginBasePath = "/v1/plugins"

type publicRouteRecord struct {
	pluginID string
	path     string
	version  string
	route    pluginapi.PublicRoute
}

// RegisterPublicRoutes rebuilds the plugin-owned public API route table.
// Public routes are mounted under /v1/plugins/<pluginID>/ and authenticated
// with the standard API access key.
func (h *Host) RegisterPublicRoutes(ctx context.Context) {
	if h == nil {
		return
	}
	nextRoutes := make(map[string]publicRouteRecord)
	for _, record := range h.activeRecords() {
		capability := record.plugin.Capabilities.PublicAPI
		if capability == nil || h.isPluginFused(record.id) {
			continue
		}
		resp, errRegister := h.callPublicRegistrar(ctx, record, capability)
		if errRegister != nil {
			log.Warnf("pluginhost: public registrar %s failed: %v", record.id, errRegister)
			continue
		}
		for _, item := range resp.Routes {
			method, path, okRoute := normalizePublicRoute(record.id, item)
			if !okRoute {
				log.Warnf("pluginhost: plugin %s declared invalid public route %s %s", record.id, item.Method, item.Path)
				continue
			}
			key := managementRouteKey(method, path)
			if _, exists := nextRoutes[key]; exists {
				log.Warnf("pluginhost: plugin %s public route %s conflicts with a higher-priority plugin and was skipped", record.id, key)
				continue
			}
			item.Method = method
			item.Path = path
			nextRoutes[key] = publicRouteRecord{
				pluginID: record.id,
				path:     record.path,
				version:  record.version,
				route:    item,
			}
		}
	}
	h.mu.Lock()
	h.publicRoutes = nextRoutes
	h.mu.Unlock()
}

func normalizePublicRoute(pluginID string, item pluginapi.PublicRoute) (string, string, bool) {
	if item.Handler == nil {
		return "", "", false
	}
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return "", "", false
	}
	method := strings.ToUpper(strings.TrimSpace(item.Method))
	if method == "" {
		method = http.MethodGet
	}
	if strings.ContainsAny(method, " \t\r\n") {
		return "", "", false
	}
	path := strings.TrimSpace(item.Path)
	if path == "" || path == "/" {
		return "", "", false
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if strings.HasPrefix(path, publicPluginBasePath+"/") {
		path = strings.TrimPrefix(path, publicPluginBasePath)
	}
	return method, publicPluginBasePath + "/" + pluginID + path, true
}

func (h *Host) callPublicRegistrar(ctx context.Context, record capabilityRecord, plugin pluginapi.PublicAPI) (resp pluginapi.PublicRegistrationResponse, err error) {
	if h == nil || plugin == nil || h.isPluginFused(record.id) || !h.recordCurrent(record) {
		return pluginapi.PublicRegistrationResponse{}, nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			h.fusePlugin(record.id, "PublicAPI.RegisterPublic", recovered)
			resp = pluginapi.PublicRegistrationResponse{}
			err = fmt.Errorf("public registrar panic: %v", recovered)
		}
	}()
	return plugin.RegisterPublic(ctx, pluginapi.PublicRegistrationRequest{
		Plugin:   record.meta,
		BasePath: publicPluginBasePath + "/" + record.id,
	})
}

// ServePublicHTTP dispatches a request to a plugin-owned public route.
// It reports whether the request was handled.
func (h *Host) ServePublicHTTP(w http.ResponseWriter, r *http.Request) bool {
	if h == nil || w == nil || r == nil || r.URL == nil {
		return false
	}
	key := managementRouteKey(r.Method, r.URL.Path)
	h.mu.Lock()
	record, okRoute := h.publicRoutes[key]
	h.mu.Unlock()
	if !okRoute || record.route.Handler == nil || h.isPluginFused(record.pluginID) {
		return false
	}

	var body []byte
	if r.Body != nil {
		var errRead error
		body, errRead = io.ReadAll(r.Body)
		if errRead != nil {
			http.Error(w, "failed to read plugin public request body", http.StatusBadRequest)
			return true
		}
		if errClose := r.Body.Close(); errClose != nil {
			log.Warnf("pluginhost: failed to close plugin public request body: %v", errClose)
		}
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	resp, errHandle := h.callManagementHandler(r.Context(), managementRouteRecord{
		pluginID: record.pluginID,
		path:     record.path,
		version:  record.version,
		route:    pluginapi.ManagementRoute{Handler: record.route.Handler},
	}, pluginapi.ManagementRequest{
		Method:  r.Method,
		Path:    r.URL.Path,
		Headers: cloneHeader(r.Header),
		Query:   cloneValues(r.URL.Query()),
		Body:    bytes.Clone(body),
	})
	if errHandle != nil {
		log.Warnf("pluginhost: public handler %s failed: %v", record.pluginID, errHandle)
		http.Error(w, "plugin public handler failed", http.StatusBadGateway)
		return true
	}

	for keyHeader, values := range resp.Headers {
		for _, value := range values {
			w.Header().Add(keyHeader, value)
		}
	}
	statusCode := resp.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	w.WriteHeader(statusCode)
	if _, errWrite := w.Write(resp.Body); errWrite != nil {
		log.Warnf("pluginhost: failed to write plugin public response: %v", errWrite)
	}
	return true
}
