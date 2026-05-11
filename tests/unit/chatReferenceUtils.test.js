import test from 'node:test';
import assert from 'node:assert/strict';
import {
  filesToChatAttachments,
  mergeChatAttachments,
  mergeSkillReference,
  mergeWorkspaceReferences,
  normalizeSkillId,
  normalizeWorkspaceReferences,
} from '../../src/hooks/chatReferenceUtils.js';

test('filesToChatAttachments deduplicates files within a batch', () => {
  const file = new File(['hello'], 'hello.txt', { type: 'text/plain' });
  const attachments = filesToChatAttachments([file, file], () => 'fixed');

  assert.equal(attachments.length, 1);
  assert.equal(attachments[0].name, 'hello.txt');
  assert.equal(attachments[0].type, 'text/plain');
  assert.match(attachments[0].id, /-fixed$/);
});

test('mergeChatAttachments keeps existing attachment fingerprints unique', () => {
  const file = new File(['hello'], 'hello.txt', { type: 'text/plain' });
  const [attachment] = filesToChatAttachments([file], () => 'a');
  const duplicate = { ...attachment, id: `${attachment.fingerprint}-b` };

  assert.equal(mergeChatAttachments([attachment], [duplicate]).length, 1);
});

test('normalizeWorkspaceReferences fills file metadata', () => {
  const [reference] = normalizeWorkspaceReferences({ path: 'src/App.jsx' });

  assert.equal(reference.id, 'workspace_src/App.jsx');
  assert.equal(reference.name, 'App.jsx');
  assert.equal(reference.kind, 'workspace_file');
  assert.equal(reference.file_type, 'text');
});

test('mergeWorkspaceReferences skips duplicate paths', () => {
  const current = normalizeWorkspaceReferences([{ path: 'a.txt' }]);
  const merged = mergeWorkspaceReferences(current, [{ path: 'a.txt' }, { path: 'b.txt' }]);

  assert.deepEqual(merged.map((item) => item.path), ['a.txt', 'b.txt']);
});

test('mergeSkillReference normalizes legacy skill ids and rejects disabled skills', () => {
  const current = mergeSkillReference([], {
    id: 'skill_skill_demo',
    name: 'Demo',
    description: 'Useful',
    enabled: true,
  });

  assert.equal(normalizeSkillId('skill_skill_demo'), 'skill_demo');
  assert.equal(current.length, 1);
  assert.equal(current[0].skill_id, 'skill_demo');
  assert.equal(mergeSkillReference(current, { id: 'skill_demo', name: 'Demo', enabled: true }), current);
  assert.equal(mergeSkillReference(current, { id: 'off', name: 'Off', enabled: false }), current);
});
