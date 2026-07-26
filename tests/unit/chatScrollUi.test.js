import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const controllerSource = readFileSync(
  new URL('../../src/components/chat/panel/useChatScrollController.js', import.meta.url),
  'utf8'
);
const messagesSource = readFileSync(
  new URL('../../src/components/chat/ChatMessages.jsx', import.meta.url),
  'utf8'
);
const panelSource = readFileSync(
  new URL('../../src/components/chat/ChatPanel.jsx', import.meta.url),
  'utf8'
);
const packageJson = JSON.parse(
  readFileSync(new URL('../../package.json', import.meta.url), 'utf8')
);

test('chat scrolling uses the streaming-aware stick-to-bottom controller', () => {
  assert.equal(packageJson.dependencies['use-stick-to-bottom'], '^1.1.6');
  assert.match(controllerSource, /useStickToBottom/);
  assert.match(controllerSource, /initial: 'instant'/);
  assert.match(controllerSource, /contentRef: messagesContentRef/);
  assert.match(controllerSource, /scrollRef: messagesRef/);
  assert.doesNotMatch(controllerSource, /BOTTOM_STICK_THRESHOLD/);
  assert.doesNotMatch(controllerSource, /scrollIntoView/);
});

test('sending and switching conversations restore bottom following', () => {
  assert.match(controllerSource, /scrollToBottom\('smooth'\)/);
  assert.match(controllerSource, /pendingConversationScrollRef/);
  assert.match(controllerSource, /conversationChanged/);
  assert.match(controllerSource, /scrollToBottom\('auto'\)/);
});

test('chat messages expose separate scroll and observed content elements', () => {
  assert.match(messagesSource, /ref=\{messagesRef\}/);
  assert.match(messagesSource, /ref=\{messagesContentRef\}/);
  assert.match(messagesSource, /isMessageListEmpty \? 'h-full' : 'min-h-full'/);
  assert.match(messagesSource, /style=\{\{ paddingBottom: `\$\{bottomInset\}px` \}\}/);
  assert.doesNotMatch(messagesSource, /bottomRef/);
});

test('the existing bottom button re-enables sticky scrolling', () => {
  assert.match(panelSource, /showScrollToBottom/);
  assert.match(panelSource, /scrollToBottom\('smooth'\)/);
  assert.match(panelSource, /data-testid="scroll-to-bottom-button"/);
});
