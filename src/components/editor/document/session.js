import { parseDocument } from './fromMarkdown.js';
import { serializeBlock } from './toMarkdown.js';
import { semanticKey } from './normalize.js';
import { classifyDocument, DocumentFormatError, DOCUMENT_PROFILE } from './profile.js';
import { prepareReferences } from './references.js';

export class DocumentSession {
  constructor(source = '') {
    const parsed = parseDocument(source);
    this.source = source; this.doc = parsed.doc; this.entries = parsed.entries;
    this.revision = 0; this.status = classifyDocument(this.doc);
    this.originalKeys = this.entries.map(entry => semanticKey(entry.node));
    this.cache = new Map();
    this.history = []; this.future = []; this.historyTime = 0;
  }

  candidate(doc) {
    if (semanticKey(doc) === semanticKey(this.entries.length ? { type: 'doc', content: this.entries.map(entry => entry.node) } : { type: 'doc', content: [{ type: 'paragraph' }] })) return this.source;
    const prepared = prepareReferences(doc); doc = prepared.doc;
    const keys = (doc.content || []).map(semanticKey);
    const queues = new Map();
    this.originalKeys.forEach((key, index) => { if (!queues.has(key)) queues.set(key, []); queues.get(key).push(index); });
    // Reserve source identities first. Identical paragraphs may use different
    // original spelling; inserting a copy must not steal another block's raw.
    const used = new Set();
    const matches = keys.map((key, index) => {
      if (prepared.affected.has(index)) return undefined;
      const id = doc.content[index].attrs?.sourceId;
      const match = /^root-(\d+)$/.exec(id || '');
      const original = match ? Number(match[1]) : undefined;
      if (original !== undefined && this.originalKeys[original] === key && !used.has(original)) { used.add(original); return original; }
      return undefined;
    });
    keys.forEach((key, index) => {
      if (prepared.affected.has(index)) return;
      if (matches[index] !== undefined || doc.content[index].attrs?.sourceId) return;
      const queue = queues.get(key) || []; while (queue.length && used.has(queue[0])) queue.shift();
      const match = queue.shift(); if (match !== undefined) { matches[index] = match; used.add(match); }
    });
    if (keys.length === this.originalKeys.length && matches.every((match, index) => match === index)) return this.source;
    const eol = this.source.includes('\r\n') ? '\r\n' : '\n';
    let result = this.entries.length ? this.source.slice(0, this.entries[0].from) : '';
    doc.content.forEach((node, index) => {
      const match = matches[index];
      const previous = matches[index - 1];
      if (index) result += match !== undefined && previous !== undefined && match === previous + 1
        ? this.source.slice(this.entries[previous].to, this.entries[match].from) : eol + eol;
      if (match !== undefined) result += this.source.slice(this.entries[match].from, this.entries[match].to);
      else {
        const key = JSON.stringify(node);
        if (!this.cache.has(key)) this.cache.set(key, serializeBlock(node).replace(/\r?\n/g, eol));
        result += this.cache.get(key);
      }
    });
    const last = matches.at(-1);
    result += last === this.entries.length - 1 && last !== undefined ? this.source.slice(this.entries[last].to)
      : /\r?\n$/.test(this.source) ? eol : '';
    const reparsed = parseDocument(result).doc;
    if (semanticKey(reparsed) !== semanticKey(doc)) {
      throw new DocumentFormatError('这次格式调整暂时无法完整保存，已保留编辑草稿。', { profile: DOCUMENT_PROFILE });
    }
    return result;
  }

  update(doc, page = 0) {
    const source = this.candidate(doc);
    if (semanticKey(this.doc) !== semanticKey(doc)) {
      const now = Date.now();
      const previous = this.history.at(-1);
      if (previous && now - this.historyTime < 650 && previous.page === page) previous.after = doc;
      else this.history.push({ before: this.doc, after: doc, page });
      if (this.history.length > 100) this.history.shift();
      this.historyTime = now; this.future = [];
    }
    this.doc = doc; this.revision++;
    this.status = classifyDocument(doc);
    return source;
  }

  undo(redo = false) {
    const entry = (redo ? this.future : this.history).pop();
    if (!entry) return null;
    (redo ? this.history : this.future).push(entry);
    this.doc = redo ? entry.after : entry.before;
    this.historyTime = 0; this.revision++;
    return { doc: this.doc, page: entry.page, source: this.candidate(this.doc) };
  }
}
