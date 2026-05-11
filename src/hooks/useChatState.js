import { useEffect, useRef, useState } from 'react';
import { API_BASE_URL, apiDelete, apiGet, apiPatch, apiPost, apiPostFormWithSignal } from '../api/client';
import {
  filesFromClipboard,
  filesToChatAttachments,
  mergeChatAttachments,
  mergeSkillReference,
  mergeWorkspaceReferences,
  normalizeSkillId,
} from './chatReferenceUtils.js';
import {
  agentEventText,
  appendUniqueProgressItem,
  buildAssistantRunShell,
  buildProgressItem,
  hasStreamingAssistantMessage,
  isPendingStatusText,
  mapPendingAssistant,
  nextLoadingStatus,
  normalizeConversationMessage,
  normalizeConversationMessages,
  updateLoadingState,
} from './chatStateUtils.js';

export default function useChatState({
  buildCurrentFileContext,
  confirm,
  onAssistantToolEvents,
  onTasksChanged,
  selectedModel,
  setSelectedModelId,
} = {}) {
  const [chatAttachments, setChatAttachments] = useState([]);
  const [isConversationMenuOpen, setIsConversationMenuOpen] = useState(false);
  const [conversations, setConversations] = useState([]);
  const [currentConversationId, setCurrentConversationId] = useState('');
  const [conversationMessages, setConversationMessages] = useState([]);
  const [chatInput, setChatInput] = useState('');
  const [chatFileReferences, setChatFileReferences] = useState([]);
  const [chatSkillReferences, setChatSkillReferences] = useState([]);
  const [queuedChatMessages, setQueuedChatMessages] = useState([]);
  const [isChatSending, setIsChatSending] = useState(false);
  const [chatError, setChatError] = useState('');
  const [resumableRun, setResumableRun] = useState(null);
  const [thinkingMode, setThinkingMode] = useState('standard');

  const suppressNextConversationLoadRef = useRef(false);
  const activeChatAbortRef = useRef(null);
  const conversationMessagesRef = useRef([]);

  useEffect(() => {
    conversationMessagesRef.current = conversationMessages;
  }, [conversationMessages]);

  function formatChatError(error) {
    const base = error?.message || String(error || '未知错误');
    if (error?.diagnosticLogId && !base.includes('诊断日志 ID')) {
      return `${base}\n已记录诊断日志 ID：${error.diagnosticLogId}，可在 设置 → 诊断日志 中导出。`;
    }
    return base;
  }

  useEffect(() => {
    if (!currentConversationId) {
      setConversationMessages([]);
      return;
    }
    if (suppressNextConversationLoadRef.current) {
      suppressNextConversationLoadRef.current = false;
      return;
    }
    loadConversationMessages(currentConversationId);
  }, [currentConversationId]);

  useEffect(() => {
    if (isChatSending || queuedChatMessages.length === 0) return;
    const [nextMessage, ...rest] = queuedChatMessages;
    setQueuedChatMessages(rest);
    sendChatPayload(nextMessage);
  }, [isChatSending, queuedChatMessages]);

  async function loadConversations() {
    setChatError('');
    setResumableRun(null);
    try {
      const data = await apiGet('/api/conversations');
      const items = data.items || [];
      setConversations(items);
      if (currentConversationId && !items.some((item) => item.id === currentConversationId)) {
        setCurrentConversationId('');
        setConversationMessages([]);
      }
    } catch (error) {
      console.error('loadConversations failed', error);
      setChatError(error.message);
    }
  }

  async function loadConversationMessages(conversationID) {
    if (!conversationID) {
      setConversationMessages([]);
      return;
    }
    try {
      const data = await apiGet(`/api/conversations/${encodeURIComponent(conversationID)}/messages`);
      setConversationMessages(normalizeConversationMessages(data.items || []));
    } catch (error) {
      console.error('loadConversationMessages failed', error);
      setChatError(error.message);
    }
  }

  async function ensureConversation(modelProviderID) {
    if (currentConversationId) return currentConversationId;
    const conversation = await apiPost('/api/conversations', {
      selected_model_provider_id: modelProviderID,
    });
    suppressNextConversationLoadRef.current = true;
    setCurrentConversationId(conversation.id);
    await loadConversations();
    return conversation.id;
  }

  async function createNewConversation() {
    if (!selectedModel?.id) {
      setChatError('请先在设置页配置并启用模型');
      return;
    }
    setChatError('');
    setResumableRun(null);
    try {
      const conversation = await apiPost('/api/conversations', {
        selected_model_provider_id: selectedModel.id,
      });
      setCurrentConversationId(conversation.id);
      setConversationMessages([]);
      setIsConversationMenuOpen(false);
      await loadConversations();
    } catch (error) {
      console.error('createNewConversation failed', error);
      setChatError(error.message);
    }
  }

  async function selectConversation(conversationID) {
    setCurrentConversationId(conversationID);
    setIsConversationMenuOpen(false);
    await loadConversationMessages(conversationID);
    const conversation = conversations.find((item) => item.id === conversationID);
    if (conversation?.selected_model_provider_id) {
      setSelectedModelId?.(conversation.selected_model_provider_id);
    }
  }

  async function deleteConversation(conversationID) {
    setChatError('');
    setResumableRun(null);
    try {
      await apiDelete(`/api/conversations/${encodeURIComponent(conversationID)}`);
      if (currentConversationId === conversationID) {
        setCurrentConversationId('');
        setConversationMessages([]);
      }
      await loadConversations();
    } catch (error) {
      console.error('deleteConversation failed', error);
      setChatError(error.message);
    }
  }

  async function renameConversation(conversationID, title) {
    const nextTitle = title.trim();
    if (!nextTitle) return;
    setChatError('');
    setResumableRun(null);
    try {
      const updated = await apiPatch(`/api/conversations/${encodeURIComponent(conversationID)}`, {
        title: nextTitle,
      });
      setConversations((current) =>
        current.map((item) => (item.id === conversationID ? { ...item, ...updated } : item))
      );
      await loadConversations();
    } catch (error) {
      console.error('renameConversation failed', error);
      setChatError(error.message);
    }
  }

  async function branchConversation(messageID) {
    if (!currentConversationId || !messageID) return;
    setChatError('');
    try {
      const result = await apiPost(`/api/conversations/${encodeURIComponent(currentConversationId)}/branch`, {
        message_id: messageID,
      });
      suppressNextConversationLoadRef.current = true;
      setCurrentConversationId(result.conversation.id);
      setConversationMessages(normalizeConversationMessages(result.messages || []));
      setIsConversationMenuOpen(false);
      await loadConversations();
    } catch (error) {
      console.error('branchConversation failed', error);
      setChatError(error.message);
    }
  }

  async function regenerateLastAssistantMessage(messageID) {
    if (!currentConversationId || !messageID || isChatSending) return;
    const selectedIndex = conversationMessages.findIndex((item) => item.id === messageID);
    const selectedMessage = selectedIndex >= 0 ? conversationMessages[selectedIndex] : null;
    let lastUserIndex = -1;
    conversationMessages.forEach((item, index) => {
      if (item.role === 'user') lastUserIndex = index;
    });
    if (!selectedMessage || selectedMessage.role !== 'assistant' || selectedIndex <= lastUserIndex) return;
    const assistantTail = conversationMessages.slice(lastUserIndex + 1).filter((item) => item.role === 'assistant');
    const hasMutationTool = assistantTail.some((message) => (message.attachments || []).some((item) =>
      ['save_workspace_file', 'manage_workspace_path', 'organize_workspace_matches', 'run_workspace_command'].includes(item?.tool_name)
    ));
    if (hasMutationTool) {
      const ok = await confirm?.({
        title: '重新生成回复',
        message: '重新生成不会回滚已经执行过的文件修改，确认继续？',
        confirmLabel: '继续重新生成',
        tone: 'red',
      });
      if (!ok) return;
    }

    setChatError('');
    setIsChatSending(true);
    const thinkingId = `temp_regenerate_${Date.now()}`;
    const control = { abort: () => {} };
    activeChatAbortRef.current = control;
    setConversationMessages((current) =>
      [
        ...current.slice(0, Math.max(0, lastUserIndex + 1)),
        buildAssistantRunShell({
          id: thinkingId,
          conversationId: currentConversationId,
          createdAt: new Date().toISOString(),
          content: '',
          attachments: [],
        }),
      ]
    );
    try {
      const result = await apiPost(`/api/conversations/${encodeURIComponent(currentConversationId)}/regenerate-last`, {
        message_id: messageID,
        thinking_mode: thinkingMode,
      });
      if (result.run?.id) {
        await waitForAgentRun(result.run.id, result.conversation.id, thinkingId, control);
      } else {
        await loadConversations();
        await loadConversationMessages(result.conversation.id);
      }
      if (result.task) {
        onTasksChanged?.(result.task);
      }
      await onAssistantToolEvents?.(result.assistant_message?.attachments || []);
    } catch (error) {
      console.error('regenerateLastAssistantMessage failed', error);
      const message = formatChatError(error);
      setChatError(message);
      setConversationMessages((current) =>
        mapPendingAssistant(current, thinkingId, (item) => ({
          ...item,
          pending: false,
          error: true,
          content: `重新生成失败：${message}`,
        }))
      );
    } finally {
      if (activeChatAbortRef.current === control) {
        activeChatAbortRef.current = null;
      }
      setIsChatSending(false);
    }
  }

  function returnChatHome() {
    setCurrentConversationId('');
    setConversationMessages([]);
    setChatError('');
    setResumableRun(null);
    setIsConversationMenuOpen(false);
  }

  async function sendChatMessage() {
    const trimmedContent = chatInput.trim();
    if (!trimmedContent && chatAttachments.length === 0 && chatFileReferences.length === 0 && chatSkillReferences.length === 0) return;
    if (!selectedModel?.id) {
      setChatError('请先在设置页配置并启用模型');
      return;
    }
    const payload = {
      content: trimmedContent,
      attachments: chatAttachments.map((item) => ({
        id: item.id,
        name: item.name,
        size: item.size,
        type: item.type,
        file: item.file,
      })),
      fileReferences: chatFileReferences,
      skillReferences: chatSkillReferences,
      currentFileContext: buildCurrentFileContext?.() || null,
      thinkingMode,
    };

    setChatInput('');
    setChatAttachments([]);
    setChatFileReferences([]);
    setChatSkillReferences([]);

    if (isChatSending) {
      setQueuedChatMessages((current) => [...current, payload]);
      setConversationMessages((current) => [
        ...current,
        {
          id: `queued_user_${Date.now()}`,
          conversation_id: currentConversationId || 'pending',
          role: 'user',
          content: trimmedContent,
          attachments: [
            ...payload.attachments.map(({ file, ...item }) => item),
            ...payload.fileReferences,
            ...payload.skillReferences,
          ],
          created_at: new Date().toISOString(),
          queued: true,
        },
      ]);
      return;
    }

    await sendChatPayload(payload);
  }

  async function sendChatPayload(payload) {
    const trimmedContent = payload.content.trim();
    const stamp = Date.now();
    const optimisticId = `temp_user_${stamp}`;
    const thinkingId = `temp_assistant_${stamp}`;
    const attachments = payload.attachments.map((item) => ({
      id: item.id,
      name: item.name,
      size: item.size,
      type: item.type,
    }));
    const metadataAttachments = [
      ...attachments,
      ...(payload.fileReferences || []),
      ...(payload.skillReferences || []),
      ...(payload.currentFileContext ? [payload.currentFileContext] : []),
    ];
    const optimisticMessage = {
      id: optimisticId,
      conversation_id: currentConversationId || 'pending',
      role: 'user',
      content: trimmedContent,
      attachments: metadataAttachments,
      created_at: new Date().toISOString(),
    };
    const thinkingMessage = buildAssistantRunShell({
      id: thinkingId,
      conversationId: currentConversationId || 'pending',
      createdAt: new Date().toISOString(),
      content: '',
      attachments: [],
    });
    const filesToSend = payload.attachments.map((item) => item.file).filter(Boolean);

    setChatError('');
    setResumableRun(null);
    setConversationMessages((current) => [
      ...current.filter((item) => !(item.queued && item.content === trimmedContent)),
      optimisticMessage,
      thinkingMessage,
    ]);
    setIsChatSending(true);

    let conversationID = currentConversationId;
    const abortController = new AbortController();
    const control = { abort: () => abortController.abort() };
    activeChatAbortRef.current = control;
    try {
      conversationID = await ensureConversation(selectedModel.id);
      const formData = new FormData();
      formData.append('content', trimmedContent);
      formData.append('model_provider_id', selectedModel.id);
      formData.append('thinking_mode', payload.thinkingMode || thinkingMode || 'standard');
      for (const attachment of payload.attachments) {
        formData.append('attachment_ids[]', attachment.id);
      }
      formData.append('workspace_refs_json', JSON.stringify(payload.fileReferences || []));
      formData.append('selected_skills_json', JSON.stringify(payload.skillReferences || []));
      if (payload.currentFileContext) {
        formData.append('current_file_context_json', JSON.stringify(payload.currentFileContext));
      }
      for (const file of filesToSend) {
        formData.append('files[]', file, file.name);
      }
      const result = await apiPostFormWithSignal(
        `/api/conversations/${encodeURIComponent(conversationID)}/messages`,
        formData,
        abortController.signal
      );
      setCurrentConversationId(result.conversation.id);
      if (result.run?.id) {
        await waitForAgentRun(result.run.id, result.conversation.id, thinkingId, control);
      } else {
        await loadConversations();
        await loadConversationMessages(result.conversation.id);
        if (result.task) {
          onTasksChanged?.(result.task);
        }
        await onAssistantToolEvents?.(result.assistant_message?.attachments || []);
      }
    } catch (error) {
      console.error('sendChatMessage failed', error);
      if (error?.name === 'AbortError') {
        setChatError('');
        setConversationMessages((current) =>
          mapPendingAssistant(current, thinkingId, (item) => ({
            ...item,
            pending: false,
            canceled: true,
            content: '已停止生成。',
          }))
        );
        return;
      }
      const message = formatChatError(error);
      setChatError(message);
      if (error?.runId) {
        setResumableRun({
          runId: error.runId,
          conversationId: error.conversationId || conversationID,
          diagnosticLogId: error.diagnosticLogId || '',
        });
      }
      const prefix = error?.runId ? '连接中断，可继续这轮任务' : '回复失败';
      setConversationMessages((current) =>
        mapPendingAssistant(current, thinkingId, (item) => ({
          ...item,
          pending: false,
          error: true,
          resumableRunId: error?.runId || '',
          diagnosticLogId: error?.diagnosticLogId || '',
          content: `${prefix}：${message}`,
        }))
      );
    } finally {
      if (activeChatAbortRef.current === control) {
        activeChatAbortRef.current = null;
      }
      setIsChatSending(false);
    }
  }

  function stopChatGeneration() {
    activeChatAbortRef.current?.abort();
  }

  async function continueAgentRun(runId = resumableRun?.runId, conversationID = resumableRun?.conversationId || currentConversationId) {
    if (!runId || !conversationID || isChatSending) return;
    const thinkingId = `temp_resume_${Date.now()}`;
    setChatError('');
    setResumableRun(null);
    setIsChatSending(true);
    setConversationMessages((current) => [
      ...current,
      buildAssistantRunShell({
        id: thinkingId,
        conversationId: conversationID,
        createdAt: new Date().toISOString(),
        content: '',
        attachments: [],
      }),
    ]);
    const control = { abort: () => {} };
    activeChatAbortRef.current = control;
    try {
      const result = await apiPost(`/api/agent-runs/${encodeURIComponent(runId)}/resume`, {});
      const afterSeq = result?.resume_event?.seq || 0;
      await waitForAgentRun(runId, conversationID, thinkingId, control, afterSeq);
    } catch (error) {
      console.error('continueAgentRun failed', error);
      const message = formatChatError(error);
      setChatError(message);
      if (error?.runId) {
        setResumableRun({
          runId: error.runId,
          conversationId: error.conversationId || conversationID,
          diagnosticLogId: error.diagnosticLogId || '',
        });
      }
      const prefix = error?.runId ? '连接中断，可继续这轮任务' : '继续失败';
      setConversationMessages((current) =>
        mapPendingAssistant(current, thinkingId, (item) => ({
          ...item,
          pending: false,
          error: true,
          resumableRunId: error?.runId || '',
          diagnosticLogId: error?.diagnosticLogId || '',
          content: `${prefix}：${message}`,
        }))
      );
    } finally {
      if (activeChatAbortRef.current === control) {
        activeChatAbortRef.current = null;
      }
      setIsChatSending(false);
    }
  }

  async function waitForAgentRun(runId, conversationID, thinkingId, control, afterSeq = 0) {
    setConversationMessages((current) =>
      mapPendingAssistant(current, thinkingId, (item) => ({
        ...item,
        loadingState: {
          text: nextLoadingStatus(0),
          visibleText: nextLoadingStatus(0),
          stage: 'holding',
          activeSince: Date.now(),
        },
      }))
    );

    let source = null;
    let settled = false;
    let fallbackStarted = false;
    let loadingTimer = null;
    const closeSource = () => {
      if (source) {
        source.close();
        source = null;
      }
      if (loadingTimer) {
        window.clearInterval(loadingTimer);
        loadingTimer = null;
      }
    };
    const refreshFromLatestAssistantMessage = async () => {
      try {
        const data = await apiGet(`/api/conversations/${encodeURIComponent(conversationID)}/messages`);
        const messages = normalizeConversationMessages(data.items || []);
        setConversationMessages(messages);
        const lastAssistant = [...messages].reverse().find((message) => message.role === 'assistant');
        await onAssistantToolEvents?.(lastAssistant?.attachments || []);
      } catch (error) {
        console.error('refreshFromLatestAssistantMessage failed', error);
      }
    };
    control.abort = () => {
      closeSource();
      settled = true;
      apiPost(`/api/agent-runs/${encodeURIComponent(runId)}/cancel`, {}).catch((error) => {
        console.error('cancelAgentRun failed', error);
      });
    };

    const appendProgress = (type, payload = {}, fallbackText = '') => {
      const nextItem = buildProgressItem(type, payload, fallbackText);
      if (!nextItem) return;
      setConversationMessages((current) =>
        mapPendingAssistant(current, thinkingId, (item) => ({
          ...item,
          progressItems: appendUniqueProgressItem(item.progressItems, nextItem),
        }))
      );
    };
    const ensureThinkingShell = () => {
      setConversationMessages((current) => {
        if (current.some((item) => item.id === thinkingId)) return current;
        if (hasStreamingAssistantMessage(current)) return current;
        return [
          ...current,
          buildAssistantRunShell({
            id: thinkingId,
            conversationId: conversationID,
            createdAt: new Date().toISOString(),
            content: '',
            attachments: [],
          }),
        ];
      });
    };
    const finish = async () => {
      closeSource();
      await loadConversations();
      await refreshFromLatestAssistantMessage();
      settled = true;
    };

    const fail = (message, diagnosticLogId) => {
      closeSource();
      const error = new Error(message || '执行失败');
      if (diagnosticLogId) error.diagnosticLogId = diagnosticLogId;
      error.runId = runId;
      error.conversationId = conversationID;
      settled = true;
      throw error;
    };

    const handleRun = async (run) => {
      if (!run) return;
      if (run.status === 'succeeded') {
        await finish();
      } else if (run.status === 'failed' || run.status === 'canceled') {
        await fail(run.error_message || '执行失败', run.diagnostic_log_id);
      }
    };

    const handleEvent = async (item) => {
      const payload = item?.payload || {};
      const message = agentEventText(item?.type, payload) || payload.message;
      if (item?.type === 'assistant_message_started') {
        const assistant = normalizeConversationMessage(payload.assistant_message);
        if (assistant) {
          setConversationMessages((current) => {
            const hasShell = current.some((messageItem) => messageItem.id === thinkingId);
            if (!hasShell) {
              return [...current, { ...assistant, pending: true }];
            }
            return current.map((messageItem) =>
              messageItem.id === thinkingId
                ? {
                    ...assistant,
                    pending: true,
                    activeMessageId: assistant.id,
                    loadingState: messageItem.loadingState,
                    progressItems: messageItem.progressItems || [],
                  }
                : messageItem
            );
          });
        }
      } else if (item?.type === 'assistant_message_delta') {
        const messageId = payload.message_id;
        const delta = payload.delta || '';
        if (messageId && delta) {
          setConversationMessages((current) =>
            current.map((messageItem) =>
              (messageItem.id === thinkingId && messageItem.activeMessageId === messageId) || messageItem.id === messageId
                ? {
                    ...messageItem,
                    content: `${messageItem.content || ''}${delta}`,
                    pending: true,
                  }
                : messageItem
            )
          );
        }
      } else if (item?.type === 'tool_call_completed') {
        if (payload.status === 'success' && payload.tool_name) {
          await onAssistantToolEvents?.([
            {
              tool_name: payload.tool_name,
              status: payload.status,
              path: payload.path || '',
              old_path: payload.old_path || '',
              new_path: payload.new_path || '',
              changed_paths: payload.changed_paths || [],
              hash: payload.hash || '',
              version_id: payload.version_id || '',
            },
          ]);
        }
      } else if (item?.type === 'model_message_started') {
        ensureThinkingShell();
      } else if (message && !['assistant_message_completed', 'run_completed', 'model_delta'].includes(item?.type)) {
        appendProgress(item?.type, payload, message);
      }
      if (item?.type === 'assistant_message_completed') {
        const assistant = normalizeConversationMessage(payload.assistant_message);
        if (assistant) {
          setConversationMessages((current) =>
            current.map((messageItem) =>
              (messageItem.id === thinkingId && (!messageItem.activeMessageId || messageItem.activeMessageId === assistant.id)) ||
              messageItem.id === assistant.id
                ? {
                    ...assistant,
                    pending: false,
                    progressItems: messageItem.progressItems || [],
                  }
                : messageItem
            )
          );
          await onAssistantToolEvents?.(assistant.attachments || []);
        }
        if (payload.task) {
          onTasksChanged?.(payload.task);
        }
      } else if (item?.type === 'workspace_file_changed') {
        if (payload.status === 'success' && payload.tool_name) {
          await onAssistantToolEvents?.([
            {
              tool_name: payload.tool_name,
              status: payload.status || 'success',
              path: payload.path || '',
              old_path: payload.old_path || '',
              new_path: payload.new_path || '',
              changed_paths: payload.changed_paths || [],
              hash: payload.hash || '',
              version_id: payload.version_id || '',
            },
          ]);
        }
      } else if (item?.type === 'run_failed') {
        await fail(payload.error || '执行失败', payload.diagnostic_log_id);
      }
    };

    const pollUntilDone = async () => {
      fallbackStarted = true;
      for (let attempt = 0; attempt < 120 && !settled; attempt += 1) {
        const run = await apiGet(`/api/agent-runs/${encodeURIComponent(runId)}`);
        await handleRun(run);
        if (settled) return;
        await new Promise((resolve) => window.setTimeout(resolve, 2000));
      }
      throw new Error('执行仍在进行，但事件流连接中断，请刷新会话查看结果');
    };

    loadingTimer = window.setInterval(() => {
      if (settled) return;
      setConversationMessages((current) =>
        mapPendingAssistant(current, thinkingId, (item) => updateLoadingState(item))
      );
    }, 80);

    await new Promise((resolve, reject) => {
      const streamPath = `${API_BASE_URL}/api/agent-runs/${encodeURIComponent(runId)}/stream${
        afterSeq ? `?after_seq=${encodeURIComponent(afterSeq)}` : ''
      }`;
      source = new EventSource(streamPath, {
        withCredentials: true,
      });
      control.abort = () => {
        closeSource();
        settled = true;
        apiPost(`/api/agent-runs/${encodeURIComponent(runId)}/cancel`, {}).catch((error) => {
          console.error('cancelAgentRun failed', error);
        });
        const error = new DOMException('Aborted', 'AbortError');
        reject(error);
      };
      source.addEventListener('run', (event) => {
        Promise.resolve(handleRun(JSON.parse(event.data)))
          .then(() => {
            if (settled) resolve();
          })
          .catch(reject);
      });
      source.addEventListener('event', (event) => {
        Promise.resolve(handleEvent(JSON.parse(event.data)))
          .then(() => {
            if (settled) resolve();
          })
          .catch(reject);
      });
      source.onerror = () => {
        closeSource();
        if (!fallbackStarted && !settled) {
          pollUntilDone().then(resolve).catch(reject);
        }
      };
    });
  }

  function addChatAttachments(fileList) {
    const nextFiles = filesToChatAttachments(fileList);
    if (nextFiles.length === 0) return;
    setChatAttachments((current) => mergeChatAttachments(current, nextFiles));
  }

  function addClipboardAttachments(clipboardData) {
    const files = filesFromClipboard(clipboardData);
    addChatAttachments(files);
    return files.length;
  }

  function removeChatAttachment(id) {
    setChatAttachments((current) => current.filter((item) => item.id !== id));
  }

  function addWorkspaceFileReferences(refs) {
    setChatFileReferences((current) => mergeWorkspaceReferences(current, refs));
  }

  function removeWorkspaceFileReference(path) {
    setChatFileReferences((current) => current.filter((item) => item.path !== path));
  }

  function addSkillReference(skill) {
    setChatSkillReferences((current) => mergeSkillReference(current, skill));
  }

  function removeSkillReference(skillId) {
    const normalized = normalizeSkillId(skillId);
    setChatSkillReferences((current) => current.filter((item) => normalizeSkillId(item.skill_id) !== normalized));
  }

  return {
    addChatAttachments,
    addClipboardAttachments,
    addSkillReference,
    addWorkspaceFileReferences,
    chatAttachments,
    chatError,
    chatFileReferences,
    chatInput,
    chatSkillReferences,
    continueAgentRun,
    branchConversation,
    conversationMessages,
    conversations,
    createNewConversation,
    currentConversationId,
    deleteConversation,
    isChatSending,
    isConversationMenuOpen,
    loadConversationMessages,
    loadConversations,
    removeChatAttachment,
    removeSkillReference,
    removeWorkspaceFileReference,
    resumableRun,
    regenerateLastAssistantMessage,
    renameConversation,
    returnChatHome,
    selectConversation,
    sendChatMessage,
    stopChatGeneration,
    setChatInput,
    setCurrentConversationId,
    setIsConversationMenuOpen,
    setThinkingMode,
    thinkingMode,
  };
}
