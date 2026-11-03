package cliproxy

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/pluginhost"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/config"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

type changingStaticCatalog struct {
	mu       sync.Mutex
	revision string
	fail     bool
}

func (p *changingStaticCatalog) StaticModels(_ context.Context, req pluginapi.StaticModelRequest) (pluginapi.ModelResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fail {
		return pluginapi.ModelResponse{}, errors.New("temporary catalog failure")
	}
	return pluginapi.ModelResponse{
		CatalogRevision: p.revision, Provider: "catalog-refresh",
		Models: []pluginapi.ModelInfo{{ID: p.revision, ContextLength: 350000}},
	}, nil
}

func (*changingStaticCatalog) ModelsForAuth(context.Context, pluginapi.AuthModelRequest) (pluginapi.ModelResponse, error) {
	return pluginapi.ModelResponse{}, nil
}

func TestServiceCatalogRefreshUpdatesClientModelsWithoutPluginReload(t *testing.T) {
	host := pluginhost.New()
	provider := &changingStaticCatalog{revision: "catalog-old"}
	host.RegisterPluginForTest(t.Name(), pluginapi.Plugin{Capabilities: pluginapi.Capabilities{
		ModelProvider:      provider,
		Executor:           &issue6718ModelProvider{provider: "catalog-refresh"},
		ExecutorModelScope: pluginapi.ExecutorModelScopeStatic,
	}})
	manager := coreauth.NewManager(nil, nil, nil)
	clientID := "plugin:" + t.Name() + ":catalog-refresh:executor"
	reg := registry.GetGlobalRegistry()
	t.Cleanup(func() { reg.UnregisterClient(clientID) })
	s := &Service{cfg: &config.Config{}, coreManager: manager, pluginHost: host}
	assertModel := func(want string) {
		t.Helper()
		models := reg.GetModelsForClient(clientID)
		if len(models) != 1 || models[0].ID != want || models[0].ContextLength != 350000 {
			t.Fatalf("client catalog=%v, want %s with context 350000", models, want)
		}
	}
	if revision, err := s.refreshPluginModelCatalog(t.Context(), t.Name()); err != nil || revision != "catalog-old" {
		t.Fatalf("initial refresh=%q %v", revision, err)
	}
	assertModel("catalog-old")
	before := host.Snapshot()
	provider.revision = "catalog-new"
	if revision, err := s.refreshPluginModelCatalog(t.Context(), t.Name()); err != nil || revision != "catalog-new" {
		t.Fatalf("refresh=%q %v", revision, err)
	}
	assertModel("catalog-new")
	if host.Snapshot() != before {
		t.Fatal("model-only update reconfigured or reloaded the plugin")
	}
	provider.fail = true
	if _, err := s.refreshPluginModelCatalog(t.Context(), t.Name()); err == nil {
		t.Fatal("temporary failure was acknowledged")
	}
	assertModel("catalog-new")
	provider.fail, provider.revision = false, "catalog-recovered"
	if _, err := s.refreshPluginModelCatalog(t.Context(), t.Name()); err != nil {
		t.Fatal(err)
	}
	assertModel("catalog-recovered")
}
