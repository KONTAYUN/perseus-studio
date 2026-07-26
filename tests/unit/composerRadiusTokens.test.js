import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const tokensCss = readFileSync(new URL('../../src/styles/tokens.css', import.meta.url), 'utf8');
const composerCss = readFileSync(new URL('../../src/styles/chat-composer.css', import.meta.url), 'utf8');
const composerQueueCss = readFileSync(new URL('../../src/styles/chat-composer-queue.css', import.meta.url), 'utf8');
const skillMenuJsx = readFileSync(new URL('../../src/components/chat/composer/SkillMenu.jsx', import.meta.url), 'utf8');

test('composer surfaces share the same semantic radius token', () => {
  assert.match(tokensCss, /--radius-composer:\s*var\(--radius-panel\);/);
  assert.match(composerCss, /\.composer-shell\s*\{[\s\S]*border-radius:\s*var\(--radius-composer\);/);

  for (const selector of [
    '.pending-run-message-panel',
    '.run-steer-notice-panel',
  ]) {
    assert.match(
      composerQueueCss,
      new RegExp(`${escapeRegExp(selector)}\\s*\\{[\\s\\S]*border-radius:\\s*var\\(--radius-composer\\);`)
    );
  }
});

test('composer popovers use composer-specific radius tokens instead of local radius utilities', () => {
  assert.match(tokensCss, /--radius-composer-popover:\s*var\(--radius-card\);/);
  assert.match(composerCss, /\.composer-popover-surface\s*\{[\s\S]*border-radius:\s*var\(--radius-composer-popover\);/);
  assert.match(skillMenuJsx, /composer-popover-surface/);
  assert.doesNotMatch(skillMenuJsx, /rounded-2xl/);
});

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}
