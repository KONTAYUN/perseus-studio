import React from 'react';
import { cn } from './cn';

const iconButtonSizes = {
  compact: 'h-5 w-5 ui-icon-button-compact',
  xs: 'h-7 w-7',
  sm: 'h-8 w-8',
  md: 'h-9 w-9',
};

export const IconButton = React.forwardRef(function IconButton({
  children,
  title,
  label,
  className = '',
  variant = 'default',
  size = 'sm',
  type = 'button',
  active = false,
  danger = false,
  ...props
}, ref) {
  const tone =
    danger || variant === 'danger'
      ? 'ui-icon-button-danger'
      : variant === 'warning'
        ? 'ui-icon-button-warning-state'
        : variant === 'dangerState'
          ? 'ui-icon-button-danger-state'
          : '';
  return (
    <button
      ref={ref}
      type={type}
      title={title}
      aria-label={label || title}
      className={cn(
        'motion-button ui-icon-button focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/30',
        iconButtonSizes[size] || iconButtonSizes.sm,
        (active || variant === 'active') && 'ui-icon-button-active',
        tone,
        className
      )}
      {...props}
    >
      {children}
    </button>
  );
});
