package pluginhost

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func TestHostModelsRefreshAcknowledgesRegisteredRevision(t *testing.T) {
	calls := 0
	revision := "first"
	fail := false
	host := newHostWithRecords(capabilityRecord{
		id: "catalog",
		plugin: pluginapi.Plugin{Capabilities: pluginapi.Capabilities{
			ModelProvider: modelProviderFunc{
				staticModels: func(_ context.Context, req pluginapi.StaticModelRequest) (pluginapi.ModelResponse, error) {
					calls++
					if !req.ModelCatalogRefresh {
						t.Error("host did not advertise catalog refresh")
					}
					if fail {
						return pluginapi.ModelResponse{}, errors.New("catalog unavailable")
					}
					return pluginapi.ModelResponse{CatalogRevision: revision,
						Provider: "catalog", Models: []pluginapi.ModelInfo{{ID: revision}}}, nil
				},
			},
		}},
	})
	modelRegistry := newFakeModelRegistry()
	host.SetModelCatalogRefresh(func(ctx context.Context, id string) (string, error) {
		if !host.RegisterModels(ctx, modelRegistry) {
			return "", errors.New("registration failed")
		}
		return host.ModelCatalogRevision(id), nil
	})
	ctx := withHostCallbackPluginID(t.Context(), "catalog")
	assertRevision := func(want string) {
		t.Helper()
		raw, err := host.callFromPlugin(ctx, pluginabi.MethodHostModelsRefresh, nil)
		if err != nil {
			t.Fatal(err)
		}
		var env pluginabi.Envelope
		if err := json.Unmarshal(raw, &env); err != nil || !env.OK {
			t.Fatalf("bad acknowledgement: %s %v", raw, err)
		}
		var result struct {
			Revision string `json:"catalog_revision"`
		}
		if err := json.Unmarshal(env.Result, &result); err != nil || result.Revision != want {
			t.Fatalf("revision=%q, want %q: %v", result.Revision, want, err)
		}
	}
	assertRevision("first")
	revision = "second"
	assertRevision("second")
	if calls != 2 || host.modelRegistration("catalog").models[0].ID != "second" {
		t.Fatal("refresh did not replace model definitions")
	}
	fail = true
	if _, err := host.callFromPlugin(ctx, pluginabi.MethodHostModelsRefresh, nil); err == nil {
		t.Fatal("failed registration was acknowledged")
	}
	if host.ModelCatalogRevision("catalog") != "second" || host.modelRegistration("catalog").models[0].ID != "second" {
		t.Fatal("failed registration destroyed the previous snapshot")
	}
	fail = false
	revision = "third"
	assertRevision("third")
}

func TestHostModelsRefreshRequiresActiveInstanceAndService(t *testing.T) {
	host := New()
	ctx := withHostCallbackPluginID(t.Context(), "missing")
	if _, err := host.callFromPlugin(ctx, pluginabi.MethodHostModelsRefresh, nil); err == nil {
		t.Fatal("missing refresh service was accepted")
	}
	calls := 0
	host.SetModelCatalogRefresh(func(context.Context, string) (string, error) {
		calls++
		return "revision", nil
	})
	for _, caller := range []context.Context{t.Context(), ctx} {
		if _, err := host.callFromPlugin(caller, pluginabi.MethodHostModelsRefresh, nil); err == nil {
			t.Fatal("unidentified or inactive caller was accepted")
		}
	}
	instance := &hostCallbackInstance{}
	instance.closed.Store(true)
	if _, err := host.callFromPlugin(withHostCallbackIdentity(t.Context(), "missing", instance),
		pluginabi.MethodHostModelsRefresh, nil); err == nil {
		t.Fatal("retired plugin instance was accepted")
	}
	if calls != 0 {
		t.Fatalf("invalid callers invoked refresh %d times", calls)
	}
}

func TestRegisterModelsCanceledCatalogKeepsPreviousSnapshot(t *testing.T) {
	cancelRead := false
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	host := newHostWithRecords(capabilityRecord{
		id: "catalog",
		plugin: pluginapi.Plugin{Capabilities: pluginapi.Capabilities{
			ModelProvider: modelProviderFunc{
				staticModels: func(context.Context, pluginapi.StaticModelRequest) (pluginapi.ModelResponse, error) {
					id := "old"
					if cancelRead {
						id = "new"
						cancel()
					}
					return pluginapi.ModelResponse{Provider: "catalog", Models: []pluginapi.ModelInfo{{ID: id}}}, nil
				},
			},
		}},
	})
	reg := newFakeModelRegistry()
	if !host.RegisterModels(ctx, reg) {
		t.Fatal("initial registration failed")
	}
	cancelRead = true
	if host.RegisterModels(ctx, reg) {
		t.Fatal("canceled registration reported success")
	}
	if host.modelRegistration("catalog").models[0].ID != "old" {
		t.Fatal("canceled catalog replaced the old snapshot")
	}
}
