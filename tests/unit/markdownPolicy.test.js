import test from 'node:test';
import assert from 'node:assert/strict';
import { documentAdapter } from './helpers/documentAdapter.js';
import { isMarkdownFile, safeMarkdownUrl } from '../../src/components/markdown/markdownPolicy.js';
import { DocumentSession } from '../../src/components/editor/document/session.js';

test('Markdown links reject executable and disguised protocols', () => {
  for (const url of ['javascript:alert(1)', 'data:text/html,hi', 'vbscript:bad', 'java\nscript:bad', '//untrusted.test', '\\evil.test', 'file:///etc/passwd']) assert.equal(safeMarkdownUrl(url), '', url);
  for (const url of ['https://example.com/a?b=1', 'mailto:hello@example.com', '#heading', '../guide.md']) assert.equal(safeMarkdownUrl(url), url);
});

test('Markdown filenames are case insensitive', () => {
  assert.equal(isMarkdownFile('GUIDE.MD'), true);
  assert.equal(isMarkdownFile('code.js'), false);
});

test('Markdown document roundtrip preserves structured content and code whitespace', () => {
  const manager = documentAdapter();
  const source = '# Guide\n\n**bold** and *italic* and ~~removed~~ and `x < 1`\n\n> Quote\n\n- [x] done\n- [ ] todo\n\n1. first\n2. second\n\n| Name | Value |\n| :--- | ---: |\n| a | 1 |\n\n~~~bash\n  echo "hello"\n\nprintf world\n~~~\n';
  const document = manager.parse(source);
  const reparsed = manager.parse(manager.serialize(document));
  assert.deepEqual(reparsed, document);
  const block = document.content.find(node => node.type === 'codeBlock');
  assert.equal(block.attrs.language, 'bash');
  assert.equal(block.content[0].text, '  echo "hello"\n\nprintf world');
});

test('edited Markdown retains original newline convention', () => {
  for (const [source, expected] of [['# Before\r\n', '# Updated\r\n'], ['Before', 'Updated']]) {
    const session = new DocumentSession(source), doc = structuredClone(session.doc);
    doc.content[0].content[0].text = 'Updated'; assert.equal(session.update(doc), expected);
  }
});

test('balanced inline spans preserve HTML wrappers and editable Markdown children', () => {
  const manager = documentAdapter();
  for (const source of [
    '* address: <span style="background-color:rgb(255,255,255);color:rgb(17,17,17)">`example`</span>',
    '<SPAN title="a > b">**bold** [link](https://example.com) <span>nested</span></SPAN>',
    'before <span>`</span>` and ``<span>`` after</span> end',
    'before <span></span> end',
  ]) {

    const parsed = manager.parse(source);
    const output = manager.serialize(parsed);
    assert.deepEqual(manager.parse(output), parsed, source);
    const wrappers = source.match(/<\/?span\b[^>]*>/ig);
    for (const wrapper of wrappers) assert.ok(output.includes(wrapper), wrapper);
  }
  for (const source of ['<span>unclosed', '<span>first\nsecond</span>', '<span/>', '<span><script>bad</script></span>', '<span><details>bad</details></span>']) {
    const parsed = manager.parse(source);
    assert.ok(manager.serialize(parsed).includes(source), source);
  }
});
