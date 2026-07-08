import fs from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const lineReviewThreshold = 450;
const failures = [];
const scorecardPath = path.join(root, 'docs', 'PROJECT_HEALTH_SCORECARD.md');
const scorecard = fs.existsSync(scorecardPath) ? fs.readFileSync(scorecardPath, 'utf8') : '';

const cssImportOrder = [
  './styles/tailwind.css',
  './styles/tokens.css',
  './styles/surfaces.css',
  './styles/motion.css',
  './styles/toast.css',
  './styles/controls.css',
  './styles/chat.css',
  './styles/chat-markdown.css',
  './styles/chat-composer.css',
  './styles/chat-composer-queue.css',
  './styles/editor.css',
  './styles/rich-text-toolbar.css',
  './styles/rich-text-block-toolbar.css',
  './styles/rich-text-selection-toolbar.css',
  './styles/rich-text-table-toolbar.css',
  './styles/rich-text-format-controls.css',
  './styles/rich-text-toolbar-menus.css',
  './styles/rich-text-history.css',
  './styles/rich-text-content.css',
  './styles/rich-text-image.css',
  './styles/rich-text-table.css',
  './styles/file-tree.css',
  './styles/settings.css',
  './styles/utilities.css',
];

function rel(file) {
  return path.relative(root, file).replaceAll(path.sep, '/');
}

function walk(dir, predicate, output = []) {
  if (!fs.existsSync(dir)) return output;
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (['node_modules', 'dist', '.git', '_local_artifacts', 'data'].includes(entry.name)) continue;
      walk(fullPath, predicate, output);
      continue;
    }
    if (predicate(fullPath, entry.name)) output.push(fullPath);
  }
  return output;
}

function lineCount(source) {
  if (source.length === 0) return 0;
  return source.split(/\r?\n/).length;
}

function fail(message) {
  failures.push(message);
}

function checkLineBudget(file, label) {
  const source = fs.readFileSync(file, 'utf8');
  const lines = lineCount(source);
  const relativePath = rel(file);
  if (lines > lineReviewThreshold && !scorecard.includes(`\`${relativePath}\``)) {
    fail(`${relativePath}:1 ${label} has ${lines} lines; add a responsibility review entry to docs/PROJECT_HEALTH_SCORECARD.md instead of splitting mechanically`);
  }
}

function checkForbiddenResidue(file) {
  const source = fs.readFileSync(file, 'utf8');
  const patterns = [
    [/\bCHAT_INPUT_BUTTON_SAFE_/g, 'composer old dynamic button safe inset constants are not allowed'],
    [/\bCHAT_INPUT_TALL_CONTENT_TRIGGER\b/g, 'composer old tall-content trigger is not allowed'],
    [/\bresizeChatInput\b/g, 'composer manual scrollHeight resize logic is not allowed'],
    [/\bchat-input-mirror\b/g, 'composer mirror/pretext layer is not allowed'],
    [/\bchat-input-pretext/g, 'composer Pretext class residue is not allowed'],
    [/@chenglou\/pretext/g, 'Pretext dependency is not allowed'],
  ];
  for (const [pattern, message] of patterns) {
    for (const match of source.matchAll(pattern)) {
      const line = source.slice(0, match.index).split(/\r?\n/).length;
      fail(`${rel(file)}:${line} ${message}`);
    }
  }
}

function checkCSS() {
  const indexPath = path.join(root, 'src', 'index.css');
  const source = fs.readFileSync(indexPath, 'utf8');
  const imports = [...source.matchAll(/^@import\s+['"]([^'"]+)['"];/gm)].map((match) => match[1]);
  const expected = JSON.stringify(cssImportOrder);
  if (JSON.stringify(imports) !== expected) {
    fail(`src/index.css:1 CSS imports must stay ordered as ${cssImportOrder.join(', ')}`);
  }
  const lines = source.split(/\r?\n/).filter((line) => line.trim());
  const firstTailwindIndex = lines.findIndex((line) => line.startsWith('@tailwind'));
  const lastImportIndex = lines.reduce((last, line, index) => (line.startsWith('@import') ? index : last), -1);
  if (firstTailwindIndex !== -1 && lastImportIndex > firstTailwindIndex) {
    fail('src/index.css:1 CSS @import statements must precede Tailwind directives to keep Vite/PostCSS warning-free');
  }
  const nonEntrypointRules = lines.filter((line) => !line.startsWith('@tailwind') && !line.startsWith('@import'));
  if (nonEntrypointRules.length > 0) {
    fail('src/index.css:1 should only contain Tailwind directives and ordered style imports');
  }

  const cssFiles = walk(path.join(root, 'src', 'styles'), (_full, name) => name.endsWith('.css'));
  const cssRel = new Set(cssFiles.map(rel));
  for (const importPath of cssImportOrder) {
    const expectedPath = `src/${importPath.replace('./', '')}`;
    if (!cssRel.has(expectedPath)) {
      fail(`${expectedPath}:1 missing required CSS section file`);
    }
  }
  for (const file of cssFiles) {
    checkLineBudget(file, 'CSS section');
    const content = fs.readFileSync(file, 'utf8');
    const balance = [...content].reduce((count, char) => {
      if (char === '{') return count + 1;
      if (char === '}') return count - 1;
      return count;
    }, 0);
    if (balance !== 0) {
      fail(`${rel(file)}:1 CSS block braces are unbalanced`);
    }
  }
}

const frontendFiles = walk(path.join(root, 'src'), (_full, name) => /\.(jsx|js)$/.test(name));
for (const file of frontendFiles) {
  const relativePath = rel(file);
  if (/\.test\.(jsx|js)$/.test(relativePath)) continue;
  checkLineBudget(file, 'frontend production file');
  checkForbiddenResidue(file);
}

const backendFiles = walk(path.join(root, 'backend'), (_full, name) => name.endsWith('.go'));
for (const file of backendFiles) {
  const relativePath = rel(file);
  if (relativePath.endsWith('_test.go')) continue;
  checkLineBudget(file, 'backend production Go file');
}

checkCSS();
checkForbiddenResidue(path.join(root, 'package.json'));
if (fs.existsSync(path.join(root, 'package-lock.json'))) {
  checkForbiddenResidue(path.join(root, 'package-lock.json'));
}

if (failures.length > 0) {
  console.error('Structure standards check failed:\n');
  for (const failure of failures) console.error(`- ${failure}`);
  console.error('\nRead docs/STRUCTURE_STANDARDS.md before changing project structure.');
  process.exit(1);
}

console.log(`Structure standards check passed (${frontendFiles.length} frontend files, ${backendFiles.length} Go files scanned).`);
