import { DocumentSession } from './session.js';
import { documentPages } from './pages.js';
let session, pages, page = 0;
function view(source) {
  const range = pages[page];
  return { source, page, pages: pages.length, range, status: session.status,
    doc: { type: 'doc', content: session.doc.content.slice(range.from, range.to) } };
}
self.onmessage = ({ data: { id, method, args } }) => {
  try {
    let result;
    if (method === 'init') {
      session = new DocumentSession(args.source); pages = documentPages(session.doc); result = view(args.source);
    } else if (method === 'update') {
      const doc = { type: 'doc', content: [...session.doc.content] };
      doc.content.splice(args.range.from, args.range.to - args.range.from, ...args.doc.content);
      const source = session.update(doc, page);
      pages[page].to += args.doc.content.length - (args.range.to - args.range.from);
      const delta = pages[page].to - args.range.to;
      for (let i = page + 1; i < pages.length; i++) { pages[i].from += delta; pages[i].to += delta; }
      result = view(source);
    } else if (method === 'page') {
      page = Math.max(0, Math.min(pages.length - 1, args.page)); result = view();
    } else if (method === 'locateNote') {
      const index = session.doc.content.findIndex(node => node.type === 'footnoteDefinition' && node.attrs.identifier === args.identifier);
      result = index < 0 ? null : pages.findIndex(range => index >= range.from && index < range.to);
    } else if (method === 'undo') {
      const restored = session.undo(args.redo);
      if (restored) { pages = documentPages(session.doc); page = Math.min(restored.page, pages.length - 1); result = view(restored.source); }
      else result = null;
    } else throw new Error('未知文稿操作');
    self.postMessage({ id, result });
  } catch (error) { self.postMessage({ id, error: { message: error.message, category: error.category || 'editor_failure' } }); }
};
