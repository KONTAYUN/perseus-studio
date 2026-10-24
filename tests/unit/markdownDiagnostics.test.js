import test from 'node:test';
import assert from 'node:assert/strict';
import { diagnoseMarkdown, buildMarkdownRepairPrompt } from '../../src/components/markdown/markdownDiagnostics.js';
import { submitMarkdownRepair } from '../../src/hooks/editorMarkdownRepair.js';

test('permissive Markdown and legitimate unknown syntax never become damage by marker matching', () => {
  for (const source of ['```bash\necho hello', 'Some `code', '**important text', '[guide](https://example.com', '| a | b |\n| --- | --- |\n| missing |', '* ** `', '[[wiki]]', '<details>raw</details>', 'x'.repeat(600000)]) assert.deepEqual(diagnoseMarkdown(source), [], source.slice(0, 100));
  assert.deepEqual(diagnoseMarkdown('正文\n\0'), [{ line: 2, code: 'nul_character', message: '文稿包含无法可靠保存的空字符' }]);
});
test('repair prompt contains only the target, version and typed diagnostic', () => {
  const prompt = buildMarkdownRepairPrompt({ path: '目录/notes".md', hash: 'saved-hash', issues: [{ code: 'nul_character', message: '空字符' }] });
  assert.ok(prompt.includes(JSON.stringify('目录/notes".md')) && prompt.includes('saved-hash'));
});
function repairFixture() {
  const snapshot = {path:'notes.md',content:'正文\0',conversationId:'chat-1',modelId:'model-1'};
  let current = {...snapshot}; const calls=[];
  const args={snapshot,getCurrent:()=>current,prepareFile:async value=>{calls.push(['save',value.path]);return{hash:'saved-hash'};},send:async(prompt,options)=>{calls.push(['send',prompt,options]);return true;}};
  return {args,calls,setCurrent:value=>{current=value;}};
}
test('explicit repair saves the exact version and submits a structured authorization', async () => {
  const {args,calls}=repairFixture();await submitMarkdownRepair(args);
  assert.equal(calls[0][0],'save');assert.equal(calls[1][2].documentAction.base_hash,'saved-hash');
  assert.equal(calls[1][2].documentAction.type,'format_repair');
});
test('repair never sends after a save failure, conflict, changed file, draft, model or conversation', async () => {
  for (const change of [{path:'other.md'},{content:'new draft'},{conversationId:'chat-2'},{modelId:'model-2'},{conflict:{}}]) {
    const fixture=repairFixture();fixture.args.prepareFile=async()=>{fixture.setCurrent({...fixture.args.snapshot,...change});return{hash:'hash'};};
    await assert.rejects(submitMarkdownRepair(fixture.args));assert.equal(fixture.calls.length,0);
  }
  const fixture=repairFixture();fixture.args.prepareFile=async()=>{throw new Error('disk failure');};await assert.rejects(submitMarkdownRepair(fixture.args),/disk failure/);
});
test('missing model, existing conflict and legal literal markers cannot start repairs', async()=>{
  for(const change of [{modelId:''},{conflict:{}},{content:'** literal'}]){const fixture=repairFixture();Object.assign(fixture.args.snapshot,change);fixture.setCurrent({...fixture.args.snapshot});await assert.rejects(submitMarkdownRepair(fixture.args));assert.equal(fixture.calls.length,0);}
});
