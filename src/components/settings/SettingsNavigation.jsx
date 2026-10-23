import React from 'react';

export const SettingsNavigation = React.createContext(null);

export function useSettingsGuard({ dirty = false, busy = false, discard = () => {} }) {
  const navigation = React.useContext(SettingsNavigation);
  React.useLayoutEffect(() => {
    if (!navigation) return undefined;
    const guard = { dirty, busy, discard };
    navigation.guard.current = guard;
    return () => { if (navigation.guard.current === guard) navigation.guard.current = null; };
  }, [navigation, dirty, busy, discard]);
  return navigation;
}

export function useSettingsNavigation(dialog) {
  const guard = React.useRef(null);
  const checking = React.useRef(false);
  const [notice, setNotice] = React.useState('');
  const attemptLeave = React.useCallback(async (action) => {
    if (checking.current) return;
    if (guard.current?.busy) {
      setNotice('操作正在提交，请等待结果后再离开。');
      return;
    }
    checking.current = true;
    try {
      if (guard.current?.dirty) {
        const ok = await dialog.confirm({ title: '放弃未保存的修改？',
          message: '当前表单还有未保存的输入。放弃后无法恢复；已经保存或通过加号添加的配置不会被撤销。',
          confirmLabel: '放弃修改', cancelLabel: '继续编辑', tone: 'red' });
        if (!ok) return;
        guard.current?.discard();
      }
      setNotice('');
      action();
    } finally { checking.current = false; }
  }, [dialog]);
  React.useEffect(() => {
    const protect = (event) => {
      if (!guard.current?.dirty && !guard.current?.busy) return;
      event.preventDefault(); event.returnValue = '';
    };
    window.addEventListener('beforeunload', protect);
    return () => window.removeEventListener('beforeunload', protect);
  }, []);
  const value = React.useMemo(() => ({ guard, attemptLeave }), [attemptLeave]);
  return { value, notice };
}
