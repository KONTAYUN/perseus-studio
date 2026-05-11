import { attachmentFingerprint, detectFileType } from './ideUtils.js';

export function filesToChatAttachments(fileList, randomSuffix = () => Math.random().toString(16).slice(2)) {
  const seenInBatch = new Set();
  return Array.from(fileList || [])
    .filter((file) => {
      const key = attachmentFingerprint(file);
      if (seenInBatch.has(key)) return false;
      seenInBatch.add(key);
      return true;
    })
    .map((file) => ({
      id: `${attachmentFingerprint(file)}-${randomSuffix(file)}`,
      file,
      name: file.name,
      size: file.size,
      type: file.type || 'unknown',
      fingerprint: attachmentFingerprint(file),
    }));
}

export function mergeChatAttachments(current, nextFiles) {
  const existing = new Set((current || []).map((item) => item.fingerprint || attachmentFingerprint(item)));
  const filtered = (nextFiles || []).filter((item) => !existing.has(item.fingerprint));
  return filtered.length > 0 ? [...(current || []), ...filtered] : current;
}

export function filesFromClipboard(clipboardData, now = Date.now()) {
  const files = [];
  for (const item of Array.from(clipboardData?.items || [])) {
    if (item.kind !== 'file') continue;
    const file = item.getAsFile();
    if (!file) continue;
    const extension = file.type.split('/')[1] || 'png';
    const name = file.name || `clipboard-${new Date(now).toISOString().replace(/[:.]/g, '-')}.${extension}`;
    files.push(new File([file], name, { type: file.type, lastModified: now }));
  }
  return files;
}

export function normalizeWorkspaceReferences(refs) {
  const items = Array.isArray(refs) ? refs : [refs];
  return items
    .filter((item) => item?.path)
    .map((item) => ({
      id: item.id || `workspace_${item.path}`,
      name: item.name || item.path.split('/').pop() || item.path,
      path: item.path,
      type: 'workspace_file',
      kind: 'workspace_file',
      file_type: item.file_type || item.fileType || detectFileType(item.path),
    }));
}

export function mergeWorkspaceReferences(current, refs) {
  const normalized = normalizeWorkspaceReferences(refs);
  if (normalized.length === 0) return current;
  const existing = new Set((current || []).map((item) => item.path));
  const next = normalized.filter((item) => !existing.has(item.path));
  return next.length > 0 ? [...(current || []), ...next] : current;
}

export function normalizeSkillId(id) {
  let value = String(id || '').trim();
  while (value.startsWith('skill_skill_')) {
    value = value.slice('skill_'.length);
  }
  return value;
}

export function skillToReference(skill) {
  if (!skill?.id || !skill.enabled) return null;
  const skillId = normalizeSkillId(skill.id);
  return {
    id: `skill_ref_${skillId}`,
    name: skill.name,
    description: skill.description || '',
    type: 'selected_skill',
    kind: 'selected_skill',
    skill_id: skillId,
  };
}

export function mergeSkillReference(current, skill) {
  const reference = skillToReference(skill);
  if (!reference) return current;
  if ((current || []).some((item) => normalizeSkillId(item.skill_id) === reference.skill_id)) return current;
  return [...(current || []), reference];
}
