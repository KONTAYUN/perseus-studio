import assert from 'node:assert/strict';
import test from 'node:test';
import contract from '../fixtures/document-contract.json' with { type: 'json' };
import { getSchema } from '@tiptap/core';
import { DocumentSession } from '../../src/components/editor/document/session.js';
import { parseDocument } from '../../src/components/editor/document/fromMarkdown.js';
import { semanticKey } from '../../src/components/editor/document/normalize.js';
import { documentExtensions } from '../../src/components/editor/document/editorExtensions.js';
import { PreservedBlock, PreservedInline } from '../../src/components/editor/richText/PreservedMarkdown.js';
import { InlineMath, BlockMath, DocumentMetadata, FootnoteReference } from '../../src/components/editor/richText/DocumentNodes.js';

const schema = getSchema(documentExtensions(null, [PreservedBlock, PreservedInline], [InlineMath, BlockMath, DocumentMetadata, FootnoteReference]));
const schemaDoc = doc => schema.nodeFromJSON(doc).toJSON();
const fixtures = [
  '# 标题\n\n正文 **加粗** _斜体_ ~~删除~~ `a|b`\n下一行',
  '- 第一项\n  - 嵌套\n\n1. 有序\n2. 第二项\n\n- [ ] 待办\n- [x] 已办',
  '> 引用 **文字**\n>\n> 第二段\n\n---',
  '| 字段 | 内容 |\n| :--- | ---: |\n| x<br>y | `a\\|b` |',
  '正文 [链接][ref] ![图片][image]\n\n[ref]: https://example.com "标题"\n[image]: 图片/a.png',
  '正文[^note]\n\n[^note]: **注释**\n\n    第二段',
  '公式 $x^2$\n\n$$\nx+y\n$$',
  '> [!NOTE]\n> 提示 **重点**',
  '---\ntitle: "保留 # 注释"\n---\n\n正文',
  '+++\ntitle = "文稿"\n+++\n\n正文',
  '<details>\n<summary>摘要</summary>\n\n正文\n\n</details>\n\n尾段',
  ':::note\n```md\n:::\n```\n正文\n:::\n\n尾段',
  '前文 [[文档]] <u>文字</u> <span class="x">**重点**</span> 后文',
  '```js title="例子"\nconst x = "<script>";\n```\n\n末尾',
  '孤立标记 * 和 ** 和 ` 都是合法文字', '',
];

test('shared frontend/backend fixtures agree on preserved/native content', () => {
  for (const fixture of contract) {
    const session = new DocumentSession(fixture.source);
    assert.equal(session.status.category === 'preserved', fixture.preserved, fixture.id);
    const found = new Set();
    const visit = node => { found.add(node.type); for (const mark of node.marks || []) found.add(mark.type); for (const child of node.content || []) visit(child); };
    visit(session.doc);
    for (const kind of fixture.nodes || []) assert.ok(found.has(kind), `${fixture.id}: ${kind}`);
  }
});

test('all supported contract fixtures survive the real editor schema without rewriting on open', () => {
  for (const source of fixtures) {
    const session = new DocumentSession(source);
    const doc = schemaDoc(session.doc);
    assert.equal(session.update(doc), source, source);
    assert.notEqual(session.status.category, 'format_issue');
  }
});

test('editing a new root leaves every original root and separator byte-for-byte intact', () => {
  for (const source of fixtures.filter(Boolean)) {
    const session = new DocumentSession(source);
    const doc = schemaDoc(session.doc);
    doc.content.push({ type: 'paragraph', content: [{ type: 'text', text: '新增中文😀段落' }] });
    const candidate = session.update(doc);
    for (const entry of session.entries) assert.ok(candidate.includes(source.slice(entry.from, entry.to)), source);
    assert.equal(semanticKey(parseDocument(candidate).doc), semanticKey(doc), source);
  }
});

test('fifty edits/save/reopens do not degrade neighboring source or special payloads', () => {
  const protectedSource = '\uFEFF# 原始标题\r\n\r\n```md\r\n<script>文字</script>\r\n```\r\n\r\n<!-- 原样  -->\r\n\r\n';
  let source = protectedSource + '正文😀\r\n';
  for (let i = 0; i < 50; i++) {
    const session = new DocumentSession(source);
    const doc = schemaDoc(session.doc);
    doc.content.at(-1).content[0].text += '文';
    source = session.update(doc);
    assert.ok(source.startsWith(protectedSource), `cycle ${i}`);
  }
  assert.ok(source.includes('正文😀' + '文'.repeat(50)));
});

test('reference links are editable without changing their shared definition or other links', () => {
  const source = '[甲][ref] [乙][ref]\n\n[ref]: https://example.com';
  const session = new DocumentSession(source);
  const doc = schemaDoc(session.doc);
  doc.content[0].content[0].marks[0].attrs.href = 'https://new.example.com';
  const result = session.update(doc);
  assert.ok(result.includes('[甲](https://new.example.com)'));
  assert.ok(result.includes('[乙][ref]'));
  assert.ok(result.endsWith('[ref]: https://example.com'));
});

test('changed tables preserve alignment, line breaks, empty cells and escaped pipes', () => {
  const session = new DocumentSession(fixtures[3]);
  const doc = schemaDoc(session.doc);
  doc.content[0].content[1].content[0].content[0].content[0].text += '修改';
  const result = session.update(doc);
  assert.equal(semanticKey(parseDocument(result).doc), semanticKey(doc));
  const invalid = structuredClone(doc);
  invalid.content[0].content[1].content[0].attrs.colspan = 2;
  assert.throws(() => session.update(invalid), /表格/);
});

test('serializer rejects unsupported structures instead of silently losing content', () => {
  const session = new DocumentSession('正文');
  const doc = schemaDoc(session.doc);
  doc.content[0].content[0].marks = [{ type: 'unknownFormat', attrs: { data: '保留' } }];
  assert.throws(() => session.update(doc), /暂时无法保存/);
  assert.equal(session.source, '正文');
});

test('identical blocks keep their own original spellings after deletion, duplication and edits', () => {
  const source = '*同文*\n\n_同文_\n\n末段';
  const session = new DocumentSession(source), doc = schemaDoc(session.doc);
  doc.content[0].content[0].text = '修改';
  assert.ok(session.update(doc).includes('\n\n_同文_\n\n末段'));
  doc.content.splice(0, 1);
  assert.equal(session.update(doc), '_同文_\n\n末段');
});
test('mixed ordered task items retain numbering and check state after real schema editing', () => {
  const source = '3. [x] 已做\n4. 普通\n5. [ ] 未做';
  const session = new DocumentSession(source), doc = schemaDoc(session.doc);
  doc.content[0].content[1].content[0].content[0].text += '修改';
  const result = session.update(doc);
  assert.match(result, /^3\. \[x\]/); assert.match(result, /4\. 普通修改/); assert.match(result, /5\. \[ \]/);
});

test('deleting a shared definition preserves dependent links and images as inline addresses', () => {
  const source = '[甲][r] ![图][r]\n\n[乙][r]\n\n[r]: https://example.com/a "标题"';
  const session = new DocumentSession(source), doc = schemaDoc(session.doc);
  doc.content.pop();
  const saved = session.update(doc);
  assert.ok(saved.includes('[甲](https://example.com/a "标题")'));
  assert.ok(saved.includes('![图](https://example.com/a "标题")'));
  assert.ok(saved.includes('[乙](https://example.com/a "标题")'));
  assert.equal(semanticKey(parseDocument(saved).doc), semanticKey(doc));
  assert.equal(session.undo().source, source);
});

test('editing display math retains its original metadata and adjacent text', () => {
  const session = new DocumentSession('$$ equation-id\nx+y\n$$\n\n尾段');
  const doc = schemaDoc(session.doc); doc.content[0].attrs.formula += '+z';
  const saved = session.update(doc);
  assert.match(saved, /^\$\$\s*equation-id/); assert.ok(saved.endsWith('尾段'));
});

test('duplicate reference definitions use the first CommonMark definition and keep both raw payloads', () => {
  const source = '[资料][r]\n\n[r]: https://first.example.com\n\n[r]: https://second.example.com';
  const session = new DocumentSession(source), doc = schemaDoc(session.doc);
  assert.equal(doc.content[0].content[0].marks[0].attrs.href, 'https://first.example.com');
  doc.content[0].content[0].text += '修改';
  const saved = session.update(doc);
  assert.ok(saved.endsWith('[r]: https://first.example.com\n\n[r]: https://second.example.com'));
  assert.equal(parseDocument(saved).doc.content[0].content[0].marks[0].attrs.href, 'https://first.example.com');
});
