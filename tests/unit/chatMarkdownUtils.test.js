import assert from 'node:assert/strict';
import test from 'node:test';

import {
  displayLanguageName,
  isLikelyBlockCode,
  languageFromClassName,
  normalizeCodeForCopy,
  reactNodeToPlainText,
  safeMarkdownUrl,
} from '../../src/components/chat/markdown/chatMarkdownUtils.js';

test('languageFromClassName extracts fenced code language', () => {
  assert.equal(languageFromClassName('hljs language-bash'), 'bash');
  assert.equal(languageFromClassName('language-js extra'), 'js');
  assert.equal(languageFromClassName('hljs'), '');
});

test('displayLanguageName normalizes common aliases', () => {
  assert.equal(displayLanguageName('bash'), 'Bash');
  assert.equal(displayLanguageName('js'), 'JavaScript');
  assert.equal(displayLanguageName('tsx'), 'TSX');
  assert.equal(displayLanguageName(''), 'Text');
  assert.equal(displayLanguageName('custom-lang'), 'Custom Lang');
});

test('reactNodeToPlainText flattens highlighted code children', () => {
  const node = [
    'cd ',
    { props: { children: '/opt/stack' } },
    '\n',
    { props: { children: ['echo ', { props: { children: 'done' } }] } },
  ];

  assert.equal(reactNodeToPlainText(node), 'cd /opt/stack\necho done');
});

test('normalizeCodeForCopy removes trailing code fence newline only from copied code', () => {
  assert.equal(normalizeCodeForCopy('echo ok\n'), 'echo ok');
  assert.equal(normalizeCodeForCopy('echo ok\n\n'), 'echo ok');
  assert.equal(normalizeCodeForCopy('echo ok\n\nnext'), 'echo ok\n\nnext');
});

test('safeMarkdownUrl allows safe links and rejects scriptable urls', () => {
  assert.equal(safeMarkdownUrl('https://example.com'), 'https://example.com');
  assert.equal(safeMarkdownUrl('/local/path'), '/local/path');
  assert.equal(safeMarkdownUrl('#section'), '#section');
  assert.equal(safeMarkdownUrl('docs/readme'), 'docs/readme');
  assert.equal(safeMarkdownUrl('javascript:alert(1)'), '');
  assert.equal(safeMarkdownUrl(' data:text/html,<script>alert(1)</script>'), '');
  assert.equal(safeMarkdownUrl('vbscript:msgbox(1)'), '');
});

test('isLikelyBlockCode distinguishes inline code from fenced blocks', () => {
  assert.equal(isLikelyBlockCode('', 'inline'), false);
  assert.equal(isLikelyBlockCode('language-bash', 'echo ok'), true);
  assert.equal(isLikelyBlockCode('', 'echo ok\n'), true);
  assert.equal(isLikelyBlockCode('', ['echo', '\nnext']), true);
});

test('fenced bash command keeps its language label and clean copy payload', () => {
  const className = 'hljs language-bash';
  const highlightedChildren = ['ssh -p 52822 ', { props: { children: 'root@example.com' } }, '\n'];
  const language = languageFromClassName(className);
  const code = normalizeCodeForCopy(reactNodeToPlainText(highlightedChildren));

  assert.equal(displayLanguageName(language), 'Bash');
  assert.equal(isLikelyBlockCode(className, highlightedChildren), true);
  assert.equal(code, 'ssh -p 52822 root@example.com');
});
