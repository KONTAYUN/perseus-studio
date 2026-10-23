import React from 'react';
import { ArrowLeft, Plus } from 'lucide-react';
import { Button, EmptyCard, Field, Input, Select } from './SettingsUI';
import { priceMicroToDisplay, providerLabel } from './usageFormatting';
import SettingsFormActions, { SettingsFeedback } from './SettingsFormActions';
import SettingsRowMenu from './SettingsRowMenu';

const priceFields = [
  ['input_price_micro', '输入 / 百万 tokens'], ['output_price_micro', '输出 / 百万 tokens'],
  ['cache_creation_price_micro', '缓存写入 / 百万 tokens'], ['cache_read_price_micro', '缓存读取 / 百万 tokens'],
];

export default function UsagePriceSettingsPanel({ providers, prices, priceDraft, editingPriceId, onClose,
  onSave, onReset, onDiscard, onDraftChange, onEdit, onDelete, saving, error }) {
  const [editing, setEditing] = React.useState(false);
  return <section className="settings-inline-price" data-testid="price-settings">
    <div className="settings-list-toolbar">
      <Button variant="ghost" onClick={onClose}><ArrowLeft size={15} />返回用量统计</Button>
      {!editing && <Button onClick={() => { onReset(); setEditing(true); }}><Plus size={15} />添加价格</Button>}
    </div>
    <SettingsFeedback error={error} />
    {editing ? <form className="settings-editor" aria-label="模型价格配置" onSubmit={async event => { if (await onSave(event)) setEditing(false); }}>
      <h2>{editingPriceId ? '编辑价格' : '添加价格'}</h2>
      <p className="settings-form-description">可选。配置后，未来调用会记录估算费用；历史费用不会重新计算。</p>
      <fieldset disabled={saving}>
        <Field label="模型配置"><Select value={priceDraft.model_provider_id} onChange={event => {
          const selected = providers.find(item => item.id === event.target.value);
          onDraftChange(draft => ({ ...draft, model_provider_id: event.target.value, model_id: selected?.model_id || draft.model_id }));
        }} required><option value="">选择模型配置</option>{providers.map(item => <option key={item.id} value={item.id}>{item.name} · {item.model_id}</option>)}</Select></Field>
        <Field label="Model ID"><Input value={priceDraft.model_id} onChange={event => onDraftChange(draft => ({ ...draft, model_id: event.target.value }))} required /></Field>
        <Field label="货币"><Input value={priceDraft.currency} onChange={event => onDraftChange(draft => ({ ...draft, currency: event.target.value.toUpperCase() }))} placeholder="USD" /></Field>
        <div className="grid gap-3 sm:grid-cols-2">{priceFields.map(([key, label]) => <Field label={label} key={key}>
          <Input type="number" min="0" step="0.000001" value={priceDraft[key]} onChange={event => onDraftChange(draft => ({ ...draft, [key]: event.target.value }))} />
        </Field>)}</div>
      </fieldset>
      <SettingsFormActions saving={saving} onCancel={() => onDiscard(() => setEditing(false))} label={editingPriceId ? '保存价格' : '添加价格'} />
    </form> : <>
      <p className="settings-form-description mb-5">模型价格用于费用估算，不影响模型调用。金额按每百万 tokens 计算。</p>
      {!prices.length ? <EmptyCard text="还没有配置模型价格。" /> : <div className="settings-provider-list">
        {prices.map(item => <div className="settings-provider-row" key={item.id}>
          <div className="settings-provider-summary"><div className="settings-provider-name"><span>{providerLabel(providers, item.model_provider_id)}</span></div>
            <p>{item.model_id} · {item.currency}</p><p>输入 {priceMicroToDisplay(item.input_price_micro) || '0'} / 输出 {priceMicroToDisplay(item.output_price_micro) || '0'}</p>
          </div>
          <SettingsRowMenu label={`价格操作 ${item.model_id}`} actions={[
            { label: '编辑价格', run: () => { onEdit(item); setEditing(true); } },
            { label: '删除价格', danger: true, run: () => onDelete(item) },
          ]} />
        </div>)}
      </div>}
    </>}
  </section>;
}
