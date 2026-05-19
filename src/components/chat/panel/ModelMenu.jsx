import React from 'react';
import { Check, ChevronDown, ChevronRight } from 'lucide-react';
import { AnchoredPopover, Button } from '../../ui';

const thinkingModes = [
  ['standard', '标准'],
  ['deep', '深度'],
  ['expert', '专家'],
];

export default function ModelMenu({ ide, panelId }) {
  const modelButtonRef = React.useRef(null);
  const modelSubmenuButtonRef = React.useRef(null);
  const [isModelSubmenuOpen, setIsModelSubmenuOpen] = React.useState(false);
  const currentThinkingLabel = thinkingModes.find(([value]) => value === ide.thinkingMode)?.[1] || '标准';

  React.useEffect(() => {
    if (!ide.isModelMenuOpen) setIsModelSubmenuOpen(false);
  }, [ide.isModelMenuOpen]);

  return (
    <div className="relative min-w-0 max-w-full">
      <button
        ref={modelButtonRef}
        data-testid="chat-model-button"
        onClick={(e) => {
          e.stopPropagation();
          ide.setActivePanel(panelId);
          ide.setIsModelMenuOpen(!ide.isModelMenuOpen);
        }}
        className="motion-button flex min-w-0 max-w-full items-center justify-center gap-1.5 overflow-hidden rounded-lg px-1.5 py-1 text-sm font-semibold text-gray-800 hover:text-blue-500 dark:text-gray-200 dark:hover:text-blue-400"
      >
        <span className="block min-w-0 truncate text-center">
          {ide.selectedModel?.name || '未配置模型'} · {currentThinkingLabel}
        </span>
        <ChevronDown size={14} className="flex-shrink-0 text-gray-400" />
      </button>

      <AnchoredPopover
        anchorRef={modelButtonRef}
        align="center"
        width={208}
        minWidth={208}
        maxHeight={520}
        open={ide.isModelMenuOpen}
        onClose={() => ide.setIsModelMenuOpen(false)}
        testId="chat-model-menu"
        className="!overflow-visible p-1.5"
      >
        {ide.enabledModels.length === 0 ? (
          <div className="p-4 text-center">
            <div className="text-sm font-bold text-gray-800 dark:text-gray-100">未配置模型</div>
            <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">请先在设置页添加并启用模型。</div>
            <Button
              data-testid="open-settings-from-model-menu"
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                ide.setIsModelMenuOpen(false);
                ide.setIsSettingsOpen(true);
              }}
              className="mt-3"
              size="sm"
            >
              打开设置
            </Button>
          </div>
        ) : (
          <>
            <div className="px-2 pb-1 pt-1 text-[11px] font-semibold text-gray-400">思考强度</div>
            {thinkingModes.map(([value, label]) => (
              <button
                key={value}
                data-testid={`thinking-mode-${value}`}
                onClick={(e) => {
                  e.stopPropagation();
                  ide.setThinkingMode?.(value);
                  ide.setIsModelMenuOpen(false);
                }}
                className={`motion-row flex w-full items-center justify-between rounded-xl px-3 py-2 text-left text-sm font-semibold ${
                  (ide.thinkingMode || 'standard') === value
                    ? 'bg-blue-50 text-blue-600 dark:bg-blue-500/15 dark:text-blue-300'
                    : 'text-gray-700 hover:bg-gray-50 dark:text-gray-200 dark:hover:bg-white/5'
                }`}
              >
                <span>{label}</span>
                {(ide.thinkingMode || 'standard') === value && <Check size={14} />}
              </button>
            ))}
            <div className="my-1 border-t border-gray-100 dark:border-white/10" />
            <div>
              <button
                ref={modelSubmenuButtonRef}
                type="button"
                data-testid="model-submenu-trigger"
                onClick={(e) => {
                  e.stopPropagation();
                  ide.setActivePanel(panelId);
                  setIsModelSubmenuOpen((current) => !current);
                }}
                className="motion-row flex w-full items-center justify-between gap-3 rounded-xl px-3 py-2 text-left text-sm font-semibold text-gray-700 hover:bg-gray-50 dark:text-gray-200 dark:hover:bg-white/5"
              >
                <span className="min-w-0 truncate">{ide.selectedModel?.name || '未配置模型'}</span>
                <ChevronRight size={14} className="shrink-0 text-gray-400" />
              </button>
              <AnchoredPopover
                anchorRef={modelSubmenuButtonRef}
                align="start"
                placement="right"
                gap={8}
                width={256}
                minWidth={256}
                maxHeight={420}
                open={ide.isModelMenuOpen && isModelSubmenuOpen}
                onClose={() => setIsModelSubmenuOpen(false)}
                showBackdrop={false}
                zIndex={460}
                testId="model-submenu"
                className="p-1.5"
              >
                {ide.enabledModels.map((model) => (
                  <button
                    key={model.id}
                    data-testid={`model-option-${model.id}`}
                    onClick={(e) => {
                      e.stopPropagation();
                      ide.setSelectedModelId(model.id);
                      setIsModelSubmenuOpen(false);
                      ide.setIsModelMenuOpen(false);
                    }}
                    className={`motion-row flex w-full items-center gap-2 rounded-xl px-3 py-2.5 text-left ${
                      ide.selectedModel?.id === model.id
                        ? 'bg-blue-50 text-blue-600 dark:bg-blue-500/15 dark:text-blue-300'
                        : 'text-gray-700 hover:bg-gray-50 dark:text-gray-200 dark:hover:bg-white/5'
                    }`}
                  >
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-bold">{model.name}</span>
                      <span className="mt-0.5 block truncate text-[11px] text-gray-400">
                        {model.protocol} · {model.model_id}
                      </span>
                    </span>
                    {ide.selectedModel?.id === model.id && <Check size={14} className="shrink-0" />}
                  </button>
                ))}
              </AnchoredPopover>
            </div>
          </>
        )}
      </AnchoredPopover>
    </div>
  );
}
