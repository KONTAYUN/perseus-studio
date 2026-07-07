import React from 'react';
import {
  Badge,
  Button,
  Checkbox,
  IconButton,
  Input,
  Select,
  SegmentedControl,
  SettingsSection,
  Switch,
  cn,
} from '../ui';

export { Badge, Button, Checkbox, IconButton, Input, Select, SegmentedControl, SettingsSection, Switch };

export function SectionTitle({ icon, title, description }) {
  return (
    <div className="mb-5 flex items-start justify-between gap-4">
      <div className="flex items-start gap-3">
        <span className="mt-0.5 flex h-7 w-7 items-center justify-center rounded-xl bg-blue-500 text-white ring-1 ring-white/20">
          {icon}
        </span>
        <div>
          <h3 className="text-[15px] font-bold text-gray-800 dark:text-gray-100">{title}</h3>
          {description && (
            <p className="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{description}</p>
          )}
        </div>
      </div>
    </div>
  );
}

export function ProviderCard({ children, className = '' }) {
  return (
    <div className={cn('settings-card motion-color flex min-h-[82px] items-center gap-4 border border-gray-100 bg-white/80 p-4 hover:border-blue-100 hover:bg-white dark:border-white/10 dark:bg-[#2C2C2E]/75 dark:hover:border-blue-500/25', className)}>
      {children}
    </div>
  );
}

export function EmptyCard({ text, className = '' }) {
  return (
    <div className={cn('settings-card border border-dashed border-gray-200 bg-white/60 p-5 text-sm text-gray-500 dark:border-white/10 dark:bg-white/5 dark:text-gray-400', className)}>
      {text}
    </div>
  );
}

export function CardActions({ children, className = '' }) {
  return <div className={cn('ml-auto flex shrink-0 items-center gap-1 self-center', className)}>{children}</div>;
}

export function FormHeader({ title, onReset }) {
  return (
    <div className="mb-4 flex items-center justify-between border-b border-gray-200/70 pb-3 dark:border-white/10">
      <div className="text-sm font-bold text-gray-800 dark:text-gray-100">{title}</div>
      {onReset && (
        <Button type="button" variant="ghost" size="xs" onClick={onReset} className="px-2 py-1 text-gray-400">
          重置
        </Button>
      )}
    </div>
  );
}

export function Field({ label, children }) {
  return (
    <label className="mt-3.5 block">
      <span className="mb-1.5 block text-xs font-bold text-gray-500 dark:text-gray-400">{label}</span>
      {children}
    </label>
  );
}

export function formatDate(value) {
  if (!value) return '-';
  return new Date(value).toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  });
}
