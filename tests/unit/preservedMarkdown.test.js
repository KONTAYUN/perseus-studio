import assert from 'node:assert/strict';
import test from 'node:test';
import { documentAdapter } from './helpers/documentAdapter.js';
import { DocumentSession } from '../../src/components/editor/document/session.js';
import { documentPages } from '../../src/components/editor/document/pages.js';
import { semanticKey } from '../../src/components/editor/document/normalize.js';

const manager = documentAdapter;
function preserved(document) {
  return (document.content || []).flatMap(node => [
    ...(node.type.startsWith('preserved') ? [node.attrs.raw] : []), ...preserved(node),
  ]);
}

test('extended syntax stays local, preserves payload and leaves surrounding prose editable', () => {
  const m = manager();
  for (const raw of [
    '---\ntitle: 原始配置\ntags: [a, b]\n---', '\uFEFF---\ntitle: BOM\n---', '+++\ntitle = "配置"\n+++',
    '<details>\n<summary>摘要</summary>\n\n内容\n\n</details>',
    '<!-- comment\nkeep spaces  \n-->', '<?instruction keep?>', '<!DOCTYPE html>',
    '<script>window.XSS=1\n\nalert(1)</script>', '<img src=x onerror="alert(1)">',
    '<br\nonclick="window.XSS=1">', '<br\n>',
    '[^note]: 脚注原文\n    第二行\n\n    第二段',
    '$$\na^2 + b^2\n$$', '\\[\na+b\n\\]',
    ':::note\n\nouter\n:::tip\ninner\n:::\n\n:::',
    '::: {.note}\n正文\n:::',
    ':::note\n```md\n:::\n```\n保留正文\n:::',
  ]) {
    const document = m.parse(`${raw}\n\n# 正文标题\n\n可以继续输入`);
    const heading = document.content.find(node => node.type === 'heading');
    assert.ok(heading, raw);
    document.content.at(-1).content[0].text += '，已编辑';
    const output = m.serialize(document);
    assert.ok(output.includes(raw), raw);
    assert.ok(output.includes('可以继续输入，已编辑'));
    for (let i = 0; i < 3; i++) {
      const reparsed = m.parse(m.serialize(document));
      assert.deepEqual(preserved(reparsed), preserved(document), raw);
    }
  }
});

test('inline HTML, wiki, footnote and math islands survive adjacent edits and nested structures', () => {
  const m = manager();
  for (const raw of ['<u title="a > b">文字 <u>嵌套</u></u>', '<br onclick="alert(1)">', '[[文档|别名]]', '![[图片.png]]']) {
    const document = m.parse(`- 前文 ${raw} 后文\n\n> 引用 ${raw} 末尾`);
    assert.equal(preserved(document).filter(value => value === raw).length, 2, raw);
    const output = m.serialize(document);
    assert.ok(output.includes(raw), raw);
    assert.equal(semanticKey(m.parse(output)), semanticKey(document), raw);
  }
});

test('reference definitions, shortcut/full/collapsed links and images retain original syntax', () => {
  const m = manager();
  const raw = '[guide]: https://example.com/a "标题"\n\n[guide] [文字][guide] [guide][] ![图片][guide]';
  const document = m.parse(raw);
  assert.equal(m.serialize(document), raw);
  assert.deepEqual(m.parse(m.serialize(document)), document);
});

test('code examples, autolinks and ordinary rules remain native editable content', () => {
  const m = manager();
  const document = m.parse('```md\n<div>sample</div>\n[^note]: demo\n:::note\n```\n\n`<div>` and <https://example.com>\n\n---\n\nbody');
  assert.deepEqual(preserved(document), []);
  assert.ok(document.content.some(node => node.type === 'horizontalRule'));
  assert.deepEqual(m.parse(m.serialize(document)), document);
});

test('unclosed special delimiters retain source; native math follows remark fence scope', () => {
  const m = manager();
  for (const opening of ['+++', '$$', '\\[', ':::note', '<span>']) {
    const document = m.parse(`${opening}\n\n# 可编辑正文\n\n结尾`);
    if (opening === '$$') assert.equal(document.content[0].type, 'blockMath');
    else assert.ok(document.content.some(node => node.type === 'heading'), opening);
    assert.ok(m.serialize(document).includes(opening));
  }
});

test('AST pages preserve mixed newlines verbatim and never split fences or containers', () => {
  const code = '```md\r\n' + '代码\n'.repeat(30000) + '```\r\n\r\n';
  const source = ('# 段落\r\n\n正文\r\n\n'.repeat(1200)) + code + '尾段\n';
  const session = new DocumentSession(source);
  const sections = documentPages(session.doc).map(range => session.doc.content.slice(range.from, range.to));
  assert.ok(sections.length > 2);
  assert.equal(session.update(session.doc), source);
  assert.equal(sections.flat().filter(node => node.type === 'codeBlock').length, 1);
  assert.ok(sections.flat().find(node => node.type === 'codeBlock').content[0].text.includes('代码'));
});

test('reference scope is never split and short dense documents are bounded too', () => {
  const referenceDocument = '[guide]: https://example.com\n\n' + '[guide]\n\n'.repeat(1200);
  const session = new DocumentSession(referenceDocument);
  assert.ok(documentPages(session.doc).length > 1);
  assert.ok(session.doc.content.slice(1).every(node => node.content[0].marks[0].attrs.href === 'https://example.com'));
  const dense = '短段\n\n'.repeat(1200);
  const chunks = documentPages(new DocumentSession(dense).doc);
  assert.ok(chunks.length > 1);
  assert.equal(chunks.at(-1).to, 1200);
  assert.ok(chunks.every(chunk => chunk.to - chunk.from <= 120));
});

test('platform math and escaped literal wiki syntax survive editing the same paragraph', () => {
  const source = '前文 &amp; \\(x+y\\) 后文 \\[[普通文字]]';
  const m = manager(), doc = m.parse(source);
  assert.ok(preserved(doc).includes('\\(x+y\\)'));
  assert.ok(!preserved(doc).includes('[[普通文字]]'));
  doc.content[0].content.at(-1).text += '继续写';
  const saved = m.serialize(doc);
  assert.ok(saved.includes('\\(x+y\\)'));
  assert.equal(semanticKey(m.parse(saved)), semanticKey(doc));
});
