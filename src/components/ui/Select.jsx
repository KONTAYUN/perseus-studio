import React from 'react';
import { ChevronDown } from 'lucide-react';
import { AnchoredPopover } from './AnchoredPopover';
import { cn } from './cn';

export function Select({ value, onChange, children, disabled = false, className = '', ...rest }) {
  const [open, setOpen] = React.useState(false);
  const buttonRef = React.useRef(null);
  const options = React.Children.toArray(children)
    .filter((child) => React.isValidElement(child))
    .map((child) => ({
      value: child.props.value,
      label: child.props.children,
      disabled: child.props.disabled,
    }));
  const selected = options.find((item) => item.value === value) || options[0];
  const estimatedPopoverHeight = Math.min(280, Math.max(44, options.length * 38 + 8));

  const choose = (option) => {
    if (option.disabled) return;
    onChange?.({ target: { value: option.value } });
    setOpen(false);
  };

  return (
    <div className={cn('relative', className)}>
      <button
        ref={buttonRef}
        {...rest}
        type="button"
        disabled={disabled}
        onClick={() => setOpen((current) => !current)}
        className={cn(
          'motion-button flex h-10 w-full items-center justify-between gap-3 rounded-xl border border-gray-200 bg-white px-3 pl-3.5 pr-3 text-left text-sm text-gray-800 outline-none focus-visible:ring-2 focus-visible:ring-blue-500/20 disabled:cursor-not-allowed disabled:opacity-60 dark:border-gray-700 dark:bg-[#1C1C1E] dark:text-white',
          open && 'border-blue-500 ring-2 ring-blue-500/20'
        )}
      >
        <span className="min-w-0 truncate">{selected?.label || ''}</span>
        <ChevronDown size={15} className={cn('motion-rotate shrink-0 text-gray-500 dark:text-gray-300', open && 'rotate-180')} />
      </button>
      <AnchoredPopover
        anchorRef={buttonRef}
        align="start"
        estimatedHeight={estimatedPopoverHeight}
        gap={6}
        matchAnchorWidth
        maxHeight={280}
        minWidth={160}
        open={open}
        onClose={() => setOpen(false)}
        className="settings-popover-enter p-1"
      >
        {options.map((option) => (
          <button
            key={option.value}
            type="button"
            disabled={option.disabled}
            onClick={() => choose(option)}
            className={cn(
              'motion-row block w-full rounded-lg px-3 py-2 text-left text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-40',
              option.value === value
                ? 'bg-blue-500 text-white'
                : 'text-gray-700 hover:bg-blue-50 hover:text-blue-600 dark:text-gray-200 dark:hover:bg-blue-500/15 dark:hover:text-blue-200'
            )}
          >
            {option.label}
          </button>
        ))}
      </AnchoredPopover>
    </div>
  );
}
