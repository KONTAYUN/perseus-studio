import React, { useCallback, useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { Check, Clock3, FileText, History, Loader2, RotateCcw, ShieldCheck, Sparkles, X } from 'lucide-react';
import { apiGet, apiPost } from '../../api/client.js';
import { Button } from '../ui';
import useModalFocus from '../../hooks/useModalFocus';
import MarkdownContent from '../markdown/MarkdownContent.jsx';
import { validateEditorCandidate } from './document/validateEditorCandidate.js';
import { hasPendingDocumentSessions } from './document/registry.js';

export default function DocumentHistoryDialog({ editor, onClose }) {
  const modal = useRef(null), generation = useRef(0);
  const path = editor.currentFilePath;
  const latest = useRef(editor); latest.current = editor;
  const originalPath = useRef(path);
  const [versions, setVersions] = useState([]), [repairs, setRepairs] = useState([]);
  const [selected, setSelected] = useState(null), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true), [previewId, setPreviewId] = useState('');
  const backdropPress = useRef(false);
  const dismiss = useCallback(() => { if (!busy) onClose(); }, [busy, onClose]);
  useModalFocus(modal, true, dismiss, '[data-history-close]');
  useEffect(() => {
    if (path !== originalPath.current) { onClose(); return; }
    const controller = new AbortController();
    Promise.all([apiGet(`/api/documents/versions?path=${encodeURIComponent(path)}`, { signal: controller.signal }),
      apiGet(`/api/documents/repairs?path=${encodeURIComponent(path)}`, { signal: controller.signal })])
      .then(([history, candidates]) => {
        if (controller.signal.aborted) return;
        setVersions(history.items); setRepairs(candidates.items);
        if (history.items.length) preview(history.items[0]);
      })
      .catch(failure => { if (!controller.signal.aborted) setError(failure.message); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => { controller.abort(); generation.current++; };
  }, [path]);
  async function preview(version) {
    const revision = ++generation.current; setError(''); setSelected(null); setPreviewId(version.id);
    try {
      const item = await apiGet(`/api/documents/versions?path=${encodeURIComponent(path)}&version_id=${encodeURIComponent(version.id)}`);
      if (generation.current === revision) setSelected({ ...item, kind: 'version' });
    } catch (failure) { if (generation.current === revision) setError(failure.message); }
    finally { if (generation.current === revision) setPreviewId(''); }
  }
  async function apply(accept = true) {
    if (!selected || busy) return;
    setBusy(true); setError('');
    const snapshot = () => latest.current.getCurrentDocumentSnapshot();
    const verify = base => {
      const current = snapshot();
      if (current.path !== path || current.conflict || hasPendingDocumentSessions(path)
          || (base && current.content !== base.content)) throw new Error('文稿或本地编辑已变化，请关闭弹窗后重新核对。');
      return current;
    };
    try {
      await editor.flushAutoSave();
      const base = verify();
      const head = await apiGet(`/api/files/read?path=${encodeURIComponent(path)}`);
      // The same frontend adapter must accept the proposed document before it
      // reaches the backend CAS boundary. Backend validation is independent.
      if (accept) validateEditorCandidate(selected.content);
      verify(base);
      if (head.content !== base.content) throw new Error('磁盘版本与本地编辑不同，请先处理保存冲突。');
      if (selected.kind === 'repair') await apiPost('/api/documents/repairs', { id: selected.id, expected_hash: head.hash, accept });
      else await apiPost('/api/documents/restore', { path, version_id: selected.id, expected_hash: head.hash });
      verify(base);
      if (accept) await latest.current.loadFileIntoEditor(path, { preserveLocalEdits: true });
      onClose();
    } catch (failure) { setError(failure.message); }
    finally { setBusy(false); }
  }
  const activeId = previewId || selected?.id;
  const filename = path.split('/').at(-1);
  return createPortal(<div className="modal-backdrop document-history-backdrop fixed inset-0 z-[500] flex items-center justify-center bg-black/45 p-4"
    onPointerDown={event => { backdropPress.current = event.target === event.currentTarget; }}
    onPointerCancel={() => { backdropPress.current = false; }}
    onClick={event => { if (backdropPress.current && event.target === event.currentTarget) dismiss(); backdropPress.current = false; }}>
    <div ref={modal} role="dialog" aria-modal="true" aria-label="文稿版本与格式建议" tabIndex={-1} className="ui-overlay-surface document-history-dialog">
      <header className="document-history-header">
        <div className="document-history-heading"><span className="document-history-symbol"><History size={20} strokeWidth={1.7} /></span>
          <div><h2>历史版本</h2><p title={path}><FileText size={12} /><span>{filename}</span></p></div>
        </div>
        <Button data-history-close className="document-history-close" variant="secondary" size="sm" disabled={busy} onClick={dismiss} aria-label="关闭历史版本"><X size={16} /><span>关闭</span></Button>
      </header>
      <div className="document-history-body">
        <nav className="document-history-sidebar custom-scrollbar" aria-label="文稿版本" aria-busy={loading}>
          {!!repairs.length && <div className="document-history-group"><h3><Sparkles size={13} />格式建议<span>{repairs.length}</span></h3>
            {repairs.map(item => <Button key={item.id} variant="ghost" disabled={busy} className="document-history-item" aria-pressed={activeId === item.id} aria-label={`格式建议 · 第 ${item.attempt} 次${!item.equivalent ? ' · 需核对正文' : ''}`}
              onClick={() => { generation.current++; setPreviewId(''); setError(''); setSelected({ ...item, kind: 'repair' }); }}>
              <Sparkles size={15} /><span><strong>格式建议 · 第 {item.attempt} 次</strong><small>{item.equivalent ? '格式调整' : '需要核对正文'}</small></span>{activeId === item.id && <Check size={13} />}
            </Button>)}
          </div>}
          <div className="document-history-group"><h3><Clock3 size={13} />保存记录<span>{versions.length}</span></h3>
            {versions.map((item, index) => <Button key={item.id} variant="ghost" disabled={busy} className="document-history-item" aria-pressed={activeId === item.id} onClick={() => preview(item)}>
              <span className="document-history-timeline-dot" /><span><strong>{versionDate(item.created_at)}</strong><small><time dateTime={item.created_at}>{versionTime(item.created_at)}</time><span>·</span>{versionSource(item)}</small></span>
              {index === 0 && <span className="document-history-latest">最近</span>}
            </Button>)}
          </div>
          {loading && <p className="document-history-sidebar-note" role="status">正在读取版本…</p>}
          {!loading && !versions.length && !repairs.length && <p className="document-history-sidebar-note">暂无保存记录</p>}
        </nav>
        <section className="document-history-preview" aria-label="版本预览" aria-busy={Boolean(previewId)}>
          <div className="document-history-preview-heading"><span>{selected?.kind === 'repair' ? '建议预览' : '版本预览'}</span>
            <span>{selected?.kind === 'version' ? `${versionDate(selected.created_at)} ${versionTime(selected.created_at)}` : '只读'}</span>
          </div>
          <div className="document-history-preview-scroll custom-scrollbar">
            {loading || previewId ? <div className="document-history-empty" role="status"><Loader2 size={25} className="animate-spin" /><strong>正在读取内容…</strong></div>
              : selected ? <>
                {selected.kind === 'repair' && <><p className="document-history-notice" role="status">这是暂存建议，原稿尚未修改。请检查内容后决定是否采用。</p><h3 className="document-history-section-title">当前原稿</h3><MarkdownContent content={editor.currentFileContent} /><h3 className="document-history-section-title">调整建议</h3></>}
                {selected.legacy && <p className="document-history-notice">这是早期按文件路径保存的快照，归属尚未确认，请在恢复前核对内容。</p>}
                <MarkdownContent content={selected.content} />
              </> : <div className="document-history-empty"><History size={30} strokeWidth={1.4} /><strong>{versions.length || repairs.length ? '选择一条记录查看内容' : '还没有历史版本'}</strong><p>{versions.length || repairs.length ? '在左侧选择版本或格式建议，即可在这里预览。' : '继续编辑并保存后，旧版本会出现在这里。'}</p></div>}
          </div>
        </section>
      </div>
      {error && <p role="alert" className="document-history-error">{error}</p>}
      <footer className="document-history-footer"><p><ShieldCheck size={15} /><span>恢复前会保留当前版本，方便再次找回。</span></p>
        <div>{selected?.kind === 'repair' && <Button variant="secondary" size="sm" disabled={busy} onClick={() => apply(false)}>不采用</Button>}
          <Button size="sm" disabled={!selected || busy || Boolean(previewId)} onClick={() => apply()}>{busy ? <Loader2 size={14} className="animate-spin" /> : <RotateCcw size={14} />}{busy ? '正在核对…' : selected?.kind === 'repair' ? '确认采用此建议' : '恢复此版本'}</Button>
        </div>
      </footer>
    </div>
  </div>, document.body);
}

function versionDate(value) {
  return new Date(value).toLocaleDateString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit' });
}
function versionTime(value) {
  return new Date(value).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
}
function versionSource(item) {
  if (item.legacy) return '旧路径快照';
  const labels = { manual_write: '手动保存', document_restore: '版本恢复', upload: '文件上传', document_repair: '格式调整' };
  return labels[item.source] || (item.source?.startsWith('ai_') ? '助手修改' : '保存快照');
}
