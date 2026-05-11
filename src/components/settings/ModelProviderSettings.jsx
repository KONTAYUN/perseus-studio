import React from 'react';
import { Check, FlaskConical, KeyRound, Loader2, Pencil, Plus, RefreshCw, Star, Trash2 } from 'lucide-react';
import { apiPost } from '../../api/client';
import {
  Badge,
  CardActions,
  Checkbox,
  EmptyCard,
  Field,
  FormHeader,
  IconButton,
  Input,
  ProviderCard,
  SectionTitle,
  Select,
} from './SettingsUI';

const emptyModel = {
  name: '',
  protocol: 'openai',
  base_url: '',
  api_key: '',
  model_id: '',
  enabled: true,
  is_default: false,
};

export default function ModelProviderSettings({ ide }) {
  const [draft, setDraft] = React.useState(emptyModel);
  const [editingId, setEditingId] = React.useState('');
  const [discoveredModels, setDiscoveredModels] = React.useState([]);
  const [discoverError, setDiscoverError] = React.useState('');
  const [discoverInfo, setDiscoverInfo] = React.useState('');
  const [isDiscovering, setIsDiscovering] = React.useState(false);
  const [testReport, setTestReport] = React.useState(null);
  const [testError, setTestError] = React.useState('');
  const [isTesting, setIsTesting] = React.useState(false);

  const resetDraft = () => {
    setEditingId('');
    setDraft(emptyModel);
    setDiscoveredModels([]);
    setDiscoverError('');
    setDiscoverInfo('');
    setTestReport(null);
    setTestError('');
  };

  const editModel = (item) => {
    setEditingId(item.id);
    setDraft({
      name: item.name,
      protocol: item.protocol,
      base_url: item.base_url,
      api_key: '',
      model_id: item.model_id,
      enabled: item.enabled,
      is_default: item.is_default,
    });
    setDiscoveredModels([]);
    setDiscoverError('');
    setDiscoverInfo('');
    setTestReport(null);
    setTestError('');
  };

  const submitModel = async (event) => {
    event.preventDefault();
    await ide.saveModelProvider(draft, editingId);
    resetDraft();
  };

  const discoverModels = async () => {
    setIsDiscovering(true);
    setDiscoverError('');
    setDiscoverInfo('');
    setDiscoveredModels([]);
    try {
      const result = await apiPost('/api/model-providers/discover', {
        provider_id: editingId,
        protocol: draft.protocol,
        base_url: draft.base_url,
        api_key: draft.api_key,
      });
      const models = Array.isArray(result?.models) ? result.models : [];
      setDiscoveredModels(models);
      setDiscoverInfo(models.length ? `已获取 ${models.length} 个模型` : '没有获取到模型');
    } catch (error) {
      setDiscoverError(error?.message || '获取模型列表失败');
    } finally {
      setIsDiscovering(false);
    }
  };

  const chooseDiscoveredModel = (model) => {
    const modelId = model?.id || '';
    if (!modelId) return;
    setDraft((current) => ({
      ...current,
      model_id: modelId,
      name: current.name || modelId,
    }));
  };

  const canDiscoverModels = Boolean(draft.base_url && (draft.api_key || editingId));
  const canTestProvider = Boolean(draft.base_url && draft.model_id && (draft.api_key || editingId));

  const testProvider = async () => {
    setIsTesting(true);
    setTestError('');
    setTestReport(null);
    try {
      const result = await apiPost('/api/model-providers/test', {
        provider_id: editingId,
        protocol: draft.protocol,
        base_url: draft.base_url,
        api_key: draft.api_key,
        model_id: draft.model_id,
      });
      setTestReport(result);
    } catch (error) {
      setTestError(error?.message || '测试失败');
    } finally {
      setIsTesting(false);
    }
  };

  return (
    <section className="settings-section-appear rounded-[24px] border border-gray-100 bg-white/55 p-5 dark:border-white/10 dark:bg-white/[0.03]">
      <SectionTitle icon={<KeyRound size={15} />} title="模型供应商" description="管理聊天窗口可选择的模型配置。" />
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_380px]">
        <div className="min-w-0 space-y-3">
          {ide.modelProviders.length === 0 ? (
            <EmptyCard text="还没有模型配置。添加一个 OpenAI-compatible 或 Anthropic 模型即可在聊天顶部选择。" />
          ) : (
            ide.modelProviders.map((item) => (
              <ProviderCard key={item.id}>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-bold text-gray-800 dark:text-gray-100">{item.name}</span>
                    {item.is_default && <Badge tone="blue">默认</Badge>}
                    <Badge tone={item.enabled ? 'green' : 'gray'}>{item.enabled ? '启用' : '禁用'}</Badge>
                  </div>
                  <div className="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">
                    {item.protocol} · {item.model_id} · {item.base_url}
                  </div>
                  <div className="mt-1 text-xs text-gray-400">Key: {item.masked_key || '未设置'}</div>
                </div>
                <CardActions>
                  <IconButton title="编辑" onClick={() => editModel(item)}>
                    <Pencil size={14} />
                  </IconButton>
                  <IconButton
                    title="设为默认"
                    onClick={() => ide.setDefaultModelProvider(item.id)}
                    disabled={item.is_default}
                  >
                    <Star size={14} />
                  </IconButton>
                  <IconButton title="删除" danger onClick={() => ide.deleteModelProvider(item.id)}>
                    <Trash2 size={14} />
                  </IconButton>
                </CardActions>
              </ProviderCard>
            ))
          )}
        </div>

        <form onSubmit={submitModel} className="settings-card rounded-3xl border border-gray-100 bg-gray-50/90 p-5 dark:border-white/10 dark:bg-[#252526]/80">
          <FormHeader title={editingId ? '编辑模型' : '新增模型'} onReset={resetDraft} />
          <Field label="自定义名称">
            <Input
              data-testid="model-name-input"
              value={draft.name}
              onChange={(e) => setDraft({ ...draft, name: e.target.value })}
              placeholder="OpenRouter GPT"
              required
            />
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label="协议">
              <Select value={draft.protocol} onChange={(e) => setDraft({ ...draft, protocol: e.target.value })}>
                <option value="openai">openai</option>
                <option value="anthropic">anthropic</option>
              </Select>
            </Field>
            <Field label="Model ID">
              <Input
                data-testid="model-id-input"
                value={draft.model_id}
                onChange={(e) => setDraft({ ...draft, model_id: e.target.value })}
                placeholder="gpt-4.1"
                required
              />
            </Field>
          </div>
          <Field label="Base URL">
            <Input
              data-testid="model-base-url-input"
              value={draft.base_url}
              onChange={(e) => setDraft({ ...draft, base_url: e.target.value })}
              placeholder="https://api.openai.com/v1"
              required
            />
          </Field>
          <Field label={editingId ? 'API Key（留空保留旧 key）' : 'API Key'}>
            <Input
              data-testid="model-api-key-input"
              type="password"
              value={draft.api_key}
              onChange={(e) => setDraft({ ...draft, api_key: e.target.value })}
              placeholder="sk-..."
              required={!editingId}
            />
          </Field>
          <div className="mt-3 rounded-2xl border border-gray-200 bg-white/70 p-3 dark:border-white/10 dark:bg-white/[0.03]">
            <div className="flex items-center justify-between gap-3">
              <div className="min-w-0">
                <div className="text-xs font-bold text-gray-600 dark:text-gray-300">自动获取模型列表</div>
                <div className="mt-1 text-[11px] leading-4 text-gray-400 dark:text-gray-500">
                  支持 OpenAI-compatible 的 <span className="font-mono">/models</span> 接口。编辑已有配置时，API Key 留空也会使用已保存的 key。
                </div>
              </div>
              <div className="flex shrink-0 gap-2">
                <button
                  type="button"
                  onClick={discoverModels}
                  disabled={!canDiscoverModels || isDiscovering}
                  className="inline-flex items-center gap-1.5 rounded-xl border border-gray-200 bg-white px-3 py-2 text-xs font-bold text-gray-700 transition-colors hover:border-blue-300 hover:text-blue-600 disabled:cursor-not-allowed disabled:opacity-50 dark:border-white/10 dark:bg-[#1C1C1E] dark:text-gray-200 dark:hover:border-blue-500/40 dark:hover:text-blue-200"
                >
                  {isDiscovering ? <Loader2 size={13} className="animate-spin" /> : <RefreshCw size={13} />}
                  获取
                </button>
                <button
                  type="button"
                  onClick={testProvider}
                  disabled={!canTestProvider || isTesting}
                  className="inline-flex items-center gap-1.5 rounded-xl border border-blue-500/25 bg-blue-500/10 px-3 py-2 text-xs font-bold text-blue-600 transition-colors hover:border-blue-400 hover:bg-blue-500/15 disabled:cursor-not-allowed disabled:opacity-50 dark:text-blue-200"
                >
                  {isTesting ? <Loader2 size={13} className="animate-spin" /> : <FlaskConical size={13} />}
                  测试
                </button>
              </div>
            </div>
            {discoverError && (
              <div className="settings-expand-enter mt-3 rounded-xl border border-red-500/20 bg-red-500/10 px-3 py-2 text-xs leading-5 text-red-600 dark:text-red-200">
                {discoverError}
              </div>
            )}
            {!discoverError && discoverInfo && (
              <div className="settings-expand-enter mt-3 rounded-xl border border-blue-500/20 bg-blue-500/10 px-3 py-2 text-xs text-blue-600 dark:text-blue-200">
                {discoverInfo}
              </div>
            )}
            {discoveredModels.length > 0 && (
              <div className="settings-expand-enter mt-3 max-h-52 space-y-1 overflow-y-auto pr-1">
                {discoveredModels.map((model) => {
                  const selected = model.id === draft.model_id;
                  return (
                    <button
                      key={model.id}
                      type="button"
                      onClick={() => chooseDiscoveredModel(model)}
                      className={`flex w-full items-center justify-between gap-3 rounded-xl px-3 py-2 text-left text-xs transition-colors ${
                        selected
                          ? 'bg-blue-500 text-white'
                          : 'bg-gray-100 text-gray-700 hover:bg-blue-50 hover:text-blue-600 dark:bg-white/[0.04] dark:text-gray-200 dark:hover:bg-blue-500/15 dark:hover:text-blue-200'
                      }`}
                    >
                      <span className="min-w-0 truncate font-mono">{model.id}</span>
                      {selected && <Check size={13} className="shrink-0" />}
                    </button>
                  );
                })}
              </div>
            )}
            {testError && (
              <div className="settings-expand-enter mt-3 rounded-xl border border-red-500/20 bg-red-500/10 px-3 py-2 text-xs leading-5 text-red-600 dark:text-red-200">
                {testError}
              </div>
            )}
            {testReport && <ProviderTestReport report={testReport} />}
          </div>
          <div className="mt-3 flex flex-wrap gap-3">
            <Checkbox label="启用" checked={draft.enabled} onChange={(value) => setDraft({ ...draft, enabled: value })} />
            <Checkbox label="设为默认" checked={draft.is_default} onChange={(value) => setDraft({ ...draft, is_default: value })} />
          </div>
          <button
            data-testid="model-save-button"
            className="mt-5 flex w-full items-center justify-center gap-2 rounded-2xl bg-blue-500 px-4 py-2.5 text-sm font-bold text-white shadow-lg shadow-blue-500/20 transition-all hover:bg-blue-600 active:scale-[0.98]"
          >
            <Plus size={15} /> {editingId ? '保存模型' : '添加模型'}
          </button>
        </form>
      </div>
    </section>
  );
}

function ProviderTestReport({ report }) {
  const tone =
    report.rating === 'agent_ready'
      ? 'border-emerald-500/20 bg-emerald-500/10 text-emerald-700 dark:text-emerald-200'
      : report.rating === 'usable'
        ? 'border-blue-500/20 bg-blue-500/10 text-blue-700 dark:text-blue-200'
        : report.rating === 'chat_only'
          ? 'border-amber-500/25 bg-amber-500/10 text-amber-700 dark:text-amber-200'
          : 'border-red-500/20 bg-red-500/10 text-red-700 dark:text-red-200';
  return (
    <div className={`settings-expand-enter mt-3 rounded-xl border px-3 py-2 text-xs ${tone}`}>
      <div className="flex items-center justify-between gap-3">
        <span className="font-bold">{report.summary || '测试完成'}</span>
        <span className="text-[11px] opacity-70">{durationLabel(report.results)}</span>
      </div>
      <div className="mt-2 grid gap-1.5">
        {(report.results || []).map((item) => (
          <div key={item.name} className="flex min-w-0 items-start justify-between gap-3 rounded-lg bg-white/45 px-2 py-1.5 dark:bg-black/10">
            <div className="min-w-0">
              <div className="font-semibold">{item.label}</div>
              {item.error && <div className="mt-0.5 break-words text-[11px] opacity-80">{item.error}</div>}
            </div>
            <div className="shrink-0 text-right text-[11px] font-bold">
              {item.status === 'passed' ? '通过' : '失败'}
              <div className="font-normal opacity-70">{item.duration_ms || 0}ms</div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function durationLabel(results = []) {
  const total = results.reduce((sum, item) => sum + (Number(item.duration_ms) || 0), 0);
  return total ? `${total}ms` : '';
}
