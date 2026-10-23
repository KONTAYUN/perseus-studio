import { useLayoutEffect } from 'react';

const focusable = 'button:not(:disabled),input:not(:disabled),select:not(:disabled),textarea:not(:disabled),a[href],[tabindex]:not([tabindex="-1"])';

export default function useModalFocus(ref, active, onEscape, initialSelector) {
  useLayoutEffect(() => {
    if (!active || !ref.current) return undefined;
    const root = ref.current;
    const previous = document.activeElement;
    const blocked = [];
    // Also works for a dialog nested below the application root.
    for (let node = root; node?.parentElement; node = node.parentElement) {
      for (const sibling of node.parentElement.children) {
        if (sibling === node || !(sibling instanceof HTMLElement) || sibling.inert) continue;
        // Only restore state this hook changed. React may remove a settings-owned
        // inert attribute in the same commit that closes a discard dialog.
        blocked.push(sibling); sibling.inert = true;
      }
      if (node.parentElement === document.body) break;
    }
    const candidates = () => [...root.querySelectorAll(focusable)].filter(el => el.getClientRects().length && !el.closest('[hidden],[inert]'));
    const initial = root.querySelector(initialSelector || '[data-dialog-cancel]') || candidates()[0] || root;
    initial.focus();
    if (initial instanceof HTMLInputElement) initial.select();
    const onKey = (event) => {
      if (event.defaultPrevented) return;
      if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); onEscape(); }
      if (event.key !== 'Tab') return;
      const items = candidates();
      if (!items.length) { event.preventDefault(); root.focus(); return; }
      const first = items[0], last = items[items.length - 1];
      if (event.shiftKey && (document.activeElement === first || !root.contains(document.activeElement))) {
        event.preventDefault(); last.focus();
      } else if (!event.shiftKey && (document.activeElement === last || !root.contains(document.activeElement))) {
        event.preventDefault(); first.focus();
      }
    };
    root.addEventListener('keydown', onKey);
    return () => {
      root.removeEventListener('keydown', onKey);
      for (const node of blocked.reverse()) node.inert = false;
      if (previous?.isConnected && !previous.closest('[hidden],[inert]')) previous.focus();
    };
  }, [active, ref, onEscape, initialSelector]);
}
