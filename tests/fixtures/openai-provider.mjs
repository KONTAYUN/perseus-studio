import http from 'node:http';

const port = Number(process.env.PORT || 9090);
const scenarioRequests = new Map();
const fixtureStats = { activeRequests: 0, peakActiveRequests: 0, totalRequests: 0 };

function writeSSEEvent(response, event) {
  if (event.id) response.write(`id: ${event.id}\n`);
  if (event.event) response.write(`event: ${event.event}\n`);
  response.write(`data: ${JSON.stringify(event.data ?? event)}\n\n`);
}

const wait = (delay) => new Promise((resolve) => setTimeout(resolve, delay));

async function sendSSE(response, chunks, delayMs = 0) {
  response.writeHead(200, {
    'Content-Type': 'text/event-stream',
    'Cache-Control': 'no-cache',
    Connection: 'keep-alive',
  });
  for (const chunk of chunks) {
    writeSSEEvent(response, chunk);
    if (delayMs > 0) await wait(delayMs);
  }
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

function isAnthropicRequest(request) {
  return request.url?.endsWith('/messages');
}

function isSnapshotRequest(request) {
  return request.url?.includes('/snapshot/');
}

function splitArguments(raw) {
  if (raw.length < 3) return [raw];
  const first = Math.max(1, Math.floor(raw.length / 3));
  const second = Math.max(first + 1, Math.floor((raw.length * 2) / 3));
  return [raw.slice(0, first), raw.slice(first, second), raw.slice(second)];
}

function snapshotArgumentFrames(raw) {
  try {
    const parsed = JSON.parse(raw);
    const replacement = parsed?.edits?.[0]?.replace;
    if (typeof replacement !== 'string' || replacement.length < 30) return [raw];
    const cut1 = Math.floor(replacement.length / 3);
    const cut2 = Math.floor((replacement.length * 2) / 3);
    return [cut1, cut2, replacement.length].map((length) => JSON.stringify({
      ...parsed,
      edits: [{ ...parsed.edits[0], replace: replacement.slice(0, length) }, ...parsed.edits.slice(1)],
    }));
  } catch {
    return [raw];
  }
}

function openAIToolChunks(id, name, rawArguments, content, snapshot) {
  const chunks = [];
  if (content) chunks.push({ id: `${id}-text`, data: { choices: [{ delta: { content } }] } });
  const parts = snapshot ? snapshotArgumentFrames(rawArguments) : splitArguments(rawArguments);
  let cumulative = '';
  for (let index = 0; index < parts.length; index += 1) {
    if (!snapshot) cumulative += parts[index];
    chunks.push({
      id: `${id}-args-${index}`,
      data: {
        choices: [{
          delta: {
            tool_calls: [{
              index: 0,
              ...(index === 0 ? { id, type: 'function' } : {}),
              function: {
                ...(index === 0 ? { name } : {}),
                arguments: parts[index],
              },
            }],
          },
          ...(index === parts.length - 1 ? { finish_reason: 'tool_calls' } : {}),
        }],
      },
    });
  }
  return chunks;
}

function anthropicToolChunks(id, name, rawArguments, content) {
  const chunks = [{
    id: `${id}-message-start`,
    event: 'message_start',
    data: { type: 'message_start', message: { id: `msg-${id}`, type: 'message', role: 'assistant', usage: { input_tokens: 10, output_tokens: 0 } } },
  }];
  let blockIndex = 0;
  if (content) {
    chunks.push({
      id: `${id}-text-start`, event: 'content_block_start',
      data: { type: 'content_block_start', index: blockIndex, content_block: { type: 'text', text: '' } },
    });
    chunks.push({
      id: `${id}-text-delta`, event: 'content_block_delta',
      data: { type: 'content_block_delta', index: blockIndex, delta: { type: 'text_delta', text: content } },
    });
    chunks.push({ id: `${id}-text-stop`, event: 'content_block_stop', data: { type: 'content_block_stop', index: blockIndex } });
    blockIndex += 1;
  }
  chunks.push({
    id: `${id}-tool-start`, event: 'content_block_start',
    data: { type: 'content_block_start', index: blockIndex, content_block: { type: 'tool_use', id, name, input: {} } },
  });
  for (const [index, part] of splitArguments(rawArguments).entries()) {
    chunks.push({
      id: `${id}-args-${index}`, event: 'content_block_delta',
      data: { type: 'content_block_delta', index: blockIndex, delta: { type: 'input_json_delta', partial_json: part } },
    });
  }
  chunks.push({ id: `${id}-tool-stop`, event: 'content_block_stop', data: { type: 'content_block_stop', index: blockIndex } });
  chunks.push({
    id: `${id}-message-delta`, event: 'message_delta',
    data: { type: 'message_delta', delta: { stop_reason: 'tool_use', stop_sequence: null }, usage: { output_tokens: 10 } },
  });
  chunks.push({ id: `${id}-message-stop`, event: 'message_stop', data: { type: 'message_stop' } });
  return chunks;
}

function nonStreamingAnthropicToolCall(response, id, name, args, content = '') {
  response.writeHead(200, { 'Content-Type': 'application/json' });
  response.end(JSON.stringify({
    id: `msg-${id}`,
    type: 'message',
    role: 'assistant',
    content: [
      ...(content ? [{ type: 'text', text: content }] : []),
      { type: 'tool_use', id, name, input: args },
    ],
    stop_reason: 'tool_use',
    usage: { input_tokens: 10, output_tokens: 10 },
  }));
}

async function respondTool(response, request, payload, id, name, args, content = '', rawArguments = '') {
  const raw = rawArguments || JSON.stringify(args);
  if (payload.stream === false) {
    if (isAnthropicRequest(request)) nonStreamingAnthropicToolCall(response, id, name, args, content);
    else nonStreamingToolCall(response, id, name, args, content);
    return;
  }
  const chunks = isAnthropicRequest(request)
    ? anthropicToolChunks(id, name, raw, content)
    : openAIToolChunks(id, name, raw, content, isSnapshotRequest(request));
  const delayMs = payload.model?.includes('slow')
    ? (name === 'edit_workspace_note_blocks' ? 650 : 60)
    : 0;
  await sendSSE(response, chunks, delayMs);
}

async function respondTools(response, request, payload, calls, content = '') {
  if (isAnthropicRequest(request)) {
    const chunks = [{
      id: 'fixture-multi-message-start', event: 'message_start',
      data: { type: 'message_start', message: { id: 'msg-fixture-multi', type: 'message', role: 'assistant', usage: { input_tokens: 10, output_tokens: 0 } } },
    }];
    let blockIndex = 0;
    if (content) {
      chunks.push({ id: 'fixture-multi-text-start', event: 'content_block_start', data: { type: 'content_block_start', index: blockIndex, content_block: { type: 'text', text: content } } });
      chunks.push({ id: 'fixture-multi-text-stop', event: 'content_block_stop', data: { type: 'content_block_stop', index: blockIndex } });
      blockIndex += 1;
    }
    for (const call of calls) {
      chunks.push({ id: `${call.id}-start`, event: 'content_block_start', data: { type: 'content_block_start', index: blockIndex, content_block: { type: 'tool_use', id: call.id, name: call.name, input: {} } } });
      chunks.push({ id: `${call.id}-args`, event: 'content_block_delta', data: { type: 'content_block_delta', index: blockIndex, delta: { type: 'input_json_delta', partial_json: JSON.stringify(call.args) } } });
      chunks.push({ id: `${call.id}-stop`, event: 'content_block_stop', data: { type: 'content_block_stop', index: blockIndex } });
      blockIndex += 1;
    }
    chunks.push({ id: 'fixture-multi-message-delta', event: 'message_delta', data: { type: 'message_delta', delta: { stop_reason: 'tool_use' }, usage: { output_tokens: 10 } } });
    chunks.push({ id: 'fixture-multi-message-stop', event: 'message_stop', data: { type: 'message_stop' } });
    await sendSSE(response, chunks);
    return;
  }

  const toolCalls = calls.map((call, index) => ({
    index,
    id: call.id,
    type: 'function',
    function: { name: call.name, arguments: JSON.stringify(call.args) },
  }));
  await sendSSE(response, [
    ...(content ? [{ id: 'fixture-multi-text', data: { choices: [{ delta: { content } }] } }] : []),
    { id: 'fixture-multi-tools', data: { choices: [{ delta: { tool_calls: toolCalls }, finish_reason: 'tool_calls' }] } },
  ]);
}

function messageText(message) {
  if (typeof message?.content === 'string') return message.content;
  if (!Array.isArray(message?.content)) return '';
  return message.content.map((part) => {
    if (typeof part?.text === 'string') return part.text;
    if (typeof part?.content === 'string') return part.content;
    if (Array.isArray(part?.content)) return part.content.map((item) => item?.text || '').join(' ');
    return '';
  }).join(' ');
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
    const content = messages[index]?.content;
    if (Array.isArray(content)) {
      const block = [...content].reverse().find((item) => item?.type === 'tool_use');
      if (block) {
        return {
          id: block.id,
          function: { name: block.name, arguments: JSON.stringify(block.input || {}) },
        };
      }
    }
  }
  return null;
}

function lastToolResult(payload) {
  const messages = Array.isArray(payload.messages) ? payload.messages : [];
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    if (messages[index]?.role === 'user' && Array.isArray(messages[index]?.content)) {
      const block = [...messages[index].content].reverse().find((item) => item?.type === 'tool_result');
      if (block) {
        try {
          const content = Array.isArray(block.content)
            ? block.content.map((item) => item?.text || '').join('')
            : block.content;
          return typeof content === 'string' ? JSON.parse(content) : content;
        } catch {
          return null;
        }
      }
    }
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

function fixturePaths(payload) {
  const text = Array.isArray(payload.messages) ? payload.messages.map(messageText).join(' ') : '';
  return text.match(/\[paths:([^\]]+)\]/)?.[1]?.split(',').map((item) => item.trim()).filter(Boolean) || [];
}

function hasProviderImage(payload) {
  const messages = Array.isArray(payload.messages) ? payload.messages : [];
  return messages.some((message) => Array.isArray(message?.content) && message.content.some((part) => (
    part?.type === 'image_url' || part?.type === 'input_image' || part?.type === 'image'
  )));
}

async function fileEditResponse(response, request, payload, scenario) {
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
    await respondTool(response, request, payload, 'fixture-file-edit-load-tools', 'load_perseus_toolset', { toolset: 'workspace' }, 'Loading the workspace tools.');
    return;
  }
  if (lastName === 'load_perseus_toolset') {
    await respondTool(response, request, payload, 'fixture-file-edit-index', 'read_workspace_note_view', { path, mode: 'index', view: 'structure' }, 'Indexing the target note.');
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
    await respondTool(response, request, payload, 'fixture-file-edit-raw', 'read_workspace_note_view', {
      path,
      mode: 'block',
      view: 'raw',
      block_id: block.block_id,
      document_version: version,
    }, 'Reading the exact block before editing.');
    return;
  }
  if (lastName === 'read_workspace_note_view' && lastArguments.mode === 'block') {
    const version = lastResult?.document_version || lastResult?.doc_hash || lastArguments.document_version || '';
    const blockID = lastResult?.block_id || lastArguments.block_id || '';
    const replacement = scenario === 'file-edit-long'
      ? `After agent edit ${'streaming preview content '.repeat(80)}`
      : 'After agent edit';
    await respondTool(response, request, payload, 'fixture-file-edit-mutation', 'edit_workspace_note_blocks', {
      path,
      document_version: version,
      edits: [{
        block_id: blockID,
        operation: 'replace_text',
        find: 'Before agent edit',
        replace: replacement,
        expected_matches: 1,
      }],
    }, 'Applying the smallest verified note edit.');
    return;
  }
  if (lastName === 'edit_workspace_note_blocks') {
    await respondTool(response, request, payload, 'fixture-file-edit-complete', 'attempt_completion', {
      result: `E2E ${scenario} completed.`,
    }, 'The note edit is committed and verified.');
    return;
  }

  response.writeHead(500, { 'Content-Type': 'application/json' });
  response.end('{"error":{"code":"fixture_file_edit_state_failed"}}');
}

async function fileCreateResponse(response, request, payload) {
  const path = fixturePath(payload);
  const lastCall = lastAssistantToolCall(payload);
  const lastName = lastCall?.function?.name || '';

  if (!lastCall) {
    await respondTool(response, request, payload, 'fixture-file-create-load-tools', 'load_perseus_toolset', { toolset: 'workspace' }, 'Loading the workspace tools.');
    return;
  }
  if (lastName === 'load_perseus_toolset') {
    const patch = [
      '*** Begin Patch',
      `*** Add File: ${path}`,
      '+# Seedance storyboard',
      '+',
      '+A deterministic streamed Markdown file.',
      '+',
      '+```text',
      '+streamed fixture text',
      '+```',
      '*** End Patch',
    ].join('\n');
    await respondTool(response, request, payload, 'fixture-file-create-mutation', 'apply_workspace_patch', { patch }, 'Creating one streamed workspace file.');
    return;
  }
  if (lastName === 'apply_workspace_patch') {
    await respondTool(response, request, payload, 'fixture-file-create-complete', 'attempt_completion', {
      result: 'E2E file-create completed.',
    }, 'The workspace file is committed and visible.');
    return;
  }

  response.writeHead(500, { 'Content-Type': 'application/json' });
  response.end('{"error":{"code":"fixture_file_create_state_failed"}}');
}

function initialToolChunks(scenario) {
  const commentary = scenario === 'replay'
    ? 'Replay-safe commentary.'
    : `Preparing the ${scenario} deterministic tools.`;
  return toolCall(`fixture-load-tools-${scenario}`, 'load_perseus_toolset', { toolset: 'workspace' }, commentary);
}

function hasToolResult(payload) {
  return Array.isArray(payload.messages) && payload.messages.some((message) => (
    message?.role === 'tool' ||
    (message?.role === 'user' && Array.isArray(message.content) && message.content.some((item) => item?.type === 'tool_result'))
  ));
}

function sendProviderError(response, request, status, code, type, message, headers = {}) {
  response.writeHead(status, { 'Content-Type': 'application/json', ...headers });
  if (isAnthropicRequest(request)) {
    response.end(JSON.stringify({ type: 'error', error: { type, message } }));
    return;
  }
  response.end(JSON.stringify({ error: { code, type, message } }));
}

const server = http.createServer((request, response) => {
  if (request.method === 'GET' && request.url === '/health') {
    response.writeHead(200, { 'Content-Type': 'application/json' });
    response.end('{"ok":true}');
    return;
  }
  if (request.method === 'GET' && request.url === '/stats') {
    response.writeHead(200, { 'Content-Type': 'application/json' });
    response.end(JSON.stringify({ ...fixtureStats, scenarios: Object.fromEntries(scenarioRequests) }));
    return;
  }
  if (request.method === 'POST' && request.url === '/stats/reset') {
    fixtureStats.activeRequests = 0;
    fixtureStats.peakActiveRequests = 0;
    fixtureStats.totalRequests = 0;
    scenarioRequests.clear();
    response.writeHead(200, { 'Content-Type': 'application/json' });
    response.end('{"ok":true}');
    return;
  }
  if (request.method !== 'POST' || (!request.url?.endsWith('/chat/completions') && !request.url?.endsWith('/messages'))) {
    response.writeHead(404).end();
    return;
  }
  fixtureStats.activeRequests += 1;
  fixtureStats.totalRequests += 1;
  fixtureStats.peakActiveRequests = Math.max(fixtureStats.peakActiveRequests, fixtureStats.activeRequests);
  let activeReleased = false;
  const releaseActive = () => {
    if (activeReleased) return;
    activeReleased = true;
    fixtureStats.activeRequests = Math.max(0, fixtureStats.activeRequests - 1);
  };
  response.once('finish', releaseActive);
  response.once('close', releaseActive);
  let raw = '';
  request.setEncoding('utf8');
  request.on('data', (chunk) => { raw += chunk; });
  request.on('end', async () => {
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
    const containsToolResult = hasToolResult(payload);
    if (scenario === 'load') {
      await wait(250);
      await respondTool(response, request, payload, `fixture-load-complete-${requestNumber}`, 'attempt_completion', {
        result: 'E2E load completed.',
      });
      return;
    }
    if (scenario === 'file-edit' || scenario === 'file-edit-long') {
      await fileEditResponse(response, request, payload, scenario);
      return;
    }
    if (scenario === 'file-create') {
      await fileCreateResponse(response, request, payload);
      return;
    }
    if (scenario === 'image' && !containsToolResult && !hasProviderImage(payload)) {
      sendProviderError(response, request, 400, 'fixture_image_contract_failed', 'invalid_request_error', 'fixture image input missing');
      return;
    }
    if (scenario === 'multi-read') {
      const lastCall = lastAssistantToolCall(payload);
      if (!lastCall) {
        await respondTool(response, request, payload, 'fixture-multi-load', 'load_perseus_toolset', { toolset: 'workspace' }, 'Loading read tools.');
        return;
      }
      if (lastCall.function?.name === 'load_perseus_toolset') {
        const paths = fixturePaths(payload);
        if (paths.length !== 2) {
          sendProviderError(response, request, 400, 'fixture_paths_required', 'invalid_request_error', 'two fixture paths required');
          return;
        }
        await respondTools(response, request, payload, [
          { id: 'fixture-multi-read-a', name: 'read_workspace_text', args: { path: paths[0] } },
          { id: 'fixture-multi-read-b', name: 'read_workspace_text', args: { path: paths[1] } },
        ], 'Reading two independent files in one provider turn.');
        return;
      }
    }
    if (containsToolResult) {
      await respondTool(response, request, payload, `fixture-complete-${scenario}`, 'attempt_completion', {
        result: `E2E ${scenario} completed.`,
      });
      return;
    }

    if (scenario === 'user-input') {
      await respondTool(response, request, payload, 'fixture-user-input', 'request_user_input', {
        question: 'Paste the external command output here.',
      }, 'I need one external result to continue.');
      return;
    }

    if (scenario === 'user-choice') {
      await respondTool(response, request, payload, 'fixture-user-choice', 'request_user_choice', {
        question: 'Choose the bounded update strategy.',
        options: ['Update in place', 'Create a copy'],
      }, 'A bounded decision is required.');
      return;
    }

    if (scenario === 'context-overflow') {
      sendProviderError(response, request, isAnthropicRequest(request) ? 413 : 400, 'context_length_exceeded', isAnthropicRequest(request) ? 'request_too_large' : 'invalid_request_error', 'fixture context limit', {
        'x-request-id': 'fixture-context-overflow-request',
      });
      return;
    }

    if (scenario === 'rate-limit' && requestNumber === 1) {
      sendProviderError(response, request, 429, 'rate_limit_exceeded', 'rate_limit_error', 'fixture limit', {
        'Retry-After': '1',
        'x-request-id': 'fixture-rate-limit-request',
      });
      return;
    }

    if (scenario === 'invalid-arguments' && requestNumber === 1) {
      await respondTool(response, request, payload,
        'fixture-invalid-arguments',
        'load_perseus_toolset',
        {},
        'Validating a malformed provider tool frame.',
        '{"toolset":',
      );
      return;
    }

    if (scenario === 'eof' && requestNumber === 1 && payload.stream !== false) {
      response.writeHead(200, {
        'Content-Type': 'text/event-stream',
        'Cache-Control': 'no-cache',
        Connection: 'keep-alive',
      });
      if (isAnthropicRequest(request)) {
        writeSSEEvent(response, {
          id: 'fixture-eof-message-start', event: 'message_start',
          data: { type: 'message_start', message: { id: 'msg-fixture-eof', type: 'message', role: 'assistant', usage: { input_tokens: 10, output_tokens: 0 } } },
        });
        writeSSEEvent(response, {
          id: 'fixture-eof-text-start', event: 'content_block_start',
          data: { type: 'content_block_start', index: 0, content_block: { type: 'text', text: '' } },
        });
        writeSSEEvent(response, {
          id: 'fixture-eof-text-delta', event: 'content_block_delta',
          data: { type: 'content_block_delta', index: 0, delta: { type: 'text_delta', text: 'Recovering interrupted fixture stream.' } },
        });
      } else {
        writeSSEEvent(response, { id: 'fixture-eof-partial', data: { choices: [{ delta: { content: 'Recovering interrupted fixture stream.' } }] } });
      }
      response.socket?.destroy();
      return;
    }

    const chunks = initialToolChunks(scenario);
    if (scenario === 'replay' && !isAnthropicRequest(request)) {
      const replayed = { id: 'fixture-replayed-commentary', data: chunks[0] };
      await sendSSE(response, [replayed, replayed, { id: 'fixture-replay-tool', data: chunks[1] }]);
      return;
    }
    await respondTool(response, request, payload, `fixture-load-tools-${scenario}`, 'load_perseus_toolset', { toolset: 'workspace' }, `Preparing the ${scenario} deterministic tools.`);
  });
});

server.listen(port, '0.0.0.0', () => {
  process.stdout.write(`fixture provider listening on ${port}\n`);
});
