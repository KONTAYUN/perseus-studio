import { useCallback, useContext, useRef, useState } from 'react';
import { SettingsNavigation } from '../components/settings/SettingsNavigation';

// A form owns its draft until a successful write or an explicit discard.
export default function useSettingsEditor({ initial, toDraft, save, busy = false }) {
  const [opened, setOpened] = useState(false);
  const [draft, setDraft] = useState(initial);
  const [baseline, setBaseline] = useState(JSON.stringify(initial));
  const [editingId, setEditingId] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const submitting = useRef(false);
  const reset = useCallback(() => {
    setOpened(false); setDraft(initial); setBaseline(JSON.stringify(initial));
    setEditingId(''); setError('');
  }, [initial]);
  const dirty = opened && JSON.stringify(draft) !== baseline;
  const navigation = useContext(SettingsNavigation);
  const leave = (action) => navigation ? navigation.attemptLeave(action) : action();
  const open = (item) => leave(() => {
    const next = item ? toDraft(item) : initial;
    setDraft(next); setBaseline(JSON.stringify(next)); setEditingId(item?.id || '');
    setError(''); setNotice(''); setOpened(true);
  });
  const cancel = () => leave(reset);
  const submit = async (event) => {
    event.preventDefault();
    if (submitting.current || busy) return;
    submitting.current = true; setSaving(true); setError(''); setNotice('');
    try {
      const result = await save(draft, editingId);
      reset();
      setNotice(result?.refreshError
        ? `配置已保存，但列表刷新失败：${result.refreshError}。请刷新列表，无需重复保存。`
        : '配置已保存。');
    } catch (failure) { setError(failure.message || '保存失败，请重试。'); }
    finally { submitting.current = false; setSaving(false); }
  };
  return { opened, draft, setDraft, editingId, saving, dirty, error, notice, setNotice, open, cancel, submit, discard: reset };
}
