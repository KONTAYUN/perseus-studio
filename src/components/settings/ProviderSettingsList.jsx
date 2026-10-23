import React from 'react';
import { Pencil, Plus } from 'lucide-react';
import { Badge, Button, EmptyCard, Input } from './SettingsUI';
import SettingsRowMenu from './SettingsRowMenu';

export default function ProviderSettingsList({ items, kind = 'models', onAdd, onEdit, onDelete, onDefault, busy }) {
  const [query, setQuery] = React.useState('');
  const label = kind === 'models' ? '模型' : '搜索工具';
  const filtered = items.filter(item => `${item.name || item.provider} ${item.model_id || ''} ${item.base_url || ''}`.toLowerCase().includes(query.trim().toLowerCase()));
  return (
    <div>
      <div className="settings-list-toolbar">
        <Input aria-label={`搜索${label}`} placeholder={`搜索${label}名称或 ID`} value={query} onChange={e => setQuery(e.target.value)} />
        <Button onClick={() => onAdd()} data-testid={kind === 'models' ? 'model-add-button' : 'search-add-button'} disabled={busy}><Plus size={16} />添加{label}</Button>
      </div>
      <div className="settings-list-caption">已配置 {items.length} 个{label}<span>密钥保存在后端，编辑时留空即可保留。</span></div>
      {!items.length ? <EmptyCard text={`还没有配置${label}，点击“添加${label}”开始。`} />
        : !filtered.length ? <EmptyCard text="没有找到匹配的配置，试试其他名称。" /> : (
          <div className="settings-provider-list">
            {filtered.map(item => (
              <div key={item.id} className="settings-provider-row" data-testid={`provider-row-${item.id}`}>
                <div className="settings-provider-summary">
                  <div className="settings-provider-name"><span>{item.name || item.provider}</span>
                    {item.is_default && <Badge tone="blue">默认</Badge>}
                    <Badge tone={item.enabled ? 'green' : 'gray'}>{item.enabled ? '启用' : '禁用'}</Badge>
                    <Badge tone="gray">{item.visibility === 'public' ? '所有用户' : '仅管理员'}</Badge>
                  </div>
                  <p>{kind === 'models' ? `${item.protocol} · ${item.model_id}` : `调用优先级 ${item.priority}`}</p>
                  {item.base_url && <p className="settings-provider-address" title={item.base_url}>{item.base_url}</p>}
                </div>
                <div className="settings-provider-actions">
                  <Button variant="ghost" size="sm" onClick={() => onEdit(item)} disabled={busy}><Pencil size={14} />编辑</Button>
                  <SettingsRowMenu label={`更多操作 ${item.name || item.provider}`} actions={[
                    ...(onDefault ? [{ label: '设为默认', disabled: item.is_default || busy, run: () => onDefault(item) }] : []),
                    { label: `删除${label}`, danger: true, disabled: busy, run: () => onDelete(item) },
                  ]} />
                </div>
              </div>
            ))}
          </div>
        )}
    </div>
  );
}
