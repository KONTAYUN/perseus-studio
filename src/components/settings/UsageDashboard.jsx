import React from 'react';
import { ArrowLeft, BarChart3, Coins, Plus, RefreshCw, Settings, Trash2 } from 'lucide-react';
import { apiDelete, apiGet, apiPatch, apiPost } from '../../api/client';
import {
  Badge,
  Button,
  EmptyCard,
  Field,
  FormHeader,
  IconButton,
  Input,
  ProviderCard,
  SectionTitle,
  SegmentedControl,
  Select,
  SettingsSection,
  formatDate,
} from './SettingsUI';

const defaultRange = '30d';
const usageViewOptions = [
  { value: 'users', label: '按用户' },
  { value: 'models', label: '按模型' },
  { value: 'events', label: '最近调用' },
];

export default function UsageDashboard({ dialog, onBack, currentUser }) {
  const isAdmin = currentUser?.role === 'admin';
  const [range, setRange] = React.useState(defaultRange);
  const [userId, setUserId] = React.useState('');
  const [providerId, setProviderId] = React.useState('');
  const [summary, setSummary] = React.useState(null);
  const [events, setEvents] = React.useState([]);
  const [usersById, setUsersById] = React.useState({});
  const [providers, setProviders] = React.useState([]);
  const [prices, setPrices] = React.useState([]);
  const [priceDraft, setPriceDraft] = React.useState(emptyPriceDraft());
  const [editingPriceId, setEditingPriceId] = React.useState('');
  const [activeView, setActiveView] = React.useState('users');
  const [showPriceSettings, setShowPriceSettings] = React.useState(false);
  const [message, setMessage] = React.useState('');
  const [isLoading, setIsLoading] = React.useState(false);

  const loadUsage = React.useCallback(async () => {
    setIsLoading(true);
    setMessage('');
    try {
      const query = usageQuery({ range, userId, providerId });
      const [summaryData, eventsData, pricesData] = await Promise.all([
        apiGet(`/api/admin/usage/summary?${query}`),
        apiGet(`/api/admin/usage/events?${query}&limit=100`),
        apiGet('/api/admin/usage/prices'),
      ]);
      setSummary(summaryData.summary || null);
      setUsersById(summaryData.users || {});
      setProviders(summaryData.providers || []);
      setEvents(eventsData.events || []);
      setPrices(pricesData.items || []);
    } catch (error) {
      setMessage(error.message);
    } finally {
      setIsLoading(false);
    }
  }, [range, userId, providerId]);

  React.useEffect(() => {
    if (isAdmin) loadUsage();
  }, [isAdmin, loadUsage]);

  if (!isAdmin) return null;

  const totals = summary?.totals || {};
  const activeTitle = activeView === 'users' ? '按用户' : activeView === 'models' ? '按模型' : '最近调用明细';
  const activeDescription = activeView === 'events'
    ? '只显示调用元数据，不包含提示词、回复正文、文件内容或 API Key。'
    : '当前筛选范围内的聚合统计。';

  async function savePrice(event) {
    event.preventDefault();
    setMessage('');
    try {
      const payload = normalizePriceDraft(priceDraft);
      if (editingPriceId) {
        await apiPatch(`/api/admin/usage/prices/${encodeURIComponent(editingPriceId)}`, payload);
      } else {
        await apiPost('/api/admin/usage/prices', payload);
      }
      resetPriceForm();
      await loadUsage();
    } catch (error) {
      setMessage(error.message);
    }
  }

  function editPrice(item) {
    setEditingPriceId(item.id);
    setPriceDraft({
      model_provider_id: item.model_provider_id || '',
      model_id: item.model_id || '',
      currency: item.currency || 'USD',
      input_price_micro: priceMicroToDisplay(item.input_price_micro),
      output_price_micro: priceMicroToDisplay(item.output_price_micro),
      cache_creation_price_micro: priceMicroToDisplay(item.cache_creation_price_micro),
      cache_read_price_micro: priceMicroToDisplay(item.cache_read_price_micro),
    });
  }

  async function deletePrice(item) {
    const ok = await dialog?.confirm?.({
      title: '删除价格配置',
      message: `确认删除 ${providerLabel(providers, item.model_provider_id)} / ${item.model_id} 的价格配置？历史用量金额不会重算。`,
      confirmLabel: '删除',
      tone: 'red',
    });
    if (!ok) return;
    try {
      await apiDelete(`/api/admin/usage/prices/${encodeURIComponent(item.id)}`);
      await loadUsage();
      if (editingPriceId === item.id) resetPriceForm();
    } catch (error) {
      setMessage(error.message);
    }
  }

  function resetPriceForm() {
    setEditingPriceId('');
    setPriceDraft(emptyPriceDraft());
  }

  return (
    <SettingsSection data-testid="usage-dashboard" className="settings-view-enter">
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 flex-wrap items-center gap-3">
          <Button type="button" variant="ghost" size="sm" onClick={onBack}>
            <ArrowLeft size={16} /> 用量统计
          </Button>
          <Select value={range} onChange={(event) => setRange(event.target.value)} className="w-32">
            <option value="today">今天</option>
            <option value="7d">7 天</option>
            <option value="30d">30 天</option>
          </Select>
          <Select value={userId} onChange={(event) => setUserId(event.target.value)} className="w-40">
            <option value="">全部用户</option>
            {Object.values(usersById).map((user) => (
              <option key={user.id} value={user.id}>{user.username || user.id}</option>
            ))}
          </Select>
          <Select value={providerId} onChange={(event) => setProviderId(event.target.value)} className="w-48">
            <option value="">全部模型配置</option>
            {providers.map((provider) => (
              <option key={provider.id} value={provider.id}>{provider.name} · {provider.model_id}</option>
            ))}
          </Select>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button type="button" variant="secondary" size="sm" onClick={() => setShowPriceSettings(true)}>
            <Settings size={14} /> 价格配置
          </Button>
          <Button type="button" variant="secondary" size="sm" onClick={loadUsage} disabled={isLoading}>
            <RefreshCw size={14} /> {isLoading ? '刷新中' : '刷新'}
          </Button>
        </div>
      </div>

      {message && (
        <div className="settings-expand-enter mb-4 rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-600 dark:border-red-500/20 dark:bg-red-500/10 dark:text-red-300">
          {message}
        </div>
      )}

      <div className="mb-5 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <UsageMetric title="请求数" value={formatNumber(totals.request_count)} caption={`失败 ${formatNumber(totals.failed_count)} 次`} />
        <UsageMetric title="总 Tokens" value={formatTokens(totals.total_tokens)} caption={`输入 ${formatTokens(totals.input_tokens)} / 输出 ${formatTokens(totals.output_tokens)}`} />
        <UsageMetric title="缓存 Tokens" value={formatTokens((totals.cache_creation_tokens || 0) + (totals.cache_read_tokens || 0))} caption={`写入 ${formatTokens(totals.cache_creation_tokens)} / 读取 ${formatTokens(totals.cache_read_tokens)}`} />
        <UsageMetric title="估算费用" value={formatCost(totals.estimated_cost_micro, totals.currency)} caption={totals.currency ? '基于已配置价格' : '未配置价格，仅统计 tokens'} />
      </div>

      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <SectionTitle icon={<BarChart3 size={15} />} title={activeTitle} description={activeDescription} />
        <SegmentedControl
          value={activeView}
          onChange={setActiveView}
          options={usageViewOptions}
        />
      </div>

      {activeView === 'users' && (
        <UsageTable rows={summary?.by_user || []} kind="user" usersById={usersById} />
      )}
      {activeView === 'models' && (
        <UsageTable rows={summary?.by_model || []} kind="model" providers={providers} />
      )}
      {activeView === 'events' && (
        <UsageEvents events={events} usersById={usersById} providers={providers} />
      )}

      {showPriceSettings && (
        <PriceSettingsPanel
          providers={providers}
          prices={prices}
          priceDraft={priceDraft}
          editingPriceId={editingPriceId}
          onClose={() => setShowPriceSettings(false)}
          onSave={savePrice}
          onReset={resetPriceForm}
          onDraftChange={setPriceDraft}
          onEdit={editPrice}
          onDelete={deletePrice}
        />
      )}
    </SettingsSection>
  );
}

function UsageMetric({ title, value, caption }) {
  return (
    <div className="settings-card rounded-2xl border border-gray-100 bg-white/80 p-4 dark:border-white/10 dark:bg-[#252526]/80">
      <div className="text-xs font-bold text-gray-400">{title}</div>
      <div className="mt-2 text-xl font-bold text-gray-800 dark:text-gray-100">{value || '-'}</div>
      <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">{caption}</div>
    </div>
  );
}

function UsageTable({ rows, kind, usersById = {}, providers = [] }) {
  return (
    <div className="settings-expand-enter">
      {rows.length === 0 ? (
        <EmptyCard text="暂无用量。" />
      ) : (
        <div className="settings-card max-h-[52vh] overflow-auto rounded-2xl border border-gray-100 bg-white/80 dark:border-white/10 dark:bg-[#252526]/80 custom-scrollbar">
          <div className="grid grid-cols-[minmax(0,1.5fr)_80px_100px_100px] gap-3 border-b border-gray-100 px-4 py-2 text-xs font-bold text-gray-400 dark:border-white/10">
            <div>{kind === 'user' ? '用户' : '模型'}</div>
            <div className="text-right">请求</div>
            <div className="text-right">Tokens</div>
            <div className="text-right">费用</div>
          </div>
          {rows.map((row) => {
            const name = kind === 'user'
              ? (usersById[row.user_id]?.username || row.username || row.user_id)
              : `${row.provider_name || providerLabel(providers, row.model_provider_id)} · ${row.model_id || 'unknown'}`;
            return (
              <div key={kind === 'user' ? row.user_id : `${row.model_provider_id}:${row.model_id}`} className="grid grid-cols-[minmax(0,1.5fr)_80px_100px_100px] gap-3 border-b border-gray-100 px-4 py-3 text-sm last:border-0 dark:border-white/10">
                <div className="min-w-0">
                  <div className="truncate font-bold text-gray-700 dark:text-gray-100">{name}</div>
                  <div className="mt-0.5 text-xs text-gray-400">失败 {formatNumber(row.failed_count)}</div>
                </div>
                <div className="text-right text-gray-500 dark:text-gray-300">{formatNumber(row.request_count)}</div>
                <div className="text-right text-gray-500 dark:text-gray-300">{formatTokens(row.total_tokens)}</div>
                <div className="text-right text-gray-500 dark:text-gray-300">{formatCost(row.estimated_cost_micro, row.currency)}</div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function UsageEvents({ events, usersById, providers }) {
  if (events.length === 0) {
    return <EmptyCard text="当前筛选范围内还没有用量记录。" />;
  }
  return (
    <div className="settings-expand-enter max-h-[52vh] space-y-3 overflow-auto pr-1 custom-scrollbar">
      {events.map((item) => (
        <ProviderCard key={item.id} className="min-h-0 items-start">
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <Badge tone={item.status === 'succeeded' ? 'green' : 'red'}>{item.status === 'succeeded' ? '成功' : '失败'}</Badge>
              <span className="truncate text-sm font-bold text-gray-800 dark:text-gray-100">
                {usersById[item.user_id]?.username || item.user_id}
              </span>
              <span className="text-xs text-gray-400">{item.stage || 'provider'}</span>
            </div>
            <div className="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">
              {item.provider_name || providerLabel(providers, item.model_provider_id)} · {item.model_id || 'unknown'}
            </div>
            <div className="mt-2 flex flex-wrap gap-2 text-[11px] font-bold text-gray-400">
              <span>输入 {formatTokens(item.input_tokens)}</span>
              <span>输出 {formatTokens(item.output_tokens)}</span>
              <span>总计 {formatTokens(item.total_tokens)}</span>
              {item.error_class && <span className="text-red-400">{item.error_class}</span>}
            </div>
          </div>
          <div className="shrink-0 text-right text-xs text-gray-400">
            <div>{formatDate(item.created_at)}</div>
            <div className="mt-1">{formatCost(item.estimated_cost_micro, item.currency)}</div>
          </div>
        </ProviderCard>
      ))}
    </div>
  );
}

function PriceSettingsPanel({
  providers,
  prices,
  priceDraft,
  editingPriceId,
  onClose,
  onSave,
  onReset,
  onDraftChange,
  onEdit,
  onDelete,
}) {
  return (
    <div className="settings-expand-enter fixed inset-0 z-[120] flex items-center justify-center bg-black/35 p-4 backdrop-blur-sm dark:bg-black/55">
      <div className="settings-modal-card ui-overlay-surface flex max-h-[82vh] w-full max-w-3xl flex-col overflow-hidden border border-white/40 bg-white/95 dark:border-white/10 dark:bg-[#1C1C1E]/95">
        <div className="flex items-center justify-between border-b border-gray-100 px-5 py-4 dark:border-white/10">
          <div>
            <div className="text-base font-bold text-gray-800 dark:text-gray-100">价格配置</div>
            <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">可选。配置后未来调用会记录估算费用。</div>
          </div>
          <Button type="button" variant="ghost" size="sm" onClick={onClose}>
            <ArrowLeft size={15} /> 返回
          </Button>
        </div>
        <div className="grid flex-1 gap-5 overflow-y-auto p-5 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)] custom-scrollbar">
          <form onSubmit={onSave} className="settings-card rounded-2xl border border-gray-100 bg-white/80 p-4 dark:border-white/10 dark:bg-[#252526]/80">
            <FormHeader title={editingPriceId ? '编辑价格' : '新增价格'} onReset={onReset} />
            <Field label="模型配置">
              <Select value={priceDraft.model_provider_id} onChange={(event) => {
                const selected = providers.find((item) => item.id === event.target.value);
                onDraftChange((draft) => ({
                  ...draft,
                  model_provider_id: event.target.value,
                  model_id: selected?.model_id || draft.model_id,
                }));
              }} required>
                <option value="">选择模型配置</option>
                {providers.map((provider) => (
                  <option key={provider.id} value={provider.id}>{provider.name} · {provider.model_id}</option>
                ))}
              </Select>
            </Field>
            <Field label="Model ID">
              <Input value={priceDraft.model_id} onChange={(event) => onDraftChange((draft) => ({ ...draft, model_id: event.target.value }))} required />
            </Field>
            <Field label="货币">
              <Input value={priceDraft.currency} onChange={(event) => onDraftChange((draft) => ({ ...draft, currency: event.target.value.toUpperCase() }))} placeholder="USD" />
            </Field>
            <div className="grid grid-cols-2 gap-3">
              <Field label="输入 / 百万">
                <Input type="number" min="0" step="0.000001" value={priceDraft.input_price_micro} onChange={(event) => onDraftChange((draft) => ({ ...draft, input_price_micro: event.target.value }))} />
              </Field>
              <Field label="输出 / 百万">
                <Input type="number" min="0" step="0.000001" value={priceDraft.output_price_micro} onChange={(event) => onDraftChange((draft) => ({ ...draft, output_price_micro: event.target.value }))} />
              </Field>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <Field label="缓存写入 / 百万">
                <Input type="number" min="0" step="0.000001" value={priceDraft.cache_creation_price_micro} onChange={(event) => onDraftChange((draft) => ({ ...draft, cache_creation_price_micro: event.target.value }))} />
              </Field>
              <Field label="缓存读取 / 百万">
                <Input type="number" min="0" step="0.000001" value={priceDraft.cache_read_price_micro} onChange={(event) => onDraftChange((draft) => ({ ...draft, cache_read_price_micro: event.target.value }))} />
              </Field>
            </div>
            <Button type="submit" className="mt-5" fullWidth>
              <Plus size={15} /> {editingPriceId ? '保存价格' : '添加价格'}
            </Button>
          </form>

          <div className="min-w-0">
            <SectionTitle icon={<Coins size={15} />} title="已配置价格" />
            {prices.length === 0 ? (
              <EmptyCard text="还没有配置模型价格。" />
            ) : (
              <div className="max-h-[56vh] space-y-2 overflow-y-auto pr-1 custom-scrollbar">
                {prices.map((item) => (
                  <ProviderCard key={item.id} className="min-h-0">
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm font-bold text-gray-800 dark:text-gray-100">
                        {providerLabel(providers, item.model_provider_id)}
                      </div>
                      <div className="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">
                        {item.model_id} · {item.currency}
                      </div>
                      <div className="mt-1 text-xs text-gray-400">
                        输入 {priceMicroToDisplay(item.input_price_micro)} / 输出 {priceMicroToDisplay(item.output_price_micro)}
                      </div>
                    </div>
                    <IconButton title="编辑价格" onClick={() => onEdit(item)}>
                      <Coins size={14} />
                    </IconButton>
                    <IconButton title="删除价格" danger onClick={() => onDelete(item)}>
                      <Trash2 size={14} />
                    </IconButton>
                  </ProviderCard>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function usageQuery({ range, userId, providerId }) {
  const params = new URLSearchParams();
  params.set('range', range || defaultRange);
  if (userId) params.set('user_id', userId);
  if (providerId) params.set('model_provider_id', providerId);
  return params.toString();
}

function emptyPriceDraft() {
  return {
    model_provider_id: '',
    model_id: '',
    currency: 'USD',
    input_price_micro: '',
    output_price_micro: '',
    cache_creation_price_micro: '',
    cache_read_price_micro: '',
  };
}

function normalizePriceDraft(draft) {
  return {
    model_provider_id: draft.model_provider_id,
    model_id: draft.model_id,
    currency: draft.currency || 'USD',
    input_price_micro: displayPriceToMicro(draft.input_price_micro),
    output_price_micro: displayPriceToMicro(draft.output_price_micro),
    cache_creation_price_micro: displayPriceToMicro(draft.cache_creation_price_micro),
    cache_read_price_micro: displayPriceToMicro(draft.cache_read_price_micro),
  };
}

function displayPriceToMicro(value) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) return 0;
  return Math.round(parsed * 1_000_000);
}

function priceMicroToDisplay(value) {
  const number = Number(value || 0) / 1_000_000;
  if (!number) return '';
  return String(Number(number.toFixed(6)));
}

function providerLabel(providers, id) {
  const item = providers.find((provider) => provider.id === id);
  return item?.name || id || '未知模型配置';
}

function formatNumber(value) {
  return Number(value || 0).toLocaleString('zh-CN');
}

function formatTokens(value) {
  const number = Number(value || 0);
  if (number >= 1_000_000) return `${(number / 1_000_000).toFixed(2)}M`;
  if (number >= 1_000) return `${(number / 1_000).toFixed(1)}K`;
  return formatNumber(number);
}

function formatCost(value, currency) {
  const micro = Number(value || 0);
  if (!currency || !micro) return '未配置';
  return `${currency} ${(micro / 1_000_000).toFixed(4)}`;
}
