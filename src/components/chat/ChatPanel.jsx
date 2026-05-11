import React from 'react';
import {
  ChevronDown,
  ChevronRight,
  Check,
  GripHorizontal,
  History,
  Home,
  Pencil,
  Plus,
  Trash2,
  X,
} from 'lucide-react';
import ChatComposer from './ChatComposer';
import ChatMessages from './ChatMessages';
import { formatDate } from './chatUtils';

const BOTTOM_STICK_THRESHOLD = 56;
const BOTTOM_BUTTON_THRESHOLD = 96;

function distanceFromBottom(element) {
  if (!element) return 0;
  return Math.max(0, element.scrollHeight - element.scrollTop - element.clientHeight);
}

export default function ChatPanel({ ide, panelId = 'chat' }) {
  const modelMenuRef = React.useRef(null);
  const modelSubmenuRef = React.useRef(null);
  const conversationMenuRef = React.useRef(null);
  const messagesRef = React.useRef(null);
  const composerRef = React.useRef(null);
  const bottomRef = React.useRef(null);
  const shouldStickToBottomRef = React.useRef(true);
  const didMountScrollRef = React.useRef(false);
  const previousConversationIdRef = React.useRef(ide.currentConversationId);
  const previousSendingRef = React.useRef(ide.isChatSending);
  const [composerHeight, setComposerHeight] = React.useState(176);
  const [isModelSubmenuOpen, setIsModelSubmenuOpen] = React.useState(false);
  const [showScrollToBottom, setShowScrollToBottom] = React.useState(false);
  const [modelSubmenuSide, setModelSubmenuSide] = React.useState('right');
  const [renamingConversation, setRenamingConversation] = React.useState(null);
  const [renameDraft, setRenameDraft] = React.useState('');
  const messagesBottomInset = composerHeight + 72;
  const thinkingModes = [
    ['standard', '标准'],
    ['deep', '深度'],
    ['expert', '专家'],
  ];
  const currentThinkingLabel = thinkingModes.find(([value]) => value === ide.thinkingMode)?.[1] || '标准';

  React.useEffect(() => {
    if (!ide.isModelMenuOpen) return undefined;

    const closeOnPointerDown = (event) => {
      if (modelMenuRef.current?.contains(event.target)) return;
      ide.setIsModelMenuOpen(false);
    };
    const closeOnEscape = (event) => {
      if (event.key === 'Escape') ide.setIsModelMenuOpen(false);
    };

    document.addEventListener('mousedown', closeOnPointerDown);
    document.addEventListener('keydown', closeOnEscape);
    return () => {
      document.removeEventListener('mousedown', closeOnPointerDown);
      document.removeEventListener('keydown', closeOnEscape);
    };
  }, [ide.isModelMenuOpen]);

  React.useEffect(() => {
    if (!ide.isModelMenuOpen) setIsModelSubmenuOpen(false);
  }, [ide.isModelMenuOpen]);

  React.useEffect(() => {
    if (!isModelSubmenuOpen) return undefined;
    const closeOnPointerDown = (event) => {
      if (modelMenuRef.current?.contains(event.target) || modelSubmenuRef.current?.contains(event.target)) return;
      setIsModelSubmenuOpen(false);
      ide.setIsModelMenuOpen(false);
    };
    const closeOnEscape = (event) => {
      if (event.key === 'Escape') {
        setIsModelSubmenuOpen(false);
        ide.setIsModelMenuOpen(false);
      }
    };
    document.addEventListener('mousedown', closeOnPointerDown);
    document.addEventListener('keydown', closeOnEscape);
    return () => {
      document.removeEventListener('mousedown', closeOnPointerDown);
      document.removeEventListener('keydown', closeOnEscape);
    };
  }, [isModelSubmenuOpen, ide]);

  React.useEffect(() => {
    if (!ide.isConversationMenuOpen) return undefined;

    const closeOnPointerDown = (event) => {
      if (conversationMenuRef.current?.contains(event.target)) return;
      ide.setIsConversationMenuOpen(false);
    };
    const closeOnEscape = (event) => {
      if (event.key === 'Escape') ide.setIsConversationMenuOpen(false);
    };

    document.addEventListener('mousedown', closeOnPointerDown);
    document.addEventListener('keydown', closeOnEscape);
    return () => {
      document.removeEventListener('mousedown', closeOnPointerDown);
      document.removeEventListener('keydown', closeOnEscape);
    };
  }, [ide.isConversationMenuOpen]);

  const currentConversation = ide.conversations.find((item) => item.id === ide.currentConversationId);
  const openRenameConversation = (conversation) => {
    setRenamingConversation(conversation);
    setRenameDraft(conversation.title || '新对话');
  };
  const closeRenameConversation = () => {
    setRenamingConversation(null);
    setRenameDraft('');
  };
  const submitRenameConversation = (e) => {
    e.preventDefault();
    if (!renamingConversation) return;
    const title = renameDraft.trim();
    if (title && title !== renamingConversation.title) {
      ide.renameConversation(renamingConversation.id, title);
    }
    closeRenameConversation();
  };

  const updateScrollStickiness = React.useCallback(() => {
    const element = messagesRef.current;
    if (!element) return;
    const scrollable = element.scrollHeight > element.clientHeight + 1;
    const distance = distanceFromBottom(element);
    const isNearBottom = !scrollable || distance <= BOTTOM_STICK_THRESHOLD;
    shouldStickToBottomRef.current = isNearBottom;
    const shouldShowButton = scrollable && distance > BOTTOM_BUTTON_THRESHOLD;
    setShowScrollToBottom((current) => (current === shouldShowButton ? current : shouldShowButton));
  }, []);

  const scrollToBottom = React.useCallback((behavior = 'auto', { force = false } = {}) => {
    if (force) {
      shouldStickToBottomRef.current = true;
      setShowScrollToBottom(false);
    }

    const run = () => {
      if (!force && !shouldStickToBottomRef.current) return;
      bottomRef.current?.scrollIntoView({ behavior, block: 'end' });
    };

    if (typeof window !== 'undefined' && window.requestAnimationFrame) {
      window.requestAnimationFrame(run);
    } else {
      run();
    }
  }, []);

  const submitChatMessage = React.useCallback(() => {
    shouldStickToBottomRef.current = true;
    setShowScrollToBottom(false);
    ide.sendChatMessage();
  }, [ide]);

  React.useEffect(() => {
    const element = messagesRef.current;
    if (!element) return undefined;
    const onScroll = () => updateScrollStickiness();
    element.addEventListener('scroll', onScroll, { passive: true });
    updateScrollStickiness();
    return () => element.removeEventListener('scroll', onScroll);
  }, [updateScrollStickiness]);

  React.useEffect(() => {
    const element = composerRef.current;
    if (!element) return undefined;

    const updateHeight = () => {
      const nextHeight = Math.ceil(element.getBoundingClientRect().height || 0);
      if (nextHeight > 0) {
        setComposerHeight((current) => (current === nextHeight ? current : nextHeight));
      }
    };

    updateHeight();

    if (typeof ResizeObserver !== 'undefined') {
      const observer = new ResizeObserver(() => updateHeight());
      observer.observe(element);
      return () => observer.disconnect();
    }

    window.addEventListener('resize', updateHeight);
    return () => window.removeEventListener('resize', updateHeight);
  }, []);

  React.useEffect(() => {
    const conversationChanged = previousConversationIdRef.current !== ide.currentConversationId;
    const startedSending = ide.isChatSending && !previousSendingRef.current;
    previousConversationIdRef.current = ide.currentConversationId;
    previousSendingRef.current = ide.isChatSending;

    if (!didMountScrollRef.current) {
      didMountScrollRef.current = true;
      if (ide.conversationMessages.length > 0) {
        scrollToBottom('auto', { force: true });
      }
      return;
    }

    if (conversationChanged || startedSending) {
      scrollToBottom('auto', { force: true });
      return;
    }

    if (shouldStickToBottomRef.current) {
      scrollToBottom('auto');
      return;
    }

    updateScrollStickiness();
  }, [
    ide.currentConversationId,
    ide.conversationMessages,
    ide.isChatSending,
    ide.chatError,
    messagesBottomInset,
    scrollToBottom,
    updateScrollStickiness,
  ]);

  return (
    <section
      id="panel-content-chat"
      data-testid="chat-panel"
      className={`relative h-full flex flex-col bg-white/85 dark:bg-[#1C1C1E]/85 backdrop-blur-2xl rounded-2xl overflow-visible transition-[border-color,box-shadow,background-color,transform] duration-300 ease-[cubic-bezier(0.19,1,0.22,1)] border ${ide.getPanelGlowStyle(
        panelId
      )}`}
      onClick={() => ide.setActivePanel(panelId)}
    >
      <div className="relative z-20 grid h-12 flex-none grid-cols-[104px_minmax(0,1fr)_104px] items-center gap-2 border-b border-gray-100 bg-transparent px-3 dark:border-white/5 group max-[420px]:grid-cols-[72px_minmax(0,1fr)_72px]">
        <div className="flex min-w-0 items-center text-gray-400">
          <span
            draggable
            onDragStart={(e) => ide.handlePanelDragStart(e, panelId)}
            onDragEnd={ide.handlePanelDragEnd}
            className="cursor-grab active:cursor-grabbing"
            title="拖动面板"
          >
            <GripHorizontal size={14} className="opacity-0 group-hover:opacity-100 transition-opacity" />
          </span>
        </div>

        <div className="flex min-w-0 justify-center">
          <div ref={modelMenuRef} className="relative min-w-0 max-w-full">
            <button
              data-testid="chat-model-button"
              onClick={(e) => {
                e.stopPropagation();
                ide.setActivePanel(panelId);
                ide.setIsModelMenuOpen(!ide.isModelMenuOpen);
              }}
              className="motion-button flex min-w-0 max-w-full items-center justify-center gap-1.5 overflow-hidden rounded-lg px-1.5 py-1 text-sm font-semibold text-gray-800 hover:text-blue-500 dark:text-gray-200 dark:hover:text-blue-400"
            >
              <span className="block min-w-0 truncate text-center">
                {ide.selectedModel?.name || '未配置模型'} · {currentThinkingLabel}
              </span>
              <ChevronDown size={14} className="flex-shrink-0 text-gray-400" />
            </button>

            {ide.isModelMenuOpen && (
              <div
                data-testid="chat-model-menu"
                className="motion-popover absolute left-1/2 top-8 z-[220] w-52 -translate-x-1/2 overflow-visible rounded-2xl border border-gray-100 bg-white/95 p-1.5 shadow-2xl backdrop-blur-xl dark:border-white/10 dark:bg-[#2C2C2E]/95"
              >
                {ide.enabledModels.length === 0 ? (
                  <div className="p-4 text-center">
                    <div className="text-sm font-bold text-gray-800 dark:text-gray-100">未配置模型</div>
                    <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">请先在设置页添加并启用模型。</div>
                    <button
                      data-testid="open-settings-from-model-menu"
                      onClick={(e) => {
                        e.stopPropagation();
                        ide.setIsModelMenuOpen(false);
                        ide.setIsSettingsOpen(true);
                      }}
                      className="mt-3 rounded-xl bg-blue-500 px-4 py-2 text-xs font-bold text-white hover:bg-blue-600"
                    >
                      打开设置
                    </button>
                  </div>
                ) : (
                  <>
                    <div className="px-2 pb-1 pt-1 text-[11px] font-semibold text-gray-400">思考强度</div>
                    {thinkingModes.map(([value, label]) => (
                      <button
                        key={value}
                        data-testid={`thinking-mode-${value}`}
                        onClick={(e) => {
                          e.stopPropagation();
                          ide.setThinkingMode?.(value);
                          ide.setIsModelMenuOpen(false);
                        }}
                        className={`motion-row flex w-full items-center justify-between rounded-xl px-3 py-2 text-left text-sm font-semibold ${
                          (ide.thinkingMode || 'standard') === value
                            ? 'bg-blue-50 text-blue-600 dark:bg-blue-500/15 dark:text-blue-300'
                            : 'text-gray-700 hover:bg-gray-50 dark:text-gray-200 dark:hover:bg-white/5'
                        }`}
                      >
                        <span>{label}</span>
                        {(ide.thinkingMode || 'standard') === value && <Check size={14} />}
                      </button>
                    ))}
                    <div className="my-1 border-t border-gray-100 dark:border-white/10" />
                    <div className="relative">
                      <button
                        type="button"
                      data-testid="model-submenu-trigger"
                        onClick={(e) => {
                          e.stopPropagation();
                          ide.setActivePanel(panelId);
                          const rect = e.currentTarget.getBoundingClientRect();
                          const submenuWidth = 256;
                          const gap = 8;
                          setModelSubmenuSide(
                            rect.right + gap + submenuWidth > window.innerWidth ? 'left' : 'right'
                          );
                          setIsModelSubmenuOpen((current) => !current);
                        }}
                        className="motion-row flex w-full items-center justify-between gap-3 rounded-xl px-3 py-2 text-left text-sm font-semibold text-gray-700 hover:bg-gray-50 dark:text-gray-200 dark:hover:bg-white/5"
                      >
                        <span className="min-w-0 truncate">{ide.selectedModel?.name || '未配置模型'}</span>
                        <ChevronRight size={14} className="shrink-0 text-gray-400" />
                      </button>
                      {isModelSubmenuOpen && (
                        <div
                          ref={modelSubmenuRef}
                          data-testid="model-submenu"
                          className={`motion-popover-right absolute top-0 z-[230] w-64 overflow-hidden rounded-2xl border border-gray-100 bg-white/95 p-1.5 shadow-2xl backdrop-blur-xl dark:border-white/10 dark:bg-[#2C2C2E]/95 ${
                            modelSubmenuSide === 'left' ? 'right-[calc(100%+8px)]' : 'left-[calc(100%+8px)]'
                          }`}
                        >
                          {ide.enabledModels.map((model) => (
                            <button
                              key={model.id}
                              data-testid={`model-option-${model.id}`}
                              onClick={(e) => {
                                e.stopPropagation();
                                ide.setSelectedModelId(model.id);
                                setIsModelSubmenuOpen(false);
                                ide.setIsModelMenuOpen(false);
                              }}
                              className={`motion-row flex w-full items-center gap-2 rounded-xl px-3 py-2.5 text-left ${
                                ide.selectedModel?.id === model.id
                                  ? 'bg-blue-50 text-blue-600 dark:bg-blue-500/15 dark:text-blue-300'
                                  : 'text-gray-700 hover:bg-gray-50 dark:text-gray-200 dark:hover:bg-white/5'
                              }`}
                            >
                              <span className="min-w-0 flex-1">
                                <span className="block truncate text-sm font-bold">{model.name}</span>
                                <span className="mt-0.5 block truncate text-[11px] text-gray-400">
                                  {model.protocol} · {model.model_id}
                                </span>
                              </span>
                              {ide.selectedModel?.id === model.id && <Check size={14} className="shrink-0" />}
                            </button>
                          ))}
                        </div>
                      )}
                    </div>
                  </>
                )}
              </div>
            )}
          </div>
        </div>

        <div className="flex min-w-0 items-center justify-end gap-1">
          {ide.currentConversationId && (
            <button
              data-testid="chat-home-button"
              onClick={(e) => {
                e.stopPropagation();
                ide.returnChatHome();
              }}
              className="motion-button flex h-8 w-8 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 hover:text-blue-500 dark:text-gray-400 dark:hover:bg-white/10 dark:hover:text-blue-300"
              title="返回首页"
            >
              <Home size={15} />
            </button>
          )}
          <button
            data-testid="new-conversation-button"
            onClick={(e) => {
              e.stopPropagation();
              ide.createNewConversation();
            }}
            className="motion-button flex h-8 w-8 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 hover:text-blue-500 dark:text-gray-400 dark:hover:bg-white/10 dark:hover:text-blue-300"
            title="新建会话"
          >
            <Plus size={15} />
          </button>
          <div ref={conversationMenuRef} className="relative">
            <button
              data-testid="conversation-history-button"
              onClick={(e) => {
                e.stopPropagation();
                ide.setIsConversationMenuOpen(!ide.isConversationMenuOpen);
              }}
              className="motion-button flex h-8 w-8 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 hover:text-blue-500 dark:text-gray-400 dark:hover:bg-white/10 dark:hover:text-blue-300"
              title="历史会话"
            >
              <History size={15} />
            </button>
            {ide.isConversationMenuOpen && (
              <div className="motion-popover absolute right-0 top-9 z-[230] w-80 overflow-hidden rounded-2xl border border-gray-100 bg-white/95 shadow-2xl backdrop-blur-xl dark:border-white/10 dark:bg-[#2C2C2E]/95">
                <div className="flex items-center justify-between border-b border-gray-100 px-3 py-2.5 dark:border-white/10">
                  <div>
                    <div className="text-sm font-bold text-gray-800 dark:text-gray-100">历史会话</div>
                    <div className="text-[11px] text-gray-400">
                      {currentConversation?.title || '当前没有选中会话'}
                    </div>
                  </div>
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      ide.createNewConversation();
                    }}
                    className="rounded-lg bg-blue-500 px-2.5 py-1.5 text-xs font-bold text-white hover:bg-blue-600"
                  >
                    新建
                  </button>
                </div>
                <div className="max-h-80 overflow-y-auto p-1.5 custom-scrollbar scrollbar-rounded-clip">
                  {ide.conversations.length === 0 ? (
                    <div className="px-3 py-6 text-center text-sm text-gray-500 dark:text-gray-400">
                      还没有历史会话。
                    </div>
                  ) : (
                    ide.conversations.map((conversation) => (
                      <div
                        key={conversation.id}
                        role="button"
                        tabIndex={0}
                        onClick={(e) => {
                          e.stopPropagation();
                          ide.selectConversation(conversation.id);
                        }}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter' || e.key === ' ') {
                            e.preventDefault();
                            e.stopPropagation();
                            ide.selectConversation(conversation.id);
                          }
                        }}
                        className={`motion-row group flex w-full items-center gap-2 rounded-xl px-3 py-2.5 text-left ${
                          conversation.id === ide.currentConversationId
                            ? 'bg-blue-50 text-blue-600 dark:bg-blue-500/15 dark:text-blue-300'
                            : 'text-gray-700 hover:bg-gray-50 dark:text-gray-200 dark:hover:bg-white/5'
                        }`}
                      >
                        <div className="min-w-0 flex-1">
                          <div className="truncate text-sm font-bold">{conversation.title || '新对话'}</div>
                          <div className="mt-0.5 truncate text-[11px] text-gray-400">
                            {formatDate(conversation.updated_at)}
                          </div>
                        </div>
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation();
                            openRenameConversation(conversation);
                          }}
                          className="motion-button rounded-lg p-1.5 text-gray-400 opacity-0 hover:bg-blue-50 hover:text-blue-500 group-hover:opacity-100 dark:hover:bg-blue-500/10 dark:hover:text-blue-300"
                          title="重命名会话"
                        >
                          <Pencil size={13} />
                        </button>
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation();
                            ide.deleteConversation(conversation.id);
                          }}
                          className="motion-button rounded-lg p-1.5 text-gray-400 opacity-0 hover:bg-red-50 hover:text-red-500 group-hover:opacity-100 dark:hover:bg-red-500/10"
                          title="删除会话"
                        >
                          <Trash2 size={13} />
                        </button>
                      </div>
                    ))
                  )}
                </div>
              </div>
            )}
          </div>
        </div>
      </div>

      <ChatMessages
        ide={ide}
        messagesRef={messagesRef}
        bottomRef={bottomRef}
        messagesBottomInset={messagesBottomInset}
      />

      {showScrollToBottom && ide.conversationMessages.length > 0 && (
        <button
          type="button"
          data-testid="scroll-to-bottom-button"
          onClick={(e) => {
            e.stopPropagation();
            scrollToBottom('smooth', { force: true });
          }}
          className="motion-soft-focus absolute left-1/2 z-30 flex h-8 w-11 items-center justify-center rounded-2xl border border-transparent bg-gray-950/[0.08] text-gray-600 shadow-[0_16px_38px_rgba(15,23,42,0.14),inset_0_1px_0_rgba(255,255,255,0.42)] backdrop-blur-2xl hover:bg-blue-500/12 hover:text-blue-600 hover:shadow-[0_18px_42px_rgba(37,99,235,0.18),inset_0_1px_0_rgba(255,255,255,0.5)] dark:bg-white/[0.09] dark:text-gray-300 dark:shadow-[0_18px_44px_rgba(0,0,0,0.36),inset_0_1px_0_rgba(255,255,255,0.12)] dark:hover:bg-blue-400/14 dark:hover:text-blue-200 dark:hover:shadow-[0_18px_44px_rgba(37,99,235,0.24),inset_0_1px_0_rgba(255,255,255,0.16)]"
          style={{ bottom: `${composerHeight + 18}px`, transform: 'translateX(-50%)' }}
          title="回到底部"
          aria-label="回到底部"
        >
          <ChevronDown size={17} />
        </button>
      )}

      <ChatComposer
        ide={ide}
        panelId={panelId}
        composerRef={composerRef}
        submitChatMessage={submitChatMessage}
      />
      {renamingConversation && (
        <div
          className="fixed inset-0 z-[320] flex items-center justify-center bg-black/45 p-4 backdrop-blur-sm"
          onMouseDown={(e) => {
            if (e.target === e.currentTarget) closeRenameConversation();
          }}
        >
          <form
            onSubmit={submitRenameConversation}
          className="motion-modal w-full max-w-md rounded-3xl border border-white/70 bg-white/95 p-5 shadow-2xl backdrop-blur-2xl dark:border-white/10 dark:bg-[#252527]/95"
          >
            <div className="flex items-start justify-between gap-4">
              <div>
                <div className="text-base font-bold text-gray-900 dark:text-gray-100">重命名会话</div>
                <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">给这段历史会话起一个更容易识别的名字。</div>
              </div>
              <button
                type="button"
                onClick={closeRenameConversation}
                className="rounded-xl p-2 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-white/10 dark:hover:text-gray-100"
                title="关闭"
              >
                <X size={16} />
              </button>
            </div>
            <input
              autoFocus
              value={renameDraft}
              onChange={(e) => setRenameDraft(e.target.value)}
              className="mt-5 h-11 w-full rounded-2xl border border-gray-200 bg-white px-4 text-sm font-semibold text-gray-800 outline-none transition-all focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 dark:border-white/10 dark:bg-[#1C1C1E] dark:text-white"
              placeholder="会话名称"
            />
            <div className="mt-5 flex justify-end gap-2">
              <button
                type="button"
                onClick={closeRenameConversation}
                className="rounded-2xl bg-gray-100 px-4 py-2 text-sm font-bold text-gray-600 transition hover:bg-gray-200 dark:bg-white/10 dark:text-gray-300 dark:hover:bg-white/15"
              >
                取消
              </button>
              <button
                type="submit"
                className="rounded-2xl bg-blue-500 px-4 py-2 text-sm font-bold text-white shadow-lg shadow-blue-500/20 transition hover:bg-blue-600"
              >
                保存
              </button>
            </div>
          </form>
        </div>
      )}
    </section>
  );
}
