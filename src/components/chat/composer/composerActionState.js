export function composerPrimaryButtonState({
  hasDraft = false,
  isChatSending = false,
  isSendDisabled = false,
} = {}) {
  const isStopAction = Boolean(isChatSending && !hasDraft);
  if (isStopAction) {
    return {
      disabled: false,
      title: '停止生成',
      toneClass: 'composer-stop-button',
      action: 'stop',
    };
  }
  if (isSendDisabled) {
    return {
      disabled: true,
      title: isChatSending ? '挂起发送' : '发送',
      toneClass: 'composer-send-button-disabled',
      action: 'send',
    };
  }
  return {
    disabled: false,
    title: isChatSending ? '挂起发送' : '发送',
    toneClass: 'composer-send-button',
    action: 'send',
  };
}
