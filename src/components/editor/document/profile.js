import profile from '../../../../backend/internal/document/profile.json' with { type: 'json' };
export const DOCUMENT_PROFILE = profile.version;
export const MAX_DOCUMENT_BYTES = profile.max_bytes;
export const ROOT_PAGE_SIZE = 160;

export class DocumentFormatError extends Error {
  constructor(message, detail = {}) {
    super(message);
    this.name = 'DocumentFormatError';
    this.category = 'format_issue';
    this.detail = detail;
  }
}

// These are editor invariants, not guesses about punctuation in user prose.
export function classifyDocument(document) {
  let preserved = false;
  function visit(node) {
    if ((node.type === 'preservedBlock' && node.attrs?.label !== '链接定义') || ['preservedInline', 'preservedSpan'].includes(node.type)) preserved = true;
    for (const child of node.content || []) visit(child);
  }
  visit(document);
  return { profile: DOCUMENT_PROFILE, category: preserved ? 'preserved' : 'compatible', issues: [] };
}
