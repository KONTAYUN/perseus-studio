import React from 'react';
import { cn } from './cn';

export function DialogActions({ children, className = '' }) {
  return <div className={cn('flex justify-end gap-2 border-t border-gray-100 bg-gray-50/70 px-5 py-4 dark:border-white/10 dark:bg-[#252526]/70', className)}>{children}</div>;
}

export function SettingsSection({ children, className = '', ...props }) {
  return (
    <section
      className={cn('settings-section', className)}
      {...props}
    >
      {children}
    </section>
  );
}

export function PanelHeader({ children, className = '', ...props }) {
  return (
    <div
      className={cn('flex h-12 flex-none items-center justify-between gap-2 border-b border-gray-100 px-3 dark:border-white/5', className)}
      {...props}
    >
      {children}
    </div>
  );
}
