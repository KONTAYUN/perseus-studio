import test from 'node:test';
import assert from 'node:assert/strict';
import { readMetadata, updateMetadata } from '../../src/components/editor/document/metadata.js';
import { documentHealth, notifyDocumentHealth } from '../../src/components/editor/document/registry.js';

test('simple YAML/TOML form changes only the selected scalar, retaining comments and Unicode offsets', () => {
  for (const [format, source, expected] of [
    ['yaml', '\uFEFF---\r\n# 中文😀\r\ntitle: "原稿" # 注释\r\ncount: 0x10\r\nempty: null\r\n---', '\uFEFF---\r\n# 中文😀\r\ntitle: 新标题 # 注释\r\ncount: 0x10\r\nempty: null\r\n---'],
    ['toml', '+++\r\n# 中文😀\r\ntitle = "原稿" # 注释\r\ncount = 0x10\r\n+++', '+++\r\n# 中文😀\r\ntitle = "新标题" # 注释\r\ncount = 0x10\r\n+++'],
  ]) {
    const model = readMetadata(source, format); assert.equal(model.editable, true);
    const values = Object.fromEntries(model.rows.map(row => [row.key, String(row.value ?? '')]));
    assert.equal(updateMetadata(source, format, values), source);
    values.title = '新标题'; assert.equal(updateMetadata(source, format, values), expected);
  }
});
test('complex metadata is preserved; blank/unsafe numeric edits are rejected', () => {
  for (const [format, source] of [['yaml', '---\nx: [a, b]\n---'], ['toml', '+++\n[a]\nx = 1\n+++'], ['toml', '+++\nx = 999999999999999999999\n+++']]) {
    assert.equal(readMetadata(source, format).editable, false);
    assert.throws(() => updateMetadata(source, format, { x: 'new' }));
  }
  assert.throws(() => updateMetadata('---\nn: 1\n---', 'yaml', { n: '' }), /数字/);
});
test('typed document health is bound to the validated source version', () => {
  notifyDocumentHealth('稿.md', { category: 'format_issue', issues: [{ code: 'roundtrip' }] }, '旧稿');
  assert.equal(documentHealth('稿.md', '新稿'), null);
  assert.equal(documentHealth('稿.md', '旧稿').category, 'format_issue');
  notifyDocumentHealth('稿.md', { category: 'compatible', issues: [] }, '新稿');
  assert.equal(documentHealth('稿.md', '旧稿'), null);
});
