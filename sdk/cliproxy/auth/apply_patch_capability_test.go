package auth

import "testing"

type patchSupportExecutor struct {
	*replaceAwareExecutor
	supported bool
}

func (e *patchSupportExecutor) SupportsApplyPatch() bool { return e.supported }

type patchModelSupportExecutor struct {
	*replaceAwareExecutor
	prefix string
}

func (e *patchModelSupportExecutor) SupportsApplyPatch() bool { return true }

func (e *patchModelSupportExecutor) SupportsApplyPatchForModel(model string) bool {
	return len(model) >= len(e.prefix) && model[:len(e.prefix)] == e.prefix
}

func TestApplyPatchManagerAllCandidates(t *testing.T) {
	m := NewManager(nil, nil, nil)
	m.RegisterExecutor(&patchSupportExecutor{&replaceAwareExecutor{id: "custom-name"}, true})
	m.RegisterExecutor(&patchSupportExecutor{&replaceAwareExecutor{id: "denied"}, false})
	m.RegisterExecutor(&replaceAwareExecutor{id: "codex"})
	for _, tc := range []struct {
		providers []string
		want      bool
	}{
		{nil, false}, {[]string{"unknown"}, false}, {[]string{"codex"}, false},
		{[]string{"custom-name"}, true}, {[]string{"custom-name", "custom-name"}, true},
		{[]string{"custom-name", "denied"}, false}, {[]string{"custom-name", "unknown"}, false},
		{[]string{"custom-name", "codex"}, false},
	} {
		if got := m.SupportsApplyPatchForProviders(tc.providers); got != tc.want {
			t.Errorf("providers %v: got %v, want %v", tc.providers, got, tc.want)
		}
	}
	var absent *Manager
	if absent.SupportsApplyPatchForProviders([]string{"custom-name"}) {
		t.Fatal("nil manager has capability")
	}
}

func TestApplyPatchManagerModelPrefixes(t *testing.T) {
	m := NewManager(nil, nil, nil)
	m.RegisterExecutor(&patchModelSupportExecutor{&replaceAwareExecutor{id: "prefixed"}, "llmbox_"})
	m.RegisterExecutor(&patchSupportExecutor{&replaceAwareExecutor{id: "all"}, true})

	for _, tc := range []struct {
		providers []string
		model     string
		want      bool
	}{
		{[]string{"prefixed"}, "llmbox_deepseek-v4.1-flash", true},
		{[]string{"prefixed", "all"}, "llmbox_gpt-5.6-terra", true},
		{[]string{"prefixed"}, "trae_gpt-5.6-sol", false},
		{[]string{"prefixed", "all"}, "trae_gpt-5.6-sol", false},
		{[]string{"prefixed"}, "", false},
		{nil, "llmbox_x", false},
		{[]string{"unknown"}, "llmbox_x", false},
	} {
		if got := m.SupportsApplyPatchForModel(tc.providers, tc.model); got != tc.want {
			t.Errorf("providers %v model %q: got %v, want %v", tc.providers, tc.model, got, tc.want)
		}
	}
}
