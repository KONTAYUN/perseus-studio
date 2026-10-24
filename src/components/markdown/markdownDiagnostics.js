// Markdown delimiters are permissive text, not evidence of corruption. The
// editor reports conversion invariants separately through its health registry.
export function diagnoseMarkdown(source = '') {
  const at = source.indexOf('\0');
  return at < 0 ? [] : [{ code: 'nul_character', line: source.slice(0, at).split('\n').length, message: '文稿包含无法可靠保存的空字符' }];
}
export function buildMarkdownRepairPrompt({ path, hash, issues }) {
  return `请检查文稿格式：${JSON.stringify(path)}。基础哈希：${hash}。诊断：${JSON.stringify(issues)}。只提交最小格式修复候选，保留正文、数值、链接、代码与特殊内容。以工具返回的暂存或应用状态为准。`;
}
