import assert from 'node:assert/strict';
import test from 'node:test';

import {
  diagnosticShortJudgement,
  formatRequestSummary,
  parseRequestSummary,
} from '../../src/components/settings/diagnosticLogUtils.js';

test('parseRequestSummary and formatRequestSummary handle JSON safely', () => {
  const raw = '{"failure_class":"provider_bad_request_with_tools","tool_names":["attempt_completion"]}';
  assert.equal(parseRequestSummary(raw).failure_class, 'provider_bad_request_with_tools');
  assert.equal(formatRequestSummary(raw).includes('\n  "tool_names": ['), true);
  assert.equal(formatRequestSummary('not-json'), 'not-json');
});

test('diagnosticShortJudgement summarizes provider request failures', () => {
  assert.equal(
    diagnosticShortJudgement({
      request_summary: '{"failure_class":"provider_rejected_required_tool_choice"}',
    }),
    '上游拒绝强制工具调用'
  );
  assert.equal(
    diagnosticShortJudgement({
      request_summary: '{"failure_class":"provider_bad_request_with_tools"}',
    }),
    '上游拒绝工具调用请求'
  );
  assert.equal(
    diagnosticShortJudgement({
      request_summary: '{"failure_class":"provider_transient_transport"}',
    }),
    '上游连接临时中断'
  );
});

test('diagnosticShortJudgement falls back for old diagnostics', () => {
  assert.equal(
    diagnosticShortJudgement({
      http_status: 400,
      request_summary: '{"tools_count":3}',
    }),
    '上游拒绝工具请求'
  );
  assert.equal(
    diagnosticShortJudgement({
      error_message: 'context canceled',
    }),
    '用户或浏览器取消请求'
  );
});
