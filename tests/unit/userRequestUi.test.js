import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const requestMessageSource = readFileSync(
  new URL('../../src/components/chat/UserRequestMessage.jsx', import.meta.url),
  'utf8'
);
const composerSource = readFileSync(
  new URL('../../src/components/chat/composer/ChatComposer.jsx', import.meta.url),
  'utf8'
);

test('open user input uses Markdown and never renders synthetic option buttons', () => {
  assert.match(requestMessageSource, /ChatMarkdownRenderer/);
  assert.match(requestMessageSource, /content=\{request\.question\}/);
  assert.match(requestMessageSource, /request\.requestKind === 'choice'/);
  assert.match(requestMessageSource, /ide\.sendUserResponse/);
  assert.match(requestMessageSource, /等待回复/);
});

test('bounded choices submit directly without copy-to-composer controls', () => {
  assert.doesNotMatch(requestMessageSource, /Copy/);
  assert.doesNotMatch(requestMessageSource, /setChatInput\(option\)/);
  assert.match(requestMessageSource, /request\.options\.map/);
});

test('answered requests render as ordinary message content without persistent status chrome', () => {
  assert.doesNotMatch(requestMessageSource, /已回答/);
  assert.doesNotMatch(requestMessageSource, /需要你的输入/);
  assert.doesNotMatch(requestMessageSource, /需要你选择/);
  assert.match(requestMessageSource, /waiting && isActive/);
  assert.match(requestMessageSource, /isChoice && interactive/);
});

test('composer keeps a single user request presentation path', () => {
  assert.doesNotMatch(composerSource, /FollowupQuestionPanel/);
  assert.doesNotMatch(composerSource, /pendingFollowupQuestion/);
  assert.match(composerSource, /activeUserRequest\?\.requestKind === 'input'/);
  assert.match(composerSource, /chatInputRef\.current\?\.focus/);
});
