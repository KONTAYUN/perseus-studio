import React from 'react';
import { Pencil, Plus, Search, Trash2 } from 'lucide-react';
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

const emptySearch = {
  provider: 'tavily',
  visibility: 'private',
  api_key: '',
  enabled: true,
  priority: 100,
};

export default function SearchProviderSettings({ ide }) {
  const [draft, setDraft] = React.useState(emptySearch);
  const [editingId, setEditingId] = React.useState('');

  const resetDraft = () => {
    setEditingId('');
    setDraft(emptySearch);
  };

  const editSearch = (item) => {
    setEditingId(item.id);
    setDraft({
      provider: item.provider,
      visibility: item.visibility || 'private',
      api_key: '',
      enabled: item.enabled,
      priority: item.priority,
    });
  };

  const submitSearch = async (event) => {
    event.preventDefault();
    await ide.saveSearchProvider({ ...draft, priority: Number(draft.priority) || 100 }, editingId);
    resetDraft();
  };

  return (
    <SettingsSection>
      <SectionTitle icon={<Search size={15} />} title="搜索工具" description="联网工具只作为后端 Agent tools 配置预留。" />
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_380px]">
        <div className="min-w-0 space-y-3">
          {ide.searchProviders.length === 0 ? (
            <EmptyCard text="还没有搜索工具配置。Tavily / Brave 只作为后端 Agent tools 预留。" />
          ) : (
            ide.searchProviders.map((item) => (
              <ProviderCard key={item.id}>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-bold capitalize text-gray-800 dark:text-gray-100">{item.provider}</span>
                    <Badge tone={item.enabled ? 'green' : 'gray'}>{item.enabled ? '启用' : '禁用'}</Badge>
                    <Badge tone={item.visibility === 'public' ? 'blue' : 'gray'}>{item.visibility === 'public' ? '公开' : '私密'}</Badge>
                    <Badge tone="blue">优先级 {item.priority}</Badge>
                  </div>
                  <div className="mt-1 text-xs text-gray-400">Key: {item.masked_key || '未设置'}</div>
                </div>
                <CardActions>
                  <IconButton title="编辑" onClick={() => editSearch(item)}>
                    <Pencil size={14} />
                  </IconButton>
                  <IconButton title="删除" danger onClick={() => ide.deleteSearchProvider(item.id)}>
                    <Trash2 size={14} />
                  </IconButton>
                </CardActions>
              </ProviderCard>
            ))
          )}
        </div>

        <form onSubmit={submitSearch} className="settings-card rounded-3xl border border-gray-100 bg-gray-50/90 p-5 dark:border-white/10 dark:bg-[#252526]/80">
          <FormHeader title={editingId ? '编辑搜索工具' : '新增搜索工具'} onReset={resetDraft} />
          <Field label="Provider">
            <Select
              data-testid="search-provider-select"
              value={draft.provider}
              onChange={(e) => setDraft({ ...draft, provider: e.target.value })}
            >
              <option value="tavily">tavily</option>
              <option value="brave">brave</option>
            </Select>
          </Field>
          <Field label="可见性">
            <Select
              value={draft.visibility || 'private'}
              onChange={(e) => setDraft({ ...draft, visibility: e.target.value })}
            >
              <option value="private">私密（仅管理员）</option>
              <option value="public">公开（所有用户）</option>
            </Select>
          </Field>
          <Field label={editingId ? 'API Key（留空保留旧 key）' : 'API Key'}>
            <Input
              data-testid="search-api-key-input"
              type="password"
              value={draft.api_key}
              onChange={(e) => setDraft({ ...draft, api_key: e.target.value })}
              required={!editingId}
            />
          </Field>
          <Field label="Priority">
            <Input
              data-testid="search-priority-input"
              type="number"
              value={draft.priority}
              onChange={(e) => setDraft({ ...draft, priority: e.target.value })}
            />
          </Field>
          <Checkbox label="启用" checked={draft.enabled} onChange={(value) => setDraft({ ...draft, enabled: value })} />
          <Button
            data-testid="search-save-button"
            type="submit"
            size="lg"
            fullWidth
            className="mt-5"
          >
            <Plus size={15} /> {editingId ? '保存搜索工具' : '添加搜索工具'}
          </Button>
        </form>
      </div>
    </SettingsSection>
  );
}
