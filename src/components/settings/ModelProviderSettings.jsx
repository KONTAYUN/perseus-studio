import React from 'react';
import useModelProviderDiscovery from '../../hooks/useModelProviderDiscovery';
import useSettingsEditor from '../../hooks/useSettingsEditor';
import { useSettingsGuard } from './SettingsNavigation';
import { Checkbox, Field, Input, Select } from './SettingsUI';
import { applyContextPreset, DEFAULT_CONTEXT_PRESET_ID } from './modelContextPresets';
import { modelReasoningDraft } from './ModelProviderReasoningControls';
import ModelContextSettings from './ModelContextSettings';
import ModelProviderDiscoveryPanel from './ModelProviderDiscoveryPanel';
import ProviderSettingsList from './ProviderSettingsList';
import SettingsFormActions, { SettingsFeedback } from './SettingsFormActions';

const emptyModel = applyContextPreset({ name: '', visibility: 'private', protocol: 'openai', base_url: '', api_key: '',
  model_id: '', enabled: true, is_default: false, reasoning_capability: 'auto', native_patch_tool_enabled: false, system_message_mode: 'single' }, DEFAULT_CONTEXT_PRESET_ID);

function toDraft(item) {
  return { name: item.name, visibility: item.visibility || 'private', protocol: item.protocol, base_url: item.base_url,
    api_key: '', model_id: item.model_id, enabled: item.enabled, is_default: item.is_default, ...modelReasoningDraft(item),
    context_window_tokens: item.context_window_tokens || 0, auto_compact_enabled: item.auto_compact_enabled !== false,
    auto_compact_threshold_percent: item.auto_compact_threshold_percent || 80, compact_reserved_tokens: item.compact_reserved_tokens || 0,
    prune_tool_outputs_enabled: item.prune_tool_outputs_enabled !== false, native_patch_tool_enabled: Boolean(item.native_patch_tool_enabled),
    system_message_mode: item.system_message_mode || 'single' };
}

export default function ModelProviderSettings({ ide, dialog }) {
  const [actionError, setActionError] = React.useState('');
  const [mutating, setMutating] = React.useState(false);
  const actionLock = React.useRef(false);
  const form = useSettingsEditor({ initial: emptyModel, toDraft,
    save: (draft, id) => ide.saveModelProvider({ ...draft, name: draft.name.trim() || draft.model_id.trim() }, id) });
  const { draft, setDraft, editingId } = form;
  const discovery = useModelProviderDiscovery({ draft, editingId, modelProviders: ide.modelProviders, saveModelProvider: ide.saveModelProvider });
  useSettingsGuard({ dirty: form.dirty, busy: form.saving || Boolean(discovery.addingModelId) || mutating,
    discard: () => { form.discard(); discovery.clearDiscoveryState(); discovery.clearTestState(); } });
  const open = item => {
    discovery.clearDiscoveryState(); discovery.clearTestState(); setActionError(''); form.open(item);
  };
  const mutate = async (action, success) => {
    if (actionLock.current) return;
    actionLock.current = true; setMutating(true); setActionError('');
    try { const result = await action(); form.setNotice(result?.refreshError ? `${success}，但列表刷新失败，请刷新列表。` : success); }
    catch (error) { setActionError(error.message); }
    finally { actionLock.current = false; setMutating(false); }
  };
  const remove = async item => {
    if (await dialog.confirm({ title: '删除模型配置？', message: `将删除“${item.name}”（${item.model_id}）。历史会话不会删除。`, confirmLabel: '删除模型', tone: 'red' })) {
      await mutate(() => ide.deleteModelProvider(item.id), '模型配置已删除');
    }
  };
  if (!form.opened) return <>
    <SettingsFeedback error={actionError} notice={form.notice} />
    <ProviderSettingsList items={ide.modelProviders} onAdd={open} onEdit={open} onDelete={remove} busy={mutating}
      onDefault={item => mutate(() => ide.setDefaultModelProvider(item.id), '默认模型已更新')} />
    <button type="button" className="settings-text-button motion-button" onClick={async () => {
      const result = await ide.loadModelProviders(); if (!result?.refreshError) form.setNotice('列表已刷新。');
    }}>刷新列表</button>
  </>;
  const chooseModel = model => setDraft(current => ({ ...current, model_id: model.id, name: current.name || model.id, ...modelReasoningDraft(model) }));
  return (
    <form onSubmit={form.submit} className="settings-editor" aria-label={editingId ? '编辑模型' : '添加模型'}>
      <h2>{editingId ? `编辑模型 · ${draft.name || draft.model_id}` : '添加模型'}</h2>
      <p className="settings-form-description">先连接服务，再选择模型。每个模型配置独立保存。</p>
      <fieldset disabled={form.saving || Boolean(discovery.addingModelId)}>
        <div className="settings-form-group">
          <h3>连接信息</h3>
          <Field label="协议"><Select value={draft.protocol} onChange={e => setDraft({ ...draft, protocol: e.target.value })}>
            <option value="openai">OpenAI-compatible</option><option value="anthropic">Anthropic</option>
          </Select></Field>
          <Field label="Base URL"><Input autoFocus data-testid="model-base-url-input" value={draft.base_url} onChange={e => setDraft({ ...draft, base_url: e.target.value })} placeholder="https://api.openai.com/v1" required /></Field>
          <Field label={editingId ? 'API Key（留空保留旧 key）' : 'API Key'}><Input type="password" autoComplete="off" data-testid="model-api-key-input" value={draft.api_key} onChange={e => setDraft({ ...draft, api_key: e.target.value })} required={!editingId} placeholder={editingId ? '已配置，留空即可保留' : '输入服务商提供的密钥'} /></Field>
          <Field label="可见范围"><Select value={draft.visibility} onChange={e => setDraft({ ...draft, visibility: e.target.value })}>
            <option value="private">仅管理员</option><option value="public">所有用户</option>
          </Select></Field>
          <ModelProviderDiscoveryPanel {...discovery} canDiscoverModels={Boolean(draft.base_url && (draft.api_key || editingId))}
            canTestProvider={Boolean(draft.base_url && draft.model_id && (draft.api_key || editingId))} draft={draft}
            onAddDiscoveredModel={discovery.addDiscoveredModel} onChooseDiscoveredModel={chooseModel}
            onDiscoverModels={discovery.discoverModels} onTestProvider={discovery.testProvider} />
        </div>
        <div className="settings-form-group">
          <h3>{editingId ? '模型配置' : '手工配置模型'}</h3>
          <p className="settings-form-description">也可以直接填写 Model ID，或点击获取结果中的模型名称填入这里。</p>
          <Field label="Model ID"><Input data-testid="model-id-input" value={draft.model_id} onChange={e => setDraft({ ...draft, model_id: e.target.value })} placeholder="服务商提供的完整模型 ID" required /></Field>
          <Field label="自定义名称"><Input data-testid="model-name-input" value={draft.name} onChange={e => setDraft({ ...draft, name: e.target.value })} placeholder="留空使用 Model ID" /></Field>
          <div className="settings-checkbox-row"><Checkbox label="启用" checked={draft.enabled} onChange={enabled => setDraft({ ...draft, enabled })} /><Checkbox label="设为默认" checked={draft.is_default} onChange={is_default => setDraft({ ...draft, is_default })} /></div>
          <ModelContextSettings draft={draft} setDraft={setDraft} />
        </div>
      </fieldset>
      <SettingsFeedback error={form.error} />
      <SettingsFormActions onCancel={form.cancel} saving={form.saving} disabled={Boolean(discovery.addingModelId)} label={editingId ? '保存模型' : '添加模型'} />
    </form>
  );
}
