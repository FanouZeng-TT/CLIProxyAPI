package pluginhost

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

type publicPluginDouble struct {
	routes []pluginapi.PublicRoute
}

func (p *publicPluginDouble) RegisterPublic(context.Context, pluginapi.PublicRegistrationRequest) (pluginapi.PublicRegistrationResponse, error) {
	return pluginapi.PublicRegistrationResponse{Routes: p.routes}, nil
}

func TestRegisterPublicRoutesMountsUnderPluginPrefix(t *testing.T) {
	plugin := &publicPluginDouble{routes: []pluginapi.PublicRoute{{
		Method: http.MethodPost,
		Path:   "/mcp",
		Handler: managementHandlerFunc(func(_ context.Context, req pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
			quoted, errMarshal := json.Marshal(req.Path)
			if errMarshal != nil {
				return pluginapi.ManagementResponse{}, errMarshal
			}
			return pluginapi.ManagementResponse{
				StatusCode: http.StatusOK,
				Headers:    http.Header{"Content-Type": []string{"application/json"}},
				Body:       []byte(`{"path":` + string(quoted) + `,"body":` + string(req.Body) + `}`),
			}, nil
		}),
	}}}
	host := newHostWithRecords(capabilityRecord{
		id:       "byteproxy",
		priority: 1,
		plugin:   pluginapi.Plugin{Capabilities: pluginapi.Capabilities{PublicAPI: plugin}},
	})
	host.RegisterPublicRoutes(context.Background())

	request := httptest.NewRequest(http.MethodPost, "/v1/plugins/byteproxy/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0"}`))
	recorder := httptest.NewRecorder()
	if !host.ServePublicHTTP(recorder, request) {
		t.Fatal("ServePublicHTTP() = false, want true")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("Code = %d, want 200", recorder.Code)
	}
	want := `{"path":"/v1/plugins/byteproxy/mcp","body":{"jsonrpc":"2.0"}}`
	if recorder.Body.String() != want {
		t.Fatalf("Body = %q, want %q", recorder.Body.String(), want)
	}

	recorder = httptest.NewRecorder()
	if host.ServePublicHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/plugins/byteproxy/mcp", nil)) {
		t.Fatal("method mismatch must not be handled")
	}
	recorder = httptest.NewRecorder()
	if host.ServePublicHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/plugins/other/mcp", nil)) {
		t.Fatal("foreign plugin prefix must not be handled")
	}
}
