import test from 'node:test';
import assert from 'node:assert/strict';
import { unified } from 'unified';
import remarkParse from 'remark-parse';
import remarkGfm from 'remark-gfm';
import remarkAutolinkBoundary from '../../src/components/markdown/remarkAutolinkBoundary.js';
import { createZipBlob } from '../../src/hooks/ideUtils.js';
import { apiGetBlob } from '../../src/api/client.js';

test('bare URL CJK boundaries preserve prose, explicit links and code', async () => {
  const source = 'https://example.com/资料，不证明事实。\n\n[中文](https://example.com/资料，合法)\n\n`https://example.com/x，code`\n\n<https://example.com/x，explicit>\n\nhttps://example.com/%E4%B8%AD';
  const processor = unified().use(remarkParse).use(remarkGfm).use(remarkAutolinkBoundary);
  const tree = await processor.run(processor.parse(source), { value: source });
  assert.equal(tree.children[0].children[0].url, 'https://example.com/资料');
  assert.equal(tree.children[0].children.slice(1).map(n => n.value).join(''), '，不证明事实。');
  assert.equal(tree.children[1].children[0].url, 'https://example.com/资料，合法');
  assert.equal(tree.children[2].children[0].type, 'inlineCode');
  assert.equal(tree.children[3].children[0].url, 'https://example.com/x，explicit');
  assert.equal(tree.children[4].children[0].url, 'https://example.com/%E4%B8%AD');
});

test('ZIP includes exact binary bytes and UTF-8 filenames', async () => {
  const bytes = new Uint8Array([0, 255, 128, 13, 10, 65]);
  const blob = createZipBlob([{ path: '中文.pdf', bytes }, { path: 'draft.md', content: '# 标题\r\n' }]);
  const data = new Uint8Array(await blob.arrayBuffer());
  const view = new DataView(data.buffer);
  assert.equal(view.getUint32(0, true), 0x04034b50);
  const nameLength = view.getUint16(26, true);
  assert.equal(new TextDecoder().decode(data.slice(30, 30 + nameLength)), '中文.pdf');
  assert.deepEqual(data.slice(30 + nameLength, 30 + nameLength + bytes.length), bytes);
  assert.equal(view.getUint16(data.length - 14, true), 2);
});

test('binary API does not decode JSON-looking files but still reports JSON errors', async () => {
  const original = globalThis.fetch;
  try {
    globalThis.fetch = async () => new Response(new Uint8Array([255, 0, 123]), { headers: { 'Content-Type': 'application/json' } });
    assert.deepEqual(new Uint8Array(await (await apiGetBlob('/test')).arrayBuffer()), new Uint8Array([255, 0, 123]));
    globalThis.fetch = async () => new Response('{"error":"missing"}', { status: 404, headers: { 'Content-Type': 'application/json' } });
    await assert.rejects(apiGetBlob('/test'), error => error.status === 404 && error.message === 'missing');
  } finally { globalThis.fetch = original; }
});
