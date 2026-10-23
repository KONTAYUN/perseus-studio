import React from 'react';
import useSettingsEditor from '../../hooks/useSettingsEditor';
import { useSettingsGuard } from './SettingsNavigation';
import { Checkbox, Field, Input, Select } from './SettingsUI';
import ProviderSettingsList from './ProviderSettingsList';
import SettingsFormActions, { SettingsFeedback } from './SettingsFormActions';

const initial = { provider: 'tavily', visibility: 'private', api_key: '', enabled: true, priority: 100 };
const toDraft = item => ({ provider: item.provider, visibility: item.visibility || 'private', api_key: '', enabled: item.enabled, priority: item.priority });

export default function SearchProviderSettings({ ide, dialog }) {
  const [error, setError] = React.useState('');
  const [busy, setBusy] = React.useState(false);
  const lock = React.useRef(false);
  const form = useSettingsEditor({ initial, toDraft, busy,
    save: (draft, id) => ide.saveSearchProvider({ ...draft, priority: Number(draft.priority) }, id) });
  const { draft, setDraft, editingId } = form;
  useSettingsGuard({ dirty: form.dirty, busy: form.saving || busy, discard: form.discard });
  const remove = async item => {
    if (lock.current) return;
    if (!await dialog.confirm({ title: '删除搜索工具？', message: `将删除 ${item.provider} 的这条配置，Agent 将无法再使用它。`, confirmLabel: '删除搜索工具', tone: 'red' })) return;
    lock.current = true; setBusy(true); setError('');
    try { const result = await ide.deleteSearchProvider(item.id); form.setNotice(result?.refreshError ? '已删除，但列表刷新失败，请刷新列表。' : '搜索工具已删除。'); }
    catch (failure) { setError(failure.message); }
    finally { lock.current = false; setBusy(false); }
  };
  if (!form.opened) return <>
    <SettingsFeedback error={error} notice={form.notice} />
    <ProviderSettingsList kind="search" items={ide.searchProviders} onAdd={form.open} onEdit={form.open} onDelete={remove} busy={busy} />
    <button type="button" className="settings-text-button motion-button" onClick={async () => { const result = await ide.loadSearchProviders(); if (!result?.refreshError) form.setNotice('列表已刷新。'); }}>刷新列表</button>
  </>;
  return <form className="settings-editor" onSubmit={form.submit} aria-label={editingId ? '编辑搜索工具' : '添加搜索工具'}>
    <h2>{editingId ? '编辑搜索工具' : '添加搜索工具'}</h2>
    <p className="settings-form-description">供 Agent 联网检索公开资料；密钥不会发送给聊天模型。</p>
    <fieldset disabled={form.saving}>
      <Field label="服务商"><Select data-testid="search-provider-select" value={draft.provider} onChange={e => setDraft({ ...draft, provider: e.target.value })}><option value="tavily">Tavily</option><option value="brave">Brave</option></Select></Field>
      <Field label={editingId ? 'API Key（留空保留旧 key）' : 'API Key'}><Input autoFocus data-testid="search-api-key-input" type="password" autoComplete="off" value={draft.api_key} onChange={e => setDraft({ ...draft, api_key: e.target.value })} required={!editingId} /></Field>
      <Field label="可见范围"><Select value={draft.visibility} onChange={e => setDraft({ ...draft, visibility: e.target.value })}><option value="private">仅管理员</option><option value="public">所有用户</option></Select></Field>
      <Field label="调用优先级"><Input data-testid="search-priority-input" type="number" value={draft.priority} onChange={e => setDraft({ ...draft, priority: e.target.value })} required /></Field>
      <p className="settings-form-description">数值越小，调用顺序越靠前。</p>
      <div className="settings-checkbox-row"><Checkbox label="启用" checked={draft.enabled} onChange={enabled => setDraft({ ...draft, enabled })} /></div>
    </fieldset>
    <SettingsFeedback error={form.error} />
    <SettingsFormActions saving={form.saving} onCancel={form.cancel} label={editingId ? '保存搜索工具' : '添加搜索工具'} />
  </form>;
}
