import { useEffect, useMemo, useState } from 'react';
import { apiDelete, apiGet, apiPatch, apiPost } from '../api/client';

export default function useSettingsData({ enabled = true } = {}) {
  const [modelProviders, setModelProviders] = useState([]);
  const [searchProviders, setSearchProviders] = useState([]);
  const [selectedModelId, setSelectedModelId] = useState('');
  const [isModelMenuOpen, setIsModelMenuOpen] = useState(false);
  const [settingsError, setSettingsError] = useState('');

  const enabledModels = useMemo(
    () => modelProviders.filter((item) => item.enabled),
    [modelProviders]
  );
  const selectedModel =
    enabledModels.find((item) => item.id === selectedModelId) ||
    enabledModels.find((item) => item.is_default) ||
    enabledModels[0] ||
    null;

  useEffect(() => {
    if (!enabled) return;
    loadModelProviders();
    loadSearchProviders();
  }, [enabled]);

  useEffect(() => {
    if (selectedModelId && enabledModels.some((item) => item.id === selectedModelId)) return;
    const nextModel = enabledModels.find((item) => item.is_default) || enabledModels[0];
    setSelectedModelId(nextModel?.id || '');
  }, [enabledModels, selectedModelId]);

  async function loadModelProviders({ report = true } = {}) {
    if (report) setSettingsError('');
    try {
      const data = await apiGet('/api/model-providers');
      setModelProviders(data.items || []);
      return {};
    } catch (error) {
      console.error('loadModelProviders failed', error);
      if (report) setSettingsError(error.message);
      return { refreshError: error.message };
    }
  }

  async function saveModelProvider(input, id = '') {
    setSettingsError('');
    try {
      if (id) {
        await apiPatch(`/api/model-providers/${encodeURIComponent(id)}`, input);
      } else {
        await apiPost('/api/model-providers', input);
      }
      return await loadModelProviders({ report: false });
    } catch (error) {
      console.error('saveModelProvider failed', error);
      throw error;
    }
  }

  async function deleteModelProvider(id) {
    setSettingsError('');
    try {
      await apiDelete(`/api/model-providers/${encodeURIComponent(id)}`);
      return await loadModelProviders({ report: false });
    } catch (error) {
      console.error('deleteModelProvider failed', error);
      throw error;
    }
  }

  async function setDefaultModelProvider(id) {
    setSettingsError('');
    try {
      await apiPost(`/api/model-providers/${encodeURIComponent(id)}/set-default`, {});
      return await loadModelProviders({ report: false });
    } catch (error) {
      console.error('setDefaultModelProvider failed', error);
      throw error;
    }
  }

  async function loadSearchProviders({ report = true } = {}) {
    if (report) setSettingsError('');
    try {
      const data = await apiGet('/api/search-providers');
      setSearchProviders(data.items || []);
      return {};
    } catch (error) {
      console.error('loadSearchProviders failed', error);
      if (report) setSettingsError(error.message);
      return { refreshError: error.message };
    }
  }

  async function saveSearchProvider(input, id = '') {
    setSettingsError('');
    try {
      if (id) {
        await apiPatch(`/api/search-providers/${encodeURIComponent(id)}`, input);
      } else {
        await apiPost('/api/search-providers', input);
      }
      return await loadSearchProviders({ report: false });
    } catch (error) {
      console.error('saveSearchProvider failed', error);
      throw error;
    }
  }

  async function deleteSearchProvider(id) {
    setSettingsError('');
    try {
      await apiDelete(`/api/search-providers/${encodeURIComponent(id)}`);
      return await loadSearchProviders({ report: false });
    } catch (error) {
      console.error('deleteSearchProvider failed', error);
      throw error;
    }
  }

  return {
    deleteModelProvider,
    deleteSearchProvider,
    enabledModels,
    isModelMenuOpen,
    loadModelProviders,
    loadSearchProviders,
    modelProviders,
    saveModelProvider,
    saveSearchProvider,
    searchProviders,
    selectedModel,
    selectedModelId,
    setDefaultModelProvider,
    setIsModelMenuOpen,
    setSelectedModelId,
    settingsError,
    setSettingsError,
  };
}
