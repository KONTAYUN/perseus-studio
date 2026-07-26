import React from 'react';
import { useStickToBottom } from 'use-stick-to-bottom';

export function useChatScrollController(ide) {
  const composerRef = React.useRef(null);
  const previousConversationIdRef = React.useRef(ide.currentConversationId);
  const previousSendingRef = React.useRef(ide.isChatSending);
  const pendingConversationScrollRef = React.useRef(true);
  const [composerHeight, setComposerHeight] = React.useState(176);
  const messagesBottomInset = composerHeight + 72;
  const messagesScrollbarBottomInset = Math.max(28, composerHeight - 140);
  const {
    contentRef: messagesContentRef,
    isAtBottom,
    scrollRef: messagesRef,
    scrollToBottom: stickToBottom,
  } = useStickToBottom({ initial: 'instant' });

  const scrollToBottom = React.useCallback((behavior = 'auto') => {
    const options = behavior === 'smooth'
      ? { animation: {} }
      : { animation: 'instant' };
    return stickToBottom(options);
  }, [stickToBottom]);

  const submitChatMessage = React.useCallback(() => {
    void scrollToBottom('smooth');
    ide.sendChatMessage();
  }, [ide, scrollToBottom]);

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

    if (conversationChanged) {
      pendingConversationScrollRef.current = true;
    }

    if (pendingConversationScrollRef.current && ide.conversationMessages.length > 0) {
      pendingConversationScrollRef.current = false;
      void scrollToBottom('auto');
      return;
    }

    if (startedSending) {
      void scrollToBottom('smooth');
    }
  }, [
    ide.currentConversationId,
    ide.conversationMessages.length,
    ide.isChatSending,
    scrollToBottom,
  ]);

  return {
    composerHeight,
    composerRef,
    isAtBottom,
    messagesBottomInset,
    messagesContentRef,
    messagesScrollbarBottomInset,
    messagesRef,
    scrollToBottom,
    showScrollToBottom: !isAtBottom,
    submitChatMessage,
  };
}
