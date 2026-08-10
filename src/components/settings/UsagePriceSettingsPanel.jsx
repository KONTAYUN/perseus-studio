import React from 'react';
import { createPortal } from 'react-dom';
import { ArrowLeft, Coins, Plus, Trash2 } from 'lucide-react';
import {
  Button,
  EmptyCard,
  Field,
  FormHeader,
  IconButton,
  Input,
  ProviderCard,
  SectionTitle,
  Select,
} from './SettingsUI';
import { priceMicroToDisplay, providerLabel } from './usageFormatting';
import useMotionPresence from '../../hooks/useMotionPresence';

export default function UsagePriceSettingsPanel({
  open = false,
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
  const { exiting, present } = useMotionPresence(open, { exitToken: '--motion-modal' });
  if (!present) return null;

  const panel = (
    <div className={`motion-fade-reveal fixed inset-0 z-[120] flex items-center justify-center bg-black/35 p-4 backdrop-blur-sm dark:bg-black/55 ${exiting ? 'opacity-0' : 'opacity-100'}`}>
      <div className={`settings-modal-card ui-overlay-surface motion-modal flex max-h-[82vh] w-full max-w-3xl flex-col overflow-hidden border border-white/40 bg-white/95 dark:border-white/10 dark:bg-[#1C1C1E]/95 ${exiting ? 'motion-presence-exit motion-presence-exit-modal' : ''}`}>
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

  if (typeof document === 'undefined') return panel;
  return createPortal(panel, document.body);
}
