import { parseDocument } from '../../../src/components/editor/document/fromMarkdown.js';
import { DocumentSession } from '../../../src/components/editor/document/session.js';
export function documentAdapter() {
  const sources = new WeakMap();
  return {
    parse(source) { const { doc } = parseDocument(source); sources.set(doc, source); return doc; },
    serialize(doc) { return new DocumentSession(sources.get(doc) || '').update(doc); },
  };
}
