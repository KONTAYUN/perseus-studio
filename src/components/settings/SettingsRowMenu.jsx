import React from 'react';
import { MoreHorizontal } from 'lucide-react';
import { Button } from './SettingsUI';

export default function SettingsRowMenu({ label, actions, text }) {
  const ref = React.useRef(null);
  const position = () => {
    if (!ref.current?.open) return;
    const trigger = ref.current.querySelector('summary').getBoundingClientRect();
    const menu = ref.current.querySelector('.settings-row-menu-content');
    const above = trigger.top - 12, below = window.innerHeight - trigger.bottom - 12;
    const upwards = below < menu.scrollHeight + 4 && above > below;
    ref.current.dataset.placement = upwards ? 'top' : 'bottom';
    menu.style.maxHeight = `${Math.max(48, (upwards ? above : below) - 4)}px`;
  };
  React.useEffect(() => {
    const close = event => {
      if (!ref.current?.open) return;
      if (event.type === 'click' && ref.current.contains(event.target)) return;
      if (event.type === 'keydown' && event.key !== 'Escape') return;
      ref.current.open = false;
      if (event.type === 'keydown') { event.preventDefault(); ref.current.querySelector('summary').focus(); }
    };
    document.addEventListener('click', close); document.addEventListener('keydown', close);
    return () => { document.removeEventListener('click', close); document.removeEventListener('keydown', close); };
  }, []);
  return <details className="settings-row-menu" ref={ref} onToggle={position}>
    <summary aria-label={label}>{text || <MoreHorizontal size={18} />}</summary>
    <div className="settings-row-menu-content">
      {actions.map(action => <Button variant="ghost" key={action.label} disabled={action.disabled}
        className={action.danger ? 'settings-danger-text settings-menu-action' : 'settings-menu-action'}
        onClick={() => { ref.current.open = false; ref.current.querySelector('summary').focus(); action.run(); }}>{action.label}</Button>)}
    </div>
  </details>;
}
