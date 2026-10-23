import React, { useState } from 'react';
import { Button } from '../ui';
import useWorkspaceChangeReview from '../../hooks/useWorkspaceChangeReview.js';

const operationLabels = { create: '新增', update: '修改', move: '移动并修改', delete: '删除' };

export default function WorkspaceChangeReview({ message, ide }) {
  const seen = new Set();
  const changes = (message.attachments || []).flatMap((event) => {
    if (!event.tool_call_id || event.status !== 'success') return [];
    return (event.changes || []).flatMap((change, index) => {
      const key = `${event.tool_call_id}:${index}`;
      if (seen.has(key)) return [];
      seen.add(key);
      return [{ ...change, index, toolCallId: event.tool_call_id, key }];
    });
  });
  if (!changes.length) return null;
  return (
    <div className="workspace-change-list" aria-label="文件变更">
      {changes.map((change) => <ChangeRow key={change.key} change={change} ide={ide} />)}
    </div>
  );
}

function ChangeRow({ change, ide }) {
  const [open, setOpen] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const { preview, busy, error, load, revert } = useWorkspaceChangeReview(change.toolCallId, change.index, ide.refreshWorkspaceAfterAssistantTools);
  const completed = preview?.revert_state === 'completed';
  return (
    <div className="workspace-change-row">
      <Button size="xs" variant="ghost" className="workspace-change-toggle" aria-expanded={open}
        onClick={() => { setOpen(!open); setConfirming(false); if (!open) load(); }}>
        <span>{operationLabels[change.operation] || '变更'}</span>
        <span className="workspace-change-path">{change.old_path ? `${change.old_path} → ` : ''}{change.path}</span>
        <span>{open ? '收起' : '查看变更'}</span>
      </Button>
      {open && (
        <div className="workspace-change-preview">
          {preview && !completed && <div className="workspace-change-columns">
            <ContentPreview title="修改前" content={preview.before} />
            <ContentPreview title="当前文件" content={preview.current} />
          </div>}
          {completed && <p role="status">已撤销此处变更。</p>}
          {preview?.reason && !completed && <p>{preview.reason}</p>}
          {error && <p className="workspace-change-error" role="alert">{error}</p>}
          <div className="workspace-change-actions">
            {preview?.can_revert && !completed && <>
              {confirming && <span>确认恢复修改前内容？新建文件会移入回收站。</span>}
              <Button size="xs" variant={confirming ? 'dangerSubtle' : 'secondary'} disabled={busy || ide.isChatSending}
                onClick={async () => { if (!confirming) { setConfirming(true); return; } await revert(); setConfirming(false); }}>
                {busy ? '处理中…' : confirming ? '确认撤销' : '撤销这一处'}
              </Button>
              {confirming && <Button size="xs" variant="ghost" disabled={busy} onClick={() => setConfirming(false)}>取消</Button>}
            </>}
            <Button size="xs" variant="ghost" disabled={busy} onClick={() => { setConfirming(false); load(); }}>{busy ? '加载中…' : '刷新状态'}</Button>
          </div>
        </div>
      )}
    </div>
  );
}

function ContentPreview({ title, content = '' }) {
  const text = content.slice(0, 12_000);
  return <div className="workspace-change-content"><div>{title}</div><pre>{text || '（无内容）'}{content.length > text.length ? '\n…预览已截断' : ''}</pre></div>;
}
