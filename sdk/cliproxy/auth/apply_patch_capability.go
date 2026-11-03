package auth

// ApplyPatchSupport optionally advertises the actual executor's tool contract.
// It does not change ProviderExecutor or the plugin ABI.
type ApplyPatchSupport interface {
	SupportsApplyPatch() bool
}

// ApplyPatchModelSupport optionally narrows the capability to specific model IDs.
type ApplyPatchModelSupport interface {
	SupportsApplyPatchForModel(model string) bool
}

// SupportsApplyPatchForProviders requires support from every routing candidate.
func (m *Manager) SupportsApplyPatchForProviders(providers []string) bool {
	if m == nil || len(providers) == 0 {
		return false
	}
	for _, provider := range providers {
		executor, okExecutor := m.Executor(provider)
		if !okExecutor {
			return false
		}
		support, okSupport := executor.(ApplyPatchSupport)
		if !okSupport || !support.SupportsApplyPatch() {
			return false
		}
	}
	return true
}

// SupportsApplyPatchForModel requires support from every routing candidate for model.
func (m *Manager) SupportsApplyPatchForModel(providers []string, model string) bool {
	if m == nil || len(providers) == 0 {
		return false
	}
	for _, provider := range providers {
		executor, okExecutor := m.Executor(provider)
		if !okExecutor {
			return false
		}
		if support, okSupport := executor.(ApplyPatchModelSupport); okSupport {
			if !support.SupportsApplyPatchForModel(model) {
				return false
			}
			continue
		}
		support, okSupport := executor.(ApplyPatchSupport)
		if !okSupport || !support.SupportsApplyPatch() {
			return false
		}
	}
	return true
}
