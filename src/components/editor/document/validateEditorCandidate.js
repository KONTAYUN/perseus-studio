import { getSchema } from '@tiptap/core';
import { documentExtensions } from './editorExtensions.js';
import { DocumentSession } from './session.js';
import { PreservedBlock, PreservedInline } from '../richText/PreservedMarkdown.js';
import { InlineMath, BlockMath, DocumentMetadata, FootnoteReference } from '../richText/DocumentNodes.js';

let schema;
export function validateEditorCandidate(source) {
  schema ||= getSchema(documentExtensions(null, [PreservedBlock, PreservedInline], [InlineMath, BlockMath, DocumentMetadata, FootnoteReference]));
  const session = new DocumentSession(source), node = schema.nodeFromJSON(session.doc);
  node.check();
  session.update(node.toJSON());
  return session.status;
}
