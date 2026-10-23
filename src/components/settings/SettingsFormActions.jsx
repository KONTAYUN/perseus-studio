import React from 'react';
import { Loader2 } from 'lucide-react';
import { Button } from './SettingsUI';

export function SettingsFeedback({ error, notice }) {
  return <>
    {error && <p role="alert" className="settings-feedback settings-feedback-error">{error}</p>}
    {notice && <p role="status" className="settings-feedback">{notice}</p>}
  </>;
}

export default function SettingsFormActions({ onCancel, saving, label, disabled = false }) {
  return <div className="settings-form-actions">
    <span>修改后需保存才会生效</span>
    <Button variant="secondary" onClick={onCancel} disabled={saving || disabled}>取消</Button>
    <Button type="submit" disabled={saving || disabled} data-testid="settings-form-save">
      {saving && <Loader2 size={15} className="animate-spin" />}{saving ? '保存中…' : label}
    </Button>
  </div>;
}
