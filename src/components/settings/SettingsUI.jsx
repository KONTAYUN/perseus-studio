import React from 'react';
import { Check, ChevronDown } from 'lucide-react';

export function SectionTitle({ icon, title, description }) {
  return (
    <div className="mb-5 flex items-start justify-between gap-4">
      <div className="flex items-start gap-3">
        <span className="mt-0.5 flex h-7 w-7 items-center justify-center rounded-xl bg-blue-500 text-white shadow-md shadow-blue-500/20">
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

export function ProviderCard({ children }) {
  return (
    <div className="settings-card flex min-h-[82px] items-center gap-4 rounded-2xl border border-gray-100 bg-white/80 p-4 shadow-sm transition-all duration-300 ease-apple hover:-translate-y-0.5 hover:border-blue-100 hover:bg-white hover:shadow-lg hover:shadow-blue-500/10 dark:border-white/10 dark:bg-[#2C2C2E]/75 dark:hover:border-blue-500/25">
      {children}
    </div>
  );
}

export function EmptyCard({ text }) {
  return (
    <div className="settings-card rounded-2xl border border-dashed border-gray-200 bg-white/60 p-5 text-sm text-gray-500 dark:border-white/10 dark:bg-white/5 dark:text-gray-400">
      {text}
    </div>
  );
}

export function CardActions({ children }) {
  return <div className="ml-auto flex shrink-0 items-center gap-1 self-center">{children}</div>;
}

export function IconButton({ children, title, onClick, danger = false, disabled = false }) {
  return (
    <button
      type="button"
      title={title}
      disabled={disabled}
      onClick={onClick}
      className={`rounded-lg p-2 transition-all duration-200 ease-apple active:scale-95 ${
        disabled
          ? 'cursor-not-allowed text-gray-300 dark:text-gray-600'
          : danger
            ? 'text-gray-400 hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-500/10'
            : 'text-gray-400 hover:bg-blue-50 hover:text-blue-500 dark:hover:bg-blue-500/10'
      }`}
    >
      {children}
    </button>
  );
}

export function Badge({ children, tone }) {
  const styles = {
    blue: 'bg-blue-50 text-blue-600 dark:bg-blue-500/10 dark:text-blue-300',
    green: 'bg-emerald-50 text-emerald-600 dark:bg-emerald-500/10 dark:text-emerald-300',
    gray: 'bg-gray-100 text-gray-500 dark:bg-white/10 dark:text-gray-400',
    red: 'bg-red-50 text-red-600 dark:bg-red-500/10 dark:text-red-300',
    yellow: 'bg-amber-50 text-amber-600 dark:bg-amber-500/10 dark:text-amber-300',
  };
  return <span className={`rounded-full px-2 py-0.5 text-[10px] font-bold ${styles[tone] || styles.gray}`}>{children}</span>;
}

export function FormHeader({ title, onReset }) {
  return (
    <div className="mb-4 flex items-center justify-between border-b border-gray-200/70 pb-3 dark:border-white/10">
      <div className="text-sm font-bold text-gray-800 dark:text-gray-100">{title}</div>
      {onReset && (
        <button
          type="button"
          onClick={onReset}
          className="text-xs font-semibold text-gray-400 hover:text-blue-500"
        >
          重置
        </button>
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

export function Input(props) {
  return (
    <input
      {...props}
      className="h-10 w-full rounded-xl border border-gray-200 bg-white px-3 text-sm text-gray-800 outline-none transition-all focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 dark:border-gray-700 dark:bg-[#1C1C1E] dark:text-white"
    />
  );
}

export function Select(props) {
  const { value, onChange, children, disabled = false, ...rest } = props;
  const [open, setOpen] = React.useState(false);
  const rootRef = React.useRef(null);
  const options = React.Children.toArray(children)
    .filter((child) => React.isValidElement(child))
    .map((child) => ({
      value: child.props.value,
      label: child.props.children,
      disabled: child.props.disabled,
    }));
  const selected = options.find((item) => item.value === value) || options[0];

  React.useEffect(() => {
    if (!open) return undefined;
    const close = (event) => {
      if (rootRef.current?.contains(event.target)) return;
      setOpen(false);
    };
    const closeOnEscape = (event) => {
      if (event.key === 'Escape') setOpen(false);
    };
    document.addEventListener('mousedown', close);
    document.addEventListener('keydown', closeOnEscape);
    return () => {
      document.removeEventListener('mousedown', close);
      document.removeEventListener('keydown', closeOnEscape);
    };
  }, [open]);

  const choose = (option) => {
    if (option.disabled) return;
    onChange?.({ target: { value: option.value } });
    setOpen(false);
  };

  return (
    <div ref={rootRef} className="relative">
      <button
        {...rest}
        type="button"
        disabled={disabled}
        onClick={() => setOpen((current) => !current)}
      className={`flex h-10 w-full items-center justify-between gap-3 rounded-xl border border-gray-200 bg-white px-3 pl-3.5 pr-3 text-left text-sm text-gray-800 outline-none transition-all focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 disabled:cursor-not-allowed disabled:opacity-60 dark:border-gray-700 dark:bg-[#1C1C1E] dark:text-white ${
          open ? 'border-blue-500 ring-2 ring-blue-500/20' : ''
        }`}
      >
        <span className="min-w-0 truncate">{selected?.label || ''}</span>
        <ChevronDown size={15} className={`shrink-0 text-gray-500 transition-transform dark:text-gray-300 ${open ? 'rotate-180' : ''}`} />
      </button>
      {open && (
        <div className="settings-popover-enter absolute left-0 right-0 top-[calc(100%+6px)] z-[240] overflow-hidden rounded-xl border border-gray-200 bg-white p-1 shadow-2xl shadow-black/10 dark:border-white/10 dark:bg-[#252527] dark:shadow-black/40">
          {options.map((option) => (
            <button
              key={option.value}
              type="button"
              disabled={option.disabled}
              onClick={() => choose(option)}
              className={`block w-full rounded-lg px-3 py-2 text-left text-sm font-semibold transition-colors disabled:cursor-not-allowed disabled:opacity-40 ${
                option.value === value
                  ? 'bg-blue-500 text-white'
                  : 'text-gray-700 hover:bg-blue-50 hover:text-blue-600 dark:text-gray-200 dark:hover:bg-blue-500/15 dark:hover:text-blue-200'
              }`}
            >
              {option.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

export function Checkbox({ label, checked, onChange }) {
  return (
    <button
      type="button"
      onClick={() => onChange(!checked)}
      className="flex items-center gap-2 rounded-xl px-2 py-1.5 text-xs font-bold text-gray-600 transition-all duration-200 ease-apple hover:-translate-y-0.5 hover:bg-white active:scale-95 dark:text-gray-300 dark:hover:bg-white/5"
    >
      <span
        className={`flex h-4 w-4 items-center justify-center rounded-md border ${
          checked ? 'border-blue-500 bg-blue-500 text-white' : 'border-gray-300 dark:border-gray-600'
        }`}
      >
        {checked && <Check size={12} />}
      </span>
      {label}
    </button>
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
