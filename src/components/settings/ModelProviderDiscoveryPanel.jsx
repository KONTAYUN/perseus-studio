import React from 'react';
import { Check, FlaskConical, Loader2, Plus, RefreshCw } from 'lucide-react';
import { Button, IconButton } from './SettingsUI';
import ProviderTestReport from './ProviderTestReport';

export default function ModelProviderDiscoveryPanel({
  addFeedback,
  addingModelId,
  isModelAdded,
  onAddDiscoveredModel,
  canDiscoverModels,
  canTestProvider,
  discoverError,
  discoverInfo,
  discoveredModels,
  draft,
  isDiscovering,
  isTesting,
  onChooseDiscoveredModel,
  onDiscoverModels,
  onTestProvider,
  testError,
  testReport,
}) {
  const [query, setQuery] = React.useState('');
  const visibleModels = discoveredModels.filter(model => model.id.toLowerCase().includes(query.trim().toLowerCase()));
  return (
    <div data-testid="model-discovery-panel" className="mt-3 rounded-2xl border border-gray-200 bg-white/70 p-3 dark:border-white/10 dark:bg-white/[0.03]">
      <div className="settings-discovery-header">
        <div className="settings-discovery-heading">
          <div className="text-sm font-bold text-gray-600 dark:text-gray-300">获取可用模型</div>
          <div className="mt-1 text-[11px] leading-4 text-gray-400 dark:text-gray-500">
            支持 OpenAI-compatible 的 <span className="font-mono">/models</span> 接口。编辑已有配置时，API Key 留空也会使用已保存的 key。
          </div>
        </div>
        <div className="settings-discovery-actions">
          <Button type="button" variant="subtle" size="sm" onClick={onDiscoverModels} disabled={!canDiscoverModels || isDiscovering}>
            {isDiscovering ? <Loader2 size={13} className="animate-spin" /> : <RefreshCw size={13} />}
            获取模型列表
          </Button>
          <Button
            type="button"
            variant="subtle"
            size="sm"
            onClick={onTestProvider}
            disabled={!canTestProvider || isTesting}
            className="border-blue-500/25 bg-blue-500/10 text-blue-600 hover:border-blue-400 hover:bg-blue-500/15 dark:text-blue-200"
          >
            {isTesting ? <Loader2 size={13} className="animate-spin" /> : <FlaskConical size={13} />}
            测试模型调用
          </Button>
        </div>
      </div>
      <p className="mt-2 text-xs text-gray-500 dark:text-gray-400">测试会调用当前填写的 Model ID，可能产生用量；不会保存表单。</p>
      {discoverError && (
        <div className="settings-expand-enter mt-3 rounded-xl border border-red-500/20 bg-red-500/10 px-3 py-2 text-xs leading-5 text-red-600 dark:text-red-200">
          {discoverError}
        </div>
      )}
      {!discoverError && discoverInfo && (
        <div role="status" className="settings-expand-enter mt-3 rounded-xl border border-blue-500/20 bg-blue-500/10 px-3 py-2 text-xs text-blue-600 dark:text-blue-200">
          {discoverInfo}
        </div>
      )}
      {discoveredModels.length > 0 && (
        <>
        <p className="mt-2 text-[11px] leading-4 text-gray-400 dark:text-gray-500">点击 ＋ 直接添加，可连续添加多个模型；默认模型保持不变。</p>
        <input aria-label="搜索获取的模型" className="settings-model-filter" value={query} onChange={e => setQuery(e.target.value)} placeholder="搜索模型 ID" />
        <div data-testid="discovered-model-list" className="settings-expand-enter mt-2 max-h-52 space-y-1 overflow-y-auto pr-1">
          {!visibleModels.length && <p className="p-2 text-xs">没有匹配的模型</p>}
          {visibleModels.map((model) => {
            const selected = model.id === draft.model_id;
            const added = isModelAdded(model.id);
            const pending = addingModelId === model.id;
            return (
              <div
                key={model.id}
                data-testid="discovered-model-row"
                className="settings-discovered-model-row"
                data-selected={selected || undefined}
              >
                <button type="button" onClick={() => onChooseDiscoveredModel(model)} title={model.id}
                  className="motion-row min-w-0 flex-1 truncate py-2 text-left font-mono"
                  aria-label={`填写模型 ${model.id}`}>
                  {model.id}
                </button>
                <IconButton type="button" size="sm" title={added ? `已添加 ${model.id}` : pending ? `正在添加 ${model.id}` : `添加模型 ${model.id}`}
                  aria-label={added ? `已添加 ${model.id}` : `添加模型 ${model.id}`}
                  aria-busy={pending || undefined}
                  disabled={added || Boolean(addingModelId)} onClick={() => onAddDiscoveredModel(model)}
                  className={added ? 'text-emerald-600 dark:text-emerald-300' : ''}>
                  {pending ? <Loader2 size={14} className="animate-spin" /> : added ? <Check size={14} /> : <Plus size={14} />}
                </IconButton>
              </div>
            );
          })}
        </div>
        </>
      )}
      {addFeedback && (
        <div role={addFeedback.error ? 'alert' : 'status'} className={`mt-2 text-xs leading-5 ${addFeedback.error ? 'text-red-600 dark:text-red-200' : 'text-emerald-600 dark:text-emerald-300'}`}>
          {addFeedback.message}
        </div>
      )}
      {testError && (
        <div className="settings-expand-enter mt-3 rounded-xl border border-red-500/20 bg-red-500/10 px-3 py-2 text-xs leading-5 text-red-600 dark:text-red-200">
          {testError}
        </div>
      )}
      {testReport && <ProviderTestReport report={testReport} />}
    </div>
  );
}
