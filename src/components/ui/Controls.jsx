import React from 'react';
import { Check } from 'lucide-react';
import { cn } from './cn';

export function Checkbox({ label, checked, onChange, className = '' }) {
  return (
    <button
      type="button"
      onClick={() => onChange(!checked)}
      className={cn(
        'motion-button flex items-center gap-2 rounded-xl px-2 py-1.5 text-xs font-bold text-gray-600 hover:bg-white dark:text-gray-300 dark:hover:bg-white/5',
        className
      )}
    >
      <span
        className={cn(
          'flex h-4 w-4 items-center justify-center rounded-md border',
          checked ? 'border-blue-500 bg-blue-500 text-white' : 'border-gray-300 dark:border-gray-600'
        )}
      >
        {checked && <Check size={12} />}
      </span>
      {label}
    </button>
  );
}

export function Badge({ children, tone = 'gray', className = '' }) {
  const styles = {
    blue: 'bg-blue-50 text-blue-600 dark:bg-blue-500/10 dark:text-blue-300',
    green: 'bg-emerald-50 text-emerald-600 dark:bg-emerald-500/10 dark:text-emerald-300',
    gray: 'bg-gray-100 text-gray-500 dark:bg-white/10 dark:text-gray-400',
    red: 'bg-red-50 text-red-600 dark:bg-red-500/10 dark:text-red-300',
    yellow: 'bg-amber-50 text-amber-600 dark:bg-amber-500/10 dark:text-amber-300',
  };
  return <span className={cn('rounded-full px-2 py-0.5 text-[10px] font-bold', styles[tone] || styles.gray, className)}>{children}</span>;
}

export function Switch({ checked, onChange, title, disabled = false }) {
  return (
    <button
      type="button"
      title={title}
      aria-pressed={checked}
      disabled={disabled}
      onClick={() => onChange?.(!checked)}
      className="motion-button ui-switch focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/30 disabled:cursor-not-allowed disabled:opacity-50"
      data-checked={checked ? 'true' : 'false'}
    >
      <span className="ui-switch-thumb" />
    </button>
  );
}

export function SegmentedControl({ options = [], value, onChange, className = '' }) {
  return (
    <div className={cn('ui-segmented-control', className)} role="group">
      {options.map((option) => {
        const active = option.value === value;
        return (
          <button
            key={option.value}
            type="button"
            className="motion-button ui-segmented-option"
            data-active={active ? 'true' : 'false'}
            aria-pressed={active}
            data-testid={option.testId}
            title={option.title}
            onClick={() => onChange?.(option.value)}
          >
            {option.label}
          </button>
        );
      })}
    </div>
  );
}
