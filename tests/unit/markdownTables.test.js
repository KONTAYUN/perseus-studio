import assert from 'node:assert/strict';
import test from 'node:test';
import { documentAdapter } from './helpers/documentAdapter.js';
import { semanticKey } from '../../src/components/editor/document/normalize.js';
import remarkMarkdownBreaks from '../../src/components/markdown/markdownLineBreak.js';

test('table HTML breaks parse as editable hardBreak nodes and survive repeated round trips', () => {
  const manager = documentAdapter();
  const source = '| 字段 | 内容 |\n| --- | --- |\n| 第一行<br>第二行<BR/><br />**第三行** | `a|b` |\n';
  const document = manager.parse(source);
  const cell = document.content[0].content[1].content[0].content[0];
  assert.equal(cell.content.filter(node => node.type === 'hardBreak').length, 3);
  let output = source;
  for (let i = 0; i < 5; i++) {
    output = manager.serialize(manager.parse(output));
    assert.deepEqual(manager.parse(output), document);
  }
  assert.equal(output, source); // Opening/reopening never normalizes untouched source.
});

test('editor-generated multiline and empty cells never trigger the extension fallback', () => {
  const manager = documentAdapter();
  const paragraph = content => ({ type: 'paragraph', content });
  const text = value => ({ type: 'text', text: value });
  const document = { type: 'doc', content: [{ type: 'table', content: [
    { type: 'tableRow', content: [{ type: 'tableHeader', content: [paragraph([{ type: 'hardBreak' }])] }] },
    { type: 'tableRow', content: [{ type: 'tableCell', content: [paragraph([text('前'), { type: 'hardBreak' }, text('后')])] }] },
  ] }] };
  const source = manager.serialize(document);
  assert.equal(semanticKey(manager.parse(source)), semanticKey(document));
});

test('literal and code break examples stay text; unsafe break attributes stay inert local nodes', () => {
  const manager = documentAdapter();
  const source = '| 例子 |\n| --- |\n| `<br>` &lt;br&gt; \\<br> |';
  const parsed = manager.parse(source);
  const cell = parsed.content[0].content[1].content[0].content[0];
  assert.equal(cell.content.some(node => node.type === 'hardBreak'), false);
  assert.deepEqual(manager.parse(manager.serialize(parsed)), parsed);
  for (const tag of ['<br onclick="alert(1)">', '<br style="display:none">', '<br><script>bad</script>', '<br-data>']) {
    const document = manager.parse(`before ${tag} after`);
    assert.ok(JSON.stringify(document).includes('preservedInline'), tag);
    assert.ok(manager.serialize(document).includes(tag.replace(/^<br>/, '')), tag);
  }
});

test('safe preview renders only attribute-free break HTML as break nodes', () => {
  const nodes = ['<br>', '<BR/>', '<br />', '<br onclick="bad()">', '<script>bad</script>'];
  const tree = { type: 'root', children: [{ type: 'paragraph', children: [
    ...nodes.map(value => ({ type: 'html', value })), { type: 'inlineCode', value: '<br>' },
  ] }] };
  remarkMarkdownBreaks()(tree);
  assert.deepEqual(tree.children[0].children.slice(0, 3), [{ type: 'break' }, { type: 'break' }, { type: 'break' }]);
  assert.equal(tree.children[0].children[3].type, 'html');
  assert.equal(tree.children[0].children[4].type, 'html');
  assert.equal(tree.children[0].children[5].type, 'inlineCode');
});
