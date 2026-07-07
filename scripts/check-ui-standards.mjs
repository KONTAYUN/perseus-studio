import fs from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const componentRoot = path.join(root, 'src', 'components');
const srcRoot = path.join(root, 'src');
const maxClassLength = 180;
const lineReviewThreshold = 450;
const scorecardPath = path.join(root, 'docs', 'PROJECT_HEALTH_SCORECARD.md');
const scorecard = fs.existsSync(scorecardPath) ? fs.readFileSync(scorecardPath, 'utf8') : '';

const failures = [];
const componentFiles = [];
const sourceFiles = [];
const styleFiles = [];
const lineBudgetAllowlist = new Map();
const longClassAllowedTokens = [
  'ui-panel-surface',
  'ui-overlay-surface',
  'settings-modal-card',
  'settings-card',
  'settings-nav-card',
  'settings-section-appear',
  'motion-popover',
  'motion-popover-right',
  'motion-modal',
  'motion-card',
  'motion-chip',
  'motion-message',
  'motion-button',
  'motion-row',
  'motion-soft-focus',
  'motion-color',
  'motion-fade-reveal',
  'motion-layout',
  'motion-panel-enter',
  'motion-rotate',
  'animate-in',
  'composer-shell',
  'composer-tool-cluster',
  'composer-icon-button',
  'login-card-surface',
  'chat-messages-scroll',
  'chat-error-card',
  'empty-chat-card',
  'scroll-to-bottom-button',
  'context-usage-popover',
  'chat-panel-header',
  'message-bubble',
  'message-actions',
  'conversation-item-row',
  'editor-panel-root',
  'editor-tabs-strip',
  'editor-drop-overlay',
  'editor-empty-tab',
  'editor-conflict-notice',
  'editor-textarea',
  'rich-text-block-trigger',
  'rich-text-color-swatch',
  'rich-text-table-menu-item',
  'table-size-picker-cell',
  'note-image-toolbar-button',
  'note-image-resize-handle',
  'file-tree-row',
  'file-drop-hint',
  'file-drop-indicator-line',
  'inline-rename-input',
  'workspace-drop-overlay',
  'trash-item-row',
  'task-item-row',
  'app-header',
  'layout-notice',
  'panel-wrapper',
  'modal-backdrop',
  'dialog-tone-icon',
  'candidate-check',
  'header-segmented',
  'custom-scrollbar',
];
const rawAbsolutePopoverAllowlist = new Map([
  [
    'src/components/chat/composer/SkillMenu.jsx',
    'Composer slash-skill autocomplete stays local to the composer so typing can continue while suggestions are visible.',
  ],
  [
    'src/components/chat/composer/ModelMenu.jsx',
    'Model provider submenu is a nested second-level menu inside the anchored model menu.',
  ],
  [
    'src/components/ui/index.jsx',
    'Shared popover primitive owns fixed-position popover geometry.',
  ],
]);

function walk(dir, output) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      walk(fullPath, output);
      continue;
    }
    if (/\.(jsx|js)$/.test(entry.name)) output.push(fullPath);
  }
}

function walkStyles(dir, output) {
  if (!fs.existsSync(dir)) return;
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      walkStyles(fullPath, output);
      continue;
    }
    if (/\.css$/.test(entry.name)) output.push(fullPath);
  }
}

function rel(file) {
  return path.relative(root, file).replaceAll(path.sep, '/');
}

function lineOf(source, index) {
  return source.slice(0, index).split('\n').length;
}

function addFailure(file, index, message) {
  failures.push(`${rel(file)}:${lineOf(fs.readFileSync(file, 'utf8'), index)} ${message}`);
}

function findButtonTags(source) {
  const tags = [];
  let index = 0;
  while (index < source.length) {
    const start = source.indexOf('<button', index);
    if (start === -1) break;

    let cursor = start + '<button'.length;
    let quote = '';
    let braceDepth = 0;
    for (; cursor < source.length; cursor += 1) {
      const char = source[cursor];
      const prev = source[cursor - 1];
      if (quote) {
        if (char === quote && prev !== '\\') quote = '';
        continue;
      }
      if (char === '"' || char === "'" || char === '`') {
        quote = char;
        continue;
      }
      if (char === '{') {
        braceDepth += 1;
        continue;
      }
      if (char === '}') {
        braceDepth = Math.max(0, braceDepth - 1);
        continue;
      }
      if (char === '>' && braceDepth === 0) {
        tags.push({ start, text: source.slice(start, cursor + 1) });
        cursor += 1;
        break;
      }
    }
    index = cursor;
  }
  return tags;
}

function checkClassNames(file, source) {
  const literalPatterns = [
    /className\s*=\s*"([^"]*)"/g,
    /className\s*=\s*'([^']*)'/g,
    /className\s*=\s*{\s*`([\s\S]*?)`\s*}/g,
  ];

  for (const pattern of literalPatterns) {
    for (const match of source.matchAll(pattern)) {
      const value = match[1].replace(/\s+/g, ' ').trim();
      const hasNamedException = longClassAllowedTokens.some((token) => value.includes(token));
      if (value.length > maxClassLength && !hasNamedException) {
        addFailure(
          file,
          match.index,
          `className is ${value.length} chars; move repeated styling into src/components/ui or the matching src/styles/*.css section, or use a documented named UI class`
        );
      }
    }
  }
}

function checkForbiddenPatterns(file, source) {
  const checks = [
    [/\bhover:-translate/g, 'button/row hover translate is not allowed'],
    [/\bactive:translate/g, 'active translate is not allowed'],
    [/translateY\(-1px\)/g, 'translateY(-1px) is not allowed'],
    [/\bhover:scale(?:-\[|-\d|\b)/g, 'hover scale utility is not allowed; use centered icon feedback'],
    [/\bactive:scale(?:-\[|-\d|\b)/g, 'active scale utility is not allowed; use shared motion classes'],
    [/\btransition-all\b/g, 'transition-all is too broad; use explicit transition properties'],
    [/\bduration-\d+\b/g, 'Tailwind duration-* utilities are not allowed; use motion tokens/classes'],
    [/\bease-\[[^\]]+\]/g, 'arbitrary easing utilities are not allowed; use motion tokens/classes'],
    [/\bease-out\b/g, 'one-off easing utilities are not allowed; use motion tokens/classes'],
    [/\bease-apple\b/g, 'legacy ease utility is not allowed in components; use motion tokens/classes'],
    [/\b(?:hover:)?shadow-(?:sm|md|lg|xl|2xl)\b/g, 'Tailwind shadow utilities are not allowed in src JS/JSX; use named CSS surface classes'],
    [/\b(?:dark:)?(?:hover:)?shadow-\[[^\]]+\]/g, 'arbitrary shadow utilities are not allowed in src JS/JSX; use named CSS surface classes'],
    [/rounded-\[[^\]]+\]/g, 'arbitrary rounded-[...] is not allowed in components; use radius tokens/classes'],
    [/inline-flex\s+rounded-2xl[^"']*\bp-0\.5/g, 'segmented controls must use SegmentedControl / ui-segmented-control so inner and outer radii match'],
    [/relative\s+h-6\s+w-11\s+rounded-full/g, 'switches must use Switch / ui-switch so thumb geometry stays centered'],
    [/\bui-panel-surface\b[^"'`]*\bring-/g, 'panel active/drop glows must use ui-panel-surface-active or ui-panel-surface-drop, not Tailwind ring utilities'],
    [/\bcomposer-shell\b[^"'`]*\bring-/g, 'composer active glow must use composer-shell-active, not Tailwind ring utilities'],
  ];

  for (const [pattern, message] of checks) {
    for (const match of source.matchAll(pattern)) {
      addFailure(file, match.index, message);
    }
  }
}

function checkMotionGovernance(file, source) {
  const relativePath = rel(file);
  const isMotionCss = relativePath === 'src/styles/motion.css';
  const isTokensCss = relativePath === 'src/styles/tokens.css';

  if (!isMotionCss) {
    for (const match of source.matchAll(/@keyframes\b/g)) {
      addFailure(file, match.index, 'keyframes must live in src/styles/motion.css');
    }
  }

  if (!isMotionCss && !isTokensCss) {
    for (const match of source.matchAll(/\b\d+ms\b/g)) {
      addFailure(file, match.index, 'hardcoded motion timing is not allowed; use motion tokens');
    }
  }
}

function checkRawAbsolutePopovers(file, source) {
  const relativePath = rel(file);
  if (rawAbsolutePopoverAllowlist.has(relativePath)) return;

  const pattern = /\bmotion-popover(?:-right)?\b[\s\S]{0,180}\babsolute\b|\babsolute\b[\s\S]{0,180}\bmotion-popover(?:-right)?\b/g;
  for (const match of source.matchAll(pattern)) {
    addFailure(file, match.index, 'raw absolute popovers are not allowed; use AnchoredPopover or document an allowlisted exception');
  }
}

function checkButtons(file, source) {
  const relativePath = rel(file);
  if (relativePath === 'src/components/ui/index.jsx') return;
  for (const tag of findButtonTags(source)) {
    const hasClassName = /\bclassName\s*=/.test(tag.text);
    const usesStandardClass = /\b(motion-button|motion-row|motion-chip|ui-icon-button|settings-nav-card|composer-icon-button|rich-text-block-trigger|rich-text-color-swatch|rich-text-table-menu-item|table-size-picker-cell|note-image-toolbar-button|note-image-resize-handle)\b/.test(tag.text);
    if (!hasClassName || !usesStandardClass) {
      addFailure(file, tag.start, 'raw <button> must use Button/IconButton or a standard motion class');
    }

    if (/\btitle\s*=/.test(tag.text) && /\b(data-tooltip|aria-describedby)\s*=/.test(tag.text)) {
      addFailure(file, tag.start, 'avoid native title plus custom tooltip on the same icon button');
    }
  }
}

function checkLineBudget(file, source) {
  const relativePath = rel(file);
  if (lineBudgetAllowlist.has(relativePath)) return;
  if (/\.test\.(jsx|js)$/.test(relativePath)) return;
  const lines = source.split('\n').length;
  if (lines > lineReviewThreshold && !scorecard.includes(`\`${relativePath}\``)) {
    failures.push(`${relativePath}:1 file has ${lines} lines; add a responsibility review entry to docs/PROJECT_HEALTH_SCORECARD.md instead of splitting mechanically`);
  }
}

function checkComposerRegressions(file, source) {
  const patterns = [
    [/\bCHAT_INPUT_BUTTON_SAFE_/g, 'composer old dynamic button safe inset constants are not allowed'],
    [/\bCHAT_INPUT_TALL_CONTENT_TRIGGER\b/g, 'composer old tall-content trigger is not allowed'],
    [/\bresizeChatInput\b/g, 'composer manual scrollHeight resize logic is not allowed; use react-textarea-autosize'],
    [/\bchat-input-mirror\b/g, 'composer mirror/pretext layer is not allowed'],
    [/\bchat-input-pretext/g, 'composer Pretext class residue is not allowed'],
    [/@chenglou\/pretext/g, 'Pretext dependency is not allowed'],
  ];
  for (const [pattern, message] of patterns) {
    for (const match of source.matchAll(pattern)) {
      addFailure(file, match.index, message);
    }
  }
}

walk(componentRoot, componentFiles);
walk(srcRoot, sourceFiles);
walkStyles(path.join(srcRoot, 'styles'), styleFiles);

for (const file of sourceFiles) {
  const source = fs.readFileSync(file, 'utf8');
  checkForbiddenPatterns(file, source);
  checkRawAbsolutePopovers(file, source);
  checkComposerRegressions(file, source);
  checkLineBudget(file, source);
}

for (const file of styleFiles) {
  const source = fs.readFileSync(file, 'utf8');
  checkMotionGovernance(file, source);
}

for (const file of componentFiles) {
  const source = fs.readFileSync(file, 'utf8');
  checkClassNames(file, source);
}

for (const file of sourceFiles) {
  const source = fs.readFileSync(file, 'utf8');
  checkButtons(file, source);
}

if (failures.length > 0) {
  console.error('UI standards check failed:\n');
  for (const failure of failures) console.error(`- ${failure}`);
  console.error('\nUse shared UI primitives from src/components/ui and document intentional exceptions in docs/UI_DESIGN_SYSTEM.md.');
  process.exit(1);
}

console.log(`UI standards check passed (${sourceFiles.length} src files scanned, ${componentFiles.length} component files checked for shared class extraction).`);
