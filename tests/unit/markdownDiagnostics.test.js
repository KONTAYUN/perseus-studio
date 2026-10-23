import test from 'node:test';
import assert from 'node:assert/strict';
import { diagnoseMarkdown, buildMarkdownRepairPrompt } from '../../src/components/markdown/markdownDiagnostics.js';
import { submitMarkdownRepair } from '../../src/hooks/editorMarkdownRepair.js';

test('diagnostics identify likely broken fences, inline code, emphasis, links and table columns', () => {
  for (const [source, code] of [
    ['# title\n\n```bash\necho hello', 'fence'],
    ['Some `code here', 'inline-code'],
    ['Some **important text', 'emphasis'],
    ['[guide](https://example.com', 'link'],
    ['| a | b |\n| --- | --- |\n| missing |', 'table'],
    ['| a | b | c |\n| --- | --- |', 'table'],
  ]) assert.ok(diagnoseMarkdown(source).some(issue => issue.code === code), source);
  assert.deepEqual(diagnoseMarkdown('# title\n\n```bash\necho hello')[0], {line:3,code:'fence',message:'代码块围栏可能缺少匹配的结束标记'});
  assert.equal(diagnoseMarkdown('A hard break\\\n**open')[0].line, 2);
  assert.equal(diagnoseMarkdown('---\n\n```bash\ncode')[0].code, 'fence');
});

test('valid Markdown, escaped syntax, extensions and code examples do not trigger warnings', () => {
  const fixtures = [
    '**bold** and __bold__ and ~~removed~~ and *italic*',
    'An **emphasis\nacross lines** and `code\nwith **text`.',
    'Visit [guide](https://example.com/a_(b)) or [next](\nhttps://example.com\n).',
    '[guide](https://example.com "the (title)")',
    '\\**literal and \\`literal and \\[guide](not a link',
    '```markdown\n**broken\n[link](\n`sample\n```',
    '````markdown\n```\n**example\n`````',
    '> ```js\n> **example\n> ```',
    '    **indented code\n    `example',
    '| a \\| b | `x|y` |\n| --- | --- |\n| 1 | 2 |',
    '---\ntitle: "**configuration"\n---\n\n# title\n\n$math$\n\n[[wiki]]\n\n<details>body</details>',
    'word_with__literal_identifiers and 2 ** 3 = 8',
  ];
  for (const source of fixtures) assert.deepEqual(diagnoseMarkdown(source), [], source);
  assert.deepEqual(diagnoseMarkdown('x'.repeat(512 * 1024 + 1)), []);
});

test('diagnostics remain bounded and repair prompt targets one saved file without including its body', () => {
  const issues = diagnoseMarkdown(Array(100).fill('[link](broken').join('\n\n'));
  assert.equal(issues.length, 8);
  const prompt = buildMarkdownRepairPrompt({path:'目录/notes".md',hash:'saved-hash',issues});
  assert.ok(prompt.includes(JSON.stringify('目录/notes".md')));
  assert.ok(prompt.includes('saved-hash'));
  assert.ok(prompt.includes('read_workspace_text') && prompt.includes('apply_workspace_patch'));
  assert.ok(prompt.includes('可能误报') && prompt.includes('正文含义') && prompt.includes('代码原文'));
  assert.ok(!prompt.includes('[link](broken'));
});

function repairFixture() {
  const snapshot = {path:'notes.md',content:'```bash\necho hello',conversationId:'chat-1',modelId:'model-1'};
  let current = {...snapshot};
  const calls = [];
  const args = {snapshot,getCurrent:()=>current,
    prepareFile:async value=>{calls.push(['save',value.path]);return{hash:'saved-hash'};},
    send:async prompt=>{calls.push(['send',prompt]);}};
  return {args,calls,setCurrent:value=>{current=value;}};
}

test('explicit repair saves the exact snapshot before submitting its prompt', async () => {
  const {args,calls} = repairFixture();
  await submitMarkdownRepair(args);
  assert.equal(calls[0][0], 'save');assert.equal(calls[1][0], 'send');
  assert.ok(calls[1][1].includes('saved-hash'));
});

test('repair never submits after a save failure, conflict, changed file, draft, model or conversation', async () => {
  for (const change of [{path:'other.md'},{content:'new local draft'},{conversationId:'chat-2'},{modelId:'model-2'},{conflict:{}}]) {
    const fixture = repairFixture();
    fixture.args.prepareFile = async () => { fixture.setCurrent({...fixture.args.snapshot,...change});return{hash:'hash'}; };
    await assert.rejects(submitMarkdownRepair(fixture.args));
    assert.equal(fixture.calls.length, 0);
  }
  const fixture = repairFixture();
  fixture.args.prepareFile = async () => {throw new Error('disk/save failure');};
  await assert.rejects(submitMarkdownRepair(fixture.args), /disk\/save failure/);
  assert.equal(fixture.calls.length, 0);
});

test('missing model, existing conflict and already valid content cannot start a repair', async () => {
  for (const change of [{modelId:''},{conflict:{}},{content:'# valid\n'}]) {
    const fixture=repairFixture();
    Object.assign(fixture.args.snapshot,change);fixture.setCurrent({...fixture.args.snapshot});
    await assert.rejects(submitMarkdownRepair(fixture.args));assert.equal(fixture.calls.length,0);
  }
});
