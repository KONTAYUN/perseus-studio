import { buildMarkdownRepairPrompt, diagnoseMarkdown } from '../components/markdown/markdownDiagnostics.js';
import { DOCUMENT_PROFILE } from '../components/editor/document/profile.js';

export async function submitMarkdownRepair({ snapshot, getCurrent, prepareFile, send }) {
  function verify() {
    const current = getCurrent();
    if (!current || current.path !== snapshot.path || current.content !== snapshot.content
        || current.conversationId !== snapshot.conversationId || current.modelId !== snapshot.modelId) {
      throw new Error('文件、内容或当前对话已变化，请重新检查后再修复。');
    }
    if (!current.modelId) throw new Error('请先选择可用模型，再请求修复。');
    if (current.conflict) throw new Error('文件存在保存冲突，请先处理冲突，再请求修复。');
  }
  verify();
  const issues = snapshot.issues?.length ? snapshot.issues : diagnoseMarkdown(snapshot.content);
  if (!issues.length) throw new Error('当前未发现可检查的格式问题，无需发送修复请求。');
  const saved = await prepareFile(snapshot);
  verify();
  const accepted = await send(buildMarkdownRepairPrompt({ path: snapshot.path, hash: saved.hash, issues }), {
    documentAction: { type: 'format_repair', path: snapshot.path, base_hash: saved.hash, profile_version: DOCUMENT_PROFILE, diagnostics: issues },
  });
  if (accepted === false) throw new Error('修复请求未发送，原稿和聊天输入已保留。');
}
