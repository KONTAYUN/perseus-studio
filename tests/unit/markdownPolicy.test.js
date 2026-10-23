import test from 'node:test';
import assert from 'node:assert/strict';
import { MarkdownManager } from '@tiptap/markdown';
import { isMarkdownFile, richEditingRestriction, safeMarkdownUrl } from '../../src/components/markdown/markdownPolicy.js';
import { markdownExtensions, serializeMarkdown } from '../../src/components/editor/richText/markdownExtensions.js';

test('Markdown links reject executable and disguised protocols', () => {
  for (const url of ['javascript:alert(1)', 'data:text/html,hi', 'vbscript:bad', 'java\nscript:bad', '//untrusted.test', '\\evil.test', 'file:///etc/passwd']) assert.equal(safeMarkdownUrl(url), '', url);
  for (const url of ['https://example.com/a?b=1', 'mailto:hello@example.com', '#heading', '../guide.md']) assert.equal(safeMarkdownUrl(url), url);
});

test('rich editing protects extended syntax while allowing fenced HTML examples', () => {
  for (const source of ['---\ntitle: retained\n---\nbody', '<details>keep me</details>', 'note[^1]\n\n[^1]: retained', '$$\na+b\n$$', '[guide]: https://example.com\n\n[guide]', '![[retained.png]]']) assert.ok(richEditingRestriction(source), source);
  assert.equal(richEditingRestriction('# Guide\n\n```html\n<div>example</div>\n```\n'), '');
  assert.equal(richEditingRestriction('Use `<div>` or `` `<span>` ``. Visit <https://example.com>.\n\n    <div>indented example</div>'), '');
  assert.equal(richEditingRestriction('````html\n<div>nested</div>\n```\n</div>\n`````\n'), '');
  assert.ok(richEditingRestriction('```html\n<div>example</div>\n```\n<div>real HTML</div>'));
  assert.equal(richEditingRestriction('## Notes\n\n- [ ] a\n\n**bold**'), '');
  assert.ok(richEditingRestriction('x'.repeat(512 * 1024 + 1)));
  assert.equal(isMarkdownFile('GUIDE.MD'), true);
  assert.equal(isMarkdownFile('code.js'), false);
});

test('Markdown document roundtrip preserves structured content and code whitespace', () => {
  const manager = new MarkdownManager({ extensions: markdownExtensions() });
  const source = '# Guide\n\n**bold** and *italic* and ~~removed~~ and `x < 1`\n\n> Quote\n\n- [x] done\n- [ ] todo\n\n1. first\n2. second\n\n| Name | Value |\n| :--- | ---: |\n| a | 1 |\n\n~~~bash\n  echo "hello"\n\nprintf world\n~~~\n';
  const document = manager.parse(source);
  const reparsed = manager.parse(manager.serialize(document));
  assert.deepEqual(reparsed, document);
  const block = document.content.find(node => node.type === 'codeBlock');
  assert.equal(block.attrs.language, 'bash');
  assert.equal(block.content[0].text, '  echo "hello"\n\nprintf world');
});

test('edited Markdown retains original newline convention', () => {
  assert.equal(serializeMarkdown({ getMarkdown: () => '# Updated\n\nText' }, '# Before\r\n'), '# Updated\r\n\r\nText\r\n');
  assert.equal(serializeMarkdown({ getMarkdown: () => 'Updated' }, 'Before'), 'Updated');
});

test('balanced inline spans preserve HTML wrappers and editable Markdown children', () => {
  const manager = new MarkdownManager({ extensions: markdownExtensions() });
  for (const source of [
    '* address: <span style="background-color:rgb(255,255,255);color:rgb(17,17,17)">`example`</span>',
    '<SPAN title="a > b">**bold** [link](https://example.com) <span>nested</span></SPAN>',
    'before <span>`</span>` and ``<span>`` after</span> end',
    'before <span></span> end',
  ]) {
    assert.equal(richEditingRestriction(source), '', source);
    const parsed = manager.parse(source);
    const output = manager.serialize(parsed);
    assert.deepEqual(manager.parse(output), parsed, source);
    const wrappers = source.match(/<\/?span\b[^>]*>/ig);
    for (const wrapper of wrappers) assert.ok(output.includes(wrapper), wrapper);
  }
  for (const source of ['<span>unclosed', '<span>first\nsecond</span>', '<span/>', '<span><script>bad</script></span>', '<span><details>bad</details></span>']) {
    assert.ok(richEditingRestriction(source), source);
  }
});
