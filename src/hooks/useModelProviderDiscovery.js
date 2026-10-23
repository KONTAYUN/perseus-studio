import { useCallback, useEffect, useRef, useState } from 'react';
import { apiPost } from '../api/client';
import { discoveredModelInput, isDiscoveredModelAdded } from '../components/settings/modelProviderDiscovery';

export default function useModelProviderDiscovery({ draft, editingId, modelProviders, saveModelProvider }) {
  const [discoveredModels, setDiscoveredModels] = useState([]);
  const [discoverError, setDiscoverError] = useState('');
  const [discoverInfo, setDiscoverInfo] = useState('');
  const [isDiscovering, setIsDiscovering] = useState(false);
  const [addingModelId, setAddingModelId] = useState('');
  const [addedModelIds, setAddedModelIds] = useState([]);
  const [addFeedback, setAddFeedback] = useState(null);
  const [testReport, setTestReport] = useState(null);
  const [testError, setTestError] = useState('');
  const [isTesting, setIsTesting] = useState(false);
  const discoveryRequest = useRef(null);
  const testRequest = useRef(null);
  const adding = useRef(false);
  const connectionKey = JSON.stringify([editingId, draft.protocol, draft.base_url, draft.api_key]);
  const connection = useRef(connectionKey);
  connection.current = connectionKey;
  const testKey = JSON.stringify([connectionKey, draft.model_id]);
  const testedModel = useRef(testKey);
  testedModel.current = testKey;

  const clearDiscoveryState = useCallback(() => {
    discoveryRequest.current?.abort();
    discoveryRequest.current = null;
    setIsDiscovering(false);
    setDiscoveredModels([]);
    setDiscoverError('');
    setDiscoverInfo('');
    setAddedModelIds([]);
    setAddFeedback(null);
  }, []);
  const clearTestState = useCallback(() => {
    testRequest.current?.abort();
    testRequest.current = null;
    setIsTesting(false);
    setTestReport(null);
    setTestError('');
  }, []);

  useEffect(() => {
    clearDiscoveryState();
    clearTestState();
    return () => {
      discoveryRequest.current?.abort();
      testRequest.current?.abort();
    };
  }, [connectionKey, clearDiscoveryState, clearTestState]);
  useEffect(() => { clearTestState(); }, [testKey, clearTestState]);

  const isModelAdded = (id) => addedModelIds.includes(id) || isDiscoveredModelAdded(modelProviders, draft, id);
  const discoverModels = async () => {
    if (discoveryRequest.current) return;
    const controller = new AbortController();
    discoveryRequest.current = controller;
    const key = connectionKey;
    setIsDiscovering(true);
    setDiscoverError('');
    setDiscoverInfo('');
    setAddFeedback(null);
    try {
      const result = await apiPost('/api/model-providers/discover', {
        provider_id: editingId, protocol: draft.protocol, base_url: draft.base_url, api_key: draft.api_key,
      }, { signal: controller.signal });
      if (controller.signal.aborted || connection.current !== key) return;
      const models = Array.isArray(result?.models) ? result.models : [];
      setDiscoveredModels(models);
      setDiscoverInfo(models.length ? `已获取 ${models.length} 个模型` : '没有获取到模型');
    } catch (error) {
      if (!controller.signal.aborted && connection.current === key) setDiscoverError(error?.message || '获取模型列表失败');
    } finally {
      if (discoveryRequest.current === controller) {
        discoveryRequest.current = null;
        setIsDiscovering(false);
      }
    }
  };

  const addDiscoveredModel = async (model) => {
    if (!model?.id || adding.current || isModelAdded(model.id)) return;
    const key = connectionKey;
    adding.current = true;
    setAddingModelId(model.id);
    setAddFeedback(null);
    try {
      const result = await saveModelProvider(discoveredModelInput(draft, model, editingId));
      if (connection.current !== key) return;
      setAddedModelIds((current) => [...current, model.id]);
      setAddFeedback({ message: result?.refreshError ? `已添加 ${model.id}，但已配置列表刷新失败。请稍后刷新，无需重复添加。` : `已添加 ${model.id}`, error: false });
    } catch (error) {
      if (connection.current === key) setAddFeedback({ message: `${model.id} 添加失败：${error?.message || '请重试'}`, error: true });
    } finally {
      adding.current = false;
      setAddingModelId('');
    }
  };

  const testProvider = async () => {
    if (testRequest.current) return;
    const controller = new AbortController();
    testRequest.current = controller;
    const key = testKey;
    setIsTesting(true);
    setTestError('');
    setTestReport(null);
    try {
      const result = await apiPost('/api/model-providers/test', {
        provider_id: editingId, protocol: draft.protocol, base_url: draft.base_url,
        api_key: draft.api_key, model_id: draft.model_id,
      }, { signal: controller.signal });
      if (!controller.signal.aborted && testedModel.current === key) setTestReport(result);
    } catch (error) {
      if (!controller.signal.aborted && testedModel.current === key) setTestError(error?.message || '测试失败');
    } finally {
      if (testRequest.current === controller) {
        testRequest.current = null;
        setIsTesting(false);
      }
    }
  };

  return {
    discoveredModels, discoverError, discoverInfo, isDiscovering, discoverModels, clearDiscoveryState,
    addDiscoveredModel, addingModelId, isModelAdded, addFeedback,
    testReport, testError, isTesting, testProvider, clearTestState,
  };
}
