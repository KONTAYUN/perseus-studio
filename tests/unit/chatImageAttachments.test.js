import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const previewSource = readFileSync(
  new URL('../../src/components/chat/ChatImageAttachments.jsx', import.meta.url),
  'utf8'
);
const bubbleSource = readFileSync(
  new URL('../../src/components/chat/ChatMessageBubble.jsx', import.meta.url),
  'utf8'
);
const sendSource = readFileSync(
  new URL('../../src/hooks/chatSendActions.js', import.meta.url),
  'utf8'
);

test('chat images use authenticated thumbnails and an in-app preview dialog', () => {
  assert.match(previewSource, /item\.content_url/);
  assert.match(previewSource, /createPortal/);
  assert.match(previewSource, /MediaPreviewDialog/);
  assert.match(previewSource, /ArrowLeft/);
  assert.match(previewSource, /ArrowRight/);
  assert.match(previewSource, /item\.download_url/);
  assert.match(previewSource, /旧图片未保存，无法预览/);
});

test('message bubbles separate images from ordinary attachment chips', () => {
  assert.match(bubbleSource, /ChatImageAttachments/);
  assert.match(bubbleSource, /item\.kind === 'image'/);
  assert.match(bubbleSource, /badgeAttachments/);
  assert.match(bubbleSource, /Boolean\(String\(item\.name \|\| ''\)\.trim\(\)\)/);
  assert.doesNotMatch(bubbleSource, /item\.name \|\| ['"]附件['"]/);
  assert.doesNotMatch(bubbleSource, /CompletedActionItems/);
});

test('message send references uploaded image ids instead of reuploading base64', () => {
  assert.match(sendSource, /formData\.append\('attachment_ids\[\]', attachment\.id\)/);
  assert.match(sendSource, /attachment\.kind === 'image' && attachment\.upload_status === 'uploaded'/);
  assert.doesNotMatch(sendSource, /readAsDataURL/);
});
