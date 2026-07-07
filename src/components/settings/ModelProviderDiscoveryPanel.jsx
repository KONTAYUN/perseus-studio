import React from 'react';
import { Check, FlaskConical, Loader2, RefreshCw } from 'lucide-react';
import { Button } from './SettingsUI';
import ProviderTestReport from './ProviderTestReport';

export default function ModelProviderDiscoveryPanel({
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
  return (
    <div className="mt-3 rounded-2xl border border-gray-200 bg-white/70 p-3 dark:border-white/10 dark:bg-white/[0.03]">
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <div className="text-xs font-bold text-gray-600 dark:text-gray-300">自动获取模型列表</div>
          <div className="mt-1 text-[11px] leading-4 text-gray-400 dark:text-gray-500">
            支持 OpenAI-compatible 的 <span className="font-mono">/models</span> 接口。编辑已有配置时，API Key 留空也会使用已保存的 key。
          </div>
        </div>
        <div className="flex shrink-0 gap-2">
          <Button type="button" variant="subtle" size="sm" onClick={onDiscoverModels} disabled={!canDiscoverModels || isDiscovering}>
            {isDiscovering ? <Loader2 size={13} className="animate-spin" /> : <RefreshCw size={13} />}
            获取
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
            测试
          </Button>
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
        <div className="settings-expand-enter mt-3 max-h-52 space-y-1 overflow-y-auto pr-1 custom-scrollbar">
          {discoveredModels.map((model) => {
            const selected = model.id === draft.model_id;
            return (
              <button
                key={model.id}
                type="button"
                onClick={() => onChooseDiscoveredModel(model)}
                className={`motion-row flex w-full items-center justify-between gap-3 rounded-xl px-3 py-2 text-left text-xs ${
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
  );
}
