import assert from 'node:assert/strict';
import test from 'node:test';

import { composerPrimaryButtonState } from '../../src/components/chat/composer/composerActionState.js';

test('composer primary action keeps stop enabled while a run is active', () => {
  assert.deepEqual(
    composerPrimaryButtonState({
      hasDraft: false,
      isChatSending: true,
      isSendDisabled: true,
    }),
    {
      disabled: false,
      title: '停止生成',
      toneClass: 'composer-stop-button',
      action: 'stop',
    }
  );
});

test('composer primary action uses disabled send only when no runnable action exists', () => {
  assert.deepEqual(
    composerPrimaryButtonState({
      hasDraft: false,
      isChatSending: false,
      isSendDisabled: true,
    }),
    {
      disabled: true,
      title: '发送',
      toneClass: 'composer-send-button-disabled',
      action: 'send',
    }
  );
});
