import React from 'react';

const BOTTOM_STICK_THRESHOLD = 56;
const BOTTOM_BUTTON_THRESHOLD = 96;

function distanceFromBottom(element) {
  if (!element) return 0;
  return Math.max(0, element.scrollHeight - element.scrollTop - element.clientHeight);
}

export function useChatScrollController(ide) {
  const messagesRef = React.useRef(null);
  const composerRef = React.useRef(null);
  const bottomRef = React.useRef(null);
  const shouldStickToBottomRef = React.useRef(true);
  const didMountScrollRef = React.useRef(false);
  const previousConversationIdRef = React.useRef(ide.currentConversationId);
  const previousSendingRef = React.useRef(ide.isChatSending);
  const [composerHeight, setComposerHeight] = React.useState(176);
  const [showScrollToBottom, setShowScrollToBottom] = React.useState(false);
  const messagesBottomInset = composerHeight + 72;
  const messagesScrollbarBottomInset = Math.max(28, composerHeight - 140);

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

  return {
    bottomRef,
    composerHeight,
    composerRef,
    messagesBottomInset,
    messagesScrollbarBottomInset,
    messagesRef,
    scrollToBottom,
    showScrollToBottom,
    submitChatMessage,
  };
}
