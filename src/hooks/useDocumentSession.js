import { useEffect, useRef, useState } from 'react';
import { createDocumentWorker } from '../components/editor/document/workerClient.js';
import { draftOperation } from '../components/editor/document/drafts.js';
import { notifyDocumentHealth, registerDocumentSession } from '../components/editor/document/registry.js';

export default function useDocumentSession(content, onChange, path, userId) {
  const [view, setView] = useState(null), [failure, setFailure] = useState(''), [recovery, setRecovery] = useState(null);
  const [archives, setArchives] = useState([]), [restart, setRestart] = useState(0);
  const state = useRef({ base: content, source: content, pending: false, revision: 0, active: true });
  const editorRef = useRef(null), worker = useRef(null), timer = useRef(null);
  const callback = useRef(onChange); callback.current = onChange;
  const saveDraft = (s = state.current) => {
    return draftOperation(userId, path, 'put', { base: s.base, source: s.source, page: s.view?.page || 0, patch: s.pending ? s.patch : null, at: Date.now() })
      .catch(() => { if (s.active && state.current === s) setFailure('浏览器草稿存储不可用，请保持页面打开并保存文稿。'); });
  };
  async function flush() {
    clearTimeout(timer.current);
    const s = state.current, editor = editorRef.current;
    if (s.flight) { await s.flight; if (s.active && s.pending) return flush(); return; }
    if (!s.pending || !editor || editor.isDestroyed || !worker.current) return;
    if (editor.view.composing) return new Promise((resolve, reject) => {
      editor.view.dom.addEventListener('compositionend', () => setTimeout(() => flush().then(resolve, reject), 0), { once: true });
    });
    const client = worker.current;
    s.flight = (async () => {
      while (s.pending && s.active) {
        const revision = s.revision, patch = s.patch;
        try {
          const result = await client.call('update', patch);
          if (!s.active || state.current !== s) return;
          s.view = result; s.source = result.source;
          if (revision !== s.revision) { s.patch = { ...s.patch, range: result.range }; continue; }
          s.pending = false; s.patch = null;
          setView(result); setFailure(''); callback.current(result.source);
          notifyDocumentHealth(path, result.status, result.source); await saveDraft(s);
        } catch (error) {
          await saveDraft(s); if (!s.active || state.current !== s) throw error;
          setFailure(error.message);
          notifyDocumentHealth(path, { category: error.category || 'editor_failure', issues: [{ code: 'roundtrip', message: error.message }] }, s.source);
          throw error;
        }
      }
    })();
    try { await s.flight; } finally { s.flight = null; }
  }
  function update(editor) {
    const s = state.current;
    editorRef.current = editor; s.pending = true; s.revision++;
    s.patch = { range: { ...s.view.range }, doc: editor.getJSON() };
    saveDraft(); clearTimeout(timer.current);
    timer.current = setTimeout(() => flush().catch(() => {}), 180);
  }
  async function viewOperation(action) {
    const s = state.current, client = worker.current, editor = editorRef.current, focused = editor?.isFocused;
    if (s.operation) return;
    s.operation = true; editor?.setEditable(false, false);
    const current = () => s.active && state.current === s && worker.current === client;
    try { return await action(s, client, current); }
    catch (error) { if (current()) setFailure(error.message); }
    finally {
      s.operation = false;
      if (current() && !editor?.isDestroyed) {
        editor?.setEditable(true, false);
        if (focused) editor?.commands.focus();
      }
    }
  }
  function navigate(page) {
    return viewOperation(async (s, client, current) => {
      await flush(); if (!current()) return;
      const result = await client.call('page', { page });
      if (!current()) return;
      s.view = result; setView(result); return result;
    });
  }
  function undo(redo = false) {
    return viewOperation(async (s, client, current) => {
      await flush(); if (!current()) return;
      const result = await client.call('undo', { redo });
      if (!result || !current()) return;
      s.view = result; s.source = result.source;
      setView(result); callback.current(result.source); saveDraft(s);
      if (current()) editorRef.current?.commands.setContent(result.doc, { emitUpdate: false });
    });
  }
  async function recover() {
    if (state.current.pending) { setFailure('当前还有新编辑，请先保存后再恢复历史草稿。'); return; }
    if (!recovery || (recovery.base !== content && recovery.source !== content)) {
      setFailure('磁盘版本已变化，草稿已保留。请先检查历史版本，避免覆盖新的正文。'); return;
    }
    return viewOperation(async (s, client, current) => {
      await client.call('init', { source: recovery.source }); if (!current()) return;
      let result = await client.call('page', { page: recovery.page || 0 }); if (!current()) return;
      if (recovery.patch) result = await client.call('update', { ...recovery.patch, range: result.range });
      if (!current()) return;
      const source = result.source ?? recovery.source;
      s.view = result; s.source = source; setView(result);
      editorRef.current?.commands.setContent(result.doc, { emitUpdate: false });
      callback.current(source); saveDraft(s);
      if (current()) setRecovery(null);
    });
  }
  useEffect(() => {
    const s = { base: content, source: content, pending: false, revision: 0, active: true };
    state.current = s; setView(null); setFailure(''); setRecovery(null);
    const client = createDocumentWorker(); worker.current = client;
    client.call('init', { source: content }).then(async result => {
      const draft = await draftOperation(userId, path, 'get');
      // Archive an outstanding draft before exposing a new editable session.
      // Starting a different edit can never overwrite the recovery payload.
      if (draft && (draft.source !== content || draft.patch)) await draftOperation(userId, path, 'archive', draft);
      const saved = await draftOperation(userId, path, 'archives');
      if (!s.active || state.current !== s) return;
      s.view = result; setView(result); notifyDocumentHealth(path, result.status, content);
      setArchives(saved || []);
      if (draft && (draft.source !== content || draft.patch)) setRecovery(draft);
    }).catch(error => { if (s.active && state.current === s) setFailure(error.message); });
    const unregister = registerDocumentSession(path, { flush, isPending: () => s.pending });
    const beforeUnload = event => { if (s.pending) { event.preventDefault(); event.returnValue = ''; saveDraft(); } };
    window.addEventListener('beforeunload', beforeUnload);
    return () => {
      if (s.pending) saveDraft(s);
      s.active = false; clearTimeout(timer.current); client.destroy(); unregister();
      window.removeEventListener('beforeunload', beforeUnload);
    };
  }, [path, userId, restart]);
  const locateNote = identifier => worker.current.call('locateNote', { identifier });
  const retry = () => {
    if (!view) { setRestart(value => value + 1); return; }
    return viewOperation(async (s, oldClient, current) => {
      const client = createDocumentWorker();
      try {
        await client.call('init', { source: s.source });
        let result = await client.call('page', { page: s.view.page });
        if (s.pending) result = await client.call('update', { ...s.patch, range: result.range });
        if (!current()) { client.destroy(); return; }
        s.view = result; s.source = result.source ?? s.source; s.pending = false; s.patch = null;
        oldClient.destroy(); worker.current = client;
        setView(result); setFailure(''); callback.current(s.source);
        notifyDocumentHealth(path, result.status, s.source); await saveDraft(s);
      } catch (error) { client.destroy(); throw error; }
      finally { if (s.active && state.current === s) editorRef.current?.setEditable(true, false); }
    });
  };
  return { view, update, flush, failure, editorRef, navigate, undo, recovery, recover, locateNote, retry, archives, selectRecovery: setRecovery };
}
