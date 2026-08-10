import http from 'node:http';

const port = Number(process.env.PORT || 9090);
const scenarioRequests = new Map();

function writeSSEEvent(response, event) {
  if (event.id) response.write(`id: ${event.id}\n`);
  response.write(`data: ${JSON.stringify(event.data ?? event)}\n\n`);
}

function sendSSE(response, chunks) {
  response.writeHead(200, {
    'Content-Type': 'text/event-stream',
    'Cache-Control': 'no-cache',
    Connection: 'keep-alive',
  });
  for (const chunk of chunks) writeSSEEvent(response, chunk);
  response.end('data: [DONE]\n\n');
}

function toolCall(id, name, args, content = '', rawArguments = '') {
  return [
    { choices: [{ delta: content ? { content } : {} }] },
    {
      choices: [{
        delta: {
          tool_calls: [{
            index: 0,
            id,
            type: 'function',
            function: { name, arguments: rawArguments || JSON.stringify(args) },
          }],
        },
        finish_reason: 'tool_calls',
      }],
    },
  ];
}

function nonStreamingToolCall(response, id, name, args, content = '') {
  response.writeHead(200, { 'Content-Type': 'application/json' });
  response.end(JSON.stringify({
    choices: [{
      message: {
        role: 'assistant',
        content,
        tool_calls: [{ id, type: 'function', function: { name, arguments: JSON.stringify(args) } }],
      },
      finish_reason: 'tool_calls',
    }],
  }));
}

function messageText(message) {
  if (typeof message?.content === 'string') return message.content;
  if (!Array.isArray(message?.content)) return '';
  return message.content.map((part) => part?.text || '').join(' ');
}

function scenarioFromPayload(payload) {
  const text = Array.isArray(payload.messages) ? payload.messages.map(messageText).join(' ') : '';
  const match = text.match(/\[fixture:([a-z-]+)\]/);
  return match?.[1] || 'normal';
}

function lastAssistantToolCall(payload) {
  const messages = Array.isArray(payload.messages) ? payload.messages : [];
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const calls = messages[index]?.tool_calls;
    if (Array.isArray(calls) && calls.length > 0) return calls[calls.length - 1];
  }
  return null;
}

function lastToolResult(payload) {
  const messages = Array.isArray(payload.messages) ? payload.messages : [];
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    if (messages[index]?.role !== 'tool') continue;
    const content = messages[index]?.content;
    try {
      return typeof content === 'string' ? JSON.parse(content) : content;
    } catch {
      return null;
    }
  }
  return null;
}

function fixturePath(payload) {
  const text = Array.isArray(payload.messages) ? payload.messages.map(messageText).join(' ') : '';
  return text.match(/\[path:([^\]]+)\]/)?.[1] || 'e2e-agent-note.html';
}

function fileEditResponse(response, payload) {
  const path = fixturePath(payload);
  const lastCall = lastAssistantToolCall(payload);
  const lastResult = lastToolResult(payload);
  const lastName = lastCall?.function?.name || '';
  let lastArguments = {};
  try {
    lastArguments = JSON.parse(lastCall?.function?.arguments || '{}');
  } catch {
    lastArguments = {};
  }

  if (!lastCall) {
    sendSSE(response, toolCall('fixture-file-edit-load-tools', 'load_perseus_toolset', { toolset: 'workspace' }, 'Loading the workspace tools.'));
    return;
  }
  if (lastName === 'load_perseus_toolset') {
    sendSSE(response, toolCall('fixture-file-edit-index', 'read_workspace_note_view', { path, mode: 'index', view: 'structure' }, 'Indexing the target note.'));
    return;
  }
  if (lastName === 'read_workspace_note_view' && lastArguments.mode === 'index') {
    const block = Array.isArray(lastResult?.blocks)
      ? lastResult.blocks.find((item) => String(item?.text_preview || '').includes('Before agent edit'))
      : null;
    const version = lastResult?.document_version || lastResult?.doc_hash || '';
    if (!block?.block_id || !version) {
      response.writeHead(500, { 'Content-Type': 'application/json' });
      response.end('{"error":{"code":"fixture_index_contract_failed"}}');
      return;
    }
    sendSSE(response, toolCall('fixture-file-edit-raw', 'read_workspace_note_view', {
      path,
      mode: 'block',
      view: 'raw',
      block_id: block.block_id,
      document_version: version,
    }, 'Reading the exact block before editing.'));
    return;
  }
  if (lastName === 'read_workspace_note_view' && lastArguments.mode === 'block') {
    const version = lastResult?.document_version || lastResult?.doc_hash || lastArguments.document_version || '';
    const blockID = lastResult?.block_id || lastArguments.block_id || '';
    sendSSE(response, toolCall('fixture-file-edit-mutation', 'edit_workspace_note_blocks', {
      path,
      document_version: version,
      edits: [{
        block_id: blockID,
        operation: 'replace_text',
        find: 'Before agent edit',
        replace: 'After agent edit',
        expected_matches: 1,
      }],
    }, 'Applying the smallest verified note edit.'));
    return;
  }
  if (lastName === 'edit_workspace_note_blocks') {
    sendSSE(response, toolCall('fixture-file-edit-complete', 'attempt_completion', {
      result: 'E2E file-edit completed.',
    }, 'The note edit is committed and verified.'));
    return;
  }

  response.writeHead(500, { 'Content-Type': 'application/json' });
  response.end('{"error":{"code":"fixture_file_edit_state_failed"}}');
}

function initialToolChunks(scenario) {
  const commentary = scenario === 'replay'
    ? 'Replay-safe commentary.'
    : `Preparing the ${scenario} deterministic tools.`;
  return toolCall(`fixture-load-tools-${scenario}`, 'load_perseus_toolset', { toolset: 'workspace' }, commentary);
}

const server = http.createServer((request, response) => {
  if (request.method === 'GET' && request.url === '/health') {
    response.writeHead(200, { 'Content-Type': 'application/json' });
    response.end('{"ok":true}');
    return;
  }
  if (request.method !== 'POST' || !request.url?.endsWith('/chat/completions')) {
    response.writeHead(404).end();
    return;
  }
  let raw = '';
  request.setEncoding('utf8');
  request.on('data', (chunk) => { raw += chunk; });
  request.on('end', () => {
    let payload;
    try {
      payload = JSON.parse(raw);
    } catch {
      response.writeHead(400).end('{"error":{"code":"invalid_json"}}');
      return;
    }
    const scenario = scenarioFromPayload(payload);
    const scenarioKey = `${payload.model || 'unknown'}:${scenario}`;
    const requestNumber = (scenarioRequests.get(scenarioKey) || 0) + 1;
    scenarioRequests.set(scenarioKey, requestNumber);
    const hasToolResult = Array.isArray(payload.messages) && payload.messages.some((message) => message?.role === 'tool');
    if (scenario === 'file-edit') {
      fileEditResponse(response, payload);
      return;
    }
    if (hasToolResult) {
      sendSSE(response, toolCall(`fixture-complete-${scenario}`, 'attempt_completion', {
        result: `E2E ${scenario} completed.`,
      }));
      return;
    }

    if (scenario === 'user-input') {
      sendSSE(response, toolCall('fixture-user-input', 'request_user_input', {
        question: 'Paste the external command output here.',
      }, 'I need one external result to continue.'));
      return;
    }

    if (scenario === 'user-choice') {
      sendSSE(response, toolCall('fixture-user-choice', 'request_user_choice', {
        question: 'Choose the bounded update strategy.',
        options: ['Update in place', 'Create a copy'],
      }, 'A bounded decision is required.'));
      return;
    }

    if (scenario === 'context-overflow') {
      response.writeHead(400, {
        'Content-Type': 'application/json',
        'x-request-id': 'fixture-context-overflow-request',
      });
      response.end('{"error":{"code":"context_length_exceeded","type":"invalid_request_error","message":"fixture context limit"}}');
      return;
    }

    if (scenario === 'rate-limit' && requestNumber === 1) {
      response.writeHead(429, {
        'Content-Type': 'application/json',
        'Retry-After': '1',
        'x-request-id': 'fixture-rate-limit-request',
      });
      response.end('{"error":{"code":"rate_limit_exceeded","type":"rate_limit_error","message":"fixture limit"}}');
      return;
    }

    if (scenario === 'invalid-arguments' && requestNumber === 1) {
      sendSSE(response, toolCall(
        'fixture-invalid-arguments',
        'load_perseus_toolset',
        {},
        'Validating a malformed provider tool frame.',
        '{"toolset":',
      ));
      return;
    }

    if (scenario === 'eof' && requestNumber === 1 && payload.stream !== false) {
      response.writeHead(200, {
        'Content-Type': 'text/event-stream',
        'Cache-Control': 'no-cache',
        Connection: 'keep-alive',
      });
      writeSSEEvent(response, { id: 'fixture-eof-partial', data: { choices: [{ delta: { content: 'Recovering interrupted fixture stream.' } }] } });
      response.socket?.destroy();
      return;
    }

    const chunks = initialToolChunks(scenario);
    if (scenario === 'replay') {
      const replayed = { id: 'fixture-replayed-commentary', data: chunks[0] };
      sendSSE(response, [replayed, replayed, { id: 'fixture-replay-tool', data: chunks[1] }]);
      return;
    }
    if (payload.stream === false) {
      nonStreamingToolCall(response, `fixture-load-tools-${scenario}`, 'load_perseus_toolset', { toolset: 'workspace' }, `Recovered ${scenario} request.`);
      return;
    }
    sendSSE(response, chunks);
  });
});

server.listen(port, '0.0.0.0', () => {
  process.stdout.write(`fixture provider listening on ${port}\n`);
});
