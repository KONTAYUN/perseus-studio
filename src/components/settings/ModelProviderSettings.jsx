import React from 'react';
import { ChevronDown, KeyRound, Pencil, Plus, SlidersHorizontal, Star, Trash2 } from 'lucide-react';
import { apiPost } from '../../api/client';
import {
  Badge,
  Button,
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
  SettingsSection,
} from './SettingsUI';
import {
  CONTEXT_LENGTH_PRESETS,
  DEFAULT_CONTEXT_PRESET_ID,
  activeContextPresetId,
  applyContextPreset,
  contextTemplateSummary,
  formatContextTokens,
  getContextPreset,
  isRecommendedContextTemplate,
} from './modelContextPresets';
import {
  ModelProviderReasoningField,
  modelReasoningDraft,
  reasoningCapabilityLabel,
  reasoningSupportLabel,
} from './ModelProviderReasoningControls';
import ModelProviderDiscoveryPanel from './ModelProviderDiscoveryPanel';

const emptyModel = applyContextPreset({
  name: '',
  visibility: 'private',
  protocol: 'openai',
  base_url: '',
  api_key: '',
  model_id: '',
  enabled: true,
  is_default: false,
  reasoning_capability: 'auto',
  native_patch_tool_enabled: false,
}, DEFAULT_CONTEXT_PRESET_ID);

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
  const [showContextAdvanced, setShowContextAdvanced] = React.useState(false);
  const clearDiscoveryState = () => {
    setDiscoveredModels([]);
    setDiscoverError('');
    setDiscoverInfo('');
  };
  const clearTestState = () => {
    setTestReport(null);
    setTestError('');
  };

  const resetDraft = () => {
    setEditingId('');
    setDraft(emptyModel);
    clearDiscoveryState();
    clearTestState();
    setShowContextAdvanced(false);
  };

  const editModel = (item) => {
    setEditingId(item.id);
    setDraft({
      name: item.name,
      visibility: item.visibility || 'private',
      protocol: item.protocol,
      base_url: item.base_url,
      api_key: '',
      model_id: item.model_id,
      enabled: item.enabled,
      is_default: item.is_default,
      ...modelReasoningDraft(item),
      context_window_tokens: item.context_window_tokens || 0,
      auto_compact_enabled: item.auto_compact_enabled !== false,
      auto_compact_threshold_percent: item.auto_compact_threshold_percent || 80,
      compact_reserved_tokens: item.compact_reserved_tokens || 0,
      prune_tool_outputs_enabled: item.prune_tool_outputs_enabled !== false,
      native_patch_tool_enabled: Boolean(item.native_patch_tool_enabled),
    });
    clearDiscoveryState();
    clearTestState();
    setShowContextAdvanced(false);
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
      ...modelReasoningDraft(model),
    }));
  };

  const canDiscoverModels = Boolean(draft.base_url && (draft.api_key || editingId));
  const canTestProvider = Boolean(draft.base_url && draft.model_id && (draft.api_key || editingId));
  const contextPresetId = activeContextPresetId(draft);
  const contextUsesRecommendedTemplate = isRecommendedContextTemplate(draft);
  const selectedContextPreset = contextPresetId === 'custom'
    ? { label: formatContextTokens(draft.context_window_tokens), caption: '自定义' }
    : getContextPreset(contextPresetId);

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
    <SettingsSection>
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
                    <Badge tone={item.visibility === 'public' ? 'blue' : 'gray'}>{item.visibility === 'public' ? '公开' : '私密'}</Badge>
                  </div>
                  <div className="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">
                    {item.protocol} · {item.model_id} · {item.base_url}
                  </div>
                  <div className="mt-1 text-xs text-gray-400">Key: {item.masked_key || '未设置'}</div>
                  <div className="mt-1 text-xs text-gray-400">思考能力: {reasoningCapabilityLabel(item.reasoning_capability, item.reasoning_capability_source)} · {reasoningSupportLabel(item.thinking_profile)}</div>
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
          <Field label="可见性">
            <Select value={draft.visibility || 'private'} onChange={(e) => setDraft({ ...draft, visibility: e.target.value })}>
              <option value="private">私密（仅管理员）</option>
              <option value="public">公开（所有用户）</option>
            </Select>
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
          <ModelProviderReasoningField
            value={draft.reasoning_capability}
            onChange={(value) => setDraft({
              ...draft,
              reasoning_capability: value,
              reasoning_capability_source: value === 'auto' ? '' : 'explicit_override',
              reasoning_profile_levels: value === 'auto' ? '' : '',
            })}
          />
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
          <ModelProviderDiscoveryPanel
            canDiscoverModels={canDiscoverModels}
            canTestProvider={canTestProvider}
            discoverError={discoverError}
            discoverInfo={discoverInfo}
            discoveredModels={discoveredModels}
            draft={draft}
            isDiscovering={isDiscovering}
            isTesting={isTesting}
            onChooseDiscoveredModel={chooseDiscoveredModel}
            onDiscoverModels={discoverModels}
            onTestProvider={testProvider}
            testError={testError}
            testReport={testReport}
          />
          <div className="mt-3 flex flex-wrap gap-3">
            <Checkbox label="启用" checked={draft.enabled} onChange={(value) => setDraft({ ...draft, enabled: value })} />
            <Checkbox label="设为默认" checked={draft.is_default} onChange={(value) => setDraft({ ...draft, is_default: value })} />
          </div>
          <div className="mt-4 rounded-2xl border border-gray-200 bg-white/70 p-3 dark:border-white/10 dark:bg-white/[0.03]">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="text-xs font-bold text-gray-600 dark:text-gray-300">上下文管理</div>
                <div className="mt-1 text-[11px] leading-4 text-gray-400 dark:text-gray-500">
                  选择大致上下文长度，系统会套用推荐压缩模板；完整聊天记录仍会保留。
                </div>
              </div>
              {!contextUsesRecommendedTemplate && (
                <span className="shrink-0 rounded-full bg-amber-50 px-2 py-0.5 text-[10px] font-bold text-amber-600 dark:bg-amber-500/10 dark:text-amber-200">
                  已自定义
                </span>
              )}
            </div>
            <Field label="上下文长度">
              <Select
                value={contextPresetId}
                onChange={(event) => setDraft((current) => applyContextPreset(current, event.target.value))}
              >
                {contextPresetId === 'custom' && (
                  <option value="custom" disabled>
                    {selectedContextPreset.label} · {selectedContextPreset.caption}
                  </option>
                )}
                {CONTEXT_LENGTH_PRESETS.map((preset) => (
                  <option key={preset.id} value={preset.id}>
                    {preset.label} · {preset.caption}
                  </option>
                ))}
              </Select>
            </Field>
            {contextPresetId === 'custom' && (
              <div className="settings-expand-enter mt-2 rounded-xl bg-amber-50/80 px-3 py-2 text-[11px] leading-4 text-amber-700 dark:bg-amber-500/10 dark:text-amber-200">
                当前窗口为 {formatContextTokens(draft.context_window_tokens)}，不属于快捷模板。可在高级设置里继续精调。
              </div>
            )}
            <div className="mt-3 flex items-center justify-between gap-3 border-t border-gray-200/70 pt-3 dark:border-white/10">
              <div className="min-w-0 text-[11px] font-semibold leading-4 text-gray-500 dark:text-gray-400">
                {contextTemplateSummary(draft)}
              </div>
              <Button
                type="button"
                variant="ghost"
                size="xs"
                onClick={() => setShowContextAdvanced((current) => !current)}
                className="shrink-0"
              >
                <SlidersHorizontal size={13} />
                高级
                <ChevronDown size={13} className={`transition-transform ${showContextAdvanced ? 'rotate-180' : ''}`} />
              </Button>
            </div>
            {showContextAdvanced && (
              <div className="settings-expand-enter mt-3 border-t border-gray-200/70 pt-3 dark:border-white/10">
                <div className="grid grid-cols-2 gap-3">
                  <Field label="窗口 tokens">
                    <Input
                      type="number"
                      min="0"
                      value={draft.context_window_tokens}
                      onChange={(e) => setDraft({ ...draft, context_window_tokens: Number(e.target.value) || 0 })}
                      placeholder="0 自动"
                    />
                  </Field>
                  <Field label="触发水位 %">
                    <Input
                      type="number"
                      min="50"
                      max="95"
                      value={draft.auto_compact_threshold_percent}
                      onChange={(e) => setDraft({ ...draft, auto_compact_threshold_percent: Number(e.target.value) || 80 })}
                    />
                  </Field>
                </div>
                <Field label="保留缓冲 tokens">
                  <Input
                    type="number"
                    min="0"
                    value={draft.compact_reserved_tokens}
                    onChange={(e) => setDraft({ ...draft, compact_reserved_tokens: Number(e.target.value) || 0 })}
                    placeholder="0 自动"
                  />
                </Field>
                <div className="mt-3 flex flex-wrap gap-3">
                  <Checkbox
                    label="自动压缩"
                    checked={draft.auto_compact_enabled}
                    onChange={(value) => setDraft({ ...draft, auto_compact_enabled: value })}
                  />
                  <Checkbox
                    label="压缩旧工具输出"
                    checked={draft.prune_tool_outputs_enabled}
                    onChange={(value) => setDraft({ ...draft, prune_tool_outputs_enabled: value })}
                  />
                  <Checkbox
                    label="原生 Patch 工具"
                    checked={draft.native_patch_tool_enabled}
                    onChange={(value) => setDraft({ ...draft, native_patch_tool_enabled: value })}
                  />
                </div>
              </div>
            )}
          </div>
          <Button
            data-testid="model-save-button"
            type="submit"
            size="lg"
            fullWidth
            className="mt-5"
          >
            <Plus size={15} /> {editingId ? '保存模型' : '添加模型'}
          </Button>
        </form>
      </div>
    </SettingsSection>
  );
}
