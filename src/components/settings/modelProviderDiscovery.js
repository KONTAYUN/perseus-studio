const endpoint = (value = '') => value.trim().replace(/\/+$/, '');

export function isDiscoveredModelAdded(providers, draft, modelId) {
  return providers.some((item) => item.protocol === draft.protocol
    && endpoint(item.base_url) === endpoint(draft.base_url) && item.model_id === modelId);
}

export function discoveredModelInput(draft, model, credentialProviderId = '') {
  return {
    ...draft,
    name: model.id,
    model_id: model.id,
    // Adding from the list never replaces the current default model.
    is_default: false,
    reasoning_capability: model.reasoning_capability || 'auto',
    reasoning_capability_source: model.reasoning_capability_source || '',
    reasoning_profile_levels: model.reasoning_profile_levels || '',
    credential_provider_id: draft.api_key ? '' : credentialProviderId,
  };
}
